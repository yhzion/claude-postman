# claude-postman

Claude Code와 사용자 사이를 이메일로 중계하는 서버 프로그램.

```
사용자 (이메일) ←→ claude-postman (중계 서버) ←→ Claude Code (tmux 세션)
```

## 특징

- **이메일 기반**: Claude Code를 이메일로 원격 제어
- **tmux 세션 관리**: 각 작업을 독립적인 tmux 세션에서 실행
- **오프라인 대응**: 네트워크 끊겨도 큐에 저장 후 온라인 시 발송
- **HTML 리치 텍스트**: 가독성 좋은 이메일 보고 (Markdown → HTML)

## 요구사항

- Mac 또는 Linux
- Go 1.24+
- tmux
- Claude Code
- 이메일 계정 (SMTP/IMAP)

## 설치

```bash
curl -fsSL https://get.claude-postman.dev | bash
```

## 빠른 시작

```bash
claude-postman init              # 설정 마법사
claude-postman serve             # 서버 실행
claude-postman doctor            # 환경 진단
```

> CLI 전체 구조는 [06-service.md](docs/architecture/06-service.md) 참조.

## 설정

`~/.claude-postman/config.toml`에 이메일 자격 증명과 설정을 저장합니다.
환경변수(`CLAUDE_POSTMAN_` 접두사)로 오버라이드 가능합니다.

> 설정 상세: [02-config.md](docs/architecture/02-config.md)

## 문서

### 아키텍처 설계 (SSOT)

- [01. tmux 출력 캡처](docs/architecture/01-tmux-output-capture.md)
- [02. Config](docs/architecture/02-config.md)
- [03. Storage (SQLite)](docs/architecture/03-storage.md)
- [04. Session 관리](docs/architecture/04-session.md)
- [05. Email (SMTP/IMAP)](docs/architecture/05-email.md)
- [06. CLI, Service, Doctor](docs/architecture/06-service.md)

### 참고

- [기획 문서](docs/ideas.md)
- [유즈케이스](docs/usecases/SUMMARY.md)
- [기술 스택 결정](docs/tech-stack/)

## 라이선스

MIT
