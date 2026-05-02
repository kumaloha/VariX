package contentstore

import (
	"sort"
	"strings"
	"time"

	"github.com/kumaloha/VariX/varix/memory"
	"github.com/kumaloha/VariX/varix/model"
)

func evaluatePosteriorState(
	node memory.AcceptedNode,
	current memory.PosteriorStateRecord,
	graphNodesByID map[string]model.GraphNode,
	predecessors map[string][]string,
	verification model.Verification,
	record model.Record,
	allUserNodes []memory.AcceptedNode,
	now time.Time,
) memory.PosteriorStateRecord {
	next := current
	next.MemoryID = node.MemoryID
	next.SourcePlatform = node.SourcePlatform
	next.SourceExternalID = node.SourceExternalID
	next.NodeID = node.NodeID
	next.NodeKind = node.NodeKind
	next.LastEvaluatedAt = now
	if next.State == "" {
		next.State = memory.PosteriorStatePending
	}

	blockedBy, blockedReason := blockedByConditions(node.NodeID, graphNodesByID, predecessors, verification)
	if len(blockedBy) > 0 {
		next.State = memory.PosteriorStateBlocked
		next.DiagnosisCode = ""
		next.Reason = blockedReason
		next.BlockedByNodeIDs = blockedBy
		next.LastEvidenceAt = time.Time{}
		return finalizePosteriorTransition(current, next, now)
	}

	next.BlockedByNodeIDs = nil
	switch strings.TrimSpace(node.NodeKind) {
	case string(model.NodePrediction):
		evaluatePredictionPosterior(node, &next, graphNodesByID[node.NodeID], verification, record, now)
	case string(model.NodeConclusion):
		evaluateConclusionPosterior(node, &next, graphNodesByID, predecessors, verification, record, allUserNodes, now)
	default:
		next.State = memory.PosteriorStatePending
		next.DiagnosisCode = ""
		next.Reason = ""
		next.LastEvidenceAt = time.Time{}
	}
	return finalizePosteriorTransition(current, next, now)
}

func finalizePosteriorTransition(current, next memory.PosteriorStateRecord, now time.Time) memory.PosteriorStateRecord {
	if next.UpdatedAt.IsZero() {
		if posteriorMateriallyChanged(current, next) || current.UpdatedAt.IsZero() {
			next.UpdatedAt = now
		} else {
			next.UpdatedAt = current.UpdatedAt
		}
	}
	return next
}

func evaluatePredictionPosterior(node memory.AcceptedNode, state *memory.PosteriorStateRecord, graphNode model.GraphNode, verification model.Verification, record model.Record, now time.Time) {
	checks := predictionStatusMap(verification)
	for _, check := range verification.PredictionChecks {
		if check.NodeID != node.NodeID {
			continue
		}
		switch check.Status {
		case model.PredictionStatusResolvedTrue:
			state.State = memory.PosteriorStateVerified
			state.DiagnosisCode = ""
			state.Reason = strings.TrimSpace(check.Reason)
			state.LastEvidenceAt = maxTime(record.CompiledAt.UTC(), check.AsOf.UTC())
			return
		case model.PredictionStatusResolvedFalse:
			state.State = memory.PosteriorStateFalsified
			state.DiagnosisCode = memory.PosteriorDiagnosisFactError
			state.Reason = firstNonBlank(check.Reason, "prediction resolved false")
			state.LastEvidenceAt = maxTime(record.CompiledAt.UTC(), check.AsOf.UTC())
			return
		}
	}

	due := graphNode.PredictionDueAt
	if due.IsZero() {
		due = node.ValidTo
	}
	if !due.IsZero() && now.Before(due) {
		state.State = memory.PosteriorStatePending
		state.DiagnosisCode = ""
		state.Reason = "prediction due time not reached"
		state.LastEvidenceAt = time.Time{}
		return
	}

	if status, ok := checks[node.NodeID]; ok && status == model.PredictionStatusStaleUnresolved {
		state.State = memory.PosteriorStatePending
		state.DiagnosisCode = ""
		state.Reason = "prediction still unresolved after due time"
		state.LastEvidenceAt = time.Time{}
		return
	}

	state.State = memory.PosteriorStatePending
	state.DiagnosisCode = ""
	state.Reason = "awaiting fresh posterior evidence"
	state.LastEvidenceAt = time.Time{}
}

