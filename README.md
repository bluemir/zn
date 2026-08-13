# be

Bluemir's Editor

## concept

철저히 개인화된 vim의 개선 판 Text Editor

### motive
ai 시대가 되면서 다양한 요구사항을 만족하기 위해 굳이 복잡한 설정파일과 플러그인을 제공 하기 보다는 
그냥 각자가 코드 수정을 AI 에게 맡기는것이 더 비용이 낮을 것 같다는 생각이 들었습니다. 
코드의 품질이 일정수준 이상이라면 AI 도 수정을 쉽게 해주므로 필요에 맞는 editor 를 code 단위에서 설정할수 있도록 합니다. 

## feature

- terminal editor
- vim 키매핑
- 현대화된 tab 기능
- 한국어 지원
- integration 잘 된 file tree
- no config(config 는 compile 됨)
- fzf, command palette
	- 특히 file matching 에서 > 를 입력하면 바로 command 로 넘어가는 기능
	- ! 를 shell command 로 매핑할수도 있을듯
- 내장화된 언어지원
	- 종류
		- golang
		- markdown
		- html
		- css
		- js
	- 기능
		- 자동 완성
		- 정의/구현으로 이동
		- 사용처로 이동
		- syntax highlight
		- 다음줄로 갈때 자동 들여쓰기
		- 저장시 hook(eg. go fmt)
- 키 하나로 현재 화면의 cat 과 동일한 형태로 text 표시
	- text 복사 붙여 넣기시 유용
- mouse 지원
	- 편집 영역을 클릭해 커서 이동. insert 중에 눌러도 insert 에 머문다
	- filetree 를 클릭해 펼치기 접기 열기
	- tabline 을 클릭해 tab 전환
	- 휠로 스크롤. 포인터가 얹힌 쪽(편집 영역 / filetree) 이 굴러간다
	- mouse 를 켜 두는 동안 터미널 자체의 드래그 복사가 막힌다.
	  대부분의 터미널에서 `shift`+드래그로 터미널 쪽 선택을 할 수 있다
- 상대 줄번호와 절대 줄번호 동시 표시
- home, end, page up, page down 의 일관적인 동작

## DONOT

feature 에 넣지 않을 기능

- config file
	- config는 code 내에 하드코딩 한다. 
	- 분기를 최대한 줄이기 위한 방안이다.
- plugin
	- 필요한 기능은 전부 코드로 작성한다.
	- 플러그인 구조를 제외 하여 복잡도를 줄인다. 


## FAQ

Q1. A 기능이 필요합니다.
A1. repo를 Fork 하셔서 AI 에 해당 기능을 추가 해달라고 하시면 됩니다.

Q2. A 기능을 B 기능으로 변경해 주세요.
A2. repo를 Fork 하셔서 AI 에 해당 기능을 변경 해달라고 하시면 됩니다.

Q3. A를 변경 할수 있는 설정이 있을까요?
A3. repo를 Fork 하셔서 code 의 설정을 변경한후 build 하여 사용하시면 됩니다. 
