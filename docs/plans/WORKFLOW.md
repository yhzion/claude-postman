# WORKFLOW: 작업 방법 문서

> claude-postman 구현 프로세스 정의.
> 역할 기반 팀, TDD 핸드오프, Phase별 의존성, 세션 간 인수인계.

---

## 1. 팀 역할

| 역할 | 담당 | 설명 |
|------|------|------|
| **Orchestrator** | Team Lead | Phase 계획, 작업 분배, 핸드오프 조율 |
| **Tester** | 테스트 작성 | SSOT 기반 테스트 코드 작성 (Red 단계) |
| **Implementer** | 구현 | 테스트 통과시키는 프로덕션 코드 작성 (Green 단계) |
| **Reviewer** | 리뷰 | 코드 품질, SSOT 일치, 컨벤션 검증 |

---

## 2. 의존성 그래프 & Phase 분류

```
Phase 1 (기반, 독립)
├── Storage (03-storage.md)
└── Config  (02-config.md)

Phase 2 (Phase 1 의존)
├── Session (04-session.md + 01-tmux-output-capture.md)  ← Storage
└── Email   (05-email.md)                                ← Config, Storage

Phase 3 (Phase 2 의존)
├── Serve   (06-service.md serve 부분)                   ← Session, Email, Config, Storage
└── Doctor/Service (06-service.md doctor/service 부분)   ← Config
```

---

## 3. TDD 핸드오프 프로토콜

각 모듈은 다음 순서로 진행:

```
Tester                  Implementer             Reviewer
  │                         │                       │
  ├─ 테스트 작성 (Red)      │                       │
  │  └─ SSOT 기반           │                       │
  │  └─ 실행 → 전부 FAIL    │                       │
  │                         │                       │
  ├──── 핸드오프 ──────────→│                       │
  │                         ├─ 구현 (Green)         │
  │                         │  └─ 테스트 통과        │
  │                         │  └─ go build 통과      │
  │                         │                       │
  │                         ├──── 핸드오프 ─────────→│
  │                         │                       ├─ 리뷰
  │                         │                       │  └─ SSOT 일치
  │                         │                       │  └─ 코드 품질
  │                         │                       │  └─ lint 통과
  │                         │                       │
  │                         │                       └─ 완료/피드백
```

### 핸드오프 시 전달 사항

**Tester → Implementer:**
- 테스트 파일 경로
- `go test ./internal/{module}/... -v` 실행 결과 (전부 FAIL 확인)
- 구현해야 할 구조체/함수 목록

**Implementer → Reviewer:**
- 구현된 파일 목록
- `go test ./internal/{module}/... -v` 실행 결과 (전부 PASS 확인)
- `go build ./...` 통과 확인

---

## 4. 파이프라인 병렬성

같은 Phase 내 모듈은 파이프라인으로 병렬 진행 가능:

```
시간 →
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
Storage:  [Tester] → [Implementer] → [Reviewer]
Config:              [Tester] → [Implementer] → [Reviewer]
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
```

- Tester가 Storage 테스트 완료 후, 바로 Config 테스트 시작
- Implementer가 Storage 구현하는 동안, Tester가 Config 테스트 작성
- 이렇게 파이프라인 방식으로 병렬성 확보

---

## 5. 세션 간 인수인계

### PROGRESS.md 규약

매 세션 종료 시 `docs/plans/PROGRESS.md`를 업데이트:

1. **모듈 상태 테이블**: 각 모듈의 현재 상태 (미착수/테스트작성/구현중/리뷰중/완료)
2. **다음 작업 섹션**: 새 세션이 바로 시작할 수 있는 구체적 지시
3. **블로커**: 해결이 필요한 이슈

### 새 세션 시작 시

1. `docs/plans/PROGRESS.md` 읽기
2. "다음 작업" 섹션의 지시에 따라 작업 시작
3. 해당 모듈의 플랜 파일 (`docs/plans/0N-*.md`) 참조

---

## 6. 검증 기준

각 Phase 완료 시:

```bash
go test ./internal/...     # 모든 테스트 통과
go build ./...             # 빌드 성공
golangci-lint run          # 린트 통과
```
