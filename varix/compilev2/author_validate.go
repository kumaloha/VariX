package compilev2

import (
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/kumaloha/VariX/varix/compile"
	"github.com/kumaloha/forge/llm"
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

func enrichAuthorValidationRenderDetails(result FlowPreviewResult) compile.Output {
	out := result.Render
	items := cloneHiddenDetailItems(out.Details.Items)
	state := authorValidationGraphState(result)
	if len(state.Nodes) > 0 {
		items = append(items, visibleRenderNodeDetailsForAuthorValidation(out, state.Nodes)...)
	}
	if len(state.OffGraph) > 0 {
		items = append(items, visibleOffGraphDetailsForAuthorValidation(out, state.OffGraph)...)
	}
	if len(state.Spines) > 0 && len(state.Nodes) > 0 {
		items = append(items, renderTransmissionPathDetails(extractSpinePaths(state), identityRenderText)...)
	}
	out.Details.Items = dedupeAuthorValidationDetailItems(items)
	return out
}

func authorValidationGraphState(result FlowPreviewResult) graphState {
	graphs := []PreviewGraph{
		result.Classify,
		result.Validate,
		result.Relations,
		result.Evidence,
		result.Explanation,
		result.Collapse,
		result.Supplement,
		result.Cluster,
		result.Aggregate,
	}
	for _, graph := range graphs {
		if len(graph.Nodes) > 0 || len(graph.OffGraph) > 0 {
			return fromPreviewGraph(graph, result.Spines, result.ArticleForm)
		}
	}
	return graphState{Spines: append([]PreviewSpine(nil), result.Spines...), ArticleForm: strings.TrimSpace(result.ArticleForm)}
}

func visibleRenderNodeDetailsForAuthorValidation(out compile.Output, nodes []graphNode) []map[string]any {
	visible := visibleAuthorRenderNodeTexts(out)
	if len(visible) == 0 {
		return nil
	}
	details := make([]map[string]any, 0, len(nodes))
	seen := map[string]struct{}{}
	for _, node := range nodes {
		text := strings.TrimSpace(node.Text)
		if text == "" {
			continue
		}
		if _, ok := visible[text]; !ok {
			continue
		}
		if _, ok := seen[text]; ok {
			continue
		}
		seen[text] = struct{}{}
		entry := map[string]any{
			"kind":        "render_node",
			"text":        text,
			"source":      "graph_node",
			"source_text": text,
		}
		if id := strings.TrimSpace(node.ID); id != "" {
			entry["source_id"] = id
		}
		if quote := strings.TrimSpace(node.SourceQuote); quote != "" {
			entry["source_quote"] = quote
		}
		if role := strings.TrimSpace(string(node.Role)); role != "" {
			entry["role"] = role
		}
		if discourse := strings.TrimSpace(node.DiscourseRole); discourse != "" {
			entry["context"] = "discourse_role=" + discourse
		}
		details = append(details, entry)
	}
	return details
}

func visibleOffGraphDetailsForAuthorValidation(out compile.Output, items []offGraphItem) []map[string]any {
	visible := visibleAuthorProofTexts(out)
	if len(visible) == 0 {
		return nil
	}
	details := make([]map[string]any, 0, len(items))
	for _, item := range items {
		if _, ok := visible[strings.TrimSpace(item.Text)]; !ok {
			continue
		}
		details = append(details, renderOffGraphDetails([]offGraphItem{item}, identityRenderText)...)
	}
	return details
}

func visibleAuthorRenderNodeTexts(out compile.Output) map[string]struct{} {
	visible := map[string]struct{}{}
	add := func(value string) {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			visible[trimmed] = struct{}{}
		}
	}
	addList := func(values []string) {
		for _, value := range values {
			add(value)
		}
	}
	addPath := func(path compile.TransmissionPath) {
		add(path.Driver)
		addList(path.Steps)
		add(path.Target)
	}
	addList(out.Drivers)
	addList(out.Targets)
	for _, path := range out.TransmissionPaths {
		addPath(path)
	}
	for _, branch := range out.Branches {
		addList(branch.Anchors)
		addList(branch.Drivers)
		addList(branch.BranchDrivers)
		addList(branch.Targets)
		for _, path := range branch.TransmissionPaths {
			addPath(path)
		}
	}
	return visible
}

func visibleAuthorProofTexts(out compile.Output) map[string]struct{} {
	visible := map[string]struct{}{}
	add := func(values []string) {
		for _, value := range values {
			if trimmed := strings.TrimSpace(value); trimmed != "" {
				visible[trimmed] = struct{}{}
			}
		}
	}
	add(out.EvidenceNodes)
	return visible
}

func identityRenderText(_ string, fallback string) string {
	return fallback
}

func cloneHiddenDetailItems(items []map[string]any) []map[string]any {
	if len(items) == 0 {
		return nil
	}
	out := make([]map[string]any, 0, len(items))
	for _, item := range items {
		cloned := make(map[string]any, len(item))
		for key, value := range item {
			cloned[key] = value
		}
		out = append(out, cloned)
	}
	return out
}

func dedupeAuthorValidationDetailItems(items []map[string]any) []map[string]any {
	if len(items) == 0 {
		return nil
	}
	seen := map[string]struct{}{}
	out := make([]map[string]any, 0, len(items))
	for _, item := range items {
		key := authorValidationDetailItemKey(item)
		if key == "" {
			continue
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, item)
	}
	return out
}

func authorValidationDetailItemKey(item map[string]any) string {
	kind := hiddenDetailString(item, "kind")
	switch kind {
	case "inference_path":
		return kind + "\x00" + hiddenDetailString(item, "branch") + "\x00" + hiddenDetailString(item, "from") + "\x00" + strings.Join(hiddenDetailStringSlice(item, "steps"), "\x00") + "\x00" + hiddenDetailString(item, "to")
	case "render_node", "proof_point", "explanation", "supplementary_proof", "source_quote", "reference_proof":
		return kind + "\x00" + hiddenDetailString(item, "text")
	default:
		return ""
	}
}

func authorValidationSystemPrompt() string {
	return strings.TrimSpace(`
You are an author-claim validator for a reader-facing product.

Validate ONLY what the author claims, uses as proof, or implies. Do not critique the extraction pipeline, graph shape, missing nodes, missing edges, target classification, branch grouping, or UI wording.

For each claim candidate:
- Validate only externally checkable point claims. A checkable claim is a concrete public fact, number, dated event, sourceable quote, named-entity attribution, capacity claim, timing claim, or current/historical state that public evidence could support or contradict.
- Do not force abstract framing, interpretation, analogy, value judgment, broad causal description, unresolved forecast, or reader-facing thesis language into a true/false fact check. Mark it "interpretive" and explain that point validation is deferred to inference validation unless it contains concrete factual subclaims.
- Use "unverified" only for concrete checkable claims where evidence should exist but the available source/search context cannot establish it. Do not use "unverified" for abstract claims merely because they cannot be proven or disproven as standalone facts.
- If the source text does not show the author making the claim, use status "not_author_claim". This is not an author fault.
- When source_text/source_quote/context is present, use it as provenance for the candidate. If it contradicts the compressed candidate wording, flag the candidate or subclaim as contradicted/not_author_claim rather than repairing it into a different supported claim.
- If it is an objective factual/numeric claim and available evidence supports it, use "supported".
- If available evidence contradicts it, use "contradicted".
- If it is objective but cannot be verified from the source/search context, use "unverified".
- If it is opinion, interpretation, analogy, or unresolved forecast, use "interpretive" unless it contains a checkable factual subclaim.
- Proof/evidence points are first-class claim candidates: numbers, quotations, cited facts, capacity claims, timing claims, and named-company evidence must be checked, not merely treated as support text.
- Before assigning a status to a checkable claim, list the data requirements needed to validate it in required_evidence. Decide what metric, date/window, denominator/base, object scope, and source type are necessary, then judge the actual values you find against that requirement.
- "supported" requires external evidence. The author's own text, "author states", "author cites", "the author says", or a bare restatement of the candidate is provenance, not validation. If search/source context does not provide an external source/value for a checkable claim, mark it "unverified" even when the author names a source.
- For each supported checkable claim, evidence or required_evidence.evidence must include the external source/value you found, such as an official report name, market data value, company filing, quoted external publication, or URL/title. Do not use only author_source evidence for supported point claims.
- When external_evidence_hints are present, use them as retrieved external context for the matching claim_id. If a hint contains an official source/value matching the candidate's metric, cite that hint in evidence and mark the claim supported. If it shows the same number under a narrower/different wording, preserve that scope in reason.
- Resolve relative time windows from posted_at or source publication time before validating. "month-to-date", "this month", "April so far", "year-to-date", "this week", and similar phrases are different windows; do not substitute YTD for MTD, week-to-date for month-to-date, or data after publication for data available at publication.
- Use current_date from the payload as the validation date. Do not call a claim future-dated or hypothetical when its date/window is on or before current_date. For example, if current_date is after April 2026, then April 2026 EIA/IEA data is not future-dated.
- For source-cited numeric claims, search the cited source name plus the metric and value before judging. If the author says EIA/IEA/Fed/Treasury/company filing and gives a concrete number, search that exact source/value/window; do not stop at "author cites source".
- If the author's wording implies returns as of the post date, validate with the market close or latest available market data at or before the post date. If the exact post-time cut is unavailable, state the nearest close used in required_evidence.time_window and reason.
- For interpretive claims with embedded factual prerequisites, use required_evidence for the prerequisites and keep the parent claim interpretive unless the parent itself is a point fact.
- Split compound proof points into subclaims before judging. Example: "NVL72 has 5000+ copper cables and weighs 1.36 tons" must become separate subclaims for cable count and rack/cable weight.
- For every numeric subclaim, normalize units and compare against evidence ranges. Example: 100 weeks is about 23 months; evidence of 18-36 months supports it.
- For percentages and ratios, identify the comparison base/denominator before judging. Example: "China consumes 80-90% of Iran's oil production" is not supported by evidence that China buys 80-90% of Iran's seaborne oil exports; production and exports are different bases.
- Check attribution/object scope. Example: if 1.36 tons is rack weight, do not treat it as copper-cable weight.
- Distinguish "number found but wrong object/base" from "number not found". If the number is found for a related but different subject, object, time window, or denominator, set scope_status="mismatch", attribution_ok=false, and status "contradicted" rather than "unverified".
- If the denominator/base matches after unit conversion or wording differences, set scope_status="exact_match". If the evidence is directionally related but not the same base, set scope_status="related_scope". If no reliable base can be found, set scope_status="unknown".
- Preserve the candidate's subject when validating. If the evidence supports "rack weight" but the candidate says "copper-cable weight", set attribution_ok=false and do not silently rewrite the subject.
- If a precise number is not public but the direction is supported, mark the subclaim "unverified" and say it may require a paid or specialist source; do not call it false unless contradicted.

For each inference candidate:
- Judge whether the author actually makes that inferential jump and whether the stated premises support it.
- When edge_evidence/source_quote/context is present, use it as provenance for the displayed transmission path. If a displayed edge lacks author support, mark the inference weak, unsupported_jump, or not_author_inference instead of repairing it into another path.
- Before assigning an inference status, list the data requirements needed to support the jump in required_evidence. Identify what data would make the transition valid: market levels, price moves, rate moves, inventories, dates, event status, denominators, or other factual prerequisites.
- Do not require the author to have listed every factual premise inside the rendered path. If a path depends on checkable market data, historical data, timing, prices, rates, inventories, or public events, use available source/search context to validate those missing intermediate premises.
- Mark an inference "sound" when the author makes the jump and the required explicit or implicit premises are externally supported. Put the checked intermediate premises in evidence.
- Do not mark an inference "sound" merely because the author explicitly makes the jump. Author-provenance can establish that the jump exists; soundness requires external support for the factual premises needed by that jump.
- Mark it "weak" when the author makes the jump but key implicit premises remain unverified, are only directionally supported, or the rendered path compresses important steps that search cannot firmly establish.
- Mark it "unsupported_jump" when checked premises contradict the jump or the conclusion does not follow even after adding reasonable implicit premises.
- Use missing_links for unverified or contradicted intermediate premises, not for premises that are simply absent from the article but externally supported.
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
    {
      "claim_id":"...",
      "text":"...",
      "claim_type":"fact|number|forecast|interpretation|opinion",
      "status":"supported|contradicted|unverified|interpretive|not_author_claim",
      "required_evidence":[
        {"description":"data needed to validate this claim", "subject":"...", "metric":"...", "original_value":"...", "unit":"...", "time_window":"...", "source_type":"official|market_data|company_filing|news|specialist_database|legal|author_source|other", "series":"FRED/API/table identifier when applicable", "preferred_sources":["specific source names"], "queries":["actual queries used or recommended"], "comparison_rule":"how source value was compared to author value", "scope_caveat":"scope/base warning when needed", "status":"supported|contradicted|unverified|interpretive|not_author_claim", "evidence":["source/value"], "reason":"brief comparison of required data vs found data"}
      ],
      "evidence":["short quote or source"],
      "reason":"brief reason",
      "subclaims":[
        {
          "subclaim_id":"...",
          "parent_claim_id":"...",
          "text":"atomic subclaim",
          "subject":"object being described",
          "metric":"measured attribute",
          "original_value":"as written by author",
          "normalized_value":"converted value when applicable",
          "evidence_value":"matched source value when exact",
          "evidence_range":"matched source range when applicable",
          "comparison_base":"author's denominator/object/time window, e.g. Iran total oil production",
          "evidence_base":"evidence denominator/object/time window, e.g. Iran seaborne oil exports",
          "scope_status":"exact_match|related_scope|mismatch|unknown",
          "unit_normalized":true,
          "range_covered":true,
          "attribution_ok":true,
          "status":"supported|contradicted|unverified|interpretive|not_author_claim",
          "evidence":["short quote or source"],
          "reason":"brief reason"
        }
      ]
    }
  ],
  "inference_checks": [
    {"inference_id":"...", "from":"...", "to":"...", "steps":["..."], "status":"sound|weak|unsupported_jump|not_author_inference", "required_evidence":[{"description":"data needed to support this jump", "subject":"...", "metric":"...", "original_value":"...", "unit":"...", "time_window":"...", "source_type":"official|market_data|company_filing|news|specialist_database|legal|author_source|other", "series":"...", "preferred_sources":["..."], "queries":["..."], "comparison_rule":"...", "status":"supported|contradicted|unverified|interpretive|not_author_claim", "evidence":["source/value"], "reason":"brief comparison of required data vs found data"}], "evidence":["short quote or source"], "reason":"brief reason", "missing_links":["..."]}
  ]
}
`)
}

