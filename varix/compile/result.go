package compile

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

var openEndedNodeTime = time.Date(9999, 12, 31, 23, 59, 59, 0, time.UTC)

type NodeKind string

const (
	NodeFact              NodeKind = "事实"
	NodeExplicitCondition NodeKind = "显式条件"
	NodeImplicitCondition NodeKind = "隐含条件"
	NodeAssumption        NodeKind = NodeImplicitCondition // backward-compatible alias
	NodeConclusion        NodeKind = "结论"
	NodePrediction        NodeKind = "预测"
)

type EdgeKind string

const (
	EdgePositive EdgeKind = "正向"
	EdgeDerives  EdgeKind = "推出"
	EdgePresets  EdgeKind = "预设"
)

type GraphNode struct {
	ID                string    `json:"id"`
	Kind              NodeKind  `json:"kind"`
	Text              string    `json:"text"`
	ValidFrom         time.Time `json:"valid_from,omitempty"`
	ValidTo           time.Time `json:"valid_to,omitempty"`
	OccurredAt        time.Time `json:"occurred_at,omitempty"`
	PredictionStartAt time.Time `json:"prediction_start_at,omitempty"`
	PredictionDueAt   time.Time `json:"prediction_due_at,omitempty"`
}

func (n GraphNode) MarshalJSON() ([]byte, error) {
	type graphNodePayload struct {
		ID                string     `json:"id"`
		Kind              NodeKind   `json:"kind"`
		Text              string     `json:"text"`
		ValidFrom         *time.Time `json:"valid_from,omitempty"`
		ValidTo           *time.Time `json:"valid_to,omitempty"`
		OccurredAt        *time.Time `json:"occurred_at,omitempty"`
		PredictionStartAt *time.Time `json:"prediction_start_at,omitempty"`
		PredictionDueAt   *time.Time `json:"prediction_due_at,omitempty"`
	}
	payload := graphNodePayload{
		ID:   n.ID,
		Kind: n.Kind,
		Text: n.Text,
	}
	switch n.Kind {
	case NodeFact, NodeImplicitCondition:
		if !n.OccurredAt.IsZero() {
			t := n.OccurredAt
			payload.OccurredAt = &t
			break
		}
		if !n.ValidFrom.IsZero() {
			t := n.ValidFrom
			payload.ValidFrom = &t
		}
		if !n.ValidTo.IsZero() {
			t := n.ValidTo
			payload.ValidTo = &t
		}
	case NodePrediction:
		if !n.PredictionStartAt.IsZero() {
			t := n.PredictionStartAt
			payload.PredictionStartAt = &t
		} else if !n.ValidFrom.IsZero() {
			t := n.ValidFrom
			payload.ValidFrom = &t
		}
		if !n.PredictionDueAt.IsZero() {
			t := n.PredictionDueAt
			payload.PredictionDueAt = &t
		} else if !n.ValidTo.IsZero() {
			t := n.ValidTo
			payload.ValidTo = &t
		}
	default:
		if !n.ValidFrom.IsZero() {
			t := n.ValidFrom
			payload.ValidFrom = &t
		}
		if !n.ValidTo.IsZero() {
			t := n.ValidTo
			payload.ValidTo = &t
		}
	}
	return json.Marshal(payload)
}

func (n GraphNode) LegacyValidityWindow() (time.Time, time.Time) {
	if !n.ValidFrom.IsZero() && !n.ValidTo.IsZero() {
		return n.ValidFrom, n.ValidTo
	}
	switch n.Kind {
	case NodeFact, NodeImplicitCondition:
		if !n.OccurredAt.IsZero() {
			return n.OccurredAt, openEndedNodeTime
		}
	case NodePrediction:
		if !n.PredictionStartAt.IsZero() {
			if !n.PredictionDueAt.IsZero() {
				return n.PredictionStartAt, n.PredictionDueAt
			}
			return n.PredictionStartAt, openEndedNodeTime
		}
	}
	return time.Time{}, time.Time{}
}

