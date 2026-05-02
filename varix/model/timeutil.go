package model

import "time"

func NowUTC() time.Time {
	return time.Now().UTC()
}

func DurationToMilliseconds(duration time.Duration) int64 {
	ms := duration.Milliseconds()
	if ms <= 0 {
		return 1
	}
	return ms
}
