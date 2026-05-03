package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	c "github.com/kumaloha/VariX/varix/compile"
	"github.com/kumaloha/VariX/varix/ingest"
	"github.com/kumaloha/VariX/varix/ingest/types"
	"github.com/kumaloha/VariX/varix/model"
	"github.com/kumaloha/VariX/varix/storage/contentstore"
)

type fakePreviewRenderer struct {
	calls  int
	result c.FlowPreviewResult
}

func (f *fakePreviewRenderer) RenderPreview(ctx context.Context, bundle model.Bundle, result c.FlowPreviewResult) (c.FlowPreviewResult, error) {
	f.calls++
	if bundle.Source != "youtube" || bundle.ExternalID != "video-1" {
		return c.FlowPreviewResult{}, fmt.Errorf("unexpected bundle %s:%s", bundle.Source, bundle.ExternalID)
	}
	if len(result.Classify.Nodes) != 1 {
		return c.FlowPreviewResult{}, fmt.Errorf("missing classify payload")
	}
	return f.result, nil
}

func seedCompileRerenderPreviewStore(t *testing.T, dbPath string) int64 {
	t.Helper()
	store, err := contentstore.NewSQLiteStore(dbPath)
	if err != nil {
		t.Fatalf("NewSQLiteStore() error = %v", err)
	}
	defer store.Close()
	if err := store.UpsertRawCapture(context.Background(), types.RawContent{
		Source:     "youtube",
		ExternalID: "video-1",
		Content:    "Greg Abel discusses capital allocation.",
		URL:        "https://www.youtube.com/watch?v=video-1",
	}); err != nil {
		t.Fatalf("UpsertRawCapture() error = %v", err)
	}
	runID, err := store.CreateCompilePreviewRun(context.Background(), contentstore.CompilePreviewRun{
		Pipeline:    "compile",
		SampleScope: "youtube:video-1",
		SampleCount: 1,
		WorkerCount: 1,
		Status:      "finished",
		StartedAt:   time.Now().UTC().Format(time.RFC3339),
		FinishedAt:  time.Now().UTC().Format(time.RFC3339),
	})
	if err != nil {
		t.Fatalf("CreateCompilePreviewRun() error = %v", err)
	}
	payload, err := json.Marshal(c.FlowPreviewResult{
		Platform:   "youtube",
		ExternalID: "video-1",
		Classify: c.PreviewGraph{Nodes: []c.PreviewNode{{
			ID:            "n1",
			Text:          "capital allocation rule",
			DiscourseRole: "capital_allocation_rule",
		}}},
		Spines: []c.PreviewSpine{{
			ID:      "s1",
			Policy:  "capital_allocation_rule",
			NodeIDs: []string{"n1"},
		}},
		Metrics: map[string]int64{"classify_ms": 10},
	})
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	if err := store.UpsertCompilePreviewRunItem(context.Background(), contentstore.CompilePreviewRunItem{
		RunID:       runID,
		Platform:    "youtube",
		ExternalID:  "video-1",
		URL:         "https://www.youtube.com/watch?v=video-1",
		Status:      "finished",
		PayloadJSON: string(payload),
		StartedAt:   time.Now().UTC().Format(time.RFC3339),
		FinishedAt:  time.Now().UTC().Format(time.RFC3339),
	}); err != nil {
		t.Fatalf("UpsertCompilePreviewRunItem() error = %v", err)
	}
	return runID
}

func TestRunCompileRerenderUsesPreviewPayloadAndWritesCompiledOutput(t *testing.T) {
	prevNewIngestRuntime := newIngestRuntime
	prevOpenSQLiteStore := openSQLiteStore
	prevBuildCompilePreviewRenderer := buildCompilePreviewRenderer
	t.Cleanup(func() {
		newIngestRuntime = prevNewIngestRuntime
		openSQLiteStore = prevOpenSQLiteStore
		buildCompilePreviewRenderer = prevBuildCompilePreviewRenderer
	})

	tmp := t.TempDir()
	dbPath := tmp + "/content.db"
	runID := seedCompileRerenderPreviewStore(t, dbPath)
	newIngestRuntime = func(projectRoot string) (*ingest.Runtime, error) {
		app := &ingest.Runtime{}
		app.Settings.ContentDBPath = dbPath
		return app, nil
	}
	openSQLiteStore = func(path string) (*contentstore.SQLiteStore, error) {
		return contentstore.NewSQLiteStore(path)
	}
	renderer := &fakePreviewRenderer{result: c.FlowPreviewResult{
		Platform:   "youtube",
		ExternalID: "video-1",
		Render: c.Output{
			Summary: "rerendered summary",
			Declarations: []c.Declaration{{
				ID:        "s1",
				Kind:      "capital_allocation_rule",
				Topic:     "capital_allocation",
				Statement: "rerendered declaration",
			}},
			Graph: c.ReasoningGraph{
				Nodes: []c.GraphNode{testGraphNode("n1", c.NodeFact, "rerendered declaration"), testGraphNode("n2", c.NodeConclusion, "rerendered summary")},
				Edges: []c.GraphEdge{{From: "n1", To: "n2", Kind: c.EdgeDerives}},
			},
			Details:    c.HiddenDetails{Caveats: []string{"rerender"}},
			Confidence: "high",
		},
		Metrics: map[string]int64{"classify_ms": 10, "render_ms": 2},
	}}
	buildCompilePreviewRenderer = func(projectRoot string) compilePreviewRenderer {
		return renderer
	}

	var stdout, stderr bytes.Buffer
	code := run([]string{"compile", "rerender", "--source-run-id", fmt.Sprint(runID), "--platform", "youtube", "--id", "video-1", "--write"}, "/tmp/project", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("run() code = %d, stderr = %s", code, stderr.String())
	}
	if renderer.calls != 1 {
		t.Fatalf("renderer calls = %d, want 1", renderer.calls)
	}
	var got c.Record
	if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
		t.Fatalf("json.Unmarshal(stdout) error = %v", err)
	}
	if got.Output.Summary != "rerendered summary" {
		t.Fatalf("Summary = %q", got.Output.Summary)
	}
	if got.Metrics.CompileStageElapsedMS["classify"] != 10 || got.Metrics.CompileStageElapsedMS["render"] != 2 {
		t.Fatalf("CompileStageElapsedMS = %#v", got.Metrics.CompileStageElapsedMS)
	}

	stdout.Reset()
	stderr.Reset()
	code = run([]string{"compile", "show", "--platform", "youtube", "--id", "video-1"}, "/tmp/project", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("compile show code = %d, stderr = %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "rerendered summary") {
		t.Fatalf("show stdout = %q, want persisted rerender", stdout.String())
	}
}
