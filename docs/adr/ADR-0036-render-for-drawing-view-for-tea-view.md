# ADR-0036: 화면을 그리는 함수는 `render` 로 시작하고, `tea.View` 를 만드는 것만 `View` 로 끝난다

- 상태: 채택
- 날짜: 2026-08-19

## 맥락

화면을 만드는 함수가 스무 곳 넘게 있는데 이름이 전부 **결과물의 명사**다 — `tabline`,
`statusBar`, `screenRows`, `lineNumber`, `jobBar`, `sidebar.cells`, `viewPalette.box`,
`viewJobs.title`. 갈래로는 잘 묶이지만 「그리는 일이 어디어디서 일어나는가」를 찾으려면
갈래를 먼저 알아야 한다. 이름만 보고는 값을 고르는 함수인지 글자를 만드는 함수인지도 모른다.

동시에 `view` 라는 낱말이 이미 깔려 있다. bubbletea 가 `View() tea.View` 를 요구하고
`tea.View` type 을 주기 때문이다. mode model type 이름(`viewEditorNormal`, `viewPalette`,
`viewJobs`), 파일 이름(`view-*.go`), 그리고 `screenView` 가 그 낱말을 쓴다.

그래서 두 낱말이 같은 일을 가리킨다. 새 함수를 만들 때마다 `render` 로 갈지 `view` 로 갈지
판단해야 했고, 판단할 근거가 없었다.

겹침이 드러난 계기는 `viewRows` 를 지운 일이다. 한 줄짜리 proxy 라 지웠는데, 지우고 나니
그 위의 `render` 가 무엇을 그리는지 이름이 말하지 않는 것이 보였다. `render` 는 편집 화면
전체이고 `:jobs` 는 그 길을 쓰지 않는데, 이름은 「그린다」고만 한다.

## 결정

**반환 타입으로 가른다.**

**`tea.View` 를 만들면 `View` 로 끝난다.** bubbletea 와 맞닿는 경계에만 붙는 이름이다.
셋뿐이다 — `View()`(인터페이스), `editorView`, `screenView`.

**화면에 나갈 글자를 만들면 `render` 로 시작한다.** `string`·`[]string` 을 만드는 것과,
글자를 담은 struct 를 만드는 것(`tabline` 의 `tablineRow`)이 여기다.

**값을 고르기만 하는 함수는 그리는 함수가 아니다.** `messageOr`(message 와 fallback 중
하나를 고른다), `viewPalette.labels`(목록 데이터), `activePath` 는 글자를 만들지 않는다.

**receiver 가 갈래를 말하면 이름에 대상을 또 넣지 않는다.** `viewPalette.renderBox`,
`treeRow.render` 다. receiver 가 `editor` 면 갈래 정보가 없으므로 대상을 붙인다 —
`renderTabline`, `renderStatusBar`, `renderScreen`.

**`render` 는 `editorView` 가 된다.** `tea.View` 를 만드니 경계 쪽이다.

**mode model type 이름과 `view-*.go` 파일 이름은 그대로다.** 그것은 함수가 아니라 화면
자체의 이름이고, 화면이 곧 model 인 구조(ADR-0002)에서 나온 이름이라 이 규칙 밖이다.

## 근거

**판단이 들지 않는다.** 반환 타입을 보면 정해진다. 규칙이 어휘 감각이 아니라 컴파일러가
아는 것에 걸려 있어서, 다음 사람이 새 함수를 만들 때 흔들릴 자리가 없다.

**`view` 경계가 좁아져서 또렷해진다.** 세 곳뿐이고 셋 다 bubbletea 와 맞닿는 자리다.
「어디까지가 bubbletea 이고 어디부터가 우리 것인가」가 이름으로 읽힌다.

**`render` 가 넓다는 문제가 저절로 풀린다.** 지금은 `render` 아래 `screenRows` 가 있어서,
`screenRows` 를 `renderScreen` 으로 바꾸면 부모와 이름이 겹쳐 보였다. `render` 가
`editorView` 가 되면 그 겹침이 사라진다.

**자동완성이 그리는 것 전부를 모은다.** 갈래로 묶는 이득은 method 라면 receiver 가 이미
준다 — `viewPalette.renderBox` 는 앞에 `viewPalette` 가 있어서 갈래를 잃지 않는다. free
function 과 `editor` method 만 대상을 이름에 붙이면 된다.

**이미 그쪽으로 가 있던 자리가 있다.** `renderParts`(render-row.go) 와 파일 이름
`render-row.go` 가 이 규칙을 따르고 있었다. 규칙이 없어서 한 곳에만 남아 있었을 뿐이다.

