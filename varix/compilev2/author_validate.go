package compilev2

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/kumaloha/VariX/varix/compile"
)

const authorValidationVersion = "author_validate_v1"

type authorClaimCandidate struct {
	ClaimID string `json:"claim_id"`
	Kind    string `json:"kind"`
	Text    string `json:"text"`
	Branch  string `json:"branch,omitempty"`
}

type authorInferenceCandidate struct {
	InferenceID string   `json:"inference_id"`
	From        string   `json:"from"`
	To          string   `json:"to"`
	Steps       []string `json:"steps,omitempty"`
	Branch      string   `json:"branch,omitempty"`
}

func (c *Client) AuthorValidatePreviewResult(ctx context.Context, bundle compile.Bundle, result FlowPreviewResult) (FlowPreviewResult, error) {
	if c == nil || c.runtime == nil {
		return FlowPreviewResult{}, fmt.Errorf("compile v2 client is nil")
	}
	if result.Metrics == nil {
		result.Metrics = map[string]int64{}
	}
	start := time.Now()
	validation, err := runAuthorValidation(ctx, c.runtime, c.model, bundle, result)
	if err != nil {
		return FlowPreviewResult{}, fmt.Errorf("author validate: %w", err)
	}
	result.AuthorValidation = &validation
	result.Render.AuthorValidation = validation
	result.Metrics["author_validate_ms"] = time.Since(start).Milliseconds()
	return result, nil
}

func runAuthorValidation(ctx context.Context, rt runtimeChat, model string, bundle compile.Bundle, result FlowPreviewResult) (compile.AuthorValidation, error) {
	claims := collectAuthorClaimCandidates(result.Render)
	inferences := collectAuthorInferenceCandidates(result.Render)
	validation := compile.AuthorValidation{
		ValidatedAt: compile.NowUTC(),
		Model:       strings.TrimSpace(model),
		Version:     authorValidationVersion,
	}
	if len(claims) == 0 && len(inferences) == 0 {
		validation.Summary.Verdict = "insufficient_evidence"
		return validation, nil
	}

	payload := map[string]any{
		"task": "Validate only the author's claims and reasoning. Do not audit the extraction graph, missing nodes, missing edges, or classification.",
		"status_contract": map[string]any{
			"claim_status": []string{
				string(compile.AuthorClaimSupported),
				string(compile.AuthorClaimContradicted),
				string(compile.AuthorClaimUnverified),
				string(compile.AuthorClaimInterpretive),
				string(compile.AuthorClaimNotAuthorClaim),
			},
			"inference_status": []string{
				string(compile.AuthorInferenceSound),
				string(compile.AuthorInferenceWeak),
				string(compile.AuthorInferenceUnsupportedJump),
				string(compile.AuthorInferenceNotAuthorInference),
			},
		},
		"unit_id":              bundle.UnitID,
		"source":               bundle.Source,
		"external_id":          bundle.ExternalID,
		"url":                  bundle.URL,
		"article_form":         result.ArticleForm,
		"text_context":         bundle.TextContext(),
		"render_summary":       result.Render.Summary,
		"claim_candidates":     claims,
		"inference_candidates": inferences,
	}
	if !bundle.PostedAt.IsZero() {
		payload["posted_at"] = bundle.PostedAt.Format(time.RFC3339)
	}
	encoded, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return compile.AuthorValidation{}, err
	}
	req, err := compile.BuildProviderRequest(model, bundle, authorValidationSystemPrompt(), string(encoded), true)
	if err != nil {
		return compile.AuthorValidation{}, err
	}
	resp, err := rt.Call(ctx, req)
	if err != nil {
		return compile.AuthorValidation{}, err
	}
	var parsed compile.AuthorValidation
	if err := parseJSONObject(resp.Text, &parsed); err != nil {
		return compile.AuthorValidation{}, fmt.Errorf("parse author validation output: %w", err)
	}
	return normalizeAuthorValidation(parsed, claims, inferences, model), nil
}

