package notification

import (
	"context"
	"errors"
)

// Sender is the single delivery abstraction used by Service.
// Implementations include SMTPSender; tests use a mock.
// Future providers (SendGrid, Amazon SES, …) implement this interface without
// requiring any changes to Service or business logic.
type Sender interface {
	// Send delivers an email to recipient with the given subject and plain-text body.
	// It must return a non-nil error if delivery fails.
	Send(ctx context.Context, recipient, subject, body string) error
}

// Sentinel errors — callers can use errors.Is for targeted handling.
var (
	// ErrUnknownType is returned when Render is called with an unregistered Type.
	ErrUnknownType = errors.New("notification: unknown type")

	// ErrMissingVar is returned when a template placeholder has no matching
	// entry in the vars map passed to Render.
	ErrMissingVar = errors.New("notification: missing template variable")
)
