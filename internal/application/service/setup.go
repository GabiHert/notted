package service

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"
	"time"
	"unicode"

	"github.com/GabiHert/notted/internal/application/port"
)

var (
	errNoAddress     = errors.New("Paste the archive's GitHub address first.")
	errNoCloneParent = errors.New("Choose the folder to clone into first.")
	errCloneTimeout  = errors.New("git clone did not finish within 10 minutes.")
	errNoProfileName = errors.New("Enter a name for the new profile.")
	errProfileFormat = errors.New(`Profile must use lowercase letters, digits and dashes, like "ana" or "ana-lima".`)
	errNoProjectKey  = errors.New("Enter a key for the new project.")
	errProjectKey    = errors.New(`Project key must use lowercase letters, digits and dashes, like "acme" or "acme-web".`)
	errProfileUnset  = errors.New("Set your profile in Settings > Instructions first.")
	errNoDeleteName  = errors.New("Choose a profile to delete.")
	errProfileInUse  = errors.New("You cannot delete the profile in use. Choose another profile first.")
)

// cloneTimeout bounds one git clone; tests shorten it.
var cloneTimeout = 10 * time.Minute

var (
	scpAddressRe = regexp.MustCompile(`^[A-Za-z0-9._-]+@[A-Za-z0-9.-]+:\S+$`)
	repoNameRe   = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)
)

// checkArchive returns nil when path holds everything a meetings archive needs, else the user-facing reason.
// It is the last check before saving the folder; InspectArchive and PrepareArchive add what is missing.
func checkArchive(path string) error {
	if !isDir(path) {
		return fmt.Errorf("That folder does not exist: %s", path)
	}
	const notReady = "That folder is not set up as a meetings archive yet: "
	if fi, err := os.Stat(filepath.Join(path, "bin", "meeting")); err != nil || fi.IsDir() {
		return errors.New(notReady + "bin/meeting is missing.")
	} else if fi.Mode()&0o111 == 0 {
		return errors.New(notReady + "bin/meeting is not executable.")
	}
	if fi, err := os.Stat(filepath.Join(path, "prompts", "base.md")); err != nil || !fi.Mode().IsRegular() {
		return errors.New(notReady + "prompts/base.md is missing.")
	}
	if fi, err := os.Stat(filepath.Join(path, "instructions")); err != nil || !fi.IsDir() {
		return errors.New(notReady + "instructions/ is missing.")
	}
	if fi, err := os.Stat(filepath.Join(path, ".claude", "skills", "meetings", "SKILL.md")); err != nil || !fi.Mode().IsRegular() {
		return errors.New(notReady + ".claude/skills/meetings/SKILL.md is missing.")
	}
	return nil
}

// SetupService clones the meetings archive and creates the person and project files in it.
type SetupService struct {
	settings port.SettingsStore
	git      port.Git
}

// NewSetupService builds the setup service.
func NewSetupService(settings port.SettingsStore, git port.Git) *SetupService {
	return &SetupService{settings: settings, git: git}
}

func validAddress(a string) bool {
	if strings.HasPrefix(a, "-") {
		return false
	}
	for _, r := range a {
		if unicode.IsSpace(r) || unicode.IsControl(r) {
			return false
		}
	}
	for _, p := range []string{"https://", "ssh://", "file://"} {
		if strings.HasPrefix(a, p) {
			return true
		}
	}
	return scpAddressRe.MatchString(a)
}

func repoName(a string) string {
	t := strings.TrimRight(a, "/")
	if i := strings.Index(t, "://"); i >= 0 && !strings.Contains(t[i+3:], "/") {
		return ""
	}
	if i := strings.LastIndexAny(t, "/:"); i >= 0 {
		t = t[i+1:]
	}
	t = strings.TrimSuffix(t, ".git")
	if !repoNameRe.MatchString(t) {
		return ""
	}
	return t
}

