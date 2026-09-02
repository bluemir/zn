package textarea

// screenRow 는 화면 한 행에 그려지는 논리 줄의 일부다.
//
// **줄을 재는 일이 아니라 화면을 어떻게 채우는지에 대한 것이라 cluster.go 가 아니라 여기
// 있다.** 만드는 것은 `visibleRows`(buffer-screen.go) 이고 쓰는 것은 그리는 쪽이다
// (layout.go·render-row.go·sticky.go).
//
// 줄을 행으로 끊는 일 자체는 `wrapOffsets` 다. 그것이 낸 자리를 이 type 이 담는다.
type ScreenRow struct {
	Line  int // lines 의 index
	Start int // 줄 안의 byte offset. 포함
	End   int // 줄 안의 byte offset. 제외
}

// rowBefore 는 화면 행 a 가 b 보다 위인지 본다.
func rowBefore(a, b ViewTop) bool {
	return a.Line < b.Line || (a.Line == b.Line && a.Row < b.Row)
}
