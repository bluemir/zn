# ADR-0127: 안쪽 살림은 필드가 아니라 문으로 낸다

## 맥락

`Buffer`+`viewport` 를 새 패키지로 낼 준비를 하며 「밖으로 열어야 할 면」을 쟀다. 그중에 **이 파일의 상태를 담아 둔 것**이 섞여 있었다.

| | 코드 | 시험 |
|---|---|---|
| `buf.disk.hash`·`size`·`mtime`·`outside` | 11 | 3 |
| `buf.git.base`·`head`·`marks` | 10 | 3 |
| `buf.syntax.revision` | 2 | 38 |
| `buf.undo`·`redo` | **0** | 0 |

되돌리기 이력은 밖에서 아무도 안 본다. 그런데 디스크 자국·git 캐시·문법 캐시는 본다. 필드를 그대로 대문자로 열면 `diskSeen`·`gitCache`·`syntaxCache` 와 그 필드까지 다 열려서 **안쪽 살림이 통째로 밖으로 드러난다.**

## 결정

필드를 여는 대신 **물음마다 문을 하나씩 낸다.**

```go
func (buf Buffer) diskSeenAt() (hash []byte, size int64, mtime time.Time)
func (buf *Buffer) markDiskStamp(size int64, mtime time.Time)
func (buf Buffer) outsideState() outsideChange
func (buf *Buffer) setOutsideState(change outsideChange)

func (buf Buffer) hasGitBase() bool
func (buf Buffer) gitHead() string
func (buf Buffer) gitMarkAt(line int) gitLineMark
func (buf *Buffer) setGitBase(base [][]byte, head string)
func (buf *Buffer) clearGitBase()

func (buf Buffer) syntaxRevision() int
```

열 개로 23 곳이 덮인다. 필드로 열었으면 type 셋과 그 필드 열둘이 나갔을 자리다.

## 1. `setGitBase` 는 짝을 안에서 끝낸다

밖에서 이렇게 하고 있었다.

```go
buf.git.base = base
buf.git.head = snapshot.status.head
...
buf.refreshGitLines()      // ← 잊으면 마커가 낡은 채로 남는다
```

문으로 내면서 그 셋을 하나로 묶었다. 「기준을 갈아끼우면 마커를 다시 잰다」가 한 자리에서 끝난다.

## 2. 문법 캐시가 창에 있는 것은 언어지원을 뺀 것과 어긋나지 않는다

`internal/syntax` 로 뺀 것은 **규칙**(`State`·`Indent`·`Outline`) 이고, `syntaxCache` 는 **이 파일 이 내용의 토큰**이다. `Buffer.lines` 와 index 가 같고 줄이 늘거나 줄면 `replaceLines` 가 같이 맞춘다.

같은 손이 세 겹에 있다.

| 규칙(밖) | 이 파일의 상태(창) |
|---|---|
| `internal/syntax` | `syntaxCache` |
| `go-git` | `gitCache` |
| `internal/lsp` | `diagnostics` (ADR-0125) |

**밖이 규칙을 알고 창은 그 결과를 든다.**

## 남는 것

- ~~**시험 38 곳이 `buf.syntax` 속을 본다.**~~ → 시험을 다시 썼다 (ADR-0129)
- `buf.undo`·`redo` 는 이미 닫혀 있다. 문도 필요 없었다
- ~~`disk` 는 필드가 대문자로 남아 문 하나를 우회하고 있었다~~ → 아래를 보라

## 문을 냈는데 필드가 열린 채였다

가르고 나서 보니 `Buffer.Disk` 가 **대문자 필드인데 type(`diskSeen`) 은 소문자**였다. 밖에서 `buf.Disk.Hash` 는 읽히는데 그 type 으로 변수는 못 만드는 어중간한 꼴이다.

그리고 실제로 한 자리가 문을 안 지나고 있었다.

