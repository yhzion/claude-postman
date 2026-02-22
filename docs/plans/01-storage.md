# Plan: Storage (SQLite)

> SSOT: [docs/architecture/03-storage.md](../architecture/03-storage.md)

---

## 구현 목록

### 파일 구조

```
internal/storage/
├── storage.go          # Store, New, Close, Migrate, Tx
├── session.go          # sessions CRUD
├── outbox.go           # outbox CRUD
├── inbox.go            # inbox CRUD
├── template.go         # template CRUD
└── migrations/
    ├── embed.go        # go:embed
    └── 001_init.sql    # 초기 스키마
```

### 구조체

| 구조체 | 파일 | 필드 |
|--------|------|------|
| `Store` | storage.go | `db *sql.DB`, `tx *sql.Tx` |
| `Session` | storage.go | ID, TmuxName, WorkingDir, Model, Status, CreatedAt, UpdatedAt, LastPrompt, LastResult |
| `OutboxMessage` | storage.go | ID, SessionID, MessageID, Subject, Body, Attachments, Status, RetryCount, NextRetryAt, CreatedAt, SentAt |
| `InboxMessage` | storage.go | ID, SessionID, Body, CreatedAt, Processed |
| `Template` | storage.go | ID, MessageID, CreatedAt |

### 함수

| 함수 | 파일 | 설명 |
|------|------|------|
| `New(dataDir string) (*Store, error)` | storage.go | DB 열기, WAL+FK PRAGMA |
| `Close() error` | storage.go | DB 닫기 |
| `Migrate() error` | storage.go | embed SQL 파일 순차 실행 |
| `Tx(ctx, fn) error` | storage.go | 트랜잭션 래퍼 |
| `CreateSession(*Session) error` | session.go | INSERT |
| `GetSession(id) (*Session, error)` | session.go | SELECT by ID |
| `UpdateSession(*Session) error` | session.go | UPDATE |
| `ListSessionsByStatus(...string) ([]*Session, error)` | session.go | SELECT by status IN |
| `CreateOutbox(*OutboxMessage) error` | outbox.go | INSERT |
| `GetPendingOutbox() ([]*OutboxMessage, error)` | outbox.go | pending + next_retry_at 조건 |
| `MarkSent(id) error` | outbox.go | status→sent, sent_at 설정 |
| `MarkFailed(id, retryCount, nextRetryAt) error` | outbox.go | retry_count, next_retry_at 업데이트 |
| `PurgeOldData(retentionDays) error` | outbox.go | 오래된 sent/processed 삭제 |
| `EnqueueMessage(*InboxMessage) error` | inbox.go | INSERT |
| `DequeueMessage(sessionID) (*InboxMessage, error)` | inbox.go | 가장 오래된 미처리 메시지 |
| `MarkProcessed(id) error` | inbox.go | processed=1 |
| `SaveTemplate(*Template) error` | template.go | INSERT |
| `IsValidTemplateRef(messageID) (bool, error)` | template.go | 존재 여부 확인 |

---

## TDD 체크리스트

### 테스트 (Red)

- [ ] `storage_test.go`: New() — DB 파일 생성 확인
- [ ] `storage_test.go`: New() — WAL 모드 확인
- [ ] `storage_test.go`: Migrate() — 테이블 생성 확인
- [ ] `storage_test.go`: Migrate() — 멱등성 (2회 실행해도 에러 없음)
- [ ] `storage_test.go`: Migrate() — schema_version 업데이트 확인
- [ ] `storage_test.go`: Close() — 정상 종료
- [ ] `storage_test.go`: Tx() — 커밋 동작
- [ ] `storage_test.go`: Tx() — 롤백 동작 (에러 반환 시)
- [ ] `session_test.go`: CreateSession + GetSession 왕복
- [ ] `session_test.go`: UpdateSession — 필드 변경 확인
- [ ] `session_test.go`: ListSessionsByStatus — 필터링 확인
- [ ] `session_test.go`: GetSession — 없는 ID → 에러
- [ ] `outbox_test.go`: CreateOutbox + GetPendingOutbox
- [ ] `outbox_test.go`: MarkSent — status 변경 + sent_at 설정
- [ ] `outbox_test.go`: MarkFailed — retry_count, next_retry_at 업데이트
- [ ] `outbox_test.go`: GetPendingOutbox — next_retry_at 미래 → 제외
- [ ] `outbox_test.go`: PurgeOldData — ended 세션의 오래된 데이터 삭제
- [ ] `inbox_test.go`: EnqueueMessage + DequeueMessage 왕복
- [ ] `inbox_test.go`: DequeueMessage — FIFO 순서 확인
- [ ] `inbox_test.go`: DequeueMessage — 빈 큐 → nil 반환
- [ ] `inbox_test.go`: MarkProcessed — processed 플래그 변경
- [ ] `template_test.go`: SaveTemplate + IsValidTemplateRef
- [ ] `template_test.go`: IsValidTemplateRef — 없는 ID → false

### 구현 (Green)

- [ ] `migrations/embed.go` — go:embed
- [ ] `migrations/001_init.sql` — 스키마 SQL
- [ ] `storage.go` — Store, New, Close, Migrate, Tx
- [ ] `session.go` — CRUD 함수들
- [ ] `outbox.go` — CRUD 함수들
- [ ] `inbox.go` — CRUD 함수들
- [ ] `template.go` — CRUD 함수들

---

## 의존성

- 없음 (Phase 1 기반 모듈)

## 완료 기준

- `go test ./internal/storage/... -v` 전부 PASS
- `go build ./...` 성공
- `golangci-lint run ./internal/storage/...` 통과
