package lsp

import (
	"bytes"
	"encoding/json"
	"slices"
	"time"

	"github.com/cockroachdb/errors"
)

// Format 은 그 파일의 글 in 을 서버에게 맞추게 하고 맞춘 글을 준다(ADR-0160).
//
// **in 을 먼저 서버에 맞춘다.** 저장은 아직 쓰지 않은 buffer 를 넘기는 길이라(ADR-0052)
// 서버가 든 판이 그 글이어야 받은 편집이 그 글의 자리를 가리킨다.
//
// **기한이 있다.** 저장은 이 답을 기다리며 화면을 멈춘다. 바깥 포매터와 같은 한계를 부르는
// 쪽이 준다(core 의 saveHookTimeout). 넘기면 답은 버리고 오류를 준다 — 늦게 온 답은 그때의
// 글이 아니다.
//
// 서버가 `null` 로 답하면 고칠 것이 없다는 뜻이라 in 을 그대로 준다. rust-analyzer 는 crate 를
// 읽는 동안에도 `null` 이고, 바쁠 때는 `content modified` 오류로 답한다(재서 본 값이다).
// 셋 다 「지금은 맞추지 않았다」라 같이 그대로 둔다. 저장 하나 때문에 되묻고 기다리면 화면이
// 그만큼 더 멈춘다.
func (c *Client) Format(path string, in []byte, timeout time.Duration) ([]byte, error) {
	// 받은 글은 마지막 줄바꿈까지 든 전문이다. 서버에 보내는 판(text) 은 줄마다 `\n` 을 붙이므로
	// 마지막 하나를 떼고 갈라야 보낸 글이 in 과 byte 하나까지 같다.
	lines := bytes.Split(bytes.TrimSuffix(in, []byte{'\n'}), []byte{'\n'})
	if err := c.SyncFull(path, lines); err != nil {
		return nil, err
	}

	type answer struct {
		result json.RawMessage
		err    error
	}

	reply := make(chan answer, 1)
	go func() {
		result, err := c.conn.call("textDocument/formatting", map[string]any{
			"textDocument": textDocumentIdentifier{URI: fileURI(path)},
			// 규격이 반드시 싣게 한 칸이다. rust-analyzer 는 이것을 보지 않고 rustfmt 설정을 따른다.
			"options": map[string]any{"tabSize": 4, "insertSpaces": true},
		})
		reply <- answer{result: result, err: err}
	}()

	var got answer
	select {
	case got = <-reply:
	case <-time.After(timeout):
		return nil, errors.Errorf("%s 안에 끝나지 않았습니다", timeout)
	}

	if response := (*responseError)(nil); errors.As(got.err, &response) && response.Code == contentModified {
		return in, nil
	}

	if got.err != nil {
		return nil, got.err
	}

	if len(got.result) == 0 || string(got.result) == "null" {
		return in, nil
	}

	var edits []TextEdit
	if err := json.Unmarshal(got.result, &edits); err != nil {
		return nil, errors.Wrap(err, "맞춘 답을 읽지 못했다")
	}

	return applyTextEdits(in, edits), nil
}

// applyTextEdits 는 글 전체에 편집들을 얹은 새 글이다.
//
// **줄을 넘는 편집을 받는다.** 포매터의 답은 `{` 앞뒤의 줄바꿈을 넣고 빼는 것이라 거의 언제나
// 줄을 넘는다. 그래서 자리를 글 전체의 byte offset 으로 바꿔서 얹는다.
//
// **뒤에서 앞으로 얹는다.** 앞부터 얹으면 뒤 편집이 가리키는 자리가 밀린다. 규격은 편집끼리
// 겹치지 않는다고 정한다. 글 밖을 가리키는 자리는 글 끝으로 당긴다.
func applyTextEdits(in []byte, edits []TextEdit) []byte {
	starts := []int{0}
	for i, b := range in {
		if b == '\n' {
			starts = append(starts, i+1)
		}
	}

	offset := func(pos Position) int {
		if pos.Line < 0 {
			return 0
		}

		if pos.Line >= len(starts) {
			return len(in)
		}

		lineEnd := len(in)
		if pos.Line+1 < len(starts) {
			lineEnd = starts[pos.Line+1] - 1
		}

		line := in[starts[pos.Line]:lineEnd]

		return starts[pos.Line] + min(ByteColumn(line, pos.Character), len(line))
	}

	sorted := slices.Clone(edits)
	slices.SortStableFunc(sorted, func(a, b TextEdit) int {
		return offset(b.Range.Start) - offset(a.Range.Start)
	})

	out := slices.Clone(in)
	for _, edit := range sorted {
		start := offset(edit.Range.Start)
		end := max(offset(edit.Range.End), start)

		out = slices.Concat(out[:start], []byte(edit.NewText), out[end:])
	}

	return out
}
