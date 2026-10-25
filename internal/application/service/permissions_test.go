package service

import (
	"context"
	"errors"
	"testing"

	"github.com/GabiHert/notted/internal/application/port"
)

type fakeChecker struct {
	status, asked port.PermissionSet
	err           error
	calls         []string
}

func (f *fakeChecker) Status(context.Context) (port.PermissionSet, error) {
	f.calls = append(f.calls, "status")
	return f.status, f.err
}

func (f *fakeChecker) Ask(context.Context) (port.PermissionSet, error) {
	f.calls = append(f.calls, "ask")
	return f.asked, f.err
}

type fakeOpener struct {
	urls []string
	err  error
}

func (f *fakeOpener) Open(url string) error {
	f.urls = append(f.urls, url)
	return f.err
}

func TestPermissionsGetNeverAsks(t *testing.T) {
	c := &fakeChecker{status: port.PermissionSet{Microphone: port.PermissionNotAsked, Screen: port.PermissionDenied}}
	svc := NewPermissionsService(c, &fakeOpener{})
	got, err := svc.Get(context.Background())
	if err != nil || got != c.status {
		t.Fatalf("Get = %+v, %v", got, err)
	}
	if len(c.calls) != 1 || c.calls[0] != "status" {
		t.Fatalf("calls = %v", c.calls)
	}
}

func TestPermissionsAskRunsTheAskingCheck(t *testing.T) {
	c := &fakeChecker{asked: port.PermissionSet{Microphone: port.PermissionAllowed, Screen: port.PermissionAllowed}}
	got, err := NewPermissionsService(c, &fakeOpener{}).Ask(context.Background())
	if err != nil || got != c.asked || len(c.calls) != 1 || c.calls[0] != "ask" {
		t.Fatalf("Ask = %+v, %v, calls %v", got, err, c.calls)
	}
}

func TestPermissionsErrorsPassThrough(t *testing.T) {
	c := &fakeChecker{err: errors.New("helper missing")}
	svc := NewPermissionsService(c, &fakeOpener{})
	if _, err := svc.Get(context.Background()); err == nil || err.Error() != "helper missing" {
		t.Fatalf("Get err = %v", err)
	}
	if _, err := svc.Ask(context.Background()); err == nil || err.Error() != "helper missing" {
		t.Fatalf("Ask err = %v", err)
	}
}

func TestPermissionsOpenSettingsMapsTheKindToThePrivacyPane(t *testing.T) {
	o := &fakeOpener{}
	svc := NewPermissionsService(&fakeChecker{}, o)
	if err := svc.OpenSettings("microphone"); err != nil {
		t.Fatal(err)
	}
	if err := svc.OpenSettings("screen"); err != nil {
		t.Fatal(err)
	}
	want := []string{
		"x-apple.systempreferences:com.apple.preference.security?Privacy_Microphone",
		"x-apple.systempreferences:com.apple.preference.security?Privacy_ScreenCapture",
	}
	if len(o.urls) != 2 || o.urls[0] != want[0] || o.urls[1] != want[1] {
		t.Fatalf("urls = %v", o.urls)
	}
}

func TestPermissionsOpenSettingsRejectsUnknownKindsAndSurfacesOpenFailures(t *testing.T) {
	o := &fakeOpener{}
	svc := NewPermissionsService(&fakeChecker{}, o)
	if err := svc.OpenSettings("camera"); err == nil || err.Error() != `Unknown permission "camera". Use microphone or screen.` {
		t.Fatalf("err = %v", err)
	}
	if len(o.urls) != 0 {
		t.Fatal("an unknown kind must open nothing")
	}
	o.err = errors.New("cannot open")
	if err := svc.OpenSettings("screen"); err == nil || err.Error() != "cannot open" {
		t.Fatalf("err = %v", err)
	}
}
