package compilev2

import (
	"context"
	"fmt"
	"github.com/kumaloha/VariX/varix/compile"
	"sort"
	"strings"
)

func stage5Render(ctx context.Context, rt runtimeChat, model string, bundle compile.Bundle, state graphState) (compile.Output, error) {
	if projected, ok := projectRolesFromSpines(state); ok {
		state = projected
	}
	drivers := filterNodesByRole(state.Nodes, roleDriver)
	targets := filterTargetNodes(state.Nodes)
	if len(targets) == 0 && len(drivers) > 0 {
		targets = fallbackTargetNodesFromOffGraph(state.OffGraph)
	}
	paths := extractSpinePaths(state)
	if len(paths) == 0 {
		paths = extractPaths(state, drivers, targets)
	}
	paths, satiricalCoveredNodes := applySatiricalRenderProjection(state, paths)
	paths = filterCyclicRenderPaths(paths)
	drivers = mergePathDrivers(drivers, paths)
	targets = mergePathTargets(targets, paths)
	drivers = filterRenderDrivers(drivers, paths)
	targets = filterRenderTargets(targets, paths, state.ArticleForm, satiricalCoveredNodes)
	translated, err := translateAll(ctx, rt, model, uniqueTexts(drivers, targets, paths, state.OffGraph))
	if err != nil {
		return compile.Output{}, err
	}
	cn := func(id, fallback string) string {
		if value, ok := translated[id]; ok && strings.TrimSpace(value) != "" {
			return value
		}
		return fallback
	}
	driversOut := make([]string, 0, len(drivers))
	for _, d := range drivers {
		driversOut = append(driversOut, cn(d.ID, d.Text))
	}
	targetsOut := make([]string, 0, len(targets))
	for _, t := range targets {
		targetsOut = append(targetsOut, cn(t.ID, t.Text))
	}
	transmission := make([]compile.TransmissionPath, 0, len(paths))
	for _, p := range paths {
		transmission = append(transmission, renderPathToTransmission(p, cn))
	}
	branches := renderBranchesFromSpines(state.Spines, paths, cn)
	evidence, explanation, supplementary := renderOffGraph(state.OffGraph, cn)
	detailItems := renderOffGraphDetails(state.OffGraph, cn)
	detailItems = append(detailItems, renderTransmissionPathDetails(paths, cn)...)
	evidence = dedupeStrings(append(evidence, renderSpineIllustrations(state, cn)...))
	summary, err := summarizeChinese(ctx, rt, model, state.ArticleForm, driversOut, targetsOut, transmission, bundle)
	if err != nil {
		summary = fallbackSummary(driversOut, targetsOut)
	}
	if strings.TrimSpace(summary) == "" {
		summary = fallbackSummary(driversOut, targetsOut)
	}
	graph := compile.ReasoningGraph{}
	for _, n := range state.Nodes {
		kind := compile.NodeMechanism
		form := compile.NodeFormObservation
		function := compile.NodeFunctionTransmission
		switch n.Role {
		case roleDriver:
			kind = compile.NodeMechanism
			form = compile.NodeFormObservation
			function = compile.NodeFunctionTransmission
		case roleTransmission:
			kind = compile.NodeMechanism
			form = compile.NodeFormObservation
			function = compile.NodeFunctionTransmission
		}
		if n.IsTarget {
			kind = compile.NodeConclusion
			form = compile.NodeFormJudgment
			function = compile.NodeFunctionClaim
			if n.Ontology == "flow" {
				kind = compile.NodeConclusion
			}
		}
		graph.Nodes = append(graph.Nodes, compile.GraphNode{
			ID:         n.ID,
			Kind:       kind,
			Form:       form,
			Function:   function,
			Text:       cn(n.ID, n.Text),
			OccurredAt: bundle.PostedAt,
		})
	}
	for _, e := range state.Edges {
		graph.Edges = append(graph.Edges, compile.GraphEdge{From: e.From, To: e.To, Kind: compile.EdgePositive})
	}
	return compile.Output{
		Summary:            summary,
		Drivers:            driversOut,
		Targets:            targetsOut,
		TransmissionPaths:  transmission,
		Branches:           branches,
		EvidenceNodes:      evidence,
		ExplanationNodes:   explanation,
		SupplementaryNodes: supplementary,
		Graph:              graph,
		Details:            compile.HiddenDetails{Caveats: []string{"compile v2 mvp"}, Items: detailItems},
		Topics:             nil,
		Confidence:         confidenceFromState(driversOut, targetsOut, transmission),
	}, nil
}

