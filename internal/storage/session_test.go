package storage

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCreateSession_GetSession(t *testing.T) {
	store := newTestStore(t)

	session := &Session{
		ID:         "sess-1",
		TmuxName:   "session-sess-1",
		WorkingDir: "/home/test/project",
		Model:      "sonnet",
		Status:     "creating",
	}
	err := store.CreateSession(session)
	require.NoError(t, err)

	got, err := store.GetSession("sess-1")
	require.NoError(t, err)
	assert.Equal(t, "sess-1", got.ID)
	assert.Equal(t, "session-sess-1", got.TmuxName)
	assert.Equal(t, "/home/test/project", got.WorkingDir)
	assert.Equal(t, "sonnet", got.Model)
	assert.Equal(t, "creating", got.Status)
	assert.False(t, got.CreatedAt.IsZero(), "CreatedAt이 설정되어야 함")
	assert.False(t, got.UpdatedAt.IsZero(), "UpdatedAt이 설정되어야 함")
	assert.Nil(t, got.LastPrompt)
	assert.Nil(t, got.LastResult)
}

func TestUpdateSession(t *testing.T) {
	store := newTestStore(t)
	session := createTestSession(t, store, "update-test")

	// 필드 변경
	session.Status = "idle"
	prompt := "updated prompt"
	session.LastPrompt = &prompt
	result := "some result"
	session.LastResult = &result

	err := store.UpdateSession(session)
	require.NoError(t, err)

	got, err := store.GetSession("update-test")
	require.NoError(t, err)
	assert.Equal(t, "idle", got.Status)
	require.NotNil(t, got.LastPrompt)
	assert.Equal(t, "updated prompt", *got.LastPrompt)
	require.NotNil(t, got.LastResult)
	assert.Equal(t, "some result", *got.LastResult)
}

func TestListSessionsByStatus(t *testing.T) {
	store := newTestStore(t)

	// 다양한 상태의 세션 생성
	for _, s := range []struct{ id, status string }{
		{"active-1", "active"},
		{"active-2", "active"},
		{"idle-1", "idle"},
		{"ended-1", "ended"},
	} {
		session := &Session{
			ID:         s.id,
			TmuxName:   "session-" + s.id,
			WorkingDir: "/tmp",
			Model:      "sonnet",
			Status:     s.status,
		}
		err := store.CreateSession(session)
		require.NoError(t, err)
	}

	// active만 조회
	active, err := store.ListSessionsByStatus("active")
	require.NoError(t, err)
	assert.Len(t, active, 2, "active 세션이 2개여야 함")

	// active + idle 조회
	activeAndIdle, err := store.ListSessionsByStatus("active", "idle")
	require.NoError(t, err)
	assert.Len(t, activeAndIdle, 3, "active+idle 세션이 3개여야 함")

	// ended 조회
	ended, err := store.ListSessionsByStatus("ended")
	require.NoError(t, err)
	assert.Len(t, ended, 1, "ended 세션이 1개여야 함")
}

func TestGetSession_NotFound(t *testing.T) {
	store := newTestStore(t)

	got, err := store.GetSession("nonexistent-id")
	assert.Error(t, err, "존재하지 않는 ID 조회 시 에러 반환")
	assert.Nil(t, got)
}
