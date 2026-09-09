package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/koto7/tuma/internal/playground"
	"github.com/koto7/tuma/internal/storage"
)

const playgroundCookie = "tuma_playground"

func (s *Server) initPlayground() {
	if !s.cfg.DemoMode {
		return
	}
	s.playgroundHub = playground.NewHub()
	s.playgroundRL = newRateLimiter()
	s.playgroundStreams = newStreamLimiter()
	go s.playgroundGC()
}

func (s *Server) registerPlaygroundRoutes(mux *http.ServeMux) {
	if !s.cfg.DemoMode {
		return
	}
	mux.HandleFunc("GET /api/playground/bootstrap", s.playgroundBootstrap)
	mux.HandleFunc("GET /api/playground/status", s.playgroundStatus)
	mux.HandleFunc("GET /api/playground/stream", s.playgroundStream)
	mux.HandleFunc("POST /api/playground/simulate", s.playgroundSimulate)
	mux.HandleFunc("POST /api/playground/break", s.playgroundBreak)
	mux.HandleFunc("POST /api/playground/fix", s.playgroundFix)
	mux.HandleFunc("POST /api/playground/replay", s.playgroundReplay)
	mux.HandleFunc("POST /playground/r/{session_id}/hook", s.playgroundReceiver)
}

func (s *Server) getConfig(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"demo_mode": s.cfg.DemoMode,
		"sink_url":  s.sinkURL(),
	})
}

func (s *Server) playgroundCookie(sessionID uuid.UUID) *http.Cookie {
	maxAge := int(s.cfg.PlaygroundSessionTTL.Seconds())
	c := &http.Cookie{
		Name:     playgroundCookie,
		Value:    sessionID.String(),
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   maxAge,
	}
	if strings.HasPrefix(s.cfg.PublicBaseURL, "https://") {
		c.Secure = true
	}
	return c
}

func (s *Server) playgroundSessionFromRequest(r *http.Request) (*storage.PlaygroundSession, error) {
	c, err := r.Cookie(playgroundCookie)
	if err != nil {
		return nil, err
	}
	id, err := uuid.Parse(c.Value)
	if err != nil {
		return nil, err
	}
	ps, err := s.store.GetPlaygroundSession(r.Context(), id)
	if err != nil || ps == nil {
		return nil, fmt.Errorf("session not found")
	}
	return ps, nil
}

