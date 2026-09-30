package alerting

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"strings"
	"sync"
	"testing"

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

func (m *mockStore) SetAlertRuleFiring(ctx context.Context, id uuid.UUID, threshold *float64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i := range m.rules {
		if m.rules[i].ID != id {
			continue
		}
		if threshold == nil {
			m.rules[i].FiringThreshold = nil
			return nil
		}
		v := *threshold
		m.rules[i].FiringThreshold = &v
		return nil
	}
	return storage.ErrAlertRuleNotFound
}

func (m *mockStore) SetAlertRuleDeliveryError(ctx context.Context, id uuid.UUID, message *string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i := range m.rules {
		if m.rules[i].ID != id {
			continue
		}
		if message == nil {
			m.rules[i].DeliveryError = nil
			return nil
		}
		v := *message
		m.rules[i].DeliveryError = &v
		return nil
	}
	return storage.ErrAlertRuleNotFound
}

func (m *mockStore) firing(id uuid.UUID) *float64 {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i := range m.rules {
		if m.rules[i].ID == id {
			return m.rules[i].FiringThreshold
		}
	}
	return nil
}

func (m *mockStore) deliveryError(id uuid.UUID) string {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i := range m.rules {
		if m.rules[i].ID == id && m.rules[i].DeliveryError != nil {
			return *m.rules[i].DeliveryError
		}
	}
	return ""
}

type mockNotifier struct {
	mu   sync.Mutex
	reqs []notification.Request
	err  error
}

func (m *mockNotifier) Send(ctx context.Context, req notification.Request) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.err != nil {
		return m.err
	}
	m.reqs = append(m.reqs, req)
	return nil
}

func (m *mockNotifier) count() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.reqs)
}

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func emailRule(id uuid.UUID, ruleType, unit string, threshold float64) storage.AlertRule {
	return storage.AlertRule{
		ID:               id,
		Name:             "rule",
		RuleType:         ruleType,
		Threshold:        threshold,
		Unit:             unit,
		Active:           true,
		NotificationType: "email",
		NotificationDest: "ops@example.com",
	}
}

func TestEvaluator_OpenIssuesExceeded_SendsOnlyOneNotification(t *testing.T) {
	ruleID := uuid.New()
	rule := emailRule(ruleID, "OPEN_ISSUES", "COUNT", 5)
	store := &mockStore{
		rules:   []storage.AlertRule{rule},
		metrics: &storage.PlatformMetrics{OpenIssues: 10},
	}
	notifier := &mockNotifier{}
	ev := New(store, notifier, nil, 0, testLogger())
	ctx := context.Background()

	if err := ev.evaluateOnce(ctx); err != nil {
		t.Fatalf("tick 1: %v", err)
	}
	if notifier.count() != 1 {
		t.Fatalf("expected 1 email on initial breach, got %d", notifier.count())
	}
	if len(store.notifications) != 1 {
		t.Fatalf("expected 1 notification row, got %d", len(store.notifications))
	}
	if store.notifications[0].CurrentValue != 10 {
		t.Errorf("current value = %v", store.notifications[0].CurrentValue)
	}
	if store.firing(ruleID) == nil {
		t.Fatal("expected firing state after a successful send")
	}
	if notifier.reqs[0].Type != notification.TypeOpenIssuesExceeded {
		t.Errorf("type = %s", notifier.reqs[0].Type)
	}
	if notifier.reqs[0].Vars["current_value"] != "10" || notifier.reqs[0].Vars["limit"] != "5" {
		t.Errorf("vars = %#v", notifier.reqs[0].Vars)
	}

	if err := ev.evaluateOnce(ctx); err != nil {
		t.Fatalf("tick 2: %v", err)
	}
	if notifier.count() != 1 || len(store.notifications) != 1 {
		t.Fatalf("still-breached tick resent: emails=%d rows=%d", notifier.count(), len(store.notifications))
	}

	store.mu.Lock()
	store.metrics.OpenIssues = 3
	store.mu.Unlock()
	if err := ev.evaluateOnce(ctx); err != nil {
		t.Fatalf("recovery: %v", err)
	}
	if store.firing(ruleID) != nil {
		t.Fatal("expected firing state cleared after recovery")
	}
	if notifier.count() != 1 {
		t.Fatalf("recovery sent mail: %d", notifier.count())
	}

	store.mu.Lock()
	store.metrics.OpenIssues = 8
	store.mu.Unlock()
	if err := ev.evaluateOnce(ctx); err != nil {
		t.Fatalf("re-breach: %v", err)
	}
	if notifier.count() != 2 || len(store.notifications) != 2 {
		t.Fatalf("re-breach: emails=%d rows=%d", notifier.count(), len(store.notifications))
	}
}