func (n *GraphNode) UnmarshalJSON(data []byte) error {
	type alias GraphNode
	var aux struct {
		alias
		Content string `json:"content"`
	}
	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}
	*n = GraphNode(aux.alias)
	if strings.TrimSpace(n.Text) == "" {
		n.Text = aux.Content
	}
	return nil
}

type GraphEdge struct {
	From string   `json:"from"`
	To   string   `json:"to"`
	Kind EdgeKind `json:"kind"`
}

func (e *GraphEdge) UnmarshalJSON(data []byte) error {
	type alias GraphEdge
	var aux struct {
		alias
		Source string `json:"source"`
		Target string `json:"target"`
	}
	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}
	*e = GraphEdge(aux.alias)
	if strings.TrimSpace(e.From) == "" {
		e.From = aux.Source
	}
	if strings.TrimSpace(e.To) == "" {
		e.To = aux.Target
	}
	return nil
}

type ReasoningGraph struct {
	Nodes []GraphNode `json:"nodes,omitempty"`
	Edges []GraphEdge `json:"edges,omitempty"`
}

type HiddenDetails struct {
	QuoteHighlights     []string         `json:"quote_highlights,omitempty"`
	ReferenceHighlights []string         `json:"reference_highlights,omitempty"`
	AttachmentNotes     []string         `json:"attachment_notes,omitempty"`
	Caveats             []string         `json:"caveats,omitempty"`
	Items               []map[string]any `json:"items,omitempty"`
}

func (d HiddenDetails) IsEmpty() bool {
	return len(d.QuoteHighlights) == 0 &&
		len(d.ReferenceHighlights) == 0 &&
		len(d.AttachmentNotes) == 0 &&
		len(d.Caveats) == 0 &&
		len(d.Items) == 0
}

type FactStatus string

const (
	FactStatusClearlyTrue  FactStatus = "clearly_true"
	FactStatusClearlyFalse FactStatus = "clearly_false"
	FactStatusUnverifiable FactStatus = "unverifiable"
)

type ExplicitConditionStatus string

const (
	ExplicitConditionStatusHigh    ExplicitConditionStatus = "high"
	ExplicitConditionStatusMedium  ExplicitConditionStatus = "medium"
	ExplicitConditionStatusLow     ExplicitConditionStatus = "low"
	ExplicitConditionStatusUnknown ExplicitConditionStatus = "unknown"
)

type PredictionStatus string

const (
	PredictionStatusUnresolved      PredictionStatus = "unresolved"
	PredictionStatusResolvedTrue    PredictionStatus = "resolved_true"
	PredictionStatusResolvedFalse   PredictionStatus = "resolved_false"
	PredictionStatusStaleUnresolved PredictionStatus = "stale_unresolved"
)

type FactCheck struct {
	NodeID string     `json:"node_id"`
	Status FactStatus `json:"status"`
	Reason string     `json:"reason,omitempty"`
}

type ExplicitConditionCheck struct {
	NodeID string                  `json:"node_id"`
	Status ExplicitConditionStatus `json:"status"`
	Reason string                  `json:"reason,omitempty"`
}

type ImplicitConditionCheck struct {
	NodeID string     `json:"node_id"`
	Status FactStatus `json:"status"`
	Reason string     `json:"reason,omitempty"`
}

type PredictionCheck struct {
	NodeID string           `json:"node_id"`
	Status PredictionStatus `json:"status"`
	Reason string           `json:"reason,omitempty"`
	AsOf   time.Time        `json:"as_of,omitempty"`
}

type VerificationPassKind string

const (
	VerificationPassFact              VerificationPassKind = "fact"
	VerificationPassExplicitCondition VerificationPassKind = "explicit_condition"
	VerificationPassImplicitCondition VerificationPassKind = "implicit_condition"
	VerificationPassPrediction        VerificationPassKind = "prediction"
)

type VerificationStageSummary struct {
	Model         string    `json:"model,omitempty"`
	CompletedAt   time.Time `json:"completed_at,omitempty"`
	ParseOK       bool      `json:"parse_ok"`
	OutputNodeIDs []string  `json:"output_node_ids,omitempty"`
}

