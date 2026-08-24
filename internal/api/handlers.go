package api

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"

	"github.com/koto7/tuma/internal/storage"
)

type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	var req loginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	user, err := s.store.GetUserByEmail(r.Context(), req.Email)
	if err != nil || user == nil {
		http.Error(w, "invalid credentials", http.StatusUnauthorized)
		return
	}
	if bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.Password)) != nil {
		http.Error(w, "invalid credentials", http.StatusUnauthorized)
		return
	}
	sess := &storage.Session{UserID: user.ID, ExpiresAt: time.Now().Add(s.cfg.SessionTTL)}
	if err := s.store.CreateSession(r.Context(), sess); err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	http.SetCookie(w, s.sessionCookie(sess.ID.String(), sess.ExpiresAt))
	writeJSON(w, http.StatusOK, map[string]any{"email": user.Email})
}

func (s *Server) sessionCookie(value string, expires time.Time) *http.Cookie {
	c := &http.Cookie{
		Name:     s.cfg.SessionCookieName,
		Value:    value,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Expires:  expires,
	}
	if strings.HasPrefix(s.cfg.PublicBaseURL, "https://") {
		c.Secure = true
	}
	return c
}

func (s *Server) logout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(s.cfg.SessionCookieName); err == nil {
		if id, err := uuid.Parse(c.Value); err == nil {
			_ = s.store.DeleteSession(r.Context(), id)
		}
	}
	c := s.sessionCookie("", time.Unix(0, 0))
	c.MaxAge = -1
	http.SetCookie(w, c)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) auth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		c, err := r.Cookie(s.cfg.SessionCookieName)
		if err != nil {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		id, err := uuid.Parse(c.Value)
		if err != nil {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		sess, err := s.store.GetSession(r.Context(), id)
		if err != nil || sess == nil {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		next(w, r)
	}
}

type connectionResponse struct {
	ID                 string               `json:"id"`
	Name               string               `json:"name"`
	SourceType         string               `json:"source_type"`
	InboundURL         string               `json:"inbound_url"`
	InboundPath        string               `json:"inbound_path"`
	DestinationURL     string               `json:"destination_url"`
	RetryAttempts      int                  `json:"retry_attempts"`
	RetryFirstDelayS   int                  `json:"retry_first_delay_s"`
	RetryBackoffFactor float64              `json:"retry_backoff_factor"`
	RetentionDays      int                  `json:"retention_days"`
	CreatedAt          string               `json:"created_at"`
	Stats              *connectionStatsJSON `json:"stats,omitempty"`
}

func toConnectionResponse(cfgPublicBase string, c storage.Connection) connectionResponse {
	return connectionResponse{
		ID:                 c.ID.String(),
		Name:               c.Name,
		SourceType:         c.SourceType,
		InboundURL:         storage.InboundURL(cfgPublicBase, c.InboundPath),
		InboundPath:        c.InboundPath,
		DestinationURL:     c.DestinationURL,
		RetryAttempts:      c.RetryAttempts,
		RetryFirstDelayS:   c.RetryFirstDelayS,
		RetryBackoffFactor: c.RetryBackoffFactor,
		RetentionDays:      c.RetentionDays,
		CreatedAt:          c.CreatedAt.Format(time.RFC3339),
	}
}

type createConnectionRequest struct {
	Name           string `json:"name"`
	SourceType     string `json:"source_type"`
	DestinationURL string `json:"destination_url"`
	SigningSecret  string `json:"signing_secret"`
}

func (s *Server) createConnection(w http.ResponseWriter, r *http.Request) {
	var req createConnectionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	if req.Name == "" || req.DestinationURL == "" || req.SourceType == "" {
		http.Error(w, "missing required fields", http.StatusBadRequest)
		return
	}
	secret := req.SigningSecret
	if secret == "" && req.SourceType != "internal" {
		secret = randomSecret()
	}
	enc, err := s.encryptor.Encrypt(secret)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	c := &storage.Connection{
		Name:               req.Name,
		SourceType:         req.SourceType,
		InboundPath:        storage.GenerateInboundPath(),
		DestinationURL:     req.DestinationURL,
		SigningSecretEnc:   enc,
		RetryAttempts:      8,
		RetryFirstDelayS:   60,
		RetryBackoffFactor: 2.0,
		RetentionDays:      7,
	}
	if err := s.store.CreateConnection(r.Context(), c); err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	resp := toConnectionResponse(s.cfg.PublicBaseURL, *c)
	writeJSON(w, http.StatusCreated, map[string]any{
		"connection": resp,
		"signing_secret": secret,
	})
}

func jsonSlice[T any](s []T) []T {
	if s == nil {
		return []T{}
	}
	return s
}

func (s *Server) listConnections(w http.ResponseWriter, r *http.Request) {
	conns, err := s.store.ListConnections(r.Context())
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	statMap, err := s.connectionStatsMap(r.Context())
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	out := make([]connectionResponse, 0, len(conns))
	for _, c := range conns {
		resp := toConnectionResponse(s.cfg.PublicBaseURL, c)
		if st, ok := statMap[c.ID]; ok {
			j := statsToJSON(st)
			resp.Stats = &j
		}
		out = append(out, resp)
	}
	writeJSON(w, http.StatusOK, map[string]any{"connections": jsonSlice(out)})
}

func (s *Server) getConnection(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}
	c, err := s.store.GetConnection(r.Context(), id)
	if err != nil || c == nil {
		http.NotFound(w, r)
		return
	}
	resp := toConnectionResponse(s.cfg.PublicBaseURL, *c)
	if st, err := s.store.GetConnectionStats(r.Context(), id); err == nil && st != nil {
		j := statsToJSON(*st)
		resp.Stats = &j
	}
	writeJSON(w, http.StatusOK, resp)
}

