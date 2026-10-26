package wails

import (
	"context"
	"log"
	"sync"

	"github.com/GabiHert/notted/internal/application/port"
	wruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

var _ port.Notifier = (*Notifier)(nil)

// Notifier shows macOS notifications through the Wails runtime.
type Notifier struct {
	emitter *Emitter
	emit    port.EventEmitter
	show    func()

	initialize      func(ctx context.Context) error
	checkAuth       func(ctx context.Context) (bool, error)
	requestAuth     func(ctx context.Context) (bool, error)
	send            func(ctx context.Context, o wruntime.NotificationOptions) error
	removeDelivered func(ctx context.Context, id string) error
	removePending   func(ctx context.Context, id string) error
	onResponse      func(ctx context.Context, cb func(wruntime.NotificationResult))

	mu        sync.Mutex // guards on, inflight and cancelled; never held while waiting on the prompt
	on        bool
	inflight  map[string]bool // ids of Notify calls that have not yet sent or given up
	cancelled map[string]bool // in-flight ids whose form closed before the send

	authMu   sync.Mutex // serializes the authorization prompt
	authDone bool
	allowed  bool
}

// NewNotifier builds a notifier that stays off until Start succeeds.
func NewNotifier(e *Emitter) *Notifier {
	return &Notifier{
		emitter:         e,
		emit:            e,
		show:            e.Show,
		initialize:      wruntime.InitializeNotifications,
		checkAuth:       wruntime.CheckNotificationAuthorization,
		requestAuth:     wruntime.RequestNotificationAuthorization,
		send:            wruntime.SendNotification,
		removeDelivered: wruntime.RemoveDeliveredNotification,
		removePending:   wruntime.RemovePendingNotification,
		onResponse:      wruntime.OnNotificationResponse,
		inflight:        map[string]bool{},
		cancelled:       map[string]bool{},
	}
}

// Start turns notifications on; it needs the emitter's context, so call it after SetContext.
func (n *Notifier) Start() {
	ctx := n.emitter.Context()
	if err := n.initialize(ctx); err != nil {
		log.Printf("notifications: %v", err)
		return
	}
	n.onResponse(ctx, func(result wruntime.NotificationResult) {
		if result.Error != nil {
			log.Printf("notifications: %v", result.Error)
			return
		}
		id, _ := result.Response.UserInfo["meetingId"].(string)
		if id == "" {
			return
		}
		n.show()
		n.emit.Emit("ask:open", map[string]any{"meetingId": id})
	})
	n.mu.Lock()
	n.on = true
	n.mu.Unlock()
}

func (n *Notifier) authorized(ctx context.Context) bool {
	n.authMu.Lock()
	defer n.authMu.Unlock()
	if n.authDone {
		return n.allowed
	}
	n.authDone = true
	ok, err := n.checkAuth(ctx)
	if err == nil && !ok {
		ok, err = n.requestAuth(ctx)
	}
	if err != nil {
		log.Printf("notifications: %v", err)
	}
	n.allowed = err == nil && ok
	if !n.allowed {
		log.Printf("notifications: not allowed; none will be shown")
	}
	return n.allowed
}

func (n *Notifier) isOn() bool {
	n.mu.Lock()
	defer n.mu.Unlock()
	return n.on
}

// Notify shows one notification; it does nothing when notifications are off or refused,
// or when Remove was called for the same id while the authorization prompt was open.
func (n *Notifier) Notify(p port.Notification) error {
	if !n.isOn() {
		return nil
	}
	ctx := n.emitter.Context()
	n.mu.Lock()
	n.inflight[p.ID] = true
	n.mu.Unlock()

	allowed := n.authorized(ctx)

	n.mu.Lock()
	defer n.mu.Unlock()
	skip := n.cancelled[p.ID] || !allowed
	delete(n.inflight, p.ID)
	delete(n.cancelled, p.ID)
	if skip {
		return nil
	}
	return n.send(ctx, wruntime.NotificationOptions{
		ID: p.ID, Title: p.Title, Subtitle: p.Subtitle, Body: p.Body,
		Data: map[string]interface{}{"meetingId": p.MeetingID},
	})
}

// Remove takes a notification away, delivered or pending. It never waits for the authorization prompt.
func (n *Notifier) Remove(id string) {
	n.mu.Lock()
	if !n.on {
		n.mu.Unlock()
		return
	}
	if n.inflight[id] {
		n.cancelled[id] = true
	}
	n.mu.Unlock()
	ctx := n.emitter.Context()
	_ = n.removeDelivered(ctx, id)
	_ = n.removePending(ctx, id)
}
