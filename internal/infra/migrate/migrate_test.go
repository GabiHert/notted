package migrate

import (
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/GabiHert/phonon/internal/infra/paths"
	"github.com/GabiHert/phonon/internal/infra/store"
)

const meetingID = "d4c5d825-30e1-44be-85af-ab869d5eaf9f"

// world is a temp home with a Notted data folder like the real one.
type world struct {
	root, old, new, archive string
}

func newWorld(t *testing.T) world {
	t.Helper()
	root := t.TempDir()
	w := world{root: root, old: filepath.Join(root, ".notted"), new: filepath.Join(root, ".phonon"), archive: filepath.Join(root, "Documents", "meetings")}
	st, err := store.Open(filepath.Join(w.old, "meetings.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	if err := st.Close(); err != nil {
		t.Fatal(err)
	}
	db := openDB(t, filepath.Join(w.old, "meetings.sqlite"))
	execSQL(t, db, `INSERT INTO meetings (id, title, started_at, language, claude_session_id, folder, state)
		VALUES (?, 'Weekly', '2026-10-30T11:00:00Z', 'pt-BR', 'sess-1', 'meetings/acme/2026/weekly', 'ready')`, meetingID)
	for _, track := range []string{"mic", "others"} {
		execSQL(t, db, `INSERT INTO chunks (meeting_id, track, idx, start_ms, end_ms, path) VALUES (?, ?, 1, 0, 30000, ?)`,
			meetingID, track, filepath.Join(w.old, "audio", meetingID, track+"-000001.wav"))
	}
	execSQL(t, db, `INSERT INTO segments (meeting_id, track, start_ms, end_ms, text) VALUES (?, 'mic', 0, 1000, 'hello')`, meetingID)
	execSQL(t, db, `INSERT INTO settings (key, value) VALUES ('checkout_path', ?), ('source_path', ?), ('profile', 'gabriel')`,
		w.archive, filepath.Join(root, "code", "notted"))
	db.Close()
	put(t, filepath.Join(w.old, "audio", meetingID, "mic-000001.wav"), "RIFF-mic")
	put(t, filepath.Join(w.old, "audio", meetingID, "others-000001.wav"), "RIFF-others")
	put(t, filepath.Join(w.old, "models", "silero_vad", "silero_vad.onnx"), "model")
	put(t, filepath.Join(w.old, "install-source"), filepath.Join(root, "code", "notted")+"\n")
	put(t, filepath.Join(w.old, "sessions", meetingID, "mcp.json"),
		`{"mcpServers":{"notted":{"command":"/Applications/Notted.app/Contents/MacOS/Notted","args":["mcp","--socket","`+filepath.Join(w.old, "run", "ask.sock")+`"]}}}`+"\n")
	if err := os.MkdirAll(filepath.Join(w.old, "run"), 0o700); err != nil {
		t.Fatal(err)
	}
	return w
}

func (w world) plan(running bool) Plan {
	return Plan{OldHome: w.old, NewHome: w.new, DBFile: "meetings.sqlite", Running: func() bool { return running }}
}

func openDB(t *testing.T, path string) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite3", path+"?_journal_mode=WAL&_busy_timeout=5000")
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	return db
}

func execSQL(t *testing.T, db *sql.DB, q string, args ...any) {
	t.Helper()
	if _, err := db.Exec(q, args...); err != nil {
		t.Fatalf("%s: %v", q, err)
	}
}

