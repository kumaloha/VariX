package verify

import (
	"context"
	"time"
)

type verifierCall = runtimeChat

var buildFactRetrievalContext = func(ctx context.Context, bundle Bundle, nodes []GraphNode) ([]map[string]any, error) {
	return nil, nil
}

var verifierNow = func() time.Time {
	return NowUTC()
}

type verifierPassResult struct {
	kind                    VerificationPassKind
	nodeIDs                 []string
	realizedChecks          []RealizedCheck
	futureConditionChecks   []FutureConditionCheck
	factChecks              []FactCheck
	explicitConditionChecks []ExplicitConditionCheck
	implicitConditionChecks []ImplicitConditionCheck
	predictionChecks        []PredictionCheck
	claim                   *VerificationStageSummary
	challenge               *VerificationStageSummary
	adjudication            *VerificationStageSummary
	coverage                VerificationPassCoverage
	retrievalSummary        *VerificationRetrievalSummary
	debateEnabled           bool
}

type factChallenge struct {
	NodeID     string `json:"node_id"`
	Assessment string `json:"assessment,omitempty"`
	Reason     string `json:"reason,omitempty"`
}

type verificationTimeBucket string

const (
	verificationBucketUndetermined verificationTimeBucket = "undetermined"
	verificationBucketRealized     verificationTimeBucket = "realized"
	verificationBucketFuture       verificationTimeBucket = "future"
)

func runVerifier(ctx context.Context, rt verifierCall, model string, prompts *promptRegistry, bundle Bundle, output Output) (Verification, error) {
	if prompts == nil {
		prompts = newPromptRegistry("")
	}
	var passResults []verifierPassResult

	realizedNodes := make([]GraphNode, 0)
	explicitConditionNodes := make([]GraphNode, 0)
	implicitConditionNodes := make([]GraphNode, 0)
	futureClaimNodes := make([]GraphNode, 0)
	for _, node := range output.Graph.Nodes {
		if isConditionVerifierNode(node) {
			if isImplicitConditionVerifierNode(node) {
				implicitConditionNodes = append(implicitConditionNodes, node)
			} else {
				explicitConditionNodes = append(explicitConditionNodes, node)
			}
			continue
		}
		switch classifyVerificationTimeBucket(bundle, node) {
		case verificationBucketRealized:
			realizedNodes = append(realizedNodes, node)
		case verificationBucketFuture:
			futureClaimNodes = append(futureClaimNodes, node)
		}
	}

	if len(realizedNodes) > 0 {
		result, err := verifyRealized(ctx, rt, model, prompts, bundle, realizedNodes)
		if err != nil {
			return Verification{}, err
		}
		passResults = append(passResults, result)
	}
	if len(explicitConditionNodes) > 0 {
		result, err := verifyExplicitConditions(ctx, rt, model, prompts, bundle, explicitConditionNodes)
		if err != nil {
			return Verification{}, err
		}
		passResults = append(passResults, result)
	}
	if len(implicitConditionNodes) > 0 {
		result, err := verifyImplicitConditions(ctx, rt, model, prompts, bundle, implicitConditionNodes)
		if err != nil {
			return Verification{}, err
		}
		passResults = append(passResults, result)
	}
	if len(futureClaimNodes) > 0 {
		result, err := verifyPredictions(ctx, rt, model, prompts, bundle, futureClaimNodes)
		if err != nil {
			return Verification{}, err
		}
		passResults = append(passResults, result)
	}

	if len(passResults) == 0 {
		return Verification{}, nil
	}
	for _, result := range passResults {
		if !result.coverage.Valid {
			return Verification{}, coverageError(result.kind, result.coverage)
		}
	}

	verification := Verification{
		Version:               "fact_verifier",
		RolloutStage:          verifierRolloutStage(passResults),
		RealizedChecks:        make([]RealizedCheck, 0),
		FutureConditionChecks: make([]FutureConditionCheck, 0),
		Passes:                make([]VerificationPass, 0, len(passResults)),
		CoverageSummary:       aggregateCoverageSummary(passResults),
	}
	for _, result := range passResults {
		verification.RealizedChecks = append(verification.RealizedChecks, result.realizedChecks...)
		verification.FutureConditionChecks = append(verification.FutureConditionChecks, result.futureConditionChecks...)
		switch result.kind {
		case VerificationPassFact, VerificationPassRealized:
			verification.FactChecks = append(verification.FactChecks, result.factChecks...)
		case VerificationPassExplicitCondition:
			verification.ExplicitConditionChecks = append(verification.ExplicitConditionChecks, result.explicitConditionChecks...)
		case VerificationPassImplicitCondition:
			verification.ImplicitConditionChecks = append(verification.ImplicitConditionChecks, result.implicitConditionChecks...)
		case VerificationPassPrediction:
			verification.PredictionChecks = append(verification.PredictionChecks, result.predictionChecks...)
		}
		verification.Passes = append(verification.Passes, VerificationPass{
			Kind:             result.kind,
			NodeIDs:          CloneStrings(result.nodeIDs),
			Coverage:         result.coverage,
			RetrievalSummary: result.retrievalSummary,
			Claim:            cloneStageSummary(result.claim),
			Challenge:        cloneStageSummary(result.challenge),
			Adjudication:     cloneStageSummary(result.adjudication),
		})
		if result.adjudication != nil && result.adjudication.CompletedAt.After(verification.VerifiedAt) {
			verification.VerifiedAt = result.adjudication.CompletedAt
		}
	}
	verification.Model = uniformAdjudicationModel(passResults)
	return verification, nil
}

