package compilev2

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/kumaloha/VariX/varix/compile"
)

type FlowPreviewOptions struct {
	StopAfter string
}

type FlowPreviewResult struct {
	Platform    string           `json:"platform"`
	ExternalID  string           `json:"external_id"`
	URL         string           `json:"url"`
	Extract     PreviewGraph     `json:"extract"`
	Refine      PreviewGraph     `json:"refine"`
	Aggregate   PreviewGraph     `json:"aggregate"`
	Support     PreviewGraph     `json:"support"`
	Cluster     PreviewGraph     `json:"cluster"`
	Supplement  PreviewGraph     `json:"supplement"`
	Collapse    PreviewGraph     `json:"collapse"`
	Evidence    PreviewGraph     `json:"evidence"`
	Explanation PreviewGraph     `json:"explanation"`
	Relations   PreviewGraph     `json:"relations"`
	Classify    PreviewGraph     `json:"classify"`
	Render      compile.Output   `json:"render"`
	Metrics     map[string]int64 `json:"metrics"`
}

type PreviewGraph struct {
	Nodes       []PreviewNode     `json:"nodes"`
	Edges       []PreviewEdge     `json:"edges"`
	AuxEdges    []PreviewEdge     `json:"aux_edges,omitempty"`
	OffGraph    []PreviewOffGraph `json:"off_graph"`
	BranchHeads []string          `json:"branch_heads,omitempty"`
	Rounds      int               `json:"rounds"`
}

type PreviewNode struct {
	ID          string `json:"id"`
	Text        string `json:"text"`
	SourceQuote string `json:"source_quote,omitempty"`
	Role        string `json:"role,omitempty"`
	Ontology    string `json:"ontology,omitempty"`
	IsTarget    bool   `json:"is_target,omitempty"`
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

func (c *Client) PreviewFlow(ctx context.Context, bundle compile.Bundle, opts FlowPreviewOptions) (FlowPreviewResult, error) {
	if c == nil || c.runtime == nil {
		return FlowPreviewResult{}, fmt.Errorf("compile v2 client is nil")
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
	relationsState, err := stage3Mainline(ctx, c.runtime, c.model, bundle, cloneGraphState(selectedState))
	if err != nil {
		return FlowPreviewResult{}, fmt.Errorf("relations: %w", err)
	}
	result.Relations = toPreviewGraph(relationsState)
	result.Metrics["relations_ms"] = time.Since(start).Milliseconds()
	if stop := strings.TrimSpace(opts.StopAfter); stop == "mainline" || stop == "relations" || stop == "drives" {
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

func BuildMainlineMarkdown(result FlowPreviewResult) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# %s:%s\n\n", result.Platform, result.ExternalID)
	fmt.Fprintf(&b, "URL: %s\n\n", result.URL)
	fmt.Fprintf(&b, "Drivers: %d\n", len(result.Render.Drivers))
	fmt.Fprintf(&b, "Targets: %d\n", len(result.Render.Targets))
	fmt.Fprintf(&b, "Paths: %d\n\n", len(result.Render.TransmissionPaths))
	b.WriteString("```mermaid\nflowchart LR\n")
	if len(result.Render.TransmissionPaths) == 0 {
		b.WriteString("  n0[\"No mainline path\"]\n")
		b.WriteString("```\n")
		return b.String()
	}
	labelToID := map[string]string{}
	nextID := 1
	nodeID := func(label string) string {
		label = strings.TrimSpace(label)
		if label == "" {
			label = "Unnamed"
		}
		if id, ok := labelToID[label]; ok {
			return id
		}
		id := "n" + strconv.Itoa(nextID)
		nextID++
		labelToID[label] = id
		fmt.Fprintf(&b, "  %s[\"%s\"]\n", id, escapeMermaidLabel(label))
		return id
	}
	for _, path := range result.Render.TransmissionPaths {
		prev := nodeID(path.Driver)
		for _, step := range path.Steps {
			cur := nodeID(step)
			fmt.Fprintf(&b, "  %s --> %s\n", prev, cur)
			prev = cur
		}
		target := nodeID(path.Target)
		fmt.Fprintf(&b, "  %s --> %s\n", prev, target)
	}
	b.WriteString("```\n")
	return b.String()
}

func toPreviewGraph(state graphState) PreviewGraph {
	out := PreviewGraph{
		Nodes:       make([]PreviewNode, 0, len(state.Nodes)),
		Edges:       make([]PreviewEdge, 0, len(state.Edges)),
		AuxEdges:    make([]PreviewEdge, 0, len(state.AuxEdges)),
		OffGraph:    make([]PreviewOffGraph, 0, len(state.OffGraph)),
		BranchHeads: append([]string(nil), state.BranchHeads...),
		Rounds:      state.Rounds,
	}
	for _, node := range state.Nodes {
		out.Nodes = append(out.Nodes, PreviewNode{
			ID:          node.ID,
			Text:        node.Text,
			SourceQuote: node.SourceQuote,
			Role:        string(node.Role),
			Ontology:    node.Ontology,
			IsTarget:    node.IsTarget,
		})
	}
	for _, edge := range state.Edges {
		out.Edges = append(out.Edges, PreviewEdge{
			From:        edge.From,
			To:          edge.To,
			SourceQuote: edge.SourceQuote,
			Reason:      edge.Reason,
		})
	}
	for _, edge := range state.AuxEdges {
		out.AuxEdges = append(out.AuxEdges, PreviewEdge{
			From:        edge.From,
			To:          edge.To,
			Kind:        edge.Kind,
			SourceQuote: edge.SourceQuote,
			Reason:      edge.Reason,
		})
	}
	for _, item := range state.OffGraph {
		out.OffGraph = append(out.OffGraph, PreviewOffGraph{
			ID:          item.ID,
			Text:        item.Text,
			Role:        item.Role,
			AttachesTo:  item.AttachesTo,
			SourceQuote: item.SourceQuote,
		})
	}
	return out
}

func cloneGraphState(state graphState) graphState {
	return graphState{
		Nodes:       append([]graphNode(nil), state.Nodes...),
		Edges:       append([]graphEdge(nil), state.Edges...),
		AuxEdges:    append([]auxEdge(nil), state.AuxEdges...),
		OffGraph:    append([]offGraphItem(nil), state.OffGraph...),
		BranchHeads: append([]string(nil), state.BranchHeads...),
		Rounds:      state.Rounds,
	}
}

func escapeMermaidLabel(value string) string {
	value = strings.ReplaceAll(value, "\"", "\\\"")
	value = strings.ReplaceAll(value, "\n", " ")
	return value
}
