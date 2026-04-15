package contentstore

import (
	"strings"

	"github.com/kumaloha/VariX/varix/memory"
)

func buildCognitiveCards(thesis memory.CausalThesis, nodesByID map[string]memory.AcceptedNode) []memory.CognitiveCard {
	if len(thesis.CorePathNodeIDs) == 0 {
		return nil
	}
	card := memory.CognitiveCard{
		CardID:          thesis.CausalThesisID + "-card-1",
		CausalThesisID:  thesis.CausalThesisID,
		CardType:        "judgment",
		Title:           cardTitle(thesis, nodesByID),
		Summary:         cardSummary(thesis, nodesByID),
		CausalChain:     buildCardChainSteps(thesis, nodesByID),
		KeyEvidence:     keyEvidenceTexts(thesis, nodesByID),
		Conditions:      cardConditions(thesis, nodesByID),
		Predictions:     predictionTexts(thesis, nodesByID),
		SourceRefs:      append([]string(nil), thesis.SourceRefs...),
		ConfidenceLabel: cardConfidence(thesis),
	}
	return []memory.CognitiveCard{card}
}

func buildCardChainSteps(thesis memory.CausalThesis, nodesByID map[string]memory.AcceptedNode) []memory.CardChainStep {
	steps := make([]memory.CardChainStep, 0, len(thesis.CorePathNodeIDs))
	for _, id := range thesis.CorePathNodeIDs {
		node, ok := nodesByID[id]
		if !ok {
			continue
		}
		steps = append(steps, memory.CardChainStep{
			Label:          node.NodeText,
			Role:           thesis.NodeRoles[id],
			BackingNodeIDs: []string{id},
		})
	}
	return steps
}

func cardTitle(thesis memory.CausalThesis, nodesByID map[string]memory.AcceptedNode) string {
	for i := len(thesis.CorePathNodeIDs) - 1; i >= 0; i-- {
		id := thesis.CorePathNodeIDs[i]
		if thesis.NodeRoles[id] == "conclusion" {
			if node, ok := nodesByID[id]; ok {
				return node.NodeText
			}
		}
	}
	last := thesis.CorePathNodeIDs[len(thesis.CorePathNodeIDs)-1]
	if node, ok := nodesByID[last]; ok {
		return node.NodeText
	}
	return thesis.CoreQuestion
}

func cardSummary(thesis memory.CausalThesis, nodesByID map[string]memory.AcceptedNode) string {
	steps := buildCardChainSteps(thesis, nodesByID)
	if len(steps) == 0 {
		return ""
	}
	labels := make([]string, 0, len(steps))
	for _, step := range steps {
		if strings.TrimSpace(step.Label) == "" {
			continue
		}
		labels = append(labels, step.Label)
	}
	return strings.Join(labels, " → ")
}

func cardConfidence(thesis memory.CausalThesis) string {
	switch {
	case thesis.CompletenessScore >= 0.8:
		return "strong"
	case thesis.CompletenessScore >= 0.6:
		return "medium"
	default:
		return "weak"
	}
}

func collectNodeTexts(ids []string, nodesByID map[string]memory.AcceptedNode) []string {
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		node, ok := nodesByID[id]
		if !ok || strings.TrimSpace(node.NodeText) == "" {
			continue
		}
		out = append(out, node.NodeText)
	}
	return out
}

func predictionTexts(thesis memory.CausalThesis, nodesByID map[string]memory.AcceptedNode) []string {
	ids := make([]string, 0)
	for _, id := range thesis.CorePathNodeIDs {
		if thesis.NodeRoles[id] == "prediction" {
			ids = append(ids, id)
		}
	}
	return collectNodeTexts(ids, nodesByID)
}

func keyEvidenceTexts(thesis memory.CausalThesis, nodesByID map[string]memory.AcceptedNode) []string {
	ids := make([]string, 0, len(thesis.SupportingNodeIDs)+len(thesis.CorePathNodeIDs))
	for _, id := range thesis.SupportingNodeIDs {
		if thesis.NodeRoles[id] == "fact" || thesis.NodeRoles[id] == "mechanism" {
			ids = append(ids, id)
		}
	}
	for _, id := range thesis.CorePathNodeIDs {
		if thesis.NodeRoles[id] == "fact" || thesis.NodeRoles[id] == "mechanism" {
			ids = append(ids, id)
		}
	}
	return uniquePreservingOrder(collectNodeTexts(uniquePreservingOrder(ids), nodesByID))
}

func uniquePreservingOrder(in []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(in))
	for _, item := range in {
		if _, ok := seen[item]; ok {
			continue
		}
		seen[item] = struct{}{}
		out = append(out, item)
	}
	return out
}

func cardConditions(thesis memory.CausalThesis, nodesByID map[string]memory.AcceptedNode) []string {
	ids := make([]string, 0, len(thesis.BoundaryNodeIDs)+len(thesis.CorePathNodeIDs))
	ids = append(ids, thesis.BoundaryNodeIDs...)
	for _, id := range thesis.CorePathNodeIDs {
		if thesis.NodeRoles[id] == "condition" {
			ids = append(ids, id)
		}
	}
	return uniquePreservingOrder(collectNodeTexts(uniquePreservingOrder(ids), nodesByID))
}
