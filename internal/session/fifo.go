package session

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/google/uuid"
	"github.com/yhzion/claude-postman/internal/storage"
)

func (m *Manager) fifoPath(sessionID string) string {
	return filepath.Join(m.fifoDir, sessionID+".fifo")
}

func (m *Manager) createFIFO(sessionID string) error {
	if err := os.MkdirAll(m.fifoDir, 0o700); err != nil {
		return err
	}
	err := syscall.Mkfifo(m.fifoPath(sessionID), 0o600)
	if err != nil && !errors.Is(err, syscall.EEXIST) {
		return err
	}
	return nil
}

func (m *Manager) removeFIFO(sessionID string) error {
	return os.Remove(m.fifoPath(sessionID))
}

// writeSentinel writes the SHUTDOWN sentinel to the FIFO in non-blocking mode.
// ENXIO is expected when no reader exists and is silently ignored.
func (m *Manager) writeSentinel(sessionID string) {
	path := m.fifoPath(sessionID)
	fd, err := syscall.Open(path, syscall.O_WRONLY|syscall.O_NONBLOCK, 0)
	if err != nil {
		return
	}
	defer syscall.Close(fd)
	_, _ = syscall.Write(fd, []byte("SHUTDOWN\n"))
}

// handleDoneTx runs the transactional part of HandleDone:
// saves result, creates outbox, and checks for the next queued inbox message.
func (m *Manager) handleDoneTx(session *storage.Session, output string) (*storage.InboxMessage, error) {
	var nextMsg *storage.InboxMessage
	err := m.store.Tx(context.Background(), func(tx *storage.Store) error {
		session.LastResult = &output

		outbox := &storage.OutboxMessage{
			ID:        uuid.New().String(),
			SessionID: session.ID,
			Subject:   "Claude Code result",
			Body:      renderOutput(output),
			Status:    "pending",
		}
		if txErr := tx.CreateOutbox(outbox); txErr != nil {
			return txErr
		}

		var txErr error
		nextMsg, txErr = tx.DequeueMessage(session.ID)
		if txErr != nil {
			return txErr
		}

		if nextMsg != nil {
			if txErr = tx.MarkProcessed(nextMsg.ID); txErr != nil {
				return txErr
			}
			session.LastPrompt = &nextMsg.Body
		} else {
			session.Status = "idle"
		}

		return tx.UpdateSession(session)
	})
	return nextMsg, err
}

// handleAskTx runs the transactional part of HandleAsk:
// saves result, creates outbox, and sets status to "waiting".
func (m *Manager) handleAskTx(session *storage.Session, output string) error {
	return m.store.Tx(context.Background(), func(tx *storage.Store) error {
		session.LastResult = &output

		outbox := &storage.OutboxMessage{
			ID:        uuid.New().String(),
			SessionID: session.ID,
			Subject:   "Claude Code is waiting for your input",
			Body:      renderOutput(output),
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

// dispatchSignal processes a single FIFO signal line.
// Returns true if the listener should exit (SHUTDOWN received).
func (m *Manager) dispatchSignal(sessionID, line string) bool {
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
		return true
	default:
		slog.Warn("unknown FIFO signal", "session_id", sessionID, "line", line)
	}
	return false
}

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
			if m.dispatchSignal(sessionID, scanner.Text()) {
				f.Close()
				return
			}
		}
		f.Close()
	}
}

// HandleDone processes a DONE signal from a session's FIFO.
// 1. Waits captureDelay for rendering to complete.
// 2. Captures tmux pane output.
// 3. In a transaction: saves result, creates outbox, checks for next inbox message.
// 4. If a queued message exists, sends it to tmux outside the transaction.
func (m *Manager) HandleDone(sessionID string) error {
	time.Sleep(m.captureDelay)

	session, err := m.store.GetSession(sessionID)
	if err != nil {
		return ErrSessionNotFound
	}

	output, err := m.tmux.CapturePane(session.TmuxName, capturePaneLines)
	if err != nil {
		return fmt.Errorf("capture-pane: %w", err)
	}

	nextMsg, err := m.handleDoneTx(session, output)
	if err != nil {
		return err
	}

	if nextMsg != nil {
		return m.tmux.SendKeys(session.TmuxName, nextMsg.Body)
	}
	return nil
}
