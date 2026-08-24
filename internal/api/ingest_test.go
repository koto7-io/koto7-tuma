package api

import (
	"bytes"
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"go.temporal.io/sdk/client"

	"github.com/koto7/tuma/internal/config"
	"github.com/koto7/tuma/internal/crypto"
	"github.com/koto7/tuma/internal/storage"
)

type noopTemporal struct {
	calls int
}

func (n *noopTemporal) ExecuteWorkflow(_ context.Context, _ client.StartWorkflowOptions, _ interface{}, _ ...interface{}) (client.WorkflowRun, error) {
	n.calls++
	return nil, nil
}

// TestIngestAckAfterCommit verifies the handler returns 200 only after the event row exists in Postgres.
func TestIngestAckAfterCommit(t *testing.T) {
	ctx := context.Background()
	pool := testPool(t)
	store := storage.New(pool)
	enc, err := crypto.NewEncryptor([]byte("01234567890123456789012345678901"))
	if err != nil {
		t.Fatal(err)
	}

	encSecret, _ := enc.Encrypt("ignored")
	conn := &storage.Connection{
		Name:               "test",
		SourceType:         "internal",
		InboundPath:        "testpath1",
		DestinationURL:     "http://example.com/hook",
		SigningSecretEnc:   encSecret,
		RetryAttempts:      3,
		RetryFirstDelayS:   1,
		RetryBackoffFactor: 2,
		RetentionDays:      7,
	}
	if err := store.CreateConnection(ctx, conn); err != nil {
		t.Fatal(err)
	}

	cfg := &config.Config{PublicBaseURL: "http://localhost:8080", MaxBodyBytes: 1 << 20}
	temporal := &noopTemporal{}
	srv := NewServer(cfg, store, enc, temporal, slog.Default())

	body := []byte(`{"id":"evt_test_1","type":"test.event"}`)
	req := httptest.NewRequest(http.MethodPost, "/e/testpath1", bytes.NewReader(body))
	req.SetPathValue("path", "testpath1")
	req.Header.Set("X-Tuma-Event-Id", "evt_test_1")
	rec := httptest.NewRecorder()

	srv.ingest(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}

	got, err := store.GetEventByProviderID(ctx, conn.ID, "evt_test_1")
	if err != nil || got == nil {
		t.Fatal("event must be persisted when 200 is returned")
	}
	if temporal.calls != 1 {
		t.Fatalf("expected 1 workflow start, got %d", temporal.calls)
	}
}

// TestIngestDuplicateAcksWithoutSecondWorkflow verifies ON CONFLICT duplicates still ack 200
// but do not enqueue another delivery workflow.
func TestIngestDuplicateAcksWithoutSecondWorkflow(t *testing.T) {
	ctx := context.Background()
	pool := testPool(t)
	store := storage.New(pool)
	enc, err := crypto.NewEncryptor([]byte("01234567890123456789012345678901"))
	if err != nil {
		t.Fatal(err)
	}

	encSecret, _ := enc.Encrypt("ignored")
	conn := &storage.Connection{
		Name:               "test",
		SourceType:         "internal",
		InboundPath:        "duppath1",
		DestinationURL:     "http://example.com/hook",
		SigningSecretEnc:   encSecret,
		RetryAttempts:      3,
		RetryFirstDelayS:   1,
		RetryBackoffFactor: 2,
		RetentionDays:      7,
	}
	if err := store.CreateConnection(ctx, conn); err != nil {
		t.Fatal(err)
	}

	cfg := &config.Config{PublicBaseURL: "http://localhost:8080", MaxBodyBytes: 1 << 20}
	temporal := &noopTemporal{}
	srv := NewServer(cfg, store, enc, temporal, slog.Default())

	body := []byte(`{"id":"evt_dup_1","type":"test.event"}`)
	post := func() *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "/e/duppath1", bytes.NewReader(body))
		req.SetPathValue("path", "duppath1")
		req.Header.Set("X-Tuma-Event-Id", "evt_dup_1")
		rec := httptest.NewRecorder()
		srv.ingest(rec, req)
		return rec
	}

	first := post()
	if first.Code != http.StatusOK {
		t.Fatalf("first ingest: expected 200, got %d", first.Code)
	}
	second := post()
	if second.Code != http.StatusOK {
		t.Fatalf("duplicate ingest: expected 200, got %d", second.Code)
	}
	if temporal.calls != 1 {
		t.Fatalf("duplicate must not start second workflow; got %d starts", temporal.calls)
	}

	events, err := store.GetEventByProviderID(ctx, conn.ID, "evt_dup_1")
	if err != nil || events == nil {
		t.Fatal("original event must still exist after duplicate ingest")
	}
}

func testPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	url := getenv("TEST_DATABASE_URL", "postgres://tuma:tuma@localhost:5432/tuma?sslmode=disable")
	pool, err := pgxpool.New(context.Background(), url)
	if err != nil {
		t.Skip("postgres not available:", err)
	}
	if err := pool.Ping(context.Background()); err != nil {
		t.Skip("postgres not available:", err)
	}
	_, _ = pool.Exec(context.Background(), `TRUNCATE issues, deliveries, events, connections, sessions, users CASCADE`)
	return pool
}

func getenv(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}
