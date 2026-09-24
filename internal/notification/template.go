// Package notification provides a layered notification system that separates
// business-logic event detection from template rendering and SMTP delivery.
//
// Flow:
//
//	Business Logic → notification.Service → Render(template) → Sender → Recipient
package notification

import (
	"fmt"
	"strings"
)

// Type identifies which notification message template to use.
type Type string

const (
	// TypeOpenIssuesExceeded fires when open issues exceed the configured limit.
	TypeOpenIssuesExceeded Type = "OPEN_ISSUES_EXCEEDED"

	// TypeTestNotification is used to verify that the notification pipeline is working.
	TypeTestNotification Type = "TEST_NOTIFICATION"
)

// Template holds the raw subject and body strings for a notification type.
// Placeholders use the {{key}} syntax and are resolved by Render.
type Template struct {
	Subject string
	Body    string
}

// registry maps every known notification type to its template.
// Add new entries here to support additional notification types — no other
// file in the notification package needs to change.
var registry = map[Type]Template{
	TypeOpenIssuesExceeded: {
		Subject: "Alert: Open Issues Exceeded — {{resource_name}}",
		Body: strings.TrimSpace(`
Your open issues have exceeded the configured limit.

Current open issues : {{current_value}}
Threshold           : {{limit}}

Please review your open issues in the Tuma console.
`),
	},
	"OPEN_ISSUES": {
		Subject: "Alert: Open Issues Exceeded — {{resource_name}}",
		Body: strings.TrimSpace(`
Your open issues have exceeded the configured limit.

Current open issues : {{current_value}}
Threshold           : {{limit}}

Please review your open issues in the Tuma console.
`),
	},
	TypeTestNotification: {
		Subject: "Tuma — Test Notification",
		Body: strings.TrimSpace(`
This is a test notification from Tuma.

If you received this email, your SMTP configuration is working correctly.
`),
	},
}

// Render looks up the template for typ, substitutes all {{key}} placeholders
// with the corresponding value from vars, and returns the rendered subject and
// body strings.
//
// Errors:
//   - Unknown type → ErrUnknownType
//   - A placeholder present in the template but missing from vars → ErrMissingVar
func Render(typ Type, vars map[string]string) (subject, body string, err error) {
	tmpl, ok := registry[typ]
	if !ok {
		return "", "", fmt.Errorf("%w: %q", ErrUnknownType, typ)
	}

	subject, err = substitute(tmpl.Subject, vars)
	if err != nil {
		return "", "", fmt.Errorf("subject: %w", err)
	}

	body, err = substitute(tmpl.Body, vars)
	if err != nil {
		return "", "", fmt.Errorf("body: %w", err)
	}

	return subject, body, nil
}

// RenderCustom renders custom subject and body templates with vars substitution.
func RenderCustom(subjectTmpl, bodyTmpl string, vars map[string]string) (subject, body string, err error) {
	subject, err = substitute(subjectTmpl, vars)
	if err != nil {
		return "", "", fmt.Errorf("subject: %w", err)
	}

	body, err = substitute(bodyTmpl, vars)
	if err != nil {
		return "", "", fmt.Errorf("body: %w", err)
	}

	return subject, body, nil
}

// GetDefaultTemplate returns the registered default template for a given Type.
func GetDefaultTemplate(typ Type) (Template, bool) {
	tmpl, ok := registry[typ]
	return tmpl, ok
}

// substitute replaces every {{key}} occurrence in s with vars[key].
// Returns ErrMissingVar if a placeholder key is absent from vars.
func substitute(s string, vars map[string]string) (string, error) {
	result := s
	for key, val := range vars {
		result = strings.ReplaceAll(result, "{{"+key+"}}", val)
	}

	// Detect any remaining un-substituted placeholders.
	start := strings.Index(result, "{{")
	if start != -1 {
		end := strings.Index(result[start:], "}}")
		if end != -1 {
			missing := result[start+2 : start+end]
			return "", fmt.Errorf("%w: %q", ErrMissingVar, missing)
		}
	}

	return result, nil
}
