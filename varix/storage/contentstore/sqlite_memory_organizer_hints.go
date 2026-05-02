package contentstore

import (
	"fmt"
	"sort"
	"strings"

	"github.com/kumaloha/VariX/varix/memory"
	"github.com/kumaloha/VariX/varix/model"
)

func ensureNodeSet(m map[string]map[string]struct{}, key string) map[string]struct{} {
	if existing, ok := m[key]; ok {
		return existing
	}
	set := map[string]struct{}{}
	m[key] = set
	return set
}

func sortedNodeSet(m map[string]struct{}) []string {
	if len(m) == 0 {
		return nil
	}
	out := make([]string, 0, len(m))
	for id := range m {
		out = append(out, id)
	}
	sort.Strings(out)
	return out
}

func buildNodeHints(nodes, active []memory.AcceptedNode, dedupeGroups []memory.DedupeGroup, contradictionGroups []memory.ContradictionGroup, hierarchy []memory.HierarchyLink, factStatusByNode map[string]model.FactStatus, explicitConditionStatusByNode map[string]model.ExplicitConditionStatus, predictionStatusByNode map[string]model.PredictionStatus) []memory.NodeHint {
	activeSet := map[string]struct{}{}
	for _, node := range active {
		activeSet[node.NodeID] = struct{}{}
	}
	preferredSet := map[string]struct{}{}
	dedupePeers := map[string]map[string]struct{}{}
	for _, group := range dedupeGroups {
		if strings.TrimSpace(group.RepresentativeNodeID) != "" {
			preferredSet[group.RepresentativeNodeID] = struct{}{}
		}
		for _, nodeID := range group.NodeIDs {
			peerSet := ensureNodeSet(dedupePeers, nodeID)
			for _, peerID := range group.NodeIDs {
				if peerID == nodeID {
					continue
				}
				peerSet[peerID] = struct{}{}
			}
		}
	}
	contradictionPeers := map[string]map[string]struct{}{}
	for _, group := range contradictionGroups {
		for _, nodeID := range group.NodeIDs {
			peerSet := ensureNodeSet(contradictionPeers, nodeID)
			for _, peerID := range group.NodeIDs {
				if peerID == nodeID {
					continue
				}
				peerSet[peerID] = struct{}{}
			}
		}
	}
	parentIDs := map[string]map[string]struct{}{}
	childIDs := map[string]map[string]struct{}{}
	for _, link := range hierarchy {
		ensureNodeSet(parentIDs, link.ChildNodeID)[link.ParentNodeID] = struct{}{}
		ensureNodeSet(childIDs, link.ParentNodeID)[link.ChildNodeID] = struct{}{}
	}
	out := make([]memory.NodeHint, 0, len(nodes))
	for _, node := range nodes {
		hint := memory.NodeHint{NodeID: node.NodeID}
		if _, ok := activeSet[node.NodeID]; ok {
			hint.State = "active"
		} else {
			hint.State = "inactive"
		}
		if _, ok := preferredSet[node.NodeID]; ok {
			hint.PreferredForDisplay = true
		}
		if status, ok := factStatusByNode[node.NodeID]; ok {
			hint.VerificationStatus = string(status)
		}
		if status, ok := explicitConditionStatusByNode[node.NodeID]; ok {
			hint.ConditionProbability = string(status)
		}
		if status, ok := predictionStatusByNode[node.NodeID]; ok {
			hint.PredictionStatus = string(status)
		}
		hint.PosteriorState = node.PosteriorState
		hint.PosteriorDiagnosis = node.PosteriorDiagnosis
		hint.PosteriorReason = node.PosteriorReason
		hint.BlockedByNodeIDs = cloneStringSlice(node.BlockedByNodeIDs)
		hint.DedupePeerNodeIDs = sortedNodeSet(dedupePeers[node.NodeID])
		hint.ContradictionNodeIDs = sortedNodeSet(contradictionPeers[node.NodeID])
		hint.ParentNodeIDs = sortedNodeSet(parentIDs[node.NodeID])
		hint.ChildNodeIDs = sortedNodeSet(childIDs[node.NodeID])
		switch node.PosteriorState {
		case memory.PosteriorStateVerified:
			hint.PreferredForDisplay = true
		case memory.PosteriorStateBlocked, memory.PosteriorStateFalsified:
			hint.PreferredForDisplay = false
		}
		if hint.State == "active" {
			switch {
			case len(hint.ParentNodeIDs) == 0 && len(hint.ChildNodeIDs) > 0:
				hint.HierarchyRole = "root"
			case len(hint.ParentNodeIDs) > 0 && len(hint.ChildNodeIDs) == 0:
				hint.HierarchyRole = "leaf"
			case len(hint.ParentNodeIDs) > 0 && len(hint.ChildNodeIDs) > 0:
				hint.HierarchyRole = "bridge"
			default:
				hint.HierarchyRole = "isolated"
			}
		}
		hint.NodeVerdict = deriveNodeVerdict(node, hint)
		out = append(out, hint)
	}
	return out
}

