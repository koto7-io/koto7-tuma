package api

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.temporal.io/sdk/client"

	"github.com/koto7/tuma/internal/adapters"
	"github.com/koto7/tuma/internal/config"
	"github.com/koto7/tuma/internal/crypto"
	"github.com/koto7/tuma/internal/metrics"
	"github.com/koto7/tuma/internal/playground"
	"github.com/koto7/tuma/internal/storage"
	tumawf "github.com/koto7/tuma/internal/workflow"
)

type workflowStarter interface {
	ExecuteWorkflow(ctx context.Context, options client.StartWorkflowOptions, workflow interface{}, args ...interface{}) (client.WorkflowRun, error)
}

type Server struct {
	cfg               *config.Config
	store             *storage.Store
	encryptor         *crypto.Encryptor
	temporal          workflowStarter
	logger            *slog.Logger
	connCache         sync.Map // inbound_path -> cachedConnection
	playgroundHub     *playground.Hub
	playgroundRL      *rateLimiter
	playgroundStreams *streamLimiter
	sink              *echoSink
}

type cachedConnection struct {
	conn      *storage.Connection
	secret    string
	expiresAt time.Time
}

func NewServer(cfg *config.Config, store *storage.Store, enc *crypto.Encryptor, temporal workflowStarter, logger *slog.Logger) *Server {
	s := &Server{cfg: cfg, store: store, encryptor: enc, temporal: temporal, logger: logger, sink: newEchoSink()}
	s.initPlayground()
	return s
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", s.healthz)
	mux.HandleFunc("GET /readyz", s.readyz)
	mux.Handle("GET /metrics", promhttp.Handler())

	mux.HandleFunc("POST /e/{path}", s.ingest)
	mux.HandleFunc("POST /sink/echo", s.sinkReceive)
	mux.HandleFunc("POST /api/auth/login", s.login)
	mux.HandleFunc("POST /api/auth/logout", s.logout)

	mux.HandleFunc("GET /api/connections", s.auth(s.listConnections))
	mux.HandleFunc("POST /api/connections", s.auth(s.createConnection))
	mux.HandleFunc("GET /api/connections/{id}", s.auth(s.getConnection))
	mux.HandleFunc("PATCH /api/connections/{id}", s.auth(s.patchConnection))
	mux.HandleFunc("GET /api/connections/{id}/deliveries", s.auth(s.listDeliveries))

	mux.HandleFunc("GET /api/metrics", s.auth(s.getMetrics))

	mux.HandleFunc("GET /api/issues", s.auth(s.listIssues))
	mux.HandleFunc("GET /api/issues/{id}", s.auth(s.getIssue))
	mux.HandleFunc("POST /api/issues/{id}/replay", s.auth(s.replayIssue))
	mux.HandleFunc("POST /api/issues/replay-bulk", s.auth(s.replayBulk))

	mux.HandleFunc("GET /api/alert-rules", s.auth(s.listAlertRules))
	mux.HandleFunc("POST /api/alert-rules", s.auth(s.createAlertRule))
	mux.HandleFunc("PATCH /api/alert-rules/{id}", s.auth(s.patchAlertRule))
	mux.HandleFunc("DELETE /api/alert-rules/{id}", s.auth(s.deleteAlertRule))
	mux.HandleFunc("GET /api/alert-notifications", s.auth(s.listAlertNotifications))

	mux.HandleFunc("GET /api/sink", s.auth(s.getSink))
	mux.HandleFunc("POST /api/sink/break", s.auth(s.sinkBreak))
	mux.HandleFunc("POST /api/sink/fix", s.auth(s.sinkFix))

	mux.HandleFunc("GET /api/config", s.getConfig)
	s.registerPlaygroundRoutes(mux)

	return mux
}

func (s *Server) healthz(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) readyz(w http.ResponseWriter, r *http.Request) {
	if err := s.store.Ping(r.Context()); err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"status": "not ready", "error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ready"})
}

