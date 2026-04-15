package contentstore

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/kumaloha/VariX/varix/compile"
	"github.com/kumaloha/VariX/varix/memory"
)

func buildCandidateTheses(nodes []memory.AcceptedNode, now time.Time) []memory.CandidateThesis {
	if len(nodes) == 0 {
		return nil
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}

	byID := make(map[string]memory.AcceptedNode, len(nodes))
	nodeIDs := make([]string, 0, len(nodes))
	sourceCounts := map[string]int{}
	for _, node := range nodes {
		ref := globalMemoryNodeRef(node)
		node.NodeID = ref
		byID[ref] = node
		nodeIDs = append(nodeIDs, ref)
		sourceCounts[node.SourcePlatform+":"+node.SourceExternalID]++
	}
	sort.Strings(nodeIDs)

	adj := map[string]map[string]struct{}{}
	addEdge := func(a, b string) {
		if strings.TrimSpace(a) == "" || strings.TrimSpace(b) == "" || a == b {
			return
		}
		if adj[a] == nil {
			adj[a] = map[string]struct{}{}
		}
		if adj[b] == nil {
			adj[b] = map[string]struct{}{}
		}
		adj[a][b] = struct{}{}
		adj[b][a] = struct{}{}
	}

	for i := 0; i < len(nodeIDs); i++ {
		for j := i + 1; j < len(nodeIDs); j++ {
			left := byID[nodeIDs[i]]
			right := byID[nodeIDs[j]]
			if left.SourcePlatform == right.SourcePlatform && left.SourceExternalID == right.SourceExternalID {
				if sameSourceCausalPairAllowed(left, right, sourceCounts[left.SourcePlatform+":"+left.SourceExternalID]) {
					addEdge(left.NodeID, right.NodeID)
					continue
				}
			}
			if !sameGlobalClusterFamily(left, right) {
				continue
			}
			if _, ok := contradictionReason(left.NodeText, right.NodeText); ok {
				addEdge(left.NodeID, right.NodeID)
				continue
			}
			if sharedMechanismTheme(left.NodeText, right.NodeText) {
				addEdge(left.NodeID, right.NodeID)
				continue
			}
			if phrase, ok := sharedSemanticPhrase(left.NodeText, right.NodeText); ok && strings.TrimSpace(phrase) != "" {
				addEdge(left.NodeID, right.NodeID)
			}
		}
	}

	seen := map[string]struct{}{}
	out := make([]memory.CandidateThesis, 0, len(nodeIDs))
	for _, start := range nodeIDs {
		if _, ok := seen[start]; ok {
			continue
		}
		component := collectComponent(start, adj, seen)
		sort.Strings(component)
		sourceRefs := make([]string, 0, len(component))
		for _, id := range component {
			node := byID[id]
			sourceRefs = append(sourceRefs, fmt.Sprintf("%s:%s", node.SourcePlatform, node.SourceExternalID))
		}
		out = append(out, memory.CandidateThesis{
			ThesisID:      fmt.Sprintf("thesis-%d", len(out)+1),
			UserID:        byID[start].UserID,
			TopicLabel:    thesisTopicLabel(component, byID),
			NodeIDs:       component,
			SourceRefs:    uniqueStrings(sourceRefs),
			ClusterReason: thesisClusterReason(component, byID),
			CoverageScore: float64(len(component)),
			CreatedAt:     now,
			UpdatedAt:     now,
		})
	}

	sort.Slice(out, func(i, j int) bool { return out[i].ThesisID < out[j].ThesisID })
	return out
}

func sameSourceCausalPairAllowed(left, right memory.AcceptedNode, sourceCount int) bool {
	if !(hierarchyTransitionAllowed(left.NodeKind, right.NodeKind) || hierarchyTransitionAllowed(right.NodeKind, left.NodeKind)) {
		return false
	}
	if sourceCount <= 4 {
		return true
	}
	if _, ok := contradictionReason(left.NodeText, right.NodeText); ok {
		return true
	}
	if sharedMechanismTheme(left.NodeText, right.NodeText) {
		return true
	}
	if sharedObjectLabel(left.NodeText, right.NodeText) != "" {
		return true
	}
	if phrase, ok := sharedSemanticPhrase(left.NodeText, right.NodeText); ok && strings.TrimSpace(phrase) != "" {
		return true
	}
	return false
}

