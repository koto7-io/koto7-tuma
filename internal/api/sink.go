package api

import (
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
)

const sinkMaxEvents = 50
const sinkBodyLimit = 4096

type sinkEvent struct {
	TS         string `json:"ts"`
	Status     int    `json:"status"`
	DeliveryID string `json:"delivery_id,omitempty"`
	Attempt    string `json:"attempt,omitempty"`
	Body       string `json:"body"`
}

type echoSink struct {
	mu     sync.Mutex
	fail   bool
	events []sinkEvent
}

func newEchoSink() *echoSink {
	return &echoSink{events: make([]sinkEvent, 0, 16)}
}

func (s *echoSink) snapshot() (bool, []sinkEvent) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]sinkEvent, len(s.events))
	copy(out, s.events)
	return s.fail, out
}

func (s *echoSink) setFail(fail bool) {
	s.mu.Lock()
	s.fail = fail
	s.mu.Unlock()
}

func (s *echoSink) record(ev sinkEvent) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	status := http.StatusOK
	if s.fail {
		status = http.StatusBadGateway
	}
	ev.Status = status
	s.events = append(s.events, ev)
	if len(s.events) > sinkMaxEvents {
		s.events = s.events[len(s.events)-sinkMaxEvents:]
	}
	return status
}

func (s *Server) sinkURL() string {
	return strings.TrimSuffix(s.cfg.PlaygroundInternalBase, "/") + "/sink/echo"
}

func (s *Server) sinkReceive(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(io.LimitReader(r.Body, s.cfg.MaxBodyBytes))
	status := s.sink.record(sinkEvent{
		TS:         time.Now().UTC().Format(time.RFC3339),
		DeliveryID: r.Header.Get("X-Tuma-Delivery-Id"),
		Attempt:    r.Header.Get("X-Tuma-Attempt"),
		Body:       clipSinkBody(body),
	})
	if status != http.StatusOK {
		http.Error(w, "test receiver failing", status)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"ok":true}`))
}

func (s *Server) getSink(w http.ResponseWriter, _ *http.Request) {
	fail, events := s.sink.snapshot()
	writeJSON(w, http.StatusOK, map[string]any{
		"url":    s.sinkURL(),
		"fail":   fail,
		"events": jsonSlice(events),
	})
}

func (s *Server) sinkBreak(w http.ResponseWriter, _ *http.Request) {
	s.sink.setFail(true)
	writeJSON(w, http.StatusOK, map[string]any{"fail": true})
}

func (s *Server) sinkFix(w http.ResponseWriter, _ *http.Request) {
	s.sink.setFail(false)
	writeJSON(w, http.StatusOK, map[string]any{"fail": false})
}

func clipSinkBody(body []byte) string {
	s := string(body)
	if !utf8.ValidString(s) {
		s = strings.ToValidUTF8(s, "�")
	}
	if len(s) > sinkBodyLimit {
		return s[:sinkBodyLimit] + "…"
	}
	return s
}