```go
// internal/core/outside.go
if buf == nil || !bytes.Equal(buf.Disk.Hash, seen) {
```

`DiskSeenAt()` 을 내 놓고도 그 하나가 남은 것은, **필드가 열려 있으면 문을 안 써도 컴파일이 되기 때문**이다. 문을 내는 것만으로는 안 되고 필드를 같이 닫아야 그 결정이 지켜진다.

그 한 줄을 문으로 돌리고 필드를 `disk` 로 닫았다. 이제 그 type 은 밖에서 아예 안 보인다 — 무엇으로 부를지가 안쪽 일이 되어 곧 `lastDiskState` 로 바뀌었다.

## 글 자체도 같은 손으로 닫았다

`Buffer.Lines`(`[][]byte`) 가 마지막으로 열려 있던 큰 필드였다. 밖에서 쓰는 꼴을 세니 셋이었다.

| 꼴 | core 본문 | 시험 | 낸 문 |
|---|---|---|---|
| `buf.Lines[i]` | 24 | 61 | `Line(n) []byte` |
| `len(buf.Lines)` | 24 | 10 | `LineCount() int` |
| 통째로 | 11 | 9 | `AllLines() [][]byte` |
| **쓰기** | **2** | 0 | 이미 있던 `ReplaceAll` |

### 쓰기 둘이 우회하고 있었다

```go
// rename.go, replace.go
buf.Lines = next
```

같은 함수의 **열린 tab 쪽 갈래는 `ReplaceAll` 을 쓰고 있었다.** 안 열린 파일 쪽만 필드에 직접 쓰고 있었던 것이라, 새 문을 낼 것 없이 그쪽도 `ReplaceAll` 로 돌렸다. `disk` 때와 같다 — **필드가 열려 있으면 문이 있어도 안 쓰는 자리가 생긴다.**

### `[][]byte` 를 돌려주는 문은 무엇을 지키나

`AllLines()` 는 **겉 slice 만 사본**이다. `[]string` 으로 하면 완전히 불변이 되지만 그러지 않았다.

- **구멍을 안 막는다.** `Line(n) []byte` 가 같은 byte 를 그대로 넘긴다. 막으려면 `Line(n) string` 도 되어야 하는데 그것은 그리는 자리(매 프레임, 보이는 줄마다) 라 감당이 안 된다
- **값이 크다.** `AllLines` 를 부르는 자리 중 하나가 편집 tick 마다 도는 언어 서버 동기화다(semantic.go). `[]string` 이면 그때마다 파일 전체를 새로 할당한다
- **`internal/lsp` 가 이미 같은 약속 위에 있다.** `copyLines` 가 겉 slice 만 복사한다

**진짜 보증은 type 이 아니라 「줄의 byte 를 제자리에서 고치지 않는다」이고, 되돌리기 설계가 그것을 강제한다.** `ReplaceLines` 가 갈아끼우는 유일한 자리이고, 제자리 수정은 옛 줄을 가리키던 undo 기록을 덮어서 돌릴 수 없게 만든다(buffer-edit.go).

그래서 겉 slice 사본은 **type 이 줄 수 있는 보증 하나**만 싸게 얻는다. 받은 쪽이 `lines[3] = ...` 로 창의 줄을 바꿔치기할 수 없다.

덤으로 안전해진 자리가 있다. 언어 서버에 보낼 줄을 goroutine 이 들고 나가는데, 이제 그 겉 slice 가 창의 것과 다른 배열이다.

## 되짚은 것

「열어야 할 면」을 세다가 **면의 크기가 아니라 결이 문제였다**는 것을 알았다. 462 곳 중 458 이 읽기이고 쓰기는 넷뿐인데, 그 읽기 안에 「이 창이 지금 어디를 보고 있나」와 「이 창이 살림을 어떻게 하고 있나」가 섞여 있었다. 앞엣것은 열어도 되고 뒤엣것은 아니다.
