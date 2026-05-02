package compile

import (
	"context"
)

type validatePatch struct {
	MissingNodes []struct {
		Text              string `json:"text"`
		SourceQuote       string `json:"source_quote"`
		SuggestedRoleHint string `json:"suggested_role_hint"`
	} `json:"missing_nodes"`
	MissingEdges []struct {
		FromText string `json:"from_text"`
		ToText   string `json:"to_text"`
	} `json:"missing_edges"`
	Misclassified []struct {
		NodeID string `json:"node_id"`
		Issue  string `json:"issue"`
	} `json:"misclassified"`
}

func runValidatePreview(ctx context.Context, rt runtimeChat, model string, bundle Bundle, state graphState, maxRounds int, paragraphLimit int) (graphState, error) {
	if maxRounds <= 0 {
		return state, nil
	}
	systemPrompt, err := renderStage4SystemPrompt()
	if err != nil {
		return graphState{}, err
	}
	paragraphs := splitParagraphs(bundle.TextContext())
	if len(paragraphs) == 0 {
		return state, nil
	}
	if paragraphLimit > 0 && paragraphLimit < len(paragraphs) {
		paragraphs = paragraphs[:paragraphLimit]
	}
	for round := 0; round < maxRounds; round++ {
		totalPatches := 0
		for _, para := range paragraphs {
			var patch validatePatch
			userPrompt, err := renderStage4UserPrompt(para, serializeNodeList(state.Nodes), serializeEdgeList(state.Edges))
			if err != nil {
				return graphState{}, err
			}
			if err := stageJSONCall(ctx, rt, model, bundle, systemPrompt, userPrompt, "validate", &patch); err != nil {
				return graphState{}, err
			}
			totalPatches += len(patch.MissingNodes) + len(patch.MissingEdges) + len(patch.Misclassified)
			state = applyValidatePatch(state, patch)
		}
		var err error
		state, err = stage1Refine(ctx, rt, model, bundle, state)
		if err != nil {
			return graphState{}, err
		}
		state, err = stage1Aggregate(ctx, rt, model, bundle, state)
		if err != nil {
			return graphState{}, err
		}
		state, err = stage2Support(ctx, rt, model, bundle, state)
		if err != nil {
			return graphState{}, err
		}
		state = collapseClusters(state)
		state, err = stage3Mainline(ctx, rt, model, bundle, state)
		if err != nil {
			return graphState{}, err
		}
		state, err = stage3Classify(ctx, rt, model, bundle, state)
		if err != nil {
			return graphState{}, err
		}
		state.Rounds++
		if totalPatches < 2 {
			break
		}
	}
	return state, nil
}
