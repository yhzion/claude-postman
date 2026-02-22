# 기술 스택: 테스트

## 결정 사항

```
go test (내장) + testify + go-cmp
```

## 도구

| 도구 | 용도 | 버전 |
|------|------|------|
| `testing` | 테스트 프레임워크 | Go 내장 |
| `github.com/stretchr/testify` | 단언, 스위트 | v1.8+ |
| `github.com/google/go-cmp/cmp` | 구조체 비교, diff | v0.6+ |

## 테스트 종류

| 종류 | 도구 | 실행 |
|------|------|------|
| 단위 테스트 | testing + testify | `go test ./...` |
| 통합 테스트 | testing + testify | `go test -tags=integration ./...` |
| 커버리지 | go test -cover | `go test -cover ./...` |
| 벤치마크 | go test -bench | `go test -bench=. ./...` |

## 테스트 스타일

### 단순한 단위 테스트
```go
func TestAdd(t *testing.T) {
    result := Add(1, 2)
    assert.Equal(t, 3, result)
}
```

### 테스트 스위트 (셋업/티어다운 필요 시)
```go
type MySuite struct {
    suite.Suite
}

func (s *MySuite) SetupTest() {}
func (s *MySuite) TearDownTest() {}

func TestMySuite(t *testing.T) {
    suite.Run(t, new(MySuite))
}
```

### 구조체 비교
```go
if diff := cmp.Diff(want, got); diff != "" {
    t.Errorf("mismatch (-want +got):\n%s", diff)
}
```

## 커버리지 목표

| 패키지 | 목표 |
|--------|------|
| internal/* | 80%+ |
| cmd/* | 50%+ |
| pkg/* | 90%+ |
