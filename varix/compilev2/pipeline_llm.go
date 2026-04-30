package compilev2

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/kumaloha/VariX/varix/compile"
	"github.com/kumaloha/forge/llm"
	"strings"
)

func translateAll(ctx context.Context, rt runtimeChat, model string, items []map[string]string) (map[string]string, error) {
	if len(items) == 0 {
		return map[string]string{}, nil
	}
	payload, err := json.Marshal(items)
	if err != nil {
		return nil, err
	}
	bundle := compile.Bundle{UnitID: "translate", Source: "compilev2", ExternalID: "translate", Content: string(payload)}
	var result struct {
		Translations []struct {
			ID   string `json:"id"`
			Text string `json:"text"`
		} `json:"translations"`
	}
	systemPrompt, err := renderStage5TranslateSystemPrompt()
	if err != nil {
		return nil, err
	}
	userPrompt, err := renderStage5TranslateUserPrompt(string(payload))
	if err != nil {
		return nil, err
	}
	if err := stageJSONCall(ctx, rt, model, bundle, systemPrompt, userPrompt, "translate", &result); err != nil {
		return nil, err
	}
	out := map[string]string{}
	for _, item := range result.Translations {
		out[item.ID] = strings.TrimSpace(item.Text)
	}
	return out, nil
}

func summarizeChinese(ctx context.Context, rt runtimeChat, model string, articleForm string, drivers, targets []string, paths []compile.TransmissionPath, bundle compile.Bundle) (string, error) {
	payload, err := json.Marshal(map[string]any{"article_form": normalizeArticleForm(articleForm), "drivers": drivers, "targets": targets, "paths": paths})
	if err != nil {
		return "", err
	}
	var result struct {
		Summary string `json:"summary"`
	}
	systemPrompt, err := renderStage5SummarySystemPrompt()
	if err != nil {
		return "", err
	}
	userPrompt, err := renderStage5SummaryUserPrompt(string(payload))
	if err != nil {
		return "", err
	}
	if err := stageJSONCall(ctx, rt, model, bundle, systemPrompt, userPrompt, "summary", &result); err != nil {
		return "", err
	}
	return strings.TrimSpace(result.Summary), nil
}

func stageJSONCall(ctx context.Context, rt runtimeChat, model string, bundle compile.Bundle, systemPrompt string, userPrompt string, stageName string, target any) error {
	req, err := compile.BuildProviderRequest(stageModel(stageName, model), bundle, systemPrompt, userPrompt, stageSearch(stageName))
	if err != nil {
		return err
	}
	req.JSONSchema = stageJSONSchema(stageName)
	resp, err := callStageRuntime(ctx, rt, stageName, req)
	if err != nil {
		return err
	}
	if err := parseJSONObject(resp.Text, target); err != nil {
		return fmt.Errorf("%s parse: %w", stageName, err)
	}
	return nil
}

