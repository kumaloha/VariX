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
		"relative return expectations keep capital allocated to defensive utilities -> defensive equity inflows persist",
		"side commentary outside projection: the article also discusses unrelated currency volatility",
		"defensive equity inflows persist",
		"do not choose: unrelated currency volatility as the main target when it is only side commentary",
		"do not choose: unrelated macro commentary as a top-level driver when it only supports a side forecast",
		"Causal projection:",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("BuildPrompt() missing %q in %q", want, prompt)
		}
	}
}

func TestBuildNodeInstructionAndPrompt(t *testing.T) {
	instruction := BuildNodeInstruction(GraphRequirements{MinNodes: 4, MinEdges: 3})
	for _, want := range []string{
		"You are a financial-analysis node extractor",
		"Produce at least 4 nodes.",
		"form values:",
		"function values:",
		"Use `observation` + `support` for evidence/fact nodes.",
		"Use `observation` + `transmission` for mechanism nodes instead of inventing a separate mechanism form.",
		"Use `judgment` + `claim` for current conclusions and `forecast` + `claim` for future outcomes.",
		"Prefer over-splitting to under-splitting when one sentence mixes evidence, mechanism, judgment, or prediction.",
		"transmission: a market transmission relation or pricing/allocation mechanism that explains how one force moves into another outcome in the current article frame",
		"Make sure the extracted node set includes separate evidence nodes, mechanism/transmission nodes, and judgment nodes whenever the article contains those roles.",
		"Prefer separate nodes for (a) observed evidence, (b) market mechanism/transmission, and (c) author judgment/slogan when the article contains all three roles.",
		"Do not collapse evidence, mechanism, and judgment into one sentence if they play different roles in the article.",
		"If a sentence contains explicit connectors such as because, therefore, so, which means, implying, driven by, due to, despite, or as a result, default to splitting the linked ideas into separate nodes.",
		"If a sentence contains coordinated claims such as A and B / not X and not Y / both X and Y, default to separate nodes unless they are truly the same proposition restated.",
		"If a statement says markets prefer or avoid an asset because one force dominates another, capture that preference relation as its own transmission node.",
		"If the article says one pricing force, return expectation, or risk regime dominates another and therefore keeps capital allocated to an asset or market, extract that dominance-to-allocation relation as a standalone transmission node.",
		"If observed flows or positioning are presented as the consequence of an allocation preference or pricing rule, extract both the transmission node and the observed evidence node.",
		"Prefer a transmission node such as \"capital stays allocated to X because Y dominates Z in market pricing\" over a higher-level slogan node when both express the same idea.",
		"For flow/positioning articles, prefer a support -> transmission -> claim decomposition: observed evidence, the allocation/transmission mechanism, then the judgment or forecast claim.",
		"If evidence nodes support a judgment/claim but the market rule, pricing rule, or allocation rule connecting them is missing, add a transmission node for that bridge.",
		"When an observed market outcome is paired with a judgment about that outcome but the intermediate pricing/allocation logic is only implicit, extract the missing transmission node instead of attaching evidence directly to claim.",
		"A statement that one force dominates another in pricing, positioning, or allocation should usually be a transmission node, not collapsed into a judgment node.",
		"When capital-flow evidence is used to support a market judgment, separate the observed flow node from the transmission node that explains why capital remains allocated that way.",
	} {
		if !strings.Contains(instruction, want) {
			t.Fatalf("node instruction missing %q in %q", want, instruction)
		}
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
		"Use `drives` for market transmission.",
		"Use `substantiates` / `gates` / `explains` for non-causal structure.",
		"Use `drives` for real-world transmission: A changes the state of the world and thereby moves B.",
		"Use `substantiates` for evidential support: A helps justify B, diagnose B, or explain why B should be believed, but does not itself make B happen.",
		"Use `gates` for prerequisite structure only: a condition node points to a downstream node that depends on that condition.",
		"Use `explains` for interpretive framing: A tells you how to understand B or what theory/frame B belongs to, without serving as direct proof or direct causal force.",
		"Treat node `function=transmission` as a default hint for `drives` edges and `function=support` as a default hint for `substantiates` edges, unless the article clearly implies a different semantic relation.",
		"Treat `function=claim` nodes as downstream judgments / forecasts that are usually supported or transmitted into, not upstream evidence by themselves.",
		"Apply the intervention test: if changing A would change B in the world, use `drives`; if A only changes how strongly the author can justify B, use `substantiates`.",
		"Apply the evidence test: if A causes B, use `drives`; if A proves, supports, or diagnoses B, use `substantiates`.",
		"If A mainly reframes B, names the governing theory behind B, or tells the reader how to interpret B without directly proving or causing it, use `explains`.",
		"Treat slogan-like or narrative-judgment nodes as `substantiates` targets unless they themselves continue the market mechanism into another downstream state.",
		"Treat `gates` as a condition-to-downstream edge only; do not aim `gates` into a condition node.",
		"In flow/positioning articles, prefer support -> claim as `substantiates` and transmission -> claim as `drives` when the transmission node describes the world-state bridge.",
		"Every edge must use exactly these keys: `from`, `to`, `kind`.",
		"Do not use alternate edge keys such as `source`, `target`, or `relation`.",
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

func TestBuildChallengePromptBuilders(t *testing.T) {
	nodes := []GraphNode{{ID: "n1", Kind: NodeFact, Text: "事实A"}}
	edges := []GraphEdge{{From: "n1", To: "n1", Kind: EdgeDerives}}
	nodeInstruction := BuildNodeChallengeInstruction(GraphRequirements{MinNodes: 2, MinEdges: 1})
	for _, want := range []string{
		"node challenger reviewing an extracted node set for recall gaps",
		"Audit observation / condition / judgment / forecast coverage and support / transmission / claim roles separately before deciding nothing is missing.",
		"Audit evidence nodes, mechanism/transmission nodes, and judgment nodes separately before deciding nothing is missing.",
		"Specifically look for the missing bridge transmission node between evidence nodes and judgment nodes.",
		"For flow/positioning articles, add the missing transmission node whenever support observations and judgment/forecast claims exist without an explicit bridge.",
		"When a market judgment depends on an allocation preference, pricing dominance, or investor positioning rule, add that missing transmission node explicitly instead of only adding more evidence or more judgment nodes.",
		"If evidence nodes support a judgment/claim but the market rule, pricing rule, or allocation rule connecting them is missing, add the missing transmission node.",
		"If an observed market outcome is paired with a claim but the intermediate pricing/allocation logic is only implicit, add the missing transmission bridge rather than accepting direct evidence -> claim structure as complete.",
		"If a statement about pricing dominance, allocation preference, or positioning rule is currently represented as a judgment, add a transmission node that captures that bridge explicitly.",
		"When capital-flow evidence is used to support a market judgment, add the transmission node that explains why capital remains allocated that way if it is missing.",
		"Look for nodes that are still too fat: if one existing node contains evidence plus judgment, mechanism plus outcome, prediction plus driver, or multiple coordinated claims, add the missing finer-grained nodes instead of accepting the coarse node as sufficient.",
		"Treat connector words such as because, therefore, so, which means, implying, driven by, due to, despite, and as a result as split signals.",
	} {
		if !strings.Contains(nodeInstruction, want) {
			t.Fatalf("node challenge instruction = %q", nodeInstruction)
		}
	}
	nodePrompt := BuildNodeChallengePrompt(Bundle{UnitID: "web:test", Source: "web", ExternalID: "test", Content: "body"}, nodes)
	if !strings.Contains(nodePrompt, "Current extracted nodes:") || !strings.Contains(nodePrompt, `"id": "n1"`) {
		t.Fatalf("node challenge prompt = %q", nodePrompt)
	}
	edgeInstruction := BuildEdgeChallengeInstruction(GraphRequirements{MinNodes: 2, MinEdges: 1})
	if !strings.Contains(edgeInstruction, "edge challenger reviewing a draft full graph for edge accuracy") {
		t.Fatalf("edge challenge instruction = %q", edgeInstruction)
	}
	edgePrompt := BuildEdgeChallengePrompt(Bundle{UnitID: "web:test", Source: "web", ExternalID: "test", Content: "body"}, nodes, edges)
	if !strings.Contains(edgePrompt, "Current draft edges:") || !strings.Contains(edgePrompt, `"kind": "substantiates"`) {
		t.Fatalf("edge challenge prompt = %q", edgePrompt)
	}
}
