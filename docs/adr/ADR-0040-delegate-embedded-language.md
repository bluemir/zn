# ADR-0040: 코드펜스와 raw text 안은 안쪽 언어의 문맥을 품어 위임한다

- 상태: 채택
- 날짜: 2026-08-21

## 맥락

ADR-0039 로 문법 강조가 들어갔지만 두 자리를 일부러 비워 뒀다. markdown 코드펜스 안과 html 의
`<script>`·`<style>` 안이다. 그때 적은 근거는 이랬다.

> 이 저장소의 fenced block 30 개 중 언어 이름이 붙은 것이 8 개, 그중 lexer 가 있는 것이 6 개다.
> 여섯을 위해 상태 공간을 언어 수만큼 곱하지 않는다.

**그 셈이 틀렸다.** 상태 공간은 언어 수만큼 곱해지지 않는다. 늘어나는 것은 이미 있는 상태 둘
(`mdFence`, `htmlRawText`) 에 붙는 interface 칸 **하나**이고, 닫는 줄이 그 사슬을 통째로 버린다.
언어를 아홉으로 늘려도 칸은 그대로 하나다.

그리고 파헤치는 동안 **버그를 찾았다.** `htmlScan` 이 여는 tag 를 닫는 순간 곧바로 돌아가서
그 줄의 나머지를 보지 않았다. `<script>x</script>` 처럼 한 줄에서 닫히면 같은 줄의 `</script>`
를 못 보고 **파일 나머지 전부가 script 안으로** 읽혔다. 기존 시험이 닫는 tag 를 늘 다음 줄에만
두어서 못 잡았다.

## 결정

### 여러 줄에 걸친 상태가 안쪽 언어의 문맥을 품는다

```go
type mdFence struct {
	marker byte
	size   int
	inner  State // nil 이면 언어를 모르는 것이라 색을 입히지 않는다
}

type htmlRawText struct {
	tag   string
	inner State
}
```

`State` 가 `==` 로 견줄 수 있어야 하는 제약(ADR-0039) 은 그대로다. interface 칸은 안에 견줄 수
있는 struct 만 들어오면 견줄 수 있고, 우리 상태 전부가 그렇다.

### 닫는 표시가 안쪽 언어보다 세다

`mdFence.Lex` 는 **닫는 펜스를 먼저 보고** 그다음에 위임한다. `htmlRawText.Lex` 도 닫는 tag 를
먼저 찾는다. 이 순서가 아니면 안쪽에서 열린 raw string 이나 블록 주석이 닫는 표시를 먹어서
블록이 영영 닫히지 않고, 그때부터 문서 나머지가 코드로 그려진다.

닫힐 때 안쪽 문맥은 **버린다.** 블록이 끝나면 안쪽에서 열려 있던 것도 같이 끝난다. 브라우저가
`</script>` 를 js 문자열 안에서도 블록의 끝으로 읽는 것과 같다.

### 모르는 언어는 nil 이고, nil 은 「색 없음」이다

`languageByName` 이 모르는 이름에 nil 을 준다. 위임하는 쪽이 nil 을 「색 없음」으로 받으므로,
이름 없는 펜스와 모르는 언어의 펜스가 **위임을 넣기 전과 한 글자도 다르지 않게** 그려진다.
이 저장소의 펜스 31 개 중 22 개가 그 자리다.

### 언어 이름은 규칙 표의 세 번째 칸이다

info string 은 경로가 아니라 이름이라 `Detect` 를 쓸 수 없다 — `go` 는 확장자도 파일 이름도
아니다. `languageRule` 에 `aliases` 를 더했다. 이름 표를 따로 만들면 언어를 더할 때 손댈 자리가
둘이 되므로, 「언어 하나가 표의 한 줄」을 지켰다.

