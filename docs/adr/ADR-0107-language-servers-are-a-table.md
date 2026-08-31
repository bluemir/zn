# ADR-0107 언어 서버를 표로 두고 python 을 한 줄로 더한다

- 상태: 채택
- 날짜: 2026-08-31
- 관련: ADR-0051(gopls 를 띄우고 맞춘다), ADR-0065(저장할 때 바깥 포매터에 통과시킨다), ADR-0066(insert 자동완성), ADR-0067(이름 바꾸기), ADR-0068(사용처로 가기), ADR-0080(언어를 표의 한 줄로 든다), ADR-0086(진단), ADR-0092(죽은 서버 되살리기), ADR-0103(문법 갈래를 언어 서버에게 묻는다)

## 맥락

`docs/tasks.md` 의 「언어 지원 추가」가 그 뜻을 이렇게 적어 두었다.

> 강조 lexer 는 일부분일 뿐이고 언어 서버·자동완성·저장 hook 까지가 한 언어를 「지원한다」는 뜻이다. 지금 거기까지 간 것은 Go 하나다

python 은 lexer·들여쓰기·뼈대가 이미 있었다(`internal/syntax/python.go`). 없던 것은 언어 서버와 그에 딸린 것들, 그리고 저장 hook 이다. 같은 목록에 미결정 항목도 있었다.

> js·python 에도 언어 서버를 붙일지 정한다. 문법 갈래 표가 LSP 규격의 이름이라 붙이면 문법 강조가 그대로 열린다

**그 마지막 문장은 pyright 에는 틀렸다.** 재보니 pyright 는 문법 토큰을 아예 알리지 않는다. 아래 §4 에 적는다.

## 1. 서버가 하나라는 가정이 열여섯 파일에 퍼져 있었다

ADR-0051 이 gopls 하나를 두고 짠 자리다. `editor` 가 `gopls`·`goplsStarting`·`goplsFailed`·`goplsDeaths` 를 직접 들고, `isGoFile` 이 확장자를 보고, `lsp.Start` 가 `findGopls` 와 `-mode=stdio` 와 `LanguageID: "go"` 를 못박고 있었다. 그 자리를 만지는 코드가 열여섯 파일이었다.

필드를 하나 더 두는 길(`e.gopls` 와 `e.pyright`) 을 보았다가 접었다. 그러면 그 열여섯 파일이 전부 분기 둘이 되고, 셋째 언어에서 다시 같은 일이 생긴다. **서버를 더할 자리가 한 곳으로 남는 쪽**을 골랐다.

- `lsp.Server` 는 표의 한 줄이자 서버 그 자체다. 이름·확장자·languageID·실행 인자·악수 설정·찾는 법·깔는 법을 한 줄이 든다. syntax 의 언어 표와 같은 손이다(ADR-0080).
- `editor` 는 이름→상태 map 하나를 든다(`servers`). 뜨는 중인지·실패했는지·몇 번 죽었는지가 **서버마다 따로**다. 하나로 묶으면 gopls 가 깔려 있지 않은 것 때문에 pyright 도 함께 물러난다.
- 부르는 쪽은 `serverPath(path)` 로 「이 파일의 서버와 절대 경로」를 한 번에 받는다. 확장자를 보는 자리가 `lsp.ServerFor` 하나로 모인다.

`gopls` 로 지어져 있던 이름을 언어 서버 일반 이름으로 옮겼다(`gopls.go` → `language-server.go`, `syncGopls` → `syncServers`, `goplsClient` → `clientOf`). 이름이 서버 하나를 가리키는 채로 여럿을 들면 그것이 곧 거짓말이 된다.

## 2. 이 표는 syntax 의 언어 표와 따로 산다

`isGoFile` 이 syntax 의 표를 빌려 오지 않은 까닭을 ADR-0051 이 적어 두었고, 그것을 그대로 지킨다. 저쪽은 lexer 를 고르는 표이고 이쪽은 「어느 언어 서버가 보는가」다. 합치면 서버 없는 언어 아홉 줄에 빈 칸이 생기고, 무엇보다 **lexer 를 더할 때마다 서버가 조용히 붙거나 떨어진다.**

