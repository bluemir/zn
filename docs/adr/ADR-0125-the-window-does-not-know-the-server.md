# ADR-0125: 창은 언어 서버를 모른다

## 맥락

`Buffer`+`viewport` 를 새 패키지로 낼 수 있는지 재던 갈래다(ADR-0124). 그 무리가 `internal/lsp` 를 네 파일에서 만지고 있었다.

| 자리 | 무엇 |
|---|---|
| `buffer.go` | `diagnostics map[int][]lsp.Diagnostic` 필드 |
| `diagnostics.go` | 담고 줄로 묻는 셋 |
| `semantic.go` | 서버 토큰을 강조 캐시에 얹기 |
| `viewport-completion.go` | 고른 후보를 줄에 넣기 |

「창은 마커의 내용을 알면 충분하고 언어 서버가 그것을 만들었다는 것은 알 필요가 없다」가 이 결정의 출발이다.

## 1. 문이 필요 없었다. 미는 쪽이다

`markerSource` 같은 interface 를 두는 길을 먼저 봤는데, **창이 서버에 물어본 적이 없었다.** 셋 다 editor 가 받아서 밀어 넣는다.

```go
e.buffers[i].setDiagnostics(client.Diagnostics(path))   // diagnostics.go
e.buffers[i].setSemanticTokens(msg)                     // semantic.go
e.activeBuffer().insertCompletion(item)                 // completion.go
```

당기는 문은 이미 editor 쪽에 있고 그 자리도 하나뿐이라, 추상할 것이 없다. 남은 것은 **넘기는 값의 type** 이었다.

## 2. 셋의 성질이 달랐다

| | 창이 하는 일 | `lsp` 가 왜 필요했나 |
|---|---|---|
| 진단 | 줄로 모아 담고 줄로 묻는 물음에 답한다 | **자료 셋뿐** — `Range.Start.Line`·`Severity`·`Message` |
| semantic | 서버 토큰을 `syntax.Token` 으로 바꿔 캐시에 넣는다 | 자료 + **UTF-16 열을 byte 로 바꾸는 셈** |
| 자동완성 | 받은 범위 자리를 갈아끼운다 | 같은 셈 + `TextEdit`·`Text()` |

진단은 자료 type 만 바꾸면 끊긴다. **뒤의 둘은 안 끊긴다** — 서버의 열은 UTF-16 이고 그것을 byte 로 옮기려면 그 줄의 글자가 필요한데, 줄을 든 것이 창이라서다.

## 3. 환산기를 내리려다 물렸다

`lsp.ByteColumn`·`UTF16Column` 을 core 로 내리면 뒤의 둘도 끊긴다고 보고 옮겼다가 되돌렸다.

**`internal/lsp` 의 시험이 그 둘을 열두 번 쓴다**(`gopls_test.go` 8, `document_test.go` 1, 그 밖). 그 시험들은 core 를 import 할 수 없다 — core 가 lsp 를 import 하니 순환이다. 그리고 그 패키지는 이미 제 안에 `utf16Len` 을 들고 있다. **UTF-16 을 세는 일은 그 패키지의 것이다.**

「부르는 열한 곳이 전부 core 다」라고 잰 것이 틀렸다. **시험을 안 셌다.**

## 4. editor 가 옮겨서 넘긴다

환산기를 내리는 대신 **editor 가 미리 옮긴다.** editor 는 세 자리 모두에서 그 창을 손에 들고 있으므로 줄의 글자를 볼 수 있다.

```go
// 진단: 열 환산이 없다. 자료만 옮긴다
func diagnosticsOf(list []lsp.Diagnostic) []diagnostic

// semantic: buf.lines 로 열을 옮겨 syntax.Token 으로 만들어 넘긴다
func (e *editor) applySemanticTokens(msg semanticTokensMsg)
func (buf *Buffer) setSemanticTokens(from, to int, tokens []semanticToken)

// 자동완성: buf.lines 로 범위를 옮겨 (시작, 끝, 넣을 글) 로 넘긴다
func (buf *viewport) insertCompletion(start, end int, text []byte)
```

창이 새로 드는 자료는 셋이다.

```go
type diagnostic struct{ line int; severity diagnosticSeverity; message string }
type diagnosticSeverity int   // LSP 눈금 그대로. 작을수록 심하다
type semanticToken struct{ line int; token syntax.Token }   // 열이 이미 byte 다
```

## 규모

| | 자리 |
|---|---|
| 진단 | 30 남짓 (절반이 시험) |
| semantic | 메서드 하나가 갈라져 옮김 + 시험 여섯이 editor 를 지나게 됨 |
| 자동완성 | 메서드 하나 + 시험 다섯 |

동작은 하나도 안 바뀐다. 앱에서 진단 마커(`✖`)·아래 줄 문구·문법 강조·자동완성(`e.has` → `e.hasGitBase`) 을 직접 몰아 확인했다.

## 얻은 것

**옮길 무리 114 개 이름 중 `lsp` 를 만지는 것이 하나도 없다.** `buffer.go` 는 그 import 를 아예 잃었다.

`diagnostics.go`·`semantic.go` 두 파일에는 아직 남아 있는데, 그 파일이 창 쪽과 editor 쪽을 같이 담고 있어서다. 파일이 걸친 것이지 이름이 걸친 것이 아니다 — 패키지를 가를 때 그 선을 따라 갈린다.

무리가 밖으로 import 하는 것은 이제 `scheme`(13)·`syntax`(8) 둘이다.

## 남는 것

- **`semanticKind`** 가 editor 쪽으로 갔다. LSP 토큰 이름을 색 갈래로 옮기는 표라 그쪽이 맞는데, `style.go` 의 색 결정과 나란히 두는 것이 나은지는 따로 본다
- **editor 가 `buf.lines` 를 더 자주 읽는다.** 패키지를 가르면 그 필드가 밖으로 열려야 한다. 이미 API 면에 들어 있던 것이다(33 곳)
- **`table.go`** 가 `syntax.MarkdownTableCells` 를 부른다. 언어 이름이 문에 박힌 유일한 자리이고, `Language` 의 넷째 칸이 될 자리다

## 되짚은 것

**「core 밖에는 없다」를 시험을 빼고 셌다.** 같은 잘못을 이 갈래에서 두 번째로 했다 — ADR-0123 에서 `pane` 을 지을 때는 저장소를 안 찾아봤고, 여기서는 찾아보되 `_test.go` 를 뺐다. **의존을 잴 때 시험은 코드다.**

그리고 무를 수 있게 해 둔 것이 값을 냈다. 옮긴 것이 커밋 전이라 `git checkout` 한 번으로 되돌아왔다.
