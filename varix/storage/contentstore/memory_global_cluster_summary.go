package contentstore

import (
	"strings"

	"github.com/kumaloha/VariX/varix/memory"
	"github.com/kumaloha/VariX/varix/model"
)

func deriveContradictionProposition(conflicting []string, byID map[string]memory.AcceptedNode) (string, bool) {
	if len(conflicting) < 2 {
		return "", false
	}
	left := canonicalNodeText(strings.TrimSpace(byID[conflicting[0]].NodeText))
	right := canonicalNodeText(strings.TrimSpace(byID[conflicting[1]].NodeText))
	if left == "" || right == "" {
		return "", false
	}
	phrase := longestCommonSubstring([]rune(left), []rune(right))
	phrase = strings.TrimSpace(phrase)
	if len([]rune(phrase)) < 2 {
		phrase = truncateText(left, 20)
	}
	if phrase == "" {
		return "", false
	}
	return "关于「" + phrase + "」的判断", true
}
func chooseRepresentativeNode(component []string, byID map[string]memory.AcceptedNode) string {
	bestID := ""
	bestRank := 999
	bestLen := -1
	for _, id := range component {
		node := byID[id]
		rank := representativeRank(node.NodeKind)
		length := len([]rune(strings.TrimSpace(node.NodeText)))
		if rank < bestRank || (rank == bestRank && length > bestLen) || (rank == bestRank && length == bestLen && id < bestID) {
			bestID = id
			bestRank = rank
			bestLen = length
		}
	}
	return bestID
}
func representativeRank(kind string) int {
	switch kind {
	case string(model.NodeConclusion):
		return 0
	case string(model.NodeFact):
		return 1
	case string(model.NodeImplicitCondition):
		return 2
	case string(model.NodeExplicitCondition):
		return 3
	case string(model.NodePrediction):
		return 4
	default:
		return 99
	}
}
func buildCanonicalProposition(component []string, byID map[string]memory.AcceptedNode) string {
	if proposition, ok := deriveMacroThemeProposition(component, byID); ok {
		return proposition
	}
	if proposition, ok := deriveNeutralProposition(component, byID); ok {
		return proposition
	}
	repID := chooseRepresentativeNode(component, byID)
	text := normalizedClusterText(byID[repID].NodeText)
	if text == "" {
		return "未命名认知簇"
	}
	return truncateText(text, 80)
}
func deriveMacroThemeProposition(component []string, byID map[string]memory.AcceptedNode) (string, bool) {
	texts := make([]string, 0, len(component))
	for _, id := range component {
		if text := strings.TrimSpace(byID[id].NodeText); text != "" {
			texts = append(texts, canonicalNodeText(text))
		}
	}
	if len(texts) < 2 {
		return "", false
	}
	hasAny := func(needles ...string) bool {
		for _, text := range texts {
			for _, needle := range needles {
				if strings.Contains(text, needle) {
					return true
				}
			}
		}
		return false
	}
	switch {
	case hasAny("银行去监管", "去监管", "金融体系安全", "银行体系更安全"):
		return "关于「银行监管与金融系统安全」的判断", true
	case hasAny("石油美元", "油价", "霍尔木兹", "伊朗", "中东") && hasAny("流动性", "挤兑", "私募信贷", "美债", "美股", "供应链", "大宗商品"):
		return "关于「石油美元、油价与流动性风险」的判断", true
	case hasAny("债务", "金融资产", "货币", "央行", "资产价格", "利率") && hasAny("购买力", "贬值", "通胀", "回报", "脆弱性"):
		return "关于「债务周期与金融资产实际回报」的判断", true
	case hasAny("黑天鹅", "挤兑", "系统性", "危机") && hasAny("华尔街", "金融市场", "联储", "qe", "量化宽松"):
		return "关于「系统性金融风险与政策应对」的判断", true
	default:
		return "", false
	}
}
func deriveNeutralProposition(component []string, byID map[string]memory.AcceptedNode) (string, bool) {
	if len(component) < 2 {
		return "", false
	}
	var canonicals []string
	for _, id := range component {
		text := normalizedClusterText(byID[id].NodeText)
		if text == "" {
			continue
		}
		canonical := canonicalNodeText(text)
		if canonical != "" {
			canonicals = append(canonicals, canonical)
		}
	}
	if len(canonicals) < 2 {
		return "", false
	}
	prefix := commonPrefix(canonicals)
	prefix = strings.TrimSpace(prefix)
	if len([]rune(prefix)) < 2 {
		return "", false
	}
	return "关于「" + truncateText(prefix, 40) + "」的判断", true
}
func buildClusterSummary(canonical string, supporting, conflicting, conditional, predictive []string, byID map[string]memory.AcceptedNode) string {
	parts := make([]string, 0, 4)
	if trimmed := strings.TrimSpace(canonical); trimmed != "" {
		parts = append(parts, trimmed)
	}
	if text := summarizeRoleTexts(supporting, byID); text != "" {
		parts = append(parts, "支持信息包括"+text)
	}
	if text := summarizeRoleTexts(conditional, byID); text != "" {
		parts = append(parts, "条件包括"+text)
	}
	if text := summarizeRoleTexts(predictive, byID); text != "" {
		parts = append(parts, "相关预测包括"+text)
	}
	if text := summarizeRoleTexts(conflicting, byID); text != "" {
		parts = append(parts, "但也存在冲突观点："+text)
	}
	if len(parts) == 0 {
		return "未命名认知簇"
	}
	return truncateText(strings.Join(parts, "；"), 180)
}
func summarizeRoleTexts(ids []string, byID map[string]memory.AcceptedNode) string {
	if len(ids) == 0 {
		return ""
	}
	items := make([]string, 0, 2)
	for _, id := range ids {
		text := normalizedClusterText(byID[id].NodeText)
		if text == "" {
			continue
		}
		items = append(items, truncateText(text, 40))
		if len(items) == 2 {
			break
		}
	}
	return strings.Join(items, "；")
}
