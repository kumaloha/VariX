package model

import (
	"fmt"
	"strings"
	"time"
)

const CompileBridgeVersion = "legacy_compile_bridge_v1"

func FromCompileRecord(record Record) (ContentSubgraph, error) {
	source := strings.TrimSpace(record.Source)
	externalID := strings.TrimSpace(record.ExternalID)
	if source == "" || externalID == "" {
		return ContentSubgraph{}, fmt.Errorf("compile record source/external id are required")
	}
	articleID := strings.TrimSpace(record.UnitID)
	if articleID == "" {
		articleID = source + ":" + externalID
	}
	compiledAt := record.CompiledAt.UTC()
	if compiledAt.IsZero() {
		compiledAt = NowUTC()
	}
	roleByText := compileRoleByText(record.Output)
	primaryTexts := compilePrimaryTexts(record.Output)
	verificationByNodeID := compileVerificationByNodeID(record.Output.Verification)
	nodes := make([]ContentNode, 0, len(record.Output.Graph.Nodes))
	for _, node := range record.Output.Graph.Nodes {
		timeStart, timeEnd, timeText := compileNodeTimeWindow(node)
		mapped := ContentNode{
			ID:                 strings.TrimSpace(node.ID),
			SourceArticleID:    articleID,
			SourcePlatform:     source,
			SourceExternalID:   externalID,
			SourceQuote:        strings.TrimSpace(node.Text),
			RawText:            strings.TrimSpace(node.Text),
			SubjectText:        strings.TrimSpace(node.Text),
			ChangeText:         strings.TrimSpace(node.Text),
			TimeText:           timeText,
			TimeStart:          timeStart,
			TimeEnd:            timeEnd,
			Kind:               mapCompileNodeKind(node.Kind),
			GraphRole:          roleByText[normalizeCompileText(node.Text)],
			IsPrimary:          primaryTexts[normalizeCompileText(node.Text)],
			VerificationStatus: verificationByNodeID[node.ID],
		}
		if mapped.VerificationStatus == "" {
			mapped.VerificationStatus = VerificationPending
		}
		nodes = append(nodes, mapped)
	}
	edges := make([]ContentEdge, 0, len(record.Output.Graph.Edges))
	for _, edge := range record.Output.Graph.Edges {
		mappedType := mapCompileEdgeKind(edge.Kind)
		edges = append(edges, ContentEdge{
			ID:                 fmt.Sprintf("%s->%s:%s", strings.TrimSpace(edge.From), strings.TrimSpace(edge.To), mappedType),
			From:               strings.TrimSpace(edge.From),
			To:                 strings.TrimSpace(edge.To),
			Type:               mappedType,
			IsPrimary:          mappedType == EdgeTypeDrives,
			VerificationStatus: VerificationPending,
		})
	}
	subgraph := ContentSubgraph{
		ID:               articleID,
		ArticleID:        articleID,
		SourcePlatform:   source,
		SourceExternalID: externalID,
		RootExternalID:   strings.TrimSpace(record.RootExternalID),
		Nodes:            nodes,
		Edges:            edges,
		CompileVersion:   CompileBridgeVersion,
		CompiledAt:       compiledAt.Format(time.RFC3339),
		UpdatedAt:        compiledAt.Format(time.RFC3339),
	}
	if err := subgraph.Validate(); err != nil {
		return ContentSubgraph{}, err
	}
	return subgraph, nil
}

func compileRoleByText(output Output) map[string]GraphRole {
	roles := map[string]GraphRole{}
	for _, driver := range output.Drivers {
		roles[normalizeCompileText(driver)] = GraphRoleDriver
	}
	for _, target := range output.Targets {
		roles[normalizeCompileText(target)] = GraphRoleTarget
	}
	for _, path := range output.TransmissionPaths {
		for _, step := range path.Steps {
			key := normalizeCompileText(step)
			if key == "" {
				continue
			}
			if _, ok := roles[key]; !ok {
				roles[key] = GraphRoleIntermediate
			}
		}
	}
	return roles
}

