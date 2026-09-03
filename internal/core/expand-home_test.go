package core

import (
	"os"
	"os/user"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// 맨 앞의 `~` 만 홈으로 풀린다(ADR-0088).
func TestExpandHome(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	tests := []struct {
		name  string
		input string
		want  string
	}{
		{name: "`~` 하나는 홈 그 자체다", input: "~", want: home},
		{name: "홈 아래", input: "~/.zshrc", want: filepath.Join(home, ".zshrc")},
		{name: "홈 아래 깊은 자리", input: "~/src/zn/main.go", want: filepath.Join(home, "src/zn/main.go")},
		{name: "`~/` 뒤가 비면 홈이다", input: "~/", want: home},

		// 맨 앞이 아닌 `~` 는 그런 이름의 파일이다.
		{name: "가운데의 `~`", input: "docs/~backup", want: "docs/~backup"},
		{name: "상대 경로", input: "internal/core/editor.go", want: "internal/core/editor.go"},
		{name: "절대 경로", input: "/etc/hosts", want: "/etc/hosts"},
		{name: "빈 경로", input: "", want: ""},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := expandHome(test.input)
			require.NoError(t, err)
			assert.Equal(t, test.want, got)
		})
	}
}

// `~사용자` 는 그 사용자의 홈이다. 없는 사용자는 오류로 낸다(ADR-0088).
func TestExpandHomeOfUser(t *testing.T) {
	me, err := user.Current()
	require.NoError(t, err)

	// 지금 사용자를 이름으로 다시 찾을 수 없는 자리가 있다(cgo 없이 빌드한 darwin).
	// 그런 자리에서는 `~사용자` 를 시험할 길이 없어 건너뛴다.
	found, err := user.Lookup(me.Username)
	if err != nil {
		t.Skip("이 자리에서는 사용자를 이름으로 찾을 수 없다: " + err.Error())
	}

	// $HOME 을 다른 곳으로 돌려놔도 `~사용자` 는 passwd 의 홈을 본다.
	t.Setenv("HOME", t.TempDir())

	got, err := expandHome("~" + me.Username)
	require.NoError(t, err)
	assert.Equal(t, found.HomeDir, got)

	got, err = expandHome("~" + me.Username + "/src")
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(found.HomeDir, "src"), got)

	_, err = expandHome("~zn-no-such-user/src")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "사용자를 찾을 수 없습니다")
}

// `:tabnew ~/<파일>` 은 홈 아래의 그 파일을 새 tab 으로 연다(ADR-0088).
func TestTabnewExpandsHome(t *testing.T) {
	start, dir := newFilesEditor(t, "a.txt")

	home := t.TempDir()
	t.Setenv("HOME", home)
	require.NoError(t, os.WriteFile(filepath.Join(home, "b.txt"), []byte("bbb\n"), 0644))

	m := runCommand(start, "tabnew ~/b.txt")

	require.IsType(t, viewEditorNormal{}, m)
	assert.Len(t, m.(viewEditorNormal).buffers, 2, "새 tab 이 하나 늘어난다")
	assert.Equal(t, filepath.Join(home, "b.txt"), bufferOf(t, m).Path)
	assert.Equal(t, "bbb", string(bufferOf(t, m).Line(0)))

	// 열려 있던 tab 은 그대로다.
	assert.Equal(t, filepath.Join(dir, "a.txt"), m.(viewEditorNormal).buffers[0].Path)
}

// `:e ~/<파일>` 도 같은 자리를 지난다. tab 수는 그대로다.
func TestEditExpandsHome(t *testing.T) {
	start, _ := newFilesEditor(t, "a.txt")

	home := t.TempDir()
	t.Setenv("HOME", home)
	require.NoError(t, os.WriteFile(filepath.Join(home, "b.txt"), []byte("bbb\n"), 0644))

	m := runCommand(start, "e ~/b.txt")

	require.IsType(t, viewEditorNormal{}, m)
	assert.Len(t, m.(viewEditorNormal).buffers, 1, "tab 은 늘지 않는다")
	assert.Equal(t, filepath.Join(home, "b.txt"), bufferOf(t, m).Path)
}

// `:w ~/<파일>` 은 홈 아래에 사본을 쓴다. 보고 있는 파일과 tab 은 그대로다.
func TestWriteExpandsHome(t *testing.T) {
	start, dir := newFilesEditor(t, "a.txt")

	home := t.TempDir()
	t.Setenv("HOME", home)

	m := runCommand(start, "w ~/copy.txt")

	require.IsType(t, viewEditorNormal{}, m)
	assert.Equal(t, filepath.Join(dir, "a.txt"), bufferOf(t, m).Path, "보고 있는 파일은 그대로다")

	content, err := os.ReadFile(filepath.Join(home, "copy.txt"))
	require.NoError(t, err)
	assert.Equal(t, "a.txt\n", string(content))
}
