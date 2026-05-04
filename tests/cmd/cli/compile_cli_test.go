package main

import (
	"bytes"
	"context"
	"encoding/json"
	c "github.com/kumaloha/VariX/varix/compile"
	"github.com/kumaloha/VariX/varix/ingest"
	"github.com/kumaloha/VariX/varix/ingest/dispatcher"
	"github.com/kumaloha/VariX/varix/ingest/polling"
	"github.com/kumaloha/VariX/varix/ingest/types"
	varixllm "github.com/kumaloha/VariX/varix/llm"
	"github.com/kumaloha/VariX/varix/memory"
	"github.com/kumaloha/VariX/varix/storage/contentstore"
	"testing"
	"time"
)

func TestRunCompileWritesCompiledRecordJSON(t *testing.T) {
	prevNewIngestRuntime := newIngestRuntime
	prevBuildCompileClientCurrent := buildCompileClientCurrent
	prevOpenSQLiteStore := openSQLiteStore
	t.Cleanup(func() {
		newIngestRuntime = prevNewIngestRuntime
		buildCompileClientCurrent = prevBuildCompileClientCurrent
		openSQLiteStore = prevOpenSQLiteStore
	})

	tmp := t.TempDir()
	newIngestRuntime = func(projectRoot string) (*ingest.Runtime, error) {
		src := fakeItemSource{
			items: []types.RawContent{{
				Source:     "weibo",
				ExternalID: "QAu4U9USk",
				Content:    "hello",
				AuthorName: "Alice",
				URL:        "https://weibo.com/1182426800/QAu4U9USk",
			}},
		}
		app := &ingest.Runtime{
			Dispatcher: dispatcher.New(
				func(raw string) (types.ParsedURL, error) {
					return types.ParsedURL{
						Platform:     types.PlatformWeb,
						ContentType:  types.ContentTypePost,
						PlatformID:   "id-1",
						CanonicalURL: raw,
					}, nil
				},
				[]dispatcher.ItemSource{src},
				nil,
				nil,
			),
		}
		app.Settings.ContentDBPath = tmp + "/content.db"
		return app, nil
	}

	buildCompileClientCurrent = func(projectRoot string) compileClient {
		return fakeCompileClient{
			record: c.Record{
				UnitID:         "weibo:QAu4U9USk",
				Source:         "weibo",
				ExternalID:     "QAu4U9USk",
				RootExternalID: "QAu4U9USk",
				Model:          "compile-default",
				Output: c.Output{
					Summary: "compile 一句话",
					Graph: c.ReasoningGraph{
						Nodes: []c.GraphNode{testGraphNode("n1", c.NodeFact, "事实A"), testGraphNode("n2", c.NodeConclusion, "结论B")},
						Edges: []c.GraphEdge{{From: "n1", To: "n2", Kind: c.EdgeDerives}},
					},
					Details: c.HiddenDetails{Caveats: []string{"说明"}},
				},
				CompiledAt: time.Now().UTC(),
			},
		}
	}

	openSQLiteStore = func(path string) (*contentstore.SQLiteStore, error) {
		return contentstore.NewSQLiteStore(path)
	}

	var stdout, stderr bytes.Buffer
	code := run([]string{"compile", "run", "https://example.com/post"}, "/tmp/project", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("run() code = %d, stderr = %s", code, stderr.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q, want empty", stderr.String())
	}
	var got c.Record
	if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
		t.Fatalf("json.Unmarshal(stdout) error = %v", err)
	}
	if got.Output.Summary != "compile 一句话" {
		t.Fatalf("Summary = %q", got.Output.Summary)
	}
}

