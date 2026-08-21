package core

import (
	"github.com/bluemir/zn/internal/syntax"
)

// syntaxCache 는 줄마다의 문법 토큰과 그 줄을 끝낸 문맥이다.
//
// 줄 하나만 따로 훑을 수 없다. 여러 줄에 걸친 문자열이나 주석 때문에 어떤 줄의 색은 그 앞
// 줄들이 정한다. 그래서 토큰과 함께 **줄을 끝낸 문맥**(after) 을 들고 있다가, 다시 훑을 때
// 어떤 줄의 after 가 전과 같아지면 그 아래는 앞과 같은 문맥에서 시작하므로 담아둔 것이
// 그대로 맞다 — 거기서 멈춘다. VSCode 가 쓰는 방식이다.
type syntaxCache struct {
	// path 는 start 를 고를 때 본 경로다. 이름 없이 열었다가 `:w foo.go` 로 이름이 붙는 길이
	// 있어서(buffer.go 의 SaveTo) 경로가 달라지면 언어부터 다시 고른다.
	// 이 비교 하나로 「아직 안 봤다」와 「강조하지 않는 파일이다」가 갈린다.
	path string

	// start 는 첫 줄을 시작하는 문맥이다. nil 이면 강조하지 않는 파일이다(syntax.Detect).
	start syntax.State

	// lines 는 Buffer.lines 와 index 가 같다. 줄이 늘거나 줄면 replaceLines 가 같이 맞춘다.
	lines []syntaxLine

	// valid 는 담아둔 것이 맞는 줄 수다. lines[:valid] 의 토큰을 그대로 쓸 수 있다.
	//
	// 화면 아래는 훑지 않고 남겨 두므로(lexSyntaxTo) 파일 끝까지 차 있지 않은 것이 보통이다.
	valid int

	// filled 는 한 번이라도 훑어서 after 가 뜻을 가지는 줄 수다.
	//
	// valid 와 따로 든다. 편집이 valid 를 끌어내려도 그 아래 담아둔 것은 여전히 견줄 값이 있고,
	// 문맥이 수렴하면 **그만큼을 되돌려 놓아야** 한다. 이것이 없으면 valid 가 고친 줄 바로
	// 뒤까지만 올라가서, 화면에 보이는 그 아래 줄들이 색을 잃는다.
	filled int

	// changedEnd 는 내용이 달라진 마지막 줄 다음이다.
	//
	// valid 와 따로 든다. **「아직 안 훑았다」와 「내용이 달라졌다」는 다른 사실이다.** 둘을
	// 한 값으로 묶으면 문맥이 수렴해도 멈출 수 없다 — 처음 훑을 때 파일 전체가 「달라진 것」이
	// 되어 버려서, 그 뒤 어떤 편집도 파일 끝까지 가기 전에는 수렴을 볼 수 없다.
	//
	// 그리고 이것 하나로는 부족하다. 한 번의 키가 떨어진 두 자리를 고치는 길이 있다 — `c` 는
	// 지우기와 넣기가 각각 replaceLines 를 부르고, 앞으로 들어올 `:%s` 는 파일 전체에
	// 흩뿌린다. 앞자리만 들면 첫 줄에서 수렴한 순간 멈춰서 아래쪽 고친 줄을 영영 안 본다.
	changedEnd int
}

// syntaxLine 은 줄 하나의 토큰과 그 줄을 끝낸 문맥이다.
// after 가 nil 이면 아직 훑지 않은 줄이다 — Lex 는 nil 을 돌려주지 않는다.
type syntaxLine struct {
	tokens []syntax.Token
	after  syntax.State
}

// replace 는 lines 의 [at, at+count) 가 with 개로 갈린 것을 캐시에 반영한다.
//
// 부르는 자리는 replaceLines 하나다(edit.go). 그것이 buf.lines 를 갈아끼우는 유일한 함수라,
// 앞으로 편집 경로가 늘어도 이 자리를 건너뛸 수 없다.
func (cache *syntaxCache) replace(at, count, with int) {
	// 아직 훑은 것이 없으면 맞출 것도 없다. 첫 그리기가 위에서부터 채운다.
	if len(cache.lines) < 1 {
		return
	}

	// 줄 수와 어긋났으면 믿을 수 없다. 비워 두면 다음 그리기가 통째로 다시 훑는다.
	if at < 0 || at+count > len(cache.lines) {
		*cache = syntaxCache{path: cache.path, start: cache.start}
		return
	}

	// 줄 수가 그대로면 자리를 옮길 것이 없다. 타이핑 한 번이 거의 다 이 길이고, 여기서
	// slice 를 새로 만들면 키 하나가 파일 크기에 비례한 복사가 된다(1 만 줄에서 0.1 ms).
	//
	// 갈아끼운 줄의 after 를 지우지 않는다. 그 값이 「이 줄이 전에는 이렇게 끝났다」라서
	// 수렴을 견주는 데 쓰인다 — 지우면 고친 줄에서 멈출 수 없고 화면 끝까지 훑는다.
	// 그 줄의 토큰은 valid 가 at 로 내려가서 아무도 읽지 않는다(syntaxTokens).
	if count != with {
		next := make([]syntaxLine, 0, len(cache.lines)-count+with)
		next = append(next, cache.lines[:at]...)
		next = append(next, make([]syntaxLine, with)...) // 새 줄은 아직 훑지 않은 줄이다
		next = append(next, cache.lines[at+count:]...)
		cache.lines = next

		// 앞서 잡아둔 자리들은 줄 수가 달라진 만큼 밀린다. 살아남은 줄의 after 가 그만큼
		// 옮겨 갔으므로 filled 도 같이 따라간다.
		if cache.changedEnd > at {
			cache.changedEnd += with - count
		}
		if cache.filled > at {
			cache.filled += with - count
		}
	}

	cache.valid = min(cache.valid, at)
	cache.changedEnd = max(cache.changedEnd, at+with)
}

