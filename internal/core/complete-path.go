package core

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// 명령줄에서 `tab` 으로 경로를 완성하는 자리다(ADR-0099).
//
// **후보는 그때그때 디렉터리를 읽어 만든다.** 팔레트의 파일 목록(`e.files`) 을 쓰지 않는데,
// 그것은 팔레트를 한 번이라도 열어야 채워지고 저장소 안만 담기 때문이다. 여기서 치는 것은
// 「지금 이 경로 다음에 무엇이 있나」라 `~`·절대 경로·저장소 밖도 되어야 한다.
//
// 갈래가 팔레트와 나뉜 것이기도 하다 — 저장소 안을 흐릿하게 찾는 것은 팔레트가 하고,
// 경로를 또박또박 짚어 가는 것은 여기다.

// completesPath 는 그 명령이 경로를 받는지다. `~` 를 푸는 자리와 같은 셋이다
// (view-editor-command.go 의 expandHome 호출).
func completesPath(name string) bool {
	switch strings.TrimSuffix(name, "!") {
	case "w", "e", "tabnew":
		return true
	default:
		return false
	}
}

// completePath 는 친 조각을 디렉터리에서 맞춰 본다.
//
// 돌려주는 것은 **조각을 대신할 글자**와 **후보 이름들**이다. 맞는 것이 없으면 조각을 그대로
// 주고 후보는 비어 있다.
//
//   - 하나면 통째로 채운다. 디렉터리면 뒤에 `/` 를 붙여서 이어 칠 수 있게 한다
//   - 여럿이면 **공통 앞부분까지만** 채우고 후보를 같이 준다. 셸·vim 의 기본 손이다
//
// 후보에는 디렉터리에 `/` 가 붙어 있다. 목록만 보고 더 들어갈 수 있는지 알아야 한다.
func completePath(fragment string) (string, []string) {
	dir, prefix := splitPathFragment(fragment)

	names, err := readDirNames(dir)
	if err != nil {
		return fragment, nil
	}

	// **숨김 파일은 앞글자가 `.` 일 때만 낸다.** 셸과 같은 손이다. 안 그러면 `:e ` 뒤에
	// tab 을 친 순간 `.git` 이 목록 맨 앞에 선다.
	matched := []string{}
	for _, name := range names {
		if !strings.HasPrefix(name, prefix) {
			continue
		}
		if strings.HasPrefix(name, ".") && !hiddenAllowed(prefix) {
			continue
		}

		matched = append(matched, name)
	}

	if len(matched) == 0 {
		return fragment, nil
	}

	// 공통 앞부분이 곧 채울 것이다. 하나뿐이면 그 이름 전체가 공통이라 통째로 채워진다.
	filled := dir + commonPrefix(matched)
	if len(matched) == 1 {
		return filled, nil
	}

	return filled, matched
}

// splitPathFragment 는 친 조각을 「디렉터리 자리」와 「맞춰 볼 앞글자」로 가른다.
//
// 디렉터리 자리는 **친 그대로 남긴다.** `~/pro` 의 `~` 를 풀어서 돌려주면 명령줄의 글자가
// 사람이 친 것과 달라진다 — 푸는 것은 실제로 여는 자리의 몫이다(expandHome, ADR-0088).
//
// 마지막 `/` 뒤가 앞글자다. `/` 가 없으면 디렉터리 자리는 비고 지금 자리에서 찾는다.
func splitPathFragment(fragment string) (dir, prefix string) {
	if slash := strings.LastIndexByte(fragment, '/'); slash >= 0 {
		return fragment[:slash+1], fragment[slash+1:]
	}

	return "", fragment
}

