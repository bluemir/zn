# ADR-0035: `command` 는 `:` 로 치는 것이고, 키로 치는 것은 `action`(동작) 이다

- 상태: 채택
- 날짜: 2026-08-18

## 맥락

편집기에는 사용자가 시키는 것이 두 갈래 있다. `:w` 처럼 명령줄에 치는 것과 `dw` 처럼 키로 치는 것이다. 우리는 말로 그것을 「command line command」 와 「normal mode command」 라 불러 왔고, 이름에 같은 낱말이 들어 있어서 대화에서도 코드에서도 어느 쪽인지 되물어야 했다.

ADR-0034 로 키 쪽이 type 이 되면서 겹침이 코드에 드러났다. 수식어 없는 `command` 는 `:` 한 줄을 뜯어 놓은 struct(command-parser.go) 인데, 키 쪽 type 24 개가 `command*` 접두를 쓰게 되어 자동완성에서 `command` 를 치면 두 갈래가 한 덩어리로 나왔다.

**vim 도 같은 겹침을 안고 있고 수식어로만 푼다.** vim 9.1 문서로 확인했다. `index.txt` 의 절 제목이 `2. Normal mode` 와 `6. EX commands` 이고, 본문은 "Normal mode command"(67 곳) 와 "Ex command"(211 곳) 로 쓴다. 키 쪽만 가리키는 낱말은 없다. 고유한 낱말이 있는 것은 그 **안쪽** 것들뿐이다. `operator`, `{motion}`, text object 다. "action" 은 vim 문서에 0 번 나온다.

## 결정

**수식어 없는 `command`(한국어로 「명령」) 는 `:` 로 치는 것이 가진다.** `command` struct 와 `parseCommand`(command-parser.go), `viewEditorCommand`·`commandMode`(view-editor-command.go), statusBar 에 찍히는 `COMMAND`, 파일 이름 `command-parser.go`·`view-editor-command.go` 가 전부 이 갈래다.

**키로 치는 것은 `action`(한국어로 「동작」) 이다.** interface `action` 과 그것을 구현하는 `actionMove`·`actionDelete` 24 개(action.go), 그리고 그것을 만드는 `standaloneAction`· `prefixAction`(normal-key-parser.go) 이다.

**「동작」은 normal mode 에 한정하지 않는다.** sidebar(TREE) 와 `:jobs` 목록도 키가 곧 동작이므로 그 주석·문서에서도 「동작」이다. 그쪽은 아직 type 이 아니라 이름 문자열이지만 (sidebar-key-parser.go, jobs-key-parser.go) 부름은 같다.

**`motion` 은 그대로다.** vim 에도 고유한 낱말이 있고 우리도 그것을 쓴다. 동작이 motion 을 들고 다닌다(`actionDelete{motion: motionWordForward{}}`).

## 근거

**`command` 는 이미 `:` 쪽 어휘였다.** 위에 열거한 곳, 곧 mode model, mode 진입 함수, 화면에 찍히는 글자, 파일 이름 둘, 테스트 파일 둘이 전부 `:` 를 가리킨다. 키 쪽의 `command*` 는 ADR-0034 직후에 붙은 이름이라 아직 얕았다. 옮기는 값이 한쪽으로 크게 기울어 있다.

**사용자가 「명령」이라 부르는 것은 `:` 쪽이다.** 화면에 `COMMAND` 라고 찍히고, 「알 수 없는 명령」이라는 오류도 그 줄에서 난다. 키를 치는 사람은 그것을 명령이라 부르지 않고 그냥 키라고 부른다.

**`motion` 과 대칭이 생긴다.** interface `motion` + 구현 `motion*`, interface `action` + 구현 `action*` 이다. 둘의 관계도 이름으로 읽힌다.

**자동완성이 갈래를 모아준다.** `action` 까지 치면 키 동작 24 개가, `command` 까지 치면 `:` 쪽이 나온다. 접두로 묶는 이유가 이것이다.

## 결과

- `command.go` → `action.go`. type 24 개와 interface, 표를 만드는 함수 둘까지 155 곳이 옮겨졌다
- 한국어 주석·문서에서 키로 치는 것은 「동작」이다. Go 주석 약 60 곳과 ADR 여덟 개 (0006·0008·0013·0014·0017·0018·0033·0034), `spec.md`, `tasks.md` 를 맞췄다
- `:`·팔레트·명령줄·`git` 호출을 가리키는 「명령」은 그대로다 (ADR-0011·0016·0021·0024 등은 한 글자도 바뀌지 않았다)
- **「동작」이 behavior 뜻으로 쓰이던 뜻과 부딪힌다.** 새 어휘 옆에 있는 것만 고쳐 썼다. 「동작은 같다」→「결과는 같다」, 「런타임 무동작」→「런타임에 아무 일도 하지 않는 것」이다. `동작한다`(작동한다) 같은 동사형은 문맥이 분명해서 그대로 두었다
- ADR-0034 에 `이후:` 줄로 이름이 옮겨간 사실을 적었다. 파일 이름 (`ADR-0034-parsed-command-knows-how-to-run.md`) 은 그대로다. ADR 의 신원은 번호다

## 대안

- **`:` 쪽을 `exCommand` 로**
	- vim 정식 용어이고 옮길 곳이 스무 곳뿐이라 가장 쌌다.
	- `ex` 가 vim 계보를 아는 사람에게만 읽히는 낱말이고, 우리가 그 갈래를 「명령줄 명령」이라 부르는 것과도 어긋난다. 우리 어휘에 없는 말을 들이는 것이 대가다.
- **`:` 쪽을 `commandLine` 로**
	- 우리가 부르는 말 그대로다.
	- 이름이 `command*` 접두 가족 사이에 섞여서(`commandLine` 이 `commandMove`·`commandDelete` 와 나란히 선다) 겹침이 자동완성 목록 안에서 되살아난다.
	- 키 쪽에 「명령줄을 여는 동작」이 있어서 `commandOpenCommandLine` 처럼 같은 낱말이 한 이름에 두 번 든다.
- **vim 처럼 양쪽에 수식어를 붙인다(`normalCommand*` 와 `exCommand`)**
	- 이름만 보고 어느 갈래인지 알 수 있어 가장 정확하다.
	- 이름이 길어지고(`normalCommandOpenCommandLine`) 그래도 `ex` 를 들여와야 한다.

## 이 ADR 이 정하지 않는 것

- sidebar·`:jobs` 의 키 동작을 `action` type 으로 올릴지. 지금은 이름 문자열이고, 숫자도 받지 않아서 type 이 할 일이 없다(ADR-0034 가 같은 이유로 미뤘다). 부름만 「동작」으로 맞췄다
- `:` 명령을 type 으로 만들지. `run` 의 `switch cmd.name` 은 아직 평평하다
- CLAUDE.md 에 이 어휘를 규칙으로 적을지. 지금은 적지 않는다. 코드와 이 ADR 이 근거다
