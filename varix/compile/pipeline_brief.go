package compile

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
)

const briefCategoryLimit = 2
const briefDefaultLimit = 14

var mandatoryMeetingBriefCategories = []string{
	"capital",
	"portfolio",
	"insurance",
	"ai",
	"energy",
	"culture",
	"succession",
	"governance",
}

var briefNumberPattern = regexp.MustCompile(`\b\d+(?:\.\d+)?%?|\$\d+(?:\.\d+)?\s*(?:billion|million)?`)

func stageBrief(state graphState) graphState {
	if !isReaderInterestSummaryForm(state.ArticleForm) {
		return state
	}
	if len(state.Ledger.Items) == 0 {
		state = stageLedger(state)
	}
	state.Brief = buildBriefFromLedger(state.Ledger, state.ArticleForm)
	return state
}

func buildBriefItems(units []SemanticUnit, limit int) []BriefItem {
	return buildBriefFromLedgerWithLimit(buildLedger(graphState{SemanticUnits: units}), "", limit)
}

func buildBriefFromLedger(ledger Ledger, articleForm string) []BriefItem {
	return buildBriefFromLedgerWithLimit(ledger, articleForm, briefDefaultLimit)
}

func buildBriefFromLedgerWithLimit(ledger Ledger, articleForm string, limit int) []BriefItem {
	if len(ledger.Items) == 0 || limit <= 0 {
		return nil
	}
	ranked := rankLedgerItems(ledger.Items)
	out := make([]BriefItem, 0, limit)
	counts := map[string]int{}

	if isReaderInterestSummaryForm(articleForm) {
		for _, category := range mandatoryMeetingBriefCategories {
			item, ok := bestLedgerItemForCategory(ranked, category, counts)
			if !ok {
				continue
			}
			out = appendBriefItem(out, item)
			counts[item.Category]++
			if len(out) == limit {
				return out
			}
		}
	}

	for _, ledgerItem := range ranked {
		item := briefItemFromLedgerItem(ledgerItem)
		if item.Category == "" || item.Claim == "" || counts[item.Category] >= briefCategoryLimit || containsBriefSource(out, item.SourceIDs) {
			continue
		}
		counts[item.Category]++
		out = appendBriefItem(out, item)
		if len(out) == limit {
			break
		}
	}
	return out
}

func appendBriefItem(items []BriefItem, item BriefItem) []BriefItem {
	item.ID = fmt.Sprintf("brief-%03d", len(items)+1)
	return append(items, item)
}

func bestLedgerItemForCategory(items []LedgerItem, category string, counts map[string]int) (BriefItem, bool) {
	for _, item := range items {
		if item.Category != category || counts[item.Category] >= briefCategoryLimit {
			continue
		}
		return briefItemFromLedgerItem(item), true
	}
	return BriefItem{}, false
}

func briefItemFromLedgerItem(item LedgerItem) BriefItem {
	kind := strings.TrimSpace(item.Kind)
	if kind == "" {
		kind = "point"
	}
	return BriefItem{
		Category:  strings.TrimSpace(item.Category),
		Kind:      kind,
		Claim:     strings.TrimSpace(item.Claim),
		Entities:  append([]string(nil), item.Entities...),
		Numbers:   append([]string(nil), item.Numbers...),
		Quote:     strings.TrimSpace(item.Quote),
		Salience:  item.Salience,
		SourceIDs: append([]string(nil), item.SourceIDs...),
	}
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

func rankLedgerItems(items []LedgerItem) []LedgerItem {
	out := append([]LedgerItem(nil), items...)
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Kind == "list" && out[j].Kind != "list" {
			return true
		}
		if out[i].Kind != "list" && out[j].Kind == "list" {
			return false
		}
		return out[i].Salience > out[j].Salience
	})
	return out
}

func containsBriefSource(items []BriefItem, sourceIDs []string) bool {
	for _, sourceID := range sourceIDs {
		sourceID = strings.TrimSpace(sourceID)
		if sourceID == "" {
			continue
		}
		for _, item := range items {
			for _, existing := range item.SourceIDs {
				if existing == sourceID {
					return true
				}
			}
		}
	}
	return false
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
		strings.Contains(text, "ai目前") ||
		strings.Contains(text, "ai基建") ||
		strings.Contains(text, "ai资本") ||
		strings.Contains(text, "ai革命") ||
		strings.Contains(text, "ai工具") ||
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
