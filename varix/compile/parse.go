package compile

import (
	"encoding/json"
	"fmt"
	"strings"
)

func ParseOutput(raw string) (Output, error) {
	clean := strings.TrimSpace(raw)
	clean = strings.TrimPrefix(clean, "```json")
	clean = strings.TrimPrefix(clean, "```")
	clean = strings.TrimSuffix(clean, "```")
	clean = strings.TrimSpace(clean)

	var payload map[string]json.RawMessage
	if err := json.Unmarshal([]byte(clean), &payload); err != nil {
		return Output{}, fmt.Errorf("parse compile output: %w", err)
	}
	var out Output
	if err := json.Unmarshal(payload["summary"], &out.Summary); err != nil {
		return Output{}, fmt.Errorf("parse compile summary: %w", err)
	}
	_ = json.Unmarshal(payload["topics"], &out.Topics)
	_ = json.Unmarshal(payload["confidence"], &out.Confidence)
	_ = json.Unmarshal(payload["graph"], &out.Graph)
	normalizeNodeTaxonomy(&out.Graph)
	_ = json.Unmarshal(payload["verification"], &out.Verification)
	if rawDetails, ok := payload["details"]; ok {
		details, err := parseHiddenDetails(rawDetails)
		if err != nil {
			return Output{}, err
		}
		out.Details = details
	}
	if err := out.Validate(); err != nil {
		return Output{}, err
	}
	return out, nil
}

func normalizeNodeTaxonomy(graph *ReasoningGraph) {
	if graph == nil {
		return
	}
	for i := range graph.Nodes {
		node := &graph.Nodes[i]
		text := strings.TrimSpace(node.Text)
		if text == "" {
			continue
		}
		if isExplicitConditionText(text) {
			node.Kind = NodeExplicitCondition
		}
	}
}

func isExplicitConditionText(text string) bool {
	text = strings.TrimSpace(text)
	prefixes := []string{"如果", "若", "一旦", "假如", "倘若", "如若"}
	for _, prefix := range prefixes {
		if strings.HasPrefix(text, prefix) {
			return true
		}
	}
	return false
}

func parseHiddenDetails(raw json.RawMessage) (HiddenDetails, error) {
	var details HiddenDetails
	if len(raw) == 0 || string(raw) == "null" {
		return details, nil
	}

	var object HiddenDetails
	if err := json.Unmarshal(raw, &object); err == nil {
		return object, nil
	}

	var list []string
	if err := json.Unmarshal(raw, &list); err == nil {
		details.Caveats = list
		return details, nil
	}

	var objects []map[string]any
	if err := json.Unmarshal(raw, &objects); err == nil {
		details.Items = objects
		return details, nil
	}

	var text string
	if err := json.Unmarshal(raw, &text); err == nil {
		details.Caveats = []string{text}
		return details, nil
	}

	return HiddenDetails{}, fmt.Errorf("parse compile details: unsupported shape")
}
