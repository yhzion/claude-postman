// Package config handles configuration loading, validation, and init wizard.
package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"

	"github.com/BurntSushi/toml"
)

// Config는 전체 설정을 나타내는 구조체
type Config struct {
	General GeneralConfig `toml:"general"`
	Email   EmailConfig   `toml:"email"`
}

// GeneralConfig는 일반 설정
type GeneralConfig struct {
	DataDir           string `toml:"data_dir"`
	DefaultModel      string `toml:"default_model"`
	PollIntervalSec   int    `toml:"poll_interval_sec"`
	SessionTimeoutMin int    `toml:"session_timeout_min"`
}

// EmailConfig는 이메일 관련 설정
type EmailConfig struct {
	Provider    string `toml:"provider"`
	SMTPHost    string `toml:"smtp_host"`
	SMTPPort    int    `toml:"smtp_port"`
	IMAPHost    string `toml:"imap_host"`
	IMAPPort    int    `toml:"imap_port"`
	User        string `toml:"user"`
	AppPassword string `toml:"app_password"`
}

// Load는 기본 설정 디렉터리에서 설정을 로드한다.
func Load() (*Config, error) {
	return LoadFrom(ConfigDir()) //nolint:revive // SSOT에서 ConfigDir로 정의
}

// LoadFrom은 지정된 디렉터리에서 config.toml을 읽어 설정을 로드한다.
func LoadFrom(configDir string) (*Config, error) {
	path := filepath.Join(configDir, "config.toml")

	var cfg Config
	if _, err := toml.DecodeFile(path, &cfg); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("config file not found: run 'claude-postman init' to create one")
		}
		return nil, fmt.Errorf("failed to parse config: %w", err)
	}

	applyDefaults(&cfg)
	applyEnvOverrides(&cfg)

	if err := validate(&cfg); err != nil {
		return nil, err
	}

	return &cfg, nil
}

// ConfigDir은 설정 디렉터리 경로(~/.claude-postman)를 반환한다.
//
//nolint:revive // SSOT 아키텍처 문서에서 ConfigDir로 정의
func ConfigDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".claude-postman")
}

// RunInit는 대화형 설정 마법사를 실행한다.
func RunInit() error {
	return errors.New("not implemented")
}

func applyDefaults(cfg *Config) {
	if cfg.General.PollIntervalSec == 0 {
		cfg.General.PollIntervalSec = 30
	}
	if cfg.General.SessionTimeoutMin == 0 {
		cfg.General.SessionTimeoutMin = 30
	}
	if cfg.General.DefaultModel == "" {
		cfg.General.DefaultModel = "sonnet"
	}
}

func applyEnvOverrides(cfg *Config) {
	envStr("CLAUDE_POSTMAN_DATA_DIR", &cfg.General.DataDir)
	envStr("CLAUDE_POSTMAN_MODEL", &cfg.General.DefaultModel)
	envInt("CLAUDE_POSTMAN_POLL_INTERVAL", &cfg.General.PollIntervalSec)
	envInt("CLAUDE_POSTMAN_SESSION_TIMEOUT", &cfg.General.SessionTimeoutMin)
	envStr("CLAUDE_POSTMAN_EMAIL_USER", &cfg.Email.User)
	envStr("CLAUDE_POSTMAN_EMAIL_PASSWORD", &cfg.Email.AppPassword)
	envStr("CLAUDE_POSTMAN_SMTP_HOST", &cfg.Email.SMTPHost)
	envInt("CLAUDE_POSTMAN_SMTP_PORT", &cfg.Email.SMTPPort)
	envStr("CLAUDE_POSTMAN_IMAP_HOST", &cfg.Email.IMAPHost)
	envInt("CLAUDE_POSTMAN_IMAP_PORT", &cfg.Email.IMAPPort)
}

func envStr(key string, dst *string) {
	if v := os.Getenv(key); v != "" {
		*dst = v
	}
}

func envInt(key string, dst *int) {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			*dst = n
		}
	}
}

func validate(cfg *Config) error {
	if cfg.General.DataDir == "" {
		return errors.New("general.data_dir is required")
	}
	if info, err := os.Stat(cfg.General.DataDir); err != nil || !info.IsDir() {
		return fmt.Errorf("general.data_dir does not exist or is not a directory: %s", cfg.General.DataDir)
	}
	if cfg.Email.User == "" {
		return errors.New("email.user is required")
	}
	if cfg.Email.AppPassword == "" {
		return errors.New("email.app_password is required")
	}
	if cfg.Email.SMTPHost == "" {
		return errors.New("email.smtp_host is required")
	}
	if cfg.Email.IMAPHost == "" {
		return errors.New("email.imap_host is required")
	}
	return nil
}
