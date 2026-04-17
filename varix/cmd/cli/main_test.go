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
	"github.com/kumaloha/VariX/varix/ingest/polling"
	"github.com/kumaloha/VariX/varix/ingest/types"
	"github.com/kumaloha/VariX/varix/memory"
	"github.com/kumaloha/VariX/varix/storage/contentstore"
)

type fakeItemSource struct {
	platform types.Platform
	kind     types.Kind
	items    []types.RawContent
	fetchFn  func(context.Context, types.ParsedURL) ([]types.RawContent, error)
}

func (f fakeItemSource) Platform() types.Platform {
	if f.platform != "" {
		return f.platform
	}
	return types.PlatformWeb
}

func (f fakeItemSource) Kind() types.Kind {
	if f.kind != "" {
		return f.kind
	}
	return types.KindNative
}

func (f fakeItemSource) Fetch(ctx context.Context, parsed types.ParsedURL) ([]types.RawContent, error) {
	if f.fetchFn != nil {
		return f.fetchFn(ctx, parsed)
	}
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

func TestRunMemoryAcceptRequiresFields(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{"memory", "accept"}, "/tmp/project", &stdout, &stderr)
	if code != 2 {
		t.Fatalf("run() code = %d, want 2", code)
	}
	if !strings.Contains(stderr.String(), "usage: varix memory accept") {
		t.Fatalf("stderr = %q, want usage", stderr.String())
	}
}

func TestRunMemoryPosteriorRunRequiresUser(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{"memory", "posterior-run"}, "/tmp/project", &stdout, &stderr)
	if code != 2 {
		t.Fatalf("run() code = %d, want 2", code)
	}
	if !strings.Contains(stderr.String(), "usage: varix memory posterior-run") {
		t.Fatalf("stderr = %q, want usage", stderr.String())
	}
}

func TestRunMemoryAcceptPersistsNodeAndJob(t *testing.T) {
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
			UnitID:         "weibo:Q1",
			Source:         "weibo",
			ExternalID:     "Q1",
			RootExternalID: "Q1",
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
		}
		if err := store.UpsertCompiledOutput(context.Background(), record); err != nil {
			return nil, err
		}
		return store, nil
	}

	var stdout, stderr bytes.Buffer
	code := run([]string{"memory", "accept", "--user", "u1", "--platform", "weibo", "--id", "Q1", "--node", "n1"}, "/tmp/project", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("run() code = %d, stderr = %s", code, stderr.String())
	}
	var got memory.AcceptResult
	if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
		t.Fatalf("json.Unmarshal(stdout) error = %v", err)
	}
	if len(got.Nodes) != 1 || got.Nodes[0].NodeID != "n1" {
		t.Fatalf("got = %#v", got)
	}
	if got.Job.JobID == 0 || got.Event.EventID == 0 {
		t.Fatalf("job/event = %#v / %#v", got.Job, got.Event)
	}
}

func TestRunMemoryAcceptBatchAndList(t *testing.T) {
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
			UnitID:         "weibo:Q1",
			Source:         "weibo",
			ExternalID:     "Q1",
			RootExternalID: "Q1",
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
		}
		if err := store.UpsertCompiledOutput(context.Background(), record); err != nil {
			return nil, err
		}
		return store, nil
	}

	var stdout, stderr bytes.Buffer
	code := run([]string{"memory", "accept-batch", "--user", "u1", "--platform", "weibo", "--id", "Q1", "--nodes", "n1,n2"}, "/tmp/project", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("accept-batch code = %d, stderr = %s", code, stderr.String())
	}
	stdout.Reset()
	stderr.Reset()
	code = run([]string{"memory", "list", "--user", "u1"}, "/tmp/project", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("list code = %d, stderr = %s", code, stderr.String())
	}
	var got []memory.AcceptedNode
	if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
		t.Fatalf("json.Unmarshal(list) error = %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("len(list) = %d, want 2", len(got))
	}
}

