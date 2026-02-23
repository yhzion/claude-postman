package serve

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yhzion/claude-postman/internal/config"
	"github.com/yhzion/claude-postman/internal/email"
	"github.com/yhzion/claude-postman/internal/storage"
)

// --- Mocks ---

type mockMgr struct {
	createFn        func(string, string, string) (*storage.Session, error)
	deliverFn       func(string) error
	listActiveFn    func() ([]*storage.Session, error)
	recoverAllFn    func() error
	handleAskFn     func(string) error
	captureOutputFn func(string) (string, error)
	createCalls     []createCall
	deliverCalls    []string
	handleAskCalls  []string
	recoverCalled   atomic.Bool
}

type createCall struct {
	workingDir string
	model      string
	prompt     string
}

func (m *mockMgr) Create(workingDir, model, prompt string) (*storage.Session, error) {
	m.createCalls = append(m.createCalls, createCall{workingDir, model, prompt})
	if m.createFn != nil {
		return m.createFn(workingDir, model, prompt)
	}
	return &storage.Session{ID: "test-id", Status: "active"}, nil
}

func (m *mockMgr) DeliverNext(sessionID string) error {
	m.deliverCalls = append(m.deliverCalls, sessionID)
	if m.deliverFn != nil {
		return m.deliverFn(sessionID)
	}
	return nil
}

func (m *mockMgr) ListActive() ([]*storage.Session, error) {
	if m.listActiveFn != nil {
		return m.listActiveFn()
	}
	return nil, nil
}

func (m *mockMgr) RecoverAll() error {
	m.recoverCalled.Store(true)
	if m.recoverAllFn != nil {
		return m.recoverAllFn()
	}
	return nil
}

func (m *mockMgr) HandleAsk(sessionID string) error {
	m.handleAskCalls = append(m.handleAskCalls, sessionID)
	if m.handleAskFn != nil {
		return m.handleAskFn(sessionID)
	}
	return nil
}

func (m *mockMgr) CaptureOutput(sessionID string) (string, error) {
	if m.captureOutputFn != nil {
		return m.captureOutputFn(sessionID)
	}
	return "", nil
}

type mockMail struct {
	pollFn         func() ([]*email.IncomingMessage, error)
	flushFn        func() error
	sendTemplateFn func() (string, error)
	pollCount      atomic.Int32
	flushCount     atomic.Int32
}

func (m *mockMail) Poll() ([]*email.IncomingMessage, error) {
	m.pollCount.Add(1)
	if m.pollFn != nil {
		return m.pollFn()
	}
	return nil, nil
}

func (m *mockMail) FlushOutbox() error {
	m.flushCount.Add(1)
	if m.flushFn != nil {
		return m.flushFn()
	}
	return nil
}

func (m *mockMail) SendTemplate() (string, error) {
	if m.sendTemplateFn != nil {
		return m.sendTemplateFn()
	}
	return "<test-template@claude-postman>", nil
}

// --- Helpers ---

func newTestStore(t *testing.T) *storage.Store {
	t.Helper()
	store, err := storage.New(t.TempDir())
	require.NoError(t, err)
	t.Cleanup(func() { store.Close() })
	require.NoError(t, store.Migrate())
	return store
}

func newTestServer(t *testing.T) (*server, *mockMgr, *mockMail) {
	t.Helper()
	store := newTestStore(t)
	mgr := &mockMgr{}
	ml := &mockMail{}
	cfg := &config.Config{
		General: config.GeneralConfig{
			DefaultModel:    "sonnet",
			PollIntervalSec: 30,
		},
	}
	s := &server{
		cfg:          cfg,
		store:        store,
		mgr:          mgr,
		mailer:       ml,
		pollInterval: 50 * time.Millisecond,
	}
	return s, mgr, ml
}

func insertSession(t *testing.T, store *storage.Store, id, status string) {
	t.Helper()
	require.NoError(t, store.CreateSession(&storage.Session{
		ID:         id,
		TmuxName:   "session-" + id,
		WorkingDir: "/tmp",
		Model:      "sonnet",
		Status:     status,
	}))
}

// --- Tests ---

