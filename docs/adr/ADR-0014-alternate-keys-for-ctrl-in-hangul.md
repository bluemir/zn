# ADR-0014: ctrl 조합은 터미널에 PC-101 자리의 키를 물어서 받는다

- 상태: 채택
- 날짜: 2026-08-13

## 맥락

한글 입력 상태에서 `ctrl+p` 를 누르면 팔레트가 열리지 않고 커서만 한 칸 움직였다.
`ctrl+w`, `ctrl+c` 도 같았다 — **한글 상태에서는 종료 키가 듣지 않았다.**

ADR-0008 은 "ctrl 조합은 입력기가 먹지 않아 한글 상태에서도 그대로 온다" 를 전제로 두었는데,
그 전제가 틀렸다. 터미널에서 `cat -v` 로 보면 `^P` 가 찍히지만 zn 은 다른 것을 받는다.

```
String="ctrl+ㅔ" Code='ㅔ'(U+3154) Mod=ctrl Text=""
```

bubbletea v2 는 kitty keyboard protocol 의 key disambiguation 을 **항상 켠다**
(`keyboardEnhancementsFlags` 가 `flags := 1` 로 시작한다). 그 모드에서 터미널은 legacy 인
`0x10` 대신 **지금 입력기가 만든 코드포인트**를 modifier 와 함께 보낸다. `cat` 은 그 모드가
아니라서 `^P` 로 보였을 뿐이다.

받은 뒤도 나빴다. `hangulKeys` 는 한글이 섞인 문자열을 자모 단위로 푸는데, 키 이름까지 글자로
보고 `"ctrl+ㅔ"` 를 `c` `t` `r` `l` `+` `p` 여섯 키로 풀어 normal mode 에 먹였다. 커서가 한 칸
움직인 것은 그중 `l` 이었다. 줄 내용에 따라 `c` `t` 가 짝을 이루면 편집이 일어날 수도 있다.

## 결정

**`ReportAlternateKeys` 를 켜서 터미널에게 PC-101 자리의 키를 같이 달라고 한다.**

`editor.render` 에서 view 마다 켠다. 터미널이 지원하면 `Key.BaseCode` 가 채워지고,
bubbletea 의 `Key.Keystroke()` 가 `Code` 대신 그것으로 이름을 만든다. Ghostty 에서 실측했다.

```
ctrl+p → Code='ㅔ'(U+3154) BaseCode='p' → String()="ctrl+p"
ctrl+c → Code='ㅊ'(U+314A) BaseCode='c' → String()="ctrl+c"
```

**modifier 가 없는 글자 키는 이것으로 낫지 않는다.** alternate key 는 escape code 로 오는
이벤트에만 실리고 맨 글자 키는 평문 텍스트로 오기 때문이다(`ㅁ` 의 `BaseCode` 는 비어 있다).
그쪽은 ADR-0008 의 두벌식 표가 그대로 맡는다.

## 근거

**터미널이 물리 키를 이미 알고 있다.** 두벌식 표는 zn 이 자모를 보고 키를 되짚는 추측이지만,
`BaseCode` 는 키를 누른 쪽이 아는 사실이다. 세벌식이든 AZERTY 든 같이 낫는다.

**한 줄이고 안 되면 조용히 원래대로다.** 터미널이 지원하지 않으면 응답하지 않을 뿐이고,
kitty protocol 자체를 모르는 터미널은 legacy `0x10` 을 보내서 `ctrl+p` 로 잘 온다.

**ctrl 조합은 자모 하나에 물리 키 하나라 되돌리기도 쉽다.** 그래도 터미널에 묻는 쪽을 골랐다 —
표를 늘리면 `ㅊ`→`c` 같은 대응을 zn 이 또 들고 있어야 한다.

## 결과

- 한글 상태에서 `ctrl+p`, `ctrl+w`, `ctrl+c`, `ctrl+r` 이 모두 듣는다
- **`ctrl+ㅔ` 가 키 여섯 개로 풀리는 길은 아직 남아 있다.** kitty protocol 은 켜고
  alternate key 는 안 주는 터미널에서 그렇다. 지금 쓰는 터미널에서는 도달하지 않아서 막지
  않았다 — `docs/tasks.md` 에 남긴다
- 조합 중인 글자 때문에 동작이 한 키 밀리는 것은 그대로다(ADR-0008)

## 대안

**전체 escape code 모드(`ReportAllKeysAsEscapeCodes` + `ReportAssociatedText`)** — 모든 키가
escape code 로 와서 맨 `ㅁ` 도 `BaseCode='a'` 를 받는다. 두벌식 표를 통째로 버릴 수 있어서
실측까지 해봤고, 세 가지 이유로 접었다.

1. **`Text` 가 다음 이벤트로 샌다.** esc 를 눌렀는데 `Text="ㅁ"`, `Text="ㅘ"` 가 실려 왔다.
   `Key.String()` 은 Text 를 먼저 쓰므로 esc 가 esc 로 먹지 않고, insert mode 에서는 글자가 꽂힌다
2. **밀림이 낫지 않는다.** `ㅗ` `ㅏ` 를 눌러도 이벤트 둘 다 `BaseCode='h'` 다. 물리 `k` 는
   끝내 자기 이벤트로 오지 않는다 — 조합 중인 키는 여전히 입력기가 들고 있다
3. `capslock`, `leftctrl` 같은 modifier 자체가 키 이벤트로 쏟아져서 다 걸러야 한다

**`hangulKeys` 안에서 modifier 붙은 키를 막는다** — `"ctrl+ㅔ"` 를 nil 로 만들어 아무 일도
일어나지 않게 한다. 원인이 아니라 증상을 막는 것이고, 그렇게 해도 `ctrl+p` 는 여전히 안 듣는다.
배타적이지 않아서 tasks 에 남긴다.

## 이 ADR 이 정하지 않는 것

- modifier 없는 한글 키. ADR-0008 의 두벌식 표가 그대로 맡는다
- 조합 지연으로 동작이 한 키 밀리는 것
- 터미널이 alternate key 를 주지 않을 때의 대비
