package contentstore

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/kumaloha/VariX/varix/compile"
	"github.com/kumaloha/VariX/varix/graphmodel"
)

func testContentSubgraph() graphmodel.ContentSubgraph {
	return graphmodel.ContentSubgraph{
		ID:               "unit-graph-1",
		ArticleID:        "unit-graph-1",
		SourcePlatform:   "twitter",
		SourceExternalID: "g1",
		RootExternalID:   "root-g1",
		CompileVersion:   graphmodel.CompileBridgeVersion,
		CompiledAt:       time.Date(2026, 4, 21, 0, 0, 0, 0, time.UTC).Format(time.RFC3339),
		UpdatedAt:        time.Date(2026, 4, 21, 0, 0, 0, 0, time.UTC).Format(time.RFC3339),
		Nodes: []graphmodel.GraphNode{{
			ID:                 "n1",
			SourceArticleID:    "unit-graph-1",
			SourcePlatform:     "twitter",
			SourceExternalID:   "g1",
			RawText:            "美联储加息0.25%",
			SubjectText:        "美联储",
			ChangeText:         "加息0.25%",
			Kind:               graphmodel.NodeKindObservation,
			IsPrimary:          true,
			VerificationStatus: graphmodel.VerificationPending,
		}},
		Edges: nil,
	}
}

func TestSQLiteStore_UpsertAndGetContentSubgraph(t *testing.T) {
	root := t.TempDir()
	store, err := NewSQLiteStore(filepath.Join(root, "data", "content.db"))
	if err != nil {
		t.Fatalf("NewSQLiteStore() error = %v", err)
	}
	defer store.Close()

	subgraph := testContentSubgraph()
	if err := store.UpsertContentSubgraph(context.Background(), subgraph); err != nil {
		t.Fatalf("UpsertContentSubgraph() error = %v", err)
	}
	got, err := store.GetContentSubgraph(context.Background(), "twitter", "g1")
	if err != nil {
		t.Fatalf("GetContentSubgraph() error = %v", err)
	}
	if got.ID != subgraph.ID {
		t.Fatalf("ID = %q, want %q", got.ID, subgraph.ID)
	}
	if len(got.Nodes) != 1 || got.Nodes[0].SubjectText != "美联储" {
		t.Fatalf("nodes = %#v, want preserved graph payload", got.Nodes)
	}
}

func TestSQLiteStore_UpsertCompiledOutputBridgesToContentSubgraphTable(t *testing.T) {
	root := t.TempDir()
	store, err := NewSQLiteStore(filepath.Join(root, "data", "content.db"))
	if err != nil {
		t.Fatalf("NewSQLiteStore() error = %v", err)
	}
	defer store.Close()

	record := compile.Record{
		UnitID:         "unit-bridge",
		Source:         "twitter",
		ExternalID:     "bridge-1",
		RootExternalID: "root-bridge-1",
		Model:          "qwen3.6-plus",
		Output: compile.Output{
			Summary: "summary text",
			Graph: compile.ReasoningGraph{
				Nodes: []compile.GraphNode{
					testCompileGraphNode("n1", compile.NodeFact, "美联储加息0.25%"),
					testCompileGraphNode("n2", compile.NodePrediction, "未来一周美股承压"),
				},
				Edges: []compile.GraphEdge{{From: "n1", To: "n2", Kind: compile.EdgePositive}},
			},
			Details: compile.HiddenDetails{Caveats: []string{"detail"}},
		},
		CompiledAt: time.Date(2026, 4, 21, 0, 0, 0, 0, time.UTC),
	}

	if err := store.UpsertCompiledOutput(context.Background(), record); err != nil {
		t.Fatalf("UpsertCompiledOutput() error = %v", err)
	}
	got, err := store.GetContentSubgraph(context.Background(), "twitter", "bridge-1")
	if err != nil {
		t.Fatalf("GetContentSubgraph() error = %v", err)
	}
	if got.ArticleID != "unit-bridge" {
		t.Fatalf("ArticleID = %q, want unit-bridge", got.ArticleID)
	}
	if len(got.Edges) != 1 || got.Edges[0].Type != graphmodel.EdgeTypeDrives {
		t.Fatalf("edges = %#v, want bridged drives edge", got.Edges)
	}
}

