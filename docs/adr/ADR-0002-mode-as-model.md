# ADR-0002: mode 를 enum 이 아니라 화면 model 교체로 나타낸다

- 상태: 채택
- 날짜: 2026-08-11
- 이후: ADR-0026 이 "editor 를 값으로 embed 한다" 를 포인터로 고쳤다. mode 를 型 으로 나눈 결정은 그대로다

## 맥락

editor 는 normal / insert / visual mode 를 가진다(`docs/spec.md`). mode 는 같은 키를 다르게
해석하는 상태다. 이것을 코드로 어떻게 들고 있을지 정해야 한다.

처음에는 `mode` enum 필드를 `viewEditor` 에 두고 `Update` 에서 분기했다. 동작은 했지만
mode 가 늘어날 때마다 분기가 여러 곳으로 번지는 형태였다.

한편 이 저장소는 이미 화면을 model 교체로 다루고 있다. `QuitConfirm` 은 부모 model 을 들고
바뀌고, `internal/tui` 의 form 예제도 탭마다 model 이 다르다. bubbletea 의 `Update` 가
`tea.Model` 을 돌려주므로 型 을 갈아끼우는 것이 이 framework 의 기본 방식이다.

## 결정

mode 마다 별개의 model 型 을 둔다. mode 전환은 그 型 으로 갈아끼우는 것이다.

- `viewEditorNormal`, `viewEditorInsert` 가 각자 `Update` 와 `View` 를 가진다.
- mode 가 바뀌어도 유지되는 상태는 `editor` 구조체에 모으고 각 mode model 이 embed 한다.
  buffer 목록, 활성 buffer index, 화면 크기가 여기 있다.
- 전환 함수(`normalMode(editor)`, `insertMode(editor)`) 가 `editor` 를 그대로 넘긴다.
- 두 mode 가 같은 화면을 그리므로 `editor.render(shape)` 를 공유한다.
  mode 마다 다른 것은 커서 모양뿐이다(normal 은 블록, insert 는 막대).

## 근거

**키 해석이 型 마다 하나씩 모인다.** `if mode == insert` 분기가 없다. 어떤 키가 어떤 mode 에서
무엇을 하는지가 그 mode 파일 안에서 끝난다.

**잘못된 상태를 만들 수 없다.** enum 이면 "mode 는 insert 인데 normal 의 키 처리를 돌린다" 가
가능하지만, 型 이 곧 mode 이므로 그런 조합이 생기지 않는다.

**mode 별 상태를 그 mode 만 들고 있으면 된다.** 이 이유가 앞으로 가장 크게 작용한다.

- visual mode 는 선택 시작 지점(anchor) 이 필요하다
- operator-pending(`d`, `c` 를 누른 뒤) 은 어떤 operator 를 기다리는지가 필요하다
- 반복 횟수(`3j`) 를 받는 중이면 그 숫자가 필요하다

enum 방식이면 이 필드들이 전부 공용 구조체에 쌓이고 대부분의 시간에 의미가 없다.
型 마다 두면 그 mode 안에서만 존재한다.

**vim 자체가 mode 상태 기계다.** 型 으로 나누는 것이 원래 모양에 더 가깝다.

**저장소의 기존 방식과 같다.** 새 패턴을 들여오는 것이 아니라 이미 쓰는 것을 따른다.

## 결과

좋은 점

- mode 를 추가하는 것이 型 을 추가하는 것이다. 기존 mode 의 코드를 건드리지 않는다.
- mode 별 상태가 그 mode 안에 있다.
- 테스트에서 mode 단정이 型 단정이 된다(`assert.IsType(t, viewEditorInsert{}, m)`).

감수하는 것

- **공용 상태를 전환할 때마다 넘겨야 한다.** `normalMode(m.editor)` 처럼 `editor` 를 빼먹지 않고
  넘겨야 한다. 빼먹으면 화면 크기와 커서 위치가 초기화된다. embed 라 한 단어로 끝나지만
  enum 방식에는 없던 실수 지점이다.
- **`WindowSizeMsg` 처리가 mode 마다 반복된다.** `editor.resize` 로 내용은 모았지만
  `case tea.WindowSizeMsg:` 자체는 각 `Update` 에 있다. mode 가 늘면 이 껍데기가 늘어난다.
- **밖에서 "지금 어느 mode 냐" 를 묻기 어렵다.** enum 이면 필드를 읽으면 됐다.
  다만 statusBar 를 그리는 곳도 결국 각 mode 의 `View` 라서, 자기 이름을 문자열로
  `editor.render` 에 넘기는 것으로 해결됐다(`m.render(tea.CursorBlock, "NORMAL")`).
  mode 를 밖에서 물을 일이 애초에 없어서 interface 는 필요하지 않았다.
- **`ctrl+c` 처럼 모든 mode 가 같이 받는 키가 mode 마다 반복된다.** 지금은 두 곳이라 그대로 뒀다.
  늘어나면 공용 처리로 뺀다.

## 대안

**`mode` enum 필드 + `Update` 안에서 분기** — 처음 구현한 방식이다. 공용 상태를 넘길 필요가 없고
"지금 어느 mode 냐" 를 필드로 읽을 수 있다. 하지만 mode 별 상태가 공용 구조체에 쌓이고,
mode 가 늘 때 분기가 `Update` 와 `View` 양쪽에서 늘어난다.

**mode 를 interface 로 두고 `viewEditor` 가 들고 있기** — `viewEditor` 는 하나로 두고
키 처리만 갈아끼운다. 공용 상태를 넘길 필요가 없어진다. 다만 bubbletea 가 이미 `tea.Model` 로
같은 일을 하고 있어서 그 위에 한 겹을 더 얹는 것이 된다. interface 는 반드시 필요하기 전에는
도입하지 않는다는 원칙에도 어긋난다.

## 이 ADR 이 정하지 않는 것

- visual mode 를 몇 종류(char/line/block) 로 둘지
- operator-pending 을 별개 model 로 둘지, normal mode 안에서 처리할지