func authorValidationSystemPrompt() string {
	return strings.TrimSpace(`
You are an author-claim validator for a reader-facing product.

Validate ONLY what the author claims or implies. Do not critique the extraction pipeline, graph shape, missing nodes, missing edges, target classification, branch grouping, or UI wording.

For each claim candidate:
- If the source text does not show the author making the claim, use status "not_author_claim". This is not an author fault.
- If it is an objective factual/numeric claim and available evidence supports it, use "supported".
- If available evidence contradicts it, use "contradicted".
- If it is objective but cannot be verified from the source/search context, use "unverified".
- If it is opinion, interpretation, analogy, or unresolved forecast, use "interpretive" unless it contains a checkable factual subclaim.

For each inference candidate:
- Judge whether the author actually makes that inferential jump and whether the stated premises support it.
- Use "sound", "weak", "unsupported_jump", or "not_author_inference".

Return strict JSON only:
{
  "summary": {
    "verdict": "credible|mixed|high_risk|insufficient_evidence",
    "supported_claims": 0,
    "contradicted_claims": 0,
    "unverified_claims": 0,
    "interpretive_claims": 0,
    "not_author_claims": 0,
    "sound_inferences": 0,
    "weak_inferences": 0,
    "unsupported_inferences": 0,
    "not_author_inferences": 0
  },
  "claim_checks": [
    {"claim_id":"...", "text":"...", "claim_type":"fact|number|forecast|interpretation|opinion", "status":"supported|contradicted|unverified|interpretive|not_author_claim", "evidence":["short quote or source"], "reason":"brief reason"}
  ],
  "inference_checks": [
    {"inference_id":"...", "from":"...", "to":"...", "steps":["..."], "status":"sound|weak|unsupported_jump|not_author_inference", "evidence":["short quote or source"], "reason":"brief reason", "missing_links":["..."]}
  ]
}
`)
}

func collectAuthorClaimCandidates(out compile.Output) []authorClaimCandidate {
	candidates := make([]authorClaimCandidate, 0)
	seen := map[string]struct{}{}
	add := func(kind, text, branch string) {
		text = strings.TrimSpace(text)
		if text == "" {
			return
		}
		key := kind + "\x00" + branch + "\x00" + text
		if _, ok := seen[key]; ok {
			return
		}
		seen[key] = struct{}{}
		id := fmt.Sprintf("claim-%03d", len(candidates)+1)
		candidates = append(candidates, authorClaimCandidate{
			ClaimID: id,
			Kind:    kind,
			Text:    text,
			Branch:  strings.TrimSpace(branch),
		})
	}
	for _, value := range out.Drivers {
		add("driver", value, "")
	}
	for _, value := range out.Targets {
		add("target", value, "")
	}
	for _, value := range out.EvidenceNodes {
		add("evidence", value, "")
	}
	for _, value := range out.ExplanationNodes {
		add("explanation", value, "")
	}
	for _, branch := range out.Branches {
		branchID := firstTrimmed(branch.ID, branch.Thesis)
		add("branch_thesis", branch.Thesis, branchID)
		for _, value := range branch.Anchors {
			add("branch_anchor", value, branchID)
		}
		for _, value := range branch.BranchDrivers {
			add("branch_driver", value, branchID)
		}
		for _, value := range branch.Targets {
			add("branch_target", value, branchID)
		}
	}
	return candidates
}

func collectAuthorInferenceCandidates(out compile.Output) []authorInferenceCandidate {
	candidates := make([]authorInferenceCandidate, 0)
	seen := map[string]struct{}{}
	add := func(path compile.TransmissionPath, branch string) {
		from := strings.TrimSpace(path.Driver)
		to := strings.TrimSpace(path.Target)
		if from == "" || to == "" {
			return
		}
		steps := cloneStrings(path.Steps)
		key := branch + "\x00" + from + "\x00" + strings.Join(steps, "\x00") + "\x00" + to
		if _, ok := seen[key]; ok {
			return
		}
		seen[key] = struct{}{}
		id := fmt.Sprintf("inference-%03d", len(candidates)+1)
		candidates = append(candidates, authorInferenceCandidate{
			InferenceID: id,
			From:        from,
			To:          to,
			Steps:       steps,
			Branch:      strings.TrimSpace(branch),
		})
	}
	for _, path := range out.TransmissionPaths {
		add(path, "")
	}
	for _, branch := range out.Branches {
		branchID := firstTrimmed(branch.ID, branch.Thesis)
		for _, path := range branch.TransmissionPaths {
			add(path, branchID)
		}
	}
	return candidates
}

