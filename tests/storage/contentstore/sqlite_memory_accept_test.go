package contentstore

import (
	"context"
	"github.com/kumaloha/VariX/varix/memory"
	"github.com/kumaloha/VariX/varix/model"
	"path/filepath"
	"testing"
	"time"
)

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
	if got.Nodes[0].NodeID != "n1" || got.Nodes[0].NodeKind != string(model.NodeFact) || got.Nodes[0].NodeText != "事实A" {
		t.Fatalf("node = %#v", got.Nodes[0])
	}
	if got.Event.EventID == 0 || got.Job.JobID == 0 {
		t.Fatalf("event/job = %#v / %#v", got.Event, got.Job)
	}
	if got.Job.TriggerEventID != got.Event.EventID {
		t.Fatalf("TriggerEventID = %d, want %d", got.Job.TriggerEventID, got.Event.EventID)
	}
}

func TestSQLiteStore_CleanupStaleMemoryJobsHandlesMixedPrecisionTimestamps(t *testing.T) {
	root := t.TempDir()
	store, err := NewSQLiteStore(filepath.Join(root, "data", "content.db"))
	if err != nil {
		t.Fatalf("NewSQLiteStore() error = %v", err)
	}
	defer store.Close()

	if _, err := store.db.Exec(`INSERT INTO memory_organization_jobs(trigger_event_id, user_id, source_platform, source_external_id, status, created_at)
		VALUES (1, 'u-mixed-precision', 'twitter', 'stale-no-fraction', 'queued', '2026-04-21T12:00:00Z')`); err != nil {
		t.Fatalf("insert stale-no-fraction error = %v", err)
	}
	if _, err := store.db.Exec(`INSERT INTO memory_organization_jobs(trigger_event_id, user_id, source_platform, source_external_id, status, created_at)
		VALUES (2, 'u-mixed-precision', 'twitter', 'fresh-fraction', 'queued', '2026-04-21T12:00:00.9Z')`); err != nil {
		t.Fatalf("insert fresh-fraction error = %v", err)
	}

	deleted, err := store.CleanupStaleMemoryJobs(context.Background(), "u-mixed-precision", "", "", time.Date(2026, 4, 21, 12, 0, 0, 500_000_000, time.UTC))
	if err != nil {
		t.Fatalf("CleanupStaleMemoryJobs() error = %v", err)
	}
	if deleted != 1 {
		t.Fatalf("deleted = %d, want 1", deleted)
	}

	jobs, err := store.ListMemoryJobs(context.Background(), "u-mixed-precision")
	if err != nil {
		t.Fatalf("ListMemoryJobs() error = %v", err)
	}
	if len(jobs) != 1 || jobs[0].SourceExternalID != "fresh-fraction" {
		t.Fatalf("jobs = %#v, want only fresh-fraction to remain", jobs)
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
	record := model.Record{
		UnitID:         "weibo:Q-time",
		Source:         "weibo",
		ExternalID:     "Q-time",
		RootExternalID: "Q-time",
		Model:          "qwen3.6-plus",
		Output: model.Output{
			Summary: "summary",
			Graph: model.ReasoningGraph{
				Nodes: []model.GraphNode{
					{ID: "n1", Kind: model.NodeFact, Text: "事实A", OccurredAt: occurredAt},
					{ID: "n2", Kind: model.NodePrediction, Text: "预测B", PredictionStartAt: predictionStart, PredictionDueAt: predictionDue},
					{ID: "n3", Kind: model.NodeConclusion, Text: "结论C"},
				},
				Edges: []model.GraphEdge{{From: "n1", To: "n3", Kind: model.EdgeDerives}, {From: "n3", To: "n2", Kind: model.EdgeDerives}},
			},
			Details: model.HiddenDetails{Caveats: []string{"detail"}},
			Verification: model.Verification{
				PredictionChecks: []model.PredictionCheck{{
					NodeID: "n2", Status: model.PredictionStatusUnresolved, Reason: "window open", AsOf: predictionStart,
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
	output, err := model.ParseOutput(raw)
	if err != nil {
		t.Fatalf("ParseOutput() error = %v", err)
	}
	record := model.Record{
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

	output, err := model.ParseOutput(`{
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

	record := model.Record{
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
