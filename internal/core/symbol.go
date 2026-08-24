package core

import (
	"context"
	"unicode"

	"golang.org/x/text/unicode/runenames"
	"golang.org/x/text/width"

	"github.com/bluemir/zn/internal/assets"
)

// symbolChunk 는 한 조각에 실어 보내는 글자 수다. 팔레트의 indexChunk 와 같은 뜻이다.
const symbolChunk = 2048

// lastCodePoint 는 유니코드의 마지막 자리다.
const lastCodePoint = 0x10FFFF

// indexSymbols 는 고를 수 있는 글자 전부를 백그라운드에서 모은다.
//
// 큐레이션한 것이 앞이고 훑어서 얻은 것이 뒤다. 빈 패턴일 때 filterPalette 가 순서를 그대로
// 주므로(palette.go), 열자마자 보이는 첫 화면이 손으로 적은 것들이다.
//
// runenames 는 낱개 조회만 주기 때문에 한 번 훑어서 목록을 만든다. 프로세스에 한 번뿐이다 —
// 유니코드 표는 도중에 바뀌지 않는다. 파일 목록이 열 때마다 다시 도는 것과 다른 점이다
// (ADR-0011, ADR-0056).
//
// **글자와 숫자는 뺀다.** `A`·`가`·`漢` 은 특수문자 입력창이 찾아줄 것이 아니고, 한글 음절과
// CJK 만 10 만 개가 넘어서 넣으면 목록이 그것으로 뒤덮인다. 큐레이션 표에 일부러 담은
// 그리스 문자와 원문자는 그 앞에서 이미 들어와 있어서 이 규칙에 걸리지 않는다.
//
// ctx 가 끊기면 그만둔다. 보내다 막히는 자리마다 빠져나오므로 취소한 뒤 아무도 받지 않는
// 채널에 goroutine 이 남지 않는다(ADR-0027).
func indexSymbols(ctx context.Context) <-chan jobProgress {
	ch := make(chan jobProgress)

	// send 는 조각 하나를 보낸다. 취소됐으면 false 를 주고 부르는 쪽이 그만둔다.
	send := func(progress jobProgress) bool {
		select {
		case ch <- progress:
			return true
		case <-ctx.Done():
			return false
		}
	}

	go func() {
		defer close(ch)

		symbols := make([]assets.Symbol, 0, len(assets.CuratedSymbols)+8192)
		symbols = append(symbols, assets.CuratedSymbols...)

		curated := make(map[string]bool, len(assets.CuratedSymbols))
		for _, entry := range assets.CuratedSymbols {
			curated[entry.Char] = true
		}

		for r := rune(0); r <= lastCodePoint; r++ {
			// 훑는 것이 이 함수에서 유일하게 오래 걸리는 자리다. 조각마다 보지 않고
			// 여기서 한 번씩 본다.
			if r%symbolChunk == 0 && ctx.Err() != nil {
				return
			}
			if !namedSymbol(r) {
				continue
			}

			char := string(r)
			if curated[char] {
				continue
			}

			symbols = append(symbols, assets.Symbol{Char: char, Name: runenames.Name(r)})
		}

		// 목록은 다 모은 뒤에 한 번에 붓는다. 다 세기 전에는 개수만 알린다 —
		// 어디까지 왔는지 모르는 채로 조금씩 보여주면 순서가 뒤에 뒤집힌다(palette.go).
		send(jobProgress{
			done:    len(symbols),
			total:   len(symbols),
			apply:   func(e *editor) { e.symbols = symbols },
			summary: formatCount(len(symbols)) + " 개",
		})
	}()

	return ch
}

// namedSymbol 은 그 자리가 목록에 들어갈 글자인지다.
//
// 이름이 없으면 아직 배정되지 않은 자리다. surrogate·private use 처럼 이름이 있어도 글자가
// 아닌 것은 unicode.C 가 걸러 준다.
func namedSymbol(r rune) bool {
	if unicode.Is(unicode.C, r) || unicode.IsLetter(r) || unicode.IsDigit(r) || unicode.IsSpace(r) {
		return false
	}

	return runenames.Name(r) != ""
}

// symbolWidth 는 그 글자가 격자에서 차지하는 칸 수다.
//
// screenWidthOf 를 그대로 쓰지 않는 것은 East Asian Width 가 Ambiguous 인 글자 때문이다.
// `→ ± × °` 가 그 갈래인데 폭 계산은 늘 한 칸으로 세고(clusterAt), 두 칸으로 그리는
// 터미널에서는 격자가 통째로 밀린다. 시작할 때 재둔 값이 그 자리를 메운다
// (ADR-0028, ADR-0056).
//
// 재지 못했으면(0) 두 칸으로 본다. 좁게 잡으면 행이 넘쳐서 그 아래가 통째로 밀리고,
// 넓게 잡으면 칸 사이가 조금 벌어질 뿐이다 — 틀리는 쪽을 고른다. 재지 못한 터미널이
// 테두리를 ASCII 로 내리는 것과 같은 태도다(ADR-0028).
func (e editor) symbolWidth(char string) int {
	properties, size := width.LookupString(char)
	if size != len(char) || properties.Kind() != width.EastAsianAmbiguous {
		return screenWidthOf(char)
	}

	if e.ambiguousWidth == 1 {
		return 1
	}

	return 2
}
