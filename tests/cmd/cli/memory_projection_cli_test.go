package main

import (
	"bytes"
	"context"
	"encoding/json"
	c "github.com/kumaloha/VariX/varix/compile"
	"github.com/kumaloha/VariX/varix/ingest"
	varixllm "github.com/kumaloha/VariX/varix/llm"
	"github.com/kumaloha/VariX/varix/memory"
	"github.com/kumaloha/VariX/varix/model"
	"github.com/kumaloha/VariX/varix/storage/contentstore"
	"strings"
	"testing"
	"time"
)

func TestRunMemoryEventGraphsPrintsProjectedEventGraphs(t *testing.T) {
	prevNewIngestRuntime := newIngestRuntime
	prevOpenSQLiteStore := openSQLiteStore
	t.Cleanup(func() {
		newIngestRuntime = prevNewIngestRuntime
		openSQLiteStore = prevOpenSQLiteStore
	})

	tmp := t.TempDir()
	newIngestRuntime = func(projectRoot string) (*ingest.Runtime, error) {
		app := &ingest.Runtime{}
		app.Settings.ContentDBPath = tmp + "/content.db"
		return app, nil
	}
	openSQLiteStore = func(path string) (*contentstore.SQLiteStore, error) {
		store, err := contentstore.NewSQLiteStore(path)
		if err != nil {
			return nil, err
		}
		record := c.Record{
			UnitID: "twitter:EG1", Source: "twitter", ExternalID: "EG1", RootExternalID: "EG1", Model: varixllm.Qwen36PlusModel,
			Output: c.Output{Summary: "一句话", Drivers: []string{"美联储加息0.25%"}, Targets: []string{"未来一周美股承压"}, Graph: c.ReasoningGraph{Nodes: []c.GraphNode{
				{ID: "n1", Kind: c.NodeFact, Text: "美联储加息0.25%", OccurredAt: time.Date(2026, 4, 10, 0, 0, 0, 0, time.UTC)},
				{ID: "n2", Kind: c.NodePrediction, Text: "未来一周美股承压", PredictionStartAt: time.Date(2026, 4, 10, 0, 0, 0, 0, time.UTC), PredictionDueAt: time.Date(2026, 4, 17, 0, 0, 0, 0, time.UTC)},
			}, Edges: []c.GraphEdge{{From: "n1", To: "n2", Kind: c.EdgePositive}}}, Details: c.HiddenDetails{Caveats: []string{"说明"}}},
			CompiledAt: time.Now().UTC(),
		}
		if err := store.UpsertCompiledOutput(context.Background(), record); err != nil {
			return nil, err
		}
		if _, err := store.AcceptMemoryNodes(context.Background(), memory.AcceptRequest{UserID: "u-event-cli", SourcePlatform: "twitter", SourceExternalID: "EG1", NodeIDs: []string{"n1", "n2"}}); err != nil {
			return nil, err
		}
		return store, nil
	}

	var stdout, stderr bytes.Buffer
	code := run([]string{"memory", "event-graphs", "--run", "--user", "u-event-cli"}, "/tmp/project", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("event-graphs code = %d, stderr = %s", code, stderr.String())
	}
	var out []contentstore.EventGraphRecord
	if err := json.Unmarshal(stdout.Bytes(), &out); err != nil {
		t.Fatalf("json.Unmarshal(event-graphs) error = %v", err)
	}
	if len(out) == 0 {
		t.Fatalf("event-graphs output = %#v, want non-empty", out)
	}
}

