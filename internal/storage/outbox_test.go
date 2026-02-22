package storage

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCreateOutbox_GetPendingOutbox(t *testing.T) {
	store := newTestStore(t)
	createTestSession(t, store, "sess-1")

	msg := &OutboxMessage{
		ID:        "outbox-1",
		SessionID: "sess-1",
		Subject:   "테스트 제목",
		Body:      "<p>테스트 본문</p>",
		Status:    "pending",
	}
	err := store.CreateOutbox(msg)
	require.NoError(t, err)

	pending, err := store.GetPendingOutbox()
	require.NoError(t, err)
	require.Len(t, pending, 1)
	assert.Equal(t, "outbox-1", pending[0].ID)
	assert.Equal(t, "sess-1", pending[0].SessionID)
	assert.Equal(t, "테스트 제목", pending[0].Subject)
	assert.Equal(t, "pending", pending[0].Status)
}

func TestMarkSent(t *testing.T) {
	store := newTestStore(t)
	createTestSession(t, store, "sess-1")

	msg := &OutboxMessage{
		ID:        "outbox-sent",
		SessionID: "sess-1",
		Subject:   "Sent test",
		Body:      "body",
		Status:    "pending",
	}
	err := store.CreateOutbox(msg)
	require.NoError(t, err)

	err = store.MarkSent("outbox-sent")
	require.NoError(t, err)

	// sent 메시지는 pending 목록에서 제외
	pending, err := store.GetPendingOutbox()
	require.NoError(t, err)
	assert.Len(t, pending, 0, "sent 메시지는 pending 목록에 없어야 함")

	// sent_at 설정 확인
	var sentAt *string
	err = store.db.QueryRow(
		"SELECT sent_at FROM outbox WHERE id = ?", "outbox-sent",
	).Scan(&sentAt)
	require.NoError(t, err)
	assert.NotNil(t, sentAt, "sent_at이 설정되어야 함")
}

func TestMarkFailed(t *testing.T) {
	store := newTestStore(t)
	createTestSession(t, store, "sess-1")

	msg := &OutboxMessage{
		ID:        "outbox-fail",
		SessionID: "sess-1",
		Subject:   "Fail test",
		Body:      "body",
		Status:    "pending",
	}
	err := store.CreateOutbox(msg)
	require.NoError(t, err)

	nextRetry := time.Now().Add(5 * time.Minute)
	err = store.MarkFailed("outbox-fail", 1, &nextRetry)
	require.NoError(t, err)

	// retry_count, status 확인
	var retryCount int
	var status string
	err = store.db.QueryRow(
		"SELECT retry_count, status FROM outbox WHERE id = ?", "outbox-fail",
	).Scan(&retryCount, &status)
	require.NoError(t, err)
	assert.Equal(t, 1, retryCount, "retry_count가 1이어야 함")
	assert.Equal(t, "failed", status, "status가 failed여야 함")
}

func TestGetPendingOutbox_ExcludesFutureRetry(t *testing.T) {
	store := newTestStore(t)
	createTestSession(t, store, "sess-1")

	// next_retry_at이 미래인 메시지 (아직 재시도 불가)
	futureRetry := time.Now().Add(time.Hour)
	msgFuture := &OutboxMessage{
		ID:          "outbox-future",
		SessionID:   "sess-1",
		Subject:     "Future retry",
		Body:        "body",
		Status:      "pending",
		NextRetryAt: &futureRetry,
	}
	err := store.CreateOutbox(msgFuture)
	require.NoError(t, err)

	// next_retry_at 없는 메시지 (즉시 발송 가능)
	msgReady := &OutboxMessage{
		ID:        "outbox-ready",
		SessionID: "sess-1",
		Subject:   "Ready",
		Body:      "body",
		Status:    "pending",
	}
	err = store.CreateOutbox(msgReady)
	require.NoError(t, err)

	pending, err := store.GetPendingOutbox()
	require.NoError(t, err)
	require.Len(t, pending, 1, "미래 retry 메시지는 제외되어야 함")
	assert.Equal(t, "outbox-ready", pending[0].ID)
}

