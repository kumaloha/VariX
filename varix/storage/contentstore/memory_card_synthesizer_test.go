package contentstore

import (
	"testing"

	"github.com/kumaloha/VariX/varix/memory"
)

func TestBuildCognitiveCards_ProducesReadableCard(t *testing.T) {
	thesis := memory.CausalThesis{
		CausalThesisID:  "ct-1",
		ThesisID:        "thesis-1",
		NodeRoles:       map[string]string{"n1": "fact", "n2": "conclusion", "n3": "prediction"},
		CorePathNodeIDs: []string{"n1", "n2", "n3"},
	}
	nodesByID := map[string]memory.AcceptedNode{
		"n1": {NodeID: "n1", NodeText: "流动性收紧"},
		"n2": {NodeID: "n2", NodeText: "风险资产承压"},
		"n3": {NodeID: "n3", NodeText: "未来数月波动加大"},
	}

	got := buildCognitiveCards(thesis, nodesByID)
	if len(got) == 0 {
		t.Fatalf("len(buildCognitiveCards) = 0, want at least one card")
	}
	if got[0].Title == "" {
		t.Fatalf("Title = empty, want readable card title")
	}
	if len(got[0].CausalChain) != 3 {
		t.Fatalf("CausalChain = %#v, want 3-step core path", got[0].CausalChain)
	}
}

func TestBuildCognitiveCards_DoesNotDumpAllNodes(t *testing.T) {
	thesis := memory.CausalThesis{
		CausalThesisID:    "ct-1",
		ThesisID:          "thesis-1",
		NodeRoles:         map[string]string{"n1": "fact", "n2": "conclusion", "n3": "prediction", "n4": "fact"},
		CorePathNodeIDs:   []string{"n1", "n2", "n3"},
		SupportingNodeIDs: []string{"n4"},
	}
	nodesByID := map[string]memory.AcceptedNode{
		"n1": {NodeID: "n1", NodeText: "流动性收紧"},
		"n2": {NodeID: "n2", NodeText: "风险资产承压"},
		"n3": {NodeID: "n3", NodeText: "未来数月波动加大"},
		"n4": {NodeID: "n4", NodeText: "美元走强"},
	}

	got := buildCognitiveCards(thesis, nodesByID)
	if len(got) == 0 {
		t.Fatalf("len(buildCognitiveCards) = 0, want at least one card")
	}
	if len(got[0].CausalChain) != 3 {
		t.Fatalf("CausalChain len = %d, want only core path nodes", len(got[0].CausalChain))
	}
	for _, step := range got[0].CausalChain {
		if step.Label == "美元走强" {
			t.Fatalf("CausalChain unexpectedly dumped supporting node: %#v", got[0].CausalChain)
		}
	}
}

func TestBuildCognitiveCards_IncludesBoundaryConditions(t *testing.T) {
	thesis := memory.CausalThesis{
		CausalThesisID:    "ct-1",
		ThesisID:          "thesis-1",
		NodeRoles:         map[string]string{"n1": "fact", "n2": "conclusion", "n3": "prediction", "n4": "condition"},
		CorePathNodeIDs:   []string{"n1", "n2", "n3"},
		BoundaryNodeIDs:   []string{"n4"},
		CompletenessScore: 0.8,
	}
	nodesByID := map[string]memory.AcceptedNode{
		"n1": {NodeID: "n1", NodeText: "流动性收紧"},
		"n2": {NodeID: "n2", NodeText: "风险资产承压"},
		"n3": {NodeID: "n3", NodeText: "未来数月波动加大"},
		"n4": {NodeID: "n4", NodeText: "若融资环境继续恶化"},
	}

	got := buildCognitiveCards(thesis, nodesByID)
	if len(got) == 0 {
		t.Fatalf("len(buildCognitiveCards) = 0, want card")
	}
	if len(got[0].Conditions) != 1 || got[0].Conditions[0] != "若融资环境继续恶化" {
		t.Fatalf("Conditions = %#v, want boundary condition text", got[0].Conditions)
	}
}

func TestBuildCognitiveCards_SummaryUsesFullCorePath(t *testing.T) {
	thesis := memory.CausalThesis{
		CausalThesisID:  "ct-1",
		ThesisID:        "thesis-1",
		NodeRoles:       map[string]string{"n1": "fact", "n2": "conclusion", "n3": "prediction"},
		CorePathNodeIDs: []string{"n1", "n2", "n3"},
	}
	nodesByID := map[string]memory.AcceptedNode{
		"n1": {NodeID: "n1", NodeText: "流动性收紧"},
		"n2": {NodeID: "n2", NodeText: "风险资产承压"},
		"n3": {NodeID: "n3", NodeText: "未来数月波动加大"},
	}

	got := buildCognitiveCards(thesis, nodesByID)
	want := "流动性收紧 → 风险资产承压 → 未来数月波动加大"
	if got[0].Summary != want {
		t.Fatalf("Summary = %q, want %q", got[0].Summary, want)
	}
}

