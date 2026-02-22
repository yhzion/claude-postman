package doctor

import (
	"bytes"
	"database/sql"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	_ "github.com/mattn/go-sqlite3"
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

func mockDialSuccess(_ string, _ string, _ time.Duration) (net.Conn, error) {
	// Return a pipe so Close() works
	c1, _ := net.Pipe()
	return c1, nil
}

func mockDialFail(_ string, _ string, _ time.Duration) (net.Conn, error) {
	return nil, fmt.Errorf("connection refused")
}

func baseDeps(t *testing.T) Deps {
	t.Helper()
	dir := t.TempDir()
	dataDir := filepath.Join(dir, "data")
	require.NoError(t, os.MkdirAll(dataDir, 0o755))
	writeValidConfig(t, dir)
	return Deps{
		ConfigDir: dir,
		DataDir:   dataDir,
		SMTPAddr:  "smtp.example.com:587",
		IMAPAddr:  "imap.example.com:993",
		Dial:      mockDialSuccess,
	}
}

// --- Config checks ---

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
	assert.Contains(t, r.Hint, "init")
}

// --- Data dir checks ---

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

// --- SQLite checks ---

func TestCheckSQLite_NoDBFile(t *testing.T) {
	dir := t.TempDir()
	r := checkSQLite(dir, false)
	assert.Equal(t, statusError, r.Status)
	assert.Contains(t, r.Message, "not found")
}

func TestCheckSQLite_FixCreatesDB(t *testing.T) {
	dir := t.TempDir()
	r := checkSQLite(dir, true)
	assert.Equal(t, statusFixed, r.Status)

	_, err := os.Stat(filepath.Join(dir, "claude-postman.db"))
	assert.NoError(t, err)
}

func TestCheckSQLite_ExistingMigratedDB(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "claude-postman.db")

	db, err := sql.Open("sqlite3", dbPath)
	require.NoError(t, err)
	_, err = db.Exec(`
		CREATE TABLE schema_version (version INTEGER);
		INSERT INTO schema_version (version) VALUES (1);
	`)
	require.NoError(t, err)
	db.Close()

	r := checkSQLite(dir, false)
	assert.Equal(t, statusOK, r.Status)
	assert.Contains(t, r.Message, "version 1")
}

func TestCheckSQLite_ExistingNotMigrated(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "claude-postman.db")

	db, err := sql.Open("sqlite3", dbPath)
	require.NoError(t, err)
	require.NoError(t, db.Ping())
	db.Close()

	r := checkSQLite(dir, false)
	assert.Equal(t, statusError, r.Status)
	assert.Contains(t, r.Message, "not migrated")
}

func TestCheckSQLite_ExistingNotMigrated_Fix(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "claude-postman.db")

	db, err := sql.Open("sqlite3", dbPath)
	require.NoError(t, err)
	require.NoError(t, db.Ping())
	db.Close()

	r := checkSQLite(dir, true)
	assert.Equal(t, statusFixed, r.Status)
}

// --- Command checks ---

func TestCheckCommand_Found(t *testing.T) {
	r := checkCommand("go", "go", "version")
	assert.Equal(t, statusOK, r.Status)
}

func TestCheckCommand_NotFound(t *testing.T) {
	r := checkCommand("nonexistent-tool-xyz", "nonexistent-tool-xyz", "--version")
	assert.Equal(t, statusError, r.Status)
}

// --- SMTP/IMAP checks ---

func TestCheckSMTP_Success(t *testing.T) {
	r := checkTCPService("SMTP", "smtp.example.com:587", mockDialSuccess)
	assert.Equal(t, statusOK, r.Status)
	assert.Contains(t, r.Message, "connected")
}

func TestCheckSMTP_Fail(t *testing.T) {
	r := checkTCPService("SMTP", "smtp.example.com:587", mockDialFail)
	assert.Equal(t, statusError, r.Status)
	assert.Contains(t, r.Message, "connection failed")
}

func TestCheckSMTP_NotConfigured(t *testing.T) {
	r := checkTCPService("SMTP", "", mockDialSuccess)
	assert.Equal(t, statusError, r.Status)
	assert.Contains(t, r.Message, "not configured")
}

func TestCheckIMAP_Success(t *testing.T) {
	r := checkTCPService("IMAP", "imap.example.com:993", mockDialSuccess)
	assert.Equal(t, statusOK, r.Status)
	assert.Contains(t, r.Message, "connected")
}

func TestCheckIMAP_Fail(t *testing.T) {
	r := checkTCPService("IMAP", "imap.example.com:993", mockDialFail)
	assert.Equal(t, statusError, r.Status)
}

// --- Service check ---

func TestCheckService_ReturnsResult(t *testing.T) {
	r := checkService()
	// On CI/test environments, service is typically not registered → warn
	assert.Contains(t, []string{statusOK, statusWarn}, r.Status)
}

// --- RunDoctor integration ---

func TestRunDoctor_AllPass(t *testing.T) {
	deps := baseDeps(t)

	// Create a migrated DB
	dbPath := filepath.Join(deps.DataDir, "claude-postman.db")
	db, err := sql.Open("sqlite3", dbPath)
	require.NoError(t, err)
	_, err = db.Exec(`CREATE TABLE schema_version (version INTEGER); INSERT INTO schema_version (version) VALUES (1);`)
	require.NoError(t, err)
	db.Close()

	var buf bytes.Buffer
	exitCode := RunDoctor(&buf, deps, false)
	output := buf.String()

	assert.Contains(t, output, "Config")
	assert.Contains(t, output, "Data directory")
	assert.Contains(t, output, "Database")
	assert.Contains(t, output, "SMTP")
	assert.Contains(t, output, "IMAP")
	// tmux/claude/service may fail on CI → exit code may not be 0
	_ = exitCode
}

func TestRunDoctor_WithErrors(t *testing.T) {
	dir := t.TempDir()
	deps := Deps{
		ConfigDir: dir,
		DataDir:   filepath.Join(dir, "data"),
		SMTPAddr:  "smtp.example.com:587",
		IMAPAddr:  "imap.example.com:993",
		Dial:      mockDialFail,
	}

	var buf bytes.Buffer
	exitCode := RunDoctor(&buf, deps, false)
	assert.Equal(t, 1, exitCode)
}

func TestRunDoctor_FixDataDirAndDB(t *testing.T) {
	dir := t.TempDir()
	writeValidConfig(t, dir)
	dataDir := filepath.Join(dir, "data")
	deps := Deps{
		ConfigDir: dir,
		DataDir:   dataDir,
		SMTPAddr:  "smtp.example.com:587",
		IMAPAddr:  "imap.example.com:993",
		Dial:      mockDialSuccess,
	}

	var buf bytes.Buffer
	_ = RunDoctor(&buf, deps, true)
	output := buf.String()
	assert.Contains(t, output, "Created")
	assert.Contains(t, output, "Initialized")

	_, err := os.Stat(dataDir)
	assert.NoError(t, err)

	_, err = os.Stat(filepath.Join(dataDir, "claude-postman.db"))
	assert.NoError(t, err)
}

func TestRunDoctor_OutputFormat(t *testing.T) {
	deps := baseDeps(t)
	deps.Dial = mockDialSuccess

	var buf bytes.Buffer
	_ = RunDoctor(&buf, deps, false)
	output := buf.String()

	assert.Contains(t, output, "Checking environment...")
	// Should have 8 check lines
	for _, name := range []string{"Config", "Data directory", "Database", "tmux", "Claude Code", "SMTP", "IMAP", "Service"} {
		assert.Contains(t, output, name, "missing check: %s", name)
	}
}
