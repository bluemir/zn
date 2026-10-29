# ADR-0160 rust 를 lexer·rust-analyzer 로 받치고, 저장할 때 맞추기를 서버에게 맡긴다

- 상태: 채택
- 날짜: 2026-10-29
- 관련: ADR-0039(줄 단위 문법 강조), ADR-0051(언어 서버), ADR-0065(저장 포매터), ADR-0066(자동완성), ADR-0080(언어는 표의 한 줄), ADR-0103(서버의 문법 토큰), ADR-0107(언어 서버는 표), ADR-0128(SaveFormat), ADR-0135(특수문자 별칭)

## 맥락

`.rs` 를 열면 통째로 흰 글자였고 자동완성도 없었다. 이 저장소에서 한 언어를 「지원한다」는 것은 lexer·언어 서버·자동완성·저장 포매터까지다(docs/tasks.md). python 이 그 깊이까지 갔고, rust 도 그 깊이로 간다.

정할 것은 넷이다.

1. lexer 가 무엇을 어떻게 칠하는가
2. 서버를 무엇으로 두고 어떻게 찾는가
3. 서버가 말하는 갈래 이름을 어떻게 읽는가
4. 저장할 때 무엇이 맞추는가

이 결정을 하면서 이 기계에 rustup 을 깔았다(`--profile minimal --no-modify-path`, 구성 요소 `rust-analyzer` `rustfmt`). rust-analyzer 1.99.1, rustfmt 1.10.0 이다. 아래의 「재서 본 값」은 그 판의 것이다.

## 1. lexer 는 손으로 짜고 이름 모양으로 어림한다

js lexer 와 같은 손짜기다. Go 처럼 표준 라이브러리에 scanner 가 있는 언어가 아니다.

**줄을 넘는 것이 넷이다.** 그래서 상태가 넷이다.

- `rustNormal{}`
- `rustBlockComment{depth}`: **블록 주석이 겹친다.** `/* 바깥 /* 안 */ 아직 */` 이다. 첫 `*/` 에서 닫으면 「아직」 뒤가 코드로 읽히고, 거기 든 따옴표가 아래 파일 전체를 문자열로 연다. 겹 수를 상태에 든다. 정수라 `==` 로 견줄 수 있다(syntax.go 의 State)
- `rustString{}`: **`"` 문자열이 줄을 넘는다.** Go·js 와 다르다. `b"`·`c"` 도 같은 상태다
- `rustRawString{hashes}`: `r#"..."#` 는 같은 수의 `#` 에서만 닫힌다. `#` 수를 상태에 든다

