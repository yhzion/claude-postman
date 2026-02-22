# 아키텍처: Email (SMTP/IMAP)

> 이메일 수신(IMAP 폴링), 발송(SMTP), 보안, 오프라인 대응.
> 날짜: 2026-02-22

---

## 1. 개요

| 항목 | 결정 |
|------|------|
| 수신 | IMAP 폴링 (기본 30초, config에서 변경 가능) |
| 발송 | SMTP (TLS, `net/smtp` 표준) |
| 식별 | 제목 태그 `[claude-postman]` |
| 세션 매칭 | Session-ID (본문) + In-Reply-To/References (스레드) |
| 발신자 검증 | config의 `email.user`와 From 일치 시만 처리 |
| 세션 생성 | 템플릿 이메일 포워드 (템플릿 참조 검증) |
| 본문 형식 | HTML (goldmark + chroma) |
| 대기열 저장 | DB inbox 테이블 |
| 오프라인 | outbox 테이블에 저장 후 재시도 |

---

## 2. 수신 (IMAP)

### 2.1 폴링 흐름

```
폴링 루프 (config.poll_interval_sec 주기, 기본 30초)
  ↓
Mailer.Poll() 호출:
  IMAP 접속 (INBOX만, defer Close로 반드시 해제)
  ↓
  검색: SUBJECT "[claude-postman]"
  ↓
  각 메일에 대해:
    ├─ From != config.email.user → 무시
    ├─ 세션 생성 요청 판별 (In-Reply-To/References → 템플릿 Message-ID)
    ├─ 기존 세션 매칭 (Session-ID 추출)
    └─ 처리 완료 표시 (SEEN 플래그)
  ↓
  []*IncomingMessage 반환 (DB 조작 없음, 파싱만)
  ↓
serve 루프 (오케스트레이터):
  각 IncomingMessage에 대해:
    ├─ IsNewSession → mgr.Create() 호출 → 세션 생성
    ├─ SessionID 있음 → store.EnqueueMessage() → inbox 삽입
    └─ 매칭 실패 → 무시
  ↓
  idle 세션의 미처리 inbox 확인 → mgr.Send()로 세션 전달
```

> **역할 분리**: Mailer는 IMAP/SMTP I/O만 담당. DB 삽입과 세션 관리는
> serve 루프가 오케스트레이션. 이렇게 하면 Mailer→Manager 순환 의존 없음.

> **IMAP 연결 정책**: 매 폴링 주기마다 새 연결을 생성하고 즉시 해제.
> 연결 유지(keep-alive) 방식은 네트워크 단절 시 복구가 복잡하므로 채택하지 않음.
> Gmail의 동시 IMAP 연결 제한(15개)에 걸리지 않도록 반드시 defer Close 사용.

### 2.2 세션 매칭 우선순위

1. 본문의 `Session-ID: {UUID}` (명시적)
2. `In-Reply-To` 헤더 → 발송 `Message-ID` 매칭 (스레드)
3. `References` 헤더 → 발송 `Message-ID` 포함 여부 (스레드)

### 2.3 대기열 (DB inbox 테이블)

**모든 수신 메시지는 세션 상태와 무관하게 항상 inbox 테이블에 삽입한다.**
이는 IMAP goroutine과 FIFO goroutine 사이의 경합 조건(lost wakeup)을 방지한다.

```
메시지 수신 (serve 루프에서 store.EnqueueMessage 호출)
  ↓
inbox 테이블에 삽입 (session_id, body, processed=0)
  ↓
소비 시점 (두 가지 트리거):
  ├─ FIFO goroutine: DONE 수신 → store.Tx() 내에서
  │   idle 전환 + inbox 확인을 단일 트랜잭션으로 처리
  └─ IMAP goroutine: 폴링 완료 후 idle 세션의 미처리 inbox 확인
  ↓
다음 메시지를 mgr.Send()로 세션에 전달 → processed=1, 세션 → active
```

> **경합 조건 방지**: FIFO goroutine에서 idle 전환과 inbox 확인을
> `store.Tx()`로 묶음. "idle로 전환 직후 메시지 도착" 시에도
> 다음 IMAP 폴링 주기에 감지.

### 2.4 세션 생성 (템플릿 포워드)

init 시 발송된 템플릿 이메일을 포워드하여 새 세션 생성:

```
사용자가 템플릿 이메일 포워드
  ↓
In-Reply-To/References 확인
  ├─ DB template.message_id와 매칭 → 세션 생성 허용
  └─ 매칭 실패 → 무시
  ↓
본문에서 구조적 템플릿 파싱:
  ├─ Directory: /path/to/dir (기본: ~)
  ├─ Model: sonnet (기본: config.default_model)
  └─ 나머지: 작업 내용 (프롬프트)
  ↓
새 세션 생성 → 세션 시작 이메일 발송
```

**구조적 템플릿 (init 시 발송):**
```
Subject: [claude-postman] New Session

Directory: /home/user
Model: sonnet

(Write your task here)
```

