package storage

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEnqueueMessage_DequeueMessage(t *testing.T) {
	store := newTestStore(t)
	createTestSession(t, store, "sess-1")

	msg := &InboxMessage{
		ID:        "inbox-1",
		SessionID: "sess-1",
		Body:      "테스트 메시지",
	}
	err := store.EnqueueMessage(msg)
	require.NoError(t, err)

	got, err := store.DequeueMessage("sess-1")
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "inbox-1", got.ID)
	assert.Equal(t, "sess-1", got.SessionID)
	assert.Equal(t, "테스트 메시지", got.Body)
	assert.False(t, got.Processed)
}

func TestDequeueMessage_FIFO(t *testing.T) {
	store := newTestStore(t)
	createTestSession(t, store, "sess-1")

	now := time.Now()
	// 시간 순서대로 3개 메시지 생성 (FIFO 확인용)
	for i, id := range []string{"first", "second", "third"} {
		msg := &InboxMessage{
			ID:        id,
			SessionID: "sess-1",
			Body:      "body-" + id,
			CreatedAt: now.Add(time.Duration(i) * time.Minute),
		}
		err := store.EnqueueMessage(msg)
		require.NoError(t, err)
	}

	// 첫 번째 dequeue → "first" (가장 오래된 메시지)
	got, err := store.DequeueMessage("sess-1")
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "first", got.ID, "FIFO: 가장 오래된 메시지가 먼저 나와야 함")

	// MarkProcessed 후 다음 dequeue → "second"
	err = store.MarkProcessed("first")
	require.NoError(t, err)

	got, err = store.DequeueMessage("sess-1")
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "second", got.ID, "FIFO: 두 번째 메시지가 나와야 함")
}

func TestDequeueMessage_EmptyQueue(t *testing.T) {
	store := newTestStore(t)
	createTestSession(t, store, "sess-1")

	// 빈 큐에서 dequeue → nil, 에러 없음
	got, err := store.DequeueMessage("sess-1")
	assert.NoError(t, err, "빈 큐에서 에러 없이 nil 반환")
	assert.Nil(t, got, "빈 큐에서 nil 반환해야 함")
}

func TestMarkProcessed(t *testing.T) {
	store := newTestStore(t)
	createTestSession(t, store, "sess-1")

	msg := &InboxMessage{
		ID:        "inbox-mark",
		SessionID: "sess-1",
		Body:      "process me",
	}
	err := store.EnqueueMessage(msg)
	require.NoError(t, err)

	err = store.MarkProcessed("inbox-mark")
	require.NoError(t, err)

	// processed 플래그 확인
	var processed int
	err = store.db.QueryRow(
		"SELECT processed FROM inbox WHERE id = ?", "inbox-mark",
	).Scan(&processed)
	require.NoError(t, err)
	assert.Equal(t, 1, processed, "processed가 1이어야 함")

	// processed 메시지는 dequeue에서 제외
	got, err := store.DequeueMessage("sess-1")
	assert.NoError(t, err)
	assert.Nil(t, got, "처리 완료된 메시지는 dequeue되면 안 됨")
}
