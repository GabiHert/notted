package wails

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/GabiHert/notted/internal/application/port"
	wruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

type notifierRig struct {
	n         *Notifier
	initErr   error
	checked   bool
	checkRes  bool
	reqRes    bool
	checks    int
	requests  int
	sent      []wruntime.NotificationOptions
	delivered []string
	pending   []string
	callback  func(wruntime.NotificationResult)
}

func newNotifierRig(checkRes, reqRes bool) *notifierRig {
	r := &notifierRig{checkRes: checkRes, reqRes: reqRes}
	r.n = NewNotifier(&Emitter{})
	r.n.initialize = func(context.Context) error { return r.initErr }
	r.n.checkAuth = func(context.Context) (bool, error) { r.checks++; return r.checkRes, nil }
	r.n.requestAuth = func(context.Context) (bool, error) { r.requests++; return r.reqRes, nil }
	r.n.send = func(_ context.Context, o wruntime.NotificationOptions) error { r.sent = append(r.sent, o); return nil }
	r.n.removeDelivered = func(_ context.Context, id string) error { r.delivered = append(r.delivered, id); return nil }
	r.n.removePending = func(_ context.Context, id string) error { r.pending = append(r.pending, id); return nil }
	r.n.onResponse = func(_ context.Context, cb func(wruntime.NotificationResult)) { r.callback = cb }
	return r
}

var sample = port.Notification{ID: "ask-f1", Title: "Waiting for you", Subtitle: "Weekly", Body: "Pick one", MeetingID: "m1"}

func TestNotifierOffAfterInitError(t *testing.T) {
	r := newNotifierRig(true, true)
	r.initErr = errors.New("no bundle")
	r.n.Start()
	if err := r.n.Notify(sample); err != nil || len(r.sent) != 0 || r.checks != 0 {
		t.Fatalf("%v %+v", err, r.sent)
	}
	if r.callback != nil {
		t.Fatal("callback registered while off")
	}
}

func TestNotifierBeforeStartSendsNothing(t *testing.T) {
	r := newNotifierRig(true, true)
	if err := r.n.Notify(sample); err != nil || len(r.sent) != 0 {
		t.Fatalf("%v %+v", err, r.sent)
	}
}

func TestNotifierAsksAuthorizationOnce(t *testing.T) {
	r := newNotifierRig(false, true)
	r.n.Start()
	for i := 0; i < 3; i++ {
		if err := r.n.Notify(sample); err != nil {
			t.Fatal(err)
		}
	}
	if r.checks != 1 || r.requests != 1 || len(r.sent) != 3 {
		t.Fatalf("checks %d requests %d sent %d", r.checks, r.requests, len(r.sent))
	}
}

func TestNotifierAlreadyAuthorizedDoesNotRequest(t *testing.T) {
	r := newNotifierRig(true, false)
	r.n.Start()
	_ = r.n.Notify(sample)
	if r.requests != 0 || len(r.sent) != 1 {
		t.Fatalf("%d %d", r.requests, len(r.sent))
	}
}

func TestNotifierRefusalSendsNothing(t *testing.T) {
	r := newNotifierRig(false, false)
	r.n.Start()
	for i := 0; i < 2; i++ {
		if err := r.n.Notify(sample); err != nil {
			t.Fatal(err)
		}
	}
	if len(r.sent) != 0 || r.requests != 1 {
		t.Fatalf("%d %d", len(r.sent), r.requests)
	}
}

func TestNotifierOptions(t *testing.T) {
	r := newNotifierRig(true, true)
	r.n.Start()
	if err := r.n.Notify(sample); err != nil {
		t.Fatal(err)
	}
	o := r.sent[0]
	if o.ID != "ask-f1" || o.Title != "Waiting for you" || o.Subtitle != "Weekly" || o.Body != "Pick one" || o.Data["meetingId"] != "m1" {
		t.Fatalf("%+v", o)
	}
}

func TestNotifierRemoveCallsBoth(t *testing.T) {
	r := newNotifierRig(true, true)
	r.n.Start()
	r.n.Remove("ask-f1")
	if len(r.delivered) != 1 || r.delivered[0] != "ask-f1" || len(r.pending) != 1 || r.pending[0] != "ask-f1" {
		t.Fatalf("%v %v", r.delivered, r.pending)
	}
}

