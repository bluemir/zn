package syntax

import (
	"bytes"
	"path/filepath"
	"slices"
	"strings"
)

// nginxNormal 은 nginx 설정의 보통 문맥이다. 다음 낱말이 directive 이름이다.
type nginxNormal struct{}

// nginxArgs 는 앞 줄의 문장이 `;` 나 `{` 없이 끝난 자리다. 줄 앞이 directive 가 아니라
// 그 문장의 인자다.
//
// `server_name a.com` 다음 줄에 `b.com;` 을 적는 것처럼 인자를 줄로 나누는 일이 흔하다.
// 줄 앞을 늘 directive 로 보면 그 줄의 `b.com` 이 이름 자리로 읽힌다.
type nginxArgs struct{}

// nginxQuoted 는 따옴표 안이다. 여는 따옴표 글자를 든다.
//
// nginx 의 따옴표는 **줄을 넘는다.** `return 200 '...'` 로 긴 글을 적는 자리가 그렇다.
type nginxQuoted struct {
	quote byte
}

func (nginxNormal) Indent() Indent { return nginxIndent{} }
func (nginxArgs) Indent() Indent   { return nginxIndent{} }
func (nginxQuoted) Indent() Indent { return nginxIndent{} }

func (s nginxNormal) Lex(line []byte) ([]Token, State) {
	return nginxLex(line, s)
}

func (s nginxArgs) Lex(line []byte) ([]Token, State) {
	return nginxLex(line, s)
}

func (s nginxQuoted) Lex(line []byte) ([]Token, State) {
	end, closed := nginxQuoteEnd(line, 0, s.quote)
	tokens := nginxStringTokens(line, 0, end)
	if !closed {
		return tokens, s
	}

	tail, next := lexTail(nginxArgs{}, line, end)

	return append(tokens, tail...), next
}

// nginxLex 는 줄 하나를 훑는다. state 는 줄을 시작하는 문맥이다. nginxNormal 이면 첫 낱말이
// directive 이름이다.
//
// 낱말은 공백과 `;` `{` `}` 로만 끊는다. nginx 가 그렇게 읽는다. `#` 과 따옴표도 낱말 앞에서만
// 뜻이 있고 낱말 가운데서는 그냥 글자다(`a#b`).
func nginxLex(line []byte, state State) ([]Token, State) {
	tokens := []Token{}
	_, atDirective := state.(nginxNormal)

	// location 은 첫 인자가 비교 표시(`~` `~*` `=` `^~`) 면 그 뒤가 정규식이나 그대로 견줄 주소다.
	// 경로 낱말처럼 `$` 를 변수로 읽으면 `\.php$` 의 끝 표시가 변수 자리가 된다.
	locationArg := 0

	for at := 0; at < len(line); {
		switch line[at] {
		case ' ', '\t', '\r':
			at++

			continue
		case '#':
			return append(tokens, Token{Start: at, End: len(line), Kind: KindComment}),
				nginxStateAfter(atDirective)
		case ';', '{', '}':
			atDirective = true
			locationArg = 0
			at++

			continue
		case '"', '\'':
			end, closed := nginxQuoteEnd(line, at+1, line[at])
			tokens = append(tokens, nginxStringTokens(line, at, end)...)
			if !closed {
				return tokens, nginxQuoted{quote: line[at]}
			}

			atDirective = false
			locationArg = 0
			at = end

			continue
		}

		end := nginxWordEnd(line, at)
		word := line[at:end]

		switch {
		case atDirective:
			if nginxDirectives[string(word)] {
				tokens = append(tokens, Token{Start: at, End: end, Kind: KindKeyword})
			}
			if string(word) == "location" {
				locationArg = 1
			}
			atDirective = false
		case locationArg == 1 && nginxLocationModifier(word):
			tokens = append(tokens, Token{Start: at, End: end, Kind: KindString})
			// 표시가 따로 선 것(`~ \.php$`) 이면 다음 낱말이 그 패턴이다. 붙여 적은 것(`=/x`) 은
			// 이 낱말이 패턴까지 든다.
			if slices.Contains(nginxLocationModifiers, string(word)) {
				locationArg = 2
			} else {
				locationArg = 0
			}
		case locationArg == 2:
			tokens = append(tokens, Token{Start: at, End: end, Kind: KindString})
			locationArg = 0
		default:
			tokens = append(tokens, nginxArgTokens(line, at, end)...)
			locationArg = 0
		}

		at = end
	}

	return tokens, nginxStateAfter(atDirective)
}

// nginxStateAfter 는 줄이 끝났을 때 다음 줄의 문맥이다. 문장이 끝나지 않았으면 다음 줄이 그
// 문장의 인자다.
func nginxStateAfter(atDirective bool) State {
	if atDirective {
		return nginxNormal{}
	}

	return nginxArgs{}
}

