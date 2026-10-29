package syntax

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRustLexLine(t *testing.T) {
	tests := []struct {
		name string
		line string
		want []string
	}{
		{
			name: "fn 뒤는 부르는 이름이다",
			line: "pub fn greet(name: &str) -> String {",
			want: []string{"keyword:pub", "keyword:fn", "func:greet", "type:str", "type:String"},
		},
		{
			name: "CamelCase 는 type 이다",
			line: "let map: HashMap<String, Vec<u8>> = HashMap::new();",
			want: []string{"keyword:let", "type:HashMap", "type:String", "type:Vec", "type:u8", "type:HashMap", "func:new"},
		},
		{
			name: "대문자로만 지은 이름은 상수다",
			line: "const MAX_LEN: usize = 10;",
			want: []string{"keyword:const", "const:MAX_LEN", "type:usize", "number:10"},
		},
		{
			name: "generic 인자 하나짜리는 type 이다",
			line: "fn id<T>(x: T) -> T { x }",
			want: []string{"keyword:fn", "func:id", "type:T", "type:T", "type:T"},
		},
		{
			name: "prelude 의 variant 는 값이다",
			line: "match x { Some(v) => Ok(v), None => Err(e) }",
			want: []string{"keyword:match", "const:Some", "const:Ok", "const:None", "const:Err"},
		},
		{
			name: "tuple 꼴 variant 는 부르는 것이 아니다",
			line: "Shape::Circle(1.5)",
			want: []string{"type:Shape", "type:Circle", "number:1.5"},
		},
		{
			name: "macro 는 `!` 까지다",
			line: `println!("{}", x != y);`,
			want: []string{"func:println!", `string:"{}"`},
		},
		{
			name: "attribute 는 이름까지다",
			line: "#[derive(Debug, Clone)]",
			want: []string{"func:#[derive", "type:Debug", "type:Clone"},
		},
		{
			name: "안쪽 attribute 와 경로",
			line: "#![allow(dead_code)] #[serde::rename]",
			want: []string{"func:#![allow", "func:#[serde::rename"},
		},
		{
			name: "문자와 lifetime",
			line: "fn f<'a>(s: &'a str) -> char { 'x' }",
			want: []string{"keyword:fn", "func:f", "var:'a", "var:'a", "type:str", "type:char", "string:'x'"},
		},
		{
			name: "한글 문자 하나도 문자다",
			line: "let c = '한';",
			want: []string{"keyword:let", "string:'한'"},
		},
		{
			name: "escape 문자",
			line: `let q = '\''; let n = '\n'; let u = '\u{1F600}';`,
			want: []string{"keyword:let", `string:'\''`, "keyword:let", `string:'\n'`, "keyword:let", `string:'\u{1F600}'`},
		},
		{
			name: "byte 문자와 byte 문자열",
			line: `let b = b'a'; let s = b"bytes\n";`,
			want: []string{"keyword:let", "string:b'a'", "keyword:let", `string:b"bytes\n"`},
		},
		{
			name: "raw 문자열은 escape 가 없다",
			line: `let r = r"C:\path"; let h = r#"안의 "따옴표""#;`,
			want: []string{"keyword:let", `string:r"C:\path"`, "keyword:let", `string:r#"안의 "따옴표""#`},
		},
		{
			name: "raw 이름은 예약어가 아니다",
			line: "let r#type = 1;",
			want: []string{"keyword:let", "number:1"},
		},
		{
			name: "b·r 로 시작하는 낱말은 앞붙이가 아니다",
			line: "let bread = rust;",
			want: []string{"keyword:let"},
		},
		{
			name: "숫자 꼴",
			line: "let n = 0x1f_u8 + 1_000u32 + 2.5f64 + 1e-9 + 0b1010;",
			want: []string{"keyword:let", "number:0x1f_u8", "number:1_000u32", "number:2.5f64", "number:1e-9", "number:0b1010"},
		},
		{
			name: "범위의 `..` 는 숫자가 아니다",
			line: "for i in 0..10 {}",
			want: []string{"keyword:for", "keyword:in", "number:0", "number:10"},
		},
		{
			name: "숫자 뒤 메서드",
			line: "1.max(2)",
			want: []string{"number:1", "func:max", "number:2"},
		},
		{
			name: "self 와 Self",
			line: "fn new(&self) -> Self",
			want: []string{"keyword:fn", "func:new", "keyword:self", "type:Self"},
		},
		{
			name: "문서 주석",
			line: "/// 한글 문서",
			want: []string{"comment:/// 한글 문서"},
		},
		{
			name: "겹친 블록 주석이 한 줄에서 닫힌다",
			line: "/* 바깥 /* 안 */ 아직 */ let x = 1;",
			want: []string{"comment:/* 바깥 /* 안 */ 아직 */", "keyword:let", "number:1"},
		},
		{
			name: "true 와 false",
			line: "let ok = true && !false;",
			want: []string{"keyword:let", "const:true", "const:false"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, next := lexed(t, rustNormal{}, test.line)

			assert.Equal(t, test.want, got)
			assert.Equal(t, rustNormal{}, next, "한 줄에서 끝나면 문맥이 그대로다")
		})
	}
}