확장자는 두 표가 지금 같은 글자를 본다(`.go`, `.py`). `.pyi` 를 서버 쪽에만 넣는 것을 보았다가 넣지 않았다 — 그러면 강조 없이 진단만 붙는 파일이 생기고, 「python 파일」의 뜻이 두 표에서 갈린다.

## 3. 서버마다 자기 파일만 받는다

서버가 여럿이 되면서 새로 생긴 규칙이다. 한 서버에 남의 언어 파일을 알리면 그 서버가 그것을 자기 언어로 읽으려 든다. 그래서 파일을 서버별로 갈라 보내는 자리가 넷이다.

- 맞추기(`syncServers`) 는 열린 목록을 서버별로 갈라 각자에게 그 몫만 준다
- 진단(`applyDiagnostics`) 은 파일마다 자기 서버에게 묻는다
- 디스크 감시(`serverWatchFiles`) 와 HEAD 이동(`gitTreeChanges`) 은 바뀐 파일을 서버별로 담아 각자에게 보낸다

진단을 기다리는 고리도 서버마다 하나다. `diagnosticsMsg` 가 **어느 서버가 울렸는지**를 싣는다 — 싣지 않으면 고리를 다시 걸 상대를 알 수 없고, 남의 고리를 다시 걸면 그 서버의 고리가 둘이 되면서 울린 쪽이 끊긴다.

## 4. pyright 는 문법 토큰을 주지 않는다. 그것이 대가가 아니다

재본 것이다. 빈 능력으로도, `textDocument.semanticTokens` 능력을 알려도, 악수 응답에 `semanticTokensProvider` 가 없다. `semanticTokens/full` 도 빈 답이다.

그래서 `docs/tasks.md` 가 적어 둔 「붙이면 문법 강조가 그대로 열린다」는 pyright 에는 성립하지 않는다. **python 의 색은 지금처럼 우리 lexer 가 맡는다.**

이것이 값을 물리지 않는 것은 ADR-0103 이 이미 그 경우를 받아 두었기 때문이다. `SemanticTokens` 는 악수에서 받은 이름표가 비면 아무것도 묻지 않고, 서버가 말하지 않은 줄은 lexer 의 답이 그대로 남는다. 코드에 손댈 것이 없었다.

`internal/lsp/pyright_test.go` 가 이 사실을 시험으로 못 박는다. python 의 색이 어느 쪽 것인지가 여기에 매달려 있어서다 — 어느 판에서 토큰이 오기 시작하면 색을 내는 자리가 조용히 갈리고, 그때는 그 판을 보고 정할 일이다.

## 5. pypi 의 `pyright[nodejs]` 다. python 만 있으면 깔린다

후보 셋을 재서 골랐다. 값은 이 기계(macOS, 워밍 후) 에서 잰 것이다.

| | pyright (npm) | **pyright[nodejs] (pypi)** | basedpyright (pypi) |
|---|---|---|---|
| 무엇만 있으면 깔리나 | node + npm | **python** | python |
| node 없는 PATH | 안 됨 | **됨** | 됨 |
| 악수 | 280ms | **114ms** | 400ms |
| 정의·사용처·이름 바꾸기·자동완성 | 됨 | 됨 | 됨 |
| `textDocumentSync` | 2(증분) | 2(증분) | 2(증분) |
| 진단(빈 능력) | 1 개 | **1 개** | **4 개** |
| 문법 토큰 | 없음 | 없음 | 있으나 `range` 없음 |

**npm 쪽을 버린 것은 python 개발자 기계에 node 가 없을 수 있어서다.** 저장 hook 을 ruff 로 정한 것과도 어긋난다 — 그쪽은 python 생태계로 깔리는데 언어 서버만 npm 인 셈이 된다. pypi 쪽은 `nodejs-wheel-binaries` 를 의존으로 들고 와서 node 를 스스로 깐다. PATH 에서 node 를 빼고 재서 확인했다.

**basedpyright 를 버린 것은 대가 둘이 실질적이어서다.** 문법 토큰을 주는 유일한 후보였는데,

