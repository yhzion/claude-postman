// Package serve implements the main event loop for claude-postman.
package serve

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/google/uuid"
	"golang.org/x/sync/errgroup"

	"github.com/yhzion/claude-postman/internal/config"
	"github.com/yhzion/claude-postman/internal/email"
	"github.com/yhzion/claude-postman/internal/session"
	"github.com/yhzion/claude-postman/internal/storage"
)

const fifoDir = "/tmp/claude-postman"

// sessionMgr abstracts session.Manager for testability.
type sessionMgr interface {
	Create(workingDir, model string) (*storage.Session, error)
	DeliverNext(sessionID string) error
	ListActive() ([]*storage.Session, error)
	RecoverAll() error
}

// mailPoller abstracts email.Mailer for testability.
type mailPoller interface {
	Poll() ([]*email.IncomingMessage, error)
	FlushOutbox() error
	SendTemplate() (string, error)
}

type server struct {
	cfg          *config.Config
	store        *storage.Store
	mgr          sessionMgr
	mailer       mailPoller
	pollInterval time.Duration // override for testing; 0 means use cfg
}

// RunServe runs the main event loop with signal handling.
func RunServe(ctx context.Context, cfg *config.Config,
	store *storage.Store, mgr *session.Manager, mailer *email.Mailer) error {
	ctx, stop := signal.NotifyContext(ctx, syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	s := &server{
		cfg:    cfg,
		store:  store,
		mgr:    mgr,
		mailer: mailer,
	}
	return s.run(ctx)
}

func (s *server) run(ctx context.Context) error {
	if err := os.MkdirAll(fifoDir, 0o700); err != nil {
		return fmt.Errorf("create FIFO dir: %w", err)
	}

	// Send template email to verify SMTP and ensure user has a fresh template.
	// Serve will not start if this fails.
	msgID, err := s.mailer.SendTemplate()
	if err != nil {
		return fmt.Errorf("send template email: %w", err)
	}
	slog.Info("template email sent", "message_id", msgID)

	if err := s.mgr.RecoverAll(); err != nil {
		return fmt.Errorf("recover sessions: %w", err)
	}

	interval := s.pollInterval
	if interval == 0 {
		interval = time.Duration(s.cfg.General.PollIntervalSec) * time.Second
	}

	slog.Info("serve started", "poll_interval", interval)

	g, ctx := errgroup.WithContext(ctx)

	g.Go(func() error {
		return s.pollLoop(ctx, interval)
	})

	g.Go(func() error {
		return s.flushLoop(ctx, interval)
	})

	return g.Wait()
}

func (s *server) pollLoop(ctx context.Context, interval time.Duration) error {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			msgs, err := s.mailer.Poll()
			if err != nil {
				slog.Error("IMAP poll failed", "error", err)
				continue
			}
			if err := s.processMessages(msgs); err != nil {
				slog.Error("process messages failed", "error", err)
			}
			if err := s.checkIdleSessions(); err != nil {
				slog.Error("check idle sessions failed", "error", err)
			}
		}
	}
}

func (s *server) flushLoop(ctx context.Context, interval time.Duration) error {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			if err := s.mailer.FlushOutbox(); err != nil {
				slog.Error("outbox flush failed", "error", err)
			}
		}
	}
}

func (s *server) processMessages(msgs []*email.IncomingMessage) error {
	for _, msg := range msgs {
		if msg.IsNewSession {
			if err := s.handleNewSession(msg); err != nil {
				slog.Error("failed to create session", "error", err)
			}
		} else if msg.SessionID != "" {
			if err := s.handleExistingSession(msg); err != nil {
				slog.Error("failed to enqueue message", "session_id", msg.SessionID, "error", err)
			}
		} else {
			slog.Warn("ignoring unmatched email", "from", msg.From, "subject", msg.Subject)
		}
	}
	return nil
}

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
	}

	sess, err := s.mgr.Create(workingDir, model)
	if err != nil {
		return fmt.Errorf("create session: %w", err)
	}

	inboxMsg := &storage.InboxMessage{
		ID:        uuid.New().String(),
		SessionID: sess.ID,
		Body:      msg.Body,
	}
	if err := s.store.EnqueueMessage(inboxMsg); err != nil {
		return fmt.Errorf("enqueue message: %w", err)
	}

	// Set session to idle so DeliverNext can process the queued message.
	sess.Status = "idle"
	if err := s.store.UpdateSession(sess); err != nil {
		return fmt.Errorf("update session: %w", err)
	}

	if err := s.mgr.DeliverNext(sess.ID); err != nil {
		slog.Warn("failed to deliver initial message", "session_id", sess.ID, "error", err)
	}

	return nil
}

func (s *server) handleExistingSession(msg *email.IncomingMessage) error {
	return s.store.EnqueueMessage(&storage.InboxMessage{
		ID:        uuid.New().String(),
		SessionID: msg.SessionID,
		Body:      msg.Body,
	})
}

func (s *server) checkIdleSessions() error {
	sessions, err := s.mgr.ListActive()
	if err != nil {
		return err
	}

	for _, sess := range sessions {
		if sess.Status == "idle" {
			if err := s.mgr.DeliverNext(sess.ID); err != nil {
				slog.Warn("failed to deliver to idle session", "session_id", sess.ID, "error", err)
			}
		}
	}

	return nil
}
