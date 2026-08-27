# ADR-0080 언어는 경로가 아니라 제 필드가 든다

- 상태: 채택
- 날짜: 2026-08-26

## 맥락

`Buffer` 의 메서드 110 개를 훑다가 나온 것이다. `buf.path` 가 **신원 둘을 겸하고 있었다.**

| 쓰임 | 부르는 곳 |
| --- | --- |
| **디스크 신원**(어디에 쓰는가) | `os.ReadFile`·`os.WriteFile`·`Reload`·`checkOutside`·`saveTo` |
| **언어 신원**(어떤 문법인가) | `syntax.Detect`·`syntax.IndentFor`·`syntax.OutlineFor`·`resolveIndentUnit` |

겸직이 눈에 띈 것은 **캐시 둘이 `path` 를 키로 잡고 있어서**다. `syntaxCache.path` 와
`indentUnit.path` 가 「경로가 달라지면 다시 고른다」고 적혀 있는데, 정말 지키려는 것은
「**언어가** 달라졌나」다. 지금은 맞아떨어진다. 이름 없는 buffer 가 `:w foo.go` 로 이름을
받는 순간 언어도 그때 정해지므로 두 물음의 답이 같다. **우연히 맞은 것**이지 그렇게 정한
것이 아니고, 그런 자리는 언젠가 갈린다.

그리고 언어를 묻는 자리가 저마다 **표를 다시 훑고 있었다.** `syntax.Detect`·`IndentFor`·
`OutlineFor` 는 셋 다 `ruleFor(path)` 로 표의 한 줄을 찾은 뒤 칸 하나씩을 꺼내는 함수다.
한 파일에서 셋을 다 부르므로 같은 줄을 세 번 찾는다.

## 1. `syntax` 가 표 한 줄을 `Language` 로 내보낸다

찾을 것은 이미 한 자리에 모여 있었다. `languageRule` 이 그 줄이고, `state`·`indent`·
`outline` 세 칸이 언어별 값이다. **없던 것을 만드는 것이 아니라 있던 것에 이름을 붙여
내보내는 것이다.**

```go
type Language struct { ... }              // 표의 한 줄. 이름은 밖에서 보이지 않는다
func LanguageFor(path string) *Language   // 이름으로 그 줄을 찾는다. nil 이면 모르는 언어다

func (lang *Language) State() State       // 첫 칸
func (lang *Language) Indent() Indent     // 둘째 칸
func (lang *Language) Outline() Outline   // 셋째 칸
```

셋 다 **nil 인 채로 불러도 된다.** 모르는 언어가 nil 이고 그때 세 칸이 다 nil 인 것은
지금과 같은 답이라, 받는 쪽에 nil 검사를 하나도 더 넣지 않는다.

언어별 칸이 넷째로 늘어도 `Language` 에 메서드가 하나 늘 뿐 `Buffer` 는 그대로다.
「언어 하나가 표의 한 줄」이라는 것이 이 package 가 스스로 적어 둔 규칙이고, 그 줄을
통째로 건네는 것이 그 규칙을 밖에서도 지키는 길이다.

## 2. `Buffer` 는 `newBuffer` 에서 한 번 고른다

```go
type Buffer struct {
	path     string
	language *syntax.Language
	...
}
```

**게으르게 고르지 않는다.** 이름이 정해지는 자리가 셋뿐이고(`newEmptyBuffer`·`newBuffer`·
`saveTo`) 표를 훑는 것이 이름 하나 보는 일이라, 미루면 「아직 안 골랐다」와 「모르는
언어다」를 가르는 값이 하나 더 필요해진다. `indentUnit.set` 이 그 값이고 그것을 하나 더
만들 값이 없다.

**이름은 빈 것에서 이름으로만 바뀐다.** `saveTo` 가 `buf.path` 를 건드리는 것은 이름 없는
buffer 가 처음 저장될 때뿐이다(ADR-0024. 이름 있는 buffer 의 `:w <파일>` 은 사본만 쓴다).
그래서 언어도 nil 에서 한 번 정해지고 그 뒤로 안 바뀐다. `:saveas` 가 들어오면 그 자리가
늘어나므로 거기서 같이 고쳐야 한다. `saveTo` 에 적어 두었다.

## 3. `syntaxCache` 는 키를 잃는다

