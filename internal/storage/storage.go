// Package storage handles SQLite database operations.
package storage

import (
	"context"
	"database/sql"
	"path/filepath"
	"time"

	_ "github.com/mattn/go-sqlite3"
	"github.com/yhzion/claude-postman/internal/storage/migrations"
)

const sqliteTimeFormat = "2006-01-02 15:04:05"

// Store wraps an SQLite database connection.
type Store struct {
	db *sql.DB
	tx *sql.Tx
}

// Session represents a Claude Code tmux session.
type Session struct {
	ID         string
	TmuxName   string
	WorkingDir string
	Model      string
	Status     string
	CreatedAt  time.Time
	UpdatedAt  time.Time
	LastPrompt *string
	LastResult *string
}

// OutboxMessage represents an outgoing email message.
type OutboxMessage struct {
	ID          string
	SessionID   string
	MessageID   *string
	Subject     string
	Body        string
	Attachments *string
	Status      string
	RetryCount  int
	NextRetryAt *time.Time
	CreatedAt   time.Time
	SentAt      *time.Time
}

// InboxMessage represents an incoming email message.
type InboxMessage struct {
	ID        string
	SessionID string
	Body      string
	CreatedAt time.Time
	Processed bool
}

// Template represents an email template record.
type Template struct {
	ID        string
	MessageID string
	CreatedAt time.Time
}

type querier interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

type scanner interface {
	Scan(dest ...any) error
}

func (s *Store) q() querier {
	if s.tx != nil {
		return s.tx
	}
	return s.db
}

func formatTime(t time.Time) string {
	return t.UTC().Format(sqliteTimeFormat)
}

func formatNullableTime(t *time.Time) any {
	if t == nil {
		return nil
	}
	return formatTime(*t)
}

// New creates a new Store backed by SQLite in dataDir.
func New(dataDir string) (*Store, error) {
	dbPath := filepath.Join(dataDir, "claude-postman.db")
	db, err := sql.Open("sqlite3", dbPath+"?_journal_mode=WAL&_foreign_keys=ON")
	if err != nil {
		return nil, err
	}
	if err := db.Ping(); err != nil {
		return nil, err
	}
	return &Store{db: db}, nil
}

// Close closes the database connection.
func (s *Store) Close() error {
	return s.db.Close()
}

// Migrate runs pending database migrations.
func (s *Store) Migrate() error {
	var name string
	err := s.db.QueryRow(
		"SELECT name FROM sqlite_master WHERE type='table' AND name='schema_version'",
	).Scan(&name)
	if err == sql.ErrNoRows {
		content, err := migrations.Files.ReadFile("001_init.sql")
		if err != nil {
			return err
		}
		_, err = s.db.Exec(string(content))
		return err
	}
	return err
}

// Tx executes fn within a database transaction.
func (s *Store) Tx(ctx context.Context, fn func(tx *Store) error) error {
	sqlTx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	txStore := &Store{db: s.db, tx: sqlTx}
	if err := fn(txStore); err != nil {
		_ = sqlTx.Rollback()
		return err
	}
	return sqlTx.Commit()
}
