// Package email handles SMTP/IMAP email operations.
package email

import (
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/yhzion/claude-postman/internal/config"
	"github.com/yhzion/claude-postman/internal/storage"
)

const maxRetries = 5

// IncomingMessage represents a parsed incoming email.
type IncomingMessage struct {
	From         string
	Subject      string
	Body         string
	SessionID    string // set when matching existing session
	MessageID    string
	IsNewSession bool   // template forward detected
	WorkingDir   string // parsed from template (IsNewSession=true)
	Model        string // parsed from template (IsNewSession=true)
}

// Mailer handles email sending and receiving.
type Mailer struct {
	cfg   *config.EmailConfig
	store *storage.Store
	imap  func() (IMAPClient, error) // factory for per-poll connections
	smtp  SMTPSender
}

// New creates a new Mailer with real IMAP/SMTP implementations.
func New(cfg *config.EmailConfig, store *storage.Store) *Mailer {
	return &Mailer{
		cfg:   cfg,
		store: store,
		imap: func() (IMAPClient, error) {
			return newIMAPClient(cfg)
		},
		smtp: newSMTPSender(cfg),
	}
}

// Poll fetches unread emails from IMAP and parses them into IncomingMessages.
// It does NOT write to the database — only reads for session matching.
func (m *Mailer) Poll() ([]*IncomingMessage, error) {
	client, err := m.imap()
	if err != nil {
		return nil, err
	}
	defer client.Close()

	raws, err := client.FetchUnread("[claude-postman]")
	if err != nil {
		return nil, err
	}

	var msgs []*IncomingMessage
	for _, raw := range raws {
		// Filter by sender
		if raw.From != m.cfg.User {
			slog.Debug("ignoring email from non-authorized sender", "from", raw.From)
			continue
		}

		// Filter out self-received template emails
		if ok, _ := m.store.IsValidTemplateRef(raw.MessageID); ok {
			slog.Debug("ignoring self-received template", "message_id", raw.MessageID)
			if markErr := client.MarkRead(raw.UID); markErr != nil {
				slog.Warn("failed to mark template as read", "uid", raw.UID, "error", markErr)
			}
			continue
		}

		msg := &IncomingMessage{
			From:      raw.From,
			Subject:   raw.Subject,
			Body:      raw.Body,
			MessageID: raw.MessageID,
		}

		// Check template reference (new session)
		if m.isTemplateRef(raw.InReplyTo, raw.References) {
			msg.IsNewSession = true
			body := raw.Body
			if looksLikeHTML(body) {
				body = ExtractTextFromHTML(body)
			}
			msg.WorkingDir, msg.Model, msg.Body = ParseTemplate(body)
		} else {
			// Try session matching
			msg.SessionID = ParseSessionID(raw.Body)
			if msg.SessionID == "" {
				msg.SessionID = m.matchByMessageID(raw.InReplyTo, raw.References)
			}
		}

		// Mark as read
		if markErr := client.MarkRead(raw.UID); markErr != nil {
			slog.Warn("failed to mark email as read", "uid", raw.UID, "error", markErr)
		}

		msgs = append(msgs, msg)
	}
	return msgs, nil
}

// Send inserts an email into the outbox for later delivery by FlushOutbox.
func (m *Mailer) Send(sessionID, subject, htmlBody string) error {
	msgID := fmt.Sprintf("<%s@claude-postman>", uuid.New().String())
	return m.store.CreateOutbox(&storage.OutboxMessage{
		ID:        uuid.New().String(),
		SessionID: sessionID,
		MessageID: &msgID,
		Subject:   subject,
		Body:      htmlBody,
		Status:    "pending",
	})
}

// FlushOutbox sends all pending outbox messages via SMTP.
// Uses exponential backoff on failure (30s * 2^(retry-1), max 5 retries).
func (m *Mailer) FlushOutbox() error {
	msgs, err := m.store.GetPendingOutbox()
	if err != nil {
		return err
	}
	for _, msg := range msgs {
		m.flushOne(msg)
	}
	return nil
}

