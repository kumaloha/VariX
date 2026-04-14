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
	"github.com/kumaloha/VariX/varix/ingest/dispatcher"
	"github.com/kumaloha/VariX/varix/ingest/types"
	"github.com/kumaloha/VariX/varix/storage/contentstore"
)

type fakeItemSource struct {
	items []types.RawContent
}

func (f fakeItemSource) Platform() types.Platform {
	return types.PlatformWeb
}

func (f fakeItemSource) Kind() types.Kind {
	return types.KindNative
}

func (f fakeItemSource) Fetch(context.Context, types.ParsedURL) ([]types.RawContent, error) {
	return f.items, nil
}

type panicItemSource struct{}

func (panicItemSource) Platform() types.Platform { return types.PlatformWeb }
func (panicItemSource) Kind() types.Kind         { return types.KindNative }
func (panicItemSource) Fetch(context.Context, types.ParsedURL) ([]types.RawContent, error) {
	panic("fetch should not be called")
}

func TestRunIngestFetchWritesJSONToStdout(t *testing.T) {
	prevBuildApp := buildApp
	prevGetwd := getwd
	t.Cleanup(func() {
		buildApp = prevBuildApp
		getwd = prevGetwd
	})

	buildApp = func(projectRoot string) (*bootstrap.App, error) {
		src := fakeItemSource{
			items: []types.RawContent{{
				Source:     "weibo",
				ExternalID: "QAzzRES0G",
				Content:    "hello",
				AuthorName: "Alice",
				URL:        "https://weibo.com/1182426800/QAzzRES0G",
			}},
		}
		return &bootstrap.App{
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
		}, nil
	}
	getwd = func() (string, error) { return "/tmp/project", nil }

	var stdout, stderr bytes.Buffer
	code := run([]string{"ingest", "fetch", "https://example.com/post"}, "/tmp/project", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("run() code = %d, stderr = %s", code, stderr.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q, want empty", stderr.String())
	}

	var got []types.RawContent
	if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
		t.Fatalf("json.Unmarshal(stdout) error = %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("len(stdout payload) = %d, want 1", len(got))
	}
	if got[0].ExternalID != "QAzzRES0G" {
		t.Fatalf("ExternalID = %q, want QAzzRES0G", got[0].ExternalID)
	}
}

func TestRunIngestFetchRequiresURL(t *testing.T) {
	prevGetwd := getwd
	t.Cleanup(func() {
		getwd = prevGetwd
	})
	getwd = func() (string, error) { return "/tmp/project", nil }

	var stdout, stderr bytes.Buffer
	code := run([]string{"ingest", "fetch"}, "/tmp/project", &stdout, &stderr)
	if code != 2 {
		t.Fatalf("run() code = %d, want 2", code)
	}
	if !strings.Contains(stderr.String(), "usage: varix ingest fetch") {
		t.Fatalf("stderr = %q, want usage", stderr.String())
	}
}

func TestRunCompileRequiresURL(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{"compile", "run"}, "/tmp/project", &stdout, &stderr)
	if code != 2 {
		t.Fatalf("run() code = %d, want 2", code)
	}
	if !strings.Contains(stderr.String(), "usage: varix compile run") {
		t.Fatalf("stderr = %q, want usage", stderr.String())
	}
}

func TestRunCompileShowRequiresLocator(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{"compile", "show"}, "/tmp/project", &stdout, &stderr)
	if code != 2 {
		t.Fatalf("run() code = %d, want 2", code)
	}
	if !strings.Contains(stderr.String(), "usage: varix compile show") {
		t.Fatalf("stderr = %q, want usage", stderr.String())
	}
}

func TestRunCompileSummaryRequiresLocator(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{"compile", "summary"}, "/tmp/project", &stdout, &stderr)
	if code != 2 {
		t.Fatalf("run() code = %d, want 2", code)
	}
	if !strings.Contains(stderr.String(), "usage: varix compile summary") {
		t.Fatalf("stderr = %q, want usage", stderr.String())
	}
}

func TestRunCompileCompareRequiresLocator(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{"compile", "compare"}, "/tmp/project", &stdout, &stderr)
	if code != 2 {
		t.Fatalf("run() code = %d, want 2", code)
	}
	if !strings.Contains(stderr.String(), "usage: varix compile compare") {
		t.Fatalf("stderr = %q, want usage", stderr.String())
	}
}

