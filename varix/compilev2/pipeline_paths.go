package compilev2

import (
	"github.com/kumaloha/VariX/varix/compile"
	"strings"
)

type renderedPath struct {
	branchID string
	driver   graphNode
	target   graphNode
	steps    []graphNode
	edges    []PreviewEdge
}

func extractSpinePaths(state graphState) []renderedPath {
	if len(state.Spines) == 0 {
		return nil
	}
	valid := map[string]struct{}{}
	nodeIndex := map[string]graphNode{}
	for _, node := range state.Nodes {
		valid[node.ID] = struct{}{}
		nodeIndex[node.ID] = node
	}
	out := make([]renderedPath, 0)
	seen := map[string]struct{}{}
	for _, spine := range state.Spines {
		nodeIDs := validSpineNodeIDs(spine, valid)
		if len(nodeIDs) < 2 {
			continue
		}
		sources, terminals := spineSourceAndTerminalIDs(spine, nodeIDs, valid, nodeIndex)
		adj := spineAdjacency(spine, valid, nodeIndex)
		if len(adj) == 0 && len(nodeIDs) >= 2 {
			for i := 0; i+1 < len(nodeIDs); i++ {
				adj[nodeIDs[i]] = append(adj[nodeIDs[i]], nodeIDs[i+1])
			}
		}
		for _, source := range sources {
			for _, terminal := range terminals {
				pathIDs := shortestPath(adj, source, terminal)
				if len(pathIDs) < 2 {
					continue
				}
				key := strings.Join(pathIDs, "->")
				if _, ok := seen[key]; ok {
					continue
				}
				driver, ok := nodeByID(state.Nodes, source)
				if !ok {
					continue
				}
				target, ok := nodeByID(state.Nodes, terminal)
				if !ok {
					continue
				}
				steps := make([]graphNode, 0, max(0, len(pathIDs)-2))
				for _, id := range pathIDs[1 : len(pathIDs)-1] {
					if node, ok := nodeByID(state.Nodes, id); ok {
						steps = append(steps, node)
					}
				}
				seen[key] = struct{}{}
				out = append(out, renderedPath{
					branchID: spine.ID,
					driver:   driver,
					target:   target,
					steps:    steps,
					edges:    previewEdgesForPath(pathIDs, spineProjectionEdges(spine, nodeIndex), nodeIndex),
				})
			}
		}
	}
	return out
}

func spineAdjacency(spine PreviewSpine, valid map[string]struct{}, nodes map[string]graphNode) map[string][]string {
	adj := map[string][]string{}
	for _, edge := range spineProjectionEdges(spine, nodes) {
		if _, ok := valid[edge.From]; !ok {
			continue
		}
		if _, ok := valid[edge.To]; !ok {
			continue
		}
		if edge.From == edge.To {
			continue
		}
		adj[edge.From] = append(adj[edge.From], edge.To)
	}
	return adj
}

func extractPaths(state graphState, drivers, targets []graphNode) []renderedPath {
	adj := map[string][]string{}
	for _, e := range state.Edges {
		adj[e.From] = append(adj[e.From], e.To)
	}
	nodeIndex := map[string]graphNode{}
	for _, node := range state.Nodes {
		nodeIndex[node.ID] = node
	}
	var out []renderedPath
	for _, d := range drivers {
		for _, t := range targets {
			pathIDs := shortestPath(adj, d.ID, t.ID)
			if len(pathIDs) < 2 {
				continue
			}
			steps := make([]graphNode, 0, max(0, len(pathIDs)-2))
			for _, id := range pathIDs[1 : len(pathIDs)-1] {
				if node, ok := nodeByID(state.Nodes, id); ok {
					steps = append(steps, node)
				}
			}
			out = append(out, renderedPath{driver: d, target: t, steps: steps, edges: graphEdgesForPath(pathIDs, state.Edges, nodeIndex)})
		}
	}
	return out
}

func graphEdgesForPath(pathIDs []string, edges []graphEdge, nodes map[string]graphNode) []PreviewEdge {
	previewEdges := make([]PreviewEdge, 0, len(edges))
	for _, edge := range edges {
		previewEdges = append(previewEdges, previewEdgeFromGraphEdge(edge))
	}
	return previewEdgesForPath(pathIDs, previewEdges, nodes)
}

