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
- Resolve relative time windows from posted_at or source publication time before validating. "month-to-date", "this month", "April so far", "year-to-date", "this week", and similar phrases are different windows; do not substitute YTD for MTD, week-to-date for month-to-date, or data after publication for data available at publication.
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
        {"description":"data needed to validate this claim", "subject":"...", "metric":"...", "time_window":"...", "source_type":"official|market_data|company_filing|news|specialist_database|author_source|other", "status":"supported|contradicted|unverified|interpretive|not_author_claim", "evidence":["source/value"], "reason":"brief comparison of required data vs found data"}
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
    {"inference_id":"...", "from":"...", "to":"...", "steps":["..."], "status":"sound|weak|unsupported_jump|not_author_inference", "required_evidence":[{"description":"data needed to support this jump", "subject":"...", "metric":"...", "time_window":"...", "source_type":"official|market_data|company_filing|news|specialist_database|author_source|other", "status":"supported|contradicted|unverified|interpretive|not_author_claim", "evidence":["source/value"], "reason":"brief comparison of required data vs found data"}], "evidence":["short quote or source"], "reason":"brief reason", "missing_links":["..."]}
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
		normalizedInferences = append(normalizedInferences, check)
	}
	validation.InferenceChecks = normalizedInferences
	validation.Summary = summarizeAuthorValidation(validation)
	return validation
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
		requirement.TimeWindow = strings.TrimSpace(requirement.TimeWindow)
		requirement.SourceType = strings.TrimSpace(requirement.SourceType)
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
