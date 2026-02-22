# 아키텍처: Session (tmux 세션 관리)

> tmux 세션 라이프사이클 관리, 다중 세션, 서버 재시작 복구.
> 날짜: 2026-02-22

---

## 1. 개요

| 항목 | 결정 |
|------|------|
| 세션 단위 | tmux 세션 1개 = Claude Code 인스턴스 1개 |
| 다중 세션 | 지원 (동시 실행 가능) |
| 식별자 | UUID |
| tmux 세션명 | `session-{UUID}` |
| 재시작 복구 | 자동 복구 시도 (`--resume`) |

---

## 2. 세션 라이프사이클

### 2.1 상태 전이

```
          ┌──────────┐
          │ creating │
          └────┬─────┘
               ↓
          ┌──────────┐
     ┌───→│  active  │←───┐
     │    └────┬─────┘    │
     │         ↓          │
     │    ┌──────────┐    │
     │    │   idle   │────┘  새 입력
     │    └────┬─────┘
     │         ↓
     │    ┌──────────┐
     │    │  ended   │
     │    └──────────┘
     │
     └── 서버 재시작 시 자동 복구
```

| 상태 | 설명 |
|------|------|
| creating | tmux 세션 생성 중 |
| active | Claude Code 작업 진행 중 |
| idle | 작업 완료, 다음 입력 대기 |
| ended | 세션 종료됨 |

### 2.2 상태 전이 규칙

| From | To | 트리거 |
|------|----|--------|
| creating | active | Claude Code 실행 완료 |
| active | idle | 완료 신호 수신 (DONE:{UUID}) |
| idle | active | 새 메시지 전송 |
| active/idle | ended | 사용자 종료 요청 또는 수동 종료 |
| ended | — | 최종 상태 |

---

## 3. 세션 생성

```
1. UUID 생성
2. DB에 session 레코드 삽입 (status: creating)
3. mkfifo /tmp/claude-postman/{UUID}.fifo
4. tmux new-session -d -s session-{UUID} -c {working_dir}
5. tmux send-keys -t session-{UUID} \
     "claude --dangerously-skip-permissions \
      --system-prompt '...' --model {model}" Enter
6. DB: status → active (send-keys 성공 즉시 전이)
7. 세션 전용 goroutine 스폰 → FIFO 블로킹 읽기 시작
```

> **creating → active 전이 시점**: `tmux send-keys` 성공 시점.
> Claude Code 로딩 완료를 기다리지 않음 (로딩 시간이 가변적이므로).

### Claude Code 실행 옵션

```bash
claude --dangerously-skip-permissions \
       --system-prompt "{시스템 프롬프트}" \
       --model {model}
```

- `--dangerously-skip-permissions`: 도구 실행 시 승인 불필요
- `--system-prompt`: 완료 신호, 응답 형식, 끈기 있는 문제 해결 지시
- `--model`: 세션 생성 시 사용자가 지정한 모델 (미지정 시 config 기본값)

---

## 4. 메시지 전송

```
1. 이메일 수신 → Session-ID로 세션 식별
2. DB에서 세션 조회 (status 확인)
   └─ ended면 에러 응답
3. tmux send-keys -t session-{UUID} "{message}" Enter
4. DB: status → active, last_prompt 업데이트
5. FIFO에서 완료 신호 대기 (DONE:{UUID})
6. 500ms 딜레이 후 tmux capture-pane -t session-{UUID} -p -S -1000
7. DB: last_result 업데이트, status → idle
8. outbox에 이메일 추가
9. inbox 대기열 확인 → 있으면 다음 메시지 전달
```

---

## 5. 세션 종료

```
1. 사용자가 "종료" / "끝" 이메일 전송
2. tmux send-keys -t session-{UUID} "/exit" Enter
3. tmux kill-session -t session-{UUID}
4. rm /tmp/claude-postman/{UUID}.fifo
5. DB: status → ended
6. 종료 확인 이메일 발송
```

---

## 6. 서버 재시작 복구

### 6.1 복구 흐름

```
claude-postman 시작
  ↓
DB에서 status가 active/idle인 세션 조회
  ↓
각 세션에 대해:
  ├─ tmux has-session -t session-{UUID}
  │  └─ 있음 → 그대로 유지 (정상)
  └─ 없음 → 복구 시도:
       1. mkfifo /tmp/claude-postman/{UUID}.fifo (FIFO 재생성)
       2. tmux new-session -d -s session-{UUID} -c {working_dir}
       3. tmux send-keys -t session-{UUID} \
            "claude --dangerously-skip-permissions \
             --resume --model {model}" Enter
       4. 세션 전용 goroutine 스폰 → FIFO 블로킹 읽기 시작
       5. DB: status 유지
       6. 사용자에게 "Session recovered" 이메일 발송
```

### 6.2 복구 실패 시

- `--resume`이 실패하면 (세션 데이터 손실 등)
- DB: status → ended
- 사용자에게 "Session could not be recovered" 이메일 발송

---

## 7. 다중 세션

- 각 세션은 독립된 tmux 세션으로 격리
- 이메일의 Session-ID 헤더로 라우팅
- 동시에 여러 세션이 active 상태 가능
- 세션 간 상호작용 없음

---

## 8. Go 인터페이스

```go
type Manager struct {
    store *storage.Store
}

func New(store *storage.Store) *Manager

// 라이프사이클
func (m *Manager) Create(workingDir, model string) (*Session, error)
func (m *Manager) Send(sessionID, message string) error
func (m *Manager) End(sessionID string) error

// 상태
func (m *Manager) Get(sessionID string) (*Session, error)
func (m *Manager) ListActive() ([]*Session, error)

// 복구
func (m *Manager) RecoverAll() error

// 신호 처리
func (m *Manager) HandleDone(sessionID string) (string, error)
```
