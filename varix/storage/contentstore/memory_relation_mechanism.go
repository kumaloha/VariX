package contentstore

import (
	"github.com/kumaloha/VariX/varix/compile"
	"github.com/kumaloha/VariX/varix/memory"
	"strings"
	"time"
)

func normalizedTransmissionNodeType(role string, index, total int) memory.MechanismNodeType {
	if total <= 0 {
		return memory.MechanismNodeMarketBehavior
	}
	if index == 0 {
		return memory.MechanismNodeDriver
	}
	if index == total-1 {
		return memory.MechanismNodeTargetEffect
	}
	switch role {
	case "condition":
		return memory.MechanismNodeCondition
	case "mechanism":
		return memory.MechanismNodeMarketBehavior
	case "fact":
		return memory.MechanismNodeMacroEvent
	case "conclusion":
		return memory.MechanismNodeTargetEffect
	case "prediction":
		return memory.MechanismNodeTargetEffect
	default:
		return memory.MechanismNodeMarketBehavior
	}
}

func normalizedTransmissionEdgeType(fromRole, toRole string) memory.MechanismEdgeType {
	switch {
	case fromRole == "condition":
		return memory.MechanismEdgePresets
	case toRole == "prediction":
		return memory.MechanismEdgeTransmits
	case fromRole == "mechanism":
		return memory.MechanismEdgeTransmits
	default:
		return memory.MechanismEdgeCauses
	}
}

func relationConditionScope(thesis memory.CausalThesis, compiledNodes map[string]compile.GraphNode) string {
	for _, nodeID := range thesis.CorePathNodeIDs {
		if thesis.NodeRoles[nodeID] == "condition" {
			return relationNodeLabel(nodeID, compiledNodes)
		}
	}
	return ""
}

func predictionContractForPath(corePath []string, nodeIDMap map[string]string, compiledNodes map[string]compile.GraphNode) ([]string, time.Time, time.Time) {
	predictionNodeIDs := make([]string, 0)
	var predictionStartAt time.Time
	var predictionDueAt time.Time
	for _, globalNodeID := range corePath {
		node, ok := compiledNodes[globalNodeID]
		if !ok || node.Kind != compile.NodePrediction {
			continue
		}
		if mechanismNodeID := nodeIDMap[globalNodeID]; mechanismNodeID != "" {
			predictionNodeIDs = append(predictionNodeIDs, mechanismNodeID)
		}
		if predictionStartAt.IsZero() || (!node.PredictionStartAt.IsZero() && node.PredictionStartAt.Before(predictionStartAt)) {
			predictionStartAt = node.PredictionStartAt
		}
		if node.PredictionDueAt.After(predictionDueAt) {
			predictionDueAt = node.PredictionDueAt
		}
	}
	return predictionNodeIDs, predictionStartAt, predictionDueAt
}

func inferOutcomePolarity(outcomeLabel, conditionScope string) memory.OutcomePolarity {
	text := normalizeCanonicalDisplay(outcomeLabel + " " + conditionScope)
	switch {
	case containsAnyText(text, "上涨", "上升", "推高", "走强", "改善", "增长", "修复", "回暖"):
		return memory.OutcomeBullish
	case containsAnyText(text, "下跌", "下降", "承压", "走弱", "恶化", "收缩", "违约", "挤兑", "冲击", "风险"):
		return memory.OutcomeBearish
	case strings.TrimSpace(conditionScope) != "":
		return memory.OutcomeConditional
	default:
		return memory.OutcomeUnresolved
	}
}

func boundedConfidence(score float64) float64 {
	switch {
	case score < 0:
		return 0
	case score > 1:
		return 1
	default:
		return score
	}
}

func traceabilityStatusForThesis(thesis memory.CausalThesis) memory.TraceabilityStatus {
	switch {
	case len(thesis.CorePathNodeIDs) >= 4:
		return memory.TraceabilityComplete
	case len(thesis.CorePathNodeIDs) >= 2:
		return memory.TraceabilityPartial
	default:
		return memory.TraceabilityWeak
	}
}
