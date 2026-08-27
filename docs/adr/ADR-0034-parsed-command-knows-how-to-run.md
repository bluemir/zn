# ADR-0034: 파싱 결과가 곧 동작이고, 동작이 자기 실행을 안다

- 상태: 채택
- 날짜: 2026-08-18
- 이후: ADR-0035 가 이름을 `command*` 에서 `action*` 으로 옮겼다. 수식어 없는 `command` 는 `:` 로 치는 것이 가지고, 키로 치는 이쪽은 `action`(한국어로 「동작」) 이다

## 맥락

normal mode 의 키 처리는 파서(`normalState`)가 키 나열을 받아 동작을 만들고, `viewEditorNormal` 이 그것을 실행하는 구조였다. 그런데 파서가 만들던 것은 동작이 아니라 **이름 문자열**이었다.

```go
type normalKey struct {
	name  string // "d w", "g g", "ctrl+w ctrl+w"
	count int
}
```

`d` 와 motion 을 공백으로 이어 붙여 `"d w"` 를 만들고, 실행하는 쪽이 그것을 다시 뜯었다.

```go
// view-editor-normal.go
if motion, found := strings.CutPrefix(key.name, "d "); found { ... }  // ×4 (d r c y)
switch key.name {
case "g g": ...   // 파서가 이어 붙인 것을 문자열로 다시 맞춘다
case "ctrl+w ctrl+w", "ctrl+w w": ...
```

그리고 뜯어낸 motion 문자열이 한 겹 더 내려가 또 switch 를 탔다.

```go
// delete.go
func (buf Buffer) charMotionTarget(motion string, ...) {
	switch motion {
	case "h", "left": ...
```

**파싱이 세 겹이었다.** 파서는 키를 이름으로 *정규화*만 하고, 실제로 "이 동작이 무엇인가" 를 정하는 일은 실행하는 쪽에서 다시 일어났다.

측정값은 이랬다. `CutPrefix` 4 곳, 이어 붙인 이름을 다시 맞추는 `case` 4 곳, motion 문자열 switch 3 곳(`delete.go` 2, `change.go` 1). `run` 은 208 줄에 case 41 개였다.

치르던 값은 셋이다.

1. **계약이 약속된 문자열이다.** 파서가 구분자를 바꾸면 컴파일이 아니라 런타임에 "모르는 이름" 으로 조용히 아무 일도 하지 않는다. type 검사가 닿지 않는다.
2. **operator 마다 갈리는 곳을 표현할 수 없다.** `d` `y` `c` 가 파서에서 완전히 같아 보였는데, 그건 셋 다 문자열로 뭉개고 있어서였다. `cw` 가 `ce` 인 예외(ADR-0033)는 실행 쪽에 따로 숨어 있었다.
3. **키맵이 코드 흐름에 흩어져 있다.** 어느 키가 무엇을 하는지가 파서의 이름 표와 `run` 의 평평한 switch 두 곳에 나뉘어 있었다.

ADR-0006 이 이 문제를 이미 짚어 두었다. *"`run` 의 switch 도 표(map)나 command type 으로 … 상태 기계와 idiom 도 맞는다"* 며 미뤄둔 대안이 이것이다.

## 결정

**파서가 이름이 아니라 동작을 만든다. 동작은 자기가 어떻게 실행되는지 안다.**

```go
type action interface {
	run(e *editor) (tea.Model, tea.Cmd)
}
```

**받는 것은 editor 이지 화면이 아니다.** 동작이 건드리는 것은 buffer·register·tab 이고 그것은 전부 editor 에 있다. 재보니 24 개 중 23 개가 `*editor` 의 것만 썼다. `viewEditorNormal` 이 그것을 embed 하고 있어서 `m.` 으로 닿았을 뿐이다.

