# Tilde Expansion + FIFO Goroutine + Prompt Detection Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Fix tilde expansion bug, implement missing FIFO listener goroutine, and add prompt-waiting detection so Claude Code's questions are forwarded as emails.

**Architecture:** Single FIFO per session handles DONE/ASK/SHUTDOWN signals via a blocking-read goroutine spawned by Manager.Create(). A fallback pattern detector in serve.pollLoop checks for Claude Code's `❯` prompt as safety net. New session status `"waiting"` enables user reply routing.

**Tech Stack:** Go 1.24+, SQLite, tmux, Unix FIFO (named pipe), testify

---

## Dependencies

```
Task 1 (tilde)     ─── independent
Task 2 (detect.go) ─── independent
Task 3 (HandleAsk) ─── independent (uses existing mock pattern)
Task 4 (DeliverNext waiting) ─── depends on Task 3 (needs "waiting" status concept)
Task 5 (listenFIFO) ─── depends on Task 3 (calls HandleAsk/HandleDone)
Task 6 (system prompt) ─── depends on Task 5 (prompt references FIFO signals)
Task 7 (ListActive waiting) ─── depends on Task 4
Task 8 (serve: tilde + checkWaitingPrompts) ─── depends on Task 1, 2, 7
Task 9 (E2E) ─── depends on all above
```

Parallelizable: Tasks 1, 2, 3 can run concurrently. Tasks 4, 5, 6 are sequential. Tasks 7, 8 are sequential after 4. Task 9 is final.

---

### Task 1: Tilde Expansion in serve.go

**Files:**
- Modify: `internal/serve/serve.go:156-176`
- Modify: `internal/serve/serve_test.go`

**Step 1: Write failing tests**

Add to `internal/serve/serve_test.go`:

```go
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
```

**Step 2: Run tests to verify they fail**

Run: `go test ./internal/serve/ -run TestProcessMessages_NewSession_TildeExpansion -v`
Expected: FAIL — `~/myproject` passed as-is, not expanded.

**Step 3: Implement tilde expansion**

In `internal/serve/serve.go`, add `"path/filepath"` and `"strings"` to imports. Then modify `handleNewSession`:

```go
func (s *server) handleNewSession(msg *email.IncomingMessage) error {
	model := msg.Model
	if model == "" {
		model = s.cfg.General.DefaultModel
	}

	workingDir := msg.WorkingDir
	if workingDir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("get home dir: %w", err)
		}
		workingDir = home
	} else if strings.HasPrefix(workingDir, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("expand home dir: %w", err)
		}
		workingDir = filepath.Join(home, workingDir[2:])
	} else if workingDir == "~" {
		home, err := os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("expand home dir: %w", err)
		}
		workingDir = home
	}

	if _, err := s.mgr.Create(workingDir, model, msg.Body); err != nil {
		return fmt.Errorf("create session: %w", err)
	}

	return nil
}
```

**Step 4: Run tests to verify they pass**

Run: `go test ./internal/serve/ -run TestProcessMessages_NewSession_TildeExpansion -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/serve/serve.go internal/serve/serve_test.go
git commit -m "fix: expand tilde in Directory path before creating tmux session"
```

---

### Task 2: HasInputPrompt detector

**Files:**
- Create: `internal/session/detect.go`
- Create: `internal/session/detect_test.go`

**Step 1: Write failing tests**

Create `internal/session/detect_test.go`:

```go
package session

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestHasInputPrompt(t *testing.T) {
	t.Run("detects prompt on last line", func(t *testing.T) {
		output := "Some output\n❯ "
		assert.True(t, HasInputPrompt(output))
	})

	t.Run("detects prompt within last 5 lines", func(t *testing.T) {
		output := "line1\nline2\nline3\n❯ \n\n"
		assert.True(t, HasInputPrompt(output))
	})

	t.Run("no prompt when thinking", func(t *testing.T) {
		output := "● Thinking about the problem...\n  Still working..."
		assert.False(t, HasInputPrompt(output))
	})

	t.Run("no prompt in middle of output", func(t *testing.T) {
		output := "❯ old prompt\nNow doing work\nMore output\nStill going\nAlmost done\nFinishing up"
		assert.False(t, HasInputPrompt(output))
	})

	t.Run("empty output returns false", func(t *testing.T) {
		assert.False(t, HasInputPrompt(""))
	})

	t.Run("only whitespace returns false", func(t *testing.T) {
		assert.False(t, HasInputPrompt("   \n  \n  "))
	})

	t.Run("detects prompt with text after it", func(t *testing.T) {
		output := "Choose a project:\n1. A\n2. B\n❯ waiting for input"
		assert.True(t, HasInputPrompt(output))
	})
}
```