**`'` 는 문자와 lifetime 을 가른다.** 글자 하나 뒤에 `'` 가 오면 문자(`'a'`) 이고, 아니면 lifetime(`'a`) 이다. 그 글자가 한글이면 세 byte 라 rune 하나를 디코드해서 본다. `\` 로 열면 늘 문자다.

**갈래는 이름 모양으로 어림한다.** Rust 는 이름 짓기를 컴파일러가 경고로 지킨다. 그래서 모양이 꽤 믿을 만하다.

- CamelCase 는 type 이다. type·trait·variant 가 모두 그 모양이라 셋이 한 색이 된다. variant 와 type 은 서버가 붙으면 갈린다
- 대문자로만 지은 두 글자 넘는 이름(`MAX_LEN`) 은 상수다. `T` 하나는 generic 인자라 type 이다
- `Some`·`None`·`Ok`·`Err` 는 대문자여도 값이다. prelude 의 variant 라 표에 둔다
- 소문자 기본 type(`u32`·`str`) 은 모양으로 갈리지 않아서 표에 둔다
- `name!` 은 `!` 까지가 macro 이고 함수 색이다. `!=` 의 `!` 는 아니다
- `fn` 뒤와 `(` 앞의 소문자 이름은 함수다. `Some(` 처럼 대문자로 시작하면 부르는 것이 아니다
- `#[derive(..)]`·`#![allow(..)]` 는 attribute 이름까지가 함수 색이다. python 의 `@decorator` 와 같은 자리다. 괄호 안은 보통 코드로 훑는다. `Debug` 는 trait 이름이라 type 색이 된다
- lifetime 은 KindVariable 이다. type 색에 섞으면 `&'a str` 의 두 이름이 한 덩어리로 보인다

**들여쓰기는 rustIndent 를 따로 둔다.** braceIndent 는 `case`·`default` 로 시작하는 줄을 한 단계 내는데, Rust 에는 switch 이름표가 없고 `default: bool,` 같은 필드가 흔하다. nginx 가 따로 선 것과 같다. 세 괄호를 세고 단위는 space 네 칸(rustfmt 기본값) 이다.

## 2. 서버는 rust-analyzer, 찾는 자리에 `~/.cargo/bin` 을 더한다

표에 한 줄이 는다(lsp/server.go). 없을 때 깔 명령은 `rustup component add rust-analyzer` 다. rust 공식 길이고 toolchain 과 판이 맞는다.

**PATH 다음에 `CARGO_HOME/bin`(없으면 `~/.cargo/bin`) 을 본다.** rustup 을 셸 설정을 건드리지 않게 깔면 그 자리가 PATH 에 없다. gopls 의 GOBIN 과 같은 자리다.

**찾았다고 깔린 것이 아니다.** `~/.cargo/bin/rust-analyzer` 는 rustup 의 대리 실행 파일이라 구성 요소를 넣지 않아도 서 있다. 그때는 띄우기가 실패하고, 실패한 서버는 `\gd` 에서 설치를 묻는다. 그 길이 `rustup component add` 로 이어지므로 따로 묻지 않는다.

재서 본 값이다.

- **빈 capabilities 로 셋 다 온다.** 정의, 완성, 문법 토큰이다. gopls·pyright 와 같아서 능력을 채우지 않는다
- **악수 뒤 1~4 초 동안 답이 비어 있다.** crate 와 표준 라이브러리를 읽는 동안이다. gopls 는 첫 요청이 기다렸다가 답하는데 이쪽은 비어서 온다. 바쁠 때(시험을 한꺼번에 돌릴 때) 는 빈 답 대신 `content modified`(-32801) 오류가 왔다
- **완성 후보에 `textEdit` 범위가 실려 온다.** `greeter.gr` 에서 `gr` 을 덮는다. ADR-0066 의 넣기가 그대로 선다. snippet 능력을 알리지 않아서 `greet()` 가 아니라 `greet` 가 온다
- **crate 를 읽어야 답한다.** `Cargo.toml` 이 없는 자리에서 띄우면 정의도 완성도 오지 않는다

## 3. rust-analyzer 의 이름표를 표에 더한다

ADR-0103 은 「LSP 규격이 정한 이름이라 서버가 달라도 같은 표로 읽힌다」고 적었다. **rust-analyzer 는 그렇지 않다.** 악수에서 알린 이름표 쉰여섯 개 가운데 절반이 제 이름이다(`struct` `enum` `interface` `builtinType` `enumMember` `const` `boolean` `character` `lifetime` `selfKeyword` `decorator` `derive` `formatSpecifier` `escapeSequence` 등).

서버가 말한 줄은 lexer 의 답을 통째로 갈아끼운다(ADR-0103 §4). 그래서 **표에 없는 이름은 색이 빠지는 것으로 끝나지 않고 lexer 가 칠하던 색까지 지운다.** 서버가 붙으면 `Greeter`·`f64`·`'x'` 가 오히려 흰 글자가 된다. 그래서 더한다.

| 서버가 말한 것 | 우리 갈래 |
|---|---|
| `struct` `enum` `interface` `union` `typeAlias` `builtinType` `selfTypeKeyword` `derive` `toolModule` | `KindType` |
| `decorator` `attribute` `builtinAttribute` `attributeBracket` `procMacro` `deriveHelper` | `KindFunction` |
| `selfKeyword` | `KindKeyword` |
| `character` `escapeSequence` `formatSpecifier` | `KindString` |
| `boolean` `enumMember` `const` | `KindConstant` |
| `lifetime` | `KindVariable` |
| `static` | `variable` 과 같은 규칙 |

**문자열 조각은 한 색이다.** rust-analyzer 는 `"{} {c}\n"` 을 따옴표, `{`, `}`, `\n` 으로 잘라 보낸다. 하나라도 빠지면 문자열 가운데에 구멍이 난다.

**선언 표시가 `declaration` 이다.** gopls 는 `definition` 이라 적는다. 둘을 같이 받는다. gopls 의 수식 표에는 `declaration` 이 아예 없어서(재서 본 값이다) Go 의 색은 그대로다.

**`label`(`'outer:`) 은 그대로 그리지 않는다.** ADR-0103 이 Go 의 이름표를 그리지 않기로 했고, 그 이름을 더하면 Go 의 이름표에도 색이 붙는다. lexer 는 `'outer` 를 lifetime 과 가르지 못해 칠하므로, 서버가 붙으면 그 자리만 색이 빠진다.

## 4. 저장할 때는 rust-analyzer 가 맞춘다

**rustfmt 를 stdin 으로 돌리면 edition 2015 로 읽는다.** `async fn` 이 든 파일에서 `async fn is not permitted in Rust 2015` 로 실패한다(재서 본 값이다). 파일 경로를 넘겨도 같았다. rustfmt 는 Cargo.toml 을 보지 않는다. `cargo fmt` 는 edition 을 읽어 `--edition` 으로 넘겨서 이 문제가 없다.

길이 넷 있었다.

- **Cargo.toml 을 직접 읽기.** 파일 위로 올라가며 `edition = "..."` 줄을 찾는다. 0ms 다. 다만 「go.mod 를 우리가 읽지 않고 go 에게 묻는다」는 이 저장소의 태도(goToolDeclares) 와 어긋난다
- **`cargo metadata` 에게 묻기.** workspace 상속까지 cargo 가 푼다. 폴더마다 처음 한 번 280ms 다
- **`rustfmt.toml` 에 맡기기.** stdin 으로 돌려도 그 파일의 edition 은 따른다(재서 확인했다). 그 파일이 없는 저장소가 대부분이다
- **rust-analyzer 에게 맡기기(`textDocument/formatting`).** 서버는 crate 를 이미 읽어 edition 을 안다

**서버에게 맡긴다.** 무엇을 아는 쪽에게 묻는 태도에 가장 가깝다. 재 보니 빈 capabilities 로 답하고, 저장하지 않은 buffer 글을 맞추고, 2021 crate 의 `async fn` 을 통과시킨다. 한 번에 20ms 쯤이다.

**창 쪽 저장 길은 늘지 않는다.** 창은 `SaveFormat{Name, Run}`, 곧 「byte 를 넣으면 byte 가 나온다」만 안다(ADR-0128). 서버에게 맡기는 길도 그 모양으로 싼다(lsp 의 Format).

1. 받은 글을 서버에 전문으로 맞춘다. 서버가 든 판이 그 글이어야 받은 편집이 그 글의 자리를 가리킨다
2. formatting 을 묻는다
3. 받은 편집 목록을 그 글에 얹어 돌려준다

**편집은 줄을 넘는다.** `{` 앞뒤의 줄바꿈을 넣고 빼는 것이라 거의 언제나 그렇다. rename 이 쓰는 applyEdits 는 한 줄 안의 편집만 받아서 새로 짰다. 글 전체의 byte offset 으로 바꿔 뒤에서 앞으로 얹는다.

**기한은 바깥 포매터와 같은 2 초다.** 저장은 이 답을 기다리며 화면을 멈춘다. 연결의 `call` 에는 기한이 없어서 Format 이 따로 둔다.

**표의 갈래.** 서버 표에 `formats` 칸을 두고 rust-analyzer 만 참이다. 저장할 때 바깥 포매터 표(core 의 formatters) 를 먼저 보고, 줄이 없으면 그 칸을 본다. Go 는 gopls 가 떠 있어도 goimports 가 맞춘다.

## 결과

- **맞추지 않고 조용히 쓰는 때가 있다.** 넷 다 저장 문구에 아무것도 붙지 않는다
	- 서버가 아직 뜨지 않았을 때
	- crate 를 읽는 동안. `null` 이 온다
	- 서버가 바쁠 때. `content modified` 가 온다. 되묻고 기다리면 저장이 그만큼 더 멈춰서 그대로 둔다
	- 문법이 깨졌을 때. rust-analyzer 가 오류가 아니라 `null` 로 답한다(재서 본 값이다). goimports·ruff 는 이때 까닭을 알리는데 rust 는 알리지 않는다
- **저장에서 설치를 묻지 않는다.** 바깥 포매터 줄이 없어서 errHookNotInstalled 가 생기지 않는다. 서버를 까는 물음은 `\gd` 의 몫이다. rustfmt 구성 요소가 없을 때 무엇이 오는지는 재지 않았다
- **`::` 바로 뒤는 특수문자 별칭 자리가 아니다.** `HashMap::ne` 의 `:ne` 가 별칭으로 읽혀 서버에 묻지 않았다(ADR-0135 의 inSymbolAlias). 걸리는 별칭이 없어도 목록을 닫고 끝나서 `::` 뒤에서는 완성이 끝내 뜨지 않았다. `::` 앞의 `:` 를 보고 가른다
- **설치 명령은 PATH 의 `rustup` 을 찾는다.** `--no-modify-path` 로 깐 기계에서는 그 명령을 못 찾는다고 알린다. 서버를 찾는 자리와 달리 `~/.cargo/bin` 을 보지 않는다(docs/tasks.md)
- 트리 색(sidebar 의 확장자 색) 에는 넣지 않았다. go.mod 과 같은 건이다

## 기각한 것

- **braceIndent 를 나눠 쓰기.** §1 에 적었다
- **lexer 가 `'outer` 를 label 로 가르기.** `loop`·`while`·`for` 앞의 `'x:` 와 `break`·`continue` 뒤를 봐야 한다. 갈라도 서버가 붙으면 그 자리는 색이 빠지므로(§3) 얻는 것이 lexer 만 있을 때뿐이다
- **Cargo.toml 직접 읽기, `cargo metadata`, `rustfmt.toml`.** §4 에 적었다
