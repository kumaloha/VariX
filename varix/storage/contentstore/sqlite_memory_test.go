package contentstore

import (
	"context"
	"path/filepath"
	"sort"
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
	if out.DedupeGroups[0].RepresentativeNodeID == "" || out.DedupeGroups[0].CanonicalText == "" {
		t.Fatalf("dedupe group missing frontend hints: %#v", out.DedupeGroups[0])
	}
}

func TestSQLiteStore_OrganizationCollapsesDuplicateSidesIntoSingleGroupedContradiction(t *testing.T) {
	root := t.TempDir()
	store, err := NewSQLiteStore(filepath.Join(root, "data", "content.db"))
	if err != nil {
		t.Fatalf("NewSQLiteStore() error = %v", err)
	}
	defer store.Close()

	record := compile.Record{
		UnitID:         "weibo:Q2b",
		Source:         "weibo",
		ExternalID:     "Q2b",
		RootExternalID: "Q2b",
		Model:          "qwen3.6-plus",
		Output: compile.Output{
			Summary: "summary",
			Graph: compile.ReasoningGraph{
				Nodes: []compile.GraphNode{
					{ID: "n1", Kind: compile.NodeFact, Text: "油价会上升", ValidFrom: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), ValidTo: time.Date(2026, 12, 31, 0, 0, 0, 0, time.UTC)},
					{ID: "n2", Kind: compile.NodeFact, Text: "油价会走强", ValidFrom: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), ValidTo: time.Date(2026, 12, 31, 0, 0, 0, 0, time.UTC)},
					{ID: "n3", Kind: compile.NodeFact, Text: "油价会下降", ValidFrom: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), ValidTo: time.Date(2026, 12, 31, 0, 0, 0, 0, time.UTC)},
					{ID: "n4", Kind: compile.NodeFact, Text: "油价会下滑", ValidFrom: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), ValidTo: time.Date(2026, 12, 31, 0, 0, 0, 0, time.UTC)},
				},
				Edges: []compile.GraphEdge{{From: "n1", To: "n3", Kind: compile.EdgeNegative}},
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
		UserID:           "u2b",
		SourcePlatform:   "weibo",
		SourceExternalID: "Q2b",
		NodeIDs:          []string{"n1", "n2", "n3", "n4"},
	}); err != nil {
		t.Fatalf("AcceptMemoryNodes() error = %v", err)
	}

	out, err := store.RunNextMemoryOrganizationJob(context.Background(), "u2b", time.Date(2026, 4, 14, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("RunNextMemoryOrganizationJob() error = %v", err)
	}
	if len(out.DedupeGroups) != 2 {
		t.Fatalf("len(DedupeGroups) = %d, want 2", len(out.DedupeGroups))
	}
	if len(out.ContradictionGroups) != 1 {
		t.Fatalf("len(ContradictionGroups) = %d, want 1", len(out.ContradictionGroups))
	}
	got := out.ContradictionGroups[0]
	wantIDs := []string{"n1", "n2", "n3", "n4"}
	if len(got.NodeIDs) != len(wantIDs) {
		t.Fatalf("contradiction group ids = %#v, want %#v", got.NodeIDs, wantIDs)
	}
	for i, want := range wantIDs {
		if got.NodeIDs[i] != want {
			t.Fatalf("contradiction group ids = %#v, want %#v", got.NodeIDs, wantIDs)
		}
	}
	if got.Reason == "" {
		t.Fatalf("contradiction group missing reason: %#v", got)
	}
}

