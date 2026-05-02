package main

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	c "github.com/kumaloha/VariX/varix/compile"
	"github.com/kumaloha/VariX/varix/ingest"
	varixllm "github.com/kumaloha/VariX/varix/llm"
	"github.com/kumaloha/VariX/varix/memory"
	"github.com/kumaloha/VariX/varix/storage/contentstore"
)

func TestRunMemoryGlobalOrganizeRunAndShow(t *testing.T) {
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
		recordA := c.Record{
			UnitID:         "weibo:G1",
			Source:         "weibo",
			ExternalID:     "G1",
			RootExternalID: "G1",
			Model:          varixllm.Qwen36PlusModel,
			Output: c.Output{
				Summary: "一句话",
				Graph: c.ReasoningGraph{
					Nodes: []c.GraphNode{
						{ID: "n1", Kind: c.NodeFact, Text: "油价会上升", OccurredAt: time.Date(2026, 4, 10, 0, 0, 0, 0, time.UTC)},
						{ID: "n2", Kind: c.NodeExplicitCondition, Text: "若地缘冲突升级"},
					},
					Edges: []c.GraphEdge{{From: "n1", To: "n2", Kind: c.EdgePositive}},
				},
				Details: c.HiddenDetails{Caveats: []string{"说明"}},
			},
			CompiledAt: time.Now().UTC(),
		}
		recordB := c.Record{
			UnitID:         "twitter:G2",
			Source:         "twitter",
			ExternalID:     "G2",
			RootExternalID: "G2",
			Model:          varixllm.Qwen36PlusModel,
			Output: c.Output{
				Summary: "一句话",
				Graph: c.ReasoningGraph{
					Nodes: []c.GraphNode{
						{ID: "n1", Kind: c.NodeFact, Text: "油价会下降", OccurredAt: time.Date(2026, 4, 11, 0, 0, 0, 0, time.UTC)},
						{ID: "n2", Kind: c.NodeConclusion, Text: "结论C"},
					},
					Edges: []c.GraphEdge{{From: "n1", To: "n2", Kind: c.EdgePositive}},
				},
				Details: c.HiddenDetails{Caveats: []string{"说明"}},
			},
			CompiledAt: time.Now().UTC(),
		}
		if err := store.UpsertCompiledOutput(context.Background(), recordA); err != nil {
			return nil, err
		}
		if err := store.UpsertCompiledOutput(context.Background(), recordB); err != nil {
			return nil, err
		}
		if _, err := store.AcceptMemoryNodes(context.Background(), memory.AcceptRequest{UserID: "u-global", SourcePlatform: "weibo", SourceExternalID: "G1", NodeIDs: []string{"n1", "n2"}}); err != nil {
			return nil, err
		}
		if _, err := store.AcceptMemoryNodes(context.Background(), memory.AcceptRequest{UserID: "u-global", SourcePlatform: "twitter", SourceExternalID: "G2", NodeIDs: []string{"n1"}}); err != nil {
			return nil, err
		}
		return store, nil
	}

	var stdout, stderr bytes.Buffer
	code := run([]string{"memory", "global-organize-run", "--user", "u-global"}, "/tmp/project", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("global-organize-run code = %d, stderr = %s", code, stderr.String())
	}
	var out memory.GlobalOrganizationOutput
	if err := json.Unmarshal(stdout.Bytes(), &out); err != nil {
		t.Fatalf("json.Unmarshal(global-organize-run) error = %v", err)
	}
	if len(out.Clusters) == 0 {
		t.Fatalf("output = %#v, want clusters", out)
	}

	stdout.Reset()
	stderr.Reset()
	code = run([]string{"memory", "global-organized", "--user", "u-global"}, "/tmp/project", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("global-organized code = %d, stderr = %s", code, stderr.String())
	}
	var got memory.GlobalOrganizationOutput
	if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
		t.Fatalf("json.Unmarshal(global-organized) error = %v", err)
	}
	if got.OutputID == 0 || len(got.Clusters) == 0 {
		t.Fatalf("global output = %#v", got)
	}
	foundNeutral := false
	for _, cluster := range got.Clusters {
		if strings.Contains(cluster.CanonicalProposition, "关于「") {
			foundNeutral = true
			break
		}
	}
	if !foundNeutral {
		t.Fatalf("clusters = %#v, want at least one neutral contradiction-centered proposition", got.Clusters)
	}
}

func TestRunMemoryGlobalCardPrintsClusterSections(t *testing.T) {
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
			UnitID:         "weibo:GC1",
			Source:         "weibo",
			ExternalID:     "GC1",
			RootExternalID: "GC1",
			Model:          varixllm.Qwen36PlusModel,
			Output: c.Output{
				Summary: "一句话",
				Graph: c.ReasoningGraph{
					Nodes: []c.GraphNode{
						{ID: "n1", Kind: c.NodeFact, Text: "事实A", OccurredAt: time.Date(2026, 4, 10, 0, 0, 0, 0, time.UTC)},
						{ID: "n2", Kind: c.NodeConclusion, Text: "结论B"},
						{ID: "n3", Kind: c.NodePrediction, Text: "预测C", PredictionStartAt: time.Date(2026, 4, 10, 0, 0, 0, 0, time.UTC)},
					},
					Edges: []c.GraphEdge{{From: "n1", To: "n2", Kind: c.EdgeDerives}, {From: "n2", To: "n3", Kind: c.EdgeDerives}},
				},
				Details: c.HiddenDetails{Caveats: []string{"说明"}},
			},
			CompiledAt: time.Now().UTC(),
		}
		if err := store.UpsertCompiledOutput(context.Background(), record); err != nil {
			return nil, err
		}
		if _, err := store.AcceptMemoryNodes(context.Background(), memory.AcceptRequest{UserID: "u-card", SourcePlatform: "weibo", SourceExternalID: "GC1", NodeIDs: []string{"n1", "n2", "n3"}}); err != nil {
			return nil, err
		}
		if _, err := store.RunGlobalMemoryOrganization(context.Background(), "u-card", time.Now().UTC()); err != nil {
			return nil, err
		}
		return store, nil
	}

	var stdout, stderr bytes.Buffer
	code := run([]string{"memory", "global-card", "--user", "u-card"}, "/tmp/project", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("global-card code = %d, stderr = %s", code, stderr.String())
	}
	out := stdout.String()
	for _, want := range []string{"Cluster", "事实A", "Current judgment", "结论B", "What next", "预测C"} {
		if !strings.Contains(out, want) {
			t.Fatalf("stdout missing %q in %q", want, out)
		}
	}
}

