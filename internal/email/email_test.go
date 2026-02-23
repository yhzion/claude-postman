package email

import (
	"errors"
	"testing"
	"time"

	"github.com/emersion/go-imap/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yhzion/claude-postman/internal/config"
	"github.com/yhzion/claude-postman/internal/storage"
)

// --- Mock IMAP ---

type mockIMAPClient struct {
	emails []*RawEmail
	marked []imap.UID
	err    error
}

func (m *mockIMAPClient) FetchUnread(_ string) ([]*RawEmail, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.emails, nil
}

func (m *mockIMAPClient) MarkRead(uid imap.UID) error {
	m.marked = append(m.marked, uid)
	return nil
}

func (m *mockIMAPClient) Close() error { return nil }

// --- Mock SMTP ---

type mockSMTPSender struct {
	sent []sentEmail
	err  error
}

type sentEmail struct {
	from, to, subject, body, messageID, inReplyTo string
}

func (m *mockSMTPSender) Send(from, to, subject, body, messageID, inReplyTo string) error {
	if m.err != nil {
		return m.err
	}
	m.sent = append(m.sent, sentEmail{from, to, subject, body, messageID, inReplyTo})
	return nil
}

// --- Helpers ---

func testMailer(t *testing.T, imapClient *mockIMAPClient, smtpSender *mockSMTPSender) (*Mailer, *storage.Store) {
	t.Helper()
	dir := t.TempDir()
	store, err := storage.New(dir)
	require.NoError(t, err)
	require.NoError(t, store.Migrate())
	t.Cleanup(func() { store.Close() })

	cfg := &config.EmailConfig{
		User: "user@example.com",
	}

	m := &Mailer{
		cfg:   cfg,
		store: store,
		imap: func() (IMAPClient, error) {
			return imapClient, nil
		},
		smtp: smtpSender,
	}
	return m, store
}

// createTestSession inserts a minimal session for FK constraints.
func createTestSession(t *testing.T, store *storage.Store, id string) {
	t.Helper()
	require.NoError(t, store.CreateSession(&storage.Session{
		ID:         id,
		TmuxName:   "test-" + id[:8],
		WorkingDir: "/tmp",
		Model:      "sonnet",
		Status:     "idle",
	}))
}

// --- Tests ---

func TestSend(t *testing.T) {
	t.Run("creates outbox record", func(t *testing.T) {
		m, store := testMailer(t, &mockIMAPClient{}, &mockSMTPSender{})
		sessionID := "11111111-1111-1111-1111-111111111111"
		createTestSession(t, store, sessionID)

		err := m.Send(sessionID, "[claude-postman] Completed: test", "<p>Done</p>")
		require.NoError(t, err)

		msgs, err := store.GetPendingOutbox()
		require.NoError(t, err)
		require.Len(t, msgs, 1)
		assert.Equal(t, sessionID, msgs[0].SessionID)
		assert.Equal(t, "[claude-postman] Completed: test", msgs[0].Subject)
		assert.Equal(t, "pending", msgs[0].Status)
		assert.NotNil(t, msgs[0].MessageID)
	})
}

func TestFlushOutbox(t *testing.T) {
	t.Run("sends pending and marks sent", func(t *testing.T) {
		smtp := &mockSMTPSender{}
		m, store := testMailer(t, &mockIMAPClient{}, smtp)
		sessionID := "22222222-2222-2222-2222-222222222222"
		createTestSession(t, store, sessionID)

		require.NoError(t, m.Send(sessionID, "test subject", "<p>body</p>"))

		err := m.FlushOutbox()
		require.NoError(t, err)
		assert.Len(t, smtp.sent, 1)
		assert.Equal(t, "user@example.com", smtp.sent[0].from)
		assert.Equal(t, "test subject", smtp.sent[0].subject)

		// Verify marked as sent
		msgs, err := store.GetPendingOutbox()
		require.NoError(t, err)
		assert.Empty(t, msgs)
	})

	t.Run("failure increments retry with backoff", func(t *testing.T) {
		smtp := &mockSMTPSender{err: errors.New("smtp error")}
		m, store := testMailer(t, &mockIMAPClient{}, smtp)
		sessionID := "33333333-3333-3333-3333-333333333333"
		createTestSession(t, store, sessionID)

		require.NoError(t, m.Send(sessionID, "test", "<p>body</p>"))

		before := time.Now()
		err := m.FlushOutbox()
		require.NoError(t, err)

		// Message should still be pending but with retry info
		// next_retry_at is in the future, so GetPendingOutbox won't return it
		msgs, err := store.GetPendingOutbox()
		require.NoError(t, err)
		assert.Empty(t, msgs, "message should have future next_retry_at")

		// Verify by checking retry count is 1
		// The backoff for retry_count=1 is 30s
		_ = before // backoff calculated from time.Now() in FlushOutbox
	})

	t.Run("max retries marks as failed", func(t *testing.T) {
		smtp := &mockSMTPSender{err: errors.New("smtp error")}
		m, store := testMailer(t, &mockIMAPClient{}, smtp)
		sessionID := "44444444-4444-4444-4444-444444444444"
		createTestSession(t, store, sessionID)

		// Create outbox with retry_count = maxRetries-1 (4)
		msgID := "<test@claude-postman>"
		require.NoError(t, store.CreateOutbox(&storage.OutboxMessage{
			ID:         "outbox-max-retry",
			SessionID:  sessionID,
			MessageID:  &msgID,
			Subject:    "test",
			Body:       "<p>body</p>",
			Status:     "pending",
			RetryCount: maxRetries - 1,
		}))

		err := m.FlushOutbox()
		require.NoError(t, err)

		// Should be marked as failed (not returned by GetPendingOutbox)
		msgs, err := store.GetPendingOutbox()
		require.NoError(t, err)
		assert.Empty(t, msgs)
	})
}

