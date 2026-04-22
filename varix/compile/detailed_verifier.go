package compile

import (
	"context"
	"fmt"
	"strings"
	"time"
)

const (
	promptNodeVerifierBull  = "node_bull"
	promptNodeVerifierBear  = "node_bear"
	promptNodeVerifierJudge = "node_judge"
	promptPathVerifierBull  = "path_bull"
	promptPathVerifierBear  = "path_bear"
	promptPathVerifierJudge = "path_judge"
)

func runDetailedVerifier(ctx context.Context, rt verifierCall, model string, prompts *promptRegistry, bundle Bundle, output Output) (Verification, error) {
	nodeVerifications, err := verifyNodesDetailed(ctx, rt, model, prompts, bundle, output.Graph.Nodes)
	if err != nil {
		return Verification{}, err
	}
	pathVerifications, err := verifyPathsDetailed(ctx, rt, model, prompts, bundle, output, nodeVerifications)
	if err != nil {
		return Verification{}, err
	}

	verification := Verification{
		Version:           "verify_v3",
		RolloutStage:      "node_and_path_dual",
		NodeVerifications: nodeVerifications,
		PathVerifications: pathVerifications,
		VerifiedAt:        verifierNow(),
		Model:             strings.TrimSpace(model),
	}
	applyCompatibilityVerificationViews(&verification, output.Graph.Nodes)
	return verification, nil
}

func verifyNodesDetailed(ctx context.Context, rt verifierCall, model string, prompts *promptRegistry, bundle Bundle, nodes []GraphNode) ([]NodeVerification, error) {
	if len(nodes) == 0 {
		return nil, nil
	}
	return verifyNodeBatchDetailed(ctx, rt, model, prompts, bundle, nodes)
}

func verifyNodeBatchDetailed(ctx context.Context, rt verifierCall, model string, prompts *promptRegistry, bundle Bundle, nodes []GraphNode) ([]NodeVerification, error) {
	bullInstruction, err := prompts.verifierInstruction(promptNodeVerifierBull)
	if err != nil {
		return nil, err
	}
	bullPrompt, err := buildNodeVerificationPrompt(bundle, nodes, nil)
	if err != nil {
		return nil, err
	}
	bullReq, err := BuildQwen36ProviderRequest(model, bundle, bullInstruction, bullPrompt)
	if err != nil {
		return nil, err
	}
	bullResp, _, err := callVerifierStage(ctx, rt, bullReq)
	if err != nil {
		return nil, err
	}
	var bullPayload struct {
		NodeVerifications []NodeVerification `json:"node_verifications"`
	}
	if err := unmarshalVerifierPayload(bullResp.Text, &bullPayload); err != nil {
		return nil, fmt.Errorf("parse node bull output: %w", err)
	}

	bearInstruction, err := prompts.verifierInstruction(promptNodeVerifierBear)
	if err != nil {
		return nil, err
	}
	bearPrompt, err := buildNodeVerificationPrompt(bundle, nodes, map[string]any{
		"bull_node_verifications": bullPayload.NodeVerifications,
	})
	if err != nil {
		return nil, err
	}
	bearReq, err := BuildQwen36ProviderRequest(model, bundle, bearInstruction, bearPrompt)
	if err != nil {
		return nil, err
	}
	bearResp, _, err := callVerifierStage(ctx, rt, bearReq)
	if err != nil {
		return nil, err
	}
	var bearPayload map[string]any
	if err := unmarshalVerifierPayload(bearResp.Text, &bearPayload); err != nil {
		return nil, fmt.Errorf("parse node bear output: %w", err)
	}

	judgeInstruction, err := prompts.verifierInstruction(promptNodeVerifierJudge)
	if err != nil {
		return nil, err
	}
	judgePrompt, err := buildNodeVerificationPrompt(bundle, nodes, map[string]any{
		"bull_node_verifications": bullPayload.NodeVerifications,
		"bear_node_objections":    bearPayload,
	})
	if err != nil {
		return nil, err
	}
	judgeReq, err := BuildQwen36ProviderRequest(model, bundle, judgeInstruction, judgePrompt)
	if err != nil {
		return nil, err
	}
	judgeResp, _, err := callVerifierStage(ctx, rt, judgeReq)
	if err != nil {
		return nil, err
	}
	var judgePayload struct {
		NodeVerifications []NodeVerification `json:"node_verifications"`
	}
	if err := unmarshalVerifierPayload(judgeResp.Text, &judgePayload); err != nil {
		return nil, fmt.Errorf("parse node judge output: %w", err)
	}
	return normalizeNodeVerifications(nodes, judgePayload.NodeVerifications), nil
}