func TestRunMemoryAcceptBatchAndListDerivesLegacyValidityFromNodeTiming(t *testing.T) {
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
		occurredAt := time.Date(2026, 4, 10, 9, 0, 0, 0, time.UTC)
		predictionStart := time.Date(2026, 4, 12, 0, 0, 0, 0, time.UTC)
		predictionDue := time.Date(2026, 4, 20, 0, 0, 0, 0, time.UTC)
		record := c.Record{
			UnitID:         "weibo:Q-time",
			Source:         "weibo",
			ExternalID:     "Q-time",
			RootExternalID: "Q-time",
			Model:          c.Qwen36PlusModel,
			Output: c.Output{
				Summary: "一句话",
				Graph: c.ReasoningGraph{
					Nodes: []c.GraphNode{
						{ID: "n1", Kind: c.NodeFact, Text: "事实A", OccurredAt: occurredAt},
						{ID: "n2", Kind: c.NodePrediction, Text: "预测B", PredictionStartAt: predictionStart, PredictionDueAt: predictionDue},
						{ID: "n3", Kind: c.NodeConclusion, Text: "结论C"},
					},
					Edges: []c.GraphEdge{{From: "n1", To: "n3", Kind: c.EdgeDerives}, {From: "n3", To: "n2", Kind: c.EdgeDerives}},
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
	code := run([]string{"memory", "accept-batch", "--user", "u1", "--platform", "weibo", "--id", "Q-time", "--nodes", "n1,n2"}, "/tmp/project", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("accept-batch code = %d, stderr = %s", code, stderr.String())
	}
	stdout.Reset()
	stderr.Reset()
	code = run([]string{"memory", "list", "--user", "u1"}, "/tmp/project", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("list code = %d, stderr = %s", code, stderr.String())
	}
	var got []memory.AcceptedNode
	if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
		t.Fatalf("json.Unmarshal(list) error = %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("len(list) = %d, want 2", len(got))
	}
	if !got[0].ValidFrom.Equal(time.Date(2026, 4, 10, 9, 0, 0, 0, time.UTC)) {
		t.Fatalf("fact ValidFrom = %s, want occurred_at-derived timestamp", got[0].ValidFrom)
	}
	if got[0].ValidTo.Year() != 9999 {
		t.Fatalf("fact ValidTo = %s, want open-ended year 9999", got[0].ValidTo)
	}
	if !got[1].ValidFrom.Equal(time.Date(2026, 4, 12, 0, 0, 0, 0, time.UTC)) || !got[1].ValidTo.Equal(time.Date(2026, 4, 20, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("prediction validity = %s..%s, want prediction_start_at/prediction_due_at-derived window", got[1].ValidFrom, got[1].ValidTo)
	}
}

func TestRunMemoryOrganizeRunAndShow(t *testing.T) {
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
			UnitID:         "weibo:Q1",
			Source:         "weibo",
			ExternalID:     "Q1",
			RootExternalID: "Q1",
			Model:          c.Qwen36PlusModel,
			Output: c.Output{
				Summary: "一句话",
				Graph: c.ReasoningGraph{
					Nodes: []c.GraphNode{
						testGraphNode("n1", c.NodeFact, "通胀下降"),
						testGraphNode("n2", c.NodePrediction, "三个月内降息"),
					},
					Edges: []c.GraphEdge{{From: "n1", To: "n2", Kind: c.EdgeDerives}},
				},
				Details: c.HiddenDetails{Caveats: []string{"说明"}},
				Verification: c.Verification{
					VerifiedAt: time.Now().UTC(),
					Model:      c.Qwen36PlusModel,
					FactChecks: []c.FactCheck{{NodeID: "n1", Status: c.FactStatusClearlyTrue, Reason: "supported"}},
					PredictionChecks: []c.PredictionCheck{{
						NodeID: "n2", Status: c.PredictionStatusUnresolved, Reason: "still active", AsOf: time.Now().UTC(),
					}},
				},
			},
			CompiledAt: time.Now().UTC(),
		}
		if err := store.UpsertCompiledOutput(context.Background(), record); err != nil {
			return nil, err
		}
		if _, err := store.AcceptMemoryNodes(context.Background(), memory.AcceptRequest{
			UserID:           "u1",
			SourcePlatform:   "weibo",
			SourceExternalID: "Q1",
			NodeIDs:          []string{"n1", "n2"},
		}); err != nil {
			return nil, err
		}
		return store, nil
	}

	var stdout, stderr bytes.Buffer
	code := run([]string{"memory", "organize-run", "--user", "u1"}, "/tmp/project", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("organize-run code = %d, stderr = %s", code, stderr.String())
	}
	var out memory.OrganizationOutput
	if err := json.Unmarshal(stdout.Bytes(), &out); err != nil {
		t.Fatalf("json.Unmarshal(organize-run) error = %v", err)
	}
	if out.JobID == 0 {
		t.Fatalf("output = %#v", out)
	}

	stdout.Reset()
	stderr.Reset()
	code = run([]string{"memory", "organized", "--user", "u1", "--platform", "weibo", "--id", "Q1"}, "/tmp/project", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("organized code = %d, stderr = %s", code, stderr.String())
	}
	var got memory.OrganizationOutput
	if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
		t.Fatalf("json.Unmarshal(organized) error = %v", err)
	}
	if got.JobID != out.JobID {
		t.Fatalf("JobID = %d, want %d", got.JobID, out.JobID)
	}
}

func TestRunMemoryOrganizedIncludesFrontendHints(t *testing.T) {
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
			UnitID:         "weibo:Q2",
			Source:         "weibo",
			ExternalID:     "Q2",
			RootExternalID: "Q2",
			Model:          c.Qwen36PlusModel,
			Output: c.Output{
				Summary: "一句话",
				Graph: c.ReasoningGraph{
					Nodes: []c.GraphNode{
						testGraphNode("n1", c.NodeFact, "油价会上升"),
						testGraphNode("n2", c.NodeFact, "油价将上行"),
						testGraphNode("n3", c.NodeConclusion, "能源股受益"),
					},
					Edges: []c.GraphEdge{{From: "n1", To: "n3", Kind: c.EdgePositive}},
				},
				Details: c.HiddenDetails{Caveats: []string{"说明"}},
				Verification: c.Verification{
					VerifiedAt: time.Now().UTC(),
					Model:      c.Qwen36PlusModel,
					FactChecks: []c.FactCheck{{NodeID: "n1", Status: c.FactStatusClearlyTrue, Reason: "supported"}},
				},
			},
			CompiledAt: time.Now().UTC(),
		}
		if err := store.UpsertCompiledOutput(context.Background(), record); err != nil {
			return nil, err
		}
		if _, err := store.AcceptMemoryNodes(context.Background(), memory.AcceptRequest{
			UserID:           "u2",
			SourcePlatform:   "weibo",
			SourceExternalID: "Q2",
			NodeIDs:          []string{"n1", "n2", "n3"},
		}); err != nil {
			return nil, err
		}
		return store, nil
	}

	var stdout, stderr bytes.Buffer
	code := run([]string{"memory", "organize-run", "--user", "u2"}, "/tmp/project", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("organize-run code = %d, stderr = %s", code, stderr.String())
	}

	var payload map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
		t.Fatalf("json.Unmarshal(organize-run payload) error = %v", err)
	}

	dedupeGroups, ok := payload["dedupe_groups"].([]any)
	if !ok || len(dedupeGroups) != 1 {
		t.Fatalf("dedupe_groups = %#v, want one frontend-ready group", payload["dedupe_groups"])
	}
	firstDedupe, ok := dedupeGroups[0].(map[string]any)
	if !ok {
		t.Fatalf("dedupe_groups[0] = %#v, want object", dedupeGroups[0])
	}
	if strings.TrimSpace(stringValue(firstDedupe["canonical_text"])) == "" {
		t.Fatalf("dedupe group missing canonical_text: %#v", firstDedupe)
	}
	if strings.TrimSpace(stringValue(firstDedupe["hint"])) == "" {
		t.Fatalf("dedupe group missing hint: %#v", firstDedupe)
	}

	hierarchy, ok := payload["hierarchy"].([]any)
	if !ok || len(hierarchy) == 0 {
		t.Fatalf("hierarchy = %#v, want frontend-ready link entries", payload["hierarchy"])
	}
	firstLink, ok := hierarchy[0].(map[string]any)
	if !ok {
		t.Fatalf("hierarchy[0] = %#v, want object", hierarchy[0])
	}
	for _, key := range []string{"parent_kind", "child_kind", "source", "hint"} {
		if strings.TrimSpace(stringValue(firstLink[key])) == "" {
			t.Fatalf("hierarchy link missing %s: %#v", key, firstLink)
		}
	}
}