func TestRunCompilePipelineUsesCurrentClient(t *testing.T) {
	prevNewIngestRuntime := newIngestRuntime
	prevBuildCompileClientCurrent := buildCompileClientCurrent
	prevOpenSQLiteStore := openSQLiteStore
	t.Cleanup(func() {
		newIngestRuntime = prevNewIngestRuntime
		buildCompileClientCurrent = prevBuildCompileClientCurrent
		openSQLiteStore = prevOpenSQLiteStore
	})

	tmp := t.TempDir()
	newIngestRuntime = func(projectRoot string) (*ingest.Runtime, error) {
		app := &ingest.Runtime{}
		app.Settings.ContentDBPath = tmp + "/content.db"
		src := fakeItemSource{
			items: []types.RawContent{{
				Source:     "web",
				ExternalID: "compile-id",
				Content:    "hello",
				URL:        "https://example.com/compile",
			}},
		}
		app.Dispatcher = dispatcher.New(
			func(raw string) (types.ParsedURL, error) {
				return types.ParsedURL{Platform: types.PlatformWeb, ContentType: types.ContentTypePost, PlatformID: "compile-id", CanonicalURL: raw}, nil
			},
			[]dispatcher.ItemSource{src},
			nil,
			nil,
		)
		return app, nil
	}
	buildCompileClientCurrent = func(projectRoot string) compileClient {
		return fakeCompileClient{record: c.Record{
			UnitID:     "web:compile-id",
			Source:     "web",
			ExternalID: "compile-id",
			Model:      "qwen3.6-plus",
			Metrics:    c.RecordMetrics{CompileElapsedMS: 777, CompileStageElapsedMS: map[string]int64{"extract": 101, "refine": 102, "aggregate": 103, "support": 104, "collapse": 105, "relations": 106, "classify": 107, "coverage": 108, "render": 109}},
			Output: c.Output{
				Summary: "compile summary",
				Graph: c.ReasoningGraph{
					Nodes: []c.GraphNode{testGraphNode("n1", c.NodeFact, "事实A"), testGraphNode("n2", c.NodeConclusion, "结论B")},
					Edges: []c.GraphEdge{{From: "n1", To: "n2", Kind: c.EdgeDerives}},
				},
				Details: c.HiddenDetails{Caveats: []string{"说明"}},
			},
			CompiledAt: time.Now().UTC(),
		}}
	}
	openSQLiteStore = func(path string) (*contentstore.SQLiteStore, error) {
		return contentstore.NewSQLiteStore(path)
	}

	var stdout, stderr bytes.Buffer
	code := run([]string{"compile", "run", "--url", "https://example.com/compile"}, "/tmp/project", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("run() code = %d, stderr = %s", code, stderr.String())
	}
	var got c.Record
	if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
		t.Fatalf("json.Unmarshal(stdout) error = %v", err)
	}
	if got.Output.Summary != "compile summary" {
		t.Fatalf("Summary = %q, want compile summary", got.Output.Summary)
	}
	if got.Metrics.CompileElapsedMS != 777 {
		t.Fatalf("CompileElapsedMS = %d, want 777", got.Metrics.CompileElapsedMS)
	}
	for _, stage := range []string{"extract", "refine", "aggregate", "support", "collapse", "relations", "classify", "coverage", "render"} {
		if got.Metrics.CompileStageElapsedMS[stage] <= 0 {
			t.Fatalf("CompileStageElapsedMS = %#v, want positive persisted compile stage metric for %q", got.Metrics.CompileStageElapsedMS, stage)
		}
	}
	stdout.Reset()
	stderr.Reset()
	code = run([]string{"compile", "show", "--platform", "web", "--id", "compile-id"}, "/tmp/project", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("compile show code = %d, stderr = %s", code, stderr.String())
	}
	var shown c.Record
	if err := json.Unmarshal(stdout.Bytes(), &shown); err != nil {
		t.Fatalf("json.Unmarshal(show stdout) error = %v", err)
	}
	if shown.Metrics.CompileElapsedMS != 777 {
		t.Fatalf("shown CompileElapsedMS = %d, want 777", shown.Metrics.CompileElapsedMS)
	}
	for _, stage := range []string{"extract", "refine", "aggregate", "support", "collapse", "relations", "classify", "coverage", "render"} {
		if shown.Metrics.CompileStageElapsedMS[stage] <= 0 {
			t.Fatalf("shown CompileStageElapsedMS = %#v, want positive persisted compile stage metric for %q", shown.Metrics.CompileStageElapsedMS, stage)
		}
	}
}