func previewEdgesForPath(pathIDs []string, edges []PreviewEdge, nodes map[string]graphNode) []PreviewEdge {
	if len(pathIDs) < 2 {
		return nil
	}
	edgeIndex := map[string]PreviewEdge{}
	for _, edge := range edges {
		from := strings.TrimSpace(edge.From)
		to := strings.TrimSpace(edge.To)
		if from == "" || to == "" || from == to {
			continue
		}
		key := from + "->" + to
		if existing, ok := edgeIndex[key]; ok && strings.TrimSpace(existing.SourceQuote) != "" {
			continue
		}
		edgeIndex[key] = edge
	}
	out := make([]PreviewEdge, 0, len(pathIDs)-1)
	for i := 0; i+1 < len(pathIDs); i++ {
		from := strings.TrimSpace(pathIDs[i])
		to := strings.TrimSpace(pathIDs[i+1])
		if from == "" || to == "" || from == to {
			continue
		}
		edge, ok := edgeIndex[from+"->"+to]
		if !ok {
			edge = fallbackPreviewEdgeForPathSegment(from, to, nodes)
		}
		if strings.TrimSpace(edge.From) == "" {
			edge.From = from
		}
		if strings.TrimSpace(edge.To) == "" {
			edge.To = to
		}
		out = append(out, edge)
	}
	return out
}

func fallbackPreviewEdgeForPathSegment(from, to string, nodes map[string]graphNode) PreviewEdge {
	edge := PreviewEdge{From: from, To: to}
	quotes := []string{strings.TrimSpace(nodes[from].SourceQuote), strings.TrimSpace(nodes[to].SourceQuote)}
	edge.SourceQuote = strings.Join(nonEmptyStrings(quotes...), " / ")
	return edge
}

func hasEdge(edges []graphEdge, from, to string) bool {
	for _, e := range edges {
		if e.From == from && e.To == to {
			return true
		}
	}
	return false
}

func shortestPath(adj map[string][]string, start, target string) []string {
	type item struct {
		id   string
		path []string
	}
	queue := []item{{id: start, path: []string{start}}}
	seen := map[string]struct{}{start: {}}
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		if cur.id == target {
			return cur.path
		}
		for _, next := range adj[cur.id] {
			if _, ok := seen[next]; ok {
				continue
			}
			seen[next] = struct{}{}
			queue = append(queue, item{id: next, path: appendPathNode(cur.path, next)})
		}
	}
	return nil
}

func appendPathNode(path []string, next string) []string {
	cloned := compile.CloneStrings(path)
	return append(cloned, next)
}

func dedupeEdges(edges []graphEdge) []graphEdge {
	seen := map[string]struct{}{}
	out := make([]graphEdge, 0, len(edges))
	for _, e := range edges {
		key := e.From + "->" + e.To
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, e)
	}
	return out
}

func pruneTransitiveRelations(edges []graphEdge) []graphEdge {
	edges = dedupeEdges(edges)
	out := make([]graphEdge, 0, len(edges))
	for i, edge := range edges {
		if hasAlternateMainlinePath(edges, i, edge.From, edge.To) {
			continue
		}
		out = append(out, edge)
	}
	return out
}

func hasAlternateMainlinePath(edges []graphEdge, skipIndex int, from, to string) bool {
	adj := map[string][]string{}
	for i, edge := range edges {
		if i == skipIndex {
			continue
		}
		if strings.TrimSpace(edge.From) == "" || strings.TrimSpace(edge.To) == "" || edge.From == edge.To {
			continue
		}
		adj[edge.From] = append(adj[edge.From], edge.To)
	}
	path := shortestPath(adj, from, to)
	return len(path) >= 3
}

func dedupeAuxEdges(edges []auxEdge) []auxEdge {
	seen := map[string]struct{}{}
	out := make([]auxEdge, 0, len(edges))
	for _, edge := range edges {
		key := edge.Kind + "|" + edge.From + "|" + edge.To
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, edge)
	}
	return out
}

func buildAuxEdgesFromSupport(nodes []graphNode, raw []struct {
	From        string `json:"from"`
	To          string `json:"to"`
	SourceQuote string `json:"source_quote"`
	Reason      string `json:"reason"`
}) []auxEdge {
	valid := map[string]struct{}{}
	for _, n := range nodes {
		valid[n.ID] = struct{}{}
	}
	out := make([]auxEdge, 0, len(raw))
	for _, e := range raw {
		if _, ok := valid[e.From]; !ok {
			continue
		}
		if _, ok := valid[e.To]; !ok {
			continue
		}
		if e.From == e.To {
			continue
		}
		out = append(out, auxEdge{
			From:        e.From,
			To:          e.To,
			Kind:        "evidence",
			SourceQuote: strings.TrimSpace(e.SourceQuote),
			Reason:      strings.TrimSpace(e.Reason),
		})
	}
	return out
}

