package storage

import (
	"context"
	"time"
)

// SaveTemplate inserts a new template record.
func (s *Store) SaveTemplate(tmpl *Template) error {
	if tmpl.CreatedAt.IsZero() {
		tmpl.CreatedAt = time.Now()
	}
	_, err := s.q().ExecContext(context.Background(),
		`INSERT INTO template (id, message_id, created_at) VALUES (?, ?, ?)`,
		tmpl.ID, tmpl.MessageID, formatTime(tmpl.CreatedAt),
	)
	return err
}

// IsValidTemplateRef checks whether a template with the given messageID exists.
func (s *Store) IsValidTemplateRef(messageID string) (bool, error) {
	var count int
	err := s.q().QueryRowContext(context.Background(),
		`SELECT COUNT(*) FROM template WHERE message_id = ?`, messageID,
	).Scan(&count)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}
