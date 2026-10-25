package audio

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/GabiHert/notted/internal/application/port"
)

func permRig(t *testing.T, mode string) (*Permissions, string) {
	t.Helper()
	args := filepath.Join(t.TempDir(), "args")
	t.Setenv("FAKE_AUDIOCAP_MODE", mode)
	t.Setenv("FAKE_AUDIOCAP_ARGS", args)
	return NewPermissions(filepath.Join("testdata", "fake-audiocap.sh")), args
}

func TestStatusRunsTheStatusSubcommandAndReadsBothPermissions(t *testing.T) {
	cases := []struct {
		mode string
		want port.PermissionSet
	}{
		{"perm-allowed", port.PermissionSet{Microphone: port.PermissionAllowed, Screen: port.PermissionAllowed}},
		{"perm-not-asked", port.PermissionSet{Microphone: port.PermissionNotAsked, Screen: port.PermissionDenied}},
		{"perm-denied", port.PermissionSet{Microphone: port.PermissionDenied, Screen: port.PermissionDenied}},
		{"perm-legacy", port.PermissionSet{Microphone: port.PermissionAllowed, Screen: port.PermissionDenied}},
	}
	for _, c := range cases {
		t.Run(c.mode, func(t *testing.T) {
			p, args := permRig(t, c.mode)
			got, err := p.Status(context.Background())
			if err != nil || got != c.want {
				t.Fatalf("Status = %+v, %v; want %+v", got, err, c.want)
			}
			b, _ := os.ReadFile(args)
			if strings.TrimSpace(string(b)) != "status" {
				t.Fatalf("argv = %q, want status", b)
			}
		})
	}
}

func TestAskRunsCheckAndKeepsTheAnswerWhenTheHelperExitsNonZero(t *testing.T) {
	p, args := permRig(t, "perm-denied")
	got, err := p.Ask(context.Background())
	if err != nil || got.Microphone != port.PermissionDenied || got.Screen != port.PermissionDenied {
		t.Fatalf("Ask = %+v, %v", got, err)
	}
	if b, _ := os.ReadFile(args); strings.TrimSpace(string(b)) != "check" {
		t.Fatalf("argv = %q, want check", b)
	}
}

func TestPermissionFailuresAreUserFacing(t *testing.T) {
	for _, mode := range []string{"perm-empty", "perm-partial", "perm-garbage", "perm-crash", "normal"} {
		t.Run(mode, func(t *testing.T) {
			p, _ := permRig(t, mode)
			_, err := p.Status(context.Background())
			if err == nil || err.Error() != msgPermissionsUnavailable {
				t.Fatalf("err = %v, want %q", err, msgPermissionsUnavailable)
			}
		})
	}
}

func TestMissingHelperIsUserFacing(t *testing.T) {
	p := NewPermissions(filepath.Join(t.TempDir(), "nope"))
	if _, err := p.Status(context.Background()); err == nil || err.Error() != msgPermissionsUnavailable {
		t.Fatalf("err = %v", err)
	}
}

func TestStatusTimesOut(t *testing.T) {
	p, _ := permRig(t, "perm-hang")
	p.statusTimeout = 100 * time.Millisecond
	start := time.Now()
	if _, err := p.Status(context.Background()); err == nil || err.Error() != msgPermissionsUnavailable {
		t.Fatalf("err = %v", err)
	}
	if time.Since(start) > 3*time.Second {
		t.Fatal("did not stop the helper at the timeout")
	}
}

func TestParsePermissions(t *testing.T) {
	line := func(name string, granted bool, status string) string {
		s := `{"event":"permission","name":"` + name + `","granted":false`
		if granted {
			s = strings.Replace(s, "false", "true", 1)
		}
		if status != "" {
			s += `,"status":"` + status + `"`
		}
		return s + "}\n"
	}
	got, ok := parsePermissions([]byte(`{"event":"started"}` + "\n" + line("microphone", false, "not_asked") + line("screen", true, "")))
	if !ok || got.Microphone != port.PermissionNotAsked || got.Screen != port.PermissionAllowed {
		t.Fatalf("got %+v ok=%v", got, ok)
	}
	if _, ok := parsePermissions([]byte(line("microphone", true, "allowed"))); ok {
		t.Fatal("one permission is not an answer")
	}
	if _, ok := parsePermissions([]byte(line("camera", true, "allowed") + line("screen", true, "allowed"))); ok {
		t.Fatal("unknown names are ignored")
	}
}
