package textarea

import (
	"bytes"
	"crypto/sha256"
	"os"

	"github.com/cockroachdb/errors"
)

// 디스크에서 읽어 오는 길이다. 밖에서 달라졌는지 맞춰 보고(checkOutside), 다시 읽고(Reload),
// 보던 자리를 새 내용에 옮겨 담는다(adopt) (ADR-0015, ADR-0038, ADR-0044).
//
// **여기 남은 것은 글만 다룬다.** 커서를 옮기며 이것을 부르는 쪽은
// viewport-reload.go 다 (ADR-0121).

// checkOutside 는 파일을 다시 읽어 읽은(또는 마지막으로 쓴) 시점과 맞춰 본다.
//
// 판정은 내용 해시로 한다. mtime 은 내용이 같아도 바뀌는 일이 흔해서(git checkout,
// 다른 도구의 되쓰기) 그것으로 막으면 헛경고가 잦다 (ADR-0015).
//
// 이름 없는 buffer 는 맞춰 볼 파일이 없으므로 그대로인 것으로 본다.
func (buf Buffer) checkOutside() (OutsideChange, error) {
	if buf.Path == "" {
		return OutsideSame, nil
	}

	data, err := os.ReadFile(buf.Path)
	switch {
	case errors.Is(err, os.ErrNotExist):
		// 열 때도 없던 파일이면 달라진 것이 없다. 저장하는 쪽에서는 지금 새로 만드는 것이 맞다.
		if buf.Disk.Hash == nil {
			return OutsideSame, nil
		}

		return OutsideRemoved, nil
	case err != nil:
		return OutsideSame, errors.Wrapf(err, "cannot read %s", buf.Path)
	case buf.Disk.Hash == nil:
		return OutsideCreated, nil
	}

	sum := sha256.Sum256(data)
	if !bytes.Equal(sum[:], buf.Disk.Hash) {
		return OutsideModified, nil
	}

	return OutsideSame, nil
}

// checkNotChangedOutside 는 파일이 읽은(또는 마지막으로 쓴) 시점과 같은지 본다.
// 다르면 무엇이 달라졌는지와 `:w!` 로 빠져나가는 길을 담은 error 를 준다 (ADR-0015).
func (buf Buffer) checkNotChangedOutside() error {
	change, err := buf.checkOutside()
	if err != nil {
		return err
	}

	switch change {
	case OutsideRemoved:
		return errors.New("파일이 밖에서 사라졌습니다. 다시 만들려면 `:w!` 입니다")
	case OutsideCreated:
		return errors.New("파일이 밖에서 새로 생겼습니다. 덮어쓰려면 `:w!` 입니다")
	case OutsideModified:
		return errors.New("파일이 밖에서 바뀌었습니다. 덮어쓰려면 `:w!` 입니다")
	}

	return nil
}

// outsideChange 는 파일이 읽은(또는 마지막으로 쓴) 시점과 어떻게 달라졌는지다.
//
// 알릴 문구는 부르는 쪽이 만든다. 같은 사실에 붙는 다음 걸음이 자리마다 다르다 —
// 저장이 막힌 자리는 빠져나가는 길(`:w!`) 을 알려야 하고, 셸에서 돌아온 자리는
// 가져오는 길(`:e`) 을 알린다 (ADR-0015, ADR-0023).
type OutsideChange int

const (
	OutsideSame     OutsideChange = iota // 읽은 시점과 같다
	OutsideModified                      // 내용이 달라졌다
	OutsideCreated                       // 없던 파일이 생겼다
	OutsideRemoved                       // 있던 파일이 사라졌다
)
