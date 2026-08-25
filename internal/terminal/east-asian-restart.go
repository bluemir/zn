package terminal

import (
	"os"
	"syscall"
)

// eastAsianEnv 는 폭 눈금을 켜는 환경변수다. `x/ansi` 가 `init()` 에서 한 번 읽는다.
//
// 이름과 달리 값 `1` 은 칸 수가 아니라 참이다 — 켜면 Ambiguous 글자가 두 칸이 된다.
// `mattn/go-runewidth` 의 `Condition.EastAsianWidth` 를 env 로 뒤집던 이름이 굳은 것이다.
const eastAsianEnv = "RUNEWIDTH_EASTASIAN"

// RestartForAmbiguousWidth 는 폭 눈금을 터미널에 맞추려고 프로세스를 다시 시작한다.
// **맞출 것이 있으면 이 함수는 돌아오지 않는다.**
//
// 터미널이 Ambiguous 글자를 두 칸으로 그리는데 우리 쪽 폭 계산은 한 칸으로 센다. 그 눈금은
// `x/ansi` 의 패키지 전역이고 손잡이가 eastAsianEnv 하나인데, 그것을 읽는 `init()` 은
// `main()` 보다 먼저 지나간다 — 재고 나서 `os.Setenv` 를 해도 늦다. 그래서 알아낸 답을 env 에
// 얹어 자기 자신을 다시 실행한다. 새 프로세스는 눈금을 켠 채로 시작한다(ADR-0072).
//
// **터미널을 바꾸는 것이 아니라 우리 장부를 터미널에 맞추는 것이다.** 그렇게 해야 우리
// 계산과 lipgloss 와 ultraviolet 이 같은 폭을 센다 — 화면을 실제로 만드는 것은 ultraviolet
// 이라, 우리 계산만 고치면 어긋나는 자리가 옮겨갈 뿐이다.
//
// 실패하면 그냥 돌아온다. Windows 의 `syscall.Exec` 이 EWINDOWS 를 주는 자리와 실행 파일을
// 못 찾는 자리다 — 눈금이 어긋난 채로 뜨는 것이 아예 못 뜨는 것보다 낫다.
func RestartForAmbiguousWidth(width int) {
	if !needsRestart(width) {
		return
	}

	// `os.Args[0]` 은 PATH 로 실행하면 이름만 온다. 그 이름으로 다시 실행하면 PATH 를 다시
	// 뒤지게 되고, 그 사이 PATH 가 다른 zn 을 가리키면 다른 물건이 뜬다.
	self, err := os.Executable()
	if err != nil {
		return
	}

	syscall.Exec(self, os.Args, append(os.Environ(), eastAsianEnv+"=1"))
}

// needsRestart 는 다시 시작할 자리인지다.
//
// **두 칸으로 확인됐을 때만이다.** 한 칸이면 지금 눈금이 이미 맞고, 재지 못했으면(0) 아무것도
// 모른다 — 대부분의 터미널이 한 칸으로 그리므로 모르는 채로 켜면 다수가 깨진다.
//
// **env 가 이미 있으면 값이 무엇이든 손대지 않는다.** 사용자가 정한 것이 먼저다. 그리고
// 우리가 세우고 다시 시작한 프로세스에서는 이 문이 닫혀 있어서 무한 재실행이 되지 않는다.
func needsRestart(width int) bool {
	if width != 2 {
		return false
	}

	_, set := os.LookupEnv(eastAsianEnv)

	return !set
}
