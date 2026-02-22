package config

import (
	"bufio"
	"crypto/tls"
	"fmt"
	"io"
	"net/smtp"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/emersion/go-imap/v2/imapclient"
)

// initWizard holds the I/O dependencies for the init wizard.
type initWizard struct {
	in         *bufio.Scanner
	out        io.Writer
	connTester func(*Config) // nil이면 기본 testConnection 사용
}

// providerHelp는 프로바이더별 앱 비밀번호 발급 안내 메시지
var providerHelp = map[string]string{
	"gmail": `  ┌─ Help ─────────────────────────────────────────┐
  │ How to get a Gmail App Password:               │
  │                                                │
  │ 1. Enable 2-Step Verification:                 │
  │    Google Account > Security > 2-Step Verify   │
  │ 2. Create App Password:                        │
  │    https://myaccount.google.com/apppasswords   │
  │ 3. Enter app name > Copy the 16-char password  │
  └────────────────────────────────────────────────┘`,
	"outlook": `  ┌─ Help ─────────────────────────────────────────┐
  │ How to get an Outlook App Password:            │
  │                                                │
  │ 1. Enable 2-Step Verification:                 │
  │    Microsoft Account > Security > Advanced     │
  │ 2. Create App Password:                        │
  │    Security > App Passwords > Create           │
  │ 3. Copy the generated password                 │
  └────────────────────────────────────────────────┘`,
}

func (w *initWizard) printf(format string, args ...any) {
	fmt.Fprintf(w.out, format, args...)
}

func (w *initWizard) readLine() string {
	if w.in.Scan() {
		return strings.TrimSpace(w.in.Text())
	}
	return ""
}

// prompt shows a prompt and returns user input. If input is empty, returns defaultVal.
func (w *initWizard) prompt(label, defaultVal string) string {
	if defaultVal != "" {
		w.printf("  %s (default: %s)\n  > ", label, defaultVal)
	} else {
		w.printf("  %s\n  > ", label)
	}
	input := w.readLine()
	if input == "" {
		return defaultVal
	}
	return input
}

// promptSecret shows a prompt for sensitive input. Shows [unchanged] if defaultVal is set.
func (w *initWizard) promptSecret(label, defaultVal string) string {
	if defaultVal != "" {
		w.printf("  %s [unchanged]\n  Change? (y/N) > ", label)
		if answer := w.readLine(); answer != "y" && answer != "Y" {
			return defaultVal
		}
	}
	w.printf("  %s\n  > ", label)
	return w.readLine()
}

// promptChoice shows a numbered menu and returns the selected index (0-based).
func (w *initWizard) promptChoice(options []string, defaultIdx int) int {
	for i, opt := range options {
		w.printf("  (%d) %s\n", i+1, opt)
	}
	w.printf("  > ")
	input := w.readLine()
	if input == "" {
		return defaultIdx
	}
	n, err := strconv.Atoi(input)
	if err != nil || n < 1 || n > len(options) {
		return defaultIdx
	}
	return n - 1
}

// promptInt reads an integer with a default value.
func (w *initWizard) promptInt(label string, current, fallback int) int {
	def := current
	if def == 0 {
		def = fallback
	}
	s := w.prompt(label, strconv.Itoa(def))
	n, err := strconv.Atoi(s)
	if err != nil {
		return def
	}
	return n
}

// run은 3단계 마법사 흐름을 실행한다.
func (w *initWizard) run() error {
	w.printf("\nClaude Postman Setup\n")
	w.printf("====================\n\n")

	// Load existing config if present
	existing, _ := Load()
	if existing != nil {
		w.printf("Existing config found. Values shown as defaults.\n\n")
	}

	cfg := &Config{}
	if existing != nil {
		*cfg = *existing
	}

	// [1/3] Data Directory
	w.printf("[1/3] Data Directory\n")
	defaultDataDir := cfg.General.DataDir
	if defaultDataDir == "" {
		defaultDataDir = filepath.Join(ConfigDir(), "data")
	}
	cfg.General.DataDir = w.prompt("Where should claude-postman store its data?", defaultDataDir)

	// [2/3] Email Account
	w.printf("\n[2/3] Email Account\n")
	w.stepEmail(cfg, existing)

	// [3/3] Default Model
	w.printf("\n[3/3] Default Model\n")
	w.stepModel(cfg, existing)

	// Save
	if err := w.save(cfg); err != nil {
		return err
	}

	// Connection test
	if w.connTester != nil {
		w.connTester(cfg)
	} else {
		w.printf("\nTesting email connection...\n")
		w.testConnection(cfg)
	}

	w.printf("\nRun 'claude-postman serve' to start.\n")
	return nil
}

// runWithoutConnTest는 연결 테스트 없이 실행 (테스트용)
func (w *initWizard) runWithoutConnTest() error {
	w.connTester = func(_ *Config) {} // no-op
	return w.run()
}

