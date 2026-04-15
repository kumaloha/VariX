package contentstore

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/kumaloha/VariX/varix/compile"
	"github.com/kumaloha/VariX/varix/memory"
)

func TestSQLiteStore_RunGlobalMemoryOrganizationV2PersistsOutput(t *testing.T) {
	root := t.TempDir()
	store, err := NewSQLiteStore(filepath.Join(root, "data", "content.db"))
	if err != nil {
		t.Fatalf("NewSQLiteStore() error = %v", err)
	}
	defer store.Close()

	seedCompiledRecordForMemory(t, store)
	if _, err := store.AcceptMemoryNodes(context.Background(), memory.AcceptRequest{
		UserID:           "u-v2",
		SourcePlatform:   "weibo",
		SourceExternalID: "Q1",
		NodeIDs:          []string{"n1"},
	}); err != nil {
		t.Fatalf("AcceptMemoryNodes() error = %v", err)
	}

	now := time.Date(2026, 4, 15, 0, 0, 0, 0, time.UTC)
	out, err := store.RunGlobalMemoryOrganizationV2(context.Background(), "u-v2", now)
	if err != nil {
		t.Fatalf("RunGlobalMemoryOrganizationV2() error = %v", err)
	}
	if out.UserID != "u-v2" {
		t.Fatalf("UserID = %q, want u-v2", out.UserID)
	}
	if out.GeneratedAt.IsZero() {
		t.Fatalf("GeneratedAt = zero, want persisted timestamp")
	}

	got, err := store.GetLatestGlobalMemoryOrganizationV2Output(context.Background(), "u-v2")
	if err != nil {
		t.Fatalf("GetLatestGlobalMemoryOrganizationV2Output() error = %v", err)
	}
	if got.OutputID == 0 {
		t.Fatalf("OutputID = 0, want persisted row id")
	}
	if got.UserID != "u-v2" {
		t.Fatalf("latest UserID = %q, want u-v2", got.UserID)
	}
}