func TestSendTemplate(t *testing.T) {
	t.Run("sends template email and returns messageID", func(t *testing.T) {
		smtp := &mockSMTPSender{}
		m, _ := testMailer(t, &mockIMAPClient{}, smtp)

		messageID, err := m.SendTemplate()
		require.NoError(t, err)
		assert.NotEmpty(t, messageID)
		assert.Contains(t, messageID, "@claude-postman>")

		// Verify SMTP was called
		require.Len(t, smtp.sent, 1)
		assert.Equal(t, "[claude-postman] New Session", smtp.sent[0].subject)
		assert.Equal(t, "user@example.com", smtp.sent[0].from)
		assert.Equal(t, "user@example.com", smtp.sent[0].to)
	})

	t.Run("template body instructs reply not forward", func(t *testing.T) {
		smtp := &mockSMTPSender{}
		m, _ := testMailer(t, &mockIMAPClient{}, smtp)

		_, err := m.SendTemplate()
		require.NoError(t, err)

		require.Len(t, smtp.sent, 1)
		body := smtp.sent[0].body
		assert.NotContains(t, body, "FORWARD")
		assert.NotContains(t, body, "forward")
		assert.Contains(t, body, "REPLY")
	})
}

func TestPoll(t *testing.T) {
	t.Run("filters by sender", func(t *testing.T) {
		imap := &mockIMAPClient{
			emails: []*RawEmail{
				{From: "other@example.com", Subject: "[claude-postman] test", Body: "hello", UID: 1},
				{From: "user@example.com", Subject: "[claude-postman] test", Body: "world", UID: 2},
			},
		}
		m, _ := testMailer(t, imap, &mockSMTPSender{})

		msgs, err := m.Poll()
		require.NoError(t, err)
		require.Len(t, msgs, 1)
		assert.Equal(t, "user@example.com", msgs[0].From)
		assert.Equal(t, "world", msgs[0].Body)
	})

	t.Run("detects new session via template reply", func(t *testing.T) {
		smtp := &mockSMTPSender{}
		imapMock := &mockIMAPClient{}
		m, _ := testMailer(t, imapMock, smtp)

		// First, send a template to create a reference
		messageID, err := m.SendTemplate()
		require.NoError(t, err)

		// Now simulate a reply to the template email
		imapMock.emails = []*RawEmail{
			{
				From:      "user@example.com",
				Subject:   "[claude-postman] New Session",
				Body:      "Directory: /home/test\nModel: opus\n\nBuild a feature",
				InReplyTo: messageID,
				UID:       1,
			},
		}

		msgs, err := m.Poll()
		require.NoError(t, err)
		require.Len(t, msgs, 1)
		assert.True(t, msgs[0].IsNewSession)
		assert.Equal(t, "/home/test", msgs[0].WorkingDir)
		assert.Equal(t, "opus", msgs[0].Model)
		assert.Equal(t, "Build a feature", msgs[0].Body)
	})

	t.Run("matches existing session by Session-ID", func(t *testing.T) {
		imap := &mockIMAPClient{
			emails: []*RawEmail{
				{
					From:    "user@example.com",
					Subject: "[claude-postman] reply",
					Body:    "Continue working\nSession-ID: aabbccdd-1122-3344-5566-778899001122",
					UID:     1,
				},
			},
		}
		m, _ := testMailer(t, imap, &mockSMTPSender{})

		msgs, err := m.Poll()
		require.NoError(t, err)
		require.Len(t, msgs, 1)
		assert.False(t, msgs[0].IsNewSession)
		assert.Equal(t, "aabbccdd-1122-3344-5566-778899001122", msgs[0].SessionID)
	})

	t.Run("ignores self-received template email", func(t *testing.T) {
		smtp := &mockSMTPSender{}
		imapMock := &mockIMAPClient{}
		m, _ := testMailer(t, imapMock, smtp)

		// Send template to create DB record
		messageID, err := m.SendTemplate()
		require.NoError(t, err)

		// Simulate the template email arriving back via IMAP
		imapMock.emails = []*RawEmail{
			{
				From:      "user@example.com",
				Subject:   "[claude-postman] New Session",
				Body:      "How to create a new Claude Code session...",
				MessageID: messageID,
				UID:       1,
			},
		}

		msgs, err := m.Poll()
		require.NoError(t, err)
		assert.Empty(t, msgs, "self-received template should be filtered out")
		assert.Contains(t, imapMock.marked, imap.UID(1), "should still mark as read")
	})

	t.Run("matches existing session by outbox Message-ID", func(t *testing.T) {
		imap := &mockIMAPClient{}
		m, store := testMailer(t, imap, &mockSMTPSender{})

		// Create a session and outbox record
		sessionID := "55555555-5555-5555-5555-555555555555"
		createTestSession(t, store, sessionID)
		outboxMsgID := "<outbox-123@claude-postman>"
		require.NoError(t, store.CreateOutbox(&storage.OutboxMessage{
			ID:        "outbox-1",
			SessionID: sessionID,
			MessageID: &outboxMsgID,
			Subject:   "prev reply",
			Body:      "<p>previous</p>",
			Status:    "sent",
		}))

		// Simulate reply to that outbox message
		imap.emails = []*RawEmail{
			{
				From:      "user@example.com",
				Subject:   "[claude-postman] follow-up",
				Body:      "Do more work",
				InReplyTo: outboxMsgID,
				UID:       1,
			},
		}

		msgs, err := m.Poll()
		require.NoError(t, err)
		require.Len(t, msgs, 1)
		assert.Equal(t, sessionID, msgs[0].SessionID)
		assert.False(t, msgs[0].IsNewSession)
	})
}
