# Plan: Email (SMTP/IMAP)

> SSOT: [docs/architecture/05-email.md](../architecture/05-email.md)

---

## 구현 목록

### 파일 구조

```
internal/email/
├── email.go       # Mailer, New, Poll, Send, FlushOutbox, SendTemplate
├── parse.go       # 이메일 파싱 (세션 매칭, 템플릿 파싱)
└── render.go      # Markdown → HTML 변환 (goldmark + chroma)
```

### 구조체/타입

| 구조체 | 설명 |
|--------|------|
| `Mailer` | config, store 보유 |
| `IncomingMessage` | 파싱된 수신 메시지 |

### 함수

| 함수 | 설명 |
|------|------|
| `New(cfg, store) *Mailer` | Mailer 생성 |
| `Poll() ([]*IncomingMessage, error)` | IMAP 폴링 (파싱만, DB 미접근) |
| `Send(sessionID, subject, htmlBody) error` | outbox에 pending 삽입 |
| `FlushOutbox() error` | pending 메시지 SMTP 발송 + 재시도 |
| `SendTemplate() (messageID, error)` | 템플릿 이메일 발송 |

---

## TDD 체크리스트

- [ ] 이메일 파싱 — 세션 매칭 (Session-ID, In-Reply-To, References)
- [ ] 이메일 파싱 — 템플릿 포워드 감지
- [ ] 이메일 파싱 — 구조적 템플릿 파싱 (Directory, Model)
- [ ] Send — outbox 삽입 확인
- [ ] FlushOutbox — 정상 발송
- [ ] FlushOutbox — 실패 시 지수 백오프
- [ ] Markdown → HTML 변환

---

## 의존성

- **Storage** (Phase 1) — outbox/template CRUD
- **Config** (Phase 1) — 이메일 설정

## 완료 기준

- `go test ./internal/email/... -v` 전부 PASS
- `go build ./...` 성공
