# 아키텍처: Storage (SQLite)

> SQLite 기반 데이터 저장소 설계.
> 날짜: 2026-02-22

---

## 1. 개요

| 항목 | 결정 |
|------|------|
| DB | SQLite |
| 드라이버 | `mattn/go-sqlite3` (CGO) |
| 커넥션 | 단일 `*sql.DB` 인스턴스 |
| 저널 모드 | WAL (Write-Ahead Logging) |
| DB 파일 | `{data_dir}/claude-postman.db` |
| 마이그레이션 | embed된 SQL 파일 + `schema_version` 테이블 |

---

## 2. 디렉터리 구조

```
internal/storage/
├── storage.go          # DB 초기화, 커넥션 관리, 마이그레이션
├── session.go          # sessions CRUD
├── outbox.go           # outbox CRUD
├── inbox.go            # inbox (대기열) CRUD
├── template.go         # template CRUD
└── migrations/
    ├── embed.go        # go:embed
    └── 001_init.sql    # 초기 스키마
```

---

## 3. 스키마

### 3.1 초기 마이그레이션 (001_init.sql)

```sql
CREATE TABLE sessions (
    id              TEXT PRIMARY KEY,
    tmux_name       TEXT NOT NULL,
    working_dir     TEXT NOT NULL,
    model           TEXT NOT NULL,
    status          TEXT NOT NULL,
    created_at      DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at      DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    last_prompt     TEXT,
    last_result     TEXT
);

CREATE TABLE outbox (
    id              TEXT PRIMARY KEY,
    session_id      TEXT NOT NULL,
    message_id      TEXT,
    subject         TEXT NOT NULL,
    body            TEXT NOT NULL,
    attachments     TEXT,
    status          TEXT NOT NULL DEFAULT 'pending',
    created_at      DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    sent_at         DATETIME,
    FOREIGN KEY (session_id) REFERENCES sessions(id)
);

CREATE TABLE inbox (
    id              TEXT PRIMARY KEY,
    session_id      TEXT NOT NULL,
    body            TEXT NOT NULL,
    created_at      DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    processed       INTEGER NOT NULL DEFAULT 0,
    FOREIGN KEY (session_id) REFERENCES sessions(id)
);

CREATE TABLE template (
    id              TEXT PRIMARY KEY,
    message_id      TEXT NOT NULL,
    created_at      DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE schema_version (
    version         INTEGER NOT NULL
);

INSERT INTO schema_version (version) VALUES (1);
```

### 3.2 sessions 필드 설명

| 필드 | 타입 | 설명 |
|------|------|------|
| id | TEXT (UUID) | 세션 식별자 |
| tmux_name | TEXT | tmux 세션명 (`session-{UUID}`) |
| working_dir | TEXT | Claude Code 시작 디렉터리 |
| model | TEXT | `sonnet`, `opus`, `haiku` |
| status | TEXT | `creating`, `active`, `idle`, `ended` |
| created_at | DATETIME | 생성 시각 |
| updated_at | DATETIME | 최종 업데이트 시각 |
| last_prompt | TEXT | 마지막 사용자 입력 |
| last_result | TEXT | 마지막 Claude Code 응답 |

### 3.3 outbox 필드 설명

| 필드 | 타입 | 설명 |
|------|------|------|
| id | TEXT (UUID) | 메시지 식별자 |
| session_id | TEXT | 소속 세션 FK |
| message_id | TEXT | 이메일 Message-ID (스레드 매칭용) |
| subject | TEXT | 이메일 제목 |
| body | TEXT | 이메일 본문 (HTML) |
| attachments | TEXT | 첨부파일 정보 (JSON) |
| status | TEXT | `pending`, `sent`, `failed` |
| created_at | DATETIME | 생성 시각 |
| sent_at | DATETIME | 발송 시각 |

### 3.4 inbox 필드 설명

| 필드 | 타입 | 설명 |
|------|------|------|
| id | TEXT (UUID) | 메시지 식별자 |
| session_id | TEXT | 대상 세션 FK |
| body | TEXT | 이메일 본문 |
| created_at | DATETIME | 수신 시각 |
| processed | INTEGER | 처리 완료 여부 (0/1) |

### 3.5 template 필드 설명

| 필드 | 타입 | 설명 |
|------|------|------|
| id | TEXT (UUID) | 템플릿 식별자 |
| message_id | TEXT | 발송된 템플릿 이메일의 Message-ID |
| created_at | DATETIME | 생성 시각 |

---

## 4. 마이그레이션

### 4.1 동작 방식

```
앱 시작
  ↓
schema_version 테이블 존재 확인
  ├─ 없음 → 첫 실행. 001부터 순차 실행
  └─ 있음 → 현재 버전 확인
              ↓
         미적용 SQL 파일 순차 실행
              ↓
         schema_version 업데이트
```

### 4.2 파일 규칙

- 파일명: `{번호}_{설명}.sql` (예: `001_init.sql`, `002_add_inbox.sql`)
- 번호는 3자리 패딩
- go:embed로 바이너리에 포함

### 4.3 embed.go

```go
package migrations

import "embed"

//go:embed *.sql
var Files embed.FS
```

---

## 5. 커넥션 관리

### 5.1 초기화

```go
func New(dataDir string) (*Store, error) {
    dbPath := filepath.Join(dataDir, "claude-postman.db")
    db, err := sql.Open("sqlite3", dbPath+"?_journal_mode=WAL&_foreign_keys=ON")
    // ...
}
```

### 5.2 PRAGMA 설정

| PRAGMA | 값 | 이유 |
|--------|---|------|
| journal_mode | WAL | 읽기 성능 향상 |
| foreign_keys | ON | FK 제약 활성화 |

### 5.3 종료

```go
func (s *Store) Close() error {
    return s.db.Close()
}
```

---

## 6. Go 인터페이스

```go
type Store struct {
    db *sql.DB
}

// 초기화
func New(dataDir string) (*Store, error)
func (s *Store) Close() error
func (s *Store) Migrate() error

// Sessions
func (s *Store) CreateSession(session *Session) error
func (s *Store) GetSession(id string) (*Session, error)
func (s *Store) UpdateSession(session *Session) error

// Outbox
func (s *Store) CreateOutbox(msg *OutboxMessage) error
func (s *Store) GetPendingOutbox() ([]*OutboxMessage, error)
func (s *Store) MarkSent(id string) error

// Inbox (대기열)
func (s *Store) EnqueueMessage(msg *InboxMessage) error
func (s *Store) DequeueMessage(sessionID string) (*InboxMessage, error)
func (s *Store) MarkProcessed(id string) error

// Template
func (s *Store) SaveTemplate(tmpl *Template) error
func (s *Store) IsValidTemplateRef(messageID string) (bool, error)
```