func renderPathToTransmission(path renderedPath, cn func(string, string) string) compile.TransmissionPath {
	steps := make([]string, 0, len(path.steps))
	for _, step := range path.steps {
		steps = append(steps, cn(step.ID, step.Text))
	}
	if len(steps) == 0 {
		steps = append(steps, cn(path.driver.ID, path.driver.Text))
	}
	return compile.TransmissionPath{
		Driver: cn(path.driver.ID, path.driver.Text),
		Target: cn(path.target.ID, path.target.Text),
		Steps:  steps,
	}
}

func renderBranchesFromSpines(spines []PreviewSpine, paths []renderedPath, cn func(string, string) string) []compile.Branch {
	if len(spines) == 0 || len(paths) == 0 {
		return nil
	}
	pathsByBranch := map[string][]renderedPath{}
	for _, path := range paths {
		branchID := strings.TrimSpace(path.branchID)
		if branchID == "" {
			continue
		}
		pathsByBranch[branchID] = append(pathsByBranch[branchID], path)
	}
	if len(pathsByBranch) == 0 {
		return nil
	}
	commonDrivers := commonRenderedDrivers(paths, cn)
	out := make([]compile.Branch, 0, len(spines))
	for _, spine := range spines {
		branchID := strings.TrimSpace(spine.ID)
		if branchID == "" {
			continue
		}
		branchPaths := pathsByBranch[branchID]
		if len(branchPaths) == 0 {
			continue
		}
		branch := compile.Branch{
			ID:     branchID,
			Level:  strings.TrimSpace(spine.Level),
			Policy: normalizePreviewSpinePolicy(spine.Policy),
			Thesis: strings.TrimSpace(spine.Thesis),
		}
		for _, path := range branchPaths {
			driver := cn(path.driver.ID, path.driver.Text)
			target := cn(path.target.ID, path.target.Text)
			if _, ok := commonDrivers[driver]; ok {
				branch.Anchors = appendUniqueString(branch.Anchors, driver)
			}
			if branchDriver := renderBranchDriver(path, commonDrivers, cn); branchDriver != "" {
				branch.BranchDrivers = appendUniqueString(branch.BranchDrivers, branchDriver)
			}
			branch.Drivers = appendUniqueString(branch.Drivers, driver)
			branch.Targets = appendUniqueString(branch.Targets, target)
			branch.TransmissionPaths = append(branch.TransmissionPaths, renderPathToTransmission(path, cn))
		}
		out = append(out, branch)
	}
	return out
}

func commonRenderedDrivers(paths []renderedPath, cn func(string, string) string) map[string]struct{} {
	driverBranches := map[string]map[string]struct{}{}
	for _, path := range paths {
		branchID := strings.TrimSpace(path.branchID)
		if branchID == "" {
			continue
		}
		driver := cn(path.driver.ID, path.driver.Text)
		if strings.TrimSpace(driver) == "" {
			continue
		}
		if driverBranches[driver] == nil {
			driverBranches[driver] = map[string]struct{}{}
		}
		driverBranches[driver][branchID] = struct{}{}
	}
	common := map[string]struct{}{}
	for driver, branches := range driverBranches {
		if len(branches) > 1 {
			common[driver] = struct{}{}
		}
	}
	return common
}

func renderBranchDriver(path renderedPath, commonDrivers map[string]struct{}, cn func(string, string) string) string {
	target := cn(path.target.ID, path.target.Text)
	candidates := make([]string, 0, len(path.steps)+1)
	candidates = append(candidates, cn(path.driver.ID, path.driver.Text))
	for _, step := range path.steps {
		candidates = append(candidates, cn(step.ID, step.Text))
	}
	for _, candidate := range candidates {
		candidate = strings.TrimSpace(candidate)
		if candidate == "" || candidate == target {
			continue
		}
		if _, ok := commonDrivers[candidate]; ok {
			continue
		}
		return candidate
	}
	return ""
}