func deriveNodeVerdict(node memory.AcceptedNode, hint memory.NodeHint) string {
	switch node.PosteriorState {
	case memory.PosteriorStateFalsified:
		return "falsified"
	case memory.PosteriorStateBlocked:
		return "blocked"
	case memory.PosteriorStateVerified:
		return "supported"
	}
	switch hint.VerificationStatus {
	case string(model.FactStatusClearlyTrue):
		return "supported"
	case string(model.FactStatusClearlyFalse):
		return "contradicted"
	case string(model.FactStatusUnverifiable):
		return "needs_review"
	}
	switch hint.ConditionProbability {
	case string(model.ExplicitConditionStatusHigh), string(model.ExplicitConditionStatusMedium), string(model.ExplicitConditionStatusLow):
		return "supported"
	case string(model.ExplicitConditionStatusUnknown):
		return "needs_review"
	}
	switch hint.PredictionStatus {
	case string(model.PredictionStatusResolvedTrue):
		return "supported"
	case string(model.PredictionStatusResolvedFalse):
		return "contradicted"
	case string(model.PredictionStatusUnresolved), string(model.PredictionStatusStaleUnresolved):
		return "needs_review"
	}
	if node.PosteriorState == memory.PosteriorStatePending {
		return "needs_review"
	}
	if len(hint.ContradictionNodeIDs) > 0 {
		return "contested"
	}
	if hint.State == "inactive" {
		return "inactive"
	}
	return "active"
}

func buildDominantDriverSummary(active []memory.AcceptedNode, hints []memory.NodeHint) *memory.DominantDriverSummary {
	if len(active) == 0 || len(hints) == 0 {
		return nil
	}
	nodesByID := make(map[string]memory.AcceptedNode, len(active))
	hintsByID := make(map[string]memory.NodeHint, len(hints))
	childrenByID := map[string][]string{}
	for _, node := range active {
		nodesByID[node.NodeID] = node
	}
	for _, hint := range hints {
		if _, ok := nodesByID[hint.NodeID]; !ok {
			continue
		}
		hintsByID[hint.NodeID] = hint
		childrenByID[hint.NodeID] = cloneStringSlice(hint.ChildNodeIDs)
	}

	candidates := make([]memory.AcceptedNode, 0)
	for _, node := range active {
		hint, ok := hintsByID[node.NodeID]
		if !ok {
			continue
		}
		if isDriverKind(node.NodeKind) && (len(hint.ParentNodeIDs) == 0 || len(hint.ChildNodeIDs) > 0) {
			candidates = append(candidates, node)
		}
	}
	if len(candidates) == 0 {
		for _, node := range active {
			if node.NodeKind == string(model.NodePrediction) {
				continue
			}
			candidates = append(candidates, node)
		}
	}
	if len(candidates) == 0 {
		return nil
	}

	type scoredDriver struct {
		node       memory.AcceptedNode
		descendant int
		score      int
	}
	scored := make([]scoredDriver, 0, len(candidates))
	for _, node := range candidates {
		hint := hintsByID[node.NodeID]
		descendantCount := countDescendants(node.NodeID, childrenByID)
		score := descendantCount*10 + len(hint.ChildNodeIDs)*3 + driverKindWeight(node.NodeKind) + verdictWeight(hint.NodeVerdict)
		if hint.HierarchyRole == "root" {
			score += 5
		}
		if hint.PreferredForDisplay {
			score += 2
		}
		scored = append(scored, scoredDriver{
			node:       node,
			descendant: descendantCount,
			score:      score,
		})
	}
	sort.Slice(scored, func(i, j int) bool {
		if scored[i].score != scored[j].score {
			return scored[i].score > scored[j].score
		}
		if scored[i].descendant != scored[j].descendant {
			return scored[i].descendant > scored[j].descendant
		}
		return scored[i].node.NodeID < scored[j].node.NodeID
	})

	primary := scored[0]
	supporting := make([]string, 0, len(scored)-1)
	supportingTexts := make([]string, 0, len(scored)-1)
	for _, candidate := range scored[1:] {
		supporting = append(supporting, candidate.node.NodeID)
		supportingTexts = append(supportingTexts, firstNonBlank(candidate.node.NodeText, candidate.node.NodeID))
	}

	explanation := fmt.Sprintf("%s is the primary driver because it reaches %d downstream node(s) with the strongest current verdict.",
		firstNonBlank(primary.node.NodeText, primary.node.NodeID),
		primary.descendant,
	)
	if len(supporting) > 0 {
		explanation += fmt.Sprintf(" %s remain supporting because they carry weaker or narrower evidence in the same source path.",
			strings.Join(supportingTexts, ", "),
		)
	}

	return &memory.DominantDriverSummary{
		NodeID:            primary.node.NodeID,
		NodeKind:          primary.node.NodeKind,
		NodeText:          primary.node.NodeText,
		SupportingNodeIDs: supporting,
		Explanation:       explanation,
	}
}

