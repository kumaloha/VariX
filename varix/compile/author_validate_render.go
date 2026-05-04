package compile

import (
	"strings"
)

func enrichAuthorValidationRenderDetails(result FlowPreviewResult) Output {
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
		result.Coverage,
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

func visibleRenderNodeDetailsForAuthorValidation(out Output, nodes []graphNode) []map[string]any {
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

func visibleOffGraphDetailsForAuthorValidation(out Output, items []offGraphItem) []map[string]any {
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

func visibleAuthorRenderNodeTexts(out Output) map[string]struct{} {
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
	addPath := func(path TransmissionPath) {
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

func visibleAuthorProofTexts(out Output) map[string]struct{} {
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
