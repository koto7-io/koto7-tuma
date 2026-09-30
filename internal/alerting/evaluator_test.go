package alerting

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/koto7/tuma/internal/notification"
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

// capturingSender records every email the notification service would send.
type capturingSender struct {
	mu   sync.Mutex
	sent []sentEmail
}

type sentEmail struct{ recipient, subject, body string }

func (c *capturingSender) Send(_ context.Context, recipient, subject, body string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.sent = append(c.sent, sentEmail{recipient, subject, body})
	return nil
}

func (c *capturingSender) emails() []sentEmail {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]sentEmail(nil), c.sent...)
}

// newEmailEvaluator wires an Evaluator to the real notification.Service backed
// by a capturingSender, so tests see the exact subject and body a user gets.
func newEmailEvaluator(store *mockStore) (*Evaluator, *capturingSender) {
	sender := &capturingSender{}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	svc := notification.NewService(sender, logger)
	return New(store, svc, 1*time.Second, logger), sender
}

// TestEvaluator_DeliverySuccess_EmailUsesDeliveryTemplate verifies that a
// DELIVERY_SUCCESS rule without custom templates sends the delivery-success
// email (not the open-issues one) with rounded values, only once breached.
func TestEvaluator_DeliverySuccess_EmailUsesDeliveryTemplate(t *testing.T) {
	store := &mockStore{
		rules: []storage.AlertRule{{
			ID:               uuid.New(),
			Name:             "Delivery test",
			RuleType:         "DELIVERY_SUCCESS",
			Threshold:        98,
			Unit:             "PERCENT",
			Active:           true,
			NotificationType: "email",
			NotificationDest: "ops@example.com",
		}},
		metrics: &storage.PlatformMetrics{}, // no traffic → treated as 100 %
	}
	evaluator, sender := newEmailEvaluator(store)
	ctx := context.Background()

	// --- Tick 1: no traffic (100 %) is not below 98 %. No email. ---
	if err := evaluator.evaluateOnce(ctx); err != nil {
		t.Fatalf("Tick 1 evaluation failed: %v", err)
	}
	if got := len(sender.emails()); got != 0 {
		t.Fatalf("expected no email with no traffic, got %d", got)
	}

	// --- Tick 2: 292 delivered, 8 failed → 97.333… %, below 98 %. One email. ---
	store.mu.Lock()
	store.metrics.DeliveriesDelivered24h = 292
	store.metrics.DeliveriesFailed24h = 8
	store.mu.Unlock()

	if err := evaluator.evaluateOnce(ctx); err != nil {
		t.Fatalf("Tick 2 evaluation failed: %v", err)
	}
	emails := sender.emails()
	if len(emails) != 1 {
		t.Fatalf("expected 1 email after breach, got %d", len(emails))
	}
	email := emails[0]
	if email.recipient != "ops@example.com" {
		t.Errorf("expected recipient ops@example.com, got %q", email.recipient)
	}
	if !strings.Contains(email.subject, "Delivery Success Dropped") {
		t.Errorf("expected delivery-success subject, got %q", email.subject)
	}
	if strings.Contains(email.subject, "Open Issues") || strings.Contains(email.body, "open issues") {
		t.Errorf("delivery-success rule must not use the open-issues template, got subject %q body:\n%s", email.subject, email.body)
	}
	for _, want := range []string{": 97.3%", ": 98%"} {
		if !strings.Contains(email.body, want) {
			t.Errorf("expected body to contain %q, got:\n%s", want, email.body)
		}
	}
	if strings.Contains(email.body, "97.33") {
		t.Errorf("expected rounded success rate, got:\n%s", email.body)
	}
}

// TestEvaluator_UnresolvedTime_EmailUsesUnresolvedTemplate verifies that an
// UNRESOLVED_TIME rule without custom templates sends the unresolved-issue
// email (not the open-issues one) with rounded hours, only once breached.
func TestEvaluator_UnresolvedTime_EmailUsesUnresolvedTemplate(t *testing.T) {
	store := &mockStore{
		rules: []storage.AlertRule{{
			ID:               uuid.New(),
			Name:             "Unresolved test",
			RuleType:         "UNRESOLVED_TIME",
			Threshold:        6,
			Unit:             "HOURS",
			Active:           true,
			NotificationType: "email",
			NotificationDest: "ops@example.com",
		}},
		metrics:   &storage.PlatformMetrics{},
		oldestAge: 5.9, // below 6 h
	}
	evaluator, sender := newEmailEvaluator(store)
	ctx := context.Background()

	// --- Tick 1: oldest issue 5.9 h is not above 6 h. No email. ---
	if err := evaluator.evaluateOnce(ctx); err != nil {
		t.Fatalf("Tick 1 evaluation failed: %v", err)
	}
	if got := len(sender.emails()); got != 0 {
		t.Fatalf("expected no email below threshold, got %d", got)
	}

	// --- Tick 2: oldest issue 6.48… h is above 6 h. One email. ---
	store.mu.Lock()
	store.oldestAge = 6.482716391666667
	store.mu.Unlock()

	if err := evaluator.evaluateOnce(ctx); err != nil {
		t.Fatalf("Tick 2 evaluation failed: %v", err)
	}
	emails := sender.emails()
	if len(emails) != 1 {
		t.Fatalf("expected 1 email after breach, got %d", len(emails))
	}
	email := emails[0]
	if !strings.Contains(email.subject, "Issue Unresolved") {
		t.Errorf("expected unresolved-issue subject, got %q", email.subject)
	}
	if strings.Contains(email.subject, "Open Issues") || strings.Contains(email.body, "open issues have exceeded") {
		t.Errorf("unresolved-time rule must not use the open-issues template, got subject %q body:\n%s", email.subject, email.body)
	}
	for _, want := range []string{": 6.5h", ": 6h"} {
		if !strings.Contains(email.body, want) {
			t.Errorf("expected body to contain %q, got:\n%s", want, email.body)
		}
	}
}

