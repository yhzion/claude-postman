# Init Wizard Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** `claude-postman init` 대화형 설정 마법사를 구현하여 사용자가 터미널에서 config.toml을 생성할 수 있게 한다.

**Architecture:** `internal/config/init.go`에 `RunInit()` 구현. stdin/stdout을 인터페이스로 추상화하여 테스트 가능하게 한다. 기존 `config.go`의 구조체/프리셋을 재사용. 이메일 연결 테스트는 `net/smtp`와 `imapclient.DialTLS`를 직접 호출 (기존 email 패키지의 실 SMTP/IMAP 클라이언트는 storage 의존성이 있어 init에서 직접 사용하기 어려움).

**Tech Stack:** Go, `BurntSushi/toml` (TOML 인코딩), `emersion/go-imap/v2` (IMAP 테스트), `net/smtp` (SMTP 테스트), `bufio` (stdin 읽기)

---

### Task 1: I/O 추상화와 프롬프트 헬퍼

**Files:**
- Create: `internal/config/init.go`

**Step 1: init.go 기본 구조 작성**

`RunInit()` 함수와 내부에서 사용할 I/O 헬퍼를 작성한다.

```go
package config

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/BurntSushi/toml"
)

// initWizard holds the I/O dependencies for the init wizard.
type initWizard struct {
	in  *bufio.Scanner
	out io.Writer
}

// RunInit는 대화형 설정 마법사를 실행한다.
func RunInit() error {
	w := &initWizard{
		in:  bufio.NewScanner(os.Stdin),
		out: os.Stdout,
	}
	return w.run()
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
```

**Step 2: Commit**

```bash
git add internal/config/init.go
git commit -m "feat(init): add I/O abstraction and prompt helpers for init wizard"
```

---

### Task 2: 마법사 메인 흐름 (run)

**Files:**
- Modify: `internal/config/init.go`

**Step 1: run() 메서드 구현**

3단계 마법사 흐름: Data Directory → Email Account → Default Model → 저장.

```go
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
		defaultDataDir = ConfigDir() + "/data"
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
	w.printf("\nTesting email connection...\n")
	w.testConnection(cfg)

	w.printf("\nRun 'claude-postman serve' to start.\n")
	return nil
}
```

**Step 2: Commit**

```bash
git add internal/config/init.go
git commit -m "feat(init): add main wizard flow (run method)"
```

---

### Task 3: 이메일 설정 단계 (stepEmail)

**Files:**
- Modify: `internal/config/init.go`

**Step 1: stepEmail, stepModel, 프로바이더 도움말 구현**

```go
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
```

**Step 2: Commit**

```bash
git add internal/config/init.go
git commit -m "feat(init): add email and model setup steps with provider presets"
```

---

### Task 4: 설정 저장 (save)

**Files:**
- Modify: `internal/config/init.go`

**Step 1: save() 메서드 구현**

config.toml을 TOML 형식으로 저장. 디렉터리 생성 + 파일 권한 600.

```go
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
```

**Step 2: Commit**

```bash
git add internal/config/init.go
git commit -m "feat(init): add config file saving with proper permissions"
```

---

### Task 5: 이메일 연결 테스트 (testConnection)

**Files:**
- Modify: `internal/config/init.go`

**Step 1: testConnection() 메서드 구현**

SMTP와 IMAP 접속만 확인 (인증까지). 템플릿 이메일 발송은 storage 의존성이 필요하므로, init에서는 연결 테스트까지만 수행한다.

```go
import (
	"crypto/tls"
	"net/smtp"

	"github.com/emersion/go-imap/v2/imapclient"
)

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
```

**Step 2: Commit**

```bash
git add internal/config/init.go
git commit -m "feat(init): add SMTP/IMAP connection test"
```

---

### Task 6: 테스트 작성

**Files:**
- Create: `internal/config/init_test.go`

**Step 1: 테스트 작성**

stdin을 시뮬레이션하여 마법사 흐름을 테스트한다. 연결 테스트는 실제 네트워크가 필요하므로 제외.

