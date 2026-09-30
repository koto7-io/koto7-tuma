package notifysettings

import (
	"testing"

	"github.com/koto7/tuma/internal/config"
)

func TestMerge_ConsoleOverridesEnv(t *testing.T) {
	cfg := &config.Config{
		SlackBotToken: "xoxb-env",
		SMTP: config.SMTPConfig{
			Host: "smtp.env", Port: 587, Username: "env-user", Password: "env-pass", From: "env@example.com",
		},
	}
	got := Merge(Console{
		SMTPHost: "smtp.console", SMTPPort: 2525, SMTPUsername: "console-user",
		SMTPPassword: "console-pass", SMTPFrom: "console@example.com", SlackToken: "xoxb-console",
	}, cfg)
	if got.SMTP.Host != "smtp.console" || got.SMTP.Port != 2525 || got.SMTP.Password != "console-pass" {
		t.Fatalf("smtp = %+v", got.SMTP)
	}
	if got.SlackToken != "xoxb-console" || !got.View.SlackTokenSaved || !got.View.EmailEnabled {
		t.Fatalf("view = %+v token %q", got.View, got.SlackToken)
	}
}

func TestMerge_BlankConsoleUsesEnv(t *testing.T) {
	cfg := &config.Config{
		SlackBotToken: "xoxb-env",
		SMTP:          config.SMTPConfig{Host: "smtp.env", Port: 587, Password: "env-pass", From: "env@example.com"},
	}
	got := Merge(Console{}, cfg)
	if got.SMTP.Host != "smtp.env" || got.SMTP.Password != "env-pass" || got.SlackToken != "xoxb-env" {
		t.Fatalf("smtp = %+v token %q", got.SMTP, got.SlackToken)
	}
	if got.View.SMTPPasswordSaved || !got.View.SMTPPasswordInEnv || !got.View.SlackTokenInEnv {
		t.Fatalf("view = %+v", got.View)
	}
	if got.View.EnvSMTPHost != "smtp.env" {
		t.Fatalf("env host hidden: %+v", got.View)
	}
}

func TestMerge_NothingConfigured(t *testing.T) {
	got := Merge(Console{}, &config.Config{})
	if got.View.EmailEnabled || got.View.SlackEnabled {
		t.Fatalf("expected both channels off: %+v", got.View)
	}
	if got.SMTP.Port != 587 {
		t.Fatalf("port = %d", got.SMTP.Port)
	}
}
