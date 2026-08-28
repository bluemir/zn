# ADR-0096 tab 폭은 파일마다 다르고 `.editorconfig` 가 정한다

- 상태: 채택
- 날짜: 2026-08-28
- 관련: ADR-0020(화면 폭을 재는 자리), ADR-0047(autoindent), ADR-0048(`.editorconfig` 를 들여쓰기에 쓴다), ADR-0052(저장할 때의 모습), ADR-0080(경로에서 고른 것을 경로와 나눠 든다), ADR-0078(검색 결과 판)

## 맥락

`docs/tasks.md` 가 오래 열어 두고 결정할 것을 적어 두었다.

> 화면의 tab 폭을 `.editorconfig` 의 `tab_width` 로 받을지 정한다.
> - 지금 `const tabWidth = 4` 에 그리기·커서 계산·줄바꿈·공백 마커와 `blankColumns` 가 매달려 있다
> - `tab_width = 8` 이라고 적힌 저장소에서 tab 하나가 화면에서는 4 칸이라 한 단계의 크기가 적힌 것과 갈린다

`.editorconfig` 를 읽는 길은 ADR-0048 이 이미 냈다. 새로 정할 것은 셋이다. **폭을 어느 자리에서 드느냐**, **`tab_width` 가 적혀 있지 않을 때 무엇을 답하느냐**, 그리고 **파일 내용이 아닌 글까지 그 폭으로 잴 것이냐**다.

## 1. 폭은 `Buffer` 가 들고 재는 함수는 인자로 받는다

`const tabWidth = 4` 를 `var` 로 바꿔 두고 활성 buffer 가 바뀔 때마다 세팅하는 길이 가장 짧다. 고칠 자리가 거의 없다. 그런데 `.editorconfig` 는 경로별이라 `tab_width = 8` 인 하위 디렉터리와 그렇지 않은 곳이 한 화면에 tab 으로 같이 열려 있을 수 있고, 그때 그 변수가 어느 순간의 값인지가 숨는다. 이 편집기는 tab 을 오갈 때 유지되어야 하는 것을 전부 `Buffer` 에 두기로 이미 정해 두었다(buffer.go 의 「파일 내용이 아니라 이 파일을 어떻게 보고 있는지다」).

그래서 **`Buffer` 에 담고, 재는 함수는 마지막 인자로 받는다.**

화면 함수를 `Buffer` 의 메서드로 바꾸는 길도 있었다. 인자가 늘지 않아 보기에 낫지만 `cluster.go` 머리글이 「`Buffer` 의 메서드가 아니라 `[]byte` 한 줄을 받는 함수들이고, 커서를 옮기는 쪽과 그리는 쪽이 같이 쓴다」고 적어 둔 근거를 뒤집는다. tabline 의 제목처럼 buffer 와 상관없는 글도 이 함수들이 재기 때문이다.

**재 보니 파급이 좁았다.** 파일 내용을 재는 호출 자리 서른 남짓 중 `buffer-move.go`·`buffer-screen.go`·`buffer-reload.go`·`buffer-indent.go` 가 전부 `Buffer` 의 메서드라 `buf` 가 이미 손에 있다. 밖에 있는 것은 `render-row.go` 의 `expandRow`·`renderRow` 와 `sticky.go` 뿐이고 그 둘도 부르는 쪽이 buffer 를 안다.

### 게으르게 정하지 않는다

처음에는 `indentUnit` 과 같은 손으로 갔다. 게으르게 정하고 담아 두었다가 경로가 달라지면 다시 정하는 것이다.

**그것이 틀렸다.** 그리는 쪽의 `visibleRows`·`advanceRows`·`retreatRows`·`positionAt`·`cursorScreenPos` 가 **값 receiver**(`func (buf Buffer)`) 다. 게으른 캐시는 담는 자리가 있어야 하니 포인터 receiver 가 되는데, Go 가 값 receiver 안의 지역 사본 주소를 알아서 넘겨 주므로 **컴파일은 되면서 담은 것이 사본에만 남는다.** 그러면 화면을 다시 그릴 때마다, 그것도 줄마다, 디스크에서 `.editorconfig` 를 찾아 올라간다. 스크롤 한 번이 파일시스템을 화면 높이만큼 훑는다.