type patchConnectionRequest struct {
	DestinationURL     *string  `json:"destination_url"`
	RetryAttempts      *int     `json:"retry_attempts"`
	RetryFirstDelayS   *int     `json:"retry_first_delay_s"`
	RetryBackoffFactor *float64 `json:"retry_backoff_factor"`
	RetentionDays      *int     `json:"retention_days"`
}

func (s *Server) patchConnection(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}
	var req patchConnectionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	c, err := s.store.UpdateConnection(r.Context(), id, req.DestinationURL, req.RetryAttempts,
		req.RetryFirstDelayS, req.RetryBackoffFactor, req.RetentionDays)
	if err != nil || c == nil {
		http.NotFound(w, r)
		return
	}
	writeJSON(w, http.StatusOK, toConnectionResponse(s.cfg.PublicBaseURL, *c))
}

func (s *Server) listDeliveries(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}
	deliveries, err := s.store.ListDeliveriesForConnection(r.Context(), id, 50)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"deliveries": jsonSlice(deliveries)})
}

func (s *Server) listIssues(w http.ResponseWriter, r *http.Request) {
	status := r.URL.Query().Get("status")
	if status == "" {
		status = "open"
	}
	issues, err := s.store.ListIssues(r.Context(), status)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"issues": jsonSlice(issues)})
}

func (s *Server) getIssue(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}
	issue, err := s.store.GetIssue(r.Context(), id)
	if err != nil || issue == nil {
		http.NotFound(w, r)
		return
	}
	event, _ := s.store.GetEvent(r.Context(), issue.EventID)
	payload := ""
	if event != nil {
		payload = string(event.RawPayload)
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"issue":   issue,
		"payload": payload,
	})
}

func (s *Server) replayIssue(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}
	issue, err := s.store.GetIssue(r.Context(), id)
	if err != nil || issue == nil {
		http.NotFound(w, r)
		return
	}
	if err := s.startReplay(r.Context(), issue.EventID); err != nil {
		http.Error(w, "failed to start replay", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "replay_started"})
}

type bulkReplayRequest struct {
	IDs []string `json:"ids"`
}

func (s *Server) replayBulk(w http.ResponseWriter, r *http.Request) {
	var req bulkReplayRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	started := 0
	for _, idStr := range req.IDs {
		id, err := uuid.Parse(idStr)
		if err != nil {
			continue
		}
		issue, err := s.store.GetIssue(r.Context(), id)
		if err != nil || issue == nil {
			continue
		}
		if err := s.startReplay(r.Context(), issue.EventID); err == nil {
			started++
		}
	}
	writeJSON(w, http.StatusOK, map[string]int{"started": started})
}
