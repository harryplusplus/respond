# AI 에이전트 작업 가이드

먼저 [README](/README.md)를 읽어라.

## 개발 환경

이 프로젝트는 Go 언어 프로젝트다.
Go 명령줄 인터페이스는 구성되어 있다.

## 작업 가이드라인

- 현재 요구사항 외 다른 경우의 수를 고려하지 마라. **YAGNI**해라.
- 기능 명세가 모호하거나 지시가 이해가 안되면 사용자한테 물어봐라.
- 구현 전 테스트부터 작성해라. **TDD(Red, Green, Refactor)**해라.
- `panic`을 쓰지 마라. 일반 비즈니스 로직은 `error` 반환 체인으로 처리해라. `panic`은 `init`이나 할당이 실패할 수 없는 리소스 초기화 등 회복 불가능한 상황에서만 허용한다.
- `_ = foo()` 식으로 에러를 무시하지 마라. Go는 에러를 명시적으로 처리하게 강제하는 언어다. 무시할 바에는 자동 전파되는 JavaScript `try/catch`를 쓰러 가라. 에러는 반드시 로깅하거나 호출자에게 전파해라.
- 작업 시 shim/glue 코드로 기존 이름을 유지하지 말고 연결된 모든 참조를 직접 수정해라.
  `rg`로 영향 범위부터 파악하고, 함수/필드/테스트 할 것 없이 하나를 바꾸면 의존하는 모든 것을 함께 바꿔라.
  예: `var oldName = newName` 금지, `import oldname "new/pkg"` 금지.
- 주석은 코드만으로 알 수 없는 맥락만 적어라.
  - 나쁨: `// http 요청을 보냄` → 함수명 `sendRequest()`가 이미 설명
  - 나쁨: `// 응답 검증` → 아래 assert가 이미 검증
  - 좋음: `// -ldflags로 빌드 시 주입` → 코드만으로 알 방법 없음
  - 좋음: `// API 키는 response body가 아닌 header로 전달` → 문서화되지 않은 동작
  - 좋음: `// name 필드는 codex validation에서 필수` — codex 외부 라이브러리 제약
- 함수/변수 위 doc string 금지. 함수명이 설명을 대신해야 한다.
- `//`, `// TODO`, `// NOTE` 모두 동일 기준 적용.
- 커밋 메시지는 [Conventional Commits](https://www.conventionalcommits.org/en/v1.0.0/) 스펙을 따라라.
- Go 파일 수정 후 `go fmt ./...`, `go vet ./...`, `errcheck ./...`, `staticcheck ./...`를 실행해라.
  통합 테스트(`test/integration/`)는 `-tags=integration` 필요:
  `go vet -tags=integration ./test/integration/`
  `errcheck -tags=integration ./test/integration/`
  `staticcheck -tags=integration ./test/integration/`

## 통합 테스트

```bash
go test -tags=integration -run TestIntegration ./test/integration/ -v -count=1 -parallel 1 -timeout=900s
```

- `//go:build integration` 태그 사용, `go test ./...`에서 제외
- `CROF_API_KEY` 환경변수 필수 (provider: `crof`, model: `kimi-k2.6-precision`)
- `codex` 바이너리가 `$PATH`에 있어야 함