func TestRunMemoryPosteriorRunMarksOrganizedOutputStaleUntilRefreshRun(t *testing.T) {
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
		record := c.Record{
			UnitID:         "weibo:Q-posterior-cli",
			Source:         "weibo",
			ExternalID:     "Q-posterior-cli",
			RootExternalID: "Q-posterior-cli",
			Model:          c.Qwen36PlusModel,
			Output: c.Output{
				Summary: "一句话",
				Graph: c.ReasoningGraph{
					Nodes: []c.GraphNode{
						{ID: "n1", Kind: c.NodeFact, Text: "事实A", OccurredAt: now.Add(-72 * time.Hour)},
						{ID: "n2", Kind: c.NodePrediction, Text: "预测B", PredictionStartAt: now.Add(-48 * time.Hour), PredictionDueAt: now.Add(-24 * time.Hour)},
					},
					Edges: []c.GraphEdge{{From: "n1", To: "n2", Kind: c.EdgeDerives}},
				},
				Details: c.HiddenDetails{Caveats: []string{"说明"}},
				Verification: c.Verification{
					PredictionChecks: []c.PredictionCheck{{
						NodeID: "n2", Status: c.PredictionStatusStaleUnresolved, Reason: "window passed", AsOf: now.Add(-12 * time.Hour),
					}},
				},
			},
			CompiledAt: now.Add(-6 * time.Hour),
		}
		if err := store.UpsertCompiledOutput(context.Background(), record); err != nil {
			return nil, err
		}
		if _, err := store.AcceptMemoryNodes(context.Background(), memory.AcceptRequest{
			UserID:           "u-posterior-cli",
			SourcePlatform:   "weibo",
			SourceExternalID: "Q-posterior-cli",
			NodeIDs:          []string{"n1", "n2"},
		}); err != nil {
			return nil, err
		}
		return store, nil
	}

	var stdout, stderr bytes.Buffer
	code := run([]string{"memory", "organize-run", "--user", "u-posterior-cli"}, "/tmp/project", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("organize-run code = %d, stderr = %s", code, stderr.String())
	}

	stdout.Reset()
	stderr.Reset()
	code = run([]string{"memory", "posterior-run", "--user", "u-posterior-cli", "--platform", "weibo", "--id", "Q-posterior-cli"}, "/tmp/project", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("posterior-run code = %d, stderr = %s", code, stderr.String())
	}
	var posterior memory.PosteriorRunResult
	if err := json.Unmarshal(stdout.Bytes(), &posterior); err != nil {
		t.Fatalf("json.Unmarshal(posterior-run) error = %v", err)
	}
	if len(posterior.Mutated) != 1 || posterior.Mutated[0].NodeID != "n2" {
		t.Fatalf("posterior mutated = %#v, want one mutated prediction node", posterior.Mutated)
	}
	if posterior.Mutated[0].State != memory.PosteriorStatePending {
		t.Fatalf("posterior state = %q, want pending", posterior.Mutated[0].State)
	}
	if len(posterior.Refreshes) != 1 || posterior.Refreshes[0].JobID == 0 {
		t.Fatalf("posterior refreshes = %#v, want one queued refresh job", posterior.Refreshes)
	}

	stdout.Reset()
	stderr.Reset()
	code = run([]string{"memory", "organized", "--user", "u-posterior-cli", "--platform", "weibo", "--id", "Q-posterior-cli"}, "/tmp/project", &stdout, &stderr)
	if code != 1 {
		t.Fatalf("organized stale code = %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), "memory organization output is stale") {
		t.Fatalf("stderr = %q, want stale output error", stderr.String())
	}
	if !strings.Contains(stderr.String(), "memory organize-run --user u-posterior-cli") {
		t.Fatalf("stderr = %q, want rerun guidance", stderr.String())
	}

	stdout.Reset()
	stderr.Reset()
	code = run([]string{"memory", "organize-run", "--user", "u-posterior-cli"}, "/tmp/project", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("refresh organize-run code = %d, stderr = %s", code, stderr.String())
	}

	stdout.Reset()
	stderr.Reset()
	code = run([]string{"memory", "organized", "--user", "u-posterior-cli", "--platform", "weibo", "--id", "Q-posterior-cli"}, "/tmp/project", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("organized refreshed code = %d, stderr = %s", code, stderr.String())
	}
	var organized memory.OrganizationOutput
	if err := json.Unmarshal(stdout.Bytes(), &organized); err != nil {
		t.Fatalf("json.Unmarshal(refreshed organized) error = %v", err)
	}
	foundPosteriorHint := false
	for _, hint := range organized.NodeHints {
		if hint.NodeID == "n2" && hint.PosteriorState == memory.PosteriorStatePending {
			foundPosteriorHint = true
			break
		}
	}
	if !foundPosteriorHint {
		t.Fatalf("NodeHints = %#v, want posterior pending hint for n2", organized.NodeHints)
	}
}

func TestRunMemoryOrganizedWithoutOutputShowsRunGuidance(t *testing.T) {
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
		return contentstore.NewSQLiteStore(path)
	}

	var stdout, stderr bytes.Buffer
	code := run([]string{"memory", "organized", "--user", "u-empty-memory", "--platform", "weibo", "--id", "Q-empty"}, "/tmp/project", &stdout, &stderr)
	if code != 1 {
		t.Fatalf("organized code = %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), "memory organize-run --user u-empty-memory") {
		t.Fatalf("stderr = %q, want organize-run guidance", stderr.String())
	}
}

func TestRunMemoryListAndShowSourceIncludePosteriorState(t *testing.T) {
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
		record := c.Record{
			UnitID:         "weibo:Q-list-posterior",
			Source:         "weibo",
			ExternalID:     "Q-list-posterior",
			RootExternalID: "Q-list-posterior",
			Model:          c.Qwen36PlusModel,
			Output: c.Output{
				Summary: "一句话",
				Graph: c.ReasoningGraph{
					Nodes: []c.GraphNode{
						{ID: "n1", Kind: c.NodeFact, Text: "事实A", OccurredAt: now.Add(-72 * time.Hour)},
						{ID: "n2", Kind: c.NodePrediction, Text: "预测B", PredictionStartAt: now.Add(-48 * time.Hour), PredictionDueAt: now.Add(-24 * time.Hour)},
					},
					Edges: []c.GraphEdge{{From: "n1", To: "n2", Kind: c.EdgeDerives}},
				},
				Details: c.HiddenDetails{Caveats: []string{"说明"}},
				Verification: c.Verification{
					PredictionChecks: []c.PredictionCheck{{
						NodeID: "n2", Status: c.PredictionStatusStaleUnresolved, Reason: "window passed", AsOf: now.Add(-12 * time.Hour),
					}},
				},
			},
			CompiledAt: now.Add(-6 * time.Hour),
		}
		if err := store.UpsertCompiledOutput(context.Background(), record); err != nil {
			return nil, err
		}
		return store, nil
	}

	var stdout, stderr bytes.Buffer
	code := run([]string{"memory", "accept-batch", "--user", "u-list-posterior", "--platform", "weibo", "--id", "Q-list-posterior", "--nodes", "n1,n2"}, "/tmp/project", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("accept-batch code = %d, stderr = %s", code, stderr.String())
	}

	stdout.Reset()
	stderr.Reset()
	code = run([]string{"memory", "posterior-run", "--user", "u-list-posterior", "--platform", "weibo", "--id", "Q-list-posterior"}, "/tmp/project", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("posterior-run code = %d, stderr = %s", code, stderr.String())
	}

	stdout.Reset()
	stderr.Reset()
	code = run([]string{"memory", "list", "--user", "u-list-posterior"}, "/tmp/project", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("list code = %d, stderr = %s", code, stderr.String())
	}
	var listed []memory.AcceptedNode
	if err := json.Unmarshal(stdout.Bytes(), &listed); err != nil {
		t.Fatalf("json.Unmarshal(list) error = %v", err)
	}
	foundListedPosterior := false
	for _, node := range listed {
		if node.NodeID == "n2" && node.PosteriorState == memory.PosteriorStatePending && node.PosteriorReason == "prediction still unresolved after due time" {
			foundListedPosterior = true
			break
		}
	}
	if !foundListedPosterior {
		t.Fatalf("listed nodes = %#v, want posterior projection on n2", listed)
	}

	stdout.Reset()
	stderr.Reset()
	code = run([]string{"memory", "show-source", "--user", "u-list-posterior", "--platform", "weibo", "--id", "Q-list-posterior"}, "/tmp/project", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("show-source code = %d, stderr = %s", code, stderr.String())
	}
	var scoped []memory.AcceptedNode
	if err := json.Unmarshal(stdout.Bytes(), &scoped); err != nil {
		t.Fatalf("json.Unmarshal(show-source) error = %v", err)
	}
	foundScopedPosterior := false
	for _, node := range scoped {
		if node.NodeID == "n2" && node.PosteriorState == memory.PosteriorStatePending && node.PosteriorReason == "prediction still unresolved after due time" {
			foundScopedPosterior = true
			break
		}
	}
	if !foundScopedPosterior {
		t.Fatalf("show-source nodes = %#v, want posterior projection on n2", scoped)
	}
}