- **진단이 기본으로 엄격하다.** `Import "os" is not accessed`·`Type annotation for attribute name is required`·`Argument type is unknown` 이 severity 2(Warning) 로 온다. `diagnostics.go` 가 거르는 것은 severity 3·4 라 이것들은 화면에 그려진다. 남의 python 저장소를 열면 도배된다. 낮추는 길을 네 가지 재봤는데 전부 먹지 않았다 — `initializationOptions.typeCheckingMode`, `workspace/didChangeConfiguration` 의 `basedpyright.analysis`·`python.analysis`, 뿌리의 `pyrightconfig.json`. `workspace.configuration` 능력을 알려 보니 진단은 그대로인데 서버가 `workspace/configuration` 을 되묻기 시작했다. ADR-0051 이 「능력을 알리면 서버가 되묻기 시작한다」고 닫아 둔 그 문이다.
- **`semanticTokens/range` 를 지원하지 않는다.** `-32601 Unhandled method` 다. `full` 뿐인데 386KB 파일에 882ms 였다. ADR-0103 이 창 단위로 묻기로 한 근거(300KB 의 60 줄이 1ms) 가 여기서는 성립하지 않는다.

얻는 것(문법 토큰) 보다 치르는 것(진단 도배와 새 물기 규칙) 이 크다고 보았다.

## 6. 찾는 순서는 「그 저장소의 환경 → PATH → uv 가 넣는 자리」다

python 은 프로젝트마다 자기 환경을 두는 언어라 그 안에 깔린 것이 그 저장소의 뜻이다. goimports 가 그 저장소의 `tool` 선언을 PATH 보다 앞에 두는 것과 같은 자리다(ADR-0065).

환경을 보는 순서는 `VIRTUAL_ENV` → `.venv/bin` → `venv/bin` 이다. 환경 변수가 앞인 것은 사람이 이미 그 환경에 들어와 편집기를 띄웠다는 뜻이라 그 뜻이 앞서기 때문이고, 뒤의 둘은 이름을 짓는 관례다.

**PATH 만 보지 않는다.** `uv tool install` 은 `~/.local/bin` 에 넣고 그 자리가 PATH 에 없는 기계가 흔하다. gopls 의 GOBIN 이 같은 일을 먼저 겪었고(ADR-0051), 우리가 깔아 주는 길이 있어서 그 자리를 보지 않으면 「깔았는데 못 찾는다」가 된다.

그 자리도 **우리가 풀지 않고 uv 에게 묻는다**(`uv tool dir --bin`). goimports 를 찾을 때 `go env` 에게 묻는 것과 같은 태도다(ADR-0065). 그 물음을 모르는 판이면 uv 가 문서에 적어 둔 기본값(`~/.local/bin`) 을 본다.

## 7. 깔는 명령은 표가 값으로 든다

gopls 는 `go install golang.org/x/tools/gopls@latest` 이고 pyright 는 `uv tool install pyright[nodejs]` 다. 둘 다 「명령 하나와 인자들」이라 표에 담긴다 — 서버마다 설치 함수를 두면 자리가 서버 수만큼 늘고 하는 일은 같다.

첫 칸의 도구가 없으면 깔지 않고 알린다. `uv` 가 없는 기계에서 pyright 를 깔 길이 없고, 그것을 우리가 대신 정해 줄 일이 아니다.

설치 확인창도 표의 줄을 읽는다(`view-server-install-confirm.go`). 문구를 창에 적어 두면 서버를 더할 때 창까지 같이 손봐야 한다. 화면에 적는 명령은 대괄호를 따옴표로 감싼다 — `pyright[nodejs]` 를 그대로 적으면 zsh 가 파일 이름 짝맞추기로 읽어서, 옮겨 친 사람이 `no matches found` 를 만난다. 우리가 돌릴 때는 셸을 거치지 않으므로 인자 그대로 간다.

## 8. 저장 hook 은 `ruff format -` 이고, 표가 switch 를 대신한다

ADR-0065 §2 가 「표는 코드 안의 switch 하나다」로 두고 「둘째 언어가 생기면 case 가 하나 는다」고 적었다. 그 둘째가 왔는데, 같이 온 것이 하나 더 있다 — **설치를 묻는 창이 이름과 명령을 값으로 읽어야 한다.** switch 로 두면 그 창이 볼 표를 따로 만들어야 하고, 그것이 바로 `askGoimports` 가 경계해 둔 「표가 두 자리에 있게 된다」다. 그래서 ADR-0065 §2 를 이 부분에서 갈아치우고 `formatters` 표를 둔다.