func appendUniqueString(values []string, value string) []string {
	value = strings.TrimSpace(value)
	if value == "" {
		return values
	}
	for _, existing := range values {
		if existing == value {
			return values
		}
	}
	return append(values, value)
}

func filterCyclicRenderPaths(paths []renderedPath) []renderedPath {
	if len(paths) < 2 {
		return paths
	}
	reaches := map[string]map[string]struct{}{}
	out := make([]renderedPath, 0, len(paths))
	for _, path := range paths {
		nodeIDs := renderedPathNodeIDs(path)
		if len(nodeIDs) < 2 {
			out = append(out, path)
			continue
		}
		if renderedPathHasCycle(nodeIDs, reaches) {
			continue
		}
		out = append(out, path)
		for i := 0; i+1 < len(nodeIDs); i++ {
			addReachability(reaches, nodeIDs[i], nodeIDs[i+1])
		}
	}
	if len(out) == 0 {
		return paths
	}
	return out
}

func renderedPathNodeIDs(path renderedPath) []string {
	nodeIDs := make([]string, 0, len(path.steps)+2)
	if id := strings.TrimSpace(path.driver.ID); id != "" {
		nodeIDs = append(nodeIDs, id)
	}
	for _, step := range path.steps {
		if id := strings.TrimSpace(step.ID); id != "" {
			nodeIDs = append(nodeIDs, id)
		}
	}
	if id := strings.TrimSpace(path.target.ID); id != "" {
		nodeIDs = append(nodeIDs, id)
	}
	return nodeIDs
}

func renderedPathHasCycle(nodeIDs []string, reaches map[string]map[string]struct{}) bool {
	seen := map[string]struct{}{}
	for _, id := range nodeIDs {
		if _, ok := seen[id]; ok {
			return true
		}
		seen[id] = struct{}{}
	}
	for i := 0; i+1 < len(nodeIDs); i++ {
		for j := i + 1; j < len(nodeIDs); j++ {
			if pathReachable(reaches, nodeIDs[j], nodeIDs[i]) {
				return true
			}
		}
	}
	return false
}

func pathReachable(reaches map[string]map[string]struct{}, from, to string) bool {
	if from == to {
		return true
	}
	_, ok := reaches[from][to]
	return ok
}

func addReachability(reaches map[string]map[string]struct{}, from, to string) {
	ensureReachSet := func(id string) map[string]struct{} {
		if reaches[id] == nil {
			reaches[id] = map[string]struct{}{}
		}
		return reaches[id]
	}
	fromSet := ensureReachSet(from)
	fromSet[to] = struct{}{}
	for next := range reaches[to] {
		fromSet[next] = struct{}{}
	}
	for source, targets := range reaches {
		if source == from {
			continue
		}
		if _, ok := targets[from]; !ok {
			continue
		}
		targets[to] = struct{}{}
		for next := range reaches[to] {
			targets[next] = struct{}{}
		}
	}
}

type satiricalProjection struct {
	path    renderedPath
	nodeSet map[string]struct{}
}

func applySatiricalRenderProjection(state graphState, paths []renderedPath) ([]renderedPath, map[string]struct{}) {
	if len(state.Spines) == 0 {
		return paths, nil
	}
	nodeIndex := map[string]graphNode{}
	valid := map[string]struct{}{}
	for _, node := range state.Nodes {
		nodeIndex[node.ID] = node
		valid[node.ID] = struct{}{}
	}
	projections := make([]satiricalProjection, 0)
	for _, spine := range state.Spines {
		if normalizePreviewSpinePolicy(spine.Policy) != "satirical_analogy" {
			continue
		}
		path, nodeSet, ok := satiricalDisplayPath(spine, nodeIndex, valid, state.OffGraph)
		if !ok {
			continue
		}
		path.branchID = spine.ID
		projections = append(projections, satiricalProjection{path: path, nodeSet: nodeSet})
	}
	if len(projections) == 0 {
		return paths, nil
	}
	covered := map[string]struct{}{}
	out := make([]renderedPath, 0, len(projections)+len(paths))
	for _, projection := range projections {
		out = append(out, projection.path)
		for id := range projection.nodeSet {
			covered[id] = struct{}{}
		}
	}
	for _, path := range paths {
		if pathWithinAnySatiricalProjection(path, projections) {
			continue
		}
		out = append(out, path)
	}
	return out, covered
}

