package model

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
	NodeMechanism         NodeKind = "机制"
	NodeAssumption        NodeKind = NodeImplicitCondition // backward-compatible alias
	NodeConclusion        NodeKind = "结论"
	NodePrediction        NodeKind = "预测"
)

type NodeForm string

const (
	NodeFormObservation NodeForm = "observation"
	NodeFormCondition   NodeForm = "condition"
	NodeFormJudgment    NodeForm = "judgment"
	NodeFormForecast    NodeForm = "forecast"
)

type NodeFunction string

const (
	NodeFunctionSupport      NodeFunction = "support"
	NodeFunctionTransmission NodeFunction = "transmission"
	NodeFunctionClaim        NodeFunction = "claim"
)

type EdgeKind string

const (
	EdgePositive EdgeKind = "drives"
	EdgeDerives  EdgeKind = "substantiates"
	EdgePresets  EdgeKind = "gates"
	EdgeExplains EdgeKind = "explains"
)

type GraphNode struct {
	ID                string       `json:"id"`
	Kind              NodeKind     `json:"kind"`
	Form              NodeForm     `json:"form,omitempty"`
	Function          NodeFunction `json:"function,omitempty"`
	Text              string       `json:"text"`
	ValidFrom         time.Time    `json:"valid_from,omitempty"`
	ValidTo           time.Time    `json:"valid_to,omitempty"`
	OccurredAt        time.Time    `json:"occurred_at,omitempty"`
	PredictionStartAt time.Time    `json:"prediction_start_at,omitempty"`
	PredictionDueAt   time.Time    `json:"prediction_due_at,omitempty"`
}

func (n GraphNode) MarshalJSON() ([]byte, error) {
	type graphNodePayload struct {
		ID                string       `json:"id"`
		Kind              NodeKind     `json:"kind"`
		Form              NodeForm     `json:"form,omitempty"`
		Function          NodeFunction `json:"function,omitempty"`
		Text              string       `json:"text"`
		ValidFrom         *time.Time   `json:"valid_from,omitempty"`
		ValidTo           *time.Time   `json:"valid_to,omitempty"`
		OccurredAt        *time.Time   `json:"occurred_at,omitempty"`
		PredictionStartAt *time.Time   `json:"prediction_start_at,omitempty"`
		PredictionDueAt   *time.Time   `json:"prediction_due_at,omitempty"`
	}
	normalized, err := n.normalizedSchema()
	if err == nil {
		n = normalized
	}
	payload := graphNodePayload{
		ID:       n.ID,
		Kind:     n.Kind,
		Form:     n.Form,
		Function: n.Function,
		Text:     n.Text,
	}
	switch n.Kind {
	case NodeFact, NodeImplicitCondition, NodeMechanism:
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
	case NodeFact, NodeImplicitCondition, NodeMechanism:
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
	normalized, err := n.normalizedSchema()
	if err != nil {
		return err
	}
	*n = normalized
	return nil
}

func (n GraphNode) normalizedSchema() (GraphNode, error) {
	normalized := n
	if shouldNormalizeToExplicitCondition(normalized.Kind, normalized.Text) {
		normalized.Kind = NodeExplicitCondition
	}
	if normalized.Kind == "" {
		kind, ok := inferNodeKindFromFormFunction(normalized.Form, normalized.Function, normalized.Text)
		if !ok {
			return GraphNode{}, fmt.Errorf("unsupported node schema: form=%s function=%s", normalized.Form, normalized.Function)
		}
		normalized.Kind = kind
	}
	expectedForm, expectedFunction, ok := defaultFormFunctionForKind(normalized.Kind)
	if !ok {
		return GraphNode{}, fmt.Errorf("unsupported node kind: %s", normalized.Kind)
	}
	if normalized.Form != "" && normalized.Form != expectedForm {
		return GraphNode{}, fmt.Errorf("node form %q does not match kind %q", normalized.Form, normalized.Kind)
	}
	if normalized.Function != "" && normalized.Function != expectedFunction {
		return GraphNode{}, fmt.Errorf("node function %q does not match kind %q", normalized.Function, normalized.Kind)
	}
	normalized.Form = expectedForm
	normalized.Function = expectedFunction
	return normalized, nil
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
		Source   string `json:"source"`
		Target   string `json:"target"`
		Relation string `json:"relation"`
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
	if strings.TrimSpace(string(e.Kind)) == "" && strings.TrimSpace(aux.Relation) != "" {
		e.Kind = normalizeEdgeKind(EdgeKind(strings.TrimSpace(aux.Relation)))
	} else {
		e.Kind = normalizeEdgeKind(e.Kind)
	}
	return nil
}

type ReasoningGraph struct {
	Nodes []GraphNode `json:"nodes,omitempty"`
	Edges []GraphEdge `json:"edges,omitempty"`
}

type TransmissionPath struct {
	Driver string   `json:"driver"`
	Target string   `json:"target"`
	Steps  []string `json:"steps,omitempty"`
}

type Branch struct {
	ID                string             `json:"id,omitempty"`
	Level             string             `json:"level,omitempty"`
	Policy            string             `json:"policy,omitempty"`
	Thesis            string             `json:"thesis,omitempty"`
	Anchors           []string           `json:"anchors,omitempty"`
	BranchDrivers     []string           `json:"branch_drivers,omitempty"`
	Drivers           []string           `json:"drivers,omitempty"`
	Targets           []string           `json:"targets,omitempty"`
	Declarations      []Declaration      `json:"declarations,omitempty"`
	TransmissionPaths []TransmissionPath `json:"transmission_paths,omitempty"`
}
