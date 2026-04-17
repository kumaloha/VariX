package compile

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/kumaloha/forge/llm"
)

type verifierCall = runtimeChat

var buildFactRetrievalContext = func(ctx context.Context, bundle Bundle, nodes []GraphNode) ([]map[string]any, error) {
	return nil, nil
}

var verifierNow = func() time.Time {
	return time.Now().UTC()
}

type verifierPassResult struct {
	kind                    VerificationPassKind
	nodeIDs                 []string
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

func runVerifier(ctx context.Context, rt verifierCall, model string, prompts *promptRegistry, bundle Bundle, output Output) (Verification, error) {
	if prompts == nil {
		prompts = newPromptRegistry("")
	}
	var passResults []verifierPassResult

	factNodes := make([]GraphNode, 0)
	explicitConditionNodes := make([]GraphNode, 0)
	implicitConditionNodes := make([]GraphNode, 0)
	predictionNodes := make([]GraphNode, 0)
	for _, node := range output.Graph.Nodes {
		switch node.Kind {
		case NodeFact:
			factNodes = append(factNodes, node)
		case NodeExplicitCondition:
			explicitConditionNodes = append(explicitConditionNodes, node)
		case NodeImplicitCondition:
			implicitConditionNodes = append(implicitConditionNodes, node)
		case NodePrediction:
			predictionNodes = append(predictionNodes, node)
		}
	}

	if len(factNodes) > 0 {
		result, err := verifyFacts(ctx, rt, model, prompts, bundle, factNodes)
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
	if len(predictionNodes) > 0 {
		result, err := verifyPredictions(ctx, rt, model, prompts, bundle, predictionNodes)
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
		Version:         "verify_v2",
		RolloutStage:    verifierRolloutStage(passResults),
		Passes:          make([]VerificationPass, 0, len(passResults)),
		CoverageSummary: aggregateCoverageSummary(passResults),
	}
	for _, result := range passResults {
		switch result.kind {
		case VerificationPassFact:
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
			NodeIDs:          cloneStrings(result.nodeIDs),
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

func verifyFacts(ctx context.Context, rt verifierCall, model string, prompts *promptRegistry, bundle Bundle, nodes []GraphNode) (verifierPassResult, error) {
	retrievalContext, retrievalSummary, err := buildFactRetrievalPayload(ctx, bundle, nodes)
	if err != nil {
		return verifierPassResult{}, err
	}
	claimInstruction, err := prompts.verifierInstruction(promptFactVerifierClaim)
	if err != nil {
		return verifierPassResult{}, err
	}

	claimPrompt, err := buildFactVerificationPrompt(bundle, nodes, retrievalContext, retrievalSummary, nil)
	if err != nil {
		return verifierPassResult{}, err
	}
	claimReq, err := BuildQwen36ProviderRequest(model, bundle, claimInstruction, claimPrompt)
	if err != nil {
		return verifierPassResult{}, err
	}
	claimResp, claimCompletedAt, err := callVerifierStage(ctx, rt, claimReq)
	if err != nil {
		return verifierPassResult{}, err
	}
	var claimPayload struct {
		FactChecks []FactCheck `json:"fact_checks"`
	}
	if err := unmarshalVerifierPayload(claimResp.Text, &claimPayload); err != nil {
		return verifierPassResult{}, fmt.Errorf("parse fact claim output: %w", err)
	}
	claimSummary := newStageSummary(firstNonEmpty(claimResp.Model, model), claimCompletedAt, true, factCheckNodeIDs(claimPayload.FactChecks))

	challengeInstruction, err := prompts.verifierInstruction(promptFactVerifierChallenge)
	if err != nil {
		return verifierPassResult{}, err
	}
	challengePrompt, err := buildFactVerificationPrompt(bundle, nodes, retrievalContext, retrievalSummary, map[string]any{
		"claim_checks": claimPayload.FactChecks,
	})
	if err != nil {
		return verifierPassResult{}, err
	}
	challengeReq, err := BuildQwen36ProviderRequest(model, bundle, challengeInstruction, challengePrompt)
	if err != nil {
		return verifierPassResult{}, err
	}
	challengeResp, challengeCompletedAt, err := callVerifierStage(ctx, rt, challengeReq)
	if err != nil {
		return verifierPassResult{}, err
	}
	var challengePayload struct {
		Challenges []factChallenge `json:"challenges"`
	}
	if err := unmarshalVerifierPayload(challengeResp.Text, &challengePayload); err != nil {
		return verifierPassResult{}, fmt.Errorf("parse fact challenge output: %w", err)
	}
	challengeSummary := newStageSummary(firstNonEmpty(challengeResp.Model, model), challengeCompletedAt, true, factChallengeNodeIDs(challengePayload.Challenges))

	adjudicationInstruction, err := prompts.verifierInstruction(promptFactVerifierAdjudication)
	if err != nil {
		return verifierPassResult{}, err
	}
	adjudicationPrompt, err := buildFactVerificationPrompt(bundle, nodes, retrievalContext, retrievalSummary, map[string]any{
		"claim_checks": claimPayload.FactChecks,
		"challenges":   challengePayload.Challenges,
	})
	if err != nil {
		return verifierPassResult{}, err
	}
	adjudicationReq, err := BuildQwen36ProviderRequest(model, bundle, adjudicationInstruction, adjudicationPrompt)
	if err != nil {
		return verifierPassResult{}, err
	}
	adjudicationResp, adjudicationCompletedAt, err := callVerifierStage(ctx, rt, adjudicationReq)
	if err != nil {
		return verifierPassResult{}, err
	}
	var adjudicationPayload struct {
		FactChecks []FactCheck `json:"fact_checks"`
	}
	if err := unmarshalVerifierPayload(adjudicationResp.Text, &adjudicationPayload); err != nil {
		return verifierPassResult{}, fmt.Errorf("parse fact adjudication output: %w", err)
	}

	finalNodeIDs := factCheckNodeIDs(adjudicationPayload.FactChecks)
	return verifierPassResult{
		kind:             VerificationPassFact,
		nodeIDs:          nodeIDs(nodes),
		factChecks:       adjudicationPayload.FactChecks,
		claim:            claimSummary,
		challenge:        challengeSummary,
		adjudication:     newStageSummary(firstNonEmpty(adjudicationResp.Model, model), adjudicationCompletedAt, true, finalNodeIDs),
		coverage:         buildCoverage(nodeIDs(nodes), finalNodeIDs),
		retrievalSummary: retrievalSummary,
		debateEnabled:    true,
	}, nil
}

func verifyPredictions(ctx context.Context, rt verifierCall, model string, prompts *promptRegistry, bundle Bundle, nodes []GraphNode) (verifierPassResult, error) {
	instruction, err := prompts.verifierInstruction(promptPredictionVerifier)
	if err != nil {
		return verifierPassResult{}, err
	}
	prompt, err := buildPredictionVerificationPrompt(bundle, nodes)
	if err != nil {
		return verifierPassResult{}, err
	}
	req, err := BuildQwen36ProviderRequest(model, bundle, instruction, prompt)
	if err != nil {
		return verifierPassResult{}, err
	}
	resp, completedAt, err := callVerifierStage(ctx, rt, req)
	if err != nil {
		return verifierPassResult{}, err
	}
	var payload struct {
		PredictionChecks []PredictionCheck `json:"prediction_checks"`
	}
	if err := unmarshalVerifierPayload(resp.Text, &payload); err != nil {
		return verifierPassResult{}, fmt.Errorf("parse prediction verifier output: %w", err)
	}
	finalNodeIDs := predictionCheckNodeIDs(payload.PredictionChecks)
	stage := newStageSummary(firstNonEmpty(resp.Model, model), completedAt, true, finalNodeIDs)
	return verifierPassResult{
		kind:             VerificationPassPrediction,
		nodeIDs:          nodeIDs(nodes),
		predictionChecks: payload.PredictionChecks,
		adjudication:     stage,
		coverage:         buildCoverage(nodeIDs(nodes), finalNodeIDs),
	}, nil
}

func verifyExplicitConditions(ctx context.Context, rt verifierCall, model string, prompts *promptRegistry, bundle Bundle, nodes []GraphNode) (verifierPassResult, error) {
	instruction, err := prompts.verifierInstruction(promptExplicitConditionVerifier)
	if err != nil {
		return verifierPassResult{}, err
	}
	prompt, err := buildExplicitConditionVerificationPrompt(bundle, nodes)
	if err != nil {
		return verifierPassResult{}, err
	}
	req, err := BuildQwen36ProviderRequest(model, bundle, instruction, prompt)
	if err != nil {
		return verifierPassResult{}, err
	}
	resp, completedAt, err := callVerifierStage(ctx, rt, req)
	if err != nil {
		return verifierPassResult{}, err
	}
	var payload struct {
		ExplicitConditionChecks []ExplicitConditionCheck `json:"explicit_condition_checks"`
	}
	if err := unmarshalVerifierPayload(resp.Text, &payload); err != nil {
		return verifierPassResult{}, fmt.Errorf("parse explicit condition verifier output: %w", err)
	}
	finalNodeIDs := explicitConditionCheckNodeIDs(payload.ExplicitConditionChecks)
	stage := newStageSummary(firstNonEmpty(resp.Model, model), completedAt, true, finalNodeIDs)
	return verifierPassResult{
		kind:                    VerificationPassExplicitCondition,
		nodeIDs:                 nodeIDs(nodes),
		explicitConditionChecks: payload.ExplicitConditionChecks,
		claim:                   cloneStageSummary(stage),
		adjudication:            stage,
		coverage:                buildCoverage(nodeIDs(nodes), finalNodeIDs),
	}, nil
}

func verifyImplicitConditions(ctx context.Context, rt verifierCall, model string, prompts *promptRegistry, bundle Bundle, nodes []GraphNode) (verifierPassResult, error) {
	instruction, err := prompts.verifierInstruction(promptImplicitConditionVerifier)
	if err != nil {
		return verifierPassResult{}, err
	}
	prompt, err := buildImplicitConditionVerificationPrompt(bundle, nodes)
	if err != nil {
		return verifierPassResult{}, err
	}
	req, err := BuildQwen36ProviderRequest(model, bundle, instruction, prompt)
	if err != nil {
		return verifierPassResult{}, err
	}
	resp, completedAt, err := callVerifierStage(ctx, rt, req)
	if err != nil {
		return verifierPassResult{}, err
	}
	var payload struct {
		ImplicitConditionChecks []ImplicitConditionCheck `json:"implicit_condition_checks"`
	}
	if err := unmarshalVerifierPayload(resp.Text, &payload); err != nil {
		return verifierPassResult{}, fmt.Errorf("parse implicit condition verifier output: %w", err)
	}
	finalNodeIDs := implicitConditionCheckNodeIDs(payload.ImplicitConditionChecks)
	stage := newStageSummary(firstNonEmpty(resp.Model, model), completedAt, true, finalNodeIDs)
	return verifierPassResult{
		kind:                    VerificationPassImplicitCondition,
		nodeIDs:                 nodeIDs(nodes),
		implicitConditionChecks: payload.ImplicitConditionChecks,
		claim:                   cloneStageSummary(stage),
		adjudication:            stage,
		coverage:                buildCoverage(nodeIDs(nodes), finalNodeIDs),
	}, nil
}

func callVerifierStage(ctx context.Context, rt verifierCall, req llm.ProviderRequest) (llm.Response, time.Time, error) {
	resp, err := rt.Call(ctx, req)
	return resp, verifierNow(), err
}

func unmarshalVerifierPayload(raw string, target any) error {
	clean := strings.TrimSpace(raw)
	clean = strings.TrimPrefix(clean, "```json")
	clean = strings.TrimPrefix(clean, "```")
	clean = strings.TrimSuffix(clean, "```")
	clean = strings.TrimSpace(clean)
	if err := json.Unmarshal([]byte(clean), target); err != nil {
		return fmt.Errorf("parse verifier output: %w", err)
	}
	return nil
}

func buildFactVerificationPrompt(bundle Bundle, nodes []GraphNode, retrievalContext []map[string]any, retrievalSummary *VerificationRetrievalSummary, extra map[string]any) (string, error) {
	payload := map[string]any{
		"as_of": verifierNow().Format(time.RFC3339),
	}
	if len(retrievalContext) > 0 {
		payload["retrieval_context"] = retrievalContext
	}
	if retrievalSummary != nil {
		payload["retrieval_summary"] = retrievalSummary
	}
	for key, value := range extra {
		payload[key] = value
	}
	return buildVerificationPrompt(bundle, nodes, payload)
}

func buildPredictionVerificationPrompt(bundle Bundle, nodes []GraphNode) (string, error) {
	return buildVerificationPrompt(bundle, nodes, map[string]any{
		"as_of": verifierNow().Format(time.RFC3339),
	})
}

func buildExplicitConditionVerificationPrompt(bundle Bundle, nodes []GraphNode) (string, error) {
	return buildVerificationPrompt(bundle, nodes, map[string]any{
		"as_of": verifierNow().Format(time.RFC3339),
	})
}

func buildImplicitConditionVerificationPrompt(bundle Bundle, nodes []GraphNode) (string, error) {
	return buildVerificationPrompt(bundle, nodes, map[string]any{
		"as_of": verifierNow().Format(time.RFC3339),
	})
}

func buildVerificationPrompt(bundle Bundle, nodes []GraphNode, extra map[string]any) (string, error) {
	payload := map[string]any{
		"unit_id":         bundle.UnitID,
		"source":          bundle.Source,
		"external_id":     bundle.ExternalID,
		"nodes":           marshalVerificationNodes(nodes),
		"quotes":          bundle.Quotes,
		"references":      bundle.References,
		"thread_segments": bundle.ThreadSegments,
		"attachments":     bundle.Attachments,
		"text_context":    bundle.TextContext(),
	}
	if trimmed := strings.TrimSpace(bundle.RootExternalID); trimmed != "" {
		payload["root_external_id"] = trimmed
	}
	if trimmed := strings.TrimSpace(bundle.AuthorName); trimmed != "" {
		payload["author_name"] = trimmed
	}
	if trimmed := strings.TrimSpace(bundle.AuthorID); trimmed != "" {
		payload["author_id"] = trimmed
	}
	if trimmed := strings.TrimSpace(bundle.URL); trimmed != "" {
		payload["url"] = trimmed
	}
	if !bundle.PostedAt.IsZero() {
		payload["posted_at"] = bundle.PostedAt.Format(time.RFC3339)
	}
	for key, value := range extra {
		payload[key] = value
	}
	encoded, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return "", err
	}
	return string(encoded), nil
}

func marshalVerificationNodes(nodes []GraphNode) []map[string]any {
	out := make([]map[string]any, 0, len(nodes))
	for _, node := range nodes {
		item := map[string]any{
			"id":   node.ID,
			"kind": node.Kind,
			"text": node.Text,
		}
		if !node.OccurredAt.IsZero() {
			item["occurred_at"] = node.OccurredAt.Format(time.RFC3339)
		}
		if !node.PredictionStartAt.IsZero() {
			item["prediction_start_at"] = node.PredictionStartAt.Format(time.RFC3339)
		}
		if !node.PredictionDueAt.IsZero() {
			item["prediction_due_at"] = node.PredictionDueAt.Format(time.RFC3339)
		}
		if !node.ValidFrom.IsZero() && !node.ValidTo.IsZero() && node.OccurredAt.IsZero() && node.PredictionStartAt.IsZero() {
			item["valid_from"] = node.ValidFrom.Format(time.RFC3339)
			item["valid_to"] = node.ValidTo.Format(time.RFC3339)
		}
		out = append(out, item)
	}
	return out
}

func buildFactRetrievalPayload(ctx context.Context, bundle Bundle, nodes []GraphNode) ([]map[string]any, *VerificationRetrievalSummary, error) {
	retrieval, err := buildFactRetrievalContext(ctx, bundle, nodes)
	if err != nil {
		return nil, nil, err
	}
	summary := &VerificationRetrievalSummary{
		RetrievedNodeIDs:     make([]string, 0, len(retrieval)),
		NoResultNodeIDs:      make([]string, 0, minInt(len(nodes), maxFactRetrievalNodes)),
		BudgetLimitedNodeIDs: cloneStrings(nodeIDs(nodes[minInt(len(nodes), maxFactRetrievalNodes):])),
		PromptContextReduced: len(nodes) > maxFactRetrievalNodes,
	}
	seen := make(map[string]struct{}, len(retrieval))
	for _, item := range retrieval {
		nodeID := strings.TrimSpace(asString(item["node_id"]))
		if nodeID == "" {
			continue
		}
		seen[nodeID] = struct{}{}
		summary.RetrievedNodeIDs = append(summary.RetrievedNodeIDs, nodeID)
		if truthy(item["results_limited"]) {
			summary.PromptContextReduced = true
		}
		if truthy(item["excerpt_truncated"]) {
			summary.ExcerptTruncated = true
		}
	}
	for _, node := range nodes {
		if _, ok := seen[node.ID]; ok {
			continue
		}
		summary.NoResultNodeIDs = append(summary.NoResultNodeIDs, node.ID)
	}
	if len(summary.RetrievedNodeIDs) == 0 && len(summary.NoResultNodeIDs) == 0 && !summary.PromptContextReduced && !summary.ExcerptTruncated {
		return retrieval, nil, nil
	}
	return retrieval, summary, nil
}

func buildCoverage(expectedNodeIDs, returnedNodeIDs []string) VerificationPassCoverage {
	coverage := VerificationPassCoverage{
		ExpectedNodeIDs: cloneStrings(expectedNodeIDs),
		ReturnedNodeIDs: cloneStrings(returnedNodeIDs),
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
	for _, result := range results {
		if result.kind == VerificationPassFact && result.debateEnabled {
			return "facts_only"
		}
	}
	return "coverage_only"
}

func newStageSummary(model string, completedAt time.Time, parseOK bool, outputNodeIDs []string) *VerificationStageSummary {
	return &VerificationStageSummary{
		Model:         strings.TrimSpace(model),
		CompletedAt:   completedAt.UTC(),
		ParseOK:       parseOK,
		OutputNodeIDs: cloneStrings(outputNodeIDs),
	}
}

func cloneStageSummary(in *VerificationStageSummary) *VerificationStageSummary {
	if in == nil {
		return nil
	}
	out := *in
	out.OutputNodeIDs = cloneStrings(in.OutputNodeIDs)
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

func cloneStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	out := make([]string, len(values))
	copy(out, values)
	return out
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func asString(value any) string {
	switch v := value.(type) {
	case string:
		return v
	default:
		return ""
	}
}

func truthy(value any) bool {
	if v, ok := value.(bool); ok {
		return v
	}
	return false
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

const (
	promptFactVerifierClaim         = "fact_claim"
	promptFactVerifierChallenge     = "fact_challenge"
	promptFactVerifierAdjudication  = "fact_adjudicate"
	promptPredictionVerifier        = "prediction"
	promptExplicitConditionVerifier = "explicit_condition"
	promptImplicitConditionVerifier = "implicit_condition"
)
