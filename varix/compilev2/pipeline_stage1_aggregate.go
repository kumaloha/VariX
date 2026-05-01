package compilev2

import (
	"fmt"
	"strings"
)

type aggregateResult struct {
	Aggregates []aggregatePatch `json:"aggregates"`
}

type aggregatePatch struct {
	Text        string   `json:"text"`
	MemberIDs   []string `json:"member_ids"`
	SourceQuote string   `json:"source_quote"`
	Reason      string   `json:"reason"`
}

func serializeAggregateCandidateGroups(nodes []graphNode) string {
	type group struct {
		quote     string
		nodes     []graphNode
		suggested string
	}
	groupsByQuote := map[string][]graphNode{}
	order := make([]string, 0)
	for _, node := range nodes {
		quote := strings.TrimSpace(node.SourceQuote)
		if quote == "" {
			continue
		}
		if _, ok := groupsByQuote[quote]; !ok {
			order = append(order, quote)
		}
		groupsByQuote[quote] = append(groupsByQuote[quote], node)
	}
	groups := make([]group, 0, len(order))
	for _, quote := range order {
		items := groupsByQuote[quote]
		if len(items) < 3 {
			continue
		}
		if !looksLikeAggregateCandidateQuote(quote) {
			continue
		}
		groups = append(groups, group{quote: quote, nodes: items, suggested: suggestAggregateLabel(items, quote)})
	}
	if len(groups) == 0 {
		return ""
	}
	var b strings.Builder
	for i, group := range groups {
		if i > 0 {
			b.WriteString("\n\n")
		}
		fmt.Fprintf(&b, "Group %d quote: %s\n", i+1, group.quote)
		if strings.TrimSpace(group.suggested) != "" {
			fmt.Fprintf(&b, "Suggested aggregate label: %s\n", group.suggested)
		}
		for _, node := range group.nodes {
			fmt.Fprintf(&b, "- %s: %s\n", node.ID, node.Text)
		}
	}
	return b.String()
}

func looksLikeAggregateCandidateQuote(quote string) bool {
	lower := strings.ToLower(strings.TrimSpace(quote))
	return containsAnyText(lower, []string{
		"、", "，", ",", "和", "以及", "等", "一连串", "全都", "统统", "所有", "各类", "多个", "both", "and", "as well as",
	})
}

func suggestAggregateLabel(nodes []graphNode, quote string) string {
	texts := make([]string, 0, len(nodes))
	for _, node := range nodes {
		texts = append(texts, strings.TrimSpace(node.Text))
	}
	switch {
	case countTextsContaining(texts, "价格被压低") >= 2 && aggregateQuoteContainsAny(quote, []string{"资产", "股票", "债券", "房产", "私募"}):
		return "资产价格被压低"
	case countTextsContaining(texts, "价格下跌") >= 2 && aggregateQuoteContainsAny(quote, []string{"资产", "股票", "债券", "房产", "私募"}):
		return "资产价格下跌"
	case countTextsContaining(texts, "价格承压") >= 2 && aggregateQuoteContainsAny(quote, []string{"资产", "股票", "债券", "房产", "私募"}):
		return "资产价格承压"
	case countTextsContaining(texts, "成本上升") >= 2:
		return "下游成本上升"
	case countTextsContaining(texts, "成本维持高位") >= 2 || countTextsContaining(texts, "成本难降") >= 2:
		return "融资成本维持高位"
	case countTextsContaining(texts, "受影响") >= 2:
		return "政府支出项目受影响"
	default:
		return ""
	}
}

func countTextsContaining(texts []string, marker string) int {
	count := 0
	for _, text := range texts {
		if strings.Contains(strings.ToLower(text), strings.ToLower(marker)) {
			count++
		}
	}
	return count
}

func aggregateQuoteContainsAny(quote string, markers []string) bool {
	return containsAnyText(strings.ToLower(quote), markers)
}

func applyAggregatePatches(state graphState, aggregates []aggregatePatch) graphState {
	if len(aggregates) == 0 {
		return state
	}
	valid := map[string]graphNode{}
	for _, node := range state.Nodes {
		valid[node.ID] = node
	}
	existingText := map[string]struct{}{}
	for _, node := range state.Nodes {
		existingText[normalizeText(node.Text)] = struct{}{}
	}
	nextIndex := 1
	for _, aggregate := range aggregates {
		text := strings.TrimSpace(aggregate.Text)
		if text == "" || containsAnyText(strings.ToLower(text), aggregateForbiddenMarkers()) {
			continue
		}
		memberIDs := dedupeStrings(aggregate.MemberIDs)
		if len(memberIDs) < 2 {
			continue
		}
		validMembers := make([]string, 0, len(memberIDs))
		for _, id := range memberIDs {
			if _, ok := valid[id]; ok {
				validMembers = append(validMembers, id)
			}
		}
		if len(validMembers) < 2 {
			continue
		}
		key := normalizeText(text)
		if _, exists := existingText[key]; exists {
			continue
		}
		id := fmt.Sprintf("agg_%d", nextIndex)
		nextIndex++
		state.Nodes = append(state.Nodes, graphNode{
			ID:            id,
			Text:          text,
			SourceQuote:   strings.TrimSpace(aggregate.SourceQuote),
			DiscourseRole: aggregateDiscourseRole(validMembers, valid),
		})
		existingText[key] = struct{}{}
		for _, memberID := range validMembers {
			state.AuxEdges = append(state.AuxEdges, auxEdge{
				From:        memberID,
				To:          id,
				Kind:        "supplementary",
				SourceQuote: strings.TrimSpace(aggregate.SourceQuote),
				Reason:      strings.TrimSpace(aggregate.Reason),
			})
		}
	}
	state.AuxEdges = dedupeAuxEdges(state.AuxEdges)
	state.BranchHeads = nil
	return state
}

func aggregateForbiddenMarkers() []string {
	return []string{"导致", "引发", "造成", "使", "影响", "推高", "推动", "压低", "拖累", "传导", "drive", "drives", "cause", "causes", "lead to", "leads to"}
}

func aggregateDiscourseRole(memberIDs []string, nodeIndex map[string]graphNode) string {
	best := ""
	bestScore := -1
	for _, id := range memberIDs {
		role := normalizeDiscourseRole(nodeIndex[id].DiscourseRole)
		score := discourseRolePriority(role)
		if score > bestScore {
			best = role
			bestScore = score
		}
	}
	if best == "" {
		return "mechanism"
	}
	return best
}