## 결과

이름을 옮긴 자리는 이렇다.

- `editor`: `render`→`editorView`, `screenRows`→`renderScreen`, `statusBar`→`renderStatusBar`,
  `tabline`→`renderTabline`, `lineNumber`→`renderLineNumber`, `jobBar`→`renderJobBar`,
  `jobText`→`renderJobText`, `position`→`renderPosition`, `withStatus`→`renderWithStatus`,
  `withShowcmd`→`renderWithShowcmd`
- `viewPalette`: `box`→`renderBox`, `inputRow`→`renderInputRow`, `listRows`→`renderListRows`,
  `counter`→`renderCounter`, `paletteRow.cell`→`paletteRow.render`
- `viewJobs`: `title`→`renderTitle`, `jobRow`→`renderJobRow`,
  `bareStatusBar`→`renderBareStatusBar`
- `sidebar`: `cells`→`renderCells`, `treeRow.cell`→`treeRow.render`
- free function: `drawBar`→`renderBar`, `highlightRow`→`renderRow`,
  `highlightMatches`→`renderMatches`
- `screenView` 와 `renderParts` 는 그대로다 — 이미 규칙에 맞다

전부 패키지 내부 이름이라 동작은 바뀌지 않는다. 테스트 helper(`textOf`, `barOf`,
`tablineOf`) 는 그리는 것이 아니라 그린 것을 읽는 것이므로 그대로 두었다.

주석은 그 함수를 가리키는 것만 옮겼다. `statusBar`·`tabline` 은 화면 요소의 이름이기도
해서 그 뜻으로 쓰인 주석 90 여 곳은 그대로다. 앞선 ADR 셋(0012·0027·0030)과 `tasks.md`
에서 코드 이름을 가리키던 자리도 새 이름으로 맞췄다.

## 대안

**전부 `view` 로 통일** — bubbletea 어휘를 그대로 넓히는 것이라 새로 들일 낱말이 없다.
고르지 않은 이유는 `tea.View` 와 글자 조각을 같은 낱말로 부르게 되어 지금의 겹침이 그대로
남기 때문이다. `screenView` 가 `tea.View` 를 만들고 `tablineView` 가 string 을 만들면,
이름만 보고는 무엇이 나오는지 여전히 모른다.

**지금처럼 결과물 명사를 유지** — 갈래로 묶이고 이름이 짧다. 고르지 않은 이유는 그 이득이
method 에서는 receiver 와 겹치고(`viewPalette.box` 의 `viewPalette`), free function 과
`editor` method 에서는 그리는 자리를 한눈에 모을 길이 없기 때문이다. 그리는 일이 스무 곳에
흩어져 있는데 그것을 한 낱말로 부르지 못하는 것이 값이다.

**`draw` 로 시작** — `drawBar` 가 이미 그 낱말이다. 고르지 않은 것은 터미널 UI 세계에서
`render` 가 더 흔한 말이고(bubbletea·lipgloss 문서가 그렇게 쓴다), 우리 파일 이름
`render-row.go` 도 이미 그쪽이기 때문이다. 옮기는 값이 `render` 쪽으로 기울어 있다.

## 이 ADR 이 정하지 않는 것

- **`editorView` 가 `screenView` 를 부르는 층 순서가 이름과 반대로 읽히는 것.** `screen`
  이 `editor` 보다 큰 말인데 바깥에 있는 쪽이 `editorView` 다. 이름을 바꿀지, 층을 바꿀지는
  옮기는 일을 끝내고 다시 본다

  이후: 층 순서가 아니라 `screenView` 가 층이 아니었던 것이 문제였다. `e` 를 한 번도 쓰지
  않고 `rows` 만 받아 터미널 설정을 붙이는 생성자인데, 이름이 `editorView` 옆에 서서 같은
  갈래의 두 층으로 읽혔다. free function `newView` 로 내렸다 — `editorView` 와
  `viewJobs.View` 가 각자 자기 화면을 만든 뒤 그것으로 감싸는 대등한 모양이 되어 크기
  관계를 물을 자리가 없어졌다
- 파일 이름을 규칙에 맞출지. `status-bar.go`·`tabline.go` 는 지금 갈래로 나뉘어 있다
- `expandRow`·`markWhitespace` 처럼 최종 글자가 아니라 그 앞 조각을 만드는 함수를 어느
  쪽으로 볼지. 지금은 손대지 않는다
