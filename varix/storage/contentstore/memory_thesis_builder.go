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
	now = normalizeNow(now)

	byID := make(map[string]memory.AcceptedNode, len(nodes))
	nodeIDs := make([]string, 0, len(nodes))
	sourceCounts := map[string]int{}
	sourceConclusionCounts := map[string]int{}
	for _, node := range nodes {
		ref := globalMemoryNodeRef(node)
		node.NodeID = ref
		byID[ref] = node
		nodeIDs = append(nodeIDs, ref)
		sourceKey := thesisSourceRef(node)
		sourceCounts[sourceKey]++
		if node.NodeKind == string(compile.NodeConclusion) {
			sourceConclusionCounts[sourceKey]++
		}
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
				sourceKey := thesisSourceRef(left)
				if sameSourceCausalPairAllowed(left, right, sourceCounts[sourceKey], sourceConclusionCounts[sourceKey]) {
					addEdge(left.NodeID, right.NodeID)
					continue
				}
			}
			if !sameGlobalClusterFamily(left, right) && sharedObjectLabel(left.NodeText, right.NodeText) != "" {
				addEdge(left.NodeID, right.NodeID)
				continue
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
			if nonEmptySemanticPhrase(left.NodeText, right.NodeText) != "" {
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
		_, sourceRefs := collectAcceptedNodes(component, byID)
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

func sameSourceCausalPairAllowed(left, right memory.AcceptedNode, sourceCount int, sourceConclusionCount int) bool {
	if !sameSourceStructuralPairAllowed(left.NodeKind, right.NodeKind) {
		return false
	}
	if sourceCount <= 4 {
		return true
	}
	if sourceCount <= 8 && sourceConclusionCount <= 1 {
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
	if nonEmptySemanticPhrase(left.NodeText, right.NodeText) != "" {
		return true
	}
	return false
}

func sameSourceStructuralPairAllowed(leftKind, rightKind string) bool {
	if hierarchyTransitionAllowed(leftKind, rightKind) || hierarchyTransitionAllowed(rightKind, leftKind) {
		return true
	}
	switch {
	case leftKind == string(compile.NodeExplicitCondition) && rightKind == string(compile.NodeConclusion):
		return true
	case rightKind == string(compile.NodeExplicitCondition) && leftKind == string(compile.NodeConclusion):
		return true
	case leftKind == string(compile.NodeImplicitCondition) && rightKind == string(compile.NodeConclusion):
		return true
	case rightKind == string(compile.NodeImplicitCondition) && leftKind == string(compile.NodeConclusion):
		return true
	default:
		return false
	}
}

func thesisTopicLabel(component []string, byID map[string]memory.AcceptedNode) string {
	if len(component) == 0 {
		return ""
	}
	if len(component) == 1 {
		return byID[component[0]].NodeText
	}
	if label := aggregateTopicLabel(component, byID); label != "" {
		return label
	}
	left := byID[component[0]].NodeText
	right := byID[component[1]].NodeText
	if label := sharedObjectLabel(left, right); label != "" {
		return label
	}
	if phrase := nonEmptySemanticPhrase(left, right); phrase != "" {
		return fmt.Sprintf("关于「%s」的判断", phrase)
	}
	if label := factConclusionTopicLabel(component, byID); label != "" {
		return label
	}
	return byID[component[0]].NodeText
}

func aggregateTopicLabel(component []string, byID map[string]memory.AcceptedNode) string {
	texts := make([]string, 0, len(component))
	for _, id := range component {
		if node, ok := byID[id]; ok {
			texts = append(texts, canonicalNodeText(node.NodeText))
		}
	}
	all := strings.Join(texts, "\n")
	switch {
	case strings.Contains(all, "石油美元") && strings.Contains(all, "私募信贷") && containsAnyText(all, "流动性隐患", "流动性风险", "流动性脆弱"):
		return "关于「石油美元与私募信贷流动性风险」的判断"
	case strings.Contains(all, "流动性收紧") && strings.Contains(all, "风险资产承压"):
		return "关于「流动性收紧与风险资产承压」的判断"
	default:
		return ""
	}
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
			if nonEmptySemanticPhrase(left.NodeText, right.NodeText) != "" {
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