func TestRunMemoryGlobalOrganizeRunAndShow(t *testing.T) {
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
		recordA := c.Record{
			UnitID:         "weibo:G1",
			Source:         "weibo",
			ExternalID:     "G1",
			RootExternalID: "G1",
			Model:          c.Qwen36PlusModel,
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
			Model:          c.Qwen36PlusModel,
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
			UnitID:         "weibo:GC1",
			Source:         "weibo",
			ExternalID:     "GC1",
			RootExternalID: "GC1",
			Model:          c.Qwen36PlusModel,
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

func TestRunMemoryGlobalV2OrganizeAndShow(t *testing.T) {
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
			UnitID:         "weibo:GV2",
			Source:         "weibo",
			ExternalID:     "GV2",
			RootExternalID: "GV2",
			Model:          c.Qwen36PlusModel,
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
		if _, err := store.AcceptMemoryNodes(context.Background(), memory.AcceptRequest{UserID: "u-v2-cli", SourcePlatform: "weibo", SourceExternalID: "GV2", NodeIDs: []string{"n1", "n2", "n3"}}); err != nil {
			return nil, err
		}
		return store, nil
	}

	var stdout, stderr bytes.Buffer
	code := run([]string{"memory", "global-v2-organize-run", "--user", "u-v2-cli"}, "/tmp/project", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("global-v2-organize-run code = %d, stderr = %s", code, stderr.String())
	}
	var out memory.GlobalMemoryV2Output
	if err := json.Unmarshal(stdout.Bytes(), &out); err != nil {
		t.Fatalf("json.Unmarshal(global-v2-organize-run) error = %v", err)
	}
	if len(out.CognitiveCards) == 0 || len(out.TopMemoryItems) == 0 {
		t.Fatalf("v2 output = %#v, want cards + top items", out)
	}

	stdout.Reset()
	stderr.Reset()
	code = run([]string{"memory", "global-v2-organized", "--user", "u-v2-cli"}, "/tmp/project", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("global-v2-organized code = %d, stderr = %s", code, stderr.String())
	}
	var got memory.GlobalMemoryV2Output
	if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
		t.Fatalf("json.Unmarshal(global-v2-organized) error = %v", err)
	}
	if got.OutputID == 0 || len(got.CognitiveConclusions) == 0 {
		t.Fatalf("v2 stored output = %#v, want persisted v2 result", got)
	}
}

func TestRunMemoryGlobalV2CardShowsItemCountHeader(t *testing.T) {
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
			UnitID: "weibo:COUNT1", Source: "weibo", ExternalID: "COUNT1", RootExternalID: "COUNT1", Model: c.Qwen36PlusModel,
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
		if _, err := store.AcceptMemoryNodes(context.Background(), memory.AcceptRequest{UserID: "u-v2-count", SourcePlatform: "weibo", SourceExternalID: "COUNT1", NodeIDs: []string{"n1", "n2", "n3"}}); err != nil {
			return nil, err
		}
		if _, err := store.RunGlobalMemoryOrganizationV2(context.Background(), "u-v2-count", time.Now().UTC()); err != nil {
			return nil, err
		}
		return store, nil
	}

	var stdout, stderr bytes.Buffer
	code := run([]string{"memory", "global-v2-card", "--user", "u-v2-count"}, "/tmp/project", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("global-v2-card code = %d, stderr = %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "Items\n1") {
		t.Fatalf("stdout = %q, want item count header", stdout.String())
	}
}

func TestRunMemoryGlobalV2CardPrintsConclusionSections(t *testing.T) {
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
			UnitID:         "weibo:GV2C",
			Source:         "weibo",
			ExternalID:     "GV2C",
			RootExternalID: "GV2C",
			Model:          c.Qwen36PlusModel,
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
		if _, err := store.AcceptMemoryNodes(context.Background(), memory.AcceptRequest{UserID: "u-v2-card", SourcePlatform: "weibo", SourceExternalID: "GV2C", NodeIDs: []string{"n1", "n2", "n3", "n4"}}); err != nil {
			return nil, err
		}
		if _, err := store.RunGlobalMemoryOrganizationV2(context.Background(), "u-v2-card", time.Now().UTC()); err != nil {
			return nil, err
		}
		return store, nil
	}

	var stdout, stderr bytes.Buffer
	code := run([]string{"memory", "global-v2-card", "--user", "u-v2-card"}, "/tmp/project", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("global-v2-card code = %d, stderr = %s", code, stderr.String())
	}
	out := stdout.String()
	for _, want := range []string{"Conclusion", "Signal", "Logic", "Why", "Conditions", "What next", "Sources", "weibo:GV2C", "流动性收紧", "若融资环境继续恶化", "未来数月波动加大"} {
		if !strings.Contains(out, want) {
			t.Fatalf("stdout missing %q in %q", want, out)
		}
	}
}

