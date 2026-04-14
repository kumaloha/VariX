package compile

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

type NodeKind string

const (
	NodeFact       NodeKind = "事实"
	NodeAssumption NodeKind = "隐含条件"
	NodeConclusion NodeKind = "结论"
	NodePrediction NodeKind = "预测"
)

type EdgeKind string

const (
	EdgePositive EdgeKind = "正向"
	EdgeNegative EdgeKind = "负向"
	EdgeDerives  EdgeKind = "推出"
	EdgePresets  EdgeKind = "预设"
)

type GraphNode struct {
	ID        string    `json:"id"`
	Kind      NodeKind  `json:"kind"`
	Text      string    `json:"text"`
	ValidFrom time.Time `json:"valid_from,omitempty"`
	ValidTo   time.Time `json:"valid_to,omitempty"`
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

type PredictionCheck struct {
	NodeID string           `json:"node_id"`
	Status PredictionStatus `json:"status"`
	Reason string           `json:"reason,omitempty"`
	AsOf   time.Time        `json:"as_of,omitempty"`
}

type Verification struct {
	VerifiedAt       time.Time         `json:"verified_at,omitempty"`
	Model            string            `json:"model,omitempty"`
	FactChecks       []FactCheck       `json:"fact_checks,omitempty"`
	PredictionChecks []PredictionCheck `json:"prediction_checks,omitempty"`
}

type Output struct {
	Summary      string         `json:"summary,omitempty"`
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

func (o Output) Validate() error {
	return o.ValidateWithThresholds(2, 1)
}

func (o Output) ValidateWithThresholds(minNodes, minEdges int) error {
	if strings.TrimSpace(o.Summary) == "" {
		return fmt.Errorf("summary is required")
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
	nodeIDs := map[string]struct{}{}
	for _, node := range o.Graph.Nodes {
		if strings.TrimSpace(node.ID) == "" {
			return fmt.Errorf("graph node id is required")
		}
		if strings.TrimSpace(node.Text) == "" {
			return fmt.Errorf("graph node text is required")
		}
		if node.ValidFrom.IsZero() || node.ValidTo.IsZero() {
			return fmt.Errorf("graph node validity window is required: %s", node.ID)
		}
		if node.ValidTo.Before(node.ValidFrom) {
			return fmt.Errorf("graph node validity window is invalid: %s", node.ID)
		}
		switch node.Kind {
		case NodeFact, NodeAssumption, NodeConclusion, NodePrediction:
		default:
			return fmt.Errorf("unsupported node kind: %s", node.Kind)
		}
		nodeIDs[node.ID] = struct{}{}
	}
	for _, edge := range o.Graph.Edges {
		if _, ok := nodeIDs[edge.From]; !ok {
			return fmt.Errorf("edge from references unknown node: %s", edge.From)
		}
		if _, ok := nodeIDs[edge.To]; !ok {
			return fmt.Errorf("edge to references unknown node: %s", edge.To)
		}
		switch edge.Kind {
		case EdgePositive, EdgeNegative, EdgeDerives, EdgePresets:
		default:
			return fmt.Errorf("unsupported edge kind: %s", edge.Kind)
		}
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
	return nil
}
