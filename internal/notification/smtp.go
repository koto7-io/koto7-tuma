package notification

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"net/smtp"
	"strconv"
	"strings"
	"time"
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
// The dial and the SMTP session share a deadline taken from ctx, or 15s.
func (s *SMTPSender) Send(ctx context.Context, recipient, subject, body string) error {
	if s.cfg.Host == "" {
		return errors.New("smtp host is not configured")
	}

	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, 15*time.Second)
		defer cancel()
	}

	addr := net.JoinHostPort(s.cfg.Host, strconv.Itoa(s.cfg.Port))
	conn, err := (&net.Dialer{}).DialContext(ctx, "tcp", addr)
	if err != nil {
		return fmt.Errorf("smtp dial %s: %w", s.cfg.Host, err)
	}
	if deadline, ok := ctx.Deadline(); ok {
		_ = conn.SetDeadline(deadline)
	}

	client, err := smtp.NewClient(conn, s.cfg.Host)
	if err != nil {
		conn.Close()
		return fmt.Errorf("smtp client: %w", err)
	}
	defer client.Close()

	if ok, _ := client.Extension("STARTTLS"); ok {
		tlsCfg := &tls.Config{ServerName: s.cfg.Host, MinVersion: tls.VersionTLS12}
		if err := client.StartTLS(tlsCfg); err != nil {
			return fmt.Errorf("smtp starttls: %w", err)
		}
	} else if s.cfg.Username != "" && s.cfg.Host != "localhost" && s.cfg.Host != "127.0.0.1" {
		return fmt.Errorf("smtp server %s does not support STARTTLS", s.cfg.Host)
	}

	if s.cfg.Username != "" {
		auth := smtp.PlainAuth("", s.cfg.Username, s.cfg.Password, s.cfg.Host)
		if err := client.Auth(auth); err != nil {
			return fmt.Errorf("smtp auth: %w", err)
		}
	}

	from := sanitizeHeader(s.cfg.From)
	to := sanitizeHeader(recipient)
	subj := sanitizeHeader(subject)
	if err := client.Mail(from); err != nil {
		return fmt.Errorf("smtp mail: %w", err)
	}
	if err := client.Rcpt(to); err != nil {
		return fmt.Errorf("smtp rcpt: %w", err)
	}
	w, err := client.Data()
	if err != nil {
		return fmt.Errorf("smtp data: %w", err)
	}
	if _, err := w.Write([]byte(buildMessage(from, to, subj, body))); err != nil {
		return fmt.Errorf("smtp write: %w", err)
	}
	if err := w.Close(); err != nil {
		return fmt.Errorf("smtp data close: %w", err)
	}
	if err := client.Quit(); err != nil {
		return fmt.Errorf("smtp quit: %w", err)
	}
	return nil
}

// sanitizeHeader strips CR/LF so a template cannot inject extra headers.
func sanitizeHeader(s string) string {
	return strings.NewReplacer("\r", "", "\n", "").Replace(s)
}

// buildMessage assembles a minimal RFC 5322 email message.
func buildMessage(from, to, subject, body string) string {
	from = sanitizeHeader(from)
	to = sanitizeHeader(to)
	subject = sanitizeHeader(subject)
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
