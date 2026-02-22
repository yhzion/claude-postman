package serve

import (
	"context"
	"errors"
	"os"
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
	createFn      func(string, string) (*storage.Session, error)
	deliverFn     func(string) error
	listActiveFn  func() ([]*storage.Session, error)
	recoverAllFn  func() error
	createCalls   []createCall
	deliverCalls  []string
	recoverCalled atomic.Bool
}

type createCall struct {
	workingDir string
	model      string
}

func (m *mockMgr) Create(workingDir, model string) (*storage.Session, error) {
	m.createCalls = append(m.createCalls, createCall{workingDir, model})
	if m.createFn != nil {
		return m.createFn(workingDir, model)
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
	t.Run("creates session and delivers initial prompt", func(t *testing.T) {
		s, mgr, _ := newTestServer(t)

		sessionID := "new-session-001"
		mgr.createFn = func(workingDir, model string) (*storage.Session, error) {
			session := &storage.Session{
				ID:         sessionID,
				TmuxName:   "session-" + sessionID,
				WorkingDir: workingDir,
				Model:      model,
				Status:     "active",
			}
			require.NoError(t, s.store.CreateSession(session))
			return session, nil
		}

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

		// Create called with correct params
		require.Len(t, mgr.createCalls, 1)
		assert.Equal(t, "/home/user/project", mgr.createCalls[0].workingDir)
		assert.Equal(t, "opus", mgr.createCalls[0].model)

		// Message enqueued
		msg, err := s.store.DequeueMessage(sessionID)
		require.NoError(t, err)
		require.NotNil(t, msg)
		assert.Equal(t, "Build a feature", msg.Body)

		// DeliverNext called
		require.Len(t, mgr.deliverCalls, 1)
		assert.Equal(t, sessionID, mgr.deliverCalls[0])
	})

	t.Run("uses default model when empty", func(t *testing.T) {
		s, mgr, _ := newTestServer(t)

		mgr.createFn = func(workingDir, model string) (*storage.Session, error) {
			session := &storage.Session{
				ID:         "default-model-session",
				TmuxName:   "session-default-model",
				WorkingDir: workingDir,
				Model:      model,
				Status:     "active",
			}
			require.NoError(t, s.store.CreateSession(session))
			return session, nil
		}

		msgs := []*email.IncomingMessage{
			{IsNewSession: true, WorkingDir: "/tmp", Model: "", Body: "task"},
		}

		err := s.processMessages(msgs)
		require.NoError(t, err)

		require.Len(t, mgr.createCalls, 1)
		assert.Equal(t, "sonnet", mgr.createCalls[0].model)
	})

	t.Run("uses home dir when workingDir empty", func(t *testing.T) {
		s, mgr, _ := newTestServer(t)

		mgr.createFn = func(workingDir, model string) (*storage.Session, error) {
			session := &storage.Session{
				ID:         "default-dir-session",
				TmuxName:   "session-default-dir",
				WorkingDir: workingDir,
				Model:      model,
				Status:     "active",
			}
			require.NoError(t, s.store.CreateSession(session))
			return session, nil
		}

		msgs := []*email.IncomingMessage{
			{IsNewSession: true, WorkingDir: "", Model: "opus", Body: "task"},
		}

		err := s.processMessages(msgs)
		require.NoError(t, err)

		home, _ := os.UserHomeDir()
		require.Len(t, mgr.createCalls, 1)
		assert.Equal(t, home, mgr.createCalls[0].workingDir)
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
