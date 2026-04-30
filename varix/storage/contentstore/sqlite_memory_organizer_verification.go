package contentstore

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/kumaloha/VariX/varix/compile"
	"github.com/kumaloha/VariX/varix/graphmodel"
	"github.com/kumaloha/VariX/varix/memory"
)

func extractPredictionStatuses(nodes []memory.AcceptedNode, verification compile.Verification) []memory.PredictionStatus {
	accepted := map[string]struct{}{}
	for _, node := range nodes {
		accepted[node.NodeID] = struct{}{}
	}
	out := make([]memory.PredictionStatus, 0)
	for _, check := range verification.PredictionChecks {
		if _, ok := accepted[check.NodeID]; !ok {
			continue
		}
		out = append(out, memory.PredictionStatus{
			NodeID: check.NodeID,
			Status: string(check.Status),
			Reason: check.Reason,
		})
	}
	return out
}

func extractFactVerifications(nodes []memory.AcceptedNode, verification compile.Verification) []memory.FactVerification {
	active := map[string]struct{}{}
	for _, node := range nodes {
		active[node.NodeID] = struct{}{}
	}
	out := make([]memory.FactVerification, 0)
	for _, check := range verification.FactChecks {
		if _, ok := active[check.NodeID]; !ok {
			continue
		}
		out = append(out, memory.FactVerification{
			NodeID: check.NodeID,
			Status: string(check.Status),
			Reason: check.Reason,
		})
	}
	for _, check := range verification.ImplicitConditionChecks {
		if _, ok := active[check.NodeID]; !ok {
			continue
		}
		out = append(out, memory.FactVerification{
			NodeID: check.NodeID,
			Status: string(check.Status),
			Reason: check.Reason,
		})
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].NodeID < out[j].NodeID
	})
	return out
}

func buildOpenQuestions(nodes []memory.AcceptedNode, verification compile.Verification) []string {
	questions := make([]string, 0)
	for _, node := range nodes {
		if node.ValidFrom.IsZero() && node.ValidTo.IsZero() && node.NodeKind != string(compile.NodeExplicitCondition) && node.NodeKind != string(compile.NodeConclusion) {
			questions = append(questions, fmt.Sprintf("node %s has no validity window", node.NodeID))
		}
		switch node.PosteriorState {
		case memory.PosteriorStatePending:
			questions = append(questions, fmt.Sprintf("posterior check pending for node %s", node.NodeID))
		case memory.PosteriorStateBlocked:
			if len(node.BlockedByNodeIDs) > 0 {
				questions = append(questions, fmt.Sprintf("node %s blocked by conditions: %s", node.NodeID, strings.Join(node.BlockedByNodeIDs, ", ")))
			} else {
				questions = append(questions, fmt.Sprintf("node %s remains posterior-blocked", node.NodeID))
			}
		case memory.PosteriorStateFalsified:
			if strings.TrimSpace(string(node.PosteriorDiagnosis)) != "" {
				questions = append(questions, fmt.Sprintf("node %s was falsified (%s)", node.NodeID, node.PosteriorDiagnosis))
			} else {
				questions = append(questions, fmt.Sprintf("node %s was falsified", node.NodeID))
			}
		}
	}
	for _, check := range verification.FactChecks {
		if check.Status == compile.FactStatusUnverifiable {
			questions = append(questions, fmt.Sprintf("fact node %s remains unverifiable", check.NodeID))
		}
	}
	for _, check := range verification.ImplicitConditionChecks {
		if check.Status == compile.FactStatusUnverifiable {
			questions = append(questions, fmt.Sprintf("implicit condition node %s remains unverifiable", check.NodeID))
		}
	}
	for _, check := range verification.ExplicitConditionChecks {
		if check.Status == compile.ExplicitConditionStatusUnknown {
			questions = append(questions, fmt.Sprintf("explicit condition node %s remains probability-unknown", check.NodeID))
		}
	}
	return questions
}

func isAcceptedNodeActiveAt(node memory.AcceptedNode, now time.Time) bool {
	switch node.NodeKind {
	case string(compile.NodeFact), string(compile.NodeImplicitCondition), string(compile.NodePrediction):
		if node.ValidFrom.IsZero() {
			return false
		}
		if !now.Before(node.ValidFrom) && (node.ValidTo.IsZero() || !now.After(node.ValidTo)) {
			return true
		}
		return false
	case string(compile.NodeExplicitCondition), string(compile.NodeConclusion):
		if node.ValidFrom.IsZero() && node.ValidTo.IsZero() {
			return true
		}
		if node.ValidFrom.IsZero() {
			return false
		}
		if node.ValidTo.IsZero() {
			return !now.Before(node.ValidFrom)
		}
		return !now.Before(node.ValidFrom) && !now.After(node.ValidTo)
	default:
		if node.ValidFrom.IsZero() {
			return false
		}
		if node.ValidTo.IsZero() {
			return !now.Before(node.ValidFrom)
		}
		return !now.Before(node.ValidFrom) && !now.After(node.ValidTo)
	}
}

