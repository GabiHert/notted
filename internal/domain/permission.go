package domain

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// ErrBadPermissionRequest marks a permission request that cannot become a form.
var ErrBadPermissionRequest = errors.New("bad permission request")

const (
	maxToolName   = 80
	maxRequestLen = 40000
)

// PermissionForm builds the card form that asks whether a tool call may run.
func PermissionForm(toolName string, input json.RawMessage) (Form, error) {
	if strings.TrimSpace(toolName) == "" {
		return Form{}, fmt.Errorf("%w: the tool name is empty", ErrBadPermissionRequest)
	}
	trimmed := bytes.TrimSpace(input)
	if len(trimmed) == 0 || trimmed[0] != '{' || !json.Valid(trimmed) {
		return Form{}, fmt.Errorf("%w: the input is not a JSON object", ErrBadPermissionRequest)
	}
	var buf bytes.Buffer
	if err := json.Indent(&buf, trimmed, "", "  "); err != nil {
		return Form{}, fmt.Errorf("%w: %v", ErrBadPermissionRequest, err)
	}
	text := buf.String()
	fence := fenceFor(text)
	body := text
	if r := []rune(text); len(r) > maxRequestLen {
		body = string(r[:maxRequestLen]) + fmt.Sprintf("\n… (cut; %d characters in all)", len(r))
	}
	name := toolName
	if r := []rune(name); len(r) > maxToolName {
		name = string(r[:maxToolName-1]) + "…"
	}
	return Form{
		Title: "Claude Code wants to use " + name,
		Pieces: []Piece{
			{Kind: PieceText, ID: "request", Markdown: fence + "json\n" + body + "\n" + fence},
			{Kind: PieceConfirm, ID: "allow", Question: "Allow this?", YesLabel: "Allow", NoLabel: "Deny"},
		},
	}, nil
}

func fenceFor(s string) string {
	longest, run := 0, 0
	for _, r := range s {
		if r == '`' {
			run++
			if run > longest {
				longest = run
			}
		} else {
			run = 0
		}
	}
	return strings.Repeat("`", max(3, longest+1))
}

type permissionAllow struct {
	Behavior     string          `json:"behavior"`
	UpdatedInput json.RawMessage `json:"updatedInput"`
}

type permissionDeny struct {
	Behavior string `json:"behavior"`
	Message  string `json:"message"`
}

// PermissionResult maps the person's answer to Claude Code's allow or deny JSON.
func PermissionResult(input json.RawMessage, a FormAnswer) string {
	if a.Status == StatusAnswered && a.Answers["allow"] == "yes" {
		b, _ := json.Marshal(permissionAllow{Behavior: "allow", UpdatedInput: input})
		return string(b)
	}
	if a.Status == StatusAnswered {
		msg := "The person denied this in Notted."
		if strings.TrimSpace(a.Note) != "" {
			msg += " Their note: " + a.Note
		}
		return PermissionDeny(msg)
	}
	return PermissionDeny("The person dismissed this request in Notted (Not now). Do not try it again unless they ask.")
}

// PermissionDeny returns a deny JSON carrying message.
func PermissionDeny(message string) string {
	b, _ := json.Marshal(permissionDeny{Behavior: "deny", Message: message})
	return string(b)
}
