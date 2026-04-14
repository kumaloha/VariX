package compile

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/kumaloha/forge/llm"
)

type verifierCall interface {
	Call(ctx context.Context, req llm.ProviderRequest) (llm.Response, error)
}

func runVerifier(ctx context.Context, rt verifierCall, model string, bundle Bundle, output Output) (Verification, error) {
	verification := Verification{}

	factNodes := make([]GraphNode, 0)
	explicitConditionNodes := make([]GraphNode, 0)
	implicitConditionNodes := make([]GraphNode, 0)
	predictionNodes := make([]GraphNode, 0)
	for _, node := range output.Graph.Nodes {
		switch node.Kind {
		case NodeFact:
			factNodes = append(factNodes, node)
		case NodeExplicitCondition:
			explicitConditionNodes = append(explicitConditionNodes, node)
		case NodeImplicitCondition:
			implicitConditionNodes = append(implicitConditionNodes, node)
		case NodePrediction:
			predictionNodes = append(predictionNodes, node)
		}
	}

	var verifierModel string
	if len(factNodes) > 0 {
		facts, modelName, err := verifyFacts(ctx, rt, model, bundle, factNodes)
		if err != nil {
			return Verification{}, err
		}
		verification.FactChecks = facts
		verifierModel = firstNonEmpty(modelName, verifierModel)
	}
	if len(explicitConditionNodes) > 0 {
		checks, modelName, err := verifyExplicitConditions(ctx, rt, model, bundle, explicitConditionNodes)
		if err != nil {
			return Verification{}, err
		}
		verification.ExplicitConditionChecks = checks
		verifierModel = firstNonEmpty(modelName, verifierModel)
	}
	if len(implicitConditionNodes) > 0 {
		checks, modelName, err := verifyImplicitConditions(ctx, rt, model, bundle, implicitConditionNodes)
		if err != nil {
			return Verification{}, err
		}
		verification.ImplicitConditionChecks = checks
		verifierModel = firstNonEmpty(modelName, verifierModel)
	}
	if len(predictionNodes) > 0 {
		predictions, modelName, err := verifyPredictions(ctx, rt, model, bundle, predictionNodes)
		if err != nil {
			return Verification{}, err
		}
		verification.PredictionChecks = predictions
		verifierModel = firstNonEmpty(modelName, verifierModel)
	}

	if len(verification.FactChecks) > 0 || len(verification.ExplicitConditionChecks) > 0 || len(verification.ImplicitConditionChecks) > 0 || len(verification.PredictionChecks) > 0 {
		verification.VerifiedAt = time.Now().UTC()
		verification.Model = firstNonEmpty(verifierModel, model)
	}
	return verification, nil
}

func verifyFacts(ctx context.Context, rt verifierCall, model string, bundle Bundle, nodes []GraphNode) ([]FactCheck, string, error) {
	prompt, err := buildFactVerificationPrompt(bundle, nodes)
	if err != nil {
		return nil, "", err
	}
	req, err := BuildQwen36ProviderRequest(model, bundle, factVerifierInstruction, prompt)
	if err != nil {
		return nil, "", err
	}
	resp, err := rt.Call(ctx, req)
	if err != nil {
		return nil, "", err
	}
	var payload struct {
		FactChecks []FactCheck `json:"fact_checks"`
	}
	if err := unmarshalVerifierPayload(resp.Text, &payload); err != nil {
		return nil, "", err
	}
	return payload.FactChecks, resp.Model, nil
}

func verifyPredictions(ctx context.Context, rt verifierCall, model string, bundle Bundle, nodes []GraphNode) ([]PredictionCheck, string, error) {
	prompt, err := buildPredictionVerificationPrompt(bundle, nodes)
	if err != nil {
		return nil, "", err
	}
	req, err := BuildQwen36ProviderRequest(model, bundle, predictionVerifierInstruction, prompt)
	if err != nil {
		return nil, "", err
	}
	resp, err := rt.Call(ctx, req)
	if err != nil {
		return nil, "", err
	}
	var payload struct {
		PredictionChecks []PredictionCheck `json:"prediction_checks"`
	}
	if err := unmarshalVerifierPayload(resp.Text, &payload); err != nil {
		return nil, "", err
	}
	return payload.PredictionChecks, resp.Model, nil
}