**Step 2: Run tests to verify they fail**

Run: `go test ./internal/session/ -run TestHasInputPrompt -v`
Expected: FAIL — `HasInputPrompt` undefined.

**Step 3: Implement detector**

Create `internal/session/detect.go`:

```go
package session

import "strings"

// HasInputPrompt checks the last 5 lines of tmux output for Claude Code's
// input prompt indicator (❯). Returns true if Claude Code is waiting for input.
func HasInputPrompt(output string) bool {
	output = strings.TrimRight(output, " \t\n")
	if output == "" {
		return false
	}
	lines := strings.Split(output, "\n")
	start := len(lines) - 5
	if start < 0 {
		start = 0
	}
	for i := start; i < len(lines); i++ {
		if strings.Contains(lines[i], "❯") {
			return true
		}
	}
	return false
}
```

**Step 4: Run tests to verify they pass**

Run: `go test ./internal/session/ -run TestHasInputPrompt -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/session/detect.go internal/session/detect_test.go
git commit -m "feat: add HasInputPrompt detector for Claude Code prompt"
```

---

### Task 3: HandleAsk

**Files:**
- Modify: `internal/session/fifo.go`
- Modify: `internal/session/session_test.go`

**Step 1: Write failing test**

Add to `internal/session/session_test.go`:

```go
func TestHandleAsk_TransitionsToWaiting(t *testing.T) {
	mgr, mock := newTestManager(t)
	createTestSession(t, mgr, "ask-1", "active")
	mock.captured = "어느 프로젝트를 분석할까요?\n1. A\n2. B\n❯ "

	err := mgr.HandleAsk("ask-1")
	require.NoError(t, err)

	got, err := mgr.Get("ask-1")
	require.NoError(t, err)
	assert.Equal(t, "waiting", got.Status)
	require.NotNil(t, got.LastResult)
	assert.Contains(t, *got.LastResult, "어느 프로젝트를 분석할까요?")

	outbox, err := mgr.store.GetPendingOutbox()
	require.NoError(t, err)
	require.Len(t, outbox, 1)
	assert.Equal(t, "ask-1", outbox[0].SessionID)
	assert.Contains(t, outbox[0].Body, "어느 프로젝트를 분석할까요?")
}

func TestHandleAsk_NotFound(t *testing.T) {
	mgr, _ := newTestManager(t)

	err := mgr.HandleAsk("nonexistent")
	assert.ErrorIs(t, err, ErrSessionNotFound)
}
```

**Step 2: Run tests to verify they fail**

Run: `go test ./internal/session/ -run TestHandleAsk -v`
Expected: FAIL — `HandleAsk` undefined.

**Step 3: Implement HandleAsk**

Add to `internal/session/fifo.go`:

```go
// handleAskTx runs the transactional part of HandleAsk:
// saves result, creates outbox, and sets status to "waiting".
func (m *Manager) handleAskTx(session *storage.Session, output string) error {
	return m.store.Tx(context.Background(), func(tx *storage.Store) error {
		session.LastResult = &output

		outbox := &storage.OutboxMessage{
			ID:        uuid.New().String(),
			SessionID: session.ID,
			Subject:   "Claude Code is waiting for your input",
			Body:      output,
			Status:    "pending",
		}
		if txErr := tx.CreateOutbox(outbox); txErr != nil {
			return txErr
		}

		session.Status = "waiting"
		return tx.UpdateSession(session)
	})
}

// HandleAsk processes an ASK signal from a session's FIFO.
// Captures tmux output, creates outbox email, and sets status to "waiting".
func (m *Manager) HandleAsk(sessionID string) error {
	time.Sleep(m.captureDelay)

	session, err := m.store.GetSession(sessionID)
	if err != nil {
		return ErrSessionNotFound
	}

	output, err := m.tmux.CapturePane(session.TmuxName, capturePaneLines)
	if err != nil {
		return fmt.Errorf("capture-pane: %w", err)
	}

	return m.handleAskTx(session, output)
}
```

