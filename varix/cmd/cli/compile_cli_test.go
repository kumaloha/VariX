package main

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"github.com/kumaloha/VariX/varix/bootstrap"
	c "github.com/kumaloha/VariX/varix/compile"
	"github.com/kumaloha/VariX/varix/graphmodel"
	"github.com/kumaloha/VariX/varix/ingest/dispatcher"
	"github.com/kumaloha/VariX/varix/ingest/polling"
	"github.com/kumaloha/VariX/varix/ingest/types"
	"github.com/kumaloha/VariX/varix/memory"
	"github.com/kumaloha/VariX/varix/storage/contentstore"
	"os"
	"path/filepath"
	"strings"
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

func TestRunCompileShowReadsCompiledRecordByPlatformAndID(t *testing.T) {
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
		app.Dispatcher = dispatcher.New(
			func(raw string) (types.ParsedURL, error) {
				return types.ParsedURL{Platform: types.PlatformWeibo, PlatformID: "QAu4U9USk", CanonicalURL: raw}, nil
			},
			[]dispatcher.ItemSource{fakeItemSource{}},
			nil,
			nil,
		)
		return app, nil
	}
	openSQLiteStore = func(path string) (*contentstore.SQLiteStore, error) {
		store, err := contentstore.NewSQLiteStore(path)
		if err != nil {
			return nil, err
		}
		record := c.Record{
			UnitID:         "weibo:QAu4U9USk",
			Source:         "weibo",
			ExternalID:     "QAu4U9USk",
			RootExternalID: "QAu4U9USk",
			Model:          c.Qwen36PlusModel,
			Metrics:        c.RecordMetrics{CompileElapsedMS: 321, CompileStageElapsedMS: map[string]int64{"unified_generator": 111, "unified_challenge": 99, "unified_judge": 88}},
			Output: c.Output{
				Summary: "一句话",
				Graph: c.ReasoningGraph{
					Nodes: []c.GraphNode{testGraphNode("n1", c.NodeFact, "事实A"), testGraphNode("n2", c.NodeConclusion, "结论B")},
					Edges: []c.GraphEdge{{From: "n1", To: "n2", Kind: c.EdgeDerives}},
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
	code := run([]string{"compile", "show", "--platform", "weibo", "--id", "QAu4U9USk"}, "/tmp/project", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("run() code = %d, stderr = %s", code, stderr.String())
	}
	var got c.Record
	if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
		t.Fatalf("json.Unmarshal(stdout) error = %v", err)
	}
	if got.Output.Summary != "一句话" {
		t.Fatalf("Summary = %q", got.Output.Summary)
	}
	if got.Metrics.CompileElapsedMS != 321 {
		t.Fatalf("CompileElapsedMS = %d, want 321", got.Metrics.CompileElapsedMS)
	}
	if got.Metrics.CompileStageElapsedMS["unified_generator"] != 111 || got.Metrics.CompileStageElapsedMS["unified_challenge"] != 99 || got.Metrics.CompileStageElapsedMS["unified_judge"] != 88 {
		t.Fatalf("CompileStageElapsedMS = %#v, want persisted stage metrics", got.Metrics.CompileStageElapsedMS)
	}
}

func TestRunCompileShowReadsCompiledRecordByURL(t *testing.T) {
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
		app.Dispatcher = dispatcher.New(
			func(raw string) (types.ParsedURL, error) {
				return types.ParsedURL{Platform: types.PlatformTwitter, PlatformID: "2026305745872998803", CanonicalURL: raw}, nil
			},
			[]dispatcher.ItemSource{fakeItemSource{}},
			nil,
			nil,
		)
		return app, nil
	}
	openSQLiteStore = func(path string) (*contentstore.SQLiteStore, error) {
		store, err := contentstore.NewSQLiteStore(path)
		if err != nil {
			return nil, err
		}
		record := c.Record{
			UnitID:         "twitter:2026305745872998803",
			Source:         "twitter",
			ExternalID:     "2026305745872998803",
			RootExternalID: "2026305745872998803",
			Model:          c.Qwen36PlusModel,
			Metrics:        c.RecordMetrics{CompileElapsedMS: 654, CompileStageElapsedMS: map[string]int64{"unified_generator": 222, "unified_challenge": 211, "unified_judge": 201}},
			Output: c.Output{
				Summary: "Dalio summary",
				Graph: c.ReasoningGraph{
					Nodes: []c.GraphNode{testGraphNode("n1", c.NodeFact, "事实A"), testGraphNode("n2", c.NodeConclusion, "结论B")},
					Edges: []c.GraphEdge{{From: "n1", To: "n2", Kind: c.EdgeDerives}},
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
	code := run([]string{"compile", "show", "--url", "https://x.com/RayDalio/status/2026305745872998803"}, "/tmp/project", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("run() code = %d, stderr = %s", code, stderr.String())
	}
	var got c.Record
	if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
		t.Fatalf("json.Unmarshal(stdout) error = %v", err)
	}
	if got.Output.Summary != "Dalio summary" {
		t.Fatalf("Summary = %q", got.Output.Summary)
	}
	if got.Metrics.CompileElapsedMS != 654 {
		t.Fatalf("CompileElapsedMS = %d, want 654", got.Metrics.CompileElapsedMS)
	}
	if got.Metrics.CompileStageElapsedMS["unified_generator"] != 222 || got.Metrics.CompileStageElapsedMS["unified_challenge"] != 211 || got.Metrics.CompileStageElapsedMS["unified_judge"] != 201 {
		t.Fatalf("CompileStageElapsedMS = %#v, want persisted URL stage metrics", got.Metrics.CompileStageElapsedMS)
	}
}

func TestRunCompileSummaryPrintsHumanReadableOutput(t *testing.T) {
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
			UnitID:         "weibo:QAu4U9USk",
			Source:         "weibo",
			ExternalID:     "QAu4U9USk",
			RootExternalID: "QAu4U9USk",
			Model:          c.Qwen36PlusModel,
			Output: c.Output{
				Summary:           "一句话",
				Drivers:           []string{"驱动A"},
				Targets:           []string{"目标B"},
				TransmissionPaths: []c.TransmissionPath{{Driver: "驱动A", Target: "目标B", Steps: []string{"中间步骤"}}},
				Graph: c.ReasoningGraph{
					Nodes: []c.GraphNode{testGraphNode("n1", c.NodeMechanism, "驱动A"), testGraphNode("n2", c.NodeConclusion, "目标B")},
					Edges: []c.GraphEdge{{From: "n1", To: "n2", Kind: c.EdgePositive}},
				},
				Details:    c.HiddenDetails{Caveats: []string{"说明"}},
				Topics:     []string{"topic-a", "topic-b"},
				Confidence: "medium",
			},
			CompiledAt: time.Now().UTC(),
		}
		if err := store.UpsertCompiledOutput(context.Background(), record); err != nil {
			return nil, err
		}
		return store, nil
	}

	var stdout, stderr bytes.Buffer
	code := run([]string{"compile", "summary", "--platform", "weibo", "--id", "QAu4U9USk"}, "/tmp/project", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("run() code = %d, stderr = %s", code, stderr.String())
	}
	out := stdout.String()
	for _, want := range []string{"Summary: 一句话", "Drivers: 1", "Targets: 1", "Paths: 1", "Topics: topic-a, topic-b", "Confidence: medium"} {
		if !strings.Contains(out, want) {
			t.Fatalf("stdout missing %q in %q", want, out)
		}
	}
	if !strings.Contains(out, "Compile elapsed:") || !strings.Contains(out, "Stages:") {
		t.Fatalf("stdout = %q, want compile metrics in summary view", out)
	}
}

func TestRunCompileGoldScoreOutputsReviewItems(t *testing.T) {
	tmp := t.TempDir()
	goldPath := filepath.Join(tmp, "gold.json")
	candidatePath := filepath.Join(tmp, "candidate.json")
	gold := c.GoldDataset{
		Version: "test-v1",
		Samples: []c.GoldSample{{
			ID:      "G04",
			Summary: "海外资金继续流入美国资产，说明美国增长叙事仍然吸引全球资金",
			Drivers: []string{
				"美国增长叙事仍然吸引全球资金",
				"政治风险没有压倒市场对美国资产的增长偏好",
			},
			Targets: []string{
				"海外资金继续流入美国资产",
				"没有形成 sell America 交易",
			},
		}},
	}
	candidates := []c.GoldCandidate{{
		SampleID: "G04",
		Output: c.Output{
			Summary: "美国政治风险导致美元走弱",
			Drivers: []string{
				"美国增长叙事吸引全球资金",
				"美联储政治化压低收益率",
			},
			Targets: []string{
				"海外资金继续流入美国资产",
				"美元下跌",
			},
		},
	}}
	writeTestJSONFile(t, goldPath, gold)
	writeTestJSONFile(t, candidatePath, candidates)

	var stdout, stderr bytes.Buffer
	code := run([]string{"compile", "gold-score", "--gold", goldPath, "--candidate", candidatePath}, "/tmp/project", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("run() code = %d, stderr = %s", code, stderr.String())
	}
	var out c.GoldScorecard
	if err := json.Unmarshal(stdout.Bytes(), &out); err != nil {
		t.Fatalf("json.Unmarshal(stdout) error = %v; stdout=%s", err, stdout.String())
	}
	if out.DatasetVersion != "test-v1" || out.SampleCount != 1 {
		t.Fatalf("scorecard = %#v, want dataset metadata", out)
	}
	if len(out.Samples) != 1 || len(out.Samples[0].ReviewItems) == 0 {
		t.Fatalf("scorecard missing review items: %#v", out)
	}
}

func TestRunCompileGoldScoreReadsCandidateDir(t *testing.T) {
	tmp := t.TempDir()
	goldPath := filepath.Join(tmp, "gold.json")
	candidateDir := filepath.Join(tmp, "candidates")
	outPath := filepath.Join(tmp, "scorecard.json")
	if err := os.Mkdir(candidateDir, 0o755); err != nil {
		t.Fatalf("os.Mkdir() error = %v", err)
	}
	gold := c.GoldDataset{
		Version: "test-v1",
		Samples: []c.GoldSample{{
			ID:      "G01",
			Summary: "资金流入硬资产以对冲货币贬值",
			Drivers: []string{
				"央行扩表压低实际利率",
			},
			Targets: []string{
				"资金流入硬资产",
			},
		}},
	}
	report := struct {
		UnitID string   `json:"unit_id"`
		Output c.Output `json:"output"`
	}{
		UnitID: "twitter:1",
		Output: c.Output{
			Summary: "实际利率为负导致资金买入黄金",
			Drivers: []string{
				"实际利率下降",
			},
			Targets: []string{
				"资金买入黄金",
			},
		},
	}
	writeTestJSONFile(t, goldPath, gold)
	writeTestJSONFile(t, filepath.Join(candidateDir, "G01.json"), report)

	var stdout, stderr bytes.Buffer
	code := run([]string{"compile", "gold-score", "--gold", goldPath, "--candidate-dir", candidateDir, "--out", outPath}, "/tmp/project", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("run() code = %d, stderr = %s", code, stderr.String())
	}
	var out c.GoldScorecard
	if err := json.Unmarshal(stdout.Bytes(), &out); err != nil {
		t.Fatalf("json.Unmarshal(stdout) error = %v; stdout=%s", err, stdout.String())
	}
	if out.SampleCount != 1 || len(out.Samples) != 1 {
		t.Fatalf("scorecard = %#v, want one scored sample", out)
	}
	if out.Samples[0].ID != "G01" {
		t.Fatalf("sample id = %q, want G01", out.Samples[0].ID)
	}
	if len(out.Samples[0].ReviewItems) == 0 {
		t.Fatalf("scorecard missing review items: %#v", out)
	}
	rawOut, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("os.ReadFile(outPath) error = %v", err)
	}
	var fileOut c.GoldScorecard
	if err := json.Unmarshal(rawOut, &fileOut); err != nil {
		t.Fatalf("json.Unmarshal(outPath) error = %v; raw=%s", err, string(rawOut))
	}
	if fileOut.DatasetVersion != "test-v1" {
		t.Fatalf("file scorecard version = %q, want test-v1", fileOut.DatasetVersion)
	}
}

func TestRunCompileSummaryReadsByURL(t *testing.T) {
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
		app.Dispatcher = dispatcher.New(
			func(raw string) (types.ParsedURL, error) {
				return types.ParsedURL{Platform: types.PlatformTwitter, PlatformID: "2026305745872998803", CanonicalURL: raw}, nil
			},
			[]dispatcher.ItemSource{fakeItemSource{}},
			nil,
			nil,
		)
		return app, nil
	}
	openSQLiteStore = func(path string) (*contentstore.SQLiteStore, error) {
		store, err := contentstore.NewSQLiteStore(path)
		if err != nil {
			return nil, err
		}
		record := c.Record{
			UnitID:         "twitter:2026305745872998803",
			Source:         "twitter",
			ExternalID:     "2026305745872998803",
			RootExternalID: "2026305745872998803",
			Model:          c.Qwen36PlusModel,
			Output: c.Output{
				Summary:           "Dalio summary",
				Drivers:           []string{"驱动A"},
				Targets:           []string{"目标B"},
				TransmissionPaths: []c.TransmissionPath{{Driver: "驱动A", Target: "目标B", Steps: []string{"中间步骤"}}},
				Graph: c.ReasoningGraph{
					Nodes: []c.GraphNode{testGraphNode("n1", c.NodeMechanism, "驱动A"), testGraphNode("n2", c.NodeConclusion, "目标B")},
					Edges: []c.GraphEdge{{From: "n1", To: "n2", Kind: c.EdgePositive}},
				},
				Details:    c.HiddenDetails{Caveats: []string{"说明"}},
				Topics:     []string{"macro"},
				Confidence: "high",
			},
			CompiledAt: time.Now().UTC(),
		}
		if err := store.UpsertCompiledOutput(context.Background(), record); err != nil {
			return nil, err
		}
		return store, nil
	}

	var stdout, stderr bytes.Buffer
	code := run([]string{"compile", "summary", "--url", "https://x.com/RayDalio/status/2026305745872998803"}, "/tmp/project", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("run() code = %d, stderr = %s", code, stderr.String())
	}
	out := stdout.String()
	for _, want := range []string{"Summary: Dalio summary", "Drivers: 1", "Targets: 1", "Paths: 1", "Topics: macro", "Confidence: high"} {
		if !strings.Contains(out, want) {
			t.Fatalf("stdout missing %q in %q", want, out)
		}
	}
	if !strings.Contains(out, "Compile elapsed:") || !strings.Contains(out, "Stages:") {
		t.Fatalf("stdout = %q, want compile metrics in summary-by-url view", out)
	}
}

