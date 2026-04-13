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
	ID   string   `json:"id"`
	Kind NodeKind `json:"kind"`
	Text string   `json:"text"`
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

type Output struct {
	Summary    string         `json:"summary,omitempty"`
	Graph      ReasoningGraph `json:"graph,omitempty"`
	Details    HiddenDetails  `json:"details,omitempty"`
	Topics     []string       `json:"topics,omitempty"`
	Confidence string         `json:"confidence,omitempty"`
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
	nodeIDs := map[string]struct{}{}
	for _, node := range o.Graph.Nodes {
		if strings.TrimSpace(node.ID) == "" {
			return fmt.Errorf("graph node id is required")
		}
		if strings.TrimSpace(node.Text) == "" {
			return fmt.Errorf("graph node text is required")
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
	return nil
}