func TestRunCompileReadsExistingRawCaptureByPlatformAndID(t *testing.T) {
	prevNewIngestRuntime := newIngestRuntime
	prevBuildCompileClientCurrent := buildCompileClientCurrent
	prevOpenSQLiteStore := openSQLiteStore
	t.Cleanup(func() {
		newIngestRuntime = prevNewIngestRuntime
		buildCompileClientCurrent = prevBuildCompileClientCurrent
		openSQLiteStore = prevOpenSQLiteStore
	})

	tmp := t.TempDir()
	newIngestRuntime = func(projectRoot string) (*ingest.Runtime, error) {
		app := &ingest.Runtime{}
		app.Settings.ContentDBPath = tmp + "/content.db"
		return app, nil
	}
	buildCompileClientCurrent = func(projectRoot string) compileClient {
		return fakeCompileClient{
			record: c.Record{
				UnitID:         "weibo:QAu4U9USk",
				Source:         "weibo",
				ExternalID:     "QAu4U9USk",
				RootExternalID: "QAu4U9USk",
				Model:          varixllm.Qwen36PlusModel,
				Output: c.Output{
					Summary: "一句话",
					Graph: c.ReasoningGraph{
						Nodes: []c.GraphNode{testGraphNode("n1", c.NodeFact, "事实A"), testGraphNode("n2", c.NodeConclusion, "结论B")},
						Edges: []c.GraphEdge{{From: "n1", To: "n2", Kind: c.EdgeDerives}},
					},
					Details: c.HiddenDetails{Caveats: []string{"说明"}},
				},
				CompiledAt: time.Now().UTC(),
			},
		}
	}
	openSQLiteStore = func(path string) (*contentstore.SQLiteStore, error) {
		store, err := contentstore.NewSQLiteStore(path)
		if err != nil {
			return nil, err
		}
		if err := store.UpsertRawCapture(context.Background(), types.RawContent{
			Source:     "weibo",
			ExternalID: "QAu4U9USk",
			Content:    "hello",
			AuthorName: "Alice",
			URL:        "https://weibo.com/1182426800/QAu4U9USk",
		}); err != nil {
			return nil, err
		}
		return store, nil
	}

	var stdout, stderr bytes.Buffer
	code := run([]string{"compile", "run", "--platform", "weibo", "--id", "QAu4U9USk"}, "/tmp/project", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("run() code = %d, stderr = %s", code, stderr.String())
	}
	var got c.Record
	if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
		t.Fatalf("json.Unmarshal(stdout) error = %v", err)
	}
	if got.ExternalID != "QAu4U9USk" {
		t.Fatalf("ExternalID = %q", got.ExternalID)
	}
}

func TestRunCompileURLPrefersStoredRawCapture(t *testing.T) {
	prevNewIngestRuntime := newIngestRuntime
	prevBuildCompileClientCurrent := buildCompileClientCurrent
	prevOpenSQLiteStore := openSQLiteStore
	t.Cleanup(func() {
		newIngestRuntime = prevNewIngestRuntime
		buildCompileClientCurrent = prevBuildCompileClientCurrent
		openSQLiteStore = prevOpenSQLiteStore
	})

	tmp := t.TempDir()
	newIngestRuntime = func(projectRoot string) (*ingest.Runtime, error) {
		app := &ingest.Runtime{}
		app.Settings.ContentDBPath = tmp + "/content.db"
		app.Dispatcher = dispatcher.New(
			func(raw string) (types.ParsedURL, error) {
				return types.ParsedURL{Platform: types.PlatformTwitter, PlatformID: "2026305745872998803", CanonicalURL: raw}, nil
			},
			[]dispatcher.ItemSource{panicItemSource{}},
			nil,
			nil,
		)
		return app, nil
	}
	buildCompileClientCurrent = func(projectRoot string) compileClient {
		return fakeCompileClient{
			record: c.Record{
				UnitID:         "twitter:2026305745872998803",
				Source:         "twitter",
				ExternalID:     "2026305745872998803",
				RootExternalID: "2026305745872998803",
				Model:          varixllm.Qwen36PlusModel,
				Output: c.Output{
					Summary: "Dalio summary",
					Graph: c.ReasoningGraph{
						Nodes: []c.GraphNode{testGraphNode("n1", c.NodeFact, "事实A"), testGraphNode("n2", c.NodeConclusion, "结论B")},
						Edges: []c.GraphEdge{{From: "n1", To: "n2", Kind: c.EdgeDerives}},
					},
					Details:    c.HiddenDetails{Caveats: []string{"说明"}},
					Confidence: "high",
				},
				CompiledAt: time.Now().UTC(),
			},
		}
	}
	openSQLiteStore = func(path string) (*contentstore.SQLiteStore, error) {
		store, err := contentstore.NewSQLiteStore(path)
		if err != nil {
			return nil, err
		}
		if err := store.UpsertRawCapture(context.Background(), types.RawContent{
			Source:     "twitter",
			ExternalID: "2026305745872998803",
			Content:    "stored raw body",
			AuthorName: "Ray Dalio",
			URL:        "https://x.com/RayDalio/status/2026305745872998803",
		}); err != nil {
			return nil, err
		}
		return store, nil
	}

	var stdout, stderr bytes.Buffer
	code := run([]string{"compile", "run", "--url", "https://x.com/RayDalio/status/2026305745872998803"}, "/tmp/project", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("run() code = %d, stderr = %s", code, stderr.String())
	}
	var got c.Record
	if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
		t.Fatalf("json.Unmarshal(stdout) error = %v", err)
	}
	if got.ExternalID != "2026305745872998803" {
		t.Fatalf("ExternalID = %q", got.ExternalID)
	}
}

