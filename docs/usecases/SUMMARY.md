# claude-postman: 유즈케이스 종합 문서

> 이 문서는 브레인스토밍을 통해 수집된 모든 유즈케이스를 통합 정리한 것입니다.
> 날짜: 2026-02-22

---

## 1. 프로젝트 개요

### 1.1 정의
**claude-postman**은 Claude Code와 사용자 사이를 **이메일로 중계**하는 서버 프로그램.

```
사용자 (이메일)
     ↕ 메시지
claude-postman (중계 서버)
     ↕ 제어
Claude Code (tmux 세션)
```

### 1.2 핵심 가치
- Claude Code를 **원격에서 이메일로 제어**
- **비동기** 작업 진행
- **오프라인** 대응 가능
- **끈기 있는 문제 해결** (최소 10번 시도)

---

## 2. 제약사항

### 2.1 플랫폼
- ✅ Mac
- ✅ Linux
- ❌ Windows (고려 안 함)

### 2.2 기술적 목표
- **적은 메모리**로 동작
- **고성능** 달성

### 2.3 종속성
- **Claude Code 전용** (다른 AI 도구 지원 안 함)
- **이메일만** 사용 (다른 메신저 확장은 향후 고려)

---

## 3. 핵심 컨셉

### 3.1 중계 서버
claude-postman은 **사용자와 Claude Code 사이를 중계**하는 역할.

```
┌─────────────────────────────────────────────────────┐
│ 사용자                                               │
│ (이메일)                                             │
└─────────────────┬───────────────────────────────────┘
                  │ 메시지 (요청/답변)
                  ▼
┌─────────────────────────────────────────────────────┐
│ claude-postman (중계 서버)                           │
│                                                     │
│  - 이메일 수신/발송                                  │
│  - 세션 관리                                        │
│  - 메시지 라우팅                                    │
└─────────────────┬───────────────────────────────────┘
                  │ Claude Code 제어
                  ▼
┌─────────────────────────────────────────────────────┐
│ tmux 세션                                           │
│  └── Claude Code 실행                               │
└─────────────────────────────────────────────────────┘
```

### 3.2 tmux 세션 관리
- **TeamWorks = tmux 세션**
- 세션은 **프로젝트 단위가 아님** → 독립적 작업 단위
- 세션마다 **시작 디렉터리** 지정 (기본: 홈 디렉터리)

### 3.3 세션 식별
- **UUID**로 세션 식별
- 이메일 본문에 `Session-ID: xxx` 포함
- 답장 시 자동으로 세션 식별

---

## 4. 세션 라이프사이클

### 4.1 세션 생성 (템플릿 포워드)
```
[init] claude-postman init → 템플릿 이메일 발송
        ↓
[사용자] 템플릿 이메일을 포워드 (디렉터리/모델/태스크 편집)
        ↓
[검증] From 확인 + 템플릿 Message-ID 참조 확인
        ↓
[파싱] 본문에서 Directory, Model, Task 추출
        ↓
[완료] 세션 생성 → tmux 세션 시작 → Claude Code 실행
```

### 4.2 작업 진행
```
사용자 요청 (이메일)
     ↓
claude-postman 수신 → 세션 식별
     ↓
tmux send-keys로 Claude Code에 입력
     ↓
Claude Code 작업 수행
     ↓
결과 수집 → 이메일 발송
     ↓
사용자 답장 → 다시 루프
```

### 4.3 세션 종료
```
[사용자] "종료" / "끝"
        ↓
tmux 세션 종료
        ↓
리소스 정리
        ↓
종료 확인 이메일 발송
```

---

## 5. 이메일 구조

### 5.1 본문 (HTML 리치 텍스트)
- **항상 HTML 형식**
- **가독성 좋게** 데코레이트
- 마크다운/코드는 **변환**해서 표시
  - 마크다운 → HTML
  - 코드 → 하이라이팅 + 스타일링

### 5.2 첨부파일
| 파일 | 포맷 | 용도 |
|------|------|------|
| report.xlsx | 엑셀 | 구조화된 상세 데이터 |
| details.html | HTML | 반응형/인터랙티브 보고서 |
| diff.patch | Patch | git diff |
| logs.txt | Text | 실행 로그 |

