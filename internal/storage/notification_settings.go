package storage

import (
	"context"

	"github.com/jackc/pgx/v5"
)

// NotificationSettings is the single console row for alert delivery.
// Encrypted fields are nil when the console has not saved a secret.
// A zero port or empty host means "use the server environment".
type NotificationSettings struct {
	SMTPHost         string
	SMTPPort         int
	SMTPUsername     string
	SMTPPasswordEnc  []byte
	SMTPFrom         string
	SlackBotTokenEnc []byte
}

func (s *Store) GetNotificationSettings(ctx context.Context) (NotificationSettings, error) {
	var row NotificationSettings
	err := s.pool.QueryRow(ctx, `
		SELECT smtp_host, smtp_port, smtp_username, smtp_password_enc, smtp_from, slack_bot_token_enc
		FROM notification_settings WHERE id = 1
	`).Scan(&row.SMTPHost, &row.SMTPPort, &row.SMTPUsername, &row.SMTPPasswordEnc, &row.SMTPFrom, &row.SlackBotTokenEnc)
	if err == pgx.ErrNoRows {
		return NotificationSettings{}, nil
	}
	return row, err
}

func (s *Store) SaveNotificationSettings(ctx context.Context, row NotificationSettings) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO notification_settings (
			id, smtp_host, smtp_port, smtp_username, smtp_password_enc, smtp_from, slack_bot_token_enc, updated_at
		) VALUES (1, $1, $2, $3, $4, $5, $6, NOW())
		ON CONFLICT (id) DO UPDATE SET
			smtp_host = EXCLUDED.smtp_host,
			smtp_port = EXCLUDED.smtp_port,
			smtp_username = EXCLUDED.smtp_username,
			smtp_password_enc = EXCLUDED.smtp_password_enc,
			smtp_from = EXCLUDED.smtp_from,
			slack_bot_token_enc = EXCLUDED.slack_bot_token_enc,
			updated_at = NOW()
	`, row.SMTPHost, row.SMTPPort, row.SMTPUsername, row.SMTPPasswordEnc, row.SMTPFrom, row.SlackBotTokenEnc)
	return err
}
