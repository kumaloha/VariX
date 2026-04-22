package compilev2

import (
	"context"
	"testing"

	"github.com/kumaloha/VariX/varix/compile"
	"github.com/kumaloha/forge/llm"
)

type fakeRuntime struct {
	responses []llm.Response
	calls     int
}

func (f *fakeRuntime) Call(ctx context.Context, req llm.ProviderRequest) (llm.Response, error) {
	if f.calls >= len(f.responses) {
		return llm.Response{}, nil
	}
	resp := f.responses[f.calls]
	f.calls++
	return resp, nil
}

func TestClientCompileRecordsStageMetrics(t *testing.T) {
	rt := &fakeRuntime{responses: []llm.Response{
		{Text: `{"nodes":[{"id":"n1","text":"Driver A"},{"id":"n2","text":"Target B"}],"edges":[{"from":"n1","to":"n2"}],"off_graph":[]}`, Model: "compilev2-model"},
		{Text: `{"equivalent":false}`, Model: "compilev2-model"},
		{Text: `{"is_market_outcome":true,"category":"equity"}`, Model: "compilev2-model"},
		{Text: `{"causal_edges":[{"from":"n1","to":"n2"}],"support_links":[],"supplement_links":[],"explanation_links":[]}`, Model: "compilev2-model"},
		{Text: `{"is_market_outcome":true,"category":"equity"}`, Model: "compilev2-model"},
		{Text: `{"missing_nodes":[],"missing_edges":[],"misclassified":[]}`, Model: "compilev2-model"},
		{Text: `{"equivalent":false}`, Model: "compilev2-model"},
		{Text: `{"is_market_outcome":true,"category":"equity"}`, Model: "compilev2-model"},
		{Text: `{"causal_edges":[{"from":"n1","to":"n2"}],"support_links":[],"supplement_links":[],"explanation_links":[]}`, Model: "compilev2-model"},
		{Text: `{"is_market_outcome":true,"category":"equity"}`, Model: "compilev2-model"},
		{Text: `{"translations":[{"id":"n1","text":"驱动A"},{"id":"n2","text":"目标B"}]}`, Model: "compilev2-model"},
		{Text: `{"summary":"驱动A推动目标B"}`, Model: "compilev2-model"},
	}}
	client := &Client{runtime: rt, model: "compilev2-model", projectRoot: ""}
	record, err := client.Compile(context.Background(), compile.Bundle{
		UnitID:     "web:v2-metrics",
		Source:     "web",
		ExternalID: "v2-metrics",
		Content:    "driver paragraph\n\ntarget paragraph",
	})
	if err != nil {
		t.Fatalf("Compile() error = %v", err)
	}
	if record.Metrics.CompileElapsedMS <= 0 {
		t.Fatalf("CompileElapsedMS = %d, want positive total metric", record.Metrics.CompileElapsedMS)
	}
	for _, stage := range []string{"stage1_extract", "stage2_dedup", "stage3_classify", "stage3_relations", "stage3_reclassify", "stage4_validate", "stage5_render"} {
		if record.Metrics.CompileStageElapsedMS[stage] <= 0 {
			t.Fatalf("CompileStageElapsedMS = %#v, want positive duration for %q", record.Metrics.CompileStageElapsedMS, stage)
		}
	}
}
