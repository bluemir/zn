package core

import (
	"slices"
)

// 방문 기록(jumplog) 이다. `:jumplogs` 와 팔레트의 「방문한 자리」로 본다(ADR-0074).
//
// **되돌아오기 이력(jumplist) 과 다른 것이다.** 브라우저에 빗대면 jumplist 가 뒤로/앞으로이고
// 이쪽이 방문 기록이다(docs/tasks.md 에 적혀 있던 그림이다).
//
// 갈리는 자리가 셋이다.
//
//   - jumplist 는 **떠난 자리**만 담는다. 이쪽은 **떠난 자리와 닿은 자리 둘 다** 담아서
//     발자취가 끊기지 않는다 — A 에서 B, B 에서 C 로 뛰면 A·B·C 가 남는다.
//   - jumplist 는 새로 뛰면 **앞쪽을 버린다**(ADR-0070 §3). 이쪽은 버리지 않는다.
//     버려진 자리를 남기는 것이 이 기록의 몫이다.
//   - jumplist 는 오래된 것이 위다(`ctrl+o` 거리 순). 이쪽은 **최근이 위**다.
const jumpLogMax = 256

// jumpLog 는 방문한 자리들이다. **맨 앞이 가장 최근**이다.
//
// 화면에 보이는 것은 열여섯 줄뿐이지만(ADR-0069) 담는 것은 그보다 훨씬 많다 — 판이
// `j`/`k` 로 훑으므로 보이는 만큼만 담을 이유가 없다. 「아까 그 자리」를 되찾는 것이
// 이 기록의 쓸모라 짧으면 값이 없다.
type jumpLog struct {
	places []jumpPlace
}

// recordVisit 은 방문한 자리를 맨 앞에 남긴다.
//
// **같은 자리면 하나만 남기고 위로 올린다.** 같은 자리의 자는 파일 + 줄이고 칸은 보지
// 않는다(jumplist 가 잇단 중복을 거르는 자와 같다). 자주 가는 자리가 목록을 가득 채우면
// 짧은 목록으로 멀리 되짚을 수가 없다 — 브라우저 주소창과 같은 손이다.
func (e *editor) recordVisit(place jumpPlace) {
	if place.path == "" {
		return
	}

	e.logs.places = slices.DeleteFunc(e.logs.places, place.sameLine)
	e.logs.places = slices.Insert(e.logs.places, 0, place)

	if len(e.logs.places) > jumpLogMax {
		e.logs.places = e.logs.places[:jumpLogMax]
	}
}

// arrive 는 지금 커서 자리를 방문으로 남긴다. **닿은 뒤에** 부른다.
//
// **둘러보는(미리보기) 자리는 여기 오지 않는다.** 판에서 `j`/`k` 로 훑는 동안은 아직
// 정해지지 않은 상태이고 `esc` 가 그것을 없던 일로 만든다(ADR-0071, ADR-0073) — 그 약속을
// 기록에서만 깨면 어긋난다. 그래서 판은 **확정할 때** 한 번 부른다.
//
// 부르는 자리는 이것으로 전부다.
//
//   - 정의·사용처가 곧바로 뛸 때(gopls.go, references.go) 와 판에서 확정할 때(view-locations.go)
//   - 검색이 옮길 때(view-editor-search.go 의 jumpToMatch)
//   - `ctrl+o`·`ctrl+i` 와 두 판의 확정(jumplist.go, view-jumps.go, view-jumplogs.go)
//   - 파일 안에서 멀리 뛸 때(`G`·`gg`·`:번호`). 그쪽은 arrive 가 아니라 recordJumpMove 가
//     떠난 자리와 닿은 자리를 같이 남긴다(ADR-0082)
//   - 파일을 여는 세 문 — 트리·팔레트·`:e`(view-sidebar.go, view-palette.go, view-editor-command.go)
//
// 떠난 자리는 여기가 아니라 recordJumpFrom 이 같이 남긴다. 그쪽이 「뛰기 직전」이 모이는
// 자리라 한 번만 걸면 된다.
func (e *editor) arrive() {
	if place, ok := e.here(); ok {
		e.recordVisit(place)
	}
}
