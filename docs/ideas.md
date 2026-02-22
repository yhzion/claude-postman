# claude-postman: 기획 문서

> 초기 브레인스토밍 문서. 상세 설계는 [아키텍처 문서](architecture/) 참조 (SSOT).
> 날짜: 2026-02-22

---

## 1. 프로젝트 정의

**claude-postman**은 Claude Code와 사용자 사이를 **이메일로 중계**하는 서버 프로그램이다.

```
사용자 (이메일) ←→ claude-postman (중계 서버) ←→ Claude Code (tmux 세션)
```

## 2. 제약사항

→ 상세: [CLAUDE.md](../CLAUDE.md)

| 항목 | 결정 |
|------|------|
| 플랫폼 | Mac, Linux만 |
| 채널 | 이메일만 |
| AI 도구 | Claude Code만 |
| 기술 목표 | 적은 메모리 + 고성능 |

## 3. 핵심 아이디어

> 아래는 초기 아이디어입니다. 최종 설계와 다를 수 있습니다.
> 최종 설계: [04-session.md](architecture/04-session.md), [05-email.md](architecture/05-email.md)

- 세션 = tmux 세션 (프로젝트 단위 아님), UUID로 식별
- 세션 생성은 **템플릿 이메일 포워드** 방식 (→ [05-email.md](architecture/05-email.md) 2.4절)
- HTML 리치 텍스트 본문, Session-ID로 답장 자동 식별
- 오프라인 시 outbox 큐에 저장, 온라인 전환 시 자동 발송
- 시스템 프롬프트: 최소 10번 시도, 포기하지 않는 태도

## 4. 데이터 저장

→ 상세: [03-storage.md](architecture/03-storage.md)

- SQLite 사용
- 설정은 `~/.claude-postman/config.toml` (→ [02-config.md](architecture/02-config.md))

## 5. 설치 및 서비스

→ 상세: [06-service.md](architecture/06-service.md)

- systemd (Linux) / launchd (macOS) 등록
- 서버 재시작 시 자동 시작

---

## 참고

- 유즈케이스: [docs/usecases/SUMMARY.md](usecases/SUMMARY.md)
- 기존 구현: `drone-vps/backend/scripts/task_notifier.py`
- 기존 구현: `drone-vps/backend/scripts/gmail_reader.py`
