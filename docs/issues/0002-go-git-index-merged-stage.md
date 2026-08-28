# 0002: go-git 의 `index.Merged` 는 1 인데 디코더는 0 을 넣는다

- 상태: **우회함** — 우리 쪽에 상수를 두고 0 과 견준다(`internal/core/git-changes.go` 의 `gitStageMerged`)
- 난 곳: `github.com/go-git/go-git/v5` v5.19.2 `plumbing/format/index`
- 날짜: 2026-08-28

## 증상

index 항목이 **전부 병합 충돌로 읽힌다.** `entry.Stage != index.Merged` 로 충돌을 가리면 충돌이 하나도 없는 저장소에서 추적하는 파일 전부가 걸린다. 이 저장소에서 426 개 전부였다.

## 재현

```go
idx, _ := repo.Storer.Index()
for _, entry := range idx.Entries {
	fmt.Println(entry.Name, entry.Stage, index.Merged) // a.txt 0 1
}
```

## 우리 코드가 아니라는 근거

라이브러리 안에서 상수와 디코더가 서로 다른 값을 쓴다.

- `plumbing/format/index/index.go:33` — `Merged Stage = 1`
- `plumbing/format/index/decoder.go:132` — `e.Stage = Stage(flags>>12) & 0x3`

디코더가 넣는 것은 **파일에 적힌 stage 값 그대로**다. git 의 index 형식에서 그 값은 0(병합됨)·1(공통 조상)·2(ours)·3(theirs) 이고, 충돌이 없는 항목은 0 이다. 그래서 디코더가 낸 값은 상수와 한 칸씩 어긋나 있다.

`ResolveUndo` 쪽은 `Stage(i+1)` 로 1 부터 세어서(decoder.go:468) 상수 쪽 셈법을 쓴다. 한 패키지 안에서 두 셈법이 같이 산다.

## 지금 어떻게 넘겼는가

우리 상수 `gitStageMerged = 0` 을 두고 그것과 견준다. 라이브러리 상수는 쓰지 않는다.

`git status` 와 판정을 통째로 견주는 시험이 이 자리를 지킨다(`git-dirty_test.go`, `git-changes_test.go` 의 「TREE 캐시가 없어도 깨끗하다」).

## 다시 나면 무엇부터 볼지

판을 올렸을 때 이 자리가 고쳐졌는지 본다. 고치는 길이 둘이라 어느 쪽인지 확인해야 한다.

- 디코더가 `+1` 해서 상수에 맞추면 우리 상수를 `index.Merged` 로 되돌린다
- 상수를 0 으로 고치면 우리 상수만 지우면 된다

## 왜 여태 안 드러났는가

우리 코드가 이 자리를 「하나라도 다른 것이 있나」에만 썼고, 그 판정은 index 의 TREE 캐시가 성하면 그 앞에서 끝났다. 캐시가 깨진 저장소에서만 이 길로 왔고 그때는 대개 실제로도 dirty 였다.

파일별 표를 만들면서(ADR-0094) 같은 자리가 「어느 파일이 다른가」를 답하게 되자 곧바로 드러났다. 트리에 안 고친 파일까지 `M` 이 섰다.