func TestRunMemoryParadigmsPrintsProjectedParadigms(t *testing.T) {
	prevNewIngestRuntime := newIngestRuntime
	prevOpenSQLiteStore := openSQLiteStore
	t.Cleanup(func() {
		newIngestRuntime = prevNewIngestRuntime
		openSQLiteStore = prevOpenSQLiteStore
	})

	tmp := t.TempDir()
	newIngestRuntime = func(projectRoot string) (*ingest.Runtime, error) {
		app := &ingest.Runtime{}
		app.Settings.ContentDBPath = tmp + "/content.db"
		return app, nil
	}
	openSQLiteStore = func(path string) (*contentstore.SQLiteStore, error) {
		store, err := contentstore.NewSQLiteStore(path)
		if err != nil {
			return nil, err
		}
		record := c.Record{
			UnitID: "twitter:PG1", Source: "twitter", ExternalID: "PG1", RootExternalID: "PG1", Model: varixllm.Qwen36PlusModel,
			Output: c.Output{Summary: "一句话", Drivers: []string{"美联储加息0.25%"}, Targets: []string{"未来一周美股承压"}, Graph: c.ReasoningGraph{Nodes: []c.GraphNode{
				{ID: "n1", Kind: c.NodeFact, Text: "美联储加息0.25%", OccurredAt: time.Date(2026, 4, 10, 0, 0, 0, 0, time.UTC)},
				{ID: "n2", Kind: c.NodePrediction, Text: "未来一周美股承压", PredictionStartAt: time.Date(2026, 4, 10, 0, 0, 0, 0, time.UTC), PredictionDueAt: time.Date(2026, 4, 17, 0, 0, 0, 0, time.UTC)},
			}, Edges: []c.GraphEdge{{From: "n1", To: "n2", Kind: c.EdgePositive}}}, Details: c.HiddenDetails{Caveats: []string{"说明"}}, Verification: c.Verification{NodeVerifications: []c.NodeVerification{{NodeID: "n2", Status: c.NodeVerificationProved}}}},
			CompiledAt: time.Now().UTC(),
		}
		if err := store.UpsertCompiledOutput(context.Background(), record); err != nil {
			return nil, err
		}
		if _, err := store.AcceptMemoryNodes(context.Background(), memory.AcceptRequest{UserID: "u-paradigm-cli", SourcePlatform: "twitter", SourceExternalID: "PG1", NodeIDs: []string{"n1", "n2"}}); err != nil {
			return nil, err
		}
		return store, nil
	}

	var stdout, stderr bytes.Buffer
	code := run([]string{"memory", "paradigms", "--run", "--user", "u-paradigm-cli"}, "/tmp/project", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("paradigms code = %d, stderr = %s", code, stderr.String())
	}
	var out []contentstore.ParadigmRecord
	if err := json.Unmarshal(stdout.Bytes(), &out); err != nil {
		t.Fatalf("json.Unmarshal(paradigms) error = %v", err)
	}
	if len(out) == 0 {
		t.Fatalf("paradigms output = %#v, want non-empty", out)
	}
}

