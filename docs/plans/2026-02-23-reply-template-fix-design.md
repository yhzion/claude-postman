# 설계: 템플릿 Forward → Reply 전환

> 날짜: 2026-02-23

## 문제

1. **Gmail Forward는 `In-Reply-To`/`References` 헤더를 설정하지 않음** — `isTemplateRef()`가 매칭 불가 → 새 세션 생성 실패 (핵심 버그)
2. **템플릿 자기수신** — 서버가 보낸 템플릿 이메일이 IMAP 폴에서 unmatched로 감지됨
3. **Gmail 자동 읽음 처리** — 사용자의 이메일이 UNSEEN 필터에 걸리지 않을 수 있음 (미해결, 발생 시 대응)

## 결정

- Forward 대신 **Reply** 방식으로 전환
- Reply는 `In-Reply-To` 헤더를 설정하므로 기존 `isTemplateRef()` 로직이 그대로 동작
- 자기수신 템플릿은 `Poll()`에서 MessageID 기반으로 필터링

## 변경 사항

### 1. 템플릿 본문 (`email.go:SendTemplate`)

안내 문구를 Forward → Reply로 변경.

### 2. 자기수신 필터 (`email.go:Poll`)

`Poll()`에서 이메일의 MessageID가 template 테이블에 존재하면 스킵 + 읽음 처리.

### 3. 아키텍처 문서 (`docs/architecture/05-email.md`)

섹션 2.4 "포워드" → "답장" 동기화.

### 4. 테스트 (`email_test.go`)

- 테스트명 "template forward" → "template reply" 변경
- 자기수신 필터링 테스트 추가

## 변경 파일

| 파일 | 변경 |
|------|------|
| `internal/email/email.go` | 템플릿 본문 수정, Poll에 자기수신 필터 추가 |
| `internal/email/email_test.go` | 테스트명 변경, 자기수신 필터 테스트 추가 |
| `docs/architecture/05-email.md` | Forward → Reply 문서 동기화 |
