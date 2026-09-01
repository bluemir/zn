# ADR-0117: `Buffer` 는 그대로 두고 그 아래 층을 낸다

> **뒤집혔다 (ADR-0119).** `internal/lines` 는 물렸다. 재는 일과 그리는 일이 한 덩어리라 가를 수 없었고, 이 글이 근거로 쓴 「buffer 60 / 그 밖 89」의 89 는 그 뒤 core 로 옮긴 `WidthOf` 가 74 를 차지한 숫자였다. **아래의 §1·§2·§6 과 이름을 고른 §5 는 그대로 서고, §4 의 결론만 뒤집혔다.**


## 맥락

ADR-0100 §7 이 「`Buffer` 를 패키지로 뺄 수 있나」를 묻고 「아직은 아니다. 다만 가깝다」로 닫았다. `docs/tasks.md` 에 그 항목이 남아 있어 다시 쟀다.

재는 데 grep 을 쓰지 않았다. `.lines` 는 `register` 와 `syntaxCache` 도 가지고 `.path` 는 sidebar 도 가져서, 이름만 세면 다른 타입의 필드가 같이 잡힌다. `go/types` 로 selector 마다 receiver 타입을 확인해 셌다.

## 1. 적어 둔 숫자가 사흘 만에 낡았다

| | ADR-0100 (8/28) | 지금 (9/1) |
|---|---|---|
| `buffer*.go` 밖에서 필드 접근 | 195 | 250 (읽기 232, 쓰기 18) |
| 밖에서 메서드 호출 | 안 쟀음 | 229 |

읽기 상위는 `lines` 56, `path` 55, `cursorLine` 47, `cursorCol` 22, `dirty` 12 다. 파일로는 motion.go 41, editor.go 24, language-server.go 12, semantic.go 12, table.go 12 다. 쓰기 18 곳은 거의 캐시류(`gitBase`·`gitLines`·`diagnostics`·`diskSize`·`diskTime`·`outside`) 이고 git.go 5, outside.go 5 에 몰려 있다.

사흘에 55 곳이 늘었다. commit graph 화면과 날짜 넣기가 그 사이에 들어온 것이다(ADR-0115, ADR-0116). **가르지 않으면 이 숫자는 계속 는다.**

## 2. §7 이 재지 않은 것이 진짜 벽이었다

§7 은 「밖에서 안을 만지는 곳」만 셌다. 그런데 가르면 컴파일이 안 되는 까닭은 그쪽이 아니라 반대쪽이다. **`buffer*.go` 가 core 의 다른 파일에서 가져다 쓰는 이름이 58 가지, 17 개 파일이다.**

| 선언 파일 | buffer 가 씀 | 밖이 씀 | 갈랐을 때 |
|---|---|---|---|
| cluster.go | 60 | 89 (29 파일) | 아래로 내려야 한다 |
| page.go | 4 | 74 | core 에 남는다 |
| range.go | 12 | 47 | core 에 남는다 |
| outside.go | 14 | 31 | core 에 남는다 |
| search.go | 9 | 29 | core 에 남는다 |
| word.go | 27 | 21 | 아래로 내려야 한다 |
| register.go | 20 | 5 | 정해야 한다 |
| indent-range.go | 19 | 7 | 정해야 한다 |
| save-hook.go | 7 | 6 | 정해야 한다 |
| indent.go | 31 | 0 | 같이 내려간다 |
| selection.go·syntax.go·sticky.go·readonly.go | 9 | 0 | 같이 내려간다 |

**「core 에 남는다」가 곧 순환이다.** `buffer → core → buffer` 가 되므로 그 이름들은 buffer 아래로 한 번 더 내려가거나 core 쪽에서 끊어야 한다. `internal/buffer` 하나를 만드는 일이 아니라 **층이 셋** 이 되는 일이다.

## 3. 그래서 `Buffer` 는 안 가른다

250 곳을 접근자로 바꾸고 58 가지 이름의 집을 새로 정해야 하는데, 얻는 것은 여전히 컴파일러의 강제뿐이다. `lines` 를 읽는 56 곳 때문에 `[][]byte` 를 그대로 내주면 캡슐화도 §7 이 적은 그대로 안 조여진다. §7 의 판단이 그대로 선다.

## 4. 아래 층 하나는 지금 낸다

`internal/lines` 로 낸 것은 cluster.go(203 줄) 와 word.go(57 줄) 다. 이 둘은 §2 의 표에서 성격이 다르다. **core 안의 어떤 이름도 쓰지 않는다.** 받는 것은 `[]byte` 한 줄과 tab 폭뿐이고 상태가 없다.

