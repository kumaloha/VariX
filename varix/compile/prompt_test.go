package compile

import (
	"strings"
	"testing"
)

func TestBuildInstructionHasSingleSentenceRoleFrame(t *testing.T) {
	instruction := BuildInstruction(GraphRequirements{MinNodes: 2, MinEdges: 1})
	if !strings.Contains(instruction, "You are a financial-analysis thesis mapper extracting the dominant driver-target thesis from a causal projection.") {
		t.Fatalf("BuildInstruction() missing role frame in %q", instruction)
	}
}

func TestBuildInstructionDefinesDriverAndTarget(t *testing.T) {
	instruction := BuildInstruction(GraphRequirements{MinNodes: 2, MinEdges: 1})
	for _, want := range []string{
		"driver: the concrete force/mechanism moving markets in the article's dominant causal spine",
		"target: the resulting market change caused by a driver; write the change, not just the asset noun",
	} {
		if !strings.Contains(instruction, want) {
			t.Fatalf("BuildInstruction() missing %q in %q", want, instruction)
		}
	}
}

func TestBuildInstructionIncludesNormalizationCriteriaAndBoundaries(t *testing.T) {
	instruction := BuildInstruction(GraphRequirements{MinNodes: 2, MinEdges: 1})
	for _, want := range []string{
		"Read the causal projection as the standardized market chain",
		"Do not switch to a secondary thesis if the article's main point is about another market relation.",
		"Do not use vague drivers such as \"many risks exist\" or \"the situation is complex\".",
		"Do not use bare targets such as \"stocks\", \"gold\", or \"housing\"",
		"Do not output graph nodes or graph edges in this stage.",
	} {
		if !strings.Contains(instruction, want) {
			t.Fatalf("BuildInstruction() missing %q in %q", want, instruction)
		}
	}
}

func TestBuildInstructionIncludesPrimaryThesisTieBreakers(t *testing.T) {
	instruction := BuildInstruction(GraphRequirements{MinNodes: 2, MinEdges: 1})
	for _, want := range []string{
		"Choose the main thesis by prioritizing the relation emphasized by the article's headline, opening setup, and closing conclusion.",
		"If the article contrasts a current flow/positioning claim with a side macro forecast, prefer the current flow/positioning claim as the main thesis.",
		"`summary`, `drivers`, and `targets` must all describe the same primary thesis.",
		"Do not promote side commentary into top-level `drivers` when it does not directly drive the chosen top-level target.",
	} {
		if !strings.Contains(instruction, want) {
			t.Fatalf("BuildInstruction() missing %q in %q", want, instruction)
		}
	}
}

func TestBuildInstructionIncludesNegatedTradeRule(t *testing.T) {
	instruction := BuildInstruction(GraphRequirements{MinNodes: 2, MinEdges: 1})
	for _, want := range []string{
		"If the article's core claim is that a popular trade/narrative is not happening, encode that rejected trade and the actually observed flow/positioning as the main target.",
		"Do not replace a 'no such trade / no exodus / continued inflow' thesis with a downstream macro forecast unless that forecast is clearly the article's main conclusion.",
	} {
		if !strings.Contains(instruction, want) {
			t.Fatalf("BuildInstruction() missing %q in %q", want, instruction)
		}
	}
}

func TestBuildInstructionKeepsSideForecastOutOfTopLevelTargets(t *testing.T) {
	instruction := BuildInstruction(GraphRequirements{MinNodes: 2, MinEdges: 1})
	for _, want := range []string{
		"Do not mix the main target with a separate currency/rates forecast in top-level `targets` when the article's thesis is about flows or positioning.",
		"Do not promote side commentary into top-level `drivers` when it does not directly drive the chosen top-level target.",
	} {
		if !strings.Contains(instruction, want) {
			t.Fatalf("BuildInstruction() missing %q in %q", want, instruction)
		}
	}
}

