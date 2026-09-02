package textarea

import (
	"bytes"
	"strconv"

	"github.com/bluemir/zn/internal/syntax"
)

// indentUnit 은 이 파일이 한 단계에 쓰는 공백이다. 정하는 근거가 셋이고 순서가 있다.
//
// syntaxCache.path 와 같은 손이다 — 게으르게 정하고 담아 두었다가 경로가 달라지면 다시
// 정한다. 이름 없이 열었다가 `:w foo.go` 로 이름이 붙는 길이 있다.
type indentUnit struct {
	// path 는 text 를 정할 때 본 경로다. text 가 비어 있는 것과 아직 안 정한 것을 이것이 가른다.
	Path string
	set  bool

	text []byte
}

// resolveIndentUnit 은 한 단계를 정한다. 근거 셋을 순서대로 본다.
//
// **적힌 것이 잰 것보다 앞선다.** `.editorconfig` 는 그 저장소가 쓰기로 한 것을 사람이 적어
// 둔 것이고, 재는 것은 이미 쓰인 것에서 되짚는 짐작이다. 짐작이 맞는 경우가 대부분이지만
// 새로 만드는 파일이나 아직 한 줄도 들여쓰지 않은 파일에서는 아무것도 알아낼 수 없다.
// 인자 셋이 근거 셋이다 — `.editorconfig` 는 경로를 거슬러 올라가며 찾고, 재는 것은 내용을
// 보고, 마지막이 언어 규칙이다. 앞의 둘이 path 하나에 실려 있던 것을 갈랐다 (ADR-0080).
func resolveIndentUnit(Path string, lang *syntax.Language, Lines [][]byte) []byte {
	if unit := editorconfigUnit(Path); unit != nil {
		return unit
	}

	if unit := measureIndentUnit(Lines); unit != nil {
		return unit
	}

	if rule := lang.Indent(); rule != nil {
		return rule.Unit()
	}

	// 언어를 모르는 파일이다. 규칙이 없어서 단계가 쓰일 일도 없지만, insert 의 `tab` 키가
	// `\t` 를 넣는 것과 같은 답을 둔다.
	return []byte{'\t'}
}

// editorconfigUnit 은 `.editorconfig` 가 정한 한 단계다. 적힌 것이 없으면 nil 이다.
//
// 여기서 보는 키는 셋이다 — `indent_style` `indent_size` `tab_width`. 저장할 때의 모습을
// 정하는 셋은 다른 자리가 본다(editorconfig.go, ADR-0052). 읽어 오는 자리는 그 파일 하나다.
func editorconfigUnit(Path string) []byte {
	def := editorconfigFor(Path)
	if def == nil {
		return nil
	}

	switch def.IndentStyle {
	case "tab":
		return []byte{'\t'}
	case "space":
		Size, err := strconv.Atoi(def.IndentSize)
		if err != nil || Size < 1 {
			// `indent_size = tab` 이면 tab_width 가 칸 수다. 라이브러리가 이미 풀어 준다.
			Size = def.TabWidth
		}
		if Size < 1 {
			return nil
		}

		return bytes.Repeat([]byte{' '}, Size)
	}

	return nil
}

// defaultTabWidth 는 tab 하나가 미는 칸 수의 기본값이다.
//
// 터미널 기본 tab stop 은 8 이지만, 8 칸은 깊게 들여쓴 코드를 화면 밖으로 밀어낸다.
//
// **줄을 재는 함수들은 이 값을 모른다.** 그것들은 tab 폭을 늘 인자로 받는다(cluster.go).
// 기본값을 고르는 것은 「적힌 것이 없을 때 무엇으로 볼까」라는 `.editorconfig` 를 읽는
// 자리의 판단이다 (ADR-0096).
const DefaultTabWidth = 4