func TestRunCompileComparePrintsRawPreviewAndSummary(t *testing.T) {
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
		if err := store.UpsertRawCapture(context.Background(), types.RawContent{
			Source:     "weibo",
			ExternalID: "QAu4U9USk",
			Content:    "原文正文",
			AuthorName: "Alice",
			URL:        "https://weibo.com/1182426800/QAu4U9USk",
		}); err != nil {
			return nil, err
		}
		record := c.Record{
			UnitID:         "weibo:QAu4U9USk",
			Source:         "weibo",
			ExternalID:     "QAu4U9USk",
			RootExternalID: "QAu4U9USk",
			Model:          c.Qwen36PlusModel,
			Output: c.Output{
				Summary:           "一句话",
				Drivers:           []string{"驱动A"},
				Targets:           []string{"目标B"},
				TransmissionPaths: []c.TransmissionPath{{Driver: "驱动A", Target: "目标B", Steps: []string{"中间步骤"}}},
				Graph: c.ReasoningGraph{
					Nodes: []c.GraphNode{testGraphNode("n1", c.NodeMechanism, "驱动A"), testGraphNode("n2", c.NodeConclusion, "目标B")},
					Edges: []c.GraphEdge{{From: "n1", To: "n2", Kind: c.EdgePositive}},
				},
				Details:    c.HiddenDetails{Caveats: []string{"说明"}},
				Confidence: "medium",
			},
			CompiledAt: time.Now().UTC(),
		}
		if err := store.UpsertCompiledOutput(context.Background(), record); err != nil {
			return nil, err
		}
		return store, nil
	}

	var stdout, stderr bytes.Buffer
	code := run([]string{"compile", "compare", "--platform", "weibo", "--id", "QAu4U9USk"}, "/tmp/project", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("run() code = %d, stderr = %s", code, stderr.String())
	}
	out := stdout.String()
	for _, want := range []string{"Raw preview: 原文正文", "Summary: 一句话", "Drivers: 1", "Targets: 1", "Paths: 1", "Confidence: medium"} {
		if !strings.Contains(out, want) {
			t.Fatalf("stdout missing %q in %q", want, out)
		}
	}
	if !strings.Contains(out, "Compile elapsed:") || !strings.Contains(out, "Stages:") {
		t.Fatalf("stdout = %q, want compile metrics in compare view", out)
	}
}

func TestRunCompileCompareReadsByURL(t *testing.T) {
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
		app.Dispatcher = dispatcher.New(
			func(raw string) (types.ParsedURL, error) {
				return types.ParsedURL{Platform: types.PlatformTwitter, PlatformID: "2026305745872998803", CanonicalURL: raw}, nil
			},
			[]dispatcher.ItemSource{fakeItemSource{}},
			nil,
			nil,
		)
		return app, nil
	}
	openSQLiteStore = func(path string) (*contentstore.SQLiteStore, error) {
		store, err := contentstore.NewSQLiteStore(path)
		if err != nil {
			return nil, err
		}
		if err := store.UpsertRawCapture(context.Background(), types.RawContent{
			Source:     "twitter",
			ExternalID: "2026305745872998803",
			Content:    "dalio raw body",
			AuthorName: "Ray Dalio",
			URL:        "https://x.com/RayDalio/status/2026305745872998803",
		}); err != nil {
			return nil, err
		}
		record := c.Record{
			UnitID:         "twitter:2026305745872998803",
			Source:         "twitter",
			ExternalID:     "2026305745872998803",
			RootExternalID: "2026305745872998803",
			Model:          c.Qwen36PlusModel,
			Output: c.Output{
				Summary:           "Dalio summary",
				Drivers:           []string{"驱动A"},
				Targets:           []string{"目标B"},
				TransmissionPaths: []c.TransmissionPath{{Driver: "驱动A", Target: "目标B", Steps: []string{"中间步骤"}}},
				Graph: c.ReasoningGraph{
					Nodes: []c.GraphNode{testGraphNode("n1", c.NodeMechanism, "驱动A"), testGraphNode("n2", c.NodeConclusion, "目标B")},
					Edges: []c.GraphEdge{{From: "n1", To: "n2", Kind: c.EdgePositive}},
				},
				Details:    c.HiddenDetails{Caveats: []string{"说明"}},
				Confidence: "high",
			},
			CompiledAt: time.Now().UTC(),
		}
		if err := store.UpsertCompiledOutput(context.Background(), record); err != nil {
			return nil, err
		}
		return store, nil
	}

	var stdout, stderr bytes.Buffer
	code := run([]string{"compile", "compare", "--url", "https://x.com/RayDalio/status/2026305745872998803"}, "/tmp/project", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("run() code = %d, stderr = %s", code, stderr.String())
	}
	out := stdout.String()
	for _, want := range []string{"Raw preview: dalio raw body", "Summary: Dalio summary", "Drivers: 1", "Targets: 1", "Paths: 1", "Confidence: high"} {
		if !strings.Contains(out, want) {
			t.Fatalf("stdout missing %q in %q", want, out)
		}
	}
	if !strings.Contains(out, "Compile elapsed:") || !strings.Contains(out, "Stages:") {
		t.Fatalf("stdout = %q, want compile metrics in compare-by-url view", out)
	}
}

