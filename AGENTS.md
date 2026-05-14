# AI 에이전트 작업 가이드

먼저 [README](/README.md)를 읽어라.

## 개발 환경

이 프로젝트는 Go 언어 프로젝트다.
Go 명령줄 인터페이스는 구성되어 있다.

## 작업 가이드라인

- 현재 요구사항 외 다른 경우의 수를 고려하지 마라. **YAGNI**해라.
- 기능 명세가 모호하거나 지시가 이해가 안되면 사용자한테 물어봐라.
- 구현 전 테스트부터 작성해라. **TDD(Red, Green, Refactor)**해라.
- 필요한 부분만 작업하되 Code smells가 보이면 사용자에게 말해라.
- 리팩터링 시 shim/glue 코드로 기존 이름을 유지하지 마라. 연결된 모든 참조를 직접 수정해라.
  예: 함수/변수/타입명 변경 시 `var oldName = newName`이나 shim wrapper를 남기지 마라. 패키지 변경 시 `import oldname "new/pkg"` 금지.
- 주석을 달지 마라. 코드 자체로 의미를 전달해라. 단, 공개 API나 복잡한 비즈니스 로직은 예외.
- 커밋 메시지는 [Conventional Commits](https://www.conventionalcommits.org/en/v1.0.0/) 스펙을 따라라.
- Go 파일 수정 후 `go fmt ./...`와 `go vet ./...`를 실행해라.