func TestRunMemoryContentGraphsPrintsStoredContentMemoryGraphs(t *testing.T) {
	prevNewIngestRuntime := newIngestRuntime
	prevOpenSQLiteStore := openSQLiteStore
	t.Cleanup(func() {
		newIngestRuntime = prevNewIngestRuntime
		openSQLiteStore = prevOpenSQLiteStore
	})

	tmp := t.TempDir()
	newIngestRuntime = func(projectRoot string) (*ingest.Runtime, error) {
		app := &ingest.Runtime{}
		app.Settings.ContentDBPath = tmp + "/content.db"
		return app, nil
	}
	openSQLiteStore = func(path string) (*contentstore.SQLiteStore, error) {
		store, err := contentstore.NewSQLiteStore(path)
		if err != nil {
			return nil, err
		}
		record := c.Record{
			UnitID: "twitter:CG1", Source: "twitter", ExternalID: "CG1", RootExternalID: "CG1", Model: varixllm.Qwen36PlusModel,
			Output: c.Output{Summary: "一句话", Drivers: []string{"美联储加息0.25%"}, Targets: []string{"未来一周美股承压"}, Graph: c.ReasoningGraph{Nodes: []c.GraphNode{
				{ID: "n1", Kind: c.NodeFact, Text: "美联储加息0.25%", OccurredAt: time.Date(2026, 4, 10, 0, 0, 0, 0, time.UTC)},
				{ID: "n2", Kind: c.NodePrediction, Text: "未来一周美股承压", PredictionStartAt: time.Date(2026, 4, 10, 0, 0, 0, 0, time.UTC), PredictionDueAt: time.Date(2026, 4, 17, 0, 0, 0, 0, time.UTC)},
			}, Edges: []c.GraphEdge{{From: "n1", To: "n2", Kind: c.EdgePositive}}}, Details: c.HiddenDetails{Caveats: []string{"说明"}}},
			CompiledAt: time.Now().UTC(),
		}
		if err := store.UpsertCompiledOutput(context.Background(), record); err != nil {
			return nil, err
		}
		if _, err := store.AcceptMemoryNodes(context.Background(), memory.AcceptRequest{UserID: "u-content-graph-cli", SourcePlatform: "twitter", SourceExternalID: "CG1", NodeIDs: []string{"n1", "n2"}}); err != nil {
			return nil, err
		}
		return store, nil
	}

	var stdout, stderr bytes.Buffer
	code := run([]string{"memory", "content-graphs", "--user", "u-content-graph-cli"}, "/tmp/project", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("content-graphs code = %d, stderr = %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "unit-sweep") && !strings.Contains(stdout.String(), "twitter") {
		var out []map[string]any
		if err := json.Unmarshal(stdout.Bytes(), &out); err != nil {
			t.Fatalf("json.Unmarshal(content-graphs) error = %v", err)
		}
		if len(out) == 0 {
			t.Fatalf("content-graphs output = %#v, want non-empty", out)
		}
	}
}

func TestRunMemoryEventGraphsRunRecomputesProjection(t *testing.T) {
	prevNewIngestRuntime := newIngestRuntime
	prevOpenSQLiteStore := openSQLiteStore
	t.Cleanup(func() {
		newIngestRuntime = prevNewIngestRuntime
		openSQLiteStore = prevOpenSQLiteStore
	})

	tmp := t.TempDir()
	newIngestRuntime = func(projectRoot string) (*ingest.Runtime, error) {
		app := &ingest.Runtime{}
		app.Settings.ContentDBPath = tmp + "/content.db"
		return app, nil
	}
	openSQLiteStore = func(path string) (*contentstore.SQLiteStore, error) {
		store, err := contentstore.NewSQLiteStore(path)
		if err != nil {
			return nil, err
		}
		sg := testDriverTargetSubgraph("manual-eg", time.Now().UTC())
		if err := store.PersistMemoryContentGraph(context.Background(), "u-event-run", sg, time.Now().UTC()); err != nil {
			return nil, err
		}
		return store, nil
	}

	var stdout, stderr bytes.Buffer
	code := run([]string{"memory", "event-graphs", "--run", "--user", "u-event-run"}, "/tmp/project", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("event-graphs --run code = %d, stderr = %s", code, stderr.String())
	}
	var out []contentstore.EventGraphRecord
	if err := json.Unmarshal(stdout.Bytes(), &out); err != nil {
		t.Fatalf("json.Unmarshal(event-graphs --run) error = %v", err)
	}
	if len(out) == 0 {
		t.Fatalf("event-graphs --run output = %#v, want non-empty", out)
	}
}

func TestRunMemoryParadigmsRunRecomputesProjection(t *testing.T) {
	prevNewIngestRuntime := newIngestRuntime
	prevOpenSQLiteStore := openSQLiteStore
	t.Cleanup(func() {
		newIngestRuntime = prevNewIngestRuntime
		openSQLiteStore = prevOpenSQLiteStore
	})

	tmp := t.TempDir()
	newIngestRuntime = func(projectRoot string) (*ingest.Runtime, error) {
		app := &ingest.Runtime{}
		app.Settings.ContentDBPath = tmp + "/content.db"
		return app, nil
	}
	openSQLiteStore = func(path string) (*contentstore.SQLiteStore, error) {
		store, err := contentstore.NewSQLiteStore(path)
		if err != nil {
			return nil, err
		}
		sg := testDriverTargetSubgraph("manual-pg", time.Now().UTC())
		if err := store.PersistMemoryContentGraph(context.Background(), "u-paradigm-run", sg, time.Now().UTC()); err != nil {
			return nil, err
		}
		return store, nil
	}

	var stdout, stderr bytes.Buffer
	code := run([]string{"memory", "paradigms", "--run", "--user", "u-paradigm-run"}, "/tmp/project", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("paradigms --run code = %d, stderr = %s", code, stderr.String())
	}
	var out []contentstore.ParadigmRecord
	if err := json.Unmarshal(stdout.Bytes(), &out); err != nil {
		t.Fatalf("json.Unmarshal(paradigms --run) error = %v", err)
	}
	if len(out) == 0 {
		t.Fatalf("paradigms --run output = %#v, want non-empty", out)
	}
}

func TestRunMemoryContentGraphsRunRebuildsSnapshotFromCompiledOutput(t *testing.T) {
	prevNewIngestRuntime := newIngestRuntime
	prevOpenSQLiteStore := openSQLiteStore
	t.Cleanup(func() {
		newIngestRuntime = prevNewIngestRuntime
		openSQLiteStore = prevOpenSQLiteStore
	})

	tmp := t.TempDir()
	newIngestRuntime = func(projectRoot string) (*ingest.Runtime, error) {
		app := &ingest.Runtime{}
		app.Settings.ContentDBPath = tmp + "/content.db"
		return app, nil
	}
	openSQLiteStore = func(path string) (*contentstore.SQLiteStore, error) {
		store, err := contentstore.NewSQLiteStore(path)
		if err != nil {
			return nil, err
		}
		record := c.Record{UnitID: "twitter:CGRUN1", Source: "twitter", ExternalID: "CGRUN1", RootExternalID: "CGRUN1", Model: varixllm.Qwen36PlusModel, Output: c.Output{Summary: "一句话", Drivers: []string{"美联储加息0.25%"}, Targets: []string{"未来一周美股承压"}, Graph: c.ReasoningGraph{Nodes: []c.GraphNode{{ID: "n1", Kind: c.NodeFact, Text: "美联储加息0.25%", OccurredAt: time.Date(2026, 4, 10, 0, 0, 0, 0, time.UTC)}, {ID: "n2", Kind: c.NodePrediction, Text: "未来一周美股承压", PredictionStartAt: time.Date(2026, 4, 10, 0, 0, 0, 0, time.UTC), PredictionDueAt: time.Date(2026, 4, 17, 0, 0, 0, 0, time.UTC)}}, Edges: []c.GraphEdge{{From: "n1", To: "n2", Kind: c.EdgePositive}}}, Details: c.HiddenDetails{Caveats: []string{"说明"}}}, CompiledAt: time.Now().UTC()}
		if err := store.UpsertCompiledOutput(context.Background(), record); err != nil {
			return nil, err
		}
		return store, nil
	}

	var stdout, stderr bytes.Buffer
	code := run([]string{"memory", "content-graphs", "--run", "--user", "u-content-run-cli", "--platform", "twitter", "--id", "CGRUN1"}, "/tmp/project", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("content-graphs --run code = %d, stderr = %s", code, stderr.String())
	}
	var out []map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &out); err != nil {
		t.Fatalf("json.Unmarshal(content-graphs --run) error = %v", err)
	}
	if len(out) == 0 {
		t.Fatalf("content-graphs --run output = %#v, want non-empty", out)
	}
}