// readDirNames 는 그 디렉터리의 이름들이다. 디렉터리에는 `/` 를 붙인다.
//
// dir 이 비어 있으면 지금 자리다. `~` 는 여기서 푼다 — 읽는 것은 실제 경로여야 한다.
//
// 거르는 것은 부르는 쪽이다. 여기는 있는 것을 그대로 준다.
func readDirNames(dir string) ([]string, error) {
	full := dir
	if full == "" {
		full = "."
	}

	full, err := expandHome(full)
	if err != nil {
		return nil, err
	}

	entries, err := os.ReadDir(full)
	if err != nil {
		return nil, err
	}

	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		name := entry.Name()

		// IsDir 은 심볼릭 링크를 따라가지 않는다. 링크된 디렉터리에도 `/` 가 붙어야
		// 이어 칠 수 있으므로 한 번 더 본다.
		if entry.IsDir() || isDirLink(filepath.Join(full, name), entry) {
			name += "/"
		}

		names = append(names, name)
	}

	slices.Sort(names)

	return names, nil
}

// isDirLink 는 그 항목이 디렉터리를 가리키는 링크인지다. 링크가 아니면 거짓이다.
func isDirLink(full string, entry os.DirEntry) bool {
	if entry.Type()&os.ModeSymlink == 0 {
		return false
	}

	info, err := os.Stat(full)

	return err == nil && info.IsDir()
}

// hiddenAllowed 는 앞글자가 `.` 로 시작하는지다. 숨김 파일을 낼지가 이것으로 갈린다.
func hiddenAllowed(prefix string) bool {
	return strings.HasPrefix(prefix, ".")
}

// commonPrefix 는 이름들이 공통으로 가진 앞부분이다.
//
// **글자 단위로 센다.** byte 로 자르면 한글 파일 이름에서 글자 가운데가 잘려 깨진 글자가
// 명령줄에 들어간다.
func commonPrefix(names []string) string {
	if len(names) == 0 {
		return ""
	}

	common := names[0]
	for _, name := range names[1:] {
		common = common[:sharedLen(common, name)]
	}

	return common
}

// sharedLen 은 두 글이 앞에서부터 같은 byte 길이다. 글자 경계에서 끊는다.
func sharedLen(a, b string) int {
	line := []byte(a)

	at := 0
	for at < len(line) && at < len(b) {
		size := glyphSize(line, at)
		if at+size > len(b) || a[at:at+size] != b[at:at+size] {
			break
		}

		at += size
	}

	return at
}

// completeCommandLine 은 친 명령줄 한 줄을 완성한다. 바꾼 줄과 후보를 준다.
//
// **경로를 받는 명령의 마지막 조각만 본다.** 이름 자리를 완성하지 않는 것은 팔레트가 명령을
// 고르는 자리이기 때문이고(ADR-0011), 인자가 경로가 아닌 명령(`:grep`·`:s`) 은 뒤를 통째로
// 받는 정규식이라 경로로 맞추면 안 된다(ADR-0077, ADR-0084).
//
// 따옴표가 든 줄은 건드리지 않는다. 완성한 글에 빈 칸이 들면 따옴표를 새로 지어야 하는데,
// 이미 친 따옴표와 겹치는 자리를 만들지 않는다. 그때는 손으로 친다.
func completeCommandLine(input string) (string, []string) {
	name, rest, ok := strings.Cut(input, " ")
	if !ok || !completesPath(name) {
		return input, nil
	}

	if strings.ContainsAny(rest, "\"'\\") {
		return input, nil
	}

	// 마지막 빈 칸 뒤가 완성할 조각이다. `:e  foo` 처럼 사이가 벌어져 있어도 맞는다.
	head, fragment := input[:len(input)-len(lastField(rest))], lastField(rest)

	filled, candidates := completePath(fragment)

	return head + filled, candidates
}

// lastField 는 마지막 빈 칸 뒤의 글이다. 빈 칸으로 끝나면 빈 글이다.
func lastField(rest string) string {
	if space := strings.LastIndexByte(rest, ' '); space >= 0 {
		return rest[space+1:]
	}

	return rest
}
