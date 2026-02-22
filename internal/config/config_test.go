package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// writeTestConfig는 임시 디렉터리에 config.toml을 작성하는 헬퍼
func writeTestConfig(t *testing.T, dir, content string) {
	t.Helper()
	err := os.WriteFile(filepath.Join(dir, "config.toml"), []byte(content), 0600)
	require.NoError(t, err)
}

// validConfigTOML은 모든 필수 필드를 포함하는 유효한 TOML 설정을 반환한다.
func validConfigTOML(dataDir string) string {
	return `[general]
data_dir = "` + dataDir + `"
default_model = "sonnet"
poll_interval_sec = 30
session_timeout_min = 30

[email]
provider = "gmail"
smtp_host = "smtp.gmail.com"
smtp_port = 587
imap_host = "imap.gmail.com"
imap_port = 993
user = "test@gmail.com"
app_password = "test-app-password"
`
}

func TestLoadFrom_ValidConfig(t *testing.T) {
	// 유효한 설정 파일로 로드 성공 확인
	dir := t.TempDir()
	dataDir := filepath.Join(dir, "data")
	require.NoError(t, os.MkdirAll(dataDir, 0755))
	writeTestConfig(t, dir, validConfigTOML(dataDir))

	cfg, err := LoadFrom(dir)
	require.NoError(t, err)
	require.NotNil(t, cfg)

	// General 필드 확인
	assert.Equal(t, dataDir, cfg.General.DataDir)
	assert.Equal(t, "sonnet", cfg.General.DefaultModel)
	assert.Equal(t, 30, cfg.General.PollIntervalSec)
	assert.Equal(t, 30, cfg.General.SessionTimeoutMin)

	// Email 필드 확인
	assert.Equal(t, "gmail", cfg.Email.Provider)
	assert.Equal(t, "smtp.gmail.com", cfg.Email.SMTPHost)
	assert.Equal(t, 587, cfg.Email.SMTPPort)
	assert.Equal(t, "imap.gmail.com", cfg.Email.IMAPHost)
	assert.Equal(t, 993, cfg.Email.IMAPPort)
	assert.Equal(t, "test@gmail.com", cfg.Email.User)
	assert.Equal(t, "test-app-password", cfg.Email.AppPassword)
}

func TestLoadFrom_MissingConfigFile(t *testing.T) {
	// config.toml이 없으면 에러 반환
	dir := t.TempDir()

	_, err := LoadFrom(dir)
	require.Error(t, err)
	// 에러 메시지에 init 안내 포함 여부 확인
	assert.Contains(t, err.Error(), "init")
}

func TestLoadFrom_EnvVarOverrides(t *testing.T) {
	// 각 환경변수가 TOML 값을 정상적으로 덮어쓰는지 확인
	dir := t.TempDir()
	dataDir := filepath.Join(dir, "data")
	require.NoError(t, os.MkdirAll(dataDir, 0755))

	overrideDataDir := filepath.Join(dir, "override-data")
	require.NoError(t, os.MkdirAll(overrideDataDir, 0755))

	writeTestConfig(t, dir, validConfigTOML(dataDir))

	tests := []struct {
		name   string
		envKey string
		envVal string
		check  func(*testing.T, *Config)
	}{
		{
			name:   "CLAUDE_POSTMAN_DATA_DIR",
			envKey: "CLAUDE_POSTMAN_DATA_DIR",
			envVal: overrideDataDir,
			check: func(t *testing.T, c *Config) {
				assert.Equal(t, overrideDataDir, c.General.DataDir)
			},
		},
		{
			name:   "CLAUDE_POSTMAN_MODEL",
			envKey: "CLAUDE_POSTMAN_MODEL",
			envVal: "opus",
			check: func(t *testing.T, c *Config) {
				assert.Equal(t, "opus", c.General.DefaultModel)
			},
		},
		{
			name:   "CLAUDE_POSTMAN_POLL_INTERVAL",
			envKey: "CLAUDE_POSTMAN_POLL_INTERVAL",
			envVal: "60",
			check: func(t *testing.T, c *Config) {
				assert.Equal(t, 60, c.General.PollIntervalSec)
			},
		},
		{
			name:   "CLAUDE_POSTMAN_SESSION_TIMEOUT",
			envKey: "CLAUDE_POSTMAN_SESSION_TIMEOUT",
			envVal: "45",
			check: func(t *testing.T, c *Config) {
				assert.Equal(t, 45, c.General.SessionTimeoutMin)
			},
		},
		{
			name:   "CLAUDE_POSTMAN_EMAIL_USER",
			envKey: "CLAUDE_POSTMAN_EMAIL_USER",
			envVal: "override@gmail.com",
			check: func(t *testing.T, c *Config) {
				assert.Equal(t, "override@gmail.com", c.Email.User)
			},
		},
		{
			name:   "CLAUDE_POSTMAN_EMAIL_PASSWORD",
			envKey: "CLAUDE_POSTMAN_EMAIL_PASSWORD",
			envVal: "new-password",
			check: func(t *testing.T, c *Config) {
				assert.Equal(t, "new-password", c.Email.AppPassword)
			},
		},
		{
			name:   "CLAUDE_POSTMAN_SMTP_HOST",
			envKey: "CLAUDE_POSTMAN_SMTP_HOST",
			envVal: "smtp.other.com",
			check: func(t *testing.T, c *Config) {
				assert.Equal(t, "smtp.other.com", c.Email.SMTPHost)
			},
		},
		{
			name:   "CLAUDE_POSTMAN_SMTP_PORT",
			envKey: "CLAUDE_POSTMAN_SMTP_PORT",
			envVal: "465",
			check: func(t *testing.T, c *Config) {
				assert.Equal(t, 465, c.Email.SMTPPort)
			},
		},
		{
			name:   "CLAUDE_POSTMAN_IMAP_HOST",
			envKey: "CLAUDE_POSTMAN_IMAP_HOST",
			envVal: "imap.other.com",
			check: func(t *testing.T, c *Config) {
				assert.Equal(t, "imap.other.com", c.Email.IMAPHost)
			},
		},
		{
			name:   "CLAUDE_POSTMAN_IMAP_PORT",
			envKey: "CLAUDE_POSTMAN_IMAP_PORT",
			envVal: "143",
			check: func(t *testing.T, c *Config) {
				assert.Equal(t, 143, c.Email.IMAPPort)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv(tt.envKey, tt.envVal)

			cfg, err := LoadFrom(dir)
			require.NoError(t, err)
			tt.check(t, cfg)
		})
	}
}

