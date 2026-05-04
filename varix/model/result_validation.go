package model

import (
	"fmt"
	"strings"
)

func (o Output) Validate() error {
	return o.ValidateWithThresholds(2, 1)
}

func (o Output) ValidateWithThresholds(minNodes, minEdges int) error {
	if err := validateRequiredSummary(o.Summary); err != nil {
		return err
	}
	if err := validateStringListEntries("drivers", o.Drivers); err != nil {
		return err
	}
	if err := validateStringListEntries("targets", o.Targets); err != nil {
		return err
	}
	if err := validateDeclarations("declarations", o.Declarations); err != nil {
		return err
	}
	if err := validateSemanticUnits("semantic_units", o.SemanticUnits); err != nil {
		return err
	}
	if err := validateTransmissionPaths("transmission_paths", o.TransmissionPaths, false); err != nil {
		return err
	}
	if err := validateStringListEntries("evidence_nodes", o.EvidenceNodes); err != nil {
		return err
	}
	if err := validateStringListEntries("explanation_nodes", o.ExplanationNodes); err != nil {
		return err
	}
	if err := validateStringListEntries("supplementary_nodes", o.SupplementaryNodes); err != nil {
		return err
	}
	if len(o.Graph.Nodes) < minNodes {
		return fmt.Errorf("graph must contain at least %d nodes", minNodes)
	}
	if len(o.Graph.Edges) < minEdges {
		return fmt.Errorf("graph must contain at least %d edges", minEdges)
	}
	if o.Details.IsEmpty() && len(o.Declarations) == 0 && len(o.SemanticUnits) == 0 && len(o.Brief) == 0 {
		return fmt.Errorf("details must not be empty")
	}
	nodeIDs, err := validateGraphNodes(o.Graph.Nodes)
	if err != nil {
		return err
	}
	if err := validateGraphEdges(o.Graph.Edges, nodeIDs, graphNodeKinds(o.Graph.Nodes), minEdges); err != nil {
		return err
	}
	for _, check := range o.Verification.FactChecks {
		if err := validateKnownNodeIDs("fact check", []string{check.NodeID}, nodeIDs); err != nil {
			return err
		}
		if err := validateFactStatus("fact", check.Status); err != nil {
			return err
		}
	}
	for _, check := range o.Verification.RealizedChecks {
		if err := validateKnownNodeIDs("realized check", []string{check.NodeID}, nodeIDs); err != nil {
			return err
		}
		if err := validateFactStatus("realized", check.Status); err != nil {
			return err
		}
	}
	for _, check := range o.Verification.FutureConditionChecks {
		if err := validateKnownNodeIDs("future condition check", []string{check.NodeID}, nodeIDs); err != nil {
			return err
		}
	}
	for _, check := range o.Verification.ExplicitConditionChecks {
		if err := validateKnownNodeIDs("explicit condition check", []string{check.NodeID}, nodeIDs); err != nil {
			return err
		}
		if err := validateExplicitConditionStatus(check.Status); err != nil {
			return err
		}
	}
	for _, check := range o.Verification.ImplicitConditionChecks {
		if err := validateKnownNodeIDs("implicit condition check", []string{check.NodeID}, nodeIDs); err != nil {
			return err
		}
		if err := validateFactStatus("implicit condition", check.Status); err != nil {
			return err
		}
	}
	for _, check := range o.Verification.PredictionChecks {
		if err := validateKnownNodeIDs("prediction check", []string{check.NodeID}, nodeIDs); err != nil {
			return err
		}
		if err := validatePredictionStatus(check.Status); err != nil {
			return err
		}
	}
	for _, pass := range o.Verification.Passes {
		if err := validateKnownNodeIDs("verification pass", pass.NodeIDs, nodeIDs); err != nil {
			return err
		}
		if err := validateKnownNodeIDs("verification coverage expected", pass.Coverage.ExpectedNodeIDs, nodeIDs); err != nil {
			return err
		}
		if err := validateKnownNodeIDs("verification coverage returned", pass.Coverage.ReturnedNodeIDs, nodeIDs); err != nil {
			return err
		}
		if err := validateKnownNodeIDs("verification coverage missing", pass.Coverage.MissingNodeIDs, nodeIDs); err != nil {
			return err
		}
		if err := validateKnownNodeIDs("verification coverage duplicate", pass.Coverage.DuplicateNodeIDs, nodeIDs); err != nil {
			return err
		}
		if err := validateKnownNodeIDs("verification coverage unexpected", pass.Coverage.UnexpectedNodeIDs, nodeIDs); err != nil {
			return err
		}
		for _, stage := range []*VerificationStageSummary{pass.Claim, pass.Challenge, pass.Adjudication} {
			if stage == nil {
				continue
			}
			if err := validateKnownNodeIDs("verification stage", stage.OutputNodeIDs, nodeIDs); err != nil {
				return err
			}
		}
	}
	if summary := o.Verification.CoverageSummary; summary != nil {
		if err := validateKnownNodeIDs("verification summary missing", summary.MissingNodeIDs, nodeIDs); err != nil {
			return err
		}
		if err := validateKnownNodeIDs("verification summary duplicate", summary.DuplicateNodeIDs, nodeIDs); err != nil {
			return err
		}
		if err := validateKnownNodeIDs("verification summary unexpected", summary.UnexpectedNodeIDs, nodeIDs); err != nil {
			return err
		}
	}
	return nil
}