type VerificationPassCoverage struct {
	ExpectedNodeIDs   []string `json:"expected_node_ids,omitempty"`
	ReturnedNodeIDs   []string `json:"returned_node_ids,omitempty"`
	MissingNodeIDs    []string `json:"missing_node_ids,omitempty"`
	DuplicateNodeIDs  []string `json:"duplicate_node_ids,omitempty"`
	UnexpectedNodeIDs []string `json:"unexpected_node_ids,omitempty"`
	Valid             bool     `json:"valid"`
}

type VerificationRetrievalSummary struct {
	RetrievedNodeIDs     []string `json:"retrieved_node_ids,omitempty"`
	NoResultNodeIDs      []string `json:"no_result_node_ids,omitempty"`
	BudgetLimitedNodeIDs []string `json:"budget_limited_node_ids,omitempty"`
	PromptContextReduced bool     `json:"prompt_context_reduced"`
	ExcerptTruncated     bool     `json:"excerpt_truncated"`
}

type VerificationPass struct {
	Kind             VerificationPassKind          `json:"kind,omitempty"`
	NodeIDs          []string                      `json:"node_ids,omitempty"`
	Coverage         VerificationPassCoverage      `json:"coverage"`
	RetrievalSummary *VerificationRetrievalSummary `json:"retrieval_summary,omitempty"`
	Claim            *VerificationStageSummary     `json:"claim,omitempty"`
	Challenge        *VerificationStageSummary     `json:"challenge,omitempty"`
	Adjudication     *VerificationStageSummary     `json:"adjudication,omitempty"`
}

type VerificationCoverageSummary struct {
	TotalExpectedNodes  int      `json:"total_expected_nodes"`
	TotalFinalizedNodes int      `json:"total_finalized_nodes"`
	MissingNodeIDs      []string `json:"missing_node_ids,omitempty"`
	DuplicateNodeIDs    []string `json:"duplicate_node_ids,omitempty"`
	UnexpectedNodeIDs   []string `json:"unexpected_node_ids,omitempty"`
	Valid               bool     `json:"valid"`
}

type Verification struct {
	VerifiedAt              time.Time                    `json:"verified_at,omitempty"`
	Model                   string                       `json:"model,omitempty"`
	Version                 string                       `json:"version,omitempty"`
	RolloutStage            string                       `json:"rollout_stage,omitempty"`
	FactChecks              []FactCheck                  `json:"fact_checks,omitempty"`
	ExplicitConditionChecks []ExplicitConditionCheck     `json:"explicit_condition_checks,omitempty"`
	ImplicitConditionChecks []ImplicitConditionCheck     `json:"implicit_condition_checks,omitempty"`
	PredictionChecks        []PredictionCheck            `json:"prediction_checks,omitempty"`
	Passes                  []VerificationPass           `json:"passes,omitempty"`
	CoverageSummary         *VerificationCoverageSummary `json:"coverage_summary,omitempty"`
}

type Output struct {
	Summary      string         `json:"summary,omitempty"`
	Drivers      []string       `json:"drivers,omitempty"`
	Targets      []string       `json:"targets,omitempty"`
	Graph        ReasoningGraph `json:"graph,omitempty"`
	Details      HiddenDetails  `json:"details,omitempty"`
	Topics       []string       `json:"topics,omitempty"`
	Confidence   string         `json:"confidence,omitempty"`
	Verification Verification   `json:"verification,omitempty"`
}

type Record struct {
	UnitID         string    `json:"unit_id"`
	Source         string    `json:"source"`
	ExternalID     string    `json:"external_id"`
	RootExternalID string    `json:"root_external_id,omitempty"`
	Model          string    `json:"model"`
	Output         Output    `json:"output"`
	CompiledAt     time.Time `json:"compiled_at"`
}

