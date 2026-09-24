package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.temporal.io/sdk/client"
	"go.temporal.io/sdk/contrib/opentelemetry"
	"go.temporal.io/sdk/interceptor"
	"go.temporal.io/sdk/worker"
	"golang.org/x/crypto/bcrypt"

	"github.com/koto7/tuma/internal/alerting"
	"github.com/koto7/tuma/internal/api"
	"github.com/koto7/tuma/internal/config"
	"github.com/koto7/tuma/internal/crypto"
	"github.com/koto7/tuma/internal/metrics"
	"github.com/koto7/tuma/internal/notification"
	"github.com/koto7/tuma/internal/storage"
	"github.com/koto7/tuma/internal/workflow"
)

func main() {
	if len(os.Args) < 3 || os.Args[1] != "serve" {
		fmt.Fprintf(os.Stderr, "usage: tuma serve api|worker\n")
		os.Exit(1)
	}

	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "config: %v\n", err)
		os.Exit(1)
	}

	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: parseLevel(cfg.LogLevel)}))
	slog.SetDefault(logger)

	if err := runMigrations(cfg.DatabaseURL); err != nil {
		logger.Error("migrations failed", "error", err)
		os.Exit(1)
	}

	pool, err := pgxpool.New(context.Background(), cfg.DatabaseURL)
	if err != nil {
		logger.Error("db connect failed", "error", err)
		os.Exit(1)
	}
	defer pool.Close()

	enc, err := crypto.NewEncryptor(cfg.EncryptionKey)
	if err != nil {
		logger.Error("encryptor init failed", "error", err)
		os.Exit(1)
	}
	payloadEnc, err := crypto.NewEncryptor(cfg.PayloadKey)
	if err != nil {
		logger.Error("payload encryptor init failed", "error", err)
		os.Exit(1)
	}
	store := storage.New(pool, payloadEnc)

	if n, err := store.BackfillPayloadEncryption(context.Background()); err != nil {
		logger.Error("payload encryption backfill failed", "error", err)
		os.Exit(1)
	} else if n > 0 {
		logger.Info("encrypted legacy event payloads", "count", n)
	}

	if err := bootstrapAdmin(context.Background(), store, logger); err != nil {
		logger.Error("bootstrap failed", "error", err)
		os.Exit(1)
	}

	var tracingInterceptor interceptor.Interceptor
	tracingInterceptor, err = opentelemetry.NewTracingInterceptor(opentelemetry.TracerOptions{})
	if err != nil {
		logger.Error("otel interceptor failed", "error", err)
		os.Exit(1)
	}

	temporalClient, err := dialTemporal(cfg, tracingInterceptor, logger)
	if err != nil {
		logger.Error("temporal client failed", "error", err)
		os.Exit(1)
	}
	defer temporalClient.Close()

	switch os.Args[2] {
	case "api":
		runAPI(cfg, store, enc, temporalClient, logger)
	case "worker":
		runWorker(cfg, store, enc, temporalClient, tracingInterceptor, logger)
	default:
		fmt.Fprintf(os.Stderr, "unknown serve mode: %s\n", os.Args[2])
		os.Exit(1)
	}
}

func dialTemporal(cfg *config.Config, tracingInterceptor interceptor.Interceptor, logger *slog.Logger) (client.Client, error) {
	deadline := time.Now().Add(3 * time.Minute)
	backoff := time.Second
	for {
		c, err := client.Dial(client.Options{
			HostPort:     cfg.TemporalHost,
			Namespace:    cfg.TemporalNamespace,
			Interceptors: []interceptor.ClientInterceptor{tracingInterceptor},
		})
		if err == nil {
			return c, nil
		}
		if time.Now().After(deadline) {
			return nil, err
		}
		logger.Warn("temporal not ready, retrying", "error", err, "retry_in", backoff)
		time.Sleep(backoff)
		if backoff < 15*time.Second {
			backoff *= 2
		}
	}
}

func runAPI(cfg *config.Config, store *storage.Store, enc *crypto.Encryptor, temporal client.Client, logger *slog.Logger) {
	if n, err := store.OpenIssuesCount(context.Background()); err == nil {
		metrics.SetDLQDepth(float64(n))
	}

	go runRetentionLoop(store, logger)

	srv := api.NewServer(cfg, store, enc, temporal, logger)
	server := &http.Server{
		Addr:              cfg.ListenAddr,
		Handler:           loggingMiddleware(logger, srv.Handler()),
		ReadHeaderTimeout: 10 * time.Second,
	}

	go func() {
		logger.Info("api listening", "addr", cfg.ListenAddr)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("api failed", "error", err)
			os.Exit(1)
		}
	}()

	waitShutdown(server, logger)
}