func TestSQLiteStore_OrganizationBuildsHierarchyFromNodeKindsWhenEdgesAreTooCoarse(t *testing.T) {
	root := t.TempDir()
	store, err := NewSQLiteStore(filepath.Join(root, "data", "content.db"))
	if err != nil {
		t.Fatalf("NewSQLiteStore() error = %v", err)
	}
	defer store.Close()

	record := compile.Record{
		UnitID:         "weibo:Q3",
		Source:         "weibo",
		ExternalID:     "Q3",
		RootExternalID: "Q3",
		Model:          "qwen3.6-plus",
		Output: compile.Output{
			Summary: "summary",
			Graph: compile.ReasoningGraph{
				Nodes: []compile.GraphNode{
					{ID: "n1", Kind: compile.NodeFact, Text: "事实A", ValidFrom: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), ValidTo: time.Date(2026, 12, 31, 0, 0, 0, 0, time.UTC)},
					{ID: "n2", Kind: compile.NodeAssumption, Text: "条件B", ValidFrom: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), ValidTo: time.Date(2026, 12, 31, 0, 0, 0, 0, time.UTC)},
					{ID: "n3", Kind: compile.NodeConclusion, Text: "结论C", ValidFrom: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), ValidTo: time.Date(2026, 12, 31, 0, 0, 0, 0, time.UTC)},
				},
				Edges: []compile.GraphEdge{
					{From: "n1", To: "n3", Kind: compile.EdgeDerives},
				},
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
		UserID:           "u3",
		SourcePlatform:   "weibo",
		SourceExternalID: "Q3",
		NodeIDs:          []string{"n1", "n2", "n3"},
	}); err != nil {
		t.Fatalf("AcceptMemoryNodes() error = %v", err)
	}

	out, err := store.RunNextMemoryOrganizationJob(context.Background(), "u3", time.Date(2026, 4, 14, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("RunNextMemoryOrganizationJob() error = %v", err)
	}
	if len(out.Hierarchy) < 2 {
		t.Fatalf("len(Hierarchy) = %d, want at least 2", len(out.Hierarchy))
	}
	var saw12, saw23 bool
	for _, link := range out.Hierarchy {
		if link.ParentNodeID == "n1" && link.ChildNodeID == "n2" {
			saw12 = true
		}
		if link.ParentNodeID == "n2" && link.ChildNodeID == "n3" {
			saw23 = true
		}
	}
	if !saw12 || !saw23 {
		t.Fatalf("hierarchy = %#v, want inferred n1->n2 and n2->n3", out.Hierarchy)
	}
	for _, link := range out.Hierarchy {
		if link.ParentNodeID == "n1" && link.ChildNodeID == "n3" {
			if link.Source != "graph" || link.Hint == "" {
				t.Fatalf("explicit hierarchy link missing frontend hint: %#v", link)
			}
		}
		if (link.ParentNodeID == "n1" && link.ChildNodeID == "n2") || (link.ParentNodeID == "n2" && link.ChildNodeID == "n3") {
			if link.Source != "inferred" || link.Hint == "" {
				t.Fatalf("inferred hierarchy link missing frontend hint: %#v", link)
			}
		}
	}
}

