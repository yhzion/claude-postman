# Plan: Serve (메인 루프)

> SSOT: [docs/architecture/06-service.md](../architecture/06-service.md) (serve 부분)

---

## 구현 목록

### 파일 구조

```
internal/serve/
└── serve.go       # RunServe (메인 루프, errgroup, goroutine 관리)
```

### 함수

| 함수 | 설명 |
|------|------|
| `RunServe(ctx, cfg, store, mgr, mailer) error` | 메인 루프 |

### goroutine 구조

1. IMAP 폴링 + 오케스트레이션
2. Outbox 플러시
3. 세션별 FIFO (동적)

---

## TDD 체크리스트

- [ ] 시작 시 초기화 순서 (config → DB → migrate → recover → goroutines)
- [ ] SIGINT/SIGTERM graceful shutdown
- [ ] IMAP 폴링 실패 시 다음 주기 재시도
- [ ] idle 세션의 미처리 inbox 확인

---

## 의존성

- **Session** (Phase 2) — Manager
- **Email** (Phase 2) — Mailer
- **Config** (Phase 1) — 설정
- **Storage** (Phase 1) — DB

## 완료 기준

- `go test ./internal/serve/... -v` 전부 PASS
- `go build ./...` 성공