func TestRunCompileCardRequiresLocator(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{"compile", "card"}, "/tmp/project", &stdout, &stderr)
	if code != 2 {
		t.Fatalf("run() code = %d, want 2", code)
	}
	if !strings.Contains(stderr.String(), "usage: varix compile card") {
		t.Fatalf("stderr = %q, want usage", stderr.String())
	}
}

type fakeCompileClient struct {
	record c.Record
	err    error
}

func (f fakeCompileClient) Compile(_ context.Context, _ c.Bundle) (c.Record, error) {
	return f.record, f.err
}

func TestRunCompileWritesCompiledRecordJSON(t *testing.T) {
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
						Nodes: []c.GraphNode{{ID: "n1", Kind: c.NodeFact, Text: "事实A"}, {ID: "n2", Kind: c.NodeConclusion, Text: "结论B"}},
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
						Nodes: []c.GraphNode{{ID: "n1", Kind: c.NodeFact, Text: "事实A"}, {ID: "n2", Kind: c.NodeConclusion, Text: "结论B"}},
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
						Nodes: []c.GraphNode{{ID: "n1", Kind: c.NodeFact, Text: "事实A"}, {ID: "n2", Kind: c.NodeConclusion, Text: "结论B"}},
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
						Nodes: []c.GraphNode{{ID: "n1", Kind: c.NodeFact, Text: "事实A"}, {ID: "n2", Kind: c.NodeConclusion, Text: "结论B"}},
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
					Nodes: []c.GraphNode{{ID: "n1", Kind: c.NodeFact, Text: "事实A"}, {ID: "n2", Kind: c.NodeConclusion, Text: "结论B"}},
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
						Nodes: []c.GraphNode{{ID: "n1", Kind: c.NodeFact, Text: "事实A"}, {ID: "n2", Kind: c.NodeConclusion, Text: "结论B"}},
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
					Nodes: []c.GraphNode{{ID: "n1", Kind: c.NodeFact, Text: "事实A"}, {ID: "n2", Kind: c.NodeConclusion, Text: "结论B"}},
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
			Output: c.Output{
				Summary: "一句话",
				Graph: c.ReasoningGraph{
					Nodes: []c.GraphNode{{ID: "n1", Kind: c.NodeFact, Text: "事实A"}, {ID: "n2", Kind: c.NodeConclusion, Text: "结论B"}},
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
			Output: c.Output{
				Summary: "Dalio summary",
				Graph: c.ReasoningGraph{
					Nodes: []c.GraphNode{{ID: "n1", Kind: c.NodeFact, Text: "事实A"}, {ID: "n2", Kind: c.NodeConclusion, Text: "结论B"}},
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
				Summary: "一句话",
				Graph: c.ReasoningGraph{
					Nodes: []c.GraphNode{{ID: "n1", Kind: c.NodeFact, Text: "事实A"}, {ID: "n2", Kind: c.NodeConclusion, Text: "结论B"}},
					Edges: []c.GraphEdge{{From: "n1", To: "n2", Kind: c.EdgeDerives}},
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
	for _, want := range []string{"Summary: 一句话", "Nodes: 2", "Edges: 1", "Topics: topic-a, topic-b", "Confidence: medium"} {
		if !strings.Contains(out, want) {
			t.Fatalf("stdout missing %q in %q", want, out)
		}
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
				Summary: "Dalio summary",
				Graph: c.ReasoningGraph{
					Nodes: []c.GraphNode{{ID: "n1", Kind: c.NodeFact, Text: "事实A"}, {ID: "n2", Kind: c.NodeConclusion, Text: "结论B"}},
					Edges: []c.GraphEdge{{From: "n1", To: "n2", Kind: c.EdgeDerives}},
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
	for _, want := range []string{"Summary: Dalio summary", "Nodes: 2", "Edges: 1", "Topics: macro", "Confidence: high"} {
		if !strings.Contains(out, want) {
			t.Fatalf("stdout missing %q in %q", want, out)
		}
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
				Summary: "一句话",
				Graph: c.ReasoningGraph{
					Nodes: []c.GraphNode{{ID: "n1", Kind: c.NodeFact, Text: "事实A"}, {ID: "n2", Kind: c.NodeConclusion, Text: "结论B"}},
					Edges: []c.GraphEdge{{From: "n1", To: "n2", Kind: c.EdgeDerives}},
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
	for _, want := range []string{"Raw preview: 原文正文", "Summary: 一句话", "Nodes: 2", "Edges: 1", "Confidence: medium"} {
		if !strings.Contains(out, want) {
			t.Fatalf("stdout missing %q in %q", want, out)
		}
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
				Summary: "Dalio summary",
				Graph: c.ReasoningGraph{
					Nodes: []c.GraphNode{{ID: "n1", Kind: c.NodeFact, Text: "事实A"}, {ID: "n2", Kind: c.NodeConclusion, Text: "结论B"}},
					Edges: []c.GraphEdge{{From: "n1", To: "n2", Kind: c.EdgeDerives}},
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
	for _, want := range []string{"Raw preview: dalio raw body", "Summary: Dalio summary", "Nodes: 2", "Edges: 1", "Confidence: high"} {
		if !strings.Contains(out, want) {
			t.Fatalf("stdout missing %q in %q", want, out)
		}
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
				Summary: "一句话总结",
				Graph: c.ReasoningGraph{
					Nodes: []c.GraphNode{{ID: "n1", Kind: c.NodeFact, Text: "事实A"}, {ID: "n2", Kind: c.NodeConclusion, Text: "结论B"}},
					Edges: []c.GraphEdge{{From: "n1", To: "n2", Kind: c.EdgeDerives}},
				},
				Details:    c.HiddenDetails{Caveats: []string{"说明"}},
				Topics:     []string{"topic-a", "topic-b"},
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
	for _, want := range []string{"Summary", "一句话总结", "Topics", "topic-a", "Logic chain", "事实A --推出--> 结论B", "Confidence", "high"} {
		if !strings.Contains(out, want) {
			t.Fatalf("stdout missing %q in %q", want, out)
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
				Summary: "一句话总结",
				Graph: c.ReasoningGraph{
					Nodes: []c.GraphNode{{ID: "n1", Kind: c.NodeFact, Text: "事实A"}, {ID: "n2", Kind: c.NodeConclusion, Text: "结论B"}, {ID: "n3", Kind: c.NodePrediction, Text: "预测C"}},
					Edges: []c.GraphEdge{{From: "n1", To: "n2", Kind: c.EdgeDerives}},
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
	for _, want := range []string{"Summary", "一句话总结", "Key points", "- 事实A", "- 结论B", "- 预测C", "Main logic", "事实A --推出--> 结论B", "Confidence", "high"} {
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
				Summary: "一句话总结",
				Graph: c.ReasoningGraph{
					Nodes: []c.GraphNode{{ID: "n1", Kind: c.NodeFact, Text: "事实A"}, {ID: "n2", Kind: c.NodeConclusion, Text: "结论B"}, {ID: "n3", Kind: c.NodePrediction, Text: "预测C"}},
					Edges: []c.GraphEdge{{From: "n1", To: "n2", Kind: c.EdgeDerives}},
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
	for _, want := range []string{"Summary", "一句话总结", "Key points", "- 事实A", "Main logic", "事实A --推出--> 结论B", "Confidence", "high"} {
		if !strings.Contains(out, want) {
			t.Fatalf("stdout missing %q in %q", want, out)
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
				Graph: c.ReasoningGraph{
					Nodes: []c.GraphNode{
						{ID: "n1", Kind: c.NodeFact, Text: "事实A"},
						{ID: "n2", Kind: c.NodeFact, Text: "事实B"},
						{ID: "n3", Kind: c.NodeConclusion, Text: "结论C"},
					},
					Edges: []c.GraphEdge{
						{From: "n1", To: "n2", Kind: c.EdgePositive},
						{From: "n2", To: "n3", Kind: c.EdgeDerives},
					},
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
	if !strings.Contains(out, "事实A --正向--> 事实B --推出--> 结论C") {
		t.Fatalf("stdout missing collapsed chain in %q", out)
	}
}
