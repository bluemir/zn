# ADR-0081 화면 행과 논리 줄이 이름과 count 를 나란히 받는다

- 상태: 채택
- 날짜: 2026-08-26
- 넓히는 것: ADR-0013 §「검색 motion 과 `↑` `↓` 는 뺐다」 (operator 이야기였던 것을 홀로 쓸 때와 갈랐다)

## 맥락

`Buffer` 메서드를 갈래별 파일로 나누다가 위아래 이동 넷의 이름이 어긋나 있는 것이 드러났다.

```go
moveUp(n, width)  → n 번 moveUpRow  ... 화면 행
moveUpRow(width)  → 한 걸음          ... 화면 행
moveUpLine(n)     → 한 번에          ... 논리 줄
```

**이름칸 하나에 축이 둘 겹쳐 있었다.** 「화면 행이냐 논리 줄이냐」와 「count 를 받느냐」다.
양끝으로 가는 짝(`moveLineStart`/`moveRowStart`, `moveLineEnd`/`moveRowEnd`) 은 두 축 중
앞의 것을 이름에 다 적어 두었는데, 위아래만 **접미 없는 `moveUp` 이 화면 행 쪽**이었다.
`moveRowStart` 를 본 눈은 `moveUp` 을 줄로 읽는다.

## 1. 넷 다 접미를 달고 넷 다 n 을 받는다

```go
moveUpRow(n, width int)    moveDownRow(n, width int)     // 화면 행
moveUpLine(n int)          moveDownLine(n int)           // 논리 줄
```

이름에 남은 축이 **하나**가 된다. Row 냐 Line 이냐다. count 는 넷 다 받으므로 가를 것이 없다.

`width` 가 Row 쪽에만 있는 것은 그대로 둔다. 화면 행을 세려면 `wrapOffsets` 로 재야 하고
논리 줄은 index 를 더하면 끝난다. 서명이 그 사실을 말하는 편이 낫다.

**두 낱말의 뜻은 `buffer-move.go` 머리글에 적었다.** 어느 것이 논리 줄이고 어느 것이 화면
행인지, 어느 키가 어느 쪽인지가 거기 있다. 저장소가 이미 그 둘만 쓰고 있었는데(코드 26 개
파일 + ADR 넷) 뜻을 적어 둔 자리가 없었다. 이름칸이 어긋난 것을 아무도 못 본 까닭의 절반이
그것이다.

한 걸음짜리는 `moveUpRowOnce`·`moveDownRowOnce` 로 남았다. 부르는 곳이 바로 위의 되풀이
하나뿐이라 밖에서는 보이지 않는다. **되풀이가 필요한 까닭이 Row 쪽에만 있다.** 줄마다
화면 행이 몇 개인지가 달라서 한 번에 셈할 수 없고, 지나는 줄을 다 재야 안다.

## 2. `↑`·`↓` 가 홀로 쓸 때 count 를 받는다. 행동이 바뀐다

이름을 고치다 **count 를 흘리고 있는 자리**가 드러났다.

```go
func (motionRowUp)    move(buf *Buffer, count, width int) { buf.moveUp(1, width) }        // 버린다
func (motionLineDown) move(buf *Buffer, count, width int) { buf.moveDownLine(max(count, 1)) }
```

`action.go` 가 `c.motion.move(buf, c.count, …)` 로 count 를 넘기는데 화살표 쪽만 버렸다.
그래서 **`3j` 는 세 줄인데 `3↑` 는 한 행이었다.**

ADR 을 뒤져 보니 정해 둔 것이 아니었다. ADR-0013 이 적은 것은 「`↑`·`↓` 를 **operator 뒤에**
받지 않는다」(`d↑` 를 어떻게 잡을지 안 정했다) 이고, **홀로 쓸 때 count 를 받을지는 어디에도
없다.** 두 물음을 하나로 뭉쳐서 흘린 것이다.

`max(count, 1)` 을 넘기게 했다. `3↑` 가 세 행이다. vim 도 `3<Up>` 이 세 줄이라 손이 기대하는
것과 같고, 무엇보다 **`3j` 와 `3↓` 가 갈릴 까닭이 없다.** 둘의 차이는 세는 단위이지 count 를
받느냐가 아니다.

operator 쪽은 그대로 안 받는다. ADR-0013 이 든 근거(`d↓` 가 wrap 된 줄에서 무엇을 잡는지
정해지지 않았다) 는 이번 일과 무관하게 살아 있다.

**눈에 안 띄던 까닭.** 화살표에 숫자를 붙이는 손이 드물다. `3j` 는 치는데 `3↓` 는 잘 안 친다.
숫자를 셀 만큼 멀리 갈 때는 대개 글자 키를 쓴다. 그래서 재지 않으면 드러나지 않는 종류다.
`TestNormalModeArrowsTakeCount` 가 이제 지킨다.

## 고르지 않은 것

- **`moveUp`·`moveDown` 이름을 남기고 count 만 고치기.** 이름이 어긋난 것이 이 건의 출발이라
  그것을 두면 다음에 또 같은 자리에서 헷갈린다. 부르는 곳이 제품 코드 넷뿐이라 값이 싸다
- **한 걸음짜리를 밖에도 열어 두기.** `moveUpRow(1, …)` 로 같은 답이 나온다. 문이 둘이면
  「한 번만 갈 때는 어느 것을 쓰나」가 매번 갈린다
- **`home`·`end` 도 count 를 받게 하기.** 화면 행의 시작으로 세 번 가는 것은 한 번 가는 것과
  같은 자리다. 셈이 뜻을 갖지 않아서 `motionRowStart`·`motionRowEnd` 는 그대로 버린다.
  `$` 가 count 를 받는 것은 「n 줄 아래의 끝」이라는 다른 뜻이 있어서다
