# ADR-0100 `Buffer` 는 범위만 받는다. motion 을 범위로 옮기는 일은 부르는 쪽이 한다

- 상태: 채택
- 날짜: 2026-08-28
- 관련: ADR-0013(operator 와 motion), ADR-0017(`d`·`y` 가 범위를 같이 쓴다), ADR-0033(`c` 의 되돌리기 구간), ADR-0037(visual 이 고른 범위), ADR-0002(mode 를 model 로 나눈 것)

## 맥락

`docs/tasks.md` 의 v1.1 에 오래 남아 있던 항목이다.

> `Buffer` 가 주는 기능을 훑어 쓸데없는 것을 걷어낸다
> - 죽은 메서드는 없다(전부 최소 한 곳에서 부른다). 훑을 대상은 **부르는 자리가 하나뿐인 것**이고, 비슷한 일을 다른 이름으로 부르는 것이 그 안에 있는지 본다

훑어 보니 이렇다.

| 본 것 | 결과 |
|---|---|
| 메서드 수 | 121 |
| 호출하는 자리가 없는 것(죽은 메서드) | 0 |
| `buf` 를 한 번도 안 쓰는 것(자유 함수여야 할 것) | 0 |
| 호출 자리가 하나뿐인 것 | 46 |

**호출 자리가 하나뿐인 것 대부분은 낭비가 아니다.** 다만 까닭이 「vim 키와 1:1 이라」가 아니다 — 그것은 결과이지 근거가 못 된다. 근거는 §3 의 읽기/쓰기 가름이다.

**그런데 셋이 달랐다.** `deleteByMotion`·`yankByMotion`·`changeByMotion` 은 글자까지 같은 여섯 줄이고 부르는 것만 다르다.

```go
func (buf *Buffer) yankByMotion(m motion, count, width int) (register, bool) {
	area, ok := m.span(*buf, count, width)
	if !ok {
		return register{}, false
	}

	return buf.yankRange(area, width)
}
```

**하는 일이 옮기는 것뿐이다.** motion 을 범위로 바꾸고 그 범위를 받는 메서드에 넘긴다.

## 1. 번역이 두 자리에 있었다

이 셋만 있었으면 「짧은 편의 메서드 셋」으로 볼 수도 있다. 재 보니 그게 아니었다.

**같은 번역을 `action.go` 도 하고 있었다.** `>`·`<`(`actionIndent`) 와 `=`(`actionReindent`) 는 부르는 자리에서 `motion.span` 을 부르고 그 범위를 `shiftLines`·`reindentLines` 에 넘긴다. **visual 도 그렇다** — `selectionRange()` 로 범위를 잡아 `deleteRange`·`yankRange`·`changeRange` 에 넘긴다.

그래서 규칙이 두 벌이었다.

| operator | 범위를 어디서 잡나 |
|---|---|
| `>` `<` `=` | `action.go` |
| visual 의 `d` `y` `c` | `action.go` |
| normal 의 `d` `y` `c` | **`Buffer` 안** |

`d` 하나가 어느 문으로 들어왔느냐에 따라 번역이 다른 겹에서 돌았다. 같은 `deleteRange` 로 모이는데 그 앞이 갈렸다.

## 2. `Buffer` 는 범위만 받는다

셋을 걷어내고 `action.go` 가 visual 과 같은 모양으로 부르게 했다.

```go
buf := e.activeBuffer()

if area, ok := c.motion.span(*buf, c.count, e.contentWidth()); ok {
	if deleted, cut := buf.deleteRange(area, e.contentWidth()); cut {
		e.registers.storeDelete(deleted, c.reg)
	}
}
```

`actionVisualDelete` 와 줄 하나까지 같은 모양이고, `selectionRange()` 자리에 `motion.span()` 이 들어간 것만 다르다. **범위를 얻는 길만 둘이고 그다음은 하나다.**

