package core

import "os"

// 읽기 전용 파일을 다루는 자리다.
//
// 정의로 뛰면 표준 라이브러리와 의존 모듈의 파일이 열린다. 그것들은 module cache 에 있고
// `r--r--r--` 이라 고칠 수 없다(ADR-0051). 그대로 두면 한참 고친 뒤 저장할 때에야 막히므로,
// 열 때 알아보고 고치는 것을 그 자리에서 거절한다.

// detectReadOnly 는 그 파일을 고칠 수 없는지다. 파일이 없으면 거짓이다 — 새로 만드는 것이다.
//
// **권한 비트만 본다.** 소유자·그룹·ACL 을 따지지 않는다. 정확한 답은 실제로 열어 보는 것뿐인데,
// 그러면 파일을 열 때마다 쓰기로 한 번 더 여는 일이 붙는다. 여기서 놓치는 것(남의 파일이지만
// 비트는 열려 있는 경우) 은 저장할 때 오류로 잡히고, 그 자리에는 이미 문구가 있다.
func detectReadOnly(path string) bool {
	if path == "" {
		return false
	}

	info, err := os.Stat(path)
	if err != nil {
		return false
	}

	if info.IsDir() {
		return false
	}

	return info.Mode().Perm()&0200 == 0
}

// refuseReadOnly 는 읽기 전용이면 알리고 참을 준다. 고치는 동작이 첫 줄에서 부른다.
//
// 표시를 buffer 에 두고 검사를 동작마다 두는 것은, 거절했다는 것을 사람이 알아야 하기
// 때문이다. 줄을 갈아끼우는 자리(edit.go 의 replaceLines) 에서 막으면 한 군데면 되지만
// 아무 일도 일어나지 않는 것처럼 보인다 — 키가 먹지 않는 편집기가 된다.
func (e *editor) refuseReadOnly() bool {
	if !e.activeBuffer().readOnly {
		return false
	}

	e.message = "읽기 전용 파일입니다"

	return true
}
