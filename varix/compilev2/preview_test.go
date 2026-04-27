package compilev2

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/kumaloha/VariX/varix/compile"
)

func TestBuildMainlineMarkdownSkipsDirectPathSelfLoop(t *testing.T) {
	body := BuildMainlineMarkdown(FlowPreviewResult{
		Platform:   "web",
		ExternalID: "direct-path",
		Render: compile.Output{
			Drivers: []string{"Driver A"},
			Targets: []string{"Target B"},
			TransmissionPaths: []compile.TransmissionPath{{
				Driver: "Driver A",
				Target: "Target B",
				Steps:  []string{"Driver A"},
			}},
		},
	})

	if strings.Contains(body, "n1 --> n1") {
		t.Fatalf("direct path rendered a self-loop:\n%s", body)
	}
	if !strings.Contains(body, "Driver A") || !strings.Contains(body, "Target B") {
		t.Fatalf("mainline markdown missing driver/target labels:\n%s", body)
	}
}

func TestBuildMainlineMarkdownPrefersRelationsGraph(t *testing.T) {
	body := BuildMainlineMarkdown(FlowPreviewResult{
		Platform:   "web",
		ExternalID: "mainline",
		Relations: PreviewGraph{
			Nodes: []PreviewNode{
				{ID: "n1", Text: "Claims expand"},
				{ID: "n2", Text: "Promises cannot be met"},
				{ID: "n3", Text: "Money is printed"},
			},
			Edges: []PreviewEdge{
				{From: "n1", To: "n2"},
				{From: "n2", To: "n3"},
			},
		},
		Render: compile.Output{
			Drivers: []string{"Unrelated display driver"},
			Targets: []string{"Unrelated display target"},
			TransmissionPaths: []compile.TransmissionPath{{
				Driver: "Unrelated display driver",
				Target: "Unrelated display target",
				Steps:  []string{"Unrelated display driver"},
			}},
		},
	})

	for _, want := range []string{"Claims expand", "Promises cannot be met", "Money is printed"} {
		if !strings.Contains(body, want) {
			t.Fatalf("mainline markdown missing relation node %q:\n%s", want, body)
		}
	}
	if strings.Contains(body, "Unrelated display driver") {
		t.Fatalf("mainline markdown used render projection instead of relations graph:\n%s", body)
	}
}

func TestDerivePreviewSpinesKeepsParallelBranchesSeparate(t *testing.T) {
	spines := derivePreviewSpines(PreviewGraph{
		Nodes: []PreviewNode{
			{ID: "g1", Text: "Geopolitical tension"},
			{ID: "g2", Text: "Global order uncertainty", IsTarget: true},
			{ID: "r1", Text: "Post-2008 regulation"},
			{ID: "r2", Text: "Productive lending pressure", IsTarget: true},
			{ID: "p1", Text: "Private credit opacity"},
			{ID: "p2", Text: "Markdown pressure", IsTarget: true},
		},
		Edges: []PreviewEdge{
			{From: "g1", To: "g2"},
			{From: "r1", To: "r2"},
			{From: "p1", To: "p2"},
		},
	})

	if len(spines) != 3 {
		t.Fatalf("len(spines) = %d, want 3 parallel spines: %#v", len(spines), spines)
	}
	for _, spine := range spines {
		if spine.Level != "branch" {
			t.Fatalf("spine level = %q, want branch for parallel two-node spine: %#v", spine.Level, spine)
		}
		if len(spine.NodeIDs) != 2 || len(spine.Edges) != 1 {
			t.Fatalf("spine shape = %#v, want two nodes and one edge", spine)
		}
	}
}

func TestBuildMainlineMarkdownUsesPreviewSpines(t *testing.T) {
	body := BuildMainlineMarkdown(FlowPreviewResult{
		Platform:   "web",
		ExternalID: "spines",
		Spines: []PreviewSpine{
			{
				ID:       "s1",
				Level:    "primary",
				Priority: 1,
				Thesis:   "Claims cycle",
				NodeIDs:  []string{"n1", "n2", "n3"},
				Edges: []PreviewEdge{
					{From: "n1", To: "n2"},
					{From: "n2", To: "n3"},
				},
			},
			{
				ID:       "s2",
				Level:    "branch",
				Priority: 2,
				Thesis:   "Regulation branch",
				NodeIDs:  []string{"n4", "n5"},
				Edges:    []PreviewEdge{{From: "n4", To: "n5"}},
			},
		},
		Relations: PreviewGraph{
			Nodes: []PreviewNode{
				{ID: "n1", Text: "Claims expand"},
				{ID: "n2", Text: "Promises cannot be met"},
				{ID: "n3", Text: "Money is printed"},
				{ID: "n4", Text: "Regulation rises"},
				{ID: "n5", Text: "Lending slows"},
			},
		},
	})

	for _, want := range []string{"Spine s1", "primary", "Claims cycle", "Spine s2", "Regulation branch"} {
		if !strings.Contains(body, want) {
			t.Fatalf("mainline markdown missing spine marker %q:\n%s", want, body)
		}
	}
}

func TestBuildMainlineMarkdownRendersSingleNodeSpines(t *testing.T) {
	body := BuildMainlineMarkdown(FlowPreviewResult{
		Platform:   "web",
		ExternalID: "single-node-spine",
		Spines: []PreviewSpine{
			{
				ID:       "s_geopolitical",
				Level:    "branch",
				Priority: 1,
				Thesis:   "Geopolitical tensions remain a major risk family",
				NodeIDs:  []string{"n1"},
			},
		},
		Relations: PreviewGraph{
			Nodes: []PreviewNode{
				{ID: "n1", Text: "Geopolitical tensions remain JPMorgan's primary risk"},
			},
		},
	})

	if !strings.Contains(body, "Spine s_geopolitical") {
		t.Fatalf("mainline markdown missing single-node spine listing:\n%s", body)
	}
	if !strings.Contains(body, "Geopolitical tensions remain JPMorgan's primary risk") {
		t.Fatalf("mainline markdown missing single-node spine node:\n%s", body)
	}
	if strings.Contains(body, "No mainline path") {
		t.Fatalf("mainline markdown rendered empty-path fallback despite single-node spine:\n%s", body)
	}
}

func TestFlowPreviewResultJSONUsesSpinesContract(t *testing.T) {
	payload, err := json.Marshal(FlowPreviewResult{
		Platform:   "web",
		ExternalID: "spines-contract",
		Spines: []PreviewSpine{{
			ID:       "s1",
			Level:    "primary",
			Priority: 1,
			Thesis:   "Article thesis",
			NodeIDs:  []string{"n1"},
		}},
	})
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	body := string(payload)
	if !strings.Contains(body, `"spines"`) {
		t.Fatalf("preview payload missing spines key: %s", body)
	}
	if strings.Contains(body, `"mainline_spines"`) {
		t.Fatalf("preview payload still exposes legacy mainline_spines key: %s", body)
	}
}
