package api

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/koto7/tuma/internal/notifysettings"
	"github.com/koto7/tuma/internal/storage"
)

func (s *Server) notificationSettings(r *http.Request) (notifysettings.Effective, error) {
	return notifysettings.Load(r.Context(), s.store, s.encryptor, s.cfg)
}

func (s *Server) getNotificationSettings(w http.ResponseWriter, r *http.Request) {
	eff, err := s.notificationSettings(r)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, eff.View)
}

type notificationSettingsUpdate struct {
	SMTPHost          string `json:"smtp_host"`
	SMTPPort          int    `json:"smtp_port"`
	SMTPUsername      string `json:"smtp_username"`
	SMTPFrom          string `json:"smtp_from"`
	SMTPPassword      string `json:"smtp_password"`
	ClearSMTPPassword bool   `json:"clear_smtp_password"`
	SlackBotToken     string `json:"slack_bot_token"`
	ClearSlackToken   bool   `json:"clear_slack_token"`
}

func (s *Server) putNotificationSettings(w http.ResponseWriter, r *http.Request) {
	var req notificationSettingsUpdate
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	req.SMTPHost = strings.TrimSpace(req.SMTPHost)
	req.SMTPUsername = strings.TrimSpace(req.SMTPUsername)
	req.SMTPFrom = strings.TrimSpace(req.SMTPFrom)
	req.SlackBotToken = strings.TrimSpace(req.SlackBotToken)
	if req.SMTPPort < 0 || req.SMTPPort > 65535 {
		http.Error(w, "smtp_port must be between 0 and 65535", http.StatusBadRequest)
		return
	}
	if strings.Contains(req.SlackBotToken, "hooks.slack.com") || strings.Contains(req.SlackBotToken, "\n") {
		http.Error(w, "slack_bot_token must be a bot token, not a webhook URL", http.StatusBadRequest)
		return
	}

	current, err := s.store.GetNotificationSettings(r.Context())
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	next := storage.NotificationSettings{
		SMTPHost:         req.SMTPHost,
		SMTPPort:         req.SMTPPort,
		SMTPUsername:     req.SMTPUsername,
		SMTPFrom:         req.SMTPFrom,
		SMTPPasswordEnc:  current.SMTPPasswordEnc,
		SlackBotTokenEnc: current.SlackBotTokenEnc,
	}
	if req.ClearSMTPPassword {
		next.SMTPPasswordEnc = nil
	}
	if req.SMTPPassword != "" {
		enc, err := s.encryptor.Encrypt(req.SMTPPassword)
		if err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		next.SMTPPasswordEnc = enc
	}
	if req.ClearSlackToken {
		next.SlackBotTokenEnc = nil
	}
	if req.SlackBotToken != "" {
		enc, err := s.encryptor.Encrypt(req.SlackBotToken)
		if err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		next.SlackBotTokenEnc = enc
	}
	if err := s.store.SaveNotificationSettings(r.Context(), next); err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	eff, err := s.notificationSettings(r)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, eff.View)
}