func TestRunMemoryGlobalSynthesisOrganizeAndShow(t *testing.T) {
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
			UnitID:         "weibo:GSYN",
			Source:         "weibo",
			ExternalID:     "GSYN",
			RootExternalID: "GSYN",
			Model:          varixllm.Qwen36PlusModel,
			Output: c.Output{
				Summary: "一句话",
				Graph: c.ReasoningGraph{
					Nodes: []c.GraphNode{
						{ID: "n1", Kind: c.NodeFact, Text: "流动性收紧", OccurredAt: time.Date(2026, 4, 10, 0, 0, 0, 0, time.UTC)},
						{ID: "n2", Kind: c.NodeConclusion, Text: "风险资产承压"},
						{ID: "n3", Kind: c.NodePrediction, Text: "未来数月波动加大", PredictionStartAt: time.Date(2026, 4, 10, 0, 0, 0, 0, time.UTC)},
					},
					Edges: []c.GraphEdge{{From: "n1", To: "n2", Kind: c.EdgeDerives}, {From: "n2", To: "n3", Kind: c.EdgeDerives}},
				},
				Details: c.HiddenDetails{Caveats: []string{"说明"}},
			},
			CompiledAt: time.Now().UTC(),
		}
		if err := store.UpsertCompiledOutput(context.Background(), record); err != nil {
			return nil, err
		}
		if _, err := store.AcceptMemoryNodes(context.Background(), memory.AcceptRequest{UserID: "u-synthesis-cli", SourcePlatform: "weibo", SourceExternalID: "GSYN", NodeIDs: []string{"n1", "n2", "n3"}}); err != nil {
			return nil, err
		}
		return store, nil
	}

	var stdout, stderr bytes.Buffer
	code := run([]string{"memory", "global-synthesis-run", "--user", "u-synthesis-cli"}, "/tmp/project", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("global-synthesis-run code = %d, stderr = %s", code, stderr.String())
	}
	var out memory.GlobalMemorySynthesisOutput
	if err := json.Unmarshal(stdout.Bytes(), &out); err != nil {
		t.Fatalf("json.Unmarshal(global-synthesis-run) error = %v", err)
	}
	if len(out.CognitiveCards) == 0 || len(out.TopMemoryItems) == 0 {
		t.Fatalf("synthesis output = %#v, want cards + top items", out)
	}

	stdout.Reset()
	stderr.Reset()
	code = run([]string{"memory", "global-synthesis", "--user", "u-synthesis-cli"}, "/tmp/project", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("global-synthesis code = %d, stderr = %s", code, stderr.String())
	}
	var got memory.GlobalMemorySynthesisOutput
	if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
		t.Fatalf("json.Unmarshal(global-synthesis) error = %v", err)
	}
	if got.OutputID == 0 || len(got.CognitiveConclusions) == 0 {
		t.Fatalf("synthesis stored output = %#v, want persisted synthesis result", got)
	}
}

func TestRunMemoryGlobalSynthesisCardShowsItemCountHeader(t *testing.T) {
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
			UnitID: "weibo:COUNT1", Source: "weibo", ExternalID: "COUNT1", RootExternalID: "COUNT1", Model: varixllm.Qwen36PlusModel,
			Output: c.Output{Summary: "一句话", Graph: c.ReasoningGraph{Nodes: []c.GraphNode{
				{ID: "n1", Kind: c.NodeFact, Text: "流动性收紧", OccurredAt: time.Date(2026, 4, 10, 0, 0, 0, 0, time.UTC)},
				{ID: "n2", Kind: c.NodeConclusion, Text: "风险资产承压"},
				{ID: "n3", Kind: c.NodePrediction, Text: "未来数月波动加大", PredictionStartAt: time.Date(2026, 4, 10, 0, 0, 0, 0, time.UTC)},
			}, Edges: []c.GraphEdge{{From: "n1", To: "n2", Kind: c.EdgeDerives}, {From: "n2", To: "n3", Kind: c.EdgeDerives}}}, Details: c.HiddenDetails{Caveats: []string{"说明"}}},
			CompiledAt: time.Now().UTC(),
		}
		if err := store.UpsertCompiledOutput(context.Background(), record); err != nil {
			return nil, err
		}
		if _, err := store.AcceptMemoryNodes(context.Background(), memory.AcceptRequest{UserID: "u-synthesis-count", SourcePlatform: "weibo", SourceExternalID: "COUNT1", NodeIDs: []string{"n1", "n2", "n3"}}); err != nil {
			return nil, err
		}
		if _, err := store.RunGlobalMemorySynthesis(context.Background(), "u-synthesis-count", time.Now().UTC()); err != nil {
			return nil, err
		}
		return store, nil
	}

	var stdout, stderr bytes.Buffer
	code := run([]string{"memory", "global-synthesis-card", "--user", "u-synthesis-count"}, "/tmp/project", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("global-synthesis-card code = %d, stderr = %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "Items\n1") {
		t.Fatalf("stdout = %q, want item count header", stdout.String())
	}
}

func TestRunMemoryGlobalSynthesisCardPrintsConclusionSections(t *testing.T) {
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
			UnitID:         "weibo:GSYNC",
			Source:         "weibo",
			ExternalID:     "GSYNC",
			RootExternalID: "GSYNC",
			Model:          varixllm.Qwen36PlusModel,
			Output: c.Output{
				Summary: "一句话",
				Graph: c.ReasoningGraph{
					Nodes: []c.GraphNode{
						{ID: "n1", Kind: c.NodeFact, Text: "流动性收紧", OccurredAt: time.Date(2026, 4, 10, 0, 0, 0, 0, time.UTC)},
						{ID: "n2", Kind: c.NodeConclusion, Text: "风险资产承压"},
						{ID: "n3", Kind: c.NodePrediction, Text: "未来数月波动加大", PredictionStartAt: time.Date(2026, 4, 10, 0, 0, 0, 0, time.UTC)},
						{ID: "n4", Kind: c.NodeExplicitCondition, Text: "若融资环境继续恶化"},
					},
					Edges: []c.GraphEdge{
						{From: "n1", To: "n2", Kind: c.EdgeDerives},
						{From: "n2", To: "n3", Kind: c.EdgeDerives},
						{From: "n4", To: "n3", Kind: c.EdgePresets},
					},
				},
				Details: c.HiddenDetails{Caveats: []string{"说明"}},
			},
			CompiledAt: time.Now().UTC(),
		}
		if err := store.UpsertCompiledOutput(context.Background(), record); err != nil {
			return nil, err
		}
		if _, err := store.AcceptMemoryNodes(context.Background(), memory.AcceptRequest{UserID: "u-synthesis-card", SourcePlatform: "weibo", SourceExternalID: "GSYNC", NodeIDs: []string{"n1", "n2", "n3", "n4"}}); err != nil {
			return nil, err
		}
		if _, err := store.RunGlobalMemorySynthesis(context.Background(), "u-synthesis-card", time.Now().UTC()); err != nil {
			return nil, err
		}
		return store, nil
	}

	var stdout, stderr bytes.Buffer
	code := run([]string{"memory", "global-synthesis-card", "--user", "u-synthesis-card"}, "/tmp/project", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("global-synthesis-card code = %d, stderr = %s", code, stderr.String())
	}
	out := stdout.String()
	for _, want := range []string{"Conclusion", "Signal", "Logic", "Why", "Conditions", "What next", "Sources", "weibo:GSYNC", "流动性收紧", "若融资环境继续恶化", "未来数月波动加大"} {
		if !strings.Contains(out, want) {
			t.Fatalf("stdout missing %q in %q", want, out)
		}
	}
}