type captureEmit struct {
	names    []string
	payloads []any
}

func (c *captureEmit) Emit(name string, p any) {
	c.names = append(c.names, name)
	c.payloads = append(c.payloads, p)
}

func TestNotifierResponseShowsWindowAndEmits(t *testing.T) {
	r := newNotifierRig(true, true)
	em := &captureEmit{}
	shown := 0
	r.n.emit = em
	r.n.show = func() { shown++ }
	r.n.Start()
	if r.callback == nil {
		t.Fatal("no callback")
	}
	r.callback(wruntime.NotificationResult{Response: wruntime.NotificationResponse{UserInfo: map[string]interface{}{"meetingId": "m1"}}})
	if shown != 1 || len(em.names) != 1 || em.names[0] != "ask:open" {
		t.Fatalf("%d %v", shown, em.names)
	}
	if p, _ := em.payloads[0].(map[string]any); p["meetingId"] != "m1" {
		t.Fatalf("%+v", em.payloads[0])
	}
}

func TestNotifierResponseWithoutMeetingOrWithErrorDoesNothing(t *testing.T) {
	r := newNotifierRig(true, true)
	em := &captureEmit{}
	shown := 0
	r.n.emit = em
	r.n.show = func() { shown++ }
	r.n.Start()
	r.callback(wruntime.NotificationResult{Response: wruntime.NotificationResponse{UserInfo: map[string]interface{}{}}})
	r.callback(wruntime.NotificationResult{Response: wruntime.NotificationResponse{UserInfo: map[string]interface{}{"meetingId": ""}}})
	r.callback(wruntime.NotificationResult{Error: errors.New("boom")})
	if shown != 0 || len(em.names) != 0 {
		t.Fatalf("%d %v", shown, em.names)
	}
}

// blockAuth makes requestAuth wait until release is closed; started is closed when it is entered.
func blockAuth(r *notifierRig, grant bool) (started, release chan struct{}) {
	started, release = make(chan struct{}), make(chan struct{})
	r.n.requestAuth = func(context.Context) (bool, error) {
		close(started)
		<-release
		return grant, nil
	}
	return started, release
}

func removeWithin(t *testing.T, n *Notifier, id string, d time.Duration) {
	t.Helper()
	done := make(chan struct{})
	go func() { n.Remove(id); close(done) }()
	select {
	case <-done:
	case <-time.After(d):
		t.Fatal("Remove blocked while the authorization prompt was pending")
	}
}

func TestNotifierRemoveDoesNotWaitForAuthorizationPrompt(t *testing.T) {
	r := newNotifierRig(false, true)
	started, release := blockAuth(r, true)
	t.Cleanup(func() {
		select {
		case <-release:
		default:
			close(release)
		}
	})
	r.n.Start()
	done := make(chan struct{})
	go func() { _ = r.n.Notify(sample); close(done) }()
	<-started
	removeWithin(t, r.n, "ask-other", 100*time.Millisecond)
	removeWithin(t, r.n, sample.ID, 100*time.Millisecond)
	close(release)
	<-done
}

func TestNotifierRemoveDuringPendingAuthorizationCancelsSend(t *testing.T) {
	r := newNotifierRig(false, true)
	started, release := blockAuth(r, true)
	r.n.Start()
	done := make(chan struct{})
	go func() { _ = r.n.Notify(sample); close(done) }()
	<-started
	removeWithin(t, r.n, sample.ID, 100*time.Millisecond)
	close(release)
	<-done
	if len(r.sent) != 0 {
		t.Fatalf("sent after remove: %+v", r.sent)
	}
	other := port.Notification{ID: "ask-f2", Title: "Waiting for you", MeetingID: "m2"}
	if err := r.n.Notify(other); err != nil {
		t.Fatal(err)
	}
	if len(r.sent) != 1 || r.sent[0].ID != "ask-f2" {
		t.Fatalf("%+v", r.sent)
	}
	// the cancelled id is forgotten: the same id can be sent again
	if err := r.n.Notify(sample); err != nil || len(r.sent) != 2 {
		t.Fatalf("%v %+v", err, r.sent)
	}
}