func TestRunCompileUsesStoredCompiledOutputUnlessForced(t *testing.T) {
	prevNewIngestRuntime := newIngestRuntime
	prevBuildCompileClientCurrent := buildCompileClientCurrent
	prevOpenSQLiteStore := openSQLiteStore
	t.Cleanup(func() {
		newIngestRuntime = prevNewIngestRuntime
		buildCompileClientCurrent = prevBuildCompileClientCurrent
		openSQLiteStore = prevOpenSQLiteStore
	})

	tmp := t.TempDir()
	newIngestRuntime = func(projectRoot string) (*ingest.Runtime, error) {
		app := &ingest.Runtime{}
		app.Settings.ContentDBPath = tmp + "/content.db"
		app.Dispatcher = dispatcher.New(
			func(raw string) (types.ParsedURL, error) {
				return types.ParsedURL{Platform: types.PlatformTwitter, PlatformID: "2026305745872998803", CanonicalURL: raw}, nil
			},
			[]dispatcher.ItemSource{panicItemSource{}},
			nil,
			nil,
		)
		return app, nil
	}
	buildCompileClientCurrent = func(projectRoot string) compileClient {
		return fakeCompileClient{
			record: c.Record{
				UnitID:         "twitter:2026305745872998803",
				Source:         "twitter",
				ExternalID:     "2026305745872998803",
				RootExternalID: "2026305745872998803",
				Model:          varixllm.Qwen36PlusModel,
				Output: c.Output{
					Summary: "new summary should not be used",
					Graph: c.ReasoningGraph{
						Nodes: []c.GraphNode{testGraphNode("n1", c.NodeFact, "事实A"), testGraphNode("n2", c.NodeConclusion, "结论B")},
						Edges: []c.GraphEdge{{From: "n1", To: "n2", Kind: c.EdgeDerives}},
					},
					Details: c.HiddenDetails{Caveats: []string{"说明"}},
				},
				CompiledAt: time.Now().UTC(),
			},
		}
	}
	openSQLiteStore = func(path string) (*contentstore.SQLiteStore, error) {
		store, err := contentstore.NewSQLiteStore(path)
		if err != nil {
			return nil, err
		}
		if err := store.UpsertCompiledOutput(context.Background(), c.Record{
			UnitID:         "twitter:2026305745872998803",
			Source:         "twitter",
			ExternalID:     "2026305745872998803",
			RootExternalID: "2026305745872998803",
			Model:          varixllm.Qwen36PlusModel,
			Output: c.Output{
				Summary: "cached summary",
				Graph: c.ReasoningGraph{
					Nodes: []c.GraphNode{testGraphNode("n1", c.NodeFact, "事实A"), testGraphNode("n2", c.NodeConclusion, "结论B")},
					Edges: []c.GraphEdge{{From: "n1", To: "n2", Kind: c.EdgeDerives}},
				},
				Details: c.HiddenDetails{Caveats: []string{"说明"}},
			},
			CompiledAt: time.Now().UTC(),
		}); err != nil {
			return nil, err
		}
		return store, nil
	}

	var stdout, stderr bytes.Buffer
	code := run([]string{"compile", "run", "--platform", "twitter", "--id", "2026305745872998803"}, "/tmp/project", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("run() code = %d, stderr = %s", code, stderr.String())
	}
	var got c.Record
	if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
		t.Fatalf("json.Unmarshal(stdout) error = %v", err)
	}
	if got.Output.Summary != "cached summary" {
		t.Fatalf("Summary = %q, want cached summary", got.Output.Summary)
	}
}

