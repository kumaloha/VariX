package contentstore

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/kumaloha/VariX/varix/compile"
	"github.com/kumaloha/VariX/varix/ingest/types"
)

func TestSQLiteStore_MarkProcessedAndIsProcessed(t *testing.T) {
	root := t.TempDir()
	store, err := NewSQLiteStore(filepath.Join(root, "data", "content.db"))
	if err != nil {
		t.Fatalf("NewSQLiteStore() error = %v", err)
	}
	defer store.Close()

	ok, err := store.IsProcessed(context.Background(), "twitter", "12345")
	if err != nil {
		t.Fatalf("IsProcessed() error = %v", err)
	}
	if ok {
		t.Fatal("expected item to be absent before MarkProcessed")
	}

	err = store.MarkProcessed(context.Background(), types.ProcessedRecord{
		Platform:    "twitter",
		ExternalID:  "12345",
		URL:         "https://x.com/a/status/12345",
		Author:      "alice",
		ProcessedAt: time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("MarkProcessed() error = %v", err)
	}

	ok, err = store.IsProcessed(context.Background(), "twitter", "12345")
	if err != nil {
		t.Fatalf("IsProcessed() error = %v", err)
	}
	if !ok {
		t.Fatal("expected item to be present after MarkProcessed")
	}
}

func TestSQLiteStore_RegisterListAndUpdateFollow(t *testing.T) {
	root := t.TempDir()
	store, err := NewSQLiteStore(filepath.Join(root, "data", "content.db"))
	if err != nil {
		t.Fatalf("NewSQLiteStore() error = %v", err)
	}
	defer store.Close()

	target := types.FollowTarget{
		Kind:       types.KindNative,
		Platform:   "weibo",
		PlatformID: "123456",
		Locator:    "https://weibo.com/123456",
		URL:        "https://weibo.com/123456",
		FollowedAt: time.Now().UTC(),
	}
	if err := store.RegisterFollow(context.Background(), target); err != nil {
		t.Fatalf("RegisterFollow() error = %v", err)
	}

	items, warnings, err := store.ListFollows(context.Background())
	if err != nil {
		t.Fatalf("ListFollows() error = %v", err)
	}
	if len(warnings) != 0 {
		t.Fatalf("len(ListFollows() warnings) = %d, want 0", len(warnings))
	}
	if len(items) != 1 {
		t.Fatalf("len(ListFollows()) = %d, want 1", len(items))
	}

	now := time.Now().UTC()
	if err := store.UpdateFollowPolled(context.Background(), types.KindNative, "weibo", "https://weibo.com/123456", now); err != nil {
		t.Fatalf("UpdateFollowPolled() error = %v", err)
	}

	items, warnings, err = store.ListFollows(context.Background())
	if err != nil {
		t.Fatalf("ListFollows() error = %v", err)
	}
	if len(warnings) != 0 {
		t.Fatalf("len(ListFollows() warnings) = %d, want 0", len(warnings))
	}
	if items[0].LastPolledAt.IsZero() {
		t.Fatal("LastPolledAt was not updated")
	}
}

func TestSQLiteStore_RemoveFollow(t *testing.T) {
	root := t.TempDir()
	store, err := NewSQLiteStore(filepath.Join(root, "data", "content.db"))
	if err != nil {
		t.Fatalf("NewSQLiteStore() error = %v", err)
	}
	defer store.Close()

	target := types.FollowTarget{
		Kind:       types.KindRSS,
		Platform:   "rss",
		Locator:    "https://feeds.example.test/feed.xml",
		URL:        "https://feeds.example.test/feed.xml",
		FollowedAt: time.Now().UTC(),
	}
	if err := store.RegisterFollow(context.Background(), target); err != nil {
		t.Fatalf("RegisterFollow() error = %v", err)
	}
	if err := store.RemoveFollow(context.Background(), types.KindRSS, "rss", "https://feeds.example.test/feed.xml"); err != nil {
		t.Fatalf("RemoveFollow() error = %v", err)
	}

	items, warnings, err := store.ListFollows(context.Background())
	if err != nil {
		t.Fatalf("ListFollows() error = %v", err)
	}
	if len(warnings) != 0 {
		t.Fatalf("len(ListFollows() warnings) = %d, want 0", len(warnings))
	}
	if len(items) != 0 {
		t.Fatalf("len(ListFollows()) = %d, want 0", len(items))
	}
}

func TestSQLiteStore_RegisterFollowRejectsInvalidRecord(t *testing.T) {
	root := t.TempDir()
	store, err := NewSQLiteStore(filepath.Join(root, "data", "content.db"))
	if err != nil {
		t.Fatalf("NewSQLiteStore() error = %v", err)
	}
	defer store.Close()

	err = store.RegisterFollow(context.Background(), types.FollowTarget{
		Kind:     types.KindSearch,
		Platform: "twitter",
	})
	if err == nil {
		t.Fatal("expected RegisterFollow() to reject invalid follow target")
	}
}

func TestSQLiteStore_RecordPollReportPersistsRunAndTargets(t *testing.T) {
	root := t.TempDir()
	store, err := NewSQLiteStore(filepath.Join(root, "data", "content.db"))
	if err != nil {
		t.Fatalf("NewSQLiteStore() error = %v", err)
	}
	defer store.Close()

	report := types.PollReport{
		StartedAt:         time.Now().UTC().Add(-time.Minute),
		FinishedAt:        time.Now().UTC(),
		TargetCount:       2,
		DiscoveredCount:   3,
		FetchedCount:      2,
		SkippedCount:      1,
		StoreWarningCount: 0,
		PollWarningCount:  1,
		Targets: []types.TargetPollReport{
			{Target: "search:twitter:nvda", DiscoveredCount: 2, FetchedCount: 1, SkippedCount: 1, WarningCount: 0, Status: "ok"},
			{Target: "rss:rss:https://example.com/feed.xml", DiscoveredCount: 1, FetchedCount: 1, SkippedCount: 0, WarningCount: 1, Status: "warning", ErrorDetail: "hydrate failed"},
		},
	}

	if err := store.RecordPollReport(context.Background(), report); err != nil {
		t.Fatalf("RecordPollReport() error = %v", err)
	}

	var runCount int
	if err := store.db.QueryRow(`SELECT COUNT(*) FROM poll_runs`).Scan(&runCount); err != nil {
		t.Fatalf("QueryRow(poll_runs) error = %v", err)
	}
	if runCount != 1 {
		t.Fatalf("poll_runs count = %d, want 1", runCount)
	}

	var targetCount int
	if err := store.db.QueryRow(`SELECT COUNT(*) FROM poll_target_runs`).Scan(&targetCount); err != nil {
		t.Fatalf("QueryRow(poll_target_runs) error = %v", err)
	}
	if targetCount != 2 {
		t.Fatalf("poll_target_runs count = %d, want 2", targetCount)
	}
}

func testCompileGraphNode(id string, kind compile.NodeKind, text string) compile.GraphNode {
	return compile.GraphNode{
		ID:        id,
		Kind:      kind,
		Text:      text,
		ValidFrom: time.Date(2026, 4, 14, 0, 0, 0, 0, time.UTC),
		ValidTo:   time.Date(2026, 7, 14, 0, 0, 0, 0, time.UTC),
	}
}

func TestSQLiteStore_UpsertAndGetCompiledOutput(t *testing.T) {
	root := t.TempDir()
	store, err := NewSQLiteStore(filepath.Join(root, "data", "content.db"))
	if err != nil {
		t.Fatalf("NewSQLiteStore() error = %v", err)
	}
	defer store.Close()

	record := compile.Record{
		UnitID:         "twitter:123",
		Source:         "twitter",
		ExternalID:     "123",
		RootExternalID: "100",
		Model:          "qwen3.6-plus",
		Output: compile.Output{
			Summary:           "summary text",
			Drivers:           []string{"driver"},
			Targets:           []string{"target"},
			TransmissionPaths: []compile.TransmissionPath{{Driver: "driver", Target: "target", Steps: []string{"step"}}},
			Branches: []compile.Branch{{
				ID:                "s1",
				Level:             "primary",
				Policy:            "forecast_inference",
				Thesis:            "branch thesis",
				Anchors:           []string{"anchor"},
				BranchDrivers:     []string{"branch driver"},
				Drivers:           []string{"driver"},
				Targets:           []string{"target"},
				TransmissionPaths: []compile.TransmissionPath{{Driver: "driver", Target: "target", Steps: []string{"step"}}},
			}},
			Graph: compile.ReasoningGraph{
				Nodes: []compile.GraphNode{
					testCompileGraphNode("n1", compile.NodeFact, "fact"),
					testCompileGraphNode("n2", compile.NodeConclusion, "conclusion"),
				},
				Edges: []compile.GraphEdge{
					{From: "n1", To: "n2", Kind: compile.EdgePositive},
				},
			},
			Details:    compile.HiddenDetails{Caveats: []string{"detail"}},
			Topics:     []string{"topic-a"},
			Confidence: "medium",
			AuthorValidation: compile.AuthorValidation{
				Version: "author_validate_v1",
				Summary: compile.AuthorValidationSummary{
					Verdict:         "mixed",
					SupportedClaims: 1,
				},
				ClaimChecks: []compile.AuthorClaimCheck{{
					ClaimID: "claim-001",
					Text:    "driver",
					Status:  compile.AuthorClaimSupported,
				}},
			},
		},
		CompiledAt: time.Now().UTC(),
	}

	if err := store.UpsertCompiledOutput(context.Background(), record); err != nil {
		t.Fatalf("UpsertCompiledOutput() error = %v", err)
	}
	got, err := store.GetCompiledOutput(context.Background(), "twitter", "123")
	if err != nil {
		t.Fatalf("GetCompiledOutput() error = %v", err)
	}
	if got.Model != "qwen3.6-plus" {
		t.Fatalf("Model = %q", got.Model)
	}
	if got.Output.Summary != "summary text" {
		t.Fatalf("Summary = %q", got.Output.Summary)
	}
	if got.RootExternalID != "100" {
		t.Fatalf("RootExternalID = %q", got.RootExternalID)
	}
	if len(got.Output.Branches) != 1 {
		t.Fatalf("Branches = %#v, want persisted branch", got.Output.Branches)
	}
	if got.Output.Branches[0].Thesis != "branch thesis" {
		t.Fatalf("Branch thesis = %q", got.Output.Branches[0].Thesis)
	}
	if len(got.Output.Branches[0].Anchors) != 1 || got.Output.Branches[0].Anchors[0] != "anchor" {
		t.Fatalf("Branch anchors = %#v", got.Output.Branches[0].Anchors)
	}
	if len(got.Output.Branches[0].BranchDrivers) != 1 || got.Output.Branches[0].BranchDrivers[0] != "branch driver" {
		t.Fatalf("Branch drivers = %#v", got.Output.Branches[0].BranchDrivers)
	}
	if len(got.Output.Branches[0].TransmissionPaths) != 1 || got.Output.Branches[0].TransmissionPaths[0].Target != "target" {
		t.Fatalf("Branch transmission paths = %#v", got.Output.Branches[0].TransmissionPaths)
	}
	if got.Output.AuthorValidation.Summary.Verdict != "mixed" || len(got.Output.AuthorValidation.ClaimChecks) != 1 {
		t.Fatalf("AuthorValidation = %#v, want persisted author validation", got.Output.AuthorValidation)
	}
}

func TestSQLiteStore_UpsertCompiledOutputBridgesLegacyEmbeddedVerificationToVerificationTable(t *testing.T) {
	root := t.TempDir()
	store, err := NewSQLiteStore(filepath.Join(root, "data", "content.db"))
	if err != nil {
		t.Fatalf("NewSQLiteStore() error = %v", err)
	}
	defer store.Close()

	record := compile.Record{
		UnitID:         "twitter:legacy-verify",
		Source:         "twitter",
		ExternalID:     "legacy-verify",
		RootExternalID: "legacy-root",
		Model:          "qwen3.6-plus",
		Output: compile.Output{
			Summary: "summary text",
			Graph: compile.ReasoningGraph{
				Nodes: []compile.GraphNode{
					testCompileGraphNode("n1", compile.NodeFact, "fact"),
					testCompileGraphNode("n2", compile.NodeConclusion, "conclusion"),
				},
				Edges: []compile.GraphEdge{
					{From: "n1", To: "n2", Kind: compile.EdgePositive},
				},
			},
			Details:    compile.HiddenDetails{Caveats: []string{"detail"}},
			Confidence: "medium",
			Verification: compile.Verification{
				Model: "verify-model",
				FactChecks: []compile.FactCheck{{
					NodeID: "n1",
					Status: compile.FactStatusClearlyTrue,
					Reason: "supported",
				}},
				VerifiedAt: time.Now().UTC(),
			},
		},
		CompiledAt: time.Now().UTC(),
	}

	if err := store.UpsertCompiledOutput(context.Background(), record); err != nil {
		t.Fatalf("UpsertCompiledOutput() error = %v", err)
	}

	gotCompile, err := store.GetCompiledOutput(context.Background(), "twitter", "legacy-verify")
	if err != nil {
		t.Fatalf("GetCompiledOutput() error = %v", err)
	}
	if !gotCompile.Output.Verification.IsZero() {
		t.Fatalf("compiled output verification = %#v, want compile store decoupled from verification payload", gotCompile.Output.Verification)
	}

	gotVerify, err := store.GetVerificationResult(context.Background(), "twitter", "legacy-verify")
	if err != nil {
		t.Fatalf("GetVerificationResult() error = %v", err)
	}
	if gotVerify.Model != "verify-model" {
		t.Fatalf("verification model = %q, want verify-model", gotVerify.Model)
	}
	if len(gotVerify.Verification.FactChecks) != 1 || gotVerify.Verification.FactChecks[0].NodeID != "n1" {
		t.Fatalf("verification = %#v, want bridged fact check", gotVerify.Verification)
	}
}