func thesisTopicLabel(component []string, byID map[string]memory.AcceptedNode) string {
	if len(component) == 0 {
		return ""
	}
	if len(component) == 1 {
		return byID[component[0]].NodeText
	}
	left := byID[component[0]].NodeText
	right := byID[component[1]].NodeText
	if label := sharedObjectLabel(left, right); label != "" {
		return label
	}
	if phrase, ok := sharedSemanticPhrase(left, right); ok && strings.TrimSpace(phrase) != "" {
		return fmt.Sprintf("关于「%s」的判断", phrase)
	}
	if label := factConclusionTopicLabel(component, byID); label != "" {
		return label
	}
	return byID[component[0]].NodeText
}

func thesisClusterReason(component []string, byID map[string]memory.AcceptedNode) string {
	if len(component) < 2 {
		return "singleton"
	}
	for i := 0; i < len(component); i++ {
		left := byID[component[i]]
		for j := i + 1; j < len(component); j++ {
			right := byID[component[j]]
			if left.SourcePlatform == right.SourcePlatform && left.SourceExternalID == right.SourceExternalID {
				if hierarchyTransitionAllowed(left.NodeKind, right.NodeKind) || hierarchyTransitionAllowed(right.NodeKind, left.NodeKind) {
					return "same_source_causal_chain"
				}
			}
			if _, ok := contradictionReason(left.NodeText, right.NodeText); ok {
				return "contradiction_pair"
			}
			if sharedMechanismTheme(left.NodeText, right.NodeText) {
				return "shared_mechanism_theme"
			}
			if phrase, ok := sharedSemanticPhrase(left.NodeText, right.NodeText); ok && strings.TrimSpace(phrase) != "" {
				return "shared_semantic_phrase"
			}
		}
	}
	return "multi_node_group"
}

func uniqueStrings(in []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(in))
	for _, item := range in {
		if _, ok := seen[item]; ok {
			continue
		}
		seen[item] = struct{}{}
		out = append(out, item)
	}
	sort.Strings(out)
	return out
}

func sharedMechanismTheme(a, b string) bool {
	themes := []string{
		mechanismThemeKey(a),
		mechanismThemeKey(b),
	}
	return themes[0] != "" && themes[0] == themes[1]
}

func mechanismThemeKey(text string) string {
	text = canonicalNodeText(text)
	switch {
	case containsAnyText(text, "私募信贷", "流动性错配", "流动性脆弱", "流动性风险"):
		return "private-credit-liquidity"
	case containsAnyText(text, "银行去监管", "监管松绑", "去监管", "金融体系安全", "系统更安全"):
		return "bank-deregulation-safety"
	case containsAnyText(text, "债务", "实际回报", "购买力"):
		return "debt-real-return"
	default:
		return ""
	}
}

func sharedObjectLabel(a, b string) string {
	a = canonicalNodeText(a)
	b = canonicalNodeText(b)
	for _, needle := range []string{
		"油价",
		"石油美元",
		"私募信贷",
		"流动性",
		"银行去监管",
		"监管松绑",
		"金融体系安全",
		"风险资产",
		"债务",
	} {
		if strings.Contains(a, needle) && strings.Contains(b, needle) {
			return fmt.Sprintf("关于「%s」的判断", needle)
		}
	}
	return ""
}

func factConclusionTopicLabel(component []string, byID map[string]memory.AcceptedNode) string {
	var fact string
	var conclusion string
	for _, id := range component {
		node, ok := byID[id]
		if !ok {
			continue
		}
		switch node.NodeKind {
		case string(compile.NodeFact):
			if fact == "" {
				fact = strings.TrimSpace(node.NodeText)
			}
		case string(compile.NodeConclusion):
			if conclusion == "" {
				conclusion = strings.TrimSpace(node.NodeText)
			}
		}
	}
	if fact != "" && conclusion != "" {
		return fmt.Sprintf("关于「%s与%s」的判断", fact, conclusion)
	}
	return ""
}