**Step 4: Run tests to verify they pass**

Run: `go test ./internal/session/ -run TestHandleAsk -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/session/fifo.go internal/session/session_test.go
git commit -m "feat: add HandleAsk for prompt-waiting detection"
```

---

### Task 4: DeliverNext accepts "waiting" status

**Files:**
- Modify: `internal/session/session.go:146-182`
- Modify: `internal/session/session_test.go`

**Step 1: Write failing test**

Add to `internal/session/session_test.go`:

```go
func TestDeliverNext_WaitingWithMessage(t *testing.T) {
	mgr, mock := newTestManager(t)
	createTestSession(t, mgr, "waiting-1", "waiting")
	mock.sessions["session-waiting-1"] = true

	msg := &storage.InboxMessage{
		ID:        "reply-1",
		SessionID: "waiting-1",
		Body:      "3번 선택",
	}
	require.NoError(t, mgr.store.EnqueueMessage(msg))

	err := mgr.DeliverNext("waiting-1")
	require.NoError(t, err)

	got, err := mgr.Get("waiting-1")
	require.NoError(t, err)
	assert.Equal(t, "active", got.Status)
	require.NotNil(t, got.LastPrompt)
	assert.Equal(t, "3번 선택", *got.LastPrompt)

	require.Len(t, mock.sentKeys, 1)
	assert.Equal(t, "3번 선택", mock.sentKeys[0].text)
}
```

**Step 2: Run tests to verify they fail**

Run: `go test ./internal/session/ -run TestDeliverNext_WaitingWithMessage -v`
Expected: FAIL — `ErrSessionNotIdle` because status is "waiting", not "idle".

**Step 3: Modify DeliverNext**

In `internal/session/session.go`, update the status check:

```go
func (m *Manager) DeliverNext(sessionID string) error {
	session, err := m.store.GetSession(sessionID)
	if err != nil {
		return ErrSessionNotFound
	}
	if session.Status != "idle" && session.Status != "waiting" {
		return ErrSessionNotIdle
	}
	// ... rest unchanged
```

**Step 4: Run all DeliverNext tests**

Run: `go test ./internal/session/ -run TestDeliverNext -v`
Expected: ALL PASS (existing tests + new waiting test).

**Step 5: Commit**

```bash
git add internal/session/session.go internal/session/session_test.go
git commit -m "feat: allow DeliverNext on waiting sessions for user replies"
```

---

### Task 5: listenFIFO goroutine

**Files:**
- Modify: `internal/session/fifo.go`
- Modify: `internal/session/session.go:84-123` (Create)
- Modify: `internal/session/session.go:197-229` (RecoverAll)
- Modify: `internal/session/session_test.go`

**Step 1: Write failing test**

Add to `internal/session/session_test.go`:

```go
func TestListenFIFO_HandlesDoneSignal(t *testing.T) {
	mgr, mock := newTestManager(t)
	createTestSession(t, mgr, "fifo-done", "active")
	mock.captured = "작업 완료"

	require.NoError(t, mgr.createFIFO("fifo-done"))

	go mgr.listenFIFO("fifo-done")

	// Write DONE signal to FIFO
	err := os.WriteFile(mgr.fifoPath("fifo-done"), []byte("DONE:fifo-done\n"), 0o600)
	// Note: WriteFile won't work on FIFO. Use os.OpenFile + Write instead.
	f, err := os.OpenFile(mgr.fifoPath("fifo-done"), os.O_WRONLY, 0)
	require.NoError(t, err)
	_, err = f.WriteString("DONE:fifo-done\n")
	require.NoError(t, err)
	f.Close()

	// Wait for processing
	time.Sleep(100 * time.Millisecond)

	got, err := mgr.Get("fifo-done")
	require.NoError(t, err)
	assert.Equal(t, "idle", got.Status)
}

func TestListenFIFO_HandlesAskSignal(t *testing.T) {
	mgr, mock := newTestManager(t)
	createTestSession(t, mgr, "fifo-ask", "active")
	mock.captured = "질문입니다\n❯ "

	require.NoError(t, mgr.createFIFO("fifo-ask"))

	go mgr.listenFIFO("fifo-ask")

	f, err := os.OpenFile(mgr.fifoPath("fifo-ask"), os.O_WRONLY, 0)
	require.NoError(t, err)
	_, err = f.WriteString("ASK:fifo-ask\n")
	require.NoError(t, err)
	f.Close()

	time.Sleep(100 * time.Millisecond)

	got, err := mgr.Get("fifo-ask")
	require.NoError(t, err)
	assert.Equal(t, "waiting", got.Status)
}

func TestListenFIFO_ShutdownExits(t *testing.T) {
	mgr, _ := newTestManager(t)
	createTestSession(t, mgr, "fifo-shut", "active")

	require.NoError(t, mgr.createFIFO("fifo-shut"))

	done := make(chan struct{})
	go func() {
		mgr.listenFIFO("fifo-shut")
		close(done)
	}()

	f, err := os.OpenFile(mgr.fifoPath("fifo-shut"), os.O_WRONLY, 0)
	require.NoError(t, err)
	_, err = f.WriteString("SHUTDOWN\n")
	require.NoError(t, err)
	f.Close()

	select {
	case <-done:
		// OK: goroutine exited
	case <-time.After(2 * time.Second):
		t.Fatal("listenFIFO did not exit on SHUTDOWN")
	}
}
```

