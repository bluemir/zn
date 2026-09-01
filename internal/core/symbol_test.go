package core

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bluemir/zn/internal/assets"
)

// collectSymbols 는 훑기를 끝까지 돌려 목록을 받는다.
func collectSymbols(t *testing.T) []assets.Symbol {
	t.Helper()

	e := &editor{}
	for progress := range indexSymbols(t.Context()) {
		if progress.apply != nil {
			progress.apply(e)
		}
	}

	require.NotEmpty(t, e.symbols)

	return e.symbols
}

// 큐레이션 표가 앞이어야 한다. 빈 패턴일 때 filterPalette 가 순서를 그대로 주므로
// 열자마자 보이는 첫 화면이 손으로 적은 것들이다(palette.go).
func TestIndexSymbolsKeepsCuratedFirst(t *testing.T) {
	symbols := collectSymbols(t)

	require.Greater(t, len(symbols), len(assets.CuratedSymbols), "훑어서 더 모아야 한다")
	assert.Equal(t, assets.CuratedSymbols, symbols[:len(assets.CuratedSymbols)])
}

// 글자와 숫자는 뺀다. 넣으면 한글 음절과 CJK 만 10 만 개가 넘어 목록이 그것으로 뒤덮인다.
func TestIndexSymbolsSkipsLettersAndDigits(t *testing.T) {
	symbols := collectSymbols(t)

	seen := map[string]bool{}
	for _, entry := range symbols {
		seen[entry.Char] = true
	}

	for _, char := range []string{"A", "z", "가", "漢", "5", "ぁ"} {
		assert.False(t, seen[char], "%q 는 목록에 없어야 한다", char)
	}
}

// 큐레이션한 글자는 훑기가 덮어쓰지 않는다. 한국어 이름이 영문으로 바뀌면 안 된다.
func TestIndexSymbolsDoesNotDuplicateCurated(t *testing.T) {
	symbols := collectSymbols(t)

	count := map[string]int{}
	for _, entry := range symbols {
		count[entry.Char]++
	}

	for _, entry := range assets.CuratedSymbols {
		assert.Equal(t, 1, count[entry.Char], "%q 가 겹친다", entry.Char)
	}
}

// 표에 없는 글자는 영문 이름으로 들어온다.
func TestIndexSymbolsNamesScannedInEnglish(t *testing.T) {
	symbols := collectSymbols(t)

	names := map[string]string{}
	for _, entry := range symbols {
		names[entry.Char] = entry.Name
	}

	assert.Equal(t, "FLATNESS", names["⏥"])
	assert.Equal(t, "APL FUNCTIONAL SYMBOL CIRCLE JOT", names["⌾"])
}

// ctx 를 끊으면 목록을 붓지 않고 물러난다.
func TestIndexSymbolsStopsWhenCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	e := &editor{}
	for progress := range indexSymbols(ctx) {
		if progress.apply != nil {
			progress.apply(e)
		}
	}

	assert.Empty(t, e.symbols)
}

// 큐레이션 표는 손으로 적는 것이라 겹치거나 비어 있기 쉽다. 그 자리를 여기서 막는다.
func TestCuratedSymbolsAreWellFormed(t *testing.T) {
	seen := map[string]bool{}

	for _, entry := range assets.CuratedSymbols {
		assert.NotEmpty(t, entry.Char, "글자가 비었다: %+v", entry)
		assert.NotEmpty(t, entry.Name, "%q 의 이름이 비었다", entry.Char)
		assert.False(t, seen[entry.Char], "%q 가 표에 두 번 있다", entry.Char)
		seen[entry.Char] = true

		// 격자가 한 칸에 한 글자를 놓는다. 두 글자가 들어오면 칸이 밀린다.
		assert.Equal(t, len(entry.Char), glyphSize([]byte(entry.Char), 0),
			"%q 는 grapheme cluster 하나가 아니다", entry.Char)

		// 보이지 않는 글자는 격자에서 빈 칸이라 서로 구별되지 않는다.
		// rune 하나하나가 아니라 글자가 통째로 차지하는 폭을 본다 — ZWJ 로 이은
		// 이모지(`👨‍💻`) 는 안에 폭 0 인 rune 이 있어도 한 글자로 보인다.
		assert.GreaterOrEqual(t, widthOf(entry.Char), 1,
			"%q 가 화면에서 보이지 않는다", entry.Name)
	}
}

// 큐레이션한 글자는 전부 격자 한 칸 안에 들어간다. 칸보다 넓으면 그 행이 통째로 밀린다.
//
// 폭은 화면의 나머지와 같은 자로 잰다. 예전에는 이 자리에만 Ambiguous 특례(symbolWidth) 가
// 있었는데, 눈금을 시작할 때 터미널에 맞추기로 하면서 없앴다(ADR-0072).
func TestCuratedSymbolsFitOneCell(t *testing.T) {
	for _, entry := range assets.CuratedSymbols {
		got := widthOf(entry.Char)

		assert.GreaterOrEqual(t, got, 1, "%q 의 폭이 0 이다", entry.Char)
		assert.LessOrEqual(t, got, symbolCellWidth, "%q 가 칸보다 넓다", entry.Char)
	}
}
