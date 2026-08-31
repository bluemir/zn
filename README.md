# zn

## concept

철저히 개인화된 vim의 개선 판 Text Editor

### name

- 두글자라 기억하기 쉽습니다.
	- 짧고 치기 쉬운 두 글자가 필요했고, 원소기호는 그런 이름이 이미 118 개 정해져 있는 목록입니다.
- `z` 와 `n` 이 양손에 나뉘어 있어 vim 처럼 빠르게 칠 수 있습니다.
- 아연은 주로 다이캐스팅에 쓰이는 금속입니다.
	- 설정도 플러그인도 없이 전부 코드에 박아 하나의 binary 로 찍어 내는 이 editor 와 닮았다고, 뜻은 나중에 갖다 붙였습니다.

### motive

ai 시대가 되면서 다양한 요구사항을 만족하기 위해 굳이 복잡한 설정파일과 플러그인을 제공 하기 보다는 그냥 각자가 코드 수정을 AI 에게 맡기는것이 더 비용이 낮을 것 같다는 생각이 들었습니다. 코드의 품질이 일정수준 이상이라면 AI 도 수정을 쉽게 해주므로 필요에 맞는 editor 를 code 단위에서 설정할수 있도록 합니다.

## feature

**지금 동작하는 기능만 적는다.** 앞으로 넣을 것은 roadmap 에 있다. 화면과 키의 자세한 규칙은 [docs/spec.md](docs/spec.md) 에 있다.

- terminal editor
- vim 과 유사한 강력한 편집 기능
- no config(config 는 compile 됨)
- 현대화된 tab 기능
- 언어별 syntax highlight
	- golang, markdown, html, css, js, python, shell, makefile, dockerfile
- sidebar에서 열리는 file tree
- vscode 와 유사한 command palette
	- 기본으로는 file matching
	- file matching 에서 `>`를 입력하면 바로 command 로 넘어가는 기능
	- `!` 를 입력하면 그 뒤를 shell 명령으로 돌린다. `:!<shell command>` 와 같다
- 이모지·특수문자 입력창
	- 팔레트에서 열면 화면 아래에 펼쳐진다. `삼각형` 으로 `▲ △ ▼ ▽` 를 걸러 골라 넣는다
- 상대 줄번호와 절대 줄번호 동시 표시
- statusBar 에 git branch, commit, dirty 여부 표시

## roadmap

### DO

앞으로 feature에 추가할 기능. 자세한 항목은 [docs/tasks.md](docs/tasks.md) 에 있다.

- 내장화된 언어지원
	- 종류
		- golang
		- markdown
		- html
		- css
		- js
		- python
	- 기능
		- 자동 완성
		- 정의/구현으로 이동
		- 사용처로 이동
		- 저장시 hook(eg. go fmt)
- home, end, page up, page down 의 일관적인 동작

### DONOT

feature 에 넣지 않을 기능

- config file
	- config는 code 내에 하드코딩 한다.
	- 분기를 최대한 줄이기 위한 방안이다.
	- `.editorconfig` 는 여기 걸리지 않는다. 편집기의 동작이 아니라 **그 파일이 tab 으로 쓰였는지 space 로 쓰였는지**라, 줄끝 형식이나 파일 이름으로 고르는 언어와 같은 갈래다(ADR-0048).
- plugin
	- 필요한 기능은 전부 코드로 작성한다.
	- 플러그인 구조를 제외 하여 복잡도를 줄인다.


## 빌드와 실행