func buildAuxEdgesFromExplanation(nodes []graphNode, raw []struct {
	From        string `json:"from"`
	To          string `json:"to"`
	SourceQuote string `json:"source_quote"`
	Reason      string `json:"reason"`
}) []auxEdge {
	valid := map[string]struct{}{}
	for _, n := range nodes {
		valid[n.ID] = struct{}{}
	}
	out := make([]auxEdge, 0, len(raw))
	for _, e := range raw {
		if _, ok := valid[e.From]; !ok {
			continue
		}
		if _, ok := valid[e.To]; !ok {
			continue
		}
		if e.From == e.To {
			continue
		}
		out = append(out, auxEdge{
			From:        e.From,
			To:          e.To,
			Kind:        "explanation",
			SourceQuote: strings.TrimSpace(e.SourceQuote),
			Reason:      strings.TrimSpace(e.Reason),
		})
	}
	return out
}

func buildAuxEdgesFromSupportEdges(nodes []graphNode, raw []supportEdgePatch) []auxEdge {
	valid := map[string]graphNode{}
	for _, n := range nodes {
		valid[n.ID] = n
	}
	out := make([]auxEdge, 0, len(raw))
	for _, e := range raw {
		fromNode, ok := valid[e.From]
		if !ok {
			continue
		}
		toNode, ok := valid[e.To]
		if !ok {
			continue
		}
		if e.From == e.To {
			continue
		}
		kind, ok := normalizeSupportKind(e.Kind)
		if !ok {
			continue
		}
		edge := auxEdge{
			From:        e.From,
			To:          e.To,
			Kind:        kind,
			SourceQuote: strings.TrimSpace(e.SourceQuote),
			Reason:      strings.TrimSpace(e.Reason),
		}
		if isLikelyMainlineAuxEdge(edge, fromNode, toNode) {
			continue
		}
		out = append(out, edge)
	}
	return out
}

func isLikelyMainlineAuxEdge(edge auxEdge, fromNode, toNode graphNode) bool {
	switch strings.TrimSpace(edge.Kind) {
	case "explanation", "supplementary":
	default:
		return false
	}
	if looksLikeAuxiliaryDetailNode(fromNode.Text) {
		return false
	}
	if !looksLikeOutcomeOrProcessEndpoint(fromNode.Text) || !looksLikeOutcomeOrProcessEndpoint(toNode.Text) {
		return false
	}
	context := strings.ToLower(strings.Join([]string{
		fromNode.Text,
		toNode.Text,
		fromNode.SourceQuote,
		toNode.SourceQuote,
		edge.SourceQuote,
		edge.Reason,
	}, " "))
	return containsAnyText(context, supportDriveMarkers())
}

func looksLikeAuxiliaryDetailNode(text string) bool {
	lower := strings.ToLower(strings.TrimSpace(text))
	if looksLikeOutcomeOrProcessEndpoint(text) && !containsAnyText(lower, []string{"赎回申请", "赎回请求", "机构资金", "占比", "比例", "不良贷款"}) {
		return false
	}
	if looksLikePureQuantOrThreshold(lower) || looksLikePureRuleOrLimit(lower) {
		return true
	}
	for _, marker := range []string{
		"底层资产", "企业贷款", "日常流动性", "机构资金", "机构资金占比", "贷款标准", "估值透明度", "pik", "不良贷款", "赎回申请", "赎回请求", "国防预算", "defense budget",
	} {
		if strings.Contains(lower, strings.ToLower(marker)) {
			return true
		}
	}
	return false
}

func looksLikeOutcomeOrProcessEndpoint(text string) bool {
	lower := strings.ToLower(strings.TrimSpace(text))
	if looksLikeSubjectChangeNode(text) || looksLikeConcreteBranchResult(lower) {
		return true
	}
	for _, marker := range []string{
		"转冷", "转向", "抛售", "被抛售", "收缩", "飙升", "回落", "被推高", "高企", "维持高位", "支出上升", "被挤压", "形成", "受影响", "被压低", "被拖累", "成本上升", "居高不下", "flight to cash", "现金为王", "现象出现",
	} {
		if strings.Contains(lower, strings.ToLower(marker)) {
			return true
		}
	}
	return false
}

func supportDriveMarkers() []string {
	return []string{
		"导致", "引发", "造成", "使", "使得", "影响", "推高", "推动", "压低", "拖累", "传导", "形成", "收缩", "飙升", "解释为什么", "因此", "然后",
		"cause", "causes", "caused", "lead to", "leads to", "led to", "trigger", "triggers", "triggered", "push", "pushes", "pushed", "drives", "driven", "forms", "formed", "creates", "created", "explains why", "consequence", "therefore", "then", "which leads",
	}
}