```go
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

func newTestWizard(input string) (*initWizard, *bytes.Buffer) {
	out := &bytes.Buffer{}
	return &initWizard{
		in:  bufio.NewScanner(strings.NewReader(input)),
		out: out,
	}, out
}

func TestPrompt(t *testing.T) {
	t.Run("returns user input", func(t *testing.T) {
		w, _ := newTestWizard("hello\n")
		got := w.prompt("Name", "default")
		assert.Equal(t, "hello", got)
	})
	t.Run("returns default on empty input", func(t *testing.T) {
		w, _ := newTestWizard("\n")
		got := w.prompt("Name", "default")
		assert.Equal(t, "default", got)
	})
}

func TestPromptChoice(t *testing.T) {
	t.Run("selects option by number", func(t *testing.T) {
		w, _ := newTestWizard("2\n")
		got := w.promptChoice([]string{"A", "B", "C"}, 0)
		assert.Equal(t, 1, got)
	})
	t.Run("returns default on empty", func(t *testing.T) {
		w, _ := newTestWizard("\n")
		got := w.promptChoice([]string{"A", "B"}, 0)
		assert.Equal(t, 0, got)
	})
	t.Run("returns default on invalid", func(t *testing.T) {
		w, _ := newTestWizard("99\n")
		got := w.promptChoice([]string{"A", "B"}, 0)
		assert.Equal(t, 0, got)
	})
}

func TestPromptSecret(t *testing.T) {
	t.Run("keeps existing on N", func(t *testing.T) {
		w, _ := newTestWizard("N\n")
		got := w.promptSecret("Password", "existing")
		assert.Equal(t, "existing", got)
	})
	t.Run("reads new on y", func(t *testing.T) {
		w, _ := newTestWizard("y\nnewpass\n")
		got := w.promptSecret("Password", "existing")
		assert.Equal(t, "newpass", got)
	})
	t.Run("reads input when no existing", func(t *testing.T) {
		w, _ := newTestWizard("mypass\n")
		got := w.promptSecret("Password", "")
		assert.Equal(t, "mypass", got)
	})
}

func TestSave(t *testing.T) {
	tmpDir := t.TempDir()
	dataDir := filepath.Join(tmpDir, "data")

	// Override ConfigDir for test
	origHome := os.Getenv("HOME")
	t.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", origHome)

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
			AppPassword: "test-password",
		},
	}

	err := w.save(cfg)
	require.NoError(t, err)

	// Verify file exists and permissions
	path := filepath.Join(tmpDir, ".claude-postman", "config.toml")
	info, err := os.Stat(path)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0600), info.Mode().Perm())

	// Verify data dir created
	_, err = os.Stat(dataDir)
	require.NoError(t, err)

	// Verify can load saved config
	loaded, err := LoadFrom(filepath.Join(tmpDir, ".claude-postman"))
	require.NoError(t, err)
	assert.Equal(t, "test@gmail.com", loaded.Email.User)
}

func TestRunInitFreshSetup(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	dataDir := filepath.Join(tmpDir, ".claude-postman", "data")

	// Simulate: Enter (default data dir) → 1 (Gmail) → email → password → 1 (Sonnet)
	input := strings.Join([]string{
		"",             // data dir (default)
		"1",            // Gmail
		"me@gmail.com", // email
		"app-pass",     // password
		"1",            // Sonnet
	}, "\n") + "\n"

	w, out := newTestWizard(input)
	// Disable connection test by overriding testConnection
	err := w.runWithoutConnTest()
	require.NoError(t, err)

	assert.Contains(t, out.String(), "Config saved")

	loaded, err := LoadFrom(filepath.Join(tmpDir, ".claude-postman"))
	require.NoError(t, err)
	assert.Equal(t, dataDir, loaded.General.DataDir)
	assert.Equal(t, "gmail", loaded.Email.Provider)
	assert.Equal(t, "me@gmail.com", loaded.Email.User)
	assert.Equal(t, "sonnet", loaded.General.DefaultModel)
}
```

**Step 2: run()에서 연결 테스트를 분리하여 테스트 가능하게 리팩터**

`run()` 내부의 연결 테스트 호출을 `connTester` 필드로 분리.

```go
// initWizard에 connTester 필드 추가
type initWizard struct {
	in         *bufio.Scanner
	out        io.Writer
	connTester func(*Config) // nil이면 기본 testConnection 사용
}

// runWithoutConnTest는 연결 테스트 없이 실행 (테스트용)
func (w *initWizard) runWithoutConnTest() error {
	w.connTester = func(_ *Config) {} // no-op
	return w.run()
}
```

**Step 3: 테스트 실행**

Run: `go test ./internal/config/ -v -run "Test(Prompt|Save|RunInit)"`
Expected: ALL PASS

**Step 4: Commit**

```bash
git add internal/config/init.go internal/config/init_test.go
git commit -m "test(init): add unit tests for init wizard"
```

---

### Task 7: 린트, 빌드 확인 및 최종 커밋

**Step 1: 빌드 확인**

Run: `go build ./...`
Expected: SUCCESS

**Step 2: 린트 확인**

Run: `golangci-lint run ./internal/config/...`
Expected: PASS

**Step 3: 전체 테스트**

Run: `go test ./... -count=1`
Expected: ALL PASS

**Step 4: PROGRESS.md 업데이트 — init 마법사 완료 기록**

---

## 구현 순서 요약

| Task | 내용 | 파일 |
|------|------|------|
| 1 | I/O 추상화, 프롬프트 헬퍼 | init.go |
| 2 | 메인 흐름 (run) | init.go |
| 3 | 이메일/모델 단계 | init.go |
| 4 | 설정 저장 (save) | init.go |
| 5 | 이메일 연결 테스트 | init.go |
| 6 | 테스트 작성 | init_test.go |
| 7 | 린트/빌드/전체 테스트 | - |
