# 기술 스택: 품질 관리 (Quality Gates)

## 개요

100% 터미널 기반 바이브코딩을 위한 자동화된 품질 관리 시스템.

**에디터 없이 Git 훅만으로 모든 검사 수행.**

## 도구 스택

| 도구 | 용도 | 필수도 |
|------|------|--------|
| `pre-commit` | Git 훅 관리 | ⭐⭐⭐⭐⭐ |
| `golangci-lint` | 통합 린터 | ⭐⭐⭐⭐⭐ |
| `gofmt` | 포맷팅 | ⭐⭐⭐⭐⭐ |
| `goimports` | import 정리 | ⭐⭐⭐⭐⭐ |
| `go vet` | 정적 분석 | ⭐⭐⭐⭐⭐ |
| `go test` | 테스트 | ⭐⭐⭐⭐⭐ |

## 검사 순서

### pre-commit (매 커밋, 1-3초)

| 단계 | 검사 | 설명 |
|------|------|------|
| 1 | trailing-whitespace | 공백 제거 |
| 2 | end-of-file-fixer | 파일 끝 개행 |
| 3 | check-yaml | YAML 문법 |
| 4 | check-added-large-files | 대용량 파일 방지 (500KB) |
| 5 | go-fmt | Go 포맷팅 |
| 6 | go-imports | import 정리 |
| 7 | go-vet | 정적 분석 |

### pre-push (푸시 전, 30-60초)

| 단계 | 검사 | 설명 |
|------|------|------|
| 1 | golangci-lint | 통합 린팅 |
| 2 | go build | 빌드 확인 |
| 3 | go test | 전체 테스트 |

## golangci-lint 설정

```yaml
# .golangci.yml
linters:
  enable:
    # LLM 실수 방지
    - unused        # 데드 코드
    - dupl          # 중복 코드 (threshold: 80)
    - errcheck      # 에러 무시 방지

    # 코드 품질
    - funlen        # 함수 길이 (lines: 50, statements: 30)
    - gocyclo       # 복잡도 (min: 12)
    - gocognit      # 인지 복잡도
    - revive        # 스타일

    # 보안
    - gosec         # 보안 취약점

    # 기본
    - gofmt
    - goimports
    - govet
    - staticcheck

linters-settings:
  funlen:
    lines: 50
    statements: 30
  gocyclo:
    min-complexity: 12
  dupl:
    threshold: 80

run:
  timeout: 5m
```

## pre-commit 설정

```yaml
# .pre-commit-config.yaml
repos:
  # === pre-commit (빠른 것) ===

  - repo: https://github.com/pre-commit/pre-commit-hooks
    rev: v4.5.0
    hooks:
      - id: trailing-whitespace
      - id: end-of-file-fixer
      - id: check-yaml
      - id: check-added-large-files
        args: ['--maxkb=500']

  - repo: https://github.com/Bahjat/pre-commit-golang
    rev: v1.0.3
    hooks:
      - id: go-fmt
      - id: go-imports
      - id: go-vet

  # === pre-push (느린 것) ===

  - repo: https://github.com/golangci/golangci-lint
    rev: v1.55.2
    hooks:
      - id: golangci-lint
        stages: [pre-push]

  - repo: local
    hooks:
      - id: go-build
        name: go-build
        entry: go build ./...
        language: system
        types: [go]
        pass_filenames: false
        stages: [pre-push]

      - id: go-test
        name: go-test
        entry: go test ./...
        language: system
        types: [go]
        pass_filenames: false
        stages: [pre-push]
```

## 설치

```bash
# pre-commit 설치
pip install pre-commit
# 또는
brew install pre-commit

# 훅 설치
pre-commit install
pre-commit install --hook-type pre-push

# 초기 실행 (모든 파일 검사)
pre-commit run --all-files
```

## 워크플로우

```
터미널에서 코드 작성
       ↓
git add .
       ↓
git commit
       ↓
┌─────────────────┐
│ pre-commit 실행  │ (1-3초)
│ - 포맷팅 자동 수정│
│ - 기본 검사      │
└─────────────────┘
       ↓ 통과
커밋 완료
       ↓
git push
       ↓
┌─────────────────┐
│ pre-push 실행    │ (30-60초)
│ - 전체 린팅      │
│ - 빌드 확인      │
│ - 테스트 실행    │
└─────────────────┘
       ↓ 통과
푸시 완료
```

## LLM 대응 포인트

| LLM 실수 | 자동 수정 | 검사 |
|----------|----------|------|
| 포맷팅 불일치 | ✅ gofmt, goimports | - |
| 사용하지 않는 코드 | - | ✅ unused |
| 중복 코드 | - | ✅ dupl |
| 에러 무시 | - | ✅ errcheck |
| 긴 함수 | - | ✅ funlen |
| 복잡한 코드 | - | ✅ gocyclo |
| 보안 취약점 | - | ✅ gosec |
