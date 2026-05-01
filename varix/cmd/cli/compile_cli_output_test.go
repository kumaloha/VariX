package main

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/kumaloha/VariX/varix/bootstrap"
	c "github.com/kumaloha/VariX/varix/compile"
	"github.com/kumaloha/VariX/varix/ingest/dispatcher"
	"github.com/kumaloha/VariX/varix/ingest/types"
	"github.com/kumaloha/VariX/varix/storage/contentstore"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

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