그리고 쓰는 쪽이 한쪽이 아니다.

| | cluster.go | word.go |
|---|---|---|
| buffer 쪽이 씀 | 60 | 27 |
| 그 밖이 씀 | 89 (29 파일) | 21 |

**양쪽이 나눠 쓰니 어느 한쪽에 넣어도 틀린다.** buffer 로 내리면 그리는 쪽이 buffer 를 거쳐야 하고, core 에 두면 buffer 가 core 를 보게 된다. 아래로 내리는 것만 맞다. ADR-0100 §7 이 「패키지로 가를 때 여기가 걸린다」고 적은 화면 폭 문제가 이 파일이기도 하다.

18 개 심볼이 전부 밖에서 쓰여 통째로 공개가 되었고, 테스트까지 375 곳이 바뀌었다. 동작은 하나도 바뀌지 않는다.

## 5. 이름을 두 번 골랐다

처음에 `internal/glyph` 로 냈다. 재는 단위가 grapheme cluster 라 그 이름이 맞아 보였다.

**틀렸다.** 밖에서 부르는 횟수로 단위를 가르면 이렇다.

| 다루는 단위 | 심볼 | 부른 횟수 |
|---|---|---|
| **줄 하나를 통째로 훑는다** | `WidthOf` 79, `ColAt` 34, `WrapOffsets` 30, `OffsetAtCol` 9, `PrevStart` 7, `WidthOfStyled` 3 | **162** |
| 글자 하나 | `WordSmall` 42, `GlyphSize` 21, `WordKind` 18, `WordBig` 13, `ClassBlank` 10, `GlyphAt` 7, `ClassAt` 5, 나머지 5 | 121 |
| 화면 행 | `RowIndexAt` 10, `Row` 6, `RowRange` 5, `RowBefore` 3 | 24 |

줄 단위가 더 많고, 그것이 우연이 아니다. (`WordKind`·`WordSmall`·`WordBig` 은 이 표를 낸 뒤에 키의 개념으로 판정되어 core 로 돌아갔다. 아래의 flag 절이 그것이다.)

### 줄 시작만이 알려진 경계다

grapheme cluster 는 뒤에서 앞으로 읽을 수 없다(`PrevStart` 의 주석이 그렇게 적혀 있다). tab 이 미는 폭은 시작 칸을 알아야 정해진다. **임의의 byte offset 이 글자 경계인지 알 방법이 없어서 모든 계산이 줄 시작에서 출발한다.** `ColAt`·`OffsetAtCol`·`WrapOffsets` 가 전부 `for offset := 0` 인 것이 그 자국이다.

그래서 함수들이 `line` 과 `offset` 을 같이 받는다. offset 은 그냥 위치가 아니라 **줄 시작에서 걸어온 거리**이고 그 줄을 빼면 뜻이 없는 값이다. 기준점이 줄이니 이름도 줄이어야 한다.

### 그림자를 다시 쟀다

처음에 「`line` 은 296 곳에서 그림자가 진다」고 한 것은 **그 이름이 쓰인 자리 전부**를 센 것이었다. 실제로 부딪히는 것은 이 패키지를 부르는 함수 안에 그 이름이 있을 때뿐이라 다시 쟀다.

| 후보 | 부딪히는 함수 | 파일 |
|---|---|---|
| `width` | 53 | 24 |
| `line` | 48 | 27 |
| `row` | 31 | 18 |
| `text` | 19 | 13 |
| `lines` | **1** | 1 |
| `screen`·`cluster`·`glyph` | 0 | 0 |

부르는 함수는 167 개다. `line`(단수) 은 그중 48 개에서 막히는데, 그 함수들에서 `line` 은 가장 자연스러운 변수 이름이라 비켜 주기 아깝다. **`lines`(복수) 는 하나뿐이었다.** `buf.lines` 는 필드라 그림자가 지지 않아서다. `strings`·`bytes` 와 같은 복수형이고, 「그 갈래를 다루는 도구 모음」이라는 뜻이 된다.

부딪힌 하나는 `git-lines_test.go` 의 시험 도우미 `lines()` 였고 `toLines()` 로 바꿨다. 지역 변수 하나(view-graph_test.go) 도 같이 바꿨다.

### 이름 표