func TestRunMemoryGlobalSynthesisCardPrintsMechanismSection(t *testing.T) {
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
			UnitID: "weibo:MECH1", Source: "weibo", ExternalID: "MECH1", RootExternalID: "MECH1", Model: varixllm.Qwen36PlusModel,
			Output: c.Output{Summary: "一句话", Graph: c.ReasoningGraph{Nodes: []c.GraphNode{
				{ID: "n1", Kind: c.NodeFact, Text: "高资产价格环境延续", OccurredAt: time.Date(2026, 4, 10, 0, 0, 0, 0, time.UTC)},
				{ID: "n2", Kind: c.NodeImplicitCondition, Text: "宏观负面冲击会放大金融系统脆弱性", OccurredAt: time.Date(2026, 4, 10, 0, 0, 0, 0, time.UTC)},
				{ID: "n3", Kind: c.NodeConclusion, Text: "风险资产承压"},
				{ID: "n4", Kind: c.NodePrediction, Text: "未来数月波动加大", PredictionStartAt: time.Date(2026, 4, 10, 0, 0, 0, 0, time.UTC)},
			}, Edges: []c.GraphEdge{{From: "n1", To: "n2", Kind: c.EdgeDerives}, {From: "n2", To: "n3", Kind: c.EdgeDerives}, {From: "n3", To: "n4", Kind: c.EdgeDerives}}}, Details: c.HiddenDetails{Caveats: []string{"说明"}}},
			CompiledAt: time.Now().UTC(),
		}
		if err := store.UpsertCompiledOutput(context.Background(), record); err != nil {
			return nil, err
		}
		if _, err := store.AcceptMemoryNodes(context.Background(), memory.AcceptRequest{UserID: "u-synthesis-mech", SourcePlatform: "weibo", SourceExternalID: "MECH1", NodeIDs: []string{"n1", "n2", "n3", "n4"}}); err != nil {
			return nil, err
		}
		if _, err := store.RunGlobalMemorySynthesis(context.Background(), "u-synthesis-mech", time.Now().UTC()); err != nil {
			return nil, err
		}
		return store, nil
	}

	var stdout, stderr bytes.Buffer
	code := run([]string{"memory", "global-synthesis-card", "--user", "u-synthesis-mech"}, "/tmp/project", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("global-synthesis-card code = %d, stderr = %s", code, stderr.String())
	}
	out := stdout.String()
	for _, want := range []string{"Mechanism", "宏观负面冲击会放大金融系统脆弱性"} {
		if !strings.Contains(out, want) {
			t.Fatalf("stdout missing %q in %q", want, out)
		}
	}
	whyStart := strings.Index(out, "Why\n")
	if whyStart == -1 {
		t.Fatalf("stdout = %q, want Why section", out)
	}
	nextStart := strings.Index(out[whyStart+4:], "\n\nWhat next")
	if nextStart == -1 {
		t.Fatalf("stdout = %q, want What next section after Why", out)
	}
	whyBlock := out[whyStart : whyStart+4+nextStart]
	if strings.Contains(whyBlock, "宏观负面冲击会放大金融系统脆弱性") {
		t.Fatalf("Why section should not repeat mechanism text: %q", whyBlock)
	}
}

func TestRunMemoryGlobalSynthesisCardRunFlagBuildsFreshOutput(t *testing.T) {
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
			UnitID:         "weibo:GSYNR",
			Source:         "weibo",
			ExternalID:     "GSYNR",
			RootExternalID: "GSYNR",
			Model:          varixllm.Qwen36PlusModel,
			Output: c.Output{
				Summary: "一句话",
				Graph: c.ReasoningGraph{
					Nodes: []c.GraphNode{
						{ID: "n1", Kind: c.NodeFact, Text: "流动性收紧", OccurredAt: time.Date(2026, 4, 10, 0, 0, 0, 0, time.UTC)},
						{ID: "n2", Kind: c.NodeConclusion, Text: "风险资产承压"},
						{ID: "n3", Kind: c.NodePrediction, Text: "未来数月波动加大", PredictionStartAt: time.Date(2026, 4, 10, 0, 0, 0, 0, time.UTC)},
					},
					Edges: []c.GraphEdge{{From: "n1", To: "n2", Kind: c.EdgeDerives}, {From: "n2", To: "n3", Kind: c.EdgeDerives}},
				},
				Details: c.HiddenDetails{Caveats: []string{"说明"}},
			},
			CompiledAt: time.Now().UTC(),
		}
		if err := store.UpsertCompiledOutput(context.Background(), record); err != nil {
			return nil, err
		}
		if _, err := store.AcceptMemoryNodes(context.Background(), memory.AcceptRequest{UserID: "u-synthesis-card-run", SourcePlatform: "weibo", SourceExternalID: "GSYNR", NodeIDs: []string{"n1", "n2", "n3"}}); err != nil {
			return nil, err
		}
		return store, nil
	}

	var stdout, stderr bytes.Buffer
	code := run([]string{"memory", "global-synthesis-card", "--run", "--user", "u-synthesis-card-run"}, "/tmp/project", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("global-synthesis-card --run code = %d, stderr = %s", code, stderr.String())
	}
	out := stdout.String()
	for _, want := range []string{"Conclusion", "Signal", "Logic", "流动性收紧", "风险资产承压"} {
		if !strings.Contains(out, want) {
			t.Fatalf("stdout missing %q in %q", want, out)
		}
	}
}

func TestRunMemoryGlobalSynthesisCardSuggestsRunWhenNoStoredOutput(t *testing.T) {
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
		return store, nil
	}

	var stdout, stderr bytes.Buffer
	code := run([]string{"memory", "global-synthesis-card", "--user", "u-empty"}, "/tmp/project", &stdout, &stderr)
	if code != 1 {
		t.Fatalf("global-synthesis-card code = %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), "memory global-synthesis-card --run --user u-empty") {
		t.Fatalf("stderr = %q, want --run guidance", stderr.String())
	}
}

func TestRunMemoryGlobalSynthesisOrganizedSuggestsOrganizeRunWhenEmpty(t *testing.T) {
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
		return store, nil
	}

	var stdout, stderr bytes.Buffer
	code := run([]string{"memory", "global-synthesis", "--user", "u-empty"}, "/tmp/project", &stdout, &stderr)
	if code != 1 {
		t.Fatalf("global-synthesis code = %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), "memory global-synthesis-run --user u-empty") {
		t.Fatalf("stderr = %q, want organize-run guidance", stderr.String())
	}
}

