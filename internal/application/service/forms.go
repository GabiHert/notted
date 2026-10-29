package service

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"log"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/GabiHert/notted/internal/application/port"
	"github.com/GabiHert/notted/internal/domain"
	"github.com/google/uuid"
)

const maxFormNote = 5000

// FormDeps are the collaborators of FormService.
type FormDeps struct {
	Meetings    port.MeetingStore
	Emit        port.EventEmitter
	Notify      port.Notifier // nil: no notifications
	Visible     func() bool   // nil: always visible
	SessionsDir string        // paths.SessionsDir()
	Executable  string        // the Notted binary (os.Executable())
	Socket      string        // paths.AskSocket()
	NewToken    func() string // nil: 32 bytes from crypto/rand, hex (64 characters)
	NewID       func() string // nil: uuid.NewString
}

// OpenForm is the open form of one meeting.
type OpenForm struct {
	ID        string
	MeetingID string
	Kind      domain.FormKind
	Form      domain.Form
	Queued    int // forms of the same meeting waiting behind this one
}

type pendingForm struct {
	id        string
	meetingID string
	title     string // the meeting's title
	kind      domain.FormKind
	form      domain.Form
	reply     chan domain.FormAnswer
	openedAt  time.Time
	openSeq   int
}

// FormService brokers the forms a meeting's session asks: one open form per meeting, the rest wait in order.
type FormService struct {
	d FormDeps

	mu      sync.Mutex
	tokens  map[string]string
	queues  map[string][]*pendingForm
	closed  bool
	openSeq int
}

var _ port.FormHandler = (*FormService)(nil)

// NewFormService builds the broker.
func NewFormService(d FormDeps) *FormService {
	if d.NewToken == nil {
		d.NewToken = randomToken
	}
	if d.NewID == nil {
		d.NewID = uuid.NewString
	}
	return &FormService{d: d, tokens: map[string]string{}, queues: map[string][]*pendingForm{}}
}

func randomToken() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b)
}

// effects are the things to do after the mutex is released.
type formEffects struct {
	notify []port.Notification
	remove []string
	emit   []string
}

func (e *formEffects) run(s *FormService) {
	if s.d.Notify != nil {
		for _, id := range e.remove {
			s.d.Notify.Remove(id)
		}
		for _, n := range e.notify {
			if err := s.d.Notify.Notify(n); err != nil {
				log.Printf("notify: %v", err)
			}
		}
	}
	if s.d.Emit != nil {
		for _, m := range e.emit {
			s.d.Emit.Emit("ask:changed", map[string]any{"meetingId": m})
		}
	}
}

// openLocked marks p as the open form of its meeting.
func (s *FormService) openLocked(p *pendingForm, fx *formEffects) {
	p.openedAt = time.Now()
	s.openSeq++
	p.openSeq = s.openSeq
	if s.d.Notify != nil && s.d.Visible != nil && !s.d.Visible() {
		fx.notify = append(fx.notify, port.Notification{ID: "ask-" + p.id, Title: "Waiting for you", Subtitle: p.title, Body: p.form.Title, MeetingID: p.meetingID})
	}
}

// popHeadLocked closes the open form of a meeting and opens the next one.
func (s *FormService) popHeadLocked(meetingID string, fx *formEffects) *pendingForm {
	q := s.queues[meetingID]
	head := q[0]
	fx.remove = append(fx.remove, "ask-"+head.id)
	s.setQueueLocked(meetingID, q[1:])
	if next := s.headLocked(meetingID); next != nil {
		s.openLocked(next, fx)
	}
	fx.emit = append(fx.emit, meetingID)
	return head
}

func (s *FormService) setQueueLocked(meetingID string, q []*pendingForm) {
	if len(q) == 0 {
		delete(s.queues, meetingID)
		return
	}
	s.queues[meetingID] = q
}

func (s *FormService) headLocked(meetingID string) *pendingForm {
	if q := s.queues[meetingID]; len(q) > 0 {
		return q[0]
	}
	return nil
}

