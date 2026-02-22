package config

import (
	"bufio"
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newTestWizard는 테스트용 initWizard를 생성한다.
func newTestWizard(input string) (*initWizard, *bytes.Buffer) {
	out := &bytes.Buffer{}
	return &initWizard{
		in:  bufio.NewScanner(strings.NewReader(input)),
		out: out,
	}, out
}

// --- prompt() ---

func TestPrompt_ReturnsUserInput(t *testing.T) {
	w, _ := newTestWizard("custom-value\n")
	got := w.prompt("Enter something", "default")
	assert.Equal(t, "custom-value", got)
}

func TestPrompt_ReturnsDefaultOnEmpty(t *testing.T) {
	w, _ := newTestWizard("\n")
	got := w.prompt("Enter something", "default-val")
	assert.Equal(t, "default-val", got)
}

// --- promptChoice() ---

func TestPromptChoice_SelectsByNumber(t *testing.T) {
	// 1-based input "2" → 0-based index 1
	w, _ := newTestWizard("2\n")
	got := w.promptChoice([]string{"A", "B", "C"}, 0)
	assert.Equal(t, 1, got)
}

func TestPromptChoice_ReturnsDefaultOnEmpty(t *testing.T) {
	w, _ := newTestWizard("\n")
	got := w.promptChoice([]string{"A", "B", "C"}, 2)
	assert.Equal(t, 2, got)
}

func TestPromptChoice_ReturnsDefaultOnInvalid(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{"out of range high", "5\n"},
		{"out of range zero", "0\n"},
		{"non-number", "abc\n"},
		{"negative", "-1\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w, _ := newTestWizard(tt.input)
			got := w.promptChoice([]string{"A", "B", "C"}, 0)
			assert.Equal(t, 0, got)
		})
	}
}

// --- promptSecret() ---

func TestPromptSecret_KeepsExistingOnN(t *testing.T) {
	w, _ := newTestWizard("N\n")
	got := w.promptSecret("Password", "existing-pass")
	assert.Equal(t, "existing-pass", got)
}

func TestPromptSecret_KeepsExistingOnEmpty(t *testing.T) {
	// Enter (empty) is treated as "N" (not "y")
	w, _ := newTestWizard("\n")
	got := w.promptSecret("Password", "existing-pass")
	assert.Equal(t, "existing-pass", got)
}

func TestPromptSecret_ReadsNewValueOnY(t *testing.T) {
	w, _ := newTestWizard("y\nnew-secret\n")
	got := w.promptSecret("Password", "existing-pass")
	assert.Equal(t, "new-secret", got)
}

func TestPromptSecret_ReadsDirectlyWhenNoExisting(t *testing.T) {
	w, _ := newTestWizard("my-secret\n")
	got := w.promptSecret("Password", "")
	assert.Equal(t, "my-secret", got)
}

// --- promptInt() ---

func TestPromptInt_ReturnsParsedInt(t *testing.T) {
	w, _ := newTestWizard("42\n")
	got := w.promptInt("Port", 587, 993)
	assert.Equal(t, 42, got)
}

func TestPromptInt_ReturnsFallbackWhenCurrentZeroAndEmpty(t *testing.T) {
	// current=0 → def=fallback(993), empty input → returns 993
	w, _ := newTestWizard("\n")
	got := w.promptInt("Port", 0, 993)
	assert.Equal(t, 993, got)
}

func TestPromptInt_ReturnsCurrentWhenNonZeroAndEmpty(t *testing.T) {
	// current=587 → def=587, empty input → returns 587
	w, _ := newTestWizard("\n")
	got := w.promptInt("Port", 587, 993)
	assert.Equal(t, 587, got)
}

// --- save() ---

