# ADR-0042: git 은 프로세스를 띄우지 않고 go-git 으로 읽는다

- 상태: 채택
- 날짜: 2026-08-21

## 맥락

지금까지 git 은 전부 외부 프로세스였다. 세 자리다.

- `internal/core/git.go` — `rev-parse --short HEAD`, `symbolic-ref --quiet --short HEAD`,
  `status --porcelain`. statusBar 오른쪽의 `master(a1b2c3d*)` 다(ADR-0009, ADR-0030)
- `internal/core/sidebar.go` — `check-ignore -z --stdin`. 트리에서 무시된 항목을 회색으로(ADR-0005)
- `internal/core/palette.go` — `ls-files -z --cached --others --exclude-standard`. 팔레트 파일 목록(ADR-0011)

ADR-0009 가 이미 go-git 을 대안으로 적고 기각했다 — "의존성이 크게 늘고 status 계산은 오히려
`git` 명령보다 느리다". `docs/tasks.md` 에는 "git 구현을 내부 라이브러리를 사용해서 구현할수
없는지 확인" 이 그대로 열려 있었다.

기각의 근거를 실제로 재보니 **절반이 틀렸다.**

**`Worktree.Status()` 는 정말 느리다.** 재보면 이렇다.

| 저장소 | 추적 파일 | 디스크 파일 | `git status` | go-git v5.19.2 | v6.0.0-alpha.5 |
|---|---|---|---|---|---|
| zn | 215 | 1,608 | 8.4ms | 9.6ms | 21.0ms |
| llama.cpp | 2,595 | 2,623 | 16.8ms | 107ms | 426ms |
| 사내 monorepo | 5,845 | 130,036 | 55.5ms | 5.84s | 2.41s |

**그런데 이유가 「계산이 무겁다」가 아니다.** go-git v5.19.2 에는 index 의 stat cache 가 이미
있고(`utils/merkletrie/filesystem/node.go` 의 `metadataMatches`), 200MB 를 추적하는 저장소도
0.6ms 에 답한다 — 내용을 다시 hash 하지 않는다. 느린 이유는 **무시된 디렉터리 안을 훑는 것**
하나다. `node_modules` 를 `.gitignore` 에 적어 두었는데도 들어간다.

두 군데서 들어간다.

1. **v5 의 `Worktree.Status()` 는 작업 트리를 통째로 훑은 다음** 그 결과를 무시 규칙으로
   걸러낸다(`excludeIgnoredChanges`). 훑기는 이미 끝난 뒤다.
   v6 가 `IgnoreMatcher` 를 넣어 이 부분을 고쳤다.
2. **`gitignore.ReadPatterns` 가 재귀할 때 부모의 규칙을 물려주지 않는다.** v5·v6 둘 다 그렇다.
   `.gitignore` 파일을 모으느라 한 번 더 훑는데, 재귀한 자리에서는 규칙 목록이 비어 있어서
   뿌리에 적은 `node_modules` 가 `packages/*/node_modules` 를 가지치기하지 못한다.

무시 파일 1 만 개를 `a/node_modules` 에 고정하고 **규칙의 위치만** 바꿔 재면 이렇게 갈린다.

| `.gitignore` 위치 | go-git v6 | git |
|---|---|---|
| `a/.gitignore` 에 `node_modules` | 2.6ms | 6.9ms |
| 뿌리 `.gitignore` 에 `node_modules` | 14.6ms | 6.1ms |
| 뿌리에 `/a/node_modules` | 23.4ms | 6.8ms |

파일 위치도 개수도 그대로다. 규칙이 바로 위 디렉터리에 있을 때만 가지치기가 된다. 판정 결과는
세 경우 다 맞다 — 순전히 값이다.