**이제 `Buffer` 의 어떤 메서드도 `motion` 을 받지 않는다.** 받는 것은 `motionRange` 뿐이다. `motion.span` 을 부르는 자리는 `action.go` 다섯 곳으로 모였다.

가르는 자리가 이름에 드러난다. `motion` 은 「키가 무엇을 가리키나」이고 `motionRange` 는 「어디부터 어디까지인가」다. 앞엣것은 키를 아는 겹의 말이고 뒤엣것은 글을 아는 겹의 말이라, `Buffer` 가 앞엣것을 알 까닭이 없다.

**`*ByMotion` 의 주석에 있던 뜻은 버리지 않았다.** 「`d` 뒤에 손이 미끄러진 키가 dirty 를 세우면 안 된다」는 그 판단이 실제로 서는 `action.go` 로, 「`cc` 는 줄을 없애지 않는다」는 그 일이 도는 `changeRange` 로 옮겼다.

## 3. 가르는 자리는 읽기와 쓰기다

`Buffer` 가 어떤 메서드를 내놓아야 하는지를 「vim 키와 1:1 인가」로 물으면 답이 안 나온다. 그 잣대라면 `Buffer` 의 면이 vim 의 키 목록을 따라 생긴 것이 되고, 그것은 §2 가 `motion` 을 거절한 까닭과 같은 결합이다.

**재 보니 실제로 서 있는 규칙은 다른 것이었다.**

| 겹 | 하는 일 | buffer 상태를 |
|---|---|---|
| `motion` | 키가 몇 줄·몇 단어인지 정하고, 잡을 것이 있는지 보고, 범위를 만든다 | **읽기만 한다** |
| `Buffer` | 커서를 옮기고 그때 지켜야 할 불변을 지킨다 | **쓴다** |

`motion.go` 는 `buf.cursorLine`·`buf.lines` 를 스물한 곳에서 읽지만 **커서 상태를 쓰는 자리는 한 곳도 없다.** 쓰는 것은 전부 `Buffer` 의 메서드를 거친다.

그 가름이 지키는 것이 `desiredCol` 이다. **그런데 「옮기면 다시 맞춘다」 하나가 아니라 규칙이 둘이다.**

- 대개는 **새로 정한다**(`updateDesiredCol`). 37 자리다
- `j`·`k` 와 `gj`·`gk` 는 반대로 **기억한 것을 읽어 쓴다**(`placeCursorInLine`·`placeCursorInRow`). 2 자리다

둘이 갈리는 것이 vim 의 손 그 자체다. 긴 줄에서 `j` 로 짧은 줄을 지나 다시 `j` 를 치면 원래 칸으로 돌아와야 하는데, 그건 `j` 가 `desiredCol` 을 **안 건드릴 때만** 된다. `G` 는 반대로 새로 정해야 한다.

**그래서 이 불변은 「부르면 되는 것」이 아니라 「이 이동이 어느 쪽인지 아는 것」이다.** 그 앎이 `buffer-*.go` 안에만 있다. `motionLineStart.move` 가 `buf.cursorCol = 0` 을 직접 하면 그 겹이 「여기는 새로 정하는 쪽인가 읽는 쪽인가」를 같이 져야 하고, 틀리면 `0` 을 친 뒤 `j` 가 엉뚱한 칸으로 간다.

**그래서 `moveDownLine` 같은 한 줄짜리도 자리가 맞다.** `motionLineDown` 은 「`j` 는 count 만큼이고 마지막 줄에서는 잡을 것이 없다」는 **정책**이고, `moveDownLine` 은 「n 줄 아래로, 파일 끝에서 멈추고, 기억한 칸을 쓴다」는 **기계**다. 얇은 것은 vim 의 이동 키가 마침 커서 원시 조작과 겹치기 때문이지 두 번 쓴 것이 아니다.

