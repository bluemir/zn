## 행동 규칙

- 애매하거나 결정이 필요한 사항은 질문을 반드시 할것
- 의도나 용어는 추측하지 말고 물어볼 것
- 명확해지기 전에는 계속 해서 재질문 할것

## Code Style

- 과도한 추상화를 하지 않는다.
	- interface 는 반드시 필요하기 전에는 도입하지 않는다.
	- Dependency Injection 은 최소화 한다.
	- 단순 중복이 있다고 helper 를 만들지 않는다.
- 코드는 의도에 따라 작성한다.
	- 변수명은 과도하게 축약하지 않는다.(eg 한글자 변수명)
		- 예외 loop 순환자는 한글자 변수명을 쓸수 있다.
	- 함수의 parameter나 return 값에 flag 를 무분별 하게 넣지 않는다.

## 문서

### Tasks

- 실행 계획이나 추후 개선점 등 해야 할 일은 `docs/tasks.md` 에 기록한다.
- `- [ ] 내용` 처럼 checklist 형태로 기록 한다.
- 하나의 list가 되도록 기록한다.

### ADR (Architecture Decision Records)

- 아키텍처/기술 선택에 trade-off가 있는 결정을 할 때 `docs/adr/`에 ADR을 작성한다
- 파일명: `ADR-NNNN-제목.md` (예: `ADR-0001-use-redis-for-caching.md`)
- 기존 ADR이 있으면 번호를 이어서 채번한다
- 버그 수정, 단순 리팩토링, 선택지가 하나뿐인 경우는 작성하지 않는다