`aliases` 만 본다. 파일 이름(`names`) 까지 견주면 ```` ```gnumakefile ```` 이 우연히 되면서
두 칸의 뜻이 흐려진다.

### 자리(offset) 되돌리기는 이미 있는 것으로 된다

세 경우가 있고 셋 다 새 helper 가 필요 없다.

| 맡기는 조각 | 방법 | 되돌릴 것 |
|---|---|---|
| 줄 전체 (펜스 안) | `s.inner.Lex(line)` | 없음 |
| 앞쪽 조각 (`</script>` 앞) | `s.inner.Lex(line[:at])` | 없음 |
| 가운데 조각 (같은 줄에서 여닫음) | `lexTail(inner, line[:end], at)` | `lexTail` 이 한다 |

셋째가 눈에 띈다. `lexTail(state, line, from)` 에 **짧게 자른 줄**을 주면 정확히
`line[from:end]` 를 훑고 `from` 만큼 되돌린다. 가운데 조각에도 같은 함수가 그대로 쓰인다.

### shell lexer 를 같이 넣었다 — heredoc 없이

`sh` 펜스가 이 저장소의 태그된 펜스 9 개 중 2 개인데 lexer 가 없었다. 상태 둘이다 —
`shNormal` 과 `shQuoted{quote byte}`. shell 의 따옴표는 **줄을 넘는다**(js·python 의 한 겹
따옴표와 다른 자리다).

heredoc(`<<EOF`) 은 넣지 않았다. `<<` 가 here-string(`<<<`) 과 산술 shift(`$((a << 2))`) 와
헷갈려 가장 틀리기 쉬운 자리이고, Dockerfile 의 `RUN <<EOF` 와 같이 봐야 한다.

`shell.go` 가 이미 make·docker 가 같이 쓰는 helper 파일이라 `expansion.go` 로 옮기고 그 이름을
언어에 줬다. `shellExpansionEnd` 를 shell 이 그대로 쓴다 — 이제 그 helper 가 셋이 나눠 쓴다.

### `<script>` 는 늘 js, `<style>` 는 늘 css 다

`type` 속성을 보지 않는다. 보려면 속성 값을 고리 안에서 기억해 `>` 를 만나는 자리까지 들고
가야 하는데, 틀렸을 때 피해가 작다 — `type="application/json"` 을 js 로 훑으면 그럴듯한 색이
나오고 자리는 가운데 조각 안이라 벗어나지 않는다.

## 근거

**경계가 그대로 버텼다.** `internal/core` 를 **한 줄도 고치지 않았다.** 위임이 `Kind` 를 늘리지
않고 `State` 안에서만 일어나기 때문이다. ADR-0039 가 「syntax 는 색을 모른다」로 그은 선이
이번에 값을 냈다.

**시험이 미리 있어서 공짜로 지켜졌다.** `languageSamples` 에 위임이 일어나는 줄을 넣자
`TestTokenInLine`·`TestTokenClusterBoundary`·`TestTokenOrder` 가 위임 뒤의 자리 계산을 그대로
지켰다. 자리를 잘못 되돌리면 그리는 쪽이 없는 byte 를 집어 죽는 자리다.

**`reflect` 로는 부족해졌다.** `TestStateComparable` 의 `reflect.TypeOf(state).Comparable()` 은
interface 칸을 든 struct 에 대해 안에 무엇이 들었든 `true` 를 준다 — `cssComment{back State}`
때문에 **이미 헛돌고 있었다.** 위임이 그 구멍을 떠받치는 자리로 만들었으므로, 실제로 `==` 를 해
보는 시험(`TestStateEqualityDoesNotPanic`) 과 그 시험이 헛돌지 않는지 보는 시험
(`TestUncomparableStatePanics`) 을 위임보다 **먼저** 세웠다.

## 결과

- `internal/syntax/shell.go` 가 새로 생겼고(언어 아홉 번째), 옛 `shell.go` 는 `expansion.go` 가
  되었다. `detect.go` 에 `aliases` 칸과 `languageByName` 이 늘었다.
- `mdFence`·`htmlRawText` 에 `inner` 칸이 붙고 각자의 `Lex` 가 위임한다. `htmlScan` 의 `>`
  가지가 같은 줄의 닫는 tag 를 보게 되었다 — 위 버그가 여기서 고쳐진다.
- `htmlRawTextTags map[string]bool` 이 `htmlRawTextLanguages map[string]State` 가 되었다.
  없는 key 가 nil 이라 표 하나가 「raw text 인가」와 「어느 lexer 인가」에 같이 답한다.
- 태그된 펜스 9 개(`go` 7, `sh` 2) 가 전부 색을 얻었다.

감수하는 것

- **여는 펜스의 들여쓰기를 벗기지 않는다.** CommonMark 는 벗기는데, 그러려면 칸이 하나 더 붙어
  모든 캐시 줄의 상태 사슬에 실리고 `==` 에 참여한다. 우리 lexer 중 앞 공백을 보는 것은
  makefile 의 조리법 판정 하나뿐이라, 틀리려면 「들여쓴 펜스 안의 makefile」이어야 한다.
- **중첩 깊이에 상한이 없다.** 여는 줄만 이어지면 문맥 사슬이 줄 수만큼 깊어지고 `==` 가
  그만큼 걷는다. 상한을 두면 모든 markdown 파일이 그 칸을 들고 다니고 깊은 경우를 조용히
  틀리게 훑는다 — 느린 것과 틀린 것을 맞바꾸지 않는다. 닫는 펜스 한 줄이 사슬을 통째로 버리므로
  실제 문서에서는 1~2 다.
- **수렴이 블록 안에서 조금 나빠질 수 있다.** 전에는 펜스 안의 모든 줄이 똑같은
  `mdFence{marker,size}` 를 내서 편집 다음 줄에 바로 수렴했다. 이제 나가는 문맥이 안쪽 언어의
  문맥을 같이 들어서, 그것이 줄마다 흔들리는 블록(css 의 여러 줄 선언, js 의 template 여닫기) 은
  그 줄들에서 수렴하지 않는다. **다만 피해 범위가 블록 끝에서 멈춘다** — 닫는 줄은 안쪽 문맥과
  무관하게 `mdNormal{}`·`htmlNormal{}` 을 내므로 그 줄에서 반드시 수렴한다.
- **`</script>` 앞까지만 넘기므로 잘린 문자열이 미완성으로 남는다.** `const s = "</script>";`
  에서 열린 `"` 가 미완성 문자열 토큰이 된다. 브라우저도 여기서 script 를 끝내므로 맞는 동작이다.

