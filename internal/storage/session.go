package storage

import (
	"context"
	"database/sql"
	"strings"
	"time"
)

// CreateSession inserts a new session record.
func (s *Store) CreateSession(session *Session) error {
	if session.CreatedAt.IsZero() {
		session.CreatedAt = time.Now()
	}
	if session.UpdatedAt.IsZero() {
		session.UpdatedAt = time.Now()
	}
	_, err := s.q().ExecContext(context.Background(),
		`INSERT INTO sessions (id, tmux_name, working_dir, model, status, created_at, updated_at, last_prompt, last_result)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		session.ID, session.TmuxName, session.WorkingDir, session.Model, session.Status,
		formatTime(session.CreatedAt), formatTime(session.UpdatedAt),
		session.LastPrompt, session.LastResult,
	)
	return err
}

// GetSession retrieves a session by ID.
func (s *Store) GetSession(id string) (*Session, error) {
	row := s.q().QueryRowContext(context.Background(),
		`SELECT id, tmux_name, working_dir, model, status, created_at, updated_at, last_prompt, last_result
		 FROM sessions WHERE id = ?`, id,
	)
	return scanSession(row)
}

// UpdateSession updates an existing session record.
func (s *Store) UpdateSession(session *Session) error {
	session.UpdatedAt = time.Now()
	_, err := s.q().ExecContext(context.Background(),
		`UPDATE sessions SET tmux_name = ?, working_dir = ?, model = ?, status = ?,
		 updated_at = ?, last_prompt = ?, last_result = ? WHERE id = ?`,
		session.TmuxName, session.WorkingDir, session.Model, session.Status,
		formatTime(session.UpdatedAt), session.LastPrompt, session.LastResult,
		session.ID,
	)
	return err
}

// ListSessionsByStatus retrieves sessions matching any of the given statuses.
func (s *Store) ListSessionsByStatus(statuses ...string) ([]*Session, error) {
	if len(statuses) == 0 {
		return nil, nil
	}
	placeholders := make([]string, len(statuses))
	args := make([]any, len(statuses))
	for i, st := range statuses {
		placeholders[i] = "?"
		args[i] = st
	}
	query := `SELECT id, tmux_name, working_dir, model, status, created_at, updated_at, last_prompt, last_result
	          FROM sessions WHERE status IN (` + strings.Join(placeholders, ",") + `)`
	rows, err := s.q().QueryContext(context.Background(), query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var sessions []*Session
	for rows.Next() {
		session, err := scanSession(rows)
		if err != nil {
			return nil, err
		}
		sessions = append(sessions, session)
	}
	return sessions, rows.Err()
}

func scanSession(row scanner) (*Session, error) {
	var s Session
	var lastPrompt, lastResult sql.NullString

	err := row.Scan(
		&s.ID, &s.TmuxName, &s.WorkingDir, &s.Model, &s.Status,
		&s.CreatedAt, &s.UpdatedAt, &lastPrompt, &lastResult,
	)
	if err != nil {
		return nil, err
	}

	if lastPrompt.Valid {
		s.LastPrompt = &lastPrompt.String
	}
	if lastResult.Valid {
		s.LastResult = &lastResult.String
	}
	return &s, nil
}
