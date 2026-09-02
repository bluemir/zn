package core

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

// register 를 담고 고르는 규칙이다. 담는 곳이 열 군데(action.go 여섯, 명령줄 둘) 라
// 규칙이 흩어지면 링이 한 자리에서만 밀리는 일이 난다. 그래서 여기 둘만 부른다(ADR-0058).
//
// **무명은 늘 마지막으로 담거나 지운 것이다.** 이름을 대도 무명에 같이 담긴다. 이름을 대지
// 않은 `p` 가 읽는 자리가 무명이고, 그것을 이름 쪽으로 옮기면 「이름 없음」을 뜻하는 자리를
// 따로 정해야 한다. register 는 buffer 의 줄을 그대로 가리키므로(ADR-0017) 겹쳐도 늘어나는
// 것은 slice header 뿐이다.
//
// **이름을 대면 숫자는 건드리지 않는다.** `"add` 는 `"a` 와 무명에만 담고 링을 밀지 않는다.
// 이름을 대는 것은 「이 것을 거기 둔다」는 뜻이라, 모아 둔 `"a` 를 쓰는 사이에 링이 조용한
// 편이 도리어 낫다. vim 과 같다 — 링 설명의 전제가 「다른 register 를 지정하지 않은 한」이다.

// registerSet 은 register 셋을 한 덩이로 든 것이다. editor 가 이것 하나를 든다.
//
// 셋을 갈라 두지 않은 것은 담는 규칙이 셋에 걸쳐 있어서다 — `y` 하나가 무명과 `"0` 을 같이
// 채우고, `d` 는 무명과 숫자 링을 같이 민다. 어느 자리에 들어가는지를 아는 곳이 이 파일
// 하나여야 한다는 것이 아래 함수 넷의 전제다(ADR-0058).
type registerSet struct {
	// unnamed 는 vim 의 무명 register 다. 이름을 대지 않은 붙여넣기가 읽는 자리라 숫자
	// register 와 겹치는 값을 들고 있다 — 겹쳐 두는 까닭은 위에 적었다.
	unnamed register

	// numbered 는 숫자 register `"0`~`"9` 다. 자리가 곧 이름이라 map 이 아니다.
	//
	// `"0` 은 복사한 것만 들어가고 `"1` 부터가 지운 것의 링이다. 지울 때마다 한 칸씩 밀려
	// `"9` 에서 떨어져 나간다.
	numbered [10]register

	// named 는 문자 register `"a`~`"z` 다. 손으로 이름을 대서 담는 자리라 저절로 밀리지
	// 않는다. 대문자(`"A`) 는 같은 자리에 뒤로 잇는다.
	//
	// 여기는 배열이 아니라 map 이다. 스물여섯 자리를 늘 들고 있을 값이 없고, `"-`·`"+` 처럼
	// 글자가 아닌 이름이 뒤에 올 자리이기도 하다.
	named map[string]register
}

// storeYank 는 복사한 것을 담는다. 이름이 비어 있으면 무명과 `"0` 이다.
//
// 이름이 없을 때 숫자 링을 밀지 않는 것은 `"0` 이 「마지막으로 복사한 것」이라 지운 것들과
// 섞이지 않아야 하기 때문이다 — 지우다 덮어버리는 일을 없애려고 넣은 것이 그 갈라짐이다.
func (regs *registerSet) storeYank(block textBlock, name string) {
	reg := register{block}
	regs.unnamed = reg

	if name == "" {
		regs.numbered[0] = reg

		return
	}

	regs.storeNamed(block, name)
}

