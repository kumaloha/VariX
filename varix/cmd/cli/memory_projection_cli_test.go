package main

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/kumaloha/VariX/varix/bootstrap"
	c "github.com/kumaloha/VariX/varix/compile"
	"github.com/kumaloha/VariX/varix/graphmodel"
	"github.com/kumaloha/VariX/varix/memory"
	"github.com/kumaloha/VariX/varix/storage/contentstore"
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

func TestRunMemoryEventGraphsPrintsProjectedEventGraphs(t *testing.T) {
	prevBuildApp := buildApp
	prevOpenSQLiteStore := openSQLiteStore
	t.Cleanup(func() {
		buildApp = prevBuildApp
		openSQLiteStore = prevOpenSQLiteStore
	})

	tmp := t.TempDir()
	buildApp = func(projectRoot string) (*bootstrap.App, error) {
		app := &bootstrap.App{}
		app.Settings.ContentDBPath = tmp + "/content.db"
		return app, nil
	}
	openSQLiteStore = func(path string) (*contentstore.SQLiteStore, error) {
		store, err := contentstore.NewSQLiteStore(path)
		if err != nil {
			return nil, err
		}
		record := c.Record{
			UnitID: "twitter:EG1", Source: "twitter", ExternalID: "EG1", RootExternalID: "EG1", Model: c.Qwen36PlusModel,
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
	prevBuildApp := buildApp
	prevOpenSQLiteStore := openSQLiteStore
	t.Cleanup(func() {
		buildApp = prevBuildApp
		openSQLiteStore = prevOpenSQLiteStore
	})

	tmp := t.TempDir()
	buildApp = func(projectRoot string) (*bootstrap.App, error) {
		app := &bootstrap.App{}
		app.Settings.ContentDBPath = tmp + "/content.db"
		return app, nil
	}
	openSQLiteStore = func(path string) (*contentstore.SQLiteStore, error) {
		store, err := contentstore.NewSQLiteStore(path)
		if err != nil {
			return nil, err
		}
		record := c.Record{
			UnitID: "twitter:PG1", Source: "twitter", ExternalID: "PG1", RootExternalID: "PG1", Model: c.Qwen36PlusModel,
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
	prevBuildApp := buildApp
	prevOpenSQLiteStore := openSQLiteStore
	t.Cleanup(func() {
		buildApp = prevBuildApp
		openSQLiteStore = prevOpenSQLiteStore
	})

	tmp := t.TempDir()
	buildApp = func(projectRoot string) (*bootstrap.App, error) {
		app := &bootstrap.App{}
		app.Settings.ContentDBPath = tmp + "/content.db"
		return app, nil
	}
	openSQLiteStore = func(path string) (*contentstore.SQLiteStore, error) {
		store, err := contentstore.NewSQLiteStore(path)
		if err != nil {
			return nil, err
		}
		record := c.Record{
			UnitID: "twitter:CG1", Source: "twitter", ExternalID: "CG1", RootExternalID: "CG1", Model: c.Qwen36PlusModel,
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
	prevBuildApp := buildApp
	prevOpenSQLiteStore := openSQLiteStore
	t.Cleanup(func() {
		buildApp = prevBuildApp
		openSQLiteStore = prevOpenSQLiteStore
	})

	tmp := t.TempDir()
	buildApp = func(projectRoot string) (*bootstrap.App, error) {
		app := &bootstrap.App{}
		app.Settings.ContentDBPath = tmp + "/content.db"
		return app, nil
	}
	openSQLiteStore = func(path string) (*contentstore.SQLiteStore, error) {
		store, err := contentstore.NewSQLiteStore(path)
		if err != nil {
			return nil, err
		}
		sg := graphmodel.ContentSubgraph{ID: "manual-eg", ArticleID: "manual-eg", SourcePlatform: "twitter", SourceExternalID: "manual-eg", CompileVersion: graphmodel.CompileBridgeVersion, CompiledAt: time.Now().UTC().Format(time.RFC3339), UpdatedAt: time.Now().UTC().Format(time.RFC3339), Nodes: []graphmodel.GraphNode{{ID: "n1", SourceArticleID: "manual-eg", SourcePlatform: "twitter", SourceExternalID: "manual-eg", RawText: "美联储加息0.25%", SubjectText: "美联储", ChangeText: "加息0.25%", Kind: graphmodel.NodeKindObservation, GraphRole: graphmodel.GraphRoleDriver, IsPrimary: true, VerificationStatus: graphmodel.VerificationPending, TimeBucket: "1w"}, {ID: "n2", SourceArticleID: "manual-eg", SourcePlatform: "twitter", SourceExternalID: "manual-eg", RawText: "未来一周美股承压", SubjectText: "美股", ChangeText: "承压", Kind: graphmodel.NodeKindPrediction, GraphRole: graphmodel.GraphRoleTarget, IsPrimary: true, VerificationStatus: graphmodel.VerificationProved, TimeBucket: "1w"}}}
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
	prevBuildApp := buildApp
	prevOpenSQLiteStore := openSQLiteStore
	t.Cleanup(func() {
		buildApp = prevBuildApp
		openSQLiteStore = prevOpenSQLiteStore
	})

	tmp := t.TempDir()
	buildApp = func(projectRoot string) (*bootstrap.App, error) {
		app := &bootstrap.App{}
		app.Settings.ContentDBPath = tmp + "/content.db"
		return app, nil
	}
	openSQLiteStore = func(path string) (*contentstore.SQLiteStore, error) {
		store, err := contentstore.NewSQLiteStore(path)
		if err != nil {
			return nil, err
		}
		sg := graphmodel.ContentSubgraph{ID: "manual-pg", ArticleID: "manual-pg", SourcePlatform: "twitter", SourceExternalID: "manual-pg", CompileVersion: graphmodel.CompileBridgeVersion, CompiledAt: time.Now().UTC().Format(time.RFC3339), UpdatedAt: time.Now().UTC().Format(time.RFC3339), Nodes: []graphmodel.GraphNode{{ID: "n1", SourceArticleID: "manual-pg", SourcePlatform: "twitter", SourceExternalID: "manual-pg", RawText: "美联储加息0.25%", SubjectText: "美联储", ChangeText: "加息0.25%", Kind: graphmodel.NodeKindObservation, GraphRole: graphmodel.GraphRoleDriver, IsPrimary: true, VerificationStatus: graphmodel.VerificationPending, TimeBucket: "1w"}, {ID: "n2", SourceArticleID: "manual-pg", SourcePlatform: "twitter", SourceExternalID: "manual-pg", RawText: "未来一周美股承压", SubjectText: "美股", ChangeText: "承压", Kind: graphmodel.NodeKindPrediction, GraphRole: graphmodel.GraphRoleTarget, IsPrimary: true, VerificationStatus: graphmodel.VerificationProved, TimeBucket: "1w"}}}
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

func TestRunMemoryEventGraphsSupportsScopeFilter(t *testing.T) {
	prevBuildApp := buildApp
	prevOpenSQLiteStore := openSQLiteStore
	t.Cleanup(func() {
		buildApp = prevBuildApp
		openSQLiteStore = prevOpenSQLiteStore
	})

	tmp := t.TempDir()
	buildApp = func(projectRoot string) (*bootstrap.App, error) {
		app := &bootstrap.App{}
		app.Settings.ContentDBPath = tmp + "/content.db"
		return app, nil
	}
	openSQLiteStore = func(path string) (*contentstore.SQLiteStore, error) {
		store, err := contentstore.NewSQLiteStore(path)
		if err != nil {
			return nil, err
		}
		sg := graphmodel.ContentSubgraph{ID: "filter-eg", ArticleID: "filter-eg", SourcePlatform: "twitter", SourceExternalID: "filter-eg", CompileVersion: graphmodel.CompileBridgeVersion, CompiledAt: time.Now().UTC().Format(time.RFC3339), UpdatedAt: time.Now().UTC().Format(time.RFC3339), Nodes: []graphmodel.GraphNode{{ID: "n1", SourceArticleID: "filter-eg", SourcePlatform: "twitter", SourceExternalID: "filter-eg", RawText: "美联储加息0.25%", SubjectText: "美联储", ChangeText: "加息0.25%", Kind: graphmodel.NodeKindObservation, GraphRole: graphmodel.GraphRoleDriver, IsPrimary: true, VerificationStatus: graphmodel.VerificationPending, TimeBucket: "1w"}, {ID: "n2", SourceArticleID: "filter-eg", SourcePlatform: "twitter", SourceExternalID: "filter-eg", RawText: "未来一周美股承压", SubjectText: "美股", ChangeText: "承压", Kind: graphmodel.NodeKindPrediction, GraphRole: graphmodel.GraphRoleTarget, IsPrimary: true, VerificationStatus: graphmodel.VerificationProved, TimeBucket: "1w"}}}
		if err := store.PersistMemoryContentGraph(context.Background(), "u-event-filter", sg, time.Now().UTC()); err != nil {
			return nil, err
		}
		return store, nil
	}
	var stdout, stderr bytes.Buffer
	code := run([]string{"memory", "event-graphs", "--user", "u-event-filter", "--scope", "target"}, "/tmp/project", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("event-graphs --scope target code = %d, stderr = %s", code, stderr.String())
	}
	var out []contentstore.EventGraphRecord
	if err := json.Unmarshal(stdout.Bytes(), &out); err != nil {
		t.Fatalf("json.Unmarshal(event-graphs --scope) error = %v", err)
	}
	if len(out) != 1 || out[0].Scope != "target" {
		t.Fatalf("event-graphs filtered output = %#v, want one target graph", out)
	}
}

func TestRunMemoryParadigmsSupportsSubjectFilter(t *testing.T) {
	prevBuildApp := buildApp
	prevOpenSQLiteStore := openSQLiteStore
	t.Cleanup(func() {
		buildApp = prevBuildApp
		openSQLiteStore = prevOpenSQLiteStore
	})

	tmp := t.TempDir()
	buildApp = func(projectRoot string) (*bootstrap.App, error) {
		app := &bootstrap.App{}
		app.Settings.ContentDBPath = tmp + "/content.db"
		return app, nil
	}
	openSQLiteStore = func(path string) (*contentstore.SQLiteStore, error) {
		store, err := contentstore.NewSQLiteStore(path)
		if err != nil {
			return nil, err
		}
		sg := graphmodel.ContentSubgraph{ID: "filter-pg", ArticleID: "filter-pg", SourcePlatform: "twitter", SourceExternalID: "filter-pg", CompileVersion: graphmodel.CompileBridgeVersion, CompiledAt: time.Now().UTC().Format(time.RFC3339), UpdatedAt: time.Now().UTC().Format(time.RFC3339), Nodes: []graphmodel.GraphNode{{ID: "n1", SourceArticleID: "filter-pg", SourcePlatform: "twitter", SourceExternalID: "filter-pg", RawText: "美联储加息0.25%", SubjectText: "美联储", ChangeText: "加息0.25%", Kind: graphmodel.NodeKindObservation, GraphRole: graphmodel.GraphRoleDriver, IsPrimary: true, VerificationStatus: graphmodel.VerificationPending, TimeBucket: "1w"}, {ID: "n2", SourceArticleID: "filter-pg", SourcePlatform: "twitter", SourceExternalID: "filter-pg", RawText: "未来一周美股承压", SubjectText: "美股", ChangeText: "承压", Kind: graphmodel.NodeKindPrediction, GraphRole: graphmodel.GraphRoleTarget, IsPrimary: true, VerificationStatus: graphmodel.VerificationProved, TimeBucket: "1w"}}}
		if err := store.PersistMemoryContentGraph(context.Background(), "u-paradigm-filter", sg, time.Now().UTC()); err != nil {
			return nil, err
		}
		return store, nil
	}
	var stdout, stderr bytes.Buffer
	code := run([]string{"memory", "paradigms", "--user", "u-paradigm-filter", "--subject", "美联储"}, "/tmp/project", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("paradigms --subject 美联储 code = %d, stderr = %s", code, stderr.String())
	}
	var out []contentstore.ParadigmRecord
	if err := json.Unmarshal(stdout.Bytes(), &out); err != nil {
		t.Fatalf("json.Unmarshal(paradigms --subject) error = %v", err)
	}
	if len(out) != 1 || out[0].DriverSubject != "美联储" {
		t.Fatalf("paradigms filtered output = %#v, want 美联储 paradigm", out)
	}
}

func TestRunMemoryContentGraphsRunRebuildsSnapshotFromCompiledOutput(t *testing.T) {
	prevBuildApp := buildApp
	prevOpenSQLiteStore := openSQLiteStore
	t.Cleanup(func() {
		buildApp = prevBuildApp
		openSQLiteStore = prevOpenSQLiteStore
	})

	tmp := t.TempDir()
	buildApp = func(projectRoot string) (*bootstrap.App, error) {
		app := &bootstrap.App{}
		app.Settings.ContentDBPath = tmp + "/content.db"
		return app, nil
	}
	openSQLiteStore = func(path string) (*contentstore.SQLiteStore, error) {
		store, err := contentstore.NewSQLiteStore(path)
		if err != nil {
			return nil, err
		}
		record := c.Record{UnitID: "twitter:CGRUN1", Source: "twitter", ExternalID: "CGRUN1", RootExternalID: "CGRUN1", Model: c.Qwen36PlusModel, Output: c.Output{Summary: "一句话", Drivers: []string{"美联储加息0.25%"}, Targets: []string{"未来一周美股承压"}, Graph: c.ReasoningGraph{Nodes: []c.GraphNode{{ID: "n1", Kind: c.NodeFact, Text: "美联储加息0.25%", OccurredAt: time.Date(2026, 4, 10, 0, 0, 0, 0, time.UTC)}, {ID: "n2", Kind: c.NodePrediction, Text: "未来一周美股承压", PredictionStartAt: time.Date(2026, 4, 10, 0, 0, 0, 0, time.UTC), PredictionDueAt: time.Date(2026, 4, 17, 0, 0, 0, 0, time.UTC)}}, Edges: []c.GraphEdge{{From: "n1", To: "n2", Kind: c.EdgePositive}}}, Details: c.HiddenDetails{Caveats: []string{"说明"}}}, CompiledAt: time.Now().UTC()}
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
	prevBuildApp := buildApp
	prevOpenSQLiteStore := openSQLiteStore
	t.Cleanup(func() {
		buildApp = prevBuildApp
		openSQLiteStore = prevOpenSQLiteStore
	})

	tmp := t.TempDir()
	cardNow := time.Date(2026, 4, 21, 0, 0, 0, 0, time.UTC)
	buildApp = func(projectRoot string) (*bootstrap.App, error) {
		app := &bootstrap.App{}
		app.Settings.ContentDBPath = tmp + "/content.db"
		return app, nil
	}
	openSQLiteStore = func(path string) (*contentstore.SQLiteStore, error) {
		store, err := contentstore.NewSQLiteStore(path)
		if err != nil {
			return nil, err
		}
		sg := graphmodel.ContentSubgraph{ID: "card-eg", ArticleID: "card-eg", SourcePlatform: "twitter", SourceExternalID: "card-eg", CompileVersion: graphmodel.CompileBridgeVersion, CompiledAt: cardNow.Format(time.RFC3339), UpdatedAt: cardNow.Format(time.RFC3339), Nodes: []graphmodel.GraphNode{{ID: "n1", SourceArticleID: "card-eg", SourcePlatform: "twitter", SourceExternalID: "card-eg", RawText: "美联储加息0.25%", SubjectText: "美联储", ChangeText: "加息0.25%", TimeStart: cardNow.Format(time.RFC3339), Kind: graphmodel.NodeKindObservation, GraphRole: graphmodel.GraphRoleDriver, IsPrimary: true, VerificationStatus: graphmodel.VerificationPending, TimeBucket: "1w"}, {ID: "n2", SourceArticleID: "card-eg", SourcePlatform: "twitter", SourceExternalID: "card-eg", RawText: "未来一周美股承压", SubjectText: "美股", ChangeText: "承压", TimeStart: cardNow.Format(time.RFC3339), Kind: graphmodel.NodeKindPrediction, GraphRole: graphmodel.GraphRoleTarget, IsPrimary: true, VerificationStatus: graphmodel.VerificationProved, TimeBucket: "1w"}}}
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
	prevBuildApp := buildApp
	prevOpenSQLiteStore := openSQLiteStore
	t.Cleanup(func() {
		buildApp = prevBuildApp
		openSQLiteStore = prevOpenSQLiteStore
	})

	tmp := t.TempDir()
	buildApp = func(projectRoot string) (*bootstrap.App, error) {
		app := &bootstrap.App{}
		app.Settings.ContentDBPath = tmp + "/content.db"
		return app, nil
	}
	openSQLiteStore = func(path string) (*contentstore.SQLiteStore, error) {
		store, err := contentstore.NewSQLiteStore(path)
		if err != nil {
			return nil, err
		}
		sg := graphmodel.ContentSubgraph{ID: "card-pg", ArticleID: "card-pg", SourcePlatform: "twitter", SourceExternalID: "card-pg", CompileVersion: graphmodel.CompileBridgeVersion, CompiledAt: time.Now().UTC().Format(time.RFC3339), UpdatedAt: time.Now().UTC().Format(time.RFC3339), Nodes: []graphmodel.GraphNode{{ID: "n1", SourceArticleID: "card-pg", SourcePlatform: "twitter", SourceExternalID: "card-pg", RawText: "美联储加息0.25%", SubjectText: "美联储", ChangeText: "加息0.25%", Kind: graphmodel.NodeKindObservation, GraphRole: graphmodel.GraphRoleDriver, IsPrimary: true, VerificationStatus: graphmodel.VerificationPending, TimeBucket: "1w"}, {ID: "n2", SourceArticleID: "card-pg", SourcePlatform: "twitter", SourceExternalID: "card-pg", RawText: "未来一周美股承压", SubjectText: "美股", ChangeText: "承压", Kind: graphmodel.NodeKindPrediction, GraphRole: graphmodel.GraphRoleTarget, IsPrimary: true, VerificationStatus: graphmodel.VerificationProved, TimeBucket: "1w"}}}
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

func TestRunMemoryContentGraphsSupportsSourceFilter(t *testing.T) {
	prevBuildApp := buildApp
	prevOpenSQLiteStore := openSQLiteStore
	t.Cleanup(func() {
		buildApp = prevBuildApp
		openSQLiteStore = prevOpenSQLiteStore
	})

	tmp := t.TempDir()
	buildApp = func(projectRoot string) (*bootstrap.App, error) {
		app := &bootstrap.App{}
		app.Settings.ContentDBPath = tmp + "/content.db"
		return app, nil
	}
	openSQLiteStore = func(path string) (*contentstore.SQLiteStore, error) {
		store, err := contentstore.NewSQLiteStore(path)
		if err != nil {
			return nil, err
		}
		for _, rec := range []c.Record{
			{UnitID: "twitter:CF1", Source: "twitter", ExternalID: "CF1", RootExternalID: "CF1", Model: c.Qwen36PlusModel, Output: c.Output{Summary: "一句话", Graph: c.ReasoningGraph{Nodes: []c.GraphNode{{ID: "n1", Kind: c.NodeFact, Text: "A", OccurredAt: time.Now().UTC()}, {ID: "n2", Kind: c.NodeConclusion, Text: "B", ValidFrom: time.Now().UTC(), ValidTo: time.Now().UTC().Add(24 * time.Hour)}}, Edges: []c.GraphEdge{{From: "n1", To: "n2", Kind: c.EdgePositive}}}, Details: c.HiddenDetails{Caveats: []string{"说明"}}}, CompiledAt: time.Now().UTC()},
			{UnitID: "twitter:CF2", Source: "twitter", ExternalID: "CF2", RootExternalID: "CF2", Model: c.Qwen36PlusModel, Output: c.Output{Summary: "一句话", Graph: c.ReasoningGraph{Nodes: []c.GraphNode{{ID: "n1", Kind: c.NodeFact, Text: "X", OccurredAt: time.Now().UTC()}, {ID: "n2", Kind: c.NodeConclusion, Text: "Y", ValidFrom: time.Now().UTC(), ValidTo: time.Now().UTC().Add(24 * time.Hour)}}, Edges: []c.GraphEdge{{From: "n1", To: "n2", Kind: c.EdgePositive}}}, Details: c.HiddenDetails{Caveats: []string{"说明"}}}, CompiledAt: time.Now().UTC()},
		} {
			if err := store.UpsertCompiledOutput(context.Background(), rec); err != nil {
				return nil, err
			}
			if err := store.PersistMemoryContentGraphFromCompiledOutput(context.Background(), "u-content-filter", rec.Source, rec.ExternalID, time.Now().UTC()); err != nil {
				return nil, err
			}
		}
		return store, nil
	}
	var stdout, stderr bytes.Buffer
	code := run([]string{"memory", "content-graphs", "--user", "u-content-filter", "--platform", "twitter", "--id", "CF2"}, "/tmp/project", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("content-graphs filtered code = %d, stderr = %s", code, stderr.String())
	}
	var out []graphmodel.ContentSubgraph
	if err := json.Unmarshal(stdout.Bytes(), &out); err != nil {
		t.Fatalf("json.Unmarshal(content-graphs filter) error = %v", err)
	}
	if len(out) != 1 || out[0].SourceExternalID != "CF2" {
		t.Fatalf("content-graphs filtered output = %#v, want CF2 only", out)
	}
}

func TestRunMemoryContentGraphsCardPrintsReadableSections(t *testing.T) {
	prevBuildApp := buildApp
	prevOpenSQLiteStore := openSQLiteStore
	t.Cleanup(func() {
		buildApp = prevBuildApp
		openSQLiteStore = prevOpenSQLiteStore
	})

	tmp := t.TempDir()
	buildApp = func(projectRoot string) (*bootstrap.App, error) {
		app := &bootstrap.App{}
		app.Settings.ContentDBPath = tmp + "/content.db"
		return app, nil
	}
	openSQLiteStore = func(path string) (*contentstore.SQLiteStore, error) {
		store, err := contentstore.NewSQLiteStore(path)
		if err != nil {
			return nil, err
		}
		record := c.Record{UnitID: "twitter:CGCARD1", Source: "twitter", ExternalID: "CGCARD1", RootExternalID: "CGCARD1", Model: c.Qwen36PlusModel, Output: c.Output{Summary: "一句话", Drivers: []string{"美联储加息0.25%"}, Targets: []string{"未来一周美股承压"}, Graph: c.ReasoningGraph{Nodes: []c.GraphNode{{ID: "n1", Kind: c.NodeFact, Text: "美联储加息0.25%", OccurredAt: time.Date(2026, 4, 10, 0, 0, 0, 0, time.UTC)}, {ID: "n2", Kind: c.NodePrediction, Text: "未来一周美股承压", PredictionStartAt: time.Date(2026, 4, 10, 0, 0, 0, 0, time.UTC), PredictionDueAt: time.Date(2026, 4, 17, 0, 0, 0, 0, time.UTC)}}, Edges: []c.GraphEdge{{From: "n1", To: "n2", Kind: c.EdgePositive}}}, Details: c.HiddenDetails{Caveats: []string{"说明"}}}, CompiledAt: time.Now().UTC()}
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

func TestRunMemoryEventGraphsSupportsSubjectFilter(t *testing.T) {
	prevBuildApp := buildApp
	prevOpenSQLiteStore := openSQLiteStore
	t.Cleanup(func() {
		buildApp = prevBuildApp
		openSQLiteStore = prevOpenSQLiteStore
	})

	tmp := t.TempDir()
	buildApp = func(projectRoot string) (*bootstrap.App, error) {
		app := &bootstrap.App{}
		app.Settings.ContentDBPath = tmp + "/content.db"
		return app, nil
	}
	openSQLiteStore = func(path string) (*contentstore.SQLiteStore, error) {
		store, err := contentstore.NewSQLiteStore(path)
		if err != nil {
			return nil, err
		}
		sg := graphmodel.ContentSubgraph{ID: "subject-eg-cli", ArticleID: "subject-eg-cli", SourcePlatform: "twitter", SourceExternalID: "subject-eg-cli", CompileVersion: graphmodel.CompileBridgeVersion, CompiledAt: time.Now().UTC().Format(time.RFC3339), UpdatedAt: time.Now().UTC().Format(time.RFC3339), Nodes: []graphmodel.GraphNode{{ID: "n1", SourceArticleID: "subject-eg-cli", SourcePlatform: "twitter", SourceExternalID: "subject-eg-cli", RawText: "美联储加息0.25%", SubjectText: "美联储", ChangeText: "加息0.25%", Kind: graphmodel.NodeKindObservation, GraphRole: graphmodel.GraphRoleDriver, IsPrimary: true, VerificationStatus: graphmodel.VerificationPending, TimeBucket: "1w"}, {ID: "n2", SourceArticleID: "subject-eg-cli", SourcePlatform: "twitter", SourceExternalID: "subject-eg-cli", RawText: "未来一周美股承压", SubjectText: "美股", ChangeText: "承压", Kind: graphmodel.NodeKindPrediction, GraphRole: graphmodel.GraphRoleTarget, IsPrimary: true, VerificationStatus: graphmodel.VerificationProved, TimeBucket: "1w"}}}
		if err := store.PersistMemoryContentGraph(context.Background(), "u-event-subject", sg, time.Now().UTC()); err != nil {
			return nil, err
		}
		return store, nil
	}
	var stdout, stderr bytes.Buffer
	code := run([]string{"memory", "event-graphs", "--user", "u-event-subject", "--subject", "美联储"}, "/tmp/project", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("event-graphs --subject 美联储 code = %d, stderr = %s", code, stderr.String())
	}
	var out []contentstore.EventGraphRecord
	if err := json.Unmarshal(stdout.Bytes(), &out); err != nil {
		t.Fatalf("json.Unmarshal(event-graphs --subject) error = %v", err)
	}
	if len(out) != 1 || out[0].AnchorSubject != "美联储" {
		t.Fatalf("event-graphs filtered output = %#v, want 美联储 graph", out)
	}
}

func TestRunMemoryContentGraphsSupportsSubjectFilter(t *testing.T) {
	prevBuildApp := buildApp
	prevOpenSQLiteStore := openSQLiteStore
	t.Cleanup(func() {
		buildApp = prevBuildApp
		openSQLiteStore = prevOpenSQLiteStore
	})

	tmp := t.TempDir()
	buildApp = func(projectRoot string) (*bootstrap.App, error) {
		app := &bootstrap.App{}
		app.Settings.ContentDBPath = tmp + "/content.db"
		return app, nil
	}
	openSQLiteStore = func(path string) (*contentstore.SQLiteStore, error) {
		store, err := contentstore.NewSQLiteStore(path)
		if err != nil {
			return nil, err
		}
		now := time.Now().UTC()
		for _, sg := range []graphmodel.ContentSubgraph{
			{ID: "subject-cg-cli-1", ArticleID: "subject-cg-cli-1", SourcePlatform: "twitter", SourceExternalID: "subject-cg-cli-1", CompileVersion: graphmodel.CompileBridgeVersion, CompiledAt: now.Format(time.RFC3339), UpdatedAt: now.Format(time.RFC3339), Nodes: []graphmodel.GraphNode{{ID: "n1", SourceArticleID: "subject-cg-cli-1", SourcePlatform: "twitter", SourceExternalID: "subject-cg-cli-1", RawText: "美联储加息0.25%", SubjectText: "美联储", ChangeText: "加息0.25%", Kind: graphmodel.NodeKindObservation, IsPrimary: true, VerificationStatus: graphmodel.VerificationPending}}},
			{ID: "subject-cg-cli-2", ArticleID: "subject-cg-cli-2", SourcePlatform: "twitter", SourceExternalID: "subject-cg-cli-2", CompileVersion: graphmodel.CompileBridgeVersion, CompiledAt: now.Format(time.RFC3339), UpdatedAt: now.Format(time.RFC3339), Nodes: []graphmodel.GraphNode{{ID: "n1", SourceArticleID: "subject-cg-cli-2", SourcePlatform: "twitter", SourceExternalID: "subject-cg-cli-2", RawText: "欧洲央行放缓缩表", SubjectText: "欧洲央行", ChangeText: "放缓缩表", Kind: graphmodel.NodeKindObservation, IsPrimary: true, VerificationStatus: graphmodel.VerificationPending}}},
		} {
			if err := store.PersistMemoryContentGraph(context.Background(), "u-content-subject", sg, now); err != nil {
				return nil, err
			}
		}
		return store, nil
	}
	var stdout, stderr bytes.Buffer
	code := run([]string{"memory", "content-graphs", "--user", "u-content-subject", "--subject", "美联储"}, "/tmp/project", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("content-graphs --subject 美联储 code = %d, stderr = %s", code, stderr.String())
	}
	var out []graphmodel.ContentSubgraph
	if err := json.Unmarshal(stdout.Bytes(), &out); err != nil {
		t.Fatalf("json.Unmarshal(content-graphs --subject) error = %v", err)
	}
	if len(out) != 1 || out[0].SourceExternalID != "subject-cg-cli-1" {
		t.Fatalf("content-graphs filtered output = %#v, want 美联储 snapshot only", out)
	}
}

func TestRunMemoryProjectAllRebuildsEventAndParadigmLayers(t *testing.T) {
	prevBuildApp := buildApp
	prevOpenSQLiteStore := openSQLiteStore
	t.Cleanup(func() {
		buildApp = prevBuildApp
		openSQLiteStore = prevOpenSQLiteStore
	})

	tmp := t.TempDir()
	buildApp = func(projectRoot string) (*bootstrap.App, error) {
		app := &bootstrap.App{}
		app.Settings.ContentDBPath = tmp + "/content.db"
		return app, nil
	}
	openSQLiteStore = func(path string) (*contentstore.SQLiteStore, error) {
		store, err := contentstore.NewSQLiteStore(path)
		if err != nil {
			return nil, err
		}
		sg := graphmodel.ContentSubgraph{ID: "project-all", ArticleID: "project-all", SourcePlatform: "twitter", SourceExternalID: "project-all", CompileVersion: graphmodel.CompileBridgeVersion, CompiledAt: time.Now().UTC().Format(time.RFC3339), UpdatedAt: time.Now().UTC().Format(time.RFC3339), Nodes: []graphmodel.GraphNode{{ID: "n1", SourceArticleID: "project-all", SourcePlatform: "twitter", SourceExternalID: "project-all", RawText: "美联储加息0.25%", SubjectText: "美联储", ChangeText: "加息0.25%", Kind: graphmodel.NodeKindObservation, GraphRole: graphmodel.GraphRoleDriver, IsPrimary: true, VerificationStatus: graphmodel.VerificationPending, TimeBucket: "1w"}, {ID: "n2", SourceArticleID: "project-all", SourcePlatform: "twitter", SourceExternalID: "project-all", RawText: "未来一周美股承压", SubjectText: "美股", ChangeText: "承压", Kind: graphmodel.NodeKindPrediction, GraphRole: graphmodel.GraphRoleTarget, IsPrimary: true, VerificationStatus: graphmodel.VerificationProved, TimeBucket: "1w"}}}
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
	if out["content_graphs"] == nil || out["event_graphs"] == nil || out["paradigms"] == nil || out["global_v2"] == nil {
		t.Fatalf("project-all output = %#v, want content_graphs/event_graphs/paradigms/global_v2 keys", out)
	}
	if ok, _ := out["ok"].(bool); !ok {
		t.Fatalf("project-all output = %#v, want ok=true", out)
	}
	metrics, ok := out["metrics"].(map[string]any)
	if !ok {
		t.Fatalf("project-all output = %#v, want metrics object", out)
	}
	for _, key := range []string{"event_graph_rebuild_ms", "paradigm_recompute_ms", "global_v2_rebuild_ms"} {
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
	prevBuildApp := buildApp
	prevOpenSQLiteStore := openSQLiteStore
	t.Cleanup(func() {
		buildApp = prevBuildApp
		openSQLiteStore = prevOpenSQLiteStore
	})

	tmp := t.TempDir()
	buildApp = func(projectRoot string) (*bootstrap.App, error) {
		app := &bootstrap.App{}
		app.Settings.ContentDBPath = tmp + "/content.db"
		return app, nil
	}
	openSQLiteStore = func(path string) (*contentstore.SQLiteStore, error) {
		store, err := contentstore.NewSQLiteStore(path)
		if err != nil {
			return nil, err
		}
		now := time.Date(2026, 4, 30, 12, 0, 0, 0, time.UTC)
		sg := graphmodel.ContentSubgraph{ID: "projection-sweep", ArticleID: "projection-sweep", SourcePlatform: "twitter", SourceExternalID: "projection-sweep", CompileVersion: graphmodel.CompileBridgeVersion, CompiledAt: now.Format(time.RFC3339), UpdatedAt: now.Format(time.RFC3339), Nodes: []graphmodel.GraphNode{{ID: "driver", SourceArticleID: "projection-sweep", SourcePlatform: "twitter", SourceExternalID: "projection-sweep", RawText: "油价继续上涨", SubjectText: "油价", ChangeText: "继续上涨", Kind: graphmodel.NodeKindObservation, GraphRole: graphmodel.GraphRoleDriver, IsPrimary: true, VerificationStatus: graphmodel.VerificationProved, TimeBucket: "1w"}, {ID: "target", SourceArticleID: "projection-sweep", SourcePlatform: "twitter", SourceExternalID: "projection-sweep", RawText: "美股从纪录高位回落", SubjectText: "美股", ChangeText: "从纪录高位回落", Kind: graphmodel.NodeKindPrediction, GraphRole: graphmodel.GraphRoleTarget, IsPrimary: true, VerificationStatus: graphmodel.VerificationProved, TimeBucket: "1w"}}}
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
	prevBuildApp := buildApp
	prevOpenSQLiteStore := openSQLiteStore
	t.Cleanup(func() {
		buildApp = prevBuildApp
		openSQLiteStore = prevOpenSQLiteStore
	})

	tmp := t.TempDir()
	buildApp = func(projectRoot string) (*bootstrap.App, error) {
		app := &bootstrap.App{}
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
			Model:          c.Qwen36PlusModel,
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
	prevBuildApp := buildApp
	prevOpenSQLiteStore := openSQLiteStore
	t.Cleanup(func() {
		buildApp = prevBuildApp
		openSQLiteStore = prevOpenSQLiteStore
	})

	tmp := t.TempDir()
	buildApp = func(projectRoot string) (*bootstrap.App, error) {
		app := &bootstrap.App{}
		app.Settings.ContentDBPath = tmp + "/content.db"
		return app, nil
	}
	openSQLiteStore = func(path string) (*contentstore.SQLiteStore, error) {
		store, err := contentstore.NewSQLiteStore(path)
		if err != nil {
			return nil, err
		}
		sg := graphmodel.ContentSubgraph{ID: "bf-all", ArticleID: "bf-all", SourcePlatform: "twitter", SourceExternalID: "bf-all", CompileVersion: graphmodel.CompileBridgeVersion, CompiledAt: time.Now().UTC().Format(time.RFC3339), UpdatedAt: time.Now().UTC().Format(time.RFC3339), Nodes: []graphmodel.GraphNode{{ID: "n1", SourceArticleID: "bf-all", SourcePlatform: "twitter", SourceExternalID: "bf-all", RawText: "美联储加息0.25%", SubjectText: "美联储", ChangeText: "加息0.25%", Kind: graphmodel.NodeKindObservation, GraphRole: graphmodel.GraphRoleDriver, IsPrimary: true, VerificationStatus: graphmodel.VerificationPending, TimeBucket: "1w"}, {ID: "n2", SourceArticleID: "bf-all", SourcePlatform: "twitter", SourceExternalID: "bf-all", RawText: "未来一周美股承压", SubjectText: "美股", ChangeText: "承压", Kind: graphmodel.NodeKindPrediction, GraphRole: graphmodel.GraphRoleTarget, IsPrimary: true, VerificationStatus: graphmodel.VerificationProved, TimeBucket: "1w"}}}
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
	if out["layer"] != "all" || out["event_graphs"] == nil || out["paradigms"] == nil || out["global_v2"] == nil {
		t.Fatalf("backfill all output = %#v, want aggregate keys", out)
	}
}

func TestRunMemoryBackfillEventRebuildsEventLayer(t *testing.T) {
	prevBuildApp := buildApp
	prevOpenSQLiteStore := openSQLiteStore
	t.Cleanup(func() {
		buildApp = prevBuildApp
		openSQLiteStore = prevOpenSQLiteStore
	})

	tmp := t.TempDir()
	buildApp = func(projectRoot string) (*bootstrap.App, error) {
		app := &bootstrap.App{}
		app.Settings.ContentDBPath = tmp + "/content.db"
		return app, nil
	}
	openSQLiteStore = func(path string) (*contentstore.SQLiteStore, error) {
		store, err := contentstore.NewSQLiteStore(path)
		if err != nil {
			return nil, err
		}
		sg := graphmodel.ContentSubgraph{ID: "bf-event", ArticleID: "bf-event", SourcePlatform: "twitter", SourceExternalID: "bf-event", CompileVersion: graphmodel.CompileBridgeVersion, CompiledAt: time.Now().UTC().Format(time.RFC3339), UpdatedAt: time.Now().UTC().Format(time.RFC3339), Nodes: []graphmodel.GraphNode{{ID: "n1", SourceArticleID: "bf-event", SourcePlatform: "twitter", SourceExternalID: "bf-event", RawText: "美联储加息0.25%", SubjectText: "美联储", ChangeText: "加息0.25%", Kind: graphmodel.NodeKindObservation, GraphRole: graphmodel.GraphRoleDriver, IsPrimary: true, VerificationStatus: graphmodel.VerificationPending, TimeBucket: "1w"}, {ID: "n2", SourceArticleID: "bf-event", SourcePlatform: "twitter", SourceExternalID: "bf-event", RawText: "未来一周美股承压", SubjectText: "美股", ChangeText: "承压", Kind: graphmodel.NodeKindPrediction, GraphRole: graphmodel.GraphRoleTarget, IsPrimary: true, VerificationStatus: graphmodel.VerificationProved, TimeBucket: "1w"}}}
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
	prevBuildApp := buildApp
	prevOpenSQLiteStore := openSQLiteStore
	t.Cleanup(func() {
		buildApp = prevBuildApp
		openSQLiteStore = prevOpenSQLiteStore
	})

	tmp := t.TempDir()
	buildApp = func(projectRoot string) (*bootstrap.App, error) {
		app := &bootstrap.App{}
		app.Settings.ContentDBPath = tmp + "/content.db"
		return app, nil
	}
	openSQLiteStore = func(path string) (*contentstore.SQLiteStore, error) {
		store, err := contentstore.NewSQLiteStore(path)
		if err != nil {
			return nil, err
		}
		sg := graphmodel.ContentSubgraph{ID: "bf-paradigm", ArticleID: "bf-paradigm", SourcePlatform: "twitter", SourceExternalID: "bf-paradigm", CompileVersion: graphmodel.CompileBridgeVersion, CompiledAt: time.Now().UTC().Format(time.RFC3339), UpdatedAt: time.Now().UTC().Format(time.RFC3339), Nodes: []graphmodel.GraphNode{{ID: "n1", SourceArticleID: "bf-paradigm", SourcePlatform: "twitter", SourceExternalID: "bf-paradigm", RawText: "美联储加息0.25%", SubjectText: "美联储", ChangeText: "加息0.25%", Kind: graphmodel.NodeKindObservation, GraphRole: graphmodel.GraphRoleDriver, IsPrimary: true, VerificationStatus: graphmodel.VerificationPending, TimeBucket: "1w"}, {ID: "n2", SourceArticleID: "bf-paradigm", SourcePlatform: "twitter", SourceExternalID: "bf-paradigm", RawText: "未来一周美股承压", SubjectText: "美股", ChangeText: "承压", Kind: graphmodel.NodeKindPrediction, GraphRole: graphmodel.GraphRoleTarget, IsPrimary: true, VerificationStatus: graphmodel.VerificationProved, TimeBucket: "1w"}}}
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

func TestRunMemoryBackfillGlobalV2RebuildsGlobalLayer(t *testing.T) {
	prevBuildApp := buildApp
	prevOpenSQLiteStore := openSQLiteStore
	t.Cleanup(func() {
		buildApp = prevBuildApp
		openSQLiteStore = prevOpenSQLiteStore
	})

	tmp := t.TempDir()
	buildApp = func(projectRoot string) (*bootstrap.App, error) {
		app := &bootstrap.App{}
		app.Settings.ContentDBPath = tmp + "/content.db"
		return app, nil
	}
	openSQLiteStore = func(path string) (*contentstore.SQLiteStore, error) {
		store, err := contentstore.NewSQLiteStore(path)
		if err != nil {
			return nil, err
		}
		sg := graphmodel.ContentSubgraph{ID: "bf-global-v2", ArticleID: "bf-global-v2", SourcePlatform: "twitter", SourceExternalID: "bf-global-v2", CompileVersion: graphmodel.CompileBridgeVersion, CompiledAt: time.Now().UTC().Format(time.RFC3339), UpdatedAt: time.Now().UTC().Format(time.RFC3339), Nodes: []graphmodel.GraphNode{{ID: "n1", SourceArticleID: "bf-global-v2", SourcePlatform: "twitter", SourceExternalID: "bf-global-v2", RawText: "美联储加息0.25%", SubjectText: "美联储", ChangeText: "加息0.25%", Kind: graphmodel.NodeKindObservation, GraphRole: graphmodel.GraphRoleDriver, IsPrimary: true, VerificationStatus: graphmodel.VerificationPending, TimeBucket: "1w"}, {ID: "n2", SourceArticleID: "bf-global-v2", SourcePlatform: "twitter", SourceExternalID: "bf-global-v2", RawText: "未来一周美股承压", SubjectText: "美股", ChangeText: "承压", Kind: graphmodel.NodeKindPrediction, GraphRole: graphmodel.GraphRoleTarget, IsPrimary: true, VerificationStatus: graphmodel.VerificationProved, TimeBucket: "1w"}}}
		if err := store.PersistMemoryContentGraph(context.Background(), "u-backfill-global-v2", sg, time.Now().UTC()); err != nil {
			return nil, err
		}
		return store, nil
	}
	var stdout, stderr bytes.Buffer
	code := run([]string{"memory", "backfill", "--layer", "global-v2", "--user", "u-backfill-global-v2"}, "/tmp/project", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("memory backfill global-v2 code = %d, stderr = %s", code, stderr.String())
	}
	var out map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &out); err != nil {
		t.Fatalf("json.Unmarshal(backfill global-v2) error = %v", err)
	}
	if out["layer"] != "global-v2" || out["global_v2"] == nil {
		t.Fatalf("backfill global-v2 output = %#v, want layer=global-v2 and global_v2 key", out)
	}
}

func TestRunMemoryEventGraphsCombinesScopeAndSubjectFilters(t *testing.T) {
	prevBuildApp := buildApp
	prevOpenSQLiteStore := openSQLiteStore
	t.Cleanup(func() {
		buildApp = prevBuildApp
		openSQLiteStore = prevOpenSQLiteStore
	})

	tmp := t.TempDir()
	buildApp = func(projectRoot string) (*bootstrap.App, error) {
		app := &bootstrap.App{}
		app.Settings.ContentDBPath = tmp + "/content.db"
		return app, nil
	}
	openSQLiteStore = func(path string) (*contentstore.SQLiteStore, error) {
		store, err := contentstore.NewSQLiteStore(path)
		if err != nil {
			return nil, err
		}
		sg := graphmodel.ContentSubgraph{ID: "combo-eg", ArticleID: "combo-eg", SourcePlatform: "twitter", SourceExternalID: "combo-eg", CompileVersion: graphmodel.CompileBridgeVersion, CompiledAt: time.Now().UTC().Format(time.RFC3339), UpdatedAt: time.Now().UTC().Format(time.RFC3339), Nodes: []graphmodel.GraphNode{{ID: "n1", SourceArticleID: "combo-eg", SourcePlatform: "twitter", SourceExternalID: "combo-eg", RawText: "美联储加息0.25%", SubjectText: "美联储", ChangeText: "加息0.25%", Kind: graphmodel.NodeKindObservation, GraphRole: graphmodel.GraphRoleDriver, IsPrimary: true, VerificationStatus: graphmodel.VerificationPending, TimeBucket: "1w"}, {ID: "n2", SourceArticleID: "combo-eg", SourcePlatform: "twitter", SourceExternalID: "combo-eg", RawText: "未来一周美股承压", SubjectText: "美股", ChangeText: "承压", Kind: graphmodel.NodeKindPrediction, GraphRole: graphmodel.GraphRoleTarget, IsPrimary: true, VerificationStatus: graphmodel.VerificationProved, TimeBucket: "1w"}}}
		if err := store.PersistMemoryContentGraph(context.Background(), "u-combo-eg", sg, time.Now().UTC()); err != nil {
			return nil, err
		}
		return store, nil
	}
	var stdout, stderr bytes.Buffer
	code := run([]string{"memory", "event-graphs", "--user", "u-combo-eg", "--scope", "driver", "--subject", "美股"}, "/tmp/project", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("event-graphs combined filter code = %d, stderr = %s", code, stderr.String())
	}
	var out []contentstore.EventGraphRecord
	if err := json.Unmarshal(stdout.Bytes(), &out); err != nil {
		t.Fatalf("json.Unmarshal(event-graphs combined) error = %v", err)
	}
	if len(out) != 0 {
		t.Fatalf("event-graphs combined output = %#v, want empty intersection", out)
	}
}

func TestRunMemoryContentGraphsCombinesSourceAndSubjectFilters(t *testing.T) {
	prevBuildApp := buildApp
	prevOpenSQLiteStore := openSQLiteStore
	t.Cleanup(func() {
		buildApp = prevBuildApp
		openSQLiteStore = prevOpenSQLiteStore
	})

	tmp := t.TempDir()
	buildApp = func(projectRoot string) (*bootstrap.App, error) {
		app := &bootstrap.App{}
		app.Settings.ContentDBPath = tmp + "/content.db"
		return app, nil
	}
	openSQLiteStore = func(path string) (*contentstore.SQLiteStore, error) {
		store, err := contentstore.NewSQLiteStore(path)
		if err != nil {
			return nil, err
		}
		now := time.Now().UTC()
		for _, sg := range []graphmodel.ContentSubgraph{
			{ID: "combo-cg-1", ArticleID: "combo-cg-1", SourcePlatform: "twitter", SourceExternalID: "combo-cg-1", CompileVersion: graphmodel.CompileBridgeVersion, CompiledAt: now.Format(time.RFC3339), UpdatedAt: now.Format(time.RFC3339), Nodes: []graphmodel.GraphNode{{ID: "n1", SourceArticleID: "combo-cg-1", SourcePlatform: "twitter", SourceExternalID: "combo-cg-1", RawText: "美联储加息0.25%", SubjectText: "美联储", ChangeText: "加息0.25%", Kind: graphmodel.NodeKindObservation, IsPrimary: true, VerificationStatus: graphmodel.VerificationPending}}},
			{ID: "combo-cg-2", ArticleID: "combo-cg-2", SourcePlatform: "twitter", SourceExternalID: "combo-cg-2", CompileVersion: graphmodel.CompileBridgeVersion, CompiledAt: now.Format(time.RFC3339), UpdatedAt: now.Format(time.RFC3339), Nodes: []graphmodel.GraphNode{{ID: "n1", SourceArticleID: "combo-cg-2", SourcePlatform: "twitter", SourceExternalID: "combo-cg-2", RawText: "欧洲央行放缓缩表", SubjectText: "欧洲央行", ChangeText: "放缓缩表", Kind: graphmodel.NodeKindObservation, IsPrimary: true, VerificationStatus: graphmodel.VerificationPending}}},
		} {
			if err := store.PersistMemoryContentGraph(context.Background(), "u-combo-cg", sg, now); err != nil {
				return nil, err
			}
		}
		return store, nil
	}
	var stdout, stderr bytes.Buffer
	code := run([]string{"memory", "content-graphs", "--user", "u-combo-cg", "--platform", "twitter", "--id", "combo-cg-2", "--subject", "美联储"}, "/tmp/project", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("content-graphs combined filter code = %d, stderr = %s", code, stderr.String())
	}
	var out []graphmodel.ContentSubgraph
	if err := json.Unmarshal(stdout.Bytes(), &out); err != nil {
		t.Fatalf("json.Unmarshal(content-graphs combined) error = %v", err)
	}
	if len(out) != 0 {
		t.Fatalf("content-graphs combined output = %#v, want empty intersection", out)
	}
}

func TestRunMemoryEventGraphsCardSupportsSubjectFilter(t *testing.T) {
	prevBuildApp := buildApp
	prevOpenSQLiteStore := openSQLiteStore
	t.Cleanup(func() {
		buildApp = prevBuildApp
		openSQLiteStore = prevOpenSQLiteStore
	})

	tmp := t.TempDir()
	buildApp = func(projectRoot string) (*bootstrap.App, error) {
		app := &bootstrap.App{}
		app.Settings.ContentDBPath = tmp + "/content.db"
		return app, nil
	}
	openSQLiteStore = func(path string) (*contentstore.SQLiteStore, error) {
		store, err := contentstore.NewSQLiteStore(path)
		if err != nil {
			return nil, err
		}
		sg := graphmodel.ContentSubgraph{ID: "card-subject-eg", ArticleID: "card-subject-eg", SourcePlatform: "twitter", SourceExternalID: "card-subject-eg", CompileVersion: graphmodel.CompileBridgeVersion, CompiledAt: time.Now().UTC().Format(time.RFC3339), UpdatedAt: time.Now().UTC().Format(time.RFC3339), Nodes: []graphmodel.GraphNode{{ID: "n1", SourceArticleID: "card-subject-eg", SourcePlatform: "twitter", SourceExternalID: "card-subject-eg", RawText: "美联储加息0.25%", SubjectText: "美联储", ChangeText: "加息0.25%", Kind: graphmodel.NodeKindObservation, GraphRole: graphmodel.GraphRoleDriver, IsPrimary: true, VerificationStatus: graphmodel.VerificationPending, TimeBucket: "1w"}, {ID: "n2", SourceArticleID: "card-subject-eg", SourcePlatform: "twitter", SourceExternalID: "card-subject-eg", RawText: "未来一周美股承压", SubjectText: "美股", ChangeText: "承压", Kind: graphmodel.NodeKindPrediction, GraphRole: graphmodel.GraphRoleTarget, IsPrimary: true, VerificationStatus: graphmodel.VerificationProved, TimeBucket: "1w"}}}
		if err := store.PersistMemoryContentGraph(context.Background(), "u-event-card-subject", sg, time.Now().UTC()); err != nil {
			return nil, err
		}
		return store, nil
	}
	var stdout, stderr bytes.Buffer
	code := run([]string{"memory", "event-graphs", "--card", "--user", "u-event-card-subject", "--subject", "美联储"}, "/tmp/project", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("event-graphs --card --subject code = %d, stderr = %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "美联储") || strings.Contains(stdout.String(), "欧洲央行") {
		t.Fatalf("stdout = %q, want filtered event card output", stdout.String())
	}
}

func TestRunMemoryEventGraphsSupportsAliasSubjectFilter(t *testing.T) {
	prevBuildApp := buildApp
	prevOpenSQLiteStore := openSQLiteStore
	t.Cleanup(func() {
		buildApp = prevBuildApp
		openSQLiteStore = prevOpenSQLiteStore
	})

	tmp := t.TempDir()
	buildApp = func(projectRoot string) (*bootstrap.App, error) {
		app := &bootstrap.App{}
		app.Settings.ContentDBPath = tmp + "/content.db"
		return app, nil
	}
	openSQLiteStore = func(path string) (*contentstore.SQLiteStore, error) {
		store, err := contentstore.NewSQLiteStore(path)
		if err != nil {
			return nil, err
		}
		now := time.Now().UTC()
		if err := store.UpsertCanonicalEntity(context.Background(), memory.CanonicalEntity{
			EntityID:      "driver-fed",
			EntityType:    memory.CanonicalEntityDriver,
			CanonicalName: "美联储",
			Aliases:       []string{"联储"},
			Status:        memory.CanonicalEntityActive,
			CreatedAt:     now,
			UpdatedAt:     now,
		}); err != nil {
			return nil, err
		}
		sg := graphmodel.ContentSubgraph{ID: "alias-eg", ArticleID: "alias-eg", SourcePlatform: "twitter", SourceExternalID: "alias-eg", CompileVersion: graphmodel.CompileBridgeVersion, CompiledAt: now.Format(time.RFC3339), UpdatedAt: now.Format(time.RFC3339), Nodes: []graphmodel.GraphNode{{ID: "n1", SourceArticleID: "alias-eg", SourcePlatform: "twitter", SourceExternalID: "alias-eg", RawText: "联储加息0.25%", SubjectText: "联储", ChangeText: "加息0.25%", Kind: graphmodel.NodeKindObservation, GraphRole: graphmodel.GraphRoleDriver, IsPrimary: true, VerificationStatus: graphmodel.VerificationPending, TimeBucket: "1w"}}}
		if err := store.PersistMemoryContentGraph(context.Background(), "u-event-alias-filter", sg, now); err != nil {
			return nil, err
		}
		return store, nil
	}
	var stdout, stderr bytes.Buffer
	code := run([]string{"memory", "event-graphs", "--user", "u-event-alias-filter", "--subject", "联储"}, "/tmp/project", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("event-graphs alias subject code = %d, stderr = %s", code, stderr.String())
	}
	var out []contentstore.EventGraphRecord
	if err := json.Unmarshal(stdout.Bytes(), &out); err != nil {
		t.Fatalf("json.Unmarshal(event-graphs alias) error = %v", err)
	}
	if len(out) != 1 || out[0].AnchorSubject != "美联储" {
		t.Fatalf("out = %#v, want alias lookup to return canonical 美联储 event graph", out)
	}
}

func TestRunMemoryContentGraphsCardSupportsSubjectFilter(t *testing.T) {
	prevBuildApp := buildApp
	prevOpenSQLiteStore := openSQLiteStore
	t.Cleanup(func() {
		buildApp = prevBuildApp
		openSQLiteStore = prevOpenSQLiteStore
	})

	tmp := t.TempDir()
	buildApp = func(projectRoot string) (*bootstrap.App, error) {
		app := &bootstrap.App{}
		app.Settings.ContentDBPath = tmp + "/content.db"
		return app, nil
	}
	openSQLiteStore = func(path string) (*contentstore.SQLiteStore, error) {
		store, err := contentstore.NewSQLiteStore(path)
		if err != nil {
			return nil, err
		}
		now := time.Now().UTC()
		for _, sg := range []graphmodel.ContentSubgraph{
			{ID: "card-subject-cg-1", ArticleID: "card-subject-cg-1", SourcePlatform: "twitter", SourceExternalID: "card-subject-cg-1", CompileVersion: graphmodel.CompileBridgeVersion, CompiledAt: now.Format(time.RFC3339), UpdatedAt: now.Format(time.RFC3339), Nodes: []graphmodel.GraphNode{{ID: "n1", SourceArticleID: "card-subject-cg-1", SourcePlatform: "twitter", SourceExternalID: "card-subject-cg-1", RawText: "美联储加息0.25%", SubjectText: "美联储", ChangeText: "加息0.25%", Kind: graphmodel.NodeKindObservation, IsPrimary: true, VerificationStatus: graphmodel.VerificationPending}}},
			{ID: "card-subject-cg-2", ArticleID: "card-subject-cg-2", SourcePlatform: "twitter", SourceExternalID: "card-subject-cg-2", CompileVersion: graphmodel.CompileBridgeVersion, CompiledAt: now.Format(time.RFC3339), UpdatedAt: now.Format(time.RFC3339), Nodes: []graphmodel.GraphNode{{ID: "n1", SourceArticleID: "card-subject-cg-2", SourcePlatform: "twitter", SourceExternalID: "card-subject-cg-2", RawText: "欧洲央行放缓缩表", SubjectText: "欧洲央行", ChangeText: "放缓缩表", Kind: graphmodel.NodeKindObservation, IsPrimary: true, VerificationStatus: graphmodel.VerificationPending}}},
		} {
			if err := store.PersistMemoryContentGraph(context.Background(), "u-content-card-subject", sg, now); err != nil {
				return nil, err
			}
		}
		return store, nil
	}
	var stdout, stderr bytes.Buffer
	code := run([]string{"memory", "content-graphs", "--card", "--user", "u-content-card-subject", "--subject", "美联储"}, "/tmp/project", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("content-graphs --card --subject code = %d, stderr = %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "美联储") || strings.Contains(stdout.String(), "欧洲央行") {
		t.Fatalf("stdout = %q, want filtered content graph card output", stdout.String())
	}
}

func TestRunMemorySubjectTimelineRendersCard(t *testing.T) {
	prevBuildApp := buildApp
	prevOpenSQLiteStore := openSQLiteStore
	t.Cleanup(func() {
		buildApp = prevBuildApp
		openSQLiteStore = prevOpenSQLiteStore
	})

	tmp := t.TempDir()
	timelineNow := time.Date(2026, 4, 30, 0, 0, 0, 0, time.UTC)
	buildApp = func(projectRoot string) (*bootstrap.App, error) {
		app := &bootstrap.App{}
		app.Settings.ContentDBPath = tmp + "/content.db"
		return app, nil
	}
	openSQLiteStore = func(path string) (*contentstore.SQLiteStore, error) {
		store, err := contentstore.NewSQLiteStore(path)
		if err != nil {
			return nil, err
		}
		for _, sg := range []graphmodel.ContentSubgraph{
			{
				ID:               "timeline-cg-1",
				ArticleID:        "timeline-cg-1",
				SourcePlatform:   "twitter",
				SourceExternalID: "timeline-cg-1",
				CompileVersion:   graphmodel.CompileBridgeVersion,
				CompiledAt:       timelineNow.Format(time.RFC3339),
				UpdatedAt:        timelineNow.Format(time.RFC3339),
				Nodes: []graphmodel.GraphNode{{
					ID:                 "n1",
					SourceArticleID:    "timeline-cg-1",
					SourcePlatform:     "twitter",
					SourceExternalID:   "timeline-cg-1",
					RawText:            "美股承压",
					SubjectText:        "美股",
					ChangeText:         "承压",
					TimeStart:          timelineNow.Add(-24 * time.Hour).Format(time.RFC3339),
					Kind:               graphmodel.NodeKindObservation,
					GraphRole:          graphmodel.GraphRoleTarget,
					IsPrimary:          true,
					VerificationStatus: graphmodel.VerificationPending,
				}},
			},
			{
				ID:               "timeline-cg-2",
				ArticleID:        "timeline-cg-2",
				SourcePlatform:   "twitter",
				SourceExternalID: "timeline-cg-2",
				CompileVersion:   graphmodel.CompileBridgeVersion,
				CompiledAt:       timelineNow.Format(time.RFC3339),
				UpdatedAt:        timelineNow.Format(time.RFC3339),
				Nodes: []graphmodel.GraphNode{{
					ID:                 "n2",
					SourceArticleID:    "timeline-cg-2",
					SourcePlatform:     "twitter",
					SourceExternalID:   "timeline-cg-2",
					RawText:            "美股反弹",
					SubjectText:        "美股",
					ChangeText:         "反弹",
					Kind:               graphmodel.NodeKindObservation,
					GraphRole:          graphmodel.GraphRoleTarget,
					IsPrimary:          true,
					VerificationStatus: graphmodel.VerificationProved,
					VerificationReason: "observed rebound",
					VerificationAsOf:   timelineNow.Format(time.RFC3339),
				}},
			},
		} {
			if err := store.PersistMemoryContentGraph(context.Background(), "u-subject-timeline", sg, timelineNow); err != nil {
				return nil, err
			}
		}
		return store, nil
	}
	var stdout, stderr bytes.Buffer
	code := run([]string{"memory", "subject-timeline", "--card", "--user", "u-subject-timeline", "--subject", "美股"}, "/tmp/project", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("subject-timeline --card code = %d, stderr = %s", code, stderr.String())
	}
	got := stdout.String()
	for _, want := range []string{"Subject Timeline", "美股", "承压", "反弹", timelineNow.Format(time.RFC3339), "twitter:timeline-cg-2#n2", "proved (observed rebound)", "contradicts"} {
		if !strings.Contains(got, want) {
			t.Fatalf("stdout = %q, want substring %q", got, want)
		}
	}
}

func TestRunMemorySubjectHorizonRendersCard(t *testing.T) {
	prevBuildApp := buildApp
	prevOpenSQLiteStore := openSQLiteStore
	t.Cleanup(func() {
		buildApp = prevBuildApp
		openSQLiteStore = prevOpenSQLiteStore
	})

	tmp := t.TempDir()
	now := time.Date(2026, 4, 30, 0, 0, 0, 0, time.UTC)
	buildApp = func(projectRoot string) (*bootstrap.App, error) {
		app := &bootstrap.App{}
		app.Settings.ContentDBPath = tmp + "/content.db"
		return app, nil
	}
	openSQLiteStore = func(path string) (*contentstore.SQLiteStore, error) {
		store, err := contentstore.NewSQLiteStore(path)
		if err != nil {
			return nil, err
		}
		sg := graphmodel.ContentSubgraph{
			ID:               "horizon-card",
			ArticleID:        "horizon-card",
			SourcePlatform:   "twitter",
			SourceExternalID: "horizon-card",
			CompileVersion:   graphmodel.CompileBridgeVersion,
			CompiledAt:       now.Format(time.RFC3339),
			UpdatedAt:        now.Format(time.RFC3339),
			Nodes: []graphmodel.GraphNode{
				{ID: "d1", SourceArticleID: "horizon-card", SourcePlatform: "twitter", SourceExternalID: "horizon-card", RawText: "油价上涨", SubjectText: "油价", ChangeText: "上涨", TimeStart: now.Format(time.RFC3339), Kind: graphmodel.NodeKindObservation, GraphRole: graphmodel.GraphRoleDriver, IsPrimary: true, VerificationStatus: graphmodel.VerificationProved},
				{ID: "t1", SourceArticleID: "horizon-card", SourcePlatform: "twitter", SourceExternalID: "horizon-card", RawText: "美股回落", SubjectText: "美股", ChangeText: "回落", TimeStart: now.Format(time.RFC3339), Kind: graphmodel.NodeKindObservation, GraphRole: graphmodel.GraphRoleTarget, IsPrimary: true, VerificationStatus: graphmodel.VerificationProved},
			},
		}
		if err := store.PersistMemoryContentGraph(context.Background(), "u-subject-horizon", sg, now); err != nil {
			return nil, err
		}
		return store, nil
	}
	var stdout, stderr bytes.Buffer
	code := run([]string{"memory", "subject-horizon", "--card", "--refresh", "--user", "u-subject-horizon", "--subject", "美股", "--horizon", "1w"}, "/tmp/project", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("subject-horizon --card code = %d, stderr = %s", code, stderr.String())
	}
	for _, want := range []string{"Subject Horizon", "Horizon: 1w", "Policy: daily", "美股", "回落", "油价"} {
		if !strings.Contains(stdout.String(), want) {
			t.Fatalf("stdout = %q, want substring %q", stdout.String(), want)
		}
	}
}

func TestRunMemorySubjectExperienceRendersCard(t *testing.T) {
	prevBuildApp := buildApp
	prevOpenSQLiteStore := openSQLiteStore
	t.Cleanup(func() {
		buildApp = prevBuildApp
		openSQLiteStore = prevOpenSQLiteStore
	})

	tmp := t.TempDir()
	now := time.Date(2026, 4, 30, 0, 0, 0, 0, time.UTC)
	buildApp = func(projectRoot string) (*bootstrap.App, error) {
		app := &bootstrap.App{}
		app.Settings.ContentDBPath = tmp + "/content.db"
		return app, nil
	}
	openSQLiteStore = func(path string) (*contentstore.SQLiteStore, error) {
		store, err := contentstore.NewSQLiteStore(path)
		if err != nil {
			return nil, err
		}
		sg := graphmodel.ContentSubgraph{
			ID:               "experience-card",
			ArticleID:        "experience-card",
			SourcePlatform:   "twitter",
			SourceExternalID: "experience-card",
			CompileVersion:   graphmodel.CompileBridgeVersion,
			CompiledAt:       now.Format(time.RFC3339),
			UpdatedAt:        now.Format(time.RFC3339),
			Nodes: []graphmodel.GraphNode{
				{ID: "d1", SourceArticleID: "experience-card", SourcePlatform: "twitter", SourceExternalID: "experience-card", RawText: "油价上涨", SubjectText: "油价", ChangeText: "上涨", TimeStart: now.Format(time.RFC3339), Kind: graphmodel.NodeKindObservation, GraphRole: graphmodel.GraphRoleDriver, IsPrimary: true, VerificationStatus: graphmodel.VerificationProved},
				{ID: "t1", SourceArticleID: "experience-card", SourcePlatform: "twitter", SourceExternalID: "experience-card", RawText: "美股回落", SubjectText: "美股", ChangeText: "回落", TimeStart: now.Format(time.RFC3339), Kind: graphmodel.NodeKindObservation, GraphRole: graphmodel.GraphRoleTarget, IsPrimary: true, VerificationStatus: graphmodel.VerificationProved},
			},
		}
		if err := store.PersistMemoryContentGraph(context.Background(), "u-subject-experience", sg, now); err != nil {
			return nil, err
		}
		return store, nil
	}
	var stdout, stderr bytes.Buffer
	code := run([]string{"memory", "subject-experience", "--card", "--refresh", "--user", "u-subject-experience", "--subject", "美股", "--horizons", "1w,1m"}, "/tmp/project", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("subject-experience --card code = %d, stderr = %s", code, stderr.String())
	}
	for _, want := range []string{"主体归因总结", "观察窗口: 最近 1w, 最近 1m", "变化数", "因素数", "归因总结", "主要因素", "变化归因", "因素关系", "油价"} {
		if !strings.Contains(stdout.String(), want) {
			t.Fatalf("stdout = %q, want substring %q", stdout.String(), want)
		}
	}
	for _, notWant := range []string{"branch driver", "driver-pattern", "Drivers:", "Key factors:", "Mechanism:", "Transfer:", "Horizons:"} {
		if strings.Contains(stdout.String(), notWant) {
			t.Fatalf("stdout = %q, should not expose internal term %q", stdout.String(), notWant)
		}
	}
	for _, notWant := range []string{"使用方式", "时间尺度含义", "支撑变化"} {
		if strings.Contains(stdout.String(), notWant) {
			t.Fatalf("stdout = %q, should avoid verbose phrase %q", stdout.String(), notWant)
		}
	}
	for _, notWant := range []string{"中间机制未展开", "下次先找"} {
		if strings.Contains(stdout.String(), notWant) {
			t.Fatalf("stdout = %q, should summarize attribution instead of warnings like %q", stdout.String(), notWant)
		}
	}
	if strings.Contains(stdout.String(), "暂不判断因果先后") {
		t.Fatalf("stdout = %q, should avoid awkward relation caveat", stdout.String())
	}
	for _, notWant := range []string{"长期", "中长期", "短期", "时间尺度提示"} {
		if strings.Contains(stdout.String(), notWant) {
			t.Fatalf("stdout = %q, should use recent-window phrasing instead of %q", stdout.String(), notWant)
		}
	}
}

func TestRunMemoryContentGraphsSupportsAliasSubjectFilter(t *testing.T) {
	prevBuildApp := buildApp
	prevOpenSQLiteStore := openSQLiteStore
	t.Cleanup(func() {
		buildApp = prevBuildApp
		openSQLiteStore = prevOpenSQLiteStore
	})

	tmp := t.TempDir()
	buildApp = func(projectRoot string) (*bootstrap.App, error) {
		app := &bootstrap.App{}
		app.Settings.ContentDBPath = tmp + "/content.db"
		return app, nil
	}
	openSQLiteStore = func(path string) (*contentstore.SQLiteStore, error) {
		store, err := contentstore.NewSQLiteStore(path)
		if err != nil {
			return nil, err
		}
		now := time.Now().UTC()
		if err := store.UpsertCanonicalEntity(context.Background(), memory.CanonicalEntity{
			EntityID:      "driver-fed",
			EntityType:    memory.CanonicalEntityDriver,
			CanonicalName: "美联储",
			Aliases:       []string{"联储"},
			Status:        memory.CanonicalEntityActive,
			CreatedAt:     now,
			UpdatedAt:     now,
		}); err != nil {
			return nil, err
		}
		sg := graphmodel.ContentSubgraph{ID: "alias-cg", ArticleID: "alias-cg", SourcePlatform: "twitter", SourceExternalID: "alias-cg", CompileVersion: graphmodel.CompileBridgeVersion, CompiledAt: now.Format(time.RFC3339), UpdatedAt: now.Format(time.RFC3339), Nodes: []graphmodel.GraphNode{{ID: "n1", SourceArticleID: "alias-cg", SourcePlatform: "twitter", SourceExternalID: "alias-cg", RawText: "联储加息0.25%", SubjectText: "联储", ChangeText: "加息0.25%", SubjectCanonical: "美联储", Kind: graphmodel.NodeKindObservation, IsPrimary: true, VerificationStatus: graphmodel.VerificationPending}}}
		if err := store.PersistMemoryContentGraph(context.Background(), "u-content-alias-filter", sg, now); err != nil {
			return nil, err
		}
		return store, nil
	}
	var stdout, stderr bytes.Buffer
	code := run([]string{"memory", "content-graphs", "--user", "u-content-alias-filter", "--subject", "联储"}, "/tmp/project", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("content-graphs alias subject code = %d, stderr = %s", code, stderr.String())
	}
	var out []graphmodel.ContentSubgraph
	if err := json.Unmarshal(stdout.Bytes(), &out); err != nil {
		t.Fatalf("json.Unmarshal(content-graphs alias) error = %v", err)
	}
	if len(out) != 1 {
		t.Fatalf("out = %#v, want one content graph for alias lookup", out)
	}
}

func TestRunMemoryContentGraphsResolvesAliasToCanonicalSubjectFilter(t *testing.T) {
	prevBuildApp := buildApp
	prevOpenSQLiteStore := openSQLiteStore
	t.Cleanup(func() {
		buildApp = prevBuildApp
		openSQLiteStore = prevOpenSQLiteStore
	})

	tmp := t.TempDir()
	buildApp = func(projectRoot string) (*bootstrap.App, error) {
		app := &bootstrap.App{}
		app.Settings.ContentDBPath = tmp + "/content.db"
		return app, nil
	}
	openSQLiteStore = func(path string) (*contentstore.SQLiteStore, error) {
		store, err := contentstore.NewSQLiteStore(path)
		if err != nil {
			return nil, err
		}
		now := time.Now().UTC()
		if err := store.UpsertCanonicalEntity(context.Background(), memory.CanonicalEntity{
			EntityID:      "driver-fed",
			EntityType:    memory.CanonicalEntityDriver,
			CanonicalName: "美联储",
			Aliases:       []string{"联储"},
			Status:        memory.CanonicalEntityActive,
			CreatedAt:     now,
			UpdatedAt:     now,
		}); err != nil {
			return nil, err
		}
		sg := graphmodel.ContentSubgraph{ID: "canonical-cg", ArticleID: "canonical-cg", SourcePlatform: "twitter", SourceExternalID: "canonical-cg", CompileVersion: graphmodel.CompileBridgeVersion, CompiledAt: now.Format(time.RFC3339), UpdatedAt: now.Format(time.RFC3339), Nodes: []graphmodel.GraphNode{{ID: "n1", SourceArticleID: "canonical-cg", SourcePlatform: "twitter", SourceExternalID: "canonical-cg", RawText: "美联储加息0.25%", SubjectText: "美联储", ChangeText: "加息0.25%", Kind: graphmodel.NodeKindObservation, IsPrimary: true, VerificationStatus: graphmodel.VerificationPending}}}
		if err := store.PersistMemoryContentGraph(context.Background(), "u-content-canonical-filter", sg, now); err != nil {
			return nil, err
		}
		return store, nil
	}
	var stdout, stderr bytes.Buffer
	code := run([]string{"memory", "content-graphs", "--user", "u-content-canonical-filter", "--subject", "联储"}, "/tmp/project", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("content-graphs canonical alias filter code = %d, stderr = %s", code, stderr.String())
	}
	var out []graphmodel.ContentSubgraph
	if err := json.Unmarshal(stdout.Bytes(), &out); err != nil {
		t.Fatalf("json.Unmarshal(content-graphs canonical alias) error = %v", err)
	}
	if len(out) != 1 {
		t.Fatalf("out = %#v, want one content graph for canonical alias lookup", out)
	}
	if out[0].Nodes[0].SubjectText != "美联储" {
		t.Fatalf("out = %#v, want canonical subject presentation in returned payload", out)
	}
}

func TestRunMemoryContentGraphsSupportsSourceAndAliasSubjectFilter(t *testing.T) {
	prevBuildApp := buildApp
	prevOpenSQLiteStore := openSQLiteStore
	t.Cleanup(func() {
		buildApp = prevBuildApp
		openSQLiteStore = prevOpenSQLiteStore
	})

	tmp := t.TempDir()
	buildApp = func(projectRoot string) (*bootstrap.App, error) {
		app := &bootstrap.App{}
		app.Settings.ContentDBPath = tmp + "/content.db"
		return app, nil
	}
	openSQLiteStore = func(path string) (*contentstore.SQLiteStore, error) {
		store, err := contentstore.NewSQLiteStore(path)
		if err != nil {
			return nil, err
		}
		now := time.Now().UTC()
		if err := store.UpsertCanonicalEntity(context.Background(), memory.CanonicalEntity{
			EntityID:      "driver-fed",
			EntityType:    memory.CanonicalEntityDriver,
			CanonicalName: "美联储",
			Aliases:       []string{"联储"},
			Status:        memory.CanonicalEntityActive,
			CreatedAt:     now,
			UpdatedAt:     now,
		}); err != nil {
			return nil, err
		}
		for _, sg := range []graphmodel.ContentSubgraph{
			{ID: "source-alias-cg-1", ArticleID: "source-alias-cg-1", SourcePlatform: "twitter", SourceExternalID: "source-alias-cg-1", CompileVersion: graphmodel.CompileBridgeVersion, CompiledAt: now.Format(time.RFC3339), UpdatedAt: now.Format(time.RFC3339), Nodes: []graphmodel.GraphNode{{ID: "n1", SourceArticleID: "source-alias-cg-1", SourcePlatform: "twitter", SourceExternalID: "source-alias-cg-1", RawText: "美联储加息0.25%", SubjectText: "美联储", ChangeText: "加息0.25%", Kind: graphmodel.NodeKindObservation, IsPrimary: true, VerificationStatus: graphmodel.VerificationPending}}},
			{ID: "source-alias-cg-2", ArticleID: "source-alias-cg-2", SourcePlatform: "twitter", SourceExternalID: "source-alias-cg-2", CompileVersion: graphmodel.CompileBridgeVersion, CompiledAt: now.Format(time.RFC3339), UpdatedAt: now.Format(time.RFC3339), Nodes: []graphmodel.GraphNode{{ID: "n1", SourceArticleID: "source-alias-cg-2", SourcePlatform: "twitter", SourceExternalID: "source-alias-cg-2", RawText: "欧洲央行放缓缩表", SubjectText: "欧洲央行", ChangeText: "放缓缩表", Kind: graphmodel.NodeKindObservation, IsPrimary: true, VerificationStatus: graphmodel.VerificationPending}}},
		} {
			if err := store.PersistMemoryContentGraph(context.Background(), "u-content-source-alias-filter", sg, now); err != nil {
				return nil, err
			}
		}
		return store, nil
	}
	var stdout, stderr bytes.Buffer
	code := run([]string{"memory", "content-graphs", "--user", "u-content-source-alias-filter", "--platform", "twitter", "--id", "source-alias-cg-1", "--subject", "联储"}, "/tmp/project", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("content-graphs source+alias subject code = %d, stderr = %s", code, stderr.String())
	}
	var out []graphmodel.ContentSubgraph
	if err := json.Unmarshal(stdout.Bytes(), &out); err != nil {
		t.Fatalf("json.Unmarshal(content-graphs source+alias) error = %v", err)
	}
	if len(out) != 1 || out[0].SourceExternalID != "source-alias-cg-1" {
		t.Fatalf("out = %#v, want one source-filtered alias match", out)
	}
}

func TestRunMemoryParadigmsCardSupportsSubjectFilter(t *testing.T) {
	prevBuildApp := buildApp
	prevOpenSQLiteStore := openSQLiteStore
	t.Cleanup(func() {
		buildApp = prevBuildApp
		openSQLiteStore = prevOpenSQLiteStore
	})

	tmp := t.TempDir()
	buildApp = func(projectRoot string) (*bootstrap.App, error) {
		app := &bootstrap.App{}
		app.Settings.ContentDBPath = tmp + "/content.db"
		return app, nil
	}
	openSQLiteStore = func(path string) (*contentstore.SQLiteStore, error) {
		store, err := contentstore.NewSQLiteStore(path)
		if err != nil {
			return nil, err
		}
		now := time.Now().UTC()
		for _, sg := range []graphmodel.ContentSubgraph{
			{ID: "card-subject-pg-1", ArticleID: "card-subject-pg-1", SourcePlatform: "twitter", SourceExternalID: "card-subject-pg-1", CompileVersion: graphmodel.CompileBridgeVersion, CompiledAt: now.Format(time.RFC3339), UpdatedAt: now.Format(time.RFC3339), Nodes: []graphmodel.GraphNode{{ID: "n1", SourceArticleID: "card-subject-pg-1", SourcePlatform: "twitter", SourceExternalID: "card-subject-pg-1", RawText: "美联储加息0.25%", SubjectText: "美联储", ChangeText: "加息0.25%", Kind: graphmodel.NodeKindObservation, GraphRole: graphmodel.GraphRoleDriver, IsPrimary: true, VerificationStatus: graphmodel.VerificationPending, TimeBucket: "1w"}, {ID: "n2", SourceArticleID: "card-subject-pg-1", SourcePlatform: "twitter", SourceExternalID: "card-subject-pg-1", RawText: "未来一周美股承压", SubjectText: "美股", ChangeText: "承压", Kind: graphmodel.NodeKindPrediction, GraphRole: graphmodel.GraphRoleTarget, IsPrimary: true, VerificationStatus: graphmodel.VerificationProved, TimeBucket: "1w"}}},
			{ID: "card-subject-pg-2", ArticleID: "card-subject-pg-2", SourcePlatform: "twitter", SourceExternalID: "card-subject-pg-2", CompileVersion: graphmodel.CompileBridgeVersion, CompiledAt: now.Format(time.RFC3339), UpdatedAt: now.Format(time.RFC3339), Nodes: []graphmodel.GraphNode{{ID: "n1", SourceArticleID: "card-subject-pg-2", SourcePlatform: "twitter", SourceExternalID: "card-subject-pg-2", RawText: "欧洲央行放缓缩表", SubjectText: "欧洲央行", ChangeText: "放缓缩表", Kind: graphmodel.NodeKindObservation, GraphRole: graphmodel.GraphRoleDriver, IsPrimary: true, VerificationStatus: graphmodel.VerificationPending, TimeBucket: "1w"}, {ID: "n2", SourceArticleID: "card-subject-pg-2", SourcePlatform: "twitter", SourceExternalID: "card-subject-pg-2", RawText: "欧股承压", SubjectText: "欧股", ChangeText: "承压", Kind: graphmodel.NodeKindPrediction, GraphRole: graphmodel.GraphRoleTarget, IsPrimary: true, VerificationStatus: graphmodel.VerificationProved, TimeBucket: "1w"}}},
		} {
			if err := store.PersistMemoryContentGraph(context.Background(), "u-paradigm-card-subject", sg, now); err != nil {
				return nil, err
			}
		}
		return store, nil
	}
	var stdout, stderr bytes.Buffer
	code := run([]string{"memory", "paradigms", "--card", "--user", "u-paradigm-card-subject", "--subject", "美联储"}, "/tmp/project", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("paradigms --card --subject code = %d, stderr = %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "美联储") || strings.Contains(stdout.String(), "欧洲央行") {
		t.Fatalf("stdout = %q, want filtered paradigm card output", stdout.String())
	}
}

func TestRunMemoryParadigmsSupportsAliasSubjectFilter(t *testing.T) {
	prevBuildApp := buildApp
	prevOpenSQLiteStore := openSQLiteStore
	t.Cleanup(func() {
		buildApp = prevBuildApp
		openSQLiteStore = prevOpenSQLiteStore
	})

	tmp := t.TempDir()
	buildApp = func(projectRoot string) (*bootstrap.App, error) {
		app := &bootstrap.App{}
		app.Settings.ContentDBPath = tmp + "/content.db"
		return app, nil
	}
	openSQLiteStore = func(path string) (*contentstore.SQLiteStore, error) {
		store, err := contentstore.NewSQLiteStore(path)
		if err != nil {
			return nil, err
		}
		now := time.Now().UTC()
		if err := store.UpsertCanonicalEntity(context.Background(), memory.CanonicalEntity{
			EntityID:      "driver-fed",
			EntityType:    memory.CanonicalEntityDriver,
			CanonicalName: "美联储",
			Aliases:       []string{"联储"},
			Status:        memory.CanonicalEntityActive,
			CreatedAt:     now,
			UpdatedAt:     now,
		}); err != nil {
			return nil, err
		}
		sg := graphmodel.ContentSubgraph{ID: "alias-pg", ArticleID: "alias-pg", SourcePlatform: "twitter", SourceExternalID: "alias-pg", CompileVersion: graphmodel.CompileBridgeVersion, CompiledAt: now.Format(time.RFC3339), UpdatedAt: now.Format(time.RFC3339), Nodes: []graphmodel.GraphNode{{ID: "n1", SourceArticleID: "alias-pg", SourcePlatform: "twitter", SourceExternalID: "alias-pg", RawText: "联储加息0.25%", SubjectText: "联储", ChangeText: "加息0.25%", Kind: graphmodel.NodeKindObservation, GraphRole: graphmodel.GraphRoleDriver, IsPrimary: true, VerificationStatus: graphmodel.VerificationPending, TimeBucket: "1w"}, {ID: "n2", SourceArticleID: "alias-pg", SourcePlatform: "twitter", SourceExternalID: "alias-pg", RawText: "未来一周美股承压", SubjectText: "美股", ChangeText: "承压", Kind: graphmodel.NodeKindPrediction, GraphRole: graphmodel.GraphRoleTarget, IsPrimary: true, VerificationStatus: graphmodel.VerificationProved, TimeBucket: "1w"}}}
		if err := store.PersistMemoryContentGraph(context.Background(), "u-paradigm-alias-filter", sg, now); err != nil {
			return nil, err
		}
		return store, nil
	}
	var stdout, stderr bytes.Buffer
	code := run([]string{"memory", "paradigms", "--user", "u-paradigm-alias-filter", "--subject", "联储"}, "/tmp/project", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("paradigms alias subject code = %d, stderr = %s", code, stderr.String())
	}
	var out []contentstore.ParadigmRecord
	if err := json.Unmarshal(stdout.Bytes(), &out); err != nil {
		t.Fatalf("json.Unmarshal(paradigms alias) error = %v", err)
	}
	if len(out) != 1 || out[0].DriverSubject != "美联储" {
		t.Fatalf("out = %#v, want alias lookup to return canonical 美联储 paradigm", out)
	}
}

func TestRunMemoryEventGraphsCardSupportsScopeFilter(t *testing.T) {
	prevBuildApp := buildApp
	prevOpenSQLiteStore := openSQLiteStore
	t.Cleanup(func() {
		buildApp = prevBuildApp
		openSQLiteStore = prevOpenSQLiteStore
	})

	tmp := t.TempDir()
	buildApp = func(projectRoot string) (*bootstrap.App, error) {
		app := &bootstrap.App{}
		app.Settings.ContentDBPath = tmp + "/content.db"
		return app, nil
	}
	openSQLiteStore = func(path string) (*contentstore.SQLiteStore, error) {
		store, err := contentstore.NewSQLiteStore(path)
		if err != nil {
			return nil, err
		}
		sg := graphmodel.ContentSubgraph{ID: "card-scope-eg", ArticleID: "card-scope-eg", SourcePlatform: "twitter", SourceExternalID: "card-scope-eg", CompileVersion: graphmodel.CompileBridgeVersion, CompiledAt: time.Now().UTC().Format(time.RFC3339), UpdatedAt: time.Now().UTC().Format(time.RFC3339), Nodes: []graphmodel.GraphNode{{ID: "n1", SourceArticleID: "card-scope-eg", SourcePlatform: "twitter", SourceExternalID: "card-scope-eg", RawText: "美联储加息0.25%", SubjectText: "美联储", ChangeText: "加息0.25%", Kind: graphmodel.NodeKindObservation, GraphRole: graphmodel.GraphRoleDriver, IsPrimary: true, VerificationStatus: graphmodel.VerificationPending, TimeBucket: "1w"}, {ID: "n2", SourceArticleID: "card-scope-eg", SourcePlatform: "twitter", SourceExternalID: "card-scope-eg", RawText: "未来一周美股承压", SubjectText: "美股", ChangeText: "承压", Kind: graphmodel.NodeKindPrediction, GraphRole: graphmodel.GraphRoleTarget, IsPrimary: true, VerificationStatus: graphmodel.VerificationProved, TimeBucket: "1w"}}}
		if err := store.PersistMemoryContentGraph(context.Background(), "u-event-card-scope", sg, time.Now().UTC()); err != nil {
			return nil, err
		}
		return store, nil
	}
	var stdout, stderr bytes.Buffer
	code := run([]string{"memory", "event-graphs", "--card", "--user", "u-event-card-scope", "--scope", "driver"}, "/tmp/project", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("event-graphs --card --scope code = %d, stderr = %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "Scope: driver") || strings.Contains(stdout.String(), "Scope: target") {
		t.Fatalf("stdout = %q, want only driver card output", stdout.String())
	}
}

func TestRunMemoryEventGraphsCardShowsNoMatchMessageForEmptyFilter(t *testing.T) {
	prevBuildApp := buildApp
	prevOpenSQLiteStore := openSQLiteStore
	t.Cleanup(func() {
		buildApp = prevBuildApp
		openSQLiteStore = prevOpenSQLiteStore
	})

	tmp := t.TempDir()
	buildApp = func(projectRoot string) (*bootstrap.App, error) {
		app := &bootstrap.App{}
		app.Settings.ContentDBPath = tmp + "/content.db"
		return app, nil
	}
	openSQLiteStore = func(path string) (*contentstore.SQLiteStore, error) {
		store, err := contentstore.NewSQLiteStore(path)
		if err != nil {
			return nil, err
		}
		sg := graphmodel.ContentSubgraph{ID: "empty-eg", ArticleID: "empty-eg", SourcePlatform: "twitter", SourceExternalID: "empty-eg", CompileVersion: graphmodel.CompileBridgeVersion, CompiledAt: time.Now().UTC().Format(time.RFC3339), UpdatedAt: time.Now().UTC().Format(time.RFC3339), Nodes: []graphmodel.GraphNode{{ID: "n1", SourceArticleID: "empty-eg", SourcePlatform: "twitter", SourceExternalID: "empty-eg", RawText: "美联储加息0.25%", SubjectText: "美联储", ChangeText: "加息0.25%", Kind: graphmodel.NodeKindObservation, GraphRole: graphmodel.GraphRoleDriver, IsPrimary: true, VerificationStatus: graphmodel.VerificationPending, TimeBucket: "1w"}, {ID: "n2", SourceArticleID: "empty-eg", SourcePlatform: "twitter", SourceExternalID: "empty-eg", RawText: "未来一周美股承压", SubjectText: "美股", ChangeText: "承压", Kind: graphmodel.NodeKindPrediction, GraphRole: graphmodel.GraphRoleTarget, IsPrimary: true, VerificationStatus: graphmodel.VerificationProved, TimeBucket: "1w"}}}
		if err := store.PersistMemoryContentGraph(context.Background(), "u-empty-eg", sg, time.Now().UTC()); err != nil {
			return nil, err
		}
		return store, nil
	}
	var stdout, stderr bytes.Buffer
	code := run([]string{"memory", "event-graphs", "--card", "--user", "u-empty-eg", "--subject", "不存在"}, "/tmp/project", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("event-graphs empty card code = %d, stderr = %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "No event graphs matched") {
		t.Fatalf("stdout = %q, want no-match message", stdout.String())
	}
}

func TestRunMemoryParadigmsCardShowsNoMatchMessageForEmptyFilter(t *testing.T) {
	prevBuildApp := buildApp
	prevOpenSQLiteStore := openSQLiteStore
	t.Cleanup(func() {
		buildApp = prevBuildApp
		openSQLiteStore = prevOpenSQLiteStore
	})

	tmp := t.TempDir()
	buildApp = func(projectRoot string) (*bootstrap.App, error) {
		app := &bootstrap.App{}
		app.Settings.ContentDBPath = tmp + "/content.db"
		return app, nil
	}
	openSQLiteStore = func(path string) (*contentstore.SQLiteStore, error) {
		store, err := contentstore.NewSQLiteStore(path)
		if err != nil {
			return nil, err
		}
		sg := graphmodel.ContentSubgraph{ID: "empty-pg", ArticleID: "empty-pg", SourcePlatform: "twitter", SourceExternalID: "empty-pg", CompileVersion: graphmodel.CompileBridgeVersion, CompiledAt: time.Now().UTC().Format(time.RFC3339), UpdatedAt: time.Now().UTC().Format(time.RFC3339), Nodes: []graphmodel.GraphNode{{ID: "n1", SourceArticleID: "empty-pg", SourcePlatform: "twitter", SourceExternalID: "empty-pg", RawText: "美联储加息0.25%", SubjectText: "美联储", ChangeText: "加息0.25%", Kind: graphmodel.NodeKindObservation, GraphRole: graphmodel.GraphRoleDriver, IsPrimary: true, VerificationStatus: graphmodel.VerificationPending, TimeBucket: "1w"}, {ID: "n2", SourceArticleID: "empty-pg", SourcePlatform: "twitter", SourceExternalID: "empty-pg", RawText: "未来一周美股承压", SubjectText: "美股", ChangeText: "承压", Kind: graphmodel.NodeKindPrediction, GraphRole: graphmodel.GraphRoleTarget, IsPrimary: true, VerificationStatus: graphmodel.VerificationProved, TimeBucket: "1w"}}}
		if err := store.PersistMemoryContentGraph(context.Background(), "u-empty-pg", sg, time.Now().UTC()); err != nil {
			return nil, err
		}
		return store, nil
	}
	var stdout, stderr bytes.Buffer
	code := run([]string{"memory", "paradigms", "--card", "--user", "u-empty-pg", "--subject", "不存在"}, "/tmp/project", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("paradigms empty card code = %d, stderr = %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "No paradigms matched") {
		t.Fatalf("stdout = %q, want no-match message", stdout.String())
	}
}

func TestRunMemoryContentGraphsCardShowsNoMatchMessageForEmptyFilter(t *testing.T) {
	prevBuildApp := buildApp
	prevOpenSQLiteStore := openSQLiteStore
	t.Cleanup(func() {
		buildApp = prevBuildApp
		openSQLiteStore = prevOpenSQLiteStore
	})

	tmp := t.TempDir()
	buildApp = func(projectRoot string) (*bootstrap.App, error) {
		app := &bootstrap.App{}
		app.Settings.ContentDBPath = tmp + "/content.db"
		return app, nil
	}
	openSQLiteStore = func(path string) (*contentstore.SQLiteStore, error) {
		store, err := contentstore.NewSQLiteStore(path)
		if err != nil {
			return nil, err
		}
		now := time.Now().UTC()
		sg := graphmodel.ContentSubgraph{ID: "empty-cg-card", ArticleID: "empty-cg-card", SourcePlatform: "twitter", SourceExternalID: "empty-cg-card", CompileVersion: graphmodel.CompileBridgeVersion, CompiledAt: now.Format(time.RFC3339), UpdatedAt: now.Format(time.RFC3339), Nodes: []graphmodel.GraphNode{{ID: "n1", SourceArticleID: "empty-cg-card", SourcePlatform: "twitter", SourceExternalID: "empty-cg-card", RawText: "美联储加息0.25%", SubjectText: "美联储", ChangeText: "加息0.25%", Kind: graphmodel.NodeKindObservation, IsPrimary: true, VerificationStatus: graphmodel.VerificationPending}}}
		if err := store.PersistMemoryContentGraph(context.Background(), "u-empty-cg-card", sg, now); err != nil {
			return nil, err
		}
		return store, nil
	}
	var stdout, stderr bytes.Buffer
	code := run([]string{"memory", "content-graphs", "--card", "--user", "u-empty-cg-card", "--subject", "不存在"}, "/tmp/project", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("content-graphs empty card code = %d, stderr = %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "No content graphs matched") {
		t.Fatalf("stdout = %q, want no-match message", stdout.String())
	}
}

func TestRunMemoryEventEvidencePrintsPersistedLinks(t *testing.T) {
	prevBuildApp := buildApp
	prevOpenSQLiteStore := openSQLiteStore
	t.Cleanup(func() {
		buildApp = prevBuildApp
		openSQLiteStore = prevOpenSQLiteStore
	})
	tmp := t.TempDir()
	buildApp = func(projectRoot string) (*bootstrap.App, error) {
		app := &bootstrap.App{}
		app.Settings.ContentDBPath = tmp + "/content.db"
		return app, nil
	}
	openSQLiteStore = func(path string) (*contentstore.SQLiteStore, error) {
		store, err := contentstore.NewSQLiteStore(path)
		if err != nil {
			return nil, err
		}
		sg := graphmodel.ContentSubgraph{ID: "ev-cli", ArticleID: "ev-cli", SourcePlatform: "twitter", SourceExternalID: "ev-cli", CompileVersion: graphmodel.CompileBridgeVersion, CompiledAt: time.Now().UTC().Format(time.RFC3339), UpdatedAt: time.Now().UTC().Format(time.RFC3339), Nodes: []graphmodel.GraphNode{{ID: "n1", SourceArticleID: "ev-cli", SourcePlatform: "twitter", SourceExternalID: "ev-cli", RawText: "美联储加息0.25%", SubjectText: "美联储", ChangeText: "加息0.25%", Kind: graphmodel.NodeKindObservation, GraphRole: graphmodel.GraphRoleDriver, IsPrimary: true, VerificationStatus: graphmodel.VerificationPending, TimeBucket: "1w"}, {ID: "n2", SourceArticleID: "ev-cli", SourcePlatform: "twitter", SourceExternalID: "ev-cli", RawText: "未来一周美股承压", SubjectText: "美股", ChangeText: "承压", Kind: graphmodel.NodeKindPrediction, GraphRole: graphmodel.GraphRoleTarget, IsPrimary: true, VerificationStatus: graphmodel.VerificationProved, TimeBucket: "1w"}}}
		if err := store.PersistMemoryContentGraph(context.Background(), "u-ev-cli", sg, time.Now().UTC()); err != nil {
			return nil, err
		}
		graphs, err := store.ListEventGraphs(context.Background(), "u-ev-cli")
		if err != nil {
			return nil, err
		}
		if len(graphs) == 0 {
			return nil, nil
		}
		return store, nil
	}
	var stdout, stderr bytes.Buffer
	code := run([]string{"memory", "event-evidence", "--user", "u-ev-cli"}, "/tmp/project", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("event-evidence code = %d, stderr = %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "event_graph_id") {
		t.Fatalf("stdout = %q, want event evidence payload", stdout.String())
	}
}

func TestRunMemoryParadigmEvidencePrintsPersistedLinks(t *testing.T) {
	prevBuildApp := buildApp
	prevOpenSQLiteStore := openSQLiteStore
	t.Cleanup(func() { buildApp = prevBuildApp; openSQLiteStore = prevOpenSQLiteStore })
	tmp := t.TempDir()
	buildApp = func(projectRoot string) (*bootstrap.App, error) {
		app := &bootstrap.App{}
		app.Settings.ContentDBPath = tmp + "/content.db"
		return app, nil
	}
	openSQLiteStore = func(path string) (*contentstore.SQLiteStore, error) {
		store, err := contentstore.NewSQLiteStore(path)
		if err != nil {
			return nil, err
		}
		sg := graphmodel.ContentSubgraph{ID: "pev-cli", ArticleID: "pev-cli", SourcePlatform: "twitter", SourceExternalID: "pev-cli", CompileVersion: graphmodel.CompileBridgeVersion, CompiledAt: time.Now().UTC().Format(time.RFC3339), UpdatedAt: time.Now().UTC().Format(time.RFC3339), Nodes: []graphmodel.GraphNode{{ID: "n1", SourceArticleID: "pev-cli", SourcePlatform: "twitter", SourceExternalID: "pev-cli", RawText: "美联储加息0.25%", SubjectText: "美联储", ChangeText: "加息0.25%", Kind: graphmodel.NodeKindObservation, GraphRole: graphmodel.GraphRoleDriver, IsPrimary: true, VerificationStatus: graphmodel.VerificationPending, TimeBucket: "1w"}, {ID: "n2", SourceArticleID: "pev-cli", SourcePlatform: "twitter", SourceExternalID: "pev-cli", RawText: "未来一周美股承压", SubjectText: "美股", ChangeText: "承压", Kind: graphmodel.NodeKindPrediction, GraphRole: graphmodel.GraphRoleTarget, IsPrimary: true, VerificationStatus: graphmodel.VerificationProved, TimeBucket: "1w"}}}
		if err := store.PersistMemoryContentGraph(context.Background(), "u-pev-cli", sg, time.Now().UTC()); err != nil {
			return nil, err
		}
		return store, nil
	}
	var stdout, stderr bytes.Buffer
	code := run([]string{"memory", "paradigm-evidence", "--user", "u-pev-cli"}, "/tmp/project", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("paradigm-evidence code = %d, stderr = %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "paradigm_id") {
		t.Fatalf("stdout = %q, want paradigm evidence payload", stdout.String())
	}
}

func TestRunMemoryEventEvidenceSupportsEventGraphIDFilter(t *testing.T) {
	prevBuildApp := buildApp
	prevOpenSQLiteStore := openSQLiteStore
	t.Cleanup(func() { buildApp = prevBuildApp; openSQLiteStore = prevOpenSQLiteStore })
	tmp := t.TempDir()
	buildApp = func(projectRoot string) (*bootstrap.App, error) {
		app := &bootstrap.App{}
		app.Settings.ContentDBPath = tmp + "/content.db"
		return app, nil
	}
	openSQLiteStore = func(path string) (*contentstore.SQLiteStore, error) {
		store, err := contentstore.NewSQLiteStore(path)
		if err != nil {
			return nil, err
		}
		now := time.Now().UTC()
		for _, sg := range []graphmodel.ContentSubgraph{
			{ID: "ee-filter-1", ArticleID: "ee-filter-1", SourcePlatform: "twitter", SourceExternalID: "ee-filter-1", CompileVersion: graphmodel.CompileBridgeVersion, CompiledAt: now.Format(time.RFC3339), UpdatedAt: now.Format(time.RFC3339), Nodes: []graphmodel.GraphNode{{ID: "n1", SourceArticleID: "ee-filter-1", SourcePlatform: "twitter", SourceExternalID: "ee-filter-1", RawText: "美联储加息0.25%", SubjectText: "美联储", ChangeText: "加息0.25%", Kind: graphmodel.NodeKindObservation, GraphRole: graphmodel.GraphRoleDriver, IsPrimary: true, VerificationStatus: graphmodel.VerificationPending, TimeBucket: "1w"}, {ID: "n2", SourceArticleID: "ee-filter-1", SourcePlatform: "twitter", SourceExternalID: "ee-filter-1", RawText: "未来一周美股承压", SubjectText: "美股", ChangeText: "承压", Kind: graphmodel.NodeKindPrediction, GraphRole: graphmodel.GraphRoleTarget, IsPrimary: true, VerificationStatus: graphmodel.VerificationProved, TimeBucket: "1w"}}},
			{ID: "ee-filter-2", ArticleID: "ee-filter-2", SourcePlatform: "twitter", SourceExternalID: "ee-filter-2", CompileVersion: graphmodel.CompileBridgeVersion, CompiledAt: now.Format(time.RFC3339), UpdatedAt: now.Format(time.RFC3339), Nodes: []graphmodel.GraphNode{{ID: "n1", SourceArticleID: "ee-filter-2", SourcePlatform: "twitter", SourceExternalID: "ee-filter-2", RawText: "欧洲央行放缓缩表", SubjectText: "欧洲央行", ChangeText: "放缓缩表", Kind: graphmodel.NodeKindObservation, GraphRole: graphmodel.GraphRoleDriver, IsPrimary: true, VerificationStatus: graphmodel.VerificationPending, TimeBucket: "1w"}, {ID: "n2", SourceArticleID: "ee-filter-2", SourcePlatform: "twitter", SourceExternalID: "ee-filter-2", RawText: "欧股承压", SubjectText: "欧股", ChangeText: "承压", Kind: graphmodel.NodeKindPrediction, GraphRole: graphmodel.GraphRoleTarget, IsPrimary: true, VerificationStatus: graphmodel.VerificationProved, TimeBucket: "1w"}}},
		} {
			if err := store.PersistMemoryContentGraph(context.Background(), "u-ee-filter", sg, now); err != nil {
				return nil, err
			}
		}
		return store, nil
	}
	var stdout, stderr bytes.Buffer
	code := run([]string{"memory", "event-evidence", "--user", "u-ee-filter", "--event-graph-id", "u-ee-filter:driver:美联储:1w"}, "/tmp/project", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("event-evidence filter code = %d, stderr = %s", code, stderr.String())
	}
	var out []contentstore.EventGraphEvidenceLink
	if err := json.Unmarshal(stdout.Bytes(), &out); err != nil {
		t.Fatalf("json.Unmarshal(event-evidence filter) error = %v", err)
	}
	for _, item := range out {
		if item.EventGraphID != "u-ee-filter:driver:美联储:1w" {
			t.Fatalf("filtered links = %#v, want only target event graph id", out)
		}
	}
}

func TestRunMemoryParadigmEvidenceSupportsParadigmIDFilter(t *testing.T) {
	prevBuildApp := buildApp
	prevOpenSQLiteStore := openSQLiteStore
	t.Cleanup(func() { buildApp = prevBuildApp; openSQLiteStore = prevOpenSQLiteStore })
	tmp := t.TempDir()
	buildApp = func(projectRoot string) (*bootstrap.App, error) {
		app := &bootstrap.App{}
		app.Settings.ContentDBPath = tmp + "/content.db"
		return app, nil
	}
	openSQLiteStore = func(path string) (*contentstore.SQLiteStore, error) {
		store, err := contentstore.NewSQLiteStore(path)
		if err != nil {
			return nil, err
		}
		now := time.Now().UTC()
		for _, sg := range []graphmodel.ContentSubgraph{
			{ID: "pe-filter-1", ArticleID: "pe-filter-1", SourcePlatform: "twitter", SourceExternalID: "pe-filter-1", CompileVersion: graphmodel.CompileBridgeVersion, CompiledAt: now.Format(time.RFC3339), UpdatedAt: now.Format(time.RFC3339), Nodes: []graphmodel.GraphNode{{ID: "n1", SourceArticleID: "pe-filter-1", SourcePlatform: "twitter", SourceExternalID: "pe-filter-1", RawText: "美联储加息0.25%", SubjectText: "美联储", ChangeText: "加息0.25%", Kind: graphmodel.NodeKindObservation, GraphRole: graphmodel.GraphRoleDriver, IsPrimary: true, VerificationStatus: graphmodel.VerificationPending, TimeBucket: "1w"}, {ID: "n2", SourceArticleID: "pe-filter-1", SourcePlatform: "twitter", SourceExternalID: "pe-filter-1", RawText: "未来一周美股承压", SubjectText: "美股", ChangeText: "承压", Kind: graphmodel.NodeKindPrediction, GraphRole: graphmodel.GraphRoleTarget, IsPrimary: true, VerificationStatus: graphmodel.VerificationProved, TimeBucket: "1w"}}},
			{ID: "pe-filter-2", ArticleID: "pe-filter-2", SourcePlatform: "twitter", SourceExternalID: "pe-filter-2", CompileVersion: graphmodel.CompileBridgeVersion, CompiledAt: now.Format(time.RFC3339), UpdatedAt: now.Format(time.RFC3339), Nodes: []graphmodel.GraphNode{{ID: "n1", SourceArticleID: "pe-filter-2", SourcePlatform: "twitter", SourceExternalID: "pe-filter-2", RawText: "欧洲央行放缓缩表", SubjectText: "欧洲央行", ChangeText: "放缓缩表", Kind: graphmodel.NodeKindObservation, GraphRole: graphmodel.GraphRoleDriver, IsPrimary: true, VerificationStatus: graphmodel.VerificationPending, TimeBucket: "1w"}, {ID: "n2", SourceArticleID: "pe-filter-2", SourcePlatform: "twitter", SourceExternalID: "pe-filter-2", RawText: "欧股承压", SubjectText: "欧股", ChangeText: "承压", Kind: graphmodel.NodeKindPrediction, GraphRole: graphmodel.GraphRoleTarget, IsPrimary: true, VerificationStatus: graphmodel.VerificationProved, TimeBucket: "1w"}}},
		} {
			if err := store.PersistMemoryContentGraph(context.Background(), "u-pe-filter", sg, now); err != nil {
				return nil, err
			}
		}
		return store, nil
	}
	var stdout, stderr bytes.Buffer
	code := run([]string{"memory", "paradigm-evidence", "--user", "u-pe-filter", "--paradigm-id", "u-pe-filter:美联储:美股:1w"}, "/tmp/project", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("paradigm-evidence filter code = %d, stderr = %s", code, stderr.String())
	}
	var out []contentstore.ParadigmEvidenceLink
	if err := json.Unmarshal(stdout.Bytes(), &out); err != nil {
		t.Fatalf("json.Unmarshal(paradigm-evidence filter) error = %v", err)
	}
	for _, item := range out {
		if item.ParadigmID != "u-pe-filter:美联储:美股:1w" {
			t.Fatalf("filtered links = %#v, want only target paradigm id", out)
		}
	}
}

func TestRunMemoryEventEvidenceShowsNoMatchMessageForUnknownID(t *testing.T) {
	prevBuildApp := buildApp
	prevOpenSQLiteStore := openSQLiteStore
	t.Cleanup(func() { buildApp = prevBuildApp; openSQLiteStore = prevOpenSQLiteStore })
	tmp := t.TempDir()
	buildApp = func(projectRoot string) (*bootstrap.App, error) {
		app := &bootstrap.App{}
		app.Settings.ContentDBPath = tmp + "/content.db"
		return app, nil
	}
	openSQLiteStore = func(path string) (*contentstore.SQLiteStore, error) {
		store, err := contentstore.NewSQLiteStore(path)
		if err != nil {
			return nil, err
		}
		now := time.Now().UTC()
		sg := graphmodel.ContentSubgraph{ID: "empty-ee", ArticleID: "empty-ee", SourcePlatform: "twitter", SourceExternalID: "empty-ee", CompileVersion: graphmodel.CompileBridgeVersion, CompiledAt: now.Format(time.RFC3339), UpdatedAt: now.Format(time.RFC3339), Nodes: []graphmodel.GraphNode{{ID: "n1", SourceArticleID: "empty-ee", SourcePlatform: "twitter", SourceExternalID: "empty-ee", RawText: "美联储加息0.25%", SubjectText: "美联储", ChangeText: "加息0.25%", Kind: graphmodel.NodeKindObservation, GraphRole: graphmodel.GraphRoleDriver, IsPrimary: true, VerificationStatus: graphmodel.VerificationPending, TimeBucket: "1w"}, {ID: "n2", SourceArticleID: "empty-ee", SourcePlatform: "twitter", SourceExternalID: "empty-ee", RawText: "未来一周美股承压", SubjectText: "美股", ChangeText: "承压", Kind: graphmodel.NodeKindPrediction, GraphRole: graphmodel.GraphRoleTarget, IsPrimary: true, VerificationStatus: graphmodel.VerificationProved, TimeBucket: "1w"}}}
		if err := store.PersistMemoryContentGraph(context.Background(), "u-empty-ee", sg, now); err != nil {
			return nil, err
		}
		return store, nil
	}
	var stdout, stderr bytes.Buffer
	code := run([]string{"memory", "event-evidence", "--user", "u-empty-ee", "--event-graph-id", "missing"}, "/tmp/project", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("event-evidence no-match code = %d, stderr = %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "No event evidence matched") {
		t.Fatalf("stdout = %q, want no-match message", stdout.String())
	}
}

func TestRunMemoryParadigmEvidenceShowsNoMatchMessageForUnknownID(t *testing.T) {
	prevBuildApp := buildApp
	prevOpenSQLiteStore := openSQLiteStore
	t.Cleanup(func() { buildApp = prevBuildApp; openSQLiteStore = prevOpenSQLiteStore })
	tmp := t.TempDir()
	buildApp = func(projectRoot string) (*bootstrap.App, error) {
		app := &bootstrap.App{}
		app.Settings.ContentDBPath = tmp + "/content.db"
		return app, nil
	}
	openSQLiteStore = func(path string) (*contentstore.SQLiteStore, error) {
		store, err := contentstore.NewSQLiteStore(path)
		if err != nil {
			return nil, err
		}
		now := time.Now().UTC()
		sg := graphmodel.ContentSubgraph{ID: "empty-pe", ArticleID: "empty-pe", SourcePlatform: "twitter", SourceExternalID: "empty-pe", CompileVersion: graphmodel.CompileBridgeVersion, CompiledAt: now.Format(time.RFC3339), UpdatedAt: now.Format(time.RFC3339), Nodes: []graphmodel.GraphNode{{ID: "n1", SourceArticleID: "empty-pe", SourcePlatform: "twitter", SourceExternalID: "empty-pe", RawText: "美联储加息0.25%", SubjectText: "美联储", ChangeText: "加息0.25%", Kind: graphmodel.NodeKindObservation, GraphRole: graphmodel.GraphRoleDriver, IsPrimary: true, VerificationStatus: graphmodel.VerificationPending, TimeBucket: "1w"}, {ID: "n2", SourceArticleID: "empty-pe", SourcePlatform: "twitter", SourceExternalID: "empty-pe", RawText: "未来一周美股承压", SubjectText: "美股", ChangeText: "承压", Kind: graphmodel.NodeKindPrediction, GraphRole: graphmodel.GraphRoleTarget, IsPrimary: true, VerificationStatus: graphmodel.VerificationProved, TimeBucket: "1w"}}}
		if err := store.PersistMemoryContentGraph(context.Background(), "u-empty-pe", sg, now); err != nil {
			return nil, err
		}
		return store, nil
	}
	var stdout, stderr bytes.Buffer
	code := run([]string{"memory", "paradigm-evidence", "--user", "u-empty-pe", "--paradigm-id", "missing"}, "/tmp/project", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("paradigm-evidence no-match code = %d, stderr = %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "No paradigm evidence matched") {
		t.Fatalf("stdout = %q, want no-match message", stdout.String())
	}
}

func TestRunMemoryEventEvidenceCardPrintsReadableSections(t *testing.T) {
	prevBuildApp := buildApp
	prevOpenSQLiteStore := openSQLiteStore
	t.Cleanup(func() { buildApp = prevBuildApp; openSQLiteStore = prevOpenSQLiteStore })
	tmp := t.TempDir()
	buildApp = func(projectRoot string) (*bootstrap.App, error) {
		app := &bootstrap.App{}
		app.Settings.ContentDBPath = tmp + "/content.db"
		return app, nil
	}
	openSQLiteStore = func(path string) (*contentstore.SQLiteStore, error) {
		store, err := contentstore.NewSQLiteStore(path)
		if err != nil {
			return nil, err
		}
		now := time.Now().UTC()
		sg := graphmodel.ContentSubgraph{ID: "event-evi-card", ArticleID: "event-evi-card", SourcePlatform: "twitter", SourceExternalID: "event-evi-card", CompileVersion: graphmodel.CompileBridgeVersion, CompiledAt: now.Format(time.RFC3339), UpdatedAt: now.Format(time.RFC3339), Nodes: []graphmodel.GraphNode{{ID: "n1", SourceArticleID: "event-evi-card", SourcePlatform: "twitter", SourceExternalID: "event-evi-card", RawText: "美联储加息0.25%", SubjectText: "美联储", ChangeText: "加息0.25%", Kind: graphmodel.NodeKindObservation, GraphRole: graphmodel.GraphRoleDriver, IsPrimary: true, VerificationStatus: graphmodel.VerificationPending, TimeBucket: "1w"}, {ID: "n2", SourceArticleID: "event-evi-card", SourcePlatform: "twitter", SourceExternalID: "event-evi-card", RawText: "未来一周美股承压", SubjectText: "美股", ChangeText: "承压", Kind: graphmodel.NodeKindPrediction, GraphRole: graphmodel.GraphRoleTarget, IsPrimary: true, VerificationStatus: graphmodel.VerificationProved, TimeBucket: "1w"}}}
		if err := store.PersistMemoryContentGraph(context.Background(), "u-event-evi-card", sg, now); err != nil {
			return nil, err
		}
		return store, nil
	}
	var stdout, stderr bytes.Buffer
	code := run([]string{"memory", "event-evidence", "--card", "--user", "u-event-evi-card"}, "/tmp/project", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("event-evidence --card code = %d, stderr = %s", code, stderr.String())
	}
	for _, want := range []string{"Event Evidence", "event_graph_id", "subgraph_id"} {
		if !strings.Contains(stdout.String(), want) {
			t.Fatalf("stdout = %q, want substring %q", stdout.String(), want)
		}
	}
}

func TestRunMemoryParadigmEvidenceCardPrintsReadableSections(t *testing.T) {
	prevBuildApp := buildApp
	prevOpenSQLiteStore := openSQLiteStore
	t.Cleanup(func() { buildApp = prevBuildApp; openSQLiteStore = prevOpenSQLiteStore })
	tmp := t.TempDir()
	buildApp = func(projectRoot string) (*bootstrap.App, error) {
		app := &bootstrap.App{}
		app.Settings.ContentDBPath = tmp + "/content.db"
		return app, nil
	}
	openSQLiteStore = func(path string) (*contentstore.SQLiteStore, error) {
		store, err := contentstore.NewSQLiteStore(path)
		if err != nil {
			return nil, err
		}
		now := time.Now().UTC()
		sg := graphmodel.ContentSubgraph{ID: "paradigm-evi-card", ArticleID: "paradigm-evi-card", SourcePlatform: "twitter", SourceExternalID: "paradigm-evi-card", CompileVersion: graphmodel.CompileBridgeVersion, CompiledAt: now.Format(time.RFC3339), UpdatedAt: now.Format(time.RFC3339), Nodes: []graphmodel.GraphNode{{ID: "n1", SourceArticleID: "paradigm-evi-card", SourcePlatform: "twitter", SourceExternalID: "paradigm-evi-card", RawText: "美联储加息0.25%", SubjectText: "美联储", ChangeText: "加息0.25%", Kind: graphmodel.NodeKindObservation, GraphRole: graphmodel.GraphRoleDriver, IsPrimary: true, VerificationStatus: graphmodel.VerificationPending, TimeBucket: "1w"}, {ID: "n2", SourceArticleID: "paradigm-evi-card", SourcePlatform: "twitter", SourceExternalID: "paradigm-evi-card", RawText: "未来一周美股承压", SubjectText: "美股", ChangeText: "承压", Kind: graphmodel.NodeKindPrediction, GraphRole: graphmodel.GraphRoleTarget, IsPrimary: true, VerificationStatus: graphmodel.VerificationProved, TimeBucket: "1w"}}}
		if err := store.PersistMemoryContentGraph(context.Background(), "u-paradigm-evi-card", sg, now); err != nil {
			return nil, err
		}
		return store, nil
	}
	var stdout, stderr bytes.Buffer
	code := run([]string{"memory", "paradigm-evidence", "--card", "--user", "u-paradigm-evi-card"}, "/tmp/project", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("paradigm-evidence --card code = %d, stderr = %s", code, stderr.String())
	}
	for _, want := range []string{"Paradigm Evidence", "paradigm_id", "subgraph_id"} {
		if !strings.Contains(stdout.String(), want) {
			t.Fatalf("stdout = %q, want substring %q", stdout.String(), want)
		}
	}
}
