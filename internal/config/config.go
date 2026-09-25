package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// SMTPConfig holds outbound email settings loaded from environment variables.
// Credentials are never logged — only Host, Port, and From are safe to print.
type SMTPConfig struct {
	Host     string // SMTP_HOST
	Port     int    // SMTP_PORT (default 587)
	Username string // SMTP_USERNAME
	Password string // SMTP_PASSWORD — never log this field
	From     string // SMTP_FROM
}

type Config struct {
	DatabaseURL        string
	TemporalHost       string
	TemporalNamespace  string
	EncryptionKey      []byte
	PayloadKey         []byte
	ListenAddr         string
	PublicBaseURL      string
	SessionCookieName  string
	SessionTTL         time.Duration
	MaxBodyBytes       int64
	PerConnConcurrency int
	LogLevel               string
	DemoMode               bool
	PlaygroundInternalBase string
	PlaygroundSessionTTL   time.Duration
	PlaygroundMaxSessions  int
	AlertEvalInterval      time.Duration
	SMTP                   SMTPConfig
}

func Load() (*Config, error) {
	key := os.Getenv("TUMA_ENCRYPTION_KEY")
	if key == "" {
		return nil, fmt.Errorf("TUMA_ENCRYPTION_KEY is required")
	}
	if len(key) != 32 {
		return nil, fmt.Errorf("TUMA_ENCRYPTION_KEY must be exactly 32 bytes")
	}

	payloadKey := []byte(env("TUMA_PAYLOAD_KEY", string(key)))
	if len(payloadKey) != 32 {
		return nil, fmt.Errorf("TUMA_PAYLOAD_KEY must be exactly 32 bytes when set")
	}

	dbURL := env("DATABASE_URL", "postgres://tuma:tuma@localhost:5432/tuma?sslmode=disable")
	perConn, _ := strconv.Atoi(env("TUMA_PER_CONN_CONCURRENCY", "10"))
	maxBody, _ := strconv.ParseInt(env("TUMA_MAX_BODY_BYTES", "1048576"), 10, 64)
	sessionHours, _ := strconv.Atoi(env("TUMA_SESSION_TTL_HOURS", "168"))
	publicBaseURL := env("TUMA_PUBLIC_BASE_URL", "http://localhost:8080")
	demoMode := demoModeEnabled(publicBaseURL)
	pgHours, _ := strconv.Atoi(env("TUMA_PLAYGROUND_SESSION_TTL_HOURS", "2"))
	pgMax, _ := strconv.Atoi(env("TUMA_PLAYGROUND_MAX_SESSIONS", "500"))
	alertEvalS, _ := strconv.Atoi(env("TUMA_ALERT_EVAL_INTERVAL_SECONDS", "60"))
	smtpPort, _ := strconv.Atoi(env("SMTP_PORT", "587"))
	pgInternal := env("TUMA_PLAYGROUND_INTERNAL_BASE", "")
	if pgInternal == "" {
		if demoMode {
			pgInternal = "http://tuma-api:8080"
		} else {
			pgInternal = "http://127.0.0.1:8080"
		}
	}

	return &Config{
		DatabaseURL:        dbURL,
		TemporalHost:       env("TEMPORAL_HOST", "localhost:7233"),
		TemporalNamespace:  env("TEMPORAL_NAMESPACE", "default"),
		EncryptionKey:      []byte(key),
		PayloadKey:         payloadKey,
		ListenAddr:         env("TUMA_LISTEN_ADDR", ":8080"),
		PublicBaseURL:      publicBaseURL,
		SessionCookieName:  env("TUMA_SESSION_COOKIE", "tuma_session"),
		SessionTTL:         time.Duration(sessionHours) * time.Hour,
		MaxBodyBytes:       maxBody,
		PerConnConcurrency: perConn,
		LogLevel:           env("TUMA_LOG_LEVEL", "info"),
		DemoMode:           demoMode,
		PlaygroundInternalBase: pgInternal,
		PlaygroundSessionTTL:   time.Duration(pgHours) * time.Hour,
		PlaygroundMaxSessions:  pgMax,
		AlertEvalInterval: time.Duration(alertEvalS) * time.Second,
		SMTP: SMTPConfig{
			Host:     env("SMTP_HOST", ""),
			Port:     smtpPort,
			Username: env("SMTP_USERNAME", ""),
			Password: os.Getenv("SMTP_PASSWORD"), // deliberately not using env() to avoid accidental logging
			From:     env("SMTP_FROM", "tuma@localhost"),
		},
	}, nil
}

// demoModeEnabled turns on playground routes when explicitly requested or when
// TUMA_PUBLIC_BASE_URL points at the public demo host (unless TUMA_DEMO_MODE=false).
func demoModeEnabled(publicBaseURL string) bool {
	switch os.Getenv("TUMA_DEMO_MODE") {
	case "true":
		return true
	case "false":
		return false
	default:
		return strings.Contains(publicBaseURL, "tuma-demo.koto7.dev")
	}
}

func env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
