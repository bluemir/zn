package textarea

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/bluemir/zn/internal/scheme"
)

// 짝 찾기다. 괄호 셋과 html 의 tag 쌍이다(ADR-0132).

// matchAt 은 그 자리에서 `%` 가 갈 곳이다. 짝이 없으면 빈 문자열이다.
// 자리는 `줄:byte offset` 이고 둘 다 0 부터다.
//
// 앞으로 가는 짝에서는 **짝의 마지막 글자**다. tag 에서만 `<` 와 갈린다(MatchPair).
func matchAt(buf Viewport, line, col int) string {
	target, _, ok := buf.MatchPair(scheme.Cursor{Line: line, Col: col})
	if !ok {
		return ""
	}

	return fmt.Sprintf("%d:%d", target.Line, target.Col)
}

func TestMatchBracket(t *testing.T) {
	tests := []struct {
		name string
		data string
		line int
		col  int
		want string
	}{
		{name: "여는 괄호에서 닫는 괄호로", data: "f(a)\n", line: 0, col: 1, want: "0:3"},
		{name: "닫는 괄호에서 여는 괄호로", data: "f(a)\n", line: 0, col: 3, want: "0:1"},
		{name: "겹을 센다", data: "f(g(a), b)\n", line: 0, col: 1, want: "0:9"},
		{name: "안쪽 짝은 안쪽끼리", data: "f(g(a), b)\n", line: 0, col: 3, want: "0:5"},
		{name: "대괄호", data: "a[0]\n", line: 0, col: 1, want: "0:3"},
		{name: "중괄호", data: "{a}\n", line: 0, col: 0, want: "0:2"},

		// 줄을 넘는다. 빈 줄도 건너뛴다.
		{name: "여러 줄", data: "func f() {\n\tx := 1\n}\n", line: 0, col: 9, want: "2:0"},
		{name: "빈 줄을 건너뛴다", data: "{\n\n}\n", line: 0, col: 0, want: "2:0"},

		// 짝이 없는 자리들이다.
		{name: "닫히지 않은 괄호", data: "f(a\n", line: 0, col: 1, want: ""},
		{name: "괄호가 아닌 글자", data: "f(a)\n", line: 0, col: 0, want: ""},
		{name: "빈 줄", data: "\n(a)\n", line: 0, col: 0, want: ""},

		// **문자열·주석 안의 괄호도 센다.** vim 의 기본과 같다(이 파일 머리글).
		// 문자열 안의 `)` 가 짝으로 잡혀서 진짜 짝인 줄 끝의 `)` 까지 가지 않는다.
		{name: "문자열 안의 괄호를 센다", data: `f(")")` + "\n", line: 0, col: 1, want: "0:3"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buf := NewBuffer("a.go", []byte(tt.data))

			assert.Equal(t, tt.want, matchAt(buf, tt.line, tt.col))
		})
	}
}

// 범위는 양쪽을 다 담는다. `d%` 가 여는 괄호와 닫는 괄호를 함께 지운다.
func TestMatchPairAreaHoldsBothSides(t *testing.T) {
	buf := NewBuffer("a.go", []byte("f(a)\n"))

	_, area, ok := buf.MatchPair(scheme.Cursor{Line: 0, Col: 1})
	assert.True(t, ok)
	assert.Equal(t, scheme.Cursor{Line: 0, Col: 1}, area.Start)
	assert.Equal(t, scheme.Cursor{Line: 0, Col: 4}, area.End, "닫는 괄호를 담는다")
	assert.False(t, area.Linewise)

	// 뒤로 가는 짝도 같은 범위다. 시작과 끝이 자리로 정해지지 커서로 정해지지 않는다.
	_, back, ok := buf.MatchPair(scheme.Cursor{Line: 0, Col: 3})
	assert.True(t, ok)
	assert.Equal(t, area, back)
}

// tag 쌍은 tag 가 짝을 이루는 언어에서만 본다.
func TestMatchTag(t *testing.T) {
	tests := []struct {
		name string
		data string
		line int
		col  int
		want string
	}{
		{name: "여는 tag 에서 닫는 tag 로", data: "<div>a</div>\n", line: 0, col: 0, want: "0:11"},
		{name: "닫는 tag 에서 여는 tag 로", data: "<div>a</div>\n", line: 0, col: 6, want: "0:0"},

		// `<` 부터 `>` 까지 안 아무 곳이면 그 tag 다.
		{name: "tag 이름 위에서도", data: "<div>a</div>\n", line: 0, col: 2, want: "0:11"},
		{name: "속성 위에서도", data: `<div class="a">b</div>` + "\n", line: 0, col: 8, want: "0:21"},
		{name: "닫는 `>` 위에서도", data: "<div>a</div>\n", line: 0, col: 4, want: "0:11"},

		// 겹을 센다. 이름이 같은 tag 만 센다.
		{name: "같은 이름이 겹치면 겹을 센다", data: "<div><div>a</div></div>\n", line: 0, col: 0, want: "0:22"},
		{name: "다른 이름은 겹이 아니다", data: "<div><p>a</p></div>\n", line: 0, col: 0, want: "0:18"},

		// 줄을 넘는다.
		{name: "여러 줄", data: "<div>\n  <p>a</p>\n</div>\n", line: 0, col: 0, want: "2:5"},
		{name: "속성이 줄에 걸친 tag", data: "<div\n  class=\"a\">\nb</div>\n", line: 0, col: 1, want: "2:6"},

		// 짝이 없는 것들이다.
		{name: "void tag 는 짝이 없다", data: "<div><br></div>\n", line: 0, col: 6, want: ""},
		{name: "스스로 닫은 tag 도", data: "<div><img /></div>\n", line: 0, col: 6, want: ""},
		{name: "tag 밖의 글", data: "<div>a</div>\n", line: 0, col: 5, want: ""},
		{name: "닫히지 않은 tag", data: "<div>a\n", line: 0, col: 0, want: ""},
		{name: "선언은 tag 가 아니다", data: "<!DOCTYPE html>\n", line: 0, col: 2, want: ""},

		// 속성 값 안의 `>` 로 tag 가 끝나지 않는다.
		{name: "따옴표 안의 `>`", data: `<div title="a > b">c</div>` + "\n", line: 0, col: 1, want: "0:25"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buf := NewBuffer("a.html", []byte(tt.data))

			assert.Equal(t, tt.want, matchAt(buf, tt.line, tt.col))
		})
	}
}

// tag 안의 괄호 위에서는 괄호가 먼저다.
func TestBracketWinsInsideTag(t *testing.T) {
	buf := NewBuffer("a.html", []byte(`<div onclick="f()">a</div>`+"\n"))

	assert.Equal(t, "0:16", matchAt(buf, 0, 15), "`(` 의 짝은 `)` 다")
	assert.Equal(t, "0:25", matchAt(buf, 0, 1), "이름 위에서는 tag 쌍이다")
}

// tag 쌍은 html 에서만이다. 다른 언어에서 `<div>` 는 그냥 글이다.
func TestTagPairsOnlyInHTML(t *testing.T) {
	buf := NewBuffer("a.go", []byte("<div>a</div>\n"))

	assert.Equal(t, "", matchAt(buf, 0, 1))
}
