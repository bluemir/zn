# Spec

## UI 구성

- 좌측 sidebar
	- filetree
- 하단 statusBar
	- mode 표시
	- 2줄로 표시


## Mode

모드는 다음과 같이 구성 한다.

- normal mode
- insert mode
- visual mode
- copy mode
- command-line mode

### Normal mode

vim 의 normal mode 와 동일하다.
cursor 를 옮기고, 파일을 편집 한다.

커서는 글자 위에 있다. 줄 끝 다음 칸에는 설 수 없다. 그 자리는 insert mode 에서만 간다.

#### key mapping

키 | 기능 | 기타
---|------|-----
i | 현재 커서 자리에서 insert mode 로 | 커서는 그대로
a | 현재 커서 바로 뒤에서 insert mode 로 | 줄 끝 다음 칸까지 갈 수 있다
: | command-line mode 로 |
u | 되돌리기 | 타이핑 연속 구간이 한 단위다
ctrl+r | 다시 적용 |
↑ ↓ ← → | 커서 이동 | 위아래는 화면 행 단위. wrap 된 줄 안에서도 한 행씩
ctrl+c | 종료 | `:q` 와 같다. 모든 mode 에서 받는다

vim 키맵(`hjkl` 등) 은 아직 없다. 개선점을 정한 뒤에 넣는다.

### insert mode

vim 의 insert mode 와 동일 하다.
현재 커서 위치에 내용을 삽입한다.

커서는 글자 사이에 있다. 줄 끝 다음 칸까지 갈 수 있다.

#### key mapping

키 | 기능 | 기타
---|------|-----
(글자) | 커서 자리에 넣는다 | grapheme cluster 단위. 한글·이모지도 한 글자
enter | 줄을 나눈다 |
backspace | 앞 글자를 지운다 | 줄 시작이면 앞 줄과 합친다
tab | tab 문자를 넣는다 | 화면에서는 8 칸으로 보인다
esc | normal mode 로 | 커서가 왼쪽으로 한 글자 간다. vim 과 같다
↑ ↓ ← → | 커서 이동 | 되돌리기 구간이 끊긴다. vim 과 같다

붙여넣기는 여러 줄이어도 그대로 들어간다.


### visual mode

vim 의 visual mode 와 동일하다

### copy mode

cat 의 text 출력과 동일하게 표현한다. 다른점은 현재 보던줄들을 출력 한다는 것이다.

### command-line mode

vim 의 command-line mode 와 동일하다.
normal mode 에서 `:` 로 들어간다. 치는 명령은 statusBar 의 아래 줄에 보인다.
줄을 새로 만들지 않고 아래 줄을 바꿔 쓰므로 편집 영역 높이가 흔들리지 않는다.

명령 | 기능
-----|-----
:w | 저장
:q | 종료. 저장하지 않은 변경이 있으면 확인창을 띄운다
:wq, :x | 저장하고 종료
:q! | 저장하지 않고 종료

`esc` 로 취소한다. `backspace` 로 `:` 까지 지워도 나간다.
명령 결과와 오류는 아래 줄에 뜨고 다음 키를 누르면 사라진다.

`:q` 와 `Ctrl+C` 는 같은 경로다. 저장하지 않은 변경이 있을 때만 확인창이 뜨고,
없으면 묻지 않고 나간다. 잃을 것이 있을 때만 묻는다.