func TestRunMemoryGlobalSynthesisCardPrintsConflictSides(t *testing.T) {
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
		recordA := c.Record{
			UnitID:         "weibo:CF1",
			Source:         "weibo",
			ExternalID:     "CF1",
			RootExternalID: "CF1",
			Model:          varixllm.Qwen36PlusModel,
			Output: c.Output{
				Summary: "一句话",
				Graph: c.ReasoningGraph{
					Nodes: []c.GraphNode{
						{ID: "n1", Kind: c.NodeFact, Text: "供给趋紧", OccurredAt: time.Date(2026, 4, 10, 0, 0, 0, 0, time.UTC)},
						{ID: "n2", Kind: c.NodeConclusion, Text: "油价会上升"},
					},
					Edges: []c.GraphEdge{{From: "n1", To: "n2", Kind: c.EdgeDerives}},
				},
				Details: c.HiddenDetails{Caveats: []string{"说明"}},
			},
			CompiledAt: time.Now().UTC(),
		}
		recordB := c.Record{
			UnitID:         "twitter:CF2",
			Source:         "twitter",
			ExternalID:     "CF2",
			RootExternalID: "CF2",
			Model:          varixllm.Qwen36PlusModel,
			Output: c.Output{
				Summary: "一句话",
				Graph: c.ReasoningGraph{
					Nodes: []c.GraphNode{
						{ID: "n1", Kind: c.NodeFact, Text: "需求走弱", OccurredAt: time.Date(2026, 4, 10, 0, 0, 0, 0, time.UTC)},
						{ID: "n2", Kind: c.NodeConclusion, Text: "油价会下降"},
					},
					Edges: []c.GraphEdge{{From: "n1", To: "n2", Kind: c.EdgeDerives}},
				},
				Details: c.HiddenDetails{Caveats: []string{"说明"}},
			},
			CompiledAt: time.Now().UTC(),
		}
		if err := store.UpsertCompiledOutput(context.Background(), recordA); err != nil {
			return nil, err
		}
		if err := store.UpsertCompiledOutput(context.Background(), recordB); err != nil {
			return nil, err
		}
		if _, err := store.AcceptMemoryNodes(context.Background(), memory.AcceptRequest{UserID: "u-synthesis-conflict-card", SourcePlatform: "weibo", SourceExternalID: "CF1", NodeIDs: []string{"n2"}}); err != nil {
			return nil, err
		}
		if _, err := store.AcceptMemoryNodes(context.Background(), memory.AcceptRequest{UserID: "u-synthesis-conflict-card", SourcePlatform: "twitter", SourceExternalID: "CF2", NodeIDs: []string{"n2"}}); err != nil {
			return nil, err
		}
		if _, err := store.RunGlobalMemorySynthesis(context.Background(), "u-synthesis-conflict-card", time.Now().UTC()); err != nil {
			return nil, err
		}
		return store, nil
	}

	var stdout, stderr bytes.Buffer
	code := run([]string{"memory", "global-synthesis-card", "--user", "u-synthesis-conflict-card"}, "/tmp/project", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("global-synthesis-card code = %d, stderr = %s", code, stderr.String())
	}
	out := stdout.String()
	for _, want := range []string{"Conflict", "Side A", "Side B", "Why A", "Why B", "Sources A", "Sources B", "weibo:CF1", "twitter:CF2", "油价会上升", "油价会下降"} {
		if !strings.Contains(out, want) {
			t.Fatalf("stdout missing %q in %q", want, out)
		}
	}
}

func TestRunMemoryGlobalSynthesisCardFiltersByItemType(t *testing.T) {
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
		recordA := c.Record{
			UnitID: "weibo:F1", Source: "weibo", ExternalID: "F1", RootExternalID: "F1", Model: varixllm.Qwen36PlusModel,
			Output: c.Output{Summary: "s", Graph: c.ReasoningGraph{Nodes: []c.GraphNode{
				{ID: "n1", Kind: c.NodeFact, Text: "流动性收紧", OccurredAt: time.Date(2026, 4, 10, 0, 0, 0, 0, time.UTC)},
				{ID: "n2", Kind: c.NodeConclusion, Text: "风险资产承压"},
				{ID: "n3", Kind: c.NodePrediction, Text: "未来数月波动加大", PredictionStartAt: time.Date(2026, 4, 10, 0, 0, 0, 0, time.UTC)},
			}, Edges: []c.GraphEdge{{From: "n1", To: "n2", Kind: c.EdgeDerives}, {From: "n2", To: "n3", Kind: c.EdgeDerives}}}, Details: c.HiddenDetails{Caveats: []string{"说明"}}},
			CompiledAt: time.Now().UTC(),
		}
		recordB := c.Record{
			UnitID: "weibo:F2", Source: "weibo", ExternalID: "F2", RootExternalID: "F2", Model: varixllm.Qwen36PlusModel,
			Output: c.Output{Summary: "s", Graph: c.ReasoningGraph{Nodes: []c.GraphNode{
				{ID: "n1", Kind: c.NodeFact, Text: "供给趋紧", OccurredAt: time.Date(2026, 4, 10, 0, 0, 0, 0, time.UTC)},
				{ID: "n2", Kind: c.NodeConclusion, Text: "油价会上升"},
			}, Edges: []c.GraphEdge{{From: "n1", To: "n2", Kind: c.EdgeDerives}}}, Details: c.HiddenDetails{Caveats: []string{"说明"}}},
			CompiledAt: time.Now().UTC(),
		}
		recordC := c.Record{
			UnitID: "twitter:F3", Source: "twitter", ExternalID: "F3", RootExternalID: "F3", Model: varixllm.Qwen36PlusModel,
			Output: c.Output{Summary: "s", Graph: c.ReasoningGraph{Nodes: []c.GraphNode{
				{ID: "n1", Kind: c.NodeFact, Text: "需求走弱", OccurredAt: time.Date(2026, 4, 10, 0, 0, 0, 0, time.UTC)},
				{ID: "n2", Kind: c.NodeConclusion, Text: "油价会下降"},
			}, Edges: []c.GraphEdge{{From: "n1", To: "n2", Kind: c.EdgeDerives}}}, Details: c.HiddenDetails{Caveats: []string{"说明"}}},
			CompiledAt: time.Now().UTC(),
		}
		for _, record := range []c.Record{recordA, recordB, recordC} {
			if err := store.UpsertCompiledOutput(context.Background(), record); err != nil {
				return nil, err
			}
		}
		for _, req := range []memory.AcceptRequest{
			{UserID: "u-synthesis-filter", SourcePlatform: "weibo", SourceExternalID: "F1", NodeIDs: []string{"n1", "n2", "n3"}},
			{UserID: "u-synthesis-filter", SourcePlatform: "weibo", SourceExternalID: "F2", NodeIDs: []string{"n2"}},
			{UserID: "u-synthesis-filter", SourcePlatform: "twitter", SourceExternalID: "F3", NodeIDs: []string{"n2"}},
		} {
			if _, err := store.AcceptMemoryNodes(context.Background(), req); err != nil {
				return nil, err
			}
		}
		if _, err := store.RunGlobalMemorySynthesis(context.Background(), "u-synthesis-filter", time.Now().UTC()); err != nil {
			return nil, err
		}
		return store, nil
	}

	var stdout, stderr bytes.Buffer
	code := run([]string{"memory", "global-synthesis-card", "--user", "u-synthesis-filter", "--item-type", "conflict"}, "/tmp/project", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("conflict filter code = %d, stderr = %s", code, stderr.String())
	}
	if strings.Contains(stdout.String(), "Conclusion") || !strings.Contains(stdout.String(), "Conflict") {
		t.Fatalf("conflict-only stdout = %q, want only conflict items", stdout.String())
	}
	if !strings.Contains(stdout.String(), "Items (1, filter=conflict)") {
		t.Fatalf("stdout = %q, want item header with filter context", stdout.String())
	}

	stdout.Reset()
	stderr.Reset()
	code = run([]string{"memory", "global-synthesis-card", "--user", "u-synthesis-filter", "--item-type", "conclusion"}, "/tmp/project", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("conclusion filter code = %d, stderr = %s", code, stderr.String())
	}
	if strings.Contains(stdout.String(), "Conflict") || !strings.Contains(stdout.String(), "Conclusion") {
		t.Fatalf("conclusion-only stdout = %q, want only conclusion items", stdout.String())
	}
	if !strings.Contains(stdout.String(), "Items (1, filter=conclusion)") {
		t.Fatalf("stdout = %q, want item header with filter context", stdout.String())
	}
}

