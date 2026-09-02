package core

// startSelection 은 커서 자리를 anchor 로 삼아 범위를 연다. `v` `V` 와 드래그가 여기로 온다.
func (e *editor) startSelection(linewise bool) {
	e.activeBuffer().StartSelection(linewise)
}
