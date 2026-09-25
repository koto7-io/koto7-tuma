package alerting

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/koto7/tuma/internal/storage"
)

type mockStore struct {
	mu            sync.Mutex
	rules         []storage.AlertRule
	metrics       *storage.PlatformMetrics
	oldestAge     float64
	notifications []*storage.AlertNotification
}

func (m *mockStore) ListAlertRules(ctx context.Context) ([]storage.AlertRule, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	copied := make([]storage.AlertRule, len(m.rules))
	copy(copied, m.rules)
	return copied, nil
}

func (m *mockStore) GetPlatformMetrics(ctx context.Context) (*storage.PlatformMetrics, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.metrics, nil
}

func (m *mockStore) OldestOpenIssueAge(ctx context.Context) (float64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.oldestAge, nil
}

func (m *mockStore) RecordAlertNotification(ctx context.Context, n *storage.AlertNotification) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.notifications = append(m.notifications, n)
	return nil
}

// TestEvaluator_OpenIssuesExceeded_SendsOnlyOneNotification verifies:
// 1. When open issues exceed the threshold, exactly 1 notification is dispatched and saved.
// 2. On subsequent ticks while open issues remain exceeded, NO duplicate notification is sent (only 1).
// 3. When open issues recover below threshold, the alert recovers.
// 4. When open issues exceed again later, a new notification is sent.
func TestEvaluator_OpenIssuesExceeded_SendsOnlyOneNotification(t *testing.T) {
	var httpRequestsMu sync.Mutex
	var receivedBodies []string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		httpRequestsMu.Lock()
		receivedBodies = append(receivedBodies, string(body))
		httpRequestsMu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	ruleID := uuid.New()
	rule := storage.AlertRule{
		ID:               ruleID,
		Name:             "Too many open issues",
		RuleType:         "OPEN_ISSUES",
		Threshold:        5,
		Unit:             "COUNT",
		Active:           true,
		NotificationType: "slack",
		NotificationDest: server.URL,
	}

	store := &mockStore{
		rules: []storage.AlertRule{rule},
		metrics: &storage.PlatformMetrics{
			OpenIssues: 10, // Exceeds threshold (10 > 5)
		},
	}

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	evaluator := New(store, nil, 1*time.Second, logger)
	ctx := context.Background()

	// --- Tick 1: Limit is exceeded (10 > 5). Should send 1 notification. ---
	if err := evaluator.evaluateOnce(ctx); err != nil {
		t.Fatalf("Tick 1 evaluation failed: %v", err)
	}

	httpRequestsMu.Lock()
	count := len(receivedBodies)
	httpRequestsMu.Unlock()

	if count != 1 {
		t.Fatalf("expected 1 notification sent on initial breach, got %d", count)
	}
	if len(store.notifications) != 1 {
		t.Fatalf("expected 1 notification recorded in DB, got %d", len(store.notifications))
	}
	if store.notifications[0].CurrentValue != 10 {
		t.Errorf("expected current value 10, got %f", store.notifications[0].CurrentValue)
	}

	// Verify Slack notification payload format
	var slackPayload map[string]string
	_ = json.Unmarshal([]byte(receivedBodies[0]), &slackPayload)
	if slackPayload["text"] == "" {
		t.Errorf("expected non-empty text in slack payload, got: %s", receivedBodies[0])
	}

	// --- Tick 2: Limit STILL exceeded (10 > 5). Must NOT send another notification. ---
	if err := evaluator.evaluateOnce(ctx); err != nil {
		t.Fatalf("Tick 2 evaluation failed: %v", err)
	}

	httpRequestsMu.Lock()
	countAfterTick2 := len(receivedBodies)
	httpRequestsMu.Unlock()

	if countAfterTick2 != 1 {
		t.Fatalf("expected ONLY 1 notification to be sent while limit remains exceeded, but got %d", countAfterTick2)
	}
	if len(store.notifications) != 1 {
		t.Fatalf("expected DB notifications count to remain 1, got %d", len(store.notifications))
	}

	// --- Tick 3: Limit STILL exceeded even higher (12 > 5). Still must NOT duplicate notification. ---
	store.mu.Lock()
	store.metrics.OpenIssues = 12
	store.mu.Unlock()

	if err := evaluator.evaluateOnce(ctx); err != nil {
		t.Fatalf("Tick 3 evaluation failed: %v", err)
	}

	httpRequestsMu.Lock()
	countAfterTick3 := len(receivedBodies)
	httpRequestsMu.Unlock()

	if countAfterTick3 != 1 {
		t.Fatalf("expected ONLY 1 notification even when issue count changed to 12, got %d", countAfterTick3)
	}

	// --- Tick 4: Issues drop to 3 (recovered below threshold 5). ---
	store.mu.Lock()
	store.metrics.OpenIssues = 3
	store.mu.Unlock()

	if err := evaluator.evaluateOnce(ctx); err != nil {
		t.Fatalf("Tick 4 evaluation failed: %v", err)
	}

	// Still only 1 total notification was sent
	httpRequestsMu.Lock()
	countAfterRecovery := len(receivedBodies)
	httpRequestsMu.Unlock()
	if countAfterRecovery != 1 {
		t.Fatalf("expected no notifications during recovery, got %d", countAfterRecovery)
	}

	// State should no longer be firing
	if _, firing := evaluator.firing[ruleID]; firing {
		t.Fatalf("expected rule to be removed from firing map after recovery")
	}

	// --- Tick 5: Issues breach again (8 > 5). Should send a SECOND notification. ---
	store.mu.Lock()
	store.metrics.OpenIssues = 8
	store.mu.Unlock()

	if err := evaluator.evaluateOnce(ctx); err != nil {
		t.Fatalf("Tick 5 evaluation failed: %v", err)
	}

	httpRequestsMu.Lock()
	countAfterSecondBreach := len(receivedBodies)
	httpRequestsMu.Unlock()

	if countAfterSecondBreach != 2 {
		t.Fatalf("expected 2 total notifications after recovery and re-breach, got %d", countAfterSecondBreach)
	}
	if len(store.notifications) != 2 {
		t.Fatalf("expected 2 DB notifications recorded, got %d", len(store.notifications))
	}
}

// TestEvaluator_InactiveRule_DoesNotNotify verifies that inactive rules do not fire.
func TestEvaluator_InactiveRule_DoesNotNotify(t *testing.T) {
	var callCount int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	rule := storage.AlertRule{
		ID:               uuid.New(),
		Name:             "Disabled Rule",
		RuleType:         "OPEN_ISSUES",
		Threshold:        2,
		Unit:             "COUNT",
		Active:           false, // Disabled
		NotificationType: "slack",
		NotificationDest: server.URL,
	}

	store := &mockStore{
		rules: []storage.AlertRule{rule},
		metrics: &storage.PlatformMetrics{
			OpenIssues: 10,
		},
	}

	evaluator := New(store, nil, 1*time.Second, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err := evaluator.evaluateOnce(context.Background()); err != nil {
		t.Fatalf("evaluation failed: %v", err)
	}

	if callCount != 0 {
		t.Fatalf("expected 0 notifications for inactive rule, got %d", callCount)
	}
}


