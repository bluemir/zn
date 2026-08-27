# ADR-0026 editor 를 포인터로 든다

## Status

Accepted

ADR-0002 의 "mode 별 model 이 editor 를 값으로 embed 한다" 를 고친다. mode 를 type 으로 나눈 결정 자체는 그대로다. ADR-0025 의 "값으로 복사되는 editor 를 그대로 둔다" 도 이 결정으로 뒤집힌다.

## Context

ADR-0002 는 mode 를 model type 교체로 나타내기로 하면서 `editor` 를 값으로 embed 했다. 값이냐 포인터냐는 그 ADR 이 따로 다루지 않았고, bubbletea 의 model 이 값이라는 관례를 따른 결과였다.

그런데 실제로는 처음부터 반은 공유였다.

- `editor.buffer()` 가 `&e.buffers[e.active]` 를 돌려준다. 커서 이동과 편집은 복사본을 넘어 남는다.
- 그 공유 때문에 `closeTab`·`newTab`·`replaceTab` 은 slice 를 제자리에서 고치지 못하고 새로 할당해야 했다. "아직 살아 있는 다른 복사본이 어긋난 내용을 보게 된다" 는 방어다.
- `sidebar.tree` 도 포인터라 복사본 사이에서 공유된다.

값 복사가 실제로 무언가를 되돌려 주지도 않았다. 검색의 `Esc` 는 값 복사가 아니라 `searchOrigin` 을 따로 들고 `restore()` 로 되돌리고, 확인창은 `editor` 를 아예 들고 있지 않았다.

비동기 작업(ADR-0025)에서 이것이 처음으로 대가를 치렀다. 나중에 도착하는 msg 는 그 사이에 다른 복사본으로 넘어간 화면에 닿을 수 있어서, 진행 상태가 어긋나도 저절로 맞도록 두 가지를 지켜야 했다. msg 가 다음 채널을 들고 다니는 것과, 조각이 누적분 전체를 갈아끼우는 것이다. 소비자가 늘 때마다 같은 것을 매번 지켜야 한다.

## Decision

`editor` 를 mode model 이 **포인터로 embed** 한다. 편집기가 도는 동안 `editor` 는 하나뿐이다.

- 전환 함수의 서명이 `normalMode(e *editor)` 로 바뀐다. `insertMode`·`commandMode`·`searchMode`· `sidebarMode`·`paletteMode`·`quitAll`·`closeTab`·`paletteCommand.run` 도 같다.
- `core.Run` 이 `&editor{…}` 하나를 만들어 첫 model 에 넘긴다.
- 값 수신자(`func (e editor) textWidth()`) 는 그대로 둔다. 포인터를 embed 해도 그대로 불리고, 값 수신자라는 것이 곧 "이 메서드는 상태를 고치지 않는다" 는 표시가 된다. 상태를 고치거나 editor 를 다음 화면으로 넘기는 것(`jumpToMatch`, `clickSidebar` 등) 만 포인터 수신자로 바꿨다.
- 값 복사 때문에 있던 방어를 걷어낸다. `closeTab`·`newTab`·`openTab`·`replaceTab` 은 `slices.Insert`/`slices.Delete` 로 제자리에서 고치고, `putJob`·`removeJob` 도 마찬가지다.
- 확인창(`viewConfirmDiscard`) 도 `*editor` 를 든다. 자기 화면을 그리므로 statusBar 는 보이지 않지만, 다른 mode 와 같은 `case` 로 백그라운드 작업의 진행을 받는다. 창이 떠 있는 동안 온 진행이 버려지지 않는다.

비동기의 두 성질(msg 가 채널을 들고 다니는 것, 조각이 누적분 전체를 보내는 것) 은 남긴다. 이제 안전을 위해 필요하지는 않지만, 채널을 들고 다니면 목록을 뒤지지 않아도 되고 조각이 누적분 전체이면 다시 부어도 같은 결과가 된다. 정렬을 끝낸 배열의 앞부분을 가리키는 것이라 복사도 없다.

## Consequences

좋은 점

- **전환할 때 editor 를 빼먹는 실수 지점이 사라졌다.** ADR-0002 가 감수 사항 첫 줄에 적어둔 것이다.
- **"복사본이 어긋난다" 는 걱정이 없다.** 나중에 도착하는 것(작업 진행, 앞으로 올 타이머·watcher) 이 언제 닿아도 지금 상태에 닿는다. 비동기 소비자가 늘 때마다 방어를 되풀이하지 않아도 된다.
- **slice 를 제자리에서 고친다.** `closeTab` 이 세 줄에서 한 줄이 됐고, 그 이유를 설명하던 주석도 같이 사라졌다.
- 값과 참조가 섞여 있던 것이 한쪽으로 정리됐다. `buffer()` 가 포인터를 주는 이유도 이제 "복사를 넘어 남기려고" 가 아니라 "제자리에서 고치려고" 하나다.

감수하는 것

- **옛 model 로 돌아가도 상태는 돌아오지 않는다.** 확인창에서 No 를 눌러 부모로 돌아가도 그 사이의 변경은 남는다. 지금 코드에는 되돌림을 값 복사에 기대는 곳이 없지만(검색은 `searchOrigin` 으로 직접 되돌린다), 앞으로 그런 것을 만들려면 검색처럼 직접 들고 있어야 한다.
- **테스트에서 model 을 복사해 서로 다른 상태를 만들 수 없다.** `narrow := wide; narrow.width = 40` 같은 것이 이제 같은 editor 를 고친다. 폭마다 새로 열어야 한다.
- **bubbletea 의 관례에서 한 걸음 벗어난다.** model 은 값이라는 전제 위에 있는 framework 다. 다만 `tea.Model` 은 interface 라 포인터를 담아도 되고, 이 저장소는 애초에 buffer 를 공유하고 있었다.

## Alternatives

- **값 embed 를 유지하고 비동기 쪽에서 매번 방어**
	- ADR-0025 가 고른 길이다. 그 하나로는 성립했다.
	- 소비자가 늘 때마다 "누적분 전체를 보낸다", "msg 가 채널을 들고 다닌다" 를 다시 지켜야 한다.
- **editor 안의 일부만 포인터로**
	- 진행 상태처럼 나중에 닿는 것만 포인터 필드로 두는 절충이다.
	- 어느 필드가 공유되고 어느 것이 복사되는지가 필드마다 갈려서, 지금도 헷갈리던 것이 더 헷갈린다.
- **mode 를 interface 로 두고 editor 가 들고 있기**
	- ADR-0002 가 이미 거절했다.
	- bubbletea 가 `tea.Model` 로 하는 일 위에 한 겹을 더 얹는 것이다.
