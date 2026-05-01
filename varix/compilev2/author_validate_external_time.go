package compilev2

import (
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

type datedAuthorEvidenceValue struct {
	Date  time.Time
	Value float64
}

type datedAuthorEvidenceStats struct {
	First datedAuthorEvidenceValue
	Last  datedAuthorEvidenceValue
	Min   datedAuthorEvidenceValue
	Max   datedAuthorEvidenceValue
}

func datedValueStats(values []datedAuthorEvidenceValue) datedAuthorEvidenceStats {
	stats := datedAuthorEvidenceStats{
		First: values[0],
		Last:  values[len(values)-1],
		Min:   values[0],
		Max:   values[0],
	}
	for _, value := range values[1:] {
		if value.Value < stats.Min.Value {
			stats.Min = value
		}
		if value.Value > stats.Max.Value {
			stats.Max = value
		}
	}
	return stats
}

func filterDatedValues(values []datedAuthorEvidenceValue, start, end time.Time, hasWindow bool) []datedAuthorEvidenceValue {
	if len(values) == 0 {
		return nil
	}
	sort.Slice(values, func(i, j int) bool { return values[i].Date.Before(values[j].Date) })
	if !hasWindow {
		if len(values) > 12 {
			return values[len(values)-12:]
		}
		return values
	}
	out := make([]datedAuthorEvidenceValue, 0, len(values))
	for _, value := range values {
		if !start.IsZero() && value.Date.Before(start) {
			continue
		}
		if !end.IsZero() && value.Date.After(end) {
			continue
		}
		out = append(out, value)
	}
	return out
}

func parseAuthorEvidenceDateWindow(window string) (time.Time, time.Time, bool) {
	window = strings.TrimSpace(window)
	if window == "" {
		return time.Time{}, time.Time{}, false
	}
	var dates []time.Time
	for _, match := range regexp.MustCompile(`20\d{2}-\d{2}-\d{2}`).FindAllString(window, -1) {
		if date, err := time.Parse("2006-01-02", match); err == nil {
			dates = append(dates, date)
		}
	}
	for _, match := range regexp.MustCompile(`20\d{2}-\d{2}`).FindAllString(window, -1) {
		if strings.Contains(match, "-") {
			if date, err := time.Parse("2006-01", match); err == nil {
				dates = append(dates, date)
			}
		}
	}
	monthPattern := regexp.MustCompile(`(?i)(jan|feb|mar|apr|may|jun|jul|aug|sep|oct|nov|dec)[a-z]*\s+20\d{2}`)
	for _, match := range monthPattern.FindAllString(window, -1) {
		if date, err := time.Parse("Jan 2006", normalizeAuthorEvidenceMonth(match)); err == nil {
			dates = append(dates, date)
		}
	}
	if len(dates) == 0 {
		return time.Time{}, time.Time{}, false
	}
	sort.Slice(dates, func(i, j int) bool { return dates[i].Before(dates[j]) })
	start := dates[0]
	end := dates[len(dates)-1]
	if end.Day() == 1 && !strings.Contains(window, end.Format("2006-01-02")) {
		end = end.AddDate(0, 1, -1)
	}
	return start, end, true
}

func normalizeAuthorEvidenceMonth(value string) string {
	fields := strings.Fields(strings.TrimSpace(value))
	if len(fields) < 2 {
		return value
	}
	month := strings.ToLower(fields[0])
	month = strings.TrimSuffix(month, ".")
	if len(month) > 3 {
		month = month[:3]
	}
	return strings.Title(month) + " " + fields[len(fields)-1]
}

func parseStablecoinTimestamp(value any) (time.Time, bool) {
	switch typed := value.(type) {
	case float64:
		return time.Unix(int64(typed), 0).UTC().Truncate(24 * time.Hour), true
	case string:
		if parsed, err := strconv.ParseInt(typed, 10, 64); err == nil {
			return time.Unix(parsed, 0).UTC().Truncate(24 * time.Hour), true
		}
		if date, err := time.Parse("2006-01-02", typed); err == nil {
			return date, true
		}
	}
	return time.Time{}, false
}
