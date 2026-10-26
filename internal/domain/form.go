package domain

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"
)

// PieceKind names one kind of block in a form.
type PieceKind string

const (
	PieceText    PieceKind = "text"
	PieceHTML    PieceKind = "html"
	PieceChoice  PieceKind = "choice"
	PieceMulti   PieceKind = "multi"
	PieceInput   PieceKind = "input"
	PieceConfirm PieceKind = "confirm"
)

// FormKind tells where a form came from.
type FormKind string

const (
	FormAsk        FormKind = "ask"
	FormPermission FormKind = "permission"
)

// FormStatus is how a form ended.
type FormStatus string

const (
	StatusAnswered  FormStatus = "answered"
	StatusDismissed FormStatus = "dismissed"
)

// Option is one choice of a choice or multi piece.
type Option struct {
	Label       string `json:"label"`
	Description string `json:"description,omitempty"`
	Recommended bool   `json:"recommended,omitempty"`
}

// Piece is one block of a form.
type Piece struct {
	Kind        PieceKind `json:"kind"`
	ID          string    `json:"id"`
	Markdown    string    `json:"markdown,omitempty"`
	HTML        string    `json:"html,omitempty"`
	Question    string    `json:"question,omitempty"`
	Options     []Option  `json:"options,omitempty"`
	Placeholder string    `json:"placeholder,omitempty"`
	Multiline   bool      `json:"multiline,omitempty"`
	Required    bool      `json:"required,omitempty"`
	YesLabel    string    `json:"yes_label,omitempty"`
	NoLabel     string    `json:"no_label,omitempty"`
}

// Form is a titled list of pieces.
type Form struct {
	Title  string  `json:"title"`
	Pieces []Piece `json:"pieces"`
}

// FormAnswer is the result handed back to Claude. Answers values are string or []string.
type FormAnswer struct {
	Status  FormStatus     `json:"status"`
	Answers map[string]any `json:"answers"`
	Note    string         `json:"note"`
}

// ErrInvalidForm wraps every refusal of ParseForm.
var ErrInvalidForm = errors.New("invalid form")

const (
	maxFormBytes = 1048576
	maxNote      = 5000
)

var idPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,39}$`)

type wireOption struct {
	Label       string `json:"label"`
	Description string `json:"description"`
	Recommended *bool  `json:"recommended"`
}

type wirePiece struct {
	Kind        string        `json:"kind"`
	ID          string        `json:"id"`
	Markdown    *string       `json:"markdown"`
	HTML        *string       `json:"html"`
	Question    *string       `json:"question"`
	Options     *[]wireOption `json:"options"`
	Placeholder *string       `json:"placeholder"`
	Multiline   *bool         `json:"multiline"`
	Required    *bool         `json:"required"`
	YesLabel    *string       `json:"yes_label"`
	NoLabel     *string       `json:"no_label"`
}

type wireForm struct {
	Title  string      `json:"title"`
	Pieces []wirePiece `json:"pieces"`
}

func invalid(format string, args ...any) error {
	return fmt.Errorf("%w: %s", ErrInvalidForm, fmt.Sprintf(format, args...))
}

func runes(s string) int { return utf8.RuneCountInString(s) }

// allowedFields lists, per kind, the optional fields a piece may carry.
var allowedFields = map[string]map[string]bool{
	"text":    {"markdown": true},
	"html":    {"html": true},
	"choice":  {"question": true, "options": true},
	"multi":   {"question": true, "options": true},
	"input":   {"question": true, "placeholder": true, "multiline": true, "required": true},
	"confirm": {"question": true, "yes_label": true, "no_label": true},
}

// ParseForm decodes and validates a form. Every refusal wraps ErrInvalidForm.
func ParseForm(raw []byte) (Form, error) {
	if len(raw) > maxFormBytes {
		return Form{}, invalid("the form is larger than 1 MB")
	}
	var w wireForm
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&w); err != nil {
		return Form{}, invalid("the form is not valid JSON: %v", err)
	}
	if _, err := dec.Token(); err != io.EOF {
		return Form{}, invalid("the form is not valid JSON: unexpected data after the form")
	}
	if strings.TrimSpace(w.Title) == "" {
		return Form{}, invalid("title is required")
	}
	if runes(w.Title) > 200 {
		return Form{}, invalid("title is longer than 200 characters")
	}
	if len(w.Pieces) < 1 || len(w.Pieces) > 20 {
		return Form{}, invalid("a form needs 1 to 20 pieces")
	}
	form := Form{Title: w.Title, Pieces: make([]Piece, 0, len(w.Pieces))}
	seen := map[string]bool{}
	for i, wp := range w.Pieces {
		if !idPattern.MatchString(wp.ID) {
			return Form{}, invalid("piece %d: id must be 1 to 40 lowercase letters, digits, dashes or underscores, starting with a letter or digit", i+1)
		}
		if seen[wp.ID] {
			return Form{}, invalid("piece %q: the id is used twice", wp.ID)
		}
		seen[wp.ID] = true
		p, err := buildPiece(wp)
		if err != nil {
			return Form{}, err
		}
		form.Pieces = append(form.Pieces, p)
	}
	return form, nil
}

func buildPiece(wp wirePiece) (Piece, error) {
	id := fmt.Sprintf("%q", wp.ID)
	allowed, ok := allowedFields[wp.Kind]
	if !ok {
		return Piece{}, invalid("piece %s: unknown kind \"%s\"; use text, html, choice, multi, input or confirm", id, wp.Kind)
	}
	present := []struct {
		name string
		has  bool
	}{
		{"markdown", wp.Markdown != nil},
		{"html", wp.HTML != nil},
		{"question", wp.Question != nil},
		{"options", wp.Options != nil},
		{"placeholder", wp.Placeholder != nil},
		{"multiline", wp.Multiline != nil},
		{"required", wp.Required != nil},
		{"yes_label", wp.YesLabel != nil},
		{"no_label", wp.NoLabel != nil},
	}
	for _, f := range present {
		if f.has && !allowed[f.name] {
			return Piece{}, invalid("piece %s: %s is not allowed for kind %s", id, f.name, wp.Kind)
		}
	}
	p := Piece{Kind: PieceKind(wp.Kind), ID: wp.ID}
	switch p.Kind {
	case PieceText:
		md := deref(wp.Markdown)
		if md == "" {
			return Piece{}, invalid("piece %s: markdown is required", id)
		}
		if runes(md) > 50000 {
			return Piece{}, invalid("piece %s: markdown is longer than 50000 characters", id)
		}
		p.Markdown = md
		return p, nil
	case PieceHTML:
		h := deref(wp.HTML)
		if h == "" {
			return Piece{}, invalid("piece %s: html is required", id)
		}
		if len(h) > 200000 {
			return Piece{}, invalid("piece %s: html is larger than 200000 bytes", id)
		}
		p.HTML = h
		return p, nil
	}
	q := deref(wp.Question)
	if strings.TrimSpace(q) == "" {
		return Piece{}, invalid("piece %s: question is required", id)
	}
	if runes(q) > 500 {
		return Piece{}, invalid("piece %s: question is longer than 500 characters", id)
	}
	p.Question = q
	switch p.Kind {
	case PieceChoice, PieceMulti:
		opts, err := buildOptions(id, p.Kind, wp.Options)
		if err != nil {
			return Piece{}, err
		}
		p.Options = opts
	case PieceInput:
		ph := deref(wp.Placeholder)
		if runes(ph) > 200 {
			return Piece{}, invalid("piece %s: placeholder is longer than 200 characters", id)
		}
		p.Placeholder = ph
		p.Multiline = wp.Multiline != nil && *wp.Multiline
		p.Required = wp.Required != nil && *wp.Required
	case PieceConfirm:
		yes, no := deref(wp.YesLabel), deref(wp.NoLabel)
		if runes(yes) > 40 {
			return Piece{}, invalid("piece %s: yes_label is longer than 40 characters", id)
		}
		if runes(no) > 40 {
			return Piece{}, invalid("piece %s: no_label is longer than 40 characters", id)
		}
		if yes == "" {
			yes = "Yes"
		}
		if no == "" {
			no = "No"
		}
		p.YesLabel, p.NoLabel = yes, no
	}
	return p, nil
}

func buildOptions(id string, kind PieceKind, wo *[]wireOption) ([]Option, error) {
	var src []wireOption
	if wo != nil {
		src = *wo
	}
	if kind == PieceChoice && (len(src) < 2 || len(src) > 10) {
		return nil, invalid("piece %s: choice needs 2 to 10 options", id)
	}
	if kind == PieceMulti && (len(src) < 1 || len(src) > 50) {
		return nil, invalid("piece %s: multi needs 1 to 50 options", id)
	}
	out := make([]Option, 0, len(src))
	seen := map[string]bool{}
	recommended := 0
	for k, o := range src {
		if strings.TrimSpace(o.Label) == "" {
			return nil, invalid("piece %s: option %d needs a label", id, k+1)
		}
		if runes(o.Label) > 120 {
			return nil, invalid("piece %s: option label \"%s\" is longer than 120 characters", id, o.Label)
		}
		if seen[o.Label] {
			return nil, invalid("piece %s: option label \"%s\" is used twice", id, o.Label)
		}
		seen[o.Label] = true
		if runes(o.Description) > 1000 {
			return nil, invalid("piece %s: option \"%s\" has a description longer than 1000 characters", id, o.Label)
		}
		if o.Recommended != nil && kind == PieceMulti {
			return nil, invalid("piece %s: recommended is not allowed for kind multi", id)
		}
		rec := o.Recommended != nil && *o.Recommended
		if rec {
			recommended++
		}
		out = append(out, Option{Label: o.Label, Description: o.Description, Recommended: rec})
	}
	if recommended > 1 {
		return nil, invalid("piece %s: only one option can be recommended", id)
	}
	return out, nil
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// Answerable reports whether the piece takes an answer.
func (p Piece) Answerable() bool {
	switch p.Kind {
	case PieceChoice, PieceMulti, PieceInput, PieceConfirm:
		return true
	}
	return false
}

// NewAnswer checks answers against the form and returns the normalized result.
func NewAnswer(f Form, answers map[string]any, note string) (FormAnswer, error) {
	answerable := map[string]bool{}
	for _, p := range f.Pieces {
		if p.Answerable() {
			answerable[p.ID] = true
		}
	}
	keys := make([]string, 0, len(answers))
	for k := range answers {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		if !answerable[k] {
			return FormAnswer{}, fmt.Errorf("The form has no question with the id %s.", k)
		}
	}
	out := map[string]any{}
	for _, p := range f.Pieces {
		if !p.Answerable() {
			continue
		}
		v, err := normalize(p, answers[p.ID])
		if err != nil {
			return FormAnswer{}, err
		}
		out[p.ID] = v
	}
	if runes(note) > maxNote {
		return FormAnswer{}, errors.New("The note is longer than 5000 characters.")
	}
	return FormAnswer{Status: StatusAnswered, Answers: out, Note: note}, nil
}

func wrongType(p Piece) error {
	return fmt.Errorf("The answer to \"%s\" has the wrong type.", p.Question)
}

func normalize(p Piece, v any) (any, error) {
	switch p.Kind {
	case PieceChoice:
		if v == nil {
			return nil, fmt.Errorf("Choose an option for \"%s\".", p.Question)
		}
		s, ok := v.(string)
		if !ok {
			return nil, wrongType(p)
		}
		if s == "" {
			return nil, fmt.Errorf("Choose an option for \"%s\".", p.Question)
		}
		if !hasLabel(p, s) {
			return nil, fmt.Errorf("The answer to \"%s\" is not one of its options.", p.Question)
		}
		return s, nil
	case PieceMulti:
		var vals []string
		switch t := v.(type) {
		case nil:
		case []string:
			vals = t
		case []any:
			for _, e := range t {
				s, ok := e.(string)
				if !ok {
					return nil, wrongType(p)
				}
				vals = append(vals, s)
			}
		default:
			return nil, wrongType(p)
		}
		picked := map[string]bool{}
		for _, s := range vals {
			if !hasLabel(p, s) {
				return nil, fmt.Errorf("The answer to \"%s\" is not one of its options.", p.Question)
			}
			if picked[s] {
				return nil, fmt.Errorf("The answer to \"%s\" lists an option twice.", p.Question)
			}
			picked[s] = true
		}
		res := []string{}
		for _, o := range p.Options {
			if picked[o.Label] {
				res = append(res, o.Label)
			}
		}
		return res, nil
	case PieceInput:
		s := ""
		if v != nil {
			var ok bool
			if s, ok = v.(string); !ok {
				return nil, wrongType(p)
			}
		}
		if p.Required && strings.TrimSpace(s) == "" {
			return nil, fmt.Errorf("Fill in \"%s\".", p.Question)
		}
		return s, nil
	default: // confirm
		if v == nil {
			return nil, fmt.Errorf("Answer yes or no to \"%s\".", p.Question)
		}
		s, ok := v.(string)
		if !ok {
			return nil, wrongType(p)
		}
		if s != "yes" && s != "no" {
			return nil, fmt.Errorf("The answer to \"%s\" must be yes or no.", p.Question)
		}
		return s, nil
	}
}

func hasLabel(p Piece, label string) bool {
	for _, o := range p.Options {
		if o.Label == label {
			return true
		}
	}
	return false
}

// Dismissed builds the answer for a form the person closed.
func Dismissed(note string) FormAnswer {
	return FormAnswer{Status: StatusDismissed, Answers: map[string]any{}, Note: note}
}

// EncodeAnswer returns the result JSON; a nil Answers map is written as {}.
func EncodeAnswer(a FormAnswer) string {
	if a.Answers == nil {
		a.Answers = map[string]any{}
	}
	b, _ := json.Marshal(a)
	return string(b)
}