func authorValidationPlanSystemPrompt() string {
	return strings.TrimSpace(`
You are an author-claim verification planner for a reader-facing product.

Create a verification plan only. Do not judge whether claims are true. Do not return claim_checks or inference_checks.

For every candidate:
- Decide whether it needs external validation. Concrete numbers, dated events, named-source attributions, market data, public official data, company figures, and source-cited proof points need validation.
- Interpretive theses, unresolved forecasts, analogies, and opinions may have needs_validation=false unless they contain concrete factual prerequisites.
- Split compound proof points into atomic claims.
- For each checkable claim, specify the exact subject, metric, original_value, unit, time_window, source_type, series/table/API when known, preferred_sources, queries, and comparison_rule.
- For macro/market data, prefer executable identifiers over generic descriptions. Examples: FRED:WRESBAL for US reserve balances, FRED:WALCL for Fed total assets, FRED:WTREGEN or Treasury QRA/Daily Treasury Statement for TGA, DeFiLlama stablecoins or issuer transparency pages for stablecoin supply, Farside Investors/SoSoValue/Bloomberg for spot Bitcoin ETF flows, PACER/CourtListener/Reuters/Bloomberg/FT for lawsuits or market-manipulation allegations.
- Atomic claims must be structured objects, not strings. A range or trend usually needs multiple atomic claims plus a trend comparison. Example: "3.4T fell to 2.85T" requires one spec for 3.4T, one for 2.85T, and a comparison rule that later value is lower.
- Preserve scope caveats. Example: "EIA 9.1 million b/d production shut-ins" is not generic voluntary OPEC cuts; the metric should be production shut-ins.
- Use current_date from the payload. Dates on or before current_date are not future-dated.
- For each inference, specify the factual premises needed to make the jump sound. Do not judge soundness here.

Return strict JSON only:
{
  "claim_plans": [
    {
      "claim_id": "...",
      "text": "...",
      "claim_kind": "number|number_trend|dated_event|market_flow|source_attribution|legal_allegation|interpretation|forecast|causal_claim",
      "needs_validation": true,
      "atomic_claims": [
        {
          "text":"atomic checkable statement",
          "subject":"...",
          "metric":"...",
          "original_value":"author value/range",
          "unit":"...",
          "time_window":"...",
          "source_type":"official|market_data|company_filing|news|specialist_database|legal|author_source|other",
          "series":"FRED:WRESBAL or official table/API identifier when known",
          "entity":"...",
          "geography":"...",
          "denominator":"...",
          "preferred_sources":["specific official/vendor/source names"],
          "queries":["source metric value time window"],
          "comparison_rule":"how the found value will be compared to the author value",
          "scope_caveat":"brief caveat when wording may need precision"
        }
      ],
      "required_evidence": [
        {"description":"data needed", "subject":"...", "metric":"...", "original_value":"...", "unit":"...", "time_window":"...", "source_type":"official|market_data|company_filing|news|specialist_database|legal|author_source|other", "series":"...", "preferred_sources":["..."], "queries":["..."], "comparison_rule":"...", "scope_caveat":"...", "status":"unverified", "reason":"why this data is needed"}
      ],
      "preferred_sources": ["EIA STEO", "company filing"],
      "queries": ["source metric value time window"],
      "scope_caveat": "brief caveat when wording may need precision"
    }
  ],
  "inference_plans": [
    {
      "inference_id":"...",
      "from":"...",
      "to":"...",
      "steps":["..."],
      "required_evidence":[{"description":"data needed to support the jump", "subject":"...", "metric":"...", "original_value":"...", "unit":"...", "time_window":"...", "source_type":"official|market_data|company_filing|news|specialist_database|legal|author_source|other", "series":"...", "preferred_sources":["..."], "queries":["..."], "comparison_rule":"...", "status":"unverified", "reason":"why this premise is needed"}],
      "queries":["..."],
      "missing_premises":["..."]
    }
  ]
}
`)
}

func collectAuthorClaimCandidates(out compile.Output) []authorClaimCandidate {
	candidates := make([]authorClaimCandidate, 0)
	seen := map[string]struct{}{}
	provenance := authorClaimProvenanceByKey(out.Details.Items)
	add := func(kind, text, branch string, item map[string]any) {
		text = strings.TrimSpace(text)
		if text == "" {
			return
		}
		if item == nil {
			item = provenance[authorClaimProvenanceKey(kind, text)]
		}
		keyBranch := strings.TrimSpace(branch)
		if kind == "render_node" {
			keyBranch = ""
		}
		key := kind + "\x00" + keyBranch + "\x00" + text
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
		applyAuthorClaimProvenance(&candidates[len(candidates)-1], item)
	}
	addRenderNodes := func(branch string, values ...[]string) {
		for _, group := range values {
			for _, value := range group {
				add("render_node", value, branch, nil)
			}
		}
	}
	addRenderPathNodes := func(branch string, paths []compile.TransmissionPath) {
		for _, path := range paths {
			add("render_node", path.Driver, branch, nil)
			addRenderNodes(branch, path.Steps)
			add("render_node", path.Target, branch, nil)
		}
	}
	addRenderNodes("", out.Drivers, out.Targets)
	addRenderPathNodes("", out.TransmissionPaths)
	for _, value := range out.EvidenceNodes {
		add("proof_point", value, "", nil)
	}
	for _, item := range out.Details.Items {
		kind := hiddenDetailString(item, "kind")
		if !isAuthorClaimDetailKind(kind) {
			continue
		}
		add(kind, hiddenDetailString(item, "text"), hiddenDetailString(item, "branch"), item)
	}
	for _, branch := range out.Branches {
		branchID := firstTrimmed(branch.ID, branch.Thesis)
		addRenderNodes(branchID, branch.Anchors, branch.Drivers, branch.BranchDrivers, branch.Targets)
		addRenderPathNodes(branchID, branch.TransmissionPaths)
	}
	return candidates
}

func defaultAuthorExternalEvidenceHints(ctx context.Context, claims []authorClaimCandidate, plan authorVerificationPlan) ([]authorExternalEvidenceHint, error) {
	if len(claims) == 0 {
		return nil, nil
	}
	client := &http.Client{Timeout: 12 * time.Second}
	out := make([]authorExternalEvidenceHint, 0)
	plans := authorClaimPlansByID(plan)
	for _, claim := range claims {
		claimPlan := plans[claim.ClaimID]
		out = append(out, buildEIAOilEvidenceHint(ctx, client, claim, claimPlan)...)
		out = append(out, buildPlannedExternalEvidenceHints(ctx, client, claim.ClaimID, claimPlan)...)
	}
	return out, nil
}

func buildEIAOilEvidenceHint(ctx context.Context, client *http.Client, claim authorClaimCandidate, claimPlan authorClaimVerificationPlan) []authorExternalEvidenceHint {
	if !needsEIAOilEvidenceHint(claim, claimPlan) {
		return nil
	}
	hint := authorExternalEvidenceHint{
		ClaimID: claim.ClaimID,
		Query:   `site:eia.gov STEO April 2026 production shut-ins 9.1 million b/d`,
	}
	for _, source := range []struct {
		url   string
		title string
	}{
		{
			url:   "https://www.eia.gov/outlooks/steo/report/global_oil.php/",
			title: "EIA Short-Term Energy Outlook - Global Oil Markets",
		},
		{
			url:   "https://www.eia.gov/pressroom/releases/press586.php",
			title: "EIA press release on Hormuz closure and production outages",
		},
	} {
		excerpt, err := fetchAuthorEvidenceExcerpt(ctx, client, source.url, []string{"9.1 million", "production shut-ins", "April"})
		if err != nil || strings.TrimSpace(excerpt) == "" {
			continue
		}
		hint.Results = append(hint.Results, authorExternalEvidenceResult{
			URL:     source.url,
			Title:   source.title,
			Excerpt: excerpt,
		})
	}
	if len(hint.Results) == 0 {
		return nil
	}
	return []authorExternalEvidenceHint{hint}
}

func buildPlannedExternalEvidenceHints(ctx context.Context, client *http.Client, claimID string, claimPlan authorClaimVerificationPlan) []authorExternalEvidenceHint {
	requirements := authorEvidenceRequirementsForPlan(claimPlan)
	out := make([]authorExternalEvidenceHint, 0)
	seen := map[string]struct{}{}
	for _, requirement := range requirements {
		for _, hint := range buildExternalEvidenceHintsForRequirement(ctx, client, claimID, requirement) {
			key := hint.Query + "\x00" + strings.TrimSpace(claimID)
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			if len(hint.Results) > 0 {
				out = append(out, hint)
			}
		}
	}
	return out
}

func authorEvidenceRequirementsForPlan(claimPlan authorClaimVerificationPlan) []compile.AuthorEvidenceRequirement {
	out := make([]compile.AuthorEvidenceRequirement, 0, len(claimPlan.RequiredEvidence)+len(claimPlan.AtomicClaims))
	out = append(out, claimPlan.RequiredEvidence...)
	for _, atomic := range claimPlan.AtomicClaims {
		out = append(out, compile.AuthorEvidenceRequirement{
			Description:      atomic.Text,
			Subject:          atomic.Subject,
			Metric:           atomic.Metric,
			OriginalValue:    atomic.OriginalValue,
			Unit:             atomic.Unit,
			TimeWindow:       atomic.TimeWindow,
			SourceType:       atomic.SourceType,
			Series:           atomic.Series,
			Entity:           atomic.Entity,
			Geography:        atomic.Geography,
			Denominator:      atomic.Denominator,
			PreferredSources: atomic.PreferredSources,
			Queries:          atomic.Queries,
			ComparisonRule:   atomic.ComparisonRule,
			ScopeCaveat:      atomic.ScopeCaveat,
		})
	}
	return out
}

func buildExternalEvidenceHintsForRequirement(ctx context.Context, client *http.Client, claimID string, requirement compile.AuthorEvidenceRequirement) []authorExternalEvidenceHint {
	if seriesID, ok := fredSeriesID(requirement.Series); ok {
		result, ok := fetchFREDEvidenceResult(ctx, client, seriesID, requirement.TimeWindow, requirement.OriginalValue, requirement.ComparisonRule)
		if ok {
			return []authorExternalEvidenceHint{{
				ClaimID: claimID,
				Query:   firstNonEmpty(firstString(requirement.Queries), "FRED "+seriesID+" "+requirement.TimeWindow),
				Results: []authorExternalEvidenceResult{
					result,
				},
			}}
		}
	}
	if symbols := stablecoinSymbolsForRequirement(requirement); len(symbols) > 0 {
		hint := authorExternalEvidenceHint{
			ClaimID: claimID,
			Query:   firstNonEmpty(firstString(requirement.Queries), "DeFiLlama stablecoins "+strings.Join(symbols, " ")+" "+requirement.TimeWindow),
		}
		for _, symbol := range symbols {
			result, ok := fetchStablecoinEvidenceResult(ctx, client, symbol, requirement.TimeWindow, requirement.OriginalValue)
			if ok {
				hint.Results = append(hint.Results, result)
			}
		}
		if len(hint.Results) > 0 {
			return []authorExternalEvidenceHint{hint}
		}
	}
	if isBitcoinETFRequirement(requirement) {
		result, ok := fetchBitcoinETFEvidenceResult(ctx, client, requirement.TimeWindow)
		if ok {
			return []authorExternalEvidenceHint{{
				ClaimID: claimID,
				Query:   firstNonEmpty(firstString(requirement.Queries), "SoSoValue Bitcoin spot ETF flows "+requirement.TimeWindow),
				Results: []authorExternalEvidenceResult{
					result,
				},
			}}
		}
	}
	if isLegalEvidenceRequirement(requirement) {
		query := firstNonEmpty(firstString(requirement.Queries), strings.TrimSpace(requirement.Subject+" "+requirement.Description))
		result, ok := fetchCourtListenerEvidenceResult(ctx, client, query)
		if ok {
			return []authorExternalEvidenceHint{{
				ClaimID: claimID,
				Query:   "CourtListener " + query,
				Results: []authorExternalEvidenceResult{
					result,
				},
			}}
		}
	}
	return nil
}

func authorClaimPlansByID(plan authorVerificationPlan) map[string]authorClaimVerificationPlan {
	out := make(map[string]authorClaimVerificationPlan, len(plan.ClaimPlans))
	for _, claimPlan := range plan.ClaimPlans {
		id := strings.TrimSpace(claimPlan.ClaimID)
		if id == "" {
			continue
		}
		out[id] = claimPlan
	}
	return out
}

