package compile

import (
	"context"
	"fmt"
	"strings"
	"time"
)

type FlowPreviewOptions struct {
	StopAfter string
}

type FlowPreviewResult struct {
	Platform         string            `json:"platform"`
	ExternalID       string            `json:"external_id"`
	URL              string            `json:"url"`
	ArticleForm      string            `json:"article_form,omitempty"`
	Extract          PreviewGraph      `json:"extract"`
	Refine           PreviewGraph      `json:"refine"`
	Aggregate        PreviewGraph      `json:"aggregate"`
	Support          PreviewGraph      `json:"support"`
	Cluster          PreviewGraph      `json:"cluster"`
	Supplement       PreviewGraph      `json:"supplement"`
	Collapse         PreviewGraph      `json:"collapse"`
	Evidence         PreviewGraph      `json:"evidence"`
	Explanation      PreviewGraph      `json:"explanation"`
	Relations        PreviewGraph      `json:"relations"`
	Spines           []PreviewSpine    `json:"spines,omitempty"`
	Classify         PreviewGraph      `json:"classify"`
	Coverage         PreviewGraph      `json:"coverage,omitempty"`
	Render           Output            `json:"render"`
	AuthorValidation *AuthorValidation `json:"author_validation,omitempty"`
	Metrics          map[string]int64  `json:"metrics"`
}

type PreviewGraph struct {
	Nodes         []PreviewNode     `json:"nodes"`
	Edges         []PreviewEdge     `json:"edges"`
	AuxEdges      []PreviewEdge     `json:"aux_edges,omitempty"`
	OffGraph      []PreviewOffGraph `json:"off_graph"`
	BranchHeads   []string          `json:"branch_heads,omitempty"`
	SemanticUnits []SemanticUnit    `json:"semantic_units,omitempty"`
	Rounds        int               `json:"rounds"`
}

type PreviewNode struct {
	ID            string `json:"id"`
	Text          string `json:"text"`
	SourceQuote   string `json:"source_quote,omitempty"`
	Role          string `json:"role,omitempty"`
	DiscourseRole string `json:"discourse_role,omitempty"`
	Ontology      string `json:"ontology,omitempty"`
	IsTarget      bool   `json:"is_target,omitempty"`
}

type PreviewEdge struct {
	From        string `json:"from"`
	To          string `json:"to"`
	Kind        string `json:"kind,omitempty"`
	SourceQuote string `json:"source_quote,omitempty"`
	Reason      string `json:"reason,omitempty"`
}

type PreviewOffGraph struct {
	ID          string `json:"id,omitempty"`
	Text        string `json:"text"`
	Role        string `json:"role"`
	AttachesTo  string `json:"attaches_to,omitempty"`
	SourceQuote string `json:"source_quote,omitempty"`
}

type PreviewSpine struct {
	ID          string        `json:"id"`
	Level       string        `json:"level"`
	Priority    int           `json:"priority"`
	Policy      string        `json:"policy,omitempty"`
	Thesis      string        `json:"thesis"`
	NodeIDs     []string      `json:"node_ids"`
	UnitIDs     []string      `json:"unit_ids,omitempty"`
	Edges       []PreviewEdge `json:"edges"`
	Scope       string        `json:"scope,omitempty"`
	FamilyKey   string        `json:"family_key,omitempty"`
	FamilyLabel string        `json:"family_label,omitempty"`
	FamilyScope string        `json:"family_scope,omitempty"`
}

