// Package alerting contains the background evaluator that periodically checks
// active alert rules against live platform metrics and sends email when a
// threshold is breached.
package alerting

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/koto7/tuma/internal/notification"
	"github.com/koto7/tuma/internal/storage"
)

// errNoSamples means the metric has nothing to measure. The rule is left alone:
// it is not a breach, and it is not a recovery.
var errNoSamples = errors.New("no samples")

// Store defines the storage operations required by Evaluator.
type Store interface {
	ListAlertRules(ctx context.Context) ([]storage.AlertRule, error)
	GetPlatformMetrics(ctx context.Context) (*storage.PlatformMetrics, error)
	OldestOpenIssueAge(ctx context.Context) (float64, error)
	RecordAlertNotification(ctx context.Context, n *storage.AlertNotification) error
	// SetAlertRuleFiring persists the delivered breach. A nil threshold clears it.
	SetAlertRuleFiring(ctx context.Context, id uuid.UUID, threshold *float64) error
}

// Notifier is the interface the Evaluator uses to dispatch email notifications.
// notification.Service satisfies this interface.
type Notifier interface {
	Send(ctx context.Context, req notification.Request) error
}

// Evaluator runs a ticker loop that checks every active alert rule and sends
// email when the rule's condition is breached.
// A rule is marked firing only after Send returns nil, and only once per
// breach, until the metric recovers. A failed send is retried on the next tick.
type Evaluator struct {
	store    Store
	notifier Notifier
	interval time.Duration
	logger   *slog.Logger
}

// New creates an Evaluator with the given store, polling interval, and logger.
// notifier handles email dispatch. A nil notifier makes email rules fail
// (and retry) instead of being treated as delivered.
func New(store Store, notifier Notifier, interval time.Duration, logger *slog.Logger) *Evaluator {
	return &Evaluator{
		store:    store,
		notifier: notifier,
		interval: interval,
		logger:   logger,
	}
}

// Run evaluates once immediately, then on every tick, until ctx is cancelled.
func (e *Evaluator) Run(ctx context.Context) {
	e.logger.Info("alert evaluator started", "interval", e.interval)
	ticker := time.NewTicker(e.interval)
	defer ticker.Stop()
	for {
		if err := e.evaluateOnce(ctx); err != nil {
			e.logger.Error("alert evaluation error", "error", err)
		}
		select {
		case <-ctx.Done():
			e.logger.Info("alert evaluator stopped")
			return
		case <-ticker.C:
		}
	}
}

func (e *Evaluator) evaluateOnce(ctx context.Context) error {
	rules, err := e.store.ListAlertRules(ctx)
	if err != nil {
		return fmt.Errorf("list rules: %w", err)
	}

	metrics, err := e.store.GetPlatformMetrics(ctx)
	if err != nil {
		return fmt.Errorf("get metrics: %w", err)
	}

	for _, rule := range rules {
		if !rule.Active {
			e.clearFiring(ctx, rule)
			continue
		}

		current, err := e.currentValue(ctx, rule, metrics)
		if errors.Is(err, errNoSamples) {
			e.logger.Info("alert rule skipped, no samples",
				"rule_id", rule.ID,
				"rule_type", rule.RuleType,
			)
			continue
		}
		if err != nil {
			e.logger.Warn("could not compute rule value", "rule_id", rule.ID, "error", err)
			continue
		}

		if !isBreached(rule, current) {
			if rule.FiringThreshold != nil {
				e.logger.Info("alert rule recovered to normal",
					"rule_id", rule.ID,
					"rule_name", rule.Name,
					"current", current,
					"threshold", rule.Threshold,
				)
				e.clearFiring(ctx, rule)
			}
			continue
		}

		if rule.FiringThreshold != nil && sameThreshold(*rule.FiringThreshold, rule.Threshold) {
			continue
		}

		e.logger.Info("alert rule breached (sending notification)",
			"rule_id", rule.ID,
			"rule_name", rule.Name,
			"rule_type", rule.RuleType,
			"threshold", rule.Threshold,
			"current", current,
		)

		if err := e.sendNotification(ctx, rule, current); err != nil {
			e.logger.Error("alert notification failed",
				"rule_id", rule.ID,
				"rule_type", rule.RuleType,
				"notification_type", rule.NotificationType,
				"error", err,
			)
			continue
		}

		n := &storage.AlertNotification{
			AlertRuleID:  rule.ID,
			RuleName:     rule.Name,
			RuleType:     rule.RuleType,
			Threshold:    rule.Threshold,
			CurrentValue: current,
		}
		if err := e.store.RecordAlertNotification(ctx, n); err != nil {
			e.logger.Error("alert sent but notification row was not saved",
				"rule_id", rule.ID, "error", err)
		}

		threshold := rule.Threshold
		if err := e.store.SetAlertRuleFiring(ctx, rule.ID, &threshold); err != nil {
			e.logger.Error("alert sent but firing state was not saved",
				"rule_id", rule.ID, "error", err)
		}
	}

	return nil
}

func (e *Evaluator) clearFiring(ctx context.Context, rule storage.AlertRule) {
	if rule.FiringThreshold == nil {
		return
	}
	if err := e.store.SetAlertRuleFiring(ctx, rule.ID, nil); err != nil {
		e.logger.Warn("failed to clear firing state", "rule_id", rule.ID, "error", err)
	}
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
			return 0, errNoSamples
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

func sameThreshold(a, b float64) bool {
	return math.Abs(a-b) < 0.001
}

func (e *Evaluator) sendNotification(ctx context.Context, rule storage.AlertRule, current float64) error {
	if rule.NotificationType != "email" {
		return fmt.Errorf("unknown notification_type: %s", rule.NotificationType)
	}
	if e.notifier == nil {
		return fmt.Errorf("email notifier is not configured")
	}
	req, err := alertRequest(rule, current)
	if err != nil {
		return err
	}
	return e.notifier.Send(ctx, req)
}

func alertRequest(rule storage.AlertRule, current float64) (notification.Request, error) {
	typ, err := notification.TypeForRule(rule.RuleType)
	if err != nil {
		return notification.Request{}, err
	}
	var customSubj, customBody string
	if rule.SubjectTemplate != nil {
		customSubj = *rule.SubjectTemplate
	}
	if rule.BodyTemplate != nil {
		customBody = *rule.BodyTemplate
	}
	return notification.Request{
		Type:          typ,
		Recipient:     rule.NotificationDest,
		CustomSubject: customSubj,
		CustomBody:    customBody,
		Vars: map[string]string{
			"resource_name": rule.Name,
			"limit":         formatMetric(rule.Unit, rule.Threshold),
			"current_value": formatMetric(rule.Unit, current),
		},
	}, nil
}

func formatMetric(unit string, v float64) string {
	if unit == "COUNT" {
		return strconv.FormatFloat(math.Round(v), 'f', 0, 64)
	}
	return strconv.FormatFloat(math.Round(v*10)/10, 'f', -1, 64)
}
