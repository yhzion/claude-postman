package config

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newTestWizard는 테스트용 initWizard를 생성한다.
// huh accessible 모드로 실행하여 string reader 기반 입력을 사용한다.
func newTestWizard(input string) (*initWizard, *bytes.Buffer) {
	out := &bytes.Buffer{}
	return &initWizard{
		in:         &byteReader{strings.NewReader(input)},
		out:        out,
		accessible: true,
	}, out
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

	// huh accessible mode:
	// Input: empty → default, text → value
	// Select: 1-based number
	// Confirm: y/n
	input := strings.Join([]string{
		"",              // data dir → default
		"1",             // Gmail (Select: 1-based)
		"me@gmail.com",  // email
		"app-pass-1234", // app password
		"1",             // Sonnet (Select: 1-based)
	}, "\n") + "\n"

	w, _ := newTestWizard(input)
	_, err := w.runWithoutConnTest()
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

	// huh accessible mode:
	// Input empty → keeps existing (default)
	// Confirm: n → false (keep existing)
	input := strings.Join([]string{
		"",  // keep data dir (Input: empty → default)
		"",  // keep email (Input: empty → default)
		"n", // keep password (Confirm: n → false)
		"n", // keep model (Confirm: n → false)
	}, "\n") + "\n"

	w, _ := newTestWizard(input)
	_, err := w.runWithoutConnTest()
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
