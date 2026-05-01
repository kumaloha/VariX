package compilev2

import (
	"context"
	"github.com/kumaloha/VariX/varix/compile"
	"strings"
)

func stage1Extract(ctx context.Context, rt runtimeChat, model string, bundle compile.Bundle) (graphState, error) {
	systemPrompt, err := renderStage1SystemPrompt()
	if err != nil {
		return graphState{}, err
	}
	userPrompt, err := renderStage1UserPrompt(bundle.TextContext())
	if err != nil {
		return graphState{}, err
	}
	var payload map[string]any
	if err := stageJSONCall(ctx, rt, model, bundle, systemPrompt, userPrompt, "extract", &payload); err != nil {
		return graphState{}, err
	}
	state := graphState{}
	state.ArticleForm = normalizeArticleForm(asString(payload["article_form"]))
	state.Nodes = decodeStage1Nodes(payload["nodes"])
	state.Edges = nil
	state.OffGraph = decodeStage1OffGraph(payload["off_graph"])
	state = fillMissingStage1IDs(state)
	state.ArticleForm = refineArticleFormFromExtract(bundle, state)
	return state, nil
}

func stage1Refine(ctx context.Context, rt runtimeChat, model string, bundle compile.Bundle, state graphState) (graphState, error) {
	state = normalizeStage1State(state)
	if len(state.Nodes) == 0 {
		return state, nil
	}
	candidates := refineCandidateNodes(state.Nodes)
	if len(candidates) == 0 {
		return state, nil
	}
	systemPrompt, err := renderRefineSystemPrompt()
	if err != nil {
		return graphState{}, err
	}
	userPrompt, err := renderRefineUserPrompt(bundle.TextContext(), serializeNodeList(candidates))
	if err != nil {
		return graphState{}, err
	}
	var result refineResult
	if err := stageJSONCall(ctx, rt, model, bundle, systemPrompt, userPrompt, "refine", &result); err != nil {
		return graphState{}, err
	}
	return applyRefineReplacements(state, result.Replacements), nil
}

func stage1Aggregate(ctx context.Context, rt runtimeChat, model string, bundle compile.Bundle, state graphState) (graphState, error) {
	state = normalizeStage1State(state)
	if len(state.Nodes) == 0 {
		return state, nil
	}
	candidates := serializeAggregateCandidateGroups(state.Nodes)
	if strings.TrimSpace(candidates) == "" {
		return state, nil
	}
	systemPrompt, err := renderAggregateSystemPrompt()
	if err != nil {
		return graphState{}, err
	}
	userPrompt, err := renderAggregateUserPrompt(bundle.TextContext(), candidates)
	if err != nil {
		return graphState{}, err
	}
	var result aggregateResult
	if err := stageJSONCall(ctx, rt, model, bundle, systemPrompt, userPrompt, "aggregate", &result); err != nil {
		return graphState{}, err
	}
	return applyAggregatePatches(state, result.Aggregates), nil
}
