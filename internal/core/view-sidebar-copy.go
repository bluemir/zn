package core

import (
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/cockroachdb/errors"

	"github.com/bluemir/zn/internal/textarea"
)

// copyPrompt 는 복제할 경로를 받는 동안 statusBar 아래 줄 앞에 서는 말이다.
// 메뉴의 「복제」 와 같은 말이라 어느 길로 들어와도 같은 이름으로 배운다.
const copyPrompt = "복제: "

// copyFileMode 는 복제할 경로를 받는 화면이다. 트리에서 `mc` 로 들어온다(ADR-0152).
//
// 이름 바꾸기(`mm`) 와 같이 **지금 경로가 뿌리 기준으로 채워져 있다.** 복제는 대개 옆에 이름만
// 조금 다른 것을 두는 일이라(`main.go` → `main_test.go`) 빈 칸에서 시작하면 지금 이름을 다시
// 치게 된다. 앞을 고치면 다른 디렉터리에 복제하는 것이 된다.
//
// 이름 바꾸기와 화면을 나눠 쓰지 않는다. 같은 것은 칸의 모양뿐이고, `enter` 에 하는 일과
// 거절하는 까닭이 다르다(ADR-0054).
func copyFileMode(e *editor, node *treeNode) (tea.Model, tea.Cmd) {
	return viewSidebarCopy{
		editor: e,
		from:   node.path,
		isDir:  node.isDir && !node.isSymlink,
		input:  newInputLine(e.sidebar.relLabel(node.path)),
	}, nil
}

type viewSidebarCopy struct {
	*editor

	from  string    // 복제할 것의 경로. 절대 경로다
	isDir bool      // 디렉터리인가. symlink 는 링크만 복제하므로 파일 쪽이다
	input inputLine // 치고 있는 경로. 뿌리 기준 상대 경로다
}

func (m viewSidebarCopy) Init() tea.Cmd { return nil }

func (m viewSidebarCopy) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.resize(msg)

		// 화면이 좁아져 트리가 숨으면 치던 것을 버리고 편집 영역으로 내보낸다.
		// 트리에 포커스를 두는 mode 는 모두 같은 규칙이다.
		if !m.sidebarVisible() {
			return normalMode(m.editor)
		}

		return m, nil
	case tea.KeyPressMsg:
		switch msg.String() {
		case "ctrl+c":
			return quitAll(m, m.editor)
		case "esc":
			return sidebarMode(m.editor)
		case "enter":
			// 앞뒤 빈 칸은 눌러 둔 자국이다. 가운데 빈 칸은 파일 이름에 쓸 수 있으므로 건드리지 않는다.
			return copyEntry(m.editor, m.from, m.isDir, strings.TrimSpace(m.input.text))
		case "left", "right", "home", "end":
			m.input.move(msg.String())

			return m, nil
		case "delete":
			m.input.deleteForward()

			return m, nil
		case "backspace":
			// 다 지워도 이 화면에 남는다. 채워져 있던 것을 지운 것이라 그만두려는 뜻으로 읽을 수
			// 없다. 이름 바꾸기와 같은 자리이고, 그만두는 것은 `esc` 다.
			m.input.deleteBackward()

			return m, nil
		default:
			// 이름이 될 수 없는 키(ctrl 조합) 는 Text 가 비어 있다. 방향키는 위에서 잡힌다.
			if msg.Text == "" {
				return m, nil
			}
			m.input.insert(msg.Text)

			return m, nil
		}
	case tea.MouseWheelMsg:
		// 치는 동안에도 화면은 둘러볼 수 있다. 클릭은 받지 않는다 —
		// 치던 것이 클릭 한 번에 조용히 사라지면 안 된다. 명령줄과 같은 규칙이다.
		m.wheel(msg.Mouse())

		return m, nil
	case jobProgressMsg, jobDoneMsg, gitTickMsg, fileTickMsg, editTickMsg, watchMsg, serverReadyMsg, definitionMsg, referencesMsg, renameMsg, diagnosticsMsg, semanticTokensMsg:
		// 백그라운드 작업의 진행도 주기 tick 도 mode 와 무관하다. 공용 처리가 statusBar 에
		// 반영하고 다음 조각과 다음 tick 을 받을 Cmd 를 준다(job.go).
		// model 이 오면 mode 가 바뀐 것이다. 오지 않으면 지금 mode 를 그대로 쓴다(job.go).
		next, cmd := m.handleJob(msg)
		if next != nil {
			return next, cmd
		}

		return m, cmd
	default:
		return m, nil
	}
}

