package api

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/google/uuid"

	"github.com/koto7/tuma/internal/storage"
)

// maxAlertRules is the hard cap enforced at creation time.
const maxAlertRules = 10

// validUnitsForType maps each rule type to its single permitted unit.
var validUnitsForType = map[string]string{
	"OPEN_ISSUES":      "COUNT",
	"DELIVERY_SUCCESS": "PERCENT",
	"UNRESOLVED_TIME":  "HOURS",
}

var validNotificationTypes = map[string]bool{
	"slack": true,
	"email": true,
}

// validateAlertRule checks all fields required for creating a rule.
func validateAlertRule(ruleType, unit string, threshold float64, notifType, notifDest string) string {
	expectedUnit, ok := validUnitsForType[ruleType]
	if !ok {
		return "invalid rule_type: must be one of OPEN_ISSUES, DELIVERY_SUCCESS, UNRESOLVED_TIME"
	}
	if unit != expectedUnit {
		return "invalid unit for rule_type " + ruleType + ": must be " + expectedUnit
	}
	if threshold < 0 {
		return "threshold must be >= 0"
	}
	if ruleType == "DELIVERY_SUCCESS" && threshold > 100 {
		return "threshold for DELIVERY_SUCCESS must be between 0 and 100"
	}
	if !validNotificationTypes[notifType] {
		return "invalid notification_type: must be one of slack, email"
	}
	if notifDest == "" {
		return "notification_dest is required"
	}
	return ""
}

// ─── List alert rules ─────────────────────────────────────────────────────────

func (s *Server) listAlertRules(w http.ResponseWriter, r *http.Request) {
	rules, err := s.store.ListAlertRules(r.Context())
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	rules = jsonSlice(rules)

	activeCount := 0
	for _, rule := range rules {
		if rule.Active {
			activeCount++
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"alert_rules":  rules,
		"active_count": activeCount,
		"total_count":  len(rules),
	})
}

// ─── Create alert rule ────────────────────────────────────────────────────────

type createAlertRuleRequest struct {
	Name             string  `json:"name"`
	RuleType         string  `json:"rule_type"`
	Threshold        float64 `json:"threshold"`
	Unit             string  `json:"unit"`
	NotificationType string  `json:"notification_type"`
	NotificationDest string  `json:"notification_dest"`
	SubjectTemplate  *string `json:"subject_template"`
	BodyTemplate     *string `json:"body_template"`
}

func (s *Server) createAlertRule(w http.ResponseWriter, r *http.Request) {
	var req createAlertRuleRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}

	if req.Name == "" {
		http.Error(w, "name is required", http.StatusBadRequest)
		return
	}
	if msg := validateAlertRule(req.RuleType, req.Unit, req.Threshold, req.NotificationType, req.NotificationDest); msg != "" {
		http.Error(w, msg, http.StatusBadRequest)
		return
	}

	count, err := s.store.CountAlertRules(r.Context())
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	if count >= maxAlertRules {
		http.Error(w, "maximum 10 alert rules allowed", http.StatusBadRequest)
		return
	}

	rule := &storage.AlertRule{
		Name:             req.Name,
		RuleType:         req.RuleType,
		Threshold:        req.Threshold,
		Unit:             req.Unit,
		Active:           true, // new rules start active
		NotificationType: req.NotificationType,
		NotificationDest: req.NotificationDest,
		SubjectTemplate:  req.SubjectTemplate,
		BodyTemplate:     req.BodyTemplate,
	}
	if err := s.store.CreateAlertRule(r.Context(), rule); err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusCreated, map[string]any{"alert_rule": rule})
}

// ─── Patch alert rule ─────────────────────────────────────────────────────────

type patchAlertRuleRequest struct {
	Active          *bool    `json:"active"`
	Threshold       *float64 `json:"threshold"`
	SubjectTemplate *string  `json:"subject_template"`
	BodyTemplate    *string  `json:"body_template"`
}

func (s *Server) patchAlertRule(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}

	var req patchAlertRuleRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}

	// Validate threshold if being changed.
	if req.Threshold != nil {
		if *req.Threshold < 0 {
			http.Error(w, "threshold must be >= 0", http.StatusBadRequest)
			return
		}
		existing, err := s.store.GetAlertRule(r.Context(), id)
		if err != nil || existing == nil {
			http.NotFound(w, r)
			return
		}
		if existing.RuleType == "DELIVERY_SUCCESS" && *req.Threshold > 100 {
			http.Error(w, "threshold for DELIVERY_SUCCESS must be between 0 and 100", http.StatusBadRequest)
			return
		}
	}

	rule, err := s.store.UpdateAlertRule(r.Context(), id, req.Active, req.Threshold, req.SubjectTemplate, req.BodyTemplate)
	if err != nil || rule == nil {
		http.NotFound(w, r)
		return
	}

	writeJSON(w, http.StatusOK, rule)
}

// ─── Delete alert rule ────────────────────────────────────────────────────────

func (s *Server) deleteAlertRule(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}

	if err := s.store.DeleteAlertRule(r.Context(), id); err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// ─── List alert notifications ─────────────────────────────────────────────────

func (s *Server) listAlertNotifications(w http.ResponseWriter, r *http.Request) {
	limit := 100
	if q := r.URL.Query().Get("limit"); q != "" {
		if n, err := strconv.Atoi(q); err == nil && n > 0 && n <= 500 {
			limit = n
		}
	}

	notifications, err := s.store.ListAlertNotifications(r.Context(), limit)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	notifications = jsonSlice(notifications)

	writeJSON(w, http.StatusOK, map[string]any{
		"notifications": notifications,
		"count":         len(notifications),
	})
}
