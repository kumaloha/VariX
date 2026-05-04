package compile

import (
	"context"
	"strings"
	"time"
)

func stage5Render(ctx context.Context, rt runtimeChat, model string, bundle Bundle, state graphState) (Output, error) {
	if len(state.Brief) == 0 {
		state = stageBrief(state)
	}
	state = applyDeclarationCoverageGate(bundle, state)
	state = dedupeGraphStateForRender(state)
	if projected, ok := projectRolesFromSpines(state); ok {
		state = dedupeGraphStateForRender(projected)
	}
	drivers := filterNodesByRole(state.Nodes, roleDriver)
	targets := filterTargetNodes(state.Nodes)
	if len(targets) == 0 && len(drivers) > 0 {
		targets = fallbackTargetNodesFromOffGraph(state.OffGraph)
	}
	paths := extractSpinePaths(state)
	if len(paths) == 0 && shouldFallbackToRolePaths(state.Spines) {
		paths = extractPaths(state, drivers, targets)
	}
	paths, satiricalCoveredNodes := applySatiricalRenderProjection(state, paths)
	paths = filterCyclicRenderPaths(paths)
	paths = attachOffGraphPremisesToPaths(paths, state.OffGraph)
	drivers = mergePathDrivers(drivers, paths)
	targets = mergePathTargets(targets, paths)
	drivers = filterRenderDrivers(drivers, paths)
	targets = filterRenderTargets(targets, paths, state.ArticleForm, satiricalCoveredNodes)
	declarationNodes := declarationTranslationNodes(state)
	spineNodes := spineTranslationNodes(state.Spines)
	translated, err := translateAll(ctx, rt, model, uniqueTexts(drivers, targets, paths, declarationNodes, spineNodes, state.OffGraph))
	if err != nil {
		return Output{}, err
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
	transmission := make([]TransmissionPath, 0, len(paths))
	for _, p := range paths {
		transmission = append(transmission, renderPathToTransmission(p, cn))
	}
	declarations := appendSourceDeclarations(bundle, renderDeclarationsFromSpines(bundle, state, cn))
	branches := attachDeclarationsToBranches(renderBranchesFromSpines(state.Spines, paths, cn), state.Spines, declarations)
	topics := renderBranchTopics(branches)
	evidence, explanation, supplementary := renderOffGraph(state.OffGraph, cn)
	detailItems := renderOffGraphDetails(state.OffGraph, cn)
	detailItems = append(detailItems, renderTransmissionPathDetails(paths, cn)...)
	detailItems = append(detailItems, renderSemanticUnitDetails(state.SemanticUnits)...)
	evidence = dedupeStrings(append(evidence, renderSpineIllustrations(state, cn)...))
	summary, err := summarizeChinese(ctx, rt, model, state.ArticleForm, driversOut, targetsOut, transmission, declarations, state.SemanticUnits, topics, bundle)
	if err != nil {
		summary = fallbackSummary(driversOut, targetsOut)
	}
	if strings.TrimSpace(summary) == "" {
		summary = fallbackSummary(driversOut, targetsOut)
	}
	summary = prioritizeDeclarationSummary(summary, state.Spines, declarations)
	summary = prioritizeSemanticSummary(summary, state.SemanticUnits, state.ArticleForm)
	summary = compactSummaryForDisplay(summary, state.ArticleForm, declarations, state.SemanticUnits)
	graph := ReasoningGraph{}
	observedAt := bundle.PostedAt
	if observedAt.IsZero() {
		observedAt = NowUTC()
	}
	for _, n := range state.Nodes {
		kind := NodeMechanism
		form := NodeFormObservation
		function := NodeFunctionTransmission
		switch n.Role {
		case roleDriver:
			kind = NodeMechanism
			form = NodeFormObservation
			function = NodeFunctionTransmission
		case roleTransmission:
			kind = NodeMechanism
			form = NodeFormObservation
			function = NodeFunctionTransmission
		}
		if n.IsTarget {
			kind = NodeConclusion
			form = NodeFormJudgment
			function = NodeFunctionClaim
			if n.Ontology == "flow" {
				kind = NodeConclusion
			}
		}
		graph.Nodes = append(graph.Nodes, GraphNode{
			ID:         n.ID,
			Kind:       kind,
			Form:       form,
			Function:   function,
			Text:       cn(n.ID, n.Text),
			OccurredAt: observedAt,
		})
	}
	for _, e := range state.Edges {
		graph.Edges = append(graph.Edges, GraphEdge{From: e.From, To: e.To, Kind: EdgePositive})
	}
	out := Output{
		Summary:            summary,
		Drivers:            driversOut,
		Targets:            targetsOut,
		Declarations:       declarations,
		SemanticUnits:      append([]SemanticUnit(nil), state.SemanticUnits...),
		Brief:              append([]BriefItem(nil), state.Brief...),
		TransmissionPaths:  transmission,
		Branches:           branches,
		EvidenceNodes:      evidence,
		ExplanationNodes:   explanation,
		SupplementaryNodes: supplementary,
		Graph:              graph,
		Details:            HiddenDetails{Caveats: []string{"compile mainline mvp"}, Items: detailItems},
		Topics:             topics,
		Confidence:         confidenceFromState(driversOut, targetsOut, transmission),
	}
	if len(graph.Nodes) < 2 || len(graph.Edges) < 1 {
		fallback := renderLowStructureOutput(bundle, summary, lowStructureSourceText(bundle, state, cn), observedAt, detailItems)
		out.Graph = fallback.Graph
		out.Details.Caveats = appendUniqueNonEmptyStep(out.Details.Caveats, "low-structure content fallback")
		if isLowStructureRenderedOutput(out) {
			out.Summary = fallback.Summary
			out.Confidence = "low"
		}
	}
	return out, nil
}

func renderLowStructureOutput(bundle Bundle, summary, sourceText string, observedAt time.Time, detailItems []map[string]any) Output {
	summary = strings.TrimSpace(summary)
	sourceText = truncateRunes(strings.TrimSpace(sourceText), 180)
	if sourceText == "" {
		sourceText = truncateRunes(strings.TrimSpace(bundle.Content), 180)
	}
	if sourceText == "" {
		sourceText = "原文内容未形成可稳定抽取的因果主线。"
	}
	if summary == "" {
		summary = sourceText
	}
	if observedAt.IsZero() {
		observedAt = NowUTC()
	}
	return Output{
		Summary: summary,
		Graph: ReasoningGraph{
			Nodes: []GraphNode{
				{
					ID:         "low_structure_source",
					Kind:       NodeFact,
					Form:       NodeFormObservation,
					Function:   NodeFunctionSupport,
					Text:       sourceText,
					OccurredAt: observedAt,
				},
				{
					ID:       "low_structure_conclusion",
					Kind:     NodeConclusion,
					Form:     NodeFormJudgment,
					Function: NodeFunctionClaim,
					Text:     "该内容主要是个人动态或低结构信息，未形成稳定的因果主线。",
				},
			},
			Edges: []GraphEdge{{
				From: "low_structure_source",
				To:   "low_structure_conclusion",
				Kind: EdgeExplains,
			}},
		},
		Details:    HiddenDetails{Caveats: []string{"low-structure content fallback"}, Items: detailItems},
		Confidence: "low",
	}
}

func lowStructureSourceText(bundle Bundle, state graphState, cn func(string, string) string) string {
	for _, node := range state.Nodes {
		if text := strings.TrimSpace(cn(node.ID, node.Text)); text != "" {
			return text
		}
	}
	for _, item := range state.OffGraph {
		if text := strings.TrimSpace(cn(item.ID, item.Text)); text != "" {
			return text
		}
	}
	return bundle.Content
}

func isLowStructureRenderedOutput(out Output) bool {
	return len(out.Drivers) == 0 &&
		len(out.Targets) == 0 &&
		len(out.Declarations) == 0 &&
		len(out.TransmissionPaths) == 0 &&
		len(out.Branches) == 0
}

func truncateRunes(value string, limit int) string {
	value = strings.TrimSpace(value)
	if limit <= 0 || value == "" {
		return ""
	}
	runes := []rune(value)
	if len(runes) <= limit {
		return value
	}
	return strings.TrimSpace(string(runes[:limit])) + "..."
}

func renderPathToTransmission(path renderedPath, cn func(string, string) string) TransmissionPath {
	steps := make([]string, 0, len(path.steps))
	for _, step := range path.steps {
		steps = appendUniqueNonEmptyStep(steps, cn(step.ID, step.Text))
	}
	return TransmissionPath{
		Driver: cn(path.driver.ID, path.driver.Text),
		Target: cn(path.target.ID, path.target.Text),
		Steps:  steps,
	}
}

func attachOffGraphPremisesToPaths(paths []renderedPath, offGraph []offGraphItem) []renderedPath {
	if len(paths) == 0 || len(offGraph) == 0 {
		return paths
	}
	premisesByTarget := map[string][]graphNode{}
	for _, item := range offGraph {
		attachTo := strings.TrimSpace(item.AttachesTo)
		text := strings.TrimSpace(item.Text)
		if attachTo == "" || text == "" || !isPathPremiseOffGraphRole(item.Role) {
			continue
		}
		id := strings.TrimSpace(item.ID)
		if id == "" {
			id = "off:" + normalizeText(text)
		}
		premisesByTarget[attachTo] = append(premisesByTarget[attachTo], graphNode{ID: id, Text: text, SourceQuote: strings.TrimSpace(item.SourceQuote)})
	}
	if len(premisesByTarget) == 0 {
		return paths
	}
	out := append([]renderedPath(nil), paths...)
	for i := range out {
		premises := premisesByTarget[strings.TrimSpace(out[i].target.ID)]
		if len(premises) == 0 {
			continue
		}
		out[i].premises = appendDistinctPathPremises(out[i], premises)
	}
	return out
}

func isPathPremiseOffGraphRole(role string) bool {
	switch strings.TrimSpace(role) {
	case "evidence", "inference":
		return true
	default:
		return false
	}
}

func appendDistinctPathPremises(path renderedPath, premises []graphNode) []graphNode {
	seen := map[string]struct{}{
		normalizeText(path.driver.Text): {},
		normalizeText(path.target.Text): {},
	}
	for _, step := range path.steps {
		seen[normalizeText(step.Text)] = struct{}{}
	}
	out := make([]graphNode, 0, len(premises))
	for _, premise := range premises {
		key := normalizeText(premise.Text)
		if key == "" {
			continue
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, premise)
	}
	return out
}

func appendUniqueNonEmptyStep(steps []string, value string) []string {
	value = strings.TrimSpace(value)
	if value == "" {
		return steps
	}
	for _, existing := range steps {
		if normalizeText(existing) == normalizeText(value) {
			return steps
		}
	}
	return append(steps, value)
}
