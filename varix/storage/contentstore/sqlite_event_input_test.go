package contentstore

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/kumaloha/VariX/varix/graphmodel"
)

func TestSQLiteStore_BuildEventInputCandidatesFromMemoryContentGraphs(t *testing.T) {
	root := t.TempDir()
	store, err := NewSQLiteStore(filepath.Join(root, "data", "content.db"))
	if err != nil {
		t.Fatalf("NewSQLiteStore() error = %v", err)
	}
	defer store.Close()

	now := time.Date(2026, 4, 21, 0, 0, 0, 0, time.UTC)
	sg1 := graphmodel.ContentSubgraph{
		ID:               "sg1",
		ArticleID:        "unit-1",
		SourcePlatform:   "twitter",
		SourceExternalID: "a1",
		CompileVersion:   graphmodel.CompileBridgeVersion,
		CompiledAt:       now.Format(time.RFC3339),
		UpdatedAt:        now.Format(time.RFC3339),
		Nodes: []graphmodel.GraphNode{
			{ID: "d1", SourceArticleID: "unit-1", SourcePlatform: "twitter", SourceExternalID: "a1", RawText: "美联储加息0.25%", SubjectText: "美联储", ChangeText: "加息0.25%", Kind: graphmodel.NodeKindObservation, GraphRole: graphmodel.GraphRoleDriver, IsPrimary: true, VerificationStatus: graphmodel.VerificationPending, TimeBucket: "1w"},
			{ID: "t1", SourceArticleID: "unit-1", SourcePlatform: "twitter", SourceExternalID: "a1", RawText: "未来一周美股承压", SubjectText: "美股", ChangeText: "承压", Kind: graphmodel.NodeKindPrediction, GraphRole: graphmodel.GraphRoleTarget, IsPrimary: true, VerificationStatus: graphmodel.VerificationPending, TimeBucket: "1w"},
		},
	}
	sg2 := graphmodel.ContentSubgraph{
		ID:               "sg2",
		ArticleID:        "unit-2",
		SourcePlatform:   "twitter",
		SourceExternalID: "a2",
		CompileVersion:   graphmodel.CompileBridgeVersion,
		CompiledAt:       now.Format(time.RFC3339),
		UpdatedAt:        now.Format(time.RFC3339),
		Nodes: []graphmodel.GraphNode{
			{ID: "d2", SourceArticleID: "unit-2", SourcePlatform: "twitter", SourceExternalID: "a2", RawText: "联储继续收紧", SubjectText: "美联储", ChangeText: "继续收紧", Kind: graphmodel.NodeKindObservation, GraphRole: graphmodel.GraphRoleDriver, IsPrimary: true, VerificationStatus: graphmodel.VerificationPending, TimeBucket: "1w"},
			{ID: "t2", SourceArticleID: "unit-2", SourcePlatform: "twitter", SourceExternalID: "a2", RawText: "最近一周美股回撤", SubjectText: "美股", ChangeText: "回撤", Kind: graphmodel.NodeKindObservation, GraphRole: graphmodel.GraphRoleTarget, IsPrimary: true, VerificationStatus: graphmodel.VerificationPending, TimeBucket: "1w"},
		},
	}
	if err := store.UpsertContentSubgraph(context.Background(), sg1); err != nil {
		t.Fatalf("UpsertContentSubgraph(sg1) error = %v", err)
	}
	if err := store.UpsertContentSubgraph(context.Background(), sg2); err != nil {
		t.Fatalf("UpsertContentSubgraph(sg2) error = %v", err)
	}
	if err := store.PersistMemoryContentGraph(context.Background(), "u-event", sg1, now); err != nil {
		t.Fatalf("PersistMemoryContentGraph(sg1) error = %v", err)
	}
	if err := store.PersistMemoryContentGraph(context.Background(), "u-event", sg2, now); err != nil {
		t.Fatalf("PersistMemoryContentGraph(sg2) error = %v", err)
	}

	candidates, err := store.BuildEventInputCandidates(context.Background(), "u-event")
	if err != nil {
		t.Fatalf("BuildEventInputCandidates() error = %v", err)
	}
	if len(candidates) != 2 {
		t.Fatalf("len(BuildEventInputCandidates()) = %d, want 2 buckets", len(candidates))
	}
	byScope := map[string]EventInputCandidate{}
	for _, candidate := range candidates {
		byScope[candidate.Scope] = candidate
	}
	target := byScope["target"]
	if target.AnchorSubject != "美股" || len(target.SourceSubgraphIDs) != 2 {
		t.Fatalf("target candidate = %#v, want merged 美股 target bucket", target)
	}
	driver := byScope["driver"]
	if driver.AnchorSubject != "美联储" || len(driver.SourceSubgraphIDs) != 2 {
		t.Fatalf("driver candidate = %#v, want merged 美联储 driver bucket", driver)
	}
}