func TestRunMemoryGlobalSynthesisCardRejectsInvalidItemType(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{"memory", "global-synthesis-card", "--user", "u-any", "--item-type", "foo"}, "/tmp/project", &stdout, &stderr)
	if code != 2 {
		t.Fatalf("invalid item-type code = %d, want 2", code)
	}
	if !strings.Contains(stderr.String(), "item-type must be one of: card, conclusion, conflict") {
		t.Fatalf("stderr = %q, want explicit item-type guidance", stderr.String())
	}
}

func TestRunMemoryGlobalSynthesisCardPrintsStandaloneCardItems(t *testing.T) {
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
			UnitID:         "weibo:CardOnly1",
			Source:         "weibo",
			ExternalID:     "CardOnly1",
			RootExternalID: "CardOnly1",
			Model:          varixllm.Qwen36PlusModel,
			Output: c.Output{
				Summary: "s",
				Graph: c.ReasoningGraph{
					Nodes: []c.GraphNode{
						{ID: "n1", Kind: c.NodeFact, Text: "事实A", OccurredAt: time.Date(2026, 4, 10, 0, 0, 0, 0, time.UTC)},
						{ID: "n2", Kind: c.NodeConclusion, Text: "结论B"},
					},
					Edges: []c.GraphEdge{{From: "n1", To: "n2", Kind: c.EdgeDerives}},
				},
				Details: c.HiddenDetails{Caveats: []string{"说明"}},
			},
			CompiledAt: time.Now().UTC(),
		}
		if err := store.UpsertCompiledOutput(context.Background(), record); err != nil {
			return nil, err
		}
		if _, err := store.AcceptMemoryNodes(context.Background(), memory.AcceptRequest{
			UserID:           "u-synthesis-card-only",
			SourcePlatform:   "weibo",
			SourceExternalID: "CardOnly1",
			NodeIDs:          []string{"n1"},
		}); err != nil {
			return nil, err
		}
		if _, err := store.RunGlobalMemorySynthesis(context.Background(), "u-synthesis-card-only", time.Now().UTC()); err != nil {
			return nil, err
		}
		return store, nil
	}

	var stdout, stderr bytes.Buffer
	code := run([]string{"memory", "global-synthesis-card", "--user", "u-synthesis-card-only", "--item-type", "card"}, "/tmp/project", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("global-synthesis-card card filter code = %d, stderr = %s", code, stderr.String())
	}
	out := stdout.String()
	for _, want := range []string{"Card", "事实A", "Logic", "Why", "Items (1, filter=card)"} {
		if !strings.Contains(out, want) {
			t.Fatalf("stdout missing %q in %q", want, out)
		}
	}
}

func TestRunMemoryGlobalSynthesisCardReportsWhenFilterMatchesNothing(t *testing.T) {
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
			UnitID: "weibo:N1", Source: "weibo", ExternalID: "N1", RootExternalID: "N1", Model: varixllm.Qwen36PlusModel,
			Output: c.Output{Summary: "s", Graph: c.ReasoningGraph{Nodes: []c.GraphNode{
				{ID: "n1", Kind: c.NodeFact, Text: "流动性收紧", OccurredAt: time.Date(2026, 4, 10, 0, 0, 0, 0, time.UTC)},
				{ID: "n2", Kind: c.NodeConclusion, Text: "风险资产承压"},
			}, Edges: []c.GraphEdge{{From: "n1", To: "n2", Kind: c.EdgeDerives}}}, Details: c.HiddenDetails{Caveats: []string{"说明"}}},
			CompiledAt: time.Now().UTC(),
		}
		if err := store.UpsertCompiledOutput(context.Background(), record); err != nil {
			return nil, err
		}
		if _, err := store.AcceptMemoryNodes(context.Background(), memory.AcceptRequest{UserID: "u-synthesis-empty-filter", SourcePlatform: "weibo", SourceExternalID: "N1", NodeIDs: []string{"n1", "n2"}}); err != nil {
			return nil, err
		}
		if _, err := store.RunGlobalMemorySynthesis(context.Background(), "u-synthesis-empty-filter", time.Now().UTC()); err != nil {
			return nil, err
		}
		return store, nil
	}

	var stdout, stderr bytes.Buffer
	code := run([]string{"memory", "global-synthesis-card", "--user", "u-synthesis-empty-filter", "--item-type", "conflict"}, "/tmp/project", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("empty filtered code = %d, stderr = %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "No conflict items") {
		t.Fatalf("stdout = %q, want no-match guidance", stdout.String())
	}
	if !strings.Contains(stdout.String(), "Items (0, filter=conflict)") {
		t.Fatalf("stdout = %q, want empty item header with filter context", stdout.String())
	}
}

