# claude-postman: 기획 문서

> Claude Code와 사용자 사이를 이메일로 중계하는 서버 프로그램.
> 날짜: 2026-02-22

---

## 1. 프로젝트 정의

**claude-postman**은 Claude Code와 사용자 사이를 **이메일로 중계**하는 서버 프로그램이다.

```
사용자 (이메일) ←→ claude-postman (중계 서버) ←→ Claude Code (tmux 세션)
```

## 2. 제약사항

| 항목 | 결정 |
|------|------|
| 플랫폼 | Mac, Linux만 |
| 채널 | 이메일만 |
| AI 도구 | Claude Code만 |
| 기술 목표 | 적은 메모리 + 고성능 |

## 3. 핵심 기능

### 3.1 세션 관리 (tmux 기반)
- 세션 = tmux 세션 (프로젝트 단위 아님)
- UUID로 식별
- 대화형 생성 (번호 선택)

### 3.2 이메일 중계
- HTML 리치 텍스트 본문
- 구조화된 첨부파일 (xlsx, html, patch, logs)
- 본문에 Session-ID 포함 → 답장으로 자동 식별

### 3.3 오프라인 대응
- 발송 큐 (Outbox) 저장
- 온라인 전환 시 자동 발송

### 3.4 시스템 프롬프트
- 최소 10번 시도
- 우회 해결 지향
- 포기하지 않는 태도

## 4. 데이터 저장

- **SQLite** 사용
- **환경 변수 필수**: `CLAUDE_POSTMAN_DATA_DIR`
- 미설정 시 실행 불가

## 5. 설치

```bash
curl -fsSL https://get.claude-postman.dev | bash
```

- systemd/launchd 등록
- 서버 재시작 시 자동 시작

---

## 참고

- 상세 유즈케이스: `docs/usecases/SUMMARY.md`
- 기존 구현: `drone-vps/backend/scripts/task_notifier.py`
- 기존 구현: `drone-vps/backend/scripts/gmail_reader.py`