// CloneArchive clones the archive at address into parentDir with the person's own git and returns the clone's path.
func (s *SetupService) CloneArchive(ctx context.Context, address, parentDir string) (string, error) {
	a := strings.TrimSpace(address)
	if a == "" {
		return "", errNoAddress
	}
	if !validAddress(a) {
		return "", fmt.Errorf("That is not a git address: %s", a)
	}
	name := repoName(a)
	if name == "" {
		return "", fmt.Errorf("That address does not end in a repository name: %s", a)
	}
	p := strings.TrimSpace(parentDir)
	if p == "" {
		return "", errNoCloneParent
	}
	if !isDir(p) {
		return "", fmt.Errorf("That folder does not exist: %s", p)
	}
	target := filepath.Join(p, name)
	if _, err := os.Lstat(target); err == nil {
		return "", fmt.Errorf("%s already exists. Choose another folder, or choose the existing archive instead.", target)
	}
	cctx, cancel := context.WithTimeout(ctx, cloneTimeout)
	defer cancel()
	out, err := s.git.Run(cctx, p, "clone", "--", a, name)
	if err != nil {
		if errors.Is(cctx.Err(), context.DeadlineExceeded) {
			return "", errCloneTimeout
		}
		return "", errors.New("git clone failed:\n" + strings.TrimSpace(out))
	}
	if out, err := s.git.Run(ctx, target, "config", "core.hooksPath", ".githooks"); err != nil {
		return "", fmt.Errorf("The archive was cloned to %s, but setting its git hooks failed:\n%s", target, strings.TrimSpace(out))
	}
	if err := prepareFiles(target); err != nil {
		return "", fmt.Errorf("Could not set up %s: %s", target, err)
	}
	if err := checkArchive(target); err != nil {
		return "", err
	}
	return target, nil
}

// ListProfiles returns the people files' names in the archive, sorted.
func (s *SetupService) ListProfiles(ctx context.Context) ([]string, error) {
	st, err := s.settings.GetSettings(ctx)
	if err != nil {
		return nil, err
	}
	if st.CheckoutPath == "" {
		return nil, errNoArchive
	}
	realRoot, err := confine(st.CheckoutPath, "instructions/people")
	if err != nil {
		return nil, err
	}
	root, err := os.OpenRoot(realRoot)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	d, err := root.Open("instructions/people")
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return []string{}, nil
		}
		return nil, mapRootErr(err)
	}
	defer d.Close()
	entries, err := d.ReadDir(-1)
	if err != nil {
		return nil, err
	}
	out := []string{}
	for _, e := range entries {
		n := e.Name()
		if !e.Type().IsRegular() || !strings.HasSuffix(n, ".md") || strings.HasSuffix(n, ".local.md") {
			continue
		}
		if stem := strings.TrimSuffix(n, ".md"); keyRe.MatchString(stem) {
			out = append(out, stem)
		}
	}
	sort.Strings(out)
	return out, nil
}

// CreateProfile creates the person's file from the template and returns its archive-relative path.
func (s *SetupService) CreateProfile(ctx context.Context, name string) (string, error) {
	st, err := s.settings.GetSettings(ctx)
	if err != nil {
		return "", err
	}
	if st.CheckoutPath == "" {
		return "", errNoArchive
	}
	n := strings.TrimSpace(name)
	if n == "" {
		return "", errNoProfileName
	}
	if !keyRe.MatchString(n) {
		return "", errProfileFormat
	}
	rel := "instructions/people/" + n + ".md"
	if err := createInstruction(st.CheckoutPath, rel, personTemplate(n)); err != nil {
		if errors.Is(err, fs.ErrExist) {
			return "", fmt.Errorf("%s already exists. Choose it from the list instead.", rel)
		}
		return "", err
	}
	return rel, nil
}

// CreateProject creates the project file and the person's file for it, whichever is missing, and returns the created paths.
func (s *SetupService) CreateProject(ctx context.Context, key string) ([]string, error) {
	st, err := s.settings.GetSettings(ctx)
	if err != nil {
		return nil, err
	}
	if st.CheckoutPath == "" {
		return nil, errNoArchive
	}
	if st.Profile == "" {
		return nil, errProfileUnset
	}
	k := strings.TrimSpace(key)
	if k == "" {
		return nil, errNoProjectKey
	}
	if !keyRe.MatchString(k) {
		return nil, errProjectKey
	}
	p := "instructions/projects/" + k + ".md"
	q := "instructions/people/" + st.Profile + "/" + k + ".md"
	pHas, err := instructionExists(st.CheckoutPath, p)
	if err != nil {
		return nil, err
	}
	qHas, err := instructionExists(st.CheckoutPath, q)
	if err != nil {
		return nil, err
	}
	if pHas && qHas {
		return nil, fmt.Errorf("Project %q and your file for it already exist.", k)
	}
	created := []string{}
	if !pHas {
		if err := createInstruction(st.CheckoutPath, p, projectTemplate(k)); err != nil {
			return nil, err
		}
		created = append(created, p)
	}
	if !qHas {
		if err := createInstruction(st.CheckoutPath, q, personProjectTemplate(st.Profile, k)); err != nil {
			return nil, err
		}
		created = append(created, q)
	}
	return created, nil
}

