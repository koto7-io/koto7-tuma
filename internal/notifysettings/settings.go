// Package notifysettings merges alert delivery settings saved in the console
// with the server environment. A saved value wins. A blank value uses the env.
package notifysettings

import (
	"context"

	"github.com/koto7/tuma/internal/config"
	"github.com/koto7/tuma/internal/crypto"
	"github.com/koto7/tuma/internal/notification"
	"github.com/koto7/tuma/internal/storage"
)

// Reader loads the console row.
type Reader interface {
	GetNotificationSettings(ctx context.Context) (storage.NotificationSettings, error)
}

// Console is the decrypted console row. Empty strings mean "not saved here".
type Console struct {
	SMTPHost     string
	SMTPPort     int
	SMTPUsername string
	SMTPPassword string
	SMTPFrom     string
	SlackToken   string
}

// View is safe to send to the browser. It never includes a password or token.
type View struct {
	SMTPHost          string `json:"smtp_host"`
	SMTPPort          int    `json:"smtp_port"`
	SMTPUsername      string `json:"smtp_username"`
	SMTPFrom          string `json:"smtp_from"`
	SMTPPasswordSaved bool   `json:"smtp_password_saved"`
	SMTPPasswordInEnv bool   `json:"smtp_password_in_env"`
	EnvSMTPHost       string `json:"env_smtp_host"`
	EnvSMTPPort       int    `json:"env_smtp_port"`
	EnvSMTPFrom       string `json:"env_smtp_from"`
	SlackTokenSaved   bool   `json:"slack_token_saved"`
	SlackTokenInEnv   bool   `json:"slack_token_in_env"`
	EmailEnabled      bool   `json:"email_enabled"`
	SlackEnabled      bool   `json:"slack_enabled"`
}

// Effective is the config the worker should send with.
type Effective struct {
	View       View
	SMTP       notification.SMTPConfig
	SlackToken string
}

// Load reads the console row, decrypts secrets, and merges them with cfg.
func Load(ctx context.Context, r Reader, enc *crypto.Encryptor, cfg *config.Config) (Effective, error) {
	row, err := r.GetNotificationSettings(ctx)
	if err != nil {
		return Effective{}, err
	}
	console, err := decrypt(enc, row)
	if err != nil {
		return Effective{}, err
	}
	return Merge(console, cfg), nil
}

func decrypt(enc *crypto.Encryptor, row storage.NotificationSettings) (Console, error) {
	c := Console{
		SMTPHost:     row.SMTPHost,
		SMTPPort:     row.SMTPPort,
		SMTPUsername: row.SMTPUsername,
		SMTPFrom:     row.SMTPFrom,
	}
	var err error
	if c.SMTPPassword, err = secret(enc, row.SMTPPasswordEnc); err != nil {
		return Console{}, err
	}
	if c.SlackToken, err = secret(enc, row.SlackBotTokenEnc); err != nil {
		return Console{}, err
	}
	return c, nil
}

func secret(enc *crypto.Encryptor, b []byte) (string, error) {
	if len(b) == 0 || enc == nil {
		return "", nil
	}
	return enc.Decrypt(b)
}

// Merge applies console overrides on top of the process environment.
func Merge(console Console, cfg *config.Config) Effective {
	env := cfg.SMTP
	smtp := notification.SMTPConfig{
		Host:     first(console.SMTPHost, env.Host),
		Port:     env.Port,
		Username: first(console.SMTPUsername, env.Username),
		Password: first(console.SMTPPassword, env.Password),
		From:     first(console.SMTPFrom, env.From),
	}
	if console.SMTPPort > 0 {
		smtp.Port = console.SMTPPort
	}
	if smtp.Port == 0 {
		smtp.Port = 587
	}
	token := first(console.SlackToken, cfg.SlackBotToken)

	envConfigured := env.Host != ""
	view := View{
		SMTPHost:          console.SMTPHost,
		SMTPPort:          console.SMTPPort,
		SMTPUsername:      console.SMTPUsername,
		SMTPFrom:          console.SMTPFrom,
		SMTPPasswordSaved: console.SMTPPassword != "",
		SMTPPasswordInEnv: env.Password != "",
		SlackTokenSaved:   console.SlackToken != "",
		SlackTokenInEnv:   cfg.SlackBotToken != "",
		EmailEnabled:      smtp.Host != "",
		SlackEnabled:      token != "",
	}
	if envConfigured {
		view.EnvSMTPHost = env.Host
		view.EnvSMTPPort = env.Port
		view.EnvSMTPFrom = env.From
	}
	return Effective{View: view, SMTP: smtp, SlackToken: token}
}

func first(console, env string) string {
	if console != "" {
		return console
	}
	return env
}