func TestEvaluator_SendFailure_DoesNotLatch(t *testing.T) {
	ruleID := uuid.New()
	store := &mockStore{
		rules:   []storage.AlertRule{emailRule(ruleID, "OPEN_ISSUES", "COUNT", 5)},
		metrics: &storage.PlatformMetrics{OpenIssues: 10},
	}
	notifier := &mockNotifier{err: errors.New("smtp down")}
	ev := New(store, notifier, nil, 0, testLogger())

	if err := ev.evaluateOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(store.notifications) != 0 {
		t.Fatalf("recorded a notification for a failed send: %d", len(store.notifications))
	}
	if store.firing(ruleID) != nil {
		t.Fatal("latched a failed send")
	}
	if !strings.Contains(store.deliveryError(ruleID), "Email failed") {
		t.Fatalf("delivery error = %q", store.deliveryError(ruleID))
	}

	notifier.mu.Lock()
	notifier.err = nil
	notifier.mu.Unlock()
	if err := ev.evaluateOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	if notifier.count() != 1 {
		t.Fatalf("expected retry after failure, got %d successful sends", notifier.count())
	}
	if store.firing(ruleID) == nil {
		t.Fatal("expected latch after the retry succeeded")
	}
	if store.deliveryError(ruleID) != "" {
		t.Fatalf("delivery error was not cleared: %q", store.deliveryError(ruleID))
	}
}

func TestEvaluator_InactiveRule_DoesNotNotify(t *testing.T) {
	rule := emailRule(uuid.New(), "OPEN_ISSUES", "COUNT", 2)
	rule.Active = false
	store := &mockStore{
		rules:   []storage.AlertRule{rule},
		metrics: &storage.PlatformMetrics{OpenIssues: 10},
	}
	notifier := &mockNotifier{}
	ev := New(store, notifier, nil, 0, testLogger())
	if err := ev.evaluateOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	if notifier.count() != 0 {
		t.Fatalf("inactive rule sent %d emails", notifier.count())
	}
}

func TestEvaluator_TemplateFollowsRuleType(t *testing.T) {
	subj := "Delivery {{resource_name}}"
	body := "rate {{current_value}} limit {{limit}}"
	rule := emailRule(uuid.New(), "DELIVERY_SUCCESS", "PERCENT", 95)
	rule.SubjectTemplate = &subj
	rule.BodyTemplate = &body
	store := &mockStore{
		rules: []storage.AlertRule{rule},
		metrics: &storage.PlatformMetrics{
			DeliveriesDelivered24h: 80,
			DeliveriesFailed24h:    20,
		},
	}
	notifier := &mockNotifier{}
	ev := New(store, notifier, nil, 0, testLogger())
	if err := ev.evaluateOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	if notifier.count() != 1 {
		t.Fatalf("sends = %d", notifier.count())
	}
	got := notifier.reqs[0]
	if got.Type != notification.TypeDeliverySuccessDropped {
		t.Errorf("type = %s", got.Type)
	}
	if got.CustomSubject != subj || got.CustomBody != body {
		t.Errorf("custom templates dropped: %#v", got)
	}
	if got.Vars["current_value"] != "80" || got.Vars["limit"] != "95" {
		t.Errorf("vars = %#v", got.Vars)
	}
}