func verifyRealized(ctx context.Context, rt verifierCall, model string, prompts *promptRegistry, bundle Bundle, nodes []GraphNode) (verifierPassResult, error) {
	retrievalContext, retrievalSummary, err := buildFactRetrievalPayload(ctx, bundle, nodes)
	if err != nil {
		return verifierPassResult{}, err
	}
	claimPrompt, err := buildFactVerificationPrompt(bundle, nodes, retrievalContext, retrievalSummary, nil)
	if err != nil {
		return verifierPassResult{}, err
	}
	var claimPayload struct {
		FactChecks []FactCheck `json:"fact_checks"`
	}
	claimResp, claimCompletedAt, err := runVerifierPromptStage(ctx, rt, model, prompts, bundle, promptFactVerifierClaim, claimPrompt, "fact claim", &claimPayload)
	if err != nil {
		return verifierPassResult{}, err
	}
	claimSummary := verifierStageSummary(claimResp.Model, model, claimCompletedAt, factCheckNodeIDs(claimPayload.FactChecks))

	challengePrompt, err := buildFactVerificationPrompt(bundle, nodes, retrievalContext, retrievalSummary, map[string]any{
		"claim_checks": claimPayload.FactChecks,
	})
	if err != nil {
		return verifierPassResult{}, err
	}
	var challengePayload struct {
		Challenges []factChallenge `json:"challenges"`
	}
	challengeResp, challengeCompletedAt, err := runVerifierPromptStage(ctx, rt, model, prompts, bundle, promptFactVerifierChallenge, challengePrompt, "fact challenge", &challengePayload)
	if err != nil {
		return verifierPassResult{}, err
	}
	challengeSummary := verifierStageSummary(challengeResp.Model, model, challengeCompletedAt, factChallengeNodeIDs(challengePayload.Challenges))

	adjudicationPrompt, err := buildFactVerificationPrompt(bundle, nodes, retrievalContext, retrievalSummary, map[string]any{
		"claim_checks": claimPayload.FactChecks,
		"challenges":   challengePayload.Challenges,
	})
	if err != nil {
		return verifierPassResult{}, err
	}
	var adjudicationPayload struct {
		FactChecks []FactCheck `json:"fact_checks"`
	}
	adjudicationResp, adjudicationCompletedAt, err := runVerifierPromptStage(ctx, rt, model, prompts, bundle, promptFactVerifierAdjudication, adjudicationPrompt, "fact adjudication", &adjudicationPayload)
	if err != nil {
		return verifierPassResult{}, err
	}

	finalNodeIDs := factCheckNodeIDs(adjudicationPayload.FactChecks)
	compatibilityFactChecks := filterCompatibilityFactChecks(nodes, adjudicationPayload.FactChecks)
	return verifierPassResult{
		kind:             VerificationPassFact,
		nodeIDs:          nodeIDs(nodes),
		realizedChecks:   toRealizedChecks(adjudicationPayload.FactChecks),
		factChecks:       compatibilityFactChecks,
		claim:            claimSummary,
		challenge:        challengeSummary,
		adjudication:     verifierStageSummary(adjudicationResp.Model, model, adjudicationCompletedAt, finalNodeIDs),
		coverage:         buildCoverage(nodeIDs(nodes), finalNodeIDs),
		retrievalSummary: retrievalSummary,
		debateEnabled:    true,
	}, nil
}