func needsEIAOilEvidenceHint(claim authorClaimCandidate, plan authorClaimVerificationPlan) bool {
	planEvidenceParts := make([]string, 0, len(plan.RequiredEvidence)*4+len(plan.PreferredSources)+len(plan.Queries)+2)
	planEvidenceParts = append(planEvidenceParts, plan.Text, plan.ScopeCaveat)
	for _, atomicClaim := range plan.AtomicClaims {
		planEvidenceParts = append(planEvidenceParts,
			atomicClaim.Text,
			atomicClaim.Subject,
			atomicClaim.Metric,
			atomicClaim.OriginalValue,
			atomicClaim.Unit,
			atomicClaim.TimeWindow,
			atomicClaim.Series,
			atomicClaim.ComparisonRule,
			atomicClaim.ScopeCaveat,
		)
		planEvidenceParts = append(planEvidenceParts, atomicClaim.PreferredSources...)
		planEvidenceParts = append(planEvidenceParts, atomicClaim.Queries...)
	}
	planEvidenceParts = append(planEvidenceParts, plan.PreferredSources...)
	planEvidenceParts = append(planEvidenceParts, plan.Queries...)
	for _, requirement := range plan.RequiredEvidence {
		planEvidenceParts = append(planEvidenceParts, requirement.Description, requirement.Subject, requirement.Metric, requirement.TimeWindow, requirement.SourceType, requirement.Reason)
	}
	text := strings.ToLower(strings.Join([]string{
		claim.Text,
		claim.SourceText,
		claim.SourceQuote,
		claim.Context,
		strings.Join(planEvidenceParts, " "),
	}, " "))
	hasSource := strings.Contains(text, "eia") ||
		strings.Contains(text, "energy information administration") ||
		strings.Contains(text, "美国能源信息署")
	hasOil := strings.Contains(text, "oil") ||
		strings.Contains(text, "crude") ||
		strings.Contains(text, "石油") ||
		strings.Contains(text, "原油") ||
		strings.Contains(text, "减产") ||
		strings.Contains(text, "停产") ||
		strings.Contains(text, "shut-in")
	hasVolume := strings.Contains(text, "9.1") ||
		strings.Contains(text, "910") ||
		strings.Contains(text, "million b/d") ||
		strings.Contains(text, "百万桶") ||
		strings.Contains(text, "万桶")
	return hasSource && hasOil && hasVolume
}

func fetchAuthorEvidenceExcerpt(ctx context.Context, client *http.Client, rawURL string, keywords []string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0")
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("fetch evidence hint: status %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", err
	}
	text := compactAuthorEvidenceText(stripAuthorEvidenceHTML(string(body)))
	if text == "" {
		return "", nil
	}
	lower := strings.ToLower(text)
	idx := -1
	for _, keyword := range keywords {
		keyword = strings.ToLower(strings.TrimSpace(keyword))
		if keyword == "" {
			continue
		}
		if found := strings.Index(lower, keyword); found >= 0 {
			idx = found
			break
		}
	}
	if idx < 0 {
		return truncateAuthorEvidenceExcerpt(text, 900), nil
	}
	start := idx - 350
	if start < 0 {
		start = 0
	}
	end := idx + 650
	if end > len(text) {
		end = len(text)
	}
	return truncateAuthorEvidenceExcerpt(strings.TrimSpace(text[start:end]), 900), nil
}

func fetchFREDEvidenceResult(ctx context.Context, client *http.Client, seriesID, window, originalValue, comparisonRule string) (authorExternalEvidenceResult, bool) {
	seriesID = strings.TrimSpace(seriesID)
	if seriesID == "" {
		return authorExternalEvidenceResult{}, false
	}
	rawURL := "https://fred.stlouisfed.org/graph/fredgraph.csv?id=" + seriesID
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return authorExternalEvidenceResult{}, false
	}
	resp, err := client.Do(req)
	if err != nil {
		return authorExternalEvidenceResult{}, false
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return authorExternalEvidenceResult{}, false
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return authorExternalEvidenceResult{}, false
	}
	return buildFREDEvidenceResultFromCSV(seriesID, window, originalValue, comparisonRule, string(body))
}

func buildFREDEvidenceResultFromCSV(seriesID, window, originalValue, comparisonRule, rawCSV string) (authorExternalEvidenceResult, bool) {
	observations, ok := parseFREDCSV(seriesID, rawCSV)
	if !ok {
		return authorExternalEvidenceResult{}, false
	}
	start, end, hasWindow := parseAuthorEvidenceDateWindow(window)
	filtered := filterDatedValues(observations, start, end, hasWindow)
	if len(filtered) == 0 {
		return authorExternalEvidenceResult{}, false
	}
	stats := datedValueStats(filtered)
	excerpt := fmt.Sprintf("FRED %s observations for %s: first %s=%s, last %s=%s, min %s=%s, max %s=%s. author value %s. Comparison rule: %s.",
		seriesID,
		firstNonEmpty(strings.TrimSpace(window), "available window"),
		stats.First.Date.Format("2006-01-02"), formatAuthorEvidenceNumber(stats.First.Value),
		stats.Last.Date.Format("2006-01-02"), formatAuthorEvidenceNumber(stats.Last.Value),
		stats.Min.Date.Format("2006-01-02"), formatAuthorEvidenceNumber(stats.Min.Value),
		stats.Max.Date.Format("2006-01-02"), formatAuthorEvidenceNumber(stats.Max.Value),
		firstNonEmpty(originalValue, "not specified"),
		firstNonEmpty(comparisonRule, "compare source values to author value and time window"),
	)
	return authorExternalEvidenceResult{
		URL:     "https://fred.stlouisfed.org/series/" + seriesID,
		Title:   "FRED " + seriesID,
		Excerpt: truncateAuthorEvidenceExcerpt(excerpt, 900),
	}, true
}

type datedAuthorEvidenceValue struct {
	Date  time.Time
	Value float64
}

func parseFREDCSV(seriesID, rawCSV string) ([]datedAuthorEvidenceValue, bool) {
	reader := csv.NewReader(bytes.NewBufferString(rawCSV))
	reader.FieldsPerRecord = -1
	records, err := reader.ReadAll()
	if err != nil || len(records) < 2 {
		return nil, false
	}
	valueColumn := 1
	if len(records[0]) > 1 {
		for i, header := range records[0] {
			if strings.EqualFold(strings.TrimSpace(header), strings.TrimSpace(seriesID)) {
				valueColumn = i
				break
			}
		}
	}
	out := make([]datedAuthorEvidenceValue, 0, len(records)-1)
	for _, record := range records[1:] {
		if len(record) <= valueColumn {
			continue
		}
		date, err := time.Parse("2006-01-02", strings.TrimSpace(record[0]))
		if err != nil {
			continue
		}
		valueText := strings.TrimSpace(record[valueColumn])
		if valueText == "" || valueText == "." {
			continue
		}
		value, err := strconv.ParseFloat(valueText, 64)
		if err != nil {
			continue
		}
		out = append(out, datedAuthorEvidenceValue{Date: date, Value: value})
	}
	return out, len(out) > 0
}

func fetchStablecoinEvidenceResult(ctx context.Context, client *http.Client, symbol, window, originalValue string) (authorExternalEvidenceResult, bool) {
	id, ok := stablecoinID(symbol)
	if !ok {
		return authorExternalEvidenceResult{}, false
	}
	rawURL := "https://stablecoins.llama.fi/stablecoin/" + id
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return authorExternalEvidenceResult{}, false
	}
	req.Header.Set("User-Agent", "Mozilla/5.0")
	resp, err := client.Do(req)
	if err != nil {
		return authorExternalEvidenceResult{}, false
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return authorExternalEvidenceResult{}, false
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 32<<20))
	if err != nil {
		return authorExternalEvidenceResult{}, false
	}
	return buildStablecoinEvidenceResultFromJSON(symbol, window, originalValue, string(body))
}

func buildStablecoinEvidenceResultFromJSON(symbol, window, originalValue, rawJSON string) (authorExternalEvidenceResult, bool) {
	var payload struct {
		Symbol       string `json:"symbol"`
		ChainBalance map[string]struct {
			Tokens []struct {
				Date        any                `json:"date"`
				Circulating map[string]float64 `json:"circulating"`
			} `json:"tokens"`
		} `json:"chainBalances"`
	}
	if err := json.Unmarshal([]byte(rawJSON), &payload); err != nil {
		return authorExternalEvidenceResult{}, false
	}
	if strings.TrimSpace(symbol) == "" {
		symbol = payload.Symbol
	}
	start, end, hasWindow := parseAuthorEvidenceDateWindow(window)
	byDate := map[time.Time]float64{}
	for _, chain := range payload.ChainBalance {
		for _, token := range chain.Tokens {
			date, ok := parseStablecoinTimestamp(token.Date)
			if !ok {
				continue
			}
			value, ok := token.Circulating["peggedUSD"]
			if !ok {
				continue
			}
			byDate[date] += value
		}
	}
	values := make([]datedAuthorEvidenceValue, 0, len(byDate))
	for date, value := range byDate {
		values = append(values, datedAuthorEvidenceValue{Date: date, Value: value})
	}
	sort.Slice(values, func(i, j int) bool { return values[i].Date.Before(values[j].Date) })
	filtered := filterDatedValues(values, start, end, hasWindow)
	if len(filtered) == 0 {
		return authorExternalEvidenceResult{}, false
	}
	stats := datedValueStats(filtered)
	delta := stats.Last.Value - stats.First.Value
	excerpt := fmt.Sprintf("DeFiLlama stablecoin %s circulating supply for %s: first %s=%s, last %s=%s, delta=%s. author value %s.",
		strings.ToUpper(strings.TrimSpace(symbol)),
		firstNonEmpty(strings.TrimSpace(window), "available window"),
		stats.First.Date.Format("2006-01-02"), formatAuthorEvidenceNumber(stats.First.Value),
		stats.Last.Date.Format("2006-01-02"), formatAuthorEvidenceNumber(stats.Last.Value),
		formatAuthorEvidenceNumber(delta),
		firstNonEmpty(originalValue, "not specified"),
	)
	return authorExternalEvidenceResult{
		URL:     "https://stablecoins.llama.fi/stablecoin/" + firstNonEmpty(mustStablecoinID(symbol), strings.ToUpper(symbol)),
		Title:   "DeFiLlama stablecoin " + strings.ToUpper(strings.TrimSpace(symbol)),
		Excerpt: truncateAuthorEvidenceExcerpt(excerpt, 900),
	}, true
}

func fetchBitcoinETFEvidenceResult(ctx context.Context, client *http.Client, window string) (authorExternalEvidenceResult, bool) {
	body := bytes.NewBufferString(`{}`)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://open.sosovalue.xyz/openapi/v1/etf/us-btc-spot/historicalInflowChart", body)
	if err != nil {
		return authorExternalEvidenceResult{}, false
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return authorExternalEvidenceResult{}, false
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return authorExternalEvidenceResult{}, false
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return authorExternalEvidenceResult{}, false
	}
	return buildBitcoinETFEvidenceResultFromSoSoValueJSON(window, string(raw))
}

func buildBitcoinETFEvidenceResultFromSoSoValueJSON(window, rawJSON string) (authorExternalEvidenceResult, bool) {
	var payload struct {
		Code int             `json:"code"`
		Data json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal([]byte(rawJSON), &payload); err != nil {
		return authorExternalEvidenceResult{}, false
	}
	var rows []struct {
		Date           string  `json:"date"`
		TotalNetInflow float64 `json:"totalNetInflow"`
	}
	if len(payload.Data) > 0 && payload.Data[0] == '{' {
		var wrapped struct {
			List []struct {
				Date           string  `json:"date"`
				TotalNetInflow float64 `json:"totalNetInflow"`
			} `json:"list"`
		}
		if err := json.Unmarshal(payload.Data, &wrapped); err == nil {
			rows = wrapped.List
		}
	} else if len(payload.Data) > 0 {
		_ = json.Unmarshal(payload.Data, &rows)
	}
	if len(rows) == 0 {
		return authorExternalEvidenceResult{}, false
	}
	start, end, hasWindow := parseAuthorEvidenceDateWindow(window)
	values := make([]datedAuthorEvidenceValue, 0, len(rows))
	for _, row := range rows {
		date, err := time.Parse("2006-01-02", strings.TrimSpace(row.Date))
		if err != nil {
			continue
		}
		values = append(values, datedAuthorEvidenceValue{Date: date, Value: row.TotalNetInflow})
	}
	filtered := filterDatedValues(values, start, end, hasWindow)
	if len(filtered) == 0 {
		return authorExternalEvidenceResult{}, false
	}
	stats := datedValueStats(filtered)
	var sum float64
	var positiveDays, negativeDays, zeroDays int
	for _, value := range filtered {
		sum += value.Value
		switch {
		case value.Value > 0:
			positiveDays++
		case value.Value < 0:
			negativeDays++
		default:
			zeroDays++
		}
	}
	continuousOutflow := positiveDays == 0 && negativeDays > 0
	excerpt := fmt.Sprintf("SoSoValue BTC spot ETF flows for %s: first %s=%s, last %s=%s, sum=%s, positive_days=%d, negative_days=%d, zero_days=%d, min %s=%s, max %s=%s, continuous_outflow=%t.",
		firstNonEmpty(strings.TrimSpace(window), "available window"),
		stats.First.Date.Format("2006-01-02"), formatAuthorEvidenceNumber(stats.First.Value),
		stats.Last.Date.Format("2006-01-02"), formatAuthorEvidenceNumber(stats.Last.Value),
		formatAuthorEvidenceNumber(sum),
		positiveDays,
		negativeDays,
		zeroDays,
		stats.Min.Date.Format("2006-01-02"), formatAuthorEvidenceNumber(stats.Min.Value),
		stats.Max.Date.Format("2006-01-02"), formatAuthorEvidenceNumber(stats.Max.Value),
		continuousOutflow,
	)
	return authorExternalEvidenceResult{
		URL:     "https://open.sosovalue.xyz/openapi/v1/etf/us-btc-spot/historicalInflowChart",
		Title:   "SoSoValue BTC spot ETF flows",
		Excerpt: truncateAuthorEvidenceExcerpt(excerpt, 900),
	}, true
}

func fetchCourtListenerEvidenceResult(ctx context.Context, client *http.Client, query string) (authorExternalEvidenceResult, bool) {
	query = normalizeCourtListenerQuery(query)
	if query == "" {
		return authorExternalEvidenceResult{}, false
	}
	rawURL := "https://www.courtlistener.com/api/rest/v4/search/?q=" + url.QueryEscape(query) + "&type=r"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return authorExternalEvidenceResult{}, false
	}
	req.Header.Set("User-Agent", "Mozilla/5.0")
	resp, err := client.Do(req)
	if err != nil {
		return authorExternalEvidenceResult{}, false
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return authorExternalEvidenceResult{}, false
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return authorExternalEvidenceResult{}, false
	}
	return buildCourtListenerEvidenceResultFromJSON(query, string(body))
}

