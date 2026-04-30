package compilev2

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/kumaloha/VariX/varix/compile"
	"github.com/kumaloha/forge/llm"
	"strings"
	"time"
)

const authorValidationVersion = "author_validate_v1"

type authorClaimCandidate struct {
	ClaimID     string `json:"claim_id"`
	Kind        string `json:"kind"`
	Text        string `json:"text"`
	Branch      string `json:"branch,omitempty"`
	SourceText  string `json:"source_text,omitempty"`
	SourceQuote string `json:"source_quote,omitempty"`
	Role        string `json:"role,omitempty"`
	AttachesTo  string `json:"attaches_to,omitempty"`
	Context     string `json:"context,omitempty"`
}

type authorInferenceCandidate struct {
	InferenceID  string                    `json:"inference_id"`
	From         string                    `json:"from"`
	To           string                    `json:"to"`
	Steps        []string                  `json:"steps,omitempty"`
	Branch       string                    `json:"branch,omitempty"`
	SourceQuote  string                    `json:"source_quote,omitempty"`
	Context      string                    `json:"context,omitempty"`
	EdgeEvidence []authorInferenceEvidence `json:"edge_evidence,omitempty"`
}

type authorInferenceEvidence struct {
	From        string `json:"from,omitempty"`
	To          string `json:"to,omitempty"`
	FromText    string `json:"from_text,omitempty"`
	ToText      string `json:"to_text,omitempty"`
	SourceQuote string `json:"source_quote,omitempty"`
	Reason      string `json:"reason,omitempty"`
}

type authorVerificationPlan = compile.AuthorVerificationPlan

type authorClaimVerificationPlan = compile.AuthorClaimVerificationPlan

type authorInferenceVerificationPlan = compile.AuthorInferenceVerificationPlan

type authorExternalEvidenceHint struct {
	ClaimID string                         `json:"claim_id,omitempty"`
	Query   string                         `json:"query,omitempty"`
	Results []authorExternalEvidenceResult `json:"results,omitempty"`
}

type authorExternalEvidenceResult struct {
	URL     string `json:"url,omitempty"`
	Title   string `json:"title,omitempty"`
	Excerpt string `json:"excerpt,omitempty"`
}

var buildAuthorExternalEvidenceHints = defaultAuthorExternalEvidenceHints

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
	renderForValidation := enrichAuthorValidationRenderDetails(result)
	claims := collectAuthorClaimCandidates(renderForValidation)
	inferences := collectAuthorInferenceCandidates(renderForValidation)
	validation := compile.AuthorValidation{
		ValidatedAt: compile.NowUTC(),
		Model:       strings.TrimSpace(model),
		Version:     authorValidationVersion,
	}
	if len(claims) == 0 && len(inferences) == 0 {
		validation.Summary.Verdict = "insufficient_evidence"
		return validation, nil
	}
	plan, err := runAuthorVerificationPlan(ctx, rt, model, bundle, result, claims, inferences)
	if err != nil {
		return compile.AuthorValidation{}, fmt.Errorf("plan: %w", err)
	}
	externalEvidenceHints, _ := buildAuthorExternalEvidenceHints(ctx, claims, plan)

	payload := map[string]any{
		"task": "Validate only the rendered author nodes, rendered evidence points, and rendered reasoning paths. Do not audit the extraction graph, missing nodes, missing edges, or classification.",
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
		"current_date":         compile.NowUTC().Format("2006-01-02"),
		"article_form":         result.ArticleForm,
		"text_context":         bundle.TextContext(),
		"render_summary":       result.Render.Summary,
		"claim_candidates":     claims,
		"inference_candidates": inferences,
		"verification_plan":    plan,
	}
	if len(externalEvidenceHints) > 0 {
		payload["external_evidence_hints"] = externalEvidenceHints
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
	var parsed compile.AuthorValidation
	if err := callAuthorValidationJSON(ctx, rt, req, &parsed); err != nil {
		return compile.AuthorValidation{}, fmt.Errorf("parse author validation output: %w", err)
	}
	normalized := normalizeAuthorValidationWithHints(parsed, claims, inferences, model, externalEvidenceHints)
	normalized.VerificationPlan = plan
	return normalized, nil
}

func runAuthorVerificationPlan(ctx context.Context, rt runtimeChat, model string, bundle compile.Bundle, result FlowPreviewResult, claims []authorClaimCandidate, inferences []authorInferenceCandidate) (authorVerificationPlan, error) {
	payload := map[string]any{
		"task":                 "Create a verification plan only. Decide what needs validation and what exact data/source/value would validate it. Do not judge truth.",
		"unit_id":              bundle.UnitID,
		"source":               bundle.Source,
		"external_id":          bundle.ExternalID,
		"url":                  bundle.URL,
		"current_date":         compile.NowUTC().Format("2006-01-02"),
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
		return authorVerificationPlan{}, err
	}
	req, err := compile.BuildProviderRequest(model, bundle, authorValidationPlanSystemPrompt(), string(encoded), false)
	if err != nil {
		return authorVerificationPlan{}, err
	}
	var plan authorVerificationPlan
	if err := callAuthorValidationJSON(ctx, rt, req, &plan); err != nil {
		return authorVerificationPlan{}, fmt.Errorf("parse author validation plan output: %w", err)
	}
	return normalizeAuthorVerificationPlan(plan), nil
}

func callAuthorValidationJSON(ctx context.Context, rt runtimeChat, req llm.ProviderRequest, target any) error {
	resp, err := rt.Call(ctx, req)
	if err != nil {
		return err
	}
	if err := parseJSONObject(resp.Text, target); err != nil {
		firstErr := err
		retryReq := req
		retryReq.System = strings.TrimSpace(req.System + "\n\nThe previous response was invalid JSON. Retry once and return only strict JSON matching the schema. Do not include markdown, comments, duplicate object separators, or trailing text.")
		resp, err = rt.Call(ctx, retryReq)
		if err != nil {
			return err
		}
		if err := parseJSONObject(resp.Text, target); err != nil {
			return fmt.Errorf("%w; retry parse: %v", firstErr, err)
		}
	}
	return nil
}
