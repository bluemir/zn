# ADR-0122: 좌표를 type 으로 가른다. `col` 이 세 단위였다

## 맥락

ADR-0121 로 커서가 viewport 로 나온 뒤 「(line, col) 쌍이 여기저기 흩어져 있으니 struct 로 묶을까」가 나왔다. 처음에는 **글이 짧아지느냐**로 재려 했다 — 쌍으로 오는 자리가 96 곳이고 한쪽만 쓰는 자리가 620 곳이니 값이 안 맞는다고 보았다.

**그 잣대가 틀렸다.** 중요한 것은 길이가 아니라 **의미가 드러나느냐**이고, 특히 논리 줄과 화면 행은 헷갈리는 개념이라 이름이 그것을 갈라야 한다.

그 잣대로 다시 보니 **같은 `col` 이 세 단위로 쓰이고 있었다.**

| 이름 | 단위 |
|---|---|
| `cursorCol`·`MotionRange` 의 col·`selection.col` | **줄 안 byte offset** |
| `desiredCol` | **화면 행 안에서 센 칸** (ADR-0108) |
| `screenColAt` 이 주는 값 | **줄 시작에서 센 화면 칸** |

셋 다 `int` 라 **바꿔 넣어도 컴파일이 되었다.** 줄 쪽도 같았다 — `cursorLine`·`top` 은 논리 줄이고 `topRow`·`rowIndexAt` 은 그 줄 안의 wrap 행인데 둘 다 `int` 다.

`buffer-move.go` 머리글이 「이름의 Line·Row 가 이 둘이다. 저장소 전체가 이 두 낱말만 쓴다」고 규칙을 적어 두었는데, **그 규칙을 지키는 것이 사람뿐**이었다.

## 1. 세 좌표계에 type 을 주었다

```go
// 파일 안의 자리. col 은 byte offset 이다
type Cursor struct{ line, col int }

// 화면에 그려진 자리. 터미널 셀 격자의 칸이다
type Cell struct{ x, y int }

// 화면 맨 위에 그릴 자리
type viewTop struct{ line, row int }
```

`Cursor.col` 과 `Cell.x` 는 이제 서로 대입되지 않는다. **컴파일러가 단위를 지킨다.**

화면 쪽을 `col` 이 아니라 `x, y` 로 둔 것은, `col` 을 쓰면 byte 쪽과 같은 낱말이 되어 다시 헷갈리기 때문이다. 받는 쪽(`tea.NewCursor`) 의 어휘이기도 하다. 그래픽 좌표처럼 보이지만 **터미널에서는 그 x, y 가 곧 칸**이라 뜻이 어긋나지 않는다.

## 2. `viewTop` 이 왜 (논리 줄, 그 안의 행) 인가

「파일 앞에서부터 몇 번째 화면 행」으로 담으면 **창 폭이 한 칸만 바뀌어도 그 값이 통째로 틀리고**, 다시 세려면 파일 앞부분을 처음부터 훑어야 한다.

**논리 줄은 폭과 무관하다.** 그래서 그것을 닻으로 삼고 그 안에서만 행을 센다 — 폭이 바뀌면 `row` 만 다시 재면 되고 `line` 은 그대로다. `rowIndexAt` 이 「그 줄 안의 몇 번째 행」을 주는 것도 같은 까닭이다.

이 까닭이 전에는 어디에도 안 적혀 있었다. 필드 둘이 나란히 있을 뿐이었다.

## 3. `desiredCol` 은 `desiredX` 가 되었다

「화면 행 안에서 센 칸」이라 단위가 `Cell.x` 와 같다. 이름에 `col` 이 있으면 옆의 `Cursor.col`(byte) 과 같은 것으로 읽힌다.

행이 없는 홀몸이라 type 을 주지 않았다. **다음에 위아래로 갈 때 서고 싶은 x** 하나다.

## 4. `MotionRange` 가 Cursor 쌍이 되었다

```go
type MotionRange struct {
	start, end Cursor
	linewise   bool
}
```

좌표 넷이 **전부 쌍으로만 쓰였다.** `textBetween(startLine, startCol, endLine, endCol)` 같은 자리가 그 증거이고, 그것도 `textBetween(start, end)` 가 되었다. `deleteText` 도 같다.

ADR-0100 이 `target` 을 걷어내 「좌표 다섯」으로 줄인 위에 얹는 정리다.

## 규모

