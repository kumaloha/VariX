package compile

import (
	"regexp"
	"strings"
	"unicode"
)

const summaryMaxRunes = 30

func compactSummaryForDisplay(summary string, articleForm string, declarations []Declaration, units []SemanticUnit) string {
	summary = strings.TrimSpace(summary)
	if summary == "" || len([]rune(summary)) <= summaryMaxRunes {
		return summary
	}
	if isReaderInterestSummaryForm(articleForm) {
		if compact := compactReaderInterestSummary(declarations, units); compact != "" {
			return compact
		}
	}
	return compactGenericSummary(summary)
}

func compactReaderInterestSummary(declarations []Declaration, units []SemanticUnit) string {
	topics := summaryInterestTopics(declarations, units)
	if len(topics) == 0 {
		return ""
	}
	speaker := summaryPrimarySpeaker(declarations, units)
	if speaker == "" {
		speaker = "伯克希尔"
	}
	if title := titleFromUnits(speaker, topics, declarations, units); title != "" {
		return title
	}
	for count := minInt(len(topics), 3); count >= 1; count-- {
		joined := joinSummaryTopics(topics[:count])
		phrase := speaker + "阐明" + joined + "纪律"
		if strings.EqualFold(speaker, "伯克希尔") {
			phrase = speaker + "强调" + joined + "纪律"
		}
		if len([]rune(phrase)) <= summaryMaxRunes {
			return phrase
		}
	}
	return compactGenericSummary(speaker + "阐明" + topics[0] + "纪律")
}

func titleFromUnits(speaker string, topics []string, declarations []Declaration, units []SemanticUnit) string {
	if title := continuityTitleFromUnits(speaker, topics, units); title != "" {
		return title
	}
	_ = declarations
	return ""
}

func continuityTitleFromUnits(speaker string, topics []string, units []SemanticUnit) string {
	if !summaryTopicContains(topics, "资本配置") || !summaryTopicContains(topics, "组合管理") {
		return ""
	}
	predecessor := continuityPredecessorFromUnits(units)
	if predecessor == "" {
		return ""
	}
	name := compactWesternSpeakerName(speaker)
	if name == "" || strings.EqualFold(name, "伯克希尔") {
		name = "伯克希尔"
	}
	candidates := []string{name + "延续" + predecessor + "式资本纪律"}
	for _, candidate := range candidates {
		if len([]rune(candidate)) <= summaryMaxRunes {
			return candidate
		}
	}
	return ""
}

func summaryTopicContains(topics []string, target string) bool {
	for _, topic := range topics {
		if topic == target {
			return true
		}
	}
	return false
}

func continuityPredecessorFromUnits(units []SemanticUnit) string {
	for _, unit := range units {
		text := strings.Join([]string{unit.Subject, unit.Claim, unit.PromptContext}, " ")
		lower := strings.ToLower(text)
		if !strings.Contains(lower, "portfolio") && !strings.Contains(text, "组合") && !strings.Contains(lower, "circle of competence") && !strings.Contains(text, "能力圈") {
			continue
		}
		if predecessor := extractContinuityPredecessor(text); predecessor != "" {
			return predecessor
		}
	}
	return ""
}

var continuityPredecessorPatterns = []*regexp.Regexp{
	regexp.MustCompile(`由\s*([A-Za-z]+(?:\s+[A-Za-z]+){0,2}|[\p{Han}]{2,8})\s*(?:建立|打造|留下|构建)`),
	regexp.MustCompile(`([A-Za-z]+(?:\s+[A-Za-z]+){0,2}|[\p{Han}]{2,8})\s*(?:建立|打造|留下|构建)的(?:组合|portfolio)`),
}

func extractContinuityPredecessor(text string) string {
	for _, pattern := range continuityPredecessorPatterns {
		match := pattern.FindStringSubmatch(text)
		if len(match) >= 2 {
			return compactEntityName(match[1])
		}
	}
	return ""
}

func compactWesternSpeakerName(speaker string) string {
	name := compactEntityName(speaker)
	if name != "" {
		return name
	}
	return strings.TrimSpace(speaker)
}

func compactEntityName(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	lower := strings.ToLower(value)
	switch lower {
	case "warren buffett", "buffett":
		return "巴菲特"
	}
	if containsHan(value) {
		return value
	}
	fields := strings.Fields(value)
	if len(fields) == 0 {
		return ""
	}
	return fields[len(fields)-1]
}

func containsHan(value string) bool {
	for _, r := range value {
		if unicode.Is(unicode.Han, r) {
			return true
		}
	}
	return false
}

func joinSummaryTopics(topics []string) string {
	switch len(topics) {
	case 0:
		return ""
	case 1:
		return topics[0]
	case 2:
		return topics[0] + "与" + topics[1]
	default:
		return strings.Join(topics[:len(topics)-1], "、") + "与" + topics[len(topics)-1]
	}
}

func summaryInterestTopics(declarations []Declaration, units []SemanticUnit) []string {
	out := make([]string, 0, 4)
	for _, declaration := range declarations {
		text := strings.ToLower(strings.Join([]string{declaration.Kind, declaration.Topic, declaration.Statement}, " "))
		if strings.Contains(text, "capital_allocation") || strings.Contains(text, "资本配置") {
			out = appendUniqueNonEmptyStep(out, "资本配置")
		}
	}
	for _, unit := range topSemanticUnitsForSummary(units, "shareholder_meeting") {
		switch semanticCoverageCategory(unit) {
		case "capital_allocation":
			out = appendUniqueNonEmptyStep(out, "资本配置")
		case "portfolio_circle":
			out = appendUniqueNonEmptyStep(out, "组合管理")
		case "technology_operating_plan":
			out = appendUniqueNonEmptyStep(out, "AI治理")
		case "cyber_insurance":
			out = appendUniqueNonEmptyStep(out, "承保边界")
		case "tokyo_marine":
			out = appendUniqueNonEmptyStep(out, "交易")
		case "culture_succession":
			out = appendUniqueNonEmptyStep(out, "继任文化")
		default:
			if summaryReaderInterestRank(unit) == 2 {
				out = appendUniqueNonEmptyStep(out, "回购")
			}
			if summaryReaderInterestRank(unit) == 5 {
				out = appendUniqueNonEmptyStep(out, "公用事业")
			}
		}
	}
	return out
}

func summaryPrimarySpeaker(declarations []Declaration, units []SemanticUnit) string {
	for _, declaration := range declarations {
		if speaker := strings.TrimSpace(declaration.Speaker); speaker != "" {
			return speaker
		}
	}
	for _, unit := range units {
		if speaker := strings.TrimSpace(unit.Speaker); speaker != "" {
			return speaker
		}
	}
	return ""
}

func compactGenericSummary(summary string) string {
	summary = strings.TrimSpace(strings.TrimRight(summary, "。.!！"))
	for _, sep := range []string{"；", "。", "，", ",", ";"} {
		if before, _, ok := strings.Cut(summary, sep); ok {
			before = strings.TrimSpace(before)
			if before != "" && len([]rune(before)) <= summaryMaxRunes {
				return before
			}
		}
	}
	return truncateRunes(summary, summaryMaxRunes)
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
