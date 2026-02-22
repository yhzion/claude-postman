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
| **언어** | Go 1.22+ |
| **DB** | SQLite |
| **세션** | tmux |
| **채널** | 이메일만 (SMTP/IMAP) |
| **서비스** | systemd / launchd / Windows Service |

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
│   ├── email/             # 이메일 송수신 (SMTP/IMAP)
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

```bash
# 필수
CLAUDE_POSTMAN_DATA_DIR=/path/to/data

# 이메일 (예정)
SMTP_HOST=smtp.gmail.com
SMTP_PORT=587
SMTP_USER=user@gmail.com
SMTP_PASS=app-password

IMAP_HOST=imap.gmail.com
IMAP_PORT=993
IMAP_USER=user@gmail.com
IMAP_PASS=app-password
```

## 문서

- [기획 문서](docs/ideas.md)
- [유즈케이스](docs/usecases/SUMMARY.md)
- [기술 스택 결정](docs/tech-stack/)
