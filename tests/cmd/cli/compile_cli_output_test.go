package main

import (
	"bytes"
	"context"
	"encoding/json"
	c "github.com/kumaloha/VariX/varix/compile"
	"github.com/kumaloha/VariX/varix/ingest"
	"github.com/kumaloha/VariX/varix/ingest/dispatcher"
	"github.com/kumaloha/VariX/varix/ingest/types"
	varixllm "github.com/kumaloha/VariX/varix/llm"
	"github.com/kumaloha/VariX/varix/storage/contentstore"
	"strings"
	"testing"
	"time"
)

func TestRunCompileShowReadsCompiledRecordByPlatformAndID(t *testing.T) {
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
			Model:          varixllm.Qwen36PlusModel,
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
	if len(got.Output.Graph.Nodes) != 2 || len(got.Output.Graph.Edges) != 1 {
		t.Fatalf("Graph = %#v, want compile show JSON to include persisted graph", got.Output.Graph)
	}
}

func TestRunCompileShowReadsCompiledRecordByURL(t *testing.T) {
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
			Model:          varixllm.Qwen36PlusModel,
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
	if len(got.Output.Graph.Nodes) != 2 || len(got.Output.Graph.Edges) != 1 {
		t.Fatalf("Graph = %#v, want compile show URL JSON to include persisted graph", got.Output.Graph)
	}
}

func TestRunCompileSummaryPrintsHumanReadableOutput(t *testing.T) {
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
			UnitID:         "weibo:QAu4U9USk",
			Source:         "weibo",
			ExternalID:     "QAu4U9USk",
			RootExternalID: "QAu4U9USk",
			Model:          varixllm.Qwen36PlusModel,
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

func TestRunCompileSummaryReadsByURL(t *testing.T) {
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
			Model:          varixllm.Qwen36PlusModel,
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
			Model:          varixllm.Qwen36PlusModel,
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
			Model:          varixllm.Qwen36PlusModel,
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
