package main

import (
	"bytes"
	"context"
	"encoding/json"
	c "github.com/kumaloha/VariX/varix/compile"
	"github.com/kumaloha/VariX/varix/ingest"
	varixllm "github.com/kumaloha/VariX/varix/llm"
	"github.com/kumaloha/VariX/varix/model"
	"github.com/kumaloha/VariX/varix/storage/contentstore"
	"strings"
	"testing"
	"time"
)

func TestRunMemoryBackfillRequiresValidLayerInputs(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{"memory", "backfill"}, "/tmp/project", &stdout, &stderr)
	if code != 2 {
		t.Fatalf("run() code = %d, want 2", code)
	}
	if !strings.Contains(stderr.String(), "usage: varix memory backfill") {
		t.Fatalf("stderr = %q, want usage", stderr.String())
	}
}

func TestRunMemoryProjectAllRebuildsEventAndParadigmLayers(t *testing.T) {
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
		sg := testDriverTargetSubgraph("project-all", time.Now().UTC())
		if err := store.PersistMemoryContentGraph(context.Background(), "u-project-all", sg, time.Now().UTC()); err != nil {
			return nil, err
		}
		return store, nil
	}
	var stdout, stderr bytes.Buffer
	code := run([]string{"memory", "project-all", "--user", "u-project-all"}, "/tmp/project", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("memory project-all code = %d, stderr = %s", code, stderr.String())
	}
	var out map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &out); err != nil {
		t.Fatalf("json.Unmarshal(project-all) error = %v", err)
	}
	if out["content_graphs"] == nil || out["event_graphs"] == nil || out["paradigms"] == nil || out["global_synthesis"] == nil {
		t.Fatalf("project-all output = %#v, want content_graphs/event_graphs/paradigms/global_synthesis keys", out)
	}
	if ok, _ := out["ok"].(bool); !ok {
		t.Fatalf("project-all output = %#v, want ok=true", out)
	}
	metrics, ok := out["metrics"].(map[string]any)
	if !ok {
		t.Fatalf("project-all output = %#v, want metrics object", out)
	}
	for _, key := range []string{"event_graph_rebuild_ms", "paradigm_recompute_ms", "global_synthesis_rebuild_ms"} {
		value, ok := metrics[key]
		if !ok {
			t.Fatalf("project-all metrics = %#v, want key %q", metrics, key)
		}
		number, ok := value.(float64)
		if !ok || number < 0 {
			t.Fatalf("project-all metrics[%q] = %#v, want non-negative number", key, value)
		}
	}
}

func TestRunMemoryProjectionSweepProcessesPendingMarks(t *testing.T) {
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
		now := time.Date(2026, 4, 30, 12, 0, 0, 0, time.UTC)
		sg := model.ContentSubgraph{ID: "projection-sweep", ArticleID: "projection-sweep", SourcePlatform: "twitter", SourceExternalID: "projection-sweep", CompileVersion: model.CompileBridgeVersion, CompiledAt: now.Format(time.RFC3339), UpdatedAt: now.Format(time.RFC3339), Nodes: []model.ContentNode{{ID: "driver", SourceArticleID: "projection-sweep", SourcePlatform: "twitter", SourceExternalID: "projection-sweep", RawText: "油价继续上涨", SubjectText: "油价", ChangeText: "继续上涨", Kind: model.NodeKindObservation, GraphRole: model.GraphRoleDriver, IsPrimary: true, VerificationStatus: model.VerificationProved, TimeBucket: "1w"}, {ID: "target", SourceArticleID: "projection-sweep", SourcePlatform: "twitter", SourceExternalID: "projection-sweep", RawText: "美股从纪录高位回落", SubjectText: "美股", ChangeText: "从纪录高位回落", Kind: model.NodeKindPrediction, GraphRole: model.GraphRoleTarget, IsPrimary: true, VerificationStatus: model.VerificationProved, TimeBucket: "1w"}}}
		if err := store.PersistMemoryContentGraphDeferred(context.Background(), "u-projection-sweep", sg, now); err != nil {
			return nil, err
		}
		return store, nil
	}

	var stdout, stderr bytes.Buffer
	code := run([]string{"memory", "projection-sweep", "--user", "u-projection-sweep", "--limit", "100", "--workers", "2"}, "/tmp/project", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("memory projection-sweep code = %d, stderr = %s", code, stderr.String())
	}
	var out map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &out); err != nil {
		t.Fatalf("json.Unmarshal(projection-sweep) error = %v", err)
	}
	if scanned, _ := out["scanned"].(float64); scanned == 0 {
		t.Fatalf("projection-sweep output = %#v, want scanned > 0", out)
	}
	if failed, _ := out["failed"].(float64); failed != 0 {
		t.Fatalf("projection-sweep output = %#v, want failed=0", out)
	}
	if remaining, _ := out["remaining"].(float64); remaining != 0 {
		t.Fatalf("projection-sweep output = %#v, want remaining=0", out)
	}
	if workers, _ := out["workers"].(float64); workers != 2 {
		t.Fatalf("projection-sweep output = %#v, want workers=2", out)
	}
}

