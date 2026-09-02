# ADR-0124: 떼어 낸 글은 register 가 아니다

## 맥락

「`Buffer` 와 `viewport` 를 같이 새 패키지로 낼 수 있나」를 재다가 나온 자리다.

그 둘에서 전이적으로 닫으면 **113 개 이름 / 60 파일**이고 `editor` 에는 닿지 않는다. 커서가 걸려 ADR-0117 이 물러섰던 매듭은 ADR-0121 이 이미 풀었다. 그런데 따라 나오는 것 중에 `register` 가 있었다.

ADR-0122 가 그것을 「**뜻을 아는 물건.** 행동이 붙어 있다」로 갈라 editor 쪽에 남기기로 한 것인데, 정작 글을 다루는 메서드 열하나가 그것을 주고받고 있었다.

| | 자리 |
|---|---|
| `register` 를 돌려주는 메서드 | 7 (`yankRange`·`yankLines`·`deleteRange`·`deleteText`·`deleteLines`·`changeRange`·`changeLines`) |
| `register` 를 받는 메서드 | 4 (`pasteAfter`·`pasteBefore`·`pasteLines`·`pasteText`) |

## 1. 행동이 어느 겹의 것인지까지 봐야 했다

ADR-0122 의 잣대는 「메서드가 붙어 있나」였다. 그 잣대로는 `register` 가 걸린다 — `filled`·`copiedMessage`·`deletedMessage`·`charCount` 넷이 있다.

**그런데 그 넷이 무엇을 하는지 세어 보니 전부 「무엇을 알릴까·몇 자인가」였고, 부르는 자리 여섯이 전부 editor 쪽이었다**(`action.go` 2 · `view-editor-command.go` 2 · `view-registers.go` 1 · 시험 1).

받는 쪽도 같았다. 붙여넣기 넷이 쓰는 것은 `reg.lines` 와 `reg.linewise` 뿐이다. **붙여넣는 데 필요한 것은 자리와 글과 줄 단위인지이고, 그것이 어느 register 에서 왔는지는 부르는 쪽의 일이다.**

「메서드가 붙어 있다」가 아니라 **「그 메서드가 어느 겹의 말인가」**로 봐야 했다.

## 2. 자료에 이름을 주었다

```go
// 글에서 떼어 낸 한 덩이. 지우기·복사가 내놓고 붙여넣기가 받는다
type textBlock struct {
	lines    [][]byte
	linewise bool
}

// 한 칸에 담긴 것. 자료에 editor 의 말을 얹은 것이다
type register struct {
	textBlock
}
```

**`linewise` 를 같이 드는 것이 이 type 이 있어야 하는 까닭이다.** 떼어 낸 뒤에는 줄들만 보아서는 줄 단위였는지 알 수 없다 — 한 줄을 통째로 복사한 것과 그 줄의 글자를 처음부터 끝까지 고른 것이 같은 모양이다. 그래서 값 둘을 따로 돌려주는 길(`(lines, linewise, ok)`) 대신 묶었다.

embed 라 `reg.lines`·`reg.filled()` 가 그대로 서고, 밖에서 고친 자리가 적다.

## 3. 감싸고 벗기는 것은 editor 가 한다

담는 문 셋(`storeYank`·`storeDelete`·`storeNamed`) 이 `textBlock` 을 받아 **안에서** `register` 로 감싼다. 「어느 칸에 담는가」를 아는 것이 `registerSet` 이므로 감싸는 것도 거기가 맞다.

꺼낼 때는 반대다. `pasteAfter(e.registers.byName(c.reg).textBlock, ...)` 로 자료만 넘긴다.

## 규모

| 바꾼 것 | 자리 |
|---|---|
| 서명이 `textBlock` 이 된 메서드 | 11 |
| `register{...}` → `textBlock{...}` | 9 |
| editor 가 감싸는 자리 | 3 (담는 문 안) |
| editor 가 벗기는 자리 | 2 (붙여넣기) |
| 말을 부르며 감싸는 자리 | 4 |

동작은 하나도 안 바뀐다.

## 얻은 것

`register.go` 가 옮길 무리에서 빠졌다. 이름 수는 113 그대로다 — `register` 가 나가고 `textBlock` 이 들어왔다. **줄어든 것은 수가 아니라 무리가 진 뜻이다.** 글을 다루는 겹이 「무명 register 가 무엇인지」·「`"0` 과 `"1` 이 어떻게 갈리는지」·「몇 자 복사되었다고 말할지」를 더는 알지 않는다.

## 남는 것

무리에 아직 남아 있고 갈래가 다른 것들이다. 각각 따로 본다.

- **화면 배치 상수.** `markerWidth`·`minAbsoluteDigits`·`minRelativeDigits`·`digits` 는 재 보니 밖에서 안 쓴다 — `editor.lineNumberDigits()` 의 「tab 없을 때」 갈래가 유일한 밖 사용인데 **그 갈래는 닿지 않는다**(`renderGutter` 가 `gutterWidth()==0` 이면 먼저 나가고, tab 이 없으면 0 이다). `minTextWidth` 하나만 sidebar 와 같이 쓴다
- **언어를 아는 자리.** `internal/syntax` 가 이미 `Language.Indent`·`Outline`·`State` 셋을 낸다. core 에 남은 것은 그 위의 얇은 층인데, `table.go`(markdown 표)·`semantic.go`·`sticky.go` 가 어디로 갈지가 아직 안 정해졌다
- **`textBlock` 을 `scheme` 으로 올릴지.** 이제 메서드가 `filled` 하나이고 겹을 오간다. ADR-0122 의 잣대에 걸리는지 다시 볼 자리다. 가르고 보니 `textarea.TextBlock` 으로 나갔다 (ADR-0128)

## 되짚은 것

**ADR-0122 의 표 한 줄이 틀렸다.**

> 뜻을 아는 물건 | 행동이 붙어 있다 | `register`(`charCount`·`copiedMessage`)

행동이 붙어 있는 것은 맞았는데, **그 행동이 그 자료의 것이 아니라 editor 가 그 자료를 두고 하는 말이었다.** 「메서드가 있다」는 세기 쉬워서 잣대로 쓰기 좋지만, 그것만으로는 자료와 말이 한 type 에 붙어 있는 경우를 못 가른다.

같은 눈으로 보면 `Buffer` 의 `Save`·`Reload` 도 다시 볼 자리다. 그쪽은 「글이 스스로 하는 일」이라 갈래가 다르지만, 판단의 근거는 같아야 한다.
