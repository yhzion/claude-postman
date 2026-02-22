package storage

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// CreateOutbox inserts a new outbox message.
func (s *Store) CreateOutbox(msg *OutboxMessage) error {
	if msg.CreatedAt.IsZero() {
		msg.CreatedAt = time.Now()
	}
	_, err := s.q().ExecContext(context.Background(),
		`INSERT INTO outbox (id, session_id, message_id, subject, body, attachments, status, retry_count, next_retry_at, created_at, sent_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		msg.ID, msg.SessionID, msg.MessageID, msg.Subject, msg.Body, msg.Attachments,
		msg.Status, msg.RetryCount, formatNullableTime(msg.NextRetryAt),
		formatTime(msg.CreatedAt), formatNullableTime(msg.SentAt),
	)
	return err
}

// GetPendingOutbox retrieves outbox messages ready to be sent.
// Conditions: status=pending AND (next_retry_at IS NULL OR next_retry_at <= now).
func (s *Store) GetPendingOutbox() ([]*OutboxMessage, error) {
	rows, err := s.q().QueryContext(context.Background(),
		`SELECT id, session_id, message_id, subject, body, attachments, status, retry_count, next_retry_at, created_at, sent_at
		 FROM outbox WHERE status = 'pending' AND (next_retry_at IS NULL OR next_retry_at <= datetime('now'))`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var msgs []*OutboxMessage
	for rows.Next() {
		msg, err := scanOutbox(rows)
		if err != nil {
			return nil, err
		}
		msgs = append(msgs, msg)
	}
	return msgs, rows.Err()
}

// MarkSent marks an outbox message as sent.
func (s *Store) MarkSent(id string) error {
	_, err := s.q().ExecContext(context.Background(),
		`UPDATE outbox SET status = 'sent', sent_at = datetime('now') WHERE id = ?`, id,
	)
	return err
}

// MarkFailed updates an outbox message's retry information.
func (s *Store) MarkFailed(id string, retryCount int, nextRetryAt *time.Time) error {
	_, err := s.q().ExecContext(context.Background(),
		`UPDATE outbox SET status = 'failed', retry_count = ?, next_retry_at = ? WHERE id = ?`,
		retryCount, formatNullableTime(nextRetryAt), id,
	)
	return err
}

// PurgeOldData removes old sent/processed data for ended sessions.
func (s *Store) PurgeOldData(retentionDays int) error {
	_, err := s.q().ExecContext(context.Background(), fmt.Sprintf(
		`DELETE FROM outbox WHERE session_id IN (SELECT id FROM sessions WHERE status = 'ended')
		 AND status = 'sent' AND sent_at < datetime('now', '-%d days')`, retentionDays),
	)
	if err != nil {
		return err
	}
	_, err = s.q().ExecContext(context.Background(), fmt.Sprintf(
		`DELETE FROM inbox WHERE session_id IN (SELECT id FROM sessions WHERE status = 'ended')
		 AND processed = 1 AND created_at < datetime('now', '-%d days')`, retentionDays),
	)
	return err
}

func scanOutbox(row scanner) (*OutboxMessage, error) {
	var msg OutboxMessage
	var messageID, attachments sql.NullString
	var nextRetryAt, sentAt sql.NullTime

	err := row.Scan(
		&msg.ID, &msg.SessionID, &messageID, &msg.Subject, &msg.Body, &attachments,
		&msg.Status, &msg.RetryCount, &nextRetryAt, &msg.CreatedAt, &sentAt,
	)
	if err != nil {
		return nil, err
	}

	if messageID.Valid {
		msg.MessageID = &messageID.String
	}
	if attachments.Valid {
		msg.Attachments = &attachments.String
	}
	if nextRetryAt.Valid {
		msg.NextRetryAt = &nextRetryAt.Time
	}
	if sentAt.Valid {
		msg.SentAt = &sentAt.Time
	}
	return &msg, nil
}
