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

- **시험 38 곳이 `buf.syntax` 속을 본다.** 그 시험들을 어떻게 할지가 따로 있다 (ADR-0129)
- `buf.undo`·`redo` 는 이미 닫혀 있다. 문도 필요 없었다

## 되짚은 것

「열어야 할 면」을 세다가 **면의 크기가 아니라 결이 문제였다**는 것을 알았다. 462 곳 중 458 이 읽기이고 쓰기는 넷뿐인데, 그 읽기 안에 「이 창이 지금 어디를 보고 있나」와 「이 창이 살림을 어떻게 하고 있나」가 섞여 있었다. 앞엣것은 열어도 되고 뒤엣것은 아니다.