func (w *initWizard) stepEmail(cfg *Config, existing *Config) {
	w.printf("  Select your email provider:\n")
	providers := []string{"Gmail", "Outlook", "Other (manual setup)"}
	providerKeys := []string{"gmail", "outlook", "other"}

	defaultIdx := 0
	if existing != nil {
		for i, k := range providerKeys {
			if k == existing.Email.Provider {
				defaultIdx = i
				break
			}
		}
	}

	idx := w.promptChoice(providers, defaultIdx)
	key := providerKeys[idx]
	cfg.Email.Provider = key

	if preset, ok := Presets[key]; ok {
		cfg.Email.SMTPHost = preset.SMTPHost
		cfg.Email.SMTPPort = preset.SMTPPort
		cfg.Email.IMAPHost = preset.IMAPHost
		cfg.Email.IMAPPort = preset.IMAPPort
		w.printf("\n  ✓ SMTP: %s:%d\n", cfg.Email.SMTPHost, cfg.Email.SMTPPort)
		w.printf("  ✓ IMAP: %s:%d\n\n", cfg.Email.IMAPHost, cfg.Email.IMAPPort)
	} else {
		// Manual setup
		cfg.Email.SMTPHost = w.prompt("SMTP host", cfg.Email.SMTPHost)
		cfg.Email.SMTPPort = w.promptInt("SMTP port", cfg.Email.SMTPPort, 587)
		cfg.Email.IMAPHost = w.prompt("IMAP host", cfg.Email.IMAPHost)
		cfg.Email.IMAPPort = w.promptInt("IMAP port", cfg.Email.IMAPPort, 993)
		w.printf("\n")
	}

	cfg.Email.User = w.prompt("Email address", cfg.Email.User)

	// Show help for known providers
	if help, ok := providerHelp[key]; ok {
		w.printf("\n%s\n\n", help)
	}

	existingPassword := ""
	if existing != nil {
		existingPassword = existing.Email.AppPassword
	}
	cfg.Email.AppPassword = w.promptSecret("App password", existingPassword)
}

func (w *initWizard) stepModel(cfg *Config, existing *Config) {
	models := []string{
		"Sonnet  - balanced speed and quality",
		"Opus    - highest quality",
		"Haiku   - fastest",
	}
	modelKeys := []string{"sonnet", "opus", "haiku"}

	defaultIdx := 0
	if existing != nil {
		for i, k := range modelKeys {
			if k == existing.General.DefaultModel {
				defaultIdx = i
				break
			}
		}
	}

	w.printf("  Which Claude model to use by default?\n")
	w.printf("  (Sessions can override this per request)\n")
	idx := w.promptChoice(models, defaultIdx)
	cfg.General.DefaultModel = modelKeys[idx]
}

func (w *initWizard) save(cfg *Config) error {
	// Apply defaults for fields not asked in wizard
	applyDefaults(cfg)

	configDir := ConfigDir()
	if err := os.MkdirAll(configDir, 0700); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}
	if err := os.MkdirAll(cfg.General.DataDir, 0700); err != nil {
		return fmt.Errorf("create data dir: %w", err)
	}

	path := filepath.Join(configDir, "config.toml")
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return fmt.Errorf("create config file: %w", err)
	}
	defer f.Close()

	if err := toml.NewEncoder(f).Encode(cfg); err != nil {
		return fmt.Errorf("encode config: %w", err)
	}

	w.printf("\n✅ Config saved: %s\n", path)
	w.printf("✅ Data directory created: %s\n", cfg.General.DataDir)
	return nil
}

func (w *initWizard) testConnection(cfg *Config) {
	// SMTP test
	smtpAddr := fmt.Sprintf("%s:%d", cfg.Email.SMTPHost, cfg.Email.SMTPPort)
	w.printf("  SMTP: %s ... ", smtpAddr)
	if err := testSMTP(cfg); err != nil {
		w.printf("✗ %v\n", err)
	} else {
		w.printf("✅ connected\n")
	}

	// IMAP test
	imapAddr := fmt.Sprintf("%s:%d", cfg.Email.IMAPHost, cfg.Email.IMAPPort)
	w.printf("  IMAP: %s ... ", imapAddr)
	if err := testIMAP(cfg); err != nil {
		w.printf("✗ %v\n", err)
	} else {
		w.printf("✅ connected\n")
	}
}

func testSMTP(cfg *Config) error {
	addr := fmt.Sprintf("%s:%d", cfg.Email.SMTPHost, cfg.Email.SMTPPort)
	c, err := smtp.Dial(addr)
	if err != nil {
		return err
	}
	defer c.Close()
	if err := c.StartTLS(&tls.Config{ServerName: cfg.Email.SMTPHost}); err != nil {
		return err
	}
	if err := c.Auth(smtp.PlainAuth("", cfg.Email.User, cfg.Email.AppPassword, cfg.Email.SMTPHost)); err != nil {
		return err
	}
	return c.Quit()
}

func testIMAP(cfg *Config) error {
	addr := fmt.Sprintf("%s:%d", cfg.Email.IMAPHost, cfg.Email.IMAPPort)
	c, err := imapclient.DialTLS(addr, nil)
	if err != nil {
		return err
	}
	defer c.Close()
	return c.Login(cfg.Email.User, cfg.Email.AppPassword).Wait()
}
