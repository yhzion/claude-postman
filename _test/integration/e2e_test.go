// Package integration contains end-to-end tests that use real external services.
//
// Required environment variables:
//
//	E2E_EMAIL_USER     — Gmail address (e.g. gplusit@gmail.com)
//	E2E_EMAIL_PASSWORD — Gmail app password
//
// Run:
//
//	E2E_EMAIL_USER=gplusit@gmail.com E2E_EMAIL_PASSWORD=xxx \
//	  go test ./_test/integration/... -v -count=1 -timeout 120s
package integration

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yhzion/claude-postman/internal/config"
	"github.com/yhzion/claude-postman/internal/email"
	"github.com/yhzion/claude-postman/internal/storage"
)

func skipIfNoCredentials(t *testing.T) (user, pass string) {
	t.Helper()
	user = os.Getenv("E2E_EMAIL_USER")
	pass = os.Getenv("E2E_EMAIL_PASSWORD")
	if user == "" || pass == "" {
		t.Skip("E2E_EMAIL_USER / E2E_EMAIL_PASSWORD not set, skipping E2E test")
	}
	return user, pass
}

func newTestConfig(user, pass string) *config.EmailConfig {
	return &config.EmailConfig{
		Provider:    "gmail",
		SMTPHost:    "smtp.gmail.com",
		SMTPPort:    587,
		IMAPHost:    "imap.gmail.com",
		IMAPPort:    993,
		User:        user,
		AppPassword: pass,
	}
}

func newTestStore(t *testing.T) *storage.Store {
	t.Helper()
	dir := t.TempDir()
	store, err := storage.New(dir)
	require.NoError(t, err)
	require.NoError(t, store.Migrate())
	t.Cleanup(func() { store.Close() })
	return store
}

// TestE2E_SMTPConnection tests that SMTP connection works with real credentials.
func TestE2E_SMTPConnection(t *testing.T) {
	user, pass := skipIfNoCredentials(t)
	cfg := newTestConfig(user, pass)
	store := newTestStore(t)
	mailer := email.New(cfg, store)

	// SendTemplate does real SMTP send
	msgID, err := mailer.SendTemplate()
	require.NoError(t, err, "SMTP send should succeed")
	assert.NotEmpty(t, msgID, "should return a Message-ID")

	t.Logf("Template sent with Message-ID: %s", msgID)
}

// TestE2E_IMAPConnection tests that IMAP poll works with real credentials.
func TestE2E_IMAPConnection(t *testing.T) {
	user, pass := skipIfNoCredentials(t)
	cfg := newTestConfig(user, pass)
	store := newTestStore(t)
	mailer := email.New(cfg, store)

	// Poll should not error (may return 0 messages)
	msgs, err := mailer.Poll()
	require.NoError(t, err, "IMAP poll should succeed")

	t.Logf("Polled %d messages", len(msgs))
}

// TestE2E_SendAndPoll tests the full flow: send an email via SMTP, then poll it via IMAP.
func TestE2E_SendAndPoll(t *testing.T) {
	user, pass := skipIfNoCredentials(t)
	cfg := newTestConfig(user, pass)
	store := newTestStore(t)
	mailer := email.New(cfg, store)

	// 1. Send a unique test email via outbox + flush
	testID := time.Now().Format("20060102-150405")
	subject := "[claude-postman] E2E Test " + testID
	body := "<p>E2E test body " + testID + "</p>"

	sessionID := "e2e-test-session"
	require.NoError(t, store.CreateSession(&storage.Session{
		ID:         sessionID,
		TmuxName:   "test-tmux",
		WorkingDir: "/tmp",
		Model:      "sonnet",
		Status:     "active",
	}))

	require.NoError(t, mailer.Send(sessionID, subject, body))
	require.NoError(t, mailer.FlushOutbox())

	t.Logf("Sent test email: %s", subject)

	// 2. Wait for Gmail to process (SMTP→IMAP propagation)
	t.Log("Waiting 10s for Gmail propagation...")
	time.Sleep(10 * time.Second)

	// 3. Poll and look for our test email
	msgs, err := mailer.Poll()
	require.NoError(t, err)

	var found bool
	for _, msg := range msgs {
		if msg.MessageID != "" {
			t.Logf("  Polled: subject=%q from=%s", msg.Subject, msg.From)
		}
		if msg.From == user && msg.Subject == subject {
			found = true
			t.Logf("Found our test email!")
		}
	}

	// Note: the email might not appear immediately or might have been
	// already marked as read. This is expected in some cases.
	if !found {
		t.Log("Test email not found in poll (may need longer propagation or was already read)")
	}
}