func TestRunMemoryGlobalV2CardPrintsMechanismSection(t *testing.T) {
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
			UnitID: "weibo:MECH1", Source: "weibo", ExternalID: "MECH1", RootExternalID: "MECH1", Model: c.Qwen36PlusModel,
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
		if _, err := store.AcceptMemoryNodes(context.Background(), memory.AcceptRequest{UserID: "u-v2-mech", SourcePlatform: "weibo", SourceExternalID: "MECH1", NodeIDs: []string{"n1", "n2", "n3", "n4"}}); err != nil {
			return nil, err
		}
		if _, err := store.RunGlobalMemoryOrganizationV2(context.Background(), "u-v2-mech", time.Now().UTC()); err != nil {
			return nil, err
		}
		return store, nil
	}

	var stdout, stderr bytes.Buffer
	code := run([]string{"memory", "global-v2-card", "--user", "u-v2-mech"}, "/tmp/project", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("global-v2-card code = %d, stderr = %s", code, stderr.String())
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

func TestRunMemoryGlobalV2CardRunFlagBuildsFreshOutput(t *testing.T) {
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
			UnitID:         "weibo:GV2R",
			Source:         "weibo",
			ExternalID:     "GV2R",
			RootExternalID: "GV2R",
			Model:          c.Qwen36PlusModel,
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
		if _, err := store.AcceptMemoryNodes(context.Background(), memory.AcceptRequest{UserID: "u-v2-card-run", SourcePlatform: "weibo", SourceExternalID: "GV2R", NodeIDs: []string{"n1", "n2", "n3"}}); err != nil {
			return nil, err
		}
		return store, nil
	}

	var stdout, stderr bytes.Buffer
	code := run([]string{"memory", "global-v2-card", "--run", "--user", "u-v2-card-run"}, "/tmp/project", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("global-v2-card --run code = %d, stderr = %s", code, stderr.String())
	}
	out := stdout.String()
	for _, want := range []string{"Conclusion", "Signal", "Logic", "流动性收紧", "风险资产承压"} {
		if !strings.Contains(out, want) {
			t.Fatalf("stdout missing %q in %q", want, out)
		}
	}
}

func TestRunMemoryGlobalV2CardSuggestsRunWhenNoStoredOutput(t *testing.T) {
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
		return store, nil
	}

	var stdout, stderr bytes.Buffer
	code := run([]string{"memory", "global-v2-card", "--user", "u-empty"}, "/tmp/project", &stdout, &stderr)
	if code != 1 {
		t.Fatalf("global-v2-card code = %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), "memory global-v2-card --run --user u-empty") {
		t.Fatalf("stderr = %q, want --run guidance", stderr.String())
	}
}

func TestRunMemoryGlobalV2OrganizedSuggestsOrganizeRunWhenEmpty(t *testing.T) {
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
		return store, nil
	}

	var stdout, stderr bytes.Buffer
	code := run([]string{"memory", "global-v2-organized", "--user", "u-empty"}, "/tmp/project", &stdout, &stderr)
	if code != 1 {
		t.Fatalf("global-v2-organized code = %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), "memory global-v2-organize-run --user u-empty") {
		t.Fatalf("stderr = %q, want organize-run guidance", stderr.String())
	}
}

func TestRunMemoryGlobalV2CardPrintsConflictSides(t *testing.T) {
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
		recordA := c.Record{
			UnitID:         "weibo:CF1",
			Source:         "weibo",
			ExternalID:     "CF1",
			RootExternalID: "CF1",
			Model:          c.Qwen36PlusModel,
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
			Model:          c.Qwen36PlusModel,
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
		if _, err := store.AcceptMemoryNodes(context.Background(), memory.AcceptRequest{UserID: "u-v2-conflict-card", SourcePlatform: "weibo", SourceExternalID: "CF1", NodeIDs: []string{"n2"}}); err != nil {
			return nil, err
		}
		if _, err := store.AcceptMemoryNodes(context.Background(), memory.AcceptRequest{UserID: "u-v2-conflict-card", SourcePlatform: "twitter", SourceExternalID: "CF2", NodeIDs: []string{"n2"}}); err != nil {
			return nil, err
		}
		if _, err := store.RunGlobalMemoryOrganizationV2(context.Background(), "u-v2-conflict-card", time.Now().UTC()); err != nil {
			return nil, err
		}
		return store, nil
	}

	var stdout, stderr bytes.Buffer
	code := run([]string{"memory", "global-v2-card", "--user", "u-v2-conflict-card"}, "/tmp/project", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("global-v2-card code = %d, stderr = %s", code, stderr.String())
	}
	out := stdout.String()
	for _, want := range []string{"Conflict", "Side A", "Side B", "Why A", "Why B", "Sources A", "Sources B", "weibo:CF1", "twitter:CF2", "油价会上升", "油价会下降"} {
		if !strings.Contains(out, want) {
			t.Fatalf("stdout missing %q in %q", want, out)
		}
	}
}

func TestRunMemoryGlobalV2CardFiltersByItemType(t *testing.T) {
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
		recordA := c.Record{
			UnitID: "weibo:F1", Source: "weibo", ExternalID: "F1", RootExternalID: "F1", Model: c.Qwen36PlusModel,
			Output: c.Output{Summary: "s", Graph: c.ReasoningGraph{Nodes: []c.GraphNode{
				{ID: "n1", Kind: c.NodeFact, Text: "流动性收紧", OccurredAt: time.Date(2026, 4, 10, 0, 0, 0, 0, time.UTC)},
				{ID: "n2", Kind: c.NodeConclusion, Text: "风险资产承压"},
				{ID: "n3", Kind: c.NodePrediction, Text: "未来数月波动加大", PredictionStartAt: time.Date(2026, 4, 10, 0, 0, 0, 0, time.UTC)},
			}, Edges: []c.GraphEdge{{From: "n1", To: "n2", Kind: c.EdgeDerives}, {From: "n2", To: "n3", Kind: c.EdgeDerives}}}, Details: c.HiddenDetails{Caveats: []string{"说明"}}},
			CompiledAt: time.Now().UTC(),
		}
		recordB := c.Record{
			UnitID: "weibo:F2", Source: "weibo", ExternalID: "F2", RootExternalID: "F2", Model: c.Qwen36PlusModel,
			Output: c.Output{Summary: "s", Graph: c.ReasoningGraph{Nodes: []c.GraphNode{
				{ID: "n1", Kind: c.NodeFact, Text: "供给趋紧", OccurredAt: time.Date(2026, 4, 10, 0, 0, 0, 0, time.UTC)},
				{ID: "n2", Kind: c.NodeConclusion, Text: "油价会上升"},
			}, Edges: []c.GraphEdge{{From: "n1", To: "n2", Kind: c.EdgeDerives}}}, Details: c.HiddenDetails{Caveats: []string{"说明"}}},
			CompiledAt: time.Now().UTC(),
		}
		recordC := c.Record{
			UnitID: "twitter:F3", Source: "twitter", ExternalID: "F3", RootExternalID: "F3", Model: c.Qwen36PlusModel,
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
			{UserID: "u-v2-filter", SourcePlatform: "weibo", SourceExternalID: "F1", NodeIDs: []string{"n1", "n2", "n3"}},
			{UserID: "u-v2-filter", SourcePlatform: "weibo", SourceExternalID: "F2", NodeIDs: []string{"n2"}},
			{UserID: "u-v2-filter", SourcePlatform: "twitter", SourceExternalID: "F3", NodeIDs: []string{"n2"}},
		} {
			if _, err := store.AcceptMemoryNodes(context.Background(), req); err != nil {
				return nil, err
			}
		}
		if _, err := store.RunGlobalMemoryOrganizationV2(context.Background(), "u-v2-filter", time.Now().UTC()); err != nil {
			return nil, err
		}
		return store, nil
	}

	var stdout, stderr bytes.Buffer
	code := run([]string{"memory", "global-v2-card", "--user", "u-v2-filter", "--item-type", "conflict"}, "/tmp/project", &stdout, &stderr)
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
	code = run([]string{"memory", "global-v2-card", "--user", "u-v2-filter", "--item-type", "conclusion"}, "/tmp/project", &stdout, &stderr)
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