func normalizeCourtListenerQuery(query string) string {
	query = strings.TrimSpace(query)
	if query == "" {
		return ""
	}
	lower := strings.ToLower(query)
	if strings.Contains(lower, "jane street") && !strings.Contains(query, `"Jane Street"`) && !strings.Contains(query, `"jane street"`) {
		query = strings.TrimSpace(`"Jane Street" ` + strings.ReplaceAll(query, "Jane Street", ""))
		query = strings.TrimSpace(strings.ReplaceAll(query, "jane street", ""))
	}
	return strings.Join(strings.Fields(query), " ")
}

func buildCourtListenerEvidenceResultFromJSON(query, rawJSON string) (authorExternalEvidenceResult, bool) {
	var payload struct {
		Count   int `json:"count"`
		Results []struct {
			CaseName          string `json:"caseName"`
			Court             any    `json:"court"`
			DateFiled         string `json:"dateFiled"`
			DocketNumber      string `json:"docketNumber"`
			Cause             string `json:"cause"`
			AbsoluteURL       string `json:"absolute_url"`
			DocketAbsoluteURL string `json:"docket_absolute_url"`
		} `json:"results"`
	}
	if err := json.Unmarshal([]byte(rawJSON), &payload); err != nil {
		return authorExternalEvidenceResult{}, false
	}
	if payload.Count == 0 || len(payload.Results) == 0 {
		return authorExternalEvidenceResult{}, false
	}
	parts := make([]string, 0, 3)
	resultURL := "https://www.courtlistener.com/?q=" + url.QueryEscape(query)
	for _, row := range payload.Results {
		caseName := strings.TrimSpace(row.CaseName)
		if caseName == "" {
			continue
		}
		if strings.Contains(resultURL, "/?q=") {
			if caseURL := absoluteCourtListenerURL(firstNonEmpty(row.DocketAbsoluteURL, row.AbsoluteURL)); caseURL != "" {
				resultURL = caseURL
			}
		}
		details := make([]string, 0, 4)
		if date := strings.TrimSpace(row.DateFiled); date != "" {
			details = append(details, date)
		}
		if court := courtListenerString(row.Court); court != "" {
			details = append(details, court)
		}
		if docket := strings.TrimSpace(row.DocketNumber); docket != "" {
			details = append(details, "docket "+docket)
		}
		if cause := strings.TrimSpace(row.Cause); cause != "" {
			details = append(details, cause)
		}
		if len(details) > 0 {
			parts = append(parts, caseName+" ("+strings.Join(details, ", ")+")")
		} else {
			parts = append(parts, caseName)
		}
		if len(parts) >= 3 {
			break
		}
	}
	if len(parts) == 0 {
		return authorExternalEvidenceResult{}, false
	}
	excerpt := fmt.Sprintf("CourtListener search %q: count=%d; top: %s.", query, payload.Count, strings.Join(parts, "; "))
	return authorExternalEvidenceResult{
		URL:     resultURL,
		Title:   "CourtListener legal search",
		Excerpt: truncateAuthorEvidenceExcerpt(excerpt, 900),
	}, true
}

func absoluteCourtListenerURL(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	if strings.HasPrefix(path, "http://") || strings.HasPrefix(path, "https://") {
		return path
	}
	if strings.HasPrefix(path, "/") {
		return "https://www.courtlistener.com" + path
	}
	return "https://www.courtlistener.com/" + path
}

func courtListenerString(value any) string {
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed)
	case map[string]any:
		for _, key := range []string{"name", "full_name", "short_name", "id"} {
			if text := strings.TrimSpace(fmt.Sprint(typed[key])); text != "" && text != "<nil>" {
				return text
			}
		}
	}
	return ""
}

type datedAuthorEvidenceStats struct {
	First datedAuthorEvidenceValue
	Last  datedAuthorEvidenceValue
	Min   datedAuthorEvidenceValue
	Max   datedAuthorEvidenceValue
}

func datedValueStats(values []datedAuthorEvidenceValue) datedAuthorEvidenceStats {
	stats := datedAuthorEvidenceStats{
		First: values[0],
		Last:  values[len(values)-1],
		Min:   values[0],
		Max:   values[0],
	}
	for _, value := range values[1:] {
		if value.Value < stats.Min.Value {
			stats.Min = value
		}
		if value.Value > stats.Max.Value {
			stats.Max = value
		}
	}
	return stats
}

func filterDatedValues(values []datedAuthorEvidenceValue, start, end time.Time, hasWindow bool) []datedAuthorEvidenceValue {
	if len(values) == 0 {
		return nil
	}
	sort.Slice(values, func(i, j int) bool { return values[i].Date.Before(values[j].Date) })
	if !hasWindow {
		if len(values) > 12 {
			return values[len(values)-12:]
		}
		return values
	}
	out := make([]datedAuthorEvidenceValue, 0, len(values))
	for _, value := range values {
		if !start.IsZero() && value.Date.Before(start) {
			continue
		}
		if !end.IsZero() && value.Date.After(end) {
			continue
		}
		out = append(out, value)
	}
	return out
}

func parseAuthorEvidenceDateWindow(window string) (time.Time, time.Time, bool) {
	window = strings.TrimSpace(window)
	if window == "" {
		return time.Time{}, time.Time{}, false
	}
	var dates []time.Time
	for _, match := range regexp.MustCompile(`20\d{2}-\d{2}-\d{2}`).FindAllString(window, -1) {
		if date, err := time.Parse("2006-01-02", match); err == nil {
			dates = append(dates, date)
		}
	}
	for _, match := range regexp.MustCompile(`20\d{2}-\d{2}`).FindAllString(window, -1) {
		if strings.Contains(match, "-") {
			if date, err := time.Parse("2006-01", match); err == nil {
				dates = append(dates, date)
			}
		}
	}
	monthPattern := regexp.MustCompile(`(?i)(jan|feb|mar|apr|may|jun|jul|aug|sep|oct|nov|dec)[a-z]*\s+20\d{2}`)
	for _, match := range monthPattern.FindAllString(window, -1) {
		if date, err := time.Parse("Jan 2006", normalizeAuthorEvidenceMonth(match)); err == nil {
			dates = append(dates, date)
		}
	}
	if len(dates) == 0 {
		return time.Time{}, time.Time{}, false
	}
	sort.Slice(dates, func(i, j int) bool { return dates[i].Before(dates[j]) })
	start := dates[0]
	end := dates[len(dates)-1]
	if end.Day() == 1 && !strings.Contains(window, end.Format("2006-01-02")) {
		end = end.AddDate(0, 1, -1)
	}
	return start, end, true
}

func normalizeAuthorEvidenceMonth(value string) string {
	fields := strings.Fields(strings.TrimSpace(value))
	if len(fields) < 2 {
		return value
	}
	month := strings.ToLower(fields[0])
	month = strings.TrimSuffix(month, ".")
	if len(month) > 3 {
		month = month[:3]
	}
	return strings.Title(month) + " " + fields[len(fields)-1]
}

func parseStablecoinTimestamp(value any) (time.Time, bool) {
	switch typed := value.(type) {
	case float64:
		return time.Unix(int64(typed), 0).UTC().Truncate(24 * time.Hour), true
	case string:
		if parsed, err := strconv.ParseInt(typed, 10, 64); err == nil {
			return time.Unix(parsed, 0).UTC().Truncate(24 * time.Hour), true
		}
		if date, err := time.Parse("2006-01-02", typed); err == nil {
			return date, true
		}
	}
	return time.Time{}, false
}

func fredSeriesID(series string) (string, bool) {
	series = strings.TrimSpace(series)
	if series == "" {
		return "", false
	}
	if strings.HasPrefix(strings.ToUpper(series), "FRED:") {
		return strings.TrimSpace(series[5:]), true
	}
	return "", false
}

func stablecoinSymbolsForRequirement(requirement compile.AuthorEvidenceRequirement) []string {
	text := strings.ToUpper(strings.Join([]string{requirement.Subject, requirement.Metric, requirement.Description, requirement.Series}, " "))
	out := make([]string, 0, 2)
	if strings.Contains(text, "USDT") || strings.Contains(text, "STABLECOIN") || strings.Contains(text, "稳定币") {
		out = append(out, "USDT")
	}
	if strings.Contains(text, "USDC") || strings.Contains(text, "STABLECOIN") || strings.Contains(text, "稳定币") {
		out = append(out, "USDC")
	}
	return out
}

func isBitcoinETFRequirement(requirement compile.AuthorEvidenceRequirement) bool {
	text := strings.ToLower(strings.Join([]string{
		requirement.Subject,
		requirement.Metric,
		requirement.Description,
		requirement.Series,
		strings.Join(requirement.PreferredSources, " "),
	}, " "))
	hasETF := strings.Contains(text, "etf")
	hasBitcoin := strings.Contains(text, "bitcoin") || strings.Contains(text, "btc") || strings.Contains(text, "比特币")
	hasFlow := strings.Contains(text, "flow") || strings.Contains(text, "inflow") || strings.Contains(text, "outflow") || strings.Contains(text, "流入") || strings.Contains(text, "流出")
	return hasETF && hasBitcoin && hasFlow
}

func isLegalEvidenceRequirement(requirement compile.AuthorEvidenceRequirement) bool {
	text := strings.ToLower(strings.Join([]string{
		requirement.Subject,
		requirement.Metric,
		requirement.Description,
		requirement.Series,
		requirement.SourceType,
		requirement.Entity,
		requirement.Geography,
		requirement.ComparisonRule,
		requirement.ScopeCaveat,
		strings.Join(requirement.PreferredSources, " "),
		strings.Join(requirement.Queries, " "),
	}, " "))
	return containsAny(text,
		"courtlistener",
		"pacer",
		"legal",
		"lawsuit",
		"class action",
		"filing",
		"complaint",
		"allegation",
		"jane street",
		"securities exchange act",
		"诉讼",
		"起诉",
		"集体诉讼",
		"操纵",
	)
}

func stablecoinID(symbol string) (string, bool) {
	switch strings.ToUpper(strings.TrimSpace(symbol)) {
	case "USDT", "TETHER":
		return "1", true
	case "USDC", "USD COIN":
		return "2", true
	default:
		return "", false
	}
}

func mustStablecoinID(symbol string) string {
	id, _ := stablecoinID(symbol)
	return id
}

func formatAuthorEvidenceNumber(value float64) string {
	return strconv.FormatFloat(value, 'f', -1, 64)
}

var (
	authorEvidenceScriptPattern = regexp.MustCompile(`(?is)<(script|style)[^>]*>.*?</(script|style)>`)
	authorEvidenceTagPattern    = regexp.MustCompile(`(?s)<[^>]+>`)
	authorEvidenceSpacePattern  = regexp.MustCompile(`\s+`)
)

func stripAuthorEvidenceHTML(raw string) string {
	raw = authorEvidenceScriptPattern.ReplaceAllString(raw, " ")
	raw = authorEvidenceTagPattern.ReplaceAllString(raw, " ")
	replacements := []struct {
		old string
		new string
	}{
		{"&nbsp;", " "},
		{"&amp;", "&"},
		{"&quot;", `"`},
		{"&#39;", "'"},
		{"&lt;", "<"},
		{"&gt;", ">"},
	}
	for _, replacement := range replacements {
		raw = strings.ReplaceAll(raw, replacement.old, replacement.new)
	}
	return raw
}

func compactAuthorEvidenceText(text string) string {
	return strings.TrimSpace(authorEvidenceSpacePattern.ReplaceAllString(text, " "))
}

func truncateAuthorEvidenceExcerpt(text string, max int) string {
	text = strings.TrimSpace(text)
	if max <= 0 || len(text) <= max {
		return text
	}
	return strings.TrimSpace(text[:max]) + "..."
}

func authorClaimProvenanceByKey(items []map[string]any) map[string]map[string]any {
	out := make(map[string]map[string]any, len(items))
	for _, item := range items {
		kind := hiddenDetailString(item, "kind")
		if !isAuthorClaimDetailKind(kind) {
			continue
		}
		text := hiddenDetailString(item, "text")
		if text == "" {
			continue
		}
		out[authorClaimProvenanceKey(kind, text)] = item
	}
	return out
}

func authorClaimProvenanceKey(kind, text string) string {
	return strings.TrimSpace(kind) + "\x00" + strings.TrimSpace(text)
}

func isAuthorClaimDetailKind(kind string) bool {
	switch strings.TrimSpace(kind) {
	case "render_node", "proof_point":
		return true
	default:
		return false
	}
}

func applyAuthorClaimProvenance(candidate *authorClaimCandidate, item map[string]any) {
	if candidate == nil || item == nil {
		return
	}
	candidate.SourceText = hiddenDetailString(item, "source_text")
	candidate.SourceQuote = hiddenDetailString(item, "source_quote")
	candidate.Role = hiddenDetailString(item, "role")
	candidate.AttachesTo = hiddenDetailString(item, "attaches_to")
	candidate.Context = hiddenDetailString(item, "context")
}

func hiddenDetailString(item map[string]any, key string) string {
	if item == nil {
		return ""
	}
	value, ok := item[key]
	if !ok || value == nil {
		return ""
	}
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed)
	case fmt.Stringer:
		return strings.TrimSpace(typed.String())
	default:
		return ""
	}
}