**Step 2: Run tests to verify they fail**

Run: `go test ./internal/session/ -run TestListenFIFO -v`
Expected: FAIL — `listenFIFO` undefined.

**Step 3: Implement listenFIFO**

Add to `internal/session/fifo.go` (add `"bufio"`, `"log/slog"` to imports):

```go
// listenFIFO blocks reading the session's FIFO for DONE/ASK/SHUTDOWN signals.
// It re-opens the FIFO after each writer EOF to wait for the next signal.
// Exits on SHUTDOWN sentinel or FIFO removal.
func (m *Manager) listenFIFO(sessionID string) {
	path := m.fifoPath(sessionID)
	for {
		f, err := os.OpenFile(path, os.O_RDONLY, 0)
		if err != nil {
			slog.Debug("fifo open failed, exiting listener", "session_id", sessionID, "error", err)
			return
		}

		scanner := bufio.NewScanner(f)
		for scanner.Scan() {
			line := scanner.Text()
			switch {
			case strings.HasPrefix(line, "DONE:"):
				if err := m.HandleDone(sessionID); err != nil {
					slog.Error("HandleDone failed", "session_id", sessionID, "error", err)
				}
			case strings.HasPrefix(line, "ASK:"):
				if err := m.HandleAsk(sessionID); err != nil {
					slog.Error("HandleAsk failed", "session_id", sessionID, "error", err)
				}
			case line == "SHUTDOWN":
				f.Close()
				return
			default:
				slog.Warn("unknown FIFO signal", "session_id", sessionID, "line", line)
			}
		}
		f.Close()
	}
}
```

Add `"bufio"`, `"log/slog"`, `"strings"` to fifo.go imports.

**Step 4: Spawn goroutine in Create and RecoverAll**

In `internal/session/session.go`, add `go m.listenFIFO(id)` at end of `Create()` (before return):

```go
	// ... existing code ...
	session.Status = "active"
	if err := m.store.UpdateSession(session); err != nil {
		return nil, fmt.Errorf("update session status: %w", err)
	}

	go m.listenFIFO(id)

	return session, nil
}
```

In `RecoverAll()`, add `go m.listenFIFO(session.ID)` after successful recovery:

```go
		cmd := m.claudeResumeCommand(session.ID, session.Model)
		if sendErr := m.tmux.SendKeys(session.TmuxName, cmd); sendErr != nil {
			session.Status = "ended"
			_ = m.store.UpdateSession(session)
			continue
		}

		go m.listenFIFO(session.ID)
	}
```

**Step 5: Run all session tests**

Run: `go test ./internal/session/ -v`
Expected: ALL PASS

**Step 6: Commit**

```bash
git add internal/session/fifo.go internal/session/session.go internal/session/session_test.go
git commit -m "feat: add listenFIFO goroutine for DONE/ASK/SHUTDOWN signal handling"
```

---

### Task 6: Update system prompt with ASK instruction

**Files:**
- Modify: `internal/session/session.go:21-30`
- Modify: `internal/session/session_test.go`

**Step 1: Write failing test**

Add to `internal/session/session_test.go`:

