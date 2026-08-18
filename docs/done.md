

## non-milestone

- [x] normal/insert 전환을 구현한다. `i` 는 커서 앞, `a` 는 커서 뒤에서 insert 로 들어가고 `Esc` 로 normal 로 돌아온다. normal 의 커서는 글자 위에 있어 줄 끝 다음 칸에 설 수 없고, `Esc` 는 vim 처럼 왼쪽으로 한 글자 옮긴다
- [x] insert mode 에서 글자 입력을 구현한다. undo 기록과 같이 넣어야 한다(ADR-0001). `Enter`·`Backspace`·`Tab`·붙여넣기가 모두 `insert` 한 경로를 지나고, normal mode 의 `u`/`ctrl+r` 로 되돌리고 다시 적용한다
- [x] 저장 키를 연결한다. command-line mode 를 만들고 `:w`·`:q`·`:wq`·`:q!` 를 붙였다
- [x] 미저장 상태를 다룬다. statusBar 에 `[+]` 로 표시하고 `:q` 를 거부한다
- [x] `Ctrl+C` 의 종료 확인창이 미저장 상태를 모른다. `Ctrl+C` 와 `:q` 가 같은 `quit` 경로를 쓰게 하고, 저장하지 않은 변경이 있을 때만 확인창을 띄운다
- [x] `docs/spec.md` 에 편집 키 규칙을 반영한다
- [x] 줄바꿈(LF/CRLF) 과 tab 폭 처리 방식을 정한다. CRLF 는 파일 단위로 기억해 저장할 때 되돌리고, tab 폭은 8 칸(vim tabstop 기본값) 으로 한다
- [x] tab 이 화면에서 사라지는 것을 고친다. `ansi` 가 tab 을 폭 0 으로 보고 bubbletea 의 셀 렌더러가 폭 0 인 제어문자를 버려서, Go 소스를 열면 들여쓰기가 통째로 없어졌다. 렌더할 때 tab stop 까지 공백으로 펼치고 커서 열 계산도 같은 규칙을 쓴다
- [x] 종료 확인 화면이 대체 화면을 빠져나가는 것을 고친다. `viewQuitConfirm` 이 `AltScreen` 을 설정하지 않아 `Ctrl+C` 마다 셸 화면이 번쩍이고 편집 내용이 사라졌다. 부모 화면의 상태를 따라가게 한다
- [x] backing buffer 하나를 읽고 각 줄을 subslice(`[][]byte`) 로 두는 buffer 를 구현한다 (ADR-0001)
- [x] 한글 폭 계산과 grapheme cluster 처리를 구현한다. `ansi.FirstGraphemeCluster` 로 글자 경계와 폭을 같은 곳에서 얻는다. NFD 한글·결합 악센트·ZWJ 이모지·국기가 한 글자로 다뤄진다
- [x] 화면에 보이는 줄만 rune/폭 계산해서 렌더한다
- [x] 화면 스크롤을 구현한다. 커서가 화면 안이면 움직이지 않고, wrap 된 줄 안에서도 행 단위로 스크롤한다
- [x] `hjkl` 이동을 넣는다. `j`/`k` 는 논리 줄이고 `↓`/`↑` 는 화면 행이다. 앞에 숫자를 붙이면(`10j`, `20k`) 그만큼 움직인다. 키 입력은 `pending` 필드가 아니라 상태 기계(`normal-key-parser.go`) 로 받는다 (ADR-0006)
- [x] tabline 을 sidebar 위에 걸치지 않고 편집 영역 위에만 그린다. statusBar 는 sidebar 아래까지 이어지되 글자만 편집 영역 아래에서 시작한다 (ADR-0005)
- [x] 편집기를 열면 filetree 가 기본으로 열려 있게 한다. cwd 를 못 읽으면 트리 없이 연다
- [x] filetree 에서 `j` `k` 로 트리를 오르내린다. `↓` `↑` 와 같은 동작이다
- [x] 줄 갈아끼우기 방식으로 편집을 구현한다. backing buffer 에는 쓰지 않는다 (ADR-0001)
- [x] CLI 인자로 받은 파일을 실제로 연다
- [x] `core.Run` 이 항상 `Not Implemented` 를 반환하는 것을 고친다. 정상 종료해도 fatal 로그가 찍힌다
- [x] bootstrap commit
- [x] `:tabnew` 로 이름 없는 빈 tab 을 연다. 보고 있던 tab 바로 뒤에 생기고 그리로 옮겨간다
- [x] 명령줄 parser 를 만든다. `이름[!] [인자 ...]` 를 뜯어서 `command{name, force, args}` 로 준다. 따옴표와 `\` 로 공백이 든 파일 이름을 쓸 수 있다. `!` 가 flag 로 빠져서 `q`/`q!`/`qa`/`qa!` 네 case 가 둘로 줄었다
- [x] `:e <파일>` 과 `:tabnew <파일>` 로 편집 중에 파일을 연다. `:e` 는 보고 있는 tab 을 갈아끼우고 `:tabnew` 는 새 tab 을 연다. 둘 다 이미 열려 있는 파일이면 그 tab 으로 옮겨가기만 한다 — tab 마다 Buffer 가 독립이라 같은 파일이 둘이면 한쪽 저장이 다른 쪽 편집을 덮어쓴다(ADR-0015). 파일 이름은 하나만 받고, 인자를 받지 않는 나머지 명령은 그대로 「알 수 없는 명령」이다 (ADR-0021)
- [x] 읽은 뒤 밖에서 바뀐 파일을 모르고 덮어쓰지 않는다. 저장 직전에 파일을 다시 읽어 내용 해시로 맞춰 보고, 다르면 쓰지 않고 알린다. 없던 파일이 생긴 것과 있던 파일이 사라진 것도 막는다. 알고도 덮어쓰려면 `:w!` 다 (ADR-0015)
- [x] 파일 다시 읽기를 넣는다. command palette 의 「파일 다시 읽기」(`reload file`) 다. 저장하지 않은 변경이 있으면 확인창으로 묻고, undo 이력은 버리며 커서는 줄과 화면 칸을 유지한다. 밖에서 지워진 파일과 이름 없는 buffer 는 읽지 않고 알린다 (ADR-0016)
- [x] `:e` 로도 다시 읽는다. `:e!` 는 묻지 않고, 그냥 `:e` 는 저장하지 않은 변경이 있으면 확인창을 띄운다. 팔레트의 「파일 다시 읽기」와 같은 길이고 그 항목에 `:e` 를 짝 명령으로 붙였다 (ADR-0016, ADR-0021)
- [x] `docs/spec.md` 가 코드와 어긋난 곳을 맞춘다. 「아직 붙여넣는 키가 없다」(`p`·`y` 는 ADR-0017 로 들어왔다), space 마커를 `·`(U+00B7) 로 적은 것(실제는 `⋅` U+22C5), 백그라운드 작업이 둘이라는 것(「디렉터리 읽기」가 빠졌다) 셋을 고쳤다. 빠져 있던 `### search mode` 절과 「복사와 붙여넣기」 절을 쓰고, normal mode key 표에 `r` `y` `p` `P` `/` `?` `n` `N` `*` `#` `ctrl+p` `ctrl+w` `ctrl+z` 를 채웠다. visual·copy mode 는 미구현임을 절 머리에 적었다
- [x] `scripts/makefile.d/dev-run.mk` 의 `run`·`dev-run` 을 에디터에 맞게 고친다. web server scaffolding 이 남아 `$< -vvv server --config runtime/config.hjson` 을 실행하며, `server` 서브커맨드가 없어 `unknown long flag '--config'` 로 실패한다
- [x] `scripts/makefile.d/build.mk` 의 `gen` 타겟이 없는 `assets/src/js/index.js` 에 의존해 실패하는 것을 고친다. `prod` 와 `install` 이 이것 때문에 막혀 있다
- [x] `go.mod` 에서 쓰이지 않는 의존성을 정리한다. grpc, swaggo, prometheus, sqlite, sessions, hjson, expr-lang, validator, mongo-driver, gin-contrib, protobuf, gorm 을 걷어냈다
- [x] filetree 에서 파일 종류 마다 글자색을 달리, gitignore 의 파일은 회색등 연한 색으로. 무시 여부는 펼칠 때 `git check-ignore` 에 묻는다 (ADR-0005)
- [x] `:w` 마다 `git status --porcelain` 이 동기로 돌던 것을 비동기 작업으로 내린다. 저장하는 길에는 이제 git 호출이 없다 (ADR-0025, ADR-0030)
- [x] 검색 기능(`/` `?` `n` `N` `*` `#`) 을 넣는다. 패턴은 Go 정규식(RE2) 이고 flag 는 뒤에 `/i` 로 붙인다. 치는 동안 첫 매칭으로 따라가고(incsearch) `Esc` 로 되돌아온다. 찾은 자리는 모두 칠하되 커서가 선 것만 색이 다르다. 파일 끝에서 감싸고 그 사실을 알린다. `:noh` 로 강조를 끈다 (ADR-0010)
