# 아키텍처: Config 설계

> 설정 파일 관리, init 마법사, 환경변수 오버라이드.
> 날짜: 2026-02-22

---

## 1. 개요

### 1.1 CLI 구조

```
claude-postman init     # 설정 마법사 (최초 또는 재설정)
claude-postman serve    # 서버 실행
```

### 1.2 설정 우선순위

```
환경변수 > config.toml
```

---

## 2. 설정 파일

### 2.1 위치 및 형식

- **경로**: `~/.claude-postman/config.toml`
- **형식**: TOML
- **권한**: 600 (소유자만 읽기/쓰기)

### 2.2 구조

```toml
[general]
data_dir = "/home/user/.claude-postman/data"
default_model = "sonnet"    # sonnet | opus | haiku
poll_interval_sec = 30      # IMAP 폴링 주기 (초)

[email]
provider = "gmail"              # gmail | outlook | other
smtp_host = "smtp.gmail.com"
smtp_port = 587
imap_host = "imap.gmail.com"
imap_port = 993
user = "user@gmail.com"
app_password = "xxxx-xxxx-xxxx-xxxx"
```

프리셋을 선택하더라도 **모든 값을 명시적으로 저장**한다.

---

## 3. 환경변수 오버라이드

config.toml 값을 환경변수로 덮어쓸 수 있다.

| 환경변수 | config.toml 필드 |
|----------|-----------------|
| `CLAUDE_POSTMAN_DATA_DIR` | `general.data_dir` |
| `CLAUDE_POSTMAN_MODEL` | `general.default_model` |
| `CLAUDE_POSTMAN_EMAIL_USER` | `email.user` |
| `CLAUDE_POSTMAN_EMAIL_PASSWORD` | `email.app_password` |
| `CLAUDE_POSTMAN_SMTP_HOST` | `email.smtp_host` |
| `CLAUDE_POSTMAN_SMTP_PORT` | `email.smtp_port` |
| `CLAUDE_POSTMAN_IMAP_HOST` | `email.imap_host` |
| `CLAUDE_POSTMAN_IMAP_PORT` | `email.imap_port` |
| `CLAUDE_POSTMAN_POLL_INTERVAL` | `general.poll_interval_sec` |

---

## 4. 이메일 프로바이더 프리셋

init 마법사에서 프로바이더를 선택하면 호스트/포트가 자동 입력된다.

### 4.1 프리셋 목록

| 프로바이더 | SMTP Host | SMTP Port | IMAP Host | IMAP Port |
|-----------|-----------|-----------|-----------|-----------|
| Gmail | smtp.gmail.com | 587 | imap.gmail.com | 993 |
| Outlook | smtp.office365.com | 587 | outlook.office365.com | 993 |

`Other` 선택 시 모든 값을 수동 입력.

### 4.2 프로바이더별 도움말

프리셋 선택 시 해당 프로바이더의 앱 비밀번호 발급 방법을 안내한다.

**Gmail:**
```
┌─ Help ─────────────────────────────────────────┐
│ How to get a Gmail App Password:               │
│                                                │
│ 1. Enable 2-Step Verification:                 │
│    Google Account > Security > 2-Step Verify   │
│ 2. Create App Password:                        │
│    https://myaccount.google.com/apppasswords   │
│ 3. Enter app name > Copy the 16-char password  │
└────────────────────────────────────────────────┘
```

**Outlook:**
```
┌─ Help ─────────────────────────────────────────┐
│ How to get an Outlook App Password:            │
│                                                │
│ 1. Enable 2-Step Verification:                 │
│    Microsoft Account > Security > Advanced     │
│ 2. Create App Password:                        │
│    Security > App Passwords > Create           │
│ 3. Copy the generated password                 │
└────────────────────────────────────────────────┘
```

---

## 5. init 마법사

### 5.1 최초 실행

```
$ claude-postman init

Claude Postman Setup
====================

[1/3] Data Directory
  Where should claude-postman store its data?
  (default: ~/.claude-postman/data)
  >

[2/3] Email Account
  Select your email provider:
  (1) Gmail
  (2) Outlook
  (3) Other (manual setup)
  > 1

  ✓ SMTP: smtp.gmail.com:587
  ✓ IMAP: imap.gmail.com:993

  Email address: > user@gmail.com
  App password:

  ┌─ Help ─────────────────────────────────────────┐
  │ How to get a Gmail App Password:               │
  │ ...                                            │
  └────────────────────────────────────────────────┘

  > ****

[3/3] Default Model
  Which Claude model to use by default?
  (Sessions can override this per request)
  (1) Sonnet  - balanced speed and quality
  (2) Opus    - highest quality
  (3) Haiku   - fastest
  > 1

✅ Config saved: ~/.claude-postman/config.toml
✅ Data directory created: ~/.claude-postman/data

Testing email connection...
  ✅ SMTP: smtp.gmail.com:587 (connected)
  ✅ IMAP: imap.gmail.com:993 (connected)
  ✅ Template email sent to user@gmail.com

To start a new session:
  Forward the template email and edit the body.

Run 'claude-postman serve' to start.
```

### 5.2 재실행 (config.toml 존재 시)

기존 설정값을 기본값으로 표시하고, Enter로 유지하거나 새로 입력하여 변경.

```
$ claude-postman init

Existing config found. Values shown as defaults.

[1/3] Data Directory
  (default: /home/user/.claude-postman/data)
  >                                          <-- Enter to keep

[2/3] Email Account
  Provider: Gmail
  Email (default: user@gmail.com)
  >                                          <-- Enter to keep
  App password: [unchanged]
  Change? (y/N) > N

[3/3] Default Model
  Current: Sonnet
  Change? (y/N) > N

✅ Config saved: ~/.claude-postman/config.toml
```

---

## 6. 설정 로딩

### 6.1 순서

```
1. ~/.claude-postman/config.toml 읽기
   └─ 없으면 에러: "Config not found. Run 'claude-postman init' first."
2. 환경변수로 덮어쓰기
3. 필수값 검증
   └─ 실패 시 에러 메시지와 함께 종료
```

### 6.2 필수값

| 필드 | 검증 |
|------|------|
| `general.data_dir` | 경로 존재 여부 |
| `email.user` | 비어있지 않음 |
| `email.app_password` | 비어있지 않음 |
| `email.smtp_host` | 비어있지 않음 |
| `email.imap_host` | 비어있지 않음 |

### 6.3 모델 (세션별 오버라이드)

- config.toml의 `default_model`이 기본값
- 세션 생성 시 사용자가 모델을 지정하면 해당 모델 사용
- 지정하지 않으면 `default_model` 사용

---

## 7. Go 구조체

```go
type Config struct {
    General GeneralConfig `toml:"general"`
    Email   EmailConfig   `toml:"email"`
}

type GeneralConfig struct {
    DataDir         string `toml:"data_dir"`
    DefaultModel    string `toml:"default_model"`
    PollIntervalSec int    `toml:"poll_interval_sec"`
}

type EmailConfig struct {
    Provider    string `toml:"provider"`
    SMTPHost    string `toml:"smtp_host"`
    SMTPPort    int    `toml:"smtp_port"`
    IMAPHost    string `toml:"imap_host"`
    IMAPPort    int    `toml:"imap_port"`
    User        string `toml:"user"`
    AppPassword string `toml:"app_password"`
}
```
