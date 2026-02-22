package doctor

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func writeValidConfig(t *testing.T, dir string) {
	t.Helper()
	content := `[general]
data_dir = "` + filepath.Join(dir, "data") + `"

[email]
user = "test@example.com"
app_password = "secret"
smtp_host = "smtp.example.com"
smtp_port = 587
imap_host = "imap.example.com"
imap_port = 993
`
	require.NoError(t, os.WriteFile(filepath.Join(dir, "config.toml"), []byte(content), 0o600))
}

func TestCheckConfig_Exists(t *testing.T) {
	dir := t.TempDir()
	writeValidConfig(t, dir)
	r := checkConfig(dir)
	assert.Equal(t, statusOK, r.Status)
}

func TestCheckConfig_Missing(t *testing.T) {
	dir := t.TempDir()
	r := checkConfig(dir)
	assert.Equal(t, statusError, r.Status)
}

func TestCheckDataDir_Exists(t *testing.T) {
	dir := t.TempDir()
	dataDir := filepath.Join(dir, "data")
	require.NoError(t, os.MkdirAll(dataDir, 0o755))
	r := checkDataDir(dataDir, false)
	assert.Equal(t, statusOK, r.Status)
}

func TestCheckDataDir_Missing(t *testing.T) {
	dir := t.TempDir()
	dataDir := filepath.Join(dir, "data")
	r := checkDataDir(dataDir, false)
	assert.Equal(t, statusError, r.Status)
}

func TestCheckDataDir_FixCreates(t *testing.T) {
	dir := t.TempDir()
	dataDir := filepath.Join(dir, "data")
	r := checkDataDir(dataDir, true)
	assert.Equal(t, statusFixed, r.Status)

	info, err := os.Stat(dataDir)
	require.NoError(t, err)
	assert.True(t, info.IsDir())
}

func TestCheckCommand_Found(t *testing.T) {
	// "go" should be available in test environment
	r := checkCommand("go", "go", "version")
	assert.Equal(t, statusOK, r.Status)
}

func TestCheckCommand_NotFound(t *testing.T) {
	r := checkCommand("nonexistent-tool-xyz", "nonexistent-tool-xyz", "--version")
	assert.Equal(t, statusError, r.Status)
}

func TestRunDoctor_AllPass(t *testing.T) {
	dir := t.TempDir()
	dataDir := filepath.Join(dir, "data")
	require.NoError(t, os.MkdirAll(dataDir, 0o755))
	writeValidConfig(t, dir)

	var buf bytes.Buffer
	exitCode := RunDoctor(&buf, dir, dataDir, false)
	assert.Equal(t, 0, exitCode)
}

func TestRunDoctor_WithErrors(t *testing.T) {
	dir := t.TempDir()
	// No config, no data dir → errors expected
	var buf bytes.Buffer
	exitCode := RunDoctor(&buf, dir, filepath.Join(dir, "data"), false)
	assert.Equal(t, 1, exitCode)
}

func TestRunDoctor_FixDataDir(t *testing.T) {
	dir := t.TempDir()
	dataDir := filepath.Join(dir, "data")
	writeValidConfig(t, dir)

	var buf bytes.Buffer
	_ = RunDoctor(&buf, dir, dataDir, true)
	// Data dir should be fixed; remaining errors (tmux, claude, email, service) may cause exit 1
	output := buf.String()
	assert.Contains(t, output, "Created")

	_, err := os.Stat(dataDir)
	assert.NoError(t, err)
}
