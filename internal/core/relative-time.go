package core

import (
	"strconv"
	"time"
)

// relativeTime 은 지난 시각을 「얼마나 전인가」로 적는다. `git log` 의 `%ar` 자리다.
//
// **나누는 단계는 git 을 그대로 따른다.** 90 초까지 초, 90 분까지 분, 36 시간까지 시간,
// 14 일까지 일, 8 주까지 주, 365 일까지 달, 그다음이 해다. 단계가 걸치는 자리(90 분) 가
// 어중간해 보이지만 그것이 git 이 쓰는 자라, 옆 터미널의 `git graph` 와 값이 갈리지 않는다.
//
// 반올림도 git 과 같다. 89 분은 `1 시간 전` 이 아니라 `89 분 전` 이고, 91 분은 `2 시간 전` 이다.
//
// 해 단위는 달까지 적지 않는다. git 은 `1 year, 4 months ago` 라고 적는데, 그만큼 오래된
// 커밋에서 넉 달의 차이를 읽을 일이 없다.
//
// 앞선 시각이면 `방금` 이다. 시계가 어긋난 저장소에서 미래의 커밋이 나오는데, 그것을 굳이
// 「몇 시간 후」로 적어 봐야 알려주는 것이 없다.
func relativeTime(when, now time.Time) string {
	seconds := int(now.Sub(when).Seconds())
	if seconds < 10 {
		return "방금"
	}

	if seconds < 90 {
		return strconv.Itoa(seconds) + " 초 전"
	}

	minutes := (seconds + 30) / 60
	if minutes < 90 {
		return strconv.Itoa(minutes) + " 분 전"
	}

	hours := (minutes + 30) / 60
	if hours < 36 {
		return strconv.Itoa(hours) + " 시간 전"
	}

	// 36 시간이 곧 이틀이라 「하루 전」이 나오는 자리가 없다. git 도 `1 day ago` 를 찍지 않는다.
	days := (hours + 12) / 24
	if days < 14 {
		return strconv.Itoa(days) + " 일 전"
	}

	if days < 70 {
		return strconv.Itoa((days+3)/7) + " 주 전"
	}

	if days < 365 {
		return strconv.Itoa((days+15)/30) + " 달 전"
	}

	return strconv.Itoa((days+182)/365) + " 년 전"
}
