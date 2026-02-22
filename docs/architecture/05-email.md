# 아키텍처: Email (SMTP/IMAP)

> 이메일 수신(IMAP 폴링), 발송(SMTP), 보안, 오프라인 대응.
> 날짜: 2026-02-22

---

## 1. 개요

| 항목 | 결정 |
|------|------|
| 수신 | IMAP 폴링 (기본 30초 주기) |
| 발송 | SMTP (TLS) |
| 식별 | 제목 태그 `[claude-postman]` |
| 세션 매칭 | Session-ID (본문) + In-Reply-To/References (스레드) |
| 발신자 검증 | config의 `email.user`와 From 일치 시만 처리 |
| 본문 형식 | HTML 리치 텍스트 |
| 오프라인 | outbox 테이블에 저장 후 재시도 |
| 작업 중 수신 | 대기열에 넣고 idle 후 순차 전송 |

---

## 2. 수신 (IMAP)

### 2.1 폴링 흐름

```
폴링 루프 (30초 주기)
  ↓
IMAP 접속 (INBOX만)
  ↓
검색: SUBJECT "[claude-postman]"
  ↓
각 메일에 대해:
  ├─ From != config.email.user → 무시
  ├─ Session-ID 추출
  │   ├─ /new 요청 → 새 세션 생성 흐름
  │   ├─ 매칭 성공 + idle → 세션에 메시지 전달
  │   ├─ 매칭 성공 + active → 대기열에 추가
  │   └─ 매칭 실패 → 무시
  └─ 처리 완료 표시 (SEEN 플래그)
```

### 2.2 세션 매칭 우선순위

1. 본문의 `Session-ID: {UUID}` (명시적)
2. `In-Reply-To` 헤더 → 발송 `Message-ID` 매칭 (스레드)
3. `References` 헤더 → 발송 `Message-ID` 포함 여부 (스레드)

### 2.3 대기열

세션이 active 상태일 때 도착한 메시지는 대기열에 저장:

```
active 세션에 메시지 도착
  ↓
대기열에 추가 (순서 보장)
  ↓
세션이 idle로 전환 (완료 신호 수신)
  ↓
대기열의 다음 메시지를 세션에 전달
  ↓
세션 → active
```

---

## 3. 발송 (SMTP)

### 3.1 발송 흐름

```
이메일 생성
  ↓
outbox 테이블에 삽입 (status: pending)
  ↓
SMTP 발송 시도
  ├─ 성공 → status: sent, sent_at 기록
  └─ 실패 → status 유지 (다음 주기에 재시도)
```

### 3.2 이메일 구조

**헤더:**
- `From`: config.email.user
- `To`: config.email.user (자기 자신)
- `Subject`: `[claude-postman] {타입}: {요약}`
- `Message-ID`: 고유 ID (스레드 매칭용)
- `In-Reply-To`: 수신 메일의 Message-ID (답장 시)

**본문 (HTML):**
- 작업 과정 요약
- 결과
- 변경된 파일 목록
- Session-ID 포함 (다음 답장을 위해)

### 3.3 이메일 타입별 제목

| 타입 | 제목 형식 |
|------|----------|
| 세션 생성 | `[claude-postman] Session started: {UUID 앞 8자}` |
| 작업 완료 | `[claude-postman] Completed: {요약}` |
| 질문 | `[claude-postman] Input needed: {요약}` |
| 에러 | `[claude-postman] Error: {요약}` |
| 세션 복구 | `[claude-postman] Session recovered: {UUID 앞 8자}` |
| 세션 종료 | `[claude-postman] Session ended: {UUID 앞 8자}` |

---

## 4. 오프라인 대응

### 4.1 Outbox 재시도

```
outbox 플러시 루프 (폴링과 동일 주기)
  ↓
pending 상태 메시지 조회
  ↓
각 메시지 SMTP 발송 시도
  ├─ 성공 → sent
  └─ 실패 → 다음 주기에 재시도
```

### 4.2 이점

- 네트워크 끊김 중에도 Claude Code 작업은 계속 진행
- 결과는 outbox에 쌓임
- 네트워크 복구 시 순차 발송

---

## 5. 보안

### 5.1 발신자 검증

```
수신 이메일 From == config.email.user
  ├─ 일치 → 처리
  └─ 불일치 → 무시 (로그만 기록)
```

### 5.2 위협 분석

| 위협 | 대응 | 잔존 위험 |
|------|------|----------|
| 외부 이메일 주입 | From 주소 검증 | 낮음 |
| From 주소 위조 | Gmail SPF/DKIM/DMARC 검증 → 스팸 분류. INBOX만 읽음 | 낮음 |
| Session-ID 추측 | UUID v4 (122비트 엔트로피) | 무시 가능 |
| 이메일 가로채기 | From 검증 + UUID 필요 | 낮음 |

개인용 도구로서 INBOX + From 검증이면 충분한 보안 수준.

---

## 6. Go 인터페이스

```go
type Mailer struct {
    config *config.EmailConfig
    store  *storage.Store
}

func New(cfg *config.EmailConfig, store *storage.Store) *Mailer

// 수신
func (m *Mailer) Poll() ([]*IncomingMessage, error)
func (m *Mailer) StartPolling(ctx context.Context, interval time.Duration)

// 발송
func (m *Mailer) Send(sessionID, subject, htmlBody string) error
func (m *Mailer) FlushOutbox() error

// 메시지 타입
type IncomingMessage struct {
    From      string
    Subject   string
    Body      string
    SessionID string
    MessageID string
}
```