func TestRunCompileCardPrintsHumanReadableCard(t *testing.T) {
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
			UnitID:         "twitter:1",
			Source:         "twitter",
			ExternalID:     "1",
			RootExternalID: "1",
			Model:          c.Qwen36PlusModel,
			Output: c.Output{
				Summary:           "一句话总结",
				Drivers:           []string{"驱动A"},
				Targets:           []string{"目标B"},
				TransmissionPaths: []c.TransmissionPath{{Driver: "驱动A", Target: "目标B", Steps: []string{"中间步骤"}}},
				Branches: []c.Branch{{
					ID:                "s1",
					Level:             "primary",
					Thesis:            "分支论点",
					Anchors:           []string{"总前提"},
					BranchDrivers:     []string{"分支机制"},
					Drivers:           []string{"驱动A"},
					Targets:           []string{"目标B"},
					TransmissionPaths: []c.TransmissionPath{{Driver: "驱动A", Target: "目标B", Steps: []string{"中间步骤"}}},
				}},
				EvidenceNodes:    []string{"证据A"},
				ExplanationNodes: []string{"解释B"},
				Graph: c.ReasoningGraph{
					Nodes: []c.GraphNode{testGraphNode("n1", c.NodeMechanism, "驱动A"), testGraphNode("n2", c.NodeConclusion, "目标B")},
					Edges: []c.GraphEdge{{From: "n1", To: "n2", Kind: c.EdgePositive}},
				},
				Details:    c.HiddenDetails{Caveats: []string{"说明"}},
				Topics:     []string{"topic-a", "topic-b"},
				Confidence: "high",
				AuthorValidation: c.AuthorValidation{
					Version: "author_validate_v1",
					Summary: c.AuthorValidationSummary{
						Verdict:               "mixed",
						SupportedClaims:       1,
						UnverifiedClaims:      1,
						SoundInferences:       1,
						UnsupportedInferences: 1,
					},
					ClaimChecks: []c.AuthorClaimCheck{{
						ClaimID: "claim-001",
						Text:    "目标B",
						Status:  c.AuthorClaimUnverified,
						Reason:  "缺少外部证据",
					}},
					InferenceChecks: []c.AuthorInferenceCheck{{
						InferenceID: "inference-001",
						From:        "驱动A",
						To:          "目标B",
						Status:      c.AuthorInferenceUnsupportedJump,
						Reason:      "中间条件不成立",
					}},
				},
			},
			CompiledAt: time.Now().UTC(),
		}
		if err := store.UpsertCompiledOutput(context.Background(), record); err != nil {
			return nil, err
		}
		return store, nil
	}

	var stdout, stderr bytes.Buffer
	code := run([]string{"compile", "card", "--platform", "twitter", "--id", "1"}, "/tmp/project", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("run() code = %d, stderr = %s", code, stderr.String())
	}
	out := stdout.String()
	for _, want := range []string{"Summary", "一句话总结", "Topics", "topic-a", "Branches", "分支论点", "Anchor: 总前提", "Branch driver: 分支机制", "驱动A -> 中间步骤 -> 目标B", "Logic chain", "Author validation", "Verdict: mixed", "Claims: supported 1, contradicted 0, unverified 1, interpretive 0", "Claim 目标B: unverified — 说明: 缺少外部证据", "Path 驱动A -> 目标B: unsupported_jump — 说明: 中间条件不成立", "Confidence", "high"} {
		if !strings.Contains(out, want) {
			t.Fatalf("stdout missing %q in %q", want, out)
		}
	}
}

func TestRunCompileCardShowsGraphFirstExpandedViewAndVerificationSummary(t *testing.T) {
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
			UnitID:         "twitter:expanded-view",
			Source:         "twitter",
			ExternalID:     "expanded-view",
			RootExternalID: "expanded-view",
			Model:          c.Qwen36PlusModel,
			Output: c.Output{
				Summary:           "一句话总结",
				Drivers:           []string{"旧驱动"},
				Targets:           []string{"旧目标"},
				TransmissionPaths: []c.TransmissionPath{{Driver: "旧驱动", Target: "旧目标", Steps: []string{"旧中间步骤"}}},
				EvidenceNodes:     []string{"旧证据"},
				ExplanationNodes:  []string{"旧解释"},
				Topics:            []string{"topic-a", "topic-b"},
				Graph: c.ReasoningGraph{
					Nodes: []c.GraphNode{testGraphNode("legacy-driver", c.NodeMechanism, "旧驱动"), testGraphNode("legacy-target", c.NodeConclusion, "旧目标")},
					Edges: []c.GraphEdge{{From: "legacy-driver", To: "legacy-target", Kind: c.EdgePositive}},
				},
				Details:    c.HiddenDetails{Caveats: []string{"说明"}},
				Confidence: "high",
			},
			CompiledAt: time.Now().UTC(),
		}
		if err := store.UpsertCompiledOutput(context.Background(), record); err != nil {
			return nil, err
		}
		subgraph := graphmodel.ContentSubgraph{
			ID:               "twitter:expanded-view",
			ArticleID:        "twitter:expanded-view",
			SourcePlatform:   "twitter",
			SourceExternalID: "expanded-view",
			RootExternalID:   "expanded-view",
			CompileVersion:   graphmodel.CompileBridgeVersion,
			CompiledAt:       time.Now().UTC().Format(time.RFC3339),
			UpdatedAt:        time.Now().UTC().Format(time.RFC3339),
			Nodes: []graphmodel.GraphNode{
				{ID: "n1", SourceArticleID: "twitter:expanded-view", SourcePlatform: "twitter", SourceExternalID: "expanded-view", RawText: "美联储加息0.25%", SubjectText: "美联储", ChangeText: "加息0.25%", Kind: graphmodel.NodeKindObservation, GraphRole: graphmodel.GraphRoleDriver, IsPrimary: true, VerificationStatus: graphmodel.VerificationProved},
				{ID: "n2", SourceArticleID: "twitter:expanded-view", SourcePlatform: "twitter", SourceExternalID: "expanded-view", RawText: "流动性收紧", SubjectText: "流动性", ChangeText: "收紧", Kind: graphmodel.NodeKindObservation, GraphRole: graphmodel.GraphRoleIntermediate, IsPrimary: true, VerificationStatus: graphmodel.VerificationPending},
				{ID: "n3", SourceArticleID: "twitter:expanded-view", SourcePlatform: "twitter", SourceExternalID: "expanded-view", RawText: "未来一周美股承压", SubjectText: "美股", ChangeText: "承压", Kind: graphmodel.NodeKindPrediction, GraphRole: graphmodel.GraphRoleTarget, IsPrimary: true, VerificationStatus: graphmodel.VerificationPending},
				{ID: "n4", SourceArticleID: "twitter:expanded-view", SourcePlatform: "twitter", SourceExternalID: "expanded-view", RawText: "CPI回落", SubjectText: "CPI", ChangeText: "回落", Kind: graphmodel.NodeKindObservation, GraphRole: graphmodel.GraphRoleEvidence, IsPrimary: false, VerificationStatus: graphmodel.VerificationProved},
				{ID: "n5", SourceArticleID: "twitter:expanded-view", SourcePlatform: "twitter", SourceExternalID: "expanded-view", RawText: "估值承压先传导到科技股", SubjectText: "科技股估值", ChangeText: "承压", Kind: graphmodel.NodeKindObservation, GraphRole: graphmodel.GraphRoleContext, IsPrimary: false, VerificationStatus: graphmodel.VerificationUnverifiable},
			},
			Edges: []graphmodel.GraphEdge{
				{ID: "n1->n2:drives", From: "n1", To: "n2", Type: graphmodel.EdgeTypeDrives, IsPrimary: true, VerificationStatus: graphmodel.VerificationProved},
				{ID: "n2->n3:drives", From: "n2", To: "n3", Type: graphmodel.EdgeTypeDrives, IsPrimary: true, VerificationStatus: graphmodel.VerificationPending},
				{ID: "n4->n1:supports", From: "n4", To: "n1", Type: graphmodel.EdgeTypeSupports, IsPrimary: false, VerificationStatus: graphmodel.VerificationProved},
				{ID: "n5->n3:explains", From: "n5", To: "n3", Type: graphmodel.EdgeTypeExplains, IsPrimary: false, VerificationStatus: graphmodel.VerificationUnverifiable},
			},
		}
		if err := store.UpsertContentSubgraph(context.Background(), subgraph); err != nil {
			return nil, err
		}
		return store, nil
	}

	var stdout, stderr bytes.Buffer
	code := run([]string{"compile", "card", "--platform", "twitter", "--id", "expanded-view"}, "/tmp/project", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("run() code = %d, stderr = %s", code, stderr.String())
	}
	out := stdout.String()
	for _, want := range []string{
		"Summary", "一句话总结",
		"Topics", "topic-a",
		"Drivers", "- 美联储加息0.25%",
		"Targets", "- 未来一周美股承压",
		"Evidence", "- CPI回落",
		"Explanations", "- 估值承压先传导到科技股",
		"Logic chain", "美联储加息0.25% -> 流动性收紧 -> 未来一周美股承压",
		"Verification",
		"Nodes: pending=2, proved=2, unverifiable=1",
		"Edges: pending=1, proved=2, unverifiable=1",
		"Confidence", "high",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("stdout missing %q in %q", want, out)
		}
	}
}

