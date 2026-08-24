package api

import (
	"context"
	"net/http"

	"github.com/google/uuid"

	"github.com/koto7/tuma/internal/storage"
)

type connectionStatsJSON struct {
	Delivered24h int    `json:"delivered_24h"`
	P95LatencyMS int  `json:"p95_latency_ms"`
	FailedCount  int    `json:"failed_count"`
	Status       string `json:"status"`
}

func statsToJSON(st storage.ConnectionStats) connectionStatsJSON {
	return connectionStatsJSON{
		Delivered24h: st.Delivered24h,
		P95LatencyMS: st.P95LatencyMS,
		FailedCount:  st.OpenIssues,
		Status:       st.Status,
	}
}

func (s *Server) getMetrics(w http.ResponseWriter, r *http.Request) {
	m, err := s.store.GetPlatformMetrics(r.Context())
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, m)
}

func (s *Server) connectionStatsMap(ctx context.Context) (map[uuid.UUID]storage.ConnectionStats, error) {
	list, err := s.store.ListAllConnectionStats(ctx)
	if err != nil {
		return nil, err
	}
	m := make(map[uuid.UUID]storage.ConnectionStats, len(list))
	for _, st := range list {
		m[st.ConnectionID] = st
	}
	return m, nil
}