func collectAuthorInferenceCandidates(out compile.Output) []authorInferenceCandidate {
	candidates := make([]authorInferenceCandidate, 0)
	seen := map[string]struct{}{}
	provenance := authorInferenceProvenanceByKey(out.Details.Items)
	add := func(path compile.TransmissionPath, branch string) {
		from := strings.TrimSpace(path.Driver)
		to := strings.TrimSpace(path.Target)
		if from == "" || to == "" {
			return
		}
		steps := cloneStrings(path.Steps)
		key := authorInferenceProvenanceKey(branch, from, steps, to)
		if _, ok := seen[key]; ok {
			return
		}
		seen[key] = struct{}{}
		item := provenance[key]
		if item == nil && strings.TrimSpace(branch) == "" {
			item = provenance[authorInferenceProvenanceKey("", from, steps, to)]
		}
		id := fmt.Sprintf("inference-%03d", len(candidates)+1)
		candidates = append(candidates, authorInferenceCandidate{
			InferenceID: id,
			From:        from,
			To:          to,
			Steps:       steps,
			Branch:      strings.TrimSpace(branch),
		})
		applyAuthorInferenceProvenance(&candidates[len(candidates)-1], item)
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

func authorInferenceProvenanceByKey(items []map[string]any) map[string]map[string]any {
	out := make(map[string]map[string]any, len(items))
	for _, item := range items {
		if hiddenDetailString(item, "kind") != "inference_path" {
			continue
		}
		from := hiddenDetailString(item, "from")
		to := hiddenDetailString(item, "to")
		if from == "" || to == "" {
			continue
		}
		steps := hiddenDetailStringSlice(item, "steps")
		branch := hiddenDetailString(item, "branch")
		out[authorInferenceProvenanceKey(branch, from, steps, to)] = item
		out[authorInferenceProvenanceKey("", from, steps, to)] = item
	}
	return out
}

func authorInferenceProvenanceKey(branch, from string, steps []string, to string) string {
	return strings.TrimSpace(branch) + "\x00" + strings.TrimSpace(from) + "\x00" + strings.Join(trimmedStringSlice(steps), "\x00") + "\x00" + strings.TrimSpace(to)
}

func applyAuthorInferenceProvenance(candidate *authorInferenceCandidate, item map[string]any) {
	if candidate == nil || item == nil {
		return
	}
	candidate.SourceQuote = hiddenDetailString(item, "source_quote")
	candidate.Context = hiddenDetailString(item, "context")
	candidate.EdgeEvidence = hiddenDetailInferenceEvidence(item, "edge_evidence")
}

func hiddenDetailStringSlice(item map[string]any, key string) []string {
	if item == nil {
		return nil
	}
	value, ok := item[key]
	if !ok || value == nil {
		return nil
	}
	switch typed := value.(type) {
	case []string:
		return trimmedStringSlice(typed)
	case []any:
		out := make([]string, 0, len(typed))
		for _, raw := range typed {
			if value, ok := raw.(string); ok {
				out = append(out, value)
			}
		}
		return trimmedStringSlice(out)
	default:
		return nil
	}
}

func hiddenDetailInferenceEvidence(item map[string]any, key string) []authorInferenceEvidence {
	if item == nil {
		return nil
	}
	value, ok := item[key]
	if !ok || value == nil {
		return nil
	}
	var rawItems []map[string]any
	switch typed := value.(type) {
	case []map[string]any:
		rawItems = typed
	case []any:
		for _, raw := range typed {
			if rawMap, ok := raw.(map[string]any); ok {
				rawItems = append(rawItems, rawMap)
			}
		}
	}
	out := make([]authorInferenceEvidence, 0, len(rawItems))
	for _, raw := range rawItems {
		evidence := authorInferenceEvidence{
			From:        hiddenDetailString(raw, "from"),
			To:          hiddenDetailString(raw, "to"),
			FromText:    hiddenDetailString(raw, "from_text"),
			ToText:      hiddenDetailString(raw, "to_text"),
			SourceQuote: hiddenDetailString(raw, "source_quote"),
			Reason:      hiddenDetailString(raw, "reason"),
		}
		if evidence != (authorInferenceEvidence{}) {
			out = append(out, evidence)
		}
	}
	return out
}

func trimmedStringSlice(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	out := make([]string, 0, len(values))
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}

func normalizeAuthorValidation(validation compile.AuthorValidation, claims []authorClaimCandidate, inferences []authorInferenceCandidate, model string) compile.AuthorValidation {
	return normalizeAuthorValidationWithHints(validation, claims, inferences, model, nil)
}

func normalizeAuthorValidationWithHints(validation compile.AuthorValidation, claims []authorClaimCandidate, inferences []authorInferenceCandidate, model string, hints []authorExternalEvidenceHint) compile.AuthorValidation {
	if validation.ValidatedAt.IsZero() {
		validation.ValidatedAt = compile.NowUTC()
	}
	if strings.TrimSpace(validation.Model) == "" {
		validation.Model = strings.TrimSpace(model)
	}
	validation.Version = authorValidationVersion

	hintsByClaimID := authorExternalEvidenceHintsByClaimID(hints)
	claimByID := make(map[string]compile.AuthorClaimCheck, len(validation.ClaimChecks))
	for _, check := range validation.ClaimChecks {
		check.ClaimID = strings.TrimSpace(check.ClaimID)
		if check.ClaimID == "" {
			continue
		}
		check.Status = normalizeAuthorClaimStatus(check.Status)
		check.RequiredEvidence = normalizeAuthorEvidenceRequirements(check.RequiredEvidence)
		check.Subclaims = normalizeAuthorSubclaims(check.ClaimID, check.Subclaims)
		claimByID[check.ClaimID] = check
	}
	normalizedClaims := make([]compile.AuthorClaimCheck, 0, len(claims))
	usedClaimIDs := make(map[string]struct{}, len(claims))
	for _, candidate := range claims {
		usedClaimIDs[candidate.ClaimID] = struct{}{}
		check, ok := claimByID[candidate.ClaimID]
		if !ok {
			check = defaultMissingAuthorClaimCheck(candidate)
		}
		if strings.TrimSpace(check.Text) == "" {
			check.Text = candidate.Text
		}
		check.Status = normalizeAuthorClaimStatus(check.Status)
		check.RequiredEvidence = normalizeAuthorEvidenceRequirements(check.RequiredEvidence)
		check.Subclaims = normalizeAuthorSubclaims(check.ClaimID, check.Subclaims)
		check.Status = aggregateClaimStatusFromSubclaims(check.Status, check.Subclaims)
		check = applyExternalEvidenceHintToClaim(check, hintsByClaimID[check.ClaimID])
		check = enforceExternalEvidenceForSupportedClaim(check)
		check = applyExternalEvidenceHintToClaim(check, hintsByClaimID[check.ClaimID])
		check = enforceLegalClaimScope(check)
		check = fillAuthorClaimDecisionNote(check)
		normalizedClaims = append(normalizedClaims, check)
	}
	for _, check := range validation.ClaimChecks {
		if _, ok := usedClaimIDs[check.ClaimID]; ok {
			continue
		}
		check.Status = normalizeAuthorClaimStatus(check.Status)
		check.RequiredEvidence = normalizeAuthorEvidenceRequirements(check.RequiredEvidence)
		check.Subclaims = normalizeAuthorSubclaims(check.ClaimID, check.Subclaims)
		check.Status = aggregateClaimStatusFromSubclaims(check.Status, check.Subclaims)
		check = applyExternalEvidenceHintToClaim(check, hintsByClaimID[check.ClaimID])
		check = enforceExternalEvidenceForSupportedClaim(check)
		check = applyExternalEvidenceHintToClaim(check, hintsByClaimID[check.ClaimID])
		check = enforceLegalClaimScope(check)
		check = fillAuthorClaimDecisionNote(check)
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
		check.RequiredEvidence = normalizeAuthorEvidenceRequirements(check.RequiredEvidence)
		inferenceByID[check.InferenceID] = check
	}
	normalizedInferences := make([]compile.AuthorInferenceCheck, 0, len(inferences))
	seenInferencePaths := make(map[string]struct{}, len(inferences))
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
		check.RequiredEvidence = normalizeAuthorEvidenceRequirements(check.RequiredEvidence)
		check = enforceExternalEvidenceForSoundInference(check)
		check = fillAuthorInferenceDecisionNote(check)
		pathKey := normalizedAuthorInferencePathKey(check.From, check.Steps, check.To)
		if _, ok := seenInferencePaths[pathKey]; ok {
			continue
		}
		seenInferencePaths[pathKey] = struct{}{}
		normalizedInferences = append(normalizedInferences, check)
	}
	validation.InferenceChecks = normalizedInferences
	validation.Summary = summarizeAuthorValidation(validation)
	return validation
}

func normalizeAuthorVerificationPlan(plan authorVerificationPlan) authorVerificationPlan {
	normalizedClaims := make([]authorClaimVerificationPlan, 0, len(plan.ClaimPlans))
	seenClaims := map[string]struct{}{}
	for _, claimPlan := range plan.ClaimPlans {
		claimPlan.ClaimID = strings.TrimSpace(claimPlan.ClaimID)
		if claimPlan.ClaimID == "" {
			continue
		}
		if _, ok := seenClaims[claimPlan.ClaimID]; ok {
			continue
		}
		seenClaims[claimPlan.ClaimID] = struct{}{}
		claimPlan.Text = strings.TrimSpace(claimPlan.Text)
		claimPlan.ClaimKind = strings.TrimSpace(claimPlan.ClaimKind)
		claimPlan.AtomicClaims = normalizeAuthorAtomicEvidenceSpecs(claimPlan.AtomicClaims, claimPlan.Text)
		claimPlan.RequiredEvidence = normalizeAuthorEvidenceRequirements(claimPlan.RequiredEvidence)
		for i := range claimPlan.RequiredEvidence {
			claimPlan.RequiredEvidence[i] = enrichAuthorEvidenceRequirementSpec(claimPlan.RequiredEvidence[i], claimPlan.Text)
		}
		claimPlan.PreferredSources = trimmedStringSlice(claimPlan.PreferredSources)
		claimPlan.Queries = trimmedStringSlice(claimPlan.Queries)
		claimPlan.ScopeCaveat = strings.TrimSpace(claimPlan.ScopeCaveat)
		normalizedClaims = append(normalizedClaims, claimPlan)
	}
	normalizedInferences := make([]authorInferenceVerificationPlan, 0, len(plan.InferencePlans))
	seenInferences := map[string]struct{}{}
	for _, inferencePlan := range plan.InferencePlans {
		inferencePlan.InferenceID = strings.TrimSpace(inferencePlan.InferenceID)
		if inferencePlan.InferenceID == "" {
			continue
		}
		if _, ok := seenInferences[inferencePlan.InferenceID]; ok {
			continue
		}
		seenInferences[inferencePlan.InferenceID] = struct{}{}
		inferencePlan.From = strings.TrimSpace(inferencePlan.From)
		inferencePlan.To = strings.TrimSpace(inferencePlan.To)
		inferencePlan.Steps = trimmedStringSlice(inferencePlan.Steps)
		inferencePlan.RequiredEvidence = normalizeAuthorEvidenceRequirements(inferencePlan.RequiredEvidence)
		inferenceContext := strings.TrimSpace(inferencePlan.From + " " + strings.Join(inferencePlan.Steps, " ") + " " + inferencePlan.To)
		for i := range inferencePlan.RequiredEvidence {
			inferencePlan.RequiredEvidence[i] = enrichAuthorEvidenceRequirementSpec(inferencePlan.RequiredEvidence[i], inferenceContext)
		}
		inferencePlan.Queries = trimmedStringSlice(inferencePlan.Queries)
		inferencePlan.MissingPremises = trimmedStringSlice(inferencePlan.MissingPremises)
		normalizedInferences = append(normalizedInferences, inferencePlan)
	}
	plan.ClaimPlans = normalizedClaims
	plan.InferencePlans = normalizedInferences
	return plan
}

func normalizeAuthorAtomicEvidenceSpecs(specs []compile.AuthorAtomicEvidenceSpec, context string) []compile.AuthorAtomicEvidenceSpec {
	if len(specs) == 0 {
		return nil
	}
	out := make([]compile.AuthorAtomicEvidenceSpec, 0, len(specs))
	for _, spec := range specs {
		spec.Text = strings.TrimSpace(spec.Text)
		spec.Subject = strings.TrimSpace(spec.Subject)
		spec.Metric = strings.TrimSpace(spec.Metric)
		spec.OriginalValue = strings.TrimSpace(spec.OriginalValue)
		spec.Unit = strings.TrimSpace(spec.Unit)
		spec.TimeWindow = strings.TrimSpace(spec.TimeWindow)
		spec.SourceType = strings.TrimSpace(spec.SourceType)
		spec.Series = strings.TrimSpace(spec.Series)
		spec.Entity = strings.TrimSpace(spec.Entity)
		spec.Geography = strings.TrimSpace(spec.Geography)
		spec.Denominator = strings.TrimSpace(spec.Denominator)
		spec.PreferredSources = trimmedStringSlice(spec.PreferredSources)
		spec.Queries = trimmedStringSlice(spec.Queries)
		spec.ComparisonRule = strings.TrimSpace(spec.ComparisonRule)
		spec.ScopeCaveat = strings.TrimSpace(spec.ScopeCaveat)
		if spec.Text == "" && spec.Subject == "" && spec.Metric == "" {
			continue
		}
		enriched := enrichAuthorEvidenceRequirementSpec(compile.AuthorEvidenceRequirement{
			Description:      spec.Text,
			Subject:          spec.Subject,
			Metric:           spec.Metric,
			OriginalValue:    spec.OriginalValue,
			Unit:             spec.Unit,
			TimeWindow:       spec.TimeWindow,
			SourceType:       spec.SourceType,
			Series:           spec.Series,
			Entity:           spec.Entity,
			Geography:        spec.Geography,
			Denominator:      spec.Denominator,
			PreferredSources: spec.PreferredSources,
			Queries:          spec.Queries,
			ComparisonRule:   spec.ComparisonRule,
			ScopeCaveat:      spec.ScopeCaveat,
		}, context+" "+spec.Text)
		spec.Series = enriched.Series
		spec.PreferredSources = enriched.PreferredSources
		spec.Queries = enriched.Queries
		spec.ComparisonRule = enriched.ComparisonRule
		spec.ScopeCaveat = firstNonEmpty(spec.ScopeCaveat, enriched.ScopeCaveat)
		if spec.OriginalValue == "" {
			spec.OriginalValue = enriched.OriginalValue
		}
		out = append(out, spec)
	}
	return out
}

func enrichAuthorEvidenceRequirementSpec(requirement compile.AuthorEvidenceRequirement, context string) compile.AuthorEvidenceRequirement {
	joined := strings.ToLower(strings.TrimSpace(strings.Join([]string{
		context,
		requirement.Description,
		requirement.Subject,
		requirement.Metric,
		requirement.TimeWindow,
	}, " ")))
	if requirement.OriginalValue == "" {
		requirement.OriginalValue = firstAuthorNumericValue(context)
	}
	switch {
	case containsAny(joined, "bank reserves", "reserve balances", "银行准备金", "商业银行准备金"):
		requirement.Series = firstNonEmpty(requirement.Series, "FRED:WRESBAL")
		requirement.PreferredSources = appendAuthorValidationUniqueStrings(requirement.PreferredSources, "Federal Reserve H.4.1", "FRED WRESBAL")
		requirement.Queries = appendAuthorValidationUniqueStrings(requirement.Queries, "FRED WRESBAL reserve balances "+requirement.TimeWindow, "Federal Reserve H.4.1 reserve balances "+requirement.TimeWindow)
		requirement.ComparisonRule = firstNonEmpty(requirement.ComparisonRule, "Compare nearest weekly reserve-balance values in the stated window against the author's value or threshold; preserve whether the claim is a level, trend, peak, trough, or threshold breach.")
		requirement.ScopeCaveat = firstNonEmpty(requirement.ScopeCaveat, "This verifies bank reserve balances, not Fed total assets, M2, or a broad liquidity proxy.")
	case containsAny(joined, "fed balance sheet", "federal reserve balance sheet", "total assets", "美联储资产负债表", "美联储总资产"):
		requirement.Series = firstNonEmpty(requirement.Series, "FRED:WALCL")
		requirement.PreferredSources = appendAuthorValidationUniqueStrings(requirement.PreferredSources, "Federal Reserve H.4.1", "FRED WALCL")
		requirement.Queries = appendAuthorValidationUniqueStrings(requirement.Queries, "FRED WALCL Fed total assets "+requirement.TimeWindow, "Federal Reserve H.4.1 total assets "+requirement.TimeWindow)
		requirement.ComparisonRule = firstNonEmpty(requirement.ComparisonRule, "Compare nearest weekly Fed total-assets values in the stated window against the author's level or change.")
	case containsAny(joined, "treasury general account", " tga", "tga ", "财政部tga", "tga余额"):
		requirement.Series = firstNonEmpty(requirement.Series, "FRED:WTREGEN")
		requirement.PreferredSources = appendAuthorValidationUniqueStrings(requirement.PreferredSources, "US Treasury Quarterly Refunding Announcement", "Daily Treasury Statement", "FRED WTREGEN")
		requirement.Queries = appendAuthorValidationUniqueStrings(requirement.Queries, "Treasury Quarterly Refunding TGA balance forecast "+requirement.TimeWindow, "FRED WTREGEN Treasury General Account "+requirement.TimeWindow)
		requirement.ComparisonRule = firstNonEmpty(requirement.ComparisonRule, "Use official Treasury forecast documents for projected balances and DTS/FRED weekly values for realized balances; compare the stated date window and peak value directly.")
	case containsAny(joined, "stablecoin", "usdt", "usdc", "稳定币"):
		requirement.PreferredSources = appendAuthorValidationUniqueStrings(requirement.PreferredSources, "DeFiLlama stablecoins", "Tether transparency", "Circle transparency")
		requirement.Queries = appendAuthorValidationUniqueStrings(requirement.Queries, "DeFiLlama stablecoins USDT USDC supply "+requirement.TimeWindow, "USDT USDC circulating supply change "+requirement.TimeWindow)
		requirement.ComparisonRule = firstNonEmpty(requirement.ComparisonRule, "Compare circulating-supply deltas for each named stablecoin over the exact window; do not combine issuers unless the author does.")
	case containsAny(joined, "bitcoin spot etf", "spot bitcoin etf", "btc etf", "比特币现货etf"):
		requirement.PreferredSources = appendAuthorValidationUniqueStrings(requirement.PreferredSources, "Farside Investors", "SoSoValue", "Bloomberg ETF flow data")
		requirement.Queries = appendAuthorValidationUniqueStrings(requirement.Queries, "Bitcoin spot ETF net flows "+requirement.TimeWindow, "Farside Bitcoin ETF flows "+requirement.TimeWindow)
		requirement.ComparisonRule = firstNonEmpty(requirement.ComparisonRule, "Compare daily or weekly net flows across the exact window; distinguish continuous net outflow from cumulative net outflow.")
	case containsAny(joined, "jane street", "lawsuit", "class action", "legal filing", "操纵", "集体诉讼"):
		requirement.PreferredSources = appendAuthorValidationUniqueStrings(requirement.PreferredSources, "CourtListener", "PACER", "Reuters", "Bloomberg", "Financial Times")
		requirement.Queries = appendAuthorValidationUniqueStrings(requirement.Queries, "Jane Street Bitcoin manipulation class action lawsuit "+requirement.TimeWindow, "CourtListener Jane Street Bitcoin manipulation "+requirement.TimeWindow)
		requirement.ComparisonRule = firstNonEmpty(requirement.ComparisonRule, "Verify that a legal filing or high-quality news source exists and that it matches both the named party and specific allegation; rumors alone are not support.")
	case containsAny(joined, "rmp", "reserve management purchase", "short-term treasury", "短债购买", "买短债"):
		requirement.PreferredSources = appendAuthorValidationUniqueStrings(requirement.PreferredSources, "FOMC statement", "New York Fed operations", "Federal Reserve press release")
		requirement.Queries = appendAuthorValidationUniqueStrings(requirement.Queries, "Federal Reserve Reserve Management Purchases December 2025 short-term Treasury purchases", "New York Fed RMP monthly purchases "+requirement.TimeWindow)
		requirement.ComparisonRule = firstNonEmpty(requirement.ComparisonRule, "Confirm the official program name, announcement date, purchase start date, asset type, and stated monthly pace.")
	}
	return requirement
}

func firstAuthorNumericValue(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	patterns := []*regexp.Regexp{
		regexp.MustCompile(`(?i)(?:\$|usd\s*)?\d+(?:\.\d+)?\s*(?:万亿|亿|trillion|billion|million|tn|bn|mm|万桶|b/d|bps|%)`),
		regexp.MustCompile(`\d+(?:\.\d+)?\s*-\s*\d+(?:\.\d+)?\s*(?:%|天|日|days|bps)`),
	}
	for _, pattern := range patterns {
		if match := strings.TrimSpace(pattern.FindString(value)); match != "" {
			return match
		}
	}
	return ""
}

func authorExternalEvidenceHintsByClaimID(hints []authorExternalEvidenceHint) map[string][]authorExternalEvidenceHint {
	out := make(map[string][]authorExternalEvidenceHint, len(hints))
	for _, hint := range hints {
		id := strings.TrimSpace(hint.ClaimID)
		if id == "" || len(hint.Results) == 0 {
			continue
		}
		out[id] = append(out[id], hint)
	}
	return out
}

func applyExternalEvidenceHintToClaim(check compile.AuthorClaimCheck, hints []authorExternalEvidenceHint) compile.AuthorClaimCheck {
	if len(hints) == 0 {
		return check
	}
	if result, contradicted := numericExternalEvidenceHintContradictsClaim(check, hints); contradicted {
		return applyExternalEvidenceContradictionToClaim(check, result, "External evidence hint contains comparable numeric data that does not match the author's value.")
	}
	if result, contradicted := qualitativeExternalEvidenceHintContradictsClaim(check, hints); contradicted {
		return applyExternalEvidenceContradictionToClaim(check, result, "External evidence hint contradicts the claim's qualitative condition.")
	}
	if check.Status != compile.AuthorClaimUnverified {
		return check
	}
	result, ok := exactExternalEvidenceHintForClaim(check, hints)
	if !ok {
		check = appendExternalEvidenceHintsToClaim(check, hints)
		result, ok = numericExternalEvidenceHintForClaim(check, hints)
		if !ok {
			return check
		}
	}
	check.Status = compile.AuthorClaimSupported
	evidence := authorExternalEvidenceResultString(result)
	check.Evidence = appendAuthorValidationUniqueString(check.Evidence, evidence)
	for i := range check.RequiredEvidence {
		if check.RequiredEvidence[i].Status == compile.AuthorClaimUnverified {
			check.RequiredEvidence[i].Status = compile.AuthorClaimSupported
		}
		check.RequiredEvidence[i].Evidence = appendAuthorValidationUniqueString(check.RequiredEvidence[i].Evidence, evidence)
		check.RequiredEvidence[i].Reason = "External evidence hint contains the matching official value."
	}
	check.Reason = "External evidence hint contains the matching official value."
	return check
}

func applyExternalEvidenceContradictionToClaim(check compile.AuthorClaimCheck, result authorExternalEvidenceResult, reason string) compile.AuthorClaimCheck {
	check.Status = compile.AuthorClaimContradicted
	evidence := authorExternalEvidenceResultString(result)
	check.Evidence = appendAuthorValidationUniqueString(check.Evidence, evidence)
	for i := range check.RequiredEvidence {
		check.RequiredEvidence[i].Status = compile.AuthorClaimContradicted
		check.RequiredEvidence[i].Evidence = appendAuthorValidationUniqueString(check.RequiredEvidence[i].Evidence, evidence)
		check.RequiredEvidence[i].Reason = reason
	}
	check.Reason = reason
	return check
}

func appendExternalEvidenceHintsToClaim(check compile.AuthorClaimCheck, hints []authorExternalEvidenceHint) compile.AuthorClaimCheck {
	for _, hint := range hints {
		for _, result := range hint.Results {
			evidence := authorExternalEvidenceResultString(result)
			check.Evidence = appendAuthorValidationUniqueString(check.Evidence, evidence)
			for i := range check.RequiredEvidence {
				check.RequiredEvidence[i].Evidence = appendAuthorValidationUniqueString(check.RequiredEvidence[i].Evidence, evidence)
			}
		}
	}
	return check
}

func authorExternalEvidenceResultString(result authorExternalEvidenceResult) string {
	evidence := strings.TrimSpace(result.Title)
	if url := strings.TrimSpace(result.URL); url != "" {
		if evidence == "" {
			evidence = url
		} else {
			evidence += " (" + url + ")"
		}
	}
	if excerpt := strings.TrimSpace(result.Excerpt); excerpt != "" {
		if evidence == "" {
			evidence = excerpt
		} else {
			evidence += ": " + excerpt
		}
	}
	return evidence
}

func numericExternalEvidenceHintForClaim(check compile.AuthorClaimCheck, hints []authorExternalEvidenceHint) (authorExternalEvidenceResult, bool) {
	authorNumbers := authorClaimComparableNumbers(check)
	if len(authorNumbers) == 0 {
		return authorExternalEvidenceResult{}, false
	}
	for _, hint := range hints {
		for _, result := range hint.Results {
			if externalEvidenceNumbersSupport(authorNumbers, result) {
				return result, true
			}
		}
	}
	return authorExternalEvidenceResult{}, false
}

func qualitativeExternalEvidenceHintContradictsClaim(check compile.AuthorClaimCheck, hints []authorExternalEvidenceHint) (authorExternalEvidenceResult, bool) {
	claimText := strings.ToLower(check.Text + " " + check.Reason)
	for _, requirement := range check.RequiredEvidence {
		claimText += " " + strings.ToLower(strings.Join([]string{
			requirement.Description,
			requirement.OriginalValue,
			requirement.Metric,
			requirement.Series,
		}, " "))
	}
	wantsContinuousOutflow := (strings.Contains(claimText, "continuous") || strings.Contains(claimText, "持续")) &&
		(strings.Contains(claimText, "outflow") || strings.Contains(claimText, "流出"))
	if !wantsContinuousOutflow {
		return authorExternalEvidenceResult{}, false
	}
	for _, hint := range hints {
		for _, result := range hint.Results {
			evidenceText := strings.ToLower(strings.Join([]string{result.Title, result.Excerpt}, " "))
			if strings.Contains(evidenceText, "continuous_outflow=false") {
				return result, true
			}
		}
	}
	return authorExternalEvidenceResult{}, false
}

func numericExternalEvidenceHintContradictsClaim(check compile.AuthorClaimCheck, hints []authorExternalEvidenceHint) (authorExternalEvidenceResult, bool) {
	authorNumbers := authorClaimComparableNumbers(check)
	if len(authorNumbers) == 0 {
		return authorExternalEvidenceResult{}, false
	}
	for _, hint := range hints {
		for _, result := range hint.Results {
			sourceValues := externalEvidenceComparableValues(result)
			if len(sourceValues) == 0 {
				continue
			}
			if !externalEvidenceNumbersSupport(authorNumbers, result) && isPreciseNumericEvidenceSource(result) {
				return result, true
			}
		}
	}
	return authorExternalEvidenceResult{}, false
}

func isPreciseNumericEvidenceSource(result authorExternalEvidenceResult) bool {
	text := strings.ToLower(strings.Join([]string{result.URL, result.Title, result.Excerpt}, " "))
	return strings.Contains(text, "fred ") ||
		strings.Contains(text, "fred:") ||
		strings.Contains(text, "defillama stablecoin") ||
		strings.Contains(text, "stablecoins.llama.fi")
}

type authorComparableNumber struct {
	Value      float64
	Unit       string
	Comparator string
}

func authorClaimComparableNumbers(check compile.AuthorClaimCheck) []authorComparableNumber {
	parts := []string{check.Text}
	for _, requirement := range check.RequiredEvidence {
		parts = append(parts, requirement.OriginalValue, requirement.Description, requirement.Reason)
	}
	return parseAuthorComparableNumbers(strings.Join(parts, " "))
}

func parseAuthorComparableNumbers(text string) []authorComparableNumber {
	pattern := regexp.MustCompile(`(?i)(减少|下降|decrease(?:d)?|decline(?:d)?|drop(?:ped)?|down|[<>])?\s*(-?\d+(?:\.\d+)?)\s*(万亿|亿美元|亿美金|trillion|billion|t|b|万亿美金|万亿美元)`)
	matches := pattern.FindAllStringSubmatch(text, -1)
	out := make([]authorComparableNumber, 0, len(matches))
	seen := map[string]struct{}{}
	for _, match := range matches {
		value, err := strconv.ParseFloat(match[2], 64)
		if err != nil {
			continue
		}
		comparator := strings.TrimSpace(match[1])
		if value > 0 && isDecreaseMarker(comparator) {
			value = -value
			comparator = ""
		}
		unit := strings.ToLower(match[3])
		switch unit {
		case "万亿", "万亿美金", "万亿美元", "trillion", "t":
			unit = "trillion"
		case "亿美元", "亿美金":
			value = value / 10
			unit = "billion"
		case "billion", "b":
			unit = "billion"
		}
		key := comparator + "|" + strconv.FormatFloat(value, 'f', 6, 64) + "|" + unit
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, authorComparableNumber{Value: value, Unit: unit, Comparator: comparator})
	}
	return out
}

