package contentstore

import (
	"fmt"
	"strings"
	"time"
)

type subjectHorizonSpec struct {
	Horizon       string
	RefreshPolicy string
	WindowStart   func(time.Time) time.Time
	NextRefresh   func(time.Time) time.Time
}

func subjectHorizonSpecFor(horizon string) (subjectHorizonSpec, error) {
	switch strings.TrimSpace(horizon) {
	case "1w":
		return subjectHorizonSpec{"1w", "daily", func(t time.Time) time.Time { return t.AddDate(0, 0, -7) }, func(t time.Time) time.Time { return t.AddDate(0, 0, 1) }}, nil
	case "1m":
		return subjectHorizonSpec{"1m", "weekly", func(t time.Time) time.Time { return t.AddDate(0, -1, 0) }, func(t time.Time) time.Time { return t.AddDate(0, 0, 7) }}, nil
	case "1q":
		return subjectHorizonSpec{"1q", "monthly", func(t time.Time) time.Time { return t.AddDate(0, -3, 0) }, func(t time.Time) time.Time { return t.AddDate(0, 1, 0) }}, nil
	case "1y":
		return subjectHorizonSpec{"1y", "quarterly", func(t time.Time) time.Time { return t.AddDate(-1, 0, 0) }, func(t time.Time) time.Time { return t.AddDate(0, 3, 0) }}, nil
	case "2y":
		return subjectHorizonSpec{"2y", "semiannual", func(t time.Time) time.Time { return t.AddDate(-2, 0, 0) }, func(t time.Time) time.Time { return t.AddDate(0, 6, 0) }}, nil
	case "5y":
		return subjectHorizonSpec{"5y", "annual", func(t time.Time) time.Time { return t.AddDate(-5, 0, 0) }, func(t time.Time) time.Time { return t.AddDate(1, 0, 0) }}, nil
	default:
		return subjectHorizonSpec{}, fmt.Errorf("unsupported subject horizon %q; supported: 1w, 1m, 1q, 1y, 2y, 5y", horizon)
	}
}
