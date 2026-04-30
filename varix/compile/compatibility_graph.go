package compile

import (
	"fmt"
	"strings"
)

func buildCompatibilityGraph(bundle Bundle, driverTarget DriverTargetOutput, paths TransmissionPathOutput, aux EvidenceExplanationOutput) ReasoningGraph {
	graph := ReasoningGraph{}
	keyToID := map[string]string{}
	nextID := 1
	now := bundle.PostedAt.UTC()
	if bundle.PostedAt.IsZero() {
		now = NowUTC()
	}

	addNode := func(kind NodeKind, text string) string {
		normalizedText := strings.TrimSpace(text)
		if normalizedText == "" {
			return ""
		}
		key := string(kind) + "|" + strings.ToLower(strings.Join(strings.Fields(normalizedText), " "))
		if existing, ok := keyToID[key]; ok {
			return existing
		}
		id := fmt.Sprintf("n%d", nextID)
		nextID++
		node := GraphNode{ID: id, Kind: kind, Text: normalizedText}
		switch kind {
		case NodeFact, NodeMechanism, NodeImplicitCondition:
			node.OccurredAt = now
		case NodePrediction:
			node.PredictionStartAt = now
		}
		graph.Nodes = append(graph.Nodes, node)
		keyToID[key] = id
		return id
	}

	addEdge := func(from, to string, kind EdgeKind) {
		if !HasDistinctNonEmptyPair(from, to) {
			return
		}
		candidate := GraphEdge{From: from, To: to, Kind: kind}
		for _, existing := range graph.Edges {
			if existing == candidate {
				return
			}
		}
		graph.Edges = append(graph.Edges, candidate)
	}

	for _, driver := range driverTarget.Drivers {
		addNode(NodeMechanism, driver)
	}

	targetNodeIDs := make([]string, 0, len(driverTarget.Targets))
	for _, target := range driverTarget.Targets {
		targetNodeIDs = append(targetNodeIDs, addNode(NodeConclusion, target))
	}
	primaryTargetID := ""
	if len(targetNodeIDs) > 0 {
		primaryTargetID = targetNodeIDs[0]
	}

	for _, path := range paths.TransmissionPaths {
		driverID := addNode(NodeMechanism, path.Driver)
		lastID := driverID
		for _, step := range path.Steps {
			stepID := addNode(NodeMechanism, step)
			if lastID != "" && stepID != "" && lastID != stepID {
				addEdge(lastID, stepID, EdgePositive)
			}
			if stepID != "" {
				lastID = stepID
			}
		}
		targetID := addNode(NodeConclusion, path.Target)
		if lastID == "" {
			lastID = driverID
		}
		addEdge(lastID, targetID, EdgePositive)
	}

	for _, evidence := range aux.EvidenceNodes {
		if hasNormalizedMechanismText(driverTarget.Drivers, paths.TransmissionPaths, evidence) {
			continue
		}
		evidenceID := addNode(NodeFact, evidence)
		if primaryTargetID != "" {
			addEdge(evidenceID, primaryTargetID, EdgeDerives)
		}
	}

	for _, explanation := range aux.ExplanationNodes {
		explanationID := addNode(NodeConclusion, explanation)
		if primaryTargetID != "" {
			addEdge(explanationID, primaryTargetID, EdgeExplains)
		}
	}

	for _, supplementary := range aux.SupplementaryNodes {
		supplementaryID := addNode(NodeConclusion, supplementary)
		if primaryTargetID != "" {
			addEdge(supplementaryID, primaryTargetID, EdgeExplains)
		}
	}

	ensureMinimumCompatibilityGraph(&graph, bundle, driverTarget, paths, aux, addNode, addEdge)
	applyBundleTimingFallbacks(bundle, &graph)
	return graph
}

func ensureMinimumCompatibilityGraph(
	graph *ReasoningGraph,
	bundle Bundle,
	driverTarget DriverTargetOutput,
	paths TransmissionPathOutput,
	aux EvidenceExplanationOutput,
	addNode func(kind NodeKind, text string) string,
	addEdge func(from, to string, kind EdgeKind),
) {
	if graph == nil {
		return
	}
	if len(graph.Nodes) >= 2 && len(graph.Edges) >= 1 {
		return
	}

	driverText := firstNonEmptyTrimmed(
		firstString(driverTarget.Drivers),
		firstPathDriver(paths.TransmissionPaths),
		firstString(aux.ExplanationNodes),
		strings.TrimSpace(bundle.Content),
		"primary driver",
	)
	targetText := firstNonEmptyTrimmed(
		firstString(driverTarget.Targets),
		firstPathTarget(paths.TransmissionPaths),
		firstString(aux.EvidenceNodes),
		strings.TrimSpace(bundle.Content),
		"primary target",
	)

	driverID := addNode(NodeMechanism, driverText)
	targetID := addNode(NodeConclusion, targetText)
	addEdge(driverID, targetID, EdgePositive)

	if len(graph.Nodes) < 2 {
		evidenceID := addNode(NodeFact, firstNonEmptyTrimmed(firstString(aux.EvidenceNodes), targetText, "supporting evidence"))
		addEdge(evidenceID, targetID, EdgeDerives)
	}
}

func firstString(values []string) string {
	if len(values) == 0 {
		return ""
	}
	return values[0]
}

func firstPathDriver(paths []TransmissionPath) string {
	if len(paths) == 0 {
		return ""
	}
	return paths[0].Driver
}

func firstPathTarget(paths []TransmissionPath) string {
	if len(paths) == 0 {
		return ""
	}
	return paths[0].Target
}

func firstNonEmptyTrimmed(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func appendUniqueNormalized(base []string, values ...string) []string {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		found := false
		for _, existing := range base {
			if normalizeLooseText(existing) == normalizeLooseText(value) {
				found = true
				break
			}
		}
		if !found {
			base = append(base, value)
		}
	}
	return base
}

func findNodeText(nodes []GraphNode, id string) string {
	for _, node := range nodes {
		if node.ID == id {
			return strings.TrimSpace(node.Text)
		}
	}
	return ""
}

func firstNormalizedOrFallback(values []string, fallback string) string {
	if len(values) > 0 && strings.TrimSpace(values[0]) != "" {
		return strings.TrimSpace(values[0])
	}
	return strings.TrimSpace(fallback)
}

func normalizeLooseText(text string) string {
	return strings.ToLower(strings.Join(strings.Fields(strings.TrimSpace(text)), " "))
}

func hasNormalizedMechanismText(drivers []string, paths []TransmissionPath, evidence string) bool {
	target := normalizeLooseText(evidence)
	for _, driver := range drivers {
		if normalizeLooseText(driver) == target {
			return true
		}
	}
	for _, path := range paths {
		for _, step := range path.Steps {
			if normalizeLooseText(step) == target {
				return true
			}
		}
	}
	return false
}
