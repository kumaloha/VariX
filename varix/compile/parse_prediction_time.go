package compile

import (
	"regexp"
	"strconv"
	"strings"
	"time"
)

var (
	relativeYearWindow  = regexp.MustCompile(`(?:未来|今后|接下来)([一二两三四五六七八九十\d]+)年`)
	relativeMonthWindow = regexp.MustCompile(`(?:未来|今后|接下来)([一二两三四五六七八九十\d]+)个?月`)
	withinMonthWindow   = regexp.MustCompile(`([一二两三四五六七八九十\d]+)个?月内`)
)

func inferPredictionDueAtFromText(text string, start time.Time) (time.Time, bool) {
	text = strings.TrimSpace(text)
	if text == "" || start.IsZero() {
		return time.Time{}, false
	}
	if strings.Contains(text, "未来几年") || strings.Contains(text, "今后几年") {
		return time.Time{}, false
	}
	if matches := relativeYearWindow.FindStringSubmatch(text); len(matches) == 2 {
		if years, ok := parseChineseOrArabicInt(matches[1]); ok && years > 0 {
			return start.AddDate(years, 0, 0), true
		}
	}
	if matches := relativeMonthWindow.FindStringSubmatch(text); len(matches) == 2 {
		if months, ok := parseChineseOrArabicInt(matches[1]); ok && months > 0 {
			return start.AddDate(0, months, 0), true
		}
	}
	if matches := withinMonthWindow.FindStringSubmatch(text); len(matches) == 2 {
		if months, ok := parseChineseOrArabicInt(matches[1]); ok && months > 0 {
			return start.AddDate(0, months, 0), true
		}
	}
	if due, ok := inferPredictionDueAtFromCalendarWindow(text, start); ok {
		return due, true
	}
	return time.Time{}, false
}
func inferPredictionDueAtFromCalendarWindow(text string, start time.Time) (time.Time, bool) {
	if start.IsZero() {
		return time.Time{}, false
	}
	switch {
	case containsBoundedPhrase(text, "本季度", "这季度", "这个季度"):
		return quarterEnd(start.Year(), quarterOf(start), start.Location()), true
	case containsBoundedPhrase(text, "下季度", "下个季度", "下一季度"):
		year, quarter := nextQuarter(start)
		return quarterEnd(year, quarter, start.Location()), true
	case containsBoundedPhrase(text, "明年") && !containsAny(text, "明年后", "明年以后", "明年之后", "明年起", "明年开始"):
		return time.Date(start.Year()+1, time.December, 31, 23, 59, 59, 0, start.Location()), true
	default:
		return time.Time{}, false
	}
}
func containsBoundedPhrase(text string, phrases ...string) bool {
	for _, phrase := range phrases {
		if strings.Contains(text, phrase) {
			return true
		}
	}
	return false
}
func containsAny(text string, phrases ...string) bool {
	for _, phrase := range phrases {
		if strings.Contains(text, phrase) {
			return true
		}
	}
	return false
}
func quarterOf(ts time.Time) int {
	return (int(ts.Month())-1)/3 + 1
}
func nextQuarter(ts time.Time) (int, int) {
	quarter := quarterOf(ts) + 1
	year := ts.Year()
	if quarter > 4 {
		quarter = 1
		year++
	}
	return year, quarter
}
func quarterEnd(year, quarter int, loc *time.Location) time.Time {
	if loc == nil {
		loc = time.UTC
	}
	endMonth := time.Month(quarter * 3)
	return time.Date(year, endMonth+1, 0, 23, 59, 59, 0, loc)
}
func parseChineseOrArabicInt(raw string) (int, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0, false
	}
	if n, err := strconv.Atoi(raw); err == nil {
		return n, true
	}
	switch raw {
	case "一":
		return 1, true
	case "二", "两":
		return 2, true
	case "三":
		return 3, true
	case "四":
		return 4, true
	case "五":
		return 5, true
	case "六":
		return 6, true
	case "七":
		return 7, true
	case "八":
		return 8, true
	case "九":
		return 9, true
	case "十":
		return 10, true
	default:
		return 0, false
	}
}