func TestPurgeOldData(t *testing.T) {
	store := newTestStore(t)

	// ended 세션과 active 세션 생성
	endedSession := createTestSession(t, store, "ended-sess")
	endedSession.Status = "ended"
	err := store.UpdateSession(endedSession)
	require.NoError(t, err)

	createTestSession(t, store, "active-sess")

	// --- ended 세션: 오래된 sent outbox (purge 대상) ---
	oldOut := &OutboxMessage{
		ID: "old-sent", SessionID: "ended-sess",
		Subject: "Old", Body: "body", Status: "pending",
	}
	err = store.CreateOutbox(oldOut)
	require.NoError(t, err)
	err = store.MarkSent("old-sent")
	require.NoError(t, err)
	_, err = store.db.Exec(
		"UPDATE outbox SET sent_at = datetime('now', '-60 days'), created_at = datetime('now', '-60 days') WHERE id = ?",
		"old-sent",
	)
	require.NoError(t, err)

	// --- ended 세션: 최근 sent outbox (보존) ---
	recentOut := &OutboxMessage{
		ID: "recent-sent", SessionID: "ended-sess",
		Subject: "Recent", Body: "body", Status: "pending",
	}
	err = store.CreateOutbox(recentOut)
	require.NoError(t, err)
	err = store.MarkSent("recent-sent")
	require.NoError(t, err)

	// --- active 세션: 오래된 sent outbox (보존 — active이므로) ---
	activeOut := &OutboxMessage{
		ID: "active-old-sent", SessionID: "active-sess",
		Subject: "Active old", Body: "body", Status: "pending",
	}
	err = store.CreateOutbox(activeOut)
	require.NoError(t, err)
	err = store.MarkSent("active-old-sent")
	require.NoError(t, err)
	_, err = store.db.Exec(
		"UPDATE outbox SET sent_at = datetime('now', '-60 days'), created_at = datetime('now', '-60 days') WHERE id = ?",
		"active-old-sent",
	)
	require.NoError(t, err)

	// --- ended 세션: 오래된 processed inbox (purge 대상) ---
	oldInbox := &InboxMessage{
		ID: "old-inbox", SessionID: "ended-sess", Body: "old inbox",
	}
	err = store.EnqueueMessage(oldInbox)
	require.NoError(t, err)
	err = store.MarkProcessed("old-inbox")
	require.NoError(t, err)
	_, err = store.db.Exec(
		"UPDATE inbox SET created_at = datetime('now', '-60 days') WHERE id = ?",
		"old-inbox",
	)
	require.NoError(t, err)

	// --- ended 세션: 미처리 inbox (보존) ---
	unprocessedInbox := &InboxMessage{
		ID: "unprocessed-inbox", SessionID: "ended-sess", Body: "unprocessed",
	}
	err = store.EnqueueMessage(unprocessedInbox)
	require.NoError(t, err)

	// PurgeOldData 실행 (30일 보존)
	err = store.PurgeOldData(30)
	require.NoError(t, err)

	// 검증
	var count int

	// 오래된 ended 세션 sent outbox → 삭제됨
	err = store.db.QueryRow("SELECT COUNT(*) FROM outbox WHERE id = ?", "old-sent").Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, 0, count, "오래된 sent outbox는 삭제되어야 함")

	// 최근 ended 세션 sent outbox → 보존
	err = store.db.QueryRow("SELECT COUNT(*) FROM outbox WHERE id = ?", "recent-sent").Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, 1, count, "최근 sent outbox는 보존되어야 함")

	// active 세션 outbox → 보존
	err = store.db.QueryRow("SELECT COUNT(*) FROM outbox WHERE id = ?", "active-old-sent").Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, 1, count, "active 세션 outbox는 보존되어야 함")

	// 오래된 ended 세션 processed inbox → 삭제됨
	err = store.db.QueryRow("SELECT COUNT(*) FROM inbox WHERE id = ?", "old-inbox").Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, 0, count, "오래된 processed inbox는 삭제되어야 함")

	// 미처리 inbox → 보존
	err = store.db.QueryRow("SELECT COUNT(*) FROM inbox WHERE id = ?", "unprocessed-inbox").Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, 1, count, "미처리 inbox는 보존되어야 함")
}
