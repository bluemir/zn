# 「한 글자」는 네 가지다 — byte, rune, grapheme cluster, 화면 칸

편집기가 글자를 셀 때 층이 넷이다. 커서를 옮기는 것과 화면 칸을 세는 것이 서로 다른 층을
봐야 해서, 층을 헷갈리면 증상이 늘 「한글이 깨진다」 한 가지로만 보인다. 우리 계산이 어긋났을
때와 남의 코드가 틀렸을 때의 생김새가 그렇게 겹치므로(`../issues/0001-bubbletea-wide-char-dch.md`
이 뒤쪽이었다 — 우리 폭 판정은 맞았고 렌더러가 틀렸다), 가르려면 층부터 정확히 알아야 한다.

- 잰 날: 2026-08-25
- 잰 판: `github.com/charmbracelet/x/ansi` v0.11.8, `github.com/clipperhouse/displaywidth` v0.11.0

## 1. 네 층

| 층 | 무엇 | 누가 쓰나 |
|---|---|---|
| **byte** | 파일에 든 바이트 | buffer 가 줄을 들고 있는 단위(`[]byte`), 저장·검색 |
| **rune** | 유니코드 코드 포인트 | Go 의 `for range`, `utf8.RuneCount` |
| **grapheme cluster** | 사람이 한 글자로 세는 단위 (UAX #29) | 커서 이동, `x` 로 지우기, 줄바꿈 자리 |
| **화면 칸(cell)** | 터미널 격자에서 먹는 칸 수 (0·1·2) | 커서를 그릴 자리, 가운데 정렬, 테두리 채우기 |

`clusterAt` 하나가 셋째 층의 경계와 넷째 층의 폭을 **함께** 돌려준다. 따로 재면 「한 글자」와
「그 폭」이 어긋나서, 한 글자를 지웠는데 화면은 반 칸만 줄어드는 식이 된다.

## 2. 실제로 재 본 값

```go
size, width := clusterAt([]byte(s), 0, 0)
```

| 글자 | 생김새 | rune | byte | `size` | `width` |
|---|---|---|---|---|---|
| `a` | | 1 | 1 | 1 | 1 |
| `한` | 완성형 U+D55C | 1 | 3 | 3 | 2 |
| `한` | **NFD** ᄒ+ᅡ+ᆫ | **3** | 9 | 9 | 2 |
| `é` | e + 결합 악센트 | **2** | 3 | 3 | 1 |
| `😀` | | 1 | 4 | 4 | 2 |
| `👨‍💻` | 👨 + ZWJ + 💻 | **3** | 11 | 11 | 2 |
| `🇰🇷` | 지역 표시 문자 둘 | **2** | 8 | 8 | 2 |
| `👍🏽` | 👍 + 살색 변경자 | **2** | 8 | 8 | 2 |
| `→` | East Asian **Ambiguous** | 1 | 3 | 3 | 1 ← 6 장 |
| `\t` | | 1 | 1 | 1 | **4** ← 5 장 |

읽을 것은 굵은 자리다. **rune 수와 글자 수가 다르다.** `for _, r := range line` 으로 커서를
옮기면 NFD 한글에서 한 글자가 세 번에 나뉘어 지나가고, 가족 이모지는 중간에서 잘린다.

## 3. glyph 는 왜 안 세나

glyph 는 **글꼴이 그려 내는 모양**이라 cluster 와 일대일이 아니다. `👨‍💻` 는 cluster 하나지만
그 합자를 모르는 글꼴에서는 `👨`·`💻` 두 모양으로 그려진다. 그래도 우리가 세는 값은 안 바뀐다 —
편집기가 알아야 하는 것은 「몇 개로 그려지는가」가 아니라 **「커서가 몇 칸 움직여야 하는가」**이고,
그건 글꼴이 아니라 터미널이 정한다.

그래서 이 문서에 glyph 라는 층은 없다. 넷째 층인 화면 칸이 그 자리를 대신한다.

## 4. 우리 코드에서 어디에 있나

전부 `internal/core/buffer.go` 에 모여 있다. 바닥돌은 `clusterAt` 하나이고 나머지는 그 위다.

| 함수 | 하는 일 | 쓰이는 자리 |
|---|---|---|
| `clusterAt(line, offset, col)` | 글자 하나의 byte 길이와 폭 | 바닥돌. 직접 부르는 곳은 여덟 자리뿐이다 |
| `clusterSize(line, offset)` | 경계만. 폭이 필요 없을 때 | 커서 한 글자 옮기기 |
| `screenColAt(line, offset)` | byte offset → 화면 칸 | **커서를 그릴 자리** |
| `screenWidthOf(text)` | 문자열이 먹는 칸 수 | 가운데 정렬, 테두리 채우기 |
| `offsetAtScreenCol(line, col)` | 화면 칸 → byte offset | `j`·`k` 로 같은 칸 찾기. 두 칸짜리 글자 가운데면 그 글자 시작으로 맞춘다 |
| `prevClusterStart(line, from, offset)` | 뒤로 한 글자 | grapheme cluster 는 **거꾸로 읽을 수 없다** — 줄 앞에서부터 훑는다 |
| `screenWidthOfStyled(text)` | 색을 입힌 줄의 폭 | **다 지은 화면 줄에만** 쓴다. escape 를 건너뛴다 |

`screenWidthOf` 는 `screenColAt` 을 부르고, 그것이 `clusterAt` 을 글자마다 돌린다. 서술이
아니라 실제 호출이다 — `ansi.StringWidth` 를 바로 부르지 않는 이유는 다음 장이다.

## 5. `clusterAt` 이 얹은 예외 둘

**tab.** `ansi.StringWidth("\t")` 는 **0** 이다(재 봤다). 그대로 두면 들여쓰기가 화면에서
사라진다. 그래서 `clusterAt` 이 `tabWidth - col%tabWidth` 로 직접 답한다. 폭이 시작 칸에
따라 1~4 로 달라지는 유일한 글자이고, `col` 인자가 있는 이유가 이것 하나다.

**깨진 UTF-8.** 폭 0 이 나오면 `offset` 이 안 움직여서 부르는 쪽 for 문이 무한히 돈다.
`1, 1` 로 억지로 한 칸 밀어낸다.

두 예외 때문에 폭을 잴 때 `ansi.StringWidth` 를 그냥 부르면 안 된다. 우리 규칙이 담긴 것은
`screenWidthOf` 쪽이다.

## 6. Ambiguous 폭 — 답이 하나가 아닌 갈래

`→ ─ ± ° ·` 는 East Asian Width 가 **Ambiguous** 다. 한 칸으로 그리는 터미널과 두 칸으로
그리는 터미널이 둘 다 있다. 유니코드가 정해 주지 않으므로 **재는 수밖에 없다** —
`internal/terminal/ambiguous-width.go` 가 글자를 하나 찍고 커서가 몇 칸 갔는지 물어본다(DSR 6).

**손잡이는 프로세스 전역 하나뿐이다.** `x/ansi` 의 `method.go` 가 그 값을 패키지 변수로 들고
있고, 바꾸는 길은 환경변수 `RUNEWIDTH_EASTASIAN` 뿐이다. 호출마다 고르는 인자가 없다
(`ansi.WcWidth`/`ansi.GraphemeWidth` 는 다른 축이다).

```go
var dwOptions = &displaywidth.Options{EastAsianWidth: false}

func init() {
	if ea, err := strconv.ParseBool(os.Getenv("RUNEWIDTH_EASTASIAN")); err == nil && ea {
		wcOptions.EastAsianWidth = true
		dwOptions.EastAsianWidth = true
	}
}
```

그 전역이 우리 계산까지 그대로 뚫고 온다. 코드를 한 줄도 안 고치고 잰 값이다:

| | `ansi.StringWidth("→")` | `screenWidthOf("→")` |
|---|---|---|
| 그냥 | 1 | 1 |
| `RUNEWIDTH_EASTASIAN=1` | **2** | **2** |

`ansi` 를 지나지 않고 `displaywidth` 를 직접 부르는 길도 있다. `Options` 를 인자로 받으므로
전역이 아니고, 이미 간접 의존으로 `go.mod` 에 들어와 있다.

**지금 zn 은 눈금이 둘이다.** `clusterAt` 은 잰 값을 안 보고 늘 1 칸으로 세고,
특수문자 격자의 `symbolWidth`(`internal/core/symbol.go`) 만 잰 값을 본다. 그래서 두 칸으로
그리는 터미널에서 격자는 맞고 파일 본문은 글자마다 한 칸씩 어긋난다 — `─` 가 든 줄에서
커서·검색 강조·줄바꿈이 같이 밀린다. 이 갈림을 없애는 것이 `../tasks.md` 의 항목이다.

## 7. 손댈 때 밟기 쉬운 자리

- **rune 으로 순회하지 않는다.** `for _, r := range line` 은 NFD 한글과 이모지 조합을 쪼갠다.
  `clusterAt` 으로 `size` 만큼 뛴다
- **뒤로는 못 읽는다.** grapheme cluster 는 앞에서부터만 경계를 안다. 뒤로 갈 때는
  `prevClusterStart` 로 알려진 경계(행 시작) 에서부터 훑는다
- **폭 0 을 그냥 두면 무한 반복이다.** 부르는 쪽 for 문이 `size` 로 도는데 그것이 0 이면 안 움직인다
- **색을 입힌 줄과 파일 내용은 다른 함수다.** escape 가 섞인 줄은 `screenWidthOfStyled`,
  파일 내용은 `screenWidthOf` 다. 파일 안의 ESC byte 는 색이 아니라 내용이다
- **폭을 라이브러리에 바로 묻지 않는다.** tab 과 깨진 UTF-8 규칙이 `clusterAt` 에만 있다

## 관련

- `../adr/ADR-0028-ambiguous-width-box-fallback.md` — 박스 그리기 문자를 재서 고르고, 못 재면 ASCII 로 내린다
- `../adr/ADR-0056-symbol-drawer.md` §6 — 특수문자 격자가 잰 값을 쓰기 시작한 자리
- `../issues/0001-bubbletea-wide-char-dch.md` — 우리 폭 계산이 전부 맞는데도 화면이 밀린 예.
  원인은 렌더러의 증분 갱신(DCH) 이었고 판을 올려서 없어졌다. 층을 의심하기 전에 이런 자리도 있다
- `../tasks.md` — 잰 값을 폭 계산 전체에 반영하는 항목