func (m *Mailer) flushOne(msg *storage.OutboxMessage) {
	messageID := ""
	if msg.MessageID != nil {
		messageID = *msg.MessageID
	}

	err := m.smtp.Send(m.cfg.User, m.cfg.User, msg.Subject, msg.Body, messageID, "")
	if err != nil {
		slog.Warn("smtp send failed", "outbox_id", msg.ID, "error", err)
		m.handleRetry(msg)
		return
	}

	if markErr := m.store.MarkSent(msg.ID); markErr != nil {
		slog.Error("failed to mark outbox as sent", "id", msg.ID, "error", markErr)
	}
}

func (m *Mailer) handleRetry(msg *storage.OutboxMessage) {
	retryCount := msg.RetryCount + 1
	if retryCount >= maxRetries {
		if err := m.store.MarkFailed(msg.ID, retryCount, nil); err != nil {
			slog.Error("failed to mark outbox as failed", "id", msg.ID, "error", err)
		}
		return
	}
	backoff := 30 * time.Second * (1 << (retryCount - 1))
	nextRetry := time.Now().Add(backoff)
	if err := m.store.UpdateRetry(msg.ID, retryCount, &nextRetry); err != nil {
		slog.Error("failed to update retry", "id", msg.ID, "error", err)
	}
}

// SendTemplate sends the session creation template email and returns its Message-ID.
func (m *Mailer) SendTemplate() (string, error) {
	templateBody := `How to create a new Claude Code session
========================================

IMPORTANT — Do NOT change:
  - The subject line (must contain [claude-postman])
  - You must REPLY to this email (do not compose a new one)
  - Send to yourself (your own email address)
  - Keep "Directory:" and "Model:" keywords exactly as written

You CAN edit:
  - The path after "Directory:" (e.g. ~/my-project)
  - The model after "Model:" — sonnet | opus | haiku
  - Replace "(Write your task here)" with your task

────────────────────────────────────

Directory: ~
Model: sonnet

(Write your task here)

────────────────────────────────────

Tips:
  - You can reply to this email multiple times
    — each reply creates a new session
  - A fresh template is sent every time the server starts`
	htmlBody, err := RenderHTML(templateBody)
	if err != nil {
		return "", fmt.Errorf("render template: %w", err)
	}

	messageID := fmt.Sprintf("<%s@claude-postman>", uuid.New().String())
	subject := "[claude-postman] New Session"

	if err := m.smtp.Send(m.cfg.User, m.cfg.User, subject, htmlBody, messageID, ""); err != nil {
		return "", fmt.Errorf("send template: %w", err)
	}

	templateID := uuid.New().String()
	if err := m.store.SaveTemplate(&storage.Template{
		ID:        templateID,
		MessageID: messageID,
	}); err != nil {
		return "", fmt.Errorf("save template: %w", err)
	}

	return messageID, nil
}

// isTemplateRef checks if any of the message references match a known template.
func (m *Mailer) isTemplateRef(inReplyTo string, references []string) bool {
	if inReplyTo != "" {
		if ok, _ := m.store.IsValidTemplateRef(inReplyTo); ok {
			return true
		}
	}
	for _, ref := range references {
		if ok, _ := m.store.IsValidTemplateRef(ref); ok {
			return true
		}
	}
	return false
}

// matchByMessageID tries to find a session ID by matching outbox Message-IDs.
func (m *Mailer) matchByMessageID(inReplyTo string, references []string) string {
	if inReplyTo != "" {
		if sid, _ := m.store.GetSessionIDByOutboxMessageID(inReplyTo); sid != "" {
			return sid
		}
	}
	for _, ref := range references {
		if sid, _ := m.store.GetSessionIDByOutboxMessageID(ref); sid != "" {
			return sid
		}
	}
	return ""
}

func looksLikeHTML(s string) bool {
	lower := strings.ToLower(s)
	return strings.Contains(lower, "<html") || strings.Contains(lower, "<body") ||
		strings.Contains(lower, "<div") || strings.Contains(lower, "<p>")
}