func satiricalDisplayPath(spine PreviewSpine, nodes map[string]graphNode, valid map[string]struct{}, offGraph []offGraphItem) (renderedPath, map[string]struct{}, bool) {
	nodeIDs := validSpineNodeIDs(spine, valid)
	if len(nodeIDs) < 2 {
		return renderedPath{}, nil, false
	}
	nodeSet := map[string]struct{}{}
	ordered := make([]graphNode, 0, len(nodeIDs))
	for _, id := range nodeIDs {
		node, ok := nodes[id]
		if !ok {
			continue
		}
		nodeSet[id] = struct{}{}
		ordered = append(ordered, node)
	}
	if len(ordered) < 2 {
		return renderedPath{}, nil, false
	}
	driver, driverOK := bestSatiricalDriverNode(ordered)
	target, targetOK := bestSatiricalTargetNode(ordered, driver.ID)
	if offTarget, ok := bestSatiricalOffGraphTarget(offGraph, nodeSet); ok && (!targetOK || satiricalTargetScore(offTarget) > satiricalTargetScore(target)) {
		target = offTarget
		targetOK = true
	}
	if !driverOK || !targetOK || driver.ID == target.ID {
		return renderedPath{}, nil, false
	}
	steps := make([]graphNode, 0, min(4, max(0, len(ordered)-2)))
	for _, node := range ordered {
		if node.ID == driver.ID || node.ID == target.ID {
			continue
		}
		steps = append(steps, node)
		if len(steps) >= 4 {
			break
		}
	}
	return renderedPath{driver: driver, target: target, steps: steps}, nodeSet, true
}

func bestSatiricalDriverNode(nodes []graphNode) (graphNode, bool) {
	best := graphNode{}
	bestScore := -999
	for _, node := range nodes {
		score := satiricalDriverScore(node)
		if score > bestScore {
			best = node
			bestScore = score
		}
	}
	return best, bestScore > 0
}

func bestSatiricalTargetNode(nodes []graphNode, driverID string) (graphNode, bool) {
	best := graphNode{}
	bestScore := -999
	for _, node := range nodes {
		if node.ID == driverID {
			continue
		}
		score := satiricalTargetScore(node)
		if score > bestScore {
			best = node
			bestScore = score
		}
	}
	return best, bestScore > 0
}

func bestSatiricalOffGraphTarget(offGraph []offGraphItem, spineNodeSet map[string]struct{}) (graphNode, bool) {
	best := graphNode{}
	bestScore := -999
	for i, item := range offGraph {
		attachTo := strings.TrimSpace(item.AttachesTo)
		if attachTo == "" {
			continue
		}
		if _, ok := spineNodeSet[attachTo]; !ok {
			continue
		}
		text := strings.TrimSpace(item.Text)
		if text == "" {
			continue
		}
		id := strings.TrimSpace(item.ID)
		if id == "" {
			id = fmt.Sprintf("satire_offgraph_target_%d", i+1)
		}
		node := graphNode{
			ID:            id,
			Text:          text,
			SourceQuote:   item.SourceQuote,
			DiscourseRole: item.Role,
			Role:          roleTransmission,
			IsTarget:      true,
		}
		score := satiricalTargetScore(node)
		if score > bestScore {
			best = node
			bestScore = score
		}
	}
	return best, bestScore > 0
}

func satiricalDriverScore(node graphNode) int {
	text := strings.ToLower(strings.TrimSpace(node.Text))
	score := 0
	switch normalizeDiscourseRole(node.DiscourseRole) {
	case "satire_target":
		score += 35
	case "implied_thesis":
		score += 30
	case "thesis":
		score += 18
	case "mechanism":
		score += 5
	}
	for _, marker := range []string{"叙事", "包装", "公平", "不公平", "牌照", "表面", "实质", "控制", "机制", "忽悠", "手续费", "零售客户", "买单"} {
		if strings.Contains(text, marker) {
			score += 7
		}
	}
	for _, marker := range []string{"2000", "每人", "每月", "年息", "委托贷款", "抽一人", "中奖者", "存银行"} {
		if strings.Contains(text, marker) {
			score -= 10
		}
	}
	return score
}

