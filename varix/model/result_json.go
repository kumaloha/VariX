package model

import (
	"strings"
)

func defaultFormFunctionForKind(kind NodeKind) (NodeForm, NodeFunction, bool) {
	switch kind {
	case NodeFact:
		return NodeFormObservation, NodeFunctionSupport, true
	case NodeExplicitCondition:
		return NodeFormCondition, NodeFunctionClaim, true
	case NodeImplicitCondition:
		return NodeFormCondition, NodeFunctionSupport, true
	case NodeMechanism:
		return NodeFormObservation, NodeFunctionTransmission, true
	case NodeConclusion:
		return NodeFormJudgment, NodeFunctionClaim, true
	case NodePrediction:
		return NodeFormForecast, NodeFunctionClaim, true
	default:
		return "", "", false
	}
}

func normalizeEdgeKind(kind EdgeKind) EdgeKind {
	switch strings.TrimSpace(string(kind)) {
	case "正向", "drives":
		return EdgePositive
	case "推出", "substantiates":
		return EdgeDerives
	case "预设", "gates":
		return EdgePresets
	case "解释", "explains":
		return EdgeExplains
	default:
		return kind
	}
}

func inferNodeKindFromFormFunction(form NodeForm, function NodeFunction, text string) (NodeKind, bool) {
	switch form {
	case NodeFormObservation:
		switch function {
		case NodeFunctionSupport:
			return NodeFact, true
		case NodeFunctionTransmission:
			return NodeMechanism, true
		}
	case NodeFormCondition:
		switch function {
		case NodeFunctionClaim:
			return NodeExplicitCondition, true
		case NodeFunctionSupport, NodeFunctionTransmission:
			return NodeImplicitCondition, true
		case "":
			if isExplicitConditionText(text) {
				return NodeExplicitCondition, true
			}
			return NodeImplicitCondition, true
		}
	case NodeFormJudgment:
		if function == NodeFunctionClaim || function == "" {
			return NodeConclusion, true
		}
	case NodeFormForecast:
		if function == NodeFunctionClaim || function == "" {
			return NodePrediction, true
		}
	}
	return "", false
}
