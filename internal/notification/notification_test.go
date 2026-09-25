package notification

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
)

// ─── mock sender ─────────────────────────────────────────────────────────────

type mockSender struct {
	calls []mockCall
	err   error // if non-nil, returned on every Send call
}

type mockCall struct {
	recipient string
	subject   string
	body      string
}

func (m *mockSender) Send(_ context.Context, recipient, subject, body string) error {
	if m.err != nil {
		return m.err
	}
	m.calls = append(m.calls, mockCall{recipient, subject, body})
	return nil
}

var discardLogger = slog.New(slog.NewTextHandler(io.Discard, nil))

// ─── Template tests ───────────────────────────────────────────────────────────

func TestRender_CorrectTemplateSelectedPerType(t *testing.T) {
	types := []Type{TypeOpenIssuesExceeded, TypeTestNotification}
	for _, typ := range types {
		t.Run(string(typ), func(t *testing.T) {
			// All templates must be resolvable with a generous vars map.
			vars := map[string]string{
				"resource_name": "API Requests",
				"limit":         "100",
				"current_value": "105",
			}
			subj, body, err := Render(typ, vars)
			if err != nil {
				t.Fatalf("Render(%q) returned unexpected error: %v", typ, err)
			}
			if subj == "" {
				t.Errorf("Render(%q): subject must not be empty", typ)
			}
			if body == "" {
				t.Errorf("Render(%q): body must not be empty", typ)
			}
		})
	}
}

func TestRender_OpenIssuesExceeded(t *testing.T) {
	vars := map[string]string{
		"resource_name": "Open Issues",
		"limit":         "5",
		"current_value": "10",
	}
	subj, body, err := Render(TypeOpenIssuesExceeded, vars)
	if err != nil {
		t.Fatalf("Render returned unexpected error: %v", err)
	}
	expectedMsg := "Your open issues have exceeded the configured limit."
	if !contains(body, expectedMsg) {
		t.Errorf("expected body to contain %q, got:\n%s", expectedMsg, body)
	}
	if !contains(subj, "Open Issues") {
		t.Errorf("expected subject to contain 'Open Issues', got: %s", subj)
	}
}

func TestRender_VariableSubstitution(t *testing.T) {
	vars := map[string]string{
		"resource_name": "Open Issues",
		"limit":         "100",
		"current_value": "105",
	}
	_, body, err := Render(TypeOpenIssuesExceeded, vars)
	if err != nil {
		t.Fatalf("Render returned unexpected error: %v", err)
	}
	if !contains(body, "Your open issues have exceeded the configured limit.") {
		t.Errorf("expected body to contain message, got:\n%s", body)
	}
	if !contains(body, "100") {
		t.Errorf("expected body to contain '100', got:\n%s", body)
	}
	if !contains(body, "105") {
		t.Errorf("expected body to contain '105', got:\n%s", body)
	}
}

func TestRender_MissingVariable_ReturnsError(t *testing.T) {
	// Provide only resource_name; limit and current_value are missing.
	vars := map[string]string{
		"resource_name": "Open Issues",
	}
	_, _, err := Render(TypeOpenIssuesExceeded, vars)
	if err == nil {
		t.Fatal("expected error for missing variable, got nil")
	}
	if !errors.Is(err, ErrMissingVar) {
		t.Errorf("expected ErrMissingVar, got: %v", err)
	}
}

func TestRender_UnknownType_ReturnsError(t *testing.T) {
	_, _, err := Render("NO_SUCH_TYPE", nil)
	if err == nil {
		t.Fatal("expected error for unknown type, got nil")
	}
	if !errors.Is(err, ErrUnknownType) {
		t.Errorf("expected ErrUnknownType, got: %v", err)
	}
}

// ─── Service tests ────────────────────────────────────────────────────────────

func TestService_Send_SuccessfulPath(t *testing.T) {
	sender := &mockSender{}
	svc := NewService(sender, discardLogger)

	req := Request{
		Type:      TypeOpenIssuesExceeded,
		Recipient: "ops@example.com",
		Vars: map[string]string{
			"resource_name": "Open Issues",
			"limit":         "50",
			"current_value": "52",
		},
	}

	if err := svc.Send(context.Background(), req); err != nil {
		t.Fatalf("Send returned unexpected error: %v", err)
	}
	if len(sender.calls) != 1 {
		t.Fatalf("expected sender to be called once, got %d", len(sender.calls))
	}
	call := sender.calls[0]
	if call.recipient != "ops@example.com" {
		t.Errorf("wrong recipient: %s", call.recipient)
	}
	if !contains(call.subject, "Open Issues") {
		t.Errorf("subject should contain 'Open Issues', got: %s", call.subject)
	}
	if !contains(call.body, "Your open issues have exceeded the configured limit.") {
		t.Errorf("body should contain 'Your open issues have exceeded the configured limit.', got:\n%s", call.body)
	}
}

