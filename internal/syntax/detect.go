package syntax

import (
	"path/filepath"
	"slices"
	"strings"
)

// languageRule 은 파일 이름에서 언어를 알아내는 규칙 하나다.
//
// 확장자와 이름으로 갈라 둔 것은 Makefile·Dockerfile 처럼 확장자가 없는 이름이 있어서다.
// 둘 다 소문자로 적는다 — 보는 쪽에서 이름을 소문자로 내려 견준다.
type languageRule struct {
	exts  []string
	names []string

	// aliases 는 사람이 손으로 적는 언어 **이름**이다. 코드펜스의 info string 이 그것이라
	// (```go) 확장자로는 찾을 수 없다.
	//
	// 칸을 따로 둔 이유는 「언어 하나가 표의 한 줄」을 지키는 것이다 — 이름 표를 따로 만들면
	// 언어를 더할 때 손댈 자리가 둘이 된다(ADR-0040).
	aliases []string

	state State

	// indent 는 그 언어의 들여쓰기 규칙이다. state 와 나란한 두 번째 언어별 값이라 같은 줄에
	// 둔다 — 언어를 더할 때 손대는 자리가 하나로 남는다.
	indent Indent

	// outline 은 그 언어의 뼈대 규칙이다. state·indent 와 나란한 세 번째 언어별 값이다.
	// 화면 위에 붙는 머리줄이 이것으로 정해진다(ADR-0049).
	outline Outline
}

// tabBraceIndent 와 spaceBraceIndent 는 indent 칸과 outline 칸이 **같은 값**을 보게 하는
// 이름이다. 「다음 줄이 들어간다」와 「아래를 거느린다」가 같은 판정이라, 표에 두 번 적으면
// 한쪽만 고쳐지는 날이 온다.
var (
	tabBraceIndent   = braceIndent{unit: "\t"}
	spaceBraceIndent = braceIndent{unit: "  "}
)

// languageRules 는 강조하는 언어 전부다. 새 언어는 여기 한 줄이 는다.
//
// 표 하나에 규칙을 늘어놓는 것은 팔레트 명령(core/palette.go 의 paletteCommands) 과 같은
// 손이다. 언어 하나가 한 줄이라 「이 언어는 이렇게 알아본다」로 읽힌다.
var languageRules = []languageRule{
	{exts: []string{".go"}, aliases: []string{"go", "golang"}, state: goNormal{},
		indent: tabBraceIndent, outline: blockOutline{opens: tabBraceIndent}},
	{exts: []string{".md", ".markdown"}, aliases: []string{"md", "markdown"}, state: mdNormal{},
		indent: mdIndent{}, outline: mdOutline{}},
	{exts: []string{".html", ".htm"}, aliases: []string{"html"}, state: htmlNormal{},
		indent: htmlIndent{}, outline: blockOutline{opens: htmlIndent{}}},
	{exts: []string{".js", ".mjs", ".cjs"}, aliases: []string{"js", "javascript"}, state: jsNormal{},
		indent: spaceBraceIndent, outline: blockOutline{opens: spaceBraceIndent}},
	{exts: []string{".css"}, aliases: []string{"css"}, state: cssNormal{},
		indent: spaceBraceIndent, outline: blockOutline{opens: spaceBraceIndent}},
	{exts: []string{".py"}, aliases: []string{"py", "python"}, state: pyNormal{},
		indent: pyIndent{}, outline: blockOutline{opens: pyIndent{}}},
	{exts: []string{".sh", ".bash", ".zsh"}, aliases: []string{"sh", "bash", "shell", "zsh"},
		state: shNormal{}, indent: shIndent{}, outline: blockOutline{opens: shIndent{}}},
	{exts: []string{".mk"}, names: []string{"makefile", "gnumakefile"},
		aliases: []string{"make", "makefile"}, state: makeNormal{}, indent: makeIndent{},
		outline: makeOutline{}},
	{exts: []string{".dockerfile"}, names: []string{"dockerfile"},
		aliases: []string{"docker", "dockerfile"}, state: dockerNormal{}, indent: dockerIndent{},
		outline: flatOutline{}},
	{exts: []string{".json", ".jsonc", ".json5", ".hjson"},
		aliases: []string{"json", "jsonc", "json5", "hjson"}, state: jsonNormal{},
		indent: jsonIndent{}, outline: blockOutline{opens: jsonIndent{}}},
	{exts: []string{".yaml", ".yml"}, aliases: []string{"yaml", "yml"}, state: yamlNormal{},
		indent: yamlIndent{}, outline: blockOutline{opens: yamlIndent{}}},
}

