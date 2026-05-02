package main

import (
	"bytes"
	"context"
	c "github.com/kumaloha/VariX/varix/compile"
	"github.com/kumaloha/VariX/varix/ingest"
	varixllm "github.com/kumaloha/VariX/varix/llm"
	"github.com/kumaloha/VariX/varix/memory"
	"github.com/kumaloha/VariX/varix/storage/contentstore"
	"strings"
	"testing"
	"time"
)

func TestRunHarnessGraphFirstFlowCommandsWorkTogether(t *testing.T) {
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
		now := time.Now().UTC()
		record := c.Record{UnitID: "twitter:FLOW1", Source: "twitter", ExternalID: "FLOW1", RootExternalID: "FLOW1", Model: varixllm.Qwen36PlusModel, Output: c.Output{Summary: "一句话", Drivers: []string{"美联储加息0.25%"}, Targets: []string{"未来一周美股承压"}, Graph: c.ReasoningGraph{Nodes: []c.GraphNode{{ID: "n1", Kind: c.NodeFact, Text: "美联储加息0.25%", OccurredAt: now}, {ID: "n2", Kind: c.NodePrediction, Text: "未来一周美股承压", PredictionStartAt: now, PredictionDueAt: now.Add(24 * time.Hour)}}, Edges: []c.GraphEdge{{From: "n1", To: "n2", Kind: c.EdgePositive}}}, Details: c.HiddenDetails{Caveats: []string{"说明"}}}, CompiledAt: now}
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
