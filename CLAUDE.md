# CLAUDE.md

claude-postman 프로젝트의 Claude Code 작업 가이드.

## 프로젝트 개요

Claude Code와 사용자 사이를 **이메일로 중계**하는 서버 프로그램.

```
사용자 (이메일) ←→ claude-postman (중계 서버) ←→ Claude Code (tmux 세션)
```

## 기술 스택

| 항목 | 결정 |
|------|------|
| **언어** | Go 1.24+ |
| **DB** | SQLite (`mattn/go-sqlite3`) |
| **세션** | tmux |
| **채널** | 이메일만 (SMTP/IMAP) |
| **서비스** | systemd / launchd |
| **CLI** | cobra |
| **로깅** | log/slog (표준) |

### 주요 외부 의존성

| 패키지 | 용도 |
|--------|------|
| `spf13/cobra` | CLI 프레임워크 |
| `mattn/go-sqlite3` | SQLite 드라이버 (CGO) |
| `emersion/go-imap` v2 | IMAP 클라이언트 |
| `emersion/go-message` | 이메일 메시지 파싱 (MIME, 헤더) |
| `BurntSushi/toml` | TOML 파서 |
| `google/uuid` | UUID 생성 |
| `yuin/goldmark` | Markdown → HTML 변환 |
| `alecthomas/chroma` | 코드 하이라이팅 |
| `stretchr/testify` | 테스트 assertions |
| `google/go-cmp` | 테스트 비교 |

## 제약사항

- ✅ Mac, Linux (Windows 향후 고려)
- ✅ 이메일만 지원 (다른 메신저 X)
- ✅ Claude Code 전용
- ✅ 적은 메모리 + 고성능

## 코딩 컨벤션

### Go 스타일
- `gofmt`, `goimports` 필수
- Effective Go 준수
- 에러 처리: 명시적 반환, panic 금지

### 품질 관리
- **pre-commit**: 포맷팅, 기본 검사 (빠름)
- **pre-push**: 린팅, 빌드, 테스트 (느림)
- **golangci-lint**: 통합 린터

### 테스트
- `testing` + `testify` + `go-cmp`
- 단위 테스트: 각 패키지
- 통합 테스트: `_test/integration`

## 프로젝트 구조

```
claude-postman/
├── cmd/
│   └── claude-postman/    # main 진입점
├── internal/
│   ├── config/            # 설정 로딩, init 마법사
│   ├── doctor/            # 환경 진단 (doctor 커맨드)
│   ├── email/             # 이메일 송수신 (SMTP/IMAP)
│   ├── serve/             # 메인 루프 (serve 커맨드)
│   ├── session/           # tmux 세션 관리
│   ├── storage/           # SQLite 저장소
│   └── service/           # 시스템 서비스 (systemd/launchd)
├── pkg/                   # 재사용 가능한 패키지
├── configs/               # 설정 파일 예시
├── docs/                  # 문서
├── .golangci.yml          # 린터 설정
├── .pre-commit-config.yaml # Git 훅 설정
├── go.mod
├── go.sum
└── CLAUDE.md
```

## 환경 변수

config.toml 값을 환경변수로 오버라이드 가능. 자세한 내용은 [02-config.md](docs/architecture/02-config.md) 참조.

```bash
CLAUDE_POSTMAN_DATA_DIR=/path/to/data
CLAUDE_POSTMAN_MODEL=sonnet
CLAUDE_POSTMAN_POLL_INTERVAL=30
CLAUDE_POSTMAN_SESSION_TIMEOUT=30
CLAUDE_POSTMAN_EMAIL_USER=user@gmail.com
CLAUDE_POSTMAN_EMAIL_PASSWORD=app-password
CLAUDE_POSTMAN_SMTP_HOST=smtp.gmail.com
CLAUDE_POSTMAN_SMTP_PORT=587
CLAUDE_POSTMAN_IMAP_HOST=imap.gmail.com
CLAUDE_POSTMAN_IMAP_PORT=993
```

## 문서

- [기획 문서](docs/ideas.md)
- [유즈케이스](docs/usecases/SUMMARY.md)
- [기술 스택 결정](docs/tech-stack/)

### 아키텍처 설계

- [01. tmux 출력 캡처](docs/architecture/01-tmux-output-capture.md)
- [02. Config 설계](docs/architecture/02-config.md)
- [03. Storage (SQLite)](docs/architecture/03-storage.md)
- [04. Session 관리](docs/architecture/04-session.md)
- [05. Email (SMTP/IMAP)](docs/architecture/05-email.md)
- [06. CLI, Service, Doctor](docs/architecture/06-service.md)
