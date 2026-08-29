package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	DatabaseURL        string
	TemporalHost       string
	TemporalNamespace  string
	EncryptionKey      []byte
	ListenAddr         string
	PublicBaseURL      string
	SessionCookieName  string
	SessionTTL         time.Duration
	MaxBodyBytes       int64
	PerConnConcurrency int
	LogLevel           string
	DemoMode           bool
	PlaygroundInternalBase string
	PlaygroundSessionTTL   time.Duration
	PlaygroundMaxSessions  int
}

func Load() (*Config, error) {
	key := os.Getenv("TUMA_ENCRYPTION_KEY")
	if key == "" {
		return nil, fmt.Errorf("TUMA_ENCRYPTION_KEY is required")
	}
	if len(key) != 32 {
		return nil, fmt.Errorf("TUMA_ENCRYPTION_KEY must be exactly 32 bytes")
	}

	dbURL := env("DATABASE_URL", "postgres://tuma:tuma@localhost:5432/tuma?sslmode=disable")
	perConn, _ := strconv.Atoi(env("TUMA_PER_CONN_CONCURRENCY", "10"))
	maxBody, _ := strconv.ParseInt(env("TUMA_MAX_BODY_BYTES", "1048576"), 10, 64)
	sessionHours, _ := strconv.Atoi(env("TUMA_SESSION_TTL_HOURS", "168"))
	publicBaseURL := env("TUMA_PUBLIC_BASE_URL", "http://localhost:8080")
	demoMode := demoModeEnabled(publicBaseURL)
	pgHours, _ := strconv.Atoi(env("TUMA_PLAYGROUND_SESSION_TTL_HOURS", "2"))
	pgMax, _ := strconv.Atoi(env("TUMA_PLAYGROUND_MAX_SESSIONS", "500"))
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