func factStatusMap(verification compile.Verification) map[string]compile.FactStatus {
	out := make(map[string]compile.FactStatus, len(verification.FactChecks)+len(verification.ImplicitConditionChecks))
	for _, check := range verification.FactChecks {
		out[check.NodeID] = check.Status
	}
	for _, check := range verification.ImplicitConditionChecks {
		out[check.NodeID] = check.Status
	}
	return out
}

func explicitConditionStatusMap(verification compile.Verification) map[string]compile.ExplicitConditionStatus {
	out := make(map[string]compile.ExplicitConditionStatus, len(verification.ExplicitConditionChecks))
	for _, check := range verification.ExplicitConditionChecks {
		out[check.NodeID] = check.Status
	}
	return out
}

func predictionStatusMap(verification compile.Verification) map[string]compile.PredictionStatus {
	out := make(map[string]compile.PredictionStatus, len(verification.PredictionChecks))
	for _, check := range verification.PredictionChecks {
		out[check.NodeID] = check.Status
	}
	return out
}

func effectiveVerification(record compile.Record, verifyRecord compile.VerificationRecord) compile.Verification {
	if !verifyRecord.VerifiedAt.IsZero() || verifyRecord.Verification.VerifiedAt.After(time.Time{}) || len(verifyRecord.Verification.Passes) > 0 || len(verifyRecord.Verification.FactChecks) > 0 || len(verifyRecord.Verification.ExplicitConditionChecks) > 0 || len(verifyRecord.Verification.ImplicitConditionChecks) > 0 || len(verifyRecord.Verification.PredictionChecks) > 0 {
		return verifyRecord.Verification
	}
	return record.Output.Verification
}

func overlayVerificationFromContentGraph(verification compile.Verification, subgraph graphmodel.ContentSubgraph) compile.Verification {
	predictionByNodeID := make(map[string]compile.PredictionCheck, len(verification.PredictionChecks))
	for _, check := range verification.PredictionChecks {
		predictionByNodeID[check.NodeID] = check
	}
	factByNodeID := make(map[string]compile.FactCheck, len(verification.FactChecks))
	for _, check := range verification.FactChecks {
		factByNodeID[check.NodeID] = check
	}
	for _, node := range subgraph.Nodes {
		if strings.TrimSpace(node.VerificationReason) == "" && strings.TrimSpace(node.VerificationAsOf) == "" && strings.TrimSpace(node.NextVerifyAt) == "" {
			continue
		}
		switch node.Kind {
		case graphmodel.NodeKindPrediction:
			status := compile.PredictionStatusUnresolved
			switch node.VerificationStatus {
			case graphmodel.VerificationProved:
				status = compile.PredictionStatusResolvedTrue
			case graphmodel.VerificationDisproved:
				status = compile.PredictionStatusResolvedFalse
			case graphmodel.VerificationPending:
				status = compile.PredictionStatusUnresolved
			case graphmodel.VerificationUnverifiable:
				status = compile.PredictionStatusStaleUnresolved
			}
			predictionByNodeID[node.ID] = compile.PredictionCheck{NodeID: node.ID, Status: status, Reason: node.VerificationReason, AsOf: parseSQLiteTime(node.VerificationAsOf)}
		default:
			var status compile.FactStatus
			switch node.VerificationStatus {
			case graphmodel.VerificationProved:
				status = compile.FactStatusClearlyTrue
			case graphmodel.VerificationDisproved:
				status = compile.FactStatusClearlyFalse
			case graphmodel.VerificationUnverifiable:
				status = compile.FactStatusUnverifiable
			default:
				continue
			}
			factByNodeID[node.ID] = compile.FactCheck{NodeID: node.ID, Status: status, Reason: node.VerificationReason}
		}
	}
	if len(predictionByNodeID) > 0 {
		verification.PredictionChecks = verification.PredictionChecks[:0]
		for _, check := range predictionByNodeID {
			verification.PredictionChecks = append(verification.PredictionChecks, check)
		}
		sort.Slice(verification.PredictionChecks, func(i, j int) bool {
			return verification.PredictionChecks[i].NodeID < verification.PredictionChecks[j].NodeID
		})
	}
	if len(factByNodeID) > 0 {
		verification.FactChecks = verification.FactChecks[:0]
		for _, check := range factByNodeID {
			verification.FactChecks = append(verification.FactChecks, check)
		}
		sort.Slice(verification.FactChecks, func(i, j int) bool { return verification.FactChecks[i].NodeID < verification.FactChecks[j].NodeID })
	}
	return verification
}

func graphFirstValidityWindow(node graphmodel.GraphNode) (time.Time, time.Time) {
	start := parseSQLiteTime(node.TimeStart)
	end := parseSQLiteTime(node.TimeEnd)
	if node.Kind == graphmodel.NodeKindObservation {
		if start.IsZero() {
			return time.Time{}, time.Time{}
		}
		if end.IsZero() || end.Equal(start) {
			return start, time.Date(9999, 12, 31, 23, 59, 59, 0, time.UTC)
		}
	}
	return start, end
}
