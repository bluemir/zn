# 0001: 한 칸짜리 글자를 지우면 그 뒤 한글이 화면에서 깨진다 (bubbletea 증분 갱신)

- 상태: **해결됨** — bubbletea v2.0.7 → v2.0.9 로 올려서 사라졌다
- 난 곳: `charm.land/bubbletea/v2` v2.0.7 / `github.com/charmbracelet/ultraviolet` v0.0.0-20260525132238
- 날짜: 2026-08-22

## 증상

빈칸을 지우면 **그 뒤의 한글이 한 칸씩 밀려 깨져 보인다.**

```
지우기 전:  - — 오른쪽 사이드바의
지운 후  :  - —오 른쪽 사이드바의     ← 없던 빈칸이 생기고 글자가 밀림
```

**파일은 멀쩡하다.** 깨진 상태로 저장해 원본과 견주면 없어진 것은 **빈칸 하나(U+0020) 뿐**이고
한글은 한 글자도 안 없어졌다. 커서를 옮겨도 안 돌아오고, 창 크기를 흔들어 **전체를 다시 그리면
저절로 낫는다.**

「한글이 사라진다」로 여러 번 올라왔던 것이 이것이다. 입력기·mode 와 무관하고(영어로도 난다)
넓은 화면과 긴 줄에서만 나서 「종종」으로 보였다.

## 재현

```
printf '\n- [x] pawn 에 mouse over 시 정보 표시 \n\t- — 오른쪽 사이드바의 "가리키는 곳" 블록. pawn·object·지형을 화면 범위 조회 결과에서 찾아 그린다 (ADR-0020 §1)\n' > /tmp/t.md
tmux new-session -d -s t -x 273 -y 65 'zn /tmp/t.md'
# ↓ ↓ → → → → 로 `—` 뒤 빈칸에 커서를 두고
tmux send-keys -t t x
tmux capture-pane -t t -p | sed -n '4p'
```

폭 **273** 에서 100% 난다(3/3). **폭 120 에서는 안 난다** — 아래의 비용 계산이 갈린다.

## 원인

빈칸이 지워지면 그 뒤가 한 칸 왼쪽으로 당겨진다. 렌더러가 그것을 이렇게 줄여서 낸다.

```
\e[4;47H오     47 칸에 `오`(두 칸짜리) 를 쓰고
\e[P           DCH — 셀 하나를 지워 나머지를 왼쪽으로 당긴다
```

**뒤따르는 것이 전부 두 칸짜리 한글인데 셀을 하나만 당기니 반 칸씩 어긋난다.**

고르는 자리는 `ultraviolet/terminal_renderer.go` 의 `transformLine` 이다.

```go
} else if oLastCell > nLastCell {
	s.move(newbuf, n+1, y)
	dchCost := 3 + oLastCell - nLastCell
	if dchCost > len(ansi.EraseLineRight)+nLastNonBlank-(n+1) {
		... 줄을 통째로 다시 그린다
	} else {
		s.deleteCells(oLastCell - nLastCell) // ← 여기
	}
}
```

순수 **비용 계산**이라 끄는 손잡이가 없다. 바로 위의 ICH 갈래는 `s.caps.Contains(capICH)` 로
막히는데 DCH 갈래에는 그런 문이 없어서, `TERM` 을 바꿔도(xterm-256color·vt100·dumb) 똑같이 난다.

넓은 화면에서만 나는 이유가 이 식이다 — 줄이 길수록 「통째로 다시 그리기」가 비싸 보여서
DCH 를 고른다. 폭 120 에서는 그 줄이 접혀 뒷부분이 짧아지고, 그래서 다시 그리기를 골라 멀쩡했다.

## 우리 코드가 아니라는 근거

- 저장한 파일이 원본과 **빈칸 하나** 차이뿐이다. 편집 결과는 정확하다
- 이벤트 로그(ADR-0050) 의 buffer 상태에도 한글이 그대로 남아 있다
- `x` 를 줄의 모든 자리에서 눌러 보면 전부 커서 밑 글자만 지운다
- 커서 좌표·글자 폭·접힘 왕복도 전부 정확하다. 터미널이 재는 폭과도 일치한다
- 전체를 다시 그리면 낫는다 → **증분 갱신만의 문제**

## 어떻게 넘겼나

**판을 올렸다.** `charm.land/bubbletea/v2` v2.0.7 → **v2.0.9**
(`ultraviolet` 20260525 → 20260812).

올린 뒤에는 그 자리에서 `\e[P` 를 아예 안 내고 줄을 통째로 다시 쓴다. 바이트는 60 → 506 으로
늘지만 화면이 맞는다. 재현 절차로 3/3 정상이고 기존 시험·smoke 는 그대로 통과한다.

v2.0.8 이 「emoji 관련 렌더링 개선」으로 ultraviolet 을 올렸는데, emoji 도 두 칸짜리라
그때 같이 고쳐진 것으로 보인다. 판 올림 하나로 끝나서 **zn 쪽 우회 코드는 넣지 않았다.**

## 비슷한 것들

- [bubbletea#1668](https://github.com/charmbracelet/bubbletea/issues/1668) (closed) — 가장 가깝다.
  일본어·키릴·결합문자에서 **내용은 맞는데 화면만** 겹쳐 보이고 어긋난다. 같은 생김새다
- [ultraviolet#109](https://github.com/charmbracelet/ultraviolet/pull/109) (merged, 2026-04) —
  같은 `transformLine` 의 두 칸짜리 글자 처리인데 **다른 버그**다(무한 반복으로 CPU 100%).
  우리가 쓰던 20260525 판에는 이미 들어 있었다
- [bubbletea#1564](https://github.com/charmbracelet/bubbletea/issues/1564) — 터미널에 따라
  View 가 뭉개지는 것. 증분 갱신 계열로 묶인다
- [bubbletea#907](https://github.com/charmbracelet/bubbletea/issues/907) — 특수문자가 제대로
  안 그려지는 것

**위에 보고하지 않았다.** 이미 올라와 있는 것들과 생김새가 겹치고, 최신 판에서 나지 않는다.

## 다시 나면

1. 판부터 확인한다. `go list -m -u charm.land/bubbletea/v2 github.com/charmbracelet/ultraviolet`
2. `tmux pipe-pane` 으로 그 순간 나가는 바이트를 받아 **`\e[P` 가 있는지** 본다
3. 있으면 같은 자리다. 없으면 새 버그이므로 위 재현 절차와 함께 위에 알린다
4. 급하면 창 크기를 한 번 흔들어 전체를 다시 그리게 하는 것으로 화면만 되살릴 수 있다