func evaluateConclusionPosterior(
	node memory.AcceptedNode,
	state *memory.PosteriorStateRecord,
	graphNodesByID map[string]model.GraphNode,
	predecessors map[string][]string,
	verification model.Verification,
	record model.Record,
	allUserNodes []memory.AcceptedNode,
	now time.Time,
) {
	factStatuses := factStatusMap(verification)
	for _, ancestorID := range collectAncestorNodeIDs(node.NodeID, predecessors) {
		ancestor, ok := graphNodesByID[ancestorID]
		if !ok {
			continue
		}
		switch ancestor.Kind {
		case model.NodeFact, model.NodeImplicitCondition:
			if factStatuses[ancestorID] == model.FactStatusClearlyFalse {
				state.State = memory.PosteriorStateFalsified
				state.DiagnosisCode = memory.PosteriorDiagnosisFactError
				state.Reason = firstNonBlank(ancestor.Text, "supporting evidence was later contradicted")
				state.LastEvidenceAt = record.CompiledAt.UTC()
				return
			}
		}
	}

	if conflictingNode, reason, ok := fresherContradictingNode(node, allUserNodes, now); ok {
		state.State = memory.PosteriorStateFalsified
		state.DiagnosisCode = memory.PosteriorDiagnosisLogicError
		state.Reason = firstNonBlank(reason, conflictingNode.NodeText, "fresher contradiction detected")
		state.LastEvidenceAt = conflictingNode.SourceCompiledAt.UTC()
		return
	}

	state.State = memory.PosteriorStatePending
	state.DiagnosisCode = ""
	state.Reason = "insufficient deterministic posterior evidence"
	state.LastEvidenceAt = time.Time{}
}

func blockedByConditions(nodeID string, graphNodesByID map[string]model.GraphNode, predecessors map[string][]string, verification model.Verification) ([]string, string) {
	explicitStatuses := explicitConditionStatusMap(verification)
	blockedBy := make([]string, 0)
	reasons := make([]string, 0)
	for _, ancestorID := range collectAncestorNodeIDs(nodeID, predecessors) {
		ancestor, ok := graphNodesByID[ancestorID]
		if !ok || ancestor.Kind != model.NodeExplicitCondition {
			continue
		}
		status, ok := explicitStatuses[ancestorID]
		if !ok {
			blockedBy = append(blockedBy, ancestorID)
			reasons = append(reasons, "condition unresolved")
			continue
		}
		switch status {
		case model.ExplicitConditionStatusHigh, model.ExplicitConditionStatusMedium:
			continue
		default:
			blockedBy = append(blockedBy, ancestorID)
			reasons = append(reasons, strings.TrimSpace(ancestor.Text))
		}
	}
	if len(blockedBy) == 0 {
		return nil, ""
	}
	sort.Strings(blockedBy)
	return blockedBy, firstNonBlank(strings.Join(uniquePreservingOrder(reasons), "; "), "required condition unresolved")
}

func fresherContradictingNode(node memory.AcceptedNode, allUserNodes []memory.AcceptedNode, now time.Time) (memory.AcceptedNode, string, bool) {
	var best memory.AcceptedNode
	var bestReason string
	found := false
	for _, candidate := range allUserNodes {
		if candidate.MemoryID == node.MemoryID {
			continue
		}
		if !isPosteriorEligibleNodeKind(candidate.NodeKind) {
			continue
		}
		if !candidate.SourceCompiledAt.After(node.SourceCompiledAt) {
			continue
		}
		if !isAcceptedNodeActiveAt(candidate, now) {
			continue
		}
		reason, ok := contradictionReason(node.NodeText, candidate.NodeText)
		if !ok {
			continue
		}
		if !found || candidate.SourceCompiledAt.After(best.SourceCompiledAt) {
			best = candidate
			bestReason = reason
			found = true
		}
	}
	return best, bestReason, found
}

func collectAncestorNodeIDs(nodeID string, predecessors map[string][]string) []string {
	seen := map[string]struct{}{}
	queue := cloneStringSlice(predecessors[nodeID])
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		if _, ok := seen[current]; ok {
			continue
		}
		seen[current] = struct{}{}
		queue = append(queue, predecessors[current]...)
	}
	out := make([]string, 0, len(seen))
	for id := range seen {
		out = append(out, id)
	}
	sort.Strings(out)
	return out
}

func graphPredecessorIndex(edges []model.GraphEdge) map[string][]string {
	out := make(map[string][]string)
	for _, edge := range edges {
		out[edge.To] = append(out[edge.To], edge.From)
	}
	for nodeID := range out {
		sort.Strings(out[nodeID])
	}
	return out
}

func posteriorMateriallyChanged(current, next memory.PosteriorStateRecord) bool {
	if current.State != next.State {
		return true
	}
	if current.DiagnosisCode != next.DiagnosisCode {
		return true
	}
	if current.Reason != next.Reason {
		return true
	}
	if !sameStringSlice(current.BlockedByNodeIDs, next.BlockedByNodeIDs) {
		return true
	}
	if !current.LastEvidenceAt.Equal(next.LastEvidenceAt) {
		return true
	}
	if current.UpdatedAt.IsZero() {
		return true
	}
	return false
}

func containsPosteriorNodeState(states []posteriorNodeState, nodeID string) bool {
	for _, state := range states {
		if state.node.NodeID == nodeID {
			return true
		}
	}
	return false
}

func sameStringSlice(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}

func firstNonBlank(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}