func (o NodeExtractionOutput) ValidateWithThresholds(minNodes int) error {
	nodeIDs, err := validateGraphNodes(o.Graph.Nodes)
	if err != nil {
		return err
	}
	if len(nodeIDs) < minNodes {
		return fmt.Errorf("graph must contain at least %d nodes", minNodes)
	}
	if len(o.Graph.Edges) > 0 {
		return fmt.Errorf("node extraction output must not contain edges")
	}
	return nil
}

func (o DriverTargetOutput) ValidateGeneratorOrJudge() error {
	if err := validateRequiredStringList("drivers", o.Drivers); err != nil {
		return err
	}
	if err := validateRequiredStringList("targets", o.Targets); err != nil {
		return err
	}
	return validateDetailsPresent(o.Details)
}

func (o DriverTargetOutput) ValidateChallenge() error {
	return validateStringLists(
		stringListField{name: "drivers", values: o.Drivers},
		stringListField{name: "targets", values: o.Targets},
	)
}

func (o FullGraphOutput) ValidateWithThresholds(minEdges int, nodeIDs map[string]struct{}, nodeKinds map[string]NodeKind) error {
	if len(o.Graph.Nodes) > 0 {
		return fmt.Errorf("full graph output must not contain nodes")
	}
	return validateGraphEdges(o.Graph.Edges, nodeIDs, nodeKinds, minEdges)
}

func (o TransmissionPathOutput) ValidateGeneratorOrJudge() error {
	if err := validateTransmissionPaths("transmission_paths", o.TransmissionPaths, true); err != nil {
		return err
	}
	return validateDetailsPresent(o.Details)
}

func (o TransmissionPathOutput) ValidateChallenge() error {
	return validateTransmissionPaths("transmission_paths", o.TransmissionPaths, false)
}

func (o EvidenceExplanationOutput) ValidateGeneratorOrJudge() error {
	if len(o.EvidenceNodes) == 0 && len(o.ExplanationNodes) == 0 && len(o.SupplementaryNodes) == 0 {
		return fmt.Errorf("evidence_nodes, explanation_nodes, and supplementary_nodes must not all be empty")
	}
	if err := validateEvidenceExplanationLists(o); err != nil {
		return err
	}
	return validateDetailsPresent(o.Details)
}

func (o EvidenceExplanationOutput) ValidateChallenge() error {
	return validateEvidenceExplanationLists(o)
}