// profileList runs every check and returns the archive folder, the trimmed name and the current list.
func (s *SetupService) profileList(ctx context.Context, name string) (checkout, n string, list []string, err error) {
	st, err := s.settings.GetSettings(ctx)
	if err != nil {
		return "", "", nil, err
	}
	if st.CheckoutPath == "" {
		return "", "", nil, errNoArchive
	}
	n = strings.TrimSpace(name)
	if n == "" {
		return "", "", nil, errNoDeleteName
	}
	if !keyRe.MatchString(n) {
		return "", "", nil, errProfileFormat
	}
	if n == st.Profile {
		return "", "", nil, errProfileInUse
	}
	list, err = profileFiles(st.CheckoutPath, n)
	if err != nil {
		return "", "", nil, err
	}
	return st.CheckoutPath, n, list, nil
}

// ListProfileFiles returns every file and folder that deleting the profile would remove.
func (s *SetupService) ListProfileFiles(ctx context.Context, name string) ([]string, error) {
	_, _, list, err := s.profileList(ctx, name)
	return list, err
}

// DeleteProfile deletes exactly the files ListProfileFiles returns now, when they equal files; it never runs git.
func (s *SetupService) DeleteProfile(ctx context.Context, name string, files []string) error {
	checkout, n, list, err := s.profileList(ctx, name)
	if err != nil {
		return err
	}
	if !slices.Equal(list, files) {
		return fmt.Errorf("The files of profile %q changed. Review the list again before deleting.", n)
	}
	return removeListed(checkout, list)
}

// ListMyProjectKeys returns the projects the saved profile has its own file for.
func (s *SetupService) ListMyProjectKeys(ctx context.Context) ([]string, error) {
	st, err := s.settings.GetSettings(ctx)
	if err != nil {
		return nil, err
	}
	if st.CheckoutPath == "" {
		return nil, errNoArchive
	}
	if st.Profile == "" {
		return nil, errProfileUnset
	}
	return listMarkdownKeys(st.CheckoutPath, "instructions/people/"+st.Profile)
}

// CreateMyProjectFile creates the saved profile's file for an existing project and returns its path.
func (s *SetupService) CreateMyProjectFile(ctx context.Context, key string) (string, error) {
	st, err := s.settings.GetSettings(ctx)
	if err != nil {
		return "", err
	}
	if st.CheckoutPath == "" {
		return "", errNoArchive
	}
	if st.Profile == "" {
		return "", errProfileUnset
	}
	k := strings.TrimSpace(key)
	if k == "" {
		return "", errNoProjectKey
	}
	if !keyRe.MatchString(k) {
		return "", errProjectKey
	}
	has, err := instructionExists(st.CheckoutPath, "instructions/projects/"+k+".md")
	if err != nil {
		return "", err
	}
	if !has {
		return "", fmt.Errorf("There is no project %q in the archive. Create it with New project.", k)
	}
	q := "instructions/people/" + st.Profile + "/" + k + ".md"
	if err := createInstruction(st.CheckoutPath, q, personProjectTemplate(st.Profile, k)); err != nil {
		if errors.Is(err, fs.ErrExist) {
			return "", fmt.Errorf("%s already exists.", q)
		}
		return "", err
	}
	return q, nil
}

func personTemplate(name string) string {
	return "# Person: " + name + "\n\n## Role\n\n## Style\n\n## Tools\n"
}

func projectTemplate(key string) string {
	return "# Project: " + key + "\n\n## Key\n" + key + "\n\n## Name\n\n## Aliases\n\n## About\n\n## Existing work\n"
}

func personProjectTemplate(profile, key string) string {
	return "# " + profile + " — " + key + "\n\n## Project root\n\n## Tools\n"
}
