# 설계: 틸드 확장 + FIFO goroutine + 프롬프트 대기 감지

> 날짜: 2026-02-23
> 상태: 승인됨

---

## 1. 문제

### 1.1 틸드 미확장

이메일에서 `Directory: ~/project`를 지정하면 `~`가 확장되지 않고 그대로 `exec.Command`에 전달됨.
`exec.Command`는 셸을 거치지 않으므로 `~`가 리터럴 문자열로 남아 tmux가 홈 디렉토리로 폴백.

### 1.2 FIFO goroutine 미구현

아키텍처 문서(04-session.md:74)에 "세션 전용 goroutine 스폰 → FIFO 블로킹 읽기 시작"이
명시되어 있으나, `serve.go`와 `session.go` 어디에도 FIFO 읽기 goroutine이 없음.
`HandleDone()`은 정의되어 있지만 호출하는 코드가 없음.

### 1.3 프롬프트 대기 미감지

Claude Code가 사용자에게 질문하며 입력을 기다릴 때, 아무 신호가 없어서
이메일이 발송되지 않음. 사용자는 답변할 수 없고 세션은 영원히 대기.

---

## 2. 결정 사항

| 항목 | 결정 |
|------|------|
| 틸드 확장 위치 | `serve.go:handleNewSession()` |
| FIFO goroutine 위치 | `session.Manager` 내부, `Create()`/`RecoverAll()` 시 스폰 |
| FIFO 프로토콜 | 단일 파이프, `DONE`/`ASK`/`SHUTDOWN` 분기 |
| 감지 방식 | 하이브리드: ASK 신호(주) + `❯` 패턴 감지(안전망) |
| 세션 상태 | 기존 + `waiting` 추가 |
| 테스트 | 단위 + E2E (tmux + 가짜 스크립트) |

---

## 3. 설계

### 3.1 틸드 확장

`serve.go:handleNewSession()`에서 `mgr.Create()` 호출 전에 `~` 확장:

```go
if strings.HasPrefix(workingDir, "~/") {
    home, _ := os.UserHomeDir()
    workingDir = filepath.Join(home, workingDir[2:])
} else if workingDir == "~" {
    home, _ := os.UserHomeDir()
    workingDir = home
}
```

### 3.2 FIFO goroutine

`session.Manager`에 `listenFIFO(sessionID string)` 추가.

```
Create()/RecoverAll() 마지막에 go m.listenFIFO(id)

listenFIFO:
  for {
    fd = open FIFO (O_RDONLY, 블로킹)
    scanner = bufio.NewScanner(fd)
    for scanner.Scan() {
      line = scanner.Text()
      switch {
        "DONE:" prefix → HandleDone(sessionID)
        "ASK:"  prefix → HandleAsk(sessionID)
        "SHUTDOWN"     → fd.Close(); return
      }
    }
    fd.Close()  // writer EOF → 재오픈
  }
```

`End()` 시 `writeSentinel` → `SHUTDOWN` 전송 → goroutine 종료.

### 3.3 HandleAsk (신규)

HandleDone과 유사하되:
- inbox 확인하지 않음 (사용자 답장 대기)
- `status = "waiting"` 설정

```go
func (m *Manager) HandleAsk(sessionID string) error {
    time.Sleep(m.captureDelay)
    session, _ := m.store.GetSession(sessionID)
    output, _ := m.tmux.CapturePane(session.TmuxName, capturePaneLines)

    m.store.Tx(ctx, func(tx *Store) error {
        session.LastResult = &output
        tx.CreateOutbox(&OutboxMessage{...body: output, status: "pending"})
        session.Status = "waiting"
        return tx.UpdateSession(session)
    })
}
```

### 3.4 세션 상태 전이

```
creating → active → DONE → idle       (작업 완료)
                  → ASK  → waiting    (질문 대기)
                              │
                     사용자 답장 → active → ...반복
```

`DeliverNext()`: `idle`과 `waiting` 모두 허용.
`checkIdleSessions()`: `idle`과 `waiting` 모두 처리.

### 3.5 시스템 프롬프트

```
작업이 완료되면 반드시 다음 명령을 실행하세요:
echo 'DONE:{UUID}' > /tmp/claude-postman/{UUID}.fifo

사용자에게 질문하거나 선택을 요청할 때는 반드시 다음 명령을 먼저 실행하세요:
echo 'ASK:{UUID}' > /tmp/claude-postman/{UUID}.fifo
그리고 사용자의 답변을 기다리세요.

최종 응답에는 반드시 다음을 포함하세요:
- 작업 과정 요약
- 결과
- 변경된 파일 목록 (있는 경우)

어떤 방법으로든 작업을 완수하세요. 최소 10번 시도하세요.
포기하지 마세요.
```

### 3.6 패턴 감지 안전망

`session/detect.go`:

```go
func HasInputPrompt(output string) bool {
    lines := strings.Split(strings.TrimSpace(output), "\n")
    start := max(0, len(lines)-5)
    for i := start; i < len(lines); i++ {
        if strings.Contains(lines[i], "❯") {
            return true
        }
    }
    return false
}
```

`serve.go:checkWaitingPrompts()`: pollLoop 매 주기마다 active 세션에 대해
capture-pane → HasInputPrompt → true면 HandleAsk 호출.

### 3.7 E2E 테스트

`_test/integration/e2e_prompt_test.go`:

가짜 Claude Code 스크립트(질문 출력 → `❯` 표시 → stdin 대기 → DONE 전송)를
실제 tmux에서 실행하여 전체 흐름 검증.

시나리오:
1. ASK → HandleAsk → outbox 생성 → waiting 상태
2. SendKeys 답변 → active → DONE → idle
3. 틸드 확장 → tmux cwd 검증
4. ASK 없이 ❯ 감지 (안전망 검증)

---

## 4. 수정 파일

| 파일 | 변경 | 신규/수정 |
|------|------|-----------|
| `internal/serve/serve.go` | 틸드 확장 + checkWaitingPrompts + checkIdleSessions 확장 | 수정 |
| `internal/session/session.go` | Create/RecoverAll에서 go listenFIFO() + systemPromptTemplate | 수정 |
| `internal/session/fifo.go` | listenFIFO() + HandleAsk() + readSignal() | 수정 |
| `internal/session/detect.go` | HasInputPrompt() | **신규** |
| `internal/serve/serve_test.go` | 틸드 확장 + waiting 세션 테스트 | 수정 |
| `internal/session/session_test.go` | HandleAsk + listenFIFO + DeliverNext(waiting) 테스트 | 수정 |
| `internal/session/detect_test.go` | HasInputPrompt 단위 테스트 | **신규** |
| `_test/integration/e2e_prompt_test.go` | E2E 테스트 | **신규** |
