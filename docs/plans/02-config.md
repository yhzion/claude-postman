# Plan: Config

> SSOT: [docs/architecture/02-config.md](../architecture/02-config.md)

---

## 구현 목록

### 파일 구조

```
internal/config/
├── config.go     # Config 구조체, Load, ConfigDir, 환경변수 오버라이드
├── init.go       # RunInit (대화형 마법사)
└── preset.go     # 이메일 프로바이더 프리셋
```

### 구조체

| 구조체 | 파일 | 필드 |
|--------|------|------|
| `Config` | config.go | General, Email |
| `GeneralConfig` | config.go | DataDir, DefaultModel, PollIntervalSec, SessionTimeoutMin |
| `EmailConfig` | config.go | Provider, SMTPHost, SMTPPort, IMAPHost, IMAPPort, User, AppPassword |

### 함수

| 함수 | 파일 | 설명 |
|------|------|------|
| `Load() (*Config, error)` | config.go | TOML 읽기 + 환경변수 오버라이드 + 검증 |
| `ConfigDir() string` | config.go | `~/.claude-postman` 경로 반환 |
| `RunInit() error` | init.go | 대화형 설정 마법사 (Phase 1에서는 스텁) |

### 환경변수 매핑

| 환경변수 | 필드 |
|----------|------|
| `CLAUDE_POSTMAN_DATA_DIR` | General.DataDir |
| `CLAUDE_POSTMAN_MODEL` | General.DefaultModel |
| `CLAUDE_POSTMAN_POLL_INTERVAL` | General.PollIntervalSec |
| `CLAUDE_POSTMAN_SESSION_TIMEOUT` | General.SessionTimeoutMin |
| `CLAUDE_POSTMAN_EMAIL_USER` | Email.User |
| `CLAUDE_POSTMAN_EMAIL_PASSWORD` | Email.AppPassword |
| `CLAUDE_POSTMAN_SMTP_HOST` | Email.SMTPHost |
| `CLAUDE_POSTMAN_SMTP_PORT` | Email.SMTPPort |
| `CLAUDE_POSTMAN_IMAP_HOST` | Email.IMAPHost |
| `CLAUDE_POSTMAN_IMAP_PORT` | Email.IMAPPort |

---

## TDD 체크리스트

### 테스트 (Red)

- [ ] `config_test.go`: Load() — 정상 TOML 파일 로딩
- [ ] `config_test.go`: Load() — config.toml 없으면 에러
- [ ] `config_test.go`: Load() — 환경변수 오버라이드 (각 필드별)
- [ ] `config_test.go`: Load() — 환경변수가 TOML 값을 덮어쓰기
- [ ] `config_test.go`: Load() — 필수값 누락 시 에러 (data_dir, user, app_password, smtp_host, imap_host)
- [ ] `config_test.go`: Load() — 기본값 적용 (poll_interval=30, session_timeout=30, model=sonnet)
- [ ] `config_test.go`: ConfigDir() — 경로 반환 확인
- [ ] `preset_test.go`: 프리셋 데이터 확인 (Gmail, Outlook)

### 구현 (Green)

- [ ] `config.go` — Config/GeneralConfig/EmailConfig 구조체
- [ ] `config.go` — Load() (TOML 파싱 + 환경변수 + 검증)
- [ ] `config.go` — ConfigDir()
- [ ] `preset.go` — 프로바이더 프리셋 데이터
- [ ] `init.go` — RunInit() 스텁 (Phase 1에서는 미구현)

---

## 의존성

- 없음 (Phase 1 기반 모듈)
- 외부: `BurntSushi/toml`

## 완료 기준

- `go test ./internal/config/... -v` 전부 PASS
- `go build ./...` 성공
- `golangci-lint run ./internal/config/...` 통과