func put(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func read(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func absent(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Lstat(path); !os.IsNotExist(err) {
		t.Fatalf("%s exists (%v)", path, err)
	}
}

// snapshot lists every file under dir with its content, to prove a folder did not change.
func snapshot(t *testing.T, dir string) map[string]string {
	t.Helper()
	out := map[string]string{}
	err := filepath.Walk(dir, func(p string, fi os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(dir, p)
		if fi.IsDir() {
			out[rel+"/"] = ""
			return nil
		}
		b, err := os.ReadFile(p)
		out[rel] = string(b)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func sameSnapshot(t *testing.T, want, got map[string]string) {
	t.Helper()
	if len(want) != len(got) {
		t.Fatalf("snapshot has %d entries, want %d", len(got), len(want))
	}
	for k, v := range want {
		if got[k] != v {
			t.Fatalf("%s changed", k)
		}
	}
}

func TestRunCopiesTheFolderAndRewritesItsPaths(t *testing.T) {
	w := newWorld(t)
	before := snapshot(t, w.old)
	res, err := w.plan(false).Run()
	if err != nil {
		t.Fatal(err)
	}
	if !res.Copied {
		t.Fatal("Copied = false")
	}
	// Notted's folder is never touched, so Notted.app keeps its meetings.
	sameSnapshot(t, before, snapshot(t, w.old))
	absent(t, w.new+".migrating")
	if read(t, filepath.Join(w.new, "audio", meetingID, "mic-000001.wav")) != "RIFF-mic" {
		t.Fatal("audio lost")
	}
	if read(t, filepath.Join(w.new, "models", "silero_vad", "silero_vad.onnx")) != "model" {
		t.Fatal("models lost")
	}
	if got := read(t, filepath.Join(w.new, "install-source")); got != filepath.Join(w.root, "code", "notted")+"\n" {
		t.Fatalf("install-source %q", got)
	}

	db := openDB(t, filepath.Join(w.new, "meetings.sqlite"))
	defer db.Close()
	rows, err := db.Query(`SELECT track, path FROM chunks ORDER BY track`)
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	for rows.Next() {
		var track, p string
		if err := rows.Scan(&track, &p); err != nil {
			t.Fatal(err)
		}
		if want := filepath.Join(w.new, "audio", meetingID, track+"-000001.wav"); p != want {
			t.Fatalf("chunk path %q, want %q", p, want)
		}
		if _, err := os.Stat(p); err != nil {
			t.Fatalf("chunk file: %v", err)
		}
		n++
	}
	rows.Close()
	if n != 2 {
		t.Fatalf("%d chunks", n)
	}
	var folder, session, text string
	if err := db.QueryRow(`SELECT folder, claude_session_id FROM meetings WHERE id = ?`, meetingID).Scan(&folder, &session); err != nil {
		t.Fatal(err)
	}
	if folder != "meetings/acme/2026/weekly" || session != "sess-1" {
		t.Fatalf("folder %q session %q", folder, session)
	}
	if err := db.QueryRow(`SELECT text FROM segments`).Scan(&text); err != nil || text != "hello" {
		t.Fatalf("segment %q %v", text, err)
	}
	// The archive lives outside the data folder and stays where it is.
	var checkout string
	if err := db.QueryRow(`SELECT value FROM settings WHERE key = 'checkout_path'`).Scan(&checkout); err != nil || checkout != w.archive {
		t.Fatalf("checkout_path %q %v", checkout, err)
	}

	mcp := read(t, filepath.Join(w.new, "sessions", meetingID, "mcp.json"))
	if strings.Contains(mcp, w.old+string(filepath.Separator)) || !strings.Contains(mcp, filepath.Join(w.new, "run", "ask.sock")) {
		t.Fatalf("mcp.json not rewritten: %s", mcp)
	}
}

// Notted quit without a checkpoint: committed rows still sit in meetings.sqlite-wal.
func TestRunCopiesCommittedWALPages(t *testing.T) {
	w := newWorld(t)
	dbPath := filepath.Join(w.old, "meetings.sqlite")
	db := openDB(t, dbPath)
	execSQL(t, db, `PRAGMA wal_autocheckpoint = 0`)
	execSQL(t, db, `INSERT INTO chunks (meeting_id, track, idx, start_ms, end_ms, path) VALUES (?, 'mic', 2, 30000, 60000, ?)`,
		meetingID, filepath.Join(w.old, "audio", meetingID, "mic-000002.wav"))
	execSQL(t, db, `INSERT INTO segments (meeting_id, track, start_ms, end_ms, text) VALUES (?, 'others', 1000, 2000, 'only in the WAL')`, meetingID)
	// Keep the files as they are while the connection is open; closing it would checkpoint.
	saved := map[string][]byte{}
	for _, f := range []string{dbPath, dbPath + "-wal"} {
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		saved[f] = b
	}
	if len(saved[dbPath+"-wal"]) == 0 {
		t.Fatal("the WAL is empty")
	}
	db.Close()
	_ = os.Remove(dbPath + "-shm")
	for f, b := range saved {
		if err := os.WriteFile(f, b, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := w.plan(false).Run(); err != nil {
		t.Fatal(err)
	}
	db = openDB(t, filepath.Join(w.new, "meetings.sqlite"))
	defer db.Close()
	var n int
	if err := db.QueryRow(`SELECT count(*) FROM segments WHERE text = 'only in the WAL'`).Scan(&n); err != nil || n != 1 {
		t.Fatalf("count %d %v", n, err)
	}
	var p string
	if err := db.QueryRow(`SELECT path FROM chunks WHERE idx = 2`).Scan(&p); err != nil || p != filepath.Join(w.new, "audio", meetingID, "mic-000002.wav") {
		t.Fatalf("path %q %v", p, err)
	}
}

func TestRunRefusesWhileNottedRuns(t *testing.T) {
	w := newWorld(t)
	before := snapshot(t, w.old)
	_, err := w.plan(true).Run()
	if !errors.Is(err, ErrOldRunning) {
		t.Fatalf("err = %v, want ErrOldRunning", err)
	}
	absent(t, w.new)
	sameSnapshot(t, before, snapshot(t, w.old))
}

func TestRunLeavesAnExistingPhononFolder(t *testing.T) {
	w := newWorld(t)
	put(t, filepath.Join(w.new, "meetings.sqlite"), "phonon's own")
	before := snapshot(t, w.old)
	res, err := w.plan(true).Run() // Notted running does not matter: nothing moves
	if err != nil || res.Copied {
		t.Fatalf("res %+v err %v", res, err)
	}
	sameSnapshot(t, before, snapshot(t, w.old))
	if read(t, filepath.Join(w.new, "meetings.sqlite")) != "phonon's own" {
		t.Fatal("Phonon's folder changed")
	}
}

// An empty Phonon folder holds nothing to lose, so Notted's data still comes over.
func TestRunReplacesAnEmptyPhononFolder(t *testing.T) {
	w := newWorld(t)
	if err := os.MkdirAll(w.new, 0o700); err != nil {
		t.Fatal(err)
	}
	res, err := w.plan(false).Run()
	if err != nil || !res.Copied {
		t.Fatalf("res %+v err %v", res, err)
	}
	if read(t, filepath.Join(w.old, "audio", meetingID, "others-000001.wav")) != "RIFF-others" {
		t.Fatal("Notted's audio changed")
	}
	if read(t, filepath.Join(w.new, "audio", meetingID, "others-000001.wav")) != "RIFF-others" {
		t.Fatal("audio lost")
	}
}

func TestRunWithoutNottedDoesNothing(t *testing.T) {
	root := t.TempDir()
	p := Plan{OldHome: filepath.Join(root, ".notted"), NewHome: filepath.Join(root, ".phonon"), DBFile: "meetings.sqlite", Running: func() bool { return true }}
	res, err := p.Run()
	if err != nil || res.Copied {
		t.Fatalf("res %+v err %v", res, err)
	}
	absent(t, p.NewHome)
}

// A copy cut short leaves Notted's folder whole and no Phonon folder, so Phonon never starts
// empty; the next launch clears the half copy and copies again.
func TestRunCutShortLeavesNoPhononFolderAndRetries(t *testing.T) {
	w := newWorld(t)
	before := snapshot(t, w.old)
	p := w.plan(false)
	p.beforeMove = func() error { return errors.New("power cut") }
	if _, err := p.Run(); err == nil || !strings.Contains(err.Error(), "power cut") {
		t.Fatalf("err = %v", err)
	}
	absent(t, w.new)
	sameSnapshot(t, before, snapshot(t, w.old))
	put(t, filepath.Join(w.new+".migrating", "stray"), "half copy")
	res, err := w.plan(false).Run()
	if err != nil || !res.Copied {
		t.Fatalf("retry: res %+v err %v", res, err)
	}
	absent(t, filepath.Join(w.new, "stray"))
	absent(t, w.new+".migrating")
	db := openDB(t, filepath.Join(w.new, "meetings.sqlite"))
	defer db.Close()
	var p1 string
	if err := db.QueryRow(`SELECT path FROM chunks WHERE track = 'mic'`).Scan(&p1); err != nil || p1 != filepath.Join(w.new, "audio", meetingID, "mic-000001.wav") {
		t.Fatalf("path %q %v", p1, err)
	}
}

func TestRunFailsWhenTheCopyFails(t *testing.T) {
	w := newWorld(t)
	p := w.plan(false)
	// The new folder's parent is a file, so the copy cannot happen.
	parent := filepath.Join(w.root, "blocked")
	put(t, parent, "file")
	p.NewHome = filepath.Join(parent, ".phonon")
	if _, err := p.Run(); err == nil {
		t.Fatal("err = nil")
	}
	if read(t, filepath.Join(w.old, "audio", meetingID, "mic-000001.wav")) != "RIFF-mic" {
		t.Fatal("audio lost")
	}
	// Notted still finds its recordings: nothing was rewritten.
	db := openDB(t, filepath.Join(w.old, "meetings.sqlite"))
	defer db.Close()
	var got string
	if err := db.QueryRow(`SELECT path FROM chunks WHERE track = 'mic'`).Scan(&got); err != nil || got != filepath.Join(w.old, "audio", meetingID, "mic-000001.wav") {
		t.Fatalf("path %q %v", got, err)
	}
}

func TestDefaultCopiesDotNottedToDotPhonon(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv(paths.HomeEnv, "")
	p, ok := Default()
	if !ok {
		t.Fatal("Default is off")
	}
	if p.OldHome != filepath.Join(home, ".notted") || p.NewHome != filepath.Join(home, ".phonon") || p.DBFile != "meetings.sqlite" || p.Running == nil {
		t.Fatalf("plan %+v", p)
	}
}

// A Phonon with its own data folder, like a test one or the wails bindings run, starts empty.
func TestDefaultIsOffWithPhononHome(t *testing.T) {
	t.Setenv(paths.HomeEnv, t.TempDir())
	if _, ok := Default(); ok {
		t.Fatal("Default is on")
	}
}

// The file by file copy, used when the folder cannot be cloned, copies the same tree.
func TestCopyWalkCopiesTheTree(t *testing.T) {
	w := newWorld(t)
	if err := os.Symlink("install-source", filepath.Join(w.old, "link")); err != nil {
		t.Fatal(err)
	}
	dst := filepath.Join(w.root, "copy")
	if err := copyWalk(w.old, dst); err != nil {
		t.Fatal(err)
	}
	sameSnapshot(t, snapshot(t, w.old), snapshot(t, dst))
	if l, err := os.Readlink(filepath.Join(dst, "link")); err != nil || l != "install-source" {
		t.Fatalf("link %q %v", l, err)
	}
}

// make install writes ~/.phonon/install-source before Phonon first opens: Notted's data still
// comes over, and the new install-source is kept.
func TestRunCopiesOverAFolderWithOnlyInstallSource(t *testing.T) {
	w := newWorld(t)
	put(t, filepath.Join(w.new, "install-source"), "/code/phonon\n")
	res, err := w.plan(false).Run()
	if err != nil || !res.Copied {
		t.Fatalf("res %+v err %v", res, err)
	}
	if read(t, filepath.Join(w.new, "audio", meetingID, "mic-000001.wav")) != "RIFF-mic" {
		t.Fatal("audio not copied")
	}
	if got := read(t, filepath.Join(w.new, "install-source")); got != "/code/phonon\n" {
		t.Fatalf("install-source %q", got)
	}
	if got := read(t, filepath.Join(w.old, "install-source")); got != filepath.Join(w.root, "code", "notted")+"\n" {
		t.Fatalf("Notted's install-source changed: %q", got)
	}
}
