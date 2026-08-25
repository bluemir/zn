package core

import (
	"bufio"
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/cockroachdb/errors"
)

// grepJobName 은 여러 파일 검색 작업의 이름이다. 인자가 패턴이라 `:jobs` 에서 이름 줄 아래로
// 접힌다 — 이름과 인자를 가른 자리가 이 쓰임을 내다보고 있었다(ADR-0075).
const grepJobName = "프로젝트 검색"

// grepMaxHits 는 담는 적중의 상한이다.
//
// **상한을 두는 첫 목록이다.** 알림 목록과 끝난 작업 목록은 코드에 있는 종류만큼만 늘어서
// 상한이 필요 없었는데(ADR-0030), 여기 오는 것은 사람이 친 패턴에 달렸다 — `.` 하나면
// 저장소의 모든 줄이 온다. 상한에 닿으면 그만두고 **닿았다는 것을 제목줄에 적는다.**
// 조용히 자르면 「이게 전부」로 읽힌다.
const grepMaxHits = 5000

// grepMaxFileSize 는 훑을 파일 크기의 상한이다. 이보다 크면 건너뛴다.
//
// 이 편집기가 열 수 있는 것보다 크게 잡았다 — 걸리는 것은 저장소에 들어온 데이터 덩어리다.
// 건너뛴 것은 세어서 요약에 적는다.
const grepMaxFileSize = 8 << 20

// grepProbeBytes 는 이진 파일인지 보려고 읽는 앞부분이다.
//
// NUL 이 있으면 이진으로 본다. git 이 쓰는 판정과 같고, 확장자로 가르지 않는 것은 저장소마다
// 이름 규칙이 다르기 때문이다.
const grepProbeBytes = 8000

// grepHit 은 적중 한 곳이다.
//
// 줄 내용을 같이 든다. 목록에 `경로:줄` 만 있으면 그 줄이 무엇인지 보려고 어차피 가 봐야 하는데
// (ADR-0073 이 `GOTO` 를 둘러보기 판으로 만든 까닭이다) 검색은 수백 곳이 와서 하나하나 가 볼
// 수가 없다. 내용이 목록에 있으면 눈으로 훑어 고른다(ADR-0077).
type grepHit struct {
	path string // 뿌리 기준 상대 경로
	line int    // 0 부터
	col  int    // 매칭이 시작하는 byte offset. 뛸 때 칸까지 맞춘다
	text string // 그 줄 통째로. 앞뒤 공백도 그대로다
}

// grepResult 는 검색 하나의 결과다. 작업이 채우고 결과 화면이 읽는다.
//
// editor 가 든다. 작업이 화면보다 오래 살고(닫았다 다시 열면 모아둔 것부터 보인다) 조각이
// 도착하는 자리가 `Update` 라 mode 와 무관하다 — 팔레트의 `files` 와 같은 자리다.
type grepResult struct {
	// input 은 친 그대로다. 제목줄에 적고, 목록 안에서 다시 강조할 때도 쓴다.
	input   string
	pattern *regexp.Regexp

	hits []grepHit

	// files 는 적중이 있던 파일 수다. 적중 수만으로는 「한 파일에 몰린 것」과
	// 「널리 퍼진 것」이 갈리지 않는다.
	files int

	// capped 는 상한에 닿아 그만두었는지다. 제목줄이 이것을 적는다.
	capped bool

	// skipped 는 너무 커서 건너뛴 파일 수다. 요약에 적는다.
	skipped int
}

// startGrep 은 검색 작업을 시작한다.
//
// **고치던 buffer 는 디스크가 아니라 buffer 의 글을 훑는다.** 사람이 화면에서 보고 있는 글을
// 찾아야 방금 친 이름이 바로 걸리고 지운 이름이 결과에 안 나온다. 그런데 작업은 goroutine 에서
// 돌고 buffer 는 `Update` 가 건드리므로, **시작하는 이 자리에서 복사해 실어 보낸다** — 작업이
// buffer 를 들여다보면 편집과 겹친다(ADR-0001).
//
// 복사하는 것은 dirty 인 buffer 뿐이다. 저장된 것은 디스크와 같아서 실어 보낼 값이 없다.
func (e *editor) startGrep(input string) tea.Cmd {
	pattern, err := parseSearchPattern(input)
	if err != nil {
		e.notifyError(err)

		return nil
	}

	root, err := os.Getwd()
	if err != nil {
		e.notifyError(err)

		return nil
	}

	// 앞선 결과는 여기서 비운다. 남겨 두면 새 검색이 도는 동안 지난 적중이 새 제목줄 아래
	// 붙어 있어서 무엇의 결과인지 갈리지 않는다.
	e.grep = grepResult{input: input, pattern: pattern}

	overlay := e.dirtyOverlay(root)

	return e.startJob(grepJobName, []string{input}, func(ctx context.Context) <-chan jobProgress {
		return grepFiles(ctx, root, input, pattern, overlay)
	})
}

