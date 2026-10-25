// Copied from github.com/brienze1/quant (internal/application/service/file_service.go), MIT License,
// Copyright (c) 2026 Luis Brienze. Modified for Notted; see NOTICE.md.

package service

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
)

const maxInstructionBytes = 2 << 20

var (
	errOutside  = errors.New("That path is outside the meetings archive folder.")
	errPatterns = errors.New("Only prompts/base.md and Markdown files under instructions/ can be opened here.")
	errTooLarge = errors.New("That file is too large to open here (over 2 MB).")
)

// instructionKinds lists the kinds in display order.
var instructionKinds = []string{"base", "readme", "project", "person", "local", "person-project"}

// normalizeRelPath lexically validates a client-supplied relative path and
// returns it cleaned, using the native separator. Backslashes are treated as
// separators on every platform so Windows-style payloads (`..\..`, `C:\x`,
// UNC) are rejected on POSIX hosts too. This is a pre-check only; os.Root
// enforces the real sandbox at the kernel level.
func normalizeRelPath(relPath string) (string, error) {
	slashed := strings.ReplaceAll(relPath, `\`, "/")
	if strings.HasPrefix(slashed, "/") {
		return "", fmt.Errorf("absolute paths are not allowed: %q", relPath)
	}

	native := filepath.FromSlash(slashed)
	if filepath.IsAbs(native) || filepath.VolumeName(native) != "" || isWindowsDrivePath(slashed) {
		return "", fmt.Errorf("absolute paths are not allowed: %q", relPath)
	}

	cleaned := filepath.Clean(native)
	if cleaned == ".." || strings.HasPrefix(cleaned, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("path escapes the session directory: %q", relPath)
	}

	return cleaned, nil
}

// isWindowsDrivePath reports whether a slash-normalized path starts with a
// Windows drive letter (e.g. "C:"), which filepath.VolumeName only detects on
// Windows hosts.
func isWindowsDrivePath(slashed string) bool {
	if len(slashed) < 2 || slashed[1] != ':' {
		return false
	}
	c := slashed[0]
	return ('a' <= c && c <= 'z') || ('A' <= c && c <= 'Z')
}

// classifyInstruction names the kind of a cleaned slash-form path, or "" when no pattern matches.
func classifyInstruction(rel string) (kind string, readOnly bool) {
	switch rel {
	case "prompts/base.md":
		return "base", true
	case "instructions/README.md":
		return "readme", true
	}
	parts := strings.Split(rel, "/")
	if len(parts) < 3 || parts[0] != "instructions" {
		return "", false
	}
	stem := func(name string) bool { return strings.HasSuffix(name, ".md") && len(name) > len(".md") }
	switch {
	case len(parts) == 3 && parts[1] == "projects" && stem(parts[2]):
		return "project", false
	case len(parts) == 3 && parts[1] == "people" && stem(parts[2]):
		if strings.HasSuffix(parts[2], ".local.md") && len(parts[2]) > len(".local.md") {
			return "local", false
		}
		return "person", false
	case len(parts) == 4 && parts[1] == "people" && parts[2] != "" && stem(parts[3]):
		return "person-project", false
	}
	return "", false
}

// cleanInstructionPath normalizes a requested path to its slash form; any failure is the outside message.
func cleanInstructionPath(p string) (string, error) {
	rel, err := normalizeRelPath(p)
	if err != nil {
		return "", errOutside
	}
	return filepath.ToSlash(rel), nil
}

// mapRootErr turns an os.Root refusal (a path that resolves outside the folder) into the outside message.
func mapRootErr(err error) error {
	if err != nil && strings.Contains(err.Error(), "path escapes from parent") {
		return errOutside
	}
	return err
}

func readInstruction(checkout, p string) (string, error) {
	rel, err := cleanInstructionPath(p)
	if err != nil {
		return "", err
	}
	if kind, _ := classifyInstruction(rel); kind == "" {
		return "", errPatterns
	}
	realRoot, err := confine(checkout, rel)
	if err != nil {
		return "", err
	}
	root, err := os.OpenRoot(realRoot)
	if err != nil {
		return "", err
	}
	defer root.Close()

	info, err := root.Stat(filepath.FromSlash(rel))
	if err != nil {
		return "", mapRootErr(err)
	}
	if info.IsDir() {
		return "", errPatterns
	}
	if info.Size() > maxInstructionBytes {
		return "", errTooLarge
	}
	data, err := root.ReadFile(filepath.FromSlash(rel))
	if err != nil {
		return "", mapRootErr(err)
	}
	return string(data), nil
}

// confine opens the archive folder resolved through its own links and refuses any link on the way to rel:
// every component under the folder is checked with Lstat, then the resolved path (or, for a file that does
// not exist yet, its resolved parent) must stay inside instructions/ or be the exact prompts/base.md.
func confine(checkout, rel string) (realRoot string, err error) {
	realRoot, err = filepath.EvalSymlinks(checkout)
	if err != nil {
		return "", err
	}
	cur := realRoot
	missing := false
	for _, part := range strings.Split(rel, "/") {
		cur = filepath.Join(cur, part)
		fi, lerr := os.Lstat(cur)
		if lerr != nil {
			if errors.Is(lerr, os.ErrNotExist) {
				missing = true
				break
			}
			return "", lerr
		}
		if fi.Mode()&os.ModeSymlink != 0 {
			return "", errOutside
		}
	}
	probe := filepath.Join(realRoot, filepath.FromSlash(rel))
	if missing {
		probe = filepath.Dir(probe)
	}
	resolved, err := filepath.EvalSymlinks(probe)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return realRoot, nil
		}
		return "", err
	}
	if missing {
		resolved = filepath.Join(resolved, filepath.Base(rel))
	}
	if !within(filepath.Join(realRoot, "instructions"), resolved) && resolved != filepath.Join(realRoot, "prompts", "base.md") {
		return "", errOutside
	}
	return realRoot, nil
}

// within reports whether p is dir or lies under it, comparing whole path elements.
func within(dir, p string) bool {
	r, err := filepath.Rel(dir, p)
	return err == nil && r != ".." && !strings.HasPrefix(r, ".."+string(filepath.Separator))
}

func writeInstruction(checkout, p, text string) error {
	rel, err := cleanInstructionPath(p)
	if err != nil {
		return err
	}
	kind, readOnly := classifyInstruction(rel)
	if kind == "" {
		return errPatterns
	}
	if err := readOnlyError(rel, readOnly); err != nil {
		return err
	}
	realRoot, err := confine(checkout, rel)
	if err != nil {
		return err
	}
	root, err := os.OpenRoot(realRoot)
	if err != nil {
		return err
	}
	defer root.Close()

	target := rel
	native := filepath.FromSlash(target)
	dir := filepath.Dir(native)
	if di, err := root.Stat(dir); err != nil {
		return mapRootErr(err)
	} else if !di.IsDir() {
		return fmt.Errorf("not a folder: %s", path.Dir(target))
	}

	var suffix [8]byte
	if _, err := rand.Read(suffix[:]); err != nil {
		return err
	}
	tmp := filepath.Join(dir, ".notted-"+hex.EncodeToString(suffix[:])+".tmp")
	f, err := root.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return mapRootErr(err)
	}
	_, werr := f.WriteString(text)
	if cerr := f.Close(); werr == nil {
		werr = cerr
	}
	if werr == nil {
		werr = root.Rename(tmp, native)
	}
	if werr != nil {
		_ = root.Remove(tmp)
		return mapRootErr(werr)
	}
	return nil
}

func readOnlyError(rel string, readOnly bool) error {
	if !readOnly {
		return nil
	}
	if rel == "prompts/base.md" {
		return errors.New("prompts/base.md is read-only in Notted. Change it through the archive repository.")
	}
	return errors.New("instructions/README.md is read-only in Notted.")
}

// listInstructions returns the existing regular files that match an instruction pattern; links are skipped.
func listInstructions(checkout string) ([]InstructionFile, error) {
	root, err := os.OpenRoot(checkout)
	if err != nil {
		return nil, err
	}
	defer root.Close()

	var out []InstructionFile
	add := func(rel string) {
		fi, err := root.Lstat(filepath.FromSlash(rel))
		if err != nil || !fi.Mode().IsRegular() {
			return
		}
		if kind, ro := classifyInstruction(rel); kind != "" {
			out = append(out, InstructionFile{Path: rel, Kind: kind, ReadOnly: ro})
		}
	}
	names := func(dir string) (files, dirs []string) {
		d, err := root.Open(filepath.FromSlash(dir))
		if err != nil {
			return nil, nil
		}
		defer d.Close()
		entries, err := d.ReadDir(-1)
		if err != nil {
			return nil, nil
		}
		for _, e := range entries {
			switch {
			case e.Type().IsRegular():
				files = append(files, e.Name())
			case e.IsDir():
				dirs = append(dirs, e.Name())
			}
		}
		return files, dirs
	}

	add("prompts/base.md")
	add("instructions/README.md")
	files, _ := names("instructions/projects")
	for _, n := range files {
		add("instructions/projects/" + n)
	}
	files, dirs := names("instructions/people")
	for _, n := range files {
		add("instructions/people/" + n)
	}
	for _, d := range dirs {
		sub, _ := names("instructions/people/" + d)
		for _, n := range sub {
			add("instructions/people/" + d + "/" + n)
		}
	}

	rank := map[string]int{}
	for i, k := range instructionKinds {
		rank[k] = i
	}
	sort.SliceStable(out, func(i, j int) bool {
		if rank[out[i].Kind] != rank[out[j].Kind] {
			return rank[out[i].Kind] < rank[out[j].Kind]
		}
		return out[i].Path < out[j].Path
	})
	return out, nil
}

// createInstruction creates rel with text; it never overwrites (fs.ErrExist when rel exists).
func createInstruction(checkout, rel, text string) error {
	rel, err := cleanInstructionPath(rel)
	if err != nil {
		return err
	}
	kind, readOnly := classifyInstruction(rel)
	if kind == "" {
		return errPatterns
	}
	if err := readOnlyError(rel, readOnly); err != nil {
		return err
	}
	realRoot, err := confine(checkout, rel)
	if err != nil {
		return err
	}
	root, err := os.OpenRoot(realRoot)
	if err != nil {
		return err
	}
	defer root.Close()

	native := filepath.FromSlash(rel)
	if err := root.MkdirAll(filepath.Dir(native), 0o755); err != nil {
		return mapRootErr(err)
	}
	f, err := root.OpenFile(native, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		if errors.Is(err, fs.ErrExist) {
			return fs.ErrExist
		}
		return mapRootErr(err)
	}
	_, werr := f.WriteString(text)
	if cerr := f.Close(); werr == nil {
		werr = cerr
	}
	if werr != nil {
		_ = root.Remove(native)
		return mapRootErr(werr)
	}
	return nil
}

// instructionExists reports whether rel exists, without following links.
func instructionExists(checkout, rel string) (bool, error) {
	realRoot, err := confine(checkout, rel)
	if err != nil {
		return false, err
	}
	root, err := os.OpenRoot(realRoot)
	if err != nil {
		return false, err
	}
	defer root.Close()
	if _, err := root.Lstat(filepath.FromSlash(rel)); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return false, nil
		}
		return false, mapRootErr(err)
	}
	return true, nil
}

func notRegularErr(rel string) error {
	return fmt.Errorf("Notted cannot delete %s: it is not a regular file.", rel)
}

// profileFiles lists every file and folder of the person name under instructions/people/, sorted;
// folder entries end in "/". It refuses links and special files and deletes nothing.
func profileFiles(checkout, name string) ([]string, error) {
	base := "instructions/people/" + name
	a, b, d := base+".md", base+".local.md", base
	var realRoot string
	for _, rel := range []string{a, b, d} {
		r, err := confine(checkout, rel)
		if err != nil {
			return nil, err
		}
		realRoot = r
	}
	root, err := os.OpenRoot(realRoot)
	if err != nil {
		return nil, err
	}
	defer root.Close()

	var list []string
	for _, rel := range []string{a, b} {
		fi, err := root.Lstat(filepath.FromSlash(rel))
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				continue
			}
			return nil, mapRootErr(err)
		}
		if fi.Mode()&fs.ModeSymlink != 0 {
			return nil, errOutside
		}
		if !fi.Mode().IsRegular() {
			return nil, notRegularErr(rel)
		}
		list = append(list, rel)
	}

	fi, err := root.Lstat(filepath.FromSlash(d))
	switch {
	case err == nil:
		if fi.Mode()&fs.ModeSymlink != 0 {
			return nil, errOutside
		}
		if !fi.IsDir() {
			return nil, fmt.Errorf("Notted cannot delete %s: it is not a folder.", d)
		}
		list = append(list, d+"/")
		list, err = walkProfileDir(root, d, list)
		if err != nil {
			return nil, err
		}
	case !errors.Is(err, fs.ErrNotExist):
		return nil, mapRootErr(err)
	}

	if len(list) == 0 {
		return nil, fmt.Errorf("There is no profile %q in the archive.", name)
	}
	sort.Strings(list)
	return list, nil
}

func walkProfileDir(root *os.Root, dir string, list []string) ([]string, error) {
	f, err := root.Open(filepath.FromSlash(dir))
	if err != nil {
		return nil, mapRootErr(err)
	}
	entries, err := f.ReadDir(-1)
	f.Close()
	if err != nil {
		return nil, mapRootErr(err)
	}
	for _, e := range entries {
		rel := dir + "/" + e.Name()
		switch {
		case e.Type()&fs.ModeSymlink != 0:
			return nil, errOutside
		case e.IsDir():
			list = append(list, rel+"/")
			if list, err = walkProfileDir(root, rel, list); err != nil {
				return nil, err
			}
		case e.Type().IsRegular():
			list = append(list, rel)
		default:
			return nil, notRegularErr(rel)
		}
	}
	return list, nil
}

// removeListed deletes exactly the listed paths: files first, in list order, then folders in
// reverse list order (deepest first), each with os.Root.Remove, so a folder goes only when empty.
func removeListed(checkout string, list []string) error {
	realRoot, err := confine(checkout, "instructions/people")
	if err != nil {
		return err
	}
	root, err := os.OpenRoot(realRoot)
	if err != nil {
		return err
	}
	defer root.Close()
	rm := func(rel string) error {
		if err := root.Remove(filepath.FromSlash(strings.TrimSuffix(rel, "/"))); err != nil {
			return fmt.Errorf("Notted could not delete %s: %v. The files listed before it are already deleted.", rel, mapRootErr(err))
		}
		return nil
	}
	for _, rel := range list {
		if !strings.HasSuffix(rel, "/") {
			if err := rm(rel); err != nil {
				return err
			}
		}
	}
	for i := len(list) - 1; i >= 0; i-- {
		if strings.HasSuffix(list[i], "/") {
			if err := rm(list[i]); err != nil {
				return err
			}
		}
	}
	return nil
}

// listMarkdownKeys returns the sorted stems of the regular *.md files directly in dir whose stem
// matches keyRe; a missing dir gives []string{}. Never nil.
func listMarkdownKeys(checkout, dir string) ([]string, error) {
	realRoot, err := confine(checkout, dir)
	if err != nil {
		return nil, err
	}
	root, err := os.OpenRoot(realRoot)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	d, err := root.Open(filepath.FromSlash(dir))
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return []string{}, nil
		}
		return nil, mapRootErr(err)
	}
	defer d.Close()
	entries, err := d.ReadDir(-1)
	if err != nil {
		return nil, mapRootErr(err)
	}
	out := []string{}
	for _, e := range entries {
		n := e.Name()
		if !e.Type().IsRegular() || !strings.HasSuffix(n, ".md") {
			continue
		}
		if stem := strings.TrimSuffix(n, ".md"); keyRe.MatchString(stem) {
			out = append(out, stem)
		}
	}
	sort.Strings(out)
	return out, nil
}
