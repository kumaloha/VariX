package contentstore

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/kumaloha/VariX/varix/compile"
	"github.com/kumaloha/VariX/varix/memory"
)

func seedCompiledRecordForMemory(t *testing.T, store *SQLiteStore) {
	t.Helper()
	record := compile.Record{
		UnitID:         "weibo:Q1",
		Source:         "weibo",
		ExternalID:     "Q1",
		RootExternalID: "Q1",
		Model:          "qwen3.6-plus",
		Output: compile.Output{
			Summary: "summary",
			Graph: compile.ReasoningGraph{
				Nodes: []compile.GraphNode{
					{ID: "n1", Kind: compile.NodeFact, Text: "事实A", ValidFrom: time.Date(2026, 4, 14, 0, 0, 0, 0, time.UTC), ValidTo: time.Date(2026, 7, 14, 0, 0, 0, 0, time.UTC)},
					{ID: "n2", Kind: compile.NodeConclusion, Text: "结论B", ValidFrom: time.Date(2026, 4, 14, 0, 0, 0, 0, time.UTC), ValidTo: time.Date(2026, 7, 14, 0, 0, 0, 0, time.UTC)},
				},
				Edges: []compile.GraphEdge{{From: "n1", To: "n2", Kind: compile.EdgeDerives}},
			},
			Details:    compile.HiddenDetails{Caveats: []string{"detail"}},
			Confidence: "medium",
		},
		CompiledAt: time.Date(2026, 4, 14, 8, 0, 0, 0, time.UTC),
	}
	if err := store.UpsertCompiledOutput(context.Background(), record); err != nil {
		t.Fatalf("UpsertCompiledOutput() error = %v", err)
	}
}

func TestSQLiteStore_AcceptMemoryNodePersistsStateEventAndJob(t *testing.T) {
	root := t.TempDir()
	store, err := NewSQLiteStore(filepath.Join(root, "data", "content.db"))
	if err != nil {
		t.Fatalf("NewSQLiteStore() error = %v", err)
	}
	defer store.Close()
	seedCompiledRecordForMemory(t, store)

	got, err := store.AcceptMemoryNodes(context.Background(), memory.AcceptRequest{
		UserID:           "u1",
		SourcePlatform:   "weibo",
		SourceExternalID: "Q1",
		NodeIDs:          []string{"n1"},
	})
	if err != nil {
		t.Fatalf("AcceptMemoryNodes() error = %v", err)
	}
	if len(got.Nodes) != 1 {
		t.Fatalf("len(Nodes) = %d, want 1", len(got.Nodes))
	}
	if got.Nodes[0].NodeID != "n1" || got.Nodes[0].NodeKind != string(compile.NodeFact) || got.Nodes[0].NodeText != "事实A" {
		t.Fatalf("node = %#v", got.Nodes[0])
	}
	if got.Event.EventID == 0 || got.Job.JobID == 0 {
		t.Fatalf("event/job = %#v / %#v", got.Event, got.Job)
	}
	if got.Job.TriggerEventID != got.Event.EventID {
		t.Fatalf("TriggerEventID = %d, want %d", got.Job.TriggerEventID, got.Event.EventID)
	}
}

func TestSQLiteStore_RepeatAcceptIsIdempotentInStateButAppendsEvent(t *testing.T) {
	root := t.TempDir()
	store, err := NewSQLiteStore(filepath.Join(root, "data", "content.db"))
	if err != nil {
		t.Fatalf("NewSQLiteStore() error = %v", err)
	}
	defer store.Close()
	seedCompiledRecordForMemory(t, store)

	first, err := store.AcceptMemoryNodes(context.Background(), memory.AcceptRequest{
		UserID:           "u1",
		SourcePlatform:   "weibo",
		SourceExternalID: "Q1",
		NodeIDs:          []string{"n1"},
	})
	if err != nil {
		t.Fatalf("AcceptMemoryNodes(first) error = %v", err)
	}
	second, err := store.AcceptMemoryNodes(context.Background(), memory.AcceptRequest{
		UserID:           "u1",
		SourcePlatform:   "weibo",
		SourceExternalID: "Q1",
		NodeIDs:          []string{"n1"},
	})
	if err != nil {
		t.Fatalf("AcceptMemoryNodes(second) error = %v", err)
	}
	nodes, err := store.ListUserMemory(context.Background(), "u1")
	if err != nil {
		t.Fatalf("ListUserMemory() error = %v", err)
	}
	if len(nodes) != 1 {
		t.Fatalf("len(ListUserMemory) = %d, want 1", len(nodes))
	}
	if second.Event.EventID == first.Event.EventID {
		t.Fatalf("second event id = %d, want new event", second.Event.EventID)
	}
	jobs, err := store.ListMemoryJobs(context.Background(), "u1")
	if err != nil {
		t.Fatalf("ListMemoryJobs() error = %v", err)
	}
	if len(jobs) != 2 {
		t.Fatalf("len(ListMemoryJobs) = %d, want 2", len(jobs))
	}
}

