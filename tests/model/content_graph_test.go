package model

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"
	"time"
)

func TestGraphNodeValidateRequiresCoreFields(t *testing.T) {
	node := ContentNode{}
	if err := node.Validate(); err == nil {
		t.Fatal("Validate() error = nil, want missing core field error")
	}

	node = ContentNode{
		ID:                 "n1",
		SourceArticleID:    "u1",
		SourcePlatform:     "twitter",
		SourceExternalID:   "123",
		RawText:            "美联储加息0.25%",
		SubjectText:        "美联储",
		ChangeText:         "加息0.25%",
		Kind:               NodeKindObservation,
		IsPrimary:          true,
		VerificationStatus: VerificationPending,
	}
	if err := node.Validate(); err != nil {
		t.Fatalf("Validate() error = %v, want nil", err)
	}
}

func TestContentSubgraphValidateRejectsUnknownEdgeNode(t *testing.T) {
	subgraph := ContentSubgraph{
		ID:               "sg1",
		ArticleID:        "u1",
		SourcePlatform:   "twitter",
		SourceExternalID: "123",
		CompileVersion:   CompileBridgeVersion,
		CompiledAt:       "2026-04-21T00:00:00Z",
		UpdatedAt:        "2026-04-21T00:00:00Z",
		Nodes: []ContentNode{{
			ID:                 "n1",
			SourceArticleID:    "u1",
			SourcePlatform:     "twitter",
			SourceExternalID:   "123",
			RawText:            "美联储加息0.25%",
			SubjectText:        "美联储",
			ChangeText:         "加息0.25%",
			Kind:               NodeKindObservation,
			IsPrimary:          true,
			VerificationStatus: VerificationPending,
		}},
		Edges: []ContentEdge{{
			ID:                 "e1",
			From:               "n1",
			To:                 "missing",
			Type:               EdgeTypeDrives,
			IsPrimary:          true,
			VerificationStatus: VerificationPending,
		}},
	}

	if err := subgraph.Validate(); err == nil {
		t.Fatal("Validate() error = nil, want unknown edge endpoint error")
	}
}

func TestOutputValidateRejectsDuplicateGraphNodeIDs(t *testing.T) {
	out := Output{
		Summary: "油价冲击推高商品价格。",
		Drivers: []string{"油价冲击"},
		Targets: []string{"商品价格上涨"},
		TransmissionPaths: []TransmissionPath{{
			Driver: "油价冲击",
			Target: "商品价格上涨",
			Steps:  []string{"油价冲击"},
		}},
		EvidenceNodes: []string{"油价上涨"},
		Details: HiddenDetails{Items: []map[string]any{{
			"kind": "proof_point",
			"text": "油价上涨",
		}}},
		Graph: ReasoningGraph{
			Nodes: []GraphNode{
				{ID: "n1", Kind: NodeMechanism, Text: "油价冲击", OccurredAt: time.Date(2026, 5, 2, 0, 0, 0, 0, time.UTC)},
				{ID: "n2", Kind: NodeConclusion, Text: "商品价格上涨"},
				{ID: "n2", Kind: NodeConclusion, Text: "商品价格上涨"},
			},
			Edges: []GraphEdge{{From: "n1", To: "n2", Kind: EdgePositive}},
		},
	}

	err := out.Validate()
	if err == nil {
		t.Fatal("Validate() error = nil, want duplicate graph node id error")
	}
	if !strings.Contains(err.Error(), "duplicate graph node id") {
		t.Fatalf("Validate() error = %q, want duplicate graph node id", err)
	}
}

func TestOutputValidateAllowsDirectTransmissionPathWithoutSteps(t *testing.T) {
	out := Output{
		Summary: "驱动A直接推动目标B。",
		Drivers: []string{"驱动A"},
		Targets: []string{"目标B"},
		TransmissionPaths: []TransmissionPath{{
			Driver: "驱动A",
			Target: "目标B",
		}},
		Details: HiddenDetails{Items: []map[string]any{{
			"kind": "inference_path",
			"from": "驱动A",
			"to":   "目标B",
		}}},
		Graph: ReasoningGraph{
			Nodes: []GraphNode{
				{ID: "n1", Kind: NodeMechanism, Text: "驱动A", OccurredAt: time.Date(2026, 5, 4, 0, 0, 0, 0, time.UTC)},
				{ID: "n2", Kind: NodeConclusion, Text: "目标B"},
			},
			Edges: []GraphEdge{{From: "n1", To: "n2", Kind: EdgePositive}},
		},
	}

	if err := out.Validate(); err != nil {
		t.Fatalf("Validate() error = %v, want nil for direct path without steps", err)
	}
}