이것은 [go-git#181](https://github.com/go-git/go-git/issues/181) 로 **2020-10-12 부터 열려 있고**
라벨이 `help wanted`·`performance`·`stale` 이다. 담당자도 PR 도 없다. 전신은
[src-d/go-git#844](https://github.com/src-d/go-git/issues/844) 다. 그 이슈의 재현 사례가 정확히
`node_modules` 다.

## 결정

**go-git v5 를 쓰되 `Worktree.Status()` 는 쓰지 않는다.** 필요한 것만 직접 조립한다.

**HEAD·index·object 는 라이브러리에게 맡긴다.** `git.PlainOpenWithOptions` 로 열고
`repo.Head()`·`repo.Storer.Index()`·`repo.TreeObject()` 를 쓴다. packed-refs, detached HEAD,
worktree, index 형식은 우리가 알 것이 아니다.

**무시 규칙은 직접 모은다(`git-ignore.go`).** `gitignore.ReadPatterns` 를 부르지 않고,
층마다 `.gitignore` 를 읽어 **부모의 규칙을 물려주며** 내려간다. 규칙 문법 자체는 라이브러리의
`gitignore.ParsePattern`·`Matcher` 를 그대로 쓴다 — 문법을 다시 구현하지는 않는다.
`git ls-files --exclude-standard` 가 세는 네 가지를 순서대로 읽는다: 시스템,
사용자(`core.excludesFile`), `.git/info/exclude`, 각 층의 `.gitignore`.

**작업 트리 훑기는 무시된 디렉터리에 들어가지 않는다(`walkGitFiles`).** `filepath.WalkDir` 을
쓰지 않는다 — 규칙이 층마다 쌓이므로 지금 어느 층인지를 들고 다녀야 하고, 평평한 callback
으로는 그 자리를 알 수 없다.

**dirty 는 셋을 본다.** `git status` 가 clean 이라고 부르는 기준 그대로다(ADR-0009).

1. index ↔ HEAD (`git add` 만 한 것)
2. index ↔ 작업 트리 (고쳐서 저장한 것, 지운 것, 권한이 바뀐 것)
3. 추적하지 않고 무시되지도 않는 파일

**싼 것부터 보고, 참이면 그 자리에서 끝낸다.** 1 번은 index 에 딸린 cached tree(git 의 TREE
확장) 의 뿌리 해시를 HEAD 트리와 견주는 것으로 대개 끝난다. 2 번은 추적 파일마다 `Lstat` 이고,
크기·mtime·mode 가 index 와 같으면 내용을 읽지 않는다. 3 번이 가장 비싸다.

**metadata 가 다른 것은 「바뀌었을 수도 있다」 까지다.** 그때는 내용을 읽어 blob 해시를 견준다.
mtime 만 바뀌고 내용은 그대로인 경우와 racy git 이 여기로 온다.

**racy git 을 본다.** 파일 mtime 이 index 파일 자신의 mtime 과 같거나 더 새로우면 metadata 를
믿지 않고 내용을 읽는다. 같은 순간에 저장되면 크기도 mtime 도 index 와 같은 채로 내용이 다를
수 있다.

**권한 차이는 `core.fileMode` 를 따른다.** 꺼져 있으면 실행 권한만 다른 것은 변경으로 보지
않는다. symlink 가 되었거나 submodule 이 된 것은 설정과 무관하게 변경이다.

**`.git/HEAD` 가 저장소의 것처럼 보이는지 확인한다(`gitHeadLooksValid`).** go-git 은 진짜 git
보다 관대해서 `.git/HEAD` 에 아무 글자나 있어도 저장소로 열고, `Head()` 가 오류 대신 전부 0 인
해시를 준다. 상징 참조는 그대로 통과시키고(갓 `git init` 한 저장소가 그렇다) 해시로 적힌 것만
0 이 아닌지 본다.

**뿌리 기준 상대 경로 계산은 `openGitRepo` 한 곳에서 한다.** go-git 은 작업 트리 뿌리에서
symlink 를 풀어 주는데 부르는 쪽 경로는 안 풀린 채일 수 있다(macOS 의 `/tmp` 가
`/private/tmp` 다). 이것이 어긋나면 무시 규칙이 통째로 듣지 않는다.

**bare 저장소는 저장소가 아닌 것으로 다룬다.** 작업 트리가 없으면 무시 규칙도 dirty 도 볼 것이 없다.

**짧은 해시는 일곱 자리 고정이다.** `rev-parse --short` 는 앞자리가 겹치지 않을 만큼 늘리지만,
늘리려면 객체를 훑어야 한다.

**의존성은 `git.PlainOpen` 전체를 쓴다.** `plumbing/format/{index,gitignore}` 만 쓰고 ref 해석을
직접 짜면 바이너리가 +2MB 로 그치지만(전체는 +6.9MB), packed-refs·worktree·detached HEAD 를
우리가 감당하게 된다.

## 근거

### 왜 `Worktree.Status()` 를 피하는가

**그것 하나가 문제의 전부다.** 피하면 남는 것은 index 읽기와 우리가 통제하는 훑기뿐이다.
같은 세 저장소에서 재보면 이렇다.

| 저장소 | `git status` | `Worktree.Status()` | 이 결정 |
|---|---|---|---|
| zn | 8.4ms | 21ms | **1ms** |
| llama.cpp | 16.8ms | 426ms | **65ms** |
| 사내 monorepo | 55.5ms | 2,412ms | **120ms** |

44 배가 2 배 남짓이 된다. 나눠 보면 monorepo 의 120ms 는 staged 0ms, 설정 읽기 0ms,
추적 파일 `Lstat` 10ms, 그리고 **훑기 139ms** 다 — 사실상 훑기 하나다.

**그래서 v5 로 갈 수 있다.** v6 의 개선(`IgnoreMatcher`) 이 필요한 자리를 안 쓰므로 alpha 를
물고 갈 이유가 없다. v6 는 llama.cpp 처럼 무시 파일이 거의 없는 저장소에서 오히려 v5 보다
4 배 느리기도 하다.

### 왜 규칙을 직접 모으는가

**라이브러리의 것이 이 한 가지를 못 한다.** 문법·우선순위·부정(`!`) 은 그대로 쓴다 —
다시 구현하는 것은 「어느 `.gitignore` 를 어느 순서로 읽는가」뿐이고, 그것이 고장난 부분이다.

**층을 들고 다니는 것이 오히려 맞는 모양이다.** git 도 그렇게 센다. sidebar 는 트리를 훑지
않고 한 디렉터리만 보므로 뿌리부터 그 자리까지 깊이만큼만 읽으면 되고(`gitIgnoreAt`),
팔레트와 dirty 판정은 내려가며 한 층씩 얹는다. 한 primitive 로 셋이 다 된다.

### 왜 dirty 를 셋으로 나눠 보는가

**`git status` 가 그 셋을 본다.** ADR-0009 가 "기준이 두 가지면 「표시는 깨끗한데 `git status`
는 아니다」 를 매번 설명해야 한다" 고 적어 둔 그대로다. 처음 구현에서 index ↔ HEAD 를 빠뜨려
`git add` 만 한 파일을 놓쳤고, 시험이 그것을 잡았다.

**기대값을 손으로 적지 않고 git 에게 묻는다.** `git-dirty_test.go` 는 경우마다 fixture 를
세우고 우리 판정과 `git status --porcelain` 을 견준다. 손으로 적으면 「git 과 같은 기준」이라는
목표가 시험에서 빠진다. 실행 권한 경우가 그렇게 잡혔다 — 내용만 견주고 mode 를 잃고 있었다.

### 왜 cached tree 를 앞잡이로 쓰는가

**staged 검사가 공짜가 된다.** 세 저장소 모두 캐시가 살아 있어서 해시 비교 한 번으로 끝난다.
`git add` 는 그 캐시를 깨므로, 정작 볼 것이 있을 때만 HEAD 트리를 훑는다(monorepo 에서 57ms).

### 왜 그래도 라이브러리인가

값이 프로세스보다 싸지 않은데도 옮기는 이유는 셋이다.

**`git` 바이너리가 없어도 동작한다.** 이것이 라이브러리만이 주는 것이다.

**취소가 in-process 다.** ctx 를 끊으면 훑기가 그 자리에서 멎는다. 프로세스를 죽이는 것과
결과는 같지만 중간까지 모은 것을 다룰 여지가 생긴다.

**글자를 파싱하지 않는다.** `--porcelain` 출력과 `-z` 구분자를 다루던 코드가 없어진다.
앞으로 붙일 것(트리의 파일별 dirty 마커, gutter) 이 구조를 그대로 쓴다.

## 결과

좋은 점

- `git` 바이너리가 없는 환경에서도 표시가 나온다.
- 큰 저장소에서 dirty 판정이 20 배 빨라졌다(monorepo 2.4s → 120ms). 무시된 디렉터리에 들어가지
  않는 것 하나가 그 차이다.
- 유휴 상태에 프로세스가 뜨지 않는다. 5 초마다 세 개씩 뜨던 것이 없어졌다.
- 세 자리(statusBar·트리·팔레트) 가 같은 무시 규칙과 같은 훑기를 쓴다. 기준이 갈릴 자리가 없다.
- 무시 규칙 판정이 `git check-ignore` 와 달라도 시험이 잡는다 — 기준선이 진짜 git 이다.
- go-git 이 진짜 git 보다 관대한 자리(망가진 `.git`) 를 막아서, 예전에 없던 `0000000` 표시가
  생기지 않는다.

감수하는 것

- **바이너리가 14.7MB 에서 21.6MB 로 는다.** 링크되는 go-git 계열 패키지가 93 개다.
  `plumbing` 만 쓰면 +2MB 로 그치지만 ref 해석을 직접 짜게 된다(위 결정에서 고르지 않았다).
- **`git status` 보다 2 배 남짓 느리다.** git 은 C 이고 `core.untrackedCache` 로 훑기를
  캐시한다. 우리는 캐시가 없다. cooldown(ADR-0043) 이 이 값을 감당할 수 있게 만든다.
- **짧은 해시가 일곱 자리로 고정이다.** 앞자리가 겹치는 큰 저장소에서 `git log` 와 달라 보인다.
- **`core.autocrlf` 를 보지 않는다.** 줄끝을 바꿔 저장하는 설정을 켠 저장소에서 dirty 판정이
  틀릴 수 있다. macOS·Linux 기본값이 아니다.
- **submodule 안이 dirty 한지 보지 않는다.** submodule 이 가리키는 commit 이 바뀐 것은 잡지만,
  그 안에서 고친 것은 잡지 않는다. `git status` 는 잡는다.
- **`core.excludesFile` 을 `~/.gitconfig` 에서만 읽는다.** go-git 의 `LoadGlobalPatterns` 가
  `~/.config/git/config` 를 보지 않는다. 그쪽에만 적어 둔 사람은 전역 무시 규칙이 듣지 않는다.
- **무시 규칙 판정이 `git check-ignore` 와 bit 단위로 같지는 않다.** 문법은 라이브러리 것이지만
  그것이 git 과 완전히 같다는 보장은 없다. 틀리면 트리에서 회색이 잘못 붙는다.
- **upstream 이 고쳐지면 우리 쪽이 남는다.** `ReadPatterns` 가 규칙을 물려주게 되면 우리가
  직접 모으는 코드는 값이 없어진다. 지우면 되지만 그때까지는 우리가 든다.
- **저장소마다 설정을 세 번 읽는다.** `core.fileMode` 를 보느라 판정마다 시스템·전역·지역
  설정을 읽는다. 재보면 0ms 라 두었다.

## 대안

**`Worktree.Status()` 를 그대로 쓴다** — 코드가 훨씬 적다. monorepo 에서 2.4 초라
cooldown 을 얹어도 duty cycle 이 32% 다. 노트북 배터리로 편집기를 띄워두는 값이 아니다.

**v6-alpha 를 쓴다** — 뿌리 직속 무시 디렉터리는 고쳐져 있다. 중첩은 그대로 남아서
monorepo 가 2.4 초이고, 무시 파일이 없는 저장소에서는 v5 보다 느리다. alpha 를 물 이유가 없다.

**프로세스를 그대로 둔다** — 값이 가장 싸고 판정이 늘 git 과 같다. `git` 이 없는 환경에서
표시가 없고, 앞으로 붙일 git 기능마다 출력 파싱을 늘린다. `docs/tasks.md` 의 항목을 닫는 것으로
끝낼 수도 있었다.

**`plumbing` 만 쓰고 ref 해석을 직접 짠다** — 바이너리가 +2MB 로 그친다. packed-refs·worktree·
detached HEAD 를 우리가 감당하게 되고, ADR-0009 가 "git 이 이미 아는 것을 다시 구현한다" 며
기각한 그 일이다.

**upstream 을 고친다** — `ReadPatterns` 에 부모 규칙을 밀어 넣는 수정 자체는 작다. 고쳐지고
풀려서 우리가 쓸 수 있게 되기까지가 이 결정을 미룰 만한 기간이 아니다. 따로 낼 일이다.

## 이 ADR 이 정하지 않는 것

- upstream 에 issue·PR 을 낼지. 별개 건이다(`docs/tasks.md`)
- 훑기를 캐시할지. git 의 `core.untrackedCache` 에 해당하는 것이 없다. 값이 거슬리면 그때 본다
- submodule 안의 dirty. 지금은 보지 않는다
- 짧은 해시를 겹치지 않을 만큼 늘릴지
- `git log`·graph 같은 새 기능(`docs/tasks.md`). 바탕은 생겼지만 무엇을 어떻게 보일지는 별개다
- 트리의 파일별 dirty 마커(`docs/tasks.md`). 판정을 파일별로 돌려주게 바꾸는 일이 앞에 있다
- 팔레트 인덱싱의 진행 표시. 훑기가 오래 걸리는 저장소에서 `gitFiles` 는 진행을 알리지 않는다
