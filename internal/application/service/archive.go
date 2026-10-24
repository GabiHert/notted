package service

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/bhlabz/notted/internal/application/port"
	"github.com/bhlabz/notted/internal/domain"
)

var clientKeyRe = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`)

// ExportResult is where an export landed, relative to the checkout.
type ExportResult struct {
	Folder string
	Files  []string
}

// ArchiveService ties a meeting to a client and writes it into the archive checkout.
type ArchiveService struct {
	store    port.MeetingStore
	settings port.SettingsStore
	git      port.Git
	emit     port.EventEmitter
}

// NewArchiveService builds the archive service.
func NewArchiveService(store port.MeetingStore, settings port.SettingsStore, git port.Git, emit port.EventEmitter) *ArchiveService {
	return &ArchiveService{store: store, settings: settings, git: git, emit: emit}
}

func (s *ArchiveService) checkout(ctx context.Context) (domain.Settings, error) {
	st, err := s.settings.GetSettings(ctx)
	if err != nil {
		return st, err
	}
	if st.CheckoutPath == "" {
		return st, errors.New("Set the meetings archive folder in Settings > Instructions first.")
	}
	if fi, err := os.Stat(filepath.Join(st.CheckoutPath, "bin", "meeting")); err != nil || fi.IsDir() {
		return st, errors.New("That folder is not a meetings archive checkout: bin/meeting is missing.")
	}
	return st, nil
}

func (s *ArchiveService) meeting(ctx context.Context, id string) (domain.Meeting, error) {
	m, err := s.store.GetMeeting(ctx, id)
	if errors.Is(err, domain.ErrNotFound) {
		return m, errors.New("Meeting not found.")
	}
	return m, err
}

// ListClients returns the client keys of the archive, sorted.
func (s *ArchiveService) ListClients(ctx context.Context) ([]string, error) {
	st, err := s.checkout(ctx)
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(filepath.Join(st.CheckoutPath, "instructions", "clients"))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return []string{}, nil
		}
		return nil, err
	}
	keys := []string{}
	for _, e := range entries {
		key, ok := strings.CutSuffix(e.Name(), ".md")
		if !ok || e.IsDir() || !clientKeyRe.MatchString(key) {
			continue
		}
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys, nil
}

// SetMeetingClient sets the meeting's client; it is fixed once a Claude session exists.
func (s *ArchiveService) SetMeetingClient(ctx context.Context, meetingID, key string) error {
	m, err := s.meeting(ctx, meetingID)
	if err != nil {
		return err
	}
	keys, err := s.ListClients(ctx)
	if err != nil {
		return err
	}
	found := false
	for _, k := range keys {
		found = found || k == key
	}
	if !found {
		return fmt.Errorf("Unknown client %q. Add instructions/clients/%s.md to the archive first.", key, key)
	}
	if m.ClientKey == key {
		return nil
	}
	if m.ClaudeSessionID != "" {
		return errors.New("This meeting's client is fixed once its summary exists.")
	}
	return s.store.SetClientKey(ctx, meetingID, key)
}

// ExportMeeting writes the meeting's four files into the archive. It never runs git.
func (s *ArchiveService) ExportMeeting(ctx context.Context, meetingID string) (ExportResult, error) {
	st, err := s.checkout(ctx)
	if err != nil {
		return ExportResult{}, err
	}
	m, err := s.meeting(ctx, meetingID)
	if err != nil {
		return ExportResult{}, err
	}
	if m.ClientKey == "" || m.ClaudeSessionID == "" || m.SummaryMD == "" || m.SuggestionsMD == "" {
		return ExportResult{}, errors.New("Nothing to export yet. Generate the summary first.")
	}
	segs, err := s.store.ListSegments(ctx, meetingID)
	if err != nil {
		return ExportResult{}, err
	}
	date := m.StartedAt.In(time.Local).Format("2006-01-02")
	folder := m.Folder
	if folder == "" {
		base := "meetings/" + m.ClientKey + "/" + date[:4] + "/" + date + "-" + slugify(m.Title)
		folder = base
		for n := 2; ; n++ {
			if _, err := os.Stat(filepath.Join(st.CheckoutPath, filepath.FromSlash(folder))); errors.Is(err, os.ErrNotExist) {
				break
			} else if err != nil {
				return ExportResult{}, err
			}
			folder = fmt.Sprintf("%s-%d", base, n)
		}
		if err := s.store.SetFolder(ctx, meetingID, folder); err != nil {
			return ExportResult{}, err
		}
	}
	dir := filepath.Join(st.CheckoutPath, filepath.FromSlash(folder))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return ExportResult{}, err
	}
	contents := []struct{ name, body string }{
		{"meeting.md", meetingFile(date, m, st.Profile)},
		{"transcript.md", transcriptFile(m.Title, segs)},
		{"summary.md", oneNewline(m.SummaryMD)},
		{"suggestions.md", oneNewline(m.SuggestionsMD)},
	}
	files := make([]string, 0, len(contents))
	for _, c := range contents {
		if err := os.WriteFile(filepath.Join(dir, c.name), []byte(c.body), 0o644); err != nil {
			return ExportResult{}, err
		}
		files = append(files, folder+"/"+c.name)
	}
	s.emit.Emit("export:ready", map[string]any{"meetingId": meetingID, "folder": folder, "files": files})
	return ExportResult{Folder: folder, Files: files}, nil
}

// CommitAndPushMeeting commits only the meeting folder and pushes the current branch.
// Call it only after the person said yes.
func (s *ArchiveService) CommitAndPushMeeting(ctx context.Context, meetingID string) (string, error) {
	st, err := s.checkout(ctx)
	if err != nil {
		return "", err
	}
	m, err := s.meeting(ctx, meetingID)
	if err != nil {
		return "", err
	}
	if m.Folder == "" {
		return "", errors.New("Export the meeting first.")
	}
	repo, folder := st.CheckoutPath, m.Folder
	date := m.StartedAt.In(time.Local).Format("2006-01-02")
	if out, err := s.git.Run(ctx, repo, "add", "--", folder); err != nil {
		return "", errors.New("The commit was rejected:\n" + out)
	}
	switch out, err := s.git.Run(ctx, repo, "diff", "--cached", "--quiet", "--", folder); port.ExitCode(err) {
	case 0:
		return "", errors.New("Nothing changed in " + folder + " since the last commit.")
	case 1:
	default:
		_, _ = s.git.Run(ctx, repo, "restore", "--staged", "--", folder)
		return "", errors.New("The commit was rejected:\n" + out)
	}
	msg := "meeting(" + m.ClientKey + "): " + date + " " + m.Title
	if out, err := s.git.Run(ctx, repo, "commit", "-m", msg, "--only", "--", folder); err != nil {
		_, _ = s.git.Run(ctx, repo, "restore", "--staged", "--", folder)
		return "", errors.New("The commit was rejected:\n" + out)
	}
	sha, err := s.git.Run(ctx, repo, "rev-parse", "--short", "HEAD")
	if err != nil {
		return "", errors.New("The commit was rejected:\n" + sha)
	}
	sha = strings.TrimSpace(sha)
	if out, err := s.git.Run(ctx, repo, "push", "-u", "origin", "HEAD"); err != nil {
		return "", errors.New("The commit " + sha + " was saved locally but not pushed:\n" + out)
	}
	return sha, nil
}