func TestService_Send_SMTPFailure_ReturnsError(t *testing.T) {
	smtpErr := errors.New("connection refused")
	sender := &mockSender{err: smtpErr}
	svc := NewService(sender, discardLogger)

	req := Request{
		Type:      TypeOpenIssuesExceeded,
		Recipient: "ops@example.com",
		Vars: map[string]string{
			"resource_name": "Open Issues",
			"limit":         "50",
			"current_value": "55",
		},
	}

	err := svc.Send(context.Background(), req)
	if err == nil {
		t.Fatal("expected error from SMTP failure, got nil")
	}
	if !errors.Is(err, smtpErr) {
		t.Errorf("expected wrapped smtpErr, got: %v", err)
	}
}

func TestService_Send_UnknownType_ReturnsError_WithoutCallingSender(t *testing.T) {
	sender := &mockSender{}
	svc := NewService(sender, discardLogger)

	err := svc.Send(context.Background(), Request{
		Type:      "INVALID_TYPE",
		Recipient: "ops@example.com",
	})

	if err == nil {
		t.Fatal("expected error for unknown type, got nil")
	}
	if !errors.Is(err, ErrUnknownType) {
		t.Errorf("expected ErrUnknownType, got: %v", err)
	}
	if len(sender.calls) != 0 {
		t.Errorf("sender must not be called when template rendering fails, got %d calls", len(sender.calls))
	}
}

func TestService_Send_MissingVar_ReturnsError_WithoutCallingSender(t *testing.T) {
	sender := &mockSender{}
	svc := NewService(sender, discardLogger)

	err := svc.Send(context.Background(), Request{
		Type:      TypeOpenIssuesExceeded,
		Recipient: "ops@example.com",
		Vars:      map[string]string{}, // all placeholders missing
	})

	if err == nil {
		t.Fatal("expected error for missing vars, got nil")
	}
	if !errors.Is(err, ErrMissingVar) {
		t.Errorf("expected ErrMissingVar, got: %v", err)
	}
	if len(sender.calls) != 0 {
		t.Errorf("sender must not be called when template rendering fails, got %d calls", len(sender.calls))
	}
}

func TestService_Send_TestNotification_NoVarsRequired(t *testing.T) {
	sender := &mockSender{}
	svc := NewService(sender, discardLogger)

	// TEST_NOTIFICATION has no placeholders — empty vars map must work.
	err := svc.Send(context.Background(), Request{
		Type:      TypeTestNotification,
		Recipient: "ops@example.com",
		Vars:      map[string]string{},
	})
	if err != nil {
		t.Fatalf("expected no error for TEST_NOTIFICATION, got: %v", err)
	}
	if len(sender.calls) != 1 {
		t.Fatalf("expected 1 sender call, got %d", len(sender.calls))
	}
}

func TestService_Send_CustomTemplate(t *testing.T) {
	sender := &mockSender{}
	svc := NewService(sender, discardLogger)

	err := svc.Send(context.Background(), Request{
		Type:          TypeOpenIssuesExceeded,
		Recipient:     "ops@example.com",
		CustomSubject: "Custom Alert: {{resource_name}}",
		CustomBody:    "Custom body with count {{current_value}} and limit {{limit}}",
		Vars: map[string]string{
			"resource_name": "My Queue",
			"current_value": "15",
			"limit":         "10",
		},
	})
	if err != nil {
		t.Fatalf("unexpected error with custom template: %v", err)
	}
	if len(sender.calls) != 1 {
		t.Fatalf("expected 1 sender call, got %d", len(sender.calls))
	}
	call := sender.calls[0]
	if call.subject != "Custom Alert: My Queue" {
		t.Errorf("expected rendered custom subject, got: %s", call.subject)
	}
	if call.body != "Custom body with count 15 and limit 10" {
		t.Errorf("expected rendered custom body, got: %s", call.body)
	}
}

// ─── helpers ─────────────────────────────────────────────────────────────────

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(sub) == 0 ||
		func() bool {
			for i := 0; i <= len(s)-len(sub); i++ {
				if s[i:i+len(sub)] == sub {
					return true
				}
			}
			return false
		}())
}