// findHeadLocked finds the form with this id when it is the open form of its meeting.
func (s *FormService) findHeadLocked(formID string) *pendingForm {
	for _, q := range s.queues {
		if q[0].id == formID {
			return q[0]
		}
	}
	return nil
}

// Handle queues one form, waits for the person's answer and returns it.
func (s *FormService) Handle(ctx context.Context, req port.FormRequest) (domain.FormAnswer, error) {
	s.mu.Lock()
	want, ok := s.tokens[req.MeetingID]
	s.mu.Unlock()
	if !ok || subtle.ConstantTimeCompare([]byte(want), []byte(req.Token)) != 1 {
		return domain.FormAnswer{}, errors.New("Notted did not recognize this session. Start it again from Notted.")
	}
	s.mu.Lock()
	closed := s.closed
	s.mu.Unlock()
	if closed {
		return domain.Dismissed(""), nil
	}
	m, err := s.d.Meetings.GetMeeting(ctx, req.MeetingID)
	if errors.Is(err, domain.ErrNotFound) {
		return domain.FormAnswer{}, errors.New("This meeting no longer exists in Notted.")
	}
	if err != nil {
		return domain.FormAnswer{}, err
	}
	raw, err := json.Marshal(req.Form)
	if err != nil {
		return domain.FormAnswer{}, err
	}
	form, err := domain.ParseForm(raw)
	if err != nil {
		return domain.FormAnswer{}, err
	}

	p := &pendingForm{id: s.d.NewID(), meetingID: req.MeetingID, title: m.Title, kind: req.Kind, form: form, reply: make(chan domain.FormAnswer, 1)}
	var fx formEffects
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return domain.Dismissed(""), nil
	}
	s.queues[req.MeetingID] = append(s.queues[req.MeetingID], p)
	if len(s.queues[req.MeetingID]) == 1 {
		s.openLocked(p, &fx)
	}
	fx.emit = append(fx.emit, req.MeetingID)
	s.mu.Unlock()
	fx.run(s)

	select {
	case a := <-p.reply:
		return a, nil
	case <-ctx.Done():
	}
	var gone formEffects
	s.mu.Lock()
	q := s.queues[req.MeetingID]
	idx := -1
	for i, x := range q {
		if x == p {
			idx = i
			break
		}
	}
	switch {
	case idx == 0:
		s.popHeadLocked(req.MeetingID, &gone)
	case idx > 0:
		s.setQueueLocked(req.MeetingID, append(append([]*pendingForm{}, q[:idx]...), q[idx+1:]...))
		gone.emit = append(gone.emit, req.MeetingID)
	}
	s.mu.Unlock()
	gone.run(s)
	if idx < 0 {
		select {
		case a := <-p.reply:
			return a, nil
		default:
		}
	}
	return domain.FormAnswer{}, ctx.Err()
}

// Answer answers the open form with the person's answers.
func (s *FormService) Answer(formID string, answers map[string]any, note string) error {
	var fx formEffects
	s.mu.Lock()
	p := s.findHeadLocked(formID)
	if p == nil {
		s.mu.Unlock()
		return errors.New("This form is no longer open.")
	}
	a, err := domain.NewAnswer(p.form, answers, note)
	if err != nil {
		s.mu.Unlock()
		return err
	}
	s.popHeadLocked(p.meetingID, &fx)
	s.mu.Unlock()
	p.reply <- a
	fx.run(s)
	return nil
}

// Dismiss answers the open form with "Not now".
func (s *FormService) Dismiss(formID string, note string) error {
	var fx formEffects
	s.mu.Lock()
	p := s.findHeadLocked(formID)
	if p == nil {
		s.mu.Unlock()
		return errors.New("This form is no longer open.")
	}
	if utf8.RuneCountInString(note) > maxFormNote {
		s.mu.Unlock()
		return errors.New("The note is longer than 5000 characters.")
	}
	s.popHeadLocked(p.meetingID, &fx)
	s.mu.Unlock()
	p.reply <- domain.Dismissed(note)
	fx.run(s)
	return nil
}