mode 를 바꾸는 동작(`i` `:` `ctrl+p`)은 새 model 을 돌려주고, 바꾸지 않는 동작은 **nil** 을 돌려준다. 부르는 쪽이 지금 mode 를 그대로 쓴다. `return m, nil` 껍데기가 `return nil, nil` 이 된다.

화면이 정말 필요했던 하나는 `actionQuit` 였다. 확인창에서 취소했을 때 돌아갈 부모 model 이 필요한데, `ctrl+c` 가 완성된 시점이라 키 상태가 비어 있어서 `normalMode(e)` 로 새로 만든 것과 지금 것이 같다. 그래서 동작이 스스로 만든다.

`viewEditorNormal.press` 는 파서가 준 동작을 차례로 `act.run(normal.editor)` 할 뿐이다. `run` 메서드 208 줄이 없어지고 `view-editor-normal.go` 는 142 줄이 되었다.

**motion 도 type 이다.** 동작이 motion 문자열을 들고 다니면 세 번째 파싱이 남는다.

```go
// motion 은 operator 가 잡을 범위를 안다.
type motion interface {
	span(buf Buffer, count, width int) (motionRange, bool)
}

// moveMotion 은 이동 키로도 칠 수 있는 motion 이다.
type moveMotion interface {
	motion
	move(buf *Buffer, count, width int)
}
```

interface 를 둘로 가른 것은 `dd` 의 「줄 전체」(`motionWholeLines`) 때문이다. operator 뒤에서만 생기고 갈 자리가 없어서 `move` 가 없다. type 이 그것을 나타낸다.

**count 해석은 motion 이 한다.** `3w` 는 되풀이이고 `3G` 는 줄 번호다. 파서는 숫자를 그대로 넘기고 0 이 "숫자 없음" 이다. 예전에는 `run` 이 `max(count, 1)` 을 미리 걸고 `G` 만 원래 값을 따로 쓰는 갈래가 있었다.

**operator 에 따라 motion 이 갈리는 곳이 표에 드러난다.**

```go
func motionFor(op, key string) (moveMotion, bool) {
	case "w":
		if op == "c" {
			return motionChangeWord{kind: smallWord}, true   // cw 는 ce 다(ADR-0033)
		}
		return motionWordForward{kind: smallWord}, true
```

`cw` 의 예외가 실행 쪽 `changeRange` 에 숨어 있던 것이 파서의 표 한 줄로 올라왔다.

## 근거

**계약이 type 이 되면 어긋남이 컴파일 오류다.** 문자열 계약에서는 `"d w"` 와 `"d  w"` 의 차이가 런타임에 아무 일도 하지 않는 것이었다. 이제 `actionDelete{motion: motionWordForward{...}}` 라 그런 어긋남이 없다.

**세 겹이 한 겹이 된다.** 키를 동작으로 바꾸는 일이 파서 한 곳에서 끝난다. `motionFor` · `standaloneAction` · `prefixMotion` · `prefixAction` 네 표가 곧 키맵이고, 한 화면에서 읽힌다.

**숫자를 받는 키 목록이 없어졌다.** 예전에는 `normalCount` 가 `case "h", "j", "k", …` 로 숫자를 받는 키를 열거하고 나머지는 숫자를 버렸다. 이제 숫자를 쓰지 않는 동작이 그냥 무시하므로 (`actionInsert` 에 count 필드가 없다) 그 표가 통째로 사라졌다. 결과는 같다(`3i` 는 `i` 다).

**operator 셋이 갈릴 곳이 생겼다.** `actionDelete` · `actionYank` · `actionChange` 가 서로 다른 type 이라 `actionChange` 만 insert mode 로 들어가는 것이 그 type 안에 적혀 있다.

## 결과