// TestEvaluator_CustomBodyWithoutSubject_UsesRuleTypeSubject verifies that a
// rule with a custom body but no custom subject falls back to the default
// subject for its own rule type, and that custom bodies get rounded values.
func TestEvaluator_CustomBodyWithoutSubject_UsesRuleTypeSubject(t *testing.T) {
	body := "Rate {{current_value}} (limit {{limit}})"
	store := &mockStore{
		rules: []storage.AlertRule{{
			ID:               uuid.New(),
			Name:             "Delivery test",
			RuleType:         "DELIVERY_SUCCESS",
			Threshold:        98,
			Unit:             "PERCENT",
			Active:           true,
			NotificationType: "email",
			NotificationDest: "ops@example.com",
			BodyTemplate:     &body,
		}},
		metrics: &storage.PlatformMetrics{
			DeliveriesDelivered24h: 292,
			DeliveriesFailed24h:    8,
		},
	}
	evaluator, sender := newEmailEvaluator(store)

	if err := evaluator.evaluateOnce(context.Background()); err != nil {
		t.Fatalf("evaluation failed: %v", err)
	}
	emails := sender.emails()
	if len(emails) != 1 {
		t.Fatalf("expected 1 email, got %d", len(emails))
	}
	if !strings.Contains(emails[0].subject, "Delivery Success Dropped") {
		t.Errorf("expected delivery-success default subject, got %q", emails[0].subject)
	}
	if emails[0].body != "Rate 97.3 (limit 98)" {
		t.Errorf("expected custom body with rounded values, got %q", emails[0].body)
	}
}

func TestFormatValue(t *testing.T) {
	cases := []struct {
		unit string
		in   float64
		want string
	}{
		{"PERCENT", 97.33333333333333, "97.3"},
		{"PERCENT", 98, "98"},
		{"PERCENT", 100, "100"},
		{"HOURS", 6.482716391666667, "6.5"},
		{"HOURS", 6, "6"},
		{"COUNT", 12, "12"},
		{"COUNT", 12.4, "12"},
	}
	for _, c := range cases {
		if got := formatValue(c.unit, c.in); got != c.want {
			t.Errorf("formatValue(%q, %v) = %q, want %q", c.unit, c.in, got, c.want)
		}
	}
}

func TestFormatMessage(t *testing.T) {
	custom := "{{resource_name}}: {{current_value}}/{{limit}}"
	cases := []struct {
		name    string
		rule    storage.AlertRule
		current float64
		want    string
	}{
		{
			name:    "delivery success default",
			rule:    storage.AlertRule{Name: "Delivery test", RuleType: "DELIVERY_SUCCESS", Unit: "PERCENT", Threshold: 98},
			current: 97.33333333333333,
			want:    "Your delivery success rate has dropped below the configured threshold. Current success rate: 97.3%, Threshold: 98%",
		},
		{
			name:    "unresolved time default",
			rule:    storage.AlertRule{Name: "Unresolved test", RuleType: "UNRESOLVED_TIME", Unit: "HOURS", Threshold: 6},
			current: 6.482716391666667,
			want:    "An open issue has remained unresolved longer than the configured threshold. Oldest issue age: 6.5h, Threshold: 6h",
		},
		{
			name:    "open issues default unchanged",
			rule:    storage.AlertRule{Name: "Open", RuleType: "OPEN_ISSUES", Unit: "COUNT", Threshold: 10},
			current: 12,
			want:    "Your open issues have exceeded the configured limit. Current open issues: 12, Threshold: 10",
		},
		{
			name:    "custom template gets rounded values",
			rule:    storage.AlertRule{Name: "Delivery test", RuleType: "DELIVERY_SUCCESS", Unit: "PERCENT", Threshold: 98, BodyTemplate: &custom},
			current: 97.33333333333333,
			want:    "Delivery test: 97.3/98",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := formatMessage(c.rule, c.current); got != c.want {
				t.Errorf("formatMessage() =\n  %q\nwant\n  %q", got, c.want)
			}
		})
	}
}


