# 아키텍처: CLI, Service, Doctor

> CLI 구조, 시스템 서비스 등록/해제, 환경 진단.
> 날짜: 2026-02-22

---

## 1. CLI 전체 구조

```
claude-postman                   # 도움말 표시 (= --help)
claude-postman --help            # 도움말
claude-postman --version         # 버전

claude-postman init              # 설정 마법사
claude-postman serve             # 서버 실행 (포그라운드)
claude-postman doctor            # 환경 진단
claude-postman doctor --fix      # 진단 + 자동 수정

claude-postman install-service   # systemd/launchd 등록 (sudo)
claude-postman uninstall-service # 서비스 해제 (sudo)
```

### --help 출력

```
claude-postman - Email relay for Claude Code

Usage:
  claude-postman <command> [flags]

Commands:
  init              Setup configuration wizard
  serve             Start the relay server
  doctor            Check environment and diagnose issues
  install-service   Register as a system service (requires sudo)
  uninstall-service Remove system service (requires sudo)

Flags:
  --help            Show help
  --version         Show version

Run 'claude-postman <command> --help' for more information.
```

### 서브커맨드별 도움말

**init --help:**
```
Setup configuration wizard

Creates ~/.claude-postman/config.toml with email credentials,
data directory, and default model settings.

Safe to re-run: existing values shown as defaults.

Usage:
  claude-postman init
```

**serve --help:**
```
Start the relay server

Runs in foreground. Polls for incoming emails, manages
tmux sessions, and sends results back via email.

Requires: claude-postman init (run first)

Usage:
  claude-postman serve
```

**doctor --help:**
```
Check environment and diagnose issues

Verifies config, dependencies, database, email connectivity,
and system service status.

Usage:
  claude-postman doctor [flags]

Flags:
  --fix    Attempt to automatically fix issues
```

---

## 2. Doctor (환경 진단)

### 2.1 검사 항목

| 항목 | 검사 내용 | --fix 동작 |
|------|----------|-----------|
| Config | config.toml 존재 및 유효성 | 불가 (init 안내) |
| Data directory | 디렉터리 존재 | 생성 |
| SQLite | DB 파일 열기 + 마이그레이션 상태 | 마이그레이션 실행 |
| tmux | `tmux -V` 실행 가능 | 불가 (설치 안내) |
| Claude Code | `claude --version` 실행 가능 | 불가 (설치 안내) |
| SMTP | 연결 테스트 | 불가 (설정 확인 안내) |
| IMAP | 연결 테스트 | 불가 (설정 확인 안내) |
| Service | 서비스 등록/실행 상태 | 불가 (명령어 안내) |

### 2.2 출력 예시

```
$ claude-postman doctor

Checking environment...

  ✅ Config: ~/.claude-postman/config.toml
  ✅ Data directory: ~/.claude-postman/data
  ✅ Database: OK (version 1)
  ❌ tmux: not found
  ✅ Claude Code: v2.1.47
  ✅ SMTP: smtp.gmail.com:587 (connected)
  ✅ IMAP: imap.gmail.com:993 (connected)
  ⚠️  Service: not registered

1 error, 1 warning found.

  ❌ tmux: Install with 'sudo apt install tmux' or 'brew install tmux'
  ⚠️  Service: Run 'sudo claude-postman install-service' to register
```

### 2.3 --fix 예시

```
$ claude-postman doctor --fix

Checking environment...

  ✅ Config: ~/.claude-postman/config.toml
  ❌ Data directory: not found → Created ✅
  ❌ Database: not found → Initialized ✅
  ❌ tmux: not found → Cannot auto-fix.
     Install: sudo apt install tmux
  ✅ Claude Code: v2.1.47
  ✅ SMTP: smtp.gmail.com:587 (connected)
  ✅ IMAP: imap.gmail.com:993 (connected)
  ⚠️  Service: not registered

Fixed 2 issues. 1 error, 1 warning remaining.
```

### 2.4 종료 코드

| 코드 | 의미 |
|------|------|
| 0 | 모든 검사 통과 |
| 1 | 에러 있음 (실행 불가) |
| 2 | 경고만 있음 (실행 가능) |

---

## 3. Service (systemd/launchd)

### 3.1 개요

| 항목 | 결정 |
|------|------|
| Linux | systemd |
| macOS | launchd |
| 등록 | `claude-postman install-service` (sudo 필요) |
| 해제 | `claude-postman uninstall-service` (sudo 필요) |
| init 연계 | init 마지막에 등록 명령어 안내만 표시 |

### 3.2 플랫폼 감지

```go
switch runtime.GOOS {
case "linux":
    // systemd
case "darwin":
    // launchd
default:
    // 미지원 에러
}
```

### 3.3 systemd (Linux)

**서비스 파일:**

```ini
# /etc/systemd/system/claude-postman.service
[Unit]
Description=claude-postman
After=network.target

[Service]
Type=simple
User={current_user}
ExecStart={binary_path} serve
Restart=on-failure
RestartSec=5
Environment=HOME={user_home}

[Install]
WantedBy=multi-user.target
```

**install-service 동작:**
```
1. 서비스 파일 생성: /etc/systemd/system/claude-postman.service
2. systemctl daemon-reload
3. systemctl enable claude-postman
4. systemctl start claude-postman
5. 상태 출력
```

**uninstall-service 동작:**
```
1. systemctl stop claude-postman
2. systemctl disable claude-postman
3. 서비스 파일 삭제
4. systemctl daemon-reload
```

### 3.4 launchd (macOS)

**plist 파일:**

```xml
<!-- ~/Library/LaunchAgents/com.claude-postman.plist -->
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN"
  "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>Label</key>
    <string>com.claude-postman</string>
    <key>ProgramArguments</key>
    <array>
        <string>{binary_path}</string>
        <string>serve</string>
    </array>
    <key>RunAtLoad</key>
    <true/>
    <key>KeepAlive</key>
    <true/>
    <key>StandardOutPath</key>
    <string>{data_dir}/claude-postman.log</string>
    <key>StandardErrorPath</key>
    <string>{data_dir}/claude-postman.err</string>
</dict>
</plist>
```

**install-service 동작:**
```
1. plist 파일 생성: ~/Library/LaunchAgents/com.claude-postman.plist
2. launchctl load ~/Library/LaunchAgents/com.claude-postman.plist
3. 상태 출력
```

**uninstall-service 동작:**
```
1. launchctl unload ~/Library/LaunchAgents/com.claude-postman.plist
2. plist 파일 삭제
```

---

## 4. Go 인터페이스

```go
// service
func InstallService() error
func UninstallService() error

// doctor
func RunDoctor(fix bool) (exitCode int, err error)
```