func verifyPathsDetailed(ctx context.Context, rt verifierCall, model string, prompts *promptRegistry, bundle Bundle, output Output, nodeVerifications []NodeVerification) ([]PathVerification, error) {
	if len(output.TransmissionPaths) == 0 {
		return nil, nil
	}
	return verifyPathBatchDetailed(ctx, rt, model, prompts, bundle, output, nodeVerifications, output.TransmissionPaths)
}

func verifyPathBatchDetailed(ctx context.Context, rt verifierCall, model string, prompts *promptRegistry, bundle Bundle, output Output, nodeVerifications []NodeVerification, batch []TransmissionPath) ([]PathVerification, error) {
	bullInstruction, err := prompts.verifierInstruction(promptPathVerifierBull)
	if err != nil {
		return nil, err
	}
	bullPrompt, err := buildPathVerificationPrompt(bundle, output, nodeVerifications, batch, nil)
	if err != nil {
		return nil, err
	}
	bullReq, err := BuildQwen36ProviderRequest(model, bundle, bullInstruction, bullPrompt)
	if err != nil {
		return nil, err
	}
	bullResp, _, err := callVerifierStage(ctx, rt, bullReq)
	if err != nil {
		return nil, err
	}
	var bullPayload struct {
		PathVerifications []PathVerification `json:"path_verifications"`
	}
	if err := unmarshalVerifierPayload(bullResp.Text, &bullPayload); err != nil {
		return nil, fmt.Errorf("parse path bull output: %w", err)
	}

	bearInstruction, err := prompts.verifierInstruction(promptPathVerifierBear)
	if err != nil {
		return nil, err
	}
	bearPrompt, err := buildPathVerificationPrompt(bundle, output, nodeVerifications, batch, map[string]any{
		"bull_path_verifications": bullPayload.PathVerifications,
	})
	if err != nil {
		return nil, err
	}
	bearReq, err := BuildQwen36ProviderRequest(model, bundle, bearInstruction, bearPrompt)
	if err != nil {
		return nil, err
	}
	bearResp, _, err := callVerifierStage(ctx, rt, bearReq)
	if err != nil {
		return nil, err
	}
	var bearPayload map[string]any
	if err := unmarshalVerifierPayload(bearResp.Text, &bearPayload); err != nil {
		return nil, fmt.Errorf("parse path bear output: %w", err)
	}

	judgeInstruction, err := prompts.verifierInstruction(promptPathVerifierJudge)
	if err != nil {
		return nil, err
	}
	judgePrompt, err := buildPathVerificationPrompt(bundle, output, nodeVerifications, batch, map[string]any{
		"bull_path_verifications": bullPayload.PathVerifications,
		"bear_path_objections":    bearPayload,
	})
	if err != nil {
		return nil, err
	}
	judgeReq, err := BuildQwen36ProviderRequest(model, bundle, judgeInstruction, judgePrompt)
	if err != nil {
		return nil, err
	}
	judgeResp, _, err := callVerifierStage(ctx, rt, judgeReq)
	if err != nil {
		return nil, err
	}
	var judgePayload struct {
		PathVerifications []PathVerification `json:"path_verifications"`
	}
	if err := unmarshalVerifierPayload(judgeResp.Text, &judgePayload); err != nil {
		return nil, fmt.Errorf("parse path judge output: %w", err)
	}
	return normalizePathVerifications(batch, judgePayload.PathVerifications), nil
}

func buildNodeVerificationPrompt(bundle Bundle, nodes []GraphNode, extra map[string]any) (string, error) {
	return buildVerificationPrompt(bundle, nodes, extra)
}

func buildPathVerificationPrompt(bundle Bundle, output Output, nodeVerifications []NodeVerification, batch []TransmissionPath, extra map[string]any) (string, error) {
	if len(batch) == 0 {
		batch = output.TransmissionPaths
	}
	payload := map[string]any{
		"unit_id":            bundle.UnitID,
		"source":             bundle.Source,
		"external_id":        bundle.ExternalID,
		"summary":            output.Summary,
		"drivers":            output.Drivers,
		"targets":            output.Targets,
		"transmission_paths": batch,
		"node_verifications": nodeVerifications,
		"nodes":              marshalVerificationNodes(output.Graph.Nodes),
		"text_context":       bundle.TextContext(),
	}
	if !bundle.PostedAt.IsZero() {
		payload["posted_at"] = bundle.PostedAt.Format(time.RFC3339)
	}
	for key, value := range extra {
		payload[key] = value
	}
	encoded, err := marshalJSON(payload)
	if err != nil {
		return "", err
	}
	return encoded, nil
}