func (o UnifiedCompileOutput) ValidateGeneratorOrJudge() error {
	if err := validateRequiredSummary(o.Summary); err != nil {
		return err
	}
	if err := validateRequiredStringList("drivers", o.Drivers); err != nil {
		return err
	}
	if err := validateRequiredStringList("targets", o.Targets); err != nil {
		return err
	}
	if err := validateTransmissionPaths("transmission_paths", o.TransmissionPaths, true); err != nil {
		return err
	}
	if err := validateEvidenceExplanationLists(EvidenceExplanationOutput{
		EvidenceNodes:      o.EvidenceNodes,
		ExplanationNodes:   o.ExplanationNodes,
		SupplementaryNodes: o.SupplementaryNodes,
	}); err != nil {
		return err
	}
	return validateDetailsPresent(o.Details)
}

func (o UnifiedCompileOutput) ValidateChallenge() error {
	if strings.TrimSpace(o.Summary) == "" && len(o.Drivers) == 0 && len(o.Targets) == 0 && len(o.TransmissionPaths) == 0 && len(o.EvidenceNodes) == 0 && len(o.ExplanationNodes) == 0 && len(o.SupplementaryNodes) == 0 {
		return fmt.Errorf("challenge output must not be entirely empty")
	}
	if err := validateStringLists(
		stringListField{name: "drivers", values: o.Drivers},
		stringListField{name: "targets", values: o.Targets},
	); err != nil {
		return err
	}
	if err := validateEvidenceExplanationLists(EvidenceExplanationOutput{
		EvidenceNodes:      o.EvidenceNodes,
		ExplanationNodes:   o.ExplanationNodes,
		SupplementaryNodes: o.SupplementaryNodes,
	}); err != nil {
		return err
	}
	if err := validateTransmissionPaths("transmission_paths", o.TransmissionPaths, false); err != nil {
		return err
	}
	return nil
}

func (o ThesisOutput) Validate() error {
	if err := validateRequiredSummary(o.Summary); err != nil {
		return err
	}
	if err := validateStringLists(
		stringListField{name: "drivers", values: o.Drivers},
		stringListField{name: "targets", values: o.Targets},
	); err != nil {
		return err
	}
	return validateDetailsPresent(o.Details)
}

func validateRequiredSummary(summary string) error {
	if strings.TrimSpace(summary) == "" {
		return fmt.Errorf("summary is required")
	}
	return nil
}

func validateKnownNodeIDs(label string, ids []string, nodeIDs map[string]struct{}) error {
	for _, id := range ids {
		if _, ok := nodeIDs[id]; !ok {
			return fmt.Errorf("%s references unknown node: %s", label, id)
		}
	}
	return nil
}

func validateFactStatus(label string, status FactStatus) error {
	switch status {
	case FactStatusClearlyTrue, FactStatusClearlyFalse, FactStatusUnverifiable:
		return nil
	default:
		return fmt.Errorf("unsupported %s status: %s", label, status)
	}
}

func validateExplicitConditionStatus(status ExplicitConditionStatus) error {
	switch status {
	case ExplicitConditionStatusHigh, ExplicitConditionStatusMedium, ExplicitConditionStatusLow, ExplicitConditionStatusUnknown:
		return nil
	default:
		return fmt.Errorf("unsupported explicit condition status: %s", status)
	}
}

func validatePredictionStatus(status PredictionStatus) error {
	switch status {
	case PredictionStatusUnresolved, PredictionStatusResolvedTrue, PredictionStatusResolvedFalse, PredictionStatusStaleUnresolved:
		return nil
	default:
		return fmt.Errorf("unsupported prediction status: %s", status)
	}
}

func validateRequiredStringList(field string, values []string) error {
	if len(values) == 0 {
		return fmt.Errorf("%s must not be empty", field)
	}
	return validateStringListEntries(field, values)
}

func validateStringLists(fields ...stringListField) error {
	for _, field := range fields {
		if err := validateStringListEntries(field.name, field.values); err != nil {
			return err
		}
	}
	return nil
}

func validateEvidenceExplanationLists(o EvidenceExplanationOutput) error {
	return validateStringLists(
		stringListField{name: "evidence_nodes", values: o.EvidenceNodes},
		stringListField{name: "explanation_nodes", values: o.ExplanationNodes},
		stringListField{name: "supplementary_nodes", values: o.SupplementaryNodes},
	)
}