// nginxWordEnd 는 그 자리에서 시작하는 낱말이 끝나는 자리다.
//
// `${name}` 의 중괄호는 낱말을 끊지 않는다. nginx 도 `$` 바로 뒤의 `{` 는 블록으로 읽지 않는다.
func nginxWordEnd(line []byte, at int) int {
	for at < len(line) && !strings.ContainsRune(" \t\r;{}", rune(line[at])) {
		if end, ok := nginxVariableEnd(line, at, len(line)); ok {
			at = end

			continue
		}

		at++
	}

	return at
}

// nginxLocationModifiers 는 location 의 비교 표시다. `=` 은 그대로 견주고, `^~` 는 앞부분으로
// 견주되 정규식을 건너뛰고, `~` `~*` 는 정규식이다(뒤의 것이 대소문자를 가리지 않는다).
var nginxLocationModifiers = []string{"=", "~", "~*", "^~"}

// nginxLocationModifier 는 낱말이 비교 표시로 시작하는지다. 패턴을 붙여 적어도(`~\.php$`) 된다.
func nginxLocationModifier(word []byte) bool {
	return len(word) > 0 && (word[0] == '=' || word[0] == '~' || bytes.HasPrefix(word, []byte("^~")))
}

// nginxQuoteEnd 는 from 부터 찾은 닫는 따옴표의 다음 자리다. 닫히지 않으면 줄 끝을 준다.
func nginxQuoteEnd(line []byte, from int, quote byte) (int, bool) {
	for at := from; at < len(line); at++ {
		switch line[at] {
		case '\\':
			at++
		case quote:
			return at + 1, true
		}
	}

	return len(line), false
}

// nginxStringTokens 는 따옴표 안을 문자열로 칠하되 변수 자리는 따로 뗀다.
//
// nginx 는 따옴표 안의 `$var` 도 값으로 바꾼다. `log_format` 은 거의 전부가 따옴표 안의 변수라,
// 통째로 문자열 색이면 어느 값을 남기는지가 화면에 보이지 않는다.
func nginxStringTokens(line []byte, from, end int) []Token {
	tokens := []Token{}

	start := from
	for at := from; at < end; {
		if line[at] == '\\' {
			at += 2

			continue
		}

		varEnd, ok := nginxVariableEnd(line, at, end)
		if !ok {
			at++

			continue
		}

		if at > start {
			tokens = append(tokens, Token{Start: start, End: at, Kind: KindString})
		}
		tokens = append(tokens, Token{Start: at, End: varEnd, Kind: KindVariable})
		at = varEnd
		start = varEnd
	}

	if end > start {
		tokens = append(tokens, Token{Start: start, End: end, Kind: KindString})
	}

	return tokens
}

// nginxArgTokens 는 따옴표 없는 인자 하나의 토큰이다.
//
// **값 전체가 그것일 때만** 숫자와 `on`·`off` 로 본다. yaml 의 따옴표 없는 값과 같은 규칙이다
// (ADR-0055). `127.0.0.1:8080` 의 `127` 은 숫자가 아니다. 나머지는 변수 자리만 집는다.
func nginxArgTokens(line []byte, from, end int) []Token {
	word := line[from:end]

	switch {
	case nginxNumber(word):
		return []Token{{Start: from, End: end, Kind: KindNumber}}
	case string(word) == "on", string(word) == "off":
		return []Token{{Start: from, End: end, Kind: KindConstant}}
	}

	tokens := []Token{}
	for at := from; at < end; {
		if varEnd, ok := nginxVariableEnd(line, at, end); ok {
			tokens = append(tokens, Token{Start: at, End: varEnd, Kind: KindVariable})
			at = varEnd

			continue
		}

		at++
	}

	return tokens
}

// nginxVariableEnd 는 그 자리에서 시작하는 `$name`·`${name}` 이 끝나는 자리다.
//
// shellExpansionEnd 를 쓰지 않았다. nginx 에는 `$(...)` 가 없고 `$` 뒤에 이름이 없으면 그냥
// 글자다. 정규식 끝 표시(`^/a$`) 가 그것이다.
func nginxVariableEnd(line []byte, at, limit int) (int, bool) {
	if line[at] != '$' || at+1 >= limit {
		return 0, false
	}

	if line[at+1] == '{' {
		end := at + 2
		for end < limit && shellNameByte(line[end]) {
			end++
		}
		if end > at+2 && end < limit && line[end] == '}' {
			return end + 1, true
		}

		return 0, false
	}

	end := at + 1
	for end < limit && shellNameByte(line[end]) {
		end++
	}

	return end, end > at+1
}

// nginxNumber 는 낱말 전체가 수인지다. 크기(`10m` `1k`) 와 시간(`30s` `1h` `500ms`) 의 단위를
// 붙여도 된다.
func nginxNumber(word []byte) bool {
	digits := 0
	for digits < len(word) && word[digits] >= '0' && word[digits] <= '9' {
		digits++
	}
	if digits == 0 {
		return false
	}

	rest := string(word[digits:])
	if strings.HasPrefix(rest, ".") {
		fraction := 1
		for fraction < len(rest) && rest[fraction] >= '0' && rest[fraction] <= '9' {
			fraction++
		}
		if fraction == 1 {
			return false
		}
		rest = rest[fraction:]
	}

	return slices.Contains(nginxUnits, rest)
}