func normalizeSupportKind(kind string) (string, bool) {
	switch strings.ToLower(strings.TrimSpace(kind)) {
	case "evidence":
		return "evidence", true
	case "inference", "inferential", "proof":
		return "inference", true
	case "explanation":
		return "explanation", true
	case "supplement", "supplementary":
		return "supplementary", true
	default:
		return "", false
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

func auxNodeRole(edge auxEdge, nodeID string) (string, bool) {
	switch edge.Kind {
	case "evidence", "inference":
		if edge.From == nodeID {
			return edge.Kind, true
		}
	case "explanation":
		if edge.From == nodeID {
			return "explanation", true
		}
	case "supplementary":
		if edge.From == nodeID {
			return "supplementary", true
		}
	}
	return "", false
}

func collectAuxComponent(adj map[string][]string, start string, visited map[string]struct{}) []string {
	stack := []string{start}
	component := make([]string, 0)
	for len(stack) > 0 {
		id := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if _, ok := visited[id]; ok {
			continue
		}
		visited[id] = struct{}{}
		component = append(component, id)
		stack = append(stack, adj[id]...)
	}
	return component
}

func chooseClusterHead(component []string, edges []auxEdge, nodeIndex map[string]graphNode) string {
	if len(component) == 0 {
		return ""
	}
	member := map[string]struct{}{}
	for _, id := range component {
		member[id] = struct{}{}
	}
	inScore := map[string]float64{}
	outScore := map[string]float64{}
	inCount := map[string]int{}
	outCount := map[string]int{}
	for _, edge := range edges {
		if _, ok := member[edge.From]; !ok {
			continue
		}
		if _, ok := member[edge.To]; !ok {
			continue
		}
		w := auxEdgeWeight(edge.Kind)
		outScore[edge.From] += w
		inScore[edge.To] += w
		outCount[edge.From]++
		inCount[edge.To]++
	}
	candidates := make([]string, 0, len(component))
	for _, candidate := range component {
		// A support edge means `from` is serving another node, so it cannot be
		// the component core. If the model creates a cycle, fall back below.
		if outCount[candidate] == 0 {
			candidates = append(candidates, candidate)
		}
	}
	if len(candidates) == 0 {
		candidates = component
	}
	best := candidates[0]
	bestScore := clusterHeadScore(best, inScore, outScore, nodeIndex)
	bestTie := clusterHeadTieBreak(nodeIndex[best].Text)
	for _, candidate := range candidates[1:] {
		score := clusterHeadScore(candidate, inScore, outScore, nodeIndex)
		tie := clusterHeadTieBreak(nodeIndex[candidate].Text)
		switch {
		case score > bestScore:
			best = candidate
			bestScore = score
			bestTie = tie
		case score == bestScore && inCount[candidate] > inCount[best]:
			best = candidate
			bestTie = tie
		case score == bestScore && inCount[candidate] == inCount[best] && outCount[candidate] < outCount[best]:
			best = candidate
			bestTie = tie
		case score == bestScore && inCount[candidate] == inCount[best] && outCount[candidate] == outCount[best] && tie > bestTie:
			best = candidate
			bestTie = tie
		}
	}
	return best
}

func auxEdgeWeight(kind string) float64 {
	switch strings.TrimSpace(kind) {
	case "evidence":
		return 3.0
	case "inference":
		return 3.25
	case "explanation":
		return 2.0
	case "supplementary":
		return 2.5
	default:
		return 1.0
	}
}

func canonicalityScore(nodeID string, inScore, outScore map[string]float64) float64 {
	return inScore[nodeID] - outScore[nodeID]
}

func clusterHeadScore(nodeID string, inScore, outScore map[string]float64, nodeIndex map[string]graphNode) float64 {
	node := nodeIndex[nodeID]
	return canonicalityScore(nodeID, inScore, outScore) + discourseRoleHeadBoost(node.DiscourseRole) + 0.35*clusterHeadTieBreak(node.Text)
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

func discourseRoleHeadBoost(role string) float64 {
	switch normalizeDiscourseRole(role) {
	case "thesis":
		return 8.0
	case "mechanism":
		return 5.0
	case "implication":
		return 3.0
	case "market_move":
		return 2.0
	case "caveat":
		return -1.0
	case "evidence":
		return -3.0
	case "example":
		return -4.0
	default:
		return 0
	}
}

func clusterHeadTieBreak(text string) float64 {
	text = strings.TrimSpace(text)
	lower := strings.ToLower(text)
	score := 0.0
	if looksLikeSubjectChangeNode(text) {
		score += 4.0
	}
	if looksLikeConcreteBranchResult(lower) {
		score += 2.5
	}
	if looksLikePureQuantOrThreshold(lower) {
		score -= 3.0
	}
	if looksLikePureRuleOrLimit(lower) {
		score -= 3.5
	}
	if looksLikeBroadCommentary(lower) {
		score -= 2.5
	}
	if looksLikeForecastOrDominoFraming(lower) {
		score -= 2.5
	}
	if looksLikeProcessSummary(lower) {
		score -= 2.0
	}
	return score
}

func isEligibleTargetHead(text string) bool {
	lower := strings.ToLower(strings.TrimSpace(text))
	if !looksLikeSubjectChangeNode(text) && !looksLikeConcreteBranchResult(lower) {
		return false
	}
	if looksLikePureQuantOrThreshold(lower) {
		return false
	}
	if looksLikePureRuleOrLimit(lower) {
		return false
	}
	if looksLikeBroadCommentary(lower) {
		return false
	}
	if looksLikeForecastOrDominoFraming(lower) {
		return false
	}
	if looksLikeProcessSummary(lower) {
		return false
	}
	return true
}

func looksLikeConcreteBranchResult(lower string) bool {
	for _, marker := range []string{"危机", "爆雷", "受阻", "上涨", "下跌", "承压", "压力", "流入减少", "流出", "减少", "流动性", "锁定", "短缺", "重洗牌", "恶化", "松动", "冻结", "挤兑", "挤提", "集中赎回", "赎回潮", "恐慌性赎回", "坏账风险", "违约风险", "下跌风险", "爆发概率", "危机爆发", "受限", "风险上升", "概率上升", "下行风险", "上涨", "下降", "spike", "surge", "freeze", "run", "shortage", "squeeze", "outflow", "pressure"} {
		if strings.Contains(lower, strings.ToLower(marker)) {
			return true
		}
	}
	return false
}

func looksLikeSubjectChangeNode(text string) bool {
	for _, marker := range []string{"上涨", "下跌", "上升", "下降", "减少", "收缩", "扩张", "恶化", "改善", "爆雷", "危机", "受阻", "承压", "压力", "流入减少", "流出", "飙升", "流动性", "锁定", "挤压", "挤兑", "挤提", "集中赎回", "赎回潮", "恐慌性赎回", "坏账风险", "违约风险", "下跌风险", "爆发概率", "危机爆发", "松动", "上涨", "rises", "falls", "surges", "drops", "faces", "suffers", "outflow", "pressure"} {
		if strings.Contains(strings.ToLower(text), strings.ToLower(marker)) {
			return true
		}
	}
	return false
}

func looksLikePureQuantOrThreshold(lower string) bool {
	for _, marker := range []string{"%", "亿", "万亿", "上限", "仅", "达到", "4.999", "44.3", "11.3", "21.9", "15.7"} {
		if strings.Contains(lower, strings.ToLower(marker)) {
			return true
		}
	}
	return false
}

func looksLikePureRuleOrLimit(lower string) bool {
	for _, marker := range []string{"最多", "上限", "允许", "规则", "每季度", "limit", "allows"} {
		if strings.Contains(lower, marker) {
			return true
		}
	}
	return false
}

func looksLikeBroadCommentary(lower string) bool {
	for _, marker := range []string{"底色", "局面", "更棘手", "气氛", "评论", "整体", "复杂", "流动性环境", "headline", "hook", "时代", "并列", "系统性问题"} {
		if strings.Contains(lower, marker) {
			return true
		}
	}
	return false
}

func looksLikeForecastOrDominoFraming(lower string) bool {
	for _, marker := range []string{"可能", "未必", "first domino", "domino", "最先", "判断", "预测", "预计"} {
		if strings.Contains(lower, marker) {
			return true
		}
	}
	return false
}

func looksLikeProcessSummary(lower string) bool {
	for _, marker := range []string{"形成", "螺旋", "交织", "拖住", "推高", "挤压", "连锁", "一层层", "重塑", "summary"} {
		if strings.Contains(lower, marker) {
			return true
		}
	}
	return false
}

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
	values = compile.CloneStrings(values)
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

func normalizeText(text string) string {
	return strings.ToLower(strings.Join(strings.Fields(strings.TrimSpace(text)), " "))
}

func uniqueTexts(nodes []graphNode, targets []graphNode, paths []renderedPath, off []offGraphItem) []map[string]string {
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
		for _, s := range p.steps {
			add(s.ID, s.Text)
		}
	}
	for _, o := range off {
		add(o.ID, o.Text)
	}
	return out
}
