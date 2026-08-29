package storage

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type PlaygroundSession struct {
	ID              uuid.UUID
	ConnectionID    uuid.UUID
	FailDestination bool
	CreatedAt       time.Time
	LastSeenAt      time.Time
}

func (s *Store) GetPlaygroundSession(ctx context.Context, id uuid.UUID) (*PlaygroundSession, error) {
	var ps PlaygroundSession
	err := s.pool.QueryRow(ctx, `
		SELECT id, connection_id, fail_destination, created_at, last_seen_at
		FROM playground_sessions WHERE id = $1
	`, id).Scan(&ps.ID, &ps.ConnectionID, &ps.FailDestination, &ps.CreatedAt, &ps.LastSeenAt)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &ps, nil
}

func (s *Store) TouchPlaygroundSession(ctx context.Context, id uuid.UUID) error {
	_, err := s.pool.Exec(ctx, `UPDATE playground_sessions SET last_seen_at = NOW() WHERE id = $1`, id)
	return err
}

func (s *Store) SetPlaygroundFailDestination(ctx context.Context, id uuid.UUID, fail bool) error {
	_, err := s.pool.Exec(ctx, `UPDATE playground_sessions SET fail_destination = $2, last_seen_at = NOW() WHERE id = $1`, id, fail)
	return err
}

func (s *Store) CreatePlaygroundSession(ctx context.Context, sessionID uuid.UUID, secretEnc []byte, destURL string) (*PlaygroundSession, *Connection, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, nil, err
	}
	defer tx.Rollback(ctx)

	conn := &Connection{
		Name:               "Playground",
		SourceType:         "stripe",
		InboundPath:        GenerateInboundPath(),
		DestinationURL:     destURL,
		SigningSecretEnc:   secretEnc,
		RetryAttempts:      3,
		RetryFirstDelayS:   5,
		RetryBackoffFactor: 2.0,
		RetentionDays:      1,
		IsPlayground:       true,
	}
	err = tx.QueryRow(ctx, `
		INSERT INTO connections (name, source_type, inbound_path, destination_url, signing_secret_encrypted,
			retry_attempts, retry_first_delay_s, retry_backoff_factor, retention_days, is_playground)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)
		RETURNING id, created_at
	`, conn.Name, conn.SourceType, conn.InboundPath, conn.DestinationURL, conn.SigningSecretEnc,
		conn.RetryAttempts, conn.RetryFirstDelayS, conn.RetryBackoffFactor, conn.RetentionDays, conn.IsPlayground,
	).Scan(&conn.ID, &conn.CreatedAt)
	if err != nil {
		return nil, nil, err
	}

	var ps PlaygroundSession
	err = tx.QueryRow(ctx, `
		INSERT INTO playground_sessions (id, connection_id) VALUES ($1, $2)
		RETURNING id, connection_id, fail_destination, created_at, last_seen_at
	`, sessionID, conn.ID).Scan(&ps.ID, &ps.ConnectionID, &ps.FailDestination, &ps.CreatedAt, &ps.LastSeenAt)
	if err != nil {
		return nil, nil, err
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, nil, err
	}
	return &ps, conn, nil
}

func (s *Store) CountPlaygroundSessions(ctx context.Context) (int, error) {
	var n int
	err := s.pool.QueryRow(ctx, `SELECT COUNT(*) FROM playground_sessions`).Scan(&n)
	return n, err
}

func (s *Store) DeletePlaygroundSessionsOlderThan(ctx context.Context, cutoff time.Time) (int64, error) {
	tag, err := s.pool.Exec(ctx, `DELETE FROM playground_sessions WHERE last_seen_at < $1`, cutoff)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}

func (s *Store) EvictOldestPlaygroundSessions(ctx context.Context, n int) (int64, error) {
	tag, err := s.pool.Exec(ctx, `
		DELETE FROM playground_sessions
		WHERE id IN (
			SELECT id FROM playground_sessions ORDER BY last_seen_at ASC LIMIT $1
		)
	`, n)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}

func (s *Store) UpdateConnectionSourceType(ctx context.Context, id uuid.UUID, sourceType string) error {
	_, err := s.pool.Exec(ctx, `UPDATE connections SET source_type = $2 WHERE id = $1`, id, sourceType)
	return err
}

func (s *Store) GetLatestOpenIssueForConnection(ctx context.Context, connID uuid.UUID) (*Issue, error) {
	var i Issue
	err := s.pool.QueryRow(ctx, `
		SELECT i.id, i.event_id, i.connection_id, i.reason, i.attempts_exhausted, i.first_failed_at,
			i.status, i.resolved_at, i.resolved_by, e.provider_event_id, c.destination_url
		FROM issues i
		JOIN events e ON e.id = i.event_id
		JOIN connections c ON c.id = i.connection_id
		WHERE i.connection_id = $1 AND i.status = 'open'
		ORDER BY i.first_failed_at DESC LIMIT 1
	`, connID).Scan(&i.ID, &i.EventID, &i.ConnectionID, &i.Reason, &i.AttemptsExhausted,
		&i.FirstFailedAt, &i.Status, &i.ResolvedAt, &i.ResolvedBy,
		&i.ProviderEventID, &i.Destination)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &i, nil
}

func (s *Store) OpenIssuesCountForConnection(ctx context.Context, connID uuid.UUID) (int, error) {
	var n int
	err := s.pool.QueryRow(ctx, `SELECT COUNT(*) FROM issues WHERE connection_id = $1 AND status = 'open'`, connID).Scan(&n)
	return n, err
}

func (s *Store) DeliveredCount24hForConnection(ctx context.Context, connID uuid.UUID) (int, error) {
	var n int
	err := s.pool.QueryRow(ctx, `
		SELECT COUNT(*)::int FROM deliveries d
		JOIN events e ON e.id = d.event_id
		WHERE e.connection_id = $1 AND d.status = 'delivered' AND d.attempted_at > NOW() - INTERVAL '24 hours'
	`, connID).Scan(&n)
	return n, err
}
