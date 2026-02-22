package storage

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newTestStore는 임시 디렉터리에 Store를 생성하고 마이그레이션을 적용한다.
func newTestStore(t *testing.T) *Store {
	t.Helper()
	store, err := New(t.TempDir())
	require.NoError(t, err, "Store 생성 실패")
	t.Cleanup(func() { store.Close() })
	err = store.Migrate()
	require.NoError(t, err, "마이그레이션 실패")
	return store
}

// createTestSession은 기본값으로 테스트 세션을 생성한다.
func createTestSession(t *testing.T, store *Store, id string) *Session {
	t.Helper()
	session := &Session{
		ID:         id,
		TmuxName:   "session-" + id,
		WorkingDir: "/tmp/test",
		Model:      "sonnet",
		Status:     "active",
	}
	err := store.CreateSession(session)
	require.NoError(t, err, "테스트 세션 생성 실패")
	return session
}

func TestNew_CreatesDBFile(t *testing.T) {
	dir := t.TempDir()
	store, err := New(dir)
	require.NoError(t, err)
	defer store.Close()

	dbPath := filepath.Join(dir, "claude-postman.db")
	_, err = os.Stat(dbPath)
	assert.NoError(t, err, "DB 파일이 생성되어야 함")
}

func TestNew_WALMode(t *testing.T) {
	dir := t.TempDir()
	store, err := New(dir)
	require.NoError(t, err)
	defer store.Close()

	var journalMode string
	err = store.db.QueryRow("PRAGMA journal_mode").Scan(&journalMode)
	require.NoError(t, err)
	assert.Equal(t, "wal", journalMode, "WAL 모드가 활성화되어야 함")
}

func TestMigrate_CreatesTables(t *testing.T) {
	store := newTestStore(t)

	// 모든 테이블 존재 확인
	tables := []string{"sessions", "outbox", "inbox", "template", "schema_version"}
	for _, table := range tables {
		var name string
		err := store.db.QueryRow(
			"SELECT name FROM sqlite_master WHERE type='table' AND name=?", table,
		).Scan(&name)
		assert.NoError(t, err, "%s 테이블이 존재해야 함", table)
		assert.Equal(t, table, name)
	}
}

func TestMigrate_Idempotent(t *testing.T) {
	store := newTestStore(t)

	// 2회 실행해도 에러 없어야 함
	err := store.Migrate()
	assert.NoError(t, err, "Migrate() 2회 실행 시 에러가 없어야 함")
}

func TestMigrate_SchemaVersion(t *testing.T) {
	store := newTestStore(t)

	var version int
	err := store.db.QueryRow("SELECT version FROM schema_version").Scan(&version)
	require.NoError(t, err)
	assert.Equal(t, 1, version, "schema_version이 1이어야 함")
}

func TestClose(t *testing.T) {
	dir := t.TempDir()
	store, err := New(dir)
	require.NoError(t, err)

	err = store.Close()
	assert.NoError(t, err, "Close()가 정상 종료되어야 함")

	// Close 후 DB 접근 실패 확인
	err = store.db.Ping()
	assert.Error(t, err, "Close 후에는 DB 접근이 실패해야 함")
}

func TestTx_Commit(t *testing.T) {
	store := newTestStore(t)

	session := &Session{
		ID:         "tx-commit-test",
		TmuxName:   "session-tx-commit",
		WorkingDir: "/tmp",
		Model:      "sonnet",
		Status:     "active",
	}

	// nil 에러 반환 → 커밋
	err := store.Tx(context.Background(), func(tx *Store) error {
		return tx.CreateSession(session)
	})
	require.NoError(t, err, "Tx 커밋이 성공해야 함")

	// 트랜잭션 외부에서 조회 가능 확인
	got, err := store.GetSession("tx-commit-test")
	require.NoError(t, err)
	assert.Equal(t, "tx-commit-test", got.ID)
}

func TestTx_Rollback(t *testing.T) {
	store := newTestStore(t)

	session := &Session{
		ID:         "tx-rollback-test",
		TmuxName:   "session-tx-rollback",
		WorkingDir: "/tmp",
		Model:      "sonnet",
		Status:     "active",
	}

	// 에러 반환 → 롤백
	testErr := errors.New("의도적 에러")
	err := store.Tx(context.Background(), func(tx *Store) error {
		if createErr := tx.CreateSession(session); createErr != nil {
			return createErr
		}
		return testErr
	})
	assert.ErrorIs(t, err, testErr, "Tx가 fn의 에러를 반환해야 함")

	// 롤백 확인: 세션이 존재하면 안 됨
	got, err := store.GetSession("tx-rollback-test")
	assert.Error(t, err, "롤백된 세션은 조회 불가해야 함")
	assert.Nil(t, got)
}