func validateDetailsPresent(details HiddenDetails) error {
	if details.IsEmpty() {
		return fmt.Errorf("details must not be empty")
	}
	return nil
}

func validateStringListEntries(field string, values []string) error {
	for i, value := range values {
		if strings.TrimSpace(value) == "" {
			return fmt.Errorf("%s[%d] must not be empty", field, i)
		}
	}
	return nil
}

func validateTransmissionPaths(field string, paths []TransmissionPath, requireAtLeastOne bool) error {
	if requireAtLeastOne && len(paths) == 0 {
		return fmt.Errorf("%s must not be empty", field)
	}
	for i, path := range paths {
		if strings.TrimSpace(path.Driver) == "" {
			return fmt.Errorf("%s[%d].driver must not be empty", field, i)
		}
		if strings.TrimSpace(path.Target) == "" {
			return fmt.Errorf("%s[%d].target must not be empty", field, i)
		}
		if err := validateStringListEntries(fmt.Sprintf("%s[%d].steps", field, i), path.Steps); err != nil {
			return err
		}
	}
	return nil
}

func validateDeclarations(field string, values []Declaration) error {
	for i, declaration := range values {
		if strings.TrimSpace(declaration.Statement) == "" {
			return fmt.Errorf("%s[%d].statement must not be empty", field, i)
		}
		if err := validateStringListEntries(fmt.Sprintf("%s[%d].conditions", field, i), declaration.Conditions); err != nil {
			return err
		}
		if err := validateStringListEntries(fmt.Sprintf("%s[%d].actions", field, i), declaration.Actions); err != nil {
			return err
		}
		if err := validateStringListEntries(fmt.Sprintf("%s[%d].constraints", field, i), declaration.Constraints); err != nil {
			return err
		}
		if err := validateStringListEntries(fmt.Sprintf("%s[%d].non_actions", field, i), declaration.NonActions); err != nil {
			return err
		}
		if err := validateStringListEntries(fmt.Sprintf("%s[%d].evidence", field, i), declaration.Evidence); err != nil {
			return err
		}
	}
	return nil
}

func validateSemanticUnits(field string, values []SemanticUnit) error {
	seen := map[string]struct{}{}
	for i, unit := range values {
		if strings.TrimSpace(unit.ID) == "" {
			return fmt.Errorf("%s[%d].id must not be empty", field, i)
		}
		if _, ok := seen[unit.ID]; ok {
			return fmt.Errorf("duplicate %s id %q", field, unit.ID)
		}
		seen[unit.ID] = struct{}{}
		if strings.TrimSpace(unit.Subject) == "" {
			return fmt.Errorf("%s[%d].subject must not be empty", field, i)
		}
		if strings.TrimSpace(unit.Claim) == "" {
			return fmt.Errorf("%s[%d].claim must not be empty", field, i)
		}
		if role := strings.TrimSpace(unit.SpeakerRole); role != "" && !isAllowedSemanticSpeakerRole(role) {
			return fmt.Errorf("%s[%d].speaker_role %q is unsupported", field, i, role)
		}
		if force := strings.TrimSpace(unit.Force); force != "" && !isAllowedSemanticForce(force) {
			return fmt.Errorf("%s[%d].force %q is unsupported", field, i, force)
		}
		if unit.Salience < 0 || unit.Salience > 1 {
			return fmt.Errorf("%s[%d].salience must be between 0 and 1", field, i)
		}
	}
	return nil
}

func isAllowedSemanticSpeakerRole(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "primary", "named_participant", "questioner", "unknown":
		return true
	default:
		return false
	}
}

func isAllowedSemanticForce(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "assert", "report", "explain", "answer", "commit", "reject", "disclose", "frame_risk", "set_boundary":
		return true
	default:
		return false
	}
}

