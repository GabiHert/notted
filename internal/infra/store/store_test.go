package store

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"testing"
	"time"

	"github.com/bhlabz/notted/internal/application/port"
	"github.com/bhlabz/notted/internal/domain"
)

var (
	_ port.MeetingStore  = (*Store)(nil)
	_ port.SettingsStore = (*Store)(nil)
)

func openTemp(t *testing.T) (*Store, string) {
	t.Helper()
	t.Setenv("NOTTED_HOME", t.TempDir())
	path := filepath.Join(t.TempDir(), "sub", "meetings.sqlite")
	s, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s, path
}

func meeting(id string, started time.Time) domain.Meeting {
	return domain.Meeting{ID: id, Title: "t-" + id, StartedAt: started, Language: domain.LangEN, State: domain.StateRecording}
}

func seed(t *testing.T, s *Store, id string, started time.Time) {
	t.Helper()
	if err := s.CreateMeeting(context.Background(), meeting(id, started)); err != nil {
		t.Fatalf("CreateMeeting: %v", err)
	}
}

func names(t *testing.T, s *Store, q string) []string {
	t.Helper()
	rows, err := s.db.Query(q)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var n string
		if err := rows.Scan(&n); err != nil {
			t.Fatal(err)
		}
		out = append(out, n)
	}
	sort.Strings(out)
	return out
}

func TestOpenCreatesSchema(t *testing.T) {
	s, path := openTemp(t)
	st, err := os.Stat(filepath.Dir(path))
	if err != nil || st.Mode().Perm() != 0o700 {
		t.Fatalf("parent folder mode = %v, err %v; want 0700", st.Mode().Perm(), err)
	}
	tables := names(t, s, `SELECT name FROM sqlite_master WHERE type='table' AND name NOT LIKE 'sqlite_%'`)
	wantTables := []string{"chunks", "meetings", "schema_migrations", "segments", "settings"}
	if !reflect.DeepEqual(tables, wantTables) {
		t.Fatalf("tables = %v, want %v", tables, wantTables)
	}
	cols := map[string][]string{
		"meetings":          {"claude_session_id", "client_key", "ended_at", "folder", "id", "language", "started_at", "state", "suggestions_md", "summary_md", "title"},
		"chunks":            {"end_ms", "idx", "meeting_id", "path", "start_ms", "track", "transcribed"},
		"segments":          {"end_ms", "id", "meeting_id", "start_ms", "text", "track"},
		"settings":          {"key", "value"},
		"schema_migrations": {"applied_at", "version"},
	}
	for tbl, want := range cols {
		got := names(t, s, `SELECT name FROM pragma_table_info('`+tbl+`')`)
		if !reflect.DeepEqual(got, want) {
			t.Errorf("%s columns = %v, want %v", tbl, got, want)
		}
	}
	trig := names(t, s, `SELECT name FROM sqlite_master WHERE type='trigger'`)
	wantTrig := []string{"meetings_folder_write_once", "meetings_session_id_write_once"}
	if !reflect.DeepEqual(trig, wantTrig) {
		t.Fatalf("triggers = %v, want %v", trig, wantTrig)
	}
}

func TestMigrationsApplyOnce(t *testing.T) {
	_, path := openTemp(t)
	s2, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s2.Close()
	var n, v int
	if err := s2.db.QueryRow(`SELECT COUNT(*), MAX(version) FROM schema_migrations`).Scan(&n, &v); err != nil {
		t.Fatal(err)
	}
	if n != 1 || v != 1 {
		t.Fatalf("schema_migrations count=%d max=%d, want 1 and 1", n, v)
	}
}

func TestChecksRejectBadValues(t *testing.T) {
	s, _ := openTemp(t)
	ctx := context.Background()
	m := meeting("m1", time.Now())
	m.State = "done"
	if err := s.CreateMeeting(ctx, m); err == nil {
		t.Error("state done accepted")
	}
	m = meeting("m1", time.Now())
	m.Language = "pt"
	if err := s.CreateMeeting(ctx, m); err == nil {
		t.Error("language pt accepted")
	}
	seed(t, s, "m2", time.Now())
	if err := s.AddChunk(ctx, domain.Chunk{MeetingID: "m2", Track: "system", Index: 1, Path: "/x"}); err == nil {
		t.Error("track system accepted")
	}
	if err := s.AddChunk(ctx, domain.Chunk{MeetingID: "m2", Track: domain.TrackMic, Index: 0, Path: "/x"}); err == nil {
		t.Error("idx 0 accepted")
	}
	if err := s.AddChunk(ctx, domain.Chunk{MeetingID: "nope", Track: domain.TrackMic, Index: 1, Path: "/x"}); err == nil {
		t.Error("chunk for unknown meeting accepted")
	}
}

