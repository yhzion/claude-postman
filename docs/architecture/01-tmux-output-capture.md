# 아키텍처: tmux 출력 캡처

> Claude Code 대화형 세션의 출력을 캡처하여 이메일로 전달하는 방식.
> 날짜: 2026-02-22

---

## 1. 설계 배경

### 1.1 핵심 요구사항

| 요구사항 | 설명 |
|----------|------|
| 롱텀 작업 | 긴 대화를 이어가며 작업 가능해야 함 |
| Auto Compact | 컨텍스트 자동 압축으로 무제한 대화 |
| 최종 응답 캡처 | 작업 결과를 이메일로 전송 |
| 완료 감지 | 작업이 끝났음을 자동으로 인지 |

### 1.2 검토한 방식

| 방식 | Auto Compact | 완료 감지 | 출력 파싱 | 결정 |
|------|-------------|----------|----------|------|
| 대화형 + capture-pane + 신호 | ✅ | ✅ 이벤트 기반 | 보통 | **채택** |
| `--print` + `--resume` | ❌ | ✅ 프로세스 종료 | 쉬움 | 탈락 |
| 대화형 + pipe-pane | ✅ | ❌ 패턴 매칭 필요 | 어려움 | 탈락 |
| 대화형 + capture-pane 폴링 | ✅ | ❌ 패턴 매칭 필요 | 보통 | 탈락 |

`--print` + `--resume`은 auto compact가 작동하지 않아 롱텀 작업에 부적합.
pipe-pane과 capture-pane 폴링은 완료 감지가 불안정.

---

## 2. 아키텍처

### 2.1 구성 요소

```
┌──────────────────────────────────────────────────┐
│ tmux: postman-control (호스트)                    │
│                                                  │
│  claude-postman (Go 프로세스)                     │
│    - tmux 이벤트 대기                             │
│    - 신호 수신 → capture-pane → 이메일 발송        │
└──────────┬───────────────────────────────────────┘
           │ 생성/관리
           ▼
┌──────────────────────────────────────────────────┐
│ tmux: session-{UUID} (워커)                       │
│                                                  │
│  claude --dangerously-skip-permissions            │
│    - 사용자 요청 작업 수행                         │
│    - 완료 시: Bash("tmux send-keys -t             │
│      postman-control 'DONE:{UUID}' Enter")        │
└──────────────────────────────────────────────────┘
```

- **postman-control**: claude-postman이 실행되는 호스트 세션. 워커로부터 완료 신호를 수신.
- **session-{UUID}**: Claude Code가 실행되는 워커 세션. 작업 완료 시 호스트에 신호 전송.

### 2.2 입출력 방식

| 항목 | 방식 | 설명 |
|------|------|------|
| 입력 | `tmux send-keys` | 워커 세션에 텍스트 전송 |
| 완료 감지 | `tmux send-keys` 신호 | 워커 → 호스트로 `DONE:{UUID}` 전송 |
| 출력 캡처 | `tmux capture-pane` | 신호 수신 후 1회 캡처 |

### 2.3 실행 옵션

```bash
claude --dangerously-skip-permissions \
       --system-prompt "..." \
       --model sonnet
```

- `--dangerously-skip-permissions`: 도구 실행 시 사용자 승인 불필요
- `--system-prompt`: 완료 신호 및 응답 형식 지시

---

## 3. 작업 흐름

```
1. 사용자 이메일 수신
     ↓
2. tmux send-keys -t session-{UUID} "작업 지시" Enter
     ↓
3. Claude Code 작업 수행 (auto compact으로 롱텀 가능)
     ↓
4. Claude Code 완료 → Bash로 "DONE:{UUID}" 신호 전송
     ↓
5. claude-postman이 신호 수신
     ↓
6. 짧은 딜레이 (렌더링 대기)
     ↓
7. tmux capture-pane -t session-{UUID} -p
     ↓
8. 최종 응답 파싱 → 이메일 발송
```

---

## 4. 시스템 프롬프트

Claude Code에 주입하는 시스템 프롬프트 핵심 내용:

```
작업이 완료되면 반드시 다음 명령을 실행하세요:
tmux send-keys -t postman-control 'DONE:{SESSION_ID}' Enter

최종 응답에는 반드시 다음을 포함하세요:
- 작업 과정 요약
- 결과
- 변경된 파일 목록 (있는 경우)

어떤 방법으로든 작업을 완수하세요. 최소 10번 시도하세요.
포기하지 마세요.
```

---

## 5. 신뢰성

### 5.1 폴백: 타임아웃

Claude Code가 신호를 보내지 못할 경우:

1. 설정된 타임아웃(예: 30분) 경과
2. `capture-pane`으로 현재 상태 확인
3. 프롬프트 대기 상태면 완료로 간주
4. 사용자에게 결과 발송

### 5.2 신호 신뢰도

| 위험 | 대응 |
|------|------|
| Claude Code가 신호 미전송 | 타임아웃 폴백 |
| 신호 형식 오류 | 정규식 파싱 + 무시 |
| 중복 신호 | UUID로 중복 제거 |

---

## 6. tmux 명령어 레퍼런스

### 입력 전송
```bash
tmux send-keys -t session-{UUID} "사용자 메시지" Enter
```

### 출력 캡처
```bash
# 현재 화면만
tmux capture-pane -t session-{UUID} -p

# 스크롤백 포함 (최대 N줄)
tmux capture-pane -t session-{UUID} -p -S -1000
```

### 세션 관리
```bash
# 생성
tmux new-session -d -s session-{UUID}

# 종료
tmux kill-session -t session-{UUID}

# 목록
tmux list-sessions
```