func TestRunMemoryGlobalSynthesisCardRespectsLimit(t *testing.T) {
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
		recordA := c.Record{
			UnitID: "weibo:L1", Source: "weibo", ExternalID: "L1", RootExternalID: "L1", Model: varixllm.Qwen36PlusModel,
			Output: c.Output{Summary: "s", Graph: c.ReasoningGraph{Nodes: []c.GraphNode{
				{ID: "n1", Kind: c.NodeFact, Text: "流动性收紧", OccurredAt: time.Date(2026, 4, 10, 0, 0, 0, 0, time.UTC)},
				{ID: "n2", Kind: c.NodeConclusion, Text: "风险资产承压"},
			}, Edges: []c.GraphEdge{{From: "n1", To: "n2", Kind: c.EdgeDerives}}}, Details: c.HiddenDetails{Caveats: []string{"说明"}}},
			CompiledAt: time.Now().UTC(),
		}
		recordB := c.Record{
			UnitID: "weibo:L2", Source: "weibo", ExternalID: "L2", RootExternalID: "L2", Model: varixllm.Qwen36PlusModel,
			Output: c.Output{Summary: "s", Graph: c.ReasoningGraph{Nodes: []c.GraphNode{
				{ID: "n1", Kind: c.NodeFact, Text: "供给趋紧", OccurredAt: time.Date(2026, 4, 10, 0, 0, 0, 0, time.UTC)},
				{ID: "n2", Kind: c.NodeConclusion, Text: "油价会上升"},
				{ID: "n3", Kind: c.NodePrediction, Text: "油价冲击扩大", PredictionStartAt: time.Date(2026, 4, 10, 0, 0, 0, 0, time.UTC)},
			}, Edges: []c.GraphEdge{{From: "n1", To: "n2", Kind: c.EdgeDerives}, {From: "n2", To: "n3", Kind: c.EdgeDerives}}}, Details: c.HiddenDetails{Caveats: []string{"说明"}}},
			CompiledAt: time.Now().UTC(),
		}
		for _, record := range []c.Record{recordA, recordB} {
			if err := store.UpsertCompiledOutput(context.Background(), record); err != nil {
				return nil, err
			}
		}
		for _, req := range []memory.AcceptRequest{
			{UserID: "u-synthesis-limit", SourcePlatform: "weibo", SourceExternalID: "L1", NodeIDs: []string{"n1", "n2"}},
			{UserID: "u-synthesis-limit", SourcePlatform: "weibo", SourceExternalID: "L2", NodeIDs: []string{"n1", "n2", "n3"}},
		} {
			if _, err := store.AcceptMemoryNodes(context.Background(), req); err != nil {
				return nil, err
			}
		}
		if _, err := store.RunGlobalMemorySynthesis(context.Background(), "u-synthesis-limit", time.Now().UTC()); err != nil {
			return nil, err
		}
		return store, nil
	}

	var stdout, stderr bytes.Buffer
	code := run([]string{"memory", "global-synthesis-card", "--user", "u-synthesis-limit", "--limit", "1"}, "/tmp/project", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("limit code = %d, stderr = %s", code, stderr.String())
	}
	if strings.Count(stdout.String(), "Conclusion\n") != 1 {
		t.Fatalf("stdout = %q, want exactly one rendered card", stdout.String())
	}
}

func TestRunMemoryGlobalCompareShowsClusterAndSynthesisSections(t *testing.T) {
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
			UnitID: "weibo:CMP1", Source: "weibo", ExternalID: "CMP1", RootExternalID: "CMP1", Model: varixllm.Qwen36PlusModel,
			Output: c.Output{Summary: "s", Graph: c.ReasoningGraph{Nodes: []c.GraphNode{
				{ID: "n1", Kind: c.NodeFact, Text: "流动性收紧", OccurredAt: time.Date(2026, 4, 10, 0, 0, 0, 0, time.UTC)},
				{ID: "n2", Kind: c.NodeConclusion, Text: "风险资产承压"},
				{ID: "n3", Kind: c.NodePrediction, Text: "未来数月波动加大", PredictionStartAt: time.Date(2026, 4, 10, 0, 0, 0, 0, time.UTC)},
			}, Edges: []c.GraphEdge{{From: "n1", To: "n2", Kind: c.EdgeDerives}, {From: "n2", To: "n3", Kind: c.EdgeDerives}}}, Details: c.HiddenDetails{Caveats: []string{"说明"}}},
			CompiledAt: time.Now().UTC(),
		}
		if err := store.UpsertCompiledOutput(context.Background(), record); err != nil {
			return nil, err
		}
		if _, err := store.AcceptMemoryNodes(context.Background(), memory.AcceptRequest{UserID: "u-compare", SourcePlatform: "weibo", SourceExternalID: "CMP1", NodeIDs: []string{"n1", "n2", "n3"}}); err != nil {
			return nil, err
		}
		if _, err := store.RunGlobalMemoryOrganization(context.Background(), "u-compare", time.Now().UTC()); err != nil {
			return nil, err
		}
		if _, err := store.RunGlobalMemorySynthesis(context.Background(), "u-compare", time.Now().UTC()); err != nil {
			return nil, err
		}
		return store, nil
	}

	var stdout, stderr bytes.Buffer
	code := run([]string{"memory", "global-compare", "--user", "u-compare"}, "/tmp/project", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("global-compare code = %d, stderr = %s", code, stderr.String())
	}
	out := stdout.String()
	for _, want := range []string{"Cluster-first", "Synthesis", "风险资产承压", "未来数月波动加大"} {
		if !strings.Contains(out, want) {
			t.Fatalf("stdout missing %q in %q", want, out)
		}
	}
}

func TestRunMemoryGlobalCompareRunFlagBuildsFreshOutputs(t *testing.T) {
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
			UnitID: "weibo:CMP2", Source: "weibo", ExternalID: "CMP2", RootExternalID: "CMP2", Model: varixllm.Qwen36PlusModel,
			Output: c.Output{Summary: "s", Graph: c.ReasoningGraph{Nodes: []c.GraphNode{
				{ID: "n1", Kind: c.NodeFact, Text: "流动性收紧", OccurredAt: time.Date(2026, 4, 10, 0, 0, 0, 0, time.UTC)},
				{ID: "n2", Kind: c.NodeConclusion, Text: "风险资产承压"},
			}, Edges: []c.GraphEdge{{From: "n1", To: "n2", Kind: c.EdgeDerives}}}, Details: c.HiddenDetails{Caveats: []string{"说明"}}},
			CompiledAt: time.Now().UTC(),
		}
		if err := store.UpsertCompiledOutput(context.Background(), record); err != nil {
			return nil, err
		}
		if _, err := store.AcceptMemoryNodes(context.Background(), memory.AcceptRequest{UserID: "u-compare-run", SourcePlatform: "weibo", SourceExternalID: "CMP2", NodeIDs: []string{"n1", "n2"}}); err != nil {
			return nil, err
		}
		return store, nil
	}

	var stdout, stderr bytes.Buffer
	code := run([]string{"memory", "global-compare", "--run", "--user", "u-compare-run"}, "/tmp/project", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("global-compare --run code = %d, stderr = %s", code, stderr.String())
	}
	out := stdout.String()
	for _, want := range []string{"Cluster-first", "Synthesis", "风险资产承压"} {
		if !strings.Contains(out, want) {
			t.Fatalf("stdout missing %q in %q", want, out)
		}
	}
}