func TestSQLiteStore_AcceptMemoryBatchIsAtomic(t *testing.T) {
	root := t.TempDir()
	store, err := NewSQLiteStore(filepath.Join(root, "data", "content.db"))
	if err != nil {
		t.Fatalf("NewSQLiteStore() error = %v", err)
	}
	defer store.Close()
	seedCompiledRecordForMemory(t, store)

	_, err = store.AcceptMemoryNodes(context.Background(), memory.AcceptRequest{
		UserID:           "u1",
		SourcePlatform:   "weibo",
		SourceExternalID: "Q1",
		NodeIDs:          []string{"n1", "missing"},
	})
	if err == nil {
		t.Fatal("AcceptMemoryNodes() error = nil, want invalid node failure")
	}
	nodes, err := store.ListUserMemory(context.Background(), "u1")
	if err != nil {
		t.Fatalf("ListUserMemory() error = %v", err)
	}
	if len(nodes) != 0 {
		t.Fatalf("len(ListUserMemory) = %d, want 0 after failed batch", len(nodes))
	}
	jobs, err := store.ListMemoryJobs(context.Background(), "u1")
	if err != nil {
		t.Fatalf("ListMemoryJobs() error = %v", err)
	}
	if len(jobs) != 0 {
		t.Fatalf("len(ListMemoryJobs) = %d, want 0 after failed batch", len(jobs))
	}
}

func TestSQLiteStore_RunNextMemoryOrganizationJobProducesOutput(t *testing.T) {
	root := t.TempDir()
	store, err := NewSQLiteStore(filepath.Join(root, "data", "content.db"))
	if err != nil {
		t.Fatalf("NewSQLiteStore() error = %v", err)
	}
	defer store.Close()

	record := compile.Record{
		UnitID:         "weibo:Q1",
		Source:         "weibo",
		ExternalID:     "Q1",
		RootExternalID: "Q1",
		Model:          "qwen3.6-plus",
		Output: compile.Output{
			Summary: "summary",
			Graph: compile.ReasoningGraph{
				Nodes: []compile.GraphNode{
					{ID: "n1", Kind: compile.NodeFact, Text: "通胀下降", ValidFrom: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), ValidTo: time.Date(2026, 12, 31, 0, 0, 0, 0, time.UTC)},
					{ID: "n2", Kind: compile.NodeFact, Text: "通胀不下降", ValidFrom: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), ValidTo: time.Date(2026, 12, 31, 0, 0, 0, 0, time.UTC)},
					{ID: "n3", Kind: compile.NodePrediction, Text: "三个月内降息", ValidFrom: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), ValidTo: time.Date(2026, 3, 31, 0, 0, 0, 0, time.UTC)},
				},
				Edges: []compile.GraphEdge{
					{From: "n1", To: "n3", Kind: compile.EdgePositive},
				},
			},
			Details: compile.HiddenDetails{Caveats: []string{"detail"}},
			Verification: compile.Verification{
				VerifiedAt: time.Date(2026, 4, 14, 0, 0, 0, 0, time.UTC),
				Model:      "qwen3.6-plus",
				FactChecks: []compile.FactCheck{
					{NodeID: "n1", Status: compile.FactStatusClearlyTrue, Reason: "data support"},
					{NodeID: "n2", Status: compile.FactStatusUnverifiable, Reason: "insufficient evidence"},
				},
				PredictionChecks: []compile.PredictionCheck{
					{NodeID: "n3", Status: compile.PredictionStatusStaleUnresolved, Reason: "window passed", AsOf: time.Date(2026, 4, 14, 0, 0, 0, 0, time.UTC)},
				},
			},
			Confidence: "medium",
		},
		CompiledAt: time.Date(2026, 4, 14, 0, 0, 0, 0, time.UTC),
	}
	if err := store.UpsertCompiledOutput(context.Background(), record); err != nil {
		t.Fatalf("UpsertCompiledOutput() error = %v", err)
	}

	if _, err := store.AcceptMemoryNodes(context.Background(), memory.AcceptRequest{
		UserID:           "u1",
		SourcePlatform:   "weibo",
		SourceExternalID: "Q1",
		NodeIDs:          []string{"n1", "n2", "n3"},
	}); err != nil {
		t.Fatalf("AcceptMemoryNodes() error = %v", err)
	}

	out, err := store.RunNextMemoryOrganizationJob(context.Background(), "u1", time.Date(2026, 4, 14, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("RunNextMemoryOrganizationJob() error = %v", err)
	}
	if len(out.ActiveNodes) != 2 {
		t.Fatalf("len(ActiveNodes) = %d, want 2", len(out.ActiveNodes))
	}
	if len(out.InactiveNodes) != 1 {
		t.Fatalf("len(InactiveNodes) = %d, want 1", len(out.InactiveNodes))
	}
	if len(out.ContradictionGroups) != 1 {
		t.Fatalf("len(ContradictionGroups) = %d, want 1", len(out.ContradictionGroups))
	}
	if len(out.FactVerifications) != 2 {
		t.Fatalf("len(FactVerifications) = %d, want 2", len(out.FactVerifications))
	}
	if len(out.PredictionStatuses) != 1 || out.PredictionStatuses[0].Status != string(compile.PredictionStatusStaleUnresolved) {
		t.Fatalf("PredictionStatuses = %#v", out.PredictionStatuses)
	}
	got, err := store.GetLatestMemoryOrganizationOutput(context.Background(), "u1", "weibo", "Q1")
	if err != nil {
		t.Fatalf("GetLatestMemoryOrganizationOutput() error = %v", err)
	}
	if got.OutputID == 0 {
		t.Fatalf("OutputID = %d, want non-zero", got.OutputID)
	}
	if got.JobID != out.JobID {
		t.Fatalf("JobID = %d, want %d", got.JobID, out.JobID)
	}
}

