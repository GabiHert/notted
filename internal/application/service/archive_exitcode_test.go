package service

import (
	"context"
	"strings"
	"testing"
)

type exitErr struct{ code int }

func (e exitErr) Error() string { return "exit" }
func (e exitErr) ExitCode() int { return e.code }

type scriptedGit struct {
	diffCode int
	calls    []string
}

func (g *scriptedGit) Run(_ context.Context, _ string, args ...string) (string, error) {
	g.calls = append(g.calls, args[0])
	switch args[0] {
	case "diff":
		if g.diffCode == 0 {
			return "", nil
		}
		return "nada adicionado ao commit\n", exitErr{g.diffCode}
	case "commit":
		return "nada adicionado ao commit\n", nil
	case "rev-parse":
		return "abc1234\n", nil
	}
	return "", nil
}

func (g *scriptedGit) did(name string) bool {
	for _, c := range g.calls {
		if c == name {
			return true
		}
	}
	return false
}

func TestCommitDecidesByExitCodeNotText(t *testing.T) {
	for _, tc := range []struct {
		code       int
		wantCommit bool
		wantErr    string
	}{
		{0, false, "Nothing changed in "},
		{1, true, ""},
		{128, false, "The commit was rejected:\n"},
	} {
		f := newArchFixture(t)
		if _, err := f.svc.ExportMeeting(f.ctx, "m1"); err != nil {
			t.Fatal(err)
		}
		g := &scriptedGit{diffCode: tc.code}
		f.svc.git = g
		_, err := f.svc.CommitAndPushMeeting(f.ctx, "m1")
		if g.did("commit") != tc.wantCommit {
			t.Fatalf("code %d: commit ran = %v", tc.code, g.did("commit"))
		}
		if tc.wantErr == "" && err != nil || tc.wantErr != "" && (err == nil || !strings.HasPrefix(err.Error(), tc.wantErr)) {
			t.Fatalf("code %d: err %v", tc.code, err)
		}
	}
}