// lexSyntaxTo 는 lastLine 까지의 토큰을 채운다. 그리는 쪽이 화면 맨 아래 줄을 알려준다.
//
// 줄 하나를 훑으려면 그 앞 줄을 끝낸 문맥이 필요해서, 담아둔 것이 없으면 위에서부터 내려온다.
func (buf *Buffer) lexSyntaxTo(lastLine int) {
	// 이름이 달라졌으면 언어부터 다시 고른다. 이름 없는 buffer 는 path 가 둘 다 빈 문자열이라
	// 고를 것이 없고 start 가 nil 로 남는다.
	if buf.syntax.path != buf.path {
		buf.syntax = syntaxCache{path: buf.path, start: syntax.Detect(buf.path)}
	}
	if buf.syntax.start == nil {
		return
	}

	// 줄 수와 어긋났으면 담아둔 것을 믿을 수 없다. 통째로 다시 훑는다. replaceLines 가 같이
	// 맞춰 주므로 여기 걸리는 것은 lines 를 다른 길로 바꾼 경우뿐이다 — 느려질 뿐 틀리지 않는다.
	if len(buf.syntax.lines) != len(buf.lines) {
		buf.syntax.lines = make([]syntaxLine, len(buf.lines))
		buf.syntax.valid, buf.syntax.filled, buf.syntax.changedEnd = 0, 0, 0
	}

	// 화면 아래까지 이미 차 있으면 할 일이 없다.
	if buf.syntax.valid > lastLine {
		return
	}

	state := buf.syntax.start
	if buf.syntax.valid > 0 {
		state = buf.syntax.lines[buf.syntax.valid-1].after
	}

	for i := buf.syntax.valid; i < len(buf.lines); i++ {
		before := buf.syntax.lines[i].after
		tokens, after := state.Lex(buf.lines[i])

		buf.syntax.lines[i] = syntaxLine{tokens: tokens, after: after}
		state = after

		// 고친 줄을 다 지난 뒤에, 이 줄을 끝낸 문맥이 전과 같으면 아래 줄들은 앞과 같은 문맥에서
		// 시작한다 — 앞서 훑어 둔 만큼은 담아둔 것이 그대로 맞다. 그래서 차 있던 자리까지
		// 되돌린다. **파일 끝까지가 아니다** — 훑지 않은 줄을 맞다고 하면 그 줄이 색 없이 그려진다.
		//
		// 아직 안 훑은 줄은 before 가 nil 이라 같아지지 않으므로, 처음 훑는 동안에는 걸리지 않는다.
		if i+1 >= buf.syntax.changedEnd && after == before {
			buf.syntax.valid = max(buf.syntax.filled, i+1)
			buf.syntax.changedEnd = 0

			return
		}

		// 화면 아래는 지금 볼 사람이 없다. 굴러 내려갈 때 이어서 훑는다.
		// changedEnd 는 그대로 둔다 — 화면 밖에 고친 줄이 남아 있을 수 있다.
		if i >= lastLine {
			buf.syntax.valid = i + 1
			buf.syntax.filled = max(buf.syntax.filled, buf.syntax.valid)

			return
		}
	}

	buf.syntax.valid, buf.syntax.filled, buf.syntax.changedEnd = len(buf.lines), len(buf.lines), 0
}

// syntaxTokens 는 그 줄에 담아둔 토큰이다.
// 강조하지 않는 파일이거나 아직 훑지 않은 줄이면 nil 이다.
func (buf Buffer) syntaxTokens(line int) []syntax.Token {
	// valid 이후는 아직 훑지 않은 줄이다. 화면 밖이라 그릴 사람이 없다.
	if line < 0 || line >= len(buf.syntax.lines) || line >= buf.syntax.valid {
		return nil
	}

	return buf.syntax.lines[line].tokens
}
