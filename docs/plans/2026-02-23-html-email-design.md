# 설계: tmux 출력 → HTML 이메일 변환

> 날짜: 2026-02-23
> 상태: 승인됨

---

## 1. 문제

`HandleDone`/`HandleAsk`가 tmux `capture-pane` raw 텍스트를 그대로 `outbox.Body`에 저장.
이메일은 `Content-Type: text/html`로 전송되지만 Body는 raw 터미널 텍스트라서
이메일 클라이언트에서 깨지거나 읽기 어려움.

---

## 2. 결정 사항

| 항목 | 결정 |
|------|------|
| 변환 범위 | capture-pane 전체 출력 (도구 로그 포함) |
| ANSI 처리 | 정규식으로 escape code 제거 |
| Markdown→HTML | 기존 `RenderHTML()` (goldmark + chroma) 재사용 |
| 원본 보존 | `session.LastResult`는 raw 텍스트 유지, HTML은 outbox에만 |
| 테스트 | 단위 + E2E |

---

## 3. 설계

### 3.1 데이터 흐름

```
capture-pane (raw text with ANSI codes)
    ↓
StripANSI()          ← ANSI escape code 제거
    ↓
RenderHTML()         ← 기존 goldmark + chroma (Markdown → HTML)
    ↓
outbox.Body = html   ← 정제된 HTML 저장
```

### 3.2 StripANSI (신규)

`internal/email/render.go`에 추가:

```go
var ansiRe = regexp.MustCompile(`\x1b\[[0-9;]*[a-zA-Z]`)

func StripANSI(s string) string {
    return ansiRe.ReplaceAllString(s, "")
}
```

### 3.3 Manager.renderOutput (신규)

`internal/session/session.go`에 추가:

```go
import "github.com/yhzion/claude-postman/internal/email"

func renderOutput(raw string) string {
    cleaned := email.StripANSI(raw)
    html, err := email.RenderHTML(cleaned)
    if err != nil {
        return cleaned // fallback to plain text
    }
    return html
}
```

### 3.4 handleDoneTx / handleAskTx 수정

`internal/session/fifo.go`:

```go
// Before:
outbox.Body = output

// After:
outbox.Body = renderOutput(output)
```

`session.LastResult`는 raw 텍스트 그대로 유지 (원본 보존).

---

## 4. 수정 파일

| 파일 | 변경 | 신규/수정 |
|------|------|-----------|
| `internal/email/render.go` | `StripANSI()` 추가 | 수정 |
| `internal/email/render_test.go` | StripANSI 단위 테스트 | **신규** |
| `internal/session/session.go` | `renderOutput()` 헬퍼 추가 | 수정 |
| `internal/session/fifo.go` | handleDoneTx/handleAskTx Body에 renderOutput 적용 | 수정 |
| `internal/session/session_test.go` | HandleDone/HandleAsk outbox Body가 HTML 포함 검증 | 수정 |
| `_test/integration/e2e_prompt_test.go` | E2E: DONE 후 outbox Body에 HTML 태그 검증 | 수정 |