// resolveTabWidth 는 tab 하나가 미는 화면 칸 수다. 적힌 것이 없으면 defaultTabWidth 다.
//
// **부르는 자리는 buffer 를 짓는 자리와 이름이 붙는 자리 둘뿐이다**(buffer.go, buffer-save.go).
// indentUnit 처럼 게으르게 정하지 않는다 — 이 값을 묻는 자리가 화면을 다시 그릴 때마다
// 줄마다 도는 wrapOffsets 이고, 그 자리의 `Buffer` 메서드는 값 receiver 라 게으른 캐시가
// 사본에만 남는다. 그러면 스크롤 한 번에 `.editorconfig` 를 줄 수만큼 찾아 올라간다.
// `language` 와 같은 손이다(ADR-0080, ADR-0096).
//
// **`indent_size` 만 적힌 파일도 그 값을 받는다.** editorconfig 명세가 「`tab_width` 의
// 기본값은 `indent_size`」라고 정해 두었고 라이브러리가 그 자리에서 채워 준다
// (definition.go 의 `tab_width defaults to indent_size`). 그것을 되돌리지 않는다 —
// 명세대로 읽는 쪽이 다른 편집기와 같은 화면을 낸다 (ADR-0096).
//
// 1 보다 작으면 기본값이다. 0 을 그대로 넘기면 glyphAt 의 나머지 연산이 죽는다.
//
// **위로는 상한을 두지 않는다.** `tab_width = 200` 이면 tab 하나가 화면을 넘는데, 적어 둔
// 사람의 뜻이 그렇다면 그대로 보이는 것이 맞다. 상한을 두면 적힌 것과 보이는 것이 갈리고
// 그것이 이 값을 읽기로 한 까닭을 지운다. 줄바꿈은 그런 폭에서도 버틴다(`col > 0`).
func resolveTabWidth(Path string) int {
	def := editorconfigFor(Path)
	if def == nil || def.TabWidth < 1 {
		return DefaultTabWidth
	}

	return def.TabWidth
}

// measureIndentUnitLimit 은 재려고 보는 줄 수다. 파일이 커도 앞쪽만 보면 답이 같고, 여는
// 순간이 아니라 첫 Enter 에서 도는 셈이라 길어지면 손에 걸린다.
const measureIndentUnitLimit = 500

// measureIndentUnit 은 파일이 이미 쓰고 있는 한 단계를 되짚는다. 알 수 없으면 nil 이다.
//
// tab 은 세기만 하면 된다 — tab 으로 들여쓴 줄이 하나라도 더 많으면 그 파일은 tab 이다.
// space 는 칸 수를 알아야 해서 **이웃한 두 줄의 들여쓰기 차이**를 본다. 들여쓰기의 절대값을
// 세면 깊이 들어간 줄이 답을 흐린다 — space 네 칸 파일에서 세 단계 들어간 줄은 12 다.
func measureIndentUnit(Lines [][]byte) []byte {
	tabs, spaces := 0, 0
	steps := map[int]int{}

	seen, prev := 0, -1
	for _, Line := range Lines {
		if seen >= measureIndentUnitLimit {
			break
		}

		blank := leadingBlank(Line)
		if len(blank) == len(Line) {
			// 빈 줄과 공백뿐인 줄은 들여쓰기가 아니다. 이어짐도 끊지 않는다.
			continue
		}

		if len(blank) > 0 {
			seen++
			if blank[0] == '\t' {
				tabs++
			} else {
				spaces++
			}
		}

		if prev >= 0 && len(blank) > prev && blank[0] != '\t' {
			steps[len(blank)-prev]++
		}
		prev = len(blank)
	}

	if tabs < 1 && spaces < 1 {
		return nil
	}
	if tabs >= spaces {
		return []byte{'\t'}
	}

	// 가장 흔한 걸음이다. 같은 수면 좁은 쪽을 고른다 — 넓게 잡아 어긋나는 것보다 낫다.
	best, count := 0, 0
	for step, n := range steps {
		if n > count || (n == count && step < best) {
			best, count = step, n
		}
	}
	if best < 1 {
		return nil
	}

	return bytes.Repeat([]byte{' '}, best)
}