func (s *Server) getCachedConnection(ctx context.Context, path string) (*storage.Connection, string, error) {
	if v, ok := s.connCache.Load(path); ok {
		c := v.(cachedConnection)
		if time.Now().Before(c.expiresAt) {
			return c.conn, c.secret, nil
		}
	}
	conn, err := s.store.GetConnectionByInboundPath(ctx, path)
	if err != nil || conn == nil {
		return nil, "", err
	}
	secret, err := s.encryptor.Decrypt(conn.SigningSecretEnc)
	if err != nil {
		return nil, "", err
	}
	s.connCache.Store(path, cachedConnection{conn: conn, secret: secret, expiresAt: time.Now().Add(30 * time.Second)})
	return conn, secret, nil
}

func (s *Server) ingest(w http.ResponseWriter, r *http.Request) {
	path := r.PathValue("path")
	body, err := readBodyLimit(w, r, s.cfg.MaxBodyBytes)
	if err != nil {
		http.Error(w, "payload too large", http.StatusRequestEntityTooLarge)
		return
	}

	conn, secret, err := s.getCachedConnection(r.Context(), path)
	if err != nil || conn == nil {
		http.NotFound(w, r)
		return
	}

	adapter, err := adapters.Get(conn.SourceType)
	if err != nil {
		http.Error(w, "misconfigured connection", http.StatusInternalServerError)
		return
	}
	if !adapter.Verify(r.Header, body, secret) {
		metrics.IngestionTotal.WithLabelValues(conn.ID.String(), "invalid_signature").Inc()
		s.logger.Warn("ingest rejected: invalid signature",
			"connection_id", conn.ID,
			"source_type", conn.SourceType,
			"has_stripe_signature", r.Header.Get("Stripe-Signature") != "",
		)
		http.Error(w, "invalid signature", http.StatusUnauthorized)
		return
	}

	providerID := adapter.ExtractEventID(r.Header, body)
	headersJSON, _ := json.Marshal(r.Header)

	event := &storage.Event{
		ConnectionID:    conn.ID,
		ProviderEventID: providerID,
		RawHeaders:      headersJSON,
		RawPayload:      body,
	}

	result, err := s.store.InsertEvent(r.Context(), event)
	if err != nil {
		s.logger.Error("insert event failed", "error", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	// Ack only after durable write commits
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"received":true}`))

	if !result.Inserted {
		metrics.IngestionTotal.WithLabelValues(conn.ID.String(), "duplicate").Inc()
		return
	}

	metrics.IngestionTotal.WithLabelValues(conn.ID.String(), "accepted").Inc()

	wfID := tumawf.WorkflowID(result.Event.ID)
	_, err = s.temporal.ExecuteWorkflow(context.Background(), client.StartWorkflowOptions{
		ID:        wfID,
		TaskQueue: tumawf.TaskQueue,
	}, tumawf.DeliveryWorkflow, result.Event.ID)
	if err != nil {
		s.logger.Error("start workflow failed", "event_id", result.Event.ID, "error", err)
		return
	}
	_ = s.store.SetEventWorkflowID(r.Context(), result.Event.ID, wfID)
}

func (s *Server) startReplay(ctx context.Context, eventID uuid.UUID) error {
	wfID := tumawf.ReplayWorkflowID(eventID)
	_, err := s.temporal.ExecuteWorkflow(ctx, client.StartWorkflowOptions{
		ID:        wfID,
		TaskQueue: tumawf.TaskQueue,
	}, tumawf.DeliveryWorkflow, eventID)
	return err
}

func readBodyLimit(w http.ResponseWriter, r *http.Request, max int64) ([]byte, error) {
	r.Body = http.MaxBytesReader(w, r.Body, max)
	defer r.Body.Close()
	return io.ReadAll(r.Body)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func randomSecret() string {
	b := make([]byte, 24)
	_, _ = rand.Read(b)
	return "whsec_" + hex.EncodeToString(b)
}
