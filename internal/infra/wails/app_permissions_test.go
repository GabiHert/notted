package wails

import (
	"context"
	"errors"
	"testing"

	"github.com/GabiHert/notted/internal/application/port"
	"github.com/GabiHert/notted/internal/application/service"
)

type stubChecker struct {
	set port.PermissionSet
	err error
}

func (s stubChecker) Status(context.Context) (port.PermissionSet, error) { return s.set, s.err }
func (s stubChecker) Ask(context.Context) (port.PermissionSet, error)    { return s.set, s.err }

type stubOpener struct{ urls *[]string }

func (s stubOpener) Open(u string) error {
	*s.urls = append(*s.urls, u)
	return nil
}

func TestPermissionBindings(t *testing.T) {
	var urls []string
	set := port.PermissionSet{Microphone: port.PermissionNotAsked, Screen: port.PermissionAllowed}
	a := New(&Emitter{}, Deps{Permissions: service.NewPermissionsService(stubChecker{set: set}, stubOpener{&urls})})
	for name, call := range map[string]func() (PermissionsDTO, error){"get": a.GetPermissions, "ask": a.AskPermissions} {
		got, err := call()
		if err != nil || got.Microphone != "not_asked" || got.Screen != "allowed" {
			t.Fatalf("%s = %+v, %v", name, got, err)
		}
	}
	if err := a.OpenPermissionSettings("screen"); err != nil || len(urls) != 1 {
		t.Fatalf("open = %v, %v", err, urls)
	}
	if err := a.OpenPermissionSettings("camera"); err == nil {
		t.Fatal("unknown kind must fail")
	}
}

func TestPermissionBindingErrorsAreUserFacing(t *testing.T) {
	a := New(&Emitter{}, Deps{Permissions: service.NewPermissionsService(stubChecker{err: errors.New("helper missing")}, stubOpener{new([]string)})})
	if _, err := a.GetPermissions(); err == nil || err.Error() != "helper missing" {
		t.Fatalf("err = %v", err)
	}
	if _, err := a.AskPermissions(); err == nil || err.Error() != "helper missing" {
		t.Fatalf("err = %v", err)
	}
}

type stubTester struct {
	seconds []int
	err     error
}

func (s *stubTester) Start(seconds int, _ func(port.HelperEvent)) (port.Recording, error) {
	s.seconds = append(s.seconds, seconds)
	if s.err != nil {
		return nil, s.err
	}
	return stubRun{make(chan port.RecorderExit, 1)}, nil
}

type stubRun struct{ exited chan port.RecorderExit }

func (r stubRun) Stop() error                      { r.exited <- port.RecorderExit{}; return nil }
func (r stubRun) Exited() <-chan port.RecorderExit { return r.exited }

func micTestApp(t *testing.T, mic port.PermissionState, recording bool, tester *stubTester) *App {
	t.Helper()
	perms := stubChecker{set: port.PermissionSet{Microphone: mic, Screen: port.PermissionAllowed}}
	svc := service.NewMicTestService(tester, perms, func() bool { return recording }, nopEmitter{})
	return New(&Emitter{}, Deps{MicTest: svc})
}

func TestMicTestBindingsStartAndStop(t *testing.T) {
	tester := &stubTester{}
	a := micTestApp(t, port.PermissionAllowed, false, tester)
	if err := a.StartMicTest(); err != nil || len(tester.seconds) != 1 {
		t.Fatalf("start = %v, %v", err, tester.seconds)
	}
	if err := a.StartMicTest(); err == nil || err.Error() != "The microphone test is already running." {
		t.Fatalf("second start = %v", err)
	}
	if err := a.StopMicTest(); err != nil {
		t.Fatalf("stop = %v", err)
	}
}

func TestMicTestBindingErrorsAreUserFacing(t *testing.T) {
	if err := micTestApp(t, port.PermissionAllowed, true, &stubTester{}).StartMicTest(); err == nil || err.Error() != "Stop the recording to test the microphone." {
		t.Fatalf("while recording = %v", err)
	}
	if err := micTestApp(t, port.PermissionDenied, false, &stubTester{}).StartMicTest(); err == nil {
		t.Fatal("a denied microphone must refuse the test")
	}
	if err := micTestApp(t, port.PermissionAllowed, false, &stubTester{err: errors.New("helper missing")}).StartMicTest(); err == nil || err.Error() != "helper missing" {
		t.Fatalf("helper failure = %v", err)
	}
}