- `strings.CutPrefix(key.name, …)` 와 motion 문자열 switch 가 전부 없어졌다. 코드에 `normalKey` 가 남아 있지 않다
- 새 파일이 둘 생겼다. `motion.go`(282 줄), `action.go`(313 줄) 다. `view-editor-normal.go` 는 350 줄에서 142 줄로 줄었고 `run` 이 없다
- **`dgd` 가 고쳐졌다.** 예전에는 `d` 뒤에 접두 키 `g` 를 기다리는 중에 `d` 가 오면 operator 가 그것을 가로채 `dd` 가 됐다. 이제 짝이 없어서 아무 일도 하지 않는다. `drd` 도 같다
- **`dy` 처럼 operator 를 겹쳐 치면 앞의 것을 무르고 새로 연다.** 이것은 이 ADR 앞에 `inner` 중첩을 걷어내면서 정한 것이고(vim 이 그렇게 한다, tmux 로 vim 9.1 확인) type 으로 옮기면서 그대로 왔다
- **짝 없는 조합이 동작을 만들지 않는다.** 예전에는 `"g x"` 같은 이름이 만들어져 실행 쪽에서 버려졌다. 이제 파서가 nil 을 준다. 버리는 곳이 하나 줄었다
- `d↑` `d↓` 는 여전히 받지 않는다. `motionRowUp`·`motionRowDown` 의 `span` 이 false 다(ADR-0013)

## 대안

- **동작을 type 으로 하되 실행은 밖에 둔다**
	- `switch c := cmd.(type)` 로 실행하는 것이다. 문자열 재파싱은 똑같이 없어진다.
	- 그러면 "이 동작이 무엇을 하는가" 가 다시 두 곳(type 선언과 switch)에 나뉜다. `run` 을 type 에 두면 한 곳이다.
- **motion 은 문자열로 두고 동작만 type 으로**
	- 범위가 파서와 `run` 으로 닫혀서 작다.
	- 세 번째 파싱(`charMotionTarget` 의 switch)이 남는다. 그 switch 가 `cw` 예외를 숨기고 있던 곳이라 같이 걷었다.
- **`run` 이 화면 model(`viewEditorNormal`)을 받는다**
	- 처음에 그렇게 만들었다. 동작이 mode 를 바꿀 수 있으니 받는 것도 model 로 맞춘 것이다.
	- 재보니 24 개 중 23 개가 화면을 보지 않았다. mode 를 바꾸지 않는 동작까지 화면을 알게 되는 대가를 치르고 있었다.
- **이름별 표(map[string]command)**
	- ADR-0006 이 같이 적어둔 대안이다. 키맵이 데이터가 되어 가장 짧다.
	- 이름이 다시 문자열 키가 되어 이 ADR 이 없애려는 것을 절반 되돌린다.
	- `d3w` 처럼 숫자와 접두가 섞이는 조합을 표 하나로 적을 수도 없다.
- **`normalStateDeleteOp` 처럼 operator 마다 상태를 나눈다**
	- type 이 operator 를 나타내어 가장 명확하다.
	- 그 뒤의 숫자·접두 상태가 operator 수만큼 곱해진다(3 × 4 = 12 개).
	- operator 는 상태가 아니라 "이미 정해진 것" 이라 `partial` 값으로 들고, type 은 "무엇을 기다리는가" 만 나타내는 쪽을 골랐다.

## 이 ADR 이 정하지 않는 것

- **키맵을 데이터로 둘지.** `docs/tasks.md` 의 "`config 는 compile 됨` 의 실제 형태를 정한다" 는 아직 열려 있다. 이 ADR 은 동작이 type 이라는 것까지만 정한다. 키에서 동작으로 가는 네 표를 선언적인 데이터로 바꿀지는 그 결정에 남는다
- text object(`ciw`)와 `f`·`t`. operator 뒤에 motion 이 아닌 것이 오는 첫 경우라 상태가 하나 더 는다(ADR-0033)
- 파서와 화면을 패키지로 가를지. `run` 이 `*editor` 만 받게 되어 동작 쪽에는 화면 의존이 없지만, mode 를 바꾸는 동작이 `insertMode(e)` 같은 것을 부르므로 아직 한 패키지다
