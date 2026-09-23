# 0007: Ghostty 가 드래그 복사에서 tab 을 공백으로 준다

- 상태: **넘김.** 우리가 내는 byte 는 tab 이 맞고, 바꾸는 것은 터미널이다. 고칠 자리가 앱에 없다
- 난 곳: **Ghostty 1.3.1 의 선택 복사.** 화면 격자에서 긁어낸 글을 클립보드에 담는 자리다
- 규격대로 동작한 것: zn 의 `catRun.Run`, bubbletea 2.0.9 의 `releaseTerminal`, tmux 3.6a
- 위에 열려 있는 것: [ghostty#8426](https://github.com/ghostty-org/ghostty/issues/8426) (2025-08-27, open)
- 날짜: 2026-09-19

## 증상

`:cat` 으로 낸 글을 마우스로 긁어 붙여넣으면 **들여쓰기의 tab 이 공백이 되어 있다.**

```
파일:      \t i f   x   {
붙여넣은 것:     i f   x   {      ← tab stop 까지의 빈 칸이 space 로 온다
```

`:cat` 이 있는 까닭이 「붙여넣을 수 있는 글을 낸다」인데(ADR-0085) 그 목적의 한 조각이 빠진다. go 파일처럼 tab 으로 들여쓰는 글에서는 붙여넣은 뒤 들여쓰기를 다시 고쳐야 한다.

## 재현

```bash
printf 'func main() {\n\tif x {\n\t\ty := 1\n\t}\n}\n' > /tmp/tabby.go
D=./.claude/skills/run-zn/driver.sh
$D build
ZN_WIDTH=60 ZN_HEIGHT=16 $D start /tmp/tabby.go
$D keys ':' '%' c a t Enter
tmux capture-pane -p -t zn-driver | od -c
```

tmux 에서는 이렇게 나온다.

```
{  \n  \t   i   f       x       {  \n  \t  \t   y       :   =       1  \n  \t   }  \n
```

`\t` 가 그대로다. **같은 것을 Ghostty 에서 눈으로 긁어 붙여넣으면 공백이다.** 격자를 읽는 길이 갈려서 이 차이는 자동으로 잴 수 없다. tmux 의 `capture-pane` 은 자기 격자에서 tab 자리를 되살려 주고, Ghostty 의 선택 복사는 되살리지 않는다.

`:cat` 없이도 같다. `printf 'a\tb\n'` 을 Ghostty 에 찍고 긁어 봐도 공백이 온다.

## 원인

**터미널 격자에는 tab 이 없다.** HT 는 글자가 아니라 커서를 다음 tab stop 으로 옮기는 제어문자라서, 지나온 칸에는 아무것도 안 담기고 빈 칸으로 남는다. 긁어낼 때 격자를 읽으면 그 빈 칸들이 나온다.

어느 칸이 tab 이 밀어낸 자리였는지 따로 표시해 두는 터미널만 원래대로 돌려줄 수 있다. Kitty·GNOME Terminal·tmux 가 그렇게 한다. Ghostty 는 xterm 을 따라 표시하지 않는다.

ghostty#8426 에서 maintainer(mitchellh) 가 고치는 쪽으로 답했다.

> the trailing `\t` is preserved up to the tabstop. If any characters *prior to that* are changed, all preceding tab spaces become explicit space (`0x20`) but the trailing data up to the tabstop remains a `\t` (`0x09`)

xterm 과 갈리는 것을 알고 고르는 것이고, 근거로 「거의 모든 다른 터미널이 어떤 형태로든 tab 을 복사한다」와 「xterm 자신이 이 동작을 bug 로 문서에 적어 두었다」를 들었다.

## 우리 코드가 아니라는 근거

- **펴는 코드가 없다.** `catRun.Run` 은 `buf.lines` 의 byte 를 그대로 `out.Write` 한다(`internal/core/cat.go:62`). `CatLines` 는 slice 를 뜨기만 한다(`internal/textarea/buffer-cat.go:7`). 사이에 아무것도 없다
- **내보내는 byte 를 실제로 쟀다.** 위 재현의 `od -c` 가 `\t` 를 그대로 보여준다. 앱이 내보낸 것은 tab 이다
- **시험이 이미 그것을 잡고 있다.** `TestCatRunPrintsFileBytes` 가 `\tif x {\n` 을 기대한다(`internal/core/cat_test.go:37`). 「tab 이 칸으로 펼쳐지지 않는다」가 그 줄의 뜻이다
- **터미널 드라이버도 아니다.** BSD 의 `OXTABS`(`TAB3`) 를 켜면 커널 tty 가 tab 을 공백으로 펴는데, bubbletea 는 `tea.Exec` 앞뒤로 `term.MakeRaw` 이전 상태를 그대로 되돌릴 뿐이라(`tty.go:33`) 이 flag 를 새로 켜지 않는다. 켜져 있었다면 tmux 캡처에도 `\t` 가 없었을 것이다
- **`:cat` 밖에서도 난다.** `printf 'a\tb\n'` 이 같은 결과다. zn 을 거치지 않는 경로다

## ADR-0085 의 전제가 여기서 깨진다

§2 가 이렇게 적혀 있다.

> 화면에서는 4 칸으로 펼쳐지지만(ADR-0020) 여기서는 byte 그대로 나간다. 터미널이 제 tab stop (대개 8) 으로 그리므로 들여쓰기 폭이 편집 화면과 달라 보이는데, **복사되는 것은 진짜 tab** 이다.

앞 문장은 맞고 뒤 문장은 **터미널이 지켜 줄 때만** 맞다. 「내보내는 byte 가 tab 이다」와 「긁으면 tab 이 온다」가 같은 말이 아니었고, 그 사이에 격자가 하나 있다.

ADR 을 고치지는 않았다. 결정 자체는 그대로 서 있고, 깨진 것은 결정이 아니라 그때 재지 않은 전제다. 이 문서가 그 자리다.

## 어떻게 넘겼나

**아무것도 하지 않았다.** 앱이 손댈 자리가 없다.

- tab 을 칸으로 펴서 내보내는 길이 있다. 붙여넣으면 눈에 보이는 들여쓰기가 그대로 온다. 대신 tab 이 아예 사라져서 ADR-0085 §2 의 「파일의 byte 그대로」를 뒤집는다. tab 을 살리는 터미널을 쓰는 사람에게서도 같이 뺏는다
- 터미널이 tab 을 되살리는지 앱이 알 방법이 없다. 알 수 있었다면 갈라 쓰는 길이 있었을 텐데, 프로브로 잴 수 있는 것은 「터미널이 셀을 몇 칸 잡는가」이고 여기서 갈리는 것은 「긁을 때 어느 칸을 tab 으로 되돌리는가」다. 클립보드 쪽이라 앱이 볼 수 없다

**우회로는 사용자 쪽에 둘 있다.**

- tmux 안에서 쓴다. tmux 의 복사는 tab 을 살린다(위에서 쟀다)
- `:w <파일>` 로 내고 그 파일을 읽는다. 긁는 길을 거치지 않는다

**진짜 답은 OSC 52 다.** 긁는 손짓 자체가 없어져서 격자를 거치지 않는다. ADR-0085 가 「배타적이지 않으므로 그때 같이 본다」로 열어 두었고 `docs/tasks.md` 에 항목이 서 있다. 이 건은 그 항목에 근거를 하나 더한 것이다.

**그 길이 열렸다**(ADR-0148). `"+yy` 로 복사한 것은 tab 이 그대로 나간다. 이 증상을 피하려면 드래그 대신 그쪽을 쓴다. 다만 `:cat` 이 하던 「화면에 보이는 것을 통째로」는 여전히 다른 일이라 이 건이 닫히지는 않는다.

## 다시 나면

1. **`od -c` 로 앱이 내보낸 byte 를 먼저 본다.** 위 재현 그대로다. `\t` 가 있으면 우리 쪽은 맞게 내고 있고 볼 곳이 터미널이다
2. `printf 'a\tb\n'` 을 같은 터미널에서 긁어 본다. 같은 증상이면 zn 과 무관하다
3. **ghostty#8426 이 닫혔는지 본다.** 고쳐졌다면 Ghostty 판을 올리는 것으로 끝난다
4. `\t` 가 아예 없으면 그때는 우리 쪽 회귀다. `TestCatRunPrintsFileBytes` 부터 돌린다
