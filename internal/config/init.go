package config

import (
	"crypto/tls"
	"fmt"
	"io"
	"net/smtp"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/charmbracelet/huh"
	"github.com/emersion/go-imap/v2/imapclient"
)

// initWizard holds the I/O dependencies for the init wizard.
type initWizard struct {
	in         io.Reader
	out        io.Writer
	accessible bool          // true = accessible mode (테스트용)
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

func (w *initWizard) runForm(f *huh.Form) error {
	return f.WithInput(w.in).WithOutput(w.out).WithAccessible(w.accessible).Run()
}

// byteReader wraps an io.Reader to return at most one byte per Read call.
// This prevents bufio.Scanner (used by huh's accessible mode) from buffering
// ahead when multiple forms share the same underlying reader.
type byteReader struct {
	r io.Reader
}

func (br *byteReader) Read(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	return br.r.Read(p[:1])
}

func (w *initWizard) newPasswordInput() *huh.Input {
	input := huh.NewInput()
	if !w.accessible {
		input.EchoMode(huh.EchoModePassword)
	}
	return input
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
	if err := w.stepDataDir(cfg); err != nil {
		return err
	}

	// [2/3] Email Account
	w.printf("\n[2/3] Email Account\n")
	if err := w.stepEmail(cfg, existing); err != nil {
		return err
	}

	// [3/3] Default Model
	w.printf("\n[3/3] Default Model\n")
	if err := w.stepModel(cfg, existing); err != nil {
		return err
	}

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

func (w *initWizard) stepDataDir(cfg *Config) error {
	if cfg.General.DataDir == "" {
		cfg.General.DataDir = filepath.Join(ConfigDir(), "data")
	}
	return w.runForm(huh.NewForm(
		huh.NewGroup(
			huh.NewInput().
				Title("Where should claude-postman store its data?").
				Value(&cfg.General.DataDir),
		),
	))
}

func (w *initWizard) stepEmail(cfg *Config, existing *Config) error {
	if existing != nil {
		return w.stepEmailRerun(cfg, existing)
	}
	return w.stepEmailFresh(cfg)
}

func (w *initWizard) stepEmailFresh(cfg *Config) error {
	providerKey, err := w.selectProvider(cfg)
	if err != nil {
		return err
	}

	// Email address
	if err := w.runForm(huh.NewForm(
		huh.NewGroup(
			huh.NewInput().Title("Email address").Value(&cfg.Email.User),
		),
	)); err != nil {
		return err
	}

	// Show help for known providers
	if help, ok := providerHelp[providerKey]; ok {
		w.printf("\n%s\n\n", help)
	}

	// App password
	return w.runForm(huh.NewForm(
		huh.NewGroup(
			w.newPasswordInput().Title("App password").Value(&cfg.Email.AppPassword),
		),
	))
}

func (w *initWizard) selectProvider(cfg *Config) (string, error) {
	var providerKey string
	err := w.runForm(huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[string]().
				Title("Select your email provider").
				Options(
					huh.NewOption("Gmail", "gmail"),
					huh.NewOption("Outlook", "outlook"),
					huh.NewOption("Other (manual setup)", "other"),
				).
				Value(&providerKey),
		),
	))
	if err != nil {
		return "", err
	}
	cfg.Email.Provider = providerKey

	if preset, ok := Presets[providerKey]; ok {
		cfg.Email.SMTPHost = preset.SMTPHost
		cfg.Email.SMTPPort = preset.SMTPPort
		cfg.Email.IMAPHost = preset.IMAPHost
		cfg.Email.IMAPPort = preset.IMAPPort
		w.printf("\n  ✓ SMTP: %s:%d\n", cfg.Email.SMTPHost, cfg.Email.SMTPPort)
		w.printf("  ✓ IMAP: %s:%d\n\n", cfg.Email.IMAPHost, cfg.Email.IMAPPort)
	} else {
		if err := w.inputManualServer(cfg); err != nil {
			return "", err
		}
	}
	return providerKey, nil
}

func (w *initWizard) inputManualServer(cfg *Config) error {
	smtpPort := "587"
	imapPort := "993"
	if err := w.runForm(huh.NewForm(
		huh.NewGroup(
			huh.NewInput().Title("SMTP host").Value(&cfg.Email.SMTPHost),
			huh.NewInput().Title("SMTP port").Value(&smtpPort),
			huh.NewInput().Title("IMAP host").Value(&cfg.Email.IMAPHost),
			huh.NewInput().Title("IMAP port").Value(&imapPort),
		),
	)); err != nil {
		return err
	}
	if n, err := strconv.Atoi(smtpPort); err == nil {
		cfg.Email.SMTPPort = n
	} else {
		cfg.Email.SMTPPort = 587
	}
	if n, err := strconv.Atoi(imapPort); err == nil {
		cfg.Email.IMAPPort = n
	} else {
		cfg.Email.IMAPPort = 993
	}
	w.printf("\n")
	return nil
}

func (w *initWizard) stepEmailRerun(cfg *Config, existing *Config) error {
	providerName := strings.ToUpper(existing.Email.Provider[:1]) + existing.Email.Provider[1:]
	w.printf("  Provider: %s\n", providerName)

	// Email (default: existing value via cfg copy)
	if err := w.runForm(huh.NewForm(
		huh.NewGroup(
			huh.NewInput().Title("Email").Value(&cfg.Email.User),
		),
	)); err != nil {
		return err
	}

	// Password: confirm change
	changePassword := false
	if err := w.runForm(huh.NewForm(
		huh.NewGroup(
			huh.NewConfirm().
				Title("App password [unchanged] Change?").
				Value(&changePassword),
		),
	)); err != nil {
		return err
	}
	if changePassword {
		cfg.Email.AppPassword = ""
		return w.runForm(huh.NewForm(
			huh.NewGroup(
				w.newPasswordInput().Title("App password").Value(&cfg.Email.AppPassword),
			),
		))
	}
	return nil
}

func (w *initWizard) stepModel(cfg *Config, existing *Config) error {
	if existing != nil {
		currentName := strings.ToUpper(existing.General.DefaultModel[:1]) + existing.General.DefaultModel[1:]
		w.printf("  Current: %s\n", currentName)

		change := false
		if err := w.runForm(huh.NewForm(
			huh.NewGroup(
				huh.NewConfirm().Title("Change?").Value(&change),
			),
		)); err != nil {
			return err
		}
		if !change {
			return nil
		}
	}

	w.printf("  (Sessions can override this per request)\n")
	return w.runForm(huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[string]().
				Title("Which Claude model to use by default?").
				Options(
					huh.NewOption("Sonnet - balanced speed and quality", "sonnet"),
					huh.NewOption("Opus - highest quality", "opus"),
					huh.NewOption("Haiku - fastest", "haiku"),
				).
				Value(&cfg.General.DefaultModel),
		),
	))
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
	if err := c.StartTLS(&tls.Config{ServerName: cfg.Email.SMTPHost, MinVersion: tls.VersionTLS12}); err != nil { //nolint:gosec // TLS 1.2 minimum
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
