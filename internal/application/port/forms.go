package port

import (
	"context"

	"github.com/GabiHert/notted/internal/domain"
)

// FormRequest is one form a meeting's session asks the app to show.
type FormRequest struct {
	MeetingID string
	Token     string
	Kind      domain.FormKind
	Form      domain.Form
}

// FormHandler answers one form request. ctx ends when the requester goes away.
type FormHandler interface {
	Handle(ctx context.Context, req FormRequest) (domain.FormAnswer, error)
}

// FormAsker sends one form to the running app and waits for the answer.
type FormAsker interface {
	Ask(ctx context.Context, kind domain.FormKind, form domain.Form) (domain.FormAnswer, error)
}

// Notification is one macOS notification about an open form.
type Notification struct {
	ID        string // "ask-<form id>"
	Title     string
	Subtitle  string
	Body      string
	MeetingID string
}

// Notifier shows and removes notifications.
type Notifier interface {
	Notify(n Notification) error
	Remove(id string)
}