Go 로 쓰였고 TUI 는 [bubbletea](https://github.com/charmbracelet/bubbletea) v2 와 lipgloss v2 다. plugin 도 설정 파일도 없어서 그 밖의 런타임 의존성이 없다. 결과물은 binary 하나다.

빌드에는 Go 1.26 이상과 GNU Make 4.3 이상이 필요하다. macOS 의 기본 make 는 3.81 이라 `brew install make` 로 받아 `gmake` 로 부른다.

```sh
make build      # build/zn 을 만든다
make test       # fmt, vet 을 거쳐 go test
make install    # prod 빌드를 $HOME/.local/bin 에 넣는다(INSTALL_DIR 로 바꾼다)
```

```sh
zn              # 이름 없는 빈 tab 하나로 시작한다
zn a.go b.go    # 인자로 준 파일이 각각 tab 이 된다
```

## Tips

### 화면을 긁어 복사하기

**`shift` 를 누른 채 드래그한다.** mouse 가 켜져 있는 동안 드래그는 편집기로 가지만, 대부분의 터미널이 `shift` 를 그 입력을 터미널 쪽으로 넘기는 용도로 쓴다.

다만 그렇게 긁으면 줄번호 칸이 같이 딸려오고 긴 줄이 wrap 된 자리에서 끊긴다. 파일의 글자 그대로가 필요하면 **`\c` 를 친다.** 편집 화면이 잠깐 물러나고 보고 있던 줄들이 `cat` 과 같은 평문으로 찍힌다. `Enter` 로 돌아온다.

- visual 로 고른 뒤 `\c` 를 치면 고른 것만 나온다
- `:cat` 도 같고, `:%cat` 은 파일 전체, `:1,50cat` 은 그 줄들이다
- command palette 의 「화면을 평문으로 내보내기」도 같다

붙여넣기는 손댈 것이 없다. insert mode 에서 터미널로 붙여넣으면 여러 줄이어도 그대로 들어간다.

### 한글 입력기 설정

한글로 쓰다가 `esc` 로 normal mode 에 나와도 자모는 두벌식 자리의 영문 키로 되돌려 받으므로 입력기를 영문으로 바꾸지 않고 그냥 명령을 칠 수 있다. 다만 **마지막으로 누른 키 하나는 입력기가 조합 중이라 편집기에 도착하지 않아서 명령이 한 키씩 밀린다.** 이것은 편집기 밖의 일이라 코드로 고칠 수 없다.

확실하게 하려면 mode 에 맞춰 입력기를 바꾸는 설정을 같이 쓴다. macOS 는 [Karabiner-Elements](https://karabiner-elements.pqrs.org/) 로 `esc` 에 영문 전환을 같이 걸고, Linux 는 쓰는 입력기에서 같은 것을 건다. 입력기는 zn 이 도는 곳이 아니라 키를 치는 쪽에 있으므로 ssh 로 붙어 쓸 때도 **로컬**에 건다.

## License

MIT. [LICENSE](LICENSE) 를 보십시오.

## FAQ

### 원하는 기능이 없거나, 동작을 바꾸고 싶습니다

fork 해서 코드를 고치고 빌드해 쓰십시오. 설정 파일도 플러그인도 없으니 바꾸고 싶은 동작이 코드에 그대로 있습니다. 코드는 AI 가 읽고 고치기 좋게 단순히 쓰려 하고 있으니, 원하는 것을 AI 에게 말하는 것으로 대개 됩니다.

### 왜 설정으로 빼지 않습니까

설정과 플러그인은 분기를 늘리고, 그 분기는 전부 유지보수가 됩니다. 쓰는 사람이 자기 것만 고치면 되는 자리에서는 코드를 고치는 편이 더 쌉니다. 무엇을 넣을지 빌드할 때 정하는 것이 드문 방식도 아닙니다.

- Linux 커널은 원래 쓰는 사람이 필요한 것만 골라 직접 컴파일해 썼습니다. 요즘은 배포판이 빌드해 준 것을 그대로 씁니다.
- nginx 는 필요한 모듈을 빌드할 때 골라 넣습니다. 동적 모듈은 한참 뒤에 붙었습니다.
- vim 도 기능 묶음을 컴파일 시점에 고릅니다. `tiny` 부터 `huge` 까지입니다.