func (s *Server) playgroundBootstrap(w http.ResponseWriter, r *http.Request) {
	ip := clientIP(r)
	if !s.playgroundRL.allow("bootstrap:"+ip, 10, 3) {
		writeRateLimited(w)
		return
	}

	if ps, err := s.playgroundSessionFromRequest(r); err == nil && ps != nil {
		_ = s.store.TouchPlaygroundSession(r.Context(), ps.ID)
		conn, _ := s.store.GetConnection(r.Context(), ps.ConnectionID)
		if conn != nil {
			http.SetCookie(w, s.playgroundCookie(ps.ID))
			writeJSON(w, http.StatusOK, s.playgroundSummary(r.Context(), ps, conn))
			return
		}
	}

	if n, err := s.store.CountPlaygroundSessions(r.Context()); err == nil && n >= s.cfg.PlaygroundMaxSessions {
		_, _ = s.store.EvictOldestPlaygroundSessions(r.Context(), 1)
	}

	sessionID := uuid.New()
	secret := randomSecret()
	enc, err := s.encryptor.Encrypt(secret)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	destURL := fmt.Sprintf("%s/playground/r/%s/hook",
		strings.TrimSuffix(s.cfg.PlaygroundInternalBase, "/"), sessionID)

	ps, conn, err := s.store.CreatePlaygroundSession(r.Context(), sessionID, enc, destURL)
	if err != nil {
		s.logger.Error("playground bootstrap failed", "error", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	http.SetCookie(w, s.playgroundCookie(ps.ID))
	writeJSON(w, http.StatusOK, s.playgroundSummary(r.Context(), ps, conn))
}

func (s *Server) playgroundSummary(ctx context.Context, ps *storage.PlaygroundSession, conn *storage.Connection) map[string]any {
	return map[string]any{
		"session_id":    ps.ID.String(),
		"connection_id": conn.ID.String(),
		"inbound_url":   storage.InboundURL(s.cfg.PublicBaseURL, conn.InboundPath),
		"inbound_path":  conn.InboundPath,
		"source_type":   conn.SourceType,
		"destination_fail": ps.FailDestination,
	}
}

func (s *Server) playgroundStatus(w http.ResponseWriter, r *http.Request) {
	ps, err := s.playgroundSessionFromRequest(r)
	if err != nil {
		http.Error(w, "session required", http.StatusUnauthorized)
		return
	}
	_ = s.store.TouchPlaygroundSession(r.Context(), ps.ID)

	conn, err := s.store.GetConnection(r.Context(), ps.ConnectionID)
	if err != nil || conn == nil {
		http.NotFound(w, r)
		return
	}

	delivered, _ := s.store.DeliveredCount24hForConnection(r.Context(), conn.ID)
	openIssues, _ := s.store.OpenIssuesCountForConnection(r.Context(), conn.ID)
	failed24h := 0
	if st, err := s.store.GetConnectionStats(r.Context(), conn.ID); err == nil && st != nil {
		failed24h = st.Failed24h
	}

	status := "delivering"
	if openIssues > 0 {
		status = "failing"
	} else if failed24h > 0 {
		status = "degraded"
	}

	deliveries, _ := s.store.ListDeliveriesForConnection(r.Context(), conn.ID, 3)

	writeJSON(w, http.StatusOK, map[string]any{
		"status":         status,
		"delivered_24h":  delivered,
		"open_issues":    openIssues,
		"inbound_url":    storage.InboundURL(s.cfg.PublicBaseURL, conn.InboundPath),
		"fail_destination": ps.FailDestination,
		"recent_deliveries": deliveries,
	})
}

func (s *Server) playgroundStream(w http.ResponseWriter, r *http.Request) {
	ip := clientIP(r)
	if !s.playgroundStreams.acquire(ip, 5) {
		writeRateLimited(w)
		return
	}
	defer s.playgroundStreams.release(ip)

	ps, err := s.playgroundSessionFromRequest(r)
	if err != nil {
		http.Error(w, "session required", http.StatusUnauthorized)
		return
	}
	_ = s.store.TouchPlaygroundSession(r.Context(), ps.ID)

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	simCh, unsubSim := s.playgroundHub.SubscribeSim(ps.ID)
	destCh, unsubDest := s.playgroundHub.SubscribeDest(ps.ID)
	defer unsubSim()
	defer unsubDest()

	ctx := r.Context()
	for {
		select {
		case <-ctx.Done():
			return
		case ev, ok := <-simCh:
			if !ok {
				return
			}
			if data, err := playground.FormatSSE("sim", ev); err == nil {
				_, _ = w.Write(data)
				flusher.Flush()
			}
		case ev, ok := <-destCh:
			if !ok {
				return
			}
			if data, err := playground.FormatSSE("dest", ev); err == nil {
				_, _ = w.Write(data)
				flusher.Flush()
			}
		}
	}
}

type simulateBody struct {
	Provider           string `json:"provider"`
	Count              int    `json:"count"`
	DuplicateEventID   string `json:"duplicate_event_id"`
}

func (s *Server) playgroundSimulate(w http.ResponseWriter, r *http.Request) {
	ip := clientIP(r)
	if !s.playgroundRL.allow("simulate:"+ip, 20, 5) {
		writeRateLimited(w)
		return
	}

	ps, err := s.playgroundSessionFromRequest(r)
	if err != nil {
		http.Error(w, "session required", http.StatusUnauthorized)
		return
	}
	_ = s.store.TouchPlaygroundSession(r.Context(), ps.ID)

	var req simulateBody
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	if req.Provider == "" {
		req.Provider = "stripe"
	}
	if req.Count < 1 {
		req.Count = 1
	}
	if req.Count > 5 {
		req.Count = 5
	}

	conn, err := s.store.GetConnection(r.Context(), ps.ConnectionID)
	if err != nil || conn == nil {
		http.NotFound(w, r)
		return
	}

	if conn.SourceType != req.Provider {
		if err := s.store.UpdateConnectionSourceType(r.Context(), conn.ID, req.Provider); err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		conn.SourceType = req.Provider
		s.connCache.Delete(conn.InboundPath)
	}

	secret, err := s.encryptor.Decrypt(conn.SigningSecretEnc)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	results, err := playground.SendEvents(s.cfg.PlaygroundInternalBase, conn.InboundPath, secret, req.Provider, playground.SimulateRequest{
		Provider:         req.Provider,
		Count:            req.Count,
		DuplicateEventID: req.DuplicateEventID,
	})
	if err != nil {
		s.logger.Error("playground simulate failed", "error", err)
		http.Error(w, "simulate failed", http.StatusInternalServerError)
		return
	}

	for _, res := range results {
		line := fmt.Sprintf("→ POST /e/%s %d %s  event=%s", conn.InboundPath, res.Status, res.Body, res.EventID)
		s.playgroundHub.PublishSim(ps.ID, line)
	}

	writeJSON(w, http.StatusOK, map[string]any{"sent": len(results), "results": results})
}

func (s *Server) playgroundBreak(w http.ResponseWriter, r *http.Request) {
	s.playgroundSetFail(w, r, true)
}

func (s *Server) playgroundFix(w http.ResponseWriter, r *http.Request) {
	s.playgroundSetFail(w, r, false)
}

func (s *Server) playgroundSetFail(w http.ResponseWriter, r *http.Request, fail bool) {
	ip := clientIP(r)
	if !s.playgroundRL.allow("control:"+ip, 30, 10) {
		writeRateLimited(w)
		return
	}
	ps, err := s.playgroundSessionFromRequest(r)
	if err != nil {
		http.Error(w, "session required", http.StatusUnauthorized)
		return
	}
	if err := s.store.SetPlaygroundFailDestination(r.Context(), ps.ID, fail); err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	action := "fixed"
	if fail {
		action = "broken"
	}
	s.playgroundHub.PublishSim(ps.ID, fmt.Sprintf("destination %s", action))
	writeJSON(w, http.StatusOK, map[string]string{"status": action})
}

func (s *Server) playgroundReplay(w http.ResponseWriter, r *http.Request) {
	ip := clientIP(r)
	if !s.playgroundRL.allow("control:"+ip, 30, 10) {
		writeRateLimited(w)
		return
	}
	ps, err := s.playgroundSessionFromRequest(r)
	if err != nil {
		http.Error(w, "session required", http.StatusUnauthorized)
		return
	}
	if ps.FailDestination {
		writeJSON(w, http.StatusConflict, map[string]string{
			"error":   "fix_first",
			"message": "Fix the destination before replaying.",
		})
		return
	}
	issue, err := s.store.GetLatestOpenIssueForConnection(r.Context(), ps.ConnectionID)
	if err != nil || issue == nil {
		writeJSON(w, http.StatusConflict, map[string]string{
			"error":   "no_open_issue",
			"message": "No failed event yet. Break, send an event, wait for retries (~15s), then Fix and Replay.",
		})
		return
	}
	if err := s.startReplay(r.Context(), issue.EventID); err != nil {
		http.Error(w, "replay failed", http.StatusInternalServerError)
		return
	}
	s.playgroundHub.PublishSim(ps.ID, fmt.Sprintf("→ replay issue %s", issue.ID))
	writeJSON(w, http.StatusOK, map[string]string{"status": "replay_started", "issue_id": issue.ID.String()})
}

func (s *Server) playgroundReceiver(w http.ResponseWriter, r *http.Request) {
	sessionID, err := uuid.Parse(r.PathValue("session_id"))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	ps, err := s.store.GetPlaygroundSession(r.Context(), sessionID)
	if err != nil || ps == nil {
		http.NotFound(w, r)
		return
	}

	body, _ := io.ReadAll(io.LimitReader(r.Body, s.cfg.MaxBodyBytes))
	deliveryID := r.Header.Get("X-Tuma-Delivery-Id")
	start := time.Now()

	if ps.FailDestination {
		line := fmt.Sprintf("POST /hook 502  fail_mode  %dms", time.Since(start).Milliseconds())
		s.playgroundHub.PublishDest(sessionID, line, deliveryID, playground.BodyPreview(body))
		http.Error(w, "destination broken (demo)", http.StatusBadGateway)
		return
	}

	latency := time.Since(start).Milliseconds()
	line := fmt.Sprintf("POST /hook 200  %dms", latency)
	s.playgroundHub.PublishDest(sessionID, line, deliveryID, playground.BodyPreview(body))
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"ok":true}`))
}

func (s *Server) playgroundGC() {
	ticker := time.NewTicker(15 * time.Minute)
	defer ticker.Stop()
	for range ticker.C {
		ctx := context.Background()
		cutoff := time.Now().Add(-s.cfg.PlaygroundSessionTTL)
		n, err := s.store.DeletePlaygroundSessionsOlderThan(ctx, cutoff)
		if err != nil {
			s.logger.Warn("playground gc failed", "error", err)
			continue
		}
		if n > 0 {
			s.logger.Info("playground gc evicted idle sessions", "count", n)
		}
		count, err := s.store.CountPlaygroundSessions(ctx)
		if err != nil {
			continue
		}
		if count > s.cfg.PlaygroundMaxSessions {
			evict := count - s.cfg.PlaygroundMaxSessions
			removed, _ := s.store.EvictOldestPlaygroundSessions(ctx, evict)
			if removed > 0 {
				s.logger.Info("playground gc evicted over cap", "count", removed)
			}
		}
	}
}
