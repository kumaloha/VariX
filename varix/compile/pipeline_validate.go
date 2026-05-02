package compile

import (
	"context"
	"fmt"
	"strings"
)

func stage4Validate(ctx context.Context, rt runtimeChat, model string, bundle Bundle, state graphState, maxRounds int) (graphState, error) {
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
	for round := 0; round < maxRounds; round++ {
		totalPatches := 0
		for _, para := range paragraphs {
			var patch struct {
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

func applyValidatePatch(state graphState, patch struct {
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
}) graphState {
	nextNode := len(state.Nodes) + 1
	textToID := map[string]string{}
	for _, n := range state.Nodes {
		textToID[normalizeText(n.Text)] = n.ID
	}
	for _, item := range patch.MissingNodes {
		text := strings.TrimSpace(item.Text)
		if text == "" {
			continue
		}
		key := normalizeText(text)
		if _, ok := textToID[key]; ok {
			continue
		}
		id := fmt.Sprintf("n%d", nextNode)
		nextNode++
		state.Nodes = append(state.Nodes, graphNode{ID: id, Text: text, SourceQuote: strings.TrimSpace(item.SourceQuote)})
		textToID[key] = id
	}
	for _, item := range patch.MissingEdges {
		fromID := textToID[normalizeText(item.FromText)]
		toID := textToID[normalizeText(item.ToText)]
		if fromID == "" || toID == "" || fromID == toID {
			continue
		}
		if !hasEdge(state.Edges, fromID, toID) {
			state.Edges = append(state.Edges, graphEdge{From: fromID, To: toID})
		}
	}
	for _, item := range patch.Misclassified {
		if strings.TrimSpace(item.NodeID) == "" {
			continue
		}
		state.OffGraph = append(state.OffGraph, offGraphItem{
			ID:         fmt.Sprintf("mis_%s", item.NodeID),
			Text:       strings.TrimSpace(item.Issue),
			Role:       "supplementary",
			AttachesTo: strings.TrimSpace(item.NodeID),
		})
	}
	return state
}