| 옛 이름 | 새 이름 | | 옛 이름 | 새 이름 |
|---|---|---|---|---|
| `clusterAt` | `lines.GlyphAt` | | `charClass` | `lines.Class` |
| `clusterSize` | `lines.GlyphSize` | | `clusterClass` | `lines.ClassAt` |
| `screenColAt` | `lines.ColAt` | | `classBlank` | `lines.ClassBlank` |
| `screenWidthOf` | `lines.WidthOf` | | `wordKind` | core 로 돌아갔다 |
| `offsetAtScreenCol` | `lines.OffsetAtCol` | | `smallWord` | core 로 돌아갔다 |
| `screenRow` | core 로 돌아갔다 | | `bigWord` | core 로 돌아갔다 |

`screen`·`cluster` 접두는 뗐다. 패키지 이름이 그 자리를 대신한다. **다만 `GlyphSize`·`GlyphAt` 은 접두를 새로 붙였다.** `lines.Size(line, offset)` 는 「줄의 길이」로 읽히는데 실제로는 그 자리 글자 하나의 byte 길이다. 패키지가 줄을 말하게 되었으니 글자를 다루는 둘은 그렇다고 밝혀야 한다.

**`smallWord`·`bigWord` 는 거꾸로 갔다.** vim 의 굳은 말은 small word 인데 `wordSmall` 로 뒤집었다. 갈래를 앞에 붙여야 `word` 를 치면 둘이 같이 나온다. 읽는 맛보다 찾는 손을 골랐다. 이 셋은 아래에서 core 로 돌아가지만 이름은 그대로 간다.

### 화면 행은 돌려보냈다

`screenRow` 와 `rowBefore` 는 core 로 되돌렸다(row.go). 줄을 행으로 끊는 일(`WrapOffsets`) 은 이 패키지의 것이지만 **그 행으로 화면을 채우는 일은 아니다.** `screenRow` 를 만드는 것은 `visibleRows`(buffer-screen.go) 이고 쓰는 것은 그리는 쪽이다. `rowBefore` 는 줄도 글자도 안 보고 좌표 넷을 견주기만 한다.

`RowIndexAt`·`RowRange` 는 남는다. `WrapOffsets` 가 낸 offset 묶음을 읽는 짝이라 그것과 떨어지면 뜻이 없다.

### `w` 와 `W` 를 가르는 flag 도 돌려보냈다

`ClassAt` 이 `WordKind` 를 받고 있었다.

```go
if kind == WordBig {
	// 큰 단어는 공백으로만 끊으므로 공백이 아닌 것은 전부 한 부류다.
	return ClassWord
}
```

**`WordBig` 은 부류를 매기는 것이 아니라 매겨진 부류를 접는다.** 공백이 아닌 셋을 하나로 합치는 것이고, 그것은 글자의 성질이 아니라 그 키의 규칙이다. `WordKind` 의 주석부터가 「vim 의 word 와 WORD 다」라고 적고 있었다.

부르는 자리를 재니 그 신호가 또 있었다.

| 부르는 것 | 곳 | `kind` 를 어떻게 넘기나 |
|---|---|---|
| `lines.ClassAt` | 5 | **4 곳이 `WordSmall` 상수** (buffer-search.go), 1 곳만 변수 |
| `Buffer.classAt` | 16 | 전부 변수 |

**flag 가 상수로 죽는 자리가 패키지 경계 바로 앞이었다.** core 안에서는 `w`·`W` 가 같은 길을 지나는 진짜 런타임 값이라(키를 읽는 자리에서 정해 motion 이 다섯 단계를 나른다) 함수를 둘로 나눠도 core 의 분기는 안 없어진다. 없앨 수 있는 것은 `lines` 쪽 flag 뿐이었다.

`wordKind`·`wordSmall`·`wordBig` 을 core 로 되돌리고(word.go) 접는 일은 `Buffer.classAt` 한 곳에 두었다. `lines.ClassAt(line, offset)` 은 네 부류를 그대로 준다. buffer-search.go 네 곳에서 상수 인자가 사라졌다.

`screenRow` 를 돌려보낸 것과 같은 잣대다. **줄과 글자는 `lines` 의 말이고, 키가 그것을 어떻게 쓰는지는 core 의 말이다.**

### 전제를 다는 함수도 돌려보냈다

`WidthOf` 의 주석이 「**UI 문자열 전용이다**」라고 적고 있었다. 재보니 그것은 **조건을 잘못 말한 것**이었다.

진짜 조건은 「tab 이 없는 글」이다. 증거가 grep 에 있다. grep 결과는 UI 문자열인데 tab 이 들어오므로 `grepExpandTabs` 가 먼저 공백으로 펼치고, 그다음에야 `WidthOf` 가 맞는다. 「UI 라서 써도 된다」가 아니라 「tab 을 미리 없앴으니 써도 된다」였다.