func TestRunMemoryEventGraphsCardPrintsReadableSections(t *testing.T) {
	prevNewIngestRuntime := newIngestRuntime
	prevOpenSQLiteStore := openSQLiteStore
	t.Cleanup(func() {
		newIngestRuntime = prevNewIngestRuntime
		openSQLiteStore = prevOpenSQLiteStore
	})

	tmp := t.TempDir()
	cardNow := time.Date(2026, 4, 21, 0, 0, 0, 0, time.UTC)
	newIngestRuntime = func(projectRoot string) (*ingest.Runtime, error) {
		app := &ingest.Runtime{}
		app.Settings.ContentDBPath = tmp + "/content.db"
		return app, nil
	}
	openSQLiteStore = func(path string) (*contentstore.SQLiteStore, error) {
		store, err := contentstore.NewSQLiteStore(path)
		if err != nil {
			return nil, err
		}
		sg := model.ContentSubgraph{ID: "card-eg", ArticleID: "card-eg", SourcePlatform: "twitter", SourceExternalID: "card-eg", CompileVersion: model.CompileBridgeVersion, CompiledAt: cardNow.Format(time.RFC3339), UpdatedAt: cardNow.Format(time.RFC3339), Nodes: []model.ContentNode{{ID: "n1", SourceArticleID: "card-eg", SourcePlatform: "twitter", SourceExternalID: "card-eg", RawText: "美联储加息0.25%", SubjectText: "美联储", ChangeText: "加息0.25%", TimeStart: cardNow.Format(time.RFC3339), Kind: model.NodeKindObservation, GraphRole: model.GraphRoleDriver, IsPrimary: true, VerificationStatus: model.VerificationPending, TimeBucket: "1w"}, {ID: "n2", SourceArticleID: "card-eg", SourcePlatform: "twitter", SourceExternalID: "card-eg", RawText: "未来一周美股承压", SubjectText: "美股", ChangeText: "承压", TimeStart: cardNow.Format(time.RFC3339), Kind: model.NodeKindPrediction, GraphRole: model.GraphRoleTarget, IsPrimary: true, VerificationStatus: model.VerificationProved, TimeBucket: "1w"}}}
		if err := store.PersistMemoryContentGraph(context.Background(), "u-event-card", sg, cardNow); err != nil {
			return nil, err
		}
		return store, nil
	}
	var stdout, stderr bytes.Buffer
	code := run([]string{"memory", "event-graphs", "--card", "--user", "u-event-card"}, "/tmp/project", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("event-graphs --card code = %d, stderr = %s", code, stderr.String())
	}
	for _, want := range []string{"Event Graph", "美联储", "美股", "Time: 2026-04-21T00:00:00Z", "Representative changes"} {
		if !strings.Contains(stdout.String(), want) {
			t.Fatalf("stdout = %q, want substring %q", stdout.String(), want)
		}
	}
}

