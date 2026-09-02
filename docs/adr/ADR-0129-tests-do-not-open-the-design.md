# ADR-0129: 시험이 설계를 열지 않는다

## 맥락

`internal/textarea` 를 가르니 코드는 넷만 고치면 되었는데(ADR-0128) **시험이 59 곳에서 창의 안쪽을 보고 있었다.**

```
buf.syntax.valid · changedEnd · Lines[n].semantic     담아둔 것의 불변      37
buf.applySaveHook(...)                                 안쪽 단계             10
buf.editing · undo · git.marks · disk.mtime · Top.row                        12
```

시험을 살리려면 그 관찰점을 대문자로 열어야 한다. 그러면 **시험 때문에 밖으로 드러나는 면**이 생긴다.

## 결정

**열지 않는다. 시험을 다시 쓴다.**

> 시험은 주어진 코드를 시험하려고 있는 것이지, 시험이 설계를 바꾸면 주객이 뒤집힌다.

## 1. 두 갈래를 갈랐다

양쪽으로 다 옮겨 보고서야 결이 보였다.

| | 어디에 있어야 하나 |
|---|---|
| **그 패키지의 불변**을 보는 시험 | **그 패키지 안**. 화이트박스여도 된다 |
| **밖에서 보이는 것**을 보는 시험 | 밖. 관찰점을 열 것이 아니라 보이는 것으로 재야 한다 |

화이트박스 시험 자체가 나쁜 것이 아니다. **밖에서 하는 화이트박스**가 나쁘다.

## 2. 안으로 들인 것 — 몰아 주는 손을 바꿨다

담아둔 것의 불변을 보는 시험 14 개를 `internal/textarea` 로 옮겼다. 걸린 것은 **그것을 몰던 손**이 editor 였다는 점이다.

| 밖에서 몰던 것 | 안에서 모는 것 |
|---|---|
| `contentRowsOf(t, m)` (그리면 화면 아래까지 훑는다) | `buf.LexSyntaxTo(4)` |
| `send(m, "j", "a")` + 타이핑 | `buf.MoveTo(1, 1)` · `buf.Insert(...)` |
| `applySemanticTo(t, buf, msg)` | `buf.SetSemanticTokens(from, to, tokens)` |

**그리기가 하는 일이 곧 `LexSyntaxTo` 다.** 시험이 그것을 직접 부르면 editor 없이 같은 것을 본다.

포매터 시험 9 개도 같이 왔다. 그러면서 **셸을 안 부르게 되었다.**

```go
// 전: shellHook("tr a-z A-Z")  → /bin/sh 를 띄운다
// 후: pipe(strings.ToUpper)    → SaveFormat 에 Go 함수를 끼운다
```

창이 아는 것은 「byte 를 넣으면 byte 가 나온다」뿐이라(ADR-0128) 그 자리에 함수를 끼우면 된다. 명령을 찾고 돌리는 것은 core 의 일이고 그쪽 시험이 본다.

## 3. 밖에 남긴 것 — 보이는 것으로 다시 잰다

| 보던 것 | 무엇으로 바꿨나 |
|---|---|
| `buf.editing` | `x` `end` `y` 를 치고 `u` 가 **`y` 만 무는지** 본다 |
| `buf.git.marks` 를 심기 | **HEAD 원본을 준다.** 마커는 그것과 지금 글을 견주어 나온다 |
| `buf.disk.mtime`·`outside` | `DiskSeenAt()`·`OutsideState()` (ADR-0127) |
| `buf.syntax.revision` | `SyntaxRevision()` (ADR-0127) |
| 답이 어느 창에 얹혔나 | 서버가 `type` 이라 말하게 해서 lexer 의 `keyword` 와 갈리게 한다 |

### 마커를 심던 것이 특히 나빴다

```go
buf.git.marks = map[int]gitLineMark{1: gitLineModified, ...}
```

마커는 HEAD 원본과 지금 글을 견주어 나오는 값인데, 심어 넣으면 **재는 것이 마커가 아니라 내가 적은 표**가 된다. 원본을 주는 쪽으로 바꾸니 시험이 실제 계산을 지난다.

### 갈리지 않는 것을 갈리게 만든다

「답이 묻지 않은 창에 얹히지 않는다」를 보려면 얹혔는지를 알아야 한다. 서버가 `package` 를 `keyword` 라 하면 lexer 의 답과 같아서 글자로는 구별할 수 없다. **서버가 `type` 이라 말하게** 하면 `SyntaxTokens(0)[0].Kind` 로 갈린다.

## 4. 파일 이름이 시험의 소속을 말하지 않았다

앞서(748caf7) 무리 파일의 시험 열여섯 중 열이 `editor` 를 쓰고 있었다. **이름이 짝이라고 그 시험이 창 시험인 것은 아니다.**

```
page_test.go          창만  0 · 키 21   → page-key_test.go
indent-range_test.go  창만  0 · 키 23   → indent-range-key_test.go
```

`-key_test.go` 는 새 손이 아니다 — `indent-key_test.go`·`hangul-key_test.go` 가 이미 있었다.

**helper 를 한 겹 지나는 것을 처음에 놓쳤다.** `joinOf`·`pressFrom` 이 안에서 `send` 를 불러서 시험 함수만 보는 그물에 안 걸렸다.

## 결과

```
관찰점을 연 것          0
textarea 로 옮긴 시험   23
core 에서 다시 쓴 시험   9
```

`go build`·`go vet`·`go test`·smoke 통과.

## 되짚은 것

**시험이 화이트박스였다는 것 자체가 신호였다.** 「기대하는 상태」가 내부 상태라는 것은, 그 시험이 무엇을 지키는지 스스로도 흐리다는 뜻이다. `buf.editing == false` 는 규칙이 아니라 그 규칙의 구현이고, 규칙은 「`u` 한 번이 커서를 옮긴 뒤에 친 것만 문다」다.

다시 쓰고 나서 시험이 더 읽힌다. 그리고 셸을 안 띄우게 되어 빨라졌다.