type NodeExtractionOutput struct {
	Graph      ReasoningGraph `json:"graph,omitempty"`
	Details    HiddenDetails  `json:"details,omitempty"`
	Topics     []string       `json:"topics,omitempty"`
	Confidence string         `json:"confidence,omitempty"`
}

type FullGraphOutput struct {
	Graph      ReasoningGraph `json:"graph,omitempty"`
	Details    HiddenDetails  `json:"details,omitempty"`
	Topics     []string       `json:"topics,omitempty"`
	Confidence string         `json:"confidence,omitempty"`
}

type ThesisOutput struct {
	Summary    string         `json:"summary,omitempty"`
	Drivers    []string       `json:"drivers,omitempty"`
	Targets    []string       `json:"targets,omitempty"`
	Details    HiddenDetails  `json:"details,omitempty"`
	Topics     []string       `json:"topics,omitempty"`
	Confidence string         `json:"confidence,omitempty"`
}

func (o Output) Validate() error {
	return o.ValidateWithThresholds(2, 1)
}

func (o Output) ValidateWithThresholds(minNodes, minEdges int) error {
	if strings.TrimSpace(o.Summary) == "" {
		return fmt.Errorf("summary is required")
	}
	if err := validateStringListEntries("drivers", o.Drivers); err != nil {
		return err
	}
	if err := validateStringListEntries("targets", o.Targets); err != nil {
		return err
	}
	if len(o.Graph.Nodes) < minNodes {
		return fmt.Errorf("graph must contain at least %d nodes", minNodes)
	}
	if len(o.Graph.Edges) < minEdges {
		return fmt.Errorf("graph must contain at least %d edges", minEdges)
	}
	if o.Details.IsEmpty() {
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
		if _, ok := nodeIDs[check.NodeID]; !ok {
			return fmt.Errorf("fact check references unknown node: %s", check.NodeID)
		}
		switch check.Status {
		case FactStatusClearlyTrue, FactStatusClearlyFalse, FactStatusUnverifiable:
		default:
			return fmt.Errorf("unsupported fact status: %s", check.Status)
		}
	}
	for _, check := range o.Verification.ExplicitConditionChecks {
		if _, ok := nodeIDs[check.NodeID]; !ok {
			return fmt.Errorf("explicit condition check references unknown node: %s", check.NodeID)
		}
		switch check.Status {
		case ExplicitConditionStatusHigh, ExplicitConditionStatusMedium, ExplicitConditionStatusLow, ExplicitConditionStatusUnknown:
		default:
			return fmt.Errorf("unsupported explicit condition status: %s", check.Status)
		}
	}
	for _, check := range o.Verification.ImplicitConditionChecks {
		if _, ok := nodeIDs[check.NodeID]; !ok {
			return fmt.Errorf("implicit condition check references unknown node: %s", check.NodeID)
		}
		switch check.Status {
		case FactStatusClearlyTrue, FactStatusClearlyFalse, FactStatusUnverifiable:
		default:
			return fmt.Errorf("unsupported implicit condition status: %s", check.Status)
		}
	}
	for _, check := range o.Verification.PredictionChecks {
		if _, ok := nodeIDs[check.NodeID]; !ok {
			return fmt.Errorf("prediction check references unknown node: %s", check.NodeID)
		}
		switch check.Status {
		case PredictionStatusUnresolved, PredictionStatusResolvedTrue, PredictionStatusResolvedFalse, PredictionStatusStaleUnresolved:
		default:
			return fmt.Errorf("unsupported prediction status: %s", check.Status)
		}
	}
	for _, pass := range o.Verification.Passes {
		for _, id := range pass.NodeIDs {
			if _, ok := nodeIDs[id]; !ok {
				return fmt.Errorf("verification pass references unknown node: %s", id)
			}
		}
		for _, id := range pass.Coverage.ExpectedNodeIDs {
			if _, ok := nodeIDs[id]; !ok {
				return fmt.Errorf("verification coverage expected references unknown node: %s", id)
			}
		}
		for _, id := range pass.Coverage.ReturnedNodeIDs {
			if _, ok := nodeIDs[id]; !ok {
				return fmt.Errorf("verification coverage returned references unknown node: %s", id)
			}
		}
		for _, id := range pass.Coverage.MissingNodeIDs {
			if _, ok := nodeIDs[id]; !ok {
				return fmt.Errorf("verification coverage missing references unknown node: %s", id)
			}
		}
		for _, id := range pass.Coverage.DuplicateNodeIDs {
			if _, ok := nodeIDs[id]; !ok {
				return fmt.Errorf("verification coverage duplicate references unknown node: %s", id)
			}
		}
		for _, id := range pass.Coverage.UnexpectedNodeIDs {
			if _, ok := nodeIDs[id]; !ok {
				return fmt.Errorf("verification coverage unexpected references unknown node: %s", id)
			}
		}
		for _, stage := range []*VerificationStageSummary{pass.Claim, pass.Challenge, pass.Adjudication} {
			if stage == nil {
				continue
			}
			for _, id := range stage.OutputNodeIDs {
				if _, ok := nodeIDs[id]; !ok {
					return fmt.Errorf("verification stage references unknown node: %s", id)
				}
			}
		}
	}
	if summary := o.Verification.CoverageSummary; summary != nil {
		for _, id := range summary.MissingNodeIDs {
			if _, ok := nodeIDs[id]; !ok {
				return fmt.Errorf("verification summary missing references unknown node: %s", id)
			}
		}
		for _, id := range summary.DuplicateNodeIDs {
			if _, ok := nodeIDs[id]; !ok {
				return fmt.Errorf("verification summary duplicate references unknown node: %s", id)
			}
		}
		for _, id := range summary.UnexpectedNodeIDs {
			if _, ok := nodeIDs[id]; !ok {
				return fmt.Errorf("verification summary unexpected references unknown node: %s", id)
			}
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

func (o FullGraphOutput) ValidateWithThresholds(minEdges int, nodeIDs map[string]struct{}, nodeKinds map[string]NodeKind) error {
	if len(o.Graph.Nodes) > 0 {
		return fmt.Errorf("full graph output must not contain nodes")
	}
	return validateGraphEdges(o.Graph.Edges, nodeIDs, nodeKinds, minEdges)
}

func (o ThesisOutput) Validate() error {
	if strings.TrimSpace(o.Summary) == "" {
		return fmt.Errorf("summary is required")
	}
	if err := validateStringListEntries("drivers", o.Drivers); err != nil {
		return err
	}
	if err := validateStringListEntries("targets", o.Targets); err != nil {
		return err
	}
	if o.Details.IsEmpty() {
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

func validateGraphNodes(nodes []GraphNode) (map[string]struct{}, error) {
	nodeIDs := map[string]struct{}{}
	for _, node := range nodes {
		if strings.TrimSpace(node.ID) == "" {
			return nil, fmt.Errorf("graph node id is required")
		}
		if strings.TrimSpace(node.Text) == "" {
			return nil, fmt.Errorf("graph node text is required")
		}
		switch node.Kind {
		case NodeFact, NodeExplicitCondition, NodeImplicitCondition, NodeConclusion, NodePrediction:
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
		out[node.ID] = node.Kind
	}
	return out
}

func validateGraphEdges(edges []GraphEdge, nodeIDs map[string]struct{}, nodeKinds map[string]NodeKind, minEdges int) error {
	if len(edges) < minEdges {
		return fmt.Errorf("graph must contain at least %d edges", minEdges)
	}
	for _, edge := range edges {
		if _, ok := nodeIDs[edge.From]; !ok {
			return fmt.Errorf("edge from references unknown node: %s", edge.From)
		}
		if _, ok := nodeIDs[edge.To]; !ok {
			return fmt.Errorf("edge to references unknown node: %s", edge.To)
		}
		switch edge.Kind {
		case EdgePositive, EdgeDerives, EdgePresets:
		default:
			return fmt.Errorf("unsupported edge kind: %s", edge.Kind)
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
	case NodeFact, NodeImplicitCondition:
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
