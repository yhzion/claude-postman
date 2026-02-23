// Package session manages tmux sessions for Claude Code.
package session

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/google/uuid"
	"github.com/yhzion/claude-postman/internal/email"
	"github.com/yhzion/claude-postman/internal/storage"
)

const (
	defaultFIFODir   = "/tmp/claude-postman"
	capturePaneLines = 1000
)

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

var (
	ErrSessionNotFound = errors.New("session not found")
	ErrSessionEnded    = errors.New("session already ended")
	ErrSessionNotIdle  = errors.New("session is not idle")
)

// renderOutput converts raw tmux output to HTML for email delivery.
// ANSI escape codes are stripped, then Markdown is rendered to HTML.
// Falls back to cleaned plain text if HTML conversion fails.
func renderOutput(raw string) string {
	cleaned := email.StripANSI(raw)
	html, err := email.RenderHTML(cleaned)
	if err != nil {
		return cleaned
	}
	return html
}

// Manager manages tmux session lifecycles.
type Manager struct {
	store        *storage.Store
	tmux         TmuxRunner
	fifoDir      string
	captureDelay time.Duration
}

// New creates a new session Manager.
func New(store *storage.Store, tmux TmuxRunner) *Manager {
	return &Manager{
		store:        store,
		tmux:         tmux,
		fifoDir:      defaultFIFODir,
		captureDelay: 500 * time.Millisecond,
	}
}

func tmuxName(sessionID string) string {
	return "session-" + sessionID
}

func (m *Manager) claudeCommand(sessionID, model, promptFile string) string {
	sysPrompt := fmt.Sprintf(systemPromptTemplate, sessionID, sessionID, sessionID, sessionID)
	return fmt.Sprintf("claude --dangerously-skip-permissions --session-id %s --system-prompt '%s' --model %s \"$(cat %s)\"",
		sessionID, sysPrompt, model, promptFile)
}

func (m *Manager) claudeResumeCommand(sessionID, model string) string {
	return fmt.Sprintf("claude --dangerously-skip-permissions --resume %s --model %s", sessionID, model)
}

func (m *Manager) promptFilePath(sessionID string) string {
	return filepath.Join(m.fifoDir, sessionID+".prompt")
}

func (m *Manager) writePromptFile(sessionID, prompt string) error {
	return os.WriteFile(m.promptFilePath(sessionID), []byte(prompt), 0o600)
}

func (m *Manager) removePromptFile(sessionID string) {
	_ = os.Remove(m.promptFilePath(sessionID))
}

// Create creates a new tmux session with Claude Code and sends the initial prompt
// as a CLI argument. This avoids timing issues with SendKeys-based prompt delivery.
func (m *Manager) Create(workingDir, model, prompt string) (*storage.Session, error) {
	id := uuid.New().String()
	name := tmuxName(id)

	session := &storage.Session{
		ID:         id,
		TmuxName:   name,
		WorkingDir: workingDir,
		Model:      model,
		Status:     "creating",
		LastPrompt: &prompt,
	}
	if err := m.store.CreateSession(session); err != nil {
		return nil, fmt.Errorf("create session record: %w", err)
	}

	if err := m.createFIFO(id); err != nil {
		return nil, fmt.Errorf("create FIFO: %w", err)
	}

	if err := m.writePromptFile(id, prompt); err != nil {
		return nil, fmt.Errorf("write prompt file: %w", err)
	}

	if err := m.tmux.NewSession(name, workingDir); err != nil {
		return nil, fmt.Errorf("tmux new-session: %w", err)
	}

	cmd := m.claudeCommand(id, model, m.promptFilePath(id))
	if err := m.tmux.SendKeys(name, cmd); err != nil {
		return nil, fmt.Errorf("tmux send-keys: %w", err)
	}

	session.Status = "active"
	if err := m.store.UpdateSession(session); err != nil {
		return nil, fmt.Errorf("update session status: %w", err)
	}

	go m.listenFIFO(id)

	return session, nil
}

// End terminates a tmux session.
func (m *Manager) End(sessionID string) error {
	session, err := m.store.GetSession(sessionID)
	if err != nil {
		return ErrSessionNotFound
	}
	if session.Status == "ended" {
		return ErrSessionEnded
	}

	_ = m.tmux.SendKeys(session.TmuxName, "/exit")
	_ = m.tmux.KillSession(session.TmuxName)

	m.writeSentinel(sessionID)
	_ = m.removeFIFO(sessionID)
	m.removePromptFile(sessionID)

	session.Status = "ended"
	return m.store.UpdateSession(session)
}

// DeliverNext sends the next queued inbox message to the tmux session.
// Only callable on idle sessions. Returns ErrSessionNotIdle for active/ended sessions.
func (m *Manager) DeliverNext(sessionID string) error {
	session, err := m.store.GetSession(sessionID)
	if err != nil {
		return ErrSessionNotFound
	}
	if session.Status != "idle" && session.Status != "waiting" {
		return ErrSessionNotIdle
	}

	var msg *storage.InboxMessage
	err = m.store.Tx(context.Background(), func(tx *storage.Store) error {
		var txErr error
		msg, txErr = tx.DequeueMessage(sessionID)
		if txErr != nil {
			return txErr
		}
		if msg == nil {
			return nil
		}
		if txErr = tx.MarkProcessed(msg.ID); txErr != nil {
			return txErr
		}
		session.Status = "active"
		session.LastPrompt = &msg.Body
		return tx.UpdateSession(session)
	})
	if err != nil {
		return err
	}

	if msg != nil {
		return m.tmux.SendKeys(session.TmuxName, msg.Body)
	}
	return nil
}

// Get returns a session by ID.
func (m *Manager) Get(sessionID string) (*storage.Session, error) {
	return m.store.GetSession(sessionID)
}

// ListActive returns all non-ended sessions (creating, active, idle, waiting).
func (m *Manager) ListActive() ([]*storage.Session, error) {
	return m.store.ListSessionsByStatus("creating", "active", "idle", "waiting")
}

// CaptureOutput captures the current tmux pane output for a session.
func (m *Manager) CaptureOutput(sessionID string) (string, error) {
	session, err := m.store.GetSession(sessionID)
	if err != nil {
		return "", ErrSessionNotFound
	}
	return m.tmux.CapturePane(session.TmuxName, capturePaneLines)
}

// RecoverAll attempts to recover sessions that were active/idle before server restart.
// For each session missing its tmux session, it recreates the tmux session with --resume.
// If recovery fails, the session is marked as ended.
func (m *Manager) RecoverAll() error {
	sessions, err := m.store.ListSessionsByStatus("active", "idle", "waiting")
	if err != nil {
		return err
	}

	for _, session := range sessions {
		if m.tmux.HasSession(session.TmuxName) {
			continue
		}

		if fifoErr := m.createFIFO(session.ID); fifoErr != nil {
			session.Status = "ended"
			_ = m.store.UpdateSession(session)
			continue
		}

		if tmuxErr := m.tmux.NewSession(session.TmuxName, session.WorkingDir); tmuxErr != nil {
			session.Status = "ended"
			_ = m.store.UpdateSession(session)
			continue
		}

		cmd := m.claudeResumeCommand(session.ID, session.Model)
		if sendErr := m.tmux.SendKeys(session.TmuxName, cmd); sendErr != nil {
			session.Status = "ended"
			_ = m.store.UpdateSession(session)
			continue
		}

		go m.listenFIFO(session.ID)
	}

	return nil
}
