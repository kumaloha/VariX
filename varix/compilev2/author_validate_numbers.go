package compilev2

import (
	"github.com/kumaloha/VariX/varix/compile"
	"regexp"
	"strconv"
	"strings"
)

func authorClaimComparableNumbers(check compile.AuthorClaimCheck) []authorComparableNumber {
	parts := []string{check.Text}
	for _, requirement := range check.RequiredEvidence {
		parts = append(parts, requirement.OriginalValue, requirement.Description, requirement.Reason)
	}
	return parseAuthorComparableNumbers(strings.Join(parts, " "))
}

func parseAuthorComparableNumbers(text string) []authorComparableNumber {
	pattern := regexp.MustCompile(`(?i)(减少|下降|decrease(?:d)?|decline(?:d)?|drop(?:ped)?|down|[<>])?\s*(-?\d+(?:\.\d+)?)\s*(万亿|亿美元|亿美金|trillion|billion|t|b|万亿美金|万亿美元)`)
	matches := pattern.FindAllStringSubmatch(text, -1)
	out := make([]authorComparableNumber, 0, len(matches))
	seen := map[string]struct{}{}
	for _, match := range matches {
		value, err := strconv.ParseFloat(match[2], 64)
		if err != nil {
			continue
		}
		comparator := strings.TrimSpace(match[1])
		if value > 0 && isDecreaseMarker(comparator) {
			value = -value
			comparator = ""
		}
		unit := strings.ToLower(match[3])
		switch unit {
		case "万亿", "万亿美金", "万亿美元", "trillion", "t":
			unit = "trillion"
		case "亿美元", "亿美金":
			value = value / 10
			unit = "billion"
		case "billion", "b":
			unit = "billion"
		}
		key := comparator + "|" + strconv.FormatFloat(value, 'f', 6, 64) + "|" + unit
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, authorComparableNumber{Value: value, Unit: unit, Comparator: comparator})
	}
	return out
}

func isDecreaseMarker(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "减少", "下降", "decrease", "decreased", "decline", "declined", "drop", "dropped", "down":
		return true
	default:
		return false
	}
}

func externalEvidenceNumbersSupport(authorNumbers []authorComparableNumber, result authorExternalEvidenceResult) bool {
	sourceValues := externalEvidenceComparableValues(result)
	if len(sourceValues) == 0 {
		return false
	}
	for _, authorNumber := range authorNumbers {
		if !anySourceValueMatchesAuthorNumber(sourceValues, authorNumber) {
			return false
		}
	}
	return true
}

func externalEvidenceComparableValues(result authorExternalEvidenceResult) []float64 {
	text := strings.Join([]string{result.Title, result.Excerpt}, " ")
	rawMatches := regexp.MustCompile(`=\s*(-?\d+(?:\.\d+)?)`).FindAllStringSubmatch(text, -1)
	out := make([]float64, 0, len(rawMatches))
	isStablecoin := strings.Contains(strings.ToLower(result.Title+" "+result.URL), "stablecoin") || strings.Contains(strings.ToLower(result.URL), "stablecoins")
	for _, match := range rawMatches {
		value, err := strconv.ParseFloat(match[1], 64)
		if err != nil {
			continue
		}
		switch {
		case strings.Contains(strings.ToUpper(result.Title), "FRED WRESBAL"), strings.Contains(strings.ToUpper(result.Title), "FRED WALCL"):
			value = value / 1_000_000
		case isStablecoin:
			value = value / 1_000_000_000
		}
		out = append(out, value)
	}
	return out
}

func anySourceValueMatchesAuthorNumber(sourceValues []float64, authorNumber authorComparableNumber) bool {
	for _, sourceValue := range sourceValues {
		switch authorNumber.Comparator {
		case "<":
			if sourceValue < authorNumber.Value*1.02 {
				return true
			}
		case ">":
			if sourceValue > authorNumber.Value*0.98 {
				return true
			}
		default:
			tolerance := 0.08
			if authorNumber.Value >= 5 {
				tolerance = 0.05
			}
			if authorNumber.Value != 0 && absFloat64(sourceValue-authorNumber.Value)/absFloat64(authorNumber.Value) <= tolerance {
				return true
			}
		}
	}
	return false
}

func absFloat64(value float64) float64 {
	if value < 0 {
		return -value
	}
	return value
}
