# Plan: Session (tmux 세션 관리)

> SSOT: [docs/architecture/04-session.md](../architecture/04-session.md), [docs/architecture/01-tmux-output-capture.md](../architecture/01-tmux-output-capture.md)

---

## 구현 목록

### 파일 구조

```
internal/session/
├── session.go     # Manager, New, Create, End, Get, ListActive, RecoverAll
├── fifo.go        # FIFO 관리, goroutine, HandleDone
└── tmux.go        # tmux 명령어 래퍼 (send-keys, capture-pane, etc.)
```

### 구조체

| 구조체 | 파일 | 설명 |
|--------|------|------|
| `Manager` | session.go | store, 활성 goroutine 관리 |

### 함수

| 함수 | 설명 |
|------|------|
| `New(store) *Manager` | Manager 생성 |
| `Create(workingDir, model) (*Session, error)` | 세션 생성 (UUID, FIFO, tmux, DB) |
| `End(sessionID) error` | 세션 종료 (tmux kill, FIFO 정리, DB 업데이트) |
| `DeliverNext(sessionID) error` | inbox → tmux send-keys (idle 세션만) |
| `Get(sessionID) (*Session, error)` | 세션 조회 |
| `ListActive() ([]*Session, error)` | active/idle 세션 목록 |
| `RecoverAll() error` | 서버 재시작 시 복구 |
| `HandleDone(sessionID) error` | FIFO 신호 처리 (capture-pane → outbox → inbox 확인) |

---

## TDD 체크리스트

- [ ] Manager 생성
- [ ] Create — DB 레코드 생성 확인
- [ ] Create — tmux 세션 생성 확인
- [ ] Create — FIFO 파일 생성 확인
- [ ] End — tmux 세션 종료 확인
- [ ] End — FIFO 파일 삭제 확인
- [ ] End — DB status → ended
- [ ] DeliverNext — idle 세션에 메시지 전달
- [ ] DeliverNext — active/ended 세션 거부
- [ ] HandleDone — capture-pane 결과 outbox 저장
- [ ] HandleDone — inbox 확인 후 다음 메시지 전달
- [ ] RecoverAll — tmux 세션 있으면 유지
- [ ] RecoverAll — tmux 세션 없으면 복구

---

## 의존성

- **Storage** (Phase 1) — DB CRUD
- **Config** (Phase 1) — 모델 기본값, 타임아웃

## 완료 기준

- `go test ./internal/session/... -v` 전부 PASS
- `go build ./...` 성공
