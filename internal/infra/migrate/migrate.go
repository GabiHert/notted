// Package migrate copies the data folder of Notted, Phonon's old name, to Phonon's, once: the
// meetings database, audio, speech models, session files and install-source in ~/.notted are
// copied to ~/.phonon. ~/.notted is never changed or deleted, so Notted.app keeps working. Notted had no keychain item and no login item to move. The meetings archive lives
// wherever its path in the settings says and stays there.
package migrate

import (
	"database/sql"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/GabiHert/phonon/internal/infra/paths"
	_ "github.com/mattn/go-sqlite3"
	"golang.org/x/sys/unix"
)

// ErrOldRunning means Notted is open, so its folder cannot be copied consistently yet.
var ErrOldRunning = errors.New("Notted is still running. Quit Notted (its menu bar icon → Quit Notted), then open Phonon again; Phonon then brings over your meetings, recordings and settings.")

// Plan says what to copy.
type Plan struct {
	OldHome, NewHome string
	DBFile           string      // the database in the folder, whose stored paths point into OldHome
	Running          func() bool // whether Notted is open

	beforeMove func() error // tests interrupt the copy here, before its final rename
}

// Result says what was copied.
type Result struct {
	Copied bool // Notted's data folder was copied to Phonon's
}

// writeOnce are columns a trigger forbids changing; they hold ids and archive-relative folders,
// never a path into the data folder.
var writeOnce = map[string]bool{"meetings.folder": true, "meetings.claude_session_id": true}

// Default is the plan for this Mac: ~/.notted to Phonon's home. It is off (false) with
// PHONON_HOME set: a separate Phonon, like a test one or the wails bindings run, starts empty.
func Default() (Plan, bool) {
	if os.Getenv(paths.HomeEnv) != "" {
		return Plan{}, false
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return Plan{}, false
	}
	return Plan{
		OldHome: filepath.Join(home, ".notted"),
		NewHome: paths.Home(),
		DBFile:  filepath.Base(paths.DBPath()),
		Running: nottedRunning,
	}, true
}

// nottedRunning reports whether a process named Notted runs for this user.
func nottedRunning() bool {
	return exec.Command("/usr/bin/pgrep", "-x", "-U", fmt.Sprint(os.Getuid()), "Notted").Run() == nil
}

// Run copies the folder when Phonon's does not exist yet (or is fresh: see freshDir) and Notted's
// does, and never while Notted runs (ErrOldRunning). The copy is built in a staging folder next
// to Phonon's, its stored paths are rewritten there, and a final rename makes it Phonon's folder:
// on an error Phonon's folder does not exist, so the caller must not start on an empty one, and
// the next launch starts over. Notted's folder is only read.
func (p Plan) Run() (Result, error) {
	var res Result
	if fi, err := os.Stat(p.OldHome); err != nil || !fi.IsDir() {
		return res, nil
	}
	if fi, err := os.Lstat(p.NewHome); err == nil {
		if !fi.IsDir() || !freshDir(p.NewHome) {
			return res, nil
		}
	} else if !os.IsNotExist(err) {
		return res, fmt.Errorf("could not check %s: %w", p.NewHome, err)
	}
	if p.Running != nil && p.Running() {
		return res, ErrOldRunning
	}
	if err := os.MkdirAll(filepath.Dir(p.NewHome), 0o755); err != nil {
		return res, fmt.Errorf("could not create %s: %w", filepath.Dir(p.NewHome), err)
	}
	staging := p.NewHome + ".migrating"
	// A staging folder left by an earlier copy cut short is Phonon's own half copy.
	if err := os.RemoveAll(staging); err != nil {
		return res, fmt.Errorf("could not clear %s: %w", staging, err)
	}
	if err := copyTree(p.OldHome, staging); err != nil {
		_ = os.RemoveAll(staging)
		return res, fmt.Errorf("could not copy %s to %s: %w", p.OldHome, staging, err)
	}
	fail := func(err error) (Result, error) {
		_ = os.RemoveAll(staging)
		return res, err
	}
	if p.DBFile != "" {
		if err := rewriteDB(filepath.Join(staging, p.DBFile), p.OldHome, p.NewHome); err != nil {
			return fail(err)
		}
	}
	if err := rewriteSessions(filepath.Join(staging, "sessions"), p.OldHome, p.NewHome); err != nil {
		return fail(err)
	}
	if p.beforeMove != nil {
		if err := p.beforeMove(); err != nil {
			return res, err // left in place on purpose: the next run must clear it
		}
	}
	if fi, err := os.Lstat(p.NewHome); err == nil && fi.IsDir() {
		// make install wrote Phonon's install-source; it wins over Notted's.
		if b, err := os.ReadFile(filepath.Join(p.NewHome, installSource)); err == nil {
			if err := os.WriteFile(filepath.Join(staging, installSource), b, 0o644); err != nil {
				return fail(err)
			}
		}
		if err := os.RemoveAll(p.NewHome); err != nil { // fresh, checked above
			return fail(fmt.Errorf("could not replace the fresh folder %s: %w", p.NewHome, err))
		}
	}
	if err := os.Rename(staging, p.NewHome); err != nil {
		return fail(fmt.Errorf("could not move %s to %s: %w", staging, p.NewHome, err))
	}
	res.Copied = true
	return res, nil
}