func TestMeetingRoundTrip(t *testing.T) {
	s, _ := openTemp(t)
	ctx := context.Background()
	t0 := time.Date(2026, 10, 1, 9, 0, 0, 123, time.UTC)
	older := meeting("older", t0)
	older.Title = "Weekly"
	older.Language = domain.LangPTBR
	if err := s.CreateMeeting(ctx, older); err != nil {
		t.Fatal(err)
	}
	seed(t, s, "newer", t0.Add(time.Hour))

	got, err := s.GetMeeting(ctx, "older")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, older) {
		t.Fatalf("got %+v, want %+v", got, older)
	}
	list, err := s.ListMeetings(ctx)
	if err != nil || len(list) != 2 || list[0].ID != "newer" || list[1].ID != "older" {
		t.Fatalf("list = %+v err %v, want newer then older", list, err)
	}

	end := t0.Add(30 * time.Minute)
	for _, err := range []error{
		s.UpdateState(ctx, "older", domain.StateReady),
		s.SetEndedAt(ctx, "older", end.In(time.FixedZone("x", 3600))),
		s.SetClientKey(ctx, "older", "acme"),
		s.SetSummary(ctx, "older", "# sum"),
		s.SetSuggestions(ctx, "older", "- a"),
	} {
		if err != nil {
			t.Fatal(err)
		}
	}
	got, _ = s.GetMeeting(ctx, "older")
	if got.State != domain.StateReady || got.ClientKey != "acme" || got.SummaryMD != "# sum" || got.SuggestionsMD != "- a" {
		t.Fatalf("updates not stored: %+v", got)
	}
	if got.EndedAt == nil || !got.EndedAt.Equal(end) || got.EndedAt.Location() != time.UTC {
		t.Fatalf("EndedAt = %v, want %v in UTC", got.EndedAt, end)
	}

	if _, err := s.GetMeeting(ctx, "zzz"); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("Get missing = %v", err)
	}
	for name, err := range map[string]error{
		"state":    s.UpdateState(ctx, "zzz", domain.StateReady),
		"ended":    s.SetEndedAt(ctx, "zzz", end),
		"client":   s.SetClientKey(ctx, "zzz", "acme"),
		"session":  s.SetClaudeSessionID(ctx, "zzz", "s"),
		"folder":   s.SetFolder(ctx, "zzz", "f"),
		"summary":  s.SetSummary(ctx, "zzz", "x"),
		"sugg":     s.SetSuggestions(ctx, "zzz", "x"),
		"markdone": s.MarkChunkTranscribed(ctx, "zzz", domain.TrackMic, 1),
	} {
		if !errors.Is(err, domain.ErrNotFound) {
			t.Errorf("%s on missing meeting = %v, want ErrNotFound", name, err)
		}
	}
}

func TestSessionIDWriteOnce(t *testing.T) {
	s, _ := openTemp(t)
	ctx := context.Background()
	seed(t, s, "m", time.Now())
	if err := s.SetClaudeSessionID(ctx, "m", "s1"); err != nil {
		t.Fatal(err)
	}
	if err := s.SetClaudeSessionID(ctx, "m", "s1"); err != nil {
		t.Fatalf("same value: %v", err)
	}
	if err := s.SetClaudeSessionID(ctx, "m", "s2"); !errors.Is(err, domain.ErrSessionIDAlreadySet) {
		t.Fatalf("different value = %v", err)
	}
	if got, _ := s.GetMeeting(ctx, "m"); got.ClaudeSessionID != "s1" {
		t.Fatalf("row changed: %q", got.ClaudeSessionID)
	}
	if _, err := s.db.Exec(`UPDATE meetings SET claude_session_id='s3' WHERE id='m'`); err == nil {
		t.Fatal("raw UPDATE not aborted")
	}
	if got, _ := s.GetMeeting(ctx, "m"); got.ClaudeSessionID != "s1" {
		t.Fatalf("row changed by raw update: %q", got.ClaudeSessionID)
	}
}

func TestFolderWriteOnce(t *testing.T) {
	s, _ := openTemp(t)
	ctx := context.Background()
	seed(t, s, "m", time.Now())
	if err := s.SetFolder(ctx, "m", "acme/2026-10-01"); err != nil {
		t.Fatal(err)
	}
	if err := s.SetFolder(ctx, "m", "acme/2026-10-01"); err != nil {
		t.Fatalf("same value: %v", err)
	}
	if err := s.SetFolder(ctx, "m", "globex/x"); !errors.Is(err, domain.ErrFolderAlreadySet) {
		t.Fatalf("different value = %v", err)
	}
	if got, _ := s.GetMeeting(ctx, "m"); got.Folder != "acme/2026-10-01" {
		t.Fatalf("row changed: %q", got.Folder)
	}
	if _, err := s.db.Exec(`UPDATE meetings SET folder='other' WHERE id='m'`); err == nil {
		t.Fatal("raw UPDATE not aborted")
	}
}

