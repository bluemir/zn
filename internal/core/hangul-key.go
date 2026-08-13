package core

// hangulKeys 는 한글 입력 상태에서 온 키를 두벌식 자리의 영문 키 나열로 바꾼다.
// 한글이 아니면 nil 이다 — 부르는 쪽은 원래 키를 그대로 쓴다.
//
// 키 하나가 여럿으로 풀린다. 입력기가 자모를 모아서 주기 때문이다.
// `hk`(왼쪽·위) 를 치면 `ㅘ` 한 글자로 오고 `dj` 는 `어` 로 온다. 그래서 음절과 겹자모를
// 자모 단위로 되돌려야 원래 누른 키를 알 수 있다(ADR-0008).
func hangulKeys(key string) []string {
	// `ctrl+w` 처럼 이름으로 오는 키는 한글이 섞일 수 없어서 그대로 nil 이 된다.
	keys := make([]string, 0, len(key))
	hangul := false

	for _, c := range key {
		mapped, ok := jamoKeys[c]
		if !ok {
			mapped, ok = syllableKeys(c)
		}
		if !ok {
			// 한글이 아닌 글자는 그대로 둔다.
			keys = append(keys, string(c))

			continue
		}

		hangul = true
		for _, k := range mapped {
			keys = append(keys, string(k))
		}
	}

	if !hangul {
		return nil
	}

	return keys
}

// jamoKeys 는 자모 하나가 두벌식 자리에서 어느 키인지다.
// 겹자모는 그것을 만드는 키 둘이다 — `ㅘ` 는 `h` 와 `k` 를 이어 누른 것이다.
//
// shift 자리가 따로 없는 자모(`ㅎ`, `ㅠ`) 는 shift 를 눌러도 같은 글자가 와서 대문자 키와
// 구분할 수 없다. 그래서 `G` 와 `B` 는 한글 상태에서 낼 수 없다. `W` `E` `T` 처럼
// shift 자리에 된소리가 있는 것은 낼 수 있다.
var jamoKeys = map[rune]string{
	'ㄱ': "r", 'ㄲ': "R", 'ㄴ': "s", 'ㄷ': "e", 'ㄸ': "E", 'ㄹ': "f", 'ㅁ': "a",
	'ㅂ': "q", 'ㅃ': "Q", 'ㅅ': "t", 'ㅆ': "T", 'ㅇ': "d", 'ㅈ': "w", 'ㅉ': "W",
	'ㅊ': "c", 'ㅋ': "z", 'ㅌ': "x", 'ㅍ': "v", 'ㅎ': "g",

	'ㄳ': "rt", 'ㄵ': "sw", 'ㄶ': "sg", 'ㄺ': "fr", 'ㄻ': "fa", 'ㄼ': "fq",
	'ㄽ': "ft", 'ㄾ': "fx", 'ㄿ': "fv", 'ㅀ': "fg", 'ㅄ': "qt",

	'ㅏ': "k", 'ㅐ': "o", 'ㅑ': "i", 'ㅒ': "O", 'ㅓ': "j", 'ㅔ': "p", 'ㅕ': "u",
	'ㅖ': "P", 'ㅗ': "h", 'ㅛ': "y", 'ㅜ': "n", 'ㅠ': "b", 'ㅡ': "m", 'ㅣ': "l",

	'ㅘ': "hk", 'ㅙ': "ho", 'ㅚ': "hl", 'ㅝ': "nj", 'ㅞ': "np", 'ㅟ': "nl", 'ㅢ': "ml",
}

// 조합된 음절이 자리한 유니코드 구간이다. `가` 부터 `힣` 까지 초성·중성·종성 순으로 늘어선다.
const (
	syllableFirst = '가'
	syllableLast  = '힣'

	// jongCount 는 종성 자리의 가짓수다. 종성이 없는 경우가 0 번이라 자모 수보다 하나 많다.
	jongCount = 28
)

// 음절을 가르는 초성·중성·종성의 순서다. 유니코드가 정한 순서라 자리를 옮기면 안 된다.
var (
	choJamo  = []rune("ㄱㄲㄴㄷㄸㄹㅁㅂㅃㅅㅆㅇㅈㅉㅊㅋㅌㅍㅎ")
	jungJamo = []rune("ㅏㅐㅑㅒㅓㅔㅕㅖㅗㅘㅙㅚㅛㅜㅝㅞㅟㅠㅡㅢㅣ")
	jongJamo = []rune("ㄱㄲㄳㄴㄵㄶㄷㄹㄺㄻㄼㄽㄾㄿㅀㅁㅂㅄㅅㅆㅇㅈㅊㅋㅌㅍㅎ")
)

// syllableKeys 는 조합된 음절 하나를 그것을 만든 키 나열로 되돌린다. 음절이 아니면 false 다.
func syllableKeys(c rune) (string, bool) {
	if c < syllableFirst || c > syllableLast {
		return "", false
	}

	index := int(c - syllableFirst)
	cho := index / (len(jungJamo) * jongCount)
	jung := index / jongCount % len(jungJamo)
	jong := index % jongCount

	keys := jamoKeys[choJamo[cho]] + jamoKeys[jungJamo[jung]]

	// 종성 없음이 0 번이라 자모는 한 자리 밀려 있다.
	if jong > 0 {
		keys += jamoKeys[jongJamo[jong-1]]
	}

	return keys, true
}