func satiricalTargetScore(node graphNode) int {
	text := strings.ToLower(strings.TrimSpace(node.Text))
	score := 0
	switch normalizeDiscourseRole(node.DiscourseRole) {
	case "implication":
		score += 18
	case "market_move":
		score += 10
	}
	for _, marker := range []string{"归管理者", "归我", "基金", "净亏", "承担", "成本", "缺口", "后75", "零售客户", "买单", "损失", "亏", "转移", "锁定", "无法取出"} {
		if strings.Contains(text, marker) {
			score += 8
		}
	}
	for _, marker := range []string{"规则", "每人", "每月", "年息", "叙事", "包装"} {
		if strings.Contains(text, marker) {
			score -= 6
		}
	}
	return score
}

func pathWithinAnySatiricalProjection(path renderedPath, projections []satiricalProjection) bool {
	for _, projection := range projections {
		if _, ok := projection.nodeSet[path.driver.ID]; !ok {
			continue
		}
		if _, ok := projection.nodeSet[path.target.ID]; ok {
			return true
		}
	}
	return false
}

func renderSpineIllustrations(state graphState, cn func(string, string) string) []string {
	if len(state.Spines) == 0 {
		return nil
	}
	byID := map[string]graphNode{}
	for _, node := range state.Nodes {
		byID[node.ID] = node
	}
	out := make([]string, 0)
	for _, spine := range state.Spines {
		if normalizePreviewSpinePolicy(spine.Policy) != "satirical_analogy" {
			continue
		}
		for _, id := range spine.NodeIDs {
			node, ok := byID[id]
			if ok && normalizeDiscourseRole(node.DiscourseRole) == "analogy" {
				out = append(out, cn(node.ID, node.Text))
			}
		}
		for _, edge := range spine.Edges {
			if !isIllustrationKind(edge.Kind) {
				continue
			}
			node, ok := byID[edge.From]
			if !ok {
				continue
			}
			out = append(out, cn(node.ID, node.Text))
		}
	}
	return out
}

func mergePathDrivers(drivers []graphNode, paths []renderedPath) []graphNode {
	out := append([]graphNode(nil), drivers...)
	seen := map[string]struct{}{}
	for _, driver := range out {
		seen[driver.ID] = struct{}{}
	}
	for _, path := range paths {
		if strings.TrimSpace(path.driver.ID) == "" {
			continue
		}
		if _, ok := seen[path.driver.ID]; ok {
			continue
		}
		seen[path.driver.ID] = struct{}{}
		out = append(out, path.driver)
	}
	return out
}

func mergePathTargets(targets []graphNode, paths []renderedPath) []graphNode {
	out := append([]graphNode(nil), targets...)
	seen := map[string]struct{}{}
	for _, target := range out {
		seen[target.ID] = struct{}{}
	}
	for _, path := range paths {
		if strings.TrimSpace(path.target.ID) == "" {
			continue
		}
		if _, ok := seen[path.target.ID]; ok {
			continue
		}
		seen[path.target.ID] = struct{}{}
		target := path.target
		target.IsTarget = true
		out = append(out, target)
	}
	return out
}

func filterRenderDrivers(drivers []graphNode, paths []renderedPath) []graphNode {
	if len(drivers) == 0 || len(paths) == 0 {
		return drivers
	}
	pathTargets := map[string]struct{}{}
	pathSteps := map[string]struct{}{}
	for _, path := range paths {
		if strings.TrimSpace(path.target.ID) != "" {
			pathTargets[path.target.ID] = struct{}{}
		}
		for _, step := range path.steps {
			if strings.TrimSpace(step.ID) != "" {
				pathSteps[step.ID] = struct{}{}
			}
		}
	}
	out := make([]graphNode, 0, len(drivers))
	for _, driver := range drivers {
		if _, ok := pathTargets[driver.ID]; ok {
			continue
		}
		if _, ok := pathSteps[driver.ID]; ok {
			continue
		}
		out = append(out, driver)
	}
	if len(out) == 0 {
		return drivers
	}
	return out
}