// Open returns the open form of every meeting that has one, oldest first.
func (s *FormService) Open() []OpenForm {
	s.mu.Lock()
	defer s.mu.Unlock()
	heads := make([]*pendingForm, 0, len(s.queues))
	for _, q := range s.queues {
		heads = append(heads, q[0])
	}
	sort.Slice(heads, func(i, j int) bool { return heads[i].openSeq < heads[j].openSeq })
	out := make([]OpenForm, 0, len(heads))
	for _, h := range heads {
		out = append(out, OpenForm{ID: h.id, MeetingID: h.meetingID, Kind: h.kind, Form: h.form, Queued: len(s.queues[h.meetingID]) - 1})
	}
	return out
}

// CloseAll dismisses every form, open or waiting, and dismisses later requests at once.
func (s *FormService) CloseAll() {
	var fx formEffects
	var all []*pendingForm
	s.mu.Lock()
	s.closed = true
	for id, q := range s.queues {
		all = append(all, q...)
		fx.remove = append(fx.remove, "ask-"+q[0].id)
		fx.emit = append(fx.emit, id)
	}
	s.queues = map[string][]*pendingForm{}
	s.mu.Unlock()
	for _, p := range all {
		p.reply <- domain.Dismissed("")
	}
	fx.run(s)
}

// DismissMeeting answers every form of the meeting, open or waiting, with "Not now", and forgets the meeting's
// session token, so a later request from its session is refused. A meeting with neither is a no-op.
func (s *FormService) DismissMeeting(meetingID string) {
	var fx formEffects
	s.mu.Lock()
	q := s.queues[meetingID]
	delete(s.queues, meetingID)
	delete(s.tokens, meetingID)
	if len(q) > 0 {
		fx.remove = append(fx.remove, "ask-"+q[0].id)
		fx.emit = append(fx.emit, meetingID)
	}
	s.mu.Unlock()
	for _, p := range q {
		p.reply <- domain.Dismissed("")
	}
	fx.run(s)
}

type mcpServerConfig struct {
	Command string   `json:"command"`
	Args    []string `json:"args"`
}

type mcpConfig struct {
	MCPServers struct {
		Notted mcpServerConfig `json:"notted"`
	} `json:"mcpServers"`
}

// SessionConfig writes the session's MCP config for one meeting and returns its path.
func (s *FormService) SessionConfig(meetingID string) (string, error) {
	if !domain.ValidMeetingID(meetingID) {
		return "", errors.New("Meeting not found.")
	}
	if s.d.Executable == "" {
		return "", errors.New("Notted could not find its own program file.")
	}
	s.mu.Lock()
	tok, ok := s.tokens[meetingID]
	if !ok {
		tok = s.d.NewToken()
		s.tokens[meetingID] = tok
	}
	s.mu.Unlock()

	var cfg mcpConfig
	cfg.MCPServers.Notted = mcpServerConfig{Command: s.d.Executable, Args: []string{"mcp", "--meeting", meetingID, "--socket", s.d.Socket, "--token", tok}}
	b, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return "", err
	}
	b = append(b, '\n')

	dir := filepath.Join(s.d.SessionsDir, meetingID)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	if err := os.Chmod(dir, 0o700); err != nil {
		return "", err
	}
	tmp, err := os.CreateTemp(dir, "mcp-*.json")
	if err != nil {
		return "", err
	}
	name := tmp.Name()
	fail := func(err error) (string, error) {
		_ = tmp.Close()
		_ = os.Remove(name)
		return "", err
	}
	if err := os.Chmod(name, 0o600); err != nil {
		return fail(err)
	}
	if _, err := tmp.Write(b); err != nil {
		return fail(err)
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(name)
		return "", err
	}
	path := filepath.Join(dir, "mcp.json")
	if err := os.Rename(name, path); err != nil {
		_ = os.Remove(name)
		return "", err
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return path, nil
	}
	return abs, nil
}