// 줄을 넘는 것은 블록 주석·문자열·raw 문자열이다.
func TestRustMultiline(t *testing.T) {
	tests := []struct {
		name  string
		lines []string
		want  [][]string
		end   State
	}{
		{
			name:  "겹친 블록 주석은 바깥 `*/` 에서 닫힌다",
			lines: []string{"/* 바깥 /* 안 */", "아직 주석 \"따옴표\"", "*/ let x = 1;"},
			want: [][]string{
				{"comment:/* 바깥 /* 안 */"},
				{`comment:아직 주석 "따옴표"`},
				{"comment:*/", "keyword:let", "number:1"},
			},
			end: rustNormal{},
		},
		{
			name:  "줄 끝에서 겹이 남는다",
			lines: []string{"/* /*"},
			want:  [][]string{{"comment:/* /*"}},
			end:   rustBlockComment{depth: 2},
		},
		{
			name:  "보통 문자열이 줄을 넘는다",
			lines: []string{`let s = "첫 줄`, `// 주석이 아니다`, `끝"; let y = 2;`},
			want: [][]string{
				{"keyword:let", `string:"첫 줄`},
				{"string:// 주석이 아니다"},
				{`string:끝"`, "keyword:let", "number:2"},
			},
			end: rustNormal{},
		},
		{
			name:  "줄 끝의 escape 된 따옴표는 닫지 않는다",
			lines: []string{`let s = "a\"`, `b";`},
			want: [][]string{
				{"keyword:let", `string:"a\"`},
				{`string:b"`},
			},
			end: rustNormal{},
		},
		{
			name:  "raw 문자열은 같은 수의 `#` 에서만 닫힌다",
			lines: []string{`let r = r##"열었다`, `"# 아직이다`, `끝"## ;`},
			want: [][]string{
				{"keyword:let", `string:r##"열었다`},
				{`string:"# 아직이다`},
				{`string:끝"##`},
			},
			end: rustNormal{},
		},
		{
			name:  "byte 문자열도 줄을 넘는다",
			lines: []string{`let b = b"열었다`},
			want:  [][]string{{"keyword:let", `string:b"열었다`}},
			end:   rustString{},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, next := lexedAll(t, rustNormal{}, test.lines...)

			assert.Equal(t, test.want, got)
			assert.Equal(t, test.end, next)
		})
	}
}

// 들여쓰기는 세 괄호만 센다. `default` 로 시작하는 필드를 이름표로 보지 않는다.
func TestRustIndent(t *testing.T) {
	rule := rustIndent{}

	depth, _ := rule.Next([]byte("fn main() {"), nil)
	assert.Equal(t, 1, depth)

	depth, _ = rule.Next([]byte("let v = vec!["), nil)
	assert.Equal(t, 1, depth)

	depth, _ = rule.Next([]byte("let x = 1;"), nil)
	assert.Equal(t, 0, depth)

	assert.Equal(t, 1, rule.Close([]byte("}")))
	assert.Equal(t, 1, rule.Close([]byte("]);")))
	assert.Equal(t, 0, rule.Close([]byte("default: bool,")), "switch 이름표가 아니다")
	assert.Equal(t, []byte("    "), rule.Unit())
}