// nginxUnits 는 수 뒤에 붙는 단위다. 빈 것은 단위 없이 끝난 것이다.
var nginxUnits = []string{
	"", "k", "K", "m", "M", "g", "G", "ms", "s", "h", "d", "w", "y",
}

// nginxPath 는 경로가 nginx 설정인지다. 확장자 `.conf` 를 nginx 말고도 여러 형식이 같이 써서
// 이름만으로는 가르지 못한다.
//
//   - `nginx` 디렉터리 아래의 `.conf` 다. `/etc/nginx/conf.d/default.conf` 가 그것이다
//   - `sites-available`·`sites-enabled` 안의 파일은 이름이 무엇이든이다. debian 계열이 그 안에
//     확장자 없이 `default` 를 둔다
//
// **상대 경로는 지금 디렉터리를 붙여 본다.** `/etc/nginx` 에서 `zn conf.d/default.conf` 로 열면
// 경로에 `nginx` 가 없다.
func nginxPath(path string) bool {
	if path == "" {
		return false
	}

	if absolute, err := filepath.Abs(path); err == nil {
		path = absolute
	}

	dirs := strings.Split(filepath.ToSlash(filepath.Dir(path)), "/")
	parent := dirs[len(dirs)-1]
	if parent == "sites-available" || parent == "sites-enabled" {
		return true
	}

	return strings.EqualFold(filepath.Ext(path), ".conf") && slices.Contains(dirs, "nginx")
}

// nginxIndent 는 nginx 설정의 들여쓰기 규칙이다. 중괄호만 센다.
//
// braceIndent 를 나눠 쓰지 않았다. 그것은 `default` 를 switch 이름표로 보는데 nginx 의 `map`
// 블록에서 `default` 는 이름표가 아니라 그냥 줄 하나라, 그 줄만 한 단계 나온다. 둥근 괄호와
// 대괄호도 세지 않는다. 따옴표 없는 정규식(`rewrite ^/(a|b`) 안의 괄호가 짝이 맞는다는 보장이
// 없다.
type nginxIndent struct{}

func (nginxIndent) Next(line []byte, tokens []Token) (int, []byte) {
	if bracketOpens(codeBytes(line, tokens), "{", "}") {
		return 1, nil
	}

	return 0, nil
}

func (nginxIndent) Close(head []byte) int {
	if closesBracket(head, "}") {
		return 1
	}

	return 0
}

func (nginxIndent) Reindents() bool { return true }

func (nginxIndent) TabIndentsLine([]byte) (int, bool) { return 0, false }

// Unit 은 space 네 칸이다. nginx 가 내놓는 기본 설정 파일이 그렇게 쓴다.
func (nginxIndent) Unit() []byte { return []byte("    ") }