## 대안

**안쪽을 통째로 문자열 색으로 두기** — 위임 없이 「코드처럼 보이게」 하는 가장 싼 길이다.
고르지 않았다. 코드블록이 서른 줄씩 한 색으로 덮이면 색이 없는 것보다 읽기 나쁘다. ADR-0039 가
안쪽을 **색 없이** 둔 것이 이 판단이었고, 그래서 이번에 위임을 넣으면서 되돌릴 것이 없었다.

**펜스마다 언어별 상태 type 을 따로 두기**(`mdFenceGo`, `mdFenceJS` …) — interface 칸이 없어서
`==` 걱정이 사라진다. 고르지 않은 이유는 그것이 정말로 상태 공간을 언어 수만큼 곱하기 때문이다.
ADR-0039 가 걱정했던 것이 이 모양이었고, interface 칸 하나로 그 걱정이 사라진다.

**`Detect` 를 이름도 받게 넓히기** — 함수 하나로 끝난다. 고르지 않은 이유는 「경로를 본다」와
「이름을 본다」가 다른 물음이고, 한 함수가 둘을 하면 `Detect("go")` 가 무엇을 뜻하는지 흐려진다.

## 이 ADR 이 정하지 않는 것

- **Makefile 조리법과 Dockerfile `RUN` 본문을 shell 로 잇는 것.** shell lexer 가 생기면서
  가능해졌다. 지금은 값 참조만 집는다.
- **heredoc**(`<<EOF`).
- **`<script type="...">` 을 가려 보는 것.**
- **info string 의 첫 낱말만 색을 줄지.** 언어는 첫 낱말로 고르는데 색은 전체에 붙는다 —
  ```` ```go title=x ```` 가 통째로 type 색이다.
- **중첩 깊이 상한.**
