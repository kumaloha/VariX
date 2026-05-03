package compile

import (
	"context"
	"testing"

	"github.com/kumaloha/forge/llm"
)

func TestRenderPreviewRerunsOnlyRenderFromClassifyPayload(t *testing.T) {
	rt := &fakeRuntime{responses: []llm.Response{
		{Text: `{"translations":[{"id":"n1","text":"资本配置纪律"},{"id":"n2","text":"等待高价值机会"}]}`},
		{Text: `{"summary":"资本配置纪律要求等待高价值机会。"}`},
	}}
	client := &Client{runtime: rt, model: "compile-model"}
	result, err := client.RenderPreview(context.Background(), Bundle{
		UnitID:     "youtube:rerender",
		Source:     "youtube",
		ExternalID: "rerender",
		Content:    "capital allocation discipline means waiting for strong value propositions.",
	}, FlowPreviewResult{
		ArticleForm: "management_qa",
		Spines: []PreviewSpine{{
			ID:      "s1",
			Policy:  "capital_allocation_rule",
			Thesis:  "capital allocation discipline",
			NodeIDs: []string{"n1", "n2"},
		}},
		Classify: PreviewGraph{
			Nodes: []PreviewNode{
				{ID: "n1", Text: "capital allocation discipline", DiscourseRole: "capital_allocation_rule"},
				{ID: "n2", Text: "waiting for strong value propositions", DiscourseRole: "condition"},
			},
		},
		Metrics: map[string]int64{"classify_ms": 10},
	})
	if err != nil {
		t.Fatalf("RenderPreview() error = %v", err)
	}
	if rt.calls != 2 {
		t.Fatalf("runtime calls = %d, want translate+summary only", rt.calls)
	}
	if result.Metrics["classify_ms"] != 10 {
		t.Fatalf("Metrics = %#v, want existing metrics preserved", result.Metrics)
	}
	if result.Metrics["render_ms"] <= 0 {
		t.Fatalf("Metrics = %#v, want render_ms", result.Metrics)
	}
	if len(result.Render.Declarations) != 1 {
		t.Fatalf("Render.Declarations = %#v, want declaration from classify payload", result.Render.Declarations)
	}
}