// copyTree copies the folder src to dst, which must not exist. It clones it first (APFS
// clonefile: instant, and it takes no extra space until a side changes), then falls back to a
// file by file copy, for example across volumes.
func copyTree(src, dst string) error {
	if err := unix.Clonefile(src, dst, unix.CLONE_NOFOLLOW); err == nil {
		return nil
	}
	_ = os.RemoveAll(dst)
	return copyWalk(src, dst)
}

// copyWalk copies src to dst file by file: folders, regular files and symlinks.
func copyWalk(src, dst string) error {
	return filepath.Walk(src, func(path string, fi os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		to := filepath.Join(dst, rel)
		switch m := fi.Mode(); {
		case m.IsDir():
			return os.MkdirAll(to, m.Perm())
		case m&os.ModeSymlink != 0:
			link, err := os.Readlink(path)
			if err != nil {
				return err
			}
			return os.Symlink(link, to)
		case m.IsRegular():
			return copyFile(path, to, m.Perm())
		default: // sockets and the like belong to a running Notted
			return nil
		}
	})
}

func copyFile(src, dst string, perm os.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_EXCL, perm)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

// installSource is the file make install writes into Phonon's folder before Phonon first opens.
const installSource = "install-source"

// freshDir reports whether dir holds nothing Phonon made: it is empty, or holds only the
// install-source file make install wrote.
func freshDir(dir string) bool {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false
	}
	for _, e := range entries {
		if e.Name() != installSource || !e.Type().IsRegular() {
			return false
		}
	}
	return true
}

// rewriteDB points every stored path into oldHome at newHome. Notted stored absolute paths, such
// as an audio chunk's file, in TEXT columns.
func rewriteDB(db, oldHome, newHome string) error {
	if _, err := os.Stat(db); err != nil {
		return nil
	}
	conn, err := sql.Open("sqlite3", "file:"+db+"?_busy_timeout=5000")
	if err != nil {
		return err
	}
	defer conn.Close()
	conn.SetMaxOpenConns(1)
	tables, err := textColumns(conn)
	if err != nil {
		return fmt.Errorf("could not read the database: %w", err)
	}
	from, to := oldHome+string(filepath.Separator), newHome+string(filepath.Separator)
	tx, err := conn.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for table, cols := range tables {
		for _, c := range cols {
			if writeOnce[table+"."+c] {
				continue
			}
			q := fmt.Sprintf(`UPDATE %s SET %s = replace(%s, ?, ?) WHERE instr(%s, ?) > 0`, quote(table), quote(c), quote(c), quote(c))
			if _, err := tx.Exec(q, from, to, from); err != nil {
				return fmt.Errorf("could not update paths in %s: %w", table, err)
			}
		}
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	// Fold the WAL into the copied database before the folder is renamed into place.
	if _, err := conn.Exec(`PRAGMA wal_checkpoint(TRUNCATE)`); err != nil {
		return fmt.Errorf("could not checkpoint the database: %w", err)
	}
	return nil
}

// textColumns lists each table's TEXT columns.
func textColumns(conn *sql.DB) (map[string][]string, error) {
	rows, err := conn.Query(`SELECT name FROM sqlite_master WHERE type = 'table' AND name NOT LIKE 'sqlite_%'`)
	if err != nil {
		return nil, err
	}
	var names []string
	for rows.Next() {
		var n string
		if err := rows.Scan(&n); err != nil {
			rows.Close()
			return nil, err
		}
		names = append(names, n)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	out := map[string][]string{}
	for _, n := range names {
		cols, err := conn.Query(`SELECT name, type FROM pragma_table_info(?)`, n)
		if err != nil {
			return nil, err
		}
		for cols.Next() {
			var c, typ string
			if err := cols.Scan(&c, &typ); err != nil {
				cols.Close()
				return nil, err
			}
			if strings.Contains(strings.ToUpper(typ), "TEXT") {
				out[n] = append(out[n], c)
			}
		}
		cols.Close()
	}
	return out, nil
}

func quote(id string) string { return `"` + strings.ReplaceAll(id, `"`, `""`) + `"` }

// rewriteSessions points the paths in each meeting's session files (the MCP config names the ask
// socket) at newHome. Phonon writes these again when a session starts; this keeps them right
// in between.
func rewriteSessions(dir, oldHome, newHome string) error {
	from, to := oldHome+string(filepath.Separator), newHome+string(filepath.Separator)
	files, err := filepath.Glob(filepath.Join(dir, "*", "*.json"))
	if err != nil {
		return err
	}
	for _, f := range files {
		fi, err := os.Lstat(f)
		if err != nil || !fi.Mode().IsRegular() {
			continue
		}
		b, err := os.ReadFile(f)
		if err != nil {
			return err
		}
		if !strings.Contains(string(b), from) {
			continue
		}
		tmp := f + ".migrate"
		if err := os.WriteFile(tmp, []byte(strings.ReplaceAll(string(b), from, to)), fi.Mode().Perm()); err != nil {
			_ = os.Remove(tmp)
			return err
		}
		if err := os.Rename(tmp, f); err != nil {
			_ = os.Remove(tmp)
			return err
		}
	}
	return nil
}