func verifyExplicitConditions(ctx context.Context, rt verifierCall, model string, bundle Bundle, nodes []GraphNode) ([]ExplicitConditionCheck, string, error) {
	prompt, err := buildExplicitConditionVerificationPrompt(bundle, nodes)
	if err != nil {
		return nil, "", err
	}
	req, err := BuildQwen36ProviderRequest(model, bundle, explicitConditionVerifierInstruction, prompt)
	if err != nil {
		return nil, "", err
	}
	resp, err := rt.Call(ctx, req)
	if err != nil {
		return nil, "", err
	}
	var payload struct {
		ExplicitConditionChecks []ExplicitConditionCheck `json:"explicit_condition_checks"`
	}
	if err := unmarshalVerifierPayload(resp.Text, &payload); err != nil {
		return nil, "", err
	}
	return payload.ExplicitConditionChecks, resp.Model, nil
}

func verifyImplicitConditions(ctx context.Context, rt verifierCall, model string, bundle Bundle, nodes []GraphNode) ([]ImplicitConditionCheck, string, error) {
	prompt, err := buildImplicitConditionVerificationPrompt(bundle, nodes)
	if err != nil {
		return nil, "", err
	}
	req, err := BuildQwen36ProviderRequest(model, bundle, implicitConditionVerifierInstruction, prompt)
	if err != nil {
		return nil, "", err
	}
	resp, err := rt.Call(ctx, req)
	if err != nil {
		return nil, "", err
	}
	var payload struct {
		ImplicitConditionChecks []ImplicitConditionCheck `json:"implicit_condition_checks"`
	}
	if err := unmarshalVerifierPayload(resp.Text, &payload); err != nil {
		return nil, "", err
	}
	return payload.ImplicitConditionChecks, resp.Model, nil
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

func buildFactVerificationPrompt(bundle Bundle, nodes []GraphNode) (string, error) {
	payload := map[string]any{
		"unit_id":      bundle.UnitID,
		"source":       bundle.Source,
		"external_id":  bundle.ExternalID,
		"nodes":        nodes,
		"text_context": bundle.TextContext(),
	}
	encoded, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return "", err
	}
	return string(encoded), nil
}

func buildPredictionVerificationPrompt(bundle Bundle, nodes []GraphNode) (string, error) {
	payload := map[string]any{
		"unit_id":      bundle.UnitID,
		"source":       bundle.Source,
		"external_id":  bundle.ExternalID,
		"as_of":        time.Now().UTC().Format(time.RFC3339),
		"nodes":        nodes,
		"text_context": bundle.TextContext(),
	}
	encoded, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return "", err
	}
	return string(encoded), nil
}

func buildExplicitConditionVerificationPrompt(bundle Bundle, nodes []GraphNode) (string, error) {
	payload := map[string]any{
		"unit_id":      bundle.UnitID,
		"source":       bundle.Source,
		"external_id":  bundle.ExternalID,
		"nodes":        nodes,
		"text_context": bundle.TextContext(),
	}
	encoded, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return "", err
	}
	return string(encoded), nil
}

func buildImplicitConditionVerificationPrompt(bundle Bundle, nodes []GraphNode) (string, error) {
	payload := map[string]any{
		"unit_id":      bundle.UnitID,
		"source":       bundle.Source,
		"external_id":  bundle.ExternalID,
		"nodes":        nodes,
		"text_context": bundle.TextContext(),
	}
	encoded, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return "", err
	}
	return string(encoded), nil
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

const factVerifierInstruction = `
你是一个事实节点验证器。只看输入中的事实节点，返回 JSON：
{
  "fact_checks": [
    {"node_id":"n1","status":"clearly_true|clearly_false|unverifiable","reason":"..."}
  ]
}
不要返回多余文本。`

const predictionVerifierInstruction = `
你是一个预测节点验证器。只看输入中的预测节点，返回 JSON：
{
  "prediction_checks": [
    {"node_id":"n1","status":"unresolved|resolved_true|resolved_false|stale_unresolved","reason":"...","as_of":"2026-04-14T00:00:00Z"}
  ]
}
不要返回多余文本。`

const explicitConditionVerifierInstruction = `
你是一个显式条件评估器。只看输入中的显式条件节点，返回 JSON：
{
  "explicit_condition_checks": [
    {"node_id":"n1","status":"high|medium|low|unknown","reason":"..."}
  ]
}
不要返回多余文本。`

const implicitConditionVerifierInstruction = `
你是一个隐含条件验证器。只看输入中的隐含条件节点，返回 JSON：
{
  "implicit_condition_checks": [
    {"node_id":"n1","status":"clearly_true|clearly_false|unverifiable","reason":"..."}
  ]
}
不要返回多余文本。`
