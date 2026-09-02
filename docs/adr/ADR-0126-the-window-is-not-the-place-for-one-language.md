# ADR-0126: 창에 한 언어만의 기능을 두지 않는다

## 맥락

`Buffer`+`viewport` 를 새 패키지로 낼 수 있는지 재던 갈래의 마지막 자리다(ADR-0124, ADR-0125). core 에서 언어 이름이 문에 박힌 곳이 하나 남아 있었다 — `syntax.MarkdownTableCells` 를 부르는 `table.go` 다.

처음에는 「이것이 gofmt·goimports 와 같은 계열인가」로 물었다. **계열은 같은데 합칠 수는 없었다.**

| | gofmt·goimports | 표 맞추기 |
|---|---|---|
| 하는 일 | 글을 규칙에 맞춰 다시 그린다 | 같다 |
| 언제 | 저장할 때 | 손으로 부를 때 |
| 어디서 | 바깥 명령(`exec`) | 안에서 |
| 무엇을 | 파일 전체 | **범위** |
| 무엇에 기대나 | stdin/stdout | **강조 문맥**과 **터미널에서 잰 자**(ADR-0072) |

`formatter` 표의 칸이 전부 바깥 명령을 찾고 깔고 돌리는 것(`find`·`install`·`args`)이라 안에서 도는 것이 들어갈 자리가 없다. 그리고 표는 「이 터미널에서 보기에」 맞춰진 것이라 그 자가 core 에만 있다.

## 1. 물음이 바뀌었다

「어느 계열인가」가 아니라 **「창의 메서드일 이유가 있는가」**였다.

`formatTables` 가 창에게서 쓰는 것을 세어 보니 이랬다.

| 무엇 | 창이라서 필요한가 |
|---|---|
| `buf.lines` · `buf.language.State()` | 읽기만 한다. 인자로 받아도 된다 |
| `beginEdit`·`replaceLines`·`endEdit` | **대량 변경 그 자체** |
| 커서를 줄 끝으로 당기기 | 대량 변경이면 어차피 하는 뒷일 |

**markdown 을 아는 부분은 창을 하나도 안 만졌다.** `tableAt`·`renderTable`·`padCell`·`tableAlignOf` 는 이미 `[][]byte` 와 `[]syntax.State` 만 받는 자유 함수였다. 창의 메서드인 것은 껍데기 하나뿐이었고, 그 껍데기가 한 일은 「새 줄들을 짓고 → 갈아끼우고 → 커서를 살린다」다.

**창 쪽에서 보면 그것은 그냥 줄 여럿을 한 번에 바꾸는 일이다.** 창이 markdown 을 알 이유가 없다.

## 2. 새 줄을 짓는 데까지가 언어의 일이다

```go
func formattedTables(lines [][]byte, state syntax.State, from, to int) (
	at int, next [][]byte, found, changed int)
```

갈아끼우는 것은 부르는 쪽(`formatTablesIn`) 이 한다. 되돌리기 구간을 열고 닫는 넉 줄이 그리로 갔다.

## 3. 「모든 언어가 가진 것」과 「어떤 언어만 가진 것」

`reindentLines` 도 언어별로 달라지는데 그쪽은 창에 남긴다. **둘이 다르기 때문**이다.

| | 언어별 규칙이 어디 있나 | 메서드 몸 안에 언어가 있나 |
|---|---|---|
| `reindentLines` | `syntax.Indent` **interface** | **없다.** `rule.Reindents()`·`Next()`·`Close()` 만 부른다 |
| `formatTables` | `syntax.MarkdownTableCells` **함수** | **있었다.** 파이프·구분줄·`:` 정렬이 core 코드다 |

`reindentLines` 는 Go 와 Python 을 가르지 않는다. 「이 줄의 규칙을 다오」하고 받아 그대로 쓴다. **언어가 달라지는 것은 맞지만 그 달라짐이 창을 지나지 않는다.**