// runGrep 은 검색을 시작하고 결과 판을 연다. 패턴을 받는 자리 셋이 이것으로 모인다 —
// `:grep <패턴>`, 아래 줄 입력창, 팔레트의 modal 이다.
//
// 결과를 기다리지 않는다. 판은 곧바로 열리고 적중이 도착하는 대로 목록이 자란다 —
// 팔레트가 인덱싱을 기다리지 않는 것과 같다(ADR-0011).
//
// 패턴이 정규식으로 말이 되지 않으면 startGrep 이 알림을 적고 nil 을 준다. 그때는 판을
// 열지 않는다 — 볼 것이 없다.
func runGrep(e *editor, input string) (tea.Model, tea.Cmd) {
	start := e.startGrep(input)
	if start == nil {
		return normalMode(e)
	}

	model, next := grepMode(e)

	return model, tea.Batch(next, start)
}

// dirtyOverlay 는 고치던 buffer 의 글을 뿌리 기준 상대 경로로 담은 것이다.
//
// 작업에 넘기고 나면 이쪽에서 건드리지 않는다. 줄을 이어 만든 새 byte 열이라 buffer 가
// 그 뒤로 바뀌어도 실어 보낸 것은 그대로다 — 검색 결과는 **시작한 순간의 글**이다.
func (e editor) dirtyOverlay(root string) map[string][]byte {
	overlay := map[string][]byte{}

	for _, buf := range e.buffers {
		if !buf.dirty || buf.path == "" {
			continue
		}

		// **buf.path 는 상대일 수도 절대일 수도 있다.** CLI 인자·트리·팔레트가 저마다
		// 주는 대로 담긴다(newBuffer 가 받은 것을 그대로 든다). 훑는 목록의 자는 뿌리 기준
		// 상대 경로라 거기에 맞춰야 얹을 자리를 찾는다 — 맞추지 않으면 Rel 이 실패해서
		// overlay 가 조용히 비고, 고치던 글이 검색에서 빠진다.
		full, err := filepath.Abs(buf.path)
		if err != nil {
			continue
		}

		rel, err := filepath.Rel(root, full)
		if err != nil || strings.HasPrefix(rel, "..") {
			// 뿌리 밖의 파일이다. 훑는 목록에 없으므로 얹을 자리도 없다.
			continue
		}

		overlay[rel] = buf.contents()
	}

	return overlay
}

// grepFiles 는 파일 목록을 훑어 패턴에 걸리는 줄을 모은다.
//
// 파일 목록은 팔레트와 같은 길로 얻는다(`gitFiles`·`walkFiles`) — `.gitignore` 가 공짜로
// 따라오고 저장소가 아니면 `.git` 만 건너뛴다. 검색만의 규칙을 새로 만들지 않는다.
//
// 조각은 파일 하나를 훑을 때마다 보낸다. `done`·`total` 이 파일 수라 `:jobs` 의 막대가
// 「몇 개 중 몇 개」를 그대로 보여준다. `apply` 는 그때까지 모은 적중을 통째로 넣는다 —
// 새 slice 를 만들지 않고 앞부분을 가리키게 두어서 보내는 쪽도 받는 쪽도 복사하지 않는다.
//
// ctx 가 끊기면 그만둔다. 보내다 막히는 자리마다 빠져나와서, 취소한 뒤 아무도 받지 않는
// 채널에 goroutine 이 남지 않는다(ADR-0027).
func grepFiles(ctx context.Context, root, input string, pattern *regexp.Regexp, overlay map[string][]byte) <-chan jobProgress {
	ch := make(chan jobProgress)

	send := func(progress jobProgress) bool {
		select {
		case ch <- progress:
			return true
		case <-ctx.Done():
			return false
		}
	}

	go func() {
		defer close(ch)

		files, ok := gitFiles(ctx, root)
		if !ok {
			files = walkFiles(ctx, root, func(int) bool { return ctx.Err() == nil })
		}
		if ctx.Err() != nil {
			return
		}

		hits := []grepHit{}
		seen := map[string]bool{}
		skipped := 0

		for i, path := range files {
			found, err := grepFile(filepath.Join(root, path), path, pattern, overlay[path])
			switch {
			case err != nil:
				// 읽을 수 없는 파일 하나로 검색 전체를 멈추지 않는다. 권한이 없거나
				// 훑는 동안 사라진 것이라 지나가는 것이 맞다.
				skipped++
			case len(found) > 0:
				hits = append(hits, found...)
				seen[path] = true
			}

			capped := len(hits) >= grepMaxHits
			if capped {
				hits = hits[:grepMaxHits]
			}

			// 마지막 파일이거나 상한에 닿았으면 이 조각이 결과를 굳힌다.
			last := capped || i == len(files)-1

			progress := jobProgress{
				done:  i + 1,
				total: len(files),
				apply: applyGrep(input, hits, len(seen), capped, skipped),
			}
			if last {
				progress.summary = grepSummary(len(hits), len(seen), capped, skipped)
			}

			if !send(progress) || last {
				return
			}
		}

		// 파일이 하나도 없었다. 빈 것도 결과이므로 한 번은 보낸다.
		send(jobProgress{
			apply:   applyGrep(input, nil, 0, false, skipped),
			summary: grepSummary(0, 0, false, skipped),
		})
	}()

	return ch
}

