package graphmodel

import (
	"fmt"
	"strings"
	"time"

	"github.com/kumaloha/VariX/varix/compile"
)

const CompileBridgeVersion = "legacy_compile_bridge_v1"

func FromCompileRecord(record compile.Record) (ContentSubgraph, error) {
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
		compiledAt = compile.NowUTC()
	}
	roleByText := compileRoleByText(record.Output)
	primaryTexts := compilePrimaryTexts(record.Output)
	verificationByNodeID := compileVerificationByNodeID(record.Output.Verification)
	nodes := make([]GraphNode, 0, len(record.Output.Graph.Nodes))
	for _, node := range record.Output.Graph.Nodes {
		timeStart, timeEnd, timeText := compileNodeTimeWindow(node)
		mapped := GraphNode{
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
			GraphRole:          roleByText[normalizeLegacyText(node.Text)],
			IsPrimary:          primaryTexts[normalizeLegacyText(node.Text)],
			VerificationStatus: verificationByNodeID[node.ID],
		}
		if mapped.VerificationStatus == "" {
			mapped.VerificationStatus = VerificationPending
		}
		nodes = append(nodes, mapped)
	}
	edges := make([]GraphEdge, 0, len(record.Output.Graph.Edges))
	for _, edge := range record.Output.Graph.Edges {
		mappedType := mapCompileEdgeKind(edge.Kind)
		edges = append(edges, GraphEdge{
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

func compileRoleByText(output compile.Output) map[string]GraphRole {
	roles := map[string]GraphRole{}
	for _, driver := range output.Drivers {
		roles[normalizeLegacyText(driver)] = GraphRoleDriver
	}
	for _, target := range output.Targets {
		roles[normalizeLegacyText(target)] = GraphRoleTarget
	}
	for _, path := range output.TransmissionPaths {
		for _, step := range path.Steps {
			key := normalizeLegacyText(step)
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

func compileVerificationByNodeID(verification compile.Verification) map[string]VerificationStatus {
	out := map[string]VerificationStatus{}
	for _, item := range verification.NodeVerifications {
		switch item.Status {
		case compile.NodeVerificationProved:
			out[item.NodeID] = VerificationProved
		case compile.NodeVerificationFalsified:
			out[item.NodeID] = VerificationDisproved
		case compile.NodeVerificationWaiting:
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
		case compile.PredictionStatusResolvedTrue:
			out[item.NodeID] = VerificationProved
		case compile.PredictionStatusResolvedFalse:
			out[item.NodeID] = VerificationDisproved
		case compile.PredictionStatusUnresolved, compile.PredictionStatusStaleUnresolved:
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

func mapCompileFactStatus(status compile.FactStatus) VerificationStatus {
	switch status {
	case compile.FactStatusClearlyTrue:
		return VerificationProved
	case compile.FactStatusClearlyFalse:
		return VerificationDisproved
	case compile.FactStatusUnverifiable:
		return VerificationUnverifiable
	default:
		return VerificationPending
	}
}

func compileNodeTimeWindow(node compile.GraphNode) (string, string, string) {
	switch node.Kind {
	case compile.NodePrediction:
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
	if !start.IsZero() && !end.IsZero() && start.Equal(end) {
		return start.UTC().Format(time.RFC3339)
	}
	if !start.IsZero() && !end.IsZero() {
		return start.UTC().Format(time.RFC3339) + " -> " + end.UTC().Format(time.RFC3339)
	}
	if !start.IsZero() {
		return start.UTC().Format(time.RFC3339)
	}
	return end.UTC().Format(time.RFC3339)
}

func mapCompileNodeKind(kind compile.NodeKind) NodeKind {
	if kind == compile.NodePrediction {
		return NodeKindPrediction
	}
	return NodeKindObservation
}

func mapCompileEdgeKind(kind compile.EdgeKind) EdgeType {
	switch kind {
	case compile.EdgePositive:
		return EdgeTypeDrives
	case compile.EdgeDerives:
		return EdgeTypeSupports
	case compile.EdgeExplains:
		return EdgeTypeExplains
	case compile.EdgePresets:
		return EdgeTypeContext
	default:
		return EdgeTypeContext
	}
}

func normalizeLegacyText(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	return strings.Join(strings.Fields(value), " ")
}

func compilePrimaryTexts(output compile.Output) map[string]bool {
	out := map[string]bool{}
	for _, driver := range output.Drivers {
		out[normalizeLegacyText(driver)] = true
	}
	for _, target := range output.Targets {
		out[normalizeLegacyText(target)] = true
	}
	for _, path := range output.TransmissionPaths {
		out[normalizeLegacyText(path.Driver)] = true
		for _, step := range path.Steps {
			out[normalizeLegacyText(step)] = true
		}
		out[normalizeLegacyText(path.Target)] = true
	}
	if len(out) == 0 {
		// fallback: preserve previous behavior when no structured path surface exists yet
		return map[string]bool{}
	}
	return out
}
