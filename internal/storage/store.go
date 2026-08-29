package storage

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Connection struct {
	ID                   uuid.UUID
	Name                 string
	SourceType           string
	InboundPath          string
	DestinationURL       string
	SigningSecretEnc     []byte
	RetryAttempts        int
	RetryFirstDelayS     int
	RetryBackoffFactor   float64
	RetentionDays        int
	IsPlayground         bool
	CreatedAt            time.Time
}

type Event struct {
	ID              uuid.UUID
	ConnectionID    uuid.UUID
	ProviderEventID string
	RawHeaders      json.RawMessage
	RawPayload      []byte
	ReceivedAt      time.Time
	WorkflowID      *string
}

type Delivery struct {
	ID            uuid.UUID `json:"id"`
	EventID       uuid.UUID `json:"event_id"`
	AttemptNumber int       `json:"attempt_number"`
	Status        string    `json:"status"`
	ResponseCode  *int      `json:"response_code,omitempty"`
	LatencyMS     *int      `json:"latency_ms,omitempty"`
	AttemptedAt   time.Time `json:"attempted_at"`
}

type Issue struct {
	ID                uuid.UUID  `json:"id"`
	EventID           uuid.UUID  `json:"event_id"`
	ConnectionID      uuid.UUID  `json:"connection_id"`
	Reason            string     `json:"reason"`
	AttemptsExhausted int        `json:"attempts_exhausted"`
	FirstFailedAt     time.Time  `json:"first_failed_at"`
	Status            string     `json:"status"`
	ResolvedAt        *time.Time `json:"resolved_at,omitempty"`
	ResolvedBy        *uuid.UUID `json:"resolved_by,omitempty"`
	// joined fields for API
	ProviderEventID string `json:"provider_event_id,omitempty"`
	Destination     string `json:"destination,omitempty"`
}

type User struct {
	ID           uuid.UUID
	Email        string
	PasswordHash string
	CreatedAt    time.Time
}

type Session struct {
	ID        uuid.UUID
	UserID    uuid.UUID
	CreatedAt time.Time
	ExpiresAt time.Time
}

type Store struct {
	pool *pgxpool.Pool
}

func New(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool}
}

func (s *Store) Ping(ctx context.Context) error {
	return s.pool.Ping(ctx)
}

func (s *Store) CreateConnection(ctx context.Context, c *Connection) error {
	return s.pool.QueryRow(ctx, `
		INSERT INTO connections (name, source_type, inbound_path, destination_url, signing_secret_encrypted,
			retry_attempts, retry_first_delay_s, retry_backoff_factor, retention_days, is_playground)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)
		RETURNING id, created_at
	`, c.Name, c.SourceType, c.InboundPath, c.DestinationURL, c.SigningSecretEnc,
		c.RetryAttempts, c.RetryFirstDelayS, c.RetryBackoffFactor, c.RetentionDays, c.IsPlayground,
	).Scan(&c.ID, &c.CreatedAt)
}