func TestRunMemoryBackfillContentRebuildsOneContentGraphFromCompiledOutput(t *testing.T) {
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
			UnitID:         "twitter:bf-content-1",
			Source:         "twitter",
			ExternalID:     "bf-content-1",
			RootExternalID: "bf-content-1",
			Model:          varixllm.Qwen36PlusModel,
			Output: c.Output{
				Summary: "一句话",
				Drivers: []string{"美联储加息0.25%"},
				Targets: []string{"未来一周美股承压"},
				Graph: c.ReasoningGraph{
					Nodes: []c.GraphNode{
						{ID: "n1", Kind: c.NodeFact, Text: "美联储加息0.25%", OccurredAt: time.Now().UTC()},
						{ID: "n2", Kind: c.NodePrediction, Text: "未来一周美股承压", PredictionStartAt: time.Now().UTC(), PredictionDueAt: time.Now().UTC().Add(24 * time.Hour)},
					},
					Edges: []c.GraphEdge{{From: "n1", To: "n2", Kind: c.EdgePositive}},
				},
				Details: c.HiddenDetails{Caveats: []string{"说明"}},
			},
			CompiledAt: time.Now().UTC(),
		}
		if err := store.UpsertCompiledOutput(context.Background(), record); err != nil {
			return nil, err
		}
		return store, nil
	}
	var stdout, stderr bytes.Buffer
	code := run([]string{"memory", "backfill", "--layer", "content", "--user", "u-backfill-content", "--platform", "twitter", "--id", "bf-content-1"}, "/tmp/project", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("memory backfill content code = %d, stderr = %s", code, stderr.String())
	}
	var out map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &out); err != nil {
		t.Fatalf("json.Unmarshal(backfill content) error = %v", err)
	}
	if out["layer"] != "content" || out["content_graphs"] == nil {
		t.Fatalf("backfill content output = %#v, want layer=content and content_graphs key", out)
	}
	if got, ok := out["content_graphs"].(float64); !ok || got != 1 {
		t.Fatalf("content_graphs = %#v, want 1", out["content_graphs"])
	}
}

func TestRunMemoryBackfillAllRebuildsAggregateLayers(t *testing.T) {
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
		sg := testDriverTargetSubgraph("bf-all", time.Now().UTC())
		if err := store.PersistMemoryContentGraph(context.Background(), "u-backfill-all", sg, time.Now().UTC()); err != nil {
			return nil, err
		}
		return store, nil
	}
	var stdout, stderr bytes.Buffer
	code := run([]string{"memory", "backfill", "--layer", "all", "--user", "u-backfill-all"}, "/tmp/project", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("memory backfill all code = %d, stderr = %s", code, stderr.String())
	}
	var out map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &out); err != nil {
		t.Fatalf("json.Unmarshal(backfill all) error = %v", err)
	}
	if out["layer"] != "all" || out["event_graphs"] == nil || out["paradigms"] == nil || out["global_synthesis"] == nil {
		t.Fatalf("backfill all output = %#v, want aggregate keys", out)
	}
}

func TestRunMemoryBackfillEventRebuildsEventLayer(t *testing.T) {
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
		sg := testDriverTargetSubgraph("bf-event", time.Now().UTC())
		if err := store.PersistMemoryContentGraph(context.Background(), "u-backfill-event", sg, time.Now().UTC()); err != nil {
			return nil, err
		}
		return store, nil
	}
	var stdout, stderr bytes.Buffer
	code := run([]string{"memory", "backfill", "--layer", "event", "--user", "u-backfill-event"}, "/tmp/project", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("memory backfill event code = %d, stderr = %s", code, stderr.String())
	}
	var out map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &out); err != nil {
		t.Fatalf("json.Unmarshal(backfill event) error = %v", err)
	}
	if out["layer"] != "event" || out["event_graphs"] == nil {
		t.Fatalf("backfill event output = %#v, want layer=event and event_graphs key", out)
	}
}

func TestRunMemoryBackfillParadigmRebuildsParadigmLayer(t *testing.T) {
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
		sg := testDriverTargetSubgraph("bf-paradigm", time.Now().UTC())
		if err := store.PersistMemoryContentGraph(context.Background(), "u-backfill-paradigm", sg, time.Now().UTC()); err != nil {
			return nil, err
		}
		return store, nil
	}
	var stdout, stderr bytes.Buffer
	code := run([]string{"memory", "backfill", "--layer", "paradigm", "--user", "u-backfill-paradigm"}, "/tmp/project", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("memory backfill paradigm code = %d, stderr = %s", code, stderr.String())
	}
	var out map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &out); err != nil {
		t.Fatalf("json.Unmarshal(backfill paradigm) error = %v", err)
	}
	if out["layer"] != "paradigm" || out["paradigms"] == nil {
		t.Fatalf("backfill paradigm output = %#v, want layer=paradigm and paradigms key", out)
	}
}

func TestRunMemoryBackfillGlobalSynthesisRebuildsGlobalLayer(t *testing.T) {
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
		sg := testDriverTargetSubgraph("bf-global-synthesis", time.Now().UTC())
		if err := store.PersistMemoryContentGraph(context.Background(), "u-backfill-global-synthesis", sg, time.Now().UTC()); err != nil {
			return nil, err
		}
		return store, nil
	}
	var stdout, stderr bytes.Buffer
	code := run([]string{"memory", "backfill", "--layer", "global-synthesis", "--user", "u-backfill-global-synthesis"}, "/tmp/project", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("memory backfill global-synthesis code = %d, stderr = %s", code, stderr.String())
	}
	var out map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &out); err != nil {
		t.Fatalf("json.Unmarshal(backfill global-synthesis) error = %v", err)
	}
	if out["layer"] != "global-synthesis" || out["global_synthesis"] == nil {
		t.Fatalf("backfill global-synthesis output = %#v, want layer=global-synthesis and global_synthesis key", out)
	}
}
