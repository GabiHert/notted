package service

import (
	"context"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/bhlabz/notted/internal/application/port"
)

// archiveTemplate holds the files a folder needs to be a meetings archive. The folder's dot-files are included.
//
//go:embed all:archive_template
var archiveTemplate embed.FS

type templateFile struct {
	path string
	mode fs.FileMode
}

// templateFiles lists the template in the order Missing reports it. Embedded files carry no mode, so each has one here.
var templateFiles = []templateFile{
	{"bin/meeting", 0o755},
	{"prompts/base.md", 0o644},
	{".claude/skills/meetings/SKILL.md", 0o644},
	{"instructions/README.md", 0o644},
	{"instructions/people/.gitkeep", 0o644},
	{"instructions/projects/.gitkeep", 0o644},
	{"meetings/.gitkeep", 0o644},
	{".githooks/pre-commit", 0o755},
	{".gitignore", 0o644},
	{"CLAUDE.md", 0o644},
}

const (
	gitignoreMarkerLine = "# Notted: recordings and session logs never enter git"
	gitignoreAddsRules  = ".gitignore (adds the recording rules)"
	makesExecutable     = " (makes it executable)"
)

// scripts are the template files that must be executable.
var scripts = map[string]bool{"bin/meeting": true, ".githooks/pre-commit": true}

// lacksExecBit reports whether rel is a regular file that no one can execute. A link is left alone.
func lacksExecBit(root *os.Root, rel string) (bool, fs.FileMode, error) {
	fi, err := root.Lstat(filepath.FromSlash(rel))
	if err != nil {
		return false, 0, mapRootErr(err)
	}
	return fi.Mode().IsRegular() && fi.Mode().Perm()&0o111 == 0, fi.Mode().Perm(), nil
}

// ArchiveCheck says what a folder still needs to be a meetings archive.
type ArchiveCheck struct {
	Path     string
	Missing  []string
	InRepo   bool
	Nested   bool // in a repository whose top is another folder
	RepoRoot string
	Ready    bool
}

func templateBytes(path string) []byte {
	b, err := archiveTemplate.ReadFile("archive_template/" + path)
	if err != nil {
		panic("archive template lacks " + path)
	}
	return b
}

func hasMarker(content string) bool {
	for _, l := range strings.Split(content, "\n") {
		if strings.TrimRight(l, "\r") == gitignoreMarkerLine {
			return true
		}
	}
	return false
}

// notThere reports whether err means the path does not exist yet, including a file standing where a folder should be.
func notThere(err error) bool {
	return errors.Is(err, fs.ErrNotExist) || errors.Is(err, syscall.ENOTDIR)
}

// exists reports whether rel exists inside root, without following a final link.
func exists(root *os.Root, rel string) (bool, error) {
	_, err := root.Lstat(filepath.FromSlash(rel))
	switch {
	case err == nil:
		return true, nil
	case notThere(err):
		return false, nil
	}
	return false, mapRootErr(err)
}

func missingTemplate(root *os.Root) ([]string, error) {
	missing := []string{}
	for _, tf := range templateFiles {
		has, err := exists(root, tf.path)
		if err != nil {
			return nil, err
		}
		switch {
		case !has:
			missing = append(missing, tf.path)
		case scripts[tf.path]:
			noExec, _, err := lacksExecBit(root, tf.path)
			if err != nil {
				return nil, err
			}
			if noExec {
				missing = append(missing, tf.path+makesExecutable)
			}
		case tf.path == ".gitignore":
			b, err := root.ReadFile(".gitignore")
			if err != nil {
				return nil, mapRootErr(err)
			}
			if !hasMarker(string(b)) {
				missing = append(missing, gitignoreAddsRules)
			}
		}
	}
	return missing, nil
}

// repoTop returns the top folder of the git repository that holds dir, or "" when dir is in none.
func (s *SetupService) repoTop(ctx context.Context, dir string) string {
	out, err := s.git.Run(ctx, dir, "rev-parse", "--show-toplevel")
	if err != nil {
		return ""
	}
	lines := strings.Split(strings.TrimSpace(out), "\n")
	return strings.TrimSpace(lines[len(lines)-1])
}

func sameFolder(a, b string) bool {
	ra, errA := filepath.EvalSymlinks(a)
	rb, errB := filepath.EvalSymlinks(b)
	return errA == nil && errB == nil && ra == rb
}