같은 까닭으로 `placeCursorInLine` 과 `moveToLine` 은 이름이 닮았지만 합칠 수 없다. 앞엣것은 `desiredCol` 을 **읽고** 뒤엣것은 **쓴다**. 그 하나가 `j` 와 `G` 를 가른다.

**이 잣대가 `*ByMotion` 셋도 제대로 설명한다.** 그 셋의 죄는 「짧다」가 아니라 **읽고 정하는 쪽이 `Buffer` 안에 있었다**는 것이다. `Buffer` 가 `motion.span` 을 불렀고, 그것이 의존이 거꾸로 가던 유일한 자리였다. 지금은 `buffer-*.go` 가 `motionRange`(자료) 를 아홉 곳에서 알고 `motion`(정하는 것) 은 한 곳도 모른다.

셋을 뺀 뒤 메서드는 118 개이고 호출 자리가 하나뿐인 것은 43 개다. 그중 둘이 §4 에서 더 걸렸고, 나머지는 위 까닭으로 그대로 둔다.

이름이 닮은 것들도 겹이 아니다.

- `moveWordForward`/`wordForward` 는 「count 만큼 돌고 desiredCol 을 맞추는 겹」과 「한 걸음」이고, 아래 겹을 `dw` 가 따로 부른다
- `Save`/`SaveForce`/`SaveTo`/`SaveToForce` 는 「제 파일/다른 파일 × 강제」 2×2 이고 각 쌍이 사적인 알맹이를 나눠 쓴다(ADR-0024)

`buffer-*.go` 안에서 세 줄 이상 똑같이 되풀이되는 자리는 열한 곳인데 대부분 우연한 가드(`if buf.cursorCol >= len(line) { return }`) 다. 뜻이 있는 것은 「줄 하나를 한 번의 되돌리기로 갈아끼우는」 손인데, 쓰는 여덟 자리가 그 뒤로 저마다 다른 일을 해서 묶을 자리가 나오지 않는다. 「단순 중복이 있다고 helper 를 만들지 않는다」(CLAUDE.md) 에 걸리는 자리이기도 하다.

## 4. operator 이름이 붙은 메서드도 올렸다

§3 의 잣대로 다시 훑으니 둘이 더 걸렸다. `wordEndToChange` 와 `wordForwardToDelete` 다.

**이름이 이미 증거다.** `ToChange`·`ToDelete` 는 「어느 operator 를 위한 것인가」이고 그것은 키 겹의 말이다. `Buffer` 가 `c` 와 `d` 를 알고 있었다.

`cw` 쪽은 더 나빴다. **정책이 두 겹에 쪼개져 있었다** — `motionChangeWord.span` 이 예외 둘(빈 줄, 공백 위) 을 들고 있고 셋째(「첫 걸음만 지금 단어 끝에서 멈춘다」) 만 `Buffer` 로 내려가 있었다. 같은 규칙이 집을 둘 가진 것이라 `*ByMotion` 과 같은 결이다.

옮기는 값이 쌌다. `wordEndToChange` 의 몸통은 **이미 있는 원시 조작 넷의 조립**뿐이라(`atWordEnd`·`wordEnd`·`moveWordEnd`·`includeCursorCluster`) 그대로 `span` 안으로 올라갔다. 새로 만지는 필드가 없다.

`wordForwardToDelete` 는 커서를 직접 썼다(`buf.cursorLine, buf.cursorCol = line+1, 0`). 그대로 올리면 `motion.go` 가 커서를 쓰게 되어 §3 이 깨진다. **`moveTo(line, col, width)` 로 바꿔 올렸다** — 자르고 `updateDesiredCol` 까지 하는 원시 조작이고, 사본 위에서 도는 자리라 `desiredCol` 이 더 맞춰지는 것은 해가 없다.

그래서 지금은 이렇다.