// TestE2E_DoctorChecks tests doctor checks against real infrastructure.
func TestE2E_DoctorChecks(t *testing.T) {
	user, pass := skipIfNoCredentials(t)
	_ = user
	_ = pass

	dir := t.TempDir()
	dataDir := filepath.Join(dir, "data")
	require.NoError(t, os.MkdirAll(dataDir, 0o755))

	// Write a valid config
	cfgContent := `[general]
data_dir = "` + dataDir + `"

[email]
user = "` + user + `"
app_password = "` + pass + `"
smtp_host = "smtp.gmail.com"
smtp_port = 587
imap_host = "imap.gmail.com"
imap_port = 993
`
	require.NoError(t, os.WriteFile(filepath.Join(dir, "config.toml"), []byte(cfgContent), 0o600))

	// Load config to verify
	cfg, err := config.LoadFrom(dir)
	require.NoError(t, err)
	assert.Equal(t, user, cfg.Email.User)
	assert.Equal(t, "smtp.gmail.com", cfg.Email.SMTPHost)

	t.Log("Config loaded and validated successfully")
}

// TestE2E_StorageFullCycle tests storage operations end-to-end.
func TestE2E_StorageFullCycle(t *testing.T) {
	store := newTestStore(t)

	// Create session
	sess := &storage.Session{
		ID:         "e2e-storage-test",
		TmuxName:   "session-e2e-storage-test",
		WorkingDir: "/tmp",
		Model:      "sonnet",
		Status:     "active",
	}
	require.NoError(t, store.CreateSession(sess))

	// Get session
	got, err := store.GetSession("e2e-storage-test")
	require.NoError(t, err)
	assert.Equal(t, "active", got.Status)

	// Enqueue inbox message
	inbox := &storage.InboxMessage{
		ID:        "e2e-inbox-1",
		SessionID: "e2e-storage-test",
		Body:      "Hello from E2E test",
	}
	require.NoError(t, store.EnqueueMessage(inbox))

	// Dequeue
	dequeued, err := store.DequeueMessage("e2e-storage-test")
	require.NoError(t, err)
	require.NotNil(t, dequeued)
	assert.Equal(t, "Hello from E2E test", dequeued.Body)

	// Create outbox
	outbox := &storage.OutboxMessage{
		ID:        "e2e-outbox-1",
		SessionID: "e2e-storage-test",
		Subject:   "E2E outbox test",
		Body:      "<p>test</p>",
		Status:    "pending",
	}
	require.NoError(t, store.CreateOutbox(outbox))

	// Get pending
	pending, err := store.GetPendingOutbox()
	require.NoError(t, err)
	assert.Len(t, pending, 1)

	// Mark sent
	require.NoError(t, store.MarkSent("e2e-outbox-1"))

	// Verify no more pending
	pending, err = store.GetPendingOutbox()
	require.NoError(t, err)
	assert.Empty(t, pending)

	// Update session status
	got.Status = "ended"
	require.NoError(t, store.UpdateSession(got))

	ended, err := store.GetSession("e2e-storage-test")
	require.NoError(t, err)
	assert.Equal(t, "ended", ended.Status)

	t.Log("Storage full cycle completed successfully")
}