func TestSQLiteStore_OrganizationPlacesExplicitAndImplicitConditionsBetweenFactsAndConclusions(t *testing.T) {
	root := t.TempDir()
	store, err := NewSQLiteStore(filepath.Join(root, "data", "content.db"))
	if err != nil {
		t.Fatalf("NewSQLiteStore() error = %v", err)
	}
	defer store.Close()

	record := compile.Record{
		UnitID:         "weibo:Q3b",
		Source:         "weibo",
		ExternalID:     "Q3b",
		RootExternalID: "Q3b",
		Model:          "qwen3.6-plus",
		Output: compile.Output{
			Summary: "summary",
			Graph: compile.ReasoningGraph{
				Nodes: []compile.GraphNode{
					{ID: "n1", Kind: compile.NodeFact, Text: "事实A", ValidFrom: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), ValidTo: time.Date(2026, 12, 31, 0, 0, 0, 0, time.UTC)},
					{ID: "n2", Kind: compile.NodeKind("显式条件"), Text: "显式条件B", ValidFrom: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), ValidTo: time.Date(2026, 12, 31, 0, 0, 0, 0, time.UTC)},
					{ID: "n3", Kind: compile.NodeAssumption, Text: "隐含条件C", ValidFrom: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), ValidTo: time.Date(2026, 12, 31, 0, 0, 0, 0, time.UTC)},
					{ID: "n4", Kind: compile.NodeConclusion, Text: "结论D", ValidFrom: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), ValidTo: time.Date(2026, 12, 31, 0, 0, 0, 0, time.UTC)},
				},
				Edges: []compile.GraphEdge{
					{From: "n1", To: "n4", Kind: compile.EdgeDerives},
				},
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
		UserID:           "u3b",
		SourcePlatform:   "weibo",
		SourceExternalID: "Q3b",
		NodeIDs:          []string{"n1", "n2", "n3", "n4"},
	}); err != nil {
		t.Fatalf("AcceptMemoryNodes() error = %v", err)
	}

	out, err := store.RunNextMemoryOrganizationJob(context.Background(), "u3b", time.Date(2026, 4, 14, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("RunNextMemoryOrganizationJob() error = %v", err)
	}
	var saw12, saw23, saw34 bool
	for _, link := range out.Hierarchy {
		if link.ParentNodeID == "n1" && link.ChildNodeID == "n2" && link.Hint == "fact-to-explicit-condition" {
			saw12 = true
		}
		if link.ParentNodeID == "n2" && link.ChildNodeID == "n3" && link.Hint == "explicit-condition-to-implicit-condition" {
			saw23 = true
		}
		if link.ParentNodeID == "n3" && link.ChildNodeID == "n4" && link.Hint == "implicit-condition-to-conclusion" {
			saw34 = true
		}
	}
	if !saw12 || !saw23 || !saw34 {
		t.Fatalf("hierarchy = %#v, want inferred fact/explicit/implicit/conclusion ladder", out.Hierarchy)
	}
}

func TestSQLiteStore_OrganizationHierarchySkipsUnverifiableFactParents(t *testing.T) {
	root := t.TempDir()
	store, err := NewSQLiteStore(filepath.Join(root, "data", "content.db"))
	if err != nil {
		t.Fatalf("NewSQLiteStore() error = %v", err)
	}
	defer store.Close()

	record := compile.Record{
		UnitID:         "weibo:Q4",
		Source:         "weibo",
		ExternalID:     "Q4",
		RootExternalID: "Q4",
		Model:          "qwen3.6-plus",
		Output: compile.Output{
			Summary: "summary",
			Graph: compile.ReasoningGraph{
				Nodes: []compile.GraphNode{
					{ID: "n1", Kind: compile.NodeFact, Text: "确定事实A", ValidFrom: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), ValidTo: time.Date(2026, 12, 31, 0, 0, 0, 0, time.UTC)},
					{ID: "n2", Kind: compile.NodeFact, Text: "存疑事实B", ValidFrom: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), ValidTo: time.Date(2026, 12, 31, 0, 0, 0, 0, time.UTC)},
					{ID: "n3", Kind: compile.NodePrediction, Text: "预测C", ValidFrom: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), ValidTo: time.Date(2026, 12, 31, 0, 0, 0, 0, time.UTC)},
				},
				Edges: []compile.GraphEdge{
					{From: "n1", To: "n3", Kind: compile.EdgePositive},
					{From: "n2", To: "n3", Kind: compile.EdgePositive},
				},
			},
			Details: compile.HiddenDetails{Caveats: []string{"detail"}},
			Verification: compile.Verification{
				VerifiedAt: time.Date(2026, 4, 14, 0, 0, 0, 0, time.UTC),
				Model:      "qwen3.6-plus",
				FactChecks: []compile.FactCheck{
					{NodeID: "n1", Status: compile.FactStatusClearlyTrue, Reason: "supported"},
					{NodeID: "n2", Status: compile.FactStatusUnverifiable, Reason: "weak evidence"},
				},
				PredictionChecks: []compile.PredictionCheck{
					{NodeID: "n3", Status: compile.PredictionStatusUnresolved, Reason: "pending", AsOf: time.Date(2026, 4, 14, 0, 0, 0, 0, time.UTC)},
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
		UserID:           "u4",
		SourcePlatform:   "weibo",
		SourceExternalID: "Q4",
		NodeIDs:          []string{"n1", "n2", "n3"},
	}); err != nil {
		t.Fatalf("AcceptMemoryNodes() error = %v", err)
	}

	out, err := store.RunNextMemoryOrganizationJob(context.Background(), "u4", time.Date(2026, 4, 14, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("RunNextMemoryOrganizationJob() error = %v", err)
	}
	for _, link := range out.Hierarchy {
		if link.ParentNodeID == "n2" && link.ChildNodeID == "n3" {
			t.Fatalf("hierarchy contains unverifiable fact parent link: %#v", out.Hierarchy)
		}
	}
	found := false
	for _, link := range out.Hierarchy {
		if link.ParentNodeID == "n1" && link.ChildNodeID == "n3" {
			found = true
		}
	}
	if !found {
		t.Fatalf("hierarchy = %#v, want n1->n3", out.Hierarchy)
	}
}