func TestEvaluator_UnresolvedTime_UsesItsTemplate(t *testing.T) {
	store := &mockStore{
		rules:     []storage.AlertRule{emailRule(uuid.New(), "UNRESOLVED_TIME", "HOURS", 4)},
		metrics:   &storage.PlatformMetrics{},
		oldestAge: 9.25,
	}
	notifier := &mockNotifier{}
	ev := New(store, notifier, nil, 0, testLogger())
	if err := ev.evaluateOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	if notifier.count() != 1 {
		t.Fatalf("sends = %d", notifier.count())
	}
	got := notifier.reqs[0]
	if got.Type != notification.TypeIssueUnresolved {
		t.Errorf("type = %s", got.Type)
	}
	if got.CustomSubject != "" || got.CustomBody != "" {
		t.Errorf("expected default template, got custom subject %q body %q", got.CustomSubject, got.CustomBody)
	}
	if got.Vars["current_value"] != "9.3" || got.Vars["limit"] != "4" {
		t.Errorf("vars = %#v", got.Vars)
	}
}

func TestEvaluator_DeliverySuccess_NoSamples_DoesNotFire(t *testing.T) {
	store := &mockStore{
		rules:   []storage.AlertRule{emailRule(uuid.New(), "DELIVERY_SUCCESS", "PERCENT", 90)},
		metrics: &storage.PlatformMetrics{},
	}
	notifier := &mockNotifier{}
	ev := New(store, notifier, nil, 0, testLogger())
	if err := ev.evaluateOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	if notifier.count() != 0 {
		t.Fatalf("zero traffic sent %d emails", notifier.count())
	}
}

type mockSlack struct {
	userID string
	text   string
	err    error
	calls  int
}

func (m *mockSlack) PostDM(ctx context.Context, userID, text string) error {
	m.calls++
	m.userID = userID
	m.text = text
	return m.err
}

func TestEvaluator_SlackDM_UsesRenderedTemplate(t *testing.T) {
	subj := "Hello {{resource_name}}"
	body := "open {{current_value}}"
	rule := emailRule(uuid.New(), "OPEN_ISSUES", "COUNT", 1)
	rule.NotificationType = "slack_dm"
	rule.NotificationDest = "U012ABCDEF"
	rule.SubjectTemplate = &subj
	rule.BodyTemplate = &body
	store := &mockStore{
		rules:   []storage.AlertRule{rule},
		metrics: &storage.PlatformMetrics{OpenIssues: 4},
	}
	email := &mockNotifier{}
	slack := &mockSlack{}
	ev := New(store, email, slack, 0, testLogger())
	if err := ev.evaluateOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	if email.count() != 0 {
		t.Fatal("slack rule sent email")
	}
	if slack.calls != 1 || slack.userID != "U012ABCDEF" {
		t.Fatalf("slack calls=%d user=%s", slack.calls, slack.userID)
	}
	if slack.text != "Hello rule\n\nopen 4" {
		t.Fatalf("dm text = %q", slack.text)
	}
}

func TestEvaluator_SlackDM_FailureDoesNotLatch(t *testing.T) {
	ruleID := uuid.New()
	rule := emailRule(ruleID, "OPEN_ISSUES", "COUNT", 1)
	rule.NotificationType = "slack_dm"
	rule.NotificationDest = "U012ABCDEF"
	store := &mockStore{
		rules:   []storage.AlertRule{rule},
		metrics: &storage.PlatformMetrics{OpenIssues: 4},
	}
	slack := &mockSlack{err: errors.New("slack: messages_tab_disabled")}
	ev := New(store, nil, slack, 0, testLogger())
	if err := ev.evaluateOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	if store.firing(ruleID) != nil || len(store.notifications) != 0 {
		t.Fatal("failed slack DM was latched")
	}
	if !strings.Contains(store.deliveryError(ruleID), "blocked this DM") {
		t.Fatalf("delivery error = %q", store.deliveryError(ruleID))
	}
}

func TestEvaluator_NilNotifier_DoesNotLatch(t *testing.T) {
	ruleID := uuid.New()
	store := &mockStore{
		rules:   []storage.AlertRule{emailRule(ruleID, "OPEN_ISSUES", "COUNT", 1)},
		metrics: &storage.PlatformMetrics{OpenIssues: 3},
	}
	ev := New(store, nil, nil, 0, testLogger())
	if err := ev.evaluateOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	if store.firing(ruleID) != nil || len(store.notifications) != 0 {
		t.Fatal("nil notifier was treated as a successful send")
	}
}
