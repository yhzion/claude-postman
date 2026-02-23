package session

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yhzion/claude-postman/internal/storage"
)

// mockTmux implements TmuxRunner for testing.
type mockTmux struct {
	sessions      map[string]bool
	sentKeys      []sentKey
	captured      string
	captureErr    error
	newSessionErr error
	sendKeysErr   error
	killErr       error
}

type sentKey struct {
	session string
	text    string
}

func newMockTmux() *mockTmux {
	return &mockTmux{
		sessions: make(map[string]bool),
	}
}

func (m *mockTmux) NewSession(name, _ string) error {
	if m.newSessionErr != nil {
		return m.newSessionErr
	}
	m.sessions[name] = true
	return nil
}

func (m *mockTmux) SendKeys(sessionName, text string) error {
	if m.sendKeysErr != nil {
		return m.sendKeysErr
	}
	m.sentKeys = append(m.sentKeys, sentKey{session: sessionName, text: text})
	return nil
}

func (m *mockTmux) CapturePane(_ string, _ int) (string, error) {
	if m.captureErr != nil {
		return "", m.captureErr
	}
	return m.captured, nil
}

func (m *mockTmux) KillSession(sessionName string) error {
	if m.killErr != nil {
		return m.killErr
	}
	delete(m.sessions, sessionName)
	return nil
}

func (m *mockTmux) HasSession(sessionName string) bool {
	return m.sessions[sessionName]
}

func newTestStore(t *testing.T) *storage.Store {
	t.Helper()
	store, err := storage.New(t.TempDir())
	require.NoError(t, err)
	t.Cleanup(func() { store.Close() })
	require.NoError(t, store.Migrate())
	return store
}

func newTestManager(t *testing.T) (*Manager, *mockTmux) {
	t.Helper()
	store := newTestStore(t)
	mock := newMockTmux()
	mgr := New(store, mock)
	mgr.fifoDir = t.TempDir()
	mgr.captureDelay = 0
	return mgr, mock
}

// createTestSession inserts a session directly into the store for testing.
func createTestSession(t *testing.T, mgr *Manager, id, status string) *storage.Session {
	t.Helper()
	session := &storage.Session{
		ID:         id,
		TmuxName:   tmuxName(id),
		WorkingDir: "/tmp/test",
		Model:      "sonnet",
		Status:     status,
	}
	require.NoError(t, mgr.store.CreateSession(session))
	return session
}

func TestCreate_DBRecordAndTmuxSession(t *testing.T) {
	mgr, mock := newTestManager(t)

	session, err := mgr.Create("/tmp/work", "sonnet", "Do something cool")
	require.NoError(t, err)

	// UUID 형식 확인
	assert.NotEmpty(t, session.ID)
	assert.Len(t, session.ID, 36, "UUID 형식이어야 함")

	// tmux 이름 형식
	assert.Equal(t, "session-"+session.ID, session.TmuxName)

	// DB 상태 확인
	assert.Equal(t, "active", session.Status)
	assert.Equal(t, "/tmp/work", session.WorkingDir)
	assert.Equal(t, "sonnet", session.Model)
	require.NotNil(t, session.LastPrompt)
	assert.Equal(t, "Do something cool", *session.LastPrompt)

	// tmux 세션 생성 확인
	assert.True(t, mock.sessions[session.TmuxName], "tmux 세션이 생성되어야 함")

	// send-keys 호출 확인 (claude 명령 + prompt via $(cat))
	require.Len(t, mock.sentKeys, 1)
	assert.Contains(t, mock.sentKeys[0].text, "claude --dangerously-skip-permissions")
	assert.Contains(t, mock.sentKeys[0].text, "--session-id "+session.ID)
	assert.Contains(t, mock.sentKeys[0].text, "--model sonnet")
	assert.Contains(t, mock.sentKeys[0].text, "$(cat ")
}

func TestCreate_TMuxNameFormat(t *testing.T) {
	mgr, _ := newTestManager(t)

	session, err := mgr.Create("/tmp/work", "opus", "task")
	require.NoError(t, err)

	assert.True(t, strings.HasPrefix(session.TmuxName, "session-"))
	assert.Equal(t, "session-"+session.ID, session.TmuxName)
}