func TestRunMemoryGlobalCompareRespectsLimit(t *testing.T) {
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
		records := []c.Record{
			{UnitID: "weibo:CL1", Source: "weibo", ExternalID: "CL1", RootExternalID: "CL1", Model: varixllm.Qwen36PlusModel, Output: c.Output{Summary: "s", Graph: c.ReasoningGraph{Nodes: []c.GraphNode{{ID: "n1", Kind: c.NodeFact, Text: "流动性收紧", OccurredAt: time.Date(2026, 4, 10, 0, 0, 0, 0, time.UTC)}, {ID: "n2", Kind: c.NodeConclusion, Text: "风险资产承压"}, {ID: "n3", Kind: c.NodePrediction, Text: "未来数月波动加大", PredictionStartAt: time.Date(2026, 4, 10, 0, 0, 0, 0, time.UTC)}}, Edges: []c.GraphEdge{{From: "n1", To: "n2", Kind: c.EdgeDerives}, {From: "n2", To: "n3", Kind: c.EdgeDerives}}}, Details: c.HiddenDetails{Caveats: []string{"说明"}}}, CompiledAt: time.Now().UTC()},
			{UnitID: "weibo:CL2", Source: "weibo", ExternalID: "CL2", RootExternalID: "CL2", Model: varixllm.Qwen36PlusModel, Output: c.Output{Summary: "s", Graph: c.ReasoningGraph{Nodes: []c.GraphNode{{ID: "n1", Kind: c.NodeFact, Text: "供给趋紧", OccurredAt: time.Date(2026, 4, 10, 0, 0, 0, 0, time.UTC)}, {ID: "n2", Kind: c.NodeConclusion, Text: "油价会上升"}, {ID: "n3", Kind: c.NodePrediction, Text: "油价冲击扩大", PredictionStartAt: time.Date(2026, 4, 10, 0, 0, 0, 0, time.UTC)}}, Edges: []c.GraphEdge{{From: "n1", To: "n2", Kind: c.EdgeDerives}, {From: "n2", To: "n3", Kind: c.EdgeDerives}}}, Details: c.HiddenDetails{Caveats: []string{"说明"}}}, CompiledAt: time.Now().UTC()},
		}
		for _, record := range records {
			if err := store.UpsertCompiledOutput(context.Background(), record); err != nil {
				return nil, err
			}
		}
		for _, req := range []memory.AcceptRequest{
			{UserID: "u-compare-limit", SourcePlatform: "weibo", SourceExternalID: "CL1", NodeIDs: []string{"n1", "n2", "n3"}},
			{UserID: "u-compare-limit", SourcePlatform: "weibo", SourceExternalID: "CL2", NodeIDs: []string{"n1", "n2", "n3"}},
		} {
			if _, err := store.AcceptMemoryNodes(context.Background(), req); err != nil {
				return nil, err
			}
		}
		if _, err := store.RunGlobalMemoryOrganization(context.Background(), "u-compare-limit", time.Now().UTC()); err != nil {
			return nil, err
		}
		if _, err := store.RunGlobalMemorySynthesis(context.Background(), "u-compare-limit", time.Now().UTC()); err != nil {
			return nil, err
		}
		return store, nil
	}

	var stdout, stderr bytes.Buffer
	code := run([]string{"memory", "global-compare", "--user", "u-compare-limit", "--limit", "1"}, "/tmp/project", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("global-compare limit code = %d, stderr = %s", code, stderr.String())
	}
	if strings.Count(stdout.String(), "- ") != 2 {
		t.Fatalf("stdout = %q, want one cluster item and one synthesis item", stdout.String())
	}
}

func TestRunMemoryGlobalCompareFiltersSynthesisItemType(t *testing.T) {
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
		records := []c.Record{
			{UnitID: "weibo:CFV1", Source: "weibo", ExternalID: "CFV1", RootExternalID: "CFV1", Model: varixllm.Qwen36PlusModel, Output: c.Output{Summary: "s", Graph: c.ReasoningGraph{Nodes: []c.GraphNode{{ID: "n1", Kind: c.NodeFact, Text: "供给趋紧", OccurredAt: time.Date(2026, 4, 10, 0, 0, 0, 0, time.UTC)}, {ID: "n2", Kind: c.NodeConclusion, Text: "油价会上升"}}, Edges: []c.GraphEdge{{From: "n1", To: "n2", Kind: c.EdgeDerives}}}, Details: c.HiddenDetails{Caveats: []string{"说明"}}}, CompiledAt: time.Now().UTC()},
			{UnitID: "twitter:CFSynthesis", Source: "twitter", ExternalID: "CFSynthesis", RootExternalID: "CFSynthesis", Model: varixllm.Qwen36PlusModel, Output: c.Output{Summary: "s", Graph: c.ReasoningGraph{Nodes: []c.GraphNode{{ID: "n1", Kind: c.NodeFact, Text: "需求走弱", OccurredAt: time.Date(2026, 4, 10, 0, 0, 0, 0, time.UTC)}, {ID: "n2", Kind: c.NodeConclusion, Text: "油价会下降"}}, Edges: []c.GraphEdge{{From: "n1", To: "n2", Kind: c.EdgeDerives}}}, Details: c.HiddenDetails{Caveats: []string{"说明"}}}, CompiledAt: time.Now().UTC()},
			{UnitID: "weibo:CF3", Source: "weibo", ExternalID: "CF3", RootExternalID: "CF3", Model: varixllm.Qwen36PlusModel, Output: c.Output{Summary: "s", Graph: c.ReasoningGraph{Nodes: []c.GraphNode{{ID: "n1", Kind: c.NodeFact, Text: "流动性收紧", OccurredAt: time.Date(2026, 4, 10, 0, 0, 0, 0, time.UTC)}, {ID: "n2", Kind: c.NodeConclusion, Text: "风险资产承压"}, {ID: "n3", Kind: c.NodePrediction, Text: "未来数月波动加大", PredictionStartAt: time.Date(2026, 4, 10, 0, 0, 0, 0, time.UTC)}}, Edges: []c.GraphEdge{{From: "n1", To: "n2", Kind: c.EdgeDerives}, {From: "n2", To: "n3", Kind: c.EdgeDerives}}}, Details: c.HiddenDetails{Caveats: []string{"说明"}}}, CompiledAt: time.Now().UTC()},
		}
		for _, record := range records {
			if err := store.UpsertCompiledOutput(context.Background(), record); err != nil {
				return nil, err
			}
		}
		for _, req := range []memory.AcceptRequest{
			{UserID: "u-compare-filter", SourcePlatform: "weibo", SourceExternalID: "CFV1", NodeIDs: []string{"n2"}},
			{UserID: "u-compare-filter", SourcePlatform: "twitter", SourceExternalID: "CFSynthesis", NodeIDs: []string{"n2"}},
			{UserID: "u-compare-filter", SourcePlatform: "weibo", SourceExternalID: "CF3", NodeIDs: []string{"n1", "n2", "n3"}},
		} {
			if _, err := store.AcceptMemoryNodes(context.Background(), req); err != nil {
				return nil, err
			}
		}
		if _, err := store.RunGlobalMemoryOrganization(context.Background(), "u-compare-filter", time.Now().UTC()); err != nil {
			return nil, err
		}
		if _, err := store.RunGlobalMemorySynthesis(context.Background(), "u-compare-filter", time.Now().UTC()); err != nil {
			return nil, err
		}
		return store, nil
	}

	var stdout, stderr bytes.Buffer
	code := run([]string{"memory", "global-compare", "--user", "u-compare-filter", "--item-type", "conflict"}, "/tmp/project", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("global-compare conflict filter code = %d, stderr = %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "conflict:") || strings.Contains(stdout.String(), "conclusion:") {
		t.Fatalf("stdout = %q, want only synthesis conflict items while keeping cluster section", stdout.String())
	}
	if !strings.Contains(stdout.String(), "Synthesis (1, filter=conflict)") {
		t.Fatalf("stdout = %q, want filter annotation in Synthesis header", stdout.String())
	}
}

