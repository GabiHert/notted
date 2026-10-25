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