func TestOutputValidateAllowsDeclarationOnlyRender(t *testing.T) {
	out := Output{
		Summary: "伯克希尔会等待市场错配，再快速、果断、大额部署资本。",
		Declarations: []Declaration{{
			ID:          "decl-1",
			Speaker:     "Greg Abel",
			Kind:        "capital_allocation_rule",
			Topic:       "capital_allocation",
			Statement:   "伯克希尔会等待市场错配",
			Conditions:  []string{"市场出现错配"},
			Actions:     []string{"快速且果断行动"},
			Scale:       "投入大量资本",
			Constraints: []string{"不会仅因现金规模大而被迫投资"},
			Evidence:    []string{"现金和短债约3800亿美元"},
			SourceQuote: "there will be dislocations in markets ... act decisively both quickly and with significant capital",
			Confidence:  "high",
		}},
		Graph: ReasoningGraph{
			Nodes: []GraphNode{
				{ID: "n1", Kind: NodeFact, Text: "现金和短债约3800亿美元", OccurredAt: time.Date(2026, 5, 2, 0, 0, 0, 0, time.UTC)},
				{ID: "n2", Kind: NodeConclusion, Text: "伯克希尔会等待市场错配"},
			},
			Edges: []GraphEdge{{From: "n1", To: "n2", Kind: EdgeExplains}},
		},
		Confidence: "medium",
	}

	if err := out.Validate(); err != nil {
		t.Fatalf("Validate() error = %v, want nil", err)
	}
}

func TestOutputValidateAllowsSpeakerSalience(t *testing.T) {
	out := Output{
		Summary: "Greg Abel 解释，伯克希尔看 Apple 不是科技股标签，而是看消费者价值和风险。",
		SemanticUnits: []SemanticUnit{{
			ID:               "u-portfolio",
			Speaker:          "Greg Abel",
			SpeakerRole:      "primary",
			Subject:          "existing portfolio / circle of competence",
			Force:            "answer",
			Claim:            "现有组合由 Warren Buffett 建立，但集中在 Greg Abel 也理解业务和经济前景的公司；Apple 说明能力圈不是行业标签，而是看产品价值、消费者依赖和风险。",
			PromptContext:    "股东询问 Greg Abel 如何在能力圈不同的情况下管理 Warren Buffett 建立的组合。",
			ImportanceReason: "这是主讲人对投资科技股/能力圈问题的直接回答。",
			SourceQuote:      "I would not say we invest in Apple because we view it as a technology stock.",
			Salience:         0.93,
			Confidence:       "high",
		}},
		Details: HiddenDetails{Items: []map[string]any{{
			"kind": "semantic_unit",
			"id":   "u-apple",
		}}},
		Graph: ReasoningGraph{
			Nodes: []GraphNode{
				{ID: "n1", Kind: NodeFact, Text: "股东询问 Apple 和能力圈", OccurredAt: time.Date(2026, 5, 2, 0, 0, 0, 0, time.UTC)},
				{ID: "n2", Kind: NodeConclusion, Text: "Apple 投资按消费者价值和风险理解"},
			},
			Edges: []GraphEdge{{From: "n1", To: "n2", Kind: EdgeExplains}},
		},
		Confidence: "medium",
	}

	if err := out.Validate(); err != nil {
		t.Fatalf("Validate() error = %v, want nil", err)
	}
}

func TestOutputPreservesLedgerItems(t *testing.T) {
	out := Output{
		Summary: "Berkshire meeting digest",
		Ledger: Ledger{
			Items: []LedgerItem{{
				ID:        "ledger-001",
				Kind:      "list",
				Category:  "portfolio",
				Claim:     "Berkshire discussed the major public holdings.",
				Entities:  []string{"Apple", "American Express", "Coca-Cola", "Bank of America"},
				SourceIDs: []string{"semantic-014"},
			}},
		},
		Graph: ReasoningGraph{
			Nodes: []GraphNode{
				{ID: "n1", Kind: NodeFact, Text: "Berkshire discussed Apple", OccurredAt: time.Date(2026, 5, 4, 0, 0, 0, 0, time.UTC)},
				{ID: "n2", Kind: NodeConclusion, Text: "Public holdings remain part of the digest"},
			},
			Edges: []GraphEdge{{From: "n1", To: "n2", Kind: EdgeExplains}},
		},
	}

	if err := out.Validate(); err != nil {
		t.Fatalf("Validate() error = %v, want nil", err)
	}
	payload, err := json.Marshal(out)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	var roundTrip Output
	if err := json.Unmarshal(payload, &roundTrip); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if got := roundTrip.Ledger.Items[0].Entities; !slices.Contains(got, "Apple") {
		t.Fatalf("ledger entities = %#v, want Apple", got)
	}
}
