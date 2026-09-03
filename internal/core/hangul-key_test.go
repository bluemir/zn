package core

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHangulKeys(t *testing.T) {
	tests := []struct {
		name string
		key  string
		want []string
	}{
		{name: "자모 하나", key: "ㅁ", want: []string{"a"}},
		{name: "모음 하나", key: "ㅓ", want: []string{"j"}},
		{name: "된소리는 shift 자리", key: "ㅉ", want: []string{"W"}},
		{name: "겹모음은 키 둘", key: "ㅘ", want: []string{"h", "k"}},
		{name: "겹받침도 키 둘", key: "ㄺ", want: []string{"f", "r"}},
		{name: "받침 없는 음절", key: "어", want: []string{"d", "j"}},
		{name: "받침 있는 음절", key: "얌", want: []string{"d", "i", "a"}},
		{name: "겹받침 음절", key: "값", want: []string{"r", "k", "q", "t"}},
		{name: "겹모음 음절", key: "화", want: []string{"g", "h", "k"}},
		{name: "구간 끝 음절", key: "힣", want: []string{"g", "l", "g"}},

		{name: "영문은 그대로", key: "a", want: nil},
		{name: "숫자는 그대로", key: "3", want: nil},
		{name: "이름으로 오는 키는 그대로", key: "ctrl+w", want: nil},

		// 여러 글자가 한 번에 올 수도 있다. 한글만 바꾸고 나머지는 그대로 둔다.
		{name: "섞여 있으면 한글만 바꾼다", key: "어b", want: []string{"d", "j", "b"}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assert.Equal(t, test.want, hangulKeys(test.key))
		})
	}
}

// 한글 상태에서 `ㅓ` 는 `j` 다. 자모 하나가 키 하나로 바뀌는 가장 단순한 경우다.
func TestNormalHangulJamoMoves(t *testing.T) {
	var m tea.Model = newTestEditor("one\ntwo\nthree\n", 40, 5)

	m = send(m, "ㅓ")

	assert.Equal(t, 1, bufferOf(t, m).Cursor.Line, "ㅓ 가 j 로 동작한다")
}

// 겹모음은 키 둘이 합쳐진 것이라 동작 둘이 된다. `ㅘ` 는 `h` 와 `k` 를 이어 누른 것이다.
func TestNormalHangulCompoundVowelRunsBothKeys(t *testing.T) {
	var m tea.Model = newTestEditor("one\ntwo\nthree\n", 40, 5)
	m = send(m, "j", "l", "l")
	require.Equal(t, 1, bufferOf(t, m).Cursor.Line)
	require.Equal(t, 2, bufferOf(t, m).Cursor.Col)

	m = send(m, "ㅘ")

	buf := bufferOf(t, m)
	assert.Equal(t, 0, buf.Cursor.Line, "ㅘ 의 k 가 위로 옮긴다")
	assert.Equal(t, 1, buf.Cursor.Col, "ㅘ 의 h 가 왼쪽으로 옮긴다")
}

// 조합된 음절도 자모로 되돌린다. `어` 는 `d` 와 `j` 라 `dj` 가 되어 두 줄이 지워진다.
// 음절 하나가 operator 와 motion 으로 갈리는 것이라, 한글 상태로 잘못 친 키 하나가
// 편집이 될 수 있다. 두벌식 자리로 되돌리는 이상 편집기가 가릴 수 없다(ADR-0008).
func TestNormalHangulSyllableRunsBothKeys(t *testing.T) {
	var m tea.Model = newTestEditor("one\ntwo\nthree\n", 40, 5)

	m = send(m, "어")

	assert.Equal(t, []string{"three"}, linesOf(bufferOf(t, m)), "어 가 dj 로 동작한다")
}

// 숫자 접두는 한글이 아니라 그대로 온다. 뒤에 오는 한글 키와 이어져야 한다.
func TestNormalHangulKeepsCount(t *testing.T) {
	var m tea.Model = newTestEditor("1\n2\n3\n4\n5\n", 40, 5)

	m = send(m, "3", "ㅓ")

	assert.Equal(t, 3, bufferOf(t, m).Cursor.Line, "3ㅓ 는 3j 다")
}

// 접두 키도 한글로 낼 수 있다. `ㅎㅎ` 는 `gg` 다.
func TestNormalHangulPrefixKey(t *testing.T) {
	var m tea.Model = newTestEditor("1\n2\n3\n4\n5\n", 40, 5)
	m = send(m, "G")
	require.Equal(t, 4, bufferOf(t, m).Cursor.Line)

	m = send(m, "ㅎ", "ㅎ")

	assert.Equal(t, 0, bufferOf(t, m).Cursor.Line, "ㅎㅎ 는 gg 로 첫 줄로 간다")
}

// mode 가 바뀌면 남은 키는 버린다.
//
// `마` 는 `a` `k` 다. `a` 에서 insert mode 로 들어가므로 남은 `k` 가 글자로 꽂히면 안 된다.
// 버리지 않으면 `okne` 가 된다 — 한글 상태로 normal mode 에 온 사고가 파일을 고치는 것이라
// ADR-0008 이 "남은 키를 버리는 쪽이 안전하다" 고 정한 자리다.
//
// `얌`(`d` `i` `a`) 으로는 이것을 재지 못한다. `d`+`i` 가 모르는 motion `d i` 가 되어 아무 일도
// 하지 않고, mode 를 바꾸는 `a` 가 마지막 키라 버릴 것이 남지 않는다.
func TestNormalHangulDropsKeysAfterModeChange(t *testing.T) {
	var m tea.Model = newTestEditor("one\n", 40, 5)

	m = send(m, "마")

	require.IsType(t, viewEditorInsert{}, m, "a 에서 insert mode 로 들어간다")
	assert.Equal(t, "one", string(bufferOf(t, m).Line(0)), "남은 k 는 버려진다")
}

// insert mode 는 한글을 글자로 받아야 한다. 여기서 자모를 키로 바꾸면 한글을 쓸 수 없다.
func TestInsertKeepsHangulAsText(t *testing.T) {
	var m tea.Model = newTestEditor("\n", 40, 5)

	m = send(m, "i", "ㅁ", "화")

	require.IsType(t, viewEditorInsert{}, m)
	assert.Equal(t, "ㅁ화", string(bufferOf(t, m).Line(0)))
}

// sidebar 도 같은 방식이다. `ㅓ` 가 `j` 로 트리를 내려간다.
func TestSidebarHangulMovesSelection(t *testing.T) {
	var m tea.Model = newTreeEditor(t, 80, 6)
	m = send(m, "ctrl+w", "ctrl+w")
	require.IsType(t, viewSidebar{}, m)

	m = send(m, "ㅓ", "ㅓ")

	assert.Equal(t, "docs", m.(viewSidebar).sidebar.selectedNode().name)
}

// 확인창의 y·n 도 한글 상태에서 동작한다. `ㅜ` 가 `n` 이다.
func TestQuitConfirmHangulSelectsNo(t *testing.T) {
	editor := newTestEditor("a\n", 40, 5)

	var confirm tea.Model = ConfirmDiscard(editor, editor.editor, "정말 종료 하시겠습니까?", Exit)
	confirm, _ = confirm.Update(key("ㅜ"))

	require.IsType(t, viewConfirmDiscard{}, confirm)
	assert.Equal(t, 1, confirm.(viewConfirmDiscard).cursor, "ㅜ 가 n 으로 No 를 고른다")
}