func TestSQLiteStore_UpsertContentSubgraphSeedsVerifyQueueForPendingObjects(t *testing.T) {
	root := t.TempDir()
	store, err := NewSQLiteStore(filepath.Join(root, "data", "content.db"))
	if err != nil {
		t.Fatalf("NewSQLiteStore() error = %v", err)
	}
	defer store.Close()

	subgraph := testContentSubgraph()
	subgraph.Nodes = append(subgraph.Nodes,
		graphmodel.GraphNode{
			ID:                 "n2",
			SourceArticleID:    "unit-graph-1",
			SourcePlatform:     "twitter",
			SourceExternalID:   "g1",
			RawText:            "未来一周美股承压",
			SubjectText:        "美股",
			ChangeText:         "未来一周承压",
			Kind:               graphmodel.NodeKindPrediction,
			IsPrimary:          true,
			VerificationStatus: graphmodel.VerificationProved,
		},
	)
	subgraph.Edges = []graphmodel.GraphEdge{{
		ID:                 "e1",
		From:               "n1",
		To:                 "n2",
		Type:               graphmodel.EdgeTypeDrives,
		IsPrimary:          true,
		VerificationStatus: graphmodel.VerificationPending,
	}}

	if err := store.UpsertContentSubgraph(context.Background(), subgraph); err != nil {
		t.Fatalf("UpsertContentSubgraph() error = %v", err)
	}
	items, err := store.ListDueVerifyQueueItems(context.Background(), time.Date(2026, 4, 21, 1, 0, 0, 0, time.UTC), 10)
	if err != nil {
		t.Fatalf("ListDueVerifyQueueItems() error = %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("len(ListDueVerifyQueueItems()) = %d, want 2 pending objects", len(items))
	}
}

func TestSQLiteStore_ApplyVerifyVerdictToContentSubgraphUpdatesNodeAndEdgeStatuses(t *testing.T) {
	root := t.TempDir()
	store, err := NewSQLiteStore(filepath.Join(root, "data", "content.db"))
	if err != nil {
		t.Fatalf("NewSQLiteStore() error = %v", err)
	}
	defer store.Close()

	subgraph := testContentSubgraph()
	subgraph.Nodes = append(subgraph.Nodes, graphmodel.GraphNode{
		ID:                 "n2",
		SourceArticleID:    "unit-graph-1",
		SourcePlatform:     "twitter",
		SourceExternalID:   "g1",
		RawText:            "未来一周美股承压",
		SubjectText:        "美股",
		ChangeText:         "未来一周承压",
		Kind:               graphmodel.NodeKindPrediction,
		IsPrimary:          true,
		VerificationStatus: graphmodel.VerificationPending,
	})
	subgraph.Edges = []graphmodel.GraphEdge{{
		ID:                 "e1",
		From:               "n1",
		To:                 "n2",
		Type:               graphmodel.EdgeTypeDrives,
		IsPrimary:          true,
		VerificationStatus: graphmodel.VerificationPending,
	}}
	if err := store.UpsertContentSubgraph(context.Background(), subgraph); err != nil {
		t.Fatalf("UpsertContentSubgraph() error = %v", err)
	}

	nodeVerdictTime := time.Date(2026, 4, 22, 0, 0, 0, 0, time.UTC)
	edgeVerdictTime := time.Date(2026, 4, 23, 0, 0, 0, 0, time.UTC)
	if err := store.ApplyVerifyVerdictToContentSubgraph(context.Background(), "twitter", "g1", graphmodel.VerifyVerdict{
		ObjectType: graphmodel.VerifyQueueObjectNode,
		ObjectID:   "n2",
		Verdict:    graphmodel.VerificationProved,
		Reason:     "observed market drawdown matched thesis",
		AsOf:       nodeVerdictTime.Format(time.RFC3339),
	}); err != nil {
		t.Fatalf("ApplyVerifyVerdictToContentSubgraph(node) error = %v", err)
	}
	if err := store.ApplyVerifyVerdictToContentSubgraph(context.Background(), "twitter", "g1", graphmodel.VerifyVerdict{
		ObjectType: graphmodel.VerifyQueueObjectEdge,
		ObjectID:   "e1",
		Verdict:    graphmodel.VerificationDisproved,
		Reason:     "causal linkage insufficient",
		AsOf:       edgeVerdictTime.Format(time.RFC3339),
	}); err != nil {
		t.Fatalf("ApplyVerifyVerdictToContentSubgraph(edge) error = %v", err)
	}

	got, err := store.GetContentSubgraph(context.Background(), "twitter", "g1")
	if err != nil {
		t.Fatalf("GetContentSubgraph() error = %v", err)
	}
	var gotNode graphmodel.GraphNode
	for _, node := range got.Nodes {
		if node.ID == "n2" {
			gotNode = node
			break
		}
	}
	if gotNode.VerificationStatus != graphmodel.VerificationProved {
		t.Fatalf("node verification_status = %q, want proved", gotNode.VerificationStatus)
	}
	if gotNode.VerificationReason != "observed market drawdown matched thesis" {
		t.Fatalf("node verification_reason = %q", gotNode.VerificationReason)
	}
	if gotNode.VerificationAsOf != nodeVerdictTime.Format(time.RFC3339) {
		t.Fatalf("node verification_as_of = %q, want %q", gotNode.VerificationAsOf, nodeVerdictTime.Format(time.RFC3339))
	}
	if got.Edges[0].VerificationStatus != graphmodel.VerificationDisproved {
		t.Fatalf("edge verification_status = %q, want disproved", got.Edges[0].VerificationStatus)
	}
	if got.Edges[0].VerificationReason != "causal linkage insufficient" {
		t.Fatalf("edge verification_reason = %q", got.Edges[0].VerificationReason)
	}
}