// copyEntry 는 from 을 뿌리 기준 경로 name 에 복제한다. 디렉터리면 안의 것까지 통째다.
//
// 없는 중간 디렉터리는 같이 만든다. 만들기·이름 바꾸기와 같은 규칙이다.
//
// 경로를 받는 화면 둘이 나눠 쓴다 — `mc` 의 아래 줄과 우클릭 메뉴의 상자다(ADR-0146).
//
// **디스크에 있는 것을 복제한다.** 그 파일을 tab 에서 고치고 저장하지 않았어도 고친 것은
// 따라가지 않는다. 트리는 디스크를 보여주는 자리이고 `cp` 와 같은 결과다(ADR-0152).
func copyEntry(e *editor, from string, isDir bool, name string) (tea.Model, tea.Cmd) {
	if name == "" {
		return sidebarModeMessage(e, "이름이 없습니다")
	}

	to := filepath.Join(e.sidebar.root, name)
	if !e.sidebar.underRoot(to) {
		return sidebarModeMessage(e, "뿌리 밖으로는 복제할 수 없습니다: "+name)
	}

	// 디렉터리를 자기 안으로 복제하면 걷는 동안 방금 만든 것을 다시 만나 끝없이 깊어진다.
	if isDir && strings.HasPrefix(to, from+string(filepath.Separator)) {
		return sidebarModeMessage(e, "디렉터리를 자기 안으로 복제할 수 없습니다")
	}

	// 이미 있는 이름 위로는 복제하지 않는다. 그대로 둔 경로(`enter` 만 누른 것) 도 여기서 걸린다.
	//
	// 이름 바꾸기와 달리 **대소문자만 바꾼 것도 막는다.** 대소문자를 가리지 않는 파일
	// 시스템에서는 그것이 자기 자신이라, 열어서 쓰는 순간 원본이 비워진다.
	if _, err := os.Lstat(to); err == nil {
		return sidebarModeMessage(e, "이미 있습니다: "+e.sidebar.relLabel(to))
	}

	if err := os.MkdirAll(filepath.Dir(to), 0755); err != nil {
		return sidebarModeError(e, errors.Mark(err, errCopyFile))
	}
	if err := copyTree(from, to); err != nil {
		return sidebarModeError(e, errors.Mark(err, errCopyFile))
	}

	// 원본 자리는 그대로이므로 새 자리만 다시 읽는다.
	//
	// 다시 읽는 것은 **트리가 아는 가장 깊은 위층**이다. `cmd/zn/main.go` 처럼 중간 디렉터리까지
	// 만들었으면 `cmd/zn/` 은 트리에 없고, 새로 생긴 `cmd/` 를 들고 있어야 할 자리는 뿌리다.
	// 그 아래는 reveal 이 한 층씩 펼치며 읽는다(ADR-0032).
	dir := filepath.Dir(to)
	for e.sidebar.nodeAt(dir) == nil && dir != e.sidebar.root {
		dir = filepath.Dir(dir)
	}
	refresh := e.refreshDir(dir)

	// 고른 자리는 새로 생긴 것이다. 복제한 뒤 대개 그것을 고치러 간다.
	reveal := e.revealInSidebar(to)

	message := "복제했습니다: " + entryLabel(e, isDir, from) + " → " + entryLabel(e, isDir, to)

	// git 이 모르는 파일이 생긴 것이라 저장소 상태가 달라진다(ADR-0030).
	model, cmd := sidebarModeMessage(e, message)

	return model, tea.Batch(cmd, refresh, reveal, e.startGitRefresh())
}

