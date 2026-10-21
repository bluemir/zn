# 0009: VTE 기반 터미널(Ptyxis·GNOME Terminal)이 OSC 52 클립보드를 무시한다

- 상태: **열림.** VTE 가 OSC 52 읽기·쓰기를 모두 지원하지 않아 터미널 밖 클립보드로 글이 오가지 않는다
- 난 곳: **VTE 위젯 라이브러리 및 Ptyxis·GNOME Terminal.** OSC 52 이스케이프 시퀀스(`ESC ] 52 ; ...`)를 처리하지 않고 버린다
- 규격대로 동작한 것: zn 의 `clipboardStore`·`askClipboard`, bubbletea 2.0.9 의 `SetClipboard`·`ReadClipboard`
- 위에 열려 있는 것: [GNOME/vte#2495](https://gitlab.gnome.org/GNOME/vte/-/issues/2495) (2020-05-18, open), [chergert/ptyxis#83](https://gitlab.gnome.org/chergert/ptyxis/-/issues/83) (2024-03-22, open)
- 날짜: 2026-10-20

## 증상

`"+y`·`"+yy` 나 visual 의 `"+y` 를 치면 화면 하단에 「복사되었습니다 (클립보드)」 알림이 뜨지만, 다른 창(브라우저 등)에서 붙여넣기(`ctrl+v`)를 하거나 `wl-paste` 로 확인해도 아무것도 복사되지 않거나 이전 클립보드 내용 그대로다.

같은 세션에서 `"+p` 로 붙여넣기를 시도해도 아무것도 붙지 않고 0.5 초 뒤에 「터미널이 클립보드를 주지 않습니다」가 뜬다. 쓰기와 읽기 둘 다 되지 않는다.

## 재현

Ptyxis 나 GNOME Terminal 에서 직접 OSC 52 시퀀스를 보내 보면 재현된다.

```bash
# 터미널에 OSC 52 복사 시퀀스를 보낸다
printf '\033]52;c;%s\a' "$(echo -n 'hello world' | base64)"

# 시스템 클립보드를 확인한다 (Wayland: wl-paste, X11: xclip -o)
wl-paste
```

터미널이 시퀀스를 무시하므로 클립보드에 `'hello world'` 가 들어가지 않고 직전 클립보드 상태를 유지한다.

zn 안에서의 재현도 같다.

1. Ptyxis 터미널에서 zn 을 띄운다.
2. `"+yy` 로 줄을 복사한다.
3. 알림에는 「1 줄 복사되었습니다 (클립보드)」가 뜨지만 터미널 밖 다른 앱에서 붙여넣으면 내용이 비어 있거나 이전 내용이다.

## 우리 코드가 아니라는 근거

- **내보낸 byte 를 실제로 쟀다.** `clipboardStore` 는 `tea.SetClipboard` 로 `\x1b]52;c;<base64>\x07` 을 내보낸다. selection 이 `c`(시스템 클립보드) 이고 데이터가 base64 로 인코딩된 xterm OSC 52 규격 그대로다.
- **OSC 52 를 지원하는 터미널에서는 정상 동작한다.** Ghostty·Kitty·Alacritty·WezTerm 등 OSC 52 가 열려 있는 터미널 에뮬레이터에서는 동일한 코드로 시스템 클립보드에 정상 복사된다.
- **VTE 및 Ptyxis 이슈 트래커에 미지원 건이 열려 있다.** VTE 이슈 2495번(OSC 52 clipboard support)과 Ptyxis 이슈 83번에 보안 논쟁 등으로 인해 OSC 52 가 구현되지 않은 채 방치되어 있다는 점이 명시되어 있다.

## 원인

**VTE 가 OSC 52 를 구현하지 않았다.**

GNOME Terminal, Ptyxis, Console(kgx), Tilix 등 GNOME 계열 터미널 에뮬레이터가 공통으로 사용하는 핵심 터미널 에뮬레이션 위젯인 VTE 가 보안상의 이유로 OSC 52 클립보드 시퀀스를 지원하지 않는다. 시퀀스가 들어오면 처리되지 않은 제어 시퀀스로 취급되어 조용히 버려진다.

그 결과 OSC 52 에 전적으로 의존하는 현재의 클립보드 구현(ADR-0148)은 VTE 기반 터미널에서는 완전히 무력화된다.

## 가능한 해결 방향

1. **OS 네이티브 클립보드 도구 병행 사용:**
	- 로컬 데스크톱 세션에서는 시스템 클립보드 도구(Wayland 의 `wl-copy`/`wl-paste`, X11 의 `xclip`/`xsel`, macOS 의 `pbcopy`/`pbpaste`, Windows API)를 호출하는 라이브러리(`github.com/atotto/clipboard`)를 사용한다.
	- 복사 시에는 네이티브 도구와 OSC 52 를 둘 다 실행하여, 로컬 VTE 터미널과 OSC 52 지원 터미널/SSH 환경을 모두 만족시킨다.
	- 붙여넣기 시에는 네이티브 도구로 즉시 읽을 수 있으면 읽고, 없을 때만 OSC 52 로 fallback 한다.
2. **OSC 52 를 지원하는 다른 터미널 사용:**
	- 터미널 에뮬레이터를 Ghostty, Kitty, Alacritty, WezTerm 등으로 변경하여 사용한다.