// nginxDirectives 는 nginx.org 의 directive 색인(docs/dirindex.html) 에 든 이름 전부다.
// 2026-10-14 에 그 쪽에서 옮겨 적었다.
//
// **표에 없는 이름은 색이 없다.** 서드파티 모듈의 directive 와 오타가 그렇다. `types` 블록 안의
// MIME 이름과 `map` 블록 안의 키도 문장 앞에 서지만 directive 가 아니라서, 문장 앞 낱말을
// 다 칠하면 그것들까지 물든다.
var nginxDirectives = map[string]bool{
	"absolute_redirect": true, "accept_mutex": true, "accept_mutex_delay": true, "accept_terms_of_service": true,
	"access_log": true, "account_key": true, "acme_certificate": true, "acme_issuer": true,
	"acme_shared_zone": true, "add_after_body": true, "add_before_body": true, "add_header": true,
	"add_header_inherit": true, "add_trailer": true, "add_trailer_inherit": true, "addition_types": true,
	"aio": true, "aio_write": true, "alias": true, "allow": true, "ancient_browser": true,
	"ancient_browser_value": true, "api": true, "auth_basic": true, "auth_basic_user_file": true,
	"auth_delay": true, "auth_http": true, "auth_http_header": true, "auth_http_pass_client_cert": true,
	"auth_http_timeout": true, "auth_jwt": true, "auth_jwt_claim_set": true, "auth_jwt_header_set": true,
	"auth_jwt_key_cache": true, "auth_jwt_key_file": true, "auth_jwt_key_request": true,
	"auth_jwt_leeway": true, "auth_jwt_require": true, "auth_jwt_type": true, "auth_oidc": true,
	"auth_request": true, "auth_request_set": true, "auth_require": true, "autoindex": true,
	"autoindex_exact_size": true, "autoindex_format": true, "autoindex_localtime": true,
	"break": true, "challenge": true, "charset": true, "charset_map": true, "charset_types": true,
	"chunked_transfer_encoding": true, "client_body_buffer_size": true, "client_body_early_read": true,
	"client_body_in_file_only": true, "client_body_in_single_buffer": true, "client_body_temp_path": true,
	"client_body_timeout": true, "client_header_buffer_size": true, "client_header_timeout": true,
	"client_id": true, "client_max_body_size": true, "client_secret": true, "common_name_in_csr": true,
	"config_url": true, "connection_pool_size": true, "contact": true, "cookie_name": true,
	"create_full_put_path": true, "daemon": true, "dav_access": true, "dav_methods": true,
	"debug_connection": true, "debug_points": true, "default_type": true, "deny": true,
	"directio": true, "directio_alignment": true, "disable_symlinks": true, "early_hints": true,
	"empty_gif": true, "enforce_initial_report": true, "env": true, "error_log": true,
	"error_log_tag": true, "error_page": true, "etag": true, "events": true, "expires": true,
	"external_account_key": true, "extra_auth_args": true, "f4f": true, "f4f_buffer_size": true,
	"fastcgi_allow_upstream": true, "fastcgi_bind": true, "fastcgi_bind_dynamic": true,
	"fastcgi_buffer_size": true, "fastcgi_buffering": true, "fastcgi_buffers": true,
	"fastcgi_busy_buffers_size": true, "fastcgi_cache": true, "fastcgi_cache_background_update": true,
	"fastcgi_cache_bypass": true, "fastcgi_cache_key": true, "fastcgi_cache_lock": true,
	"fastcgi_cache_lock_age": true, "fastcgi_cache_lock_timeout": true, "fastcgi_cache_max_range_offset": true,
	"fastcgi_cache_methods": true, "fastcgi_cache_min_uses": true, "fastcgi_cache_path": true,
	"fastcgi_cache_purge": true, "fastcgi_cache_revalidate": true, "fastcgi_cache_use_stale": true,
	"fastcgi_cache_valid": true, "fastcgi_catch_stderr": true, "fastcgi_connect_timeout": true,
	"fastcgi_force_ranges": true, "fastcgi_hide_header": true, "fastcgi_ignore_client_abort": true,
	"fastcgi_ignore_headers": true, "fastcgi_index": true, "fastcgi_intercept_errors": true,
	"fastcgi_keep_conn": true, "fastcgi_limit_rate": true, "fastcgi_max_temp_file_size": true,
	"fastcgi_next_upstream": true, "fastcgi_next_upstream_timeout": true, "fastcgi_next_upstream_tries": true,
	"fastcgi_no_cache": true, "fastcgi_param": true, "fastcgi_pass": true, "fastcgi_pass_header": true,
	"fastcgi_pass_request_body": true, "fastcgi_pass_request_headers": true, "fastcgi_read_timeout": true,
	"fastcgi_request_buffering": true, "fastcgi_request_dynamic": true, "fastcgi_send_lowat": true,
	"fastcgi_send_timeout": true, "fastcgi_socket_keepalive": true, "fastcgi_socket_rcvbuf": true,
	"fastcgi_socket_sndbuf": true, "fastcgi_split_path_info": true, "fastcgi_store": true,
	"fastcgi_store_access": true, "fastcgi_temp_file_write_size": true, "fastcgi_temp_path": true,
	"flv": true, "frontchannel_logout_uri": true, "geo": true, "geoip_city": true, "geoip_country": true,
	"geoip_org": true, "geoip_proxy": true, "geoip_proxy_recursive": true, "google_perftools_profiles": true,
	"grpc_allow_upstream": true, "grpc_bind": true, "grpc_bind_dynamic": true, "grpc_buffer_size": true,
	"grpc_connect_timeout": true, "grpc_hide_header": true, "grpc_ignore_headers": true,
	"grpc_intercept_errors": true, "grpc_next_upstream": true, "grpc_next_upstream_timeout": true,
	"grpc_next_upstream_tries": true, "grpc_pass": true, "grpc_pass_header": true, "grpc_read_timeout": true,
	"grpc_request_dynamic": true, "grpc_send_timeout": true, "grpc_set_header": true,
	"grpc_socket_keepalive": true, "grpc_socket_rcvbuf": true, "grpc_socket_sndbuf": true,
	"grpc_ssl_certificate": true, "grpc_ssl_certificate_cache": true, "grpc_ssl_certificate_key": true,
	"grpc_ssl_ciphers": true, "grpc_ssl_conf_command": true, "grpc_ssl_crl": true, "grpc_ssl_key_log": true,
	"grpc_ssl_name": true, "grpc_ssl_password_file": true, "grpc_ssl_protocols": true,
	"grpc_ssl_server_name": true, "grpc_ssl_session_reuse": true, "grpc_ssl_trusted_certificate": true,
	"grpc_ssl_verify": true, "grpc_ssl_verify_depth": true, "gunzip": true, "gunzip_buffers": true,
	"gzip": true, "gzip_buffers": true, "gzip_comp_level": true, "gzip_disable": true,
	"gzip_http_version": true, "gzip_min_length": true, "gzip_proxied": true, "gzip_static": true,
	"gzip_types": true, "gzip_vary": true, "hash": true, "health_check": true, "health_check_timeout": true,
	"hls": true, "hls_buffers": true, "hls_forward_args": true, "hls_fragment": true,
	"hls_mp4_buffer_size": true, "hls_mp4_max_buffer_size": true, "http": true, "http2": true,
	"http2_body_preread_size": true, "http2_chunk_size": true, "http2_idle_timeout": true,
	"http2_max_concurrent_pushes": true, "http2_max_concurrent_streams": true, "http2_max_field_size": true,
	"http2_max_header_size": true, "http2_max_requests": true, "http2_push": true, "http2_push_preload": true,
	"http2_recv_buffer_size": true, "http2_recv_timeout": true, "http3": true, "http3_hq": true,
	"http3_max_concurrent_streams": true, "http3_stream_buffer_size": true, "if": true,
	"if_modified_since": true, "ignore_invalid_headers": true, "image_filter": true,
	"image_filter_buffer": true, "image_filter_interlace": true, "image_filter_jpeg_quality": true,
	"image_filter_sharpen": true, "image_filter_transparency": true, "image_filter_webp_quality": true,
	"imap_auth": true, "imap_capabilities": true, "imap_client_buffer": true, "include": true,
	"index": true, "internal": true, "internal_redirect": true, "ip_hash": true, "issuer": true,
	"js_access": true, "js_body_filter": true, "js_content": true, "js_context_reuse": true,
	"js_engine": true, "js_fetch_buffer_size": true, "js_fetch_ciphers": true, "js_fetch_keepalive": true,
	"js_fetch_keepalive_requests": true, "js_fetch_keepalive_time": true, "js_fetch_keepalive_timeout": true,
	"js_fetch_max_response_buffer_size": true, "js_fetch_protocols": true, "js_fetch_proxy": true,
	"js_fetch_timeout": true, "js_fetch_trusted_certificate": true, "js_fetch_verify": true,
	"js_fetch_verify_depth": true, "js_filter": true, "js_header_filter": true, "js_import": true,
	"js_include": true, "js_load_http_native_module": true, "js_load_stream_native_module": true,
	"js_path": true, "js_periodic": true, "js_preload_object": true, "js_preread": true,
	"js_set": true, "js_shared_dict_zone": true, "js_var": true, "json_max_depth": true,
	"json_set": true, "keepalive": true, "keepalive_disable": true, "keepalive_min_timeout": true,
	"keepalive_requests": true, "keepalive_time": true, "keepalive_timeout": true, "keyval": true,
	"keyval_zone": true, "large_client_header_buffers": true, "least_conn": true, "least_time": true,
	"license_pending_token": true, "license_token": true, "limit_conn": true, "limit_conn_dry_run": true,
	"limit_conn_log_level": true, "limit_conn_status": true, "limit_conn_zone": true,
	"limit_except": true, "limit_rate": true, "limit_rate_after": true, "limit_req": true,
	"limit_req_dry_run": true, "limit_req_log_level": true, "limit_req_status": true,
	"limit_req_zone": true, "limit_zone": true, "lingering_close": true, "lingering_time": true,
	"lingering_timeout": true, "listen": true, "load_module": true, "location": true,
	"lock_file": true, "log_format": true, "log_not_found": true, "log_subrequest": true,
	"logout_token_hint": true, "logout_uri": true, "mail": true, "map": true, "map_hash_bucket_size": true,
	"map_hash_max_size": true, "master_process": true, "match": true, "max_errors": true,
	"max_headers": true, "max_ranges": true, "memcached_allow_upstream": true, "memcached_bind": true,
	"memcached_bind_dynamic": true, "memcached_buffer_size": true, "memcached_connect_timeout": true,
	"memcached_gzip_flag": true, "memcached_next_upstream": true, "memcached_next_upstream_timeout": true,
	"memcached_next_upstream_tries": true, "memcached_pass": true, "memcached_read_timeout": true,
	"memcached_send_timeout": true, "memcached_socket_keepalive": true, "merge_slashes": true,
	"mgmt": true, "min_delete_depth": true, "mirror": true, "mirror_request_body": true,
	"modern_browser": true, "modern_browser_value": true, "mp4": true, "mp4_buffer_size": true,
	"mp4_limit_rate": true, "mp4_limit_rate_after": true, "mp4_max_buffer_size": true,
	"mp4_start_key_frame": true, "mqtt": true, "mqtt_buffers": true, "mqtt_preread": true,
	"mqtt_rewrite_buffer_size": true, "mqtt_set_connect": true, "msie_padding": true,
	"msie_refresh": true, "multi_accept": true, "ntlm": true, "num_map": true, "oidc_provider": true,
	"open_file_cache": true, "open_file_cache_errors": true, "open_file_cache_min_uses": true,
	"open_file_cache_valid": true, "open_log_file_cache": true, "otel_exporter": true,
	"otel_resource_attr": true, "otel_service_name": true, "otel_span_attr": true, "otel_span_name": true,
	"otel_trace": true, "otel_trace_context": true, "output_buffers": true, "override_charset": true,
	"pass": true, "pcre_jit": true, "perl": true, "perl_modules": true, "perl_require": true,
	"perl_set": true, "pid": true, "pkce": true, "pop3_auth": true, "pop3_capabilities": true,
	"port_in_redirect": true, "post_logout_uri": true, "postpone_output": true, "preferred_chain": true,
	"preread_buffer_size": true, "preread_timeout": true, "profile": true, "protocol": true,
	"proxy": true, "proxy_allow_upstream": true, "proxy_bind": true, "proxy_bind_dynamic": true,
	"proxy_buffer": true, "proxy_buffer_size": true, "proxy_buffering": true, "proxy_buffers": true,
	"proxy_busy_buffers_size": true, "proxy_cache": true, "proxy_cache_background_update": true,
	"proxy_cache_bypass": true, "proxy_cache_convert_head": true, "proxy_cache_key": true,
	"proxy_cache_lock": true, "proxy_cache_lock_age": true, "proxy_cache_lock_timeout": true,
	"proxy_cache_max_range_offset": true, "proxy_cache_methods": true, "proxy_cache_min_uses": true,
	"proxy_cache_path": true, "proxy_cache_purge": true, "proxy_cache_revalidate": true,
	"proxy_cache_use_stale": true, "proxy_cache_valid": true, "proxy_connect_timeout": true,
	"proxy_cookie_domain": true, "proxy_cookie_flags": true, "proxy_cookie_path": true,
	"proxy_download_rate": true, "proxy_force_ranges": true, "proxy_half_close": true,
	"proxy_headers_hash_bucket_size": true, "proxy_headers_hash_max_size": true, "proxy_hide_header": true,
	"proxy_http_version": true, "proxy_ignore_client_abort": true, "proxy_ignore_headers": true,
	"proxy_intercept_errors": true, "proxy_limit_rate": true, "proxy_max_temp_file_size": true,
	"proxy_method": true, "proxy_next_upstream": true, "proxy_next_upstream_timeout": true,
	"proxy_next_upstream_tries": true, "proxy_no_cache": true, "proxy_pass": true, "proxy_pass_error_message": true,
	"proxy_pass_header": true, "proxy_pass_request_body": true, "proxy_pass_request_headers": true,
	"proxy_pass_trailers": true, "proxy_password": true, "proxy_protocol": true, "proxy_protocol_timeout": true,
	"proxy_read_timeout": true, "proxy_redirect": true, "proxy_request_buffering": true,
	"proxy_request_dynamic": true, "proxy_requests": true, "proxy_responses": true, "proxy_send_lowat": true,
	"proxy_send_timeout": true, "proxy_session_drop": true, "proxy_set_body": true, "proxy_set_header": true,
	"proxy_smtp_auth": true, "proxy_socket_keepalive": true, "proxy_socket_rcvbuf": true,
	"proxy_socket_sndbuf": true, "proxy_ssl": true, "proxy_ssl_alpn": true, "proxy_ssl_certificate": true,
	"proxy_ssl_certificate_cache": true, "proxy_ssl_certificate_key": true, "proxy_ssl_ciphers": true,
	"proxy_ssl_conf_command": true, "proxy_ssl_crl": true, "proxy_ssl_key_log": true,
	"proxy_ssl_name": true, "proxy_ssl_password_file": true, "proxy_ssl_protocols": true,
	"proxy_ssl_server_name": true, "proxy_ssl_session_reuse": true, "proxy_ssl_trusted_certificate": true,
	"proxy_ssl_verify": true, "proxy_ssl_verify_depth": true, "proxy_store": true, "proxy_store_access": true,
	"proxy_temp_file_write_size": true, "proxy_temp_path": true, "proxy_timeout": true,
	"proxy_upload_rate": true, "proxy_username": true, "queue": true, "quic_active_connection_id_limit": true,
	"quic_bpf": true, "quic_gso": true, "quic_host_key": true, "quic_retry": true, "random": true,
	"random_index": true, "read_ahead": true, "real_ip_header": true, "real_ip_recursive": true,
	"recursive_error_pages": true, "redirect_uri": true, "referer_hash_bucket_size": true,
	"referer_hash_max_size": true, "request_pool_size": true, "reset_timedout_connection": true,
	"resolver": true, "resolver_timeout": true, "return": true, "rewrite": true, "rewrite_log": true,
	"root": true, "satisfy": true, "scgi_allow_upstream": true, "scgi_bind": true, "scgi_bind_dynamic": true,
	"scgi_buffer_size": true, "scgi_buffering": true, "scgi_buffers": true, "scgi_busy_buffers_size": true,
	"scgi_cache": true, "scgi_cache_background_update": true, "scgi_cache_bypass": true,
	"scgi_cache_key": true, "scgi_cache_lock": true, "scgi_cache_lock_age": true, "scgi_cache_lock_timeout": true,
	"scgi_cache_max_range_offset": true, "scgi_cache_methods": true, "scgi_cache_min_uses": true,
	"scgi_cache_path": true, "scgi_cache_purge": true, "scgi_cache_revalidate": true,
	"scgi_cache_use_stale": true, "scgi_cache_valid": true, "scgi_connect_timeout": true,
	"scgi_force_ranges": true, "scgi_hide_header": true, "scgi_ignore_client_abort": true,
	"scgi_ignore_headers": true, "scgi_intercept_errors": true, "scgi_limit_rate": true,
	"scgi_max_temp_file_size": true, "scgi_next_upstream": true, "scgi_next_upstream_timeout": true,
	"scgi_next_upstream_tries": true, "scgi_no_cache": true, "scgi_param": true, "scgi_pass": true,
	"scgi_pass_header": true, "scgi_pass_request_body": true, "scgi_pass_request_headers": true,
	"scgi_read_timeout": true, "scgi_request_buffering": true, "scgi_request_dynamic": true,
	"scgi_send_timeout": true, "scgi_socket_keepalive": true, "scgi_socket_rcvbuf": true,
	"scgi_socket_sndbuf": true, "scgi_store": true, "scgi_store_access": true, "scgi_temp_file_write_size": true,
	"scgi_temp_path": true, "scope": true, "secure_link": true, "secure_link_md5": true,
	"secure_link_secret": true, "send_lowat": true, "send_timeout": true, "sendfile": true,
	"sendfile_max_chunk": true, "server": true, "server_name": true, "server_name_in_redirect": true,
	"server_names_hash_bucket_size": true, "server_names_hash_max_size": true, "server_tokens": true,
	"session_log": true, "session_log_format": true, "session_log_zone": true, "session_store": true,
	"session_timeout": true, "set": true, "set_real_ip_from": true, "slice": true, "smtp_auth": true,
	"smtp_capabilities": true, "smtp_client_buffer": true, "smtp_greeting_delay": true,
	"source_charset": true, "split_clients": true, "ssi": true, "ssi_last_modified": true,
	"ssi_min_file_chunk": true, "ssi_silent_errors": true, "ssi_types": true, "ssi_value_length": true,
	"ssl": true, "ssl_alpn": true, "ssl_buffer_size": true, "ssl_certificate": true,
	"ssl_certificate_cache": true, "ssl_certificate_compression": true, "ssl_certificate_key": true,
	"ssl_ciphers": true, "ssl_client_certificate": true, "ssl_conf_command": true, "ssl_crl": true,
	"ssl_dhparam": true, "ssl_early_data": true, "ssl_ecdh_curve": true, "ssl_ech_file": true,
	"ssl_engine": true, "ssl_handshake_timeout": true, "ssl_key_log": true, "ssl_object_cache_inheritable": true,
	"ssl_ocsp": true, "ssl_ocsp_cache": true, "ssl_ocsp_responder": true, "ssl_password_file": true,
	"ssl_prefer_server_ciphers": true, "ssl_preread": true, "ssl_protocols": true, "ssl_reject_handshake": true,
	"ssl_session_cache": true, "ssl_session_ticket_key": true, "ssl_session_tickets": true,
	"ssl_session_timeout": true, "ssl_stapling": true, "ssl_stapling_file": true, "ssl_stapling_responder": true,
	"ssl_stapling_verify": true, "ssl_trusted_certificate": true, "ssl_verify": true,
	"ssl_verify_client": true, "ssl_verify_depth": true, "stall_threshold": true, "starttls": true,
	"state": true, "state_path": true, "status": true, "status_format": true, "status_zone": true,
	"sticky": true, "stream": true, "stub_status": true, "sub_filter": true, "sub_filter_last_modified": true,
	"sub_filter_once": true, "sub_filter_types": true, "subrequest_output_buffer_size": true,
	"tcp_nodelay": true, "tcp_nopush": true, "thread_pool": true, "timeout": true, "timer_resolution": true,
	"try_files": true, "tunnel_allow_upstream": true, "tunnel_bind": true, "tunnel_bind_dynamic": true,
	"tunnel_buffer_size": true, "tunnel_connect_timeout": true, "tunnel_next_upstream": true,
	"tunnel_next_upstream_timeout": true, "tunnel_next_upstream_tries": true, "tunnel_pass": true,
	"tunnel_read_timeout": true, "tunnel_send_lowat": true, "tunnel_send_timeout": true,
	"tunnel_socket_keepalive": true, "tunnel_socket_rcvbuf": true, "tunnel_socket_sndbuf": true,
	"types": true, "types_hash_bucket_size": true, "types_hash_max_size": true, "underscores_in_headers": true,
	"uninitialized_variable_warn": true, "upstream": true, "upstream_conf": true, "uri": true,
	"usage_report": true, "use": true, "user": true, "userid": true, "userid_domain": true,
	"userid_expires": true, "userid_flags": true, "userid_mark": true, "userid_name": true,
	"userid_p3p": true, "userid_path": true, "userid_service": true, "userinfo": true,
	"uwsgi_allow_upstream": true, "uwsgi_bind": true, "uwsgi_bind_dynamic": true, "uwsgi_buffer_size": true,
	"uwsgi_buffering": true, "uwsgi_buffers": true, "uwsgi_busy_buffers_size": true,
	"uwsgi_cache": true, "uwsgi_cache_background_update": true, "uwsgi_cache_bypass": true,
	"uwsgi_cache_key": true, "uwsgi_cache_lock": true, "uwsgi_cache_lock_age": true,
	"uwsgi_cache_lock_timeout": true, "uwsgi_cache_max_range_offset": true, "uwsgi_cache_methods": true,
	"uwsgi_cache_min_uses": true, "uwsgi_cache_path": true, "uwsgi_cache_purge": true,
	"uwsgi_cache_revalidate": true, "uwsgi_cache_use_stale": true, "uwsgi_cache_valid": true,
	"uwsgi_connect_timeout": true, "uwsgi_force_ranges": true, "uwsgi_hide_header": true,
	"uwsgi_ignore_client_abort": true, "uwsgi_ignore_headers": true, "uwsgi_intercept_errors": true,
	"uwsgi_limit_rate": true, "uwsgi_max_temp_file_size": true, "uwsgi_modifier1": true,
	"uwsgi_modifier2": true, "uwsgi_next_upstream": true, "uwsgi_next_upstream_timeout": true,
	"uwsgi_next_upstream_tries": true, "uwsgi_no_cache": true, "uwsgi_param": true, "uwsgi_pass": true,
	"uwsgi_pass_header": true, "uwsgi_pass_request_body": true, "uwsgi_pass_request_headers": true,
	"uwsgi_read_timeout": true, "uwsgi_request_buffering": true, "uwsgi_request_dynamic": true,
	"uwsgi_send_timeout": true, "uwsgi_socket_keepalive": true, "uwsgi_socket_rcvbuf": true,
	"uwsgi_socket_sndbuf": true, "uwsgi_ssl_certificate": true, "uwsgi_ssl_certificate_cache": true,
	"uwsgi_ssl_certificate_key": true, "uwsgi_ssl_ciphers": true, "uwsgi_ssl_conf_command": true,
	"uwsgi_ssl_crl": true, "uwsgi_ssl_key_log": true, "uwsgi_ssl_name": true, "uwsgi_ssl_password_file": true,
	"uwsgi_ssl_protocols": true, "uwsgi_ssl_server_name": true, "uwsgi_ssl_session_reuse": true,
	"uwsgi_ssl_trusted_certificate": true, "uwsgi_ssl_verify": true, "uwsgi_ssl_verify_depth": true,
	"uwsgi_store": true, "uwsgi_store_access": true, "uwsgi_temp_file_write_size": true,
	"uwsgi_temp_path": true, "valid_referers": true, "variables_hash_bucket_size": true,
	"variables_hash_max_size": true, "worker_aio_requests": true, "worker_connections": true,
	"worker_cpu_affinity": true, "worker_priority": true, "worker_processes": true, "worker_rlimit_core": true,
	"worker_rlimit_nofile": true, "worker_shutdown_timeout": true, "working_directory": true,
	"xclient": true, "xml_entities": true, "xml_external_entities": true, "xslt_last_modified": true,
	"xslt_param": true, "xslt_string_param": true, "xslt_stylesheet": true, "xslt_types": true,
	"zone": true, "zone_sync": true, "zone_sync_buffers": true, "zone_sync_connect_retry_interval": true,
	"zone_sync_connect_timeout": true, "zone_sync_interval": true, "zone_sync_recv_buffer_size": true,
	"zone_sync_server": true, "zone_sync_ssl": true, "zone_sync_ssl_certificate": true,
	"zone_sync_ssl_certificate_key": true, "zone_sync_ssl_ciphers": true, "zone_sync_ssl_conf_command": true,
	"zone_sync_ssl_crl": true, "zone_sync_ssl_name": true, "zone_sync_ssl_password_file": true,
	"zone_sync_ssl_protocols": true, "zone_sync_ssl_server_name": true, "zone_sync_ssl_trusted_certificate": true,
	"zone_sync_ssl_verify": true, "zone_sync_ssl_verify_depth": true, "zone_sync_timeout": true,
}
