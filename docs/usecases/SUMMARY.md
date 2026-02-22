# claude-postman: 유즈케이스 종합 문서

> 초기 브레인스토밍을 통해 수집된 유즈케이스 모음. 상세 설계는 [아키텍처 문서](../architecture/) 참조 (SSOT).
> 날짜: 2026-02-22

---

## 1. 프로젝트 개요

**claude-postman**은 Claude Code와 사용자 사이를 **이메일로 중계**하는 서버 프로그램.

핵심 가치:
- Claude Code를 **원격에서 이메일로 제어**
- **비동기** 작업 진행
- **오프라인** 대응 가능
- **끈기 있는 문제 해결** (최소 10번 시도)

> 제약사항, 기술 스택: [CLAUDE.md](../../CLAUDE.md)

---

## 2. 유즈케이스: 세션 라이프사이클

> 상세 설계: [04-session.md](../architecture/04-session.md)

### 2.1 세션 생성
- `init` 실행 → 템플릿 이메일 발송
- 사용자가 템플릿을 편집(디렉터리/모델/태스크)하여 포워드
- 검증 (From 확인 + 템플릿 Message-ID 참조) 후 세션 생성

### 2.2 작업 진행
- 사용자 이메일 → 세션 식별 → tmux로 입력 전달
- Claude Code 작업 완료 → 결과 이메일 발송
- 사용자 답장으로 반복

### 2.3 세션 종료
- 사용자가 "종료" 이메일 → tmux 세션 정리 → 종료 확인 이메일

---

## 3. 유즈케이스: 이메일 처리

> 상세 설계: [05-email.md](../architecture/05-email.md)

### 3.1 HTML 리치 텍스트 본문
- 항상 HTML 형식
- Markdown → HTML 변환 (goldmark + chroma)

### 3.2 이메일 타입

> 정확한 제목 형식: [05-email.md §3.3](../architecture/05-email.md)

| 타입 | 발송 시점 |
|------|----------|
| 세션 시작 | 세션 생성 완료 시 |
| 작업 완료 | Claude Code 작업 완료 시 |
| 입력 요청 | 사용자 입력 필요 시 |
| 에러 | 오류 발생 시 |
| 세션 복구/종료 | 서버 재시작 또는 세션 종료 시 |

---

## 4. 유즈케이스: 오프라인 대응

> 상세 설계: [05-email.md §4](../architecture/05-email.md)

- 오프라인 시 outbox 큐에 저장 (pending)
- 온라인 전환 시 자동 발송 (지수 백오프 재시도)
- 끊김 없는 작업 (비행기, 카페 등)

---

## 5. 유즈케이스: 시스템 프롬프트

> 상세 설계: [01-tmux-output-capture.md](../architecture/01-tmux-output-capture.md)

- 사용자가 원하는 작업을 어떤 방법으로든 완수
- 최소 10번 시도, 우회 방법 탐색
- "안 됩니다"보다 "이렇게 하면 됩니다"

---

## 6. 유즈케이스: 설치 및 복구

> 상세 설계: [06-service.md](../architecture/06-service.md)

- systemd (Linux) / launchd (Mac) 등록
- 서버 재시작 시 `--resume`으로 자동 복구 시도

---

## 7. 참조

- 데이터 모델 (SQL 스키마): [03-storage.md](../architecture/03-storage.md)
- 설정: [02-config.md](../architecture/02-config.md)
- 세션 관리: [04-session.md](../architecture/04-session.md)
- 이메일 처리: [05-email.md](../architecture/05-email.md)
- CLI/서비스: [06-service.md](../architecture/06-service.md)