func TestRunMemoryGlobalCompareReportsWhenFilteredSynthesisSideIsEmpty(t *testing.T) {
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
			UnitID: "weibo:EC1", Source: "weibo", ExternalID: "EC1", RootExternalID: "EC1", Model: varixllm.Qwen36PlusModel,
			Output: c.Output{Summary: "s", Graph: c.ReasoningGraph{Nodes: []c.GraphNode{
				{ID: "n1", Kind: c.NodeFact, Text: "流动性收紧", OccurredAt: time.Date(2026, 4, 10, 0, 0, 0, 0, time.UTC)},
				{ID: "n2", Kind: c.NodeConclusion, Text: "风险资产承压"},
				{ID: "n3", Kind: c.NodePrediction, Text: "未来数月波动加大", PredictionStartAt: time.Date(2026, 4, 10, 0, 0, 0, 0, time.UTC)},
			}, Edges: []c.GraphEdge{{From: "n1", To: "n2", Kind: c.EdgeDerives}, {From: "n2", To: "n3", Kind: c.EdgeDerives}}}, Details: c.HiddenDetails{Caveats: []string{"说明"}}},
			CompiledAt: time.Now().UTC(),
		}
		if err := store.UpsertCompiledOutput(context.Background(), record); err != nil {
			return nil, err
		}
		if _, err := store.AcceptMemoryNodes(context.Background(), memory.AcceptRequest{UserID: "u-compare-empty-filter", SourcePlatform: "weibo", SourceExternalID: "EC1", NodeIDs: []string{"n1", "n2", "n3"}}); err != nil {
			return nil, err
		}
		if _, err := store.RunGlobalMemoryOrganization(context.Background(), "u-compare-empty-filter", time.Now().UTC()); err != nil {
			return nil, err
		}
		if _, err := store.RunGlobalMemorySynthesis(context.Background(), "u-compare-empty-filter", time.Now().UTC()); err != nil {
			return nil, err
		}
		return store, nil
	}

	var stdout, stderr bytes.Buffer
	code := run([]string{"memory", "global-compare", "--user", "u-compare-empty-filter", "--item-type", "conflict"}, "/tmp/project", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("global-compare empty filter code = %d, stderr = %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "No conflict items") {
		t.Fatalf("stdout = %q, want no-match guidance while keeping compare context", stdout.String())
	}
	if !strings.Contains(stdout.String(), "Synthesis (0, filter=conflict)") {
		t.Fatalf("stdout = %q, want filtered count annotation even when empty", stdout.String())
	}
}

func TestRunMemoryGlobalCompareShowsSectionCounts(t *testing.T) {
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
			UnitID: "weibo:CC1", Source: "weibo", ExternalID: "CC1", RootExternalID: "CC1", Model: varixllm.Qwen36PlusModel,
			Output: c.Output{Summary: "s", Graph: c.ReasoningGraph{Nodes: []c.GraphNode{
				{ID: "n1", Kind: c.NodeFact, Text: "流动性收紧", OccurredAt: time.Date(2026, 4, 10, 0, 0, 0, 0, time.UTC)},
				{ID: "n2", Kind: c.NodeConclusion, Text: "风险资产承压"},
				{ID: "n3", Kind: c.NodePrediction, Text: "未来数月波动加大", PredictionStartAt: time.Date(2026, 4, 10, 0, 0, 0, 0, time.UTC)},
			}, Edges: []c.GraphEdge{{From: "n1", To: "n2", Kind: c.EdgeDerives}, {From: "n2", To: "n3", Kind: c.EdgeDerives}}}, Details: c.HiddenDetails{Caveats: []string{"说明"}}},
			CompiledAt: time.Now().UTC(),
		}
		if err := store.UpsertCompiledOutput(context.Background(), record); err != nil {
			return nil, err
		}
		if _, err := store.AcceptMemoryNodes(context.Background(), memory.AcceptRequest{UserID: "u-compare-count", SourcePlatform: "weibo", SourceExternalID: "CC1", NodeIDs: []string{"n1", "n2", "n3"}}); err != nil {
			return nil, err
		}
		if _, err := store.RunGlobalMemoryOrganization(context.Background(), "u-compare-count", time.Now().UTC()); err != nil {
			return nil, err
		}
		if _, err := store.RunGlobalMemorySynthesis(context.Background(), "u-compare-count", time.Now().UTC()); err != nil {
			return nil, err
		}
		return store, nil
	}

	var stdout, stderr bytes.Buffer
	code := run([]string{"memory", "global-compare", "--user", "u-compare-count"}, "/tmp/project", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("global-compare code = %d, stderr = %s", code, stderr.String())
	}
	out := stdout.String()
	for _, want := range []string{"Cluster-first (", "Synthesis ("} {
		if !strings.Contains(out, want) {
			t.Fatalf("stdout missing %q in %q", want, out)
		}
	}
}

func TestRunMemoryGlobalCompareSuggestsRunWhenNoStoredOutputs(t *testing.T) {
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
		return contentstore.NewSQLiteStore(path)
	}

	var stdout, stderr bytes.Buffer
	code := run([]string{"memory", "global-compare", "--user", "u-empty-compare"}, "/tmp/project", &stdout, &stderr)
	if code != 1 {
		t.Fatalf("global-compare code = %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), "memory global-compare --run --user u-empty-compare") {
		t.Fatalf("stderr = %q, want --run guidance", stderr.String())
	}
}