func applyDominantDriverRoles(hints []memory.NodeHint, dominant *memory.DominantDriverSummary) []memory.NodeHint {
	if dominant == nil {
		return hints
	}
	supporting := map[string]struct{}{}
	for _, nodeID := range dominant.SupportingNodeIDs {
		supporting[nodeID] = struct{}{}
	}
	out := make([]memory.NodeHint, 0, len(hints))
	for _, hint := range hints {
		switch {
		case hint.NodeID == dominant.NodeID:
			hint.DriverRole = "primary"
			hint.PreferredForDisplay = true
		case len(supporting) > 0:
			if _, ok := supporting[hint.NodeID]; ok {
				hint.DriverRole = "supporting"
			}
		}
		out = append(out, hint)
	}
	return out
}

func buildOrganizationFeedback(nodes []memory.AcceptedNode, hints []memory.NodeHint) []memory.OrganizationFeedback {
	nodesByID := make(map[string]memory.AcceptedNode, len(nodes))
	for _, node := range nodes {
		nodesByID[node.NodeID] = node
	}
	type rankedFeedback struct {
		item     memory.OrganizationFeedback
		severity int
		priority int
	}
	ranked := make([]rankedFeedback, 0, len(hints))
	for _, hint := range hints {
		node, ok := nodesByID[hint.NodeID]
		if !ok {
			continue
		}
		feedback, severity, priority, ok := feedbackForHint(node, hint)
		if !ok {
			continue
		}
		ranked = append(ranked, rankedFeedback{
			item:     feedback,
			severity: severity,
			priority: priority,
		})
	}
	sort.Slice(ranked, func(i, j int) bool {
		if ranked[i].severity != ranked[j].severity {
			return ranked[i].severity > ranked[j].severity
		}
		if ranked[i].priority != ranked[j].priority {
			return ranked[i].priority > ranked[j].priority
		}
		return ranked[i].item.NodeID < ranked[j].item.NodeID
	})
	out := make([]memory.OrganizationFeedback, 0, len(ranked))
	for _, item := range ranked {
		out = append(out, item.item)
	}
	return out
}

