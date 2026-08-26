package core

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
	path string
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
func resolveIndentUnit(path string, lang *syntax.Language, lines [][]byte) []byte {
	if unit := editorconfigUnit(path); unit != nil {
		return unit
	}

	if unit := measureIndentUnit(lines); unit != nil {
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
func editorconfigUnit(path string) []byte {
	def := editorconfigFor(path)
	if def == nil {
		return nil
	}

	switch def.IndentStyle {
	case "tab":
		return []byte{'\t'}
	case "space":
		size, err := strconv.Atoi(def.IndentSize)
		if err != nil || size < 1 {
			// `indent_size = tab` 이면 tab_width 가 칸 수다. 라이브러리가 이미 풀어 준다.
			size = def.TabWidth
		}
		if size < 1 {
			return nil
		}

		return bytes.Repeat([]byte{' '}, size)
	}

	return nil
}

// measureIndentUnitLimit 은 재려고 보는 줄 수다. 파일이 커도 앞쪽만 보면 답이 같고, 여는
// 순간이 아니라 첫 Enter 에서 도는 셈이라 길어지면 손에 걸린다.
const measureIndentUnitLimit = 500

// measureIndentUnit 은 파일이 이미 쓰고 있는 한 단계를 되짚는다. 알 수 없으면 nil 이다.
//
// tab 은 세기만 하면 된다 — tab 으로 들여쓴 줄이 하나라도 더 많으면 그 파일은 tab 이다.
// space 는 칸 수를 알아야 해서 **이웃한 두 줄의 들여쓰기 차이**를 본다. 들여쓰기의 절대값을
// 세면 깊이 들어간 줄이 답을 흐린다 — space 네 칸 파일에서 세 단계 들어간 줄은 12 다.
func measureIndentUnit(lines [][]byte) []byte {
	tabs, spaces := 0, 0
	steps := map[int]int{}

	seen, prev := 0, -1
	for _, line := range lines {
		if seen >= measureIndentUnitLimit {
			break
		}

		blank := leadingBlank(line)
		if len(blank) == len(line) {
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
func appendIndentLevel(base, unit []byte, level int, prefix []byte) []byte {
	next := shiftBlank(base, unit, level*blankColumns(unit))

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
// 재는 자리가 **화면**인 것도 일부러다. `.editorconfig` 의 `tab_width` 가 아니라 zn 이
// 실제로 그리는 폭(`tabWidth`) 을 쓴다 — 눈에 보이는 것과 움직이는 것이 어긋나면 안 된다.
func blankColumns(blank []byte) int {
	return screenColAt(blank, len(blank))
}

// shiftBlank 는 들여쓰기를 칸으로 재서 by 칸만큼 늘리거나 줄인 것이다.
//
// **채우는 글자는 그 줄이 이미 쓰던 것이다** — tab 으로 시작했으면 tab, 아니면 space.
// 들여쓰기가 없던 줄만 unit 을 따른다. 칸 수는 맞추되 글자는 두는 것이라, tab 파일에 섞여
// 있는 space 들여쓰기가 `>` 한 번에 조용히 tab 으로 바뀌지 않는다.
//
// by 가 0 이면 손대지 않는다. 다시 지으면 `  \t` 처럼 섞인 것이 뜻 없이 바뀐다.
func shiftBlank(blank, unit []byte, by int) []byte {
	if by == 0 {
		return blank
	}

	useTab := len(unit) > 0 && unit[0] == '\t'
	if len(blank) > 0 {
		useTab = blank[0] == '\t'
	}

	return makeBlank(max(blankColumns(blank)+by, 0), useTab)
}

// makeBlank 는 cols 칸짜리 공백이다.
// tab 으로 채울 때 남는 칸은 space 다 — tab 은 칸 경계까지만 미는 글자라 그 아래를 못 만든다.
func makeBlank(cols int, useTab bool) []byte {
	if !useTab {
		return bytes.Repeat([]byte{' '}, cols)
	}

	return append(
		bytes.Repeat([]byte{'\t'}, cols/tabWidth),
		bytes.Repeat([]byte{' '}, cols%tabWidth)...,
	)
}

// leadingBlank 는 줄 앞의 공백과 tab 이다.
//
// 줄은 제자리에서 바뀌지 않으므로(ADR-0001) 잘라낸 조각을 그대로 새 줄로 써도 된다.
func leadingBlank(line []byte) []byte {
	col := 0
	for col < len(line) && (line[col] == ' ' || line[col] == '\t') {
		col++
	}

	return line[:col]
}