### 5.3 이메일 타입
| 타입 | 내용 | 발송 시점 |
|------|------|----------|
| 진행 알림 | 작업 시작 | 작업 시작 시 |
| 완료 보고 | 요약 + 진행과정 + 통찰 | 작업 완료 시 |
| 질문 | 확인 필요 사항 | 사용자 입력 필요 시 |
| 에러 | 에러 내용 + 해결 제안 | 오류 발생 시 |

### 5.4 출력 포맷
- 사용자가 지정하면 그 포맷 사용
- 지정 없으면 **추천 포맷** 자동 선택
- **별도로 포맷 질문 이메일 안 보냄**

---

## 6. 오프라인 대응

### 6.1 발송 큐 (Outbox)
```
오프라인 상태
     ↓
이메일 생성 → Outbox에 저장 (pending)
     ↓
온라인 전환 감지
     ↓
pending 이메일 순차 발송
```

### 6.2 이점
- 끊김 없는 작업
- 비행기, 카페 등에서도 사용
- 발송 실패해도 큐에 남아있음

---

## 7. 시스템 프롬프트

### 7.1 핵심 원칙
Claude Code는 **사용자가 원하는 작업을 어떤 방법으로든 완수**하려는 의지를 가진다.

### 7.2 동작 방식
- **최소 10번 시도**
- 기능이 안 되면 **우회 방법** 시도
- "안 됩니다"보다 "이렇게 하면 됩니다"
- 중간에 포기하지 않음

---

## 8. 데이터 모델

### 8.1 sessions 테이블
```sql
CREATE TABLE sessions (
    id              TEXT PRIMARY KEY,    -- UUID
    tmux_name       TEXT NOT NULL,       -- tmux 세션명
    working_dir     TEXT NOT NULL,       -- 시작 디렉터리
    model           TEXT NOT NULL,       -- sonnet | opus | haiku
    status          TEXT NOT NULL,       -- creating | active | idle | ended
    created_at      DATETIME NOT NULL,
    updated_at      DATETIME NOT NULL,
    last_prompt     TEXT,
    last_result     TEXT
);
```

### 8.2 outbox 테이블
```sql
CREATE TABLE outbox (
    id              TEXT PRIMARY KEY,
    session_id      TEXT NOT NULL,
    message_id      TEXT,              -- 이메일 Message-ID (스레드 매칭)
    subject         TEXT NOT NULL,
    body            TEXT NOT NULL,
    attachments     TEXT,               -- JSON
    status          TEXT NOT NULL,      -- pending | sent | failed
    created_at      DATETIME NOT NULL,
    sent_at         DATETIME,

    FOREIGN KEY (session_id) REFERENCES sessions(id)
);
```

### 8.3 inbox 테이블 (대기열)
```sql
CREATE TABLE inbox (
    id              TEXT PRIMARY KEY,
    session_id      TEXT NOT NULL,
    body            TEXT NOT NULL,
    created_at      DATETIME NOT NULL,
    processed       INTEGER NOT NULL DEFAULT 0,

    FOREIGN KEY (session_id) REFERENCES sessions(id)
);
```

### 8.4 template 테이블
```sql
CREATE TABLE template (
    id              TEXT PRIMARY KEY,
    message_id      TEXT NOT NULL,     -- 템플릿 이메일의 Message-ID
    created_at      DATETIME NOT NULL
);
```

### 8.5 DB 위치
- **환경 변수 필수**: `CLAUDE_POSTMAN_DATA_DIR`
- 미설정 시 프로그램 실행 불가 (오류 발생)
- 버전 관리 제외 (git ignore)

---

## 9. 설치 및 배포

### 9.1 설치 방식
```bash
curl -fsSL https://get.claude-postman.dev | bash
```

### 9.2 시스템 서비스
- systemd (Linux) / launchd (Mac) 등록
- 서버 재시작 시 자동 시작

### 9.3 재시작 복구
- 서버 재시작 → tmux 세션 사라짐
- DB에서 active/idle 세션 조회
- tmux 세션 재생성 + `claude --resume`으로 문맥 복구 시도
- 사용자에게 "세션 복구됨" 알림

---

## 10. 설정

### 10.1 설정 파일
- `~/.claude-postman/config.toml` (init 마법사로 생성)
- 환경변수로 오버라이드 가능 (접두사: `CLAUDE_POSTMAN_`)
- 자세한 내용: [02-config.md](../architecture/02-config.md)