// Detect 는 경로를 보고 시작 문맥을 고른다. nil 이면 강조하지 않는다.
//
// **이름만 본다 — 내용을 들여다보지 않는다.** shebang(`#!/bin/sh`) 을 보려면 첫 줄을 읽어야
// 하고, 그러면 이 함수가 파일 이름이 아니라 내용에 매인다. 이름 하나로 정해지는 것이라
// 파일이 없어도(새로 만드는 buffer) 같은 답이 나온다.
//
// core/sidebar.go 의 트리 색도 확장자를 보지만 합치지 않았다. 묶음이 더 굵고 일부러 다르다 —
// `.html`·`.css`·`.js` 가 한 색이고 `.txt` 도 색이 있는데 여기서는 lexer 가 없다. 합치면
// lexer 를 더할 때마다 트리 색이 조용히 바뀐다.
func Detect(path string) State {
	rule := ruleFor(path)
	if rule == nil {
		return nil
	}

	return rule.state
}

// IndentFor 는 경로를 보고 들여쓰기 규칙을 고른다. nil 이면 앞 줄의 들여쓰기를 그대로 잇는다.
// Detect 와 짝이다 — 같은 표의 다른 칸을 본다.
func IndentFor(path string) Indent {
	rule := ruleFor(path)
	if rule == nil {
		return nil
	}

	return rule.indent
}

// OutlineFor 는 경로를 보고 뼈대 규칙을 고른다. nil 이면 머리줄을 붙이지 않는다.
// Detect·IndentFor 와 짝이다 — 같은 표의 또 다른 칸을 본다.
func OutlineFor(path string) Outline {
	rule := ruleFor(path)
	if rule == nil {
		return nil
	}

	return rule.outline
}

// ruleFor 는 이름에 맞는 표의 한 줄이다. Detect·IndentFor·OutlineFor 가 나눠 쓴다 — 이름을
// 보는 법이 갈리면 한쪽만 아는 언어가 생긴다.
func ruleFor(path string) *languageRule {
	name := strings.ToLower(filepath.Base(path))

	for i, rule := range languageRules {
		if slices.Contains(rule.names, name) || slices.Contains(rule.exts, filepath.Ext(name)) {
			return &languageRules[i]
		}
	}

	// `Dockerfile.dev` 처럼 이름 뒤에 무엇이 붙는 것. 규칙 표에 접두 칸을 두면 언어 하나
	// 때문에 표 전체가 그 칸을 들고 다닌다.
	if strings.HasPrefix(name, "dockerfile.") {
		return ruleByAlias("dockerfile")
	}

	return nil
}

// ruleByAlias 는 사람이 적는 언어 이름으로 표의 한 줄을 찾는다. 모르는 이름이면 nil 이다.
func ruleByAlias(name string) *languageRule {
	name = strings.ToLower(name)
	if name == "" {
		return nil
	}

	for i, rule := range languageRules {
		if slices.Contains(rule.aliases, name) {
			return &languageRules[i]
		}
	}

	return nil
}

// languageByName 은 언어 이름으로 시작 문맥을 고른다. nil 이면 그 안을 훑지 않는다.
//
// **경로가 아니라 이름이다.** Detect 를 쓸 수 없다 — `go` 는 확장자도 파일 이름도 아니다.
//
// 모르는 이름에 nil 을 주는 것이 이 함수의 요점이다. 위임하는 쪽이 nil 을 「색 없음」으로
// 받아서, 모르는 언어의 코드펜스가 위임을 넣기 전과 한 글자도 다르지 않게 그려진다.
//
// aliases 만 본다. 파일 이름(names) 까지 견주면 ```gnumakefile 이 우연히 되면서 두 칸의 뜻이
// 흐려진다.
func languageByName(name string) State {
	rule := ruleByAlias(name)
	if rule == nil {
		return nil
	}

	return rule.state
}