func TestRunMemoryGlobalV2CardRejectsInvalidItemType(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{"memory", "global-v2-card", "--user", "u-any", "--item-type", "foo"}, "/tmp/project", &stdout, &stderr)
	if code != 2 {
		t.Fatalf("invalid item-type code = %d, want 2", code)
	}
	if !strings.Contains(stderr.String(), "item-type must be one of: card, conclusion, conflict") {
		t.Fatalf("stderr = %q, want explicit item-type guidance", stderr.String())
	}
}

func TestRunMemoryGlobalV2CardPrintsStandaloneCardItems(t *testing.T) {
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
			UnitID:         "weibo:CardOnly1",
			Source:         "weibo",
			ExternalID:     "CardOnly1",
			RootExternalID: "CardOnly1",
			Model:          c.Qwen36PlusModel,
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
			UserID:           "u-v2-card-only",
			SourcePlatform:   "weibo",
			SourceExternalID: "CardOnly1",
			NodeIDs:          []string{"n1"},
		}); err != nil {
			return nil, err
		}
		if _, err := store.RunGlobalMemoryOrganizationV2(context.Background(), "u-v2-card-only", time.Now().UTC()); err != nil {
			return nil, err
		}
		return store, nil
	}

	var stdout, stderr bytes.Buffer
	code := run([]string{"memory", "global-v2-card", "--user", "u-v2-card-only", "--item-type", "card"}, "/tmp/project", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("global-v2-card card filter code = %d, stderr = %s", code, stderr.String())
	}
	out := stdout.String()
	for _, want := range []string{"Card", "事实A", "Logic", "Why", "Items (1, filter=card)"} {
		if !strings.Contains(out, want) {
			t.Fatalf("stdout missing %q in %q", want, out)
		}
	}
}

func TestRunMemoryGlobalV2CardReportsWhenFilterMatchesNothing(t *testing.T) {
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
			UnitID: "weibo:N1", Source: "weibo", ExternalID: "N1", RootExternalID: "N1", Model: c.Qwen36PlusModel,
			Output: c.Output{Summary: "s", Graph: c.ReasoningGraph{Nodes: []c.GraphNode{
				{ID: "n1", Kind: c.NodeFact, Text: "流动性收紧", OccurredAt: time.Date(2026, 4, 10, 0, 0, 0, 0, time.UTC)},
				{ID: "n2", Kind: c.NodeConclusion, Text: "风险资产承压"},
			}, Edges: []c.GraphEdge{{From: "n1", To: "n2", Kind: c.EdgeDerives}}}, Details: c.HiddenDetails{Caveats: []string{"说明"}}},
			CompiledAt: time.Now().UTC(),
		}
		if err := store.UpsertCompiledOutput(context.Background(), record); err != nil {
			return nil, err
		}
		if _, err := store.AcceptMemoryNodes(context.Background(), memory.AcceptRequest{UserID: "u-v2-empty-filter", SourcePlatform: "weibo", SourceExternalID: "N1", NodeIDs: []string{"n1", "n2"}}); err != nil {
			return nil, err
		}
		if _, err := store.RunGlobalMemoryOrganizationV2(context.Background(), "u-v2-empty-filter", time.Now().UTC()); err != nil {
			return nil, err
		}
		return store, nil
	}

	var stdout, stderr bytes.Buffer
	code := run([]string{"memory", "global-v2-card", "--user", "u-v2-empty-filter", "--item-type", "conflict"}, "/tmp/project", &stdout, &stderr)
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

func TestRunMemoryGlobalV2CardRespectsLimit(t *testing.T) {
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
		recordA := c.Record{
			UnitID: "weibo:L1", Source: "weibo", ExternalID: "L1", RootExternalID: "L1", Model: c.Qwen36PlusModel,
			Output: c.Output{Summary: "s", Graph: c.ReasoningGraph{Nodes: []c.GraphNode{
				{ID: "n1", Kind: c.NodeFact, Text: "流动性收紧", OccurredAt: time.Date(2026, 4, 10, 0, 0, 0, 0, time.UTC)},
				{ID: "n2", Kind: c.NodeConclusion, Text: "风险资产承压"},
			}, Edges: []c.GraphEdge{{From: "n1", To: "n2", Kind: c.EdgeDerives}}}, Details: c.HiddenDetails{Caveats: []string{"说明"}}},
			CompiledAt: time.Now().UTC(),
		}
		recordB := c.Record{
			UnitID: "weibo:L2", Source: "weibo", ExternalID: "L2", RootExternalID: "L2", Model: c.Qwen36PlusModel,
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
			{UserID: "u-v2-limit", SourcePlatform: "weibo", SourceExternalID: "L1", NodeIDs: []string{"n1", "n2"}},
			{UserID: "u-v2-limit", SourcePlatform: "weibo", SourceExternalID: "L2", NodeIDs: []string{"n1", "n2", "n3"}},
		} {
			if _, err := store.AcceptMemoryNodes(context.Background(), req); err != nil {
				return nil, err
			}
		}
		if _, err := store.RunGlobalMemoryOrganizationV2(context.Background(), "u-v2-limit", time.Now().UTC()); err != nil {
			return nil, err
		}
		return store, nil
	}

	var stdout, stderr bytes.Buffer
	code := run([]string{"memory", "global-v2-card", "--user", "u-v2-limit", "--limit", "1"}, "/tmp/project", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("limit code = %d, stderr = %s", code, stderr.String())
	}
	if strings.Count(stdout.String(), "Conclusion\n") != 1 {
		t.Fatalf("stdout = %q, want exactly one rendered card", stdout.String())
	}
}