func compileVerificationByNodeID(verification Verification) map[string]VerificationStatus {
	out := map[string]VerificationStatus{}
	for _, item := range verification.NodeVerifications {
		switch item.Status {
		case NodeVerificationProved:
			out[item.NodeID] = VerificationProved
		case NodeVerificationFalsified:
			out[item.NodeID] = VerificationDisproved
		case NodeVerificationWaiting:
			out[item.NodeID] = VerificationPending
		}
	}
	for _, item := range verification.FactChecks {
		if _, ok := out[item.NodeID]; ok {
			continue
		}
		out[item.NodeID] = mapCompileFactStatus(item.Status)
	}
	for _, item := range verification.ImplicitConditionChecks {
		if _, ok := out[item.NodeID]; ok {
			continue
		}
		out[item.NodeID] = mapCompileFactStatus(item.Status)
	}
	for _, item := range verification.PredictionChecks {
		if _, ok := out[item.NodeID]; ok {
			continue
		}
		switch item.Status {
		case PredictionStatusResolvedTrue:
			out[item.NodeID] = VerificationProved
		case PredictionStatusResolvedFalse:
			out[item.NodeID] = VerificationDisproved
		case PredictionStatusUnresolved, PredictionStatusStaleUnresolved:
			out[item.NodeID] = VerificationPending
		}
	}
	for _, item := range verification.ExplicitConditionChecks {
		if _, ok := out[item.NodeID]; ok {
			continue
		}
		out[item.NodeID] = VerificationPending
	}
	return out
}

func mapCompileFactStatus(status FactStatus) VerificationStatus {
	switch status {
	case FactStatusClearlyTrue:
		return VerificationProved
	case FactStatusClearlyFalse:
		return VerificationDisproved
	case FactStatusUnverifiable:
		return VerificationUnverifiable
	default:
		return VerificationPending
	}
}

func compileNodeTimeWindow(node GraphNode) (string, string, string) {
	switch node.Kind {
	case NodePrediction:
		start := firstTime(node.PredictionStartAt, node.ValidFrom)
		end := firstTime(node.PredictionDueAt, node.ValidTo)
		return formatMaybeTime(start), formatMaybeTime(end), timeText(start, end)
	default:
		start := firstTime(node.OccurredAt, node.ValidFrom)
		end := firstTime(node.ValidTo)
		if end.IsZero() && !start.IsZero() {
			end = start
		}
		return formatMaybeTime(start), formatMaybeTime(end), timeText(start, end)
	}
}

func firstTime(values ...time.Time) time.Time {
	for _, value := range values {
		if !value.IsZero() {
			return value.UTC()
		}
	}
	return time.Time{}
}

func formatMaybeTime(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return value.UTC().Format(time.RFC3339)
}

func timeText(start, end time.Time) string {
	if start.IsZero() && end.IsZero() {
		return ""
	}
	startText := formatMaybeTime(start)
	endText := formatMaybeTime(end)
	if !start.IsZero() && !end.IsZero() && start.Equal(end) {
		return startText
	}
	if !start.IsZero() && !end.IsZero() {
		return startText + " -> " + endText
	}
	if !start.IsZero() {
		return startText
	}
	return endText
}

func mapCompileNodeKind(kind NodeKind) ContentNodeKind {
	if kind == NodePrediction {
		return NodeKindPrediction
	}
	return NodeKindObservation
}

func mapCompileEdgeKind(kind EdgeKind) EdgeType {
	switch kind {
	case EdgePositive:
		return EdgeTypeDrives
	case EdgeDerives:
		return EdgeTypeSupports
	case EdgeExplains:
		return EdgeTypeExplains
	case EdgePresets:
		return EdgeTypeContext
	default:
		return EdgeTypeContext
	}
}

func normalizeCompileText(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	return strings.Join(strings.Fields(value), " ")
}

func compilePrimaryTexts(output Output) map[string]bool {
	out := map[string]bool{}
	for _, driver := range output.Drivers {
		out[normalizeCompileText(driver)] = true
	}
	for _, target := range output.Targets {
		out[normalizeCompileText(target)] = true
	}
	for _, path := range output.TransmissionPaths {
		out[normalizeCompileText(path.Driver)] = true
		for _, step := range path.Steps {
			out[normalizeCompileText(step)] = true
		}
		out[normalizeCompileText(path.Target)] = true
	}
	if len(out) == 0 {
		// fallback: preserve previous behavior when no structured path surface exists yet
		return map[string]bool{}
	}
	return out
}