// storeDelete 는 지우거나 바꾼 것을 담는다. 이름이 비어 있으면 무명과 `"1` 이고,
// 숫자 링을 한 칸 밀어낸다.
//
// **vim 과 갈리는 자리다.** vim 은 한 줄 안에서 지운 것(`x` `dw`) 을 숫자 링에 넣지 않고
// 소삭제 register `"-` 로 보내는데, 여기서는 그것도 링에 넣는다. `register` 가 아는 갈래가
// 줄 단위인지 하나뿐이라 「한 줄 안인지」를 새로 들어야 하고, 규칙이 둘로 갈리는 값이
// 그 무게보다 작다. 그래서 `x` 를 세 번 치면 `"1` `"2` `"3` 이 글자 하나씩이 된다(ADR-0058).
//
// `"0` 은 건드리지 않는다. 복사 전용이다.
func (regs *registerSet) storeDelete(block textBlock, name string) {
	reg := register{block}
	regs.unnamed = reg

	if name != "" {
		regs.storeNamed(block, name)

		return
	}

	// `"9` 는 떨어져 나간다. 뒤에서부터 옮겨야 덮이지 않는다.
	for at := 9; at > 1; at-- {
		regs.numbered[at] = regs.numbered[at-1]
	}
	regs.numbered[1] = reg
}

// storeNamed 는 이름 있는 register 에 담는다. **대문자면 덮지 않고 뒤에 잇는다**(ADR-0058).
//
// `"A` 는 `"a` 와 같은 자리이고 담는 법만 다르다. 그래서 자리를 소문자로 맞춘다 — 대문자를
// 따로 두면 `"ap` 가 `"A` 로 모은 것을 못 본다.
func (regs *registerSet) storeNamed(block textBlock, name string) {
	reg := register{block}

	if regs.named == nil {
		regs.named = map[string]register{}
	}

	lower := strings.ToLower(name)
	if lower == name {
		regs.named[lower] = reg

		return
	}

	regs.named[lower] = appendRegister(regs.named[lower], reg)
}

// appendRegister 는 뒤에 잇는다. `"A` 가 부른다.
//
// **둘 중 하나라도 줄 단위면 결과도 줄 단위다.** 줄로 모아 둔 것에 글자 조각을 이으면 줄
// 사이에 끼울 자리가 없고, 글자로 모은 것에 줄을 이으면 그 줄이 어디서 끝나는지가 사라진다.
// vim 과 같다.
//
// 둘 다 글자 단위면 이음매에서 줄이 하나로 붙는다. `"ayw` 와 `"Ayw` 로 낱말 둘을 모으면
// 한 줄이어야 한다.
//
// **줄을 제자리에서 늘리지 않는다.** register 가 가리키는 것은 buffer 의 줄 그대로라
// (ADR-0017) `append` 가 그 backing array 에 닿으면 파일이 조용히 바뀐다.
func appendRegister(base, extra register) register {
	if !base.filled() {
		return extra
	}
	if !extra.filled() {
		return base
	}

	lines := make([][]byte, 0, len(base.lines)+len(extra.lines))
	lines = append(lines, base.lines...)

	if base.linewise || extra.linewise {
		return register{textBlock{lines: append(lines, extra.lines...), linewise: true}}
	}

	last := len(lines) - 1
	lines[last] = append(append([]byte{}, lines[last]...), extra.lines[0]...)

	return register{textBlock{lines: append(lines, extra.lines[1:]...)}}
}

// byName 은 이름으로 고른 register 다. 이름이 비어 있으면 무명이다.
//
// **아직 없는 이름은 빈 것이다.** 무명으로 떨어뜨리지 않는다 — `"q` 를 친 손은 무명을
// 부탁한 것이 아니라서, 엉뚱한 것을 붙이는 것보다 아무 일도 하지 않는 편이 낫다. 빈
// register 는 조용하다는 규칙이 이미 있어서(ADR-0017) 새 갈래를 만들지 않는다.
//
// 대문자는 소문자와 같은 자리다. `"Ap` 도 `"ap` 와 같은 것을 붙인다 — 대문자로 갈리는 것은
// 담을 때뿐이다(storeNamed).
func (regs registerSet) byName(name string) register {
	if name == "" {
		return regs.unnamed
	}
	if at, ok := keyDigit(name); ok {
		return regs.numbered[at]
	}

	return regs.named[strings.ToLower(name)]
}

