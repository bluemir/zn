package core

import (
	"bytes"
	"crypto/sha256"
	"os"

	"github.com/cockroachdb/errors"
)

// 디스크에서 읽어 오는 길이다. 밖에서 달라졌는지 맞춰 보고(checkOutside), 다시 읽고(Reload),
// 보던 자리를 새 내용에 옮겨 담는다(adopt) (ADR-0015, ADR-0038, ADR-0044).

// Reload 는 파일을 다시 읽어 내용을 갈아끼운다. 팔레트의 「파일 다시 읽기」가 쓴다 (ADR-0016).
//
// 다시 읽기를 "이 파일을 새로 연 것" 으로 본다. 저장하지 않은 변경은 사라지고 undo·redo 이력도
// 버린다. 잃을 것이 있을 때 묻는 것은 부르는 쪽이 한다 — 여기까지 왔으면 이미 정해진 것이다.
//
// 커서는 줄 번호와 화면 칸을 이어받는다. 밖에서 포매터를 돌린 뒤 같은 자리에서 이어 보게 된다.
// 파일이 짧아졌으면 범위 안으로 끌어온다.
func (buf *Buffer) Reload() error {
	if buf.path == "" {
		return errors.New("파일 이름이 없습니다")
	}

	data, err := os.ReadFile(buf.path)
	switch {
	case errors.Is(err, os.ErrNotExist):
		// 없는 파일을 빈 buffer 로 갈아끼우지 않는다. 밖에서 지워졌다면 지금 손에 든 것이
		// 마지막 사본이라, 다시 읽기가 그것을 지우는 명령이 되어서는 안 된다.
		// OpenBuffer 가 없는 파일을 빈 buffer 로 여는 것과 여기서 갈리는 이유다.
		return errors.Errorf("파일이 없습니다: %s", buf.path)
	case err != nil:
		return errors.Mark(err, errOpenFile)
	}

	*buf = buf.adopt(newBuffer(buf.path, data))

	return nil
}

// adopt 은 새로 읽은 내용에 지금 보고 있던 자리를 옮겨 담는다.
//
// 읽기와 나누기에서 떼어 둔 것은 값이 갈리기 때문이다. 새 Buffer 를 만드는 것은 파일 크기만큼
// 드는 일이라 백그라운드에서 하고(ADR-0044), 자리를 옮겨 담는 것은 값이 없어서 `Update` 안에서
// 해도 된다 — 옮겨 담을 「지금 자리」는 그 순간에만 알 수 있는 것이라 미리 할 수도 없다.
func (buf Buffer) adopt(next Buffer) Buffer {
	// 커서 자리를 화면 칸으로 옮겨 둔다. byte offset 은 새 내용에서 다른 글자의 중간일 수 있다.
	// statusBar 가 보여주는 `줄:칸` 이 이 칸이라, 유지되는 것이 눈에 보이는 값과 같다.
	col := screenColAt(buf.lines[buf.cursorLine], buf.cursorCol)

	next.cursorLine = min(buf.cursorLine, len(next.lines)-1)
	next.cursorCol = offsetAtScreenCol(next.lines[next.cursorLine], col)
	next.desiredCol = buf.desiredCol

	// 화면도 보고 있던 자리를 유지한다. topRow 는 폭에 따라 있을 수도 없을 수도 있는 행이라
	// 여기서 맞추지 못한다. 부르는 쪽의 scrollTo 가 clampTop 으로 맞춘다.
	next.top = min(buf.top, len(next.lines)-1)
	next.topRow = buf.topRow

	return next
}

// checkOutside 는 파일을 다시 읽어 읽은(또는 마지막으로 쓴) 시점과 맞춰 본다.
//
// 판정은 내용 해시로 한다. mtime 은 내용이 같아도 바뀌는 일이 흔해서(git checkout,
// 다른 도구의 되쓰기) 그것으로 막으면 헛경고가 잦다 (ADR-0015).
//
// 이름 없는 buffer 는 맞춰 볼 파일이 없으므로 그대로인 것으로 본다.
func (buf Buffer) checkOutside() (outsideChange, error) {
	if buf.path == "" {
		return outsideSame, nil
	}

	data, err := os.ReadFile(buf.path)
	switch {
	case errors.Is(err, os.ErrNotExist):
		// 열 때도 없던 파일이면 달라진 것이 없다. 저장하는 쪽에서는 지금 새로 만드는 것이 맞다.
		if buf.diskHash == nil {
			return outsideSame, nil
		}

		return outsideRemoved, nil
	case err != nil:
		return outsideSame, errors.Wrapf(err, "cannot read %s", buf.path)
	case buf.diskHash == nil:
		return outsideCreated, nil
	}

	sum := sha256.Sum256(data)
	if !bytes.Equal(sum[:], buf.diskHash) {
		return outsideModified, nil
	}

	return outsideSame, nil
}

// checkNotChangedOutside 는 파일이 읽은(또는 마지막으로 쓴) 시점과 같은지 본다.
// 다르면 무엇이 달라졌는지와 `:w!` 로 빠져나가는 길을 담은 error 를 준다 (ADR-0015).
func (buf Buffer) checkNotChangedOutside() error {
	change, err := buf.checkOutside()
	if err != nil {
		return err
	}

	switch change {
	case outsideRemoved:
		return errors.New("파일이 밖에서 사라졌습니다. 다시 만들려면 `:w!` 입니다")
	case outsideCreated:
		return errors.New("파일이 밖에서 새로 생겼습니다. 덮어쓰려면 `:w!` 입니다")
	case outsideModified:
		return errors.New("파일이 밖에서 바뀌었습니다. 덮어쓰려면 `:w!` 입니다")
	}

	return nil
}