func TestRunCompileForceBypassesStoredCompiledOutput(t *testing.T) {
	prevNewIngestRuntime := newIngestRuntime
	prevBuildCompileClientCurrent := buildCompileClientCurrent
	prevOpenSQLiteStore := openSQLiteStore
	t.Cleanup(func() {
		newIngestRuntime = prevNewIngestRuntime
		buildCompileClientCurrent = prevBuildCompileClientCurrent
		openSQLiteStore = prevOpenSQLiteStore
	})

	tmp := t.TempDir()
	newIngestRuntime = func(projectRoot string) (*ingest.Runtime, error) {
		app := &ingest.Runtime{}
		app.Settings.ContentDBPath = tmp + "/content.db"
		return app, nil
	}
	buildCompileClientCurrent = func(projectRoot string) compileClient {
		return fakeCompileClient{
			record: c.Record{
				UnitID:         "weibo:QAu4U9USk",
				Source:         "weibo",
				ExternalID:     "QAu4U9USk",
				RootExternalID: "QAu4U9USk",
				Model:          varixllm.Qwen36PlusModel,
				Output: c.Output{
					Summary: "forced summary",
					Graph: c.ReasoningGraph{
						Nodes: []c.GraphNode{testGraphNode("n1", c.NodeFact, "事实A"), testGraphNode("n2", c.NodeConclusion, "结论B")},
						Edges: []c.GraphEdge{{From: "n1", To: "n2", Kind: c.EdgeDerives}},
					},
					Details: c.HiddenDetails{Caveats: []string{"说明"}},
				},
				CompiledAt: time.Now().UTC(),
			},
		}
	}
	openSQLiteStore = func(path string) (*contentstore.SQLiteStore, error) {
		store, err := contentstore.NewSQLiteStore(path)
		if err != nil {
			return nil, err
		}
		if err := store.UpsertRawCapture(context.Background(), types.RawContent{
			Source:     "weibo",
			ExternalID: "QAu4U9USk",
			Content:    "hello",
			AuthorName: "Alice",
			URL:        "https://weibo.com/1182426800/QAu4U9USk",
		}); err != nil {
			return nil, err
		}
		if err := store.UpsertCompiledOutput(context.Background(), c.Record{
			UnitID:         "weibo:QAu4U9USk",
			Source:         "weibo",
			ExternalID:     "QAu4U9USk",
			RootExternalID: "QAu4U9USk",
			Model:          varixllm.Qwen36PlusModel,
			Output: c.Output{
				Summary: "cached summary",
				Graph: c.ReasoningGraph{
					Nodes: []c.GraphNode{testGraphNode("n1", c.NodeFact, "事实A"), testGraphNode("n2", c.NodeConclusion, "结论B")},
					Edges: []c.GraphEdge{{From: "n1", To: "n2", Kind: c.EdgeDerives}},
				},
				Details: c.HiddenDetails{Caveats: []string{"说明"}},
			},
			CompiledAt: time.Now().UTC(),
		}); err != nil {
			return nil, err
		}
		return store, nil
	}

	var stdout, stderr bytes.Buffer
	code := run([]string{"compile", "run", "--force", "--platform", "weibo", "--id", "QAu4U9USk"}, "/tmp/project", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("run() code = %d, stderr = %s", code, stderr.String())
	}
	var got c.Record
	if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
		t.Fatalf("json.Unmarshal(stdout) error = %v", err)
	}
	if got.Output.Summary != "forced summary" {
		t.Fatalf("Summary = %q, want forced summary", got.Output.Summary)
	}
}