그 가름이 언어 표에 이미 새겨져 있었다.

```go
var languageRules = []Language{
	{exts: {".go"}, state: goNormal{}, indent: tabBraceIndent, outline: blockOutline{...}},
	...열한 줄 전부 셋을 채운다
}
```

**칸이 nil 인 줄이 하나도 없다.** nil 이 나오는 자리는 언어를 못 알아봤을 때뿐이다. 그러니 이 표의 불변이 곧 그 가름이다 — **칸에 있는 셋은 모든 언어가 가진 개념이고 규칙만 다르다.** `Table` 을 넷째 칸으로 넣으면 열한 줄 중 열 줄이 nil 이 되어 그 불변이 깨진다.

반대쪽 손도 이 저장소에 있다. `formatters` 는 **기능이 자기가 걸리는 언어를 안다**(`exts`).

| | 방향 | 채움 |
|---|---|---|
| `Language` 의 칸 | 언어가 자기 규칙을 든다 | 모두 |
| `formatters` 의 `exts` | 기능이 자기가 걸리는 언어를 안다 | 일부 |

**gofmt 와 같은 계열이라는 직관이 여기서 맞았다.** 돌리는 방식이 아니라 **누가 누구를 아는가의 방향**이 같다.

## 4. 그래서 `MarkdownTableCells` 는 그대로 둔다

셋째 손이 있고 그것이 지금 것이다. **확장자가 아니라 문맥으로 가른다.**

```go
if _, markdown := state.(mdNormal); !markdown { return nil, false, false }
```

`.md` 파일 안의 코드펜스는 markdown 문맥이 아니라 표로 보지 않고, 반대로 markdown 문맥이면 어디서든 본다. `exts` 손으로 바꾸면 이 분별을 잃는다. `Language` 의 칸으로 올리면 표의 불변이 깨진다.

남는 흠은 이름 하나 — core 의 낱말에 `Markdown` 이 박힌 것이다. 그런데 부르는 자리가 `table.go` 한 곳이고 그 파일 자체가 markdown 표 맞추기다.

## 규모

| | |
|---|---|
| 창에서 나간 메서드 | 1 (`formatTables`) |
| 옮길 무리 | 114 → **101** 개 이름, 60 → **57** 파일 |
| 시험 | 13 곳이 새 길을 지나게 됨 |

동작은 안 바뀐다. `\mt` 로 한글 든 표를 맞추고 `u` 한 번에 통째로 무르는 것을 앱에서 확인했다.

## 남는 것

- **`reindentLines`** 는 창에 남는다. 그런데 그것은 캐시를 쓴다(`lexSyntaxTo`·`syntaxTokens`·`indentRuleAt`). 밖으로 내려면 함수를 넘기거나(DI) 인자가 일곱이 되어, 지금은 값보다 짐이 크다
- **`sortLines`·`changeCaseRange`** 도 창의 메서드다. 언어와 무관하므로 이 잣대에는 안 걸리지만, 「범위를 받아 줄을 다시 쓴다」는 꼴은 같다. 넷을 한 표로 볼지는 열려 있다
- **되풀이되는 넉 줄.** `trimTrailingSpace`·`sortLines`·이제 `formatTablesIn` 이 「구간 열기·갈기·커서 살리기·닫기」를 손으로 되풀이한다. 묶을지는 ADR-0100 이 미뤄 둔 「줄 하나를 한 번의 되돌리기로 갈아끼우는 손 여덟 자리」와 같은 건이다

## 되짚은 것

**「어느 계열인가」로 물었을 때는 답이 안 나왔다.** 계열은 같은데 합칠 수 없었고, 거기서 멈추면 아무것도 안 바뀐다.

**「누가 그것을 알아야 하는가」로 바꾸니 답이 나왔다.** 창은 대량 변경만 알면 되고, markdown 은 새 줄을 짓는 데까지만 알면 된다. 그 둘 사이에 걸쳐 있던 것이 껍데기 하나였다.
