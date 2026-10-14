package syntax

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNginxLexLine(t *testing.T) {
	tests := []struct {
		name string
		line string
		want []string
	}{
		{
			name: "directive 와 수",
			line: "    listen 80;",
			want: []string{"keyword:listen", "number:80"},
		},
		{
			name: "단위가 붙은 수",
			line: "client_max_body_size 10m; keepalive_timeout 1.5s;",
			want: []string{"keyword:client_max_body_size", "number:10m", "keyword:keepalive_timeout", "number:1.5s"},
		},
		{
			name: "주소 가운데의 수는 수가 아니다",
			line: "listen 127.0.0.1:8080;",
			want: []string{"keyword:listen"},
		},
		{
			name: "블록을 여는 directive",
			line: "server {",
			want: []string{"keyword:server"},
		},
		{
			name: "`;` 뒤가 다음 directive 다",
			line: "gzip on; gzip_vary off;",
			want: []string{"keyword:gzip", "const:on", "keyword:gzip_vary", "const:off"},
		},
		{
			name: "변수",
			line: "return 301 https://$host${request_uri};",
			want: []string{"keyword:return", "number:301", "var:$host", "var:${request_uri}"},
		},
		{
			name: "따옴표 안의 변수는 떼어 칠한다",
			line: `proxy_set_header X-Real-IP "$remote_addr:x";`,
			want: []string{"keyword:proxy_set_header", `string:"`, "var:$remote_addr", `string::x"`},
		},
		{
			name: "주석",
			line: "worker_processes auto; # 코어 수만큼",
			want: []string{"keyword:worker_processes", "comment:# 코어 수만큼"},
		},
		{
			name: "낱말 가운데의 `#` 은 주석이 아니다",
			line: "root /srv/a#b;",
			want: []string{"keyword:root"},
		},
		{
			name: "location 의 정규식",
			line: `location ~* \.(png|jpg)$ {`,
			want: []string{"keyword:location", "string:~*", `string:\.(png|jpg)$`},
		},
		{
			name: "붙여 적은 비교 표시",
			line: "location =/50x.html {",
			want: []string{"keyword:location", "string:=/50x.html"},
		},
		{
			name: "비교 표시 없는 location 은 경로다",
			line: "location /api/$x {",
			want: []string{"keyword:location", "var:$x"},
		},
		{
			name: "표에 없는 이름은 색이 없다",
			line: "text/html html htm;",
			want: []string{},
		},
		{
			name: "`map` 블록의 `default` 는 directive 가 아니다",
			line: "    default 0;",
			want: []string{"number:0"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, _ := lexed(t, nginxNormal{}, test.line)

			assert.Equal(t, test.want, got)
		})
	}
}

// 인자를 줄로 나누면 다음 줄 앞은 directive 자리가 아니다. `;` 를 만나야 다시 directive 다.
func TestNginxArgumentsAcrossLines(t *testing.T) {
	got, next := lexedAll(t, nginxNormal{},
		"server_name example.com",
		"    root.example.com;",
		"root /srv;",
	)

	assert.Equal(t, [][]string{
		{"keyword:server_name"},
		{},
		{"keyword:root"},
	}, got)
	assert.Equal(t, nginxNormal{}, next)
}

// 따옴표는 줄을 넘는다. 닫힌 뒤는 같은 문장의 인자다.
func TestNginxQuoteAcrossLines(t *testing.T) {
	got, next := lexedAll(t, nginxNormal{},
		"return 200 'first",
		"$uri second' ;",
		"root /srv;",
	)

	assert.Equal(t, [][]string{
		{"keyword:return", "number:200", "string:'first"},
		{"var:$uri", "string: second'"},
		{"keyword:root"},
	}, got)
	assert.Equal(t, nginxNormal{}, next)
}

func TestDetectNginx(t *testing.T) {
	tests := []struct {
		name string
		path string
		want State
	}{
		{name: "nginx.conf", path: "/etc/nginx/nginx.conf", want: nginxNormal{}},
		{name: "어디 있든 nginx.conf", path: "deploy/nginx.conf", want: nginxNormal{}},
		{name: "확장자 .nginx", path: "deploy/site.nginx", want: nginxNormal{}},
		{name: "nginx 디렉터리 아래의 .conf", path: "/etc/nginx/conf.d/default.conf", want: nginxNormal{}},
		{name: "sites-enabled 안은 확장자가 없어도", path: "/etc/nginx/sites-enabled/default", want: nginxNormal{}},
		{name: "nginx 밖의 .conf", path: "/etc/supervisor/conf.d/app.conf", want: nil},
		{name: "nginx 가 이름에 들었을 뿐", path: "/etc/nginx-proxy/app.conf", want: nil},
		{name: "nginx 디렉터리 아래라도 .conf 가 아니면", path: "/etc/nginx/mime.types", want: nil},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assert.Equal(t, test.want, LanguageFor(test.path).State())
		})
	}
}

// 상대 경로는 지금 디렉터리를 붙여 본다. `/etc/nginx` 에서 연 `conf.d/default.conf` 다.
func TestDetectNginxRelative(t *testing.T) {
	t.Chdir(t.TempDir())
	assert.Nil(t, LanguageFor("conf.d/default.conf").State())

	dir := filepath.Join(t.TempDir(), "nginx")
	require.NoError(t, os.Mkdir(dir, 0o755))
	t.Chdir(dir)
	assert.Equal(t, nginxNormal{}, LanguageFor("conf.d/default.conf").State())
}

// `map` 블록 안의 `default` 가 나오지 않는다. braceIndent 를 쓰지 않은 까닭이다.
func TestNginxIndent(t *testing.T) {
	rule := nginxIndent{}

	level, _ := rule.Next([]byte("map $uri $x {"), nil)
	assert.Equal(t, 1, level)

	level, _ = rule.Next([]byte(`location ~ "^/(a" {`), []Token{{Start: 11, End: 16, Kind: KindString}})
	assert.Equal(t, 1, level, "따옴표 안의 괄호는 세지 않는다")

	assert.Equal(t, 0, rule.Close([]byte("default 0;")))
	assert.Equal(t, 1, rule.Close([]byte("}")))
}
