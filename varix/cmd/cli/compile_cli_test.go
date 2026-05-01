package main

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/kumaloha/VariX/varix/bootstrap"
	c "github.com/kumaloha/VariX/varix/compile"
	"github.com/kumaloha/VariX/varix/ingest/dispatcher"
	"github.com/kumaloha/VariX/varix/ingest/polling"
	"github.com/kumaloha/VariX/varix/ingest/types"
	"github.com/kumaloha/VariX/varix/memory"
	"github.com/kumaloha/VariX/varix/storage/contentstore"
	"testing"
	"time"
)

func TestRunCompileWritesCompiledRecordJSON(t *testing.T) {
	prevBuildApp := buildApp
	prevBuildCompileClient := buildCompileClient
	prevBuildCompileClientNoVerify := buildCompileClientNoVerify
	prevBuildCompileClientV2 := buildCompileClientV2
	prevOpenSQLiteStore := openSQLiteStore
	t.Cleanup(func() {
		buildApp = prevBuildApp
		buildCompileClient = prevBuildCompileClient
		buildCompileClientNoVerify = prevBuildCompileClientNoVerify
		buildCompileClientV2 = prevBuildCompileClientV2
		openSQLiteStore = prevOpenSQLiteStore
	})

	tmp := t.TempDir()
	buildApp = func(projectRoot string) (*bootstrap.App, error) {
		src := fakeItemSource{
			items: []types.RawContent{{
				Source:     "weibo",
				ExternalID: "QAu4U9USk",
				Content:    "hello",
				AuthorName: "Alice",
				URL:        "https://weibo.com/1182426800/QAu4U9USk",
			}},
		}
		app := &bootstrap.App{
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

	buildCompileClient = func(projectRoot string) compileClient {
		return fakeCompileClient{
			record: c.Record{
				UnitID:         "weibo:QAu4U9USk",
				Source:         "weibo",
				ExternalID:     "QAu4U9USk",
				RootExternalID: "QAu4U9USk",
				Model:          c.Qwen36PlusModel,
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
	if got.Output.Summary != "一句话" {
		t.Fatalf("Summary = %q", got.Output.Summary)
	}
}

func TestRunCompilePipelineV2UsesV2Client(t *testing.T) {
	prevBuildApp := buildApp
	prevBuildCompileClient := buildCompileClient
	prevBuildCompileClientNoVerify := buildCompileClientNoVerify
	prevBuildCompileClientV2 := buildCompileClientV2
	prevOpenSQLiteStore := openSQLiteStore
	t.Cleanup(func() {
		buildApp = prevBuildApp
		buildCompileClient = prevBuildCompileClient
		buildCompileClientNoVerify = prevBuildCompileClientNoVerify
		buildCompileClientV2 = prevBuildCompileClientV2
		openSQLiteStore = prevOpenSQLiteStore
	})

	tmp := t.TempDir()
	buildApp = func(projectRoot string) (*bootstrap.App, error) {
		app := &bootstrap.App{}
		app.Settings.ContentDBPath = tmp + "/content.db"
		src := fakeItemSource{
			items: []types.RawContent{{
				Source:     "web",
				ExternalID: "v2-id",
				Content:    "hello",
				URL:        "https://example.com/v2",
			}},
		}
		app.Dispatcher = dispatcher.New(
			func(raw string) (types.ParsedURL, error) {
				return types.ParsedURL{Platform: types.PlatformWeb, ContentType: types.ContentTypePost, PlatformID: "v2-id", CanonicalURL: raw}, nil
			},
			[]dispatcher.ItemSource{src},
			nil,
			nil,
		)
		return app, nil
	}
	buildCompileClient = func(projectRoot string) compileClient {
		t.Fatal("legacy compile client should not be used")
		return nil
	}
	buildCompileClientV2 = func(projectRoot string) compileClient {
		return fakeCompileClient{record: c.Record{
			UnitID:     "web:v2-id",
			Source:     "web",
			ExternalID: "v2-id",
			Model:      "qwen3.6-plus",
			Metrics:    c.RecordMetrics{CompileElapsedMS: 777, CompileStageElapsedMS: map[string]int64{"extract": 101, "refine": 102, "aggregate": 103, "support": 104, "collapse": 105, "relations": 106, "classify": 107, "render": 108}},
			Output: c.Output{
				Summary: "v2 summary",
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
	code := run([]string{"compile", "run", "--pipeline", "v2", "--url", "https://example.com/v2"}, "/tmp/project", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("run() code = %d, stderr = %s", code, stderr.String())
	}
	var got c.Record
	if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
		t.Fatalf("json.Unmarshal(stdout) error = %v", err)
	}
	if got.Output.Summary != "v2 summary" {
		t.Fatalf("Summary = %q, want v2 summary", got.Output.Summary)
	}
	if got.Metrics.CompileElapsedMS != 777 {
		t.Fatalf("CompileElapsedMS = %d, want 777", got.Metrics.CompileElapsedMS)
	}
	for _, stage := range []string{"extract", "refine", "aggregate", "support", "collapse", "relations", "classify", "render"} {
		if got.Metrics.CompileStageElapsedMS[stage] <= 0 {
			t.Fatalf("CompileStageElapsedMS = %#v, want positive persisted v2 stage metric for %q", got.Metrics.CompileStageElapsedMS, stage)
		}
	}
	if _, ok := got.Metrics.CompileStageElapsedMS["validate"]; ok {
		t.Fatalf("CompileStageElapsedMS = %#v, compile metrics must not include validate", got.Metrics.CompileStageElapsedMS)
	}
	stdout.Reset()
	stderr.Reset()
	code = run([]string{"compile", "show", "--platform", "web", "--id", "v2-id"}, "/tmp/project", &stdout, &stderr)
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
	for _, stage := range []string{"extract", "refine", "aggregate", "support", "collapse", "relations", "classify", "render"} {
		if shown.Metrics.CompileStageElapsedMS[stage] <= 0 {
			t.Fatalf("shown CompileStageElapsedMS = %#v, want positive persisted v2 stage metric for %q", shown.Metrics.CompileStageElapsedMS, stage)
		}
	}
	if _, ok := shown.Metrics.CompileStageElapsedMS["validate"]; ok {
		t.Fatalf("shown CompileStageElapsedMS = %#v, compile metrics must not include validate", shown.Metrics.CompileStageElapsedMS)
	}
}

func TestSelectCompileClientKeepsLegacyPipelineIsolated(t *testing.T) {
	prevBuildCompileClient := buildCompileClient
	prevBuildCompileClientNoVerify := buildCompileClientNoVerify
	prevBuildCompileClientV2 := buildCompileClientV2
	t.Cleanup(func() {
		buildCompileClient = prevBuildCompileClient
		buildCompileClientNoVerify = prevBuildCompileClientNoVerify
		buildCompileClientV2 = prevBuildCompileClientV2
	})

	legacyCalls := 0
	noVerifyCalls := 0
	v2Calls := 0
	buildCompileClient = func(projectRoot string) compileClient {
		legacyCalls++
		return fakeCompileClient{}
	}
	buildCompileClientNoVerify = func(projectRoot string) compileClient {
		noVerifyCalls++
		return fakeCompileClient{}
	}
	buildCompileClientV2 = func(projectRoot string) compileClient {
		v2Calls++
		return fakeCompileClient{}
	}

	if _, err := selectCompileClient("/tmp/project", "", false); err != nil {
		t.Fatalf("selectCompileClient(default legacy) error = %v", err)
	}
	if _, err := selectCompileClient("/tmp/project", "legacy", false); err != nil {
		t.Fatalf("selectCompileClient(explicit legacy) error = %v", err)
	}
	if _, err := selectCompileClient("/tmp/project", "legacy", true); err != nil {
		t.Fatalf("selectCompileClient(no verify) error = %v", err)
	}

	if legacyCalls != 2 {
		t.Fatalf("legacy builder calls = %d, want 2", legacyCalls)
	}
	if noVerifyCalls != 1 {
		t.Fatalf("no-verify builder calls = %d, want 1", noVerifyCalls)
	}
	if v2Calls != 0 {
		t.Fatalf("v2 builder calls = %d, want 0 for legacy pipeline selections", v2Calls)
	}
}

func TestSelectCompileClientUsesV2OnlyWhenRequested(t *testing.T) {
	prevBuildCompileClient := buildCompileClient
	prevBuildCompileClientNoVerify := buildCompileClientNoVerify
	prevBuildCompileClientV2 := buildCompileClientV2
	t.Cleanup(func() {
		buildCompileClient = prevBuildCompileClient
		buildCompileClientNoVerify = prevBuildCompileClientNoVerify
		buildCompileClientV2 = prevBuildCompileClientV2
	})

	legacyCalls := 0
	v2Calls := 0
	buildCompileClient = func(projectRoot string) compileClient {
		legacyCalls++
		return fakeCompileClient{}
	}
	buildCompileClientNoVerify = func(projectRoot string) compileClient {
		t.Fatal("legacy no-verify builder should not be used for v2 pipeline")
		return nil
	}
	buildCompileClientV2 = func(projectRoot string) compileClient {
		v2Calls++
		return fakeCompileClient{}
	}

	if _, err := selectCompileClient("/tmp/project", "v2", false); err != nil {
		t.Fatalf("selectCompileClient(v2) error = %v", err)
	}
	if v2Calls != 1 {
		t.Fatalf("v2 builder calls = %d, want 1", v2Calls)
	}
	if legacyCalls != 0 {
		t.Fatalf("legacy builder calls = %d, want 0 for v2 selection", legacyCalls)
	}

	if _, err := selectCompileClient("/tmp/project", "v2", true); err == nil {
		t.Fatal("selectCompileClient(v2, --no-verify) error = nil, want unsupported flag error")
	}
	if v2Calls != 1 {
		t.Fatalf("v2 builder calls after unsupported flag checks = %d, want 1", v2Calls)
	}
}

func TestRunCompileReadsExistingRawCaptureByPlatformAndID(t *testing.T) {
	prevBuildApp := buildApp
	prevBuildCompileClient := buildCompileClient
	prevOpenSQLiteStore := openSQLiteStore
	t.Cleanup(func() {
		buildApp = prevBuildApp
		buildCompileClient = prevBuildCompileClient
		openSQLiteStore = prevOpenSQLiteStore
	})

	tmp := t.TempDir()
	buildApp = func(projectRoot string) (*bootstrap.App, error) {
		app := &bootstrap.App{}
		app.Settings.ContentDBPath = tmp + "/content.db"
		return app, nil
	}
	buildCompileClient = func(projectRoot string) compileClient {
		return fakeCompileClient{
			record: c.Record{
				UnitID:         "weibo:QAu4U9USk",
				Source:         "weibo",
				ExternalID:     "QAu4U9USk",
				RootExternalID: "QAu4U9USk",
				Model:          c.Qwen36PlusModel,
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
	prevBuildApp := buildApp
	prevBuildCompileClient := buildCompileClient
	prevOpenSQLiteStore := openSQLiteStore
	t.Cleanup(func() {
		buildApp = prevBuildApp
		buildCompileClient = prevBuildCompileClient
		openSQLiteStore = prevOpenSQLiteStore
	})

	tmp := t.TempDir()
	buildApp = func(projectRoot string) (*bootstrap.App, error) {
		app := &bootstrap.App{}
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
	buildCompileClient = func(projectRoot string) compileClient {
		return fakeCompileClient{
			record: c.Record{
				UnitID:         "twitter:2026305745872998803",
				Source:         "twitter",
				ExternalID:     "2026305745872998803",
				RootExternalID: "2026305745872998803",
				Model:          c.Qwen36PlusModel,
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
	prevBuildApp := buildApp
	prevBuildCompileClient := buildCompileClient
	prevOpenSQLiteStore := openSQLiteStore
	t.Cleanup(func() {
		buildApp = prevBuildApp
		buildCompileClient = prevBuildCompileClient
		openSQLiteStore = prevOpenSQLiteStore
	})

	tmp := t.TempDir()
	buildApp = func(projectRoot string) (*bootstrap.App, error) {
		app := &bootstrap.App{}
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
	buildCompileClient = func(projectRoot string) compileClient {
		return fakeCompileClient{
			record: c.Record{
				UnitID:         "twitter:2026305745872998803",
				Source:         "twitter",
				ExternalID:     "2026305745872998803",
				RootExternalID: "2026305745872998803",
				Model:          c.Qwen36PlusModel,
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
			Model:          c.Qwen36PlusModel,
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
	prevBuildApp := buildApp
	prevBuildCompileClient := buildCompileClient
	prevOpenSQLiteStore := openSQLiteStore
	t.Cleanup(func() {
		buildApp = prevBuildApp
		buildCompileClient = prevBuildCompileClient
		openSQLiteStore = prevOpenSQLiteStore
	})

	tmp := t.TempDir()
	buildApp = func(projectRoot string) (*bootstrap.App, error) {
		app := &bootstrap.App{}
		app.Settings.ContentDBPath = tmp + "/content.db"
		return app, nil
	}
	buildCompileClient = func(projectRoot string) compileClient {
		return fakeCompileClient{
			record: c.Record{
				UnitID:         "weibo:QAu4U9USk",
				Source:         "weibo",
				ExternalID:     "QAu4U9USk",
				RootExternalID: "QAu4U9USk",
				Model:          c.Qwen36PlusModel,
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
			Model:          c.Qwen36PlusModel,
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

func TestRunHarnessPersistsNoNetworkIngestCompileAndMemoryFlow(t *testing.T) {
	prevBuildApp := buildApp
	prevBuildCompileClient := buildCompileClient
	prevOpenSQLiteStore := openSQLiteStore
	t.Cleanup(func() {
		buildApp = prevBuildApp
		buildCompileClient = prevBuildCompileClient
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
	buildApp = func(projectRoot string) (*bootstrap.App, error) {
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
		app := &bootstrap.App{
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

	buildCompileClient = func(projectRoot string) compileClient {
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
					Model:          c.Qwen36PlusModel,
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

	app, err := buildApp("/tmp/project")
	if err != nil {
		t.Fatalf("buildApp() error = %v", err)
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