func TestRunCompileCardFallsBackToLegacyWhenGraphFirstSubgraphMissing(t *testing.T) {
	prevBuildApp := buildApp
	prevOpenSQLiteStore := openSQLiteStore
	t.Cleanup(func() {
		buildApp = prevBuildApp
		openSQLiteStore = prevOpenSQLiteStore
	})

	tmp := t.TempDir()
	dbPath := filepath.Join(tmp, "content.db")
	buildApp = func(projectRoot string) (*bootstrap.App, error) {
		app := &bootstrap.App{}
		app.Settings.ContentDBPath = dbPath
		return app, nil
	}
	openSQLiteStore = func(path string) (*contentstore.SQLiteStore, error) {
		store, err := contentstore.NewSQLiteStore(path)
		if err != nil {
			return nil, err
		}
		record := c.Record{
			UnitID:         "twitter:legacy-fallback-standard",
			Source:         "twitter",
			ExternalID:     "legacy-fallback-standard",
			RootExternalID: "legacy-fallback-standard",
			Model:          c.Qwen36PlusModel,
			Output: c.Output{
				Summary:           "一句话总结",
				Drivers:           []string{"驱动A"},
				Targets:           []string{"目标B"},
				TransmissionPaths: []c.TransmissionPath{{Driver: "驱动A", Target: "目标B", Steps: []string{"中间步骤"}}},
				EvidenceNodes:     []string{"证据A"},
				ExplanationNodes:  []string{"解释B"},
				Graph: c.ReasoningGraph{
					Nodes: []c.GraphNode{testGraphNode("legacy-driver", c.NodeMechanism, "驱动A"), testGraphNode("legacy-target", c.NodeConclusion, "目标B")},
					Edges: []c.GraphEdge{{From: "legacy-driver", To: "legacy-target", Kind: c.EdgePositive}},
				},
				Details:    c.HiddenDetails{Caveats: []string{"说明"}},
				Confidence: "high",
			},
			CompiledAt: time.Now().UTC(),
		}
		if err := store.UpsertCompiledOutput(context.Background(), record); err != nil {
			return nil, err
		}
		rawDB, err := sql.Open("sqlite", path)
		if err != nil {
			return nil, err
		}
		defer rawDB.Close()
		if _, err := rawDB.Exec(`DELETE FROM content_subgraphs WHERE platform = ? AND external_id = ?`, "twitter", "legacy-fallback-standard"); err != nil {
			return nil, err
		}
		return store, nil
	}

	var stdout, stderr bytes.Buffer
	code := run([]string{"compile", "card", "--platform", "twitter", "--id", "legacy-fallback-standard"}, "/tmp/project", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("run() code = %d, stderr = %s", code, stderr.String())
	}
	out := stdout.String()
	for _, want := range []string{"Drivers", "- 驱动A", "Targets", "- 目标B", "Evidence", "- 证据A", "Explanations", "- 解释B", "Logic chain", "驱动A -> 中间步骤 -> 目标B"} {
		if !strings.Contains(out, want) {
			t.Fatalf("stdout missing %q in %q", want, out)
		}
	}
	if strings.Contains(out, "Verification") {
		t.Fatalf("stdout = %q, did not want verification summary without subgraph", out)
	}
}

func TestRunCompileCardFallsBackToLegacyWhenGraphFirstProjectionIsLessInformative(t *testing.T) {
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
			UnitID:         "twitter:less-informative-standard",
			Source:         "twitter",
			ExternalID:     "less-informative-standard",
			RootExternalID: "less-informative-standard",
			Model:          c.Qwen36PlusModel,
			Output: c.Output{
				Summary:           "一句话总结",
				Drivers:           []string{"旧驱动A", "旧驱动B"},
				Targets:           []string{"旧目标A", "旧目标B"},
				TransmissionPaths: []c.TransmissionPath{{Driver: "旧驱动A", Target: "旧目标A", Steps: []string{"旧中间步骤"}}},
				EvidenceNodes:     []string{"旧证据A", "旧证据B"},
				ExplanationNodes:  []string{"旧解释A", "旧解释B"},
				Graph: c.ReasoningGraph{
					Nodes: []c.GraphNode{testGraphNode("legacy-driver", c.NodeMechanism, "旧驱动A"), testGraphNode("legacy-target", c.NodeConclusion, "旧目标A")},
					Edges: []c.GraphEdge{{From: "legacy-driver", To: "legacy-target", Kind: c.EdgePositive}},
				},
				Details:    c.HiddenDetails{Caveats: []string{"说明"}},
				Confidence: "high",
			},
			CompiledAt: time.Now().UTC(),
		}
		if err := store.UpsertCompiledOutput(context.Background(), record); err != nil {
			return nil, err
		}
		subgraph := graphmodel.ContentSubgraph{
			ID:               "twitter:less-informative-standard",
			ArticleID:        "twitter:less-informative-standard",
			SourcePlatform:   "twitter",
			SourceExternalID: "less-informative-standard",
			RootExternalID:   "less-informative-standard",
			CompileVersion:   graphmodel.CompileBridgeVersion,
			CompiledAt:       time.Now().UTC().Format(time.RFC3339),
			UpdatedAt:        time.Now().UTC().Format(time.RFC3339),
			Nodes: []graphmodel.GraphNode{
				{ID: "n1", SourceArticleID: "twitter:less-informative-standard", SourcePlatform: "twitter", SourceExternalID: "less-informative-standard", RawText: "新驱动", SubjectText: "新驱动", ChangeText: "新驱动", Kind: graphmodel.NodeKindObservation, GraphRole: graphmodel.GraphRoleDriver, IsPrimary: true, VerificationStatus: graphmodel.VerificationPending},
				{ID: "n2", SourceArticleID: "twitter:less-informative-standard", SourcePlatform: "twitter", SourceExternalID: "less-informative-standard", RawText: "新目标", SubjectText: "新目标", ChangeText: "新目标", Kind: graphmodel.NodeKindPrediction, GraphRole: graphmodel.GraphRoleTarget, IsPrimary: true, VerificationStatus: graphmodel.VerificationPending},
				{ID: "n3", SourceArticleID: "twitter:less-informative-standard", SourcePlatform: "twitter", SourceExternalID: "less-informative-standard", RawText: "新证据", SubjectText: "新证据", ChangeText: "新证据", Kind: graphmodel.NodeKindObservation, GraphRole: graphmodel.GraphRoleEvidence, IsPrimary: false, VerificationStatus: graphmodel.VerificationPending},
				{ID: "n4", SourceArticleID: "twitter:less-informative-standard", SourcePlatform: "twitter", SourceExternalID: "less-informative-standard", RawText: "新解释", SubjectText: "新解释", ChangeText: "新解释", Kind: graphmodel.NodeKindObservation, GraphRole: graphmodel.GraphRoleContext, IsPrimary: false, VerificationStatus: graphmodel.VerificationPending},
			},
			Edges: []graphmodel.GraphEdge{
				{ID: "n1->n2:drives", From: "n1", To: "n2", Type: graphmodel.EdgeTypeDrives, IsPrimary: true, VerificationStatus: graphmodel.VerificationPending},
			},
		}
		if err := store.UpsertContentSubgraph(context.Background(), subgraph); err != nil {
			return nil, err
		}
		return store, nil
	}

	var stdout, stderr bytes.Buffer
	code := run([]string{"compile", "card", "--platform", "twitter", "--id", "less-informative-standard"}, "/tmp/project", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("run() code = %d, stderr = %s", code, stderr.String())
	}
	out := stdout.String()
	for _, want := range []string{"- 旧驱动A", "- 旧驱动B", "- 旧目标A", "- 旧目标B", "- 旧证据A", "- 旧证据B", "- 旧解释A", "- 旧解释B"} {
		if !strings.Contains(out, want) {
			t.Fatalf("stdout missing legacy section item %q in %q", want, out)
		}
	}
	for _, avoid := range []string{"- 新驱动", "- 新目标", "- 新证据", "- 新解释"} {
		if strings.Contains(out, avoid) {
			t.Fatalf("stdout = %q, did not want less-informative graph-first item %q", out, avoid)
		}
	}
}

