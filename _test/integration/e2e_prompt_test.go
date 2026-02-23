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
	"github.com/yhzion/claude-postman/internal/email"
	"github.com/yhzion/claude-postman/internal/storage"
)

func skipIfNoTmux(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Skip("tmux not found, skipping E2E test")
	}
}

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
printf "❯ "
echo "ASK:$1" > ` + fifoPath + `
read -r answer
echo "선택: $answer"
echo "분석 결과입니다."
echo "DONE:$1" > ` + fifoPath + `
`
}

func TestE2E_AskSignalAndReply(t *testing.T) {
	skipIfNoTmux(t)

	// Setup
	tmpDir := t.TempDir()
	store := newTestStore(t)

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
	t.Cleanup(func() {
		_ = exec.Command("tmux", "kill-session", "-t", tmuxName).Run()
	})

	// Insert session record
	sess := &storage.Session{
		ID:         sessionID,
		TmuxName:   tmuxName,
		WorkingDir: tmpDir,
		Model:      "sonnet",
		Status:     "active",
	}
	require.NoError(t, store.CreateSession(sess))

	// Run script in tmux
	cmd := scriptPath + " " + sessionID
	require.NoError(t, exec.Command("tmux", "send-keys", "-t", tmuxName, cmd, "Enter").Run())

	// Read ASK signal from FIFO (this blocks until script writes to it)
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

func TestE2E_HTMLEmailFromTmuxOutput(t *testing.T) {
	skipIfNoTmux(t)

	tmpDir := t.TempDir()
	tmuxName := "e2e-html-email"

	// Create tmux session
	require.NoError(t, exec.Command("tmux", "new-session", "-d", "-s", tmuxName, "-c", tmpDir).Run())
	t.Cleanup(func() {
		_ = exec.Command("tmux", "kill-session", "-t", tmuxName).Run()
	})

	// Run a script that produces ANSI-colored output (simulates Claude Code)
	script := `printf '\033[1m## 분석 결과\033[0m\n\033[32m코드베이스가 정상입니다.\033[0m\n'`
	require.NoError(t, exec.Command("tmux", "send-keys", "-t", tmuxName, script, "Enter").Run())
	time.Sleep(500 * time.Millisecond)

	// Capture raw tmux output (as HandleDone would)
	out, err := exec.Command("tmux", "capture-pane", "-t", tmuxName, "-p", "-S", "-50").Output()
	require.NoError(t, err)
	raw := string(out)

	// Apply the same pipeline as renderOutput: StripANSI → RenderHTML
	cleaned := email.StripANSI(raw)
	assert.NotContains(t, cleaned, "\x1b[", "ANSI codes should be stripped")

	html, err := email.RenderHTML(cleaned)
	require.NoError(t, err)
	assert.Contains(t, html, "<html>")
	assert.Contains(t, html, "분석 결과")
	assert.Contains(t, html, "코드베이스가 정상입니다")
}

func TestE2E_TildeExpansion(t *testing.T) {
	skipIfNoTmux(t)

	home, err := os.UserHomeDir()
	require.NoError(t, err)

	// Create a temp dir inside home for testing
	testDir := filepath.Join(home, ".claude-postman-e2e-test")
	require.NoError(t, os.MkdirAll(testDir, 0o755))
	t.Cleanup(func() { os.RemoveAll(testDir) })

	tmuxName := "e2e-tilde-test"
	require.NoError(t, exec.Command("tmux", "new-session", "-d", "-s", tmuxName, "-c", testDir).Run())
	t.Cleanup(func() {
		_ = exec.Command("tmux", "kill-session", "-t", tmuxName).Run()
	})

	// Verify working directory
	time.Sleep(200 * time.Millisecond)
	require.NoError(t, exec.Command("tmux", "send-keys", "-t", tmuxName, "pwd", "Enter").Run())
	time.Sleep(200 * time.Millisecond)
	out, err := exec.Command("tmux", "capture-pane", "-t", tmuxName, "-p", "-S", "-5").Output()
	require.NoError(t, err)
	assert.Contains(t, string(out), testDir)
}