func TestSQLiteStore_OrganizationBackfillsLegacyNodeValidityFromCurrentCompile(t *testing.T) {
	root := t.TempDir()
	store, err := NewSQLiteStore(filepath.Join(root, "data", "content.db"))
	if err != nil {
		t.Fatalf("NewSQLiteStore() error = %v", err)
	}
	defer store.Close()

	record := compile.Record{
		UnitID:         "weibo:Q4b",
		Source:         "weibo",
		ExternalID:     "Q4b",
		RootExternalID: "Q4b",
		Model:          "qwen3.6-plus",
		Output: compile.Output{
			Summary: "summary",
			Graph: compile.ReasoningGraph{
				Nodes: []compile.GraphNode{
					{ID: "n1", Kind: compile.NodeFact, Text: "事实A", ValidFrom: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), ValidTo: time.Date(2026, 12, 31, 0, 0, 0, 0, time.UTC)},
					{ID: "n2", Kind: compile.NodeConclusion, Text: "结论B", ValidFrom: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), ValidTo: time.Date(2026, 12, 31, 0, 0, 0, 0, time.UTC)},
				},
				Edges: []compile.GraphEdge{{From: "n1", To: "n2", Kind: compile.EdgeDerives}},
			},
			Details:    compile.HiddenDetails{Caveats: []string{"detail"}},
			Confidence: "medium",
		},
		CompiledAt: time.Date(2026, 4, 14, 0, 0, 0, 0, time.UTC),
	}
	if err := store.UpsertCompiledOutput(context.Background(), record); err != nil {
		t.Fatalf("UpsertCompiledOutput() error = %v", err)
	}

	// simulate a legacy accepted node before validity fields existed
	if _, err := store.db.Exec(`INSERT INTO user_memory_nodes(user_id, source_platform, source_external_id, root_external_id, node_id, node_kind, node_text, source_model, source_compiled_at, valid_from, valid_to, accepted_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		"u4b", "weibo", "Q4b", "Q4b", "n1", string(compile.NodeFact), "事实A", "old-model", time.Date(2026, 4, 13, 0, 0, 0, 0, time.UTC).Format(time.RFC3339Nano), "", "", time.Date(2026, 4, 13, 0, 0, 0, 0, time.UTC).Format(time.RFC3339Nano),
	); err != nil {
		t.Fatalf("seed legacy user_memory_nodes error = %v", err)
	}
	if _, err := store.db.Exec(`INSERT INTO memory_acceptance_events(user_id, trigger_type, source_platform, source_external_id, root_external_id, source_model, source_compiled_at, payload_json, accepted_count, accepted_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		"u4b", "accept_single", "weibo", "Q4b", "Q4b", "old-model", time.Date(2026, 4, 13, 0, 0, 0, 0, time.UTC).Format(time.RFC3339Nano), `[{"node_id":"n1","node_kind":"事实","node_text":"事实A"}]`, 1, time.Date(2026, 4, 13, 0, 0, 0, 0, time.UTC).Format(time.RFC3339Nano),
	); err != nil {
		t.Fatalf("seed event error = %v", err)
	}
	if _, err := store.db.Exec(`INSERT INTO memory_organization_jobs(trigger_event_id, user_id, source_platform, source_external_id, status, created_at) VALUES (1, ?, ?, ?, 'queued', ?)`,
		"u4b", "weibo", "Q4b", time.Date(2026, 4, 14, 0, 0, 0, 0, time.UTC).Format(time.RFC3339Nano),
	); err != nil {
		t.Fatalf("seed job error = %v", err)
	}

	out, err := store.RunNextMemoryOrganizationJob(context.Background(), "u4b", time.Date(2026, 4, 14, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("RunNextMemoryOrganizationJob() error = %v", err)
	}
	if len(out.ActiveNodes) != 1 {
		t.Fatalf("len(ActiveNodes) = %d, want 1 after derived backfill", len(out.ActiveNodes))
	}
	if out.ActiveNodes[0].ValidFrom.IsZero() || out.ActiveNodes[0].ValidTo.IsZero() {
		t.Fatalf("active node missing backfilled validity: %#v", out.ActiveNodes[0])
	}
	if len(out.InactiveNodes) != 0 {
		t.Fatalf("len(InactiveNodes) = %d, want 0 after derived backfill", len(out.InactiveNodes))
	}
}

func TestSQLiteStore_OrganizationCollapsesDuplicateSidesIntoSingleContradictionGroup(t *testing.T) {
	root := t.TempDir()
	store, err := NewSQLiteStore(filepath.Join(root, "data", "content.db"))
	if err != nil {
		t.Fatalf("NewSQLiteStore() error = %v", err)
	}
	defer store.Close()

	record := compile.Record{
		UnitID:         "weibo:Q5",
		Source:         "weibo",
		ExternalID:     "Q5",
		RootExternalID: "Q5",
		Model:          "qwen3.6-plus",
		Output: compile.Output{
			Summary: "summary",
			Graph: compile.ReasoningGraph{
				Nodes: []compile.GraphNode{
					{ID: "n1", Kind: compile.NodeFact, Text: "油价会上升。", ValidFrom: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), ValidTo: time.Date(2026, 12, 31, 0, 0, 0, 0, time.UTC)},
					{ID: "n2", Kind: compile.NodeFact, Text: "油价将上行", ValidFrom: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), ValidTo: time.Date(2026, 12, 31, 0, 0, 0, 0, time.UTC)},
					{ID: "n3", Kind: compile.NodeFact, Text: "油价会下降", ValidFrom: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), ValidTo: time.Date(2026, 12, 31, 0, 0, 0, 0, time.UTC)},
					{ID: "n4", Kind: compile.NodeFact, Text: "油价将下行", ValidFrom: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), ValidTo: time.Date(2026, 12, 31, 0, 0, 0, 0, time.UTC)},
				},
				Edges: []compile.GraphEdge{{From: "n1", To: "n3", Kind: compile.EdgeNegative}},
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
		UserID:           "u5",
		SourcePlatform:   "weibo",
		SourceExternalID: "Q5",
		NodeIDs:          []string{"n1", "n2", "n3", "n4"},
	}); err != nil {
		t.Fatalf("AcceptMemoryNodes() error = %v", err)
	}

	out, err := store.RunNextMemoryOrganizationJob(context.Background(), "u5", time.Date(2026, 4, 14, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("RunNextMemoryOrganizationJob() error = %v", err)
	}
	if len(out.DedupeGroups) != 2 {
		t.Fatalf("len(DedupeGroups) = %d, want 2", len(out.DedupeGroups))
	}
	if len(out.ContradictionGroups) != 1 {
		t.Fatalf("len(ContradictionGroups) = %d, want 1 grouped contradiction", len(out.ContradictionGroups))
	}
	gotIDs := append([]string(nil), out.ContradictionGroups[0].NodeIDs...)
	sort.Strings(gotIDs)
	wantIDs := []string{"n1", "n2", "n3", "n4"}
	if len(gotIDs) != len(wantIDs) {
		t.Fatalf("grouped contradiction ids = %#v, want %#v", gotIDs, wantIDs)
	}
	for i := range wantIDs {
		if gotIDs[i] != wantIDs[i] {
			t.Fatalf("grouped contradiction ids = %#v, want %#v", gotIDs, wantIDs)
		}
	}
}
