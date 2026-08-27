package storage

import (
	"context"
	"math"
	"time"

	"github.com/google/uuid"
)

type ConnectionStats struct {
	ConnectionID   uuid.UUID
	Delivered24h   int
	Failed24h      int
	OpenIssues     int
	P95LatencyMS   int
	Status         string // healthy | degraded | failing
}

type HourlyCount struct {
	Hour  time.Time `json:"hour"`
	Count int       `json:"count"`
}

type PlatformMetrics struct {
	EventsReceived24h      int           `json:"events_received_24h"`
	DeliveriesDelivered24h int           `json:"deliveries_delivered_24h"`
	DeliveriesFailed24h    int           `json:"deliveries_failed_24h"`
	P95LatencyMS           int           `json:"p95_latency_ms"`
	OpenIssues             int           `json:"open_issues"`
	IngestionHourly        []HourlyCount `json:"ingestion_hourly"`
	DeliveryHourly         []HourlyCount `json:"delivery_hourly"`
	Connections            []ConnectionMetricsRow `json:"connections"`
}

type ConnectionMetricsRow struct {
	ConnectionID uuid.UUID `json:"connection_id"`
	Name         string    `json:"name"`
	Delivered24h int       `json:"delivered_24h"`
	Failed24h    int       `json:"failed_24h"`
	OpenIssues   int       `json:"open_issues"`
	P95LatencyMS int       `json:"p95_latency_ms"`
	Status       string    `json:"status"`
}

func DeriveConnectionStatus(openIssues, failed24h, delivered24h int) string {
	if openIssues > 0 {
		return "failing"
	}
	if failed24h > 0 {
		return "degraded"
	}
	if delivered24h > 0 {
		return "healthy"
	}
	return "healthy"
}

func (s *Store) GetConnectionStats(ctx context.Context, connID uuid.UUID) (*ConnectionStats, error) {
	all, err := s.ListAllConnectionStats(ctx)
	if err != nil {
		return nil, err
	}
	for _, st := range all {
		if st.ConnectionID == connID {
			cp := st
			return &cp, nil
		}
	}
	return &ConnectionStats{ConnectionID: connID, Status: "healthy"}, nil
}

func (s *Store) ListAllConnectionStats(ctx context.Context) ([]ConnectionStats, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT c.id,
			COALESCE(del.delivered_24h, 0)::int,
			COALESCE(del.failed_24h, 0)::int,
			COALESCE(iss.open_count, 0)::int,
			COALESCE(del.p95_ms, 0)::float8
		FROM connections c
		LEFT JOIN (
			SELECT e.connection_id,
				COUNT(*) FILTER (WHERE d.status = 'delivered') AS delivered_24h,
				COUNT(*) FILTER (WHERE d.status = 'failed') AS failed_24h,
				percentile_cont(0.95) WITHIN GROUP (ORDER BY d.latency_ms)
					FILTER (WHERE d.status = 'delivered' AND d.latency_ms IS NOT NULL) AS p95_ms
			FROM deliveries d
			JOIN events e ON e.id = d.event_id
			WHERE d.attempted_at > NOW() - INTERVAL '24 hours'
			GROUP BY e.connection_id
		) del ON del.connection_id = c.id
		LEFT JOIN (
			SELECT connection_id, COUNT(*) AS open_count
			FROM issues WHERE status = 'open'
			GROUP BY connection_id
		) iss ON iss.connection_id = c.id
		ORDER BY c.created_at DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []ConnectionStats
	for rows.Next() {
		var st ConnectionStats
		var p95 float64
		if err := rows.Scan(&st.ConnectionID, &st.Delivered24h, &st.Failed24h, &st.OpenIssues, &p95); err != nil {
			return nil, err
		}
		st.P95LatencyMS = int(math.Round(p95))
		st.Status = DeriveConnectionStatus(st.OpenIssues, st.Failed24h, st.Delivered24h)
		out = append(out, st)
	}
	return out, rows.Err()
}

func (s *Store) GetPlatformMetrics(ctx context.Context) (*PlatformMetrics, error) {
	m := &PlatformMetrics{
		IngestionHourly: []HourlyCount{},
		DeliveryHourly:  []HourlyCount{},
		Connections:     []ConnectionMetricsRow{},
	}
	var p95 float64

	err := s.pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM events WHERE received_at > NOW() - INTERVAL '24 hours'
	`).Scan(&m.EventsReceived24h)
	if err != nil {
		return nil, err
	}

	err = s.pool.QueryRow(ctx, `
		SELECT
			COUNT(*) FILTER (WHERE d.status = 'delivered'),
			COUNT(*) FILTER (WHERE d.status = 'failed'),
			COALESCE(percentile_cont(0.95) WITHIN GROUP (ORDER BY d.latency_ms)
				FILTER (WHERE d.status = 'delivered' AND d.latency_ms IS NOT NULL), 0)::float8
		FROM deliveries d
		WHERE d.attempted_at > NOW() - INTERVAL '24 hours'
	`).Scan(&m.DeliveriesDelivered24h, &m.DeliveriesFailed24h, &p95)
	if err != nil {
		return nil, err
	}
	m.P95LatencyMS = int(math.Round(p95))

	m.OpenIssues, err = s.OpenIssuesCount(ctx)
	if err != nil {
		return nil, err
	}

	m.IngestionHourly, err = s.hourlyEventCounts(ctx)
	if err != nil {
		return nil, err
	}
	m.DeliveryHourly, err = s.hourlyDeliveryCounts(ctx)
	if err != nil {
		return nil, err
	}

	conns, err := s.ListConnections(ctx)
	if err != nil {
		return nil, err
	}
	stats, err := s.ListAllConnectionStats(ctx)
	if err != nil {
		return nil, err
	}
	statMap := make(map[uuid.UUID]ConnectionStats, len(stats))
	for _, st := range stats {
		statMap[st.ConnectionID] = st
	}
	for _, c := range conns {
		st := statMap[c.ID]
		m.Connections = append(m.Connections, ConnectionMetricsRow{
			ConnectionID: c.ID,
			Name:         c.Name,
			Delivered24h: st.Delivered24h,
			Failed24h:    st.Failed24h,
			OpenIssues:   st.OpenIssues,
			P95LatencyMS: st.P95LatencyMS,
			Status:       DeriveConnectionStatus(st.OpenIssues, st.Failed24h, st.Delivered24h),
		})
	}
	return m, nil
}

func (s *Store) hourlyEventCounts(ctx context.Context) ([]HourlyCount, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT date_trunc('hour', received_at) AS hour, COUNT(*)::int
		FROM events
		WHERE received_at > NOW() - INTERVAL '24 hours'
		GROUP BY 1 ORDER BY 1
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanHourly(rows)
}

func (s *Store) hourlyDeliveryCounts(ctx context.Context) ([]HourlyCount, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT date_trunc('hour', attempted_at) AS hour, COUNT(*)::int
		FROM deliveries
		WHERE status = 'delivered' AND attempted_at > NOW() - INTERVAL '24 hours'
		GROUP BY 1 ORDER BY 1
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanHourly(rows)
}

func scanHourly(rows interface {
	Next() bool
	Scan(dest ...any) error
}) ([]HourlyCount, error) {
	var out []HourlyCount
	for rows.Next() {
		var h HourlyCount
		if err := rows.Scan(&h.Hour, &h.Count); err != nil {
			return nil, err
		}
		out = append(out, h)
	}
	if out == nil {
		out = []HourlyCount{}
	}
	return out, nil
}
