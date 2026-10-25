package claude

import (
	"os"
	"path/filepath"
	"testing"
)

func copyExec(t *testing.T, src, dst string) {
	t.Helper()
	data, err := os.ReadFile(src)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dst, data, 0o755); err != nil {
		t.Fatal(err)
	}
}

// fixture builds a checkout with a stub bin/meeting and a bin dir holding a stub claude.
func fixture(t *testing.T) (checkout, binDir, out string) {
	t.Helper()
	root := t.TempDir()
	checkout = filepath.Join(root, "checkout")
	binDir = filepath.Join(root, "bin")
	out = filepath.Join(root, "out")
	copyExec(t, "testdata/stub-meeting.sh", filepath.Join(checkout, "bin", "meeting"))
	copyExec(t, "testdata/stub-claude.sh", filepath.Join(binDir, "claude"))
	return
}

func read(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}