| | 옮기기 전 | 지금 |
|---|---|---|
| `cw` 예외가 사는 곳 | motion.go 둘 + `Buffer` 하나 | **motion.go 셋** |
| `dw` 예외가 사는 곳 | `Buffer` 둘 | **motion.go 둘** |
| operator 이름이 붙은 `Buffer` 메서드 | 둘 | **0** |
| `motion.go` 가 커서를 직접 쓰는 자리 | 0 | **0** (그대로) |

`atWordEnd`·`wordEnd`·`moveWordEnd`·`includeCursorCluster`·`wordForward`·`moveWordForward`·`moveTo` 는 남는다. 전부 기계이고, 이제 `motion.go` 가 그것들을 조립해 정책을 만든다.

메서드는 116 개가 되었다.

## 5. 「복사하고, 커서를 옮긴다」를 부르는 쪽이 그 차례로 한다

`yankRange` 가 복사하고 나서 커서까지 옮기고 있었다. **복사는 커서를 옮기는 일이 아니다.** vim 의 `y` 가 커서를 옮기는 것은 맞지만(`yb` 는 앞으로, `yw` 는 제자리) 그것은 복사의 성질이 아니라 **키의 규칙**이고, §3 의 잣대로는 위 겹의 말이다.

**이 저장소에 이미 그 모양이 있었다.** `actionVisualCat`(`\c`) 은 「범위를 잡고 → 커서를 옮기고 → 내보낸다」를 `action.go` 에서 차례로 한다. `moveToRangeStart` 를 부르는 다섯 자리 중 그 하나만 밖이었고, `yankRange` 와 `changeCaseRange` 가 안으로 삼키고 있었다.

둘을 밖으로 냈다.

```go
if yanked, copied := buf.yankRange(area); copied {
	e.registers.storeYank(yanked, c.reg)
	e.notify(yanked.copiedMessage())
	buf.moveToRangeStart(area, e.contentWidth())
}
```

**`yankRange` 는 아무것도 건드리지 않는 함수가 되었다.** 값 receiver 로 바뀐 것이 그 증거이고, `width` 인자도 사라졌다 — 그것이 오직 `moveToRangeStart` 때문에 있었다.

`moveToRangeStart` 는 `Buffer` 메서드로 남는다. 커서를 **쓰는** 일이라 §3 대로 그쪽이 맞다. 바뀐 것은 **언제 부를지를 누가 정하느냐**다.

**밖으로 내니 묻혀 있던 차이가 드러났다.** `y` 는 복사한 것이 없으면 옮기지 않고, `~` 는 바뀐 것이 없어도 옮긴다. 전에는 각 메서드 안에 흩어져 있어 견줄 수 없었는데 이제 부르는 자리에서 나란히 보인다.

## 6. `motionRange` 는 좌표 다섯이 되었다

§5 로 커서 옮기기가 밖으로 나오자 다음 물음이 왔다. **`targetLine`·`targetCol` 도 밖에서 만들 수 있지 않나.**

그 둘은 「범위를 잡은 손이 커서를 둔 자리」였고 범위의 성질이 아니었다. 재 보니 **다섯 생산자 전부에서 그 값이 범위의 시작과 같았다.**

| 생산자 | target 이 무엇이었나 |
|---|---|
| visual | 범위의 시작 |
| `:cat`·단어 text object | 범위의 시작 |
| motion (글자 단위) | 뒤로 갔으면 시작, 앞으로 갔으면 끝(그때는 안 옮긴다) |
| motion (줄 단위) | 닿은 줄과 **그 칸** |
| `:` 손으로 친 범위 | 지금 커서 — 「안 옮긴다」를 그렇게 적어 둔 것 |

까닭은 `span` 이 **커서 자리와 이동이 닿은 자리로 범위를 짓기** 때문이다(charSpan·lineSpan). 뒤로 간 이동은 닿은 자리가 곧 시작이 된다.

