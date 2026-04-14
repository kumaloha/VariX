package compile

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

func ParseOutput(raw string) (Output, error) {
	clean := strings.TrimSpace(raw)
	clean = strings.TrimPrefix(clean, "```json")
	clean = strings.TrimPrefix(clean, "```")
	clean = strings.TrimSuffix(clean, "```")
	clean = strings.TrimSpace(clean)

	var payload map[string]json.RawMessage
	if err := json.Unmarshal([]byte(clean), &payload); err != nil {
		return Output{}, fmt.Errorf("parse compile output: %w", err)
	}
	var out Output
	if err := json.Unmarshal(payload["summary"], &out.Summary); err != nil {
		return Output{}, fmt.Errorf("parse compile summary: %w", err)
	}
	_ = json.Unmarshal(payload["topics"], &out.Topics)
	_ = json.Unmarshal(payload["confidence"], &out.Confidence)
	_ = json.Unmarshal(payload["graph"], &out.Graph)
	normalizeNodeTaxonomy(&out.Graph)
	normalizeNodeTiming(&out.Graph)
	_ = json.Unmarshal(payload["verification"], &out.Verification)
	if rawDetails, ok := payload["details"]; ok {
		details, err := parseHiddenDetails(rawDetails)
		if err != nil {
			return Output{}, err
		}
		out.Details = details
	}
	if err := out.Validate(); err != nil {
		return Output{}, err
	}
	return out, nil
}

func normalizeNodeTiming(graph *ReasoningGraph) {
	if graph == nil {
		return
	}
	for i := range graph.Nodes {
		node := &graph.Nodes[i]
		switch node.Kind {
		case NodeFact, NodeImplicitCondition:
			if node.OccurredAt.IsZero() && !node.ValidFrom.IsZero() {
				node.OccurredAt = node.ValidFrom
			}
		case NodePrediction:
			if node.PredictionStartAt.IsZero() && !node.ValidFrom.IsZero() {
				node.PredictionStartAt = node.ValidFrom
			}
			if node.PredictionDueAt.IsZero() && !node.ValidTo.IsZero() {
				node.PredictionDueAt = node.ValidTo
			}
			if node.PredictionDueAt.IsZero() && !node.PredictionStartAt.IsZero() {
				if due, ok := inferPredictionDueAtFromText(node.Text, node.PredictionStartAt); ok {
					node.PredictionDueAt = due
				}
			}
		}
	}
}

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

func normalizeNodeTaxonomy(graph *ReasoningGraph) {
	if graph == nil {
		return
	}
	for i := range graph.Nodes {
		node := &graph.Nodes[i]
		text := strings.TrimSpace(node.Text)
		if text == "" {
			continue
		}
		if shouldNormalizeToExplicitCondition(node.Kind, text) {
			node.Kind = NodeExplicitCondition
		}
	}
}

func shouldNormalizeToExplicitCondition(kind NodeKind, text string) bool {
	if !isExplicitConditionText(text) {
		return false
	}
	switch kind {
	case "", NodeFact:
		return true
	default:
		return false
	}
}

func isExplicitConditionText(text string) bool {
	text = strings.TrimSpace(text)
	prefixes := []string{"如果", "若", "一旦", "假如", "倘若", "如若"}
	for _, prefix := range prefixes {
		if strings.HasPrefix(text, prefix) {
			return true
		}
	}
	return false
}

func parseHiddenDetails(raw json.RawMessage) (HiddenDetails, error) {
	var details HiddenDetails
	if len(raw) == 0 || string(raw) == "null" {
		return details, nil
	}

	var object HiddenDetails
	if err := json.Unmarshal(raw, &object); err == nil {
		return object, nil
	}

	var list []string
	if err := json.Unmarshal(raw, &list); err == nil {
		details.Caveats = list
		return details, nil
	}

	var objects []map[string]any
	if err := json.Unmarshal(raw, &objects); err == nil {
		details.Items = objects
		return details, nil
	}

	var text string
	if err := json.Unmarshal(raw, &text); err == nil {
		details.Caveats = []string{text}
		return details, nil
	}

	return HiddenDetails{}, fmt.Errorf("parse compile details: unsupported shape")
}