`ruff format -` 은 stdin 을 받고 black 호환 포맷을 낸다. import 는 정리하지 않는다 — goimports 가 하는 그 일은 `ruff check --select I --fix -` 라 명령이 둘이 되고, 저장 하나가 명령 둘을 지나는 것은 이 자리가 아직 하지 않는 일이다.

설정은 명령을 돌리는 자리(그 파일의 폴더) 에서 위로 올라가며 찾는다. 재보니 그 자리의 `pyproject.toml` 의 `line-length`·`indent-width` 가 stdin 에도 그대로 들었다. `--stdin-filename` 은 주지 않는다 — `exclude` 는 stdin 에서 주든 안 주든 따르지 않았고(재서 확인했다), 주면 담아 두는 답이 파일마다 하나가 되어 폴더 단위 캐시가 깨진다.

문법이 깨진 글은 exit 2 와 `error: Failed to parse at 1:7: ...` 로 온다. ADR-0065 §5 가 정한 대로 저장을 막지 않고 첫 줄만 아래에 적는다.

## 9. 담아 두는 답의 키가 폴더만이 아니다

찾은 답을 폴더마다 담아 두던 자리(`saveHooks`) 에 **버그가 있었다.** 한 폴더에 `.go` 와 `.py` 가 같이 있으면 먼저 저장한 쪽의 답이 다른 쪽에 실려 간다 — goimports 를 찾아 둔 자리에서 python 파일을 저장하면 python 이 goimports 를 통과한다. 언어가 하나일 때는 드러나지 않던 자리다. 키를 「어느 포매터를 어느 폴더에서」 짝으로 바꿨다.

거절도 포매터마다 따로 적는다. goimports 를 깔지 않기로 한 것이 ruff 까지 묻지 않게 하면, python 파일을 저장하는 사람이 자기가 거절한 적 없는 물음을 잃는다.

## 10. 포매터 확인창은 묶고 언어 서버 확인창과는 갈라 둔다

ADR-0065 가 gopls 확인창과 goimports 확인창을 따로 둔 근거는 「그 둘은 뜨는 까닭도 거절의 뜻도 다르다」였다. 그 기준을 그대로 쓰면 goimports 와 ruff 는 **같다** — 둘 다 저장에 딸려 오고 둘 다 거절을 적어 둔다. 그래서 포매터끼리는 하나로 묶고(`view-formatter-install-confirm.go`), 언어 서버 쪽과는 계속 갈라 둔다.

## 11. 문 앞의 검사는 정의와 사용처가 따로 적는다

`references.go` 가 「하나로 묶지 않은 것은 알리는 문구가 갈리기 때문이다」를 근거까지 적어 두었다. 서버 이름을 문구에 넣으면서 그 대여섯 줄을 한 함수로 묶어 보았다가 물렸다. 이 결정을 뒤집을 새 근거가 이번 일에서 나오지 않았고, 뒤집으려면 그 자리에서 따로 볼 일이다.

## 이 ADR 이 정하지 않는 것

- **자동완성의 `resolveProvider`.** pyright 가 `true` 로 알리는데 우리는 후보에 딸린 문서를 쓰지 않는다(ADR-0066 이 남겨 둔 자리와 같다)
- **`.pyi`(stub) 를 볼지.** 두 표의 확장자를 갈라야 하는 첫 자리다
- **import 정리.** 저장 하나가 명령 둘을 지나는 길이 새로 생긴다
- **ruff 의 `exclude` 를 따를지.** stdin 으로는 따르지 않는다. 따르려면 파일 이름을 넘겨야 하고 폴더 단위 캐시가 깨진다
- **js 에 서버를 붙일지.** 표에 줄이 늘 자리는 있다
- **python 의 진단을 걸러 볼지.** pyright 는 온건해서 지금 걸 것이 없다. basedpyright 를 쓰기로 하면 그때 같이 온다
