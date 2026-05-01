package compile

import (
	"fmt"
	"strings"
	"time"
)

func buildCoverage(expectedNodeIDs, returnedNodeIDs []string) VerificationPassCoverage {
	coverage := VerificationPassCoverage{
		ExpectedNodeIDs: CloneStrings(expectedNodeIDs),
		ReturnedNodeIDs: CloneStrings(returnedNodeIDs),
		Valid:           true,
	}
	expected := make(map[string]struct{}, len(expectedNodeIDs))
	for _, id := range expectedNodeIDs {
		expected[id] = struct{}{}
	}
	returnedCounts := make(map[string]int, len(returnedNodeIDs))
	for _, id := range returnedNodeIDs {
		returnedCounts[id]++
		if _, ok := expected[id]; !ok {
			coverage.UnexpectedNodeIDs = append(coverage.UnexpectedNodeIDs, id)
		}
	}
	for _, id := range expectedNodeIDs {
		if returnedCounts[id] == 0 {
			coverage.MissingNodeIDs = append(coverage.MissingNodeIDs, id)
		}
	}
	for _, id := range returnedNodeIDs {
		if returnedCounts[id] > 1 && !containsString(coverage.DuplicateNodeIDs, id) {
			coverage.DuplicateNodeIDs = append(coverage.DuplicateNodeIDs, id)
		}
	}
	coverage.Valid = len(coverage.MissingNodeIDs) == 0 && len(coverage.DuplicateNodeIDs) == 0 && len(coverage.UnexpectedNodeIDs) == 0 && len(expectedNodeIDs) == len(returnedNodeIDs)
	return coverage
}

func coverageError(kind VerificationPassKind, coverage VerificationPassCoverage) error {
	return fmt.Errorf(
		"verification pass %s coverage mismatch: expected=%v returned=%v missing=%v duplicate=%v unexpected=%v",
		kind,
		coverage.ExpectedNodeIDs,
		coverage.ReturnedNodeIDs,
		coverage.MissingNodeIDs,
		coverage.DuplicateNodeIDs,
		coverage.UnexpectedNodeIDs,
	)
}

func aggregateCoverageSummary(results []verifierPassResult) *VerificationCoverageSummary {
	summary := &VerificationCoverageSummary{Valid: true}
	for _, result := range results {
		summary.TotalExpectedNodes += len(result.coverage.ExpectedNodeIDs)
		summary.TotalFinalizedNodes += len(result.coverage.ReturnedNodeIDs)
		summary.MissingNodeIDs = append(summary.MissingNodeIDs, result.coverage.MissingNodeIDs...)
		summary.DuplicateNodeIDs = append(summary.DuplicateNodeIDs, result.coverage.DuplicateNodeIDs...)
		summary.UnexpectedNodeIDs = append(summary.UnexpectedNodeIDs, result.coverage.UnexpectedNodeIDs...)
		if !result.coverage.Valid {
			summary.Valid = false
		}
	}
	return summary
}

func uniformAdjudicationModel(results []verifierPassResult) string {
	var models []string
	for _, result := range results {
		if result.adjudication == nil {
			continue
		}
		model := strings.TrimSpace(result.adjudication.Model)
		if model == "" {
			return ""
		}
		if len(models) == 0 {
			models = append(models, model)
			continue
		}
		if model != models[0] {
			return ""
		}
	}
	if len(models) == 0 {
		return ""
	}
	return models[0]
}

func verifierRolloutStage(results []verifierPassResult) string {
	sawRealized := false
	sawFuture := false
	for _, result := range results {
		if result.kind == VerificationPassFact && result.debateEnabled {
			sawRealized = true
			continue
		}
		if result.kind == VerificationPassExplicitCondition || result.kind == VerificationPassImplicitCondition || result.kind == VerificationPassPrediction {
			sawFuture = true
		}
	}
	if sawRealized && sawFuture {
		return "time_split"
	}
	if sawRealized {
		return "facts_only"
	}
	if sawFuture {
		return "future_only"
	}
	return "coverage_only"
}

func newStageSummary(model string, completedAt time.Time, parseOK bool, outputNodeIDs []string) *VerificationStageSummary {
	return &VerificationStageSummary{
		Model:         strings.TrimSpace(model),
		CompletedAt:   completedAt.UTC(),
		ParseOK:       parseOK,
		OutputNodeIDs: CloneStrings(outputNodeIDs),
	}
}

func verifierStageSummary(responseModel, fallbackModel string, completedAt time.Time, outputNodeIDs []string) *VerificationStageSummary {
	return newStageSummary(FirstNonEmpty(responseModel, fallbackModel), completedAt, true, outputNodeIDs)
}

func cloneStageSummary(in *VerificationStageSummary) *VerificationStageSummary {
	if in == nil {
		return nil
	}
	out := *in
	out.OutputNodeIDs = CloneStrings(in.OutputNodeIDs)
	return &out
}

func nodeIDs(nodes []GraphNode) []string {
	ids := make([]string, 0, len(nodes))
	for _, node := range nodes {
		ids = append(ids, node.ID)
	}
	return ids
}

func factCheckNodeIDs(checks []FactCheck) []string {
	ids := make([]string, 0, len(checks))
	for _, check := range checks {
		ids = append(ids, check.NodeID)
	}
	return ids
}

func explicitConditionCheckNodeIDs(checks []ExplicitConditionCheck) []string {
	ids := make([]string, 0, len(checks))
	for _, check := range checks {
		ids = append(ids, check.NodeID)
	}
	return ids
}

func implicitConditionCheckNodeIDs(checks []ImplicitConditionCheck) []string {
	ids := make([]string, 0, len(checks))
	for _, check := range checks {
		ids = append(ids, check.NodeID)
	}
	return ids
}

func predictionCheckNodeIDs(checks []PredictionCheck) []string {
	ids := make([]string, 0, len(checks))
	for _, check := range checks {
		ids = append(ids, check.NodeID)
	}
	return ids
}

func factChallengeNodeIDs(challenges []factChallenge) []string {
	ids := make([]string, 0, len(challenges))
	for _, challenge := range challenges {
		ids = append(ids, challenge.NodeID)
	}
	return ids
}
