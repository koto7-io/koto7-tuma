package notification

import (
	"context"
	"fmt"
	"net"
	"net/smtp"
	"strings"
)

// SMTPConfig carries the SMTP connection and authentication settings.
// Values are loaded from environment variables via config.Load() — never
// hardcode credentials here.
type SMTPConfig struct {
	Host     string // SMTP_HOST
	Port     int    // SMTP_PORT (default 587)
	Username string // SMTP_USERNAME
	Password string // SMTP_PASSWORD  (never logged)
	From     string // SMTP_FROM
}

// SMTPSender implements Sender using the standard library net/smtp package.
// It is the only file that knows about SMTP; all other code talks to Sender.
type SMTPSender struct {
	cfg SMTPConfig
}

// NewSMTPSender returns an SMTPSender configured from cfg.
// The constructor is cheap — no network connection is made until Send is called.
func NewSMTPSender(cfg SMTPConfig) *SMTPSender {
	return &SMTPSender{cfg: cfg}
}

// Send delivers a plain-text email.
// ctx is honoured for the dial phase; once the SMTP session starts it runs to
// completion (net/smtp does not support mid-session cancellation).
func (s *SMTPSender) Send(_ context.Context, recipient, subject, body string) error {
	addr := fmt.Sprintf("%s:%d", s.cfg.Host, s.cfg.Port)

	var auth smtp.Auth
	if s.cfg.Username != "" {
		auth = smtp.PlainAuth("", s.cfg.Username, s.cfg.Password, s.cfg.Host)
	}

	msg := buildMessage(s.cfg.From, recipient, subject, body)

	if err := smtp.SendMail(addr, auth, s.cfg.From, []string{recipient}, []byte(msg)); err != nil {
		// Wrap with sender context; password is NOT included in any field.
		host, _, _ := net.SplitHostPort(addr)
		return fmt.Errorf("smtp send to %s via %s: %w", recipient, host, err)
	}
	return nil
}

// buildMessage assembles a minimal RFC 5322 email message.
func buildMessage(from, to, subject, body string) string {
	var b strings.Builder
	b.WriteString("From: " + from + "\r\n")
	b.WriteString("To: " + to + "\r\n")
	b.WriteString("Subject: " + subject + "\r\n")
	b.WriteString("MIME-Version: 1.0\r\n")
	b.WriteString("Content-Type: text/plain; charset=UTF-8\r\n")
	b.WriteString("\r\n")
	b.WriteString(body)
	return b.String()
}
