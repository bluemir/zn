# ADR-0111 팔레트는 고른 범위를 가변 옵션으로 실어 보낸다

- 상태: 채택
- 날짜: 2026-08-31
- 관련: ADR-0011(팔레트와 `>` 명령 표), ADR-0037(visual mode), ADR-0083(대소문자 맞추기), ADR-0089(visual 의 `:`), ADR-0100(범위 시작으로 가는 커서), ADR-0106(markdown 표 맞추기), ADR-0109(고른 범위가 필요 없는 명령 다섯)

## 맥락

ADR-0109 가 「팔레트가 고른 범위를 어떻게 넘겨주는가」를 미결로 남겼고, `docs/tasks.md` 가 세 자리에서 그것을 기다리고 있었다. 대소문자 맞추기, `format selection`, 그리고 고른 범위의 markdown 표 맞추기다.

visual 에서 `ctrl+p` 는 이미 된다. 고른 것을 놓지 않고 열어서 상자 뒤로 칠해져 있다(ADR-0037). 막히는 자리는 그 다음이다.

```go
// view-palette.go 의 run()
if m.hasTab() {
	m.activeBuffer().clearSelection()
}
...
return commands[index].run(m.editor)
```

무엇을 고르든 이 문에서 범위를 놓는다. 그래서 명령이 도는 시점에는 범위가 없다. 놓는 근거는 그 자리에 셋이 적혀 있고 셋 다 **범위를 안 쓰는 명령**의 방어다. 커서를 옮기며 둘러보는 판에서 강조가 널뛰고, 특수문자 판이 고른 것이 칠해진 채 글자를 넣으며, 다른 파일을 열면 원래 tab 에 유령 강조가 남는다.

거기에 `paletteCommand.run` 의 서명이 `func(e *editor)` 라 범위를 받을 칸이 아예 없다. `selection.go` 가 anchor 를 mode model 이 아니라 Buffer 에 둔 것도 그 서명 때문이었다.

## 결정

**`run` 을 가변 옵션으로 받게 하고, 팔레트가 범위를 놓기 전에 사본으로 실어 보낸다.**

```go
type runOption func(*runOptions)

type runOptions struct {
	area    motionRange
	hasArea bool
}

func withRange(area motionRange) runOption

// paletteCommand
run func(e *editor, opts ...runOption) (tea.Model, tea.Cmd)
```

`run()` 은 놓기 전에 싣는다. 놓는 일 자체는 지금까지와 똑같이 무조건이라 위 방어 셋이 그대로 산다.

```go
opts := []runOption{}
if m.hasTab() {
	if area, ok := m.activeBuffer().selectionRange(); ok {
		opts = append(opts, withRange(area))
	}

	m.activeBuffer().clearSelection()
}
```

## 왜 옵션인가

`run` 옆에 `runRange` 를 두는 안을 먼저 검토했다. 타입이 「범위를 쓰는 명령」을 스스로 말해서 `when` 을 따로 붙이지 않아도 되고, 스물둘 중 열아홉이 한 글자도 안 바뀐다.

**기각한 것은 앞으로 들어올 명령 때문이다.** 텍스트를 다루는 명령은 대체로 범위를 받는 쪽이다. 지금 셋뿐이라 열아홉이 안 쓰는 인자를 다는 것처럼 보이지만, 그 비율은 명령이 늘수록 뒤집힌다. 서명을 둘로 갈라 두면 명령을 넣을 때마다 어느 쪽인지 고르는 일이 생기고, 옵션이 범위 말고 하나 더 늘면 갈래가 넷이 된다.

부르는 곳이 한 곳이라는 것은 옵션 안의 약점이 맞다. functional options 는 부르는 쪽이 여럿일 때 값이 나오는 손이고 여기는 `run()` 한 줄이다. 그래도 **서명이 흔들리지 않는 쪽**을 골랐다.

`when` 이 갈래를 못 읽는 문제는 그대로 남는다. 「범위가 있어야 성립한다」를 `whenSelection` 으로 손으로 적어야 하고, 그것이 명령 안의 조건과 어긋날 수 있다. `paletteCommand.when` 이 이미 경고한 자리다. 지금은 그 짝이 둘뿐이라 눈으로 지킬 수 있다.

## 범위를 줄로 넓히는 것과 칸까지 보는 것

`runOptions.lines` 가 「범위가 있으면 그 줄, 없으면 파일 전체」를 한 자리에서 정한다.

```go
func (o runOptions) lines(lineCount int) (from, to int) {
	if !o.hasArea {
		return 0, lineCount
	}

	return o.area.startLine, o.area.endLine + 1
}
```

이 손을 쓰는 넷(「줄 끝 공백 지우기」·「중복 공백 지우기」·「줄 정렬」·「표 맞추기」) 은 다 줄을 통째로 고치는 것이라 칸을 볼 자리가 없다. 그리고 **범위가 없어도 뜻이 서므로 normal 에서 연 팔레트에도 그대로 뜬다.** 지금까지의 동작이 곧 「범위가 없을 때」라 퇴보가 없다.

대소문자 맞추기 둘은 이것을 쓰지 않고 `hasArea` 를 직접 본다. 고른 칸을 그대로 봐야 하고, 파일 전체로 갈음할 수도 없다 — 손이 미끄러졌을 때 잃는 것이 너무 크고, 되돌리기가 있어도 시킬 만한 일이 아니다. 그래서 `whenSelection` 으로 **목록에서 감춘다.** 골라 놓고 거절하는 것보다 고를 수 없는 편이 낫다는 `when` 의 태도 그대로다.

## 버그 하나

`run()` 이 범위를 놓은 **뒤에** `m.commands()` 를 부르고 있었다. `whenSelection` 이 범위를 보므로 놓은 뒤에는 목록에서 둘이 빠지고, `hits` 의 자리가 그만큼 어긋난다. 「대문자로 맞추기」를 골랐는데 엉뚱한 명령이 도는 자리다.

`commands()` 주석이 「목록도 고르는 것도 그리는 것도 이것을 지난다. 셋이 같은 것을 보아야 한다」고 이미 적어 둔 것을, `when` 이 상태를 보게 되면서 처음으로 깨뜨렸다. 집는 자리를 놓기 전으로 올렸다.

## 결과

- 팔레트 명령이 스물둘에서 스물넷이 되었다. 「대문자로 맞추기」·「소문자로 맞추기」다.
- 넷이 고른 줄만 다루게 되었다. ADR-0109 가 「파일 전부가 대상이다」로 두었던 「줄 정렬」이 그중 하나다.
- `buf.trimTrailingSpace`·`squeezeSpaces`·`sortLines` 가 `[from, to)` 를 받는다. 저장 hook 은 파일 전체를 댄다.
- 「표 맞추기」를 visual 에서 고르면 `\mt` 와 같은 일이 된다. 길이 둘이 되었지만 둘 다 남긴다 — 키는 손이 빠르고 팔레트는 이름으로 찾는 자리다.

## 나중에 볼 것

- **command mode 와 옵션을 공유할지.** `:` 는 `'<,'>` 를 명령줄에 실어 보내고 `lineRange.area` 가 Buffer 에서 읽는 다른 길이다(ADR-0089). 둘을 한 손으로 모을 수 있는지는 이 결정에 넣지 않았다.
- **`whenSelection` 과 명령 안 조건이 갈리는 것.** 지금은 짝이 둘이라 눈으로 지킨다.
- **`format selection`.** tasks.md 가 지목한 셋 중 이것만 남았다. 포매터를 범위에 거는 일이라 저장 hook 쪽을 먼저 봐야 한다.