**걸리는 것은 하나였다. 줄 단위 범위가 칸을 안 담았다.** `yk` 는 칸을 지키고 `ygg` 는 첫 비공백으로 가는데 `[startLine, endLine]` 에는 그 칸이 없었다. 그래서 `lineSpan` 과 visual 의 줄 단위 갈래가 그 칸을 `target` 에만 몰래 실어 보내고 있었다.

**줄 단위 범위도 칸을 담게 했다.** 안전한지 먼저 쟀다 — `startCol`·`endCol` 을 읽는 자리는 여섯인데 **전부 `linewise` 로 갈라져 있어서** 줄 단위일 때는 그 칸에 닿지 않는다(`deleteLines`·`changeCaseRange`·`selectionOn`·`catLines`·`changeRange`·`yankRange`). 담아도 고치는 쪽은 못 본다.

그러고 나니 `target` 은 순수한 되풀이가 되어 지웠다. `moveToRangeStart` 가 `start*` 를 본다.

`:` 만 갈래가 하나 필요했다. 손으로 친 범위는 안 옮기고 `'<,'>` 는 옮기는데, 전에는 `target` 에 「지금 커서」를 구워 넣어 표현했다. 이제는 **부르는 자리가 `isSelection()` 을 보고 부를지 말지 정한다** — 그 판단이 원래 있어야 할 자리다.

```go
type motionRange struct {
	startLine, startCol int
	endLine, endCol     int
	linewise            bool
}
```

`range.go` 라는 이름이 그제야 맞는 말이 되었다.

## 7. `Buffer` 를 패키지로 뺄 수 있나

여기까지 오면 물어볼 만한 것이다. **층이 정말 갈렸다면 `internal/buffer` 로 빠질 수 있어야 한다.** 재 보았다.

**아직은 아니다. 다만 가깝다.** `buffer-*.go` 밖에서 `Buffer` 의 필드를 직접 만지는 자리가 **195 곳**이다.

| 필드 | 곳 | | 파일 | 곳 |
|---|---|---|---|---|
| `cursorLine`·`cursorCol` | 81 | | motion.go | 41 |
| `lines` | 39 | | editor.go | 13 |
| `path` | 30 | | 나머지 | 한 자릿수로 흩어짐 |
| 나머지 | 45 | | | |

성격이 셋으로 갈린다.

- **`motion.go` 41 곳은 정당하다.** 범위를 만들려면 커서와 줄을 읽어야 한다. 다만 패키지가 갈리면 읽기 API 가 필요하다
- **`path` 30 곳은 싸다.** 접근자 하나면 끝난다
- **`cursorLine`·`cursorCol` 81 곳이 진짜 일이다.** 대부분 읽기이지만 그 API 를 어떤 모양으로 낼지가 설계다

**그래서 지금 가르지 않는다.** 195 곳 중 상당수가 「편집기가 제 buffer 를 들여다본다」는 정당한 읽기라, 갈라도 읽기 API 가 넓게 열린다 — 컴파일러가 층을 강제해 주는 것은 얻지만 캡슐화는 생각만큼 안 조여진다. 이 결정의 §1~§4 가 얻은 것(의존 방향, 정책 자리) 은 이미 손에 있다.

**한 가지는 지금 고쳤다.** `motionRange` 가 `motion.go` 에 살고 있었다. 한 패키지 안에서는 안 보이지만 `Buffer` 가 열세 곳에서 쓰고 `motion` 도 `Buffer` 를 쓰므로 **가르면 그 자리가 곧 순환**이다. 그리고 §2 가 「`motionRange` 는 글을 아는 겹의 말」이라고 정해 놓은 것과도 어긋났다.

메서드도 없는 좌표 뭉치이고 만드는 곳이 넷(`motion.go` 의 span, visual, 줄 범위, `:cat`) 이라 어느 한쪽에 세 들어 살 자료가 아니다. `range.go` 로 옮겼다.

## 8. 노출 면은 헛것이지만 두기로 한다