func (s *Store) ListConnections(ctx context.Context) ([]Connection, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, name, source_type, inbound_path, destination_url, signing_secret_encrypted,
			retry_attempts, retry_first_delay_s, retry_backoff_factor, retention_days, is_playground, created_at
		FROM connections WHERE is_playground = FALSE ORDER BY created_at DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Connection
	for rows.Next() {
		var c Connection
		if err := rows.Scan(&c.ID, &c.Name, &c.SourceType, &c.InboundPath, &c.DestinationURL,
			&c.SigningSecretEnc, &c.RetryAttempts, &c.RetryFirstDelayS, &c.RetryBackoffFactor,
			&c.RetentionDays, &c.IsPlayground, &c.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (s *Store) GetConnection(ctx context.Context, id uuid.UUID) (*Connection, error) {
	var c Connection
	err := s.pool.QueryRow(ctx, `
		SELECT id, name, source_type, inbound_path, destination_url, signing_secret_encrypted,
			retry_attempts, retry_first_delay_s, retry_backoff_factor, retention_days, is_playground, created_at
		FROM connections WHERE id = $1
	`, id).Scan(&c.ID, &c.Name, &c.SourceType, &c.InboundPath, &c.DestinationURL,
		&c.SigningSecretEnc, &c.RetryAttempts, &c.RetryFirstDelayS, &c.RetryBackoffFactor,
		&c.RetentionDays, &c.IsPlayground, &c.CreatedAt)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &c, nil
}

func (s *Store) GetConnectionByInboundPath(ctx context.Context, path string) (*Connection, error) {
	var c Connection
	err := s.pool.QueryRow(ctx, `
		SELECT id, name, source_type, inbound_path, destination_url, signing_secret_encrypted,
			retry_attempts, retry_first_delay_s, retry_backoff_factor, retention_days, is_playground, created_at
		FROM connections WHERE inbound_path = $1
	`, path).Scan(&c.ID, &c.Name, &c.SourceType, &c.InboundPath, &c.DestinationURL,
		&c.SigningSecretEnc, &c.RetryAttempts, &c.RetryFirstDelayS, &c.RetryBackoffFactor,
		&c.RetentionDays, &c.IsPlayground, &c.CreatedAt)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &c, nil
}

func (s *Store) UpdateConnection(ctx context.Context, id uuid.UUID, dest *string, retryAttempts, retryFirstDelayS *int, retryBackoff *float64, retentionDays *int) (*Connection, error) {
	c, err := s.GetConnection(ctx, id)
	if err != nil || c == nil {
		return c, err
	}
	if dest != nil {
		c.DestinationURL = *dest
	}
	if retryAttempts != nil {
		c.RetryAttempts = *retryAttempts
	}
	if retryFirstDelayS != nil {
		c.RetryFirstDelayS = *retryFirstDelayS
	}
	if retryBackoff != nil {
		c.RetryBackoffFactor = *retryBackoff
	}
	if retentionDays != nil {
		c.RetentionDays = *retentionDays
	}
	_, err = s.pool.Exec(ctx, `
		UPDATE connections SET destination_url=$2, retry_attempts=$3, retry_first_delay_s=$4,
			retry_backoff_factor=$5, retention_days=$6 WHERE id=$1
	`, id, c.DestinationURL, c.RetryAttempts, c.RetryFirstDelayS, c.RetryBackoffFactor, c.RetentionDays)
	return c, err
}

type InsertEventResult struct {
	Event   *Event
	Inserted bool
}

func (s *Store) InsertEvent(ctx context.Context, e *Event) (*InsertEventResult, error) {
	var id uuid.UUID
	var receivedAt time.Time
	err := s.pool.QueryRow(ctx, `
		INSERT INTO events (connection_id, provider_event_id, raw_headers, raw_payload)
		VALUES ($1,$2,$3,$4)
		ON CONFLICT (connection_id, provider_event_id) DO NOTHING
		RETURNING id, received_at
	`, e.ConnectionID, e.ProviderEventID, e.RawHeaders, e.RawPayload).Scan(&id, &receivedAt)
	if err == pgx.ErrNoRows {
		existing, err2 := s.GetEventByProviderID(ctx, e.ConnectionID, e.ProviderEventID)
		if err2 != nil {
			return nil, err2
		}
		return &InsertEventResult{Event: existing, Inserted: false}, nil
	}
	if err != nil {
		return nil, err
	}
	e.ID = id
	e.ReceivedAt = receivedAt
	return &InsertEventResult{Event: e, Inserted: true}, nil
}

func (s *Store) GetEventByProviderID(ctx context.Context, connID uuid.UUID, providerID string) (*Event, error) {
	var e Event
	err := s.pool.QueryRow(ctx, `
		SELECT id, connection_id, provider_event_id, raw_headers, raw_payload, received_at, workflow_id
		FROM events WHERE connection_id=$1 AND provider_event_id=$2
	`, connID, providerID).Scan(&e.ID, &e.ConnectionID, &e.ProviderEventID, &e.RawHeaders,
		&e.RawPayload, &e.ReceivedAt, &e.WorkflowID)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &e, nil
}

func (s *Store) GetEvent(ctx context.Context, id uuid.UUID) (*Event, error) {
	var e Event
	err := s.pool.QueryRow(ctx, `
		SELECT id, connection_id, provider_event_id, raw_headers, raw_payload, received_at, workflow_id
		FROM events WHERE id=$1
	`, id).Scan(&e.ID, &e.ConnectionID, &e.ProviderEventID, &e.RawHeaders,
		&e.RawPayload, &e.ReceivedAt, &e.WorkflowID)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &e, nil
}

func (s *Store) SetEventWorkflowID(ctx context.Context, eventID uuid.UUID, workflowID string) error {
	_, err := s.pool.Exec(ctx, `UPDATE events SET workflow_id=$2 WHERE id=$1`, eventID, workflowID)
	return err
}

func (s *Store) RecordDelivery(ctx context.Context, d *Delivery) error {
	return s.pool.QueryRow(ctx, `
		INSERT INTO deliveries (event_id, attempt_number, status, response_code, latency_ms)
		VALUES ($1,$2,$3,$4,$5) RETURNING id, attempted_at
	`, d.EventID, d.AttemptNumber, d.Status, d.ResponseCode, d.LatencyMS).Scan(&d.ID, &d.AttemptedAt)
}

func (s *Store) ListDeliveriesForConnection(ctx context.Context, connID uuid.UUID, limit int) ([]Delivery, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT d.id, d.event_id, d.attempt_number, d.status, d.response_code, d.latency_ms, d.attempted_at
		FROM deliveries d
		JOIN events e ON e.id = d.event_id
		WHERE e.connection_id = $1
		ORDER BY d.attempted_at DESC LIMIT $2
	`, connID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanDeliveries(rows)
}

func (s *Store) RecordIssue(ctx context.Context, issue *Issue) error {
	return s.pool.QueryRow(ctx, `
		INSERT INTO issues (event_id, connection_id, reason, attempts_exhausted, first_failed_at, status)
		VALUES ($1,$2,$3,$4,$5,'open')
		ON CONFLICT (event_id) DO UPDATE SET
			reason=EXCLUDED.reason, attempts_exhausted=EXCLUDED.attempts_exhausted, status='open', resolved_at=NULL
		RETURNING id
	`, issue.EventID, issue.ConnectionID, issue.Reason, issue.AttemptsExhausted, issue.FirstFailedAt).Scan(&issue.ID)
}

func (s *Store) ListIssues(ctx context.Context, status string) ([]Issue, error) {
	q := `
		SELECT i.id, i.event_id, i.connection_id, i.reason, i.attempts_exhausted, i.first_failed_at,
			i.status, i.resolved_at, i.resolved_by,
			e.provider_event_id, c.destination_url
		FROM issues i
		JOIN events e ON e.id = i.event_id
		JOIN connections c ON c.id = i.connection_id
	`
	args := []any{}
	if status != "" && status != "all" {
		q += ` WHERE i.status = $1`
		args = append(args, status)
	}
	q += ` ORDER BY i.first_failed_at DESC`
	rows, err := s.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Issue
	for rows.Next() {
		var i Issue
		if err := rows.Scan(&i.ID, &i.EventID, &i.ConnectionID, &i.Reason, &i.AttemptsExhausted,
			&i.FirstFailedAt, &i.Status, &i.ResolvedAt, &i.ResolvedBy,
			&i.ProviderEventID, &i.Destination); err != nil {
			return nil, err
		}
		out = append(out, i)
	}
	return out, rows.Err()
}

func (s *Store) GetIssue(ctx context.Context, id uuid.UUID) (*Issue, error) {
	var i Issue
	var payload []byte
	err := s.pool.QueryRow(ctx, `
		SELECT i.id, i.event_id, i.connection_id, i.reason, i.attempts_exhausted, i.first_failed_at,
			i.status, i.resolved_at, i.resolved_by, e.provider_event_id, c.destination_url, e.raw_payload
		FROM issues i
		JOIN events e ON e.id = i.event_id
		JOIN connections c ON c.id = i.connection_id
		WHERE i.id = $1
	`, id).Scan(&i.ID, &i.EventID, &i.ConnectionID, &i.Reason, &i.AttemptsExhausted,
		&i.FirstFailedAt, &i.Status, &i.ResolvedAt, &i.ResolvedBy,
		&i.ProviderEventID, &i.Destination, &payload)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	_ = payload
	return &i, nil
}

func (s *Store) ResolveIssueByEventID(ctx context.Context, eventID uuid.UUID) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE issues SET status='resolved', resolved_at=NOW()
		WHERE event_id=$1 AND status='open'
	`, eventID)
	return err
}

func (s *Store) ResolveIssue(ctx context.Context, id uuid.UUID, userID uuid.UUID) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE issues SET status='resolved', resolved_at=NOW(), resolved_by=$2 WHERE id=$1
	`, id, userID)
	return err
}

func (s *Store) OpenIssuesCount(ctx context.Context) (int, error) {
	var n int
	err := s.pool.QueryRow(ctx, `SELECT COUNT(*) FROM issues WHERE status='open'`).Scan(&n)
	return n, err
}

func (s *Store) CreateUser(ctx context.Context, u *User) error {
	return s.pool.QueryRow(ctx, `
		INSERT INTO users (email, password_hash) VALUES ($1,$2) RETURNING id, created_at
	`, u.Email, u.PasswordHash).Scan(&u.ID, &u.CreatedAt)
}

func (s *Store) GetUserByEmail(ctx context.Context, email string) (*User, error) {
	var u User
	err := s.pool.QueryRow(ctx, `
		SELECT id, email, password_hash, created_at FROM users WHERE email=$1
	`, email).Scan(&u.ID, &u.Email, &u.PasswordHash, &u.CreatedAt)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &u, nil
}

func (s *Store) CreateSession(ctx context.Context, sess *Session) error {
	return s.pool.QueryRow(ctx, `
		INSERT INTO sessions (user_id, expires_at) VALUES ($1,$2) RETURNING id, created_at
	`, sess.UserID, sess.ExpiresAt).Scan(&sess.ID, &sess.CreatedAt)
}

func (s *Store) GetSession(ctx context.Context, id uuid.UUID) (*Session, error) {
	var sess Session
	err := s.pool.QueryRow(ctx, `
		SELECT id, user_id, created_at, expires_at FROM sessions WHERE id=$1 AND expires_at > NOW()
	`, id).Scan(&sess.ID, &sess.UserID, &sess.CreatedAt, &sess.ExpiresAt)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &sess, nil
}

func (s *Store) DeleteSession(ctx context.Context, id uuid.UUID) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM sessions WHERE id=$1`, id)
	return err
}

func (s *Store) CountUsers(ctx context.Context) (int, error) {
	var n int
	err := s.pool.QueryRow(ctx, `SELECT COUNT(*) FROM users`).Scan(&n)
	return n, err
}

func scanDeliveries(rows pgx.Rows) ([]Delivery, error) {
	var out []Delivery
	for rows.Next() {
		var d Delivery
		if err := rows.Scan(&d.ID, &d.EventID, &d.AttemptNumber, &d.Status, &d.ResponseCode, &d.LatencyMS, &d.AttemptedAt); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

func GenerateInboundPath() string {
	return uuid.New().String()[:8]
}

func InboundURL(base, path string) string {
	return fmt.Sprintf("%s/e/%s", base, path)
}