func TestEnd_KillsSessionAndUpdatesDB(t *testing.T) {
	mgr, mock := newTestManager(t)

	// 세션을 직접 DB에 생성하고 mock에 추가
	session := createTestSession(t, mgr, "end-test-1", "active")
	mock.sessions[session.TmuxName] = true

	err := mgr.End("end-test-1")
	require.NoError(t, err)

	// tmux 세션 삭제 확인
	assert.False(t, mock.sessions[session.TmuxName], "tmux 세션이 삭제되어야 함")

	// DB 상태 확인
	got, err := mgr.Get("end-test-1")
	require.NoError(t, err)
	assert.Equal(t, "ended", got.Status)
}

func TestEnd_AlreadyEnded(t *testing.T) {
	mgr, _ := newTestManager(t)
	createTestSession(t, mgr, "ended-1", "ended")

	err := mgr.End("ended-1")
	assert.ErrorIs(t, err, ErrSessionEnded)
}

func TestEnd_NotFound(t *testing.T) {
	mgr, _ := newTestManager(t)

	err := mgr.End("nonexistent")
	assert.ErrorIs(t, err, ErrSessionNotFound)
}

func TestDeliverNext_IdleWithMessage(t *testing.T) {
	mgr, mock := newTestManager(t)

	createTestSession(t, mgr, "deliver-1", "idle")
	mock.sessions["session-deliver-1"] = true

	// inbox 메시지 추가
	msg := &storage.InboxMessage{
		ID:        "msg-1",
		SessionID: "deliver-1",
		Body:      "hello claude",
	}
	require.NoError(t, mgr.store.EnqueueMessage(msg))

	err := mgr.DeliverNext("deliver-1")
	require.NoError(t, err)

	// 세션 상태 → active
	got, err := mgr.Get("deliver-1")
	require.NoError(t, err)
	assert.Equal(t, "active", got.Status)
	require.NotNil(t, got.LastPrompt)
	assert.Equal(t, "hello claude", *got.LastPrompt)

	// tmux send-keys 호출 확인
	require.Len(t, mock.sentKeys, 1)
	assert.Equal(t, "hello claude", mock.sentKeys[0].text)
	assert.Equal(t, "session-deliver-1", mock.sentKeys[0].session)
}

func TestDeliverNext_ActiveSession(t *testing.T) {
	mgr, _ := newTestManager(t)
	createTestSession(t, mgr, "active-1", "active")

	err := mgr.DeliverNext("active-1")
	assert.ErrorIs(t, err, ErrSessionNotIdle)
}

func TestDeliverNext_EmptyInbox(t *testing.T) {
	mgr, mock := newTestManager(t)
	createTestSession(t, mgr, "idle-empty", "idle")

	err := mgr.DeliverNext("idle-empty")
	require.NoError(t, err)

	// 상태 변경 없음
	got, err := mgr.Get("idle-empty")
	require.NoError(t, err)
	assert.Equal(t, "idle", got.Status)

	// send-keys 호출 없음
	assert.Empty(t, mock.sentKeys)
}

func TestHandleDone_TransitionsToIdle(t *testing.T) {
	mgr, mock := newTestManager(t)
	createTestSession(t, mgr, "done-1", "active")
	mock.captured = "작업 완료 결과입니다"

	err := mgr.HandleDone("done-1")
	require.NoError(t, err)

	// 세션 상태 → idle
	got, err := mgr.Get("done-1")
	require.NoError(t, err)
	assert.Equal(t, "idle", got.Status)
	require.NotNil(t, got.LastResult)
	assert.Equal(t, "작업 완료 결과입니다", *got.LastResult)

	// outbox 생성 확인
	outbox, err := mgr.store.GetPendingOutbox()
	require.NoError(t, err)
	require.Len(t, outbox, 1)
	assert.Equal(t, "done-1", outbox[0].SessionID)
	assert.Equal(t, "작업 완료 결과입니다", outbox[0].Body)
	assert.Equal(t, "pending", outbox[0].Status)
}