func TestSQLiteStore_RunGlobalMemoryOrganizationV2SurfacesConflictSets(t *testing.T) {
	root := t.TempDir()
	store, err := NewSQLiteStore(filepath.Join(root, "data", "content.db"))
	if err != nil {
		t.Fatalf("NewSQLiteStore() error = %v", err)
	}
	defer store.Close()

	records := []compile.Record{
		{
			UnitID:         "weibo:C1",
			Source:         "weibo",
			ExternalID:     "C1",
			RootExternalID: "C1",
			Model:          "qwen3.6-plus",
			Output: compile.Output{
				Summary: "summary",
				Graph: compile.ReasoningGraph{
					Nodes: []compile.GraphNode{
						{ID: "n1", Kind: compile.NodeFact, Text: "供给趋紧", OccurredAt: time.Date(2026, 4, 10, 0, 0, 0, 0, time.UTC)},
						{ID: "n2", Kind: compile.NodeConclusion, Text: "油价会上升", OccurredAt: time.Date(2026, 4, 10, 0, 0, 0, 0, time.UTC)},
					},
					Edges: []compile.GraphEdge{{From: "n1", To: "n2", Kind: compile.EdgeDerives}},
				},
				Details:    compile.HiddenDetails{Caveats: []string{"detail"}},
				Confidence: "medium",
			},
			CompiledAt: time.Date(2026, 4, 14, 8, 0, 0, 0, time.UTC),
		},
		{
			UnitID:         "twitter:C2",
			Source:         "twitter",
			ExternalID:     "C2",
			RootExternalID: "C2",
			Model:          "qwen3.6-plus",
			Output: compile.Output{
				Summary: "summary",
				Graph: compile.ReasoningGraph{
					Nodes: []compile.GraphNode{
						{ID: "n1", Kind: compile.NodeFact, Text: "需求走弱", OccurredAt: time.Date(2026, 4, 11, 0, 0, 0, 0, time.UTC)},
						{ID: "n2", Kind: compile.NodeConclusion, Text: "油价会下降", OccurredAt: time.Date(2026, 4, 11, 0, 0, 0, 0, time.UTC)},
					},
					Edges: []compile.GraphEdge{{From: "n1", To: "n2", Kind: compile.EdgeDerives}},
				},
				Details:    compile.HiddenDetails{Caveats: []string{"detail"}},
				Confidence: "medium",
			},
			CompiledAt: time.Date(2026, 4, 14, 8, 10, 0, 0, time.UTC),
		},
	}
	for _, record := range records {
		if err := store.UpsertCompiledOutput(context.Background(), record); err != nil {
			t.Fatalf("UpsertCompiledOutput(%s) error = %v", record.UnitID, err)
		}
	}
	for _, req := range []memory.AcceptRequest{
		{UserID: "u-v2-conflict", SourcePlatform: "weibo", SourceExternalID: "C1", NodeIDs: []string{"n2"}},
		{UserID: "u-v2-conflict", SourcePlatform: "twitter", SourceExternalID: "C2", NodeIDs: []string{"n2"}},
	} {
		if _, err := store.AcceptMemoryNodes(context.Background(), req); err != nil {
			t.Fatalf("AcceptMemoryNodes(%s:%s) error = %v", req.SourcePlatform, req.SourceExternalID, err)
		}
	}

	out, err := store.RunGlobalMemoryOrganizationV2(context.Background(), "u-v2-conflict", time.Date(2026, 4, 15, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("RunGlobalMemoryOrganizationV2() error = %v", err)
	}
	if len(out.ConflictSets) == 0 {
		t.Fatalf("ConflictSets = %#v, want at least one blocked conflict", out.ConflictSets)
	}
}

func TestSQLiteStore_RunGlobalMemoryOrganizationV2BuildsCausalThesesAndCards(t *testing.T) {
	root := t.TempDir()
	store, err := NewSQLiteStore(filepath.Join(root, "data", "content.db"))
	if err != nil {
		t.Fatalf("NewSQLiteStore() error = %v", err)
	}
	defer store.Close()

	record := compile.Record{
		UnitID:         "weibo:T1",
		Source:         "weibo",
		ExternalID:     "T1",
		RootExternalID: "T1",
		Model:          "qwen3.6-plus",
		Output: compile.Output{
			Summary: "summary",
			Graph: compile.ReasoningGraph{
				Nodes: []compile.GraphNode{
					{ID: "n1", Kind: compile.NodeFact, Text: "流动性收紧", OccurredAt: time.Date(2026, 4, 10, 0, 0, 0, 0, time.UTC)},
					{ID: "n2", Kind: compile.NodeConclusion, Text: "风险资产承压", OccurredAt: time.Date(2026, 4, 10, 0, 0, 0, 0, time.UTC)},
					{ID: "n3", Kind: compile.NodePrediction, Text: "未来数月波动加大", PredictionStartAt: time.Date(2026, 4, 10, 0, 0, 0, 0, time.UTC)},
				},
				Edges: []compile.GraphEdge{
					{From: "n1", To: "n2", Kind: compile.EdgeDerives},
					{From: "n2", To: "n3", Kind: compile.EdgeDerives},
				},
			},
			Details:    compile.HiddenDetails{Caveats: []string{"detail"}},
			Confidence: "medium",
		},
		CompiledAt: time.Date(2026, 4, 14, 8, 0, 0, 0, time.UTC),
	}
	if err := store.UpsertCompiledOutput(context.Background(), record); err != nil {
		t.Fatalf("UpsertCompiledOutput() error = %v", err)
	}
	if _, err := store.AcceptMemoryNodes(context.Background(), memory.AcceptRequest{
		UserID:           "u-v2-causal",
		SourcePlatform:   "weibo",
		SourceExternalID: "T1",
		NodeIDs:          []string{"n1", "n2", "n3"},
	}); err != nil {
		t.Fatalf("AcceptMemoryNodes() error = %v", err)
	}

	out, err := store.RunGlobalMemoryOrganizationV2(context.Background(), "u-v2-causal", time.Date(2026, 4, 15, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("RunGlobalMemoryOrganizationV2() error = %v", err)
	}
	if len(out.CausalTheses) == 0 {
		t.Fatalf("CausalTheses = %#v, want at least one causal thesis", out.CausalTheses)
	}
	if len(out.CognitiveCards) == 0 {
		t.Fatalf("CognitiveCards = %#v, want at least one card", out.CognitiveCards)
	}
	if len(out.CognitiveConclusions) == 0 {
		t.Fatalf("CognitiveConclusions = %#v, want at least one conclusion", out.CognitiveConclusions)
	}
	if len(out.TopMemoryItems) == 0 || out.TopMemoryItems[0].ItemType != "conclusion" {
		t.Fatalf("TopMemoryItems = %#v, want first-layer conclusion item", out.TopMemoryItems)
	}
}