**잘못 적은 조건이 사람을 더 나쁜 길로 보냈다.** 테스트 15 곳이 `ColAt([]byte(plain), len(plain), DefaultTabWidth)` 라는 긴 길로 가고 있었는데, 그 자리는 전부 `ansi.Strip` 한 화면 글이라 `WidthOf(plain)` 이면 되는 곳이었다. 「이게 UI 인가?」를 판정하기 애매하니 안전해 보이는 쪽으로 간 것이다.

`WidthOf`·`WidthOfStyled`·`EscapeSizeAt` 셋을 core 로 옮겼다(width.go). 셋 다 **「다 지은 화면 글」이라는 전제를 달고 tab 폭을 스스로 정한다.** `lines` 의 불변은 「받은 줄과 tab 폭만으로 답한다」인데 이 셋만 그것을 어기고 있었고, 전제를 지키는 코드(`grepExpandTabs`) 는 이미 core 에 있었다.

### 그러고 나니 둘이 하나였다

옮겨 놓고 나란히 보니 `WidthOf` 와 `WidthOfStyled` 의 차이가 escape 하나였다.

**`WidthOfStyled` 는 escape 가 없어도 맞는 답을 낸다.** 건너뛸 것이 없을 뿐이다. 거꾸로 `WidthOf` 는 escape 가 들어오면 그것을 글자로 세어 **조용히 틀린다.**

그런데 쓰이는 횟수는 90 대 4 로 **좁고 틀릴 수 있는 쪽이 압도적이었다.** 갈라 두면 부르는 쪽이 매번 「이 글에 색이 붙었나」를 맞혀야 하는데, 그 물음은 UI 코드에서 답하기 어렵다. 렌더링을 거쳐 온 글인지 아닌지가 호출부에서 안 보인다.

합쳐서 `widthOf` 하나로 두었다. 대가는 offset 마다 ESC byte 비교 한 번이다.

**재 보고 나서야 알았다.** 「화면 폭을 재는 코드가 여기저기 각자 구현되어 있는 것 아니냐」는 물음에 라이브러리 직접 호출을 세어 보니 프로덕션은 0 이었다(`ansi.StringWidth` 0, `lipgloss.Width` 0, `runewidth` 0). **갈래는 프로덕션에 없었고 우리 함수 안에 있었다.** 시험에 있던 `ansi.StringWidth` 14 곳은 `widthOf` 로 모았고, 바꾼 뒤 시험이 그대로 통과한 것이 「같은 것을 재고 있었다」는 증거다.

### 기본값을 고르는 일은 `lines` 의 것이 아니다

`DefaultTabWidth` 를 `lines` 에서 뺐다. 「적힌 것이 없으면 몇 칸으로 볼까」는 `.editorconfig` 를 읽는 겹의 판단이고, 줄을 재는 겹은 그 답을 인자로 받으면 된다.

그러고 나니 `lines` 의 불변이 예외 없이 선다. **tab 폭을 인자로 안 받는 함수가 둘 남는데 둘 다 답이 폭에 닿지 않아서다** — `GlyphSize` 는 byte 길이를 주고(`GlyphAt` 에 넘기는 1 은 0 나눗셈을 피하는 자리 채움이다), `RowIndexAt` 은 이미 끊어 놓은 offset 만 본다.

core 에는 `defaultTabWidth` 가 남아 `.editorconfig` 의 기본값(indent.go) 과 지어 붙인 글을 재는 기본값(width.go) 을 겸한다. 값이 같아서 지금은 갈릴 자리가 없다.

### 전제 대신 기본값으로

`widthOf` 가 tab 폭을 옵션으로 받는다.

```go
widthOf(title)                              // 지어 붙인 글
widthOf(text, withTabSize(buf.tabWidth()))  // 파일에서 온 글
```

**바뀐 것은 인자가 아니라 주석이다.** 전에는 「tab 이 없는 글에만 쓴다」였다. 조건을 어기면 조용히 틀리는데 고칠 길이 없어서 금지로 적을 수밖에 없었다. 이제는 「적지 않으면 기본 폭이다」이고, 다르면 옵션을 준다. **금지가 기본값이 되었다.**

옵션 하나에 functional options 를 쓰는 것은 무거운 편이지만(CLAUDE.md 의 「과도한 추상화를 하지 않는다」), 여기서 사는 것은 확장성이 아니라 **90 곳이 짧은 쪽을 그대로 쓰면서 예외가 말로 드러나는 것**이다. `widthOf(text, tab)` 로 두면 90 곳이 기본값을 손으로 적어야 하고, 함수를 둘로 나누면 이름이 둘이 되어 부르는 쪽이 다시 고르게 된다.