func TestRunCompileCardCompactPrintsCompactView(t *testing.T) {
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
			UnitID:         "twitter:1",
			Source:         "twitter",
			ExternalID:     "1",
			RootExternalID: "1",
			Model:          c.Qwen36PlusModel,
			Output: c.Output{
				Summary:           "一句话总结",
				Drivers:           []string{"驱动A"},
				Targets:           []string{"目标B"},
				TransmissionPaths: []c.TransmissionPath{{Driver: "驱动A", Target: "目标B", Steps: []string{"中间步骤"}}},
				EvidenceNodes:     []string{"证据A"},
				ExplanationNodes:  []string{"解释B"},
				Graph: c.ReasoningGraph{
					Nodes: []c.GraphNode{testGraphNode("n1", c.NodeMechanism, "驱动A"), testGraphNode("n2", c.NodeConclusion, "目标B")},
					Edges: []c.GraphEdge{{From: "n1", To: "n2", Kind: c.EdgePositive}},
				},
				Details:    c.HiddenDetails{Caveats: []string{"说明"}},
				Confidence: "high",
			},
			CompiledAt: time.Now().UTC(),
		}
		if err := store.UpsertCompiledOutput(context.Background(), record); err != nil {
			return nil, err
		}
		return store, nil
	}

	var stdout, stderr bytes.Buffer
	code := run([]string{"compile", "card", "--compact", "--platform", "twitter", "--id", "1"}, "/tmp/project", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("run() code = %d, stderr = %s", code, stderr.String())
	}
	out := stdout.String()
	for _, want := range []string{"Summary", "一句话总结", "Drivers", "- 驱动A", "Targets", "- 目标B", "Evidence", "- 证据A", "Explanations", "- 解释B", "Main logic", "驱动A -> 中间步骤 -> 目标B", "Confidence", "high"} {
		if !strings.Contains(out, want) {
			t.Fatalf("stdout missing %q in %q", want, out)
		}
	}
}

func TestRunCompileCardCompactReadsByURL(t *testing.T) {
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
		app.Dispatcher = dispatcher.New(
			func(raw string) (types.ParsedURL, error) {
				return types.ParsedURL{Platform: types.PlatformWeibo, PlatformID: "QAu4U9USk", CanonicalURL: raw}, nil
			},
			[]dispatcher.ItemSource{fakeItemSource{}},
			nil,
			nil,
		)
		return app, nil
	}
	openSQLiteStore = func(path string) (*contentstore.SQLiteStore, error) {
		store, err := contentstore.NewSQLiteStore(path)
		if err != nil {
			return nil, err
		}
		record := c.Record{
			UnitID:         "weibo:QAu4U9USk",
			Source:         "weibo",
			ExternalID:     "QAu4U9USk",
			RootExternalID: "QAu4U9USk",
			Model:          c.Qwen36PlusModel,
			Output: c.Output{
				Summary:           "一句话总结",
				Drivers:           []string{"驱动A"},
				Targets:           []string{"目标B"},
				TransmissionPaths: []c.TransmissionPath{{Driver: "驱动A", Target: "目标B", Steps: []string{"中间步骤"}}},
				EvidenceNodes:     []string{"证据A"},
				ExplanationNodes:  []string{"解释B"},
				Graph: c.ReasoningGraph{
					Nodes: []c.GraphNode{testGraphNode("n1", c.NodeMechanism, "驱动A"), testGraphNode("n2", c.NodeConclusion, "目标B")},
					Edges: []c.GraphEdge{{From: "n1", To: "n2", Kind: c.EdgePositive}},
				},
				Details:    c.HiddenDetails{Caveats: []string{"说明"}},
				Confidence: "high",
			},
			CompiledAt: time.Now().UTC(),
		}
		if err := store.UpsertCompiledOutput(context.Background(), record); err != nil {
			return nil, err
		}
		return store, nil
	}

	var stdout, stderr bytes.Buffer
	code := run([]string{"compile", "card", "--compact", "--url", "https://weibo.com/1182426800/QAu4U9USk"}, "/tmp/project", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("run() code = %d, stderr = %s", code, stderr.String())
	}
	out := stdout.String()
	for _, want := range []string{"Summary", "一句话总结", "Drivers", "- 驱动A", "Targets", "- 目标B", "Evidence", "- 证据A", "Explanations", "- 解释B", "Main logic", "驱动A -> 中间步骤 -> 目标B", "Confidence", "high"} {
		if !strings.Contains(out, want) {
			t.Fatalf("stdout missing %q in %q", want, out)
		}
	}
}

func TestRunCompileCardPrefersGraphFirstLogicChain(t *testing.T) {
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
			UnitID:         "twitter:graph-first-logic",
			Source:         "twitter",
			ExternalID:     "graph-first-logic",
			RootExternalID: "graph-first-logic",
			Model:          c.Qwen36PlusModel,
			Output: c.Output{
				Summary:           "一句话总结",
				Drivers:           []string{"旧驱动"},
				Targets:           []string{"旧目标"},
				TransmissionPaths: []c.TransmissionPath{{Driver: "旧驱动", Target: "旧目标", Steps: []string{"旧中间步骤"}}},
				Graph: c.ReasoningGraph{
					Nodes: []c.GraphNode{testGraphNode("legacy-driver", c.NodeMechanism, "旧驱动"), testGraphNode("legacy-target", c.NodeConclusion, "旧目标")},
					Edges: []c.GraphEdge{{From: "legacy-driver", To: "legacy-target", Kind: c.EdgePositive}},
				},
				Details:    c.HiddenDetails{Caveats: []string{"说明"}},
				Confidence: "high",
			},
			CompiledAt: time.Now().UTC(),
		}
		if err := store.UpsertCompiledOutput(context.Background(), record); err != nil {
			return nil, err
		}
		subgraph := graphmodel.ContentSubgraph{
			ID:               "twitter:graph-first-logic",
			ArticleID:        "twitter:graph-first-logic",
			SourcePlatform:   "twitter",
			SourceExternalID: "graph-first-logic",
			RootExternalID:   "graph-first-logic",
			CompileVersion:   graphmodel.CompileBridgeVersion,
			CompiledAt:       time.Now().UTC().Format(time.RFC3339),
			UpdatedAt:        time.Now().UTC().Format(time.RFC3339),
			Nodes: []graphmodel.GraphNode{
				{ID: "n1", SourceArticleID: "twitter:graph-first-logic", SourcePlatform: "twitter", SourceExternalID: "graph-first-logic", RawText: "美联储加息0.25%", SubjectText: "美联储", ChangeText: "加息0.25%", Kind: graphmodel.NodeKindObservation, GraphRole: graphmodel.GraphRoleDriver, IsPrimary: true, VerificationStatus: graphmodel.VerificationPending},
				{ID: "n2", SourceArticleID: "twitter:graph-first-logic", SourcePlatform: "twitter", SourceExternalID: "graph-first-logic", RawText: "流动性收紧", SubjectText: "流动性", ChangeText: "收紧", Kind: graphmodel.NodeKindObservation, GraphRole: graphmodel.GraphRoleIntermediate, IsPrimary: true, VerificationStatus: graphmodel.VerificationPending},
				{ID: "n3", SourceArticleID: "twitter:graph-first-logic", SourcePlatform: "twitter", SourceExternalID: "graph-first-logic", RawText: "未来一周美股承压", SubjectText: "美股", ChangeText: "承压", Kind: graphmodel.NodeKindPrediction, GraphRole: graphmodel.GraphRoleTarget, IsPrimary: true, VerificationStatus: graphmodel.VerificationPending},
			},
			Edges: []graphmodel.GraphEdge{
				{ID: "n1->n2:drives", From: "n1", To: "n2", Type: graphmodel.EdgeTypeDrives, IsPrimary: true, VerificationStatus: graphmodel.VerificationPending},
				{ID: "n2->n3:drives", From: "n2", To: "n3", Type: graphmodel.EdgeTypeDrives, IsPrimary: true, VerificationStatus: graphmodel.VerificationPending},
			},
		}
		if err := store.UpsertContentSubgraph(context.Background(), subgraph); err != nil {
			return nil, err
		}
		return store, nil
	}

	var stdout, stderr bytes.Buffer
	code := run([]string{"compile", "card", "--platform", "twitter", "--id", "graph-first-logic"}, "/tmp/project", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("run() code = %d, stderr = %s", code, stderr.String())
	}
	out := stdout.String()
	if !strings.Contains(out, "美联储加息0.25% -> 流动性收紧 -> 未来一周美股承压") {
		t.Fatalf("stdout = %q, want graph-first logic chain", out)
	}
	if strings.Contains(out, "旧驱动 -> 旧中间步骤 -> 旧目标") {
		t.Fatalf("stdout = %q, did not want stale legacy logic chain", out)
	}
}