func TestChunksAndPending(t *testing.T) {
	s, _ := openTemp(t)
	ctx := context.Background()
	t0 := time.Date(2026, 10, 1, 9, 0, 0, 0, time.UTC)
	seed(t, s, "late", t0.Add(time.Hour))
	seed(t, s, "early", t0)
	add := func(m string, tr domain.Track, idx int) {
		t.Helper()
		c := domain.Chunk{MeetingID: m, Track: tr, Index: idx, StartMs: int64(idx) * 1000, EndMs: int64(idx)*1000 + 900, Path: "/a/" + m}
		if err := s.AddChunk(ctx, c); err != nil {
			t.Fatal(err)
		}
	}
	add("late", domain.TrackOthers, 1)
	add("late", domain.TrackMic, 1)
	add("early", domain.TrackOthers, 2)
	add("early", domain.TrackMic, 2)
	add("early", domain.TrackOthers, 1)
	add("early", domain.TrackMic, 1)

	if err := s.AddChunk(ctx, domain.Chunk{MeetingID: "early", Track: domain.TrackMic, Index: 1, Path: "/dup"}); err == nil {
		t.Fatal("duplicate key accepted")
	}
	key := func(cs []domain.Chunk) []string {
		var o []string
		for _, c := range cs {
			o = append(o, c.MeetingID+"/"+string(c.Track)+"/"+string(rune('0'+c.Index)))
		}
		return o
	}
	list, err := s.ListChunks(ctx, "early")
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"early/mic/1", "early/others/1", "early/mic/2", "early/others/2"}
	if !reflect.DeepEqual(key(list), want) {
		t.Fatalf("ListChunks = %v, want %v", key(list), want)
	}
	if err := s.MarkChunkTranscribed(ctx, "early", domain.TrackMic, 1); err != nil {
		t.Fatal(err)
	}
	if err := s.MarkChunkTranscribed(ctx, "early", domain.TrackMic, 99); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("mark missing chunk = %v", err)
	}
	pend, err := s.PendingChunks(ctx)
	if err != nil {
		t.Fatal(err)
	}
	want = []string{"early/others/1", "early/mic/2", "early/others/2", "late/mic/1", "late/others/1"}
	if !reflect.DeepEqual(key(pend), want) {
		t.Fatalf("PendingChunks = %v, want %v", key(pend), want)
	}
	list, _ = s.ListChunks(ctx, "early")
	if !list[0].Transcribed || list[1].Transcribed {
		t.Fatalf("transcribed flags wrong: %+v", list)
	}
}

func TestSegmentsOrder(t *testing.T) {
	s, _ := openTemp(t)
	ctx := context.Background()
	seed(t, s, "m", time.Now())
	in := []domain.Segment{
		{MeetingID: "m", Track: domain.TrackOthers, StartMs: 500, EndMs: 900, Text: "c"},
		{MeetingID: "m", Track: domain.TrackOthers, StartMs: 100, EndMs: 200, Text: "b"},
		{MeetingID: "m", Track: domain.TrackMic, StartMs: 100, EndMs: 300, Text: "a"},
		{MeetingID: "m", Track: domain.TrackMic, StartMs: 100, EndMs: 400, Text: "a2"},
	}
	var ids []int64
	for _, sg := range in {
		id, err := s.AddSegment(ctx, sg)
		if err != nil || id == 0 {
			t.Fatalf("AddSegment id %d err %v", id, err)
		}
		ids = append(ids, id)
	}
	got, err := s.ListSegments(ctx, "m")
	if err != nil {
		t.Fatal(err)
	}
	var texts []string
	for _, sg := range got {
		texts = append(texts, sg.Text)
	}
	if want := []string{"a", "a2", "b", "c"}; !reflect.DeepEqual(texts, want) {
		t.Fatalf("order = %v, want %v", texts, want)
	}
	if got[0].ID != ids[2] || got[0].MeetingID != "m" || got[0].EndMs != 300 {
		t.Fatalf("segment fields: %+v", got[0])
	}
}

func TestSettingsRoundTrip(t *testing.T) {
	s, _ := openTemp(t)
	ctx := context.Background()
	got, err := s.GetSettings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if got.Profile != "" || got.CheckoutPath != "" || got.ProjectRoots == nil || len(got.ProjectRoots) != 0 {
		t.Fatalf("empty read = %+v", got)
	}
	in := domain.Settings{Profile: "work", CheckoutPath: "/co", ProjectRoots: map[string]string{"acme": "/p/acme", "globex": "/p/globex"}}
	if err := s.SaveSettings(ctx, in); err != nil {
		t.Fatal(err)
	}
	got, _ = s.GetSettings(ctx)
	if !reflect.DeepEqual(got, in) {
		t.Fatalf("got %+v, want %+v", got, in)
	}
	in2 := domain.Settings{Profile: "home", CheckoutPath: "", ProjectRoots: map[string]string{"acme": "/q"}}
	if err := s.SaveSettings(ctx, in2); err != nil {
		t.Fatal(err)
	}
	got, _ = s.GetSettings(ctx)
	if !reflect.DeepEqual(got, in2) {
		t.Fatalf("overwrite got %+v, want %+v", got, in2)
	}
	if err := s.SaveSettings(ctx, domain.Settings{}); err != nil {
		t.Fatal(err)
	}
	got, _ = s.GetSettings(ctx)
	if got.ProjectRoots == nil || len(got.ProjectRoots) != 0 {
		t.Fatalf("nil roots read back as %v", got.ProjectRoots)
	}
}
