package contentstore

import (
	"context"
	"path/filepath"
	"sort"
	"strings"
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

func TestSQLiteStore_AcceptMemoryNodesDeriveValidityFromOccurredAndPredictionTimes(t *testing.T) {
	root := t.TempDir()
	store, err := NewSQLiteStore(filepath.Join(root, "data", "content.db"))
	if err != nil {
		t.Fatalf("NewSQLiteStore() error = %v", err)
	}
	defer store.Close()

	occurredAt := time.Date(2026, 4, 10, 9, 0, 0, 0, time.UTC)
	predictionStart := time.Date(2026, 4, 12, 0, 0, 0, 0, time.UTC)
	predictionDue := time.Date(2026, 4, 20, 0, 0, 0, 0, time.UTC)
	record := compile.Record{
		UnitID:         "weibo:Q-time",
		Source:         "weibo",
		ExternalID:     "Q-time",
		RootExternalID: "Q-time",
		Model:          "qwen3.6-plus",
		Output: compile.Output{
			Summary: "summary",
			Graph: compile.ReasoningGraph{
				Nodes: []compile.GraphNode{
					{ID: "n1", Kind: compile.NodeFact, Text: "事实A", OccurredAt: occurredAt},
					{ID: "n2", Kind: compile.NodePrediction, Text: "预测B", PredictionStartAt: predictionStart, PredictionDueAt: predictionDue},
					{ID: "n3", Kind: compile.NodeConclusion, Text: "结论C"},
				},
				Edges: []compile.GraphEdge{{From: "n1", To: "n3", Kind: compile.EdgeDerives}, {From: "n3", To: "n2", Kind: compile.EdgeDerives}},
			},
			Details: compile.HiddenDetails{Caveats: []string{"detail"}},
			Verification: compile.Verification{
				PredictionChecks: []compile.PredictionCheck{{
					NodeID: "n2", Status: compile.PredictionStatusUnresolved, Reason: "window open", AsOf: predictionStart,
				}},
			},
			Confidence: "medium",
		},
		CompiledAt: time.Date(2026, 4, 14, 8, 0, 0, 0, time.UTC),
	}
	if err := store.UpsertCompiledOutput(context.Background(), record); err != nil {
		t.Fatalf("UpsertCompiledOutput() error = %v", err)
	}

	got, err := store.AcceptMemoryNodes(context.Background(), memory.AcceptRequest{
		UserID:           "u-time",
		SourcePlatform:   "weibo",
		SourceExternalID: "Q-time",
		NodeIDs:          []string{"n1", "n2"},
	})
	if err != nil {
		t.Fatalf("AcceptMemoryNodes() error = %v", err)
	}
	if len(got.Nodes) != 2 {
		t.Fatalf("len(Nodes) = %d, want 2", len(got.Nodes))
	}
	if !got.Nodes[0].ValidFrom.Equal(occurredAt) {
		t.Fatalf("fact ValidFrom = %s, want %s", got.Nodes[0].ValidFrom, occurredAt)
	}
	if got.Nodes[0].ValidTo.Year() != 9999 {
		t.Fatalf("fact ValidTo = %s, want open-ended year 9999", got.Nodes[0].ValidTo)
	}
	if !got.Nodes[1].ValidFrom.Equal(predictionStart) || !got.Nodes[1].ValidTo.Equal(predictionDue) {
		t.Fatalf("prediction validity = %s..%s, want %s..%s", got.Nodes[1].ValidFrom, got.Nodes[1].ValidTo, predictionStart, predictionDue)
	}
	if !got.Event.AcceptedNodeState[0].ValidFrom.Equal(occurredAt) {
		t.Fatalf("event fact ValidFrom = %s, want %s", got.Event.AcceptedNodeState[0].ValidFrom, occurredAt)
	}
	if !got.Event.AcceptedNodeState[1].ValidTo.Equal(predictionDue) {
		t.Fatalf("event prediction ValidTo = %s, want %s", got.Event.AcceptedNodeState[1].ValidTo, predictionDue)
	}

	out, err := store.RunNextMemoryOrganizationJob(context.Background(), "u-time", time.Date(2026, 4, 25, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("RunNextMemoryOrganizationJob() error = %v", err)
	}
	if len(out.ActiveNodes) != 1 || out.ActiveNodes[0].NodeID != "n1" {
		t.Fatalf("ActiveNodes = %#v, want only fact node active after prediction due date", out.ActiveNodes)
	}
	if len(out.InactiveNodes) != 1 || out.InactiveNodes[0].NodeID != "n2" {
		t.Fatalf("InactiveNodes = %#v, want only prediction node inactive after due date", out.InactiveNodes)
	}
}

func TestSQLiteStore_AcceptMemoryNodesDeriveQuarterDueAtFromParsedPredictionText(t *testing.T) {
	root := t.TempDir()
	store, err := NewSQLiteStore(filepath.Join(root, "data", "content.db"))
	if err != nil {
		t.Fatalf("NewSQLiteStore() error = %v", err)
	}
	defer store.Close()

	raw := `{
	  "summary":"一句话",
	  "graph":{
	    "nodes":[
	      {"id":"n1","kind":"事实","text":"事实A","occurred_at":"2026-04-10T09:00:00Z"},
	      {"id":"n2","kind":"预测","text":"下季度市场会承压","prediction_start_at":"2026-04-12T00:00:00Z"},
	      {"id":"n3","kind":"结论","text":"结论C"}
	    ],
	    "edges":[
	      {"from":"n1","to":"n3","kind":"推出"},
	      {"from":"n3","to":"n2","kind":"推出"}
	    ]
	  },
	  "details":{"caveats":["detail"]},
	  "confidence":"medium"
	}`
	output, err := compile.ParseOutput(raw)
	if err != nil {
		t.Fatalf("ParseOutput() error = %v", err)
	}
	record := compile.Record{
		UnitID:         "weibo:Q-quarter",
		Source:         "weibo",
		ExternalID:     "Q-quarter",
		RootExternalID: "Q-quarter",
		Model:          "qwen3.6-plus",
		Output:         output,
		CompiledAt:     time.Date(2026, 4, 14, 8, 0, 0, 0, time.UTC),
	}
	if err := store.UpsertCompiledOutput(context.Background(), record); err != nil {
		t.Fatalf("UpsertCompiledOutput() error = %v", err)
	}

	got, err := store.AcceptMemoryNodes(context.Background(), memory.AcceptRequest{
		UserID:           "u-quarter",
		SourcePlatform:   "weibo",
		SourceExternalID: "Q-quarter",
		NodeIDs:          []string{"n2"},
	})
	if err != nil {
		t.Fatalf("AcceptMemoryNodes() error = %v", err)
	}
	if len(got.Nodes) != 1 {
		t.Fatalf("len(Nodes) = %d, want 1", len(got.Nodes))
	}
	wantStart := time.Date(2026, 4, 12, 0, 0, 0, 0, time.UTC)
	wantDue := time.Date(2026, 9, 30, 23, 59, 59, 0, time.UTC)
	if !got.Nodes[0].ValidFrom.Equal(wantStart) || !got.Nodes[0].ValidTo.Equal(wantDue) {
		t.Fatalf("prediction validity = %s..%s, want %s..%s", got.Nodes[0].ValidFrom, got.Nodes[0].ValidTo, wantStart, wantDue)
	}
}

func TestSQLiteStore_AcceptMemoryNodesUseInferredPredictionDueAtForOrganizer(t *testing.T) {
	root := t.TempDir()
	store, err := NewSQLiteStore(filepath.Join(root, "data", "content.db"))
	if err != nil {
		t.Fatalf("NewSQLiteStore() error = %v", err)
	}
	defer store.Close()

	output, err := compile.ParseOutput(`{
	  "summary":"summary",
	  "graph":{
	    "nodes":[
	      {"id":"n1","kind":"事实","text":"事实A","occurred_at":"2026-04-14T00:00:00Z"},
	      {"id":"n2","kind":"预测","text":"下季度市场会承压","prediction_start_at":"2026-04-14T00:00:00Z"}
	    ],
	    "edges":[{"from":"n1","to":"n2","kind":"推出"}]
	  },
	  "details":{"caveats":["detail"]},
	  "confidence":"medium"
	}`)
	if err != nil {
		t.Fatalf("ParseOutput() error = %v", err)
	}

	record := compile.Record{
		UnitID:         "weibo:Q-quarter",
		Source:         "weibo",
		ExternalID:     "Q-quarter",
		RootExternalID: "Q-quarter",
		Model:          "qwen3.6-plus",
		Output:         output,
		CompiledAt:     time.Date(2026, 4, 14, 8, 0, 0, 0, time.UTC),
	}
	if err := store.UpsertCompiledOutput(context.Background(), record); err != nil {
		t.Fatalf("UpsertCompiledOutput() error = %v", err)
	}

	got, err := store.AcceptMemoryNodes(context.Background(), memory.AcceptRequest{
		UserID:           "u-quarter",
		SourcePlatform:   "weibo",
		SourceExternalID: "Q-quarter",
		NodeIDs:          []string{"n1", "n2"},
	})
	if err != nil {
		t.Fatalf("AcceptMemoryNodes() error = %v", err)
	}
	wantDue := time.Date(2026, 9, 30, 23, 59, 59, 0, time.UTC)
	if !got.Nodes[1].ValidTo.Equal(wantDue) {
		t.Fatalf("prediction ValidTo = %s, want %s", got.Nodes[1].ValidTo, wantDue)
	}

	out, err := store.RunNextMemoryOrganizationJob(context.Background(), "u-quarter", time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("RunNextMemoryOrganizationJob() error = %v", err)
	}
	if len(out.ActiveNodes) != 1 || out.ActiveNodes[0].NodeID != "n1" {
		t.Fatalf("ActiveNodes = %#v, want only fact node active after inferred prediction window", out.ActiveNodes)
	}
	if len(out.InactiveNodes) != 1 || out.InactiveNodes[0].NodeID != "n2" {
		t.Fatalf("InactiveNodes = %#v, want inferred-window prediction inactive after quarter end", out.InactiveNodes)
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
	var saw12, saw13, saw34 bool
	for _, link := range out.Hierarchy {
		if link.ParentNodeID == "n1" && link.ChildNodeID == "n2" && link.Hint == "fact-to-explicit-condition" {
			saw12 = true
		}
		if link.ParentNodeID == "n1" && link.ChildNodeID == "n3" && link.Hint == "fact-to-implicit-condition" {
			saw13 = true
		}
		if link.ParentNodeID == "n3" && link.ChildNodeID == "n4" && link.Hint == "implicit-condition-to-conclusion" {
			saw34 = true
		}
		if link.ParentNodeID == "n2" && link.ChildNodeID == "n3" {
			t.Fatalf("hierarchy = %#v, do not want explicit condition to implicit condition link", out.Hierarchy)
		}
	}
	if !saw12 || !saw13 || !saw34 {
		t.Fatalf("hierarchy = %#v, want fact->explicit, fact->implicit, implicit->conclusion", out.Hierarchy)
	}
}

func TestSQLiteStore_OrganizationBuildsPredictionSkeletonFromExplicitConditionsAndConclusions(t *testing.T) {
	root := t.TempDir()
	store, err := NewSQLiteStore(filepath.Join(root, "data", "content.db"))
	if err != nil {
		t.Fatalf("NewSQLiteStore() error = %v", err)
	}
	defer store.Close()

	record := compile.Record{
		UnitID:         "weibo:Q3c",
		Source:         "weibo",
		ExternalID:     "Q3c",
		RootExternalID: "Q3c",
		Model:          "qwen3.6-plus",
		Output: compile.Output{
			Summary: "summary",
			Graph: compile.ReasoningGraph{
				Nodes: []compile.GraphNode{
					{ID: "n1", Kind: compile.NodeFact, Text: "事实A", ValidFrom: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), ValidTo: time.Date(2026, 12, 31, 0, 0, 0, 0, time.UTC)},
					{ID: "n2", Kind: compile.NodeExplicitCondition, Text: "如果发生条件B", ValidFrom: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), ValidTo: time.Date(2026, 12, 31, 0, 0, 0, 0, time.UTC)},
					{ID: "n3", Kind: compile.NodeConclusion, Text: "结论C", ValidFrom: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), ValidTo: time.Date(2026, 12, 31, 0, 0, 0, 0, time.UTC)},
					{ID: "n4", Kind: compile.NodePrediction, Text: "预测D", ValidFrom: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), ValidTo: time.Date(2026, 12, 31, 0, 0, 0, 0, time.UTC)},
				},
				Edges: []compile.GraphEdge{{From: "n3", To: "n4", Kind: compile.EdgeDerives}},
			},
			Details: compile.HiddenDetails{Caveats: []string{"detail"}},
			Verification: compile.Verification{
				VerifiedAt:              time.Date(2026, 4, 14, 0, 0, 0, 0, time.UTC),
				Model:                   "qwen3.6-plus",
				FactChecks:              []compile.FactCheck{{NodeID: "n1", Status: compile.FactStatusClearlyTrue, Reason: "supported"}},
				ExplicitConditionChecks: []compile.ExplicitConditionCheck{{NodeID: "n2", Status: compile.ExplicitConditionStatusHigh, Reason: "likely"}},
				PredictionChecks:        []compile.PredictionCheck{{NodeID: "n4", Status: compile.PredictionStatusUnresolved, Reason: "pending", AsOf: time.Date(2026, 4, 14, 0, 0, 0, 0, time.UTC)}},
			},
			Confidence: "medium",
		},
		CompiledAt: time.Date(2026, 4, 14, 0, 0, 0, 0, time.UTC),
	}
	if err := store.UpsertCompiledOutput(context.Background(), record); err != nil {
		t.Fatalf("UpsertCompiledOutput() error = %v", err)
	}
	if _, err := store.AcceptMemoryNodes(context.Background(), memory.AcceptRequest{
		UserID:           "u3c",
		SourcePlatform:   "weibo",
		SourceExternalID: "Q3c",
		NodeIDs:          []string{"n1", "n2", "n3", "n4"},
	}); err != nil {
		t.Fatalf("AcceptMemoryNodes() error = %v", err)
	}

	out, err := store.RunNextMemoryOrganizationJob(context.Background(), "u3c", time.Date(2026, 4, 14, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("RunNextMemoryOrganizationJob() error = %v", err)
	}
	var saw12, saw24, saw34 bool
	for _, link := range out.Hierarchy {
		if link.ParentNodeID == "n1" && link.ChildNodeID == "n2" && link.Hint == "fact-to-explicit-condition" {
			saw12 = true
		}
		if link.ParentNodeID == "n2" && link.ChildNodeID == "n4" && link.Hint == "explicit-condition-to-prediction" {
			saw24 = true
		}
		if link.ParentNodeID == "n3" && link.ChildNodeID == "n4" {
			saw34 = true
		}
		if link.ParentNodeID == "n2" && link.ChildNodeID == "n3" {
			t.Fatalf("hierarchy = %#v, do not want explicit condition to conclusion link", out.Hierarchy)
		}
	}
	if !saw12 || !saw24 || !saw34 {
		t.Fatalf("hierarchy = %#v, want fact->explicit, explicit->prediction, conclusion->prediction", out.Hierarchy)
	}
}