func TestRunCompileSweepCompilesOnlyUncompiledCapturesAndBackfillsMemory(t *testing.T) {
	prevNewIngestRuntime := newIngestRuntime
	prevBuildCompileClientCurrent := buildCompileClientCurrent
	t.Cleanup(func() {
		newIngestRuntime = prevNewIngestRuntime
		buildCompileClientCurrent = prevBuildCompileClientCurrent
	})

	tmp := t.TempDir()
	dbPath := tmp + "/content.db"
	store, err := contentstore.NewSQLiteStore(dbPath)
	if err != nil {
		t.Fatalf("NewSQLiteStore() error = %v", err)
	}
	for _, raw := range []types.RawContent{
		{Source: "twitter", ExternalID: "raw-new", Content: "CPI cooled again, yields may fall.", URL: "https://x.com/a/status/raw-new", PostedAt: time.Date(2026, 4, 16, 9, 0, 0, 0, time.UTC)},
		{Source: "twitter", ExternalID: "raw-old", Content: "Already compiled.", URL: "https://x.com/a/status/raw-old", PostedAt: time.Date(2026, 4, 15, 9, 0, 0, 0, time.UTC)},
	} {
		if err := store.UpsertRawCapture(context.Background(), raw); err != nil {
			t.Fatalf("UpsertRawCapture(%s) error = %v", raw.ExternalID, err)
		}
	}
	if err := store.UpsertCompiledOutput(context.Background(), c.Record{
		UnitID:         "twitter:raw-old",
		Source:         "twitter",
		ExternalID:     "raw-old",
		RootExternalID: "raw-old",
		Model:          "test-model",
		Output: c.Output{
			Summary: "old summary",
			Graph: c.ReasoningGraph{
				Nodes: []c.GraphNode{testGraphNode("old-1", c.NodeFact, "old fact"), testGraphNode("old-2", c.NodeConclusion, "old conclusion")},
				Edges: []c.GraphEdge{{From: "old-1", To: "old-2", Kind: c.EdgeDerives}},
			},
			Details: c.HiddenDetails{Caveats: []string{"old caveat"}},
		},
		CompiledAt: time.Date(2026, 4, 16, 8, 0, 0, 0, time.UTC),
	}); err != nil {
		t.Fatalf("UpsertCompiledOutput(raw-old) error = %v", err)
	}
	store.Close()

	newIngestRuntime = func(projectRoot string) (*ingest.Runtime, error) {
		store, err := contentstore.NewSQLiteStore(dbPath)
		if err != nil {
			return nil, err
		}
		app := &ingest.Runtime{Store: store}
		app.Settings.ContentDBPath = dbPath
		return app, nil
	}

	var compiled []string
	buildCompileClientCurrent = func(projectRoot string) compileClient {
		return fakeCompileClient{
			compileFn: func(_ context.Context, bundle c.Bundle) (c.Record, error) {
				compiled = append(compiled, bundle.ExternalID)
				return c.Record{
					UnitID:         bundle.Source + ":" + bundle.ExternalID,
					Source:         bundle.Source,
					ExternalID:     bundle.ExternalID,
					RootExternalID: bundle.ExternalID,
					Model:          "test-model",
					Output: c.Output{
						Summary: "compiled " + bundle.ExternalID,
						Graph: c.ReasoningGraph{
							Nodes: []c.GraphNode{
								{ID: "n1", Kind: c.NodeFact, Text: bundle.Content, OccurredAt: bundle.PostedAt},
								{ID: "n2", Kind: c.NodeConclusion, Text: "memory conclusion", PredictionStartAt: bundle.PostedAt, PredictionDueAt: bundle.PostedAt.Add(24 * time.Hour)},
							},
							Edges: []c.GraphEdge{{From: "n1", To: "n2", Kind: c.EdgeDerives}},
						},
						Details: c.HiddenDetails{Caveats: []string{"sweep caveat"}},
					},
					CompiledAt: time.Date(2026, 4, 16, 10, 0, 0, 0, time.UTC),
				}, nil
			},
		}
	}

	var stdout, stderr bytes.Buffer
	code := run([]string{"compile", "sweep", "--user", "u-sweep", "--limit", "10", "--workers", "2"}, "/tmp/project", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("compile sweep code = %d, stderr = %s", code, stderr.String())
	}
	var summary struct {
		Scanned                 int    `json:"scanned"`
		Compiled                int    `json:"compiled"`
		Failed                  int    `json:"failed"`
		ContentGraphsBackfilled int    `json:"content_graphs_backfilled"`
		User                    string `json:"user"`
		Status                  string `json:"status"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &summary); err != nil {
		t.Fatalf("json.Unmarshal(compile sweep stdout) error = %v", err)
	}
	if summary.Scanned != 1 || summary.Compiled != 1 || summary.Failed != 0 || summary.ContentGraphsBackfilled != 1 || summary.User != "u-sweep" || summary.Status != "ok" {
		t.Fatalf("summary = %#v, want one successful compile/backfill", summary)
	}
	if len(compiled) != 1 || compiled[0] != "raw-new" {
		t.Fatalf("compiled = %#v, want only raw-new", compiled)
	}

	store, err = contentstore.NewSQLiteStore(dbPath)
	if err != nil {
		t.Fatalf("NewSQLiteStore(reopen) error = %v", err)
	}
	defer store.Close()
	if got, err := store.GetCompiledOutput(context.Background(), "twitter", "raw-new"); err != nil || got.Output.Summary != "compiled raw-new" {
		t.Fatalf("GetCompiledOutput(raw-new) = %#v, %v", got, err)
	}
	graphs, err := store.ListMemoryContentGraphs(context.Background(), "u-sweep")
	if err != nil {
		t.Fatalf("ListMemoryContentGraphs() error = %v", err)
	}
	if len(graphs) != 1 || graphs[0].SourceExternalID != "raw-new" {
		t.Fatalf("memory graphs = %#v, want raw-new backfilled", graphs)
	}
}

func TestRunHarnessPersistsNoNetworkIngestCompileAndMemoryFlow(t *testing.T) {
	prevNewIngestRuntime := newIngestRuntime
	prevBuildCompileClientCurrent := buildCompileClientCurrent
	prevOpenSQLiteStore := openSQLiteStore
	t.Cleanup(func() {
		newIngestRuntime = prevNewIngestRuntime
		buildCompileClientCurrent = prevBuildCompileClientCurrent
		openSQLiteStore = prevOpenSQLiteStore
	})

	tmp := t.TempDir()
	dbPath := tmp + "/content.db"
	rawURL := "https://x.com/VarixHarness/status/12345"
	rawCapture := types.RawContent{
		Source:     "twitter",
		ExternalID: "12345",
		Content:    "CPI cooled again, so yields may fall and equities could re-rate.",
		AuthorName: "Macro Alice",
		URL:        rawURL,
		PostedAt:   time.Date(2026, 4, 15, 9, 30, 0, 0, time.UTC),
	}
	compiledAt := time.Date(2026, 4, 16, 8, 0, 0, 0, time.UTC)

	openStore := func(t *testing.T) *contentstore.SQLiteStore {
		t.Helper()
		store, err := contentstore.NewSQLiteStore(dbPath)
		if err != nil {
			t.Fatalf("NewSQLiteStore(%q) error = %v", dbPath, err)
		}
		return store
	}

	var fetchCount int
	newIngestRuntime = func(projectRoot string) (*ingest.Runtime, error) {
		store, err := contentstore.NewSQLiteStore(dbPath)
		if err != nil {
			return nil, err
		}
		dispatch := dispatcher.New(
			func(raw string) (types.ParsedURL, error) {
				return types.ParsedURL{
					Platform:     types.PlatformTwitter,
					ContentType:  types.ContentTypePost,
					PlatformID:   rawCapture.ExternalID,
					CanonicalURL: raw,
				}, nil
			},
			[]dispatcher.ItemSource{fakeItemSource{
				platform: types.PlatformTwitter,
				kind:     types.KindNative,
				items: []types.RawContent{{
					Source:     rawCapture.Source,
					ExternalID: rawCapture.ExternalID,
					Content:    rawCapture.Content,
					AuthorName: rawCapture.AuthorName,
					URL:        rawCapture.URL,
					PostedAt:   rawCapture.PostedAt,
				}},
			}},
			nil,
			nil,
		)
		app := &ingest.Runtime{
			Dispatcher: dispatch,
			Polling: polling.New(
				store,
				dispatch,
				nil,
			),
		}
		app.Settings.ContentDBPath = dbPath
		return app, nil
	}

	buildCompileClientCurrent = func(projectRoot string) compileClient {
		return fakeCompileClient{
			compileFn: func(_ context.Context, bundle c.Bundle) (c.Record, error) {
				fetchCount++
				if bundle.Source != rawCapture.Source {
					t.Fatalf("bundle.Source = %q, want %q", bundle.Source, rawCapture.Source)
				}
				if bundle.ExternalID != rawCapture.ExternalID {
					t.Fatalf("bundle.ExternalID = %q, want %q", bundle.ExternalID, rawCapture.ExternalID)
				}
				if bundle.Content != rawCapture.Content {
					t.Fatalf("bundle.Content = %q, want persisted raw capture content", bundle.Content)
				}
				return c.Record{
					UnitID:         "twitter:12345",
					Source:         rawCapture.Source,
					ExternalID:     rawCapture.ExternalID,
					RootExternalID: rawCapture.ExternalID,
					Model:          varixllm.Qwen36PlusModel,
					Metrics:        c.RecordMetrics{CompileElapsedMS: 123, CompileStageElapsedMS: map[string]int64{"unified_generator": 11, "unified_challenge": 22, "unified_judge": 33}},
					Output: c.Output{
						Summary: "Cooling CPI points to lower yields and a bullish risk setup.",
						Graph: c.ReasoningGraph{
							Nodes: []c.GraphNode{
								{ID: "n1", Kind: c.NodeFact, Text: "CPI cooled again", OccurredAt: rawCapture.PostedAt},
								{ID: "n2", Kind: c.NodeConclusion, Text: "Yields may fall"},
								{ID: "n3", Kind: c.NodePrediction, Text: "Equities may re-rate higher", PredictionStartAt: rawCapture.PostedAt},
							},
							Edges: []c.GraphEdge{
								{From: "n1", To: "n2", Kind: c.EdgeDerives},
								{From: "n2", To: "n3", Kind: c.EdgeDerives},
							},
						},
						Details: c.HiddenDetails{Caveats: []string{"Macro path can reverse quickly."}},
					},
					CompiledAt: compiledAt,
				}, nil
			},
		}
	}

	openSQLiteStore = func(path string) (*contentstore.SQLiteStore, error) {
		return contentstore.NewSQLiteStore(path)
	}

	app, err := newIngestRuntime("/tmp/project")
	if err != nil {
		t.Fatalf("newIngestRuntime() error = %v", err)
	}
	fetched, err := app.Polling.FetchURL(context.Background(), rawURL)
	if err != nil {
		t.Fatalf("app.Polling.FetchURL() error = %v", err)
	}
	if len(fetched) != 1 || fetched[0].ExternalID != rawCapture.ExternalID {
		t.Fatalf("FetchURL() = %#v, want one persisted raw capture", fetched)
	}

	store := openStore(t)
	persistedRaw, err := store.GetRawCapture(context.Background(), rawCapture.Source, rawCapture.ExternalID)
	store.Close()
	if err != nil {
		t.Fatalf("GetRawCapture() error = %v", err)
	}
	if persistedRaw.Content != rawCapture.Content {
		t.Fatalf("persisted raw content = %q, want %q", persistedRaw.Content, rawCapture.Content)
	}

	var stdout, stderr bytes.Buffer
	code := run([]string{"compile", "run", "--platform", rawCapture.Source, "--id", rawCapture.ExternalID}, "/tmp/project", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("compile run code = %d, stderr = %s", code, stderr.String())
	}
	if fetchCount != 1 {
		t.Fatalf("compile client calls = %d, want 1", fetchCount)
	}
	var compiled c.Record
	if err := json.Unmarshal(stdout.Bytes(), &compiled); err != nil {
		t.Fatalf("json.Unmarshal(compile stdout) error = %v", err)
	}
	if compiled.Output.Summary == "" {
		t.Fatalf("compiled stdout = %#v, want summary", compiled)
	}
	if compiled.Metrics.CompileElapsedMS <= 0 {
		t.Fatalf("compiled metrics = %#v, want positive compile elapsed ms", compiled.Metrics)
	}
	for _, stage := range []string{"unified_generator", "unified_challenge", "unified_judge"} {
		if compiled.Metrics.CompileStageElapsedMS[stage] <= 0 {
			t.Fatalf("compiled stage metrics = %#v, want positive duration for %q", compiled.Metrics.CompileStageElapsedMS, stage)
		}
	}

	store = openStore(t)
	persistedCompiled, err := store.GetCompiledOutput(context.Background(), rawCapture.Source, rawCapture.ExternalID)
	store.Close()
	if err != nil {
		t.Fatalf("GetCompiledOutput() error = %v", err)
	}
	if persistedCompiled.Output.Summary != compiled.Output.Summary {
		t.Fatalf("persisted compiled summary = %q, want %q", persistedCompiled.Output.Summary, compiled.Output.Summary)
	}
	if persistedCompiled.Metrics.CompileElapsedMS <= 0 {
		t.Fatalf("persisted compiled metrics = %#v, want positive compile elapsed ms", persistedCompiled.Metrics)
	}
	for _, stage := range []string{"unified_generator", "unified_challenge", "unified_judge"} {
		if persistedCompiled.Metrics.CompileStageElapsedMS[stage] <= 0 {
			t.Fatalf("persisted stage metrics = %#v, want positive duration for %q", persistedCompiled.Metrics.CompileStageElapsedMS, stage)
		}
	}

	stdout.Reset()
	stderr.Reset()
	code = run([]string{"memory", "accept-batch", "--user", "u-harness", "--platform", rawCapture.Source, "--id", rawCapture.ExternalID, "--nodes", "n1,n2"}, "/tmp/project", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("memory accept-batch code = %d, stderr = %s", code, stderr.String())
	}
	var accepted memory.AcceptResult
	if err := json.Unmarshal(stdout.Bytes(), &accepted); err != nil {
		t.Fatalf("json.Unmarshal(accept-batch stdout) error = %v", err)
	}
	if len(accepted.Nodes) != 2 {
		t.Fatalf("accept-batch nodes = %#v, want 2", accepted.Nodes)
	}

	stdout.Reset()
	stderr.Reset()
	code = run([]string{"memory", "organize-run", "--user", "u-harness"}, "/tmp/project", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("memory organize-run code = %d, stderr = %s", code, stderr.String())
	}
	var organized memory.OrganizationOutput
	if err := json.Unmarshal(stdout.Bytes(), &organized); err != nil {
		t.Fatalf("json.Unmarshal(organize-run stdout) error = %v", err)
	}
	if organized.JobID == 0 {
		t.Fatalf("organize-run stdout = %#v, want job id", organized)
	}

	store = openStore(t)
	defer store.Close()
	nodes, err := store.ListUserMemoryBySource(context.Background(), "u-harness", rawCapture.Source, rawCapture.ExternalID)
	if err != nil {
		t.Fatalf("ListUserMemoryBySource() error = %v", err)
	}
	if len(nodes) != 2 {
		t.Fatalf("len(ListUserMemoryBySource) = %d, want 2", len(nodes))
	}
	persistedOutput, err := store.GetLatestMemoryOrganizationOutput(context.Background(), "u-harness", rawCapture.Source, rawCapture.ExternalID)
	if err != nil {
		t.Fatalf("GetLatestMemoryOrganizationOutput() error = %v", err)
	}
	if persistedOutput.JobID != organized.JobID {
		t.Fatalf("persisted JobID = %d, want %d", persistedOutput.JobID, organized.JobID)
	}
	if len(persistedOutput.ActiveNodes) != 2 {
		t.Fatalf("len(persisted active nodes) = %d, want 2", len(persistedOutput.ActiveNodes))
	}
	if len(persistedOutput.Hierarchy) == 0 {
		t.Fatalf("persisted hierarchy = %#v, want derived links", persistedOutput.Hierarchy)
	}
	if fetchCount != 1 {
		t.Fatalf("unexpected refetch/compile count = %d, want 1 total compile invocation", fetchCount)
	}
}
