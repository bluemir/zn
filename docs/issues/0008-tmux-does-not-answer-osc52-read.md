# 0008: tmux 가 OSC 52 읽기 요청에 답하지 않는다

- 상태: **넘김.** 규격대로 물었고 답하지 않는 것은 tmux 가 그렇게 만들어진 것이다. 고칠 자리가 앱에 없다
- 난 곳: **tmux 3.6a 의 `set-clipboard`.** 쓰기(`ESC]52;c;<base64>`) 는 받고 읽기(`ESC]52;c;?`) 는 답하지 않는다
- 규격대로 동작한 것: zn 의 `askClipboard`, bubbletea 2.0.9 의 `ReadClipboard`
- 위에 보고하지 않았다: 아래 「왜 올리지 않았나」를 본다
- 날짜: 2026-09-23

## 증상

`"+p` 를 쳐도 아무것도 붙지 않고 0.5 초 뒤에 「터미널이 클립보드를 주지 않습니다」가 뜬다. tmux buffer 에 글이 들어 있어도 같다.

같은 세션에서 **쓰기는 된다.** `"+yy` 를 치면 tmux buffer 에 그 줄이 담긴다. 한쪽만 되는 것이 이 건의 모양이다.

## 재현

```bash
D=./.claude/skills/run-zn/driver.sh
$D build
printf 'foo bar\nbaz qux\n' > /tmp/zn-clip.txt
ZN_WIDTH=60 ZN_HEIGHT=12 $D start /tmp/zn-clip.txt
tmux set-option -g set-clipboard on

# 쓰기는 도착한다
$D keys '"' '+' y y
tmux list-buffers          # buffer0: 8 bytes: "foo bar\n"

# 읽기는 답이 없다
tmux set-buffer "PASTED FROM OUTSIDE"$'\n'
ZN_KEY_DELAY=1.2 $D keys '"' '+' p
$D screen | tail -1        # 터미널이 클립보드를 주지 않습니다
```

`set-clipboard` 를 `on` 으로 두고 잰 것이다. 기본값 `external` 에서는 쓰기도 tmux buffer 에 안 담긴다.

## 우리 코드가 아니라는 근거

- **내보낸 byte 를 실제로 쟀다.** `tmux pipe-pane` 으로 앱이 보낸 것을 잡으면 `ESC ] 52 ; c ; ?` 가 그대로 나간다. selection 이 `c`(시스템 클립보드) 이고 `?` 가 읽기 요청이다. xterm 규격 그대로다

```bash
rm -f /tmp/zn-osc.log
tmux pipe-pane -o -t zn-driver 'cat >> /tmp/zn-osc.log'
ZN_KEY_DELAY=0.6 $D keys '"' '+' p
tmux pipe-pane -t zn-driver
grep -ao $'\x1b\]52;[^\x07\x1b]*' /tmp/zn-osc.log | cat -v   # ^[]52;c;?
```

- **쓰기는 같은 길로 도착한다.** 같은 세션 같은 설정에서 `"+yy` 가 tmux buffer 를 채운다. 앱이 OSC 52 를 못 내보내는 것이 아니다
- **답을 받는 자리는 시험이 덮고 있다.** `tea.ClipboardMsg` 를 직접 넣으면 그 글이 붙는다(`clipboard_test.go` 의 `TestClipboardPasteWaitsForAnswer`). 답만 오면 동작한다
- **bubbletea 도 규격대로 낸다.** `readClipboardMsg` 를 받으면 `ansi.RequestSystemClipboard` 를 쓴다(`tea.go:818`)

## 원인

**tmux 가 읽기를 다루지 않는다.** `man tmux` 의 `set-clipboard` 항목은 세 값을 전부 **설정(set)** 으로만 설명한다.

> If set to on, tmux will both accept the escape sequence to create a buffer and attempt to set the terminal clipboard.

「accept the escape sequence to create a buffer」가 쓰기를 받는다는 말이고, 읽기 요청에 답한다는 서술은 문서 어디에도 없다. 값 셋 중 무엇으로도 열 수 없다.

이유가 짐작되는 자리가 있다. OSC 52 읽기는 터미널 안의 프로그램이 클립보드를 **훔쳐 읽을 수 있는** 길이라, 답하는 쪽을 기본으로 막아 두는 것이 요즘 터미널의 보통이다. Ghostty·iTerm2 도 기본으로 막거나 물어보는 창을 띄운다.

## 왜 올리지 않았나

**bug 가 아니라 만들어진 대로다.** 문서가 쓰기만 말하고 있어서 「되어야 하는데 안 된다」가 아니다. 읽기를 열어 달라는 것은 기능 요청이고, 그것은 클립보드를 훔쳐 읽는 길을 여는 것이라 거절될 쪽에 가깝다.

**우리가 잃는 것도 크지 않다.** 쓰기가 이 기능의 주된 몫이고(ADR-0148), 읽기는 되는 터미널에서만 얹히는 것이다. 안 될 때 알리는 길이 이미 있다.

## 어떻게 넘겼나

**시간초과 알림 하나로 끝냈다.** 0.5 초 안에 답이 없으면 「터미널이 클립보드를 주지 않습니다」를 띄운다. 아무 일도 안 일어난 까닭을 손이 볼 수 있으면 그것으로 족하다.

터미널이 답할지 미리 알아보는 길은 두지 않았다. DA1 응답으로 짐작하는 방법이 vim 도움말에 적혀 있지만(`v:termda1`) 그것은 터미널 종류를 보고 미루어 아는 것이라, 틀리면 되는 터미널에서 길을 막는다. 물어보고 기다리는 편이 정확하다.

**사용자 쪽 우회로는 둘이다.**

- tmux 밖에서 쓴다. 읽기를 여는 터미널이면 그대로 동작한다
- 편집기 안의 register 를 쓴다. `y` 와 `p` 는 클립보드를 거치지 않는다

## 다시 나면

1. **`tmux pipe-pane` 으로 `ESC]52;c;?` 가 나가는지 먼저 본다.** 위 재현 그대로다. 나가고 있으면 우리 쪽은 맞고 볼 곳이 터미널이다
2. `tmux list-buffers` 로 쓰기가 되는지 본다. 쓰기도 안 되면 `set-clipboard` 가 `external` 이거나 `off` 다
3. **`man tmux` 의 `set-clipboard` 에 읽기 이야기가 생겼는지 본다.** 생겼다면 tmux 판을 올리는 것으로 끝난다
4. `ESC]52;c;?` 가 아예 안 나가면 그때는 우리 쪽 회귀다. `TestClipboardPasteWaitsForAnswer` 부터 돌린다