func TestRunServe_StartsAndStops(t *testing.T) {
	s, _, _ := newTestServer(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	err := s.run(ctx)
	assert.NoError(t, err)
}

func TestRunServe_RecoversOnStart(t *testing.T) {
	s, mgr, _ := newTestServer(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_ = s.run(ctx)
	assert.True(t, mgr.recoverCalled.Load(), "RecoverAll should be called on startup")
}

func TestProcessMessages_NewSession(t *testing.T) {
	t.Run("creates session with prompt as CLI argument", func(t *testing.T) {
		s, mgr, _ := newTestServer(t)

		msgs := []*email.IncomingMessage{
			{
				IsNewSession: true,
				WorkingDir:   "/home/user/project",
				Model:        "opus",
				Body:         "Build a feature",
			},
		}

		err := s.processMessages(msgs)
		require.NoError(t, err)

		// Create called with correct params including prompt
		require.Len(t, mgr.createCalls, 1)
		assert.Equal(t, "/home/user/project", mgr.createCalls[0].workingDir)
		assert.Equal(t, "opus", mgr.createCalls[0].model)
		assert.Equal(t, "Build a feature", mgr.createCalls[0].prompt)

		// No enqueue or DeliverNext — prompt passed directly to Create
		assert.Empty(t, mgr.deliverCalls)
	})

	t.Run("uses default model when empty", func(t *testing.T) {
		s, mgr, _ := newTestServer(t)

		msgs := []*email.IncomingMessage{
			{IsNewSession: true, WorkingDir: "/tmp", Model: "", Body: "task"},
		}
		require.NoError(t, s.processMessages(msgs))
		require.Len(t, mgr.createCalls, 1)
		assert.Equal(t, "sonnet", mgr.createCalls[0].model)
	})

	t.Run("uses home dir when workingDir empty", func(t *testing.T) {
		s, mgr, _ := newTestServer(t)

		msgs := []*email.IncomingMessage{
			{IsNewSession: true, WorkingDir: "", Model: "opus", Body: "task"},
		}
		require.NoError(t, s.processMessages(msgs))

		home, _ := os.UserHomeDir()
		require.Len(t, mgr.createCalls, 1)
		assert.Equal(t, home, mgr.createCalls[0].workingDir)
	})
}

func TestProcessMessages_NewSession_TildeExpansion(t *testing.T) {
	home, err := os.UserHomeDir()
	require.NoError(t, err)

	t.Run("expands ~/path to absolute", func(t *testing.T) {
		s, mgr, _ := newTestServer(t)
		msgs := []*email.IncomingMessage{
			{IsNewSession: true, WorkingDir: "~/myproject", Model: "opus", Body: "task"},
		}
		require.NoError(t, s.processMessages(msgs))
		require.Len(t, mgr.createCalls, 1)
		assert.Equal(t, filepath.Join(home, "myproject"), mgr.createCalls[0].workingDir)
	})

	t.Run("expands bare ~ to home dir", func(t *testing.T) {
		s, mgr, _ := newTestServer(t)
		msgs := []*email.IncomingMessage{
			{IsNewSession: true, WorkingDir: "~", Model: "opus", Body: "task"},
		}
		require.NoError(t, s.processMessages(msgs))
		require.Len(t, mgr.createCalls, 1)
		assert.Equal(t, home, mgr.createCalls[0].workingDir)
	})

	t.Run("leaves absolute path unchanged", func(t *testing.T) {
		s, mgr, _ := newTestServer(t)
		msgs := []*email.IncomingMessage{
			{IsNewSession: true, WorkingDir: "/abs/path", Model: "opus", Body: "task"},
		}
		require.NoError(t, s.processMessages(msgs))
		require.Len(t, mgr.createCalls, 1)
		assert.Equal(t, "/abs/path", mgr.createCalls[0].workingDir)
	})
}

func TestProcessMessages_ExistingSession(t *testing.T) {
	s, mgr, _ := newTestServer(t)

	sessionID := "existing-session-001"
	insertSession(t, s.store, sessionID, "active")

	msgs := []*email.IncomingMessage{
		{SessionID: sessionID, Body: "Continue working"},
	}

	err := s.processMessages(msgs)
	require.NoError(t, err)

	// No Create calls
	assert.Empty(t, mgr.createCalls)

	// Message enqueued
	msg, err := s.store.DequeueMessage(sessionID)
	require.NoError(t, err)
	require.NotNil(t, msg)
	assert.Equal(t, "Continue working", msg.Body)
}

func TestPollLoop_ContinuesOnError(t *testing.T) {
	s, _, ml := newTestServer(t)

	pollCh := make(chan struct{}, 10)
	ml.pollFn = func() ([]*email.IncomingMessage, error) {
		pollCh <- struct{}{}
		return nil, errors.New("imap connection failed")
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errCh := make(chan error, 1)
	go func() {
		errCh <- s.run(ctx)
	}()

	// Wait for at least 2 polls (proves loop continues after error)
	<-pollCh
	<-pollCh
	cancel()

	err := <-errCh
	assert.NoError(t, err, "loop should not crash on IMAP errors")
}

func TestCheckIdleSessions(t *testing.T) {
	s, mgr, _ := newTestServer(t)

	mgr.listActiveFn = func() ([]*storage.Session, error) {
		return []*storage.Session{
			{ID: "idle-1", Status: "idle"},
			{ID: "active-1", Status: "active"},
			{ID: "idle-2", Status: "idle"},
		}, nil
	}

	err := s.checkIdleSessions()
	require.NoError(t, err)

	// Only idle sessions should get DeliverNext
	assert.Len(t, mgr.deliverCalls, 2)
	assert.Contains(t, mgr.deliverCalls, "idle-1")
	assert.Contains(t, mgr.deliverCalls, "idle-2")
}

func TestCheckIdleSessions_IncludesWaiting(t *testing.T) {
	s, mgr, _ := newTestServer(t)

	mgr.listActiveFn = func() ([]*storage.Session, error) {
		return []*storage.Session{
			{ID: "idle-1", Status: "idle"},
			{ID: "waiting-1", Status: "waiting"},
			{ID: "active-1", Status: "active"},
		}, nil
	}

	err := s.checkIdleSessions()
	require.NoError(t, err)

	assert.Len(t, mgr.deliverCalls, 2)
	assert.Contains(t, mgr.deliverCalls, "idle-1")
	assert.Contains(t, mgr.deliverCalls, "waiting-1")
}

func TestCheckWaitingPrompts_DetectsPrompt(t *testing.T) {
	s, mgr, _ := newTestServer(t)

	mgr.listActiveFn = func() ([]*storage.Session, error) {
		return []*storage.Session{
			{ID: "active-1", Status: "active"},
		}, nil
	}
	mgr.captureOutputFn = func(_ string) (string, error) {
		return "Choose:\n1. A\n2. B\n❯ ", nil
	}

	err := s.checkWaitingPrompts()
	require.NoError(t, err)

	assert.Len(t, mgr.handleAskCalls, 1)
	assert.Equal(t, "active-1", mgr.handleAskCalls[0])
}

func TestCheckWaitingPrompts_SkipsNonActive(t *testing.T) {
	s, mgr, _ := newTestServer(t)

	mgr.listActiveFn = func() ([]*storage.Session, error) {
		return []*storage.Session{
			{ID: "idle-1", Status: "idle"},
			{ID: "waiting-1", Status: "waiting"},
		}, nil
	}

	err := s.checkWaitingPrompts()
	require.NoError(t, err)

	assert.Empty(t, mgr.handleAskCalls)
}

func TestCheckWaitingPrompts_NoPromptNoAction(t *testing.T) {
	s, mgr, _ := newTestServer(t)

	mgr.listActiveFn = func() ([]*storage.Session, error) {
		return []*storage.Session{
			{ID: "active-1", Status: "active"},
		}, nil
	}
	mgr.captureOutputFn = func(_ string) (string, error) {
		return "● Thinking about the problem...", nil
	}

	err := s.checkWaitingPrompts()
	require.NoError(t, err)

	assert.Empty(t, mgr.handleAskCalls)
}
