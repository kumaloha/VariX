package compile

import (
	"context"
	"fmt"
	"strconv"
	"strings"
)

func stageCoverage(ctx context.Context, rt runtimeChat, model string, bundle Bundle, state graphState, maxRounds int) (graphState, error) {
	if maxRounds <= 0 {
		return state, nil
	}
	systemPrompt, err := renderCoverageSystemPrompt()
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
			var patch coveragePatch
			userPrompt, err := renderCoverageUserPrompt(para, serializeNodeList(state.Nodes), serializeEdgeList(state.Edges))
			if err != nil {
				return graphState{}, err
			}
			if err := stageJSONCall(ctx, rt, model, bundle, systemPrompt, userPrompt, "coverage", &patch); err != nil {
				return graphState{}, err
			}
			totalPatches += len(patch.MissingNodes) + len(patch.MissingEdges) + len(patch.Misclassified)
			state = applyCoveragePatch(state, patch)
		}
		if totalPatches == 0 {
			break
		}
		state.Rounds++
	}
	return state, nil
}

func applyCoveragePatch(state graphState, patch coveragePatch) graphState {
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
		id := nextAvailableCoverageNodeID(state.Nodes)
		state.Nodes = append(state.Nodes, graphNode{
			ID:            id,
			Text:          text,
			SourceQuote:   strings.TrimSpace(item.SourceQuote),
			DiscourseRole: coverageDiscourseRole(item.SuggestedRoleHint),
		})
		if len(state.BranchHeads) > 0 && coverageHintAddsBranchHead(item.SuggestedRoleHint) {
			state.BranchHeads = appendUniqueString(state.BranchHeads, id)
		}
		textToID[key] = id
	}
	for _, item := range patch.MissingEdges {
		fromID := textToID[normalizeText(item.FromText)]
		toID := textToID[normalizeText(item.ToText)]
		if fromID == "" || toID == "" || fromID == toID {
			continue
		}
		if !hasCoverageHint(state.CoverageHints, fromID, toID) {
			state.CoverageHints = append(state.CoverageHints, coverageHint{
				From:        fromID,
				To:          toID,
				SourceQuote: coverageHintSourceQuote(state.Nodes, fromID, toID),
				Reason:      "coverage auditor suggested this article-grounded relation",
			})
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

func coverageDiscourseRole(hint string) string {
	switch strings.ToLower(strings.TrimSpace(hint)) {
	case "synthesis_bridge", "bridge", "midstream", "mechanism":
		return "mechanism"
	case "downstream", "outcome", "implication":
		return "implication"
	case "upstream", "driver", "thesis":
		return "thesis"
	default:
		return ""
	}
}

func coverageHintAddsBranchHead(hint string) bool {
	switch strings.ToLower(strings.TrimSpace(hint)) {
	case "downstream", "outcome", "implication", "target":
		return true
	default:
		return false
	}
}

func coverageHintSourceQuote(nodes []graphNode, fromID, toID string) string {
	from, _ := nodeByID(nodes, fromID)
	to, _ := nodeByID(nodes, toID)
	quote := combineQuoteWindow(from.SourceQuote, to.SourceQuote)
	if strings.TrimSpace(quote) != "" {
		return quote
	}
	return strings.TrimSpace(from.Text + " -> " + to.Text)
}

func hasCoverageHint(hints []coverageHint, fromID, toID string) bool {
	for _, hint := range hints {
		if strings.TrimSpace(hint.From) == fromID && strings.TrimSpace(hint.To) == toID {
			return true
		}
	}
	return false
}

func nextAvailableCoverageNodeID(nodes []graphNode) string {
	used := map[string]struct{}{}
	next := len(nodes) + 1
	for _, node := range nodes {
		id := strings.TrimSpace(node.ID)
		if id == "" {
			continue
		}
		used[id] = struct{}{}
		if !strings.HasPrefix(id, "n") {
			continue
		}
		number, err := strconv.Atoi(strings.TrimPrefix(id, "n"))
		if err != nil {
			continue
		}
		if number >= next {
			next = number + 1
		}
	}
	for {
		id := fmt.Sprintf("n%d", next)
		if _, ok := used[id]; !ok {
			return id
		}
		next++
	}
}