func runWorker(cfg *config.Config, store *storage.Store, enc *crypto.Encryptor, temporal client.Client, tracingInterceptor interceptor.Interceptor, logger *slog.Logger) {
	acts := &workflow.Activities{
		Store:     store,
		Encryptor: enc,
		HTTPClient: &http.Client{
			Timeout: 15 * time.Second,
		},
		Semaphores: workflow.NewConnectionSemaphores(cfg.PerConnConcurrency),
	}

	w := worker.New(temporal, workflow.TaskQueue, worker.Options{
		Interceptors: []interceptor.WorkerInterceptor{tracingInterceptor},
	})
	w.RegisterWorkflow(workflow.DeliveryWorkflow)
	w.RegisterActivity(acts.LoadEventMeta)
	w.RegisterActivity(acts.DeliverActivity)
	w.RegisterActivity(acts.RecordDelivered)
	w.RegisterActivity(acts.RecordIssue)

	// Build the notification chain: SMTP sender → service.
	// When SMTP_HOST is empty the sender is still constructed but will fail
	// gracefully at send-time (best-effort, errors are logged not fatal).
	smtpSender := notification.NewSMTPSender(notification.SMTPConfig{
		Host:     cfg.SMTP.Host,
		Port:     cfg.SMTP.Port,
		Username: cfg.SMTP.Username,
		Password: cfg.SMTP.Password,
		From:     cfg.SMTP.From,
	})
	notifSvc := notification.NewService(smtpSender, logger)

	// Launch alert evaluator alongside the Temporal worker.
	evalCtx, cancelEval := context.WithCancel(context.Background())
	defer cancelEval()
	go alerting.New(store, notifSvc, cfg.AlertEvalInterval, logger).Run(evalCtx)

	logger.Info("worker starting", "task_queue", workflow.TaskQueue)
	if err := w.Run(worker.InterruptCh()); err != nil {
		logger.Error("worker failed", "error", err)
		os.Exit(1)
	}
}

func runMigrations(dbURL string) error {
	source := envOr("MIGRATIONS_PATH", "file://migrations")
	m, err := migrate.New(source, dbURL)
	if err != nil {
		return err
	}
	defer m.Close()
	if err := m.Up(); err != nil && err != migrate.ErrNoChange {
		return err
	}
	return nil
}

func bootstrapAdmin(ctx context.Context, store *storage.Store, logger *slog.Logger) error {
	n, err := store.CountUsers(ctx)
	if err != nil {
		return err
	}
	if n > 0 {
		return nil
	}
	email := envOr("TUMA_ADMIN_EMAIL", "admin@localhost")
	pass := os.Getenv("TUMA_ADMIN_PASSWORD")
	generated := pass == ""
	if generated {
		pass, err = randomPassword(24)
		if err != nil {
			return err
		}
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(pass), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	if err := store.CreateUser(ctx, &storage.User{Email: email, PasswordHash: string(hash)}); err != nil {
		return err
	}
	if generated {
		logger.Warn("created bootstrap admin with generated password — save this now; set TUMA_ADMIN_PASSWORD to pin a fixed password on fresh installs",
			"email", email,
			"password", pass,
		)
		fmt.Fprintf(os.Stderr, "\n=== TUMA FIRST BOOT ===\n  Email:    %s\n  Password: %s\n  (also logged above; set TUMA_ADMIN_PASSWORD to use a fixed password)\n=======================\n\n", email, pass)
	}
	return nil
}

func randomPassword(byteLen int) (string, error) {
	b := make([]byte, byteLen)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func parseLevel(level string) slog.Level {
	switch level {
	case "debug":
		return slog.LevelDebug
	case "warn":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

func loggingMiddleware(logger *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		logger.Info("request",
			"method", r.Method,
			"path", r.URL.Path,
			"duration_ms", time.Since(start).Milliseconds(),
		)
	})
}

func waitShutdown(server *http.Server, logger *slog.Logger) {
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGINT, syscall.SIGTERM)
	<-ch
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = server.Shutdown(ctx)
	logger.Info("api stopped")
}

func runRetentionLoop(store *storage.Store, logger *slog.Logger) {
	ticker := time.NewTicker(time.Hour)
	defer ticker.Stop()
	runOnce := func() {
		n, err := store.PurgeExpiredEvents(context.Background())
		if err != nil {
			logger.Error("retention purge failed", "error", err)
			return
		}
		if n > 0 {
			logger.Info("retention purge completed", "events_deleted", n)
		}
	}
	runOnce()
	for range ticker.C {
		runOnce()
	}
}