func (c *Client) PreviewFlow(ctx context.Context, bundle Bundle, opts FlowPreviewOptions) (FlowPreviewResult, error) {
	if c == nil || c.runtime == nil {
		return FlowPreviewResult{}, fmt.Errorf("compile client is nil")
	}
	result := FlowPreviewResult{
		Platform:   bundle.Source,
		ExternalID: bundle.ExternalID,
		URL:        bundle.URL,
		Metrics:    map[string]int64{},
	}

	start := time.Now()
	extractState, err := stage1Extract(ctx, c.runtime, c.model, bundle)
	if err != nil {
		return FlowPreviewResult{}, fmt.Errorf("extract: %w", err)
	}
	result.Extract = toPreviewGraph(extractState)
	result.ArticleForm = extractState.ArticleForm
	result.Metrics["extract_ms"] = time.Since(start).Milliseconds()
	if strings.TrimSpace(opts.StopAfter) == "extract" {
		return result, nil
	}

	start = time.Now()
	refineState, err := stage1Refine(ctx, c.runtime, c.model, bundle, cloneGraphState(extractState))
	if err != nil {
		return FlowPreviewResult{}, fmt.Errorf("refine: %w", err)
	}
	result.Refine = toPreviewGraph(refineState)
	result.Metrics["refine_ms"] = time.Since(start).Milliseconds()
	if strings.TrimSpace(opts.StopAfter) == "refine" {
		return result, nil
	}

	start = time.Now()
	aggregateState, err := stage1Aggregate(ctx, c.runtime, c.model, bundle, cloneGraphState(refineState))
	if err != nil {
		return FlowPreviewResult{}, fmt.Errorf("aggregate: %w", err)
	}
	result.Aggregate = toPreviewGraph(aggregateState)
	result.Metrics["aggregate_ms"] = time.Since(start).Milliseconds()
	if strings.TrimSpace(opts.StopAfter) == "aggregate" {
		return result, nil
	}

	start = time.Now()
	supportState, err := stage2Support(ctx, c.runtime, c.model, bundle, cloneGraphState(aggregateState))
	if err != nil {
		return FlowPreviewResult{}, fmt.Errorf("support: %w", err)
	}
	result.Support = toPreviewGraph(supportState)
	result.Cluster = result.Support
	result.Supplement = result.Support
	result.Metrics["support_ms"] = time.Since(start).Milliseconds()
	result.Metrics["cluster_ms"] = result.Metrics["support_ms"]
	if stop := strings.TrimSpace(opts.StopAfter); stop == "support" || stop == "supplement" || stop == "cluster" {
		return result, nil
	}

	start = time.Now()
	selectedState := collapseClusters(cloneGraphState(supportState))
	result.Collapse = toPreviewGraph(selectedState)
	result.Metrics["collapse_ms"] = time.Since(start).Milliseconds()
	if strings.TrimSpace(opts.StopAfter) == "collapse" {
		return result, nil
	}

	start = time.Now()
	selectedState, err = stageSalience(ctx, c.runtime, c.model, bundle, selectedState)
	if err != nil {
		return FlowPreviewResult{}, fmt.Errorf("salience: %w", err)
	}
	result.Collapse = toPreviewGraph(selectedState)
	result.Metrics["salience_ms"] = time.Since(start).Milliseconds()
	if stop := strings.TrimSpace(opts.StopAfter); stop == "semantic" || stop == "salience" || stop == "semantic_coverage" {
		return result, nil
	}

	start = time.Now()
	selectedState, err = stageCoverage(ctx, c.runtime, c.model, bundle, selectedState, 1)
	if err != nil {
		return FlowPreviewResult{}, fmt.Errorf("coverage: %w", err)
	}
	result.Coverage = toPreviewGraph(selectedState)
	result.Metrics["coverage_ms"] = time.Since(start).Milliseconds()
	if strings.TrimSpace(opts.StopAfter) == "coverage" {
		return result, nil
	}

	start = time.Now()
	relationsState, err := stage3Mainline(ctx, c.runtime, c.model, bundle, cloneGraphState(selectedState))
	if err != nil {
		return FlowPreviewResult{}, fmt.Errorf("relations: %w", err)
	}
	result.Relations = toPreviewGraph(relationsState)
	result.Spines = relationsState.Spines
	if len(result.Spines) == 0 {
		result.Spines = derivePreviewSpines(result.Relations)
	}
	result.Metrics["relations_ms"] = time.Since(start).Milliseconds()
	if stop := strings.TrimSpace(opts.StopAfter); stop == "mainline" || stop == "relations" || stop == "spines" || stop == "drives" {
		return result, nil
	}

	start = time.Now()
	classifyState, err := stage3Classify(ctx, c.runtime, c.model, bundle, cloneGraphState(relationsState))
	if err != nil {
		return FlowPreviewResult{}, fmt.Errorf("classify: %w", err)
	}
	result.Classify = toPreviewGraph(classifyState)
	result.Metrics["classify_ms"] = time.Since(start).Milliseconds()
	if strings.TrimSpace(opts.StopAfter) == "classify" {
		return result, nil
	}

	start = time.Now()
	rendered, err := stage5Render(ctx, c.runtime, c.model, bundle, cloneGraphState(classifyState))
	if err != nil {
		return FlowPreviewResult{}, fmt.Errorf("render: %w", err)
	}
	result.Render = rendered
	result.Metrics["render_ms"] = time.Since(start).Milliseconds()
	return result, nil
}