func applyCompatibilityVerificationViews(verification *Verification, nodes []GraphNode) {
	if verification == nil {
		return
	}
	nodeByID := make(map[string]GraphNode, len(nodes))
	for _, node := range nodes {
		nodeByID[node.ID] = node
	}
	for _, item := range verification.NodeVerifications {
		node, ok := nodeByID[item.NodeID]
		if !ok {
			continue
		}
		reason := compatibilityReason(item)
		switch node.Kind {
		case NodeFact, NodeMechanism:
			verification.FactChecks = append(verification.FactChecks, FactCheck{
				NodeID: item.NodeID,
				Status: mapNodeStatusToFactStatus(item.Status),
				Reason: reason,
			})
		case NodeExplicitCondition:
			verification.ExplicitConditionChecks = append(verification.ExplicitConditionChecks, ExplicitConditionCheck{
				NodeID: item.NodeID,
				Status: mapNodeStatusToExplicitStatus(item.Status),
				Reason: reason,
			})
		case NodeImplicitCondition:
			verification.ImplicitConditionChecks = append(verification.ImplicitConditionChecks, ImplicitConditionCheck{
				NodeID: item.NodeID,
				Status: mapNodeStatusToFactStatus(item.Status),
				Reason: reason,
			})
		case NodePrediction:
			verification.PredictionChecks = append(verification.PredictionChecks, PredictionCheck{
				NodeID: item.NodeID,
				Status: mapNodeStatusToPredictionStatus(item.Status),
				Reason: reason,
				AsOf:   item.AsOf,
			})
		}
	}
}

func compatibilityReason(item NodeVerification) string {
	return FirstNonEmpty(strings.TrimSpace(item.Reason), strings.TrimSpace(strings.Join(item.Evidence, "; ")))
}

func mapNodeStatusToFactStatus(status NodeVerificationStatus) FactStatus {
	switch status {
	case NodeVerificationProved:
		return FactStatusClearlyTrue
	case NodeVerificationFalsified:
		return FactStatusClearlyFalse
	default:
		return FactStatusUnverifiable
	}
}

func mapNodeStatusToExplicitStatus(status NodeVerificationStatus) ExplicitConditionStatus {
	switch status {
	case NodeVerificationProved:
		return ExplicitConditionStatusHigh
	case NodeVerificationFalsified:
		return ExplicitConditionStatusLow
	default:
		return ExplicitConditionStatusUnknown
	}
}

func mapNodeStatusToPredictionStatus(status NodeVerificationStatus) PredictionStatus {
	switch status {
	case NodeVerificationProved:
		return PredictionStatusResolvedTrue
	case NodeVerificationFalsified:
		return PredictionStatusResolvedFalse
	default:
		return PredictionStatusUnresolved
	}
}

func cloneGraphNodes(nodes []GraphNode) []GraphNode {
	out := make([]GraphNode, len(nodes))
	copy(out, nodes)
	return out
}

func normalizeNodeVerifications(nodes []GraphNode, got []NodeVerification) []NodeVerification {
	if len(nodes) == 0 {
		return nil
	}
	index := make(map[string]NodeVerification, len(got))
	for _, item := range got {
		index[strings.TrimSpace(item.NodeID)] = item
	}
	out := make([]NodeVerification, 0, len(nodes))
	for _, node := range nodes {
		item, ok := index[node.ID]
		if !ok {
			item = NodeVerification{
				NodeID:   node.ID,
				Status:   NodeVerificationWaiting,
				Reason:   "模型未返回该节点的验证结果",
				NodeText: node.Text,
				NodeKind: string(node.Kind),
			}
		}
		if strings.TrimSpace(item.NodeText) == "" {
			item.NodeText = node.Text
		}
		if strings.TrimSpace(item.NodeKind) == "" {
			item.NodeKind = string(node.Kind)
		}
		out = append(out, item)
	}
	return out
}

func normalizePathVerifications(paths []TransmissionPath, got []PathVerification) []PathVerification {
	if len(paths) == 0 {
		return nil
	}
	type key struct {
		driver string
		target string
	}
	index := make(map[key]PathVerification, len(got))
	for _, item := range got {
		index[key{driver: strings.TrimSpace(item.Driver), target: strings.TrimSpace(item.Target)}] = item
	}
	out := make([]PathVerification, 0, len(paths))
	for _, path := range paths {
		item, ok := index[key{driver: strings.TrimSpace(path.Driver), target: strings.TrimSpace(path.Target)}]
		if !ok {
			item = PathVerification{
				Driver:   path.Driver,
				Target:   path.Target,
				Steps:    CloneStrings(path.Steps),
				Status:   PathVerificationProblem,
				Complete: false,
				Rigorous: false,
				Reason:   "模型未返回该路径的验证结果",
			}
		}
		if len(item.Steps) == 0 {
			item.Steps = CloneStrings(path.Steps)
		}
		out = append(out, item)
	}
	return out
}
