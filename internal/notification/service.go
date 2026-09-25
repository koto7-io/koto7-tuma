package notification

import (
	"context"
	"fmt"
	"log/slog"
)

// Request describes a single notification to send.
// Business logic builds this struct and passes it to Service.Send — it does
// not need to know anything about templates or SMTP.
type Request struct {
	// Type selects which template to use.
	Type Type

	// Recipient is the destination email address.
	Recipient string

	// Vars are the placeholder values that will be substituted into the
	// selected template's subject and body (e.g. "resource_name", "limit").
	Vars map[string]string

	// CustomSubject optionally overrides the template subject.
	CustomSubject string

	// CustomBody optionally overrides the template body.
	// If empty, the default template registered for Type is used.
	CustomBody string
}

// Service orchestrates notification delivery:
//  1. Validates the request.
//  2. Selects and renders the correct template.
//  3. Passes the rendered subject+body to the Sender.
//
// It is the only entry point that business logic should call.
type Service struct {
	sender Sender
	logger *slog.Logger
}

// NewService returns a Service that uses sender for delivery.
// Pass slog.Default() or the application-wide logger.
func NewService(sender Sender, logger *slog.Logger) *Service {
	return &Service{sender: sender, logger: logger}
}

// Send processes req: renders the template and dispatches the email.
// All errors bubble up — callers decide whether to treat them as fatal.
func (svc *Service) Send(ctx context.Context, req Request) error {
	svc.logger.Info("notification requested",
		"type", req.Type,
		"recipient", req.Recipient,
	)

	var subject, body string
	var err error

	if req.CustomBody != "" {
		subjTmpl := req.CustomSubject
		if subjTmpl == "" {
			if tmpl, ok := GetDefaultTemplate(req.Type); ok {
				subjTmpl = tmpl.Subject
			}
		}
		subject, body, err = RenderCustom(subjTmpl, req.CustomBody, req.Vars)
	} else {
		subject, body, err = Render(req.Type, req.Vars)
	}

	if err != nil {
		return fmt.Errorf("notification render: %w", err)
	}

	svc.logger.Info("notification template rendered",
		"type", req.Type,
		"subject", subject,
	)

	svc.logger.Info("notification sending started",
		"type", req.Type,
		"recipient", req.Recipient,
	)

	if err := svc.sender.Send(ctx, req.Recipient, subject, body); err != nil {
		svc.logger.Error("notification sending failed",
			"type", req.Type,
			"recipient", req.Recipient,
			"error", err,
		)
		return fmt.Errorf("notification send: %w", err)
	}

	svc.logger.Info("notification sent successfully",
		"type", req.Type,
		"recipient", req.Recipient,
	)
	return nil
}