func TestSave_CreatesConfigAndDataDir(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	dataDir := filepath.Join(tmpDir, "mydata")
	w, _ := newTestWizard("")
	cfg := &Config{
		General: GeneralConfig{
			DataDir:      dataDir,
			DefaultModel: "sonnet",
		},
		Email: EmailConfig{
			Provider:    "gmail",
			SMTPHost:    "smtp.gmail.com",
			SMTPPort:    587,
			IMAPHost:    "imap.gmail.com",
			IMAPPort:    993,
			User:        "test@gmail.com",
			AppPassword: "test-pass",
		},
	}

	err := w.save(cfg)
	require.NoError(t, err)

	// Config dir created
	configDir := filepath.Join(tmpDir, ".claude-postman")
	info, err := os.Stat(configDir)
	require.NoError(t, err)
	assert.True(t, info.IsDir())

	// Data dir created
	info, err = os.Stat(dataDir)
	require.NoError(t, err)
	assert.True(t, info.IsDir())

	// config.toml created with 0600
	configPath := filepath.Join(configDir, "config.toml")
	info, err = os.Stat(configPath)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0600), info.Mode().Perm())

	// Config can be loaded back
	loaded, err := LoadFrom(configDir)
	require.NoError(t, err)
	assert.Equal(t, "test@gmail.com", loaded.Email.User)
	assert.Equal(t, "sonnet", loaded.General.DefaultModel)
	assert.Equal(t, dataDir, loaded.General.DataDir)
}

// --- Full wizard: fresh setup ---

func TestWizardRun_FreshSetup(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	// Simulate: Enter (default data dir) → 1 (Gmail) → email → password → 1 (Sonnet)
	input := strings.Join([]string{
		"",              // data dir → default
		"1",             // Gmail
		"me@gmail.com",  // email
		"app-pass-1234", // app password
		"1",             // Sonnet
	}, "\n") + "\n"

	w, _ := newTestWizard(input)
	err := w.runWithoutConnTest()
	require.NoError(t, err)

	// Verify saved config
	configDir := filepath.Join(tmpDir, ".claude-postman")
	cfg, err := LoadFrom(configDir)
	require.NoError(t, err)

	assert.Equal(t, "gmail", cfg.Email.Provider)
	assert.Equal(t, "smtp.gmail.com", cfg.Email.SMTPHost)
	assert.Equal(t, 587, cfg.Email.SMTPPort)
	assert.Equal(t, "imap.gmail.com", cfg.Email.IMAPHost)
	assert.Equal(t, 993, cfg.Email.IMAPPort)
	assert.Equal(t, "me@gmail.com", cfg.Email.User)
	assert.Equal(t, "app-pass-1234", cfg.Email.AppPassword)
	assert.Equal(t, "sonnet", cfg.General.DefaultModel)
}

// --- Full wizard: re-run with existing config ---

func TestWizardRun_RerunPreservesExisting(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	// Pre-create a valid config.toml so Load() inside run() finds it
	configDir := filepath.Join(tmpDir, ".claude-postman")
	require.NoError(t, os.MkdirAll(configDir, 0700))

	dataDir := filepath.Join(configDir, "data")
	require.NoError(t, os.MkdirAll(dataDir, 0700))

	writeTestConfig(t, configDir, validConfigTOML(dataDir))

	// Re-run flow: Enter (keep data dir) → Enter (keep email) → N (keep password) → N (keep model)
	input := strings.Join([]string{
		"",  // keep data dir
		"",  // keep email
		"N", // keep password
		"N", // keep model
	}, "\n") + "\n"

	w, _ := newTestWizard(input)
	err := w.runWithoutConnTest()
	require.NoError(t, err)

	// Verify existing values are preserved
	cfg, err := LoadFrom(configDir)
	require.NoError(t, err)

	assert.Equal(t, dataDir, cfg.General.DataDir)
	assert.Equal(t, "gmail", cfg.Email.Provider)
	assert.Equal(t, "test@gmail.com", cfg.Email.User)
	assert.Equal(t, "test-app-password", cfg.Email.AppPassword)
	assert.Equal(t, "sonnet", cfg.General.DefaultModel)
}
