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
│ claude-postman (Go 프로세스)                      │
│                                                  │
│  - 세션별 FIFO 파일 감시                          │
│  - 신호 수신 → capture-pane → 이메일 발송         │
│  - 3개 goroutine: IMAP폴링, Outbox플러시, FIFO   │
└──────────┬───────────────────────────────────────┘
           │ 생성/관리
           ▼
┌──────────────────────────────────────────────────┐
│ tmux: session-{UUID} (워커)                       │
│                                                  │
│  claude --dangerously-skip-permissions            │
│    - 사용자 요청 작업 수행                         │
│    - 완료 시: Bash("echo 'DONE:{UUID}'            │
│      > /tmp/claude-postman/{UUID}.fifo")          │
└──────────────────────────────────────────────────┘
```

- **claude-postman**: 메인 프로세스. 세션별 FIFO 파일에서 완료 신호를 블로킹 읽기.
- **session-{UUID}**: Claude Code가 실행되는 워커 세션. 작업 완료 시 FIFO에 신호 기록.

### 2.2 입출력 방식

| 항목 | 방식 | 설명 |
|------|------|------|
| 입력 | `tmux send-keys` | 워커 세션에 텍스트 전송 |
| 완료 감지 | Unix FIFO (named pipe) | 워커가 `echo 'DONE:{UUID}' > {FIFO}` |
| 출력 캡처 | `tmux capture-pane` | 신호 수신 후 500ms 딜레이, 1회 캡처 |
| 출력 처리 | 전체 전달 | capture-pane 결과를 파싱 없이 그대로 사용 |

### 2.3 신호 수신 메커니즘 (FIFO)

```
세션 생성 시:
  1. mkfifo /tmp/claude-postman/{UUID}.fifo
  2. goroutine에서 블로킹 read 시작

Claude Code 작업 완료 시:
  1. echo 'DONE:{UUID}' > /tmp/claude-postman/{UUID}.fifo

claude-postman 수신:
  1. FIFO에서 DONE:{UUID} 읽기 (즉시 감지)
  2. 500ms 딜레이 (렌더링 대기)
  3. capture-pane 실행
  4. FIFO 파일은 세션 종료 시 삭제
```

**FIFO goroutine 관리:**
- 세션 생성 시 전용 goroutine 스폰 (세션 1개 = goroutine 1개)
- 세션 종료 시 goroutine도 종료
- goroutine 내부: `for` 루프에서 FIFO 읽기 → DONE 수신 → 처리 → 다시 대기

**Graceful shutdown:**
```
서버 종료 신호 (SIGINT/SIGTERM)
  ↓
context.Cancel()
  ↓
각 세션의 FIFO에 sentinel 값 "SHUTDOWN" 쓰기 (non-blocking)
  ├─ O_WRONLY|O_NONBLOCK으로 열기
  ├─ 성공 → "SHUTDOWN" 쓰기 → 블로킹 읽기 해제
  └─ ENXIO (읽기 측 없음) → goroutine 이미 종료 → 스킵
  ↓
goroutine 종료 대기 (타임아웃 5초)
```

FIFO의 장점:
- **즉시 감지**: 블로킹 읽기로 폴링 없이 즉시 신호 감지
- **간단**: 추가 tmux 세션 불필요
- **신뢰성**: OS 레벨 파이프, 데이터 손실 없음

### 2.4 실행 옵션

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
4. Claude Code 완료 → echo 'DONE:{UUID}' > /tmp/claude-postman/{UUID}.fifo
     ↓
5. claude-postman이 FIFO에서 신호 수신 (즉시 감지)
     ↓
6. 500ms 딜레이 (렌더링 대기)
     ↓
7. tmux capture-pane -t session-{UUID} -p -S -1000
     ↓
8. 전체 출력을 이메일 본문으로 발송
```

---

## 4. 시스템 프롬프트

Claude Code에 주입하는 시스템 프롬프트 핵심 내용:

```
작업이 완료되면 반드시 다음 명령을 실행하세요:
echo 'DONE:{SESSION_ID}' > /tmp/claude-postman/{SESSION_ID}.fifo

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

1. 설정된 타임아웃 (기본: 30분, config에서 변경 가능) 경과
2. `capture-pane`으로 현재 상태 확인
3. 프롬프트 대기 상태면 완료로 간주
4. 사용자에게 결과 발송

### 5.2 신호 신뢰도

| 위험 | 대응 |
|------|------|
| Claude Code가 신호 미전송 | 타임아웃 폴백 (30분) |
| FIFO 파일 손실 | 세션 시작 시 재생성 |
| 신호 형식 오류 | 정규식 파싱 + 무시 |
| 중복 신호 | UUID로 중복 제거 |

### 5.3 FIFO 라이프사이클

```
세션 생성:  mkfifo /tmp/claude-postman/{UUID}.fifo
세션 종료:  rm /tmp/claude-postman/{UUID}.fifo
서버 시작:  /tmp/claude-postman/ 디렉터리 생성 (없으면)
서버 종료:  FIFO 파일들 정리
```

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