func TestBuildCognitiveCards_KeyEvidenceUsesReadableTexts(t *testing.T) {
	thesis := memory.CausalThesis{
		CausalThesisID:    "ct-1",
		ThesisID:          "thesis-1",
		NodeRoles:         map[string]string{"n1": "fact", "n2": "conclusion", "n3": "prediction", "n4": "fact"},
		CorePathNodeIDs:   []string{"n1", "n2", "n3"},
		SupportingNodeIDs: []string{"n4"},
		SourceRefs:        []string{"weibo:S1", "twitter:S2"},
	}
	nodesByID := map[string]memory.AcceptedNode{
		"n1": {NodeID: "n1", NodeText: "流动性收紧"},
		"n2": {NodeID: "n2", NodeText: "风险资产承压"},
		"n3": {NodeID: "n3", NodeText: "未来数月波动加大"},
		"n4": {NodeID: "n4", NodeText: "美元走强"},
	}

	got := buildCognitiveCards(thesis, nodesByID)
	if len(got) == 0 {
		t.Fatalf("len(buildCognitiveCards) = 0, want card")
	}
	if len(got[0].KeyEvidence) == 0 {
		t.Fatalf("KeyEvidence = empty, want readable evidence texts")
	}
	for _, evidence := range got[0].KeyEvidence {
		if evidence == "n1" || evidence == "n4" {
			t.Fatalf("KeyEvidence contains raw ids: %#v", got[0].KeyEvidence)
		}
	}
	if len(got[0].SourceRefs) != 2 {
		t.Fatalf("SourceRefs = %#v, want inherited thesis sources", got[0].SourceRefs)
	}
}

func TestBuildCognitiveCards_ConditionInCorePathAlsoAppearsInConditions(t *testing.T) {
	thesis := memory.CausalThesis{
		CausalThesisID:  "ct-1",
		ThesisID:        "thesis-1",
		NodeRoles:       map[string]string{"n1": "fact", "n2": "condition", "n3": "conclusion", "n4": "prediction"},
		CorePathNodeIDs: []string{"n1", "n2", "n3", "n4"},
	}
	nodesByID := map[string]memory.AcceptedNode{
		"n1": {NodeID: "n1", NodeText: "流动性收紧"},
		"n2": {NodeID: "n2", NodeText: "若融资环境继续恶化"},
		"n3": {NodeID: "n3", NodeText: "风险资产承压"},
		"n4": {NodeID: "n4", NodeText: "未来数月波动加大"},
	}

	got := buildCognitiveCards(thesis, nodesByID)
	if len(got) == 0 {
		t.Fatalf("len(buildCognitiveCards) = 0, want card")
	}
	if len(got[0].Conditions) != 1 || got[0].Conditions[0] != "若融资环境继续恶化" {
		t.Fatalf("Conditions = %#v, want condition mirrored from core path", got[0].Conditions)
	}
	if len(got[0].KeyEvidence) != 1 || got[0].KeyEvidence[0] != "流动性收紧" {
		t.Fatalf("KeyEvidence = %#v, want core evidence separated from condition", got[0].KeyEvidence)
	}
}

func TestBuildCognitiveCards_DoesNotUseSupportingPredictionsAsWhy(t *testing.T) {
	thesis := memory.CausalThesis{
		CausalThesisID:    "ct-1",
		ThesisID:          "thesis-1",
		NodeRoles:         map[string]string{"n1": "fact", "n2": "conclusion", "n3": "prediction", "n4": "prediction"},
		CorePathNodeIDs:   []string{"n1", "n2", "n3"},
		SupportingNodeIDs: []string{"n4"},
	}
	nodesByID := map[string]memory.AcceptedNode{
		"n1": {NodeID: "n1", NodeText: "流动性收紧"},
		"n2": {NodeID: "n2", NodeText: "风险资产承压"},
		"n3": {NodeID: "n3", NodeText: "未来数月波动加大"},
		"n4": {NodeID: "n4", NodeText: "明年市场将持续恶化"},
	}

	got := buildCognitiveCards(thesis, nodesByID)
	if len(got) == 0 {
		t.Fatalf("len(buildCognitiveCards) = 0, want card")
	}
	for _, evidence := range got[0].KeyEvidence {
		if evidence == "明年市场将持续恶化" {
			t.Fatalf("KeyEvidence = %#v, want supporting prediction excluded from Why", got[0].KeyEvidence)
		}
	}
}