**파싱 규칙 (키워드 기반):**
```
정규식으로 추출:
  ^Directory:\s*(.+)$  → working_dir (미매칭 시 config.data_dir의 부모 또는 ~)
  ^Model:\s*(.+)$      → model (미매칭 시 config.default_model)
  나머지 텍스트         → 태스크 프롬프트

포워딩 아티팩트 처리:
  - "---------- Forwarded message ----------" 이후 텍스트는 무시
  - "> " 인용 접두사 제거 후 파싱
  - HTML 본문인 경우 텍스트 추출 후 파싱
```

사용자는 Directory, Model을 수정하고 태스크 내용을 입력한 후 자기 자신에게 포워드.

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

### 4.1 Outbox 재시도 (지수 백오프)

```
outbox 플러시 루프 (폴링과 동일 주기)
  ↓
pending 상태 + next_retry_at <= now 메시지 조회
  ↓
각 메시지 SMTP 발송 시도
  ├─ 성공 → status: sent, sent_at 기록
  └─ 실패:
       retry_count += 1
       ├─ retry_count < max_retries (기본: 5)
       │   → next_retry_at = now + 30s × 2^(retry_count-1)
       │     (30s, 1m, 2m, 4m, 8m)
       └─ retry_count >= max_retries
           → status: failed (더 이상 재시도 안 함)
```

| retry_count | 대기 시간 | 누적 |
|-------------|----------|------|
| 1 | 30초 | 30초 |
| 2 | 1분 | 1분 30초 |
| 3 | 2분 | 3분 30초 |
| 4 | 4분 | 7분 30초 |
| 5 | failed | — |

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

### 5.2 템플릿 참조 검증

새 세션 생성은 템플릿 이메일의 포워드/답장만 허용:

```
수신 이메일의 In-Reply-To/References
  ↓
DB template.message_id와 매칭
  ├─ 매칭 → 세션 생성 허용
  └─ 미매칭 → 세션 생성 거부 (로그 기록)
```

### 5.3 위협 분석

| 위협 | 대응 | 잔존 위험 |
|------|------|----------|
| 외부 이메일 주입 | From 주소 검증 | 낮음 |
| From 주소 위조 | Gmail SPF/DKIM/DMARC 검증 → 스팸 분류. INBOX만 읽음 | 낮음 |
| 무단 세션 생성 | 템플릿 Message-ID 참조 필수 (이중 검증) | 매우 낮음 |
| Session-ID 추측 | UUID v4 (122비트 엔트로피) | 무시 가능 |
| 이메일 가로채기 | From 검증 + 템플릿 참조 + UUID 필요 | 매우 낮음 |

개인용 도구로서 From 검증 + 템플릿 참조 이중 보안은 충분한 수준.

---

## 6. 라이브러리 및 HTML 변환

| 라이브러리 | 용도 |
|-----------|------|
| `net/smtp` (표준) | SMTP 발송 |
| `emersion/go-imap` v2 | IMAP 수신 |
| `emersion/go-message` | 이메일 메시지 파싱 (MIME, 헤더, 본문) |
| `yuin/goldmark` | Markdown → HTML 변환 |
| `alecthomas/chroma` | 코드 하이라이팅 |

**HTML 변환 흐름**: capture-pane 출력을 그대로 이메일 본문으로 사용.
시스템 프롬프트로 Claude Code에게 마크다운 형식 응답을 지시하므로,
goldmark + chroma로 HTML 변환하여 리치 이메일 생성.

---

## 7. Go 인터페이스

```go
type Mailer struct {
    config *config.EmailConfig
    store  *storage.Store
}

func New(cfg *config.EmailConfig, store *storage.Store) *Mailer

// 수신 (매 호출 시 IMAP 연결 생성/해제, defer Close 보장)
// Poll()은 IMAP에서 메시지를 가져와 파싱만 수행. DB 삽입이나 세션 전달은 하지 않음.
// 호출자(serve 루프)가 반환값을 받아 inbox 삽입, 세션 생성 등을 오케스트레이션.
func (m *Mailer) Poll() ([]*IncomingMessage, error)

// 발송 (outbox 테이블 경유)
func (m *Mailer) Send(sessionID, subject, htmlBody string) error  // outbox에 pending으로 삽입
func (m *Mailer) FlushOutbox() error  // pending + next_retry_at <= now 조회, 지수 백오프 재시도

// 템플릿
func (m *Mailer) SendTemplate() (messageID string, err error)

// 메시지 타입
type IncomingMessage struct {
    From         string
    Subject      string
    Body         string
    SessionID    string  // 기존 세션 메시지일 때 세트
    MessageID    string
    IsNewSession bool    // 템플릿 포워드 여부
    WorkingDir   string  // IsNewSession=true일 때 파싱된 디렉터리
    Model        string  // IsNewSession=true일 때 파싱된 모델
}
```
