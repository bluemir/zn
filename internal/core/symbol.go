package core

import (
	"context"
	"unicode"

	"golang.org/x/text/unicode/runenames"

	"github.com/bluemir/zn/internal/assets"
)

// symbolChunk 는 한 조각에 실어 보내는 글자 수다. 팔레트의 indexChunk 와 같은 뜻이다.
const symbolChunk = 2048

// lastCodePoint 는 유니코드의 마지막 자리다.
const lastCodePoint = 0x10FFFF

// 조합 기호가 사는 두 구간이다. 각 속성(`Emoji_Modifier`·`Regional_Indicator`) 이 담는
// 글자가 이 구간 그대로여서 표를 들 필요가 없다.
const (
	emojiModifierFirst = 0x1F3FB // 🏻 EMOJI MODIFIER FITZPATRICK TYPE-1-2
	emojiModifierLast  = 0x1F3FF // 🏿 EMOJI MODIFIER FITZPATRICK TYPE-6

	regionalIndicatorFirst = 0x1F1E6 // 🇦 REGIONAL INDICATOR SYMBOL LETTER A
	regionalIndicatorLast  = 0x1F1FF // 🇿 REGIONAL INDICATOR SYMBOL LETTER Z
)

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
//
// **spacing mark(Mc) 는 넣지 않는다.** 인도계 문자의 모음 기호들(`ि` `ा` `ी`) 452 개다.
// 폭을 맞출 길이 우리에게 없어서 격자에 세우면 화면이 깨진다 — 우리와 tmux 는 한 칸으로
// 세는데 Ghostty 는 두 칸으로 세고, 그것은 `mode 2027` 을 켜도 그대로다. 게다가 그 글자가
// 든 행은 렌더러가 마지막 칸을 빠뜨린다(docs/issues/0005).
//
// **조합 기호도 넣지 않는다.** 살색 기호(`🏻`~`🏿`) 는 앞 이모지의 살색을 바꾸는 것이고,
// 지역 표시(`🇦`~`🇿`) 는 둘이 모여 국기 하나(`🇰`+`🇷` = `🇰🇷`) 가 된다. 낱개로 넣을 자리가
// 없는데 폭까지 갈린다 — 살색 기호는 우리가 0 인데 터미널이 2 이고, 지역 표시는 우리가
// 2 인데 터미널이 1 이다(docs/issues/0006).
//
// 보이지 않는 글자를 큐레이션 표에서 뺀 것과 같은 규칙이다 — **낼 수 없는 글자는 목록에
// 세우지 않는다.** 폭 0 인 결합 문자(Mn·Me) 는 폭이 터미널과 맞으므로 `◌` 에 얹어 남긴다
// (view-symbol.go 의 symbolGlyph).
func namedSymbol(r rune) bool {
	if unicode.Is(unicode.C, r) || unicode.IsLetter(r) || unicode.IsDigit(r) || unicode.IsSpace(r) {
		return false
	}
	if unicode.Is(unicode.Mc, r) {
		return false
	}
	if r >= emojiModifierFirst && r <= emojiModifierLast {
		return false
	}
	if r >= regionalIndicatorFirst && r <= regionalIndicatorLast {
		return false
	}

	return runenames.Name(r) != ""
}
