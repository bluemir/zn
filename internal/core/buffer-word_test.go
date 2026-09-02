package core

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

// wordStops 는 이동 키를 계속 눌렀을 때 커서가 서는 자리를 차례로 모은다.
// 자리는 `줄:byte offset` 이고 둘 다 0 부터다. 더 움직이지 않으면 멈춘다.
func wordStops(buf viewport, move func(*viewport)) []string {
	stops := []string{}

	for range 20 {
		line, col := buf.cursor.line, buf.cursor.col
		move(&buf)

		if buf.cursor.line == line && buf.cursor.col == col {
			break
		}

		stops = append(stops, fmt.Sprintf("%d:%d", buf.cursor.line, buf.cursor.col))
	}

	return stops
}

func TestWordForward(t *testing.T) {
	tests := []struct {
		name string
		data string
		kind wordKind
		want []string
	}{
		{
			name: "공백으로 끊는다",
			data: "foo bar baz\n",
			kind: wordSmall,
			want: []string{"0:4", "0:8", "0:11"}, // 마지막 단어 뒤는 줄 끝이다. normal mode 는 clamp 로 마지막 글자에 선다
		},
		{
			// 문장부호는 단어 글자와 다른 부류라 따로 끊긴다. vim 과 같다.
			name: "문장부호가 부류를 가른다",
			data: "a.b(c)\n",
			kind: wordSmall,
			want: []string{"0:1", "0:2", "0:3", "0:4", "0:5", "0:6"},
		},
		{
			name: "큰 단어는 공백으로만 끊는다",
			data: "a.b(c) x\n",
			kind: wordBig,
			want: []string{"0:7", "0:8"},
		},
		{
			// `_` 와 숫자는 단어 글자다. camelCase 도 한 단어다(ADR-0006).
			name: "snake_case 와 camelCase 는 한 단어",
			data: "foo_bar fooBar99 x\n",
			kind: wordSmall,
			want: []string{"0:8", "0:17", "0:18"},
		},
		{
			name: "줄을 넘는다",
			data: "ab\ncd\n",
			kind: wordSmall,
			want: []string{"1:0", "1:2"},
		},
		{
			name: "빈 줄은 그 자체로 단어다",
			data: "ab\n\ncd\n",
			kind: wordSmall,
			want: []string{"1:0", "2:0", "2:2"},
		},
		{
			// 한글은 단어 글자와 다른 부류다(ADR-0006). `한글abc` 가 두 단어다.
			name: "한글과 영문이 갈린다",
			data: "한글abc 123나라\n",
			kind: wordSmall,
			want: []string{"0:6", "0:10", "0:13", "0:19"},
		},
		{
			name: "큰 단어는 한글도 안 가른다",
			data: "한글abc 123나라\n",
			kind: wordBig,
			want: []string{"0:10", "0:19"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			buf := newBuffer("test.txt", []byte(test.data))

			got := wordStops(buf, func(b *viewport) { b.moveWordForward(1, test.kind, wide) })

			assert.Equal(t, test.want, got)
		})
	}
}

func TestWordEnd(t *testing.T) {
	tests := []struct {
		name string
		data string
		kind wordKind
		want []string
	}{
		{
			name: "단어의 마지막 글자에 선다",
			data: "foo bar\n",
			kind: wordSmall,
			want: []string{"0:2", "0:6"},
		},
		{
			name: "문장부호도 한 덩어리다",
			data: "a.. b\n",
			kind: wordSmall,
			want: []string{"0:2", "0:4"},
		},
		{
			name: "줄을 넘는다",
			data: "ab\ncd\n",
			kind: wordSmall,
			want: []string{"0:1", "1:1"},
		},
		{
			// e 는 끝낼 단어가 없는 빈 줄에서 멈추지 않는다. vim 과 같다.
			name: "빈 줄에서 멈추지 않는다",
			data: "ab\n\ncd\n",
			kind: wordSmall,
			want: []string{"0:1", "2:1"},
		},
		{
			name: "큰 단어",
			data: "a.b(c) xy\n",
			kind: wordBig,
			want: []string{"0:5", "0:8"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			buf := newBuffer("test.txt", []byte(test.data))

			got := wordStops(buf, func(b *viewport) { b.moveWordEnd(1, test.kind, wide) })

			assert.Equal(t, test.want, got)
		})
	}
}

func TestWordBackward(t *testing.T) {
	tests := []struct {
		name  string
		data  string
		kind  wordKind
		start int // 시작 offset. 마지막 줄에서 시작한다
		want  []string
	}{
		{
			name:  "단어의 첫 글자로 되돌아간다",
			data:  "foo bar baz\n",
			kind:  wordSmall,
			start: 10,
			want:  []string{"0:8", "0:4", "0:0"},
		},
		{
			name:  "단어 중간에서는 그 단어의 처음으로",
			data:  "foo bar\n",
			kind:  wordSmall,
			start: 5,
			want:  []string{"0:4", "0:0"},
		},
		{
			name:  "문장부호가 부류를 가른다",
			data:  "a.b\n",
			kind:  wordSmall,
			start: 2,
			want:  []string{"0:1", "0:0"},
		},
		{
			name:  "큰 단어는 공백으로만 끊는다",
			data:  "a.b c\n",
			kind:  wordBig,
			start: 4,
			want:  []string{"0:0"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			buf := newBuffer("test.txt", []byte(test.data))
			buf.cursor.col = test.start

			got := wordStops(buf, func(b *viewport) { b.moveWordBackward(1, test.kind, wide) })

			assert.Equal(t, test.want, got)
		})
	}
}

// 줄을 거꾸로 넘고, 빈 줄에서는 멈춘다.
func TestWordBackwardCrossesLines(t *testing.T) {
	buf := newBuffer("test.txt", []byte("ab\n\ncd\n"))
	buf.cursor.line, buf.cursor.col = 2, 1

	got := wordStops(buf, func(b *viewport) { b.moveWordBackward(1, wordSmall, wide) })

	assert.Equal(t, []string{"2:0", "1:0", "0:0"}, got)
}

// count 는 그만큼 되풀이한다. 파일 양끝에서 멈춘다.
func TestWordMoveCounted(t *testing.T) {
	buf := newBuffer("test.txt", []byte("one two three four\n"))

	buf.moveWordForward(2, wordSmall, wide)
	assert.Equal(t, 8, buf.cursor.col, "2w")

	buf.moveWordBackward(2, wordSmall, wide)
	assert.Equal(t, 0, buf.cursor.col, "2b")

	buf.moveWordForward(99, wordSmall, wide)
	assert.Equal(t, 18, buf.cursor.col, "파일 끝에서 멈춘다")
}

// 위아래로 움직일 때 유지할 칸도 같이 갱신된다. 가로로 움직인 것이기 때문이다.
func TestWordMoveUpdatesDesiredCol(t *testing.T) {
	buf := newBuffer("test.txt", []byte("one two\nx\nabcdefg\n"))

	buf.moveWordForward(1, wordSmall, wide)
	assert.Equal(t, 4, buf.cursor.col)

	buf.moveDownLine(2)
	assert.Equal(t, 4, buf.cursor.col, "단어 이동 뒤에도 칸이 유지된다")
}
