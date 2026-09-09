package storage

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
)

func (s *Store) sealEventFields(headers json.RawMessage, payload []byte) (headersEnc, payloadEnc []byte, err error) {
	if s.payloadEnc == nil {
		return []byte(headers), payload, nil
	}
	headersEnc, err = s.payloadEnc.EncryptBytes([]byte(headers))
	if err != nil {
		return nil, nil, fmt.Errorf("encrypt headers: %w", err)
	}
	payloadEnc, err = s.payloadEnc.EncryptBytes(payload)
	if err != nil {
		return nil, nil, fmt.Errorf("encrypt payload: %w", err)
	}
	return headersEnc, payloadEnc, nil
}

func (s *Store) openEventFields(headersEnc, payloadEnc []byte, encrypted bool) (json.RawMessage, []byte, error) {
	if !encrypted || s.payloadEnc == nil {
		return json.RawMessage(headersEnc), payloadEnc, nil
	}
	headers, err := s.payloadEnc.DecryptBytes(headersEnc)
	if err != nil {
		return nil, nil, fmt.Errorf("decrypt headers: %w", err)
	}
	payload, err := s.payloadEnc.DecryptBytes(payloadEnc)
	if err != nil {
		return nil, nil, fmt.Errorf("decrypt payload: %w", err)
	}
	return json.RawMessage(headers), payload, nil
}

// BackfillPayloadEncryption encrypts legacy plaintext events in place.
func (s *Store) BackfillPayloadEncryption(ctx context.Context) (int, error) {
	if s.payloadEnc == nil {
		return 0, nil
	}
	rows, err := s.pool.Query(ctx, `
		SELECT id, raw_headers, raw_payload FROM events WHERE payload_encrypted = FALSE
	`)
	if err != nil {
		return 0, err
	}
	defer rows.Close()

	var count int
	for rows.Next() {
		var id uuid.UUID
		var headersEnc, payloadEnc []byte
		if err := rows.Scan(&id, &headersEnc, &payloadEnc); err != nil {
			return count, err
		}
		sealedHeaders, sealedPayload, err := s.sealEventFields(json.RawMessage(headersEnc), payloadEnc)
		if err != nil {
			return count, err
		}
		tag, err := s.pool.Exec(ctx, `
			UPDATE events SET raw_headers=$2, raw_payload=$3, payload_encrypted=TRUE WHERE id=$1
		`, id, sealedHeaders, sealedPayload)
		if err != nil {
			return count, err
		}
		count += int(tag.RowsAffected())
	}
	return count, rows.Err()
}

// PurgeExpiredEvents deletes events older than each connection's retention_days.
func (s *Store) PurgeExpiredEvents(ctx context.Context) (int64, error) {
	tag, err := s.pool.Exec(ctx, `
		DELETE FROM events e
		USING connections c
		WHERE e.connection_id = c.id
		  AND e.received_at < NOW() - (c.retention_days * INTERVAL '1 day')
	`)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}