func TestRunMemoryParadigmsCardPrintsReadableSections(t *testing.T) {
	prevNewIngestRuntime := newIngestRuntime
	prevOpenSQLiteStore := openSQLiteStore
	t.Cleanup(func() {
		newIngestRuntime = prevNewIngestRuntime
		openSQLiteStore = prevOpenSQLiteStore
	})

	tmp := t.TempDir()
	newIngestRuntime = func(projectRoot string) (*ingest.Runtime, error) {
		app := &ingest.Runtime{}
		app.Settings.ContentDBPath = tmp + "/content.db"
		return app, nil
	}
	openSQLiteStore = func(path string) (*contentstore.SQLiteStore, error) {
		store, err := contentstore.NewSQLiteStore(path)
		if err != nil {
			return nil, err
		}
		sg := testDriverTargetSubgraph("card-pg", time.Now().UTC())
		if err := store.PersistMemoryContentGraph(context.Background(), "u-paradigm-card", sg, time.Now().UTC()); err != nil {
			return nil, err
		}
		return store, nil
	}
	var stdout, stderr bytes.Buffer
	code := run([]string{"memory", "paradigms", "--card", "--user", "u-paradigm-card"}, "/tmp/project", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("paradigms --card code = %d, stderr = %s", code, stderr.String())
	}
	for _, want := range []string{"Paradigm", "美联储", "美股", "Credibility"} {
		if !strings.Contains(stdout.String(), want) {
			t.Fatalf("stdout = %q, want substring %q", stdout.String(), want)
		}
	}
}

func TestRunMemoryContentGraphsCardPrintsReadableSections(t *testing.T) {
	prevNewIngestRuntime := newIngestRuntime
	prevOpenSQLiteStore := openSQLiteStore
	t.Cleanup(func() {
		newIngestRuntime = prevNewIngestRuntime
		openSQLiteStore = prevOpenSQLiteStore
	})

	tmp := t.TempDir()
	newIngestRuntime = func(projectRoot string) (*ingest.Runtime, error) {
		app := &ingest.Runtime{}
		app.Settings.ContentDBPath = tmp + "/content.db"
		return app, nil
	}
	openSQLiteStore = func(path string) (*contentstore.SQLiteStore, error) {
		store, err := contentstore.NewSQLiteStore(path)
		if err != nil {
			return nil, err
		}
		record := c.Record{UnitID: "twitter:CGCARD1", Source: "twitter", ExternalID: "CGCARD1", RootExternalID: "CGCARD1", Model: varixllm.Qwen36PlusModel, Output: c.Output{Summary: "一句话", Drivers: []string{"美联储加息0.25%"}, Targets: []string{"未来一周美股承压"}, Graph: c.ReasoningGraph{Nodes: []c.GraphNode{{ID: "n1", Kind: c.NodeFact, Text: "美联储加息0.25%", OccurredAt: time.Date(2026, 4, 10, 0, 0, 0, 0, time.UTC)}, {ID: "n2", Kind: c.NodePrediction, Text: "未来一周美股承压", PredictionStartAt: time.Date(2026, 4, 10, 0, 0, 0, 0, time.UTC), PredictionDueAt: time.Date(2026, 4, 17, 0, 0, 0, 0, time.UTC)}}, Edges: []c.GraphEdge{{From: "n1", To: "n2", Kind: c.EdgePositive}}}, Details: c.HiddenDetails{Caveats: []string{"说明"}}}, CompiledAt: time.Now().UTC()}
		if err := store.UpsertCompiledOutput(context.Background(), record); err != nil {
			return nil, err
		}
		if err := store.PersistMemoryContentGraphFromCompiledOutput(context.Background(), "u-content-card", "twitter", "CGCARD1", time.Now().UTC()); err != nil {
			return nil, err
		}
		return store, nil
	}
	var stdout, stderr bytes.Buffer
	code := run([]string{"memory", "content-graphs", "--card", "--user", "u-content-card", "--platform", "twitter", "--id", "CGCARD1"}, "/tmp/project", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("content-graphs --card code = %d, stderr = %s", code, stderr.String())
	}
	for _, want := range []string{"Content Graph", "twitter", "CGCARD1", "Primary nodes"} {
		if !strings.Contains(stdout.String(), want) {
			t.Fatalf("stdout = %q, want substring %q", stdout.String(), want)
		}
	}
}