func normalizeAuthorValidation(validation compile.AuthorValidation, claims []authorClaimCandidate, inferences []authorInferenceCandidate, model string) compile.AuthorValidation {
	if validation.ValidatedAt.IsZero() {
		validation.ValidatedAt = compile.NowUTC()
	}
	if strings.TrimSpace(validation.Model) == "" {
		validation.Model = strings.TrimSpace(model)
	}
	validation.Version = authorValidationVersion

	claimByID := make(map[string]compile.AuthorClaimCheck, len(validation.ClaimChecks))
	for _, check := range validation.ClaimChecks {
		check.ClaimID = strings.TrimSpace(check.ClaimID)
		if check.ClaimID == "" {
			continue
		}
		check.Status = normalizeAuthorClaimStatus(check.Status)
		claimByID[check.ClaimID] = check
	}
	normalizedClaims := make([]compile.AuthorClaimCheck, 0, len(claims))
	for _, candidate := range claims {
		check, ok := claimByID[candidate.ClaimID]
		if !ok {
			check = compile.AuthorClaimCheck{
				ClaimID: candidate.ClaimID,
				Text:    candidate.Text,
				Status:  compile.AuthorClaimUnverified,
				Reason:  "validator did not return this candidate",
			}
		}
		if strings.TrimSpace(check.Text) == "" {
			check.Text = candidate.Text
		}
		check.Status = normalizeAuthorClaimStatus(check.Status)
		normalizedClaims = append(normalizedClaims, check)
	}
	validation.ClaimChecks = normalizedClaims

	inferenceByID := make(map[string]compile.AuthorInferenceCheck, len(validation.InferenceChecks))
	for _, check := range validation.InferenceChecks {
		check.InferenceID = strings.TrimSpace(check.InferenceID)
		if check.InferenceID == "" {
			continue
		}
		check.Status = normalizeAuthorInferenceStatus(check.Status)
		inferenceByID[check.InferenceID] = check
	}
	normalizedInferences := make([]compile.AuthorInferenceCheck, 0, len(inferences))
	for _, candidate := range inferences {
		check, ok := inferenceByID[candidate.InferenceID]
		if !ok {
			check = compile.AuthorInferenceCheck{
				InferenceID: candidate.InferenceID,
				From:        candidate.From,
				To:          candidate.To,
				Steps:       cloneStrings(candidate.Steps),
				Status:      compile.AuthorInferenceWeak,
				Reason:      "validator did not return this candidate",
			}
		}
		if strings.TrimSpace(check.From) == "" {
			check.From = candidate.From
		}
		if strings.TrimSpace(check.To) == "" {
			check.To = candidate.To
		}
		if len(check.Steps) == 0 {
			check.Steps = cloneStrings(candidate.Steps)
		}
		check.Status = normalizeAuthorInferenceStatus(check.Status)
		normalizedInferences = append(normalizedInferences, check)
	}
	validation.InferenceChecks = normalizedInferences
	validation.Summary = summarizeAuthorValidation(validation)
	return validation
}

func normalizeAuthorClaimStatus(status compile.AuthorClaimStatus) compile.AuthorClaimStatus {
	switch status {
	case compile.AuthorClaimSupported, compile.AuthorClaimContradicted, compile.AuthorClaimUnverified, compile.AuthorClaimInterpretive, compile.AuthorClaimNotAuthorClaim:
		return status
	default:
		return compile.AuthorClaimUnverified
	}
}

func normalizeAuthorInferenceStatus(status compile.AuthorInferenceStatus) compile.AuthorInferenceStatus {
	switch status {
	case compile.AuthorInferenceSound, compile.AuthorInferenceWeak, compile.AuthorInferenceUnsupportedJump, compile.AuthorInferenceNotAuthorInference:
		return status
	default:
		return compile.AuthorInferenceWeak
	}
}

func summarizeAuthorValidation(validation compile.AuthorValidation) compile.AuthorValidationSummary {
	var summary compile.AuthorValidationSummary
	for _, check := range validation.ClaimChecks {
		switch check.Status {
		case compile.AuthorClaimSupported:
			summary.SupportedClaims++
		case compile.AuthorClaimContradicted:
			summary.ContradictedClaims++
		case compile.AuthorClaimUnverified:
			summary.UnverifiedClaims++
		case compile.AuthorClaimInterpretive:
			summary.InterpretiveClaims++
		case compile.AuthorClaimNotAuthorClaim:
			summary.NotAuthorClaims++
		}
	}
	for _, check := range validation.InferenceChecks {
		switch check.Status {
		case compile.AuthorInferenceSound:
			summary.SoundInferences++
		case compile.AuthorInferenceWeak:
			summary.WeakInferences++
		case compile.AuthorInferenceUnsupportedJump:
			summary.UnsupportedInferences++
		case compile.AuthorInferenceNotAuthorInference:
			summary.NotAuthorInferences++
		}
	}
	switch {
	case len(validation.ClaimChecks) == 0 && len(validation.InferenceChecks) == 0:
		summary.Verdict = "insufficient_evidence"
	case summary.ContradictedClaims > 0 || summary.UnsupportedInferences > 0:
		summary.Verdict = "high_risk"
	case summary.UnverifiedClaims > 0 || summary.WeakInferences > 0 || summary.NotAuthorClaims > 0 || summary.NotAuthorInferences > 0:
		summary.Verdict = "mixed"
	default:
		summary.Verdict = "credible"
	}
	return summary
}

func firstTrimmed(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func cloneStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	return append([]string(nil), values...)
}