**다만 지금 `withTabSize` 를 부르는 자리는 0 곳이다.** 파일에서 온 글을 재는 자리는 `[]byte` 를 들고 있고 `widthOf` 는 `string` 을 받아서, 문은 열렸지만 타입이 아직 막고 있다. 열어 둔 것은 다음에 그 자리가 왔을 때 금지 문구를 다시 만나지 않기 위해서다.

## 6. 파일 수는 이 결정으로 줄지 않는다

같이 물었던 것이라 재고 적어 둔다. core 는 131 파일 34,858 줄이고 이 결정은 그것을 130 파일로 만든다(둘이 나가고 row.go 가 생겼다). **덩치는 다른 데 있고 그쪽은 못 뗀다.**

| 묶음 | 파일 | 줄 | 밖으로 나가는 참조 |
|---|---|---|---|
| view-* | 28 | 9,252 | 그 밖 717, render-* 37, git* 36, 키 35 |
| 그 밖 | 59 | 13,104 | view-* 104, buffer* 81 |
| buffer* | 21 | 3,581 | 그 밖 219 |
| 키 | 6 | 2,250 | 그 밖 142 |
| git* | 8 | 1,901 | 그 밖 13 |
| render-* | 9 | 1,186 | 그 밖 69 |

view-* 는 밖으로 717 회 나가고 밖도 그쪽을 104 회 쓴다. 양방향이라 떼면 순환이다. 나가는 의존이 0 인 잎을 전부 빼도 131 → 119 다.

**파일 수를 줄이는 축은 이 결정이 아니다.** 이 결정이 사는 것은 층 하나이고, 그 값은 「cluster.go 에 `Buffer` 가 들어오는 것을 컴파일러가 막는다」이다.

## 되짚은 것

§7 이 「가깝다」고 한 것은 **한쪽만 보고 한 말** 이었다. 밖에서 안을 만지는 195 곳을 세고 「이 정도면 가깝다」고 했는데, 반대 방향의 58 가지를 재니 오히려 멀었다.

패키지를 가를 수 있는지는 두 방향을 다 재야 답이 나온다. 밖에서 안을 만지는 것은 **고치면 되는 일**이고, 안이 밖을 쓰는 것은 **집을 새로 지어야 하는 일**이다. 뒤엣것이 값을 정한다.

그리고 그 잣대로 보면 「가를 수 있는 것」을 찾는 물음이 뒤집힌다. 큰 것부터 떼려 하지 말고 **나가는 의존이 0 인 잎부터** 본다. cluster.go 와 word.go 가 그렇게 나왔다.

### 이름도 한 번 틀렸다

처음에 `glyph` 로 낸 것은 **재는 단위를 보고 고른 이름** 이었다. 그런데 이 패키지가 다루는 것은 줄이고 글자는 그 줄을 훑는 단위일 뿐이다. 밖에서 부른 횟수가 162 대 121 로 그것을 말하고 있었는데, 세어 보기 전에 이름부터 골랐다.

「`line` 은 296 곳이라 못 쓴다」도 잘못 잰 것이었다. **그 이름이 쓰인 자리를 센 것이지 실제로 부딪히는 자리를 센 것이 아니었다.** 부르는 함수 안에 그 이름이 있을 때만 부딪히는데 그 조건을 빼고 셌다. 제대로 재니 `lines` 는 한 곳이었다.

§1 에 「적어 둔 숫자를 근거로 쓸 때는 다시 잰다」고 적었는데, 그것으로는 모자란다. **새로 잰 숫자도 무엇을 세는지 틀릴 수 있다.** 숫자를 낼 때는 그 숫자가 어떤 물음의 답인지를 같이 적는다.

### 옮기는 손과 고치는 손을 갈라 놓고 하나를 놓쳤다

`ClassAt` 의 flag 도, `screenRow` 가 여기 있던 것도 **옮기기 전부터 있던 것**이다. 한 패키지 안에서는 안 보이던 것이 경계를 그으니 드러났다.

그런데 처음 옮길 때는 그것을 못 봤다. 「동작은 하나도 바꾸지 않는다」를 지키느라 이름만 바꿔 그대로 실어 날랐기 때문이다. **경계를 긋는 일은 그 자체가 「무엇이 어느 쪽 말인가」를 묻는 일이라, 옮기는 김에 그 물음을 심볼마다 한 번씩 해야 했다.** 옮긴 뒤에 물어서 두 번 고쳤다.