재는 동안 나온 것이다. `cmd/main.go` 가 부르는 것은 `core.Run` **하나뿐인데** `Buffer`·`OpenBuffer`·`ConfirmDiscard`·`Exit` 와 `Buffer` 의 메서드 다섯이 대문자로 서 있다. Go 에서 대문자는 「밖에서 쓰라」는 약속이라 거짓 신호다.

**내리지 않는다.** `Buffer` 라는 낱말이 330 곳, 나머지를 합치면 440 곳 남짓인데 그것을 전부 건드려서 동작은 하나도 안 바뀐다. 그리고 `internal/` 아래라 밖에서 부를 길이 애초에 없으므로 이 거짓 신호가 실제로 누구를 헛디디게 하지 않는다.

재 놓고 둔다. 다시 정하고 싶으면 이 절이 그대로 근거이고, 그때의 일은 기계적인 이름 바꾸기 한 번이다.

## 결론: 경계가 어디인가

여기까지의 §들이 하나씩 옮긴 결과다. 말로 적으면 이렇다.

> `Buffer` 는 **글과 그 글을 보고 있는 상태**를 들고, **한 걸음짜리 조작**을 내놓는다.
> 여러 걸음을 엮는 것과 키가 무엇을 뜻하는지는 위 겹이 한다.

| | |
|---|---|
| **드는 것** | 줄과 커서, 그리고 화면 스크롤 자리·고른 범위·되돌리기 이력·디스크와 맞춰 본 것·밖에서 채워 넣는 캐시 셋(진단·git·문법) |
| **내놓는 것** | 커서를 옮기고, 줄을 넣고 지우고, 파일에 쓴다. 그때의 불변(`desiredCol`·되돌리기 구간) 을 안에서 지킨다 |
| **모르는 것** | 키가 무엇을 뜻하는지(`motion`), 범위만으로 정해지는 커서 규칙, register 를 어느 이름에 담는지 |

**커서를 두는 일이 둘로 갈린다.** 이 결정을 낸 뒤 다시 훑어 보고 알았다.

- **범위만 있으면 정해지는 자리**는 부르는 쪽이 둔다. `y` 와 visual `~` 가 그랬다 — 부르는 쪽이 이미 범위를 들고 있어서 `Buffer` 가 알 것이 없었다(§5)
- **편집이 만들어 낸 자리**는 `Buffer` 가 둔다. 붙인 글이 끝난 자리, 지운 자리, 이은 자리, 지운 줄을 메운 줄은 그 편집만 안다. 밖으로 내면 그 자리를 돌려주는 값이 하나 늘 뿐이고, §6 에서 `target` 을 없앤 것과 반대 방향이 된다

편집하면서 커서 자리를 고르는 메서드가 열여섯인데 전부 뒤쪽이다. 「여러 걸음을 엮지 않는다」로 적으면 그것들이 전부 어긋난 것처럼 읽혀서 이렇게 갈라 적는다.

**「범위를 묻는 곳」이 아니다.** `Buffer` 에 「다음 단어까지 어디까지인가」를 묻는 API 는 없다. 답하는 것은 「이 이동을 하면 커서가 어디로 가나」뿐이고(사본 위에서 실제로 옮겨서), 두 자리를 범위로 만드는 것은 `motion` 겹이다.

**흐린 자리 둘은 알고 둔다.**

- **커서와 스크롤**은 tab 마다 유지하려고 여기 든 것이다. `Buffer` 의 본질이라서가 아니라 실용이고, 화면 분할이 오면 밖으로 빼야 한다(buffer.go 가 적어 둔 그대로다)
- **화면 폭**을 메서드 예순 남짓이 받는다. 화면 행 이동과 `desiredCol` 이 줄바꿈에 걸려 있어서다. `Buffer` 가 순수한 텍스트 모형이 아닌 가장 큰 자리이고, 패키지로 가를 때도 여기가 걸린다(§7)