func TestRunCompileCardCompactPrefersGraphFirstProjection(t *testing.T) {
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
			UnitID:         "twitter:graph-first-compact",
			Source:         "twitter",
			ExternalID:     "graph-first-compact",
			RootExternalID: "graph-first-compact",
			Model:          c.Qwen36PlusModel,
			Output: c.Output{
				Summary:           "一句话总结",
				Drivers:           []string{"旧驱动"},
				Targets:           []string{"旧目标"},
				TransmissionPaths: []c.TransmissionPath{{Driver: "旧驱动", Target: "旧目标", Steps: []string{"旧中间步骤"}}},
				EvidenceNodes:     []string{"旧证据"},
				ExplanationNodes:  []string{"旧解释"},
				Graph: c.ReasoningGraph{
					Nodes: []c.GraphNode{testGraphNode("legacy-driver", c.NodeMechanism, "旧驱动"), testGraphNode("legacy-target", c.NodeConclusion, "旧目标")},
					Edges: []c.GraphEdge{{From: "legacy-driver", To: "legacy-target", Kind: c.EdgePositive}},
				},
				Details:    c.HiddenDetails{Caveats: []string{"说明"}},
				Confidence: "high",
			},
			CompiledAt: time.Now().UTC(),
		}
		if err := store.UpsertCompiledOutput(context.Background(), record); err != nil {
			return nil, err
		}
		subgraph := graphmodel.ContentSubgraph{
			ID:               "twitter:graph-first-compact",
			ArticleID:        "twitter:graph-first-compact",
			SourcePlatform:   "twitter",
			SourceExternalID: "graph-first-compact",
			RootExternalID:   "graph-first-compact",
			CompileVersion:   graphmodel.CompileBridgeVersion,
			CompiledAt:       time.Now().UTC().Format(time.RFC3339),
			UpdatedAt:        time.Now().UTC().Format(time.RFC3339),
			Nodes: []graphmodel.GraphNode{
				{ID: "n1", SourceArticleID: "twitter:graph-first-compact", SourcePlatform: "twitter", SourceExternalID: "graph-first-compact", RawText: "美联储加息0.25%", SubjectText: "美联储", ChangeText: "加息0.25%", Kind: graphmodel.NodeKindObservation, GraphRole: graphmodel.GraphRoleDriver, IsPrimary: true, VerificationStatus: graphmodel.VerificationPending},
				{ID: "n2", SourceArticleID: "twitter:graph-first-compact", SourcePlatform: "twitter", SourceExternalID: "graph-first-compact", RawText: "流动性收紧", SubjectText: "流动性", ChangeText: "收紧", Kind: graphmodel.NodeKindObservation, GraphRole: graphmodel.GraphRoleIntermediate, IsPrimary: true, VerificationStatus: graphmodel.VerificationPending},
				{ID: "n3", SourceArticleID: "twitter:graph-first-compact", SourcePlatform: "twitter", SourceExternalID: "graph-first-compact", RawText: "未来一周美股承压", SubjectText: "美股", ChangeText: "承压", Kind: graphmodel.NodeKindPrediction, GraphRole: graphmodel.GraphRoleTarget, IsPrimary: true, VerificationStatus: graphmodel.VerificationPending},
				{ID: "n4", SourceArticleID: "twitter:graph-first-compact", SourcePlatform: "twitter", SourceExternalID: "graph-first-compact", RawText: "CPI回落", SubjectText: "CPI", ChangeText: "回落", Kind: graphmodel.NodeKindObservation, GraphRole: graphmodel.GraphRoleEvidence, IsPrimary: false, VerificationStatus: graphmodel.VerificationPending},
				{ID: "n5", SourceArticleID: "twitter:graph-first-compact", SourcePlatform: "twitter", SourceExternalID: "graph-first-compact", RawText: "估值承压先传导到科技股", SubjectText: "科技股估值", ChangeText: "承压", Kind: graphmodel.NodeKindObservation, GraphRole: graphmodel.GraphRoleContext, IsPrimary: false, VerificationStatus: graphmodel.VerificationPending},
			},
			Edges: []graphmodel.GraphEdge{
				{ID: "n1->n2:drives", From: "n1", To: "n2", Type: graphmodel.EdgeTypeDrives, IsPrimary: true, VerificationStatus: graphmodel.VerificationPending},
				{ID: "n2->n3:drives", From: "n2", To: "n3", Type: graphmodel.EdgeTypeDrives, IsPrimary: true, VerificationStatus: graphmodel.VerificationPending},
				{ID: "n4->n1:supports", From: "n4", To: "n1", Type: graphmodel.EdgeTypeSupports, IsPrimary: false, VerificationStatus: graphmodel.VerificationPending},
				{ID: "n5->n3:explains", From: "n5", To: "n3", Type: graphmodel.EdgeTypeExplains, IsPrimary: false, VerificationStatus: graphmodel.VerificationPending},
			},
		}
		if err := store.UpsertContentSubgraph(context.Background(), subgraph); err != nil {
			return nil, err
		}
		return store, nil
	}

	var stdout, stderr bytes.Buffer
	code := run([]string{"compile", "card", "--compact", "--platform", "twitter", "--id", "graph-first-compact"}, "/tmp/project", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("run() code = %d, stderr = %s", code, stderr.String())
	}
	out := stdout.String()
	for _, want := range []string{"- 美联储加息0.25%", "- 未来一周美股承压", "- CPI回落", "- 估值承压先传导到科技股", "美联储加息0.25% -> 流动性收紧 -> 未来一周美股承压"} {
		if !strings.Contains(out, want) {
			t.Fatalf("stdout missing %q in %q", want, out)
		}
	}
	for _, stale := range []string{"- 旧驱动", "- 旧目标", "- 旧证据", "- 旧解释", "旧驱动 -> 旧中间步骤 -> 旧目标"} {
		if strings.Contains(out, stale) {
			t.Fatalf("stdout = %q, did not want stale legacy projection %q", out, stale)
		}
	}
}

func TestRunCompileCardCompactFallsBackToLegacyWhenGraphFirstSubgraphMissing(t *testing.T) {
	prevBuildApp := buildApp
	prevOpenSQLiteStore := openSQLiteStore
	t.Cleanup(func() {
		buildApp = prevBuildApp
		openSQLiteStore = prevOpenSQLiteStore
	})

	tmp := t.TempDir()
	dbPath := filepath.Join(tmp, "content.db")
	buildApp = func(projectRoot string) (*bootstrap.App, error) {
		app := &bootstrap.App{}
		app.Settings.ContentDBPath = dbPath
		return app, nil
	}
	openSQLiteStore = func(path string) (*contentstore.SQLiteStore, error) {
		store, err := contentstore.NewSQLiteStore(path)
		if err != nil {
			return nil, err
		}
		record := c.Record{
			UnitID:         "twitter:legacy-fallback-card",
			Source:         "twitter",
			ExternalID:     "legacy-fallback-card",
			RootExternalID: "legacy-fallback-card",
			Model:          c.Qwen36PlusModel,
			Output: c.Output{
				Summary:           "一句话总结",
				Drivers:           []string{"驱动A"},
				Targets:           []string{"目标B"},
				TransmissionPaths: []c.TransmissionPath{{Driver: "驱动A", Target: "目标B", Steps: []string{"中间步骤"}}},
				EvidenceNodes:     []string{"证据A"},
				ExplanationNodes:  []string{"解释B"},
				Graph: c.ReasoningGraph{
					Nodes: []c.GraphNode{testGraphNode("legacy-driver", c.NodeMechanism, "驱动A"), testGraphNode("legacy-target", c.NodeConclusion, "目标B")},
					Edges: []c.GraphEdge{{From: "legacy-driver", To: "legacy-target", Kind: c.EdgePositive}},
				},
				Details:    c.HiddenDetails{Caveats: []string{"说明"}},
				Confidence: "high",
			},
			CompiledAt: time.Now().UTC(),
		}
		if err := store.UpsertCompiledOutput(context.Background(), record); err != nil {
			return nil, err
		}
		rawDB, err := sql.Open("sqlite", path)
		if err != nil {
			return nil, err
		}
		defer rawDB.Close()
		if _, err := rawDB.Exec(`DELETE FROM content_subgraphs WHERE platform = ? AND external_id = ?`, "twitter", "legacy-fallback-card"); err != nil {
			return nil, err
		}
		return store, nil
	}

	var stdout, stderr bytes.Buffer
	code := run([]string{"compile", "card", "--compact", "--platform", "twitter", "--id", "legacy-fallback-card"}, "/tmp/project", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("run() code = %d, stderr = %s", code, stderr.String())
	}
	out := stdout.String()
	for _, want := range []string{"- 驱动A", "- 目标B", "- 证据A", "- 解释B", "驱动A -> 中间步骤 -> 目标B"} {
		if !strings.Contains(out, want) {
			t.Fatalf("stdout missing %q in %q", want, out)
		}
	}
}

func TestRunCompileCardFailsWhenGraphFirstLookupReturnsUnexpectedStoreError(t *testing.T) {
	prevBuildApp := buildApp
	prevOpenSQLiteStore := openSQLiteStore
	t.Cleanup(func() {
		buildApp = prevBuildApp
		openSQLiteStore = prevOpenSQLiteStore
	})

	tmp := t.TempDir()
	dbPath := filepath.Join(tmp, "content.db")
	buildApp = func(projectRoot string) (*bootstrap.App, error) {
		app := &bootstrap.App{}
		app.Settings.ContentDBPath = dbPath
		return app, nil
	}
	openSQLiteStore = func(path string) (*contentstore.SQLiteStore, error) {
		store, err := contentstore.NewSQLiteStore(path)
		if err != nil {
			return nil, err
		}
		record := c.Record{
			UnitID:         "twitter:graph-first-store-error",
			Source:         "twitter",
			ExternalID:     "graph-first-store-error",
			RootExternalID: "graph-first-store-error",
			Model:          c.Qwen36PlusModel,
			Output: c.Output{
				Summary:           "一句话总结",
				Drivers:           []string{"驱动A"},
				Targets:           []string{"目标B"},
				TransmissionPaths: []c.TransmissionPath{{Driver: "驱动A", Target: "目标B", Steps: []string{"中间步骤"}}},
				EvidenceNodes:     []string{"证据A"},
				ExplanationNodes:  []string{"解释B"},
				Graph: c.ReasoningGraph{
					Nodes: []c.GraphNode{testGraphNode("legacy-driver", c.NodeMechanism, "驱动A"), testGraphNode("legacy-target", c.NodeConclusion, "目标B")},
					Edges: []c.GraphEdge{{From: "legacy-driver", To: "legacy-target", Kind: c.EdgePositive}},
				},
				Details:    c.HiddenDetails{Caveats: []string{"说明"}},
				Confidence: "high",
			},
			CompiledAt: time.Now().UTC(),
		}
		if err := store.UpsertCompiledOutput(context.Background(), record); err != nil {
			return nil, err
		}
		rawDB, err := sql.Open("sqlite", path)
		if err != nil {
			return nil, err
		}
		defer rawDB.Close()
		if _, err := rawDB.Exec(`DROP TABLE content_subgraphs`); err != nil {
			return nil, err
		}
		return store, nil
	}

	var stdout, stderr bytes.Buffer
	code := run([]string{"compile", "card", "--compact", "--platform", "twitter", "--id", "graph-first-store-error"}, "/tmp/project", &stdout, &stderr)
	if code != 1 {
		t.Fatalf("run() code = %d, want 1, stdout = %s, stderr = %s", code, stdout.String(), stderr.String())
	}
	if !strings.Contains(stderr.String(), "content_subgraphs") {
		t.Fatalf("stderr = %q, want surfaced content_subgraphs lookup error", stderr.String())
	}
}

