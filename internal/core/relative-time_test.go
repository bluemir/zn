package core

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// 단계가 갈리는 자리를 하나씩 짚는다. git 의 `%ar` 과 같은 자리에서 갈려야 한다.
func TestRelativeTime(t *testing.T) {
	now := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)

	cases := []struct {
		ago  time.Duration
		want string
	}{
		{0, "방금"},
		{9 * time.Second, "방금"},
		{10 * time.Second, "10 초 전"},
		{89 * time.Second, "89 초 전"},
		{90 * time.Second, "2 분 전"},
		{89 * time.Minute, "89 분 전"},
		{91 * time.Minute, "2 시간 전"},
		{35 * time.Hour, "35 시간 전"},
		{36 * time.Hour, "2 일 전"},
		{47 * time.Hour, "2 일 전"},
		{13 * 24 * time.Hour, "13 일 전"},
		{14 * 24 * time.Hour, "2 주 전"},
		{69 * 24 * time.Hour, "10 주 전"},
		{70 * 24 * time.Hour, "2 달 전"},
		{364 * 24 * time.Hour, "12 달 전"},
		{365 * 24 * time.Hour, "1 년 전"},
		{800 * 24 * time.Hour, "2 년 전"},
	}

	for _, c := range cases {
		assert.Equal(t, c.want, relativeTime(now.Add(-c.ago), now), "%s 전", c.ago)
	}
}

// 앞선 시각은 `방금` 이다. 시계가 어긋난 저장소에서 미래의 커밋이 온다.
func TestRelativeTimeInFuture(t *testing.T) {
	now := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)

	assert.Equal(t, "방금", relativeTime(now.Add(3*time.Hour), now))
}
