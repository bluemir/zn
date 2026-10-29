package lsp

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// 여기서는 진짜 rust-analyzer 에게 묻는다. gopls·pyright 쪽과 같은 자리이고 같은 까닭이다
// (ADR-0051, ADR-0107).
//
// rust-analyzer 가 없으면 건너뛴다. `-short` 로도 건너뛴다.

// startRustForTest 는 crate 하나가 든 임시 자리에 rust-analyzer 를 띄운다.
//
// **Cargo.toml 을 같이 둔다.** rust-analyzer 는 파일 하나가 아니라 crate 를 읽는다. 그것이
// 없으면 정의도 완성도 오지 않는다.
func startRustForTest(t *testing.T) (*Client, string) {
	t.Helper()

	if testing.Short() {
		t.Skip("rust-analyzer 에게 실제로 묻는 시험이라 -short 에서는 건너뛴다")
	}

	root := t.TempDir()

	server := serverNamed("rust-analyzer")
	require.NotNil(t, server, "표에 rust-analyzer 줄이 있어야 한다")

	if _, err := server.find(root); err != nil {
		t.Skip("rust-analyzer 가 없다: " + err.Error())
	}

	require.NoError(t, os.WriteFile(filepath.Join(root, "Cargo.toml"), []byte(
		"[package]\n"+
			"name = \"probe\"\n"+
			"version = \"0.1.0\"\n"+
			"edition = \"2021\"\n"), 0o644))

	require.NoError(t, os.MkdirAll(filepath.Join(root, "src"), 0o755))

	path := filepath.Join(root, "src", "main.rs")
	require.NoError(t, os.WriteFile(path, []byte(
		"struct Greeter {\n"+
			"    name: String,\n"+
			"}\n"+
			"\n"+
			"impl Greeter {\n"+
			"    fn greet(&self) -> String {\n"+
			"        format!(\"안녕 {}\", self.name)\n"+
			"    }\n"+
			"}\n"+
			"\n"+
			"fn main() {\n"+
			"    let greeter = Greeter { name: \"세상\".to_string() };\n"+
			"    println!(\"{}\", greeter.greet());\n"+
			"}\n"), 0o644))

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	client, err := Start(ctx, root, server)
	require.NoError(t, err)
	t.Cleanup(client.Shutdown)

	return client, path
}

// waitRust 는 rust-analyzer 가 crate 를 다 읽을 때까지 같은 것을 되묻는다.
//
// **악수가 끝나도 곧바로 답하지 않는다.** 뒤에서 `cargo metadata` 와 표준 라이브러리를
// 읽는 동안에는 빈 답이 온다. gopls 는 첫 요청이 기다렸다가 답하는데 이쪽은 비어서 온다.
//
// 빈 답 대신 `content modified`(-32801) 오류가 오기도 한다. 기계가 바쁠 때 그랬다(시험을
// 한꺼번에 돌릴 때다). 규격은 그것을 「다시 물어라」로 정한다.
func waitRust[T any](t *testing.T, ask func() ([]T, error)) []T {
	t.Helper()

	started := time.Now()
	for time.Since(started) < 60*time.Second {
		got, err := ask()
		if response := (*responseError)(nil); errors.As(err, &response) && response.Code == contentModified {
			time.Sleep(200 * time.Millisecond)

			continue
		}
		require.NoError(t, err)

		if len(got) > 0 {
			t.Logf("답이 오기까지 %s", time.Since(started))

			return got
		}

		time.Sleep(200 * time.Millisecond)
	}

	t.Fatal("60 초 안에 답이 오지 않았다")

	return nil
}

// `Greeter {` 에서 물으면 그 struct 를 선언한 자리로 간다. 한글이 든 줄이다.
func TestRustAnalyzerDefinition(t *testing.T) {
	client, path := startRustForTest(t)

	lines := readLines(t, path)
	require.NoError(t, client.Open(path, lines))

	use := findLine(t, lines, "let greeter = Greeter {")
	column := UTF16Column(lines[use], indexIn(lines[use], "Greeter {"))

	locations := waitRust(t, func() ([]Location, error) {
		return client.Definition(path, Position{Line: use, Character: column})
	})
	require.Len(t, locations, 1)

	assert.Equal(t, path, locations[0].Path())
	assert.Equal(t, 0, locations[0].Range.Start.Line, "`struct Greeter` 줄이다")
}