func feedbackForHint(node memory.AcceptedNode, hint memory.NodeHint) (memory.OrganizationFeedback, int, int, bool) {
	base := memory.OrganizationFeedback{
		NodeID:   node.NodeID,
		NodeText: node.NodeText,
		NodeKind: node.NodeKind,
	}
	label := firstNonBlank(node.NodeText, node.NodeID)
	switch {
	case node.PosteriorState == memory.PosteriorStateFalsified:
		base.Severity = "error"
		base.Code = "posterior_falsified"
		base.Message = fmt.Sprintf("%s is falsified by posterior verification.", label)
		base.Reason = node.PosteriorReason
		return base, 3, 9, true
	case node.PosteriorState == memory.PosteriorStateBlocked:
		base.Severity = "error"
		base.Code = "posterior_blocked"
		base.Message = fmt.Sprintf("%s is blocked until its required conditions are resolved.", label)
		base.Reason = node.PosteriorReason
		return base, 3, 8, true
	case hint.VerificationStatus == string(model.FactStatusClearlyFalse):
		base.Severity = "error"
		base.Code = "fact_contradicted"
		base.Message = fmt.Sprintf("%s is contradicted by fact verification.", label)
		return base, 3, 7, true
	case hint.PredictionStatus == string(model.PredictionStatusResolvedFalse):
		base.Severity = "error"
		base.Code = "prediction_missed"
		base.Message = fmt.Sprintf("%s resolved false and should be treated as a failed prediction.", label)
		return base, 3, 6, true
	case len(hint.ContradictionNodeIDs) > 0:
		base.Severity = "warning"
		base.Code = "conflicting_nodes"
		base.Message = fmt.Sprintf("%s conflicts with node(s) %s.", label, strings.Join(hint.ContradictionNodeIDs, ", "))
		return base, 2, 5, true
	case node.PosteriorState == memory.PosteriorStatePending:
		base.Severity = "warning"
		base.Code = "posterior_pending"
		base.Message = fmt.Sprintf("%s still needs posterior verification.", label)
		base.Reason = node.PosteriorReason
		return base, 2, 4, true
	case hint.VerificationStatus == string(model.FactStatusUnverifiable):
		base.Severity = "warning"
		base.Code = "needs_evidence"
		base.Message = fmt.Sprintf("%s remains unverifiable and needs stronger evidence.", label)
		return base, 2, 3, true
	case hint.ConditionProbability == string(model.ExplicitConditionStatusUnknown):
		base.Severity = "warning"
		base.Code = "condition_unknown"
		base.Message = fmt.Sprintf("%s still has an unknown condition probability.", label)
		return base, 2, 2, true
	case hint.PredictionStatus == string(model.PredictionStatusUnresolved) || hint.PredictionStatus == string(model.PredictionStatusStaleUnresolved):
		base.Severity = "warning"
		base.Code = "prediction_open"
		base.Message = fmt.Sprintf("%s still needs prediction follow-through.", label)
		return base, 2, 1, true
	case len(hint.DedupePeerNodeIDs) > 0 && !hint.PreferredForDisplay:
		base.Severity = "info"
		base.Code = "near_duplicate"
		base.Message = fmt.Sprintf("%s overlaps with near-duplicate node(s) %s.", label, strings.Join(hint.DedupePeerNodeIDs, ", "))
		return base, 1, 1, true
	default:
		return memory.OrganizationFeedback{}, 0, 0, false
	}
}

func isDriverKind(kind string) bool {
	switch kind {
	case string(model.NodeFact), string(model.NodeExplicitCondition), string(model.NodeImplicitCondition):
		return true
	default:
		return false
	}
}

func countDescendants(nodeID string, childrenByID map[string][]string) int {
	seen := map[string]struct{}{}
	var walk func(string)
	walk = func(current string) {
		for _, childID := range childrenByID[current] {
			if _, ok := seen[childID]; ok {
				continue
			}
			seen[childID] = struct{}{}
			walk(childID)
		}
	}
	walk(nodeID)
	return len(seen)
}

func driverKindWeight(kind string) int {
	switch kind {
	case string(model.NodeFact):
		return 40
	case string(model.NodeExplicitCondition):
		return 35
	case string(model.NodeImplicitCondition):
		return 30
	case string(model.NodeConclusion):
		return 20
	case string(model.NodePrediction):
		return 10
	default:
		return 0
	}
}

func verdictWeight(verdict string) int {
	switch verdict {
	case "supported":
		return 20
	case "active":
		return 10
	case "needs_review":
		return 0
	case "contested":
		return -5
	case "blocked":
		return -20
	case "falsified", "contradicted":
		return -25
	case "inactive":
		return -30
	default:
		return 0
	}
}