func TestBuildInstructionRequiresNonEmptyDetailsCaveats(t *testing.T) {
	instruction := BuildInstruction(GraphRequirements{MinNodes: 2, MinEdges: 1})
	for _, want := range []string{
		"Populate `details.caveats` with at least one concise string.",
		"Use `details.caveats` for ambiguity, evidence limits, or why side commentary stayed out of top-level thesis.",
	} {
		if !strings.Contains(instruction, want) {
			t.Fatalf("BuildInstruction() missing %q in %q", want, instruction)
		}
	}
}

func TestBuildInstructionIncludesOutputFormatContract(t *testing.T) {
	instruction := BuildInstruction(GraphRequirements{MinNodes: 2, MinEdges: 1})
	for _, want := range []string{
		"Output exactly one valid JSON object with keys `summary`, `drivers`, `targets`, `details`, `topics`, `confidence`.",
	} {
		if !strings.Contains(instruction, want) {
			t.Fatalf("BuildInstruction() missing %q in %q", want, instruction)
		}
	}
}

func TestBuildPromptIncludesNegatedTradeNormalizationExample(t *testing.T) {
	prompt := BuildPrompt(Bundle{
		UnitID:     "web:test",
		Source:     "web",
		ExternalID: "test",
		Content:    "body",
	})
	for _, want := range []string{
		"growth / return expectations still dominate political-risk pricing -> foreign capital keeps flowing into US assets; no sell/hedge America trade forms",
		"side commentary outside projection: the article also discusses rate cuts and dollar weakness",
		"foreign capital keeps flowing into US assets; no sell/hedge America trade forms",
		"do not choose: dollar weakness as the main target when it is only side commentary",
		"do not choose: Fed/rates/dollar commentary as a top-level driver when it only supports a side forecast",
		"Causal projection:",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("BuildPrompt() missing %q in %q", want, prompt)
		}
	}
}

func TestBuildNodeInstructionAndPrompt(t *testing.T) {
	instruction := BuildNodeInstruction(GraphRequirements{MinNodes: 4, MinEdges: 3})
	if !strings.Contains(instruction, "You are a financial-analysis node extractor") || !strings.Contains(instruction, "Produce at least 4 nodes.") {
		t.Fatalf("node instruction missing expected guidance: %q", instruction)
	}
	prompt := BuildNodePrompt(Bundle{UnitID: "web:test", Source: "web", ExternalID: "test", Content: "body"})
	if !strings.Contains(prompt, "Node extraction payload:") || !strings.Contains(prompt, "[ROOT CONTENT]") {
		t.Fatalf("node prompt missing payload context: %q", prompt)
	}
}

func TestBuildGraphInstructionAndPrompt(t *testing.T) {
	instruction := BuildGraphInstruction(GraphRequirements{MinNodes: 2, MinEdges: 3})
	for _, want := range []string{
		"You are a financial-analysis graph builder connecting extracted nodes into a full reasoning graph.",
		"Use `正向` for market transmission.",
		"Use `推出` / `预设` for argument structure or proof scaffolding.",
		"Apply the intervention test: if changing A would change B in the world, use `正向`; if A only changes how strongly the author can justify B, use `推出`.",
		"Apply the evidence test: if A causes B, use `正向`; if A proves, supports, or diagnoses B, use `推出`.",
		"Treat slogans or judgment nodes such as \"there is no sell America trade\" as `推出` targets unless they themselves continue the market mechanism into another downstream state.",
		"Treat `预设` as a condition-to-downstream edge only; do not aim `预设` into a condition node.",
	} {
		if !strings.Contains(instruction, want) {
			t.Fatalf("graph instruction missing %q in %q", want, instruction)
		}
	}
	prompt := BuildGraphPrompt(Bundle{UnitID: "web:test", Source: "web", ExternalID: "test", Content: "body"}, []GraphNode{{ID: "n1", Kind: NodeFact, Text: "事实A"}})
	if !strings.Contains(prompt, "Extracted nodes:") || !strings.Contains(prompt, `"id": "n1"`) {
		t.Fatalf("graph prompt missing nodes json: %q", prompt)
	}
}