func validateGraphNodes(nodes []GraphNode) (map[string]struct{}, error) {
	nodeIDs := map[string]struct{}{}
	for _, node := range nodes {
		normalized, err := node.normalizedSchema()
		if err != nil {
			return nil, err
		}
		node = normalized
		if strings.TrimSpace(node.ID) == "" {
			return nil, fmt.Errorf("graph node id is required")
		}
		if _, ok := nodeIDs[node.ID]; ok {
			return nil, fmt.Errorf("duplicate graph node id %q", node.ID)
		}
		if strings.TrimSpace(node.Text) == "" {
			return nil, fmt.Errorf("graph node text is required")
		}
		switch node.Kind {
		case NodeFact, NodeExplicitCondition, NodeImplicitCondition, NodeMechanism, NodeConclusion, NodePrediction:
		default:
			return nil, fmt.Errorf("unsupported node kind: %s", node.Kind)
		}
		if err := validateNodeTiming(node); err != nil {
			return nil, err
		}
		nodeIDs[node.ID] = struct{}{}
	}
	return nodeIDs, nil
}

func graphNodeKinds(nodes []GraphNode) map[string]NodeKind {
	out := make(map[string]NodeKind, len(nodes))
	for _, node := range nodes {
		if normalized, err := node.normalizedSchema(); err == nil {
			node = normalized
		}
		out[node.ID] = node.Kind
	}
	return out
}

func validateGraphEdges(edges []GraphEdge, nodeIDs map[string]struct{}, nodeKinds map[string]NodeKind, minEdges int) error {
	if len(edges) < minEdges {
		return fmt.Errorf("graph must contain at least %d edges", minEdges)
	}
	for _, edge := range edges {
		if !HasDistinctNonEmptyPair(edge.From, edge.To) || strings.TrimSpace(string(edge.Kind)) == "" {
			return fmt.Errorf("graph edge has empty required field: from=%q to=%q kind=%q", edge.From, edge.To, edge.Kind)
		}
		if _, ok := nodeIDs[edge.From]; !ok {
			return fmt.Errorf("edge from references unknown node: %s", edge.From)
		}
		if _, ok := nodeIDs[edge.To]; !ok {
			return fmt.Errorf("edge to references unknown node: %s", edge.To)
		}
		switch edge.Kind {
		case EdgePositive, EdgeDerives, EdgePresets, EdgeExplains:
		default:
			return fmt.Errorf("unsupported edge kind for edge %s->%s: %q", edge.From, edge.To, edge.Kind)
		}
		if edge.Kind == EdgePresets {
			sourceKind, ok := nodeKinds[edge.From]
			if !ok {
				return fmt.Errorf("edge from references unknown node: %s", edge.From)
			}
			if sourceKind != NodeExplicitCondition && sourceKind != NodeImplicitCondition {
				return fmt.Errorf("preset edge must start from a condition node: %s", edge.From)
			}
		}
	}
	return nil
}

func validateNodeTiming(node GraphNode) error {
	if !node.ValidFrom.IsZero() || !node.ValidTo.IsZero() {
		if node.ValidFrom.IsZero() || node.ValidTo.IsZero() {
			return fmt.Errorf("graph node validity window is incomplete: %s", node.ID)
		}
		if node.ValidTo.Before(node.ValidFrom) {
			return fmt.Errorf("graph node validity window is invalid: %s", node.ID)
		}
	}
	switch node.Kind {
	case NodeFact, NodeImplicitCondition, NodeMechanism:
		if node.OccurredAt.IsZero() && node.ValidFrom.IsZero() {
			return fmt.Errorf("graph node fact timing is required: %s", node.ID)
		}
	case NodePrediction:
		if node.PredictionStartAt.IsZero() && node.ValidFrom.IsZero() {
			return fmt.Errorf("graph node prediction start is required: %s", node.ID)
		}
		if !node.PredictionDueAt.IsZero() && !node.PredictionStartAt.IsZero() && node.PredictionDueAt.Before(node.PredictionStartAt) {
			return fmt.Errorf("graph node prediction window is invalid: %s", node.ID)
		}
		if !node.PredictionDueAt.IsZero() && node.PredictionStartAt.IsZero() && node.ValidFrom.IsZero() {
			return fmt.Errorf("graph node prediction due requires start: %s", node.ID)
		}
	}
	return nil
}
