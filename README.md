# claude-postman

Claude Code와 사용자 사이를 이메일로 중계하는 서버 프로그램.

## 개요

```
사용자 (이메일) ←→ claude-postman (중계 서버) ←→ Claude Code (tmux 세션)
```

## 특징

- **이메일 기반**: Claude Code를 이메일로 원격 제어
- **tmux 세션 관리**: 각 작업을 독립적인 tmux 세션에서 실행
- **오프라인 대응**: 네트워크 끊겨도 큐에 저장 후 온라인 시 발송
- **HTML 리치 텍스트**: 가독성 좋은 이메일 보고

## 요구사항

- Mac 또는 Linux
- tmux
- 이메일 계정 (SMTP/IMAP)

## 설치

```bash
curl -fsSL https://get.claude-postman.dev | bash
```

## 환경 변수

```bash
# 필수
export CLAUDE_POSTMAN_DATA_DIR=/path/to/data
```

## 문서

- [기획 문서](docs/ideas.md)
- [유즈케이스 상세](docs/usecases/SUMMARY.md)

## 라이선스

MIT