func TestLoadFrom_EnvVarOverwritesToMLValue(t *testing.T) {
	// TOML에 "sonnet"으로 설정된 값을 환경변수 "haiku"로 덮어쓰기 확인
	dir := t.TempDir()
	dataDir := filepath.Join(dir, "data")
	require.NoError(t, os.MkdirAll(dataDir, 0755))
	writeTestConfig(t, dir, validConfigTOML(dataDir))

	t.Setenv("CLAUDE_POSTMAN_MODEL", "haiku")

	cfg, err := LoadFrom(dir)
	require.NoError(t, err)
	assert.Equal(t, "haiku", cfg.General.DefaultModel)
}

func TestLoadFrom_MissingRequiredFields(t *testing.T) {
	// 각 필수 필드가 누락되면 에러를 반환하는지 확인
	tests := []struct {
		name      string
		setupTOML func(t *testing.T, dir string) string
	}{
		{
			name: "data_dir 경로 미존재",
			setupTOML: func(t *testing.T, dir string) string {
				return `[general]
data_dir = "/nonexistent/path/that/does/not/exist"

[email]
smtp_host = "smtp.gmail.com"
imap_host = "imap.gmail.com"
user = "test@gmail.com"
app_password = "test-password"
`
			},
		},
		{
			name: "data_dir 비어있음",
			setupTOML: func(t *testing.T, dir string) string {
				return `[general]

[email]
smtp_host = "smtp.gmail.com"
imap_host = "imap.gmail.com"
user = "test@gmail.com"
app_password = "test-password"
`
			},
		},
		{
			name: "email.user 비어있음",
			setupTOML: func(t *testing.T, dir string) string {
				dataDir := filepath.Join(dir, "data")
				require.NoError(t, os.MkdirAll(dataDir, 0755))
				return `[general]
data_dir = "` + dataDir + `"

[email]
smtp_host = "smtp.gmail.com"
imap_host = "imap.gmail.com"
user = ""
app_password = "test-password"
`
			},
		},
		{
			name: "email.app_password 비어있음",
			setupTOML: func(t *testing.T, dir string) string {
				dataDir := filepath.Join(dir, "data")
				require.NoError(t, os.MkdirAll(dataDir, 0755))
				return `[general]
data_dir = "` + dataDir + `"

[email]
smtp_host = "smtp.gmail.com"
imap_host = "imap.gmail.com"
user = "test@gmail.com"
app_password = ""
`
			},
		},
		{
			name: "email.smtp_host 비어있음",
			setupTOML: func(t *testing.T, dir string) string {
				dataDir := filepath.Join(dir, "data")
				require.NoError(t, os.MkdirAll(dataDir, 0755))
				return `[general]
data_dir = "` + dataDir + `"

[email]
smtp_host = ""
imap_host = "imap.gmail.com"
user = "test@gmail.com"
app_password = "test-password"
`
			},
		},
		{
			name: "email.imap_host 비어있음",
			setupTOML: func(t *testing.T, dir string) string {
				dataDir := filepath.Join(dir, "data")
				require.NoError(t, os.MkdirAll(dataDir, 0755))
				return `[general]
data_dir = "` + dataDir + `"

[email]
smtp_host = "smtp.gmail.com"
imap_host = ""
user = "test@gmail.com"
app_password = "test-password"
`
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			content := tt.setupTOML(t, dir)
			writeTestConfig(t, dir, content)

			_, err := LoadFrom(dir)
			require.Error(t, err)
		})
	}
}

func TestLoadFrom_DefaultsApplied(t *testing.T) {
	// poll_interval_sec, session_timeout_min, default_model을 생략하면 기본값 적용
	dir := t.TempDir()
	dataDir := filepath.Join(dir, "data")
	require.NoError(t, os.MkdirAll(dataDir, 0755))

	// 기본값이 적용되어야 하는 필드를 생략한 최소 설정
	content := `[general]
data_dir = "` + dataDir + `"

[email]
smtp_host = "smtp.gmail.com"
imap_host = "imap.gmail.com"
user = "test@gmail.com"
app_password = "test-password"
`
	writeTestConfig(t, dir, content)

	cfg, err := LoadFrom(dir)
	require.NoError(t, err)

	assert.Equal(t, 30, cfg.General.PollIntervalSec, "poll_interval_sec 기본값은 30")
	assert.Equal(t, 30, cfg.General.SessionTimeoutMin, "session_timeout_min 기본값은 30")
	assert.Equal(t, "sonnet", cfg.General.DefaultModel, "default_model 기본값은 sonnet")
}

func TestConfigDir(t *testing.T) {
	// ConfigDir은 ~/.claude-postman 경로를 반환해야 함
	home, err := os.UserHomeDir()
	require.NoError(t, err)

	expected := filepath.Join(home, ".claude-postman")
	assert.Equal(t, expected, ConfigDir())
}