func TestRunCompileCardFailsWhenExpandedGraphFirstLookupReturnsUnexpectedStoreError(t *testing.T) {
	prevBuildApp := buildApp
	prevOpenSQLiteStore := openSQLiteStore
	t.Cleanup(func() {
		buildApp = prevBuildApp
		openSQLiteStore = prevOpenSQLiteStore
	})

	tmp := t.TempDir()
	dbPath := filepath.Join(tmp, "content.db")
	buildApp = func(projectRoot string) (*bootstrap.App, error) {
		app := &bootstrap.App{}
		app.Settings.ContentDBPath = dbPath
		return app, nil
	}
	openSQLiteStore = func(path string) (*contentstore.SQLiteStore, error) {
		store, err := contentstore.NewSQLiteStore(path)
		if err != nil {
			return nil, err
		}
		record := c.Record{
			UnitID:         "twitter:graph-first-store-error-expanded",
			Source:         "twitter",
			ExternalID:     "graph-first-store-error-expanded",
			RootExternalID: "graph-first-store-error-expanded",
			Model:          c.Qwen36PlusModel,
			Output: c.Output{
				Summary:           "一句话总结",
				Drivers:           []string{"驱动A"},
				Targets:           []string{"目标B"},
				TransmissionPaths: []c.TransmissionPath{{Driver: "驱动A", Target: "目标B", Steps: []string{"中间步骤"}}},
				EvidenceNodes:     []string{"证据A"},
				ExplanationNodes:  []string{"解释B"},
				Graph: c.ReasoningGraph{
					Nodes: []c.GraphNode{testGraphNode("legacy-driver", c.NodeMechanism, "驱动A"), testGraphNode("legacy-target", c.NodeConclusion, "目标B")},
					Edges: []c.GraphEdge{{From: "legacy-driver", To: "legacy-target", Kind: c.EdgePositive}},
				},
				Details:    c.HiddenDetails{Caveats: []string{"说明"}},
				Confidence: "high",
			},
			CompiledAt: time.Now().UTC(),
		}
		if err := store.UpsertCompiledOutput(context.Background(), record); err != nil {
			return nil, err
		}
		rawDB, err := sql.Open("sqlite", path)
		if err != nil {
			return nil, err
		}
		defer rawDB.Close()
		if _, err := rawDB.Exec(`DROP TABLE content_subgraphs`); err != nil {
			return nil, err
		}
		return store, nil
	}

	var stdout, stderr bytes.Buffer
	code := run([]string{"compile", "card", "--platform", "twitter", "--id", "graph-first-store-error-expanded"}, "/tmp/project", &stdout, &stderr)
	if code != 1 {
		t.Fatalf("run() code = %d, want 1, stdout = %s, stderr = %s", code, stdout.String(), stderr.String())
	}
	if !strings.Contains(stderr.String(), "content_subgraphs") {
		t.Fatalf("stderr = %q, want surfaced content_subgraphs lookup error", stderr.String())
	}
}

func TestRunCompileCardFailsWhenURLGraphFirstLookupReturnsUnexpectedStoreError(t *testing.T) {
	prevBuildApp := buildApp
	prevOpenSQLiteStore := openSQLiteStore
	t.Cleanup(func() {
		buildApp = prevBuildApp
		openSQLiteStore = prevOpenSQLiteStore
	})

	tmp := t.TempDir()
	dbPath := filepath.Join(tmp, "content.db")
	buildApp = func(projectRoot string) (*bootstrap.App, error) {
		app := &bootstrap.App{}
		app.Settings.ContentDBPath = dbPath
		app.Dispatcher = dispatcher.New(
			func(raw string) (types.ParsedURL, error) {
				return types.ParsedURL{Platform: types.PlatformWeibo, PlatformID: "Q-store-url", CanonicalURL: raw}, nil
			},
			[]dispatcher.ItemSource{fakeItemSource{}},
			nil,
			nil,
		)
		return app, nil
	}
	openSQLiteStore = func(path string) (*contentstore.SQLiteStore, error) {
		store, err := contentstore.NewSQLiteStore(path)
		if err != nil {
			return nil, err
		}
		record := c.Record{
			UnitID:         "weibo:Q-store-url",
			Source:         "weibo",
			ExternalID:     "Q-store-url",
			RootExternalID: "Q-store-url",
			Model:          c.Qwen36PlusModel,
			Output: c.Output{
				Summary:           "一句话总结",
				Drivers:           []string{"驱动A"},
				Targets:           []string{"目标B"},
				TransmissionPaths: []c.TransmissionPath{{Driver: "驱动A", Target: "目标B", Steps: []string{"中间步骤"}}},
				EvidenceNodes:     []string{"证据A"},
				ExplanationNodes:  []string{"解释B"},
				Graph: c.ReasoningGraph{
					Nodes: []c.GraphNode{testGraphNode("legacy-driver", c.NodeMechanism, "驱动A"), testGraphNode("legacy-target", c.NodeConclusion, "目标B")},
					Edges: []c.GraphEdge{{From: "legacy-driver", To: "legacy-target", Kind: c.EdgePositive}},
				},
				Details:    c.HiddenDetails{Caveats: []string{"说明"}},
				Confidence: "high",
			},
			CompiledAt: time.Now().UTC(),
		}
		if err := store.UpsertCompiledOutput(context.Background(), record); err != nil {
			return nil, err
		}
		rawDB, err := sql.Open("sqlite", path)
		if err != nil {
			return nil, err
		}
		defer rawDB.Close()
		if _, err := rawDB.Exec(`DROP TABLE content_subgraphs`); err != nil {
			return nil, err
		}
		return store, nil
	}

	var stdout, stderr bytes.Buffer
	code := run([]string{"compile", "card", "--url", "https://weibo.com/1182426800/Q-store-url"}, "/tmp/project", &stdout, &stderr)
	if code != 1 {
		t.Fatalf("run() code = %d, want 1, stdout = %s, stderr = %s", code, stdout.String(), stderr.String())
	}
	if !strings.Contains(stderr.String(), "content_subgraphs") {
		t.Fatalf("stderr = %q, want surfaced content_subgraphs lookup error", stderr.String())
	}
}

func TestRunCompileCardCompactFailsWhenURLGraphFirstLookupReturnsUnexpectedStoreError(t *testing.T) {
	prevBuildApp := buildApp
	prevOpenSQLiteStore := openSQLiteStore
	t.Cleanup(func() {
		buildApp = prevBuildApp
		openSQLiteStore = prevOpenSQLiteStore
	})

	tmp := t.TempDir()
	dbPath := filepath.Join(tmp, "content.db")
	buildApp = func(projectRoot string) (*bootstrap.App, error) {
		app := &bootstrap.App{}
		app.Settings.ContentDBPath = dbPath
		app.Dispatcher = dispatcher.New(
			func(raw string) (types.ParsedURL, error) {
				return types.ParsedURL{Platform: types.PlatformWeibo, PlatformID: "Q-store-url-compact", CanonicalURL: raw}, nil
			},
			[]dispatcher.ItemSource{fakeItemSource{}},
			nil,
			nil,
		)
		return app, nil
	}
	openSQLiteStore = func(path string) (*contentstore.SQLiteStore, error) {
		store, err := contentstore.NewSQLiteStore(path)
		if err != nil {
			return nil, err
		}
		record := c.Record{
			UnitID:         "weibo:Q-store-url-compact",
			Source:         "weibo",
			ExternalID:     "Q-store-url-compact",
			RootExternalID: "Q-store-url-compact",
			Model:          c.Qwen36PlusModel,
			Output: c.Output{
				Summary:           "一句话总结",
				Drivers:           []string{"驱动A"},
				Targets:           []string{"目标B"},
				TransmissionPaths: []c.TransmissionPath{{Driver: "驱动A", Target: "目标B", Steps: []string{"中间步骤"}}},
				EvidenceNodes:     []string{"证据A"},
				ExplanationNodes:  []string{"解释B"},
				Graph: c.ReasoningGraph{
					Nodes: []c.GraphNode{testGraphNode("legacy-driver", c.NodeMechanism, "驱动A"), testGraphNode("legacy-target", c.NodeConclusion, "目标B")},
					Edges: []c.GraphEdge{{From: "legacy-driver", To: "legacy-target", Kind: c.EdgePositive}},
				},
				Details:    c.HiddenDetails{Caveats: []string{"说明"}},
				Confidence: "high",
			},
			CompiledAt: time.Now().UTC(),
		}
		if err := store.UpsertCompiledOutput(context.Background(), record); err != nil {
			return nil, err
		}
		rawDB, err := sql.Open("sqlite", path)
		if err != nil {
			return nil, err
		}
		defer rawDB.Close()
		if _, err := rawDB.Exec(`DROP TABLE content_subgraphs`); err != nil {
			return nil, err
		}
		return store, nil
	}

	var stdout, stderr bytes.Buffer
	code := run([]string{"compile", "card", "--compact", "--url", "https://weibo.com/1182426800/Q-store-url-compact"}, "/tmp/project", &stdout, &stderr)
	if code != 1 {
		t.Fatalf("run() code = %d, want 1, stdout = %s, stderr = %s", code, stdout.String(), stderr.String())
	}
	if !strings.Contains(stderr.String(), "content_subgraphs") {
		t.Fatalf("stderr = %q, want surfaced content_subgraphs lookup error", stderr.String())
	}
}