// appendIndentLevel 은 base 에서 level 단계만큼 들이거나 내고 prefix 를 붙인다.
//
// **한 번에 한 단계뿐이다.** 한 줄에서 두 겹을 열어도 한 단계다 — 겹을 세어 따라가려면 참
// 파서가 있어야 하고, 어긋났을 때 손으로 되돌리는 값이 얻는 것보다 크다.
func appendIndentLevel(base, unit []byte, level int, prefix []byte, tab int) []byte {
	next := shiftBlank(base, unit, level*blankColumns(unit, tab), tab)

	if len(prefix) < 1 {
		return next
	}

	return concat(next, prefix)
}

// blankColumns 는 공백이 화면에서 몇 칸인지다. tab 은 다음 tab stop 까지라 byte 수와 다르다.
//
// **화면 칸으로 세는 것이 이 파일의 기준이다.** 들여쓰기를 옮기는 모든 자리(`tab` 키,
// `>` `<`, 닫는 표시를 칠 때, 새 줄) 가 이것을 쓴다. byte 로 세면 space 로 들여쓴 줄에
// `<` 를 칠 때 한 단계가 아니라 한 칸만 떨어진다.
//
// 재는 자리가 **화면**인 것도 일부러다. 눈에 보이는 것과 움직이는 것이 어긋나면 안 된다.
// 그래서 tab 은 부르는 쪽이 그 파일을 실제로 그리는 폭을 넘긴다(`buf.tabWidth()`).
// 그 폭이 곧 `.editorconfig` 의 `tab_width` 가 되면서 둘이 갈릴 자리가 없어졌다 (ADR-0096).
func blankColumns(blank []byte, tab int) int {
	return ScreenColAt(blank, len(blank), tab)
}

// shiftBlank 는 들여쓰기를 칸으로 재서 by 칸만큼 늘리거나 줄인 것이다.
//
// **채우는 글자는 그 줄이 이미 쓰던 것이다** — tab 으로 시작했으면 tab, 아니면 space.
// 들여쓰기가 없던 줄만 unit 을 따른다. 칸 수는 맞추되 글자는 두는 것이라, tab 파일에 섞여
// 있는 space 들여쓰기가 `>` 한 번에 조용히 tab 으로 바뀌지 않는다.
//
// by 가 0 이면 손대지 않는다. 다시 지으면 `  \t` 처럼 섞인 것이 뜻 없이 바뀐다.
func shiftBlank(blank, unit []byte, by, tab int) []byte {
	if by == 0 {
		return blank
	}

	useTab := len(unit) > 0 && unit[0] == '\t'
	if len(blank) > 0 {
		useTab = blank[0] == '\t'
	}

	return makeBlank(max(blankColumns(blank, tab)+by, 0), useTab, tab)
}

// makeBlank 는 cols 칸짜리 공백이다.
// tab 으로 채울 때 남는 칸은 space 다 — tab 은 칸 경계까지만 미는 글자라 그 아래를 못 만든다.
//
// tab 몇 개로 나눌지가 그 파일의 폭에 달려 있다. `tab_width = 8` 인 파일에서 8 칸은 tab
// 하나이고 4 칸짜리 파일에서는 둘이다.
func makeBlank(cols int, useTab bool, tab int) []byte {
	if !useTab {
		return bytes.Repeat([]byte{' '}, cols)
	}

	return append(
		bytes.Repeat([]byte{'\t'}, cols/tab),
		bytes.Repeat([]byte{' '}, cols%tab)...,
	)
}

// leadingBlank 는 줄 앞의 공백과 tab 이다.
//
// 줄은 제자리에서 바뀌지 않으므로(ADR-0001) 잘라낸 조각을 그대로 새 줄로 써도 된다.
func leadingBlank(Line []byte) []byte {
	Col := 0
	for Col < len(Line) && (Line[Col] == ' ' || Line[Col] == '\t') {
		Col++
	}

	return Line[:Col]
}