// copyTree 는 from 을 to 로 복제한다. to 는 아직 없어야 한다.
//
// **symlink 는 따라가지 않고 링크로 복제한다.** `cp -R` 의 기본과 같고, 트리가 링크를 따라가지
// 않는 것과도 맞는다(ADR-0005). 따라가면 링크가 가리키는 뿌리 밖의 것이나 자기 위층이 통째로
// 딸려 온다. WalkDir 은 시작점도 Lstat 으로 보므로 고른 것이 링크여도 같다.
//
// **중간에 실패하면 만든 것을 지운다.** 새 자리는 방금 만든 것이라 지워도 잃는 것이 없고,
// 반쯤 찬 복제본이 남으면 원본과 같은 것처럼 보인다.
//
// 맨 처음 만드는 것(to 자체) 이 실패했으면 지우지 않는다. 그 사이에 누가 같은 이름을 만든
// 것이라면 그것은 우리 것이 아니다.
func copyTree(from, to string) error {
	made := false

	err := filepath.WalkDir(from, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		rel, err := filepath.Rel(from, path)
		if err != nil {
			return err
		}
		target := filepath.Join(to, rel)

		info, err := entry.Info()
		if err != nil {
			return err
		}

		switch {
		case entry.Type()&fs.ModeSymlink != 0:
			link, err := os.Readlink(path)
			if err != nil {
				return err
			}
			if err := os.Symlink(link, target); err != nil {
				return err
			}
		case entry.IsDir():
			// 주인 쓰기 권한은 늘 붙인다. 읽기 전용 디렉터리를 그 권한 그대로 만들면 그 안에
			// 아무것도 복제해 넣을 수 없다.
			if err := os.Mkdir(target, info.Mode().Perm()|0700); err != nil {
				return err
			}
		case info.Mode().IsRegular():
			if err := copyRegularFile(path, target, info.Mode().Perm()); err != nil {
				return err
			}
		default:
			// FIFO 를 열면 쓰는 쪽이 나타날 때까지 돌아오지 않아 편집기가 통째로 멈춘다.
			// 소켓과 장치는 내용을 복제한다는 뜻이 없다.
			//
			// PathError 로 싸서 알림이 다른 실패와 같이 `복제할 수 없습니다: 경로: 까닭` 으로 선다(noticeText).
			return &fs.PathError{Op: "copy", Path: path, Err: errors.New("일반 파일이 아닙니다")}
		}

		made = true

		return nil
	})
	if err != nil && made {
		os.RemoveAll(to)
	}

	return err
}

// copyRegularFile 은 파일 하나를 복제한다. 권한은 원본 그대로라 실행 파일은 실행 파일로 남는다.
//
// O_EXCL 이다. 앞에서 없는 것을 보았어도 그 사이에 생긴 것을 덮어쓰지 않는다.
// 쓰다 실패하면 반쯤 쓴 것을 지운다 — 디렉터리 안이면 copyTree 가 통째로 지우지만, 고른 것이
// 파일 하나면 지울 사람이 여기뿐이다.
func copyRegularFile(from, to string, perm fs.FileMode) error {
	source, err := os.Open(from)
	if err != nil {
		return err
	}
	defer source.Close()

	target, err := os.OpenFile(to, os.O_CREATE|os.O_EXCL|os.O_WRONLY, perm)
	if err != nil {
		return err
	}

	if _, err := io.Copy(target, source); err != nil {
		target.Close()
		os.Remove(to)

		return err
	}
	if err := target.Close(); err != nil {
		os.Remove(to)

		return err
	}

	return nil
}

func (m viewSidebarCopy) View() tea.View {
	view := m.editorView(tea.CursorBlock, "TREE", copyPrompt+m.input.text)

	// 커서는 치고 있는 자리다. 아래 줄은 편집 영역 아래에서 시작하므로 sidebar 만큼
	// 오른쪽으로 옮긴다. 명령줄과 같다.
	view.Cursor = tea.NewCursor(textarea.WidthOf(copyPrompt)+m.input.screenCursor()+m.sidebarLeft(), m.height-1)

	return view
}