```go
type syntaxCache struct {
-	path  string
	start syntax.State
	...
}
```

`start` 는 이제 `buf.language.State()` 에서 **파생된다.** 필드 하나를 읽는 일이라 `lexSyntaxTo`
가 부를 때마다 그냥 다시 담는다. 표를 훑던 때는 담아 둘 값이 있었지만 지금은 없다.

담아둔 줄들을 버리는 것은 이름이 바뀌는 그 한 자리(`saveTo`) 가 한다. 캐시가 스스로
「내가 다른 언어의 것인가」를 물을 필요가 없어졌다.

주석 한 줄이 사라진다. 「이 비교 하나로 『아직 안 봤다』와 『강조하지 않는 파일이다』가
갈린다」. 두 사실을 한 비교에 겹쳐 두었던 것인데, 언어를 미리 고르면 「아직 안 봤다」가
아예 없다.

## 4. `indentUnit` 은 그대로 `path` 를 든다. 대칭이 아닌 것이 맞다

나눠 놓고 보니 캐시 둘의 사정이 다르다.

- `syntaxCache` 는 **언어만** 본다 → 키가 없어진다
- `indentUnit` 은 근거가 셋이다 → 키가 남는다

```go
func resolveIndentUnit(path string, lang *syntax.Language, lines [][]byte) []byte
//                     └ .editorconfig    └ 언어 규칙          └ 이미 쓰인 것을 잰다
```

인자 셋이 근거 셋과 **하나씩 짝지어졌다.** 원래 주석이 「근거 셋을 순서대로 본다」고 적어
두고 있었는데 그 셋 중 둘이 `path` 하나에 실려 있었다. 이제 서명이 그 문장과 같다.

`.editorconfig` 는 디렉터리를 거슬러 오르며 찾는 것이라 진짜 경로가 필요하다. 언어로 대신할
수 없고 대신해서도 안 된다.

## 5. `Detect`·`IndentFor`·`OutlineFor` 를 걷는다

셋을 남겨 두면 **경로로 언어를 묻는 문이 그대로 열려 있다.** 방금 `Buffer` 에서 걷어낸
겸직을 아무나 다시 만들 수 있고, 그것을 막는 것은 규율뿐이다.

`LanguageFor(path).State()` 로 바뀌므로 글자는 늘지만, **경로를 언어로 읽는 일이 한 줄에
한 번만 일어난다.** 시험 24 자리가 같이 바뀐다. 기계적인 치환이다.

`languageByName`(코드펜스의 ```` ```go ````) 은 그대로 둔다. 그것은 경로가 아니라 사람이
적은 이름을 보는 다른 문이고, 안에서 `Language` 를 쓴다.

## 고르지 않은 것

- **`isGoFile` 을 표로 끌어오기.** `gopls.go` 가 `strings.HasSuffix(path, ".go")` 로 따로
  답한다. 언어를 알아보는 법이 저장소에 둘이고, `ruleFor` 의 주석이 스스로 「이름을 보는
  법이 갈리면 한쪽만 아는 언어가 생긴다」고 적어 둔 그 일이다. 그래도 합치지 않았다:
  그것이 답하는 물음은 「gopls 에게 알릴 파일인가」이고 표가 답하는 것은 「강조·들여쓰기·
  뼈대 규칙이 있는 언어인가」다. **합치면 표가 물음 둘에 답하게 되어, 방금 `path` 에서
  걷어낸 겸직을 표에 다시 만드는 꼴이다.** 언어 지원과 LSP 지원이 실제로 어긋나는 날
  (`.pyi` 를 훑는데 gopls 는 모르는 것 같은) 이 오면 그때가 가르는 자리다.
  `docs/tasks.md` 에 적어 두었다
- **언어를 `Buffer` 가 고르게 하기**(`buf.detectLanguage()`). 고르는 규칙은 표에 있고
  `Buffer` 는 답만 든다. 고르는 일이 `Buffer` 로 오면 `syntax` 를 두 번 아는 것이 된다
- **`path` 를 아예 두 필드로 쪼개기.** 언어를 뗀 뒤 `path` 에 남은 것은 디스크 신원 하나라
  더 쪼갤 것이 없다. `goplsPath`·`editorconfigFor` 가 보는 것도 그 하나다