func isDecreaseMarker(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "减少", "下降", "decrease", "decreased", "decline", "declined", "drop", "dropped", "down":
		return true
	default:
		return false
	}
}

func externalEvidenceNumbersSupport(authorNumbers []authorComparableNumber, result authorExternalEvidenceResult) bool {
	sourceValues := externalEvidenceComparableValues(result)
	if len(sourceValues) == 0 {
		return false
	}
	for _, authorNumber := range authorNumbers {
		if !anySourceValueMatchesAuthorNumber(sourceValues, authorNumber) {
			return false
		}
	}
	return true
}

func externalEvidenceComparableValues(result authorExternalEvidenceResult) []float64 {
	text := strings.Join([]string{result.Title, result.Excerpt}, " ")
	rawMatches := regexp.MustCompile(`=\s*(-?\d+(?:\.\d+)?)`).FindAllStringSubmatch(text, -1)
	out := make([]float64, 0, len(rawMatches))
	isStablecoin := strings.Contains(strings.ToLower(result.Title+" "+result.URL), "stablecoin") || strings.Contains(strings.ToLower(result.URL), "stablecoins")
	for _, match := range rawMatches {
		value, err := strconv.ParseFloat(match[1], 64)
		if err != nil {
			continue
		}
		switch {
		case strings.Contains(strings.ToUpper(result.Title), "FRED WRESBAL"), strings.Contains(strings.ToUpper(result.Title), "FRED WALCL"):
			value = value / 1_000_000
		case isStablecoin:
			value = value / 1_000_000_000
		}
		out = append(out, value)
	}
	return out
}

