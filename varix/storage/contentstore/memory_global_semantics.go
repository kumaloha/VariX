package contentstore

import (
	"strings"
	"time"

	"github.com/kumaloha/VariX/varix/compile"
	"github.com/kumaloha/VariX/varix/memory"
)

func sameGlobalClusterFamily(left, right memory.AcceptedNode) bool {
	bucket := func(kind string) string {
		switch kind {
		case string(compile.NodeFact), string(compile.NodeConclusion), string(compile.NodePrediction):
			return "claim"
		case string(compile.NodeExplicitCondition), string(compile.NodeImplicitCondition):
			return "condition"
		default:
			return "other"
		}
	}
	return bucket(left.NodeKind) == bucket(right.NodeKind)
}
func sharedSemanticPhrase(a, b string) (string, bool) {
	a = canonicalNodeText(a)
	b = canonicalNodeText(b)
	if a == "" || b == "" {
		return "", false
	}
	phrase := longestCommonSubstring([]rune(a), []rune(b))
	phrase = strings.TrimSpace(phrase)
	if len([]rune(phrase)) < 4 {
		return "", false
	}
	for _, banned := range []string{"金融资产", "投资者", "周期", "风险", "资产", "市场", "美国"} {
		if phrase == banned {
			return "", false
		}
	}
	return phrase, true
}
func sharedMacroTheme(a, b string) string {
	left := macroThemeKey(a)
	right := macroThemeKey(b)
	if left == "" || right == "" {
		return ""
	}
	if left == right {
		return left
	}
	return ""
}
func macroThemeKey(text string) string {
	text = canonicalNodeText(text)
	switch {
	case containsAnyText(text, "银行去监管", "去监管", "金融体系安全", "银行体系更安全", "支持经济增长"):
		return "macro-bank-regulation"
	case containsAnyText(text, "石油美元", "油价", "霍尔木兹", "私募信贷", "流动性", "挤兑", "美债", "美股", "华尔街", "伊朗", "中东", "大宗商品", "供应链"):
		return "macro-liquidity"
	case containsAnyText(text, "债务", "金融资产", "货币", "央行", "通胀", "购买力", "回报", "资产价格", "利率", "脆弱性"):
		return "macro-debt"
	case containsAnyText(text, "能源短缺", "生活成本", "k型"):
		return "macro-supply-shock"
	default:
		return ""
	}
}
func globalMemoryNodeRef(node memory.AcceptedNode) string {
	return node.SourcePlatform + ":" + node.SourceExternalID + ":" + node.NodeID
}
func normalizedClusterText(text string) string {
	text = strings.TrimSpace(text)
	for _, prefix := range []string{"若", "如果", "一旦", "假如"} {
		text = strings.TrimPrefix(text, prefix)
	}
	return strings.TrimSpace(text)
}
func globalizeAcceptedNodes(nodes []memory.AcceptedNode) []memory.AcceptedNode {
	globalNodes := make([]memory.AcceptedNode, 0, len(nodes))
	for _, node := range nodes {
		node.NodeID = globalMemoryNodeRef(node)
		globalNodes = append(globalNodes, node)
	}
	return globalNodes
}
func splitAcceptedNodesByActivity(nodes []memory.AcceptedNode, now time.Time) ([]memory.AcceptedNode, []memory.AcceptedNode) {
	active := make([]memory.AcceptedNode, 0, len(nodes))
	inactive := make([]memory.AcceptedNode, 0, len(nodes))
	for _, node := range nodes {
		if isAcceptedNodeActiveAt(node, now) {
			active = append(active, node)
		} else {
			inactive = append(inactive, node)
		}
	}
	return active, inactive
}
