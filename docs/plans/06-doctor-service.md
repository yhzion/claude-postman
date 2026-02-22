# Plan: Doctor & Service

> SSOT: [docs/architecture/06-service.md](../architecture/06-service.md) (doctor/service 부분)

---

## 구현 목록

### 파일 구조

```
internal/doctor/
└── doctor.go      # RunDoctor(fix) — 환경 진단

internal/service/
└── service.go     # InstallService, UninstallService
```

### 함수

| 함수 | 설명 |
|------|------|
| `doctor.RunDoctor(fix) (exitCode, error)` | 환경 진단 (8개 항목) |
| `service.InstallService() error` | systemd/launchd 등록 |
| `service.UninstallService() error` | 서비스 해제 |

### Doctor 검사 항목

1. Config — config.toml 존재 및 유효성
2. Data directory — 디렉터리 존재
3. SQLite — DB 열기 + 마이그레이션 상태
4. tmux — `tmux -V` 실행 가능
5. Claude Code — `claude --version` 실행 가능
6. SMTP — 연결 테스트
7. IMAP — 연결 테스트
8. Service — 서비스 상태

---

## TDD 체크리스트

- [ ] RunDoctor — 정상 환경에서 모든 검사 통과
- [ ] RunDoctor — 개별 항목 실패 시 적절한 메시지
- [ ] RunDoctor --fix — 자동 수정 가능 항목 수정
- [ ] InstallService — 플랫폼별 서비스 파일 생성
- [ ] UninstallService — 서비스 파일 삭제

---

## 의존성

- **Config** (Phase 1) — 설정 로딩

## 완료 기준

- `go test ./internal/doctor/... ./internal/service/... -v` 전부 PASS
- `go build ./...` 성공