```go
func TestClaudeCommand_ContainsAskInstruction(t *testing.T) {
	mgr, _ := newTestManager(t)
	cmd := mgr.claudeCommand("test-id", "sonnet", "/tmp/prompt")
	assert.Contains(t, cmd, "ASK:test-id")
	assert.Contains(t, cmd, "DONE:test-id")
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/session/ -run TestClaudeCommand_ContainsAskInstruction -v`
Expected: FAIL — current prompt doesn't contain "ASK".

**Step 3: Update systemPromptTemplate**

In `internal/session/session.go`, replace lines 21-30:

```go
const systemPromptTemplate = `작업이 완료되면 반드시 다음 명령을 실행하세요:
echo 'DONE:%s' > /tmp/claude-postman/%s.fifo

사용자에게 질문하거나 선택을 요청할 때는 반드시 다음 명령을 먼저 실행하세요:
echo 'ASK:%s' > /tmp/claude-postman/%s.fifo
그리고 사용자의 답변을 기다리세요.

최종 응답에는 반드시 다음을 포함하세요:
- 작업 과정 요약
- 결과
- 변경된 파일 목록 (있는 경우)

어떤 방법으로든 작업을 완수하세요. 최소 10번 시도하세요.
포기하지 마세요.`
```

Update `claudeCommand` to pass 4 format args (sessionID x4):

```go
func (m *Manager) claudeCommand(sessionID, model, promptFile string) string {
	sysPrompt := fmt.Sprintf(systemPromptTemplate, sessionID, sessionID, sessionID, sessionID)
	return fmt.Sprintf("claude --dangerously-skip-permissions --session-id %s --system-prompt '%s' --model %s \"$(cat %s)\"",
		sessionID, sysPrompt, model, promptFile)
}
```

**Step 4: Run tests**

Run: `go test ./internal/session/ -v`
Expected: ALL PASS

**Step 5: Commit**

```bash
git add internal/session/session.go internal/session/session_test.go
git commit -m "feat: add ASK signal instruction to Claude Code system prompt"
```

---

### Task 7: ListActive includes "waiting" status

**Files:**
- Modify: `internal/session/session.go:189-192`
- Modify: `internal/session/session_test.go`

**Step 1: Write failing test**

Add to `internal/session/session_test.go`:

```go
func TestListActive_IncludesWaiting(t *testing.T) {
	mgr, _ := newTestManager(t)

	createTestSession(t, mgr, "active-1", "active")
	createTestSession(t, mgr, "waiting-1", "waiting")
	createTestSession(t, mgr, "idle-1", "idle")
	createTestSession(t, mgr, "ended-1", "ended")

	active, err := mgr.ListActive()
	require.NoError(t, err)
	assert.Len(t, active, 3, "active + waiting + idle = 3")

	ids := make(map[string]bool)
	for _, s := range active {
		ids[s.ID] = true
	}
	assert.True(t, ids["active-1"])
	assert.True(t, ids["waiting-1"])
	assert.True(t, ids["idle-1"])
	assert.False(t, ids["ended-1"])
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/session/ -run TestListActive_IncludesWaiting -v`
Expected: FAIL — "waiting" not included (only 2 returned).

**Step 3: Update ListActive**

In `internal/session/session.go`:

```go
func (m *Manager) ListActive() ([]*storage.Session, error) {
	return m.store.ListSessionsByStatus("creating", "active", "idle", "waiting")
}
```

**Step 4: Run tests**

Run: `go test ./internal/session/ -run TestListActive -v`
Expected: ALL PASS

**Step 5: Commit**

```bash
git add internal/session/session.go internal/session/session_test.go
git commit -m "feat: include waiting sessions in ListActive"
```

---

### Task 8: serve.go — checkWaitingPrompts + checkIdleSessions expansion

**Files:**
- Modify: `internal/serve/serve.go`
- Modify: `internal/serve/serve_test.go`

**Step 1: Update sessionMgr interface and mock**

In `internal/serve/serve.go`, add `HandleAsk` and `CaptureOutput` to interface:

```go
type sessionMgr interface {
	Create(workingDir, model, prompt string) (*storage.Session, error)
	DeliverNext(sessionID string) error
	ListActive() ([]*storage.Session, error)
	RecoverAll() error
	HandleAsk(sessionID string) error
	CaptureOutput(sessionID string) (string, error)
}
```