// applyGrep 은 모은 것을 editor 에 넣는 함수다.
//
// **패턴이 그 사이 바뀌었으면 넣지 않는다.** 앞 검색의 늦은 조각이 새 결과 위에 덮이는 것을
// 막는다 — 작업 이름이 같아도 인자가 다르면 다른 작업이라(ADR-0075) 둘이 겹칠 수 있다.
func applyGrep(input string, hits []grepHit, files int, capped bool, skipped int) func(*editor) {
	return func(e *editor) {
		if e.grep.input != input {
			return
		}

		e.grep.hits = hits
		e.grep.files = files
		e.grep.capped = capped
		e.grep.skipped = skipped
	}
}

// grepSummary 는 `:jobs` 의 끝난 목록에 남는 한 줄이다.
func grepSummary(hits, files int, capped bool, skipped int) string {
	summary := formatCount(hits) + " 곳"
	if capped {
		summary = formatCount(hits) + "+ 곳(상한)"
	}

	summary += " · 파일 " + formatCount(files)
	if skipped > 0 {
		summary += " · 건너뜀 " + formatCount(skipped)
	}

	return summary
}

// grepFile 은 파일 하나에서 걸리는 줄을 모은다.
//
// content 가 있으면 그것을 훑는다 — 고치던 buffer 다. 없으면 디스크를 읽는다.
//
// **이진 파일은 건너뛴다.** 앞부분에 NUL 이 있으면 이진으로 본다(git 과 같은 판정이고
// 확장자로 가르지 않는다). 너무 큰 파일도 건너뛴다 — 둘 다 오류가 아니라 빈 결과다.
func grepFile(full, rel string, pattern *regexp.Regexp, content []byte) ([]grepHit, error) {
	if content != nil {
		return grepLines(bytes.NewReader(content), rel, pattern)
	}

	info, err := os.Stat(full)
	if err != nil {
		return nil, err
	}
	if info.IsDir() || info.Size() > grepMaxFileSize {
		return nil, nil
	}

	file, err := os.Open(full)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	probe := make([]byte, min(int(info.Size()), grepProbeBytes))
	// ReadFull 은 요청보다 짧으면 EOF 계열을 준다. 파일이 probe 보다 작은 흔한 경우다.
	read, err := io.ReadFull(file, probe)
	if err != nil && !errors.Is(err, io.EOF) && !errors.Is(err, io.ErrUnexpectedEOF) {
		return nil, err
	}
	if bytes.IndexByte(probe[:read], 0) >= 0 {
		return nil, nil
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return nil, err
	}

	return grepLines(file, rel, pattern)
}

// grepLines 는 한 줄씩 읽으며 걸리는 줄을 모은다.
//
// 줄마다 첫 매칭만 담는다. 같은 줄에 두 번 걸려도 목록에 같은 글이 두 줄 서면 훑는 데
// 방해가 된다 — 그 줄로 뛴 뒤 `n` 이 나머지를 짚는다.
//
// 줄이 아주 길면 건너뛴다. 한 줄이 수 MB 인 파일(작게 만든 js, 데이터 덩어리) 이 목록을
// 통째로 먹는 것을 막는다.
func grepLines(r io.Reader, rel string, pattern *regexp.Regexp) ([]grepHit, error) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64<<10), grepMaxLineLen)

	hits := []grepHit{}
	for line := 0; scanner.Scan(); line++ {
		at := pattern.FindIndex(scanner.Bytes())
		if at == nil {
			continue
		}

		hits = append(hits, grepHit{
			path: rel,
			line: line,
			col:  at[0],
			text: string(scanner.Bytes()),
		})
	}

	// 너무 긴 줄을 만난 것은 오류가 아니다. 거기까지 모은 것을 준다.
	if err := scanner.Err(); err != nil && !errors.Is(err, bufio.ErrTooLong) {
		return hits, err
	}

	return hits, nil
}

// grepMaxLineLen 은 훑을 한 줄의 길이 상한이다.
const grepMaxLineLen = 1 << 20
