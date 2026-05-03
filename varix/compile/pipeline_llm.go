package compile

import (
	"context"
	"encoding/json"
	"fmt"
	varixllm "github.com/kumaloha/VariX/varix/llm"
	"github.com/kumaloha/forge/llm"
	"sort"
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
	bundle := Bundle{UnitID: "translate", Source: "compile", ExternalID: "translate", Content: string(payload)}
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

func summarizeChinese(ctx context.Context, rt runtimeChat, model string, articleForm string, drivers, targets []string, paths []TransmissionPath, declarations []Declaration, semanticUnits []SemanticUnit, bundle Bundle) (string, error) {
	payload, err := json.Marshal(map[string]any{
		"article_form":   normalizeArticleForm(articleForm),
		"drivers":        drivers,
		"targets":        targets,
		"paths":          paths,
		"declarations":   declarations,
		"semantic_units": topSemanticUnitsForSummary(semanticUnits, articleForm),
	})
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

func topSemanticUnitsForSummary(units []SemanticUnit, articleForm string) []SemanticUnit {
	if len(units) == 0 {
		return nil
	}
	ranked := rankSemanticUnits(units, "shareholder_meeting")
	if isReaderInterestSummaryForm(articleForm) {
		sortSemanticUnitsForReaderInterest(ranked)
	}
	if len(ranked) > 6 {
		ranked = ranked[:6]
	}
	return ranked
}

func isReaderInterestSummaryForm(articleForm string) bool {
	switch strings.ToLower(strings.TrimSpace(articleForm)) {
	case "management_qa", "shareholder_meeting", "earnings_call", "capital_allocation_discussion":
		return true
	default:
		return false
	}
}

func sortSemanticUnitsForReaderInterest(units []SemanticUnit) {
	sort.SliceStable(units, func(i, j int) bool {
		left := summaryReaderInterestRank(units[i])
		right := summaryReaderInterestRank(units[j])
		if left != right {
			return left < right
		}
		if units[i].Salience != units[j].Salience {
			return units[i].Salience > units[j].Salience
		}
		return units[i].ID < units[j].ID
	})
}

func summaryReaderInterestRank(unit SemanticUnit) int {
	text := strings.ToLower(strings.Join([]string{unit.Subject, unit.Force, unit.Claim, unit.PromptContext}, " "))
	switch {
	case strings.Contains(text, "capital allocation") || strings.Contains(text, "资本配置") || strings.Contains(text, "deploy capital") || strings.Contains(text, "部署资本"):
		return 0
	case strings.Contains(text, "circle of competence") || strings.Contains(text, "existing portfolio") || strings.Contains(text, "能力圈") || strings.Contains(text, "现有组合"):
		return 1
	case strings.Contains(text, "buyback") || strings.Contains(text, "repurchase") || strings.Contains(text, "回购") || strings.Contains(text, "内在价值"):
		return 2
	case strings.Contains(text, "underwriting boundary") || strings.Contains(text, "承保边界") || strings.Contains(text, "承保纪律"):
		return 3
	case strings.Contains(text, "ai") || strings.Contains(text, "technology") || strings.Contains(text, "人工智能") || strings.Contains(text, "技术"):
		return 4
	case strings.Contains(text, "utility") || strings.Contains(text, "utilities") || strings.Contains(text, "公用事业") || strings.Contains(text, "监管契约"):
		return 5
	case strings.Contains(text, "tokyo marine") || strings.Contains(text, "东京海上") || strings.Contains(text, "transaction") || strings.Contains(text, "交易"):
		return 6
	case strings.Contains(text, "succession") || strings.Contains(text, "culture") || strings.Contains(text, "继任") || strings.Contains(text, "文化"):
		return 7
	case strings.Contains(text, "market softening") || strings.Contains(text, "市场软化") || strings.Contains(text, "资本涌入"):
		return 8
	default:
		switch strings.ToLower(strings.TrimSpace(unit.Force)) {
		case "commit", "answer", "set_boundary":
			return 9
		case "disclose":
			return 10
		case "frame_risk":
			return 12
		default:
			return 11
		}
	}
}

func stageJSONCall(ctx context.Context, rt runtimeChat, model string, bundle Bundle, systemPrompt string, userPrompt string, stageName string, target any) error {
	req, err := varixllm.BuildProviderRequest(stageModel(stageName, model), bundle, systemPrompt, userPrompt, stageSearch(stageName))
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
	case "semantic_coverage":
		return semanticCoverageSchema()
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

func semanticCoverageSchema() *llm.Schema {
	return &llm.Schema{
		Name:     "compile_semantic_coverage",
		Required: []string{"semantic_units"},
		Properties: map[string]any{
			"semantic_units": map[string]any{
				"type": "array",
				"items": map[string]any{
					"type":     "object",
					"required": []string{"id", "speaker_role", "subject", "force", "claim", "source_quote", "salience", "confidence"},
					"properties": map[string]any{
						"id":                map[string]any{"type": "string"},
						"span":              map[string]any{"type": "string"},
						"speaker":           map[string]any{"type": "string"},
						"speaker_role":      map[string]any{"type": "string"},
						"subject":           map[string]any{"type": "string"},
						"force":             map[string]any{"type": "string"},
						"claim":             map[string]any{"type": "string"},
						"prompt_context":    map[string]any{"type": "string"},
						"importance_reason": map[string]any{"type": "string"},
						"source_quote":      map[string]any{"type": "string"},
						"salience":          map[string]any{"type": "number"},
						"confidence":        map[string]any{"type": "string"},
					},
				},
			},
		},
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
				"unit_ids":     map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
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
		return varixllm.Qwen36PlusModel
	default:
		if strings.TrimSpace(fallback) != "" {
			return strings.TrimSpace(fallback)
		}
		return varixllm.Qwen3MaxModel
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