In `internal/session/session.go`, add `CaptureOutput` method:

```go
// CaptureOutput captures the current tmux pane output for a session.
func (m *Manager) CaptureOutput(sessionID string) (string, error) {
	session, err := m.store.GetSession(sessionID)
	if err != nil {
		return "", ErrSessionNotFound
	}
	return m.tmux.CapturePane(session.TmuxName, capturePaneLines)
}
```

In `internal/serve/serve_test.go`, update mockMgr:

```go
type mockMgr struct {
	createFn       func(string, string, string) (*storage.Session, error)
	deliverFn      func(string) error
	listActiveFn   func() ([]*storage.Session, error)
	recoverAllFn   func() error
	handleAskFn    func(string) error
	captureOutputFn func(string) (string, error)
	createCalls    []createCall
	deliverCalls   []string
	handleAskCalls []string
	recoverCalled  atomic.Bool
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
```

**Step 2: Write failing tests**

Add to `internal/serve/serve_test.go`:

```go
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
```

**Step 3: Run tests to verify they fail**

Run: `go test ./internal/serve/ -run "TestCheckIdleSessions_IncludesWaiting|TestCheckWaitingPrompts" -v`
Expected: FAIL

**Step 4: Implement checkWaitingPrompts and update checkIdleSessions**

In `internal/serve/serve.go`:

```go
func (s *server) checkIdleSessions() error {
	sessions, err := s.mgr.ListActive()
	if err != nil {
		return err
	}

	for _, sess := range sessions {
		if sess.Status == "idle" || sess.Status == "waiting" {
			if err := s.mgr.DeliverNext(sess.ID); err != nil {
				slog.Warn("failed to deliver to session", "session_id", sess.ID, "error", err)
			}
		}
	}

	return nil
}

func (s *server) checkWaitingPrompts() error {
	sessions, err := s.mgr.ListActive()
	if err != nil {
		return err
	}

	for _, sess := range sessions {
		if sess.Status != "active" {
			continue
		}
		output, err := s.mgr.CaptureOutput(sess.ID)
		if err != nil {
			slog.Warn("failed to capture output", "session_id", sess.ID, "error", err)
			continue
		}
		if session.HasInputPrompt(output) {
			if err := s.mgr.HandleAsk(sess.ID); err != nil {
				slog.Warn("fallback HandleAsk failed", "session_id", sess.ID, "error", err)
			}
		}
	}

	return nil
}
```

Add to `pollLoop`, after `checkIdleSessions`:

```go
		if err := s.checkWaitingPrompts(); err != nil {
			slog.Error("check waiting prompts failed", "error", err)
		}
```

**Step 5: Run all serve tests**

Run: `go test ./internal/serve/ -v`
Expected: ALL PASS

**Step 6: Commit**

```bash
git add internal/serve/serve.go internal/serve/serve_test.go internal/session/session.go
git commit -m "feat: add prompt detection fallback and waiting session delivery in serve loop"
```

---

### Task 9: E2E Test

**Files:**
- Create: `_test/integration/e2e_prompt_test.go`

**Step 1: Create test helper script**

The E2E test creates a temporary shell script that simulates Claude Code behavior.

**Step 2: Write E2E test**

Create `_test/integration/e2e_prompt_test.go`:

```go
//go:build integration

package integration

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yhzion/claude-postman/internal/session"
	"github.com/yhzion/claude-postman/internal/storage"
)

// fakeClaudeScript returns a shell script that simulates Claude Code:
// 1. Prints a question
// 2. Sends ASK signal to FIFO
// 3. Reads user input from stdin
// 4. Prints result with the input
// 5. Sends DONE signal to FIFO
func fakeClaudeScript(fifoPath string) string {
	return `#!/bin/bash
