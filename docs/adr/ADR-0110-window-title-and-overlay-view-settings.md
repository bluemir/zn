# ADR-0110 터미널 창 제목을 띄우고, 겹쳐 그리는 화면은 부모의 view 를 그대로 쓴다

- 상태: 채택
- 날짜: 2026-08-31
- 관련: ADR-0014(한글 입력과 키 확장), ADR-0031(창 포커스 보고), ADR-0036(render 와 View), ADR-0041(OSC 11 기각), ADR-0064(빈 화면)

## 맥락

`docs/tasks.md` 에 「terminal 제목줄에 현재 파일 표시」가 있었고 「이건 terminal spec 에 따라 못할수도 있다」가 딸려 있었다.

재보니 된다. 그리고 붙이려는 자리에 **이미 버그가 하나 있었다.**

## 1. bubbletea 가 이미 칸을 준다

`tea.View.WindowTitle` 이다. 우리가 escape 를 손으로 쓰지 않는다.

값이 **바뀔 때만** OSC 2 를 쓴다(`cursed_renderer.go` 의 렌더 경로). 프레임마다 쓰지 않으므로 그리는 자리에서 매번 만들어 넘겨도 값이 붙지 않는다.

## 2. ADR-0041 이 OSC 11 을 기각한 것과 성격이 다르다

그 기각 근거는 둘이었다. 「답하지 않는 터미널이 있어서 『모를 때 무엇을 쓰나』가 남는다」와 「값을 두 벌 들고 다니는 것이 『config 는 compile 됨』과 어긋난다」.

제목줄은 **답을 기다리지 않는 일방 통보**다. 안 받는 터미널에서는 아무 일도 일어나지 않고, 들고 다닐 값도 없다. 그래서 두 근거가 걸리지 않는다.

## 3. 제목은 `zn` 이 앞이다

```
zn                                  볼 파일이 없을 때
zn editor.go (internal/core)        저장소 아래의 파일
zn editor.go + (internal/core)      저장하지 않은 변경이 있을 때
zn go.mod                           뿌리에 있는 파일이라 적을 폴더가 없다
zn [No Name]                        아직 이름이 없는 buffer
```

**`zn` 을 앞에 두는 것은 창 목록과 tmux 상태줄이 뒤를 자르기 때문이다.** 무엇이 띄운 제목인지가 먼저 서야 잘려도 남는다.

`+` 는 tabline 이 쓰는 것과 같은 글자다(`tabLabel`). **창을 여럿 띄워 둔 사람이 어느 창에 저장하지 않은 것이 있는지 보는 것이 이 기능의 가장 큰 값**이라, 그것을 제목에 올린다.

폴더는 파일 이름 뒤 괄호에 넣는다. 같은 이름의 파일을 여러 창에 열어 둔 때를 가른다. 뿌리에 있는 파일은 적을 폴더가 없어서 괄호가 아예 없다.

**tab 개수는 적지 않는다.** 제목이 tabline 을 옮겨 적는 자리가 아니고, 창 밖에서 알고 싶은 것은 「지금 무엇을 고치고 있나」다.

## 4. tmux 안에서는 pane 제목이다. 우리가 어쩔 수 없다

재보니 tmux 의 `set-titles` 는 기본이 `off` 다. 그래서 tmux 안에서 띄우면 **pane 제목**이 바뀌고 바깥 창 제목까지는 가지 않는다. tmux 상태줄의 `#T` 로는 보이고, `set-titles on` 을 켠 사람은 창 제목으로도 본다.

우리 쪽에서 할 것이 없다. `docs/spec.md` 에 적어 둔다.

## 5. 나갈 때는 빈 제목이다. 되돌리지 않는다

bubbletea 가 나가며 `SetWindowTitle("")` 을 쓴다. **원래 것으로 되돌리지 않는다.** 재보니 그대로였다.

```
[AL03140325.local]  →  [zn go.mod]  →  []
```

되돌리는 길은 있다. OSC 22/23(제목 밀어넣기·꺼내기) 이 tmux 에서 동작하는 것을 확인했다(`BEFORE` → `AFTER` → `BEFORE`).