| 바꾼 것 | 자리 |
|---|---|
| `cursorLine`·`cursorCol` → `cursor.line`·`cursor.col` | 720 |
| `top`·`topRow` → `top.line`·`top.row` | 244 |
| `motionRange` 의 좌표 넷 → `start`·`end Cursor` | 87 |
| `desiredCol` → `desiredX` | 40 남짓 |

전부 기계적이고 동작은 하나도 안 바뀐다.

## 5. 셋만 대문자다

`Cursor`·`Cell`·`MotionRange` 만 대문자이고 `viewTop`·`selection`·`edit` 은 소문자다. 대문자를 **「나중에 패키지를 가를 때 그 경계를 넘을 것」** 이라는 표시로 쓰기로 했다.

무엇이 넘을지를 처음에는 「지금 몇 겹이 부르나」로 재려 했다. 그 잣대로는 `viewport`(5 겹)·`editor`(3 겹, 613 회) 가 위에 오는데, 그것들은 경계가 아니라 중심이다.

**잣대는 「이 말이 어디까지 통용되나」였다.**

| | 무엇 | 예 |
|---|---|---|
| 오가는 자료 | 값만 들고 아무 일도 안 한다 | `Cursor`·`Cell`·`MotionRange` |
| 한 겹의 상태 | 메서드가 없어도 오가지 않는다 | `viewTop`·`selection`·`edit` |
| 뜻을 아는 물건 | 행동이 붙어 있다 | `register`(`charCount`·`copiedMessage`) |

`Cell` 은 지금 `cursorScreenPos` 한 자리에서만 쓰이는데도 여기 든다. **화면 칸이라는 개념 자체가 공용**이라서다. 반대로 `viewTop` 은 여러 곳에서 읽히지만 「이 창이 어디부터 그리나」라 창 밖에서는 할 말이 없다.

**이 잣대는 경계가 정해지기 전에도 쓸 수 있다.** 어디로 가르든 오가는 자료는 그 선을 넘고 상태는 자기 겹에 남는다.

`Buffer` 가 대문자인 것은 이 잣대와 별개다. ADR-0100 §8 이 「헛것이지만 두기로 한다」고 적어 둔 자리다.

## 6. `internal/scheme` 으로 냈다

셋을 `internal/scheme` 으로 옮겼다. **DTO 라서가 아니라 이 편집기 전체에서 통용되는 개념이라서**다. core 를 앞으로 어떻게 가르든 이 셋은 그 선을 넘나든다.

필드도 대문자가 되었다(`Cursor.Line`·`Cell.X`·`MotionRange.Start`). 패키지가 갈리면 소문자 필드는 밖에서 못 읽는다 — 그래서 이 이동은 **type 이름만 올리는 것으로 끝나지 않고** 필드 접근 자리를 전부 지난다.

**동작을 두지 않는 것이 이 패키지의 규칙이다.** 메서드가 붙기 시작하면 그것은 이미 「뜻을 아는 물건」이고, 그 뜻은 어느 한 겹의 것이라 이 패키지가 그 겹을 알게 된다. `internal/lines` 가 정확히 그 길로 갔다가 물렸다 — 나가는 의존이 0 인 잎이었지만 tab 폭과 제어문자 폭이라는 **편집기의 결정**을 몸에 지니고 있었다(ADR-0119).

여기 셋은 그 함정에 안 걸린다. 메서드가 하나도 없고, 담는 것이 좌표와 「줄 단위인가」 하나뿐이다.

## 남는 것

- **`rowHighlight.cursorCol`** 은 그대로다. 다른 type 의 필드이고 byte offset 인데, 그 type 이 렌더 쪽이라 이번에 안 건드렸다
- **`inputLine.visible` 이 돌려주는 `cursorCol`** 도 남았다. 입력줄은 파일이 아니라 한 줄짜리 글이라 좌표계가 또 다르다
- **`Cell` 을 쓰는 자리가 아직 `cursorScreenPos` 하나뿐**이다. 화면 좌표를 다루는 자리가 늘면 그때 더 쓰인다

## 되짚은 것

**「글이 짧아지나」로 재려 했던 것이 잘못이었다.** 그 잣대로는 620 대 96 이라 안 하는 쪽이 답이었다. 그런데 묶는 값은 짧아지는 것이 아니라 **틀린 대입이 컴파일에서 막히는 것**이었다.

같은 낱말이 여러 단위로 쓰이는 것을 주석으로 막아 두었는데(`desiredCol` 의 「화면 행 안에서 센 칸이다」, buffer-move.go 의 「이름의 Line·Row 가 이 둘이다」), **주석은 지켜지는지 검사되지 않는다.** type 은 검사된다.
