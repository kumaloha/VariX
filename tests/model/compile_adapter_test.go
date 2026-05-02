package model

import (
	"testing"
	"time"
)

func TestFromCompileRecordMapsLegacyGraphIntoContentSubgraph(t *testing.T) {
	occurredAt := time.Date(2026, 4, 20, 0, 0, 0, 0, time.UTC)
	predictionStart := time.Date(2026, 4, 21, 0, 0, 0, 0, time.UTC)
	predictionDue := time.Date(2026, 4, 28, 0, 0, 0, 0, time.UTC)

	record := Record{
		UnitID:         "unit-1",
		Source:         "twitter",
		ExternalID:     "123",
		RootExternalID: "root-1",
		Model:          "test-model",
		CompiledAt:     occurredAt,
		Output: Output{
			Graph: ReasoningGraph{
				Nodes: []GraphNode{
					{ID: "n1", Kind: NodeFact, Text: "美联储加息0.25%", OccurredAt: occurredAt},
					{ID: "n2", Kind: NodePrediction, Text: "未来一周美股承压", PredictionStartAt: predictionStart, PredictionDueAt: predictionDue},
				},
				Edges: []GraphEdge{{From: "n1", To: "n2", Kind: EdgePositive}},
			},
			Verification: Verification{
				NodeVerifications: []NodeVerification{
					{NodeID: "n1", Status: NodeVerificationProved},
					{NodeID: "n2", Status: NodeVerificationWaiting},
				},
			},
		},
	}

	subgraph, err := FromCompileRecord(record)
	if err != nil {
		t.Fatalf("FromCompileRecord() error = %v", err)
	}
	if err := subgraph.Validate(); err != nil {
		t.Fatalf("subgraph.Validate() error = %v", err)
	}
	if subgraph.ArticleID != "unit-1" {
		t.Fatalf("ArticleID = %q, want unit-1", subgraph.ArticleID)
	}
	if len(subgraph.Nodes) != 2 {
		t.Fatalf("len(Nodes) = %d, want 2", len(subgraph.Nodes))
	}
	if len(subgraph.Edges) != 1 {
		t.Fatalf("len(Edges) = %d, want 1", len(subgraph.Edges))
	}

	nodeByID := map[string]ContentNode{}
	for _, node := range subgraph.Nodes {
		nodeByID[node.ID] = node
	}
	fact := nodeByID["n1"]
	if fact.Kind != NodeKindObservation {
		t.Fatalf("fact.Kind = %q, want observation", fact.Kind)
	}
	if fact.VerificationStatus != VerificationProved {
		t.Fatalf("fact.VerificationStatus = %q, want proved", fact.VerificationStatus)
	}
	if fact.SubjectText != "美联储加息0.25%" || fact.ChangeText != "美联储加息0.25%" {
		t.Fatalf("fact subject/change = %q/%q, want legacy text mirrored", fact.SubjectText, fact.ChangeText)
	}
	if fact.RawText != "美联储加息0.25%" {
		t.Fatalf("fact.RawText = %q, want legacy text", fact.RawText)
	}
	pred := nodeByID["n2"]
	if pred.Kind != NodeKindPrediction {
		t.Fatalf("pred.Kind = %q, want prediction", pred.Kind)
	}
	if pred.VerificationStatus != VerificationPending {
		t.Fatalf("pred.VerificationStatus = %q, want pending", pred.VerificationStatus)
	}
	if pred.TimeStart == "" || pred.TimeEnd == "" {
		t.Fatalf("pred time window = %q..%q, want non-empty", pred.TimeStart, pred.TimeEnd)
	}
	edge := subgraph.Edges[0]
	if edge.Type != EdgeTypeDrives {
		t.Fatalf("edge.Type = %q, want drives", edge.Type)
	}
	if !edge.IsPrimary {
		t.Fatal("edge.IsPrimary = false, want true")
	}
	if edge.VerificationStatus != VerificationPending {
		t.Fatalf("edge.VerificationStatus = %q, want pending", edge.VerificationStatus)
	}
}

func TestFromCompileRecordMarksOnlyDriverTargetAndPathNodesPrimary(t *testing.T) {
	record := Record{
		UnitID:     "unit-primary",
		Source:     "twitter",
		ExternalID: "primary-1",
		Model:      "test-model",
		CompiledAt: time.Date(2026, 4, 21, 0, 0, 0, 0, time.UTC),
		Output: Output{
			Drivers:           []string{"Driver A"},
			Targets:           []string{"Target C"},
			TransmissionPaths: []TransmissionPath{{Driver: "Driver A", Steps: []string{"Bridge B"}, Target: "Target C"}},
			Graph:             ReasoningGraph{Nodes: []GraphNode{{ID: "n1", Kind: NodeFact, Text: "Driver A", OccurredAt: time.Date(2026, 4, 21, 0, 0, 0, 0, time.UTC)}, {ID: "n2", Kind: NodeMechanism, Text: "Bridge B", OccurredAt: time.Date(2026, 4, 21, 0, 0, 0, 0, time.UTC)}, {ID: "n3", Kind: NodePrediction, Text: "Target C", PredictionStartAt: time.Date(2026, 4, 21, 0, 0, 0, 0, time.UTC), PredictionDueAt: time.Date(2026, 4, 28, 0, 0, 0, 0, time.UTC)}, {ID: "n4", Kind: NodeFact, Text: "Side note D", OccurredAt: time.Date(2026, 4, 21, 0, 0, 0, 0, time.UTC)}}, Edges: []GraphEdge{{From: "n1", To: "n2", Kind: EdgePositive}, {From: "n2", To: "n3", Kind: EdgePositive}}},
		},
	}
	subgraph, err := FromCompileRecord(record)
	if err != nil {
		t.Fatalf("FromCompileRecord() error = %v", err)
	}
	primary := map[string]bool{}
	for _, node := range subgraph.Nodes {
		primary[node.ID] = node.IsPrimary
	}
	if !primary["n1"] || !primary["n2"] || !primary["n3"] {
		t.Fatalf("primary map = %#v, want path nodes primary", primary)
	}
	if primary["n4"] {
		t.Fatalf("primary map = %#v, want side note non-primary", primary)
	}
}
