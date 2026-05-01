package compilev2

import (
	"context"
	"strings"

	"github.com/kumaloha/VariX/varix/compile"
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
