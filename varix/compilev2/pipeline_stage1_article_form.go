package compilev2

import (
	"github.com/kumaloha/VariX/varix/compile"
	"strings"
)

func normalizeArticleForm(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "single_thesis", "main_narrative_plus_investment_implication", "evidence_backed_forecast", "risk_list", "macro_framework", "market_update", "institutional_satire", "satirical_financial_commentary":
		return strings.ToLower(strings.TrimSpace(value))
	default:
		return ""
	}
}

func refineArticleFormFromExtract(bundle compile.Bundle, state graphState) string {
	form := normalizeArticleForm(state.ArticleForm)
	if form != "" && form != "main_narrative_plus_investment_implication" {
		return form
	}
	if satireArticleScore(bundle.TextContext(), state.Nodes) >= 5 {
		return "satirical_financial_commentary"
	}
	if !isLongFormMacroSource(bundle) {
		return form
	}
	if evidenceBackedForecastScore(bundle.TextContext(), state.Nodes) >= 4 {
		return "evidence_backed_forecast"
	}
	if longFormMacroFrameworkScore(bundle.TextContext(), state.Nodes) < 4 {
		return form
	}
	return "macro_framework"
}

func satireArticleScore(article string, nodes []graphNode) int {
	textParts := []string{article}
	analogyRoles := 0
	satireTargetRoles := 0
	for _, node := range nodes {
		textParts = append(textParts, node.Text, node.SourceQuote)
		switch normalizeDiscourseRole(node.DiscourseRole) {
		case "analogy":
			analogyRoles++
		case "satire_target", "implied_thesis":
			satireTargetRoles++
		}
	}
	score := 0
	if analogyRoles > 0 {
		score += 2
	}
	if satireTargetRoles > 0 {
		score += 3
	}
	text := strings.ToLower(strings.Join(textParts, " "))
	for _, family := range [][]string{
		{"讽刺", "satire", "satirical", "irony"},
		{"寓言", "类比", "故事", "analogy", "allegory"},
		{"村长", "新富", "幸运游戏", "抽奖", "幸运观众"},
		{"叙事", "包装成公平", "包装", "忽悠", "牌照"},
	} {
		if containsAnyText(text, family) {
			score++
		}
	}
	return score
}

func evidenceBackedForecastScore(article string, nodes []graphNode) int {
	textParts := []string{article}
	for _, node := range nodes {
		textParts = append(textParts, node.Text, node.SourceQuote)
	}
	text := strings.ToLower(strings.Join(textParts, " "))
	score := 0
	for _, family := range [][]string{
		{"推断", "推导", "可能", "如果", "would", "could", "likely", "probability", "forecast"},
		{"调研", "研究", "证据", "历史", "precedent", "evidence", "research"},
		{"沃什", "warsh"},
		{"美联储", "fed", "federal reserve"},
		{"金融抑制", "金融压抑", "financial repression"},
	} {
		if containsAnyText(text, family) {
			score++
		}
	}
	return score
}

func isLongFormMacroSource(bundle compile.Bundle) bool {
	switch strings.ToLower(strings.TrimSpace(bundle.Source)) {
	case "youtube":
		return true
	default:
		return false
	}
}

func longFormMacroFrameworkScore(article string, nodes []graphNode) int {
	textParts := []string{article}
	for _, node := range nodes {
		textParts = append(textParts, node.Text, node.SourceQuote)
	}
	text := strings.ToLower(strings.Join(textParts, " "))
	score := 0
	for _, family := range longFormMacroFrameworkFamilies() {
		if containsAnyText(text, family) {
			score++
		}
	}
	return score
}

func longFormMacroFrameworkFamilies() [][]string {
	return [][]string{
		{"法币", "fiat"},
		{"信用", "credit"},
		{"债务", "debt"},
		{"人口老龄化", "老龄化", "demographic", "aging"},
		{"税基", "tax base"},
		{"主权债", "主权债务", "sovereign debt"},
		{"金融压抑", "financial repression"},
		{"美元信用", "美元单核", "dollar hegemony"},
		{"outside money", "外部货币", "实物商品"},
	}
}

func normalizeDiscourseRole(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "thesis", "mechanism", "evidence", "example", "implication", "caveat", "market_move", "analogy", "satire_target", "implied_thesis":
		return strings.ToLower(strings.TrimSpace(value))
	default:
		return ""
	}
}
