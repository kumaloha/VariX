package compile

import (
	"fmt"
	"regexp"
	"strings"
)

const briefCategoryLimit = 2

var briefNumberPattern = regexp.MustCompile(`\b\d+(?:\.\d+)?%?|\$\d+(?:\.\d+)?\s*(?:billion|million)?`)

func stageBrief(state graphState) graphState {
	if !isReaderInterestSummaryForm(state.ArticleForm) || len(state.SemanticUnits) == 0 {
		return state
	}
	state.Brief = buildBriefItems(state.SemanticUnits, 14)
	return state
}

func buildBriefItems(units []SemanticUnit, limit int) []BriefItem {
	if len(units) == 0 || limit <= 0 {
		return nil
	}
	out := make([]BriefItem, 0, limit)
	counts := map[string]int{}
	for _, unit := range rankSemanticUnits(units, "") {
		item := briefItemFromSemanticUnit(unit)
		if item.Category == "" || item.Claim == "" {
			continue
		}
		if counts[item.Category] >= briefCategoryLimit {
			continue
		}
		counts[item.Category]++
		item.ID = fmt.Sprintf("brief-%03d", len(out)+1)
		out = append(out, item)
		if len(out) == limit {
			break
		}
	}
	return out
}

func briefItemFromSemanticUnit(unit SemanticUnit) BriefItem {
	text := strings.Join([]string{unit.Subject, unit.Force, unit.Claim, unit.PromptContext}, " ")
	claim := strings.TrimSpace(unit.Claim)
	item := BriefItem{
		Category:  briefCategory(text),
		Kind:      briefKind(text),
		Claim:     claim,
		Entities:  briefEntities(text),
		Numbers:   briefNumbers(text),
		Quote:     strings.TrimSpace(unit.SourceQuote),
		Salience:  unit.Salience,
		SourceIDs: []string{strings.TrimSpace(unit.ID)},
	}
	if item.Kind == "" {
		item.Kind = "point"
	}
	if len(item.Numbers) > 0 && item.Kind == "point" {
		item.Kind = "numeric"
	}
	return item
}

func briefCategory(text string) string {
	lower := strings.ToLower(text)
	switch {
	case containsAnyText(lower, []string{"buyback", "repurchase", "回购"}):
		return "buyback"
	case containsAnyText(lower, []string{"capital allocation", "cash", "treasury", "资本配置", "现金", "国债", "美债"}):
		return "capital"
	case containsAnyText(lower, []string{"portfolio", "holding", "apple", "coca-cola", "american express", "bank of america", "trading house", "现有组合", "股票组合", "持仓", "能力圈"}):
		return "portfolio"
	case containsAnyText(lower, []string{"cyber", "insurance", "underwriting", "geico", "float", "网络", "保险", "承保", "浮存金"}):
		return "insurance"
	case containsAIReference(lower) || containsAnyText(lower, []string{"artificial intelligence", "technology", "deepfake", "deep fake", "技术", "人工智能"}):
		return "ai"
	case containsAnyText(lower, []string{"data center", "energy", "utility", "utilities", "electric", "数据中心", "能源", "电力", "公用事业"}):
		return "energy"
	case containsAnyText(lower, []string{"succession", "successor", "greg abel", "ajit jain", "继任", "接班"}):
		return "succession"
	case containsAnyText(lower, []string{"culture", "values", "bureaucracy", "current form", "文化", "价值观", "官僚", "现有形式"}):
		return "culture"
	case containsAnyText(lower, []string{"tariff", "trade war", "关税"}):
		return "macro"
	case containsAnyText(lower, []string{"canada", "canadian", "shareholder", "股东", "加拿大"}):
		return "shareholder"
	case containsAnyText(lower, []string{"bnsf", "clayton", "rail", "铁路", "克莱顿", "建筑"}):
		return "operations"
	default:
		return "governance"
	}
}

func containsAIReference(text string) bool {
	return strings.Contains(" "+text+" ", " ai ") ||
		strings.Contains(text, "ai应用") ||
		strings.Contains(text, "ai在") ||
		strings.Contains(text, "ai算力") ||
		strings.Contains(text, "ai数据")
}

func briefKind(text string) string {
	lower := strings.ToLower(text)
	switch {
	case len(briefEntities(text)) >= 3:
		return "list"
	case containsAnyText(lower, []string{"will not", "不会", "拒绝", "避免", "boundary", "边界"}):
		return "boundary"
	case containsAnyText(lower, []string{"commit", "plan", "will", "计划", "承诺"}):
		return "commitment"
	case containsAnyText(lower, []string{"disclose", "披露"}):
		return "disclosure"
	default:
		return "point"
	}
}

func briefEntities(text string) []string {
	candidates := []string{
		"Apple",
		"American Express",
		"Coca-Cola",
		"Bank of America",
		"Tokyo Marine",
		"GEICO",
		"BNSF",
		"Clayton",
		"Ajit Jain",
		"Greg Abel",
	}
	out := make([]string, 0, len(candidates))
	lower := strings.ToLower(text)
	for _, candidate := range candidates {
		if strings.Contains(lower, strings.ToLower(candidate)) {
			out = append(out, candidate)
		}
	}
	return out
}

func briefNumbers(text string) []string {
	matches := briefNumberPattern.FindAllString(text, -1)
	if len(matches) == 0 {
		return nil
	}
	out := make([]string, 0, len(matches))
	seen := map[string]struct{}{}
	for _, match := range matches {
		match = strings.TrimSpace(match)
		if match == "" {
			continue
		}
		if _, ok := seen[match]; ok {
			continue
		}
		seen[match] = struct{}{}
		out = append(out, match)
	}
	return out
}
