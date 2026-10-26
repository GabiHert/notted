package store

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"testing"
	"time"

	"github.com/GabiHert/notted/internal/application/port"
	"github.com/GabiHert/notted/internal/domain"
)

var (
	_ port.MeetingStore  = (*Store)(nil)
	_ port.SettingsStore = (*Store)(nil)
	_ port.SourceStore   = (*Store)(nil)
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
		"meetings":          {"claude_session_id", "ended_at", "folder", "id", "language", "project_key", "started_at", "state", "suggestions_md", "summary_md", "title"},
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
	if n != 4 || v != 4 {
		t.Fatalf("schema_migrations count=%d max=%d, want 4 and 4", n, v)
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
		s.SetProjectKey(ctx, "older", "acme"),
		s.SetSummary(ctx, "older", "# sum"),
		s.SetSuggestions(ctx, "older", "- a"),
	} {
		if err != nil {
			t.Fatal(err)
		}
	}
	got, _ = s.GetMeeting(ctx, "older")
	if got.State != domain.StateReady || got.ProjectKey != "acme" || got.SummaryMD != "# sum" || got.SuggestionsMD != "- a" {
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
		"project":  s.SetProjectKey(ctx, "zzz", "acme"),
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

func versions(t *testing.T, s *Store) []int {
	t.Helper()
	rows, err := s.db.Query(`SELECT version FROM schema_migrations ORDER BY version`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var out []int
	for rows.Next() {
		var v int
		if err := rows.Scan(&v); err != nil {
			t.Fatal(err)
		}
		out = append(out, v)
	}
	return out
}

func TestMigration2RenamesMeetingKeyColumn(t *testing.T) {
	t.Setenv("NOTTED_HOME", t.TempDir())
	path := filepath.Join(t.TempDir(), "meetings.sqlite")
	old, err := sql.Open("sqlite3", path)
	if err != nil {
		t.Fatal(err)
	}
	for _, q := range []string{
		`CREATE TABLE schema_migrations (version INTEGER PRIMARY KEY, applied_at TEXT NOT NULL)`,
		migrations[0],
		`INSERT INTO schema_migrations (version, applied_at) VALUES (1, '2026-01-01T00:00:00Z')`,
		`INSERT INTO meetings (id, title, started_at, language, client_key, state) VALUES ('m1', 'Old', '2026-01-01T10:00:00Z', 'en', 'acme', 'ready')`,
	} {
		if _, err := old.Exec(q); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}
	if err := old.Close(); err != nil {
		t.Fatal(err)
	}

	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	m, err := s.GetMeeting(context.Background(), "m1")
	if err != nil || m.ProjectKey != "acme" {
		t.Fatalf("meeting = %+v, err %v; want ProjectKey acme", m, err)
	}
	cols := names(t, s, `SELECT name FROM pragma_table_info('meetings')`)
	hasProject, hasOld := false, false
	for _, c := range cols {
		hasProject = hasProject || c == "project_key"
		hasOld = hasOld || c == "client_key"
	}
	if !hasProject || hasOld {
		t.Fatalf("meetings columns = %v, want project_key and no client_key", cols)
	}
	if got := versions(t, s); !reflect.DeepEqual(got, []int{1, 2, 3, 4}) {
		t.Fatalf("versions = %v, want [1 2 3 4]", got)
	}
	s.Close()

	s2, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s2.Close()
	if got := versions(t, s2); !reflect.DeepEqual(got, []int{1, 2, 3, 4}) {
		t.Fatalf("second open versions = %v, want [1 2 3 4]", got)
	}
}

func TestSourcePathRoundTrip(t *testing.T) {
	s, _ := openTemp(t)
	ctx := context.Background()
	if got, err := s.GetSourcePath(ctx); err != nil || got != "" {
		t.Fatalf("first = %q, %v", got, err)
	}
	for _, p := range []string{"/a", "/b"} {
		if err := s.SetSourcePath(ctx, p); err != nil {
			t.Fatal(err)
		}
	}
	if got, _ := s.GetSourcePath(ctx); got != "/b" {
		t.Fatalf("got %q", got)
	}
	want := domain.Settings{Profile: "ana", CheckoutPath: "/c", ProjectRoots: map[string]string{"acme": "/p/acme"}}
	if err := s.SaveSettings(ctx, want); err != nil {
		t.Fatal(err)
	}
	if got, _ := s.GetSourcePath(ctx); got != "/b" {
		t.Fatalf("SaveSettings touched the source: %q", got)
	}
	got, err := s.GetSettings(ctx)
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("settings = %+v, %v", got, err)
	}
}

func TestMigration3KeepsSettings(t *testing.T) {
	t.Setenv("NOTTED_HOME", t.TempDir())
	all := migrations
	t.Cleanup(func() { migrations = all })
	migrations = all[:2]
	path := filepath.Join(t.TempDir(), "meetings.sqlite")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	want := domain.Settings{Profile: "ana", CheckoutPath: "/a", ProjectRoots: map[string]string{"acme": "/p/acme"}}
	if err := s.SaveSettings(ctx, want); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	migrations = all
	s, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	got, err := s.GetSettings(ctx)
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("settings = %+v, %v", got, err)
	}
	if err := s.SetSourcePath(ctx, "/src"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(`INSERT INTO settings (key, value) VALUES ('bogus', 'x')`); err == nil {
		t.Error("bogus key accepted")
	}
	var n int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM schema_migrations`).Scan(&n); err != nil || n != 4 {
		t.Fatalf("schema_migrations rows = %d, %v", n, err)
	}
}

func TestDeleteMeetingCascadesToChunksAndSegments(t *testing.T) {
	s, _ := openTemp(t)
	ctx := context.Background()
	now := time.Date(2026, 10, 25, 10, 0, 0, 0, time.UTC)
	seed(t, s, "gone", now)
	seed(t, s, "kept", now.Add(time.Hour))
	for _, id := range []string{"gone", "kept"} {
		if err := s.AddChunk(ctx, domain.Chunk{MeetingID: id, Track: domain.TrackMic, Index: 1, Path: "/x/" + id}); err != nil {
			t.Fatal(err)
		}
		if _, err := s.AddSegment(ctx, domain.Segment{MeetingID: id, Track: domain.TrackMic, Text: "hi"}); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.DeleteMeeting(ctx, "gone"); err != nil {
		t.Fatalf("DeleteMeeting: %v", err)
	}
	if _, err := s.GetMeeting(ctx, "gone"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("GetMeeting after delete err = %v, want ErrNotFound", err)
	}
	for _, q := range []string{`SELECT COUNT(*) FROM chunks WHERE meeting_id='gone'`, `SELECT COUNT(*) FROM segments WHERE meeting_id='gone'`} {
		var n int
		if err := s.db.QueryRow(q).Scan(&n); err != nil || n != 0 {
			t.Fatalf("%s = %d, %v; want 0", q, n, err)
		}
	}
	if cs, _ := s.ListChunks(ctx, "kept"); len(cs) != 1 {
		t.Fatalf("kept chunks = %d, want 1", len(cs))
	}
	if ss, _ := s.ListSegments(ctx, "kept"); len(ss) != 1 {
		t.Fatalf("kept segments = %d, want 1", len(ss))
	}
	if pend, _ := s.PendingChunks(ctx); len(pend) != 1 || pend[0].MeetingID != "kept" {
		t.Fatalf("pending = %+v, want only the kept meeting's chunk", pend)
	}
}

func TestDeleteMeetingMissingIsNotFound(t *testing.T) {
	s, _ := openTemp(t)
	if err := s.DeleteMeeting(context.Background(), "nope"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

func TestDeleteMeetingFailureRollsBack(t *testing.T) {
	s, _ := openTemp(t)
	ctx := context.Background()
	seed(t, s, "a", time.Date(2026, 10, 25, 10, 0, 0, 0, time.UTC))
	if err := s.AddChunk(ctx, domain.Chunk{MeetingID: "a", Track: domain.TrackMic, Index: 1, Path: "/x"}); err != nil {
		t.Fatal(err)
	}
	// A trigger that aborts the meeting delete after the chunks and segments went: the whole delete must roll back.
	if _, err := s.db.Exec(`CREATE TRIGGER block_delete BEFORE DELETE ON meetings BEGIN SELECT RAISE(ABORT, 'blocked'); END;`); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteMeeting(ctx, "a"); err == nil {
		t.Fatal("DeleteMeeting must fail when the meeting row cannot be deleted")
	}
	if cs, _ := s.ListChunks(ctx, "a"); len(cs) != 1 {
		t.Fatalf("chunks after a failed delete = %d, want 1 (rolled back)", len(cs))
	}
}

func TestReviewActWithoutAskingRoundTrip(t *testing.T) {
	s, _ := openTemp(t)
	ctx := context.Background()
	if err := s.SaveSettings(ctx, domain.Settings{Profile: "ana", CheckoutPath: "/a", ProjectRoots: map[string]string{"acme": "/p"}}); err != nil {
		t.Fatal(err)
	}
	before, err := s.GetSettings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if on, err := s.GetReviewActWithoutAsking(ctx); err != nil || on {
		t.Fatalf("fresh = %v, %v", on, err)
	}
	raw := func() string {
		var v string
		if err := s.db.QueryRow(`SELECT value FROM settings WHERE key = 'review_act_without_asking'`).Scan(&v); err != nil {
			t.Fatal(err)
		}
		return v
	}
	if err := s.SetReviewActWithoutAsking(ctx, true); err != nil {
		t.Fatal(err)
	}
	if on, err := s.GetReviewActWithoutAsking(ctx); err != nil || !on || raw() != "1" {
		t.Fatalf("after true = %v, %v, raw %q", on, err, raw())
	}
	if err := s.SetReviewActWithoutAsking(ctx, false); err != nil {
		t.Fatal(err)
	}
	if on, err := s.GetReviewActWithoutAsking(ctx); err != nil || on || raw() != "0" {
		t.Fatalf("after false = %v, %v, raw %q", on, err, raw())
	}
	after, err := s.GetSettings(ctx)
	if err != nil || !reflect.DeepEqual(before, after) {
		t.Fatalf("settings changed: %+v vs %+v, %v", before, after, err)
	}
}

func TestMigration4KeepsSettings(t *testing.T) {
	t.Setenv("NOTTED_HOME", t.TempDir())
	all := migrations
	t.Cleanup(func() { migrations = all })
	migrations = all[:3]
	path := filepath.Join(t.TempDir(), "meetings.sqlite")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	want := domain.Settings{Profile: "ana", CheckoutPath: "/a", ProjectRoots: map[string]string{"acme": "/p/acme"}}
	if err := s.SaveSettings(ctx, want); err != nil {
		t.Fatal(err)
	}
	if err := s.SetSourcePath(ctx, "/src"); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	migrations = all
	s, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	got, err := s.GetSettings(ctx)
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("settings = %+v, %v", got, err)
	}
	if src, err := s.GetSourcePath(ctx); err != nil || src != "/src" {
		t.Fatalf("source path = %q, %v", src, err)
	}
	if on, err := s.GetReviewActWithoutAsking(ctx); err != nil || on {
		t.Fatalf("flag = %v, %v", on, err)
	}
	if _, err := s.db.Exec(`INSERT INTO settings (key, value) VALUES ('bogus', 'x')`); err == nil {
		t.Error("bogus key accepted")
	}
	var n int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM schema_migrations`).Scan(&n); err != nil || n != 4 {
		t.Fatalf("schema_migrations rows = %d, %v", n, err)
	}
}