func stageJSONSchema(stageName string) *llm.Schema {
	switch strings.TrimSpace(stageName) {
	case "extract":
		return &llm.Schema{
			Name:     "compile_extract",
			Required: []string{"article_form", "nodes", "off_graph"},
			Properties: map[string]any{
				"article_form": map[string]any{"type": "string"},
				"nodes": map[string]any{
					"type": "array",
					"items": map[string]any{
						"type":     "object",
						"required": []string{"id", "text", "source_quote", "role"},
						"properties": map[string]any{
							"id":           map[string]any{"type": "string"},
							"text":         map[string]any{"type": "string"},
							"source_quote": map[string]any{"type": "string"},
							"role":         map[string]any{"type": "string"},
						},
					},
				},
				"off_graph": map[string]any{
					"type": "array",
					"items": map[string]any{
						"type":     "object",
						"required": []string{"id", "text", "role", "attaches_to", "source_quote"},
						"properties": map[string]any{
							"id":           map[string]any{"type": "string"},
							"text":         map[string]any{"type": "string"},
							"role":         map[string]any{"type": "string"},
							"attaches_to":  map[string]any{"type": "string"},
							"source_quote": map[string]any{"type": "string"},
						},
					},
				},
			},
		}
	case "refine":
		return &llm.Schema{
			Name:     "compile_refine",
			Required: []string{"replacements"},
			Properties: map[string]any{
				"replacements": map[string]any{
					"type": "array",
					"items": map[string]any{
						"type":     "object",
						"required": []string{"replace_id", "nodes", "reason"},
						"properties": map[string]any{
							"replace_id": map[string]any{"type": "string"},
							"reason":     map[string]any{"type": "string"},
							"nodes": map[string]any{
								"type": "array",
								"items": map[string]any{
									"type":     "object",
									"required": []string{"text", "source_quote", "role"},
									"properties": map[string]any{
										"text":         map[string]any{"type": "string"},
										"source_quote": map[string]any{"type": "string"},
										"role":         map[string]any{"type": "string"},
									},
								},
							},
						},
					},
				},
			},
		}
	case "aggregate":
		return &llm.Schema{
			Name:     "compile_aggregate",
			Required: []string{"aggregates"},
			Properties: map[string]any{
				"aggregates": map[string]any{
					"type": "array",
					"items": map[string]any{
						"type":     "object",
						"required": []string{"text", "member_ids", "source_quote", "reason"},
						"properties": map[string]any{
							"text": map[string]any{"type": "string"},
							"member_ids": map[string]any{
								"type":  "array",
								"items": map[string]any{"type": "string"},
							},
							"source_quote": map[string]any{"type": "string"},
							"reason":       map[string]any{"type": "string"},
						},
					},
				},
			},
		}
	case "support":
		return &llm.Schema{
			Name:     "compile_support",
			Required: []string{"support_edges"},
			Properties: map[string]any{
				"support_edges": map[string]any{
					"type": "array",
					"items": map[string]any{
						"type":     "object",
						"required": []string{"from", "to", "kind", "source_quote", "reason"},
						"properties": map[string]any{
							"from":         map[string]any{"type": "string"},
							"to":           map[string]any{"type": "string"},
							"kind":         map[string]any{"type": "string"},
							"source_quote": map[string]any{"type": "string"},
							"reason":       map[string]any{"type": "string"},
						},
					},
				},
			},
		}
	case "evidence":
		return linkListSchema("compile_evidence", "support_links", "from", "to")
	case "explanation":
		return linkListSchema("compile_explanation", "explanation_links", "from", "to")
	case "supplement":
		return linkListSchema("compile_supplement", "supplement_links", "a", "b")
	case "mainline":
		return mainlineSchema()
	case "validate":
		return &llm.Schema{
			Name:     "compile_validate",
			Required: []string{"missing_nodes", "missing_edges", "misclassified"},
			Properties: map[string]any{
				"missing_nodes": map[string]any{
					"type": "array",
					"items": map[string]any{
						"type":     "object",
						"required": []string{"text", "source_quote", "suggested_role_hint"},
						"properties": map[string]any{
							"text":                map[string]any{"type": "string"},
							"source_quote":        map[string]any{"type": "string"},
							"suggested_role_hint": map[string]any{"type": "string"},
						},
					},
				},
				"missing_edges": map[string]any{
					"type": "array",
					"items": map[string]any{
						"type":     "object",
						"required": []string{"from_text", "to_text"},
						"properties": map[string]any{
							"from_text": map[string]any{"type": "string"},
							"to_text":   map[string]any{"type": "string"},
						},
					},
				},
				"misclassified": map[string]any{
					"type": "array",
					"items": map[string]any{
						"type":     "object",
						"required": []string{"node_id", "issue"},
						"properties": map[string]any{
							"node_id": map[string]any{"type": "string"},
							"issue":   map[string]any{"type": "string"},
						},
					},
				},
			},
		}
	case "translate":
		return &llm.Schema{
			Name:     "compile_translate",
			Required: []string{"translations"},
			Properties: map[string]any{
				"translations": map[string]any{
					"type": "array",
					"items": map[string]any{
						"type":     "object",
						"required": []string{"id", "text"},
						"properties": map[string]any{
							"id":   map[string]any{"type": "string"},
							"text": map[string]any{"type": "string"},
						},
					},
				},
			},
		}
	case "summary":
		return &llm.Schema{
			Name:       "compile_summary",
			Required:   []string{"summary"},
			Properties: map[string]any{"summary": map[string]any{"type": "string"}},
		}
	default:
		return nil
	}
}

func mainlineSchema() *llm.Schema {
	schema := linkListSchema("compile_relations", "relations", "from", "to")
	if relations, ok := schema.Properties["relations"].(map[string]any); ok {
		if items, ok := relations["items"].(map[string]any); ok {
			if props, ok := items["properties"].(map[string]any); ok {
				props["kind"] = map[string]any{"type": "string"}
			}
		}
	}
	schema.Properties["spines"] = map[string]any{
		"type": "array",
		"items": map[string]any{
			"type":     "object",
			"required": []string{"id", "level", "priority", "thesis", "node_ids", "edge_indexes", "scope", "why"},
			"properties": map[string]any{
				"id":           map[string]any{"type": "string"},
				"level":        map[string]any{"type": "string"},
				"priority":     map[string]any{"type": "integer"},
				"policy":       map[string]any{"type": "string"},
				"thesis":       map[string]any{"type": "string"},
				"node_ids":     map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
				"edge_indexes": map[string]any{"type": "array", "items": map[string]any{"type": "integer"}},
				"scope":        map[string]any{"type": "string"},
				"why":          map[string]any{"type": "string"},
			},
		},
	}
	return schema
}

func linkListSchema(name string, key string, fromKey string, toKey string) *llm.Schema {
	return &llm.Schema{
		Name:     name,
		Required: []string{key},
		Properties: map[string]any{
			key: map[string]any{
				"type": "array",
				"items": map[string]any{
					"type":     "object",
					"required": []string{fromKey, toKey, "source_quote", "reason"},
					"properties": map[string]any{
						fromKey:        map[string]any{"type": "string"},
						toKey:          map[string]any{"type": "string"},
						"source_quote": map[string]any{"type": "string"},
						"reason":       map[string]any{"type": "string"},
					},
				},
			},
		},
	}
}

func stageModel(stageName, fallback string) string {
	switch strings.TrimSpace(stageName) {
	case "validate":
		return compile.Qwen36PlusModel
	default:
		if strings.TrimSpace(fallback) != "" {
			return strings.TrimSpace(fallback)
		}
		return compile.Qwen3MaxModel
	}
}

func stageSearch(stageName string) bool {
	switch strings.TrimSpace(stageName) {
	case "validate":
		return true
	default:
		return false
	}
}