이 글은 `internal/core/buffer.go` 머리글에도 같이 있다. 고칠 일이 생기면 그쪽이 먼저 눈에 든다.

### 적고 나서 다시 훑었다

경계를 말로 적고 나서 그 잣대로 코드를 다시 쟀다. 세 규칙(`motion` 을 아나·register 이름을 아나·operator 이름이 붙었나) 은 **0 곳**이었는데, **반대 방향에서 여덟 곳이 나왔다.** `Buffer` **밖에서 커서·화면·선택 상태를 직접 쓰고 있었다.**

여덟 곳 전부 뒤에서 불변을 손수 챙기고 있었다(`updateDesiredCol` 또는 `moveLineFirstNonBlank`+`clampToNormal`). 오늘은 맞지만 §3 이 「그 겹이 불변을 같이 져야 하고 틀리면 깨진다」고 적어 둔 그 모양이다. 셋으로 갈라 고쳤다.

| 패턴 | 곳 | 어떻게 |
|---|---|---|
| `cursorLine = X` + 첫 비공백 + clamp | 3 (`:s` 끝·`:s///c` 끝·`:번호`) | **이미 있던 `moveToLine` 을 쓴다.** 있는 것을 안 쓰고 있었다 |
| 커서와 화면을 담아 둔 자리로 되돌림 | 2 (검색 무르기·치환 그만두기) | `viewPlace` 와 `place()`·`moveToPlace()` 를 두었다 |
| `selection` 을 직접 열고 비움 | 5 (여는 것 1, 비우는 것 4) | `startSelection()`·`clearSelection()` 을 두었다 |

**둘째는 부르는 쪽도 줄었다.** 검색은 `searchOrigin` 이 네 필드를, 치환은 `backLine`·`backCol`·`backTop`·`backTopRow` 를 따로 들고 있었는데 둘 다 `viewPlace` 하나가 되었다. 「커서만 되돌리면 보이는 곳이 달라진 채로 남는다」는 같은 주석이 두 파일에 있던 것도 한 자리로 모였다.

다시 재니 **8 → 0** 이다. 이제 `Buffer` 의 상태를 쓰는 것은 `Buffer` 안뿐이다.

## 되짚은 것

**처음에는 「걷어낼 것이 없다」로 닫으려 했다.** 죽은 메서드가 없고 중복도 우연한 가드뿐이어서, 호출 자리가 하나뿐인 것들을 「vim 키가 1:1 이라 그렇다」로 넘겼다.

그 잣대가 두 번 틀렸다.

- **`*ByMotion` 셋을 놓쳤다.** 셋 다 그 46 개 안에 있었는데 1:1 로 설명하면 나머지와 갈리지 않는다
- **반대로 `moveDownLine` 같은 한 줄짜리를 의심하게 만들었다.** 「키가 집을 둘 가진다」로 읽혔는데, 재 보니 한쪽은 정책이고 한쪽은 기계라 둘 다 자리가 맞았다

**1:1 은 결과이지 근거가 아니었다.** vim 의 이동 키가 마침 커서 원시 조작과 겹치는 것뿐이다. 실제로 서 있던 규칙은 §3 의 읽기/쓰기 가름이고, 그것으로 보면 두 물음에 한 번에 답이 나온다 — `*ByMotion` 은 읽고 정하는 쪽이 `Buffer` 안에 있었고, `moveDownLine` 은 쓰는 쪽이라 `Buffer` 안이 맞다.

**규칙을 코드에서 읽어내지 않고 겉모양에서 지어낸 것이 잘못이었다.** 다음에 이런 훑기를 할 때는 「무엇과 1:1 인가」가 아니라 **어느 겹이 상태를 쓰는가**를 먼저 잰다.

`docs/tasks.md` 가 적어 둔 숫자(메서드 107 개, 호출 자리 하나뿐인 것 39 개) 도 낡아 있었다. 적어 둔 숫자를 근거로 쓸 때는 다시 잰다.
