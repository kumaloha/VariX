package contentstore

import (
	"context"
	"github.com/kumaloha/VariX/varix/model"
	"testing"
	"time"
)

func seedCompiledRecordForMemory(t *testing.T, store *SQLiteStore) {
	t.Helper()
	record := model.Record{
		UnitID:         "weibo:Q1",
		Source:         "weibo",
		ExternalID:     "Q1",
		RootExternalID: "Q1",
		Model:          "qwen3.6-plus",
		Output: model.Output{
			Summary: "summary",
			Graph: model.ReasoningGraph{
				Nodes: []model.GraphNode{
					{ID: "n1", Kind: model.NodeFact, Text: "事实A", ValidFrom: time.Date(2026, 4, 14, 0, 0, 0, 0, time.UTC), ValidTo: time.Date(2026, 7, 14, 0, 0, 0, 0, time.UTC)},
					{ID: "n2", Kind: model.NodeConclusion, Text: "结论B", ValidFrom: time.Date(2026, 4, 14, 0, 0, 0, 0, time.UTC), ValidTo: time.Date(2026, 7, 14, 0, 0, 0, 0, time.UTC)},
				},
				Edges: []model.GraphEdge{{From: "n1", To: "n2", Kind: model.EdgeDerives}},
			},
			Details:    model.HiddenDetails{Caveats: []string{"detail"}},
			Confidence: "medium",
		},
		CompiledAt: time.Date(2026, 4, 14, 8, 0, 0, 0, time.UTC),
	}
	if err := store.UpsertCompiledOutput(context.Background(), record); err != nil {
		t.Fatalf("UpsertCompiledOutput() error = %v", err)
	}
}
