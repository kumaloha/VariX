package compile

import (
	"strings"
)

func collectBranchMainlineNodes(edges []graphEdge, branchHeads []string) map[string]struct{} {
	keep := map[string]struct{}{}
	reverse := map[string][]string{}
	for _, edge := range edges {
		reverse[edge.To] = append(reverse[edge.To], edge.From)
	}
	stack := append([]string(nil), branchHeads...)
	for len(stack) > 0 {
		id := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if _, ok := keep[id]; ok {
			continue
		}
		keep[id] = struct{}{}
		stack = append(stack, reverse[id]...)
	}
	return keep
}

func chooseBranchAttachment(edges []graphEdge, nodeID string, keep map[string]struct{}, branchHeads []string) string {
	for _, edge := range edges {
		if edge.From == nodeID {
			if _, ok := keep[edge.To]; ok {
				return edge.To
			}
		}
	}
	for _, edge := range edges {
		if edge.To == nodeID {
			if _, ok := keep[edge.From]; ok {
				return edge.From
			}
		}
	}
	return ""
}

func dedupeStrings(values []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}

func filterNodesByRole(nodes []graphNode, role graphRole) []graphNode {
	out := make([]graphNode, 0)
	for _, n := range nodes {
		if n.Role == role {
			out = append(out, n)
		}
	}
	return out
}

func filterTargetNodes(nodes []graphNode) []graphNode {
	out := make([]graphNode, 0)
	for _, n := range nodes {
		if n.IsTarget {
			out = append(out, n)
		}
	}
	return out
}

func predecessorOf(edges []graphEdge, id string) string {
	for _, e := range edges {
		if e.To == id {
			return e.From
		}
	}
	return ""
}

func predecessorTexts(state graphState, id string) []string {
	out := make([]string, 0)
	for _, edge := range state.Edges {
		if edge.To != id {
			continue
		}
		if node, ok := nodeByID(state.Nodes, edge.From); ok {
			out = append(out, node.Text)
		}
	}
	return out
}

func successorTexts(state graphState, id string) []string {
	out := make([]string, 0)
	for _, edge := range state.Edges {
		if edge.From != id {
			continue
		}
		if node, ok := nodeByID(state.Nodes, edge.To); ok {
			out = append(out, node.Text)
		}
	}
	return out
}

func serializeNeighborTexts(values []string) string {
	values = CloneStrings(values)
	for i := range values {
		values[i] = strings.TrimSpace(values[i])
	}
	out := make([]string, 0, len(values))
	for _, value := range values {
		if value == "" {
			continue
		}
		out = append(out, "- "+value)
	}
	if len(out) == 0 {
		return "- (none)"
	}
	return strings.Join(out, "\n")
}

func nodeByID(nodes []graphNode, id string) (graphNode, bool) {
	for _, n := range nodes {
		if n.ID == id {
			return n, true
		}
	}
	return graphNode{}, false
}

func dedupeGraphStateForRender(state graphState) graphState {
	if len(state.Nodes) == 0 {
		return state
	}
	seen := map[string]int{}
	nodes := make([]graphNode, 0, len(state.Nodes))
	for _, node := range state.Nodes {
		id := strings.TrimSpace(node.ID)
		if id == "" {
			nodes = append(nodes, node)
			continue
		}
		node.ID = id
		if idx, ok := seen[id]; ok {
			nodes[idx] = mergeDuplicateGraphNodeForRender(nodes[idx], node)
			continue
		}
		seen[id] = len(nodes)
		nodes = append(nodes, node)
	}
	state.Nodes = nodes
	return pruneDanglingEdges(state)
}

func mergeDuplicateGraphNodeForRender(base, next graphNode) graphNode {
	if strings.TrimSpace(base.Text) == "" {
		base.Text = next.Text
	}
	if strings.TrimSpace(base.SourceQuote) == "" {
		base.SourceQuote = next.SourceQuote
	}
	if strings.TrimSpace(string(base.Role)) == "" {
		base.Role = next.Role
	}
	if strings.TrimSpace(base.Ontology) == "" {
		base.Ontology = next.Ontology
	}
	if strings.TrimSpace(base.DiscourseRole) == "" {
		base.DiscourseRole = next.DiscourseRole
	}
	base.IsTarget = base.IsTarget || next.IsTarget
	return base
}

func normalizeText(text string) string {
	return strings.ToLower(strings.Join(strings.Fields(strings.TrimSpace(text)), " "))
}

func uniqueTexts(nodes []graphNode, targets []graphNode, paths []renderedPath, declarationNodes []graphNode, off []offGraphItem) []map[string]string {
	seen := map[string]struct{}{}
	out := make([]map[string]string, 0)
	add := func(id, text string) {
		if _, ok := seen[id]; ok {
			return
		}
		seen[id] = struct{}{}
		out = append(out, map[string]string{"id": id, "text": text})
	}
	for _, n := range nodes {
		add(n.ID, n.Text)
	}
	for _, n := range targets {
		add(n.ID, n.Text)
	}
	for _, p := range paths {
		add(p.driver.ID, p.driver.Text)
		add(p.target.ID, p.target.Text)
		for _, premise := range p.premises {
			add(premise.ID, premise.Text)
		}
		for _, s := range p.steps {
			add(s.ID, s.Text)
		}
	}
	for _, n := range declarationNodes {
		add(n.ID, n.Text)
	}
	for _, o := range off {
		add(o.ID, o.Text)
	}
	return out
}

func supportDriveMarkers() []string {
	return []string{
		"导致", "引发", "造成", "使", "使得", "影响", "推高", "推动", "压低", "拖累", "传导", "形成", "收缩", "飙升", "解释为什么", "因此", "然后",
		"cause", "causes", "caused", "lead to", "leads to", "led to", "trigger", "triggers", "triggered", "push", "pushes", "pushed", "drives", "driven", "forms", "formed", "creates", "created", "explains why", "consequence", "therefore", "then", "which leads",
	}
}

func normalizeMainlineRelationKind(kind string) string {
	switch strings.ToLower(strings.TrimSpace(kind)) {
	case "inference", "inferential", "proof":
		return "inference"
	case "illustration", "analogy", "satire", "satirical":
		return "illustration"
	default:
		return "causal"
	}
}

func discourseRolePriority(role string) int {
	switch normalizeDiscourseRole(role) {
	case "thesis":
		return 7
	case "mechanism":
		return 6
	case "implication":
		return 5
	case "market_move":
		return 4
	case "caveat":
		return 3
	case "evidence":
		return 2
	case "example":
		return 1
	default:
		return 0
	}
}