**쓰지 않기로 했다.** bubbletea 가 그것을 쓰지 않으므로 `core.Run` 앞뒤에서 우리가 raw escape 를 써야 하고, 그러면 터미널에 글자를 내보내는 자리가 bubbletea 밖에 하나 생긴다. 대개 셸 프롬프트가 다음 줄에서 제목을 다시 쓰므로 얻는 것이 그 값을 넘지 않는다.

## 6. 겹쳐 그리는 화면이 터미널 설정을 잃고 있었다

제목을 붙일 자리를 찾다 발견했다. `newView` 가 「터미널 설정이 여기 한 곳에 있다」고 적어 두었는데 **부르는 곳이 4 자리뿐**이었다. 겹쳐 그리는 여섯은 `tea.NewView` 를 새로 만들고 설정을 손으로 베꼈다.

```go
next.AltScreen = view.AltScreen
next.MouseMode = view.MouseMode
```

`newView` 가 켜 두는 것은 **넷**인데 베끼는 것은 둘이었다. 재보니 이랬다.

```
normal           AltScreen=true MouseMode=1 ReportFocus=true  ReportAlternateKeys=true
quit-confirm     AltScreen=true MouseMode=1 ReportFocus=false ReportAlternateKeys=false
rename-input     AltScreen=true MouseMode=1 ReportFocus=false ReportAlternateKeys=false
palette          AltScreen=true MouseMode=1 ReportFocus=true  ReportAlternateKeys=true
```

bubbletea 는 `ReportFocus` 가 참에서 거짓으로 가면 `ResetModeFocusEvent` 를 쓴다. **종료 확인창이나 이름 바꾸기 창이 열려 있는 동안 창 포커스 보고가 꺼졌다** — ADR-0031 이 「다른 창에서 파일을 고치고 돌아오는 순간」을 잡는 그 길이다.

팔레트만 멀쩡했다. 새 view 를 만들지 않고 `view.Content` 만 갈아끼우기 때문이다.

### 고치는 법은 코드를 없애는 쪽이다

여섯을 팔레트와 같은 손으로 맞췄다. **받은(또는 부모의) view 를 그대로 쓰고 `Content` 만 갈아끼운다.** 새 함수도 새 타입도 만들지 않았고, 손으로 베끼던 두 줄이 여섯 자리에서 사라졌다.

**베끼는 자리를 두면 목록이 늘 때 반드시 뒤처진다.** 그것이 이 버그의 모양이었고, 제목 칸을 더하면 셋째 줄이 늘 자리였다. 그대로 쓰면 늘 자리가 없다.

시험은 칸을 하나씩 세지 않고 **부모 것과 통째로 견준다**(`TestOverlaysKeepEveryTerminalSetting`). 다음에 칸이 늘어도 그 시험이 먼저 걸린다.

## 7. 그래서 제목을 붙일 자리는 `newView` 하나다

`newView(rows []string, title string)` 로 제목을 같이 받는다. **editor 를 받지 않는 것은 그대로다** — 다 만들어진 글로 받으므로 ADR-0036 이 정한 「층이 아니라 설정을 붙여 주는 자리」가 유지된다.

겹쳐 그리는 화면들이 부모의 view 를 쓰므로 제목을 붙이는 자리도 이 하나로 남는다.

## 결과

잰 값이다(tmux, 실제 편집기).

```
1) 첫 tab :        zn editor.go (internal/core)
2) gt 로 go.mod :  zn go.mod
3) 팔레트 열고 :   zn go.mod
4) 종료 확인창 :   zn go.mod +
5) 나간 뒤 :       []
```

## 이 ADR 이 정하지 않는 것

- **OSC 22/23 으로 제목을 되돌리는 것.** §5 에서 값이 안 맞는다고 보았고, 셸이 다시 쓰지 않는 판에서 거슬리면 그때 다시 본다
- **tmux 의 `set-titles` 를 우리가 켜는 것.** 남의 설정이다
- **제목에 tab 개수·mode·git branch 를 적을지.** 지금은 「무엇을 고치고 있나」 하나다
- **OSC 52(클립보드) 와 진행 표시(`tea.View.ProgressBar`).** 같은 「터미널에 알리는 칸」이지만 각자 정할 일이다