func filterRenderTargets(targets []graphNode, paths []renderedPath, articleForm string, satiricalCoveredNodes map[string]struct{}) []graphNode {
	if len(targets) == 0 || len(paths) == 0 {
		return targets
	}
	pathDrivers := map[string]struct{}{}
	pathSteps := map[string]struct{}{}
	for _, path := range paths {
		if strings.TrimSpace(path.driver.ID) != "" {
			pathDrivers[path.driver.ID] = struct{}{}
		}
		for _, step := range path.steps {
			if strings.TrimSpace(step.ID) != "" {
				pathSteps[step.ID] = struct{}{}
			}
		}
	}
	out := make([]graphNode, 0, len(targets))
	for _, target := range targets {
		if _, ok := pathDrivers[target.ID]; ok {
			continue
		}
		if _, ok := pathSteps[target.ID]; ok && hasID(satiricalCoveredNodes, target.ID) {
			continue
		}
		if isRenderProcessStateTarget(target) {
			continue
		}
		if isLowWeightForecastTarget(target, articleForm) {
			continue
		}
		out = append(out, target)
	}
	if len(out) == 0 {
		return targets
	}
	return out
}

func isLowWeightForecastTarget(node graphNode, articleForm string) bool {
	if normalizeArticleForm(articleForm) != "evidence_backed_forecast" {
		return false
	}
	text := strings.ToLower(strings.TrimSpace(node.Text))
	if text == "" {
		return false
	}
	if containsAnyText(text, []string{"ai", "人工智能"}) &&
		containsAnyText(text, []string{"通胀", "inflation", "反通胀", "disinflation", "deflation"}) {
		return true
	}
	if containsAnyText(text, []string{"跨境", "税务", "税负", "pfic", "tax", "jurisdiction"}) {
		return true
	}
	return normalizeDiscourseRole(node.DiscourseRole) == "caveat"
}

func isRenderProcessStateTarget(node graphNode) bool {
	text := strings.ToLower(strings.TrimSpace(node.Text))
	if text == "" {
		return false
	}
	if containsAnyText(text, []string{"核心机制", "core mechanism", "机制是", "mechanism is"}) {
		return true
	}
	if containsAnyText(text, []string{"金融抑制", "financial repression"}) &&
		containsAnyText(text, []string{"存款利率上限", "资本管制", "锁定资金", "购买国债", "capital control", "rate cap"}) {
		return true
	}
	hasProcessSubject := containsAnyText(text, []string{
		"金融抑制", "financial repression", "机制", "制度", "regime",
	})
	if !hasProcessSubject {
		return false
	}
	return containsAnyText(text, []string{
		"启动", "开启", "正式开启", "正式启动", "launch", "starts", "begins", "triggered",
	})
}

func fallbackTargetNodesFromOffGraph(off []offGraphItem) []graphNode {
	type candidate struct {
		node  graphNode
		score int
	}
	candidates := make([]candidate, 0)
	for i, item := range off {
		text := strings.TrimSpace(item.Text)
		if text == "" {
			continue
		}
		score := fallbackTargetScore(text)
		if score <= 0 {
			continue
		}
		id := strings.TrimSpace(item.ID)
		if id == "" {
			id = fmt.Sprintf("fallback_target_%d", i+1)
		}
		candidates = append(candidates, candidate{
			node: graphNode{
				ID:       id,
				Text:     text,
				Role:     roleTransmission,
				Ontology: inferTargetKind(text, true),
				IsTarget: true,
			},
			score: score,
		})
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		return candidates[i].score > candidates[j].score
	})
	limit := 3
	if len(candidates) < limit {
		limit = len(candidates)
	}
	out := make([]graphNode, 0, limit)
	for i := 0; i < limit; i++ {
		out = append(out, candidates[i].node)
	}
	return out
}

func fallbackTargetScore(text string) int {
	lower := strings.ToLower(strings.TrimSpace(text))
	score := 0
	for _, marker := range []string{
		"风险", "压力", "挤兑", "危机", "承压", "下降", "流出", "减少",
		"重估", "上升", "紧张", "爆发", "违约", "撤资", "赎回", "系统性",
		"run", "stress", "risk", "pressure", "outflow", "redemption", "default",
	} {
		if strings.Contains(lower, marker) {
			score += 2
		}
	}
	for _, marker := range []string{"私募信贷", "美债", "美股", "拥挤交易", "资金", "流动性"} {
		if strings.Contains(lower, marker) {
			score++
		}
	}
	for _, marker := range []string{"指", "本质", "形成", "怎么回事", "不受银行监管"} {
		if strings.Contains(lower, marker) {
			score -= 2
		}
	}
	return score
}
