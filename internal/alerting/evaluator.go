// Package alerting contains the background evaluator that periodically checks
// active alert rules against live platform metrics and fires notifications when
// a threshold is breached.
package alerting

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/koto7/tuma/internal/notification"
	"github.com/koto7/tuma/internal/storage"
)

// Store defines the storage operations required by Evaluator.
type Store interface {
	ListAlertRules(ctx context.Context) ([]storage.AlertRule, error)
	GetPlatformMetrics(ctx context.Context) (*storage.PlatformMetrics, error)
	OldestOpenIssueAge(ctx context.Context) (float64, error)
	RecordAlertNotification(ctx context.Context, n *storage.AlertNotification) error
}

// Notifier is the interface the Evaluator uses to dispatch email notifications.
// notification.Service satisfies this interface.
type Notifier interface {
	Send(ctx context.Context, req notification.Request) error
}

// Evaluator runs a ticker loop that checks every active alert rule, records a
// notification row in the DB, and dispatches an HTTP notification (Slack or Email)
// when the rule's condition is breached.
// It fires ONLY ONCE per breach event — it will not spam or resend notifications
// on subsequent ticks while the metric remains crossed, until the metric recovers.
type Evaluator struct {
	store      Store
	httpClient *http.Client
	notifier   Notifier
	interval   time.Duration
	logger     *slog.Logger
	firing     map[uuid.UUID]float64 // tracks rules currently in breached state and their threshold
}

// New creates an Evaluator with the given store, polling interval, and logger.
// notifier handles email dispatch; pass nil to disable email notifications.
func New(store Store, notifier Notifier, interval time.Duration, logger *slog.Logger) *Evaluator {
	return &Evaluator{
		store:      store,
		httpClient: &http.Client{Timeout: 10 * time.Second},
		notifier:   notifier,
		interval:   interval,
		logger:     logger,
		firing:     make(map[uuid.UUID]float64),
	}
}

// Run starts the evaluation loop and blocks until ctx is cancelled.
func (e *Evaluator) Run(ctx context.Context) {
	e.logger.Info("alert evaluator started", "interval", e.interval)
	ticker := time.NewTicker(e.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			e.logger.Info("alert evaluator stopped")
			return
		case <-ticker.C:
			if err := e.evaluateOnce(ctx); err != nil {
				e.logger.Error("alert evaluation error", "error", err)
			}
		}
	}
}

func (e *Evaluator) evaluateOnce(ctx context.Context) error {
	rules, err := e.store.ListAlertRules(ctx)
	if err != nil {
		return fmt.Errorf("list rules: %w", err)
	}

	// Fetch platform metrics once — shared by OPEN_ISSUES and DELIVERY_SUCCESS.
	metrics, err := e.store.GetPlatformMetrics(ctx)
	if err != nil {
		return fmt.Errorf("get metrics: %w", err)
	}

	activeRuleIDs := make(map[uuid.UUID]bool)

	for _, rule := range rules {
		if !rule.Active {
			delete(e.firing, rule.ID)
			continue
		}
		activeRuleIDs[rule.ID] = true

		current, err := e.currentValue(ctx, rule, metrics)
		if err != nil {
			e.logger.Warn("could not compute rule value", "rule_id", rule.ID, "error", err)
			continue
		}

		breached := isBreached(rule, current)

		if !breached {
			// Condition is normal. If it was previously firing, it has recovered.
			if _, wasFiring := e.firing[rule.ID]; wasFiring {
				e.logger.Info("alert rule recovered to normal",
					"rule_id", rule.ID,
					"rule_name", rule.Name,
					"current", current,
					"threshold", rule.Threshold,
				)
				delete(e.firing, rule.ID)
			}
			continue
		}

		// Condition is breached!
		// If we have already sent a notification for this breach at this threshold, skip sending again.
		prevThreshold, alreadyFiring := e.firing[rule.ID]
		if alreadyFiring && prevThreshold == rule.Threshold {
			continue
		}

		// Mark as firing so future ticks do not spam duplicate notifications.
		e.firing[rule.ID] = rule.Threshold

		e.logger.Info("alert rule breached (sending notification)",
			"rule_id", rule.ID,
			"rule_name", rule.Name,
			"rule_type", rule.RuleType,
			"threshold", rule.Threshold,
			"current", current,
		)

		// Persist the firing so the frontend can read it via GET /api/alert-notifications.
		n := &storage.AlertNotification{
			AlertRuleID:  rule.ID,
			RuleName:     rule.Name,
			RuleType:     rule.RuleType,
			Threshold:    rule.Threshold,
			CurrentValue: current,
		}
		if err := e.store.RecordAlertNotification(ctx, n); err != nil {
			e.logger.Warn("failed to record alert notification", "error", err)
		}

		// Dispatch external notification (best-effort).
		if err := e.sendNotification(ctx, rule, current); err != nil {
			e.logger.Warn("notification dispatch failed", "rule_id", rule.ID,
				"type", rule.NotificationType, "error", err)
		}
	}

	// Clean up any deleted rules from state map.
	for id := range e.firing {
		if !activeRuleIDs[id] {
			delete(e.firing, id)
		}
	}

	return nil
}

