package storage

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestPurgeExpiredEvents(t *testing.T) {
	url := os.Getenv("DATABASE_URL")
	if url == "" {
		t.Skip("DATABASE_URL not set")
	}

	m, err := migrate.New("file://../../migrations", url)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = m.Close() })
	if err := m.Up(); err != nil && err != migrate.ErrNoChange {
		t.Fatal(err)
	}

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)

	store := New(pool, nil)
	path := "retention-" + uuid.NewString()
	var connID uuid.UUID
	err = pool.QueryRow(ctx, `
		INSERT INTO connections (
			name, source_type, inbound_path, destination_url, signing_secret_encrypted, retention_days
		) VALUES ('retention test', 'internal', $1, 'http://example.invalid/hook', '\x00', 1)
		RETURNING id
	`, path).Scan(&connID)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM connections WHERE id = $1`, connID)
	})

	insert := func(eventID string, received time.Time) {
		t.Helper()
		_, err := pool.Exec(ctx, `
			INSERT INTO events (
				connection_id, provider_event_id, raw_headers, raw_payload, payload_encrypted, received_at
			) VALUES ($1, $2, '\x00', '\x00', TRUE, $3)
		`, connID, eventID, received)
		if err != nil {
			t.Fatal(err)
		}
	}
	insert("old", time.Now().Add(-72*time.Hour))
	insert("fresh", time.Now())

	if _, err := store.PurgeExpiredEvents(ctx); err != nil {
		t.Fatal(err)
	}

	var oldLeft, freshLeft int
	err = pool.QueryRow(ctx, `
		SELECT
			count(*) FILTER (WHERE provider_event_id = 'old'),
			count(*) FILTER (WHERE provider_event_id = 'fresh')
		FROM events WHERE connection_id = $1
	`, connID).Scan(&oldLeft, &freshLeft)
	if err != nil {
		t.Fatal(err)
	}
	if oldLeft != 0 || freshLeft != 1 {
		t.Fatalf("old=%d fresh=%d", oldLeft, freshLeft)
	}
}
