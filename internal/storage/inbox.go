package storage

import (
	"context"
	"database/sql"
	"time"
)

// EnqueueMessage inserts a new inbox message.
func (s *Store) EnqueueMessage(msg *InboxMessage) error {
	if msg.CreatedAt.IsZero() {
		msg.CreatedAt = time.Now()
	}
	_, err := s.q().ExecContext(context.Background(),
		`INSERT INTO inbox (id, session_id, body, created_at, processed)
		 VALUES (?, ?, ?, ?, ?)`,
		msg.ID, msg.SessionID, msg.Body, formatTime(msg.CreatedAt), boolToInt(msg.Processed),
	)
	return err
}

// DequeueMessage retrieves the oldest unprocessed message for a session.
func (s *Store) DequeueMessage(sessionID string) (*InboxMessage, error) {
	row := s.q().QueryRowContext(context.Background(),
		`SELECT id, session_id, body, created_at, processed
		 FROM inbox WHERE session_id = ? AND processed = 0 ORDER BY created_at ASC LIMIT 1`,
		sessionID,
	)

	var msg InboxMessage
	var processed int

	err := row.Scan(&msg.ID, &msg.SessionID, &msg.Body, &msg.CreatedAt, &processed)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	msg.Processed = processed != 0
	return &msg, nil
}

// MarkProcessed marks an inbox message as processed.
func (s *Store) MarkProcessed(id string) error {
	_, err := s.q().ExecContext(context.Background(),
		`UPDATE inbox SET processed = 1 WHERE id = ?`, id,
	)
	return err
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