// currentValue returns the live metric for the rule type.
//
//   - OPEN_ISSUES      → total open issues count
//   - DELIVERY_SUCCESS → success percentage over the last 24 h (0-100)
//   - UNRESOLVED_TIME  → hours the oldest open issue has been open
func (e *Evaluator) currentValue(
	ctx context.Context,
	rule storage.AlertRule,
	metrics *storage.PlatformMetrics,
) (float64, error) {
	switch rule.RuleType {
	case "OPEN_ISSUES":
		return float64(metrics.OpenIssues), nil

	case "DELIVERY_SUCCESS":
		total := metrics.DeliveriesDelivered24h + metrics.DeliveriesFailed24h
		if total == 0 {
			return 100, nil // no traffic → treat as 100 % (no alarm)
		}
		return float64(metrics.DeliveriesDelivered24h) / float64(total) * 100, nil

	case "UNRESOLVED_TIME":
		return e.store.OldestOpenIssueAge(ctx)

	default:
		return 0, fmt.Errorf("unknown rule_type: %s", rule.RuleType)
	}
}

// isBreached returns true when the threshold condition is violated.
//
//   - DELIVERY_SUCCESS fires when current < threshold (success rate too low)
//   - All others fire when current > threshold (value too high)
func isBreached(rule storage.AlertRule, current float64) bool {
	if rule.RuleType == "DELIVERY_SUCCESS" {
		return current < rule.Threshold
	}
	return current > rule.Threshold
}

// ─── Notification dispatch ────────────────────────────────────────────────────

func (e *Evaluator) sendNotification(ctx context.Context, rule storage.AlertRule, current float64) error {
	switch rule.NotificationType {
	case "slack":
		return e.postSlack(ctx, rule.NotificationDest, formatMessage(rule, current))
	case "email":
		if e.notifier == nil {
			e.logger.Warn("email notification skipped: no notifier configured", "rule_id", rule.ID)
			return nil
		}
		var customSubj, customBody string
		if rule.SubjectTemplate != nil {
			customSubj = *rule.SubjectTemplate
		}
		if rule.BodyTemplate != nil {
			customBody = *rule.BodyTemplate
		}
		return e.notifier.Send(ctx, notification.Request{
			Type:          notificationTypeFor(rule.RuleType),
			Recipient:     rule.NotificationDest,
			CustomSubject: customSubj,
			CustomBody:    customBody,
			Vars: map[string]string{
				"resource_name": rule.Name,
				"limit":         formatValue(rule.Unit, rule.Threshold),
				"current_value": formatValue(rule.Unit, current),
			},
		})
	default:
		return fmt.Errorf("unknown notification_type: %s", rule.NotificationType)
	}
}

// notificationTypeFor maps an alert rule type to its default email template.
func notificationTypeFor(ruleType string) notification.Type {
	switch ruleType {
	case "DELIVERY_SUCCESS":
		return notification.TypeDeliverySuccessDropped
	case "UNRESOLVED_TIME":
		return notification.TypeIssueUnresolved
	default:
		return notification.TypeOpenIssuesExceeded
	}
}

// formatValue renders a metric for humans: whole numbers for counts,
// one decimal place for percentages and hours (trailing ".0" dropped).
func formatValue(unit string, v float64) string {
	if unit == "COUNT" {
		return strconv.FormatFloat(math.Round(v), 'f', 0, 64)
	}
	return strconv.FormatFloat(math.Round(v*10)/10, 'f', -1, 64)
}

func formatMessage(rule storage.AlertRule, current float64) string {
	limit := formatValue(rule.Unit, rule.Threshold)
	value := formatValue(rule.Unit, current)
	if rule.BodyTemplate != nil && *rule.BodyTemplate != "" {
		msg := *rule.BodyTemplate
		msg = strings.ReplaceAll(msg, "{{resource_name}}", rule.Name)
		msg = strings.ReplaceAll(msg, "{{limit}}", limit)
		msg = strings.ReplaceAll(msg, "{{current_value}}", value)
		return msg
	}
	switch rule.RuleType {
	case "OPEN_ISSUES":
		return fmt.Sprintf("Your open issues have exceeded the configured limit. Current open issues: %s, Threshold: %s", value, limit)
	case "DELIVERY_SUCCESS":
		return fmt.Sprintf("Your delivery success rate has dropped below the configured threshold. Current success rate: %s%%, Threshold: %s%%", value, limit)
	case "UNRESOLVED_TIME":
		return fmt.Sprintf("An open issue has remained unresolved longer than the configured threshold. Oldest issue age: %sh, Threshold: %sh", value, limit)
	default:
		return fmt.Sprintf("[TUMA ALERT] %s — %s exceeded threshold (current: %s, limit: %s)", rule.Name, rule.RuleType, value, limit)
	}
}

// postSlack sends a plain-text message to a Slack incoming webhook URL.
func (e *Evaluator) postSlack(ctx context.Context, webhookURL, text string) error {
	body, _ := json.Marshal(map[string]string{"text": text})
	return e.doPost(ctx, webhookURL, "application/json", body)
}

func (e *Evaluator) doPost(ctx context.Context, url, contentType string, body []byte) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", contentType)
	resp, err := e.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("notification endpoint returned HTTP %d", resp.StatusCode)
	}
	return nil
}