func TestRunCompileCardCompactFallsBackToLegacyWhenGraphFirstProjectionIsLessInformative(t *testing.T) {
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
			UnitID:         "twitter:less-informative-card",
			Source:         "twitter",
			ExternalID:     "less-informative-card",
			RootExternalID: "less-informative-card",
			Model:          c.Qwen36PlusModel,
			Output: c.Output{
				Summary:           "一句话总结",
				Drivers:           []string{"旧驱动A", "旧驱动B"},
				Targets:           []string{"旧目标A", "旧目标B"},
				TransmissionPaths: []c.TransmissionPath{{Driver: "旧驱动A", Target: "旧目标A", Steps: []string{"旧中间步骤"}}},
				EvidenceNodes:     []string{"旧证据A", "旧证据B"},
				ExplanationNodes:  []string{"旧解释A", "旧解释B"},
				Graph: c.ReasoningGraph{
					Nodes: []c.GraphNode{testGraphNode("legacy-driver", c.NodeMechanism, "旧驱动A"), testGraphNode("legacy-target", c.NodeConclusion, "旧目标A")},
					Edges: []c.GraphEdge{{From: "legacy-driver", To: "legacy-target", Kind: c.EdgePositive}},
				},
				Details:    c.HiddenDetails{Caveats: []string{"说明"}},
				Confidence: "high",
			},
			CompiledAt: time.Now().UTC(),
		}
		if err := store.UpsertCompiledOutput(context.Background(), record); err != nil {
			return nil, err
		}
		subgraph := graphmodel.ContentSubgraph{
			ID:               "twitter:less-informative-card",
			ArticleID:        "twitter:less-informative-card",
			SourcePlatform:   "twitter",
			SourceExternalID: "less-informative-card",
			RootExternalID:   "less-informative-card",
			CompileVersion:   graphmodel.CompileBridgeVersion,
			CompiledAt:       time.Now().UTC().Format(time.RFC3339),
			UpdatedAt:        time.Now().UTC().Format(time.RFC3339),
			Nodes: []graphmodel.GraphNode{
				{ID: "n1", SourceArticleID: "twitter:less-informative-card", SourcePlatform: "twitter", SourceExternalID: "less-informative-card", RawText: "新驱动", SubjectText: "新驱动", ChangeText: "新驱动", Kind: graphmodel.NodeKindObservation, GraphRole: graphmodel.GraphRoleDriver, IsPrimary: true, VerificationStatus: graphmodel.VerificationPending},
				{ID: "n2", SourceArticleID: "twitter:less-informative-card", SourcePlatform: "twitter", SourceExternalID: "less-informative-card", RawText: "新目标", SubjectText: "新目标", ChangeText: "新目标", Kind: graphmodel.NodeKindPrediction, GraphRole: graphmodel.GraphRoleTarget, IsPrimary: true, VerificationStatus: graphmodel.VerificationPending},
				{ID: "n3", SourceArticleID: "twitter:less-informative-card", SourcePlatform: "twitter", SourceExternalID: "less-informative-card", RawText: "新证据", SubjectText: "新证据", ChangeText: "新证据", Kind: graphmodel.NodeKindObservation, GraphRole: graphmodel.GraphRoleEvidence, IsPrimary: false, VerificationStatus: graphmodel.VerificationPending},
				{ID: "n4", SourceArticleID: "twitter:less-informative-card", SourcePlatform: "twitter", SourceExternalID: "less-informative-card", RawText: "新解释", SubjectText: "新解释", ChangeText: "新解释", Kind: graphmodel.NodeKindObservation, GraphRole: graphmodel.GraphRoleContext, IsPrimary: false, VerificationStatus: graphmodel.VerificationPending},
			},
			Edges: []graphmodel.GraphEdge{
				{ID: "n1->n2:drives", From: "n1", To: "n2", Type: graphmodel.EdgeTypeDrives, IsPrimary: true, VerificationStatus: graphmodel.VerificationPending},
			},
		}
		if err := store.UpsertContentSubgraph(context.Background(), subgraph); err != nil {
			return nil, err
		}
		return store, nil
	}

	var stdout, stderr bytes.Buffer
	code := run([]string{"compile", "card", "--compact", "--platform", "twitter", "--id", "less-informative-card"}, "/tmp/project", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("run() code = %d, stderr = %s", code, stderr.String())
	}
	out := stdout.String()
	for _, want := range []string{"- 旧驱动A", "- 旧驱动B", "- 旧目标A", "- 旧目标B", "- 旧证据A", "- 旧证据B", "- 旧解释A", "- 旧解释B"} {
		if !strings.Contains(out, want) {
			t.Fatalf("stdout missing legacy section item %q in %q", want, out)
		}
	}
	for _, avoid := range []string{"- 新驱动", "- 新目标", "- 新证据", "- 新解释"} {
		if strings.Contains(out, avoid) {
			t.Fatalf("stdout = %q, did not want less-informative graph-first item %q", out, avoid)
		}
	}
}

func TestRunCompileCardCollapsesLinearChain(t *testing.T) {
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
			UnitID:         "twitter:1",
			Source:         "twitter",
			ExternalID:     "1",
			RootExternalID: "1",
			Model:          c.Qwen36PlusModel,
			Output: c.Output{
				Summary: "一句话总结",
				Drivers: []string{"驱动A"},
				Targets: []string{"目标C"},
				TransmissionPaths: []c.TransmissionPath{{
					Driver: "驱动A",
					Target: "目标C",
					Steps:  []string{"步骤B"},
				}},
				Graph: c.ReasoningGraph{
					Nodes: []c.GraphNode{testGraphNode("n1", c.NodeMechanism, "驱动A"), testGraphNode("n2", c.NodeMechanism, "步骤B"), testGraphNode("n3", c.NodeConclusion, "目标C")},
					Edges: []c.GraphEdge{{From: "n1", To: "n2", Kind: c.EdgePositive}, {From: "n2", To: "n3", Kind: c.EdgePositive}},
				},
				Details:    c.HiddenDetails{Caveats: []string{"说明"}},
				Confidence: "high",
			},
			CompiledAt: time.Now().UTC(),
		}
		if err := store.UpsertCompiledOutput(context.Background(), record); err != nil {
			return nil, err
		}
		return store, nil
	}

	var stdout, stderr bytes.Buffer
	code := run([]string{"compile", "card", "--platform", "twitter", "--id", "1"}, "/tmp/project", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("run() code = %d, stderr = %s", code, stderr.String())
	}
	out := stdout.String()
	if !strings.Contains(out, "驱动A -> 步骤B -> 目标C") {
		t.Fatalf("stdout missing collapsed chain in %q", out)
	}
}

func TestRunHarnessGraphFirstFlowCommandsWorkTogether(t *testing.T) {
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
		record := c.Record{UnitID: "twitter:FLOW1", Source: "twitter", ExternalID: "FLOW1", RootExternalID: "FLOW1", Model: c.Qwen36PlusModel, Output: c.Output{Summary: "一句话", Drivers: []string{"美联储加息0.25%"}, Targets: []string{"未来一周美股承压"}, Graph: c.ReasoningGraph{Nodes: []c.GraphNode{{ID: "n1", Kind: c.NodeFact, Text: "美联储加息0.25%", OccurredAt: now}, {ID: "n2", Kind: c.NodePrediction, Text: "未来一周美股承压", PredictionStartAt: now, PredictionDueAt: now.Add(24 * time.Hour)}}, Edges: []c.GraphEdge{{From: "n1", To: "n2", Kind: c.EdgePositive}}}, Details: c.HiddenDetails{Caveats: []string{"说明"}}}, CompiledAt: now}
		if err := store.UpsertCompiledOutput(context.Background(), record); err != nil {
			return nil, err
		}
		if _, err := store.AcceptMemoryNodes(context.Background(), memory.AcceptRequest{UserID: "u-flow", SourcePlatform: "twitter", SourceExternalID: "FLOW1", NodeIDs: []string{"n1", "n2"}}); err != nil {
			return nil, err
		}
		return store, nil
	}

	for _, argv := range [][]string{
		{"memory", "content-graphs", "--user", "u-flow"},
		{"memory", "event-graphs", "--run", "--user", "u-flow"},
		{"memory", "paradigms", "--run", "--user", "u-flow"},
		{"verify", "queue", "--limit", "10"},
		{"verify", "sweep", "--limit", "10"},
	} {
		var stdout, stderr bytes.Buffer
		code := run(argv, "/tmp/project", &stdout, &stderr)
		if code != 0 {
			t.Fatalf("run(%v) code = %d, stderr = %s", argv, code, stderr.String())
		}
		if strings.TrimSpace(stdout.String()) == "" {
			t.Fatalf("run(%v) stdout empty", argv)
		}
	}
}