func TestSQLiteStore_OrganizationDetectsNearDuplicateAndAntonymContradiction(t *testing.T) {
	root := t.TempDir()
	store, err := NewSQLiteStore(filepath.Join(root, "data", "content.db"))
	if err != nil {
		t.Fatalf("NewSQLiteStore() error = %v", err)
	}
	defer store.Close()

	record := compile.Record{
		UnitID:         "weibo:Q2",
		Source:         "weibo",
		ExternalID:     "Q2",
		RootExternalID: "Q2",
		Model:          "qwen3.6-plus",
		Output: compile.Output{
			Summary: "summary",
			Graph: compile.ReasoningGraph{
				Nodes: []compile.GraphNode{
					{ID: "n1", Kind: compile.NodeFact, Text: "美国安全信誉下降会削弱石油美元回流。", ValidFrom: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), ValidTo: time.Date(2026, 12, 31, 0, 0, 0, 0, time.UTC)},
					{ID: "n2", Kind: compile.NodeFact, Text: "美国安全信誉下滑会削弱石油美元回流", ValidFrom: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), ValidTo: time.Date(2026, 12, 31, 0, 0, 0, 0, time.UTC)},
					{ID: "n3", Kind: compile.NodeFact, Text: "油价会上升", ValidFrom: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), ValidTo: time.Date(2026, 12, 31, 0, 0, 0, 0, time.UTC)},
					{ID: "n4", Kind: compile.NodeFact, Text: "油价会下降", ValidFrom: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), ValidTo: time.Date(2026, 12, 31, 0, 0, 0, 0, time.UTC)},
				},
				Edges: []compile.GraphEdge{{From: "n1", To: "n3", Kind: compile.EdgePositive}},
			},
			Details:    compile.HiddenDetails{Caveats: []string{"detail"}},
			Confidence: "medium",
		},
		CompiledAt: time.Date(2026, 4, 14, 0, 0, 0, 0, time.UTC),
	}
	if err := store.UpsertCompiledOutput(context.Background(), record); err != nil {
		t.Fatalf("UpsertCompiledOutput() error = %v", err)
	}
	if _, err := store.AcceptMemoryNodes(context.Background(), memory.AcceptRequest{
		UserID:           "u2",
		SourcePlatform:   "weibo",
		SourceExternalID: "Q2",
		NodeIDs:          []string{"n1", "n2", "n3", "n4"},
	}); err != nil {
		t.Fatalf("AcceptMemoryNodes() error = %v", err)
	}
	out, err := store.RunNextMemoryOrganizationJob(context.Background(), "u2", time.Date(2026, 4, 14, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("RunNextMemoryOrganizationJob() error = %v", err)
	}
	if len(out.DedupeGroups) != 1 {
		t.Fatalf("len(DedupeGroups) = %d, want 1", len(out.DedupeGroups))
	}
	if len(out.DedupeGroups[0].NodeIDs) != 2 {
		t.Fatalf("dedupe group = %#v, want 2 ids", out.DedupeGroups[0])
	}
	if len(out.ContradictionGroups) != 1 {
		t.Fatalf("len(ContradictionGroups) = %d, want 1", len(out.ContradictionGroups))
	}
	if len(out.ContradictionGroups[0].NodeIDs) != 2 {
		t.Fatalf("contradiction group = %#v, want 2 ids", out.ContradictionGroups[0])
	}
}