func (c *Client) CoveragePreviewResult(ctx context.Context, bundle Bundle, result FlowPreviewResult, maxRounds int, paragraphLimit int) (FlowPreviewResult, error) {
	if c == nil || c.runtime == nil {
		return FlowPreviewResult{}, fmt.Errorf("compile client is nil")
	}
	if result.Metrics == nil {
		result.Metrics = map[string]int64{}
	}
	state := fromPreviewGraph(result.Classify, result.Spines, result.ArticleForm)
	if len(state.Nodes) == 0 {
		state = fromPreviewGraph(result.Relations, result.Spines, result.ArticleForm)
	}
	if len(state.Spines) == 0 {
		state.Spines = result.Spines
	}
	var err error
	state, err = stageSalience(ctx, c.runtime, c.model, bundle, state)
	if err != nil {
		return FlowPreviewResult{}, fmt.Errorf("salience: %w", err)
	}
	start := time.Now()
	covered, err := runCoverage(ctx, c.runtime, c.model, bundle, state, maxRounds, paragraphLimit)
	if err != nil {
		return FlowPreviewResult{}, fmt.Errorf("coverage: %w", err)
	}
	result.Coverage = toPreviewGraph(covered)
	result.Spines = covered.Spines
	result.Metrics["coverage_ms"] = time.Since(start).Milliseconds()

	start = time.Now()
	rendered, err := stage5Render(ctx, c.runtime, c.model, bundle, cloneGraphState(covered))
	if err != nil {
		return FlowPreviewResult{}, fmt.Errorf("render: %w", err)
	}
	result.Render = rendered
	result.Metrics["render_ms"] = time.Since(start).Milliseconds()
	return result, nil
}

func (c *Client) RenderPreview(ctx context.Context, bundle Bundle, result FlowPreviewResult) (FlowPreviewResult, error) {
	if c == nil || c.runtime == nil {
		return FlowPreviewResult{}, fmt.Errorf("compile client is nil")
	}
	if result.Metrics == nil {
		result.Metrics = map[string]int64{}
	}
	if strings.TrimSpace(result.Platform) == "" {
		result.Platform = bundle.Source
	}
	if strings.TrimSpace(result.ExternalID) == "" {
		result.ExternalID = bundle.ExternalID
	}
	if strings.TrimSpace(result.URL) == "" {
		result.URL = bundle.URL
	}

	state := fromPreviewGraph(result.Coverage, result.Spines, result.ArticleForm)
	if len(state.Nodes) == 0 {
		state = fromPreviewGraph(result.Classify, result.Spines, result.ArticleForm)
	}
	if len(state.Nodes) == 0 {
		state = fromPreviewGraph(result.Relations, result.Spines, result.ArticleForm)
	}
	if len(state.Nodes) == 0 {
		return FlowPreviewResult{}, fmt.Errorf("rerender requires coverage, classify, or relations preview graph")
	}
	if len(state.Spines) == 0 {
		state.Spines = derivePreviewSpines(toPreviewGraph(state))
		result.Spines = state.Spines
	}
	start := time.Now()
	rendered, err := stage5Render(ctx, c.runtime, c.model, bundle, cloneGraphState(state))
	if err != nil {
		return FlowPreviewResult{}, fmt.Errorf("render: %w", err)
	}
	result.Render = rendered
	elapsed := time.Since(start).Milliseconds()
	if elapsed <= 0 {
		elapsed = 1
	}
	result.Metrics["render_ms"] = elapsed
	return result, nil
}