그래서 **`language` 와 같은 손으로 간다.** buffer 를 지을 때 경로에서 한 번 정해 `int` 하나로 들고 있고, 읽는 것은 값 receiver 다. 이름이 바뀌는 자리는 `buffer-save.go` 하나뿐이라고 그 파일이 못박아 두었고 거기서 `language` 를 이미 다시 고르므로, 붙일 자리도 그 한 줄 옆이다(ADR-0080).

빈 칸(`0`) 은 기본값으로 읽는다. `Buffer{path: "b.txt"}` 처럼 손으로 지은 것이 시험에 있고, 0 을 그대로 넘기면 `clusterAt` 의 나머지 연산이 죽는다.

## 2. `indent_size` 만 적힌 파일도 그 값을 받는다

editorconfig 명세가 「`tab_width` 의 기본값은 `indent_size`」라고 정해 두었고 `editorconfig-core-go` 가 읽어 오는 자리에서 그것을 채워 준다(`definition.go` 의 `tab_width defaults to indent_size`).

그것을 되돌리지 않는다. 되돌리려면 `def.Raw["tab_width"]` 를 직접 봐야 하는데, 그렇게 해서 얻는 것은 「우리만 다르게 읽는다」이다. 명세대로 읽는 쪽이 다른 편집기와 같은 화면을 낸다.

이 저장소로 재 보면 `[*]` 에 `indent_size` 가 없어서 `.go` 는 그대로 4 이고, `[*.yaml]` 은 `indent_size = 2` 라 tab 이 2 칸이 된다. yaml 은 tab 을 쓰지 않으므로 실제로 달라지는 것은 없다.

## 3. 적힌 것이 없으면 4 다

터미널과 vim 의 기본 tab stop 은 8 이다. 그것을 따르지 않는다.

`cluster.go` 가 4 를 고른 근거가 그대로 서 있다. 8 칸은 깊게 들여쓴 코드를 화면 밖으로 밀어낸다. 이 결정이 더하는 것은 **「적힌 것이 있으면 따른다」** 하나이고, 아무 말도 없는 저장소에서 보이는 것을 바꾸지 않는다. 지금 열려 있는 모든 파일의 들여쓰기가 두 배로 넓어지는 것은 이 결정이 사려던 것이 아니다.

1 보다 작은 값도 기본값으로 물러난다. 0 을 그대로 넘기면 `clusterAt` 의 나머지 연산이 죽는다.

## 4. UI 문자열은 기본 폭으로 잰다

`screenWidthOf` 를 부르는 자리 쉰 곳은 tabline 의 제목, statusBar, 팔레트 후보처럼 **파일 내용이 아닌 글**이다. tab 이 들어올 일이 없어서 어느 폭으로 재든 답이 같다. 그 자리에 뜻 없는 인자를 붙이지 않는다.

가르는 선은 함수 이름에 두었다. tab 폭을 마지막 인자로 받는 함수는 파일 내용을 재는 것이고, 받지 않는 함수(`screenWidthOf`·`screenWidthOfStyled`·`truncateToWidth`·`trimLeftToWidth`) 는 UI 쪽이다. `clusterSize` 도 받지 않는데 까닭이 다르다. tab 은 어느 폭으로 그리든 1 byte 라 폭이 답에 닿지 않는다.

**검색 결과 판은 이번에 넘기지 않았다.** `:grep` 목록의 줄 내용은 파일에서 온 것이라 tab 이 들어올 수 있는데, 그 폭을 알려면 결과 파일마다 `.editorconfig` 를 읽고 담아 두는 겹이 새로 생긴다. 한 목록에 저장소 곳곳의 파일이 섞여 오는 자리다. 지금은 기본 폭으로 그리고 `docs/tasks.md` 에 남겼다.

## 되짚은 것

`indent.go` 의 `blankColumns` 가 「`.editorconfig` 의 `tab_width` 가 아니라 zn 이 실제로 그리는 폭을 쓴다 — 눈에 보이는 것과 움직이는 것이 어긋나면 안 된다」고 적어 두었다. 그 주석은 **둘이 갈릴 때의 규칙**이었고, 이 결정으로 둘이 같아져서 갈릴 자리 자체가 없어졌다. 규칙은 그대로 서 있다. 재는 자리가 화면인 것도 그대로다.
