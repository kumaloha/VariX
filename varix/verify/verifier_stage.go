package verify

import (
	"context"
	"encoding/json"
	"fmt"
	varixllm "github.com/kumaloha/VariX/varix/llm"
	"github.com/kumaloha/forge/llm"
	"strings"
	"time"
)

func callVerifierStage(ctx context.Context, rt verifierCall, stageName string, req llm.ProviderRequest) (llm.Response, time.Time, error) {
	resp, err := varixllm.CallStage(ctx, rt, stageName, req)
	return resp, verifierNow(), err
}

func runVerifierPromptStage(ctx context.Context, rt verifierCall, model string, prompts *promptRegistry, bundle Bundle, instructionKey string, prompt string, parseLabel string, target any) (llm.Response, time.Time, error) {
	instruction, err := prompts.verifierInstruction(instructionKey)
	if err != nil {
		return llm.Response{}, time.Time{}, err
	}
	req, err := BuildQwen36ProviderRequest(model, bundle, instruction, prompt)
	if err != nil {
		return llm.Response{}, time.Time{}, err
	}
	resp, completedAt, err := callVerifierStage(ctx, rt, instructionKey, req)
	if err != nil {
		return llm.Response{}, time.Time{}, err
	}
	if err := unmarshalVerifierPayload(resp.Text, target); err != nil {
		return llm.Response{}, time.Time{}, fmt.Errorf("parse %s output: %w", parseLabel, err)
	}
	return resp, completedAt, nil
}

func unmarshalVerifierPayload(raw string, target any) error {
	clean := strings.TrimSpace(raw)
	clean = strings.TrimPrefix(clean, "```json")
	clean = strings.TrimPrefix(clean, "```")
	clean = strings.TrimSuffix(clean, "```")
	clean = strings.TrimSpace(clean)
	if err := json.Unmarshal([]byte(clean), target); err != nil {
		return fmt.Errorf("parse verifier output: %w", err)
	}
	return nil
}

func buildFactVerificationPrompt(bundle Bundle, nodes []GraphNode, retrievalContext []map[string]any, retrievalSummary *VerificationRetrievalSummary, extra map[string]any) (string, error) {
	payload := map[string]any{
		"as_of": verifierNow().Format(time.RFC3339),
	}
	if len(retrievalContext) > 0 {
		payload["retrieval_context"] = retrievalContext
	}
	if retrievalSummary != nil {
		payload["retrieval_summary"] = retrievalSummary
	}
	for key, value := range extra {
		payload[key] = value
	}
	return buildVerificationPrompt(bundle, nodes, payload)
}

func buildPredictionVerificationPrompt(bundle Bundle, nodes []GraphNode) (string, error) {
	return buildVerificationPrompt(bundle, nodes, map[string]any{
		"as_of": verifierNow().Format(time.RFC3339),
	})
}

func buildExplicitConditionVerificationPrompt(bundle Bundle, nodes []GraphNode) (string, error) {
	return buildVerificationPrompt(bundle, nodes, map[string]any{
		"as_of": verifierNow().Format(time.RFC3339),
	})
}

func buildImplicitConditionVerificationPrompt(bundle Bundle, nodes []GraphNode) (string, error) {
	return buildVerificationPrompt(bundle, nodes, map[string]any{
		"as_of": verifierNow().Format(time.RFC3339),
	})
}

func buildVerificationPrompt(bundle Bundle, nodes []GraphNode, extra map[string]any) (string, error) {
	payload := map[string]any{
		"unit_id":         bundle.UnitID,
		"source":          bundle.Source,
		"external_id":     bundle.ExternalID,
		"nodes":           marshalVerificationNodes(nodes),
		"quotes":          bundle.Quotes,
		"references":      bundle.References,
		"thread_segments": bundle.ThreadSegments,
		"attachments":     bundle.Attachments,
		"text_context":    bundle.TextContext(),
	}
	if trimmed := strings.TrimSpace(bundle.RootExternalID); trimmed != "" {
		payload["root_external_id"] = trimmed
	}
	if trimmed := strings.TrimSpace(bundle.AuthorName); trimmed != "" {
		payload["author_name"] = trimmed
	}
	if trimmed := strings.TrimSpace(bundle.AuthorID); trimmed != "" {
		payload["author_id"] = trimmed
	}
	if trimmed := strings.TrimSpace(bundle.URL); trimmed != "" {
		payload["url"] = trimmed
	}
	if !bundle.PostedAt.IsZero() {
		payload["posted_at"] = bundle.PostedAt.Format(time.RFC3339)
	}
	for key, value := range extra {
		payload[key] = value
	}
	encoded, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return "", err
	}
	return string(encoded), nil
}

func marshalVerificationNodes(nodes []GraphNode) []map[string]any {
	out := make([]map[string]any, 0, len(nodes))
	for _, node := range nodes {
		item := map[string]any{
			"id":   node.ID,
			"kind": node.Kind,
			"text": node.Text,
		}
		if !node.OccurredAt.IsZero() {
			item["occurred_at"] = node.OccurredAt.Format(time.RFC3339)
		}
		if !node.PredictionStartAt.IsZero() {
			item["prediction_start_at"] = node.PredictionStartAt.Format(time.RFC3339)
		}
		if !node.PredictionDueAt.IsZero() {
			item["prediction_due_at"] = node.PredictionDueAt.Format(time.RFC3339)
		}
		if !node.ValidFrom.IsZero() && !node.ValidTo.IsZero() && node.OccurredAt.IsZero() && node.PredictionStartAt.IsZero() {
			item["valid_from"] = node.ValidFrom.Format(time.RFC3339)
			item["valid_to"] = node.ValidTo.Format(time.RFC3339)
		}
		out = append(out, item)
	}
	return out
}

func buildFactRetrievalPayload(ctx context.Context, bundle Bundle, nodes []GraphNode) ([]map[string]any, *VerificationRetrievalSummary, error) {
	retrieval, err := buildFactRetrievalContext(ctx, bundle, nodes)
	if err != nil {
		return nil, nil, err
	}
	summary := &VerificationRetrievalSummary{
		RetrievedNodeIDs:     make([]string, 0, len(retrieval)),
		NoResultNodeIDs:      make([]string, 0, minInt(len(nodes), maxFactRetrievalNodes)),
		BudgetLimitedNodeIDs: CloneStrings(nodeIDs(nodes[minInt(len(nodes), maxFactRetrievalNodes):])),
		PromptContextReduced: len(nodes) > maxFactRetrievalNodes,
	}
	seen := make(map[string]struct{}, len(retrieval))
	for _, item := range retrieval {
		nodeID := strings.TrimSpace(asString(item["node_id"]))
		if nodeID == "" {
			continue
		}
		seen[nodeID] = struct{}{}
		summary.RetrievedNodeIDs = append(summary.RetrievedNodeIDs, nodeID)
		if truthy(item["results_limited"]) {
			summary.PromptContextReduced = true
		}
		if truthy(item["excerpt_truncated"]) {
			summary.ExcerptTruncated = true
		}
	}
	for _, node := range nodes {
		if _, ok := seen[node.ID]; ok {
			continue
		}
		summary.NoResultNodeIDs = append(summary.NoResultNodeIDs, node.ID)
	}
	if len(summary.RetrievedNodeIDs) == 0 && len(summary.NoResultNodeIDs) == 0 && !summary.PromptContextReduced && !summary.ExcerptTruncated {
		return retrieval, nil, nil
	}
	return retrieval, summary, nil
}