// registerWritable 은 그 이름에 담을 수 있는지다.
//
// **문자 register 만 담을 수 있다.** 숫자는 지울 때마다 저절로 채워지는 자리라 손으로
// 담아 두어도 다음 지우기가 링을 밀면 그 값이 옆자리로 내려간다 — 넣은 자리에 없는 것을
// 「넣었다」고 기억하게 된다. 그 밖의 이름(`"%` `"+`) 은 아직 아무것도 아니다(ADR-0058).
func registerWritable(name string) bool {
	if len(name) != 1 {
		return false
	}

	return (name[0] >= 'a' && name[0] <= 'z') || (name[0] >= 'A' && name[0] <= 'Z')
}

// register 는 한 칸에 담긴 것이다. vim 의 무명 register 에 해당한다.
//
// **떼어 낸 자료(textBlock) 에 말을 얹은 것이다.** 담긴 것 자체는 글에서 떼어 낸 덩이라
// 글을 다루는 겹의 것이고, 「몇 자인가」·「무엇을 알릴까」는 editor 가 하는 말이다.
// embed 라 `reg.lines`·`reg.filled()` 가 그대로 서고, 붙여넣기에 넘길 때는 `reg.textBlock`
// 으로 자료만 준다 — 그쪽이 필요한 것은 자리와 글과 줄 단위인지뿐이다(text-block.go).
type register struct {
	textBlock
}

// copiedMessage 는 `y` 가 statusBar 아래 줄에 띄우는 알림이다. 다음 키를 누르면 사라진다.
//
// 복사는 화면에 아무 자국을 남기지 않는다 — 지우기와 달리 글자가 그대로 있어서, 손이 친 `y`
// 가 먹었는지 범위를 어디까지 잡았는지 볼 길이 없다. 그래서 한 줄짜리도 알린다.
// vim 은 `report` 만큼 넘을 때만 알리지만, 그 기준은 "파일이 크게 바뀌었다" 는 경고 쪽이다.
//
// 줄 단위면 줄 수, 글자 단위면 글자 수다. 붙여넣기가 줄로 들어갈지 글자로 들어갈지가 곧
// 이 갈래라, 알림이 그것을 같이 보여 준다.
func (reg register) copiedMessage() string {
	if reg.linewise {
		return fmt.Sprintf("%d 줄 복사되었습니다", len(reg.lines))
	}

	return fmt.Sprintf("%d 글자 복사되었습니다", reg.charCount())
}

// deletedMessage 는 `:d` 가 알리는 문구다.
//
// 세는 법은 copiedMessage 와 같은 자리에 둔다 — 같은 것을 세는 법이 두 벌이 되면
// `:'<,'>d` 와 `:'<,'>y` 가 같은 범위를 두고 다르게 말한다.
//
// 키로 치는 `dd`·visual 의 `d` 는 알리지 않는다. 글자가 사라지는 것이 화면에 보여서다
// (ADR-0017). 손으로 친 범위는 화면 밖일 수 있어서 여기만 알린다.
func (reg register) deletedMessage() string {
	if reg.linewise {
		return fmt.Sprintf("%d 줄 지웠습니다", len(reg.lines))
	}

	return fmt.Sprintf("%d 글자 지웠습니다", reg.charCount())
}

// charCount 는 글자 단위 register 에 담긴 글자 수다. byte 가 아니라 rune 으로 센다 —
// 한글 한 자가 3 글자로 세어지면 안 된다.
//
// 여러 줄에 걸친 것이면 사이의 줄바꿈도 한 글자다. 담긴 그대로가 붙여넣기로 나가는 글자다.
func (reg register) charCount() int {
	count := max(len(reg.lines)-1, 0)
	for _, line := range reg.lines {
		count += utf8.RuneCount(line)
	}

	return count
}