// 빈 capabilities 로도 후보가 오고, 이미 친 접두를 TextEdit 범위가 덮는다(ADR-0066).
func TestRustAnalyzerCompletion(t *testing.T) {
	client, path := startRustForTest(t)

	lines := readLines(t, path)
	require.NoError(t, client.Open(path, lines))

	// `main` 의 몸 끝에 접두를 친 줄을 넣는다.
	probe := append([][]byte{}, lines[:len(lines)-1]...)
	probe = append(probe, []byte("    greeter.gr"), []byte("}"))
	require.NoError(t, client.SyncFull(path, probe))

	at := len(probe) - 2
	items := waitRust(t, func() ([]CompletionItem, error) {
		return client.Completion(path, Position{Line: at, Character: utf16Len(probe[at])})
	})

	labels := []string{}
	var greet *CompletionItem
	for i := range items {
		labels = append(labels, items[i].Label)
		if items[i].Text() == "greet()" || items[i].Text() == "greet" {
			greet = &items[i]
		}
	}
	t.Logf("후보 %d 개: %v", len(items), labels)

	require.NotNil(t, greet, "`greet` 가 후보에 있다")
	t.Logf("greet: label=%q detail=%q text=%q", greet.Label, greet.Detail, greet.Text())

	require.NotNil(t, greet.TextEdit, "무엇을 지우고 넣을지 서버가 준다")
	assert.Equal(t, len("    greeter."), greet.TextEdit.Range.Start.Character,
		"이미 친 접두 `gr` 을 범위가 덮는다")
	assert.Equal(t, len("    greeter.gr"), greet.TextEdit.Range.End.Character)
}

// 문법 토큰을 빈 capabilities 로 주는지 잰다.
func TestRustAnalyzerSemanticTokens(t *testing.T) {
	client, path := startRustForTest(t)

	t.Logf("이름표: %v", client.semantic.types)

	lines := readLines(t, path)
	require.NoError(t, client.Open(path, lines))

	tokens := waitRust(t, func() ([]SemanticToken, error) {
		return client.SemanticTokens(path, 0, len(lines))
	})

	got := map[string]string{}
	for _, token := range tokens {
		start := ByteColumn(lines[token.Line], token.Start)
		end := ByteColumn(lines[token.Line], token.Start+token.Length)
		got[string(lines[token.Line][start:end])] = token.Type
	}
	t.Logf("토큰: %v", got)
}

// 저장하지 않은 글을 맞춘다. **edition 을 우리가 넘기지 않아도** `async fn` 이 지난다 —
// rustfmt 를 stdin 으로 돌리면 2015 로 읽어 여기서 실패한다(ADR-0160).
func TestRustAnalyzerFormat(t *testing.T) {
	client, path := startRustForTest(t)

	require.NoError(t, client.Open(path, readLines(t, path)))

	in := []byte("async fn 인사(){ let x=1; }\nfn main(){}\n")
	want := "async fn 인사() {\n    let x = 1;\n}\nfn main() {}\n"

	// crate 를 읽는 동안은 `null` 이라 그대로 온다. 맞춘 답이 올 때까지 되묻는다.
	started := time.Now()
	for time.Since(started) < 60*time.Second {
		out, err := client.Format(path, in, 2*time.Second)
		require.NoError(t, err)

		if string(out) != string(in) {
			t.Logf("맞춘 답이 오기까지 %s", time.Since(started))
			assert.Equal(t, want, string(out))

			return
		}

		time.Sleep(200 * time.Millisecond)
	}

	t.Fatal("60 초 안에 맞춘 답이 오지 않았다")
}

// 문법이 깨진 글은 **오류가 아니라 그대로** 온다. 재보니 rust-analyzer 가 `null` 로 답한다.
// 그래서 저장 문구에 아무것도 붙지 않는다 — goimports·ruff 는 이때 까닭을 알린다(ADR-0160).
func TestRustAnalyzerFormatBrokenSource(t *testing.T) {
	client, path := startRustForTest(t)

	require.NoError(t, client.Open(path, readLines(t, path)))

	// 먼저 맞춘 답이 오는 것을 보고, 그 다음에 깨진 글을 넘긴다. 읽는 중의 `null` 과 섞이지 않게다.
	waitRust(t, func() ([]Location, error) {
		lines := readLines(t, path)
		use := findLine(t, lines, "let greeter = Greeter {")

		return client.Definition(path, Position{Line: use, Character: UTF16Column(lines[use], indexIn(lines[use], "Greeter {"))})
	})

	in := []byte("fn main( {\n")
	out, err := client.Format(path, in, 2*time.Second)
	require.NoError(t, err)
	assert.Equal(t, string(in), string(out))
}