// InspectArchive reports which template files the folder lacks and whether it is in a git repository. It changes nothing.
func (s *SetupService) InspectArchive(ctx context.Context, path string) (ArchiveCheck, error) {
	if !isDir(path) {
		return ArchiveCheck{}, fmt.Errorf("That folder does not exist: %s", path)
	}
	root, err := os.OpenRoot(path)
	if err != nil {
		return ArchiveCheck{}, fmt.Errorf("Could not read %s: %w", path, unwrapPathErr(err))
	}
	defer root.Close()
	missing, err := missingTemplate(root)
	if err != nil {
		return ArchiveCheck{}, err
	}
	top := s.repoTop(ctx, path)
	return ArchiveCheck{Path: path, Missing: missing, InRepo: top != "", Nested: top != "" && !sameFolder(top, path), RepoRoot: top, Ready: len(missing) == 0 && top != ""}, nil
}

// PrepareArchive creates the template files the folder lacks and, outside any repository, runs git init in it.
// It never changes an existing file, except that it appends the recording rules to a .gitignore that lacks them.
func (s *SetupService) PrepareArchive(ctx context.Context, path string) error {
	if !isDir(path) {
		return fmt.Errorf("That folder does not exist: %s", path)
	}
	fail := func(reason string) error {
		return fmt.Errorf("Could not set up %s: %s", path, strings.TrimSpace(reason))
	}
	if err := prepareFiles(path); err != nil {
		return fail(err.Error())
	}
	top := s.repoTop(ctx, path)
	switch {
	case top == "":
		if out, err := s.git.Run(ctx, path, "init"); err != nil {
			return fail("git init failed:\n" + out)
		}
		if out, err := s.git.Run(ctx, path, "config", "core.hooksPath", ".githooks"); err != nil {
			return fail("setting the git hooks failed:\n" + out)
		}
	case sameFolder(top, path):
		out, err := s.git.Run(ctx, path, "config", "--get", "core.hooksPath")
		switch port.ExitCode(err) {
		case 0:
		case 1:
			if out, err := s.git.Run(ctx, path, "config", "core.hooksPath", ".githooks"); err != nil {
				return fail("setting the git hooks failed:\n" + out)
			}
		default:
			return fail("reading the git hooks setting failed:\n" + out)
		}
	}
	return nil
}

// prepareFiles creates each missing template file with its mode and appends the recording rules when they are absent.
func prepareFiles(path string) error {
	root, err := os.OpenRoot(path)
	if err != nil {
		return err
	}
	defer root.Close()
	for _, tf := range templateFiles {
		has, err := exists(root, tf.path)
		if err != nil {
			return err
		}
		if has {
			if scripts[tf.path] {
				if err := makeExecutable(root, tf.path); err != nil {
					return err
				}
			}
			if tf.path == ".gitignore" {
				if err := addRecordingRules(root); err != nil {
					return err
				}
			}
			continue
		}
		if err := createTemplateFile(root, tf); err != nil {
			return err
		}
	}
	return nil
}

func createTemplateFile(root *os.Root, tf templateFile) error {
	name := filepath.FromSlash(tf.path)
	if dir := filepath.Dir(name); dir != "." {
		if err := root.MkdirAll(dir, 0o755); err != nil {
			return mapRootErr(err)
		}
	}
	f, err := root.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, tf.mode)
	if err != nil {
		if errors.Is(err, fs.ErrExist) {
			return nil
		}
		return mapRootErr(err)
	}
	_, werr := f.Write(templateBytes(tf.path))
	if cerr := f.Close(); werr == nil {
		werr = cerr
	}
	if werr == nil {
		werr = root.Chmod(name, tf.mode)
	}
	if werr != nil {
		_ = root.Remove(name)
		return mapRootErr(werr)
	}
	return nil
}

func addRecordingRules(root *os.Root) error {
	b, err := root.ReadFile(".gitignore")
	if err != nil {
		return mapRootErr(err)
	}
	if hasMarker(string(b)) {
		return nil
	}
	f, err := root.OpenFile(".gitignore", os.O_WRONLY|os.O_APPEND, 0)
	if err != nil {
		return mapRootErr(err)
	}
	add := "\n" + string(templateBytes(".gitignore"))
	if len(b) > 0 && b[len(b)-1] != '\n' {
		add = "\n" + add
	}
	_, werr := f.WriteString(add)
	if cerr := f.Close(); werr == nil {
		werr = cerr
	}
	return mapRootErr(werr)
}

// makeExecutable adds an execute bit wherever the file already has the matching read bit. It never touches the content.
func makeExecutable(root *os.Root, rel string) error {
	noExec, perm, err := lacksExecBit(root, rel)
	if err != nil || !noExec {
		return err
	}
	return mapRootErr(root.Chmod(filepath.FromSlash(rel), perm|(perm&0o444)>>2))
}

// unwrapPathErr drops the "open <path>:" prefix of a path error so the reason reads on its own.
func unwrapPathErr(err error) error {
	var pe *fs.PathError
	if errors.As(err, &pe) {
		return pe.Err
	}
	return err
}
