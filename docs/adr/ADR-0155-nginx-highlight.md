# ADR-0155 nginx 설정 강조. 놓인 디렉터리로 알아보고 알려진 directive 만 칠한다

- 상태: 채택
- 날짜: 2026-10-14
- 관련: ADR-0039(줄 단위 문법 강조), ADR-0040(코드펜스 위임), ADR-0047(자동 들여쓰기), ADR-0055(json·yaml 강조), ADR-0080(언어는 표의 한 줄)

## 맥락

nginx 설정을 열면 통째로 흰 글자였다. 정할 것은 셋이다.

1. 무엇으로 nginx 설정인 줄 아는가. 흔한 이름이 `.conf` 인데 apache·supervisord·fontconfig 도 그 확장자를 쓴다
2. directive 이름을 무엇으로 고르는가
3. directive 말고 무엇을 칠하는가

## 1. 이름에 더해 놓인 디렉터리를 본다

- `nginx.conf` 와 확장자 `.nginx` 는 이름만으로 안다. 표의 한 줄이다
- `nginx` 디렉터리 아래의 `.conf` 는 nginx 다. `/etc/nginx/conf.d/default.conf`, homebrew 의 `/opt/homebrew/etc/nginx/servers/a.conf` 가 그것이다
- `sites-available`·`sites-enabled` 안의 파일은 이름이 무엇이든 nginx 다. debian 계열이 거기 확장자 없는 `default` 를 둔다

**`LanguageFor` 가 경로의 디렉터리를 보는 것은 이것이 처음이다.** 그 함수는 지금까지 마지막 이름만 봤다(`Dockerfile.dev` 의 접두도 이름이다). 그래도 「내용을 보지 않는다」는 그대로다. 파일이 없어도(새로 만드는 buffer) 같은 답이 나온다.

**상대 경로는 지금 디렉터리를 붙여 본다.** `/etc/nginx` 에서 `zn conf.d/default.conf` 로 열면 경로에 `nginx` 가 없다. 그래서 답이 cwd 에 매인다. 같은 이름이 어디서 여느냐로 달라지는데, 그 cwd 가 실제로 그 파일이 놓인 자리라 틀린 답은 아니다.

디렉터리 이름은 정확히 `nginx` 여야 한다. `nginx-proxy/app.conf` 는 아니다. 이름에 nginx 가 들었다고 그 안의 `.conf` 가 nginx 설정이라는 보장이 없다.

## 2. nginx.org 의 directive 색인에 든 이름만 칠한다

문장 앞 낱말(줄 앞, `;` `{` `}` 뒤) 이 directive 자리다. 그 자리의 낱말 중 **표에 든 것만** keyword 색이다. 표는 nginx.org 의 directive 색인(`docs/dirindex.html`) 을 2026-10-14 에 옮겨 적은 808 개다.

문장 앞 낱말을 다 칠하지 않은 것은 문장 앞에 directive 가 아닌 것이 서기 때문이다. `types { text/html html; }` 의 MIME 이름과 `map` 블록의 키(`default` 포함) 가 그렇다. 대신 서드파티 모듈의 directive 와 오타는 색이 없다. 오타가 색을 받지 않는 것은 덤이다.

**문장은 줄을 넘는다.** `server_name a.com` 다음 줄의 `b.com;` 은 인자다. 그래서 문맥이 셋이다.

- `nginxNormal` 은 다음 낱말이 directive 이름인 자리다
- `nginxArgs` 는 앞 줄의 문장이 `;` 나 `{` 없이 끝난 자리다
- `nginxQuoted` 는 줄을 넘은 따옴표 안이다

`;` 를 빠뜨리면 다음 `;` 까지가 인자로 읽혀 directive 색이 빠진다. nginx 도 그렇게 읽으므로 화면이 틀린 것을 틀린 대로 보인다.

## 3. 변수·문자열·주석·수·`on`/`off`·location 패턴

- **변수**(`$host` `${x}`) 는 따옴표 안에서도 떼어 칠한다. nginx 가 따옴표 안의 변수도 값으로 바꾼다. `log_format` 은 거의 전부가 따옴표 안의 변수라, 통째로 문자열 색이면 무엇을 남기는지가 보이지 않는다. `$` 뒤에 이름이 없으면 그냥 글자다. 정규식 끝 표시(`^/a$`) 가 그것이다
- **주석**은 낱말 앞의 `#` 부터다. `a#b` 의 `#` 은 글자다. nginx 가 그렇게 읽는다
- **수와 `on`/`off` 는 값 전체가 그것일 때만**이다. yaml 과 같은 규칙이다(ADR-0055). 크기·시간 단위(`10m` `1k` `30s` `500ms`) 와 소수(`1.5s`) 를 받는다. `127.0.0.1:8080` 의 `127` 은 수가 아니다
- **location 의 비교 표시**(`=` `~` `~*` `^~`) 와 그 뒤 패턴은 문자열 색이다. 붙여 적은 것(`=/50x.html`) 도 같다. 패턴 안의 `$` 는 변수로 읽지 않는다. 표시 없는 `location /api/` 는 경로라 변수만 집는다

정규식이 서는 다른 자리(`if ($a ~ b)`, `rewrite`, `server_name ~^www`, `map` 의 `~키`) 는 칠하지 않았다. 고른 범위가 location 이었다.

## 들여쓰기와 뼈대

`nginxIndent` 를 따로 둔다. `braceIndent`(go·js·css) 는 `default` 를 switch 이름표로 보고 한 단계 내는데, `map` 블록의 `default 0;` 이 그 자리에 걸린다. 둥근 괄호·대괄호도 세지 않는다. 따옴표 없는 정규식 안의 괄호가 짝이 맞는다는 보장이 없다. 중괄호만 센다. 관례 단계는 nginx 가 내놓는 기본 설정과 같은 space 넷이다.

뼈대는 `blockOutline` 이다. 깊은 `server` → `location` 안에서 지금 어디인지가 위에 남는다(ADR-0047).

## 기각한 것

- **`.conf` 전부를 nginx 로.** 다른 형식의 `.conf` 에 엉뚱한 색이 입는다
- **이름만.** `/etc/nginx/conf.d/*.conf` 가 강조되지 않는다. nginx 설정 파일의 대부분이 거기 있다
- **내용 보기**(첫 줄 몇에서 `server {` 찾기). 이 함수가 내용에 매인다(ADR-0080). shebang 과 같이 볼 일이라 tasks.md 의 그 건에 남는다
- **문장 앞 낱말 전부를 directive 로.** 2 에 적었다

## 결과

- 언어 표가 열세 줄에서 열네 줄이 되었다. 코드펜스 이름 `nginx` 가 늘었다
- 갈래는 늘지 않았다. directive 는 keyword, 패턴은 string, `on`/`off` 는 constant 다
- directive 표는 손으로 옮긴 것이라 nginx 가 directive 를 더하면 따라가지 않는다. 새 이름은 색이 없을 뿐 틀리게 칠해지지는 않는다
- `content_by_lua_block { ... }` 처럼 블록 안에 다른 언어를 품는 자리는 nginx 로 훑는다. 그 안의 낱말은 대개 표에 없어 색이 없다