func TestHandleDone_WithPendingInbox(t *testing.T) {
	mgr, mock := newTestManager(t)
	createTestSession(t, mgr, "done-2", "active")
	mock.captured = "첫 번째 결과"

	// inbox에 대기 메시지 추가
	msg := &storage.InboxMessage{
		ID:        "next-msg",
		SessionID: "done-2",
		Body:      "다음 작업 부탁",
	}
	require.NoError(t, mgr.store.EnqueueMessage(msg))

	err := mgr.HandleDone("done-2")
	require.NoError(t, err)

	// 세션 상태: active 유지 (idle이 아님)
	got, err := mgr.Get("done-2")
	require.NoError(t, err)
	assert.Equal(t, "active", got.Status, "대기 메시지가 있으면 active 유지")
	require.NotNil(t, got.LastPrompt)
	assert.Equal(t, "다음 작업 부탁", *got.LastPrompt)

	// outbox 생성 확인
	outbox, err := mgr.store.GetPendingOutbox()
	require.NoError(t, err)
	require.Len(t, outbox, 1)

	// tmux send-keys 호출 확인 (다음 메시지 전달)
	require.Len(t, mock.sentKeys, 1)
	assert.Equal(t, "다음 작업 부탁", mock.sentKeys[0].text)

	// inbox 메시지 처리 확인 (다시 dequeue하면 nil)
	dequeued, err := mgr.store.DequeueMessage("done-2")
	require.NoError(t, err)
	assert.Nil(t, dequeued, "메시지가 이미 처리되어야 함")
}

func TestGet(t *testing.T) {
	mgr, _ := newTestManager(t)
	createTestSession(t, mgr, "get-test", "active")

	got, err := mgr.Get("get-test")
	require.NoError(t, err)
	assert.Equal(t, "get-test", got.ID)
	assert.Equal(t, "active", got.Status)
}

func TestGet_NotFound(t *testing.T) {
	mgr, _ := newTestManager(t)

	got, err := mgr.Get("nonexistent")
	assert.Error(t, err)
	assert.Nil(t, got)
}

func TestListActive(t *testing.T) {
	mgr, _ := newTestManager(t)

	createTestSession(t, mgr, "active-1", "active")
	createTestSession(t, mgr, "idle-1", "idle")
	createTestSession(t, mgr, "creating-1", "creating")
	createTestSession(t, mgr, "ended-1", "ended")

	active, err := mgr.ListActive()
	require.NoError(t, err)
	assert.Len(t, active, 3, "ended 제외 3개 세션이 반환되어야 함")

	ids := make(map[string]bool)
	for _, s := range active {
		ids[s.ID] = true
	}
	assert.True(t, ids["active-1"])
	assert.True(t, ids["idle-1"])
	assert.True(t, ids["creating-1"])
	assert.False(t, ids["ended-1"])
}

func TestRecoverAll_ExistingSession(t *testing.T) {
	mgr, mock := newTestManager(t)
	createTestSession(t, mgr, "recover-exists", "active")
	mock.sessions["session-recover-exists"] = true

	err := mgr.RecoverAll()
	require.NoError(t, err)

	// tmux 세션 유지, 추가 send-keys 없음
	assert.True(t, mock.sessions["session-recover-exists"])
	assert.Empty(t, mock.sentKeys)

	// DB 상태 변경 없음
	got, err := mgr.Get("recover-exists")
	require.NoError(t, err)
	assert.Equal(t, "active", got.Status)
}

func TestRecoverAll_MissingSession(t *testing.T) {
	mgr, mock := newTestManager(t)
	createTestSession(t, mgr, "recover-missing", "active")
	// mock.sessions에 추가하지 않음 → tmux 세션 없음

	err := mgr.RecoverAll()
	require.NoError(t, err)

	// 새 tmux 세션 생성 확인
	assert.True(t, mock.sessions["session-recover-missing"], "복구용 tmux 세션이 생성되어야 함")

	// --resume 명령 확인 (세션 ID 포함)
	require.Len(t, mock.sentKeys, 1)
	assert.Contains(t, mock.sentKeys[0].text, "--resume recover-missing")
	assert.Contains(t, mock.sentKeys[0].text, "--model sonnet")

	// DB 상태 유지
	got, err := mgr.Get("recover-missing")
	require.NoError(t, err)
	assert.Equal(t, "active", got.Status)
}

func TestRecoverAll_RecoveryFailure(t *testing.T) {
	mgr, mock := newTestManager(t)
	createTestSession(t, mgr, "recover-fail", "idle")

	// tmux NewSession 실패 시뮬레이션
	mock.newSessionErr = assert.AnError

	err := mgr.RecoverAll()
	require.NoError(t, err, "RecoverAll 자체는 에러를 반환하지 않음")

	// 복구 실패한 세션은 ended로 전환
	got, err := mgr.Get("recover-fail")
	require.NoError(t, err)
	assert.Equal(t, "ended", got.Status)
}