echo "분석할 코드베이스를 선택해주세요:"
echo "1. project-a"
echo "2. project-b"
echo "❯ "
echo "ASK:$1" > ` + fifoPath + `
read -r answer
echo "선택: $answer"
echo "분석 결과입니다."
echo "DONE:$1" > ` + fifoPath + `
`
}

func TestE2E_AskSignalAndReply(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping E2E test in short mode")
	}

	// Setup
	tmpDir := t.TempDir()
	store, err := storage.New(tmpDir)
	require.NoError(t, err)
	defer store.Close()
	require.NoError(t, store.Migrate())

	tmuxRunner := session.NewTmuxRunner()
	mgr := session.New(store, tmuxRunner)

	sessionID := "e2e-test-ask"
	tmuxName := "session-" + sessionID
	fifoDir := filepath.Join(tmpDir, "fifo")
	require.NoError(t, os.MkdirAll(fifoDir, 0o700))

	fifoPath := filepath.Join(fifoDir, sessionID+".fifo")
	require.NoError(t, syscall.Mkfifo(fifoPath, 0o600))

	// Create script
	scriptPath := filepath.Join(tmpDir, "fake-claude.sh")
	require.NoError(t, os.WriteFile(scriptPath, []byte(fakeClaudeScript(fifoPath)), 0o755))

	// Create tmux session
	require.NoError(t, exec.Command("tmux", "new-session", "-d", "-s", tmuxName, "-c", tmpDir).Run())
	defer exec.Command("tmux", "kill-session", "-t", tmuxName).Run()

	// Insert session record
	sess := &storage.Session{
		ID:       sessionID,
		TmuxName: tmuxName,
		WorkingDir: tmpDir,
		Model:    "sonnet",
		Status:   "active",
	}
	require.NoError(t, store.CreateSession(sess))

	// Run script in tmux
	cmd := scriptPath + " " + sessionID
	require.NoError(t, exec.Command("tmux", "send-keys", "-t", tmuxName, cmd, "Enter").Run())

	// Read ASK signal from FIFO
	f, err := os.OpenFile(fifoPath, os.O_RDONLY, 0)
	require.NoError(t, err)
	buf := make([]byte, 256)
	n, err := f.Read(buf)
	require.NoError(t, err)
	f.Close()
	signal := strings.TrimSpace(string(buf[:n]))
	assert.Equal(t, "ASK:"+sessionID, signal)

	// Simulate user reply via SendKeys
	time.Sleep(200 * time.Millisecond)
	require.NoError(t, exec.Command("tmux", "send-keys", "-t", tmuxName, "1", "Enter").Run())

	// Read DONE signal from FIFO
	f, err = os.OpenFile(fifoPath, os.O_RDONLY, 0)
	require.NoError(t, err)
	n, err = f.Read(buf)
	require.NoError(t, err)
	f.Close()
	signal = strings.TrimSpace(string(buf[:n]))
	assert.Equal(t, "DONE:"+sessionID, signal)

	// Capture final output
	time.Sleep(200 * time.Millisecond)
	out, err := exec.Command("tmux", "capture-pane", "-t", tmuxName, "-p", "-S", "-50").Output()
	require.NoError(t, err)
	output := string(out)
	assert.Contains(t, output, "분석 결과입니다")
}

func TestE2E_TildeExpansion(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping E2E test in short mode")
	}

	home, err := os.UserHomeDir()
	require.NoError(t, err)

	// Create a temp dir inside home for testing
	testDir := filepath.Join(home, ".claude-postman-e2e-test")
	require.NoError(t, os.MkdirAll(testDir, 0o755))
	defer os.RemoveAll(testDir)

	tmuxName := "e2e-tilde-test"
	require.NoError(t, exec.Command("tmux", "new-session", "-d", "-s", tmuxName, "-c", testDir).Run())
	defer exec.Command("tmux", "kill-session", "-t", tmuxName).Run()

	// Verify working directory
	time.Sleep(200 * time.Millisecond)
	require.NoError(t, exec.Command("tmux", "send-keys", "-t", tmuxName, "pwd", "Enter").Run())
	time.Sleep(200 * time.Millisecond)
	out, err := exec.Command("tmux", "capture-pane", "-t", tmuxName, "-p", "-S", "-5").Output()
	require.NoError(t, err)
	assert.Contains(t, string(out), testDir)
}
```

**Step 3: Run E2E tests**

Run: `go test ./_test/integration/ -tags integration -run TestE2E -v -timeout 30s`
Expected: PASS (requires tmux installed)

**Step 4: Commit**

```bash
git add _test/integration/e2e_prompt_test.go
git commit -m "test: add E2E tests for ASK signal, reply flow, and tilde expansion"
```

---

## Final Verification

Run all tests:

```bash
go test ./... -v
go build ./...
golangci-lint run ./...
```

Expected: ALL PASS, build clean, no lint errors.