func TestRunMemoryGlobalCompareShowsV1AndV2Sections(t *testing.T) {
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
			UnitID: "weibo:CMP1", Source: "weibo", ExternalID: "CMP1", RootExternalID: "CMP1", Model: c.Qwen36PlusModel,
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
		if _, err := store.RunGlobalMemoryOrganizationV2(context.Background(), "u-compare", time.Now().UTC()); err != nil {
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
	for _, want := range []string{"V1 cluster-first", "V2 thesis-first", "风险资产承压", "未来数月波动加大"} {
		if !strings.Contains(out, want) {
			t.Fatalf("stdout missing %q in %q", want, out)
		}
	}
}

func TestRunMemoryGlobalCompareRunFlagBuildsFreshOutputs(t *testing.T) {
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
			UnitID: "weibo:CMP2", Source: "weibo", ExternalID: "CMP2", RootExternalID: "CMP2", Model: c.Qwen36PlusModel,
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
	for _, want := range []string{"V1 cluster-first", "V2 thesis-first", "风险资产承压"} {
		if !strings.Contains(out, want) {
			t.Fatalf("stdout missing %q in %q", want, out)
		}
	}
}

func TestRunMemoryGlobalCompareRespectsLimit(t *testing.T) {
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
		records := []c.Record{
			{UnitID: "weibo:CL1", Source: "weibo", ExternalID: "CL1", RootExternalID: "CL1", Model: c.Qwen36PlusModel, Output: c.Output{Summary: "s", Graph: c.ReasoningGraph{Nodes: []c.GraphNode{{ID: "n1", Kind: c.NodeFact, Text: "流动性收紧", OccurredAt: time.Date(2026, 4, 10, 0, 0, 0, 0, time.UTC)}, {ID: "n2", Kind: c.NodeConclusion, Text: "风险资产承压"}, {ID: "n3", Kind: c.NodePrediction, Text: "未来数月波动加大", PredictionStartAt: time.Date(2026, 4, 10, 0, 0, 0, 0, time.UTC)}}, Edges: []c.GraphEdge{{From: "n1", To: "n2", Kind: c.EdgeDerives}, {From: "n2", To: "n3", Kind: c.EdgeDerives}}}, Details: c.HiddenDetails{Caveats: []string{"说明"}}}, CompiledAt: time.Now().UTC()},
			{UnitID: "weibo:CL2", Source: "weibo", ExternalID: "CL2", RootExternalID: "CL2", Model: c.Qwen36PlusModel, Output: c.Output{Summary: "s", Graph: c.ReasoningGraph{Nodes: []c.GraphNode{{ID: "n1", Kind: c.NodeFact, Text: "供给趋紧", OccurredAt: time.Date(2026, 4, 10, 0, 0, 0, 0, time.UTC)}, {ID: "n2", Kind: c.NodeConclusion, Text: "油价会上升"}, {ID: "n3", Kind: c.NodePrediction, Text: "油价冲击扩大", PredictionStartAt: time.Date(2026, 4, 10, 0, 0, 0, 0, time.UTC)}}, Edges: []c.GraphEdge{{From: "n1", To: "n2", Kind: c.EdgeDerives}, {From: "n2", To: "n3", Kind: c.EdgeDerives}}}, Details: c.HiddenDetails{Caveats: []string{"说明"}}}, CompiledAt: time.Now().UTC()},
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
		if _, err := store.RunGlobalMemoryOrganizationV2(context.Background(), "u-compare-limit", time.Now().UTC()); err != nil {
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
		t.Fatalf("stdout = %q, want one v1 item and one v2 item", stdout.String())
	}
}

func TestRunMemoryGlobalCompareFiltersV2ItemType(t *testing.T) {
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
		records := []c.Record{
			{UnitID: "weibo:CFV1", Source: "weibo", ExternalID: "CFV1", RootExternalID: "CFV1", Model: c.Qwen36PlusModel, Output: c.Output{Summary: "s", Graph: c.ReasoningGraph{Nodes: []c.GraphNode{{ID: "n1", Kind: c.NodeFact, Text: "供给趋紧", OccurredAt: time.Date(2026, 4, 10, 0, 0, 0, 0, time.UTC)}, {ID: "n2", Kind: c.NodeConclusion, Text: "油价会上升"}}, Edges: []c.GraphEdge{{From: "n1", To: "n2", Kind: c.EdgeDerives}}}, Details: c.HiddenDetails{Caveats: []string{"说明"}}}, CompiledAt: time.Now().UTC()},
			{UnitID: "twitter:CFV2", Source: "twitter", ExternalID: "CFV2", RootExternalID: "CFV2", Model: c.Qwen36PlusModel, Output: c.Output{Summary: "s", Graph: c.ReasoningGraph{Nodes: []c.GraphNode{{ID: "n1", Kind: c.NodeFact, Text: "需求走弱", OccurredAt: time.Date(2026, 4, 10, 0, 0, 0, 0, time.UTC)}, {ID: "n2", Kind: c.NodeConclusion, Text: "油价会下降"}}, Edges: []c.GraphEdge{{From: "n1", To: "n2", Kind: c.EdgeDerives}}}, Details: c.HiddenDetails{Caveats: []string{"说明"}}}, CompiledAt: time.Now().UTC()},
			{UnitID: "weibo:CFV3", Source: "weibo", ExternalID: "CFV3", RootExternalID: "CFV3", Model: c.Qwen36PlusModel, Output: c.Output{Summary: "s", Graph: c.ReasoningGraph{Nodes: []c.GraphNode{{ID: "n1", Kind: c.NodeFact, Text: "流动性收紧", OccurredAt: time.Date(2026, 4, 10, 0, 0, 0, 0, time.UTC)}, {ID: "n2", Kind: c.NodeConclusion, Text: "风险资产承压"}, {ID: "n3", Kind: c.NodePrediction, Text: "未来数月波动加大", PredictionStartAt: time.Date(2026, 4, 10, 0, 0, 0, 0, time.UTC)}}, Edges: []c.GraphEdge{{From: "n1", To: "n2", Kind: c.EdgeDerives}, {From: "n2", To: "n3", Kind: c.EdgeDerives}}}, Details: c.HiddenDetails{Caveats: []string{"说明"}}}, CompiledAt: time.Now().UTC()},
		}
		for _, record := range records {
			if err := store.UpsertCompiledOutput(context.Background(), record); err != nil {
				return nil, err
			}
		}
		for _, req := range []memory.AcceptRequest{
			{UserID: "u-compare-filter", SourcePlatform: "weibo", SourceExternalID: "CFV1", NodeIDs: []string{"n2"}},
			{UserID: "u-compare-filter", SourcePlatform: "twitter", SourceExternalID: "CFV2", NodeIDs: []string{"n2"}},
			{UserID: "u-compare-filter", SourcePlatform: "weibo", SourceExternalID: "CFV3", NodeIDs: []string{"n1", "n2", "n3"}},
		} {
			if _, err := store.AcceptMemoryNodes(context.Background(), req); err != nil {
				return nil, err
			}
		}
		if _, err := store.RunGlobalMemoryOrganization(context.Background(), "u-compare-filter", time.Now().UTC()); err != nil {
			return nil, err
		}
		if _, err := store.RunGlobalMemoryOrganizationV2(context.Background(), "u-compare-filter", time.Now().UTC()); err != nil {
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
		t.Fatalf("stdout = %q, want only v2 conflict items while keeping v1 section", stdout.String())
	}
	if !strings.Contains(stdout.String(), "V2 thesis-first (1, filter=conflict)") {
		t.Fatalf("stdout = %q, want filter annotation in V2 header", stdout.String())
	}
}

func TestRunMemoryGlobalCompareReportsWhenFilteredV2SideIsEmpty(t *testing.T) {
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
			UnitID: "weibo:EC1", Source: "weibo", ExternalID: "EC1", RootExternalID: "EC1", Model: c.Qwen36PlusModel,
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
		if _, err := store.RunGlobalMemoryOrganizationV2(context.Background(), "u-compare-empty-filter", time.Now().UTC()); err != nil {
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
	if !strings.Contains(stdout.String(), "V2 thesis-first (0, filter=conflict)") {
		t.Fatalf("stdout = %q, want filtered count annotation even when empty", stdout.String())
	}
}

func TestRunMemoryGlobalCompareShowsSectionCounts(t *testing.T) {
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
			UnitID: "weibo:CC1", Source: "weibo", ExternalID: "CC1", RootExternalID: "CC1", Model: c.Qwen36PlusModel,
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
		if _, err := store.RunGlobalMemoryOrganizationV2(context.Background(), "u-compare-count", time.Now().UTC()); err != nil {
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
	for _, want := range []string{"V1 cluster-first (", "V2 thesis-first ("} {
		if !strings.Contains(out, want) {
			t.Fatalf("stdout missing %q in %q", want, out)
		}
	}
}

func TestRunMemoryGlobalCompareSuggestsRunWhenNoStoredOutputs(t *testing.T) {
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

type fakeCompileClient struct {
	record    c.Record
	err       error
	compileFn func(context.Context, c.Bundle) (c.Record, error)
}

func (f fakeCompileClient) Compile(ctx context.Context, bundle c.Bundle) (c.Record, error) {
	if f.compileFn != nil {
		return f.compileFn(ctx, bundle)
	}
	return f.record, f.err
}

func testGraphNode(id string, kind c.NodeKind, text string) c.GraphNode {
	return c.GraphNode{
		ID:        id,
		Kind:      kind,
		Text:      text,
		ValidFrom: time.Date(2026, 4, 14, 0, 0, 0, 0, time.UTC),
		ValidTo:   time.Date(2026, 7, 14, 0, 0, 0, 0, time.UTC),
	}
}

func stringValue(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
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

	store = openStore(t)
	persistedCompiled, err := store.GetCompiledOutput(context.Background(), rawCapture.Source, rawCapture.ExternalID)
	store.Close()
	if err != nil {
		t.Fatalf("GetCompiledOutput() error = %v", err)
	}
	if persistedCompiled.Output.Summary != compiled.Output.Summary {
		t.Fatalf("persisted compiled summary = %q, want %q", persistedCompiled.Output.Summary, compiled.Output.Summary)
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
					Nodes: []c.GraphNode{testGraphNode("n1", c.NodeFact, "事实A"), testGraphNode("n2", c.NodeConclusion, "结论B")},
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
					Nodes: []c.GraphNode{testGraphNode("n1", c.NodeFact, "事实A"), testGraphNode("n2", c.NodeConclusion, "结论B")},
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
					Nodes: []c.GraphNode{testGraphNode("n1", c.NodeFact, "事实A"), testGraphNode("n2", c.NodeConclusion, "结论B")},
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
					Nodes: []c.GraphNode{testGraphNode("n1", c.NodeFact, "事实A"), testGraphNode("n2", c.NodeConclusion, "结论B")},
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
					Nodes: []c.GraphNode{testGraphNode("n1", c.NodeFact, "事实A"), testGraphNode("n2", c.NodeConclusion, "结论B")},
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
					Nodes: []c.GraphNode{testGraphNode("n1", c.NodeFact, "事实A"), testGraphNode("n2", c.NodeConclusion, "结论B"), testGraphNode("n3", c.NodePrediction, "预测C")},
					Edges: []c.GraphEdge{{From: "n1", To: "n2", Kind: c.EdgeDerives}},
				},
				Details: c.HiddenDetails{Caveats: []string{"说明"}},
				Verification: c.Verification{
					PredictionChecks: []c.PredictionCheck{{NodeID: "n3", Status: c.PredictionStatusUnresolved, Reason: "pending", AsOf: time.Now().UTC()}},
				},
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
	for _, want := range []string{"Summary", "一句话总结", "Facts", "- 事实A", "Conclusions", "- 结论B", "Predictions", "- [预|unresolved] 预测C", "Main logic", "事实A --推出--> 结论B", "Confidence", "high"} {
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
					Nodes: []c.GraphNode{testGraphNode("n1", c.NodeFact, "事实A"), testGraphNode("n2", c.NodeConclusion, "结论B"), testGraphNode("n3", c.NodePrediction, "预测C")},
					Edges: []c.GraphEdge{{From: "n1", To: "n2", Kind: c.EdgeDerives}},
				},
				Details: c.HiddenDetails{Caveats: []string{"说明"}},
				Verification: c.Verification{
					PredictionChecks: []c.PredictionCheck{{NodeID: "n3", Status: c.PredictionStatusUnresolved, Reason: "pending", AsOf: time.Now().UTC()}},
				},
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
	for _, want := range []string{"Summary", "一句话总结", "Facts", "- 事实A", "Conclusions", "- 结论B", "Predictions", "- [预|unresolved] 预测C", "Main logic", "事实A --推出--> 结论B", "Confidence", "high"} {
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
						testGraphNode("n1", c.NodeFact, "事实A"),
						testGraphNode("n2", c.NodeFact, "事实B"),
						testGraphNode("n3", c.NodeConclusion, "结论C"),
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