func TestSQLiteStore_OrganizationIncludesImplicitVerificationsAndExplicitConditionHints(t *testing.T) {
	root := t.TempDir()
	store, err := NewSQLiteStore(filepath.Join(root, "data", "content.db"))
	if err != nil {
		t.Fatalf("NewSQLiteStore() error = %v", err)
	}
	defer store.Close()

	record := compile.Record{
		UnitID:         "weibo:Q3d",
		Source:         "weibo",
		ExternalID:     "Q3d",
		RootExternalID: "Q3d",
		Model:          "qwen3.6-plus",
		Output: compile.Output{
			Summary: "summary",
			Graph: compile.ReasoningGraph{
				Nodes: []compile.GraphNode{
					{ID: "n1", Kind: compile.NodeImplicitCondition, Text: "隐含条件A", ValidFrom: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), ValidTo: time.Date(2026, 12, 31, 0, 0, 0, 0, time.UTC)},
					{ID: "n2", Kind: compile.NodeExplicitCondition, Text: "如果发生条件B", ValidFrom: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), ValidTo: time.Date(2026, 12, 31, 0, 0, 0, 0, time.UTC)},
				},
				Edges: []compile.GraphEdge{{From: "n1", To: "n2", Kind: compile.EdgePositive}},
			},
			Details: compile.HiddenDetails{Caveats: []string{"detail"}},
			Verification: compile.Verification{
				VerifiedAt:              time.Date(2026, 4, 14, 0, 0, 0, 0, time.UTC),
				Model:                   "qwen3.6-plus",
				ImplicitConditionChecks: []compile.ImplicitConditionCheck{{NodeID: "n1", Status: compile.FactStatusUnverifiable, Reason: "implicit premise unclear"}},
				ExplicitConditionChecks: []compile.ExplicitConditionCheck{{NodeID: "n2", Status: compile.ExplicitConditionStatusUnknown, Reason: "cannot forecast"}},
			},
			Confidence: "medium",
		},
		CompiledAt: time.Date(2026, 4, 14, 0, 0, 0, 0, time.UTC),
	}
	if err := store.UpsertCompiledOutput(context.Background(), record); err != nil {
		t.Fatalf("UpsertCompiledOutput() error = %v", err)
	}
	if _, err := store.AcceptMemoryNodes(context.Background(), memory.AcceptRequest{
		UserID:           "u3d",
		SourcePlatform:   "weibo",
		SourceExternalID: "Q3d",
		NodeIDs:          []string{"n1", "n2"},
	}); err != nil {
		t.Fatalf("AcceptMemoryNodes() error = %v", err)
	}

	out, err := store.RunNextMemoryOrganizationJob(context.Background(), "u3d", time.Date(2026, 4, 14, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("RunNextMemoryOrganizationJob() error = %v", err)
	}
	if len(out.FactVerifications) != 1 || out.FactVerifications[0].NodeID != "n1" || out.FactVerifications[0].Status != string(compile.FactStatusUnverifiable) {
		t.Fatalf("FactVerifications = %#v, want implicit condition verification for n1", out.FactVerifications)
	}
	foundUnknownQuestion := false
	foundExplicitHint := false
	for _, question := range out.OpenQuestions {
		if strings.Contains(question, "n1") || strings.Contains(question, "n2") {
			foundUnknownQuestion = true
		}
	}
	for _, hint := range out.NodeHints {
		if hint.NodeID == "n2" && hint.ConditionProbability == string(compile.ExplicitConditionStatusUnknown) {
			foundExplicitHint = true
		}
	}
	if !foundUnknownQuestion {
		t.Fatalf("OpenQuestions = %#v, want implicit/explicit verifier uncertainty surfaced", out.OpenQuestions)
	}
	if !foundExplicitHint {
		t.Fatalf("NodeHints = %#v, want explicit condition probability hint", out.NodeHints)
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
					{ID: "n3", Kind: compile.NodeConclusion, Text: "结论C", ValidFrom: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), ValidTo: time.Date(2026, 12, 31, 0, 0, 0, 0, time.UTC)},
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

func TestSQLiteStore_OrganizationBackfillsLegacyNodeTaxonomyFromCurrentCompile(t *testing.T) {
	root := t.TempDir()
	store, err := NewSQLiteStore(filepath.Join(root, "data", "content.db"))
	if err != nil {
		t.Fatalf("NewSQLiteStore() error = %v", err)
	}
	defer store.Close()

	record := compile.Record{
		UnitID:         "weibo:Q4c",
		Source:         "weibo",
		ExternalID:     "Q4c",
		RootExternalID: "Q4c",
		Model:          "qwen3.6-plus",
		Output: compile.Output{
			Summary: "summary",
			Graph: compile.ReasoningGraph{
				Nodes: []compile.GraphNode{
					{ID: "n1", Kind: compile.NodeFact, Text: "事实A", ValidFrom: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), ValidTo: time.Date(2026, 12, 31, 0, 0, 0, 0, time.UTC)},
					{ID: "n2", Kind: compile.NodeExplicitCondition, Text: "如果发生条件B", ValidFrom: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), ValidTo: time.Date(2026, 12, 31, 0, 0, 0, 0, time.UTC)},
					{ID: "n3", Kind: compile.NodeConclusion, Text: "结论C", ValidFrom: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), ValidTo: time.Date(2026, 12, 31, 0, 0, 0, 0, time.UTC)},
				},
				Edges: []compile.GraphEdge{
					{From: "n2", To: "n3", Kind: compile.EdgePresets},
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

	// legacy accepted node stored before taxonomy split
	if _, err := store.db.Exec(`INSERT INTO user_memory_nodes(user_id, source_platform, source_external_id, root_external_id, node_id, node_kind, node_text, source_model, source_compiled_at, valid_from, valid_to, accepted_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		"u4c", "weibo", "Q4c", "Q4c", "n2", string(compile.NodeFact), "条件B", "old-model", time.Date(2026, 4, 13, 0, 0, 0, 0, time.UTC).Format(time.RFC3339Nano), "", "", time.Date(2026, 4, 13, 0, 0, 0, 0, time.UTC).Format(time.RFC3339Nano),
	); err != nil {
		t.Fatalf("seed legacy user_memory_nodes error = %v", err)
	}
	if _, err := store.db.Exec(`INSERT INTO memory_acceptance_events(user_id, trigger_type, source_platform, source_external_id, root_external_id, source_model, source_compiled_at, payload_json, accepted_count, accepted_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		"u4c", "accept_single", "weibo", "Q4c", "Q4c", "old-model", time.Date(2026, 4, 13, 0, 0, 0, 0, time.UTC).Format(time.RFC3339Nano), `[{"node_id":"n2","node_kind":"事实","node_text":"条件B"}]`, 1, time.Date(2026, 4, 13, 0, 0, 0, 0, time.UTC).Format(time.RFC3339Nano),
	); err != nil {
		t.Fatalf("seed event error = %v", err)
	}
	if _, err := store.db.Exec(`INSERT INTO memory_organization_jobs(trigger_event_id, user_id, source_platform, source_external_id, status, created_at) VALUES (1, ?, ?, ?, 'queued', ?)`,
		"u4c", "weibo", "Q4c", time.Date(2026, 4, 14, 0, 0, 0, 0, time.UTC).Format(time.RFC3339Nano),
	); err != nil {
		t.Fatalf("seed job error = %v", err)
	}

	out, err := store.RunNextMemoryOrganizationJob(context.Background(), "u4c", time.Date(2026, 4, 14, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("RunNextMemoryOrganizationJob() error = %v", err)
	}
	foundExplicit := false
	for _, node := range out.ActiveNodes {
		if node.NodeID == "n2" {
			if node.NodeKind != string(compile.NodeExplicitCondition) {
				t.Fatalf("node kind = %q, want explicit condition", node.NodeKind)
			}
			if node.NodeText != "如果发生条件B" {
				t.Fatalf("node text = %q, want compile-derived text", node.NodeText)
			}
			foundExplicit = true
		}
	}
	if !foundExplicit {
		t.Fatalf("active nodes missing n2: %#v", out.ActiveNodes)
	}
}

func TestSQLiteStore_OrganizationDoesNotBackfillMismatchedNodeTextByIDOnly(t *testing.T) {
	root := t.TempDir()
	store, err := NewSQLiteStore(filepath.Join(root, "data", "content.db"))
	if err != nil {
		t.Fatalf("NewSQLiteStore() error = %v", err)
	}
	defer store.Close()

	record := compile.Record{
		UnitID:         "weibo:Q4d",
		Source:         "weibo",
		ExternalID:     "Q4d",
		RootExternalID: "Q4d",
		Model:          "qwen3.6-plus",
		Output: compile.Output{
			Summary: "summary",
			Graph: compile.ReasoningGraph{
				Nodes: []compile.GraphNode{
					{ID: "n2", Kind: compile.NodeExplicitCondition, Text: "如果发生条件B", ValidFrom: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), ValidTo: time.Date(2026, 12, 31, 0, 0, 0, 0, time.UTC)},
					{ID: "n3", Kind: compile.NodeConclusion, Text: "结论C", ValidFrom: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), ValidTo: time.Date(2026, 12, 31, 0, 0, 0, 0, time.UTC)},
				},
				Edges: []compile.GraphEdge{{From: "n2", To: "n3", Kind: compile.EdgePresets}},
			},
			Details:    compile.HiddenDetails{Caveats: []string{"detail"}},
			Confidence: "medium",
		},
		CompiledAt: time.Date(2026, 4, 14, 0, 0, 0, 0, time.UTC),
	}
	if err := store.UpsertCompiledOutput(context.Background(), record); err != nil {
		t.Fatalf("UpsertCompiledOutput() error = %v", err)
	}

	// same node id, different semantic text
	if _, err := store.db.Exec(`INSERT INTO user_memory_nodes(user_id, source_platform, source_external_id, root_external_id, node_id, node_kind, node_text, source_model, source_compiled_at, valid_from, valid_to, accepted_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		"u4d", "weibo", "Q4d", "Q4d", "n2", string(compile.NodeFact), "完全不同的旧节点", "old-model", time.Date(2026, 4, 13, 0, 0, 0, 0, time.UTC).Format(time.RFC3339Nano), "", "", time.Date(2026, 4, 13, 0, 0, 0, 0, time.UTC).Format(time.RFC3339Nano),
	); err != nil {
		t.Fatalf("seed legacy user_memory_nodes error = %v", err)
	}
	if _, err := store.db.Exec(`INSERT INTO memory_acceptance_events(user_id, trigger_type, source_platform, source_external_id, root_external_id, source_model, source_compiled_at, payload_json, accepted_count, accepted_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		"u4d", "accept_single", "weibo", "Q4d", "Q4d", "old-model", time.Date(2026, 4, 13, 0, 0, 0, 0, time.UTC).Format(time.RFC3339Nano), `[{"node_id":"n2","node_kind":"事实","node_text":"完全不同的旧节点"}]`, 1, time.Date(2026, 4, 13, 0, 0, 0, 0, time.UTC).Format(time.RFC3339Nano),
	); err != nil {
		t.Fatalf("seed event error = %v", err)
	}
	if _, err := store.db.Exec(`INSERT INTO memory_organization_jobs(trigger_event_id, user_id, source_platform, source_external_id, status, created_at) VALUES (1, ?, ?, ?, 'queued', ?)`,
		"u4d", "weibo", "Q4d", time.Date(2026, 4, 14, 0, 0, 0, 0, time.UTC).Format(time.RFC3339Nano),
	); err != nil {
		t.Fatalf("seed job error = %v", err)
	}

	out, err := store.RunNextMemoryOrganizationJob(context.Background(), "u4d", time.Date(2026, 4, 14, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("RunNextMemoryOrganizationJob() error = %v", err)
	}
	if len(out.ActiveNodes) != 0 {
		t.Fatalf("len(ActiveNodes) = %d, want 0 because mismatched legacy text should not backfill", len(out.ActiveNodes))
	}
	if len(out.InactiveNodes) != 1 {
		t.Fatalf("len(InactiveNodes) = %d, want 1", len(out.InactiveNodes))
	}
	if out.InactiveNodes[0].NodeText != "完全不同的旧节点" {
		t.Fatalf("inactive node text = %q, want original legacy text", out.InactiveNodes[0].NodeText)
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

func TestSQLiteStore_RunGlobalMemoryOrganizationBuildsNeutralClustersAcrossSources(t *testing.T) {
	root := t.TempDir()
	store, err := NewSQLiteStore(filepath.Join(root, "data", "content.db"))
	if err != nil {
		t.Fatalf("NewSQLiteStore() error = %v", err)
	}
	defer store.Close()

	recordA := compile.Record{
		UnitID:         "weibo:G1",
		Source:         "weibo",
		ExternalID:     "G1",
		RootExternalID: "G1",
		Model:          "qwen3.6-plus",
		Output: compile.Output{
			Summary: "summary",
			Graph: compile.ReasoningGraph{
				Nodes: []compile.GraphNode{
					{ID: "n1", Kind: compile.NodeFact, Text: "油价会上升", OccurredAt: time.Date(2026, 4, 10, 0, 0, 0, 0, time.UTC)},
					{ID: "n2", Kind: compile.NodeExplicitCondition, Text: "若地缘冲突升级"},
					{ID: "n3", Kind: compile.NodePrediction, Text: "未来几年风险资产承压", PredictionStartAt: time.Date(2026, 4, 10, 0, 0, 0, 0, time.UTC)},
				},
				Edges: []compile.GraphEdge{
					{From: "n1", To: "n2", Kind: compile.EdgePositive},
					{From: "n2", To: "n3", Kind: compile.EdgePresets},
				},
			},
			Details:    compile.HiddenDetails{Caveats: []string{"detail"}},
			Confidence: "medium",
		},
		CompiledAt: time.Date(2026, 4, 14, 8, 0, 0, 0, time.UTC),
	}
	recordB := compile.Record{
		UnitID:         "twitter:G2",
		Source:         "twitter",
		ExternalID:     "G2",
		RootExternalID: "G2",
		Model:          "qwen3.6-plus",
		Output: compile.Output{
			Summary: "summary",
			Graph: compile.ReasoningGraph{
				Nodes: []compile.GraphNode{
					{ID: "n1", Kind: compile.NodeFact, Text: "油价会下降", OccurredAt: time.Date(2026, 4, 11, 0, 0, 0, 0, time.UTC)},
					{ID: "n2", Kind: compile.NodeImplicitCondition, Text: "供给收缩会改变油价走势", OccurredAt: time.Date(2026, 4, 11, 0, 0, 0, 0, time.UTC)},
				},
				Edges: []compile.GraphEdge{
					{From: "n2", To: "n1", Kind: compile.EdgeDerives},
				},
			},
			Details:    compile.HiddenDetails{Caveats: []string{"detail"}},
			Confidence: "medium",
		},
		CompiledAt: time.Date(2026, 4, 14, 8, 10, 0, 0, time.UTC),
	}
	if err := store.UpsertCompiledOutput(context.Background(), recordA); err != nil {
		t.Fatalf("UpsertCompiledOutput(recordA) error = %v", err)
	}
	if err := store.UpsertCompiledOutput(context.Background(), recordB); err != nil {
		t.Fatalf("UpsertCompiledOutput(recordB) error = %v", err)
	}
	if _, err := store.AcceptMemoryNodes(context.Background(), memory.AcceptRequest{
		UserID:           "u-global",
		SourcePlatform:   "weibo",
		SourceExternalID: "G1",
		NodeIDs:          []string{"n1", "n2", "n3"},
	}); err != nil {
		t.Fatalf("AcceptMemoryNodes(weibo) error = %v", err)
	}
	if _, err := store.AcceptMemoryNodes(context.Background(), memory.AcceptRequest{
		UserID:           "u-global",
		SourcePlatform:   "twitter",
		SourceExternalID: "G2",
		NodeIDs:          []string{"n1", "n2"},
	}); err != nil {
		t.Fatalf("AcceptMemoryNodes(twitter) error = %v", err)
	}

	out, err := store.RunGlobalMemoryOrganization(context.Background(), "u-global", time.Date(2026, 4, 14, 9, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("RunGlobalMemoryOrganization() error = %v", err)
	}
	if len(out.Clusters) == 0 {
		t.Fatalf("len(Clusters) = 0, want clusters")
	}
	foundContradictionCluster := false
	for _, cluster := range out.Clusters {
		if len(cluster.ConflictingNodeIDs) > 0 {
			foundContradictionCluster = true
			if cluster.RepresentativeNodeID == "" {
				t.Fatalf("cluster missing representative node: %#v", cluster)
			}
			if cluster.CanonicalProposition == "" || !strings.HasPrefix(cluster.CanonicalProposition, "关于「") {
				t.Fatalf("cluster proposition should be neutral rather than raw representative text: %#v", cluster)
			}
		}
	}
	if !foundContradictionCluster {
		t.Fatalf("Clusters = %#v, want contradiction cluster", out.Clusters)
	}
	got, err := store.GetLatestGlobalMemoryOrganizationOutput(context.Background(), "u-global")
	if err != nil {
		t.Fatalf("GetLatestGlobalMemoryOrganizationOutput() error = %v", err)
	}
	if got.OutputID == 0 || len(got.Clusters) == 0 {
		t.Fatalf("latest global output = %#v", got)
	}
}
