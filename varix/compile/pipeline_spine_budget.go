package compile

import (
	"sort"
	"strings"
)

func enforceSpineBudget(spines []PreviewSpine, valid map[string]graphNode, policy spinePolicy) []PreviewSpine {
	if policy.MaxSpines <= 0 || len(spines) <= policy.MaxSpines || policy.PreserveRiskFamilies {
		return renumberSpinePriorities(spines)
	}
	primary := make([]int, 0, 1)
	candidates := make([]int, 0, len(spines))
	for i, spine := range spines {
		if spine.Level == "primary" {
			primary = append(primary, i)
			continue
		}
		candidates = append(candidates, i)
	}
	keep := map[int]struct{}{}
	for _, index := range primary {
		keep[index] = struct{}{}
	}
	remaining := policy.MaxSpines - len(keep)
	if remaining < 0 {
		remaining = 0
	}
	type scoredSpine struct {
		index int
		score float64
	}
	primaryText := spineTextForScoring(primarySpine(spines), valid)
	scored := make([]scoredSpine, 0, len(candidates))
	for _, index := range candidates {
		scored = append(scored, scoredSpine{
			index: index,
			score: summarySpineScore(spines[index], valid, primaryText, policy),
		})
	}
	sort.SliceStable(scored, func(i, j int) bool {
		if scored[i].score != scored[j].score {
			return scored[i].score > scored[j].score
		}
		return spines[scored[i].index].Priority < spines[scored[j].index].Priority
	})
	for i := 0; i < remaining && i < len(scored); i++ {
		keep[scored[i].index] = struct{}{}
	}
	out := make([]PreviewSpine, 0, len(keep))
	for i, spine := range spines {
		if _, ok := keep[i]; ok {
			out = append(out, spine)
		}
	}
	return renumberSpinePriorities(out)
}
func primarySpine(spines []PreviewSpine) PreviewSpine {
	for _, spine := range spines {
		if spine.Level == "primary" {
			return spine
		}
	}
	return PreviewSpine{}
}
func summarySpineScore(spine PreviewSpine, valid map[string]graphNode, primaryText string, policy spinePolicy) float64 {
	score := 100.0 - float64(spine.Priority)*2.5
	score += float64(len(spine.Edges)) * 4
	score += float64(len(spine.NodeIDs)) * 1.25
	switch spine.Level {
	case "branch":
		score += 4
	case "local":
		score -= 8
	}
	if spineHasDiscourseRole(spine, valid, "thesis") {
		score += 8
	}
	if spineHasDiscourseRole(spine, valid, "market_move", "implication") {
		score += 6
	}
	if spineHasDiscourseRole(spine, valid, "mechanism") {
		score += 3
	}
	text := spineTextForScoring(spine, valid)
	if policy.ArticleForm == "macro_framework" {
		if summaryTextLooksLocalBehavior(text) {
			score -= 24
		}
		if summaryTextRepeatsPrimaryFamily(text, primaryText) && spine.Priority > 2 {
			score -= 18
		}
	}
	if policy.ArticleForm == "evidence_backed_forecast" {
		if forecastSpineLooksLikeLightSideCaveat(text) {
			score -= 28
		}
		if containsAnyText(text, []string{"货币政策", "monetary policy", "利率", "实际利率", "降息", "美联储", "fed", "financial repression", "金融抑制", "金融压抑"}) {
			score += 12
		}
	}
	return score
}
func forecastSpineLooksLikeLightSideCaveat(text string) bool {
	return containsAnyText(text, []string{"ai", "人工智能"}) &&
		containsAnyText(text, []string{"通胀", "inflation", "反通胀", "disinflation", "deflation"})
}
func spineTextForScoring(spine PreviewSpine, valid map[string]graphNode) string {
	parts := []string{spine.Thesis}
	for _, id := range spine.NodeIDs {
		if node, ok := valid[id]; ok {
			parts = append(parts, node.Text)
		}
	}
	for _, edge := range spine.Edges {
		parts = append(parts, edge.SourceQuote, edge.Reason)
	}
	return strings.ToLower(strings.Join(parts, " "))
}
func summaryTextLooksLocalBehavior(text string) bool {
	return containsAnyText(text, []string{
		"emotional trading", "underperform", "investor behavior", "sentiment", "心理", "情绪", "行为",
	})
}
func summaryTextRepeatsPrimaryFamily(text, primaryText string) bool {
	if strings.TrimSpace(text) == "" || strings.TrimSpace(primaryText) == "" {
		return false
	}
	overlap := 0
	for _, family := range macroSummaryAnchorFamilies() {
		if containsAnyText(primaryText, family) && containsAnyText(text, family) {
			overlap++
		}
	}
	return overlap >= 2
}
func macroSummaryAnchorFamilies() [][]string {
	return [][]string{
		{"debt", "债务"},
		{"credit", "信贷", "信用"},
		{"promise", "promises", "承诺", "欠条"},
		{"crisis", "default", "crash", "depression", "危机", "违约", "崩盘"},
		{"money printing", "printed", "货币印刷", "印钱"},
		{"currency devaluation", "devaluation", "贬值"},
		{"financial wealth", "金融财富"},
		{"real wealth", "tangible wealth", "实际财富", "有形财富"},
	}
}
