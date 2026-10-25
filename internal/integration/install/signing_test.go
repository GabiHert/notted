package install

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func fakeSecurity(t *testing.T, body string) (tool, argsFile string) {
	t.Helper()
	dir := t.TempDir()
	argsFile = filepath.Join(dir, "args.txt")
	tool = filepath.Join(dir, "security")
	src := "#!/bin/bash\necho \"$*\" > '" + argsFile + "'\n" + body + "\n"
	if err := os.WriteFile(tool, []byte(src), 0o755); err != nil {
		t.Fatal(err)
	}
	return tool, argsFile
}

func TestSigningPresentWhenListed(t *testing.T) {
	tool, argsFile := fakeSecurity(t, `echo '  1) 13696039679D3B35D7E756B113E7576F4DE421A7 "Notted Local Signing"'`)
	if !(Signing{Security: tool}).Present(context.Background()) {
		t.Fatal("Present = false, want true")
	}
	got, err := os.ReadFile(argsFile)
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(string(got)) != "find-identity -v -p codesigning" {
		t.Fatalf("arguments = %q", got)
	}
}

func TestSigningAbsentWhenNotListed(t *testing.T) {
	tool, _ := fakeSecurity(t, `echo '     0 valid identities found'`)
	if (Signing{Security: tool}).Present(context.Background()) {
		t.Fatal("Present = true, want false")
	}
}

func TestSigningFalseOnCancel(t *testing.T) {
	tool, _ := fakeSecurity(t, `exec /bin/sleep 10`)
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	start := time.Now()
	if (Signing{Security: tool}).Present(ctx) {
		t.Fatal("Present = true, want false")
	}
	if d := time.Since(start); d > time.Second {
		t.Fatalf("took %v, want under 1s", d)
	}
}

func TestSigningFalseWhenToolMissing(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "nope")
	if (Signing{Security: missing}).Present(context.Background()) {
		t.Fatal("Present = true, want false")
	}
}