func verifyPredictions(ctx context.Context, rt verifierCall, model string, prompts *promptRegistry, bundle Bundle, nodes []GraphNode) (verifierPassResult, error) {
	prompt, err := buildPredictionVerificationPrompt(bundle, nodes)
	if err != nil {
		return verifierPassResult{}, err
	}
	var payload struct {
		PredictionChecks []PredictionCheck `json:"prediction_checks"`
	}
	resp, completedAt, err := runVerifierPromptStage(ctx, rt, model, prompts, bundle, promptPredictionVerifier, prompt, "prediction verifier", &payload)
	if err != nil {
		return verifierPassResult{}, err
	}
	finalNodeIDs := predictionCheckNodeIDs(payload.PredictionChecks)
	stage := verifierStageSummary(resp.Model, model, completedAt, finalNodeIDs)
	return verifierPassResult{
		kind:                  VerificationPassPrediction,
		nodeIDs:               nodeIDs(nodes),
		futureConditionChecks: toFutureConditionChecksFromPredictions(payload.PredictionChecks),
		predictionChecks:      payload.PredictionChecks,
		adjudication:          stage,
		coverage:              buildCoverage(nodeIDs(nodes), finalNodeIDs),
	}, nil
}

func verifyExplicitConditions(ctx context.Context, rt verifierCall, model string, prompts *promptRegistry, bundle Bundle, nodes []GraphNode) (verifierPassResult, error) {
	prompt, err := buildExplicitConditionVerificationPrompt(bundle, nodes)
	if err != nil {
		return verifierPassResult{}, err
	}
	var payload struct {
		ExplicitConditionChecks []ExplicitConditionCheck `json:"explicit_condition_checks"`
	}
	resp, completedAt, err := runVerifierPromptStage(ctx, rt, model, prompts, bundle, promptExplicitConditionVerifier, prompt, "explicit condition verifier", &payload)
	if err != nil {
		return verifierPassResult{}, err
	}
	finalNodeIDs := explicitConditionCheckNodeIDs(payload.ExplicitConditionChecks)
	stage := verifierStageSummary(resp.Model, model, completedAt, finalNodeIDs)
	return verifierPassResult{
		kind:                    VerificationPassExplicitCondition,
		nodeIDs:                 nodeIDs(nodes),
		futureConditionChecks:   toFutureConditionChecksFromExplicit(payload.ExplicitConditionChecks),
		explicitConditionChecks: payload.ExplicitConditionChecks,
		claim:                   cloneStageSummary(stage),
		adjudication:            stage,
		coverage:                buildCoverage(nodeIDs(nodes), finalNodeIDs),
	}, nil
}

func verifyImplicitConditions(ctx context.Context, rt verifierCall, model string, prompts *promptRegistry, bundle Bundle, nodes []GraphNode) (verifierPassResult, error) {
	prompt, err := buildImplicitConditionVerificationPrompt(bundle, nodes)
	if err != nil {
		return verifierPassResult{}, err
	}
	var payload struct {
		ImplicitConditionChecks []ImplicitConditionCheck `json:"implicit_condition_checks"`
	}
	resp, completedAt, err := runVerifierPromptStage(ctx, rt, model, prompts, bundle, promptImplicitConditionVerifier, prompt, "implicit condition verifier", &payload)
	if err != nil {
		return verifierPassResult{}, err
	}
	finalNodeIDs := implicitConditionCheckNodeIDs(payload.ImplicitConditionChecks)
	stage := verifierStageSummary(resp.Model, model, completedAt, finalNodeIDs)
	return verifierPassResult{
		kind:                    VerificationPassImplicitCondition,
		nodeIDs:                 nodeIDs(nodes),
		futureConditionChecks:   toFutureConditionChecksFromImplicit(payload.ImplicitConditionChecks),
		implicitConditionChecks: payload.ImplicitConditionChecks,
		claim:                   cloneStageSummary(stage),
		adjudication:            stage,
		coverage:                buildCoverage(nodeIDs(nodes), finalNodeIDs),
	}, nil
}
