package core

import "github.com/bluemir/zn/internal/syntax"

// syntaxCache 는 줄마다의 문법 토큰과 그 줄을 끝낸 문맥이다.
//
// 줄 하나만 따로 훑을 수 없다. 여러 줄에 걸친 문자열이나 주석 때문에 어떤 줄의 색은 그 앞
// 줄들이 정한다. 그래서 토큰과 함께 **줄을 끝낸 문맥**(after) 을 들고 있다가, 다시 훑을 때
// 어떤 줄의 after 가 전과 같아지면 그 아래는 앞과 같은 문맥에서 시작하므로 담아둔 것이
// 그대로 맞다 — 거기서 멈춘다. VSCode 가 쓰는 방식이다.
type syntaxCache struct {
	// start 는 첫 줄을 시작하는 문맥이다. nil 이면 강조하지 않는 파일이다.
	//
	// **buf.language 에서 파생된 값이다.** 언어를 이름에서 미리 골라 두므로(buffer.go) 이것을
	// 고르는 일이 필드 하나 읽기가 되었고, 그래서 「무엇을 보고 골랐는지」를 담아 둘 값이
	// 없다. 담아둔 줄들을 버리는 것은 이름이 바뀌는 그 한 자리가 한다 (ADR-0080).
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
		*cache = syntaxCache{start: cache.start}
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