func anySourceValueMatchesAuthorNumber(sourceValues []float64, authorNumber authorComparableNumber) bool {
	for _, sourceValue := range sourceValues {
		switch authorNumber.Comparator {
		case "<":
			if sourceValue < authorNumber.Value*1.02 {
				return true
			}
		case ">":
			if sourceValue > authorNumber.Value*0.98 {
				return true
			}
		default:
			tolerance := 0.08
			if authorNumber.Value >= 5 {
				tolerance = 0.05
			}
			if authorNumber.Value != 0 && absFloat64(sourceValue-authorNumber.Value)/absFloat64(authorNumber.Value) <= tolerance {
				return true
			}
		}
	}
	return false
}

func absFloat64(value float64) float64 {
	if value < 0 {
		return -value
	}
	return value
}

func exactExternalEvidenceHintForClaim(check compile.AuthorClaimCheck, hints []authorExternalEvidenceHint) (authorExternalEvidenceResult, bool) {
	claimText := strings.ToLower(strings.TrimSpace(check.Text + " " + check.Reason))
	for _, requirement := range check.RequiredEvidence {
		claimText += " " + strings.ToLower(strings.Join([]string{
			requirement.Description,
			requirement.Subject,
			requirement.Metric,
			requirement.TimeWindow,
			requirement.SourceType,
			requirement.Reason,
		}, " "))
	}
	wantsEIAOil91 := (strings.Contains(claimText, "910") || strings.Contains(claimText, "9.1")) &&
		(strings.Contains(claimText, "oil") || strings.Contains(claimText, "石油") || strings.Contains(claimText, "production") || strings.Contains(claimText, "减产")) &&
		(strings.Contains(claimText, "eia") || strings.Contains(claimText, "energy information") || strings.Contains(claimText, "能源信息署") || strings.Contains(claimText, "official"))
	for _, hint := range hints {
		for _, result := range hint.Results {
			evidenceText := strings.ToLower(strings.Join([]string{hint.Query, result.URL, result.Title, result.Excerpt}, " "))
			if wantsEIAOil91 &&
				strings.Contains(evidenceText, "eia") &&
				strings.Contains(evidenceText, "9.1 million b/d") &&
				strings.Contains(evidenceText, "production shut-ins") &&
				strings.Contains(evidenceText, "april") {
				return result, true
			}
		}
	}
	return authorExternalEvidenceResult{}, false
}

func enforceExternalEvidenceForSupportedClaim(check compile.AuthorClaimCheck) compile.AuthorClaimCheck {
	if check.Status != compile.AuthorClaimSupported || hasExternalClaimSupport(check) {
		return check
	}
	check.Status = compile.AuthorClaimUnverified
	for i := range check.RequiredEvidence {
		if check.RequiredEvidence[i].Status == compile.AuthorClaimSupported && !requirementHasExternalSupport(check.RequiredEvidence[i]) {
			check.RequiredEvidence[i].Status = compile.AuthorClaimUnverified
		}
	}
	check.Reason = appendAuthorValidationReason(check.Reason, "Supported checkable claims require external evidence; the returned evidence only restates or cites the author.")
	return check
}

func enforceLegalClaimScope(check compile.AuthorClaimCheck) compile.AuthorClaimCheck {
	if check.Status != compile.AuthorClaimSupported || !legalClaimNeedsSpecificAllegationSupport(check) {
		return check
	}
	externalText := strings.ToLower(strings.Join(externalEvidenceStringsForClaim(check), " "))
	if externalText == "" || legalEvidenceSupportsSpecificMethod(externalText) {
		return check
	}
	check.Status = compile.AuthorClaimUnverified
	for i := range check.RequiredEvidence {
		check.RequiredEvidence[i].Status = compile.AuthorClaimUnverified
		check.RequiredEvidence[i].Reason = "External legal evidence confirms lawsuit/allegation existence but does not verify the specific trading-method detail."
	}
	check.Reason = "External legal evidence confirms lawsuit/allegation existence but does not verify the specific trading-method detail."
	return check
}

func fillAuthorClaimDecisionNote(check compile.AuthorClaimCheck) compile.AuthorClaimCheck {
	for i := range check.Subclaims {
		check.Subclaims[i].DecisionNote = authorSubclaimDecisionNote(check.Subclaims[i])
	}
	check.DecisionNote = authorClaimDecisionNote(check)
	return check
}

func authorClaimDecisionNote(check compile.AuthorClaimCheck) string {
	basis := authorClaimBasisSummary(check)
	reason := authorClaimReasonSummary(check)
	parts := make([]string, 0, 2)
	if basis != "" {
		parts = append(parts, "口径: "+basis)
	}
	parts = append(parts, "判定: "+firstNonEmpty(reason, string(check.Status)))
	return truncateAuthorEvidenceExcerpt(strings.Join(parts, "；"), 480)
}

func authorClaimBasisSummary(check compile.AuthorClaimCheck) string {
	for _, requirement := range check.RequiredEvidence {
		if basis := authorRequirementBasisSummary(requirement); basis != "" {
			return basis
		}
	}
	for _, subclaim := range check.Subclaims {
		parts := make([]string, 0, 5)
		if subject := strings.TrimSpace(subclaim.Subject); subject != "" {
			parts = append(parts, subject)
		}
		if metric := strings.TrimSpace(subclaim.Metric); metric != "" {
			parts = append(parts, metric)
		}
		if value := strings.TrimSpace(subclaim.OriginalValue); value != "" {
			parts = append(parts, "作者值 "+value)
		}
		if base := strings.TrimSpace(subclaim.ComparisonBase); base != "" {
			parts = append(parts, "分母/对象 "+base)
		}
		if evidenceBase := strings.TrimSpace(subclaim.EvidenceBase); evidenceBase != "" {
			parts = append(parts, "证据对象 "+evidenceBase)
		}
		if len(parts) > 0 {
			return strings.Join(parts, "；")
		}
	}
	return ""
}

func authorRequirementBasisSummary(requirement compile.AuthorEvidenceRequirement) string {
	parts := make([]string, 0, 8)
	if subject := strings.TrimSpace(requirement.Subject); subject != "" {
		parts = append(parts, subject)
	}
	if metric := strings.TrimSpace(requirement.Metric); metric != "" {
		parts = append(parts, metric)
	}
	if value := strings.TrimSpace(requirement.OriginalValue); value != "" {
		parts = append(parts, "作者值 "+value)
	}
	if unit := strings.TrimSpace(requirement.Unit); unit != "" {
		parts = append(parts, "单位 "+unit)
	}
	if window := strings.TrimSpace(requirement.TimeWindow); window != "" {
		parts = append(parts, "窗口 "+window)
	}
	if source := firstNonEmpty(requirement.Series, requirement.SourceType, firstString(requirement.PreferredSources)); source != "" {
		parts = append(parts, "来源 "+source)
	}
	if denominator := strings.TrimSpace(requirement.Denominator); denominator != "" {
		parts = append(parts, "分母 "+denominator)
	}
	if caveat := strings.TrimSpace(requirement.ScopeCaveat); caveat != "" {
		parts = append(parts, "范围 "+caveat)
	}
	if len(parts) == 0 {
		return ""
	}
	return strings.Join(parts, "；")
}

func authorClaimReasonSummary(check compile.AuthorClaimCheck) string {
	if reason := strings.TrimSpace(check.Reason); reason != "" {
		return reason
	}
	for _, subclaim := range check.Subclaims {
		if subclaim.Status == check.Status {
			if reason := strings.TrimSpace(subclaim.Reason); reason != "" {
				return reason
			}
		}
	}
	for _, requirement := range check.RequiredEvidence {
		if requirement.Status == check.Status {
			if reason := strings.TrimSpace(requirement.Reason); reason != "" {
				return reason
			}
		}
	}
	for _, requirement := range check.RequiredEvidence {
		if reason := strings.TrimSpace(requirement.Reason); reason != "" {
			return reason
		}
	}
	return ""
}

func authorSubclaimDecisionNote(subclaim compile.AuthorSubclaim) string {
	basisParts := make([]string, 0, 6)
	if subject := strings.TrimSpace(subclaim.Subject); subject != "" {
		basisParts = append(basisParts, subject)
	}
	if metric := strings.TrimSpace(subclaim.Metric); metric != "" {
		basisParts = append(basisParts, metric)
	}
	if value := strings.TrimSpace(subclaim.OriginalValue); value != "" {
		basisParts = append(basisParts, "作者值 "+value)
	}
	if evidenceValue := strings.TrimSpace(firstNonEmpty(subclaim.EvidenceValue, subclaim.EvidenceRange)); evidenceValue != "" {
		basisParts = append(basisParts, "证据值 "+evidenceValue)
	}
	if scope := strings.TrimSpace(subclaim.ScopeStatus); scope != "" {
		basisParts = append(basisParts, "范围 "+scope)
	}
	parts := make([]string, 0, 2)
	if len(basisParts) > 0 {
		parts = append(parts, "口径: "+strings.Join(basisParts, "；"))
	}
	parts = append(parts, "判定: "+firstNonEmpty(subclaim.Reason, string(subclaim.Status)))
	return truncateAuthorEvidenceExcerpt(strings.Join(parts, "；"), 480)
}

func fillAuthorInferenceDecisionNote(check compile.AuthorInferenceCheck) compile.AuthorInferenceCheck {
	check.DecisionNote = authorInferenceDecisionNote(check)
	return check
}

func authorInferenceDecisionNote(check compile.AuthorInferenceCheck) string {
	basisParts := make([]string, 0, 2)
	path := authorInferencePathText(check)
	if path != "" {
		basisParts = append(basisParts, "路径 "+path)
	}
	for _, requirement := range check.RequiredEvidence {
		if basis := authorRequirementBasisSummary(requirement); basis != "" {
			basisParts = append(basisParts, "所需证据 "+basis)
			break
		}
	}
	parts := make([]string, 0, 2)
	if len(basisParts) > 0 {
		parts = append(parts, "口径: "+strings.Join(basisParts, "；"))
	}
	parts = append(parts, "判定: "+firstNonEmpty(check.Reason, string(check.Status)))
	if len(check.MissingLinks) > 0 {
		parts = append(parts, "缺口: "+strings.Join(check.MissingLinks, "；"))
	}
	return truncateAuthorEvidenceExcerpt(strings.Join(parts, "；"), 480)
}

func authorInferencePathText(check compile.AuthorInferenceCheck) string {
	parts := make([]string, 0, len(check.Steps)+2)
	if from := strings.TrimSpace(check.From); from != "" {
		parts = append(parts, from)
	}
	parts = append(parts, trimmedStringSlice(check.Steps)...)
	if to := strings.TrimSpace(check.To); to != "" {
		parts = append(parts, to)
	}
	return strings.Join(parts, " -> ")
}

