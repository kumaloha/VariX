package compile

import (
	"sort"
	"strings"
)

func enrichLeadMarketUpdateSpine(spines []PreviewSpine, valid map[string]graphNode, policy spinePolicy) []PreviewSpine {
	if policy.ArticleForm != "market_update" || len(spines) == 0 || len(valid) == 0 {
		return spines
	}
	index := leadMarketUpdateSpineIndex(spines)
	if index < 0 {
		return spines
	}
	hubID, families := leadMarketUpdateHub(spines[index], valid)
	if hubID == "" || len(families) == 0 {
		return spines
	}
	used := usedSpineNodeIDs(spines)
	ids := make([]string, 0, len(valid))
	for id := range valid {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	added := 0
	for _, id := range ids {
		if _, ok := used[id]; ok {
			continue
		}
		node := valid[id]
		if normalizeDiscourseRole(node.DiscourseRole) != "market_move" {
			continue
		}
		if !marketMoveMatchesFamilies(node, families) {
			continue
		}
		spines[index].NodeIDs = append(spines[index].NodeIDs, id)
		spines[index].Edges = append(spines[index].Edges, PreviewEdge{
			From:        hubID,
			To:          id,
			Kind:        "causal",
			SourceQuote: strings.Join(nonEmptyStrings(valid[hubID].SourceQuote, node.SourceQuote), " / "),
			Reason:      "market_update lead branch sibling market move",
		})
		added++
		if added == 3 {
			break
		}
	}
	return spines
}

func leadMarketUpdateSpineIndex(spines []PreviewSpine) int {
	for i, spine := range spines {
		if spine.Level == "primary" && normalizePreviewSpinePolicy(spine.Policy) == "market_update" {
			return i
		}
	}
	for i, spine := range spines {
		if spine.Level == "primary" {
			return i
		}
	}
	return -1
}

func leadMarketUpdateHub(spine PreviewSpine, valid map[string]graphNode) (string, map[string]struct{}) {
	nodeSet := map[string]struct{}{}
	for _, id := range spine.NodeIDs {
		if strings.TrimSpace(id) != "" {
			nodeSet[id] = struct{}{}
		}
	}
	type hub struct {
		id       string
		count    int
		families map[string]struct{}
	}
	hubs := map[string]hub{}
	for _, edge := range spine.Edges {
		if _, ok := nodeSet[edge.From]; !ok {
			continue
		}
		if _, ok := nodeSet[edge.To]; !ok {
			continue
		}
		target, ok := valid[edge.To]
		if !ok || normalizeDiscourseRole(target.DiscourseRole) != "market_move" {
			continue
		}
		families := marketMoveFamilies(target)
		if len(families) == 0 {
			continue
		}
		item := hubs[edge.From]
		item.id = edge.From
		item.count++
		if item.families == nil {
			item.families = map[string]struct{}{}
		}
		for family := range families {
			item.families[family] = struct{}{}
		}
		hubs[edge.From] = item
	}
	best := hub{}
	for _, id := range spine.NodeIDs {
		item := hubs[id]
		if item.count < 2 {
			continue
		}
		if item.count > best.count {
			best = item
		}
	}
	return best.id, best.families
}

func usedSpineNodeIDs(spines []PreviewSpine) map[string]struct{} {
	used := map[string]struct{}{}
	for _, spine := range spines {
		for _, id := range spine.NodeIDs {
			id = strings.TrimSpace(id)
			if id != "" {
				used[id] = struct{}{}
			}
		}
	}
	return used
}

func marketMoveMatchesFamilies(node graphNode, families map[string]struct{}) bool {
	for family := range marketMoveFamilies(node) {
		if _, ok := families[family]; ok {
			return true
		}
	}
	return false
}

func marketMoveFamilies(node graphNode) map[string]struct{} {
	text := strings.ToLower(strings.Join(nonEmptyStrings(node.Text, node.SourceQuote), " "))
	families := map[string]struct{}{}
	if containsAnyText(text, []string{"s&p", "sp500", "nasdaq", "equity", "stock", "股票", "美股", "韩股", "台股", "芯片"}) {
		families["equity"] = struct{}{}
	}
	if containsAnyText(text, []string{"etf", "fund flow", "inflow", "outflow", "资金", "流入", "流出"}) {
		families["flow"] = struct{}{}
	}
	if containsAnyText(text, []string{"treasury", "bond", "yield", "美债", "国债", "收益率"}) {
		families["rates"] = struct{}{}
	}
	if containsAnyText(text, []string{"dollar", "yen", "fx", "dxy", "美元", "日元", "汇率"}) {
		families["fx"] = struct{}{}
	}
	if containsAnyText(text, []string{"oil", "brent", "gold", "silver", "commodity", "原油", "布伦特", "黄金", "白银", "商品"}) {
		families["commodity"] = struct{}{}
	}
	if containsAnyText(text, []string{"vix", "volatility", "恐慌", "波动率"}) {
		families["volatility"] = struct{}{}
	}
	return families
}

func mergeSpineEdgesIntoGraph(edges []graphEdge, spines []PreviewSpine, valid map[string]graphNode) []graphEdge {
	if len(spines) == 0 {
		return edges
	}
	out := append([]graphEdge(nil), edges...)
	seen := map[string]struct{}{}
	for _, edge := range out {
		seen[edge.From+"->"+edge.To] = struct{}{}
	}
	for _, spine := range spines {
		for _, edge := range spine.Edges {
			from := strings.TrimSpace(edge.From)
			to := strings.TrimSpace(edge.To)
			if from == "" || to == "" || from == to {
				continue
			}
			if _, ok := valid[from]; !ok {
				continue
			}
			if _, ok := valid[to]; !ok {
				continue
			}
			key := from + "->" + to
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			out = append(out, graphEdge{
				From:        from,
				To:          to,
				Kind:        normalizeMainlineRelationKind(edge.Kind),
				SourceQuote: strings.TrimSpace(edge.SourceQuote),
				Reason:      strings.TrimSpace(edge.Reason),
			})
		}
	}
	return out
}