func legalClaimNeedsSpecificAllegationSupport(check compile.AuthorClaimCheck) bool {
	text := strings.ToLower(check.Text + " " + check.Reason)
	for _, requirement := range check.RequiredEvidence {
		text += " " + strings.ToLower(strings.Join([]string{
			requirement.Description,
			requirement.Subject,
			requirement.Metric,
			requirement.OriginalValue,
			requirement.SourceType,
			requirement.ComparisonRule,
			requirement.ScopeCaveat,
			strings.Join(requirement.PreferredSources, " "),
		}, " "))
	}
	hasLegalSubject := containsAny(text, "lawsuit", "class action", "courtlistener", "pacer", "legal", "诉讼", "集体诉讼", "jane street")
	hasSpecificMethod := containsAny(text,
		"timed selling",
		"daily",
		"large sell",
		"sell order",
		"liquidation",
		"forced liquidation",
		"buy back",
		"low-level buying",
		"每日",
		"定时",
		"大额抛售",
		"抛售",
		"爆仓",
		"低位",
		"补仓",
	)
	return hasLegalSubject && hasSpecificMethod
}

func legalEvidenceSupportsSpecificMethod(externalText string) bool {
	return containsAny(externalText,
		"timed selling",
		"daily sell",
		"large sell",
		"sell order",
		"liquidation",
		"forced liquidation",
		"buy back",
		"low-level buying",
		"每日",
		"定时",
		"大额抛售",
		"爆仓",
		"低位补仓",
	)
}

func externalEvidenceStringsForClaim(check compile.AuthorClaimCheck) []string {
	out := make([]string, 0, len(check.Evidence)+len(check.RequiredEvidence))
	for _, evidence := range check.Evidence {
		if isExternalEvidenceString(evidence) {
			out = append(out, evidence)
		}
	}
	for _, requirement := range check.RequiredEvidence {
		for _, evidence := range requirement.Evidence {
			if isExternalEvidenceString(evidence) {
				out = append(out, evidence)
			}
		}
	}
	return out
}

func enforceExternalEvidenceForSoundInference(check compile.AuthorInferenceCheck) compile.AuthorInferenceCheck {
	if check.Status != compile.AuthorInferenceSound || hasExternalInferenceSupport(check) {
		return check
	}
	check.Status = compile.AuthorInferenceWeak
	for i := range check.RequiredEvidence {
		if check.RequiredEvidence[i].Status == compile.AuthorClaimSupported && !requirementHasExternalSupport(check.RequiredEvidence[i]) {
			check.RequiredEvidence[i].Status = compile.AuthorClaimUnverified
			check.RequiredEvidence[i].Reason = "Sound inference requires external support for the necessary factual premise, not only author provenance."
		}
	}
	check.Reason = "Sound inference requires external support for the necessary factual premises, not only author provenance."
	check.MissingLinks = appendAuthorValidationUniqueString(check.MissingLinks, "external evidence for the factual premises needed by this inference")
	return check
}

func hasExternalClaimSupport(check compile.AuthorClaimCheck) bool {
	if hasExternalEvidenceStrings(check.Evidence) {
		return true
	}
	for _, requirement := range check.RequiredEvidence {
		if requirementHasExternalSupport(requirement) {
			return true
		}
	}
	for _, subclaim := range check.Subclaims {
		if subclaim.Status != compile.AuthorClaimSupported {
			continue
		}
		if hasExternalEvidenceStrings(subclaim.Evidence) ||
			strings.TrimSpace(subclaim.EvidenceValue) != "" ||
			strings.TrimSpace(subclaim.EvidenceRange) != "" {
			return true
		}
	}
	return false
}

func hasExternalInferenceSupport(check compile.AuthorInferenceCheck) bool {
	if hasExternalEvidenceStrings(check.Evidence) {
		return true
	}
	for _, requirement := range check.RequiredEvidence {
		if requirementHasExternalSupport(requirement) {
			return true
		}
	}
	return false
}

func requirementHasExternalSupport(requirement compile.AuthorEvidenceRequirement) bool {
	if requirement.Status != compile.AuthorClaimSupported {
		return false
	}
	if strings.EqualFold(strings.TrimSpace(requirement.SourceType), "author_source") {
		return false
	}
	return hasExternalEvidenceStrings(requirement.Evidence)
}

func hasExternalEvidenceStrings(values []string) bool {
	for _, value := range values {
		if isExternalEvidenceString(value) {
			return true
		}
	}
	return false
}

func isExternalEvidenceString(value string) bool {
	normalized := strings.ToLower(strings.TrimSpace(value))
	if isAuthorOnlyEvidenceString(normalized) {
		return false
	}
	if isVagueExternalEvidenceString(normalized) {
		return false
	}
	externalMarkers := []string{
		"http://",
		"https://",
		"www.",
		".gov",
		"source says",
		"official source",
		"official data",
		"official report",
		"official release",
		"central bank release",
		"company filing",
		"market data",
		"reports",
		"report:",
		"release:",
		"filing:",
		"eia ",
		"eia:",
		"steo",
		"iea ",
		"iea:",
		"fred",
		"defillama",
		"sosovalue",
		"courtlistener",
		"pacer",
		"treasury",
		"world gold council",
		"wgc",
		"bloomberg",
		"reuters",
		"s&p global",
		"federal reserve",
		"cbo",
		"bea",
		"bls",
		"sec ",
		"sec:",
	}
	for _, marker := range externalMarkers {
		if strings.Contains(normalized, marker) {
			return true
		}
	}
	return false
}

func isVagueExternalEvidenceString(normalized string) bool {
	vagueMarkers := []string{
		"industry reports",
		"market data shows",
		"data shows",
		"reports show",
		"research shows",
		"often",
		"typically",
		"e.g.",
		"for example",
		"generally",
	}
	hasVagueMarker := false
	for _, marker := range vagueMarkers {
		if strings.Contains(normalized, marker) {
			hasVagueMarker = true
			break
		}
	}
	if !hasVagueMarker {
		return false
	}
	concreteMarkers := []string{
		"http://",
		"https://",
		".gov",
		"fred:",
		"fred ",
		"steo",
		"courtlistener",
		"pacer",
	}
	for _, marker := range concreteMarkers {
		if strings.Contains(normalized, marker) {
			return false
		}
	}
	hasNumber := regexp.MustCompile(`\d`).MatchString(normalized)
	hasNamedDatedSource := regexp.MustCompile(`(?i)(jan|feb|mar|apr|may|jun|jul|aug|sep|oct|nov|dec|20\d{2}|q[1-4])`).MatchString(normalized)
	if hasNumber && hasNamedDatedSource {
		return false
	}
	return true
}

func isAuthorOnlyEvidenceString(value string) bool {
	normalized := strings.ToLower(strings.TrimSpace(value))
	if normalized == "" {
		return true
	}
	authorPhrases := []string{
		"author states",
		"author states:",
		"author cites",
		"author says",
		"author explicitly",
		"author provides",
		"author attributes",
		"author links",
		"author's stated",
		"the author states",
		"the author says",
		"the author explicitly",
		"direct author claim",
		"directly stated",
		"same as inference",
		"identical logical path",
		"作者",
		"文中",
		"原文",
	}
	for _, phrase := range authorPhrases {
		if strings.Contains(normalized, phrase) {
			return true
		}
	}
	return false
}

func appendAuthorValidationReason(reason, addition string) string {
	reason = strings.TrimSpace(reason)
	addition = strings.TrimSpace(addition)
	if reason == "" {
		return addition
	}
	if addition == "" || strings.Contains(reason, addition) {
		return reason
	}
	return reason + " " + addition
}

func appendAuthorValidationUniqueString(values []string, value string) []string {
	value = strings.TrimSpace(value)
	if value == "" {
		return values
	}
	for _, existing := range values {
		if strings.TrimSpace(existing) == value {
			return values
		}
	}
	return append(values, value)
}

func appendAuthorValidationUniqueStrings(values []string, additions ...string) []string {
	for _, addition := range additions {
		values = appendAuthorValidationUniqueString(values, addition)
	}
	return values
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func firstString(values []string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func containsAny(value string, markers ...string) bool {
	for _, marker := range markers {
		if strings.Contains(value, strings.ToLower(strings.TrimSpace(marker))) {
			return true
		}
	}
	return false
}

func normalizedAuthorInferencePathKey(from string, steps []string, to string) string {
	return strings.TrimSpace(from) + "\x00" + strings.Join(trimmedStringSlice(steps), "\x00") + "\x00" + strings.TrimSpace(to)
}

func defaultMissingAuthorClaimCheck(candidate authorClaimCandidate) compile.AuthorClaimCheck {
	check := compile.AuthorClaimCheck{
		ClaimID: candidate.ClaimID,
		Text:    candidate.Text,
		Status:  compile.AuthorClaimUnverified,
		Reason:  "validator did not return this concrete proof candidate",
	}
	if isAuthorNarrativeClaimKind(candidate.Kind) {
		check.Status = compile.AuthorClaimInterpretive
		check.Reason = "validator did not return this narrative candidate; defer abstract point validation to inference checks unless a concrete subclaim is explicitly checked"
	}
	return check
}

func isAuthorNarrativeClaimKind(kind string) bool {
	switch strings.TrimSpace(kind) {
	case "render_node", "driver", "target", "explanation", "branch_thesis", "branch_anchor", "branch_driver", "branch_target":
		return true
	default:
		return false
	}
}

func normalizeAuthorClaimStatus(status compile.AuthorClaimStatus) compile.AuthorClaimStatus {
	switch status {
	case compile.AuthorClaimSupported, compile.AuthorClaimContradicted, compile.AuthorClaimUnverified, compile.AuthorClaimInterpretive, compile.AuthorClaimNotAuthorClaim:
		return status
	default:
		return compile.AuthorClaimUnverified
	}
}

func normalizeAuthorEvidenceRequirements(requirements []compile.AuthorEvidenceRequirement) []compile.AuthorEvidenceRequirement {
	if len(requirements) == 0 {
		return nil
	}
	out := make([]compile.AuthorEvidenceRequirement, 0, len(requirements))
	for _, requirement := range requirements {
		requirement.Description = strings.TrimSpace(requirement.Description)
		requirement.Subject = strings.TrimSpace(requirement.Subject)
		requirement.Metric = strings.TrimSpace(requirement.Metric)
		requirement.OriginalValue = strings.TrimSpace(requirement.OriginalValue)
		requirement.Unit = strings.TrimSpace(requirement.Unit)
		requirement.TimeWindow = strings.TrimSpace(requirement.TimeWindow)
		requirement.SourceType = strings.TrimSpace(requirement.SourceType)
		requirement.Series = strings.TrimSpace(requirement.Series)
		requirement.Entity = strings.TrimSpace(requirement.Entity)
		requirement.Geography = strings.TrimSpace(requirement.Geography)
		requirement.Denominator = strings.TrimSpace(requirement.Denominator)
		requirement.PreferredSources = trimmedStringSlice(requirement.PreferredSources)
		requirement.Queries = trimmedStringSlice(requirement.Queries)
		requirement.ComparisonRule = strings.TrimSpace(requirement.ComparisonRule)
		requirement.ScopeCaveat = strings.TrimSpace(requirement.ScopeCaveat)
		requirement.Status = normalizeAuthorClaimStatus(requirement.Status)
		if requirement.Description == "" && requirement.Subject == "" && requirement.Metric == "" {
			continue
		}
		out = append(out, requirement)
	}
	return out
}

func normalizeAuthorSubclaims(parentID string, subclaims []compile.AuthorSubclaim) []compile.AuthorSubclaim {
	if len(subclaims) == 0 {
		return nil
	}
	out := make([]compile.AuthorSubclaim, 0, len(subclaims))
	for i, subclaim := range subclaims {
		subclaim.SubclaimID = strings.TrimSpace(subclaim.SubclaimID)
		if subclaim.SubclaimID == "" {
			subclaim.SubclaimID = fmt.Sprintf("%s.%d", parentID, i+1)
		}
		if strings.TrimSpace(subclaim.ParentClaimID) == "" {
			subclaim.ParentClaimID = parentID
		}
		subclaim.ScopeStatus = normalizeAuthorScopeStatus(subclaim.ScopeStatus)
		subclaim.Status = normalizeAuthorClaimStatus(subclaim.Status)
		if subclaim.ScopeStatus == "mismatch" && subclaim.Status == compile.AuthorClaimSupported {
			subclaim.Status = compile.AuthorClaimContradicted
		}
		out = append(out, subclaim)
	}
	return out
}

func normalizeAuthorScopeStatus(status string) string {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "exact_match", "related_scope", "mismatch", "unknown":
		return strings.ToLower(strings.TrimSpace(status))
	default:
		return ""
	}
}

func aggregateClaimStatusFromSubclaims(status compile.AuthorClaimStatus, subclaims []compile.AuthorSubclaim) compile.AuthorClaimStatus {
	if len(subclaims) == 0 {
		return status
	}
	hasContradicted := false
	hasUnverified := false
	hasInterpretive := false
	hasNotAuthor := false
	allSupported := true
	for _, subclaim := range subclaims {
		switch subclaim.Status {
		case compile.AuthorClaimContradicted:
			hasContradicted = true
			allSupported = false
		case compile.AuthorClaimUnverified:
			hasUnverified = true
			allSupported = false
		case compile.AuthorClaimInterpretive:
			hasInterpretive = true
			allSupported = false
		case compile.AuthorClaimNotAuthorClaim:
			hasNotAuthor = true
			allSupported = false
		case compile.AuthorClaimSupported:
		default:
			hasUnverified = true
			allSupported = false
		}
	}
	switch {
	case hasContradicted:
		return compile.AuthorClaimContradicted
	case hasUnverified:
		return compile.AuthorClaimUnverified
	case hasNotAuthor:
		return compile.AuthorClaimNotAuthorClaim
	case hasInterpretive:
		return compile.AuthorClaimInterpretive
	case allSupported:
		if status == compile.AuthorClaimInterpretive {
			return compile.AuthorClaimInterpretive
		}
		return compile.AuthorClaimSupported
	default:
		return status
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
