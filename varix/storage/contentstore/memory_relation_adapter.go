package contentstore

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/kumaloha/VariX/varix/compile"
	"github.com/kumaloha/VariX/varix/memory"
)

type relationFirstProjection struct {
	canonicalEntities []memory.CanonicalEntity
	relations         []memory.Relation
	mechanisms        []memory.Mechanism
	mechanismNodes    []memory.MechanismNode
	mechanismEdges    []memory.MechanismEdge
	pathOutcomes      []memory.PathOutcome
	driverAggregates  []memory.DriverAggregate
	targetAggregates  []memory.TargetAggregate
	conflictViews     []memory.ConflictView
}

func (s *SQLiteStore) buildRelationFirstProjection(
	ctx context.Context,
	now time.Time,
	causalTheses []memory.CausalThesis,
	conclusions []memory.CognitiveConclusion,
) (relationFirstProjection, error) {
	if len(causalTheses) == 0 {
		return relationFirstProjection{}, nil
	}

	compiledNodes, err := s.compiledNodesForTheses(ctx, causalTheses)
	if err != nil {
		return relationFirstProjection{}, err
	}

	entityStates := map[string]*memory.CanonicalEntity{}
	relationIndex := map[string]memory.Relation{}
	mechanisms := make([]memory.Mechanism, 0, len(causalTheses))
	mechanismNodes := make([]memory.MechanismNode, 0)
	mechanismEdges := make([]memory.MechanismEdge, 0)
	pathOutcomes := make([]memory.PathOutcome, 0, len(causalTheses))
	driverStates := map[string]*aggregateState{}
	targetStates := map[string]*aggregateState{}

	conclusionByThesis := make(map[string][]memory.CognitiveConclusion)
	for _, conclusion := range conclusions {
		if strings.TrimSpace(conclusion.CausalThesisID) == "" {
			continue
		}
		conclusionByThesis[conclusion.CausalThesisID] = append(conclusionByThesis[conclusion.CausalThesisID], conclusion)
	}

	for _, thesis := range causalTheses {
		if len(thesis.CorePathNodeIDs) == 0 {
			continue
		}
		driverLabel, targetLabel, ok := relationBoundaryLabels(thesis, compiledNodes)
		if !ok {
			continue
		}
		driverEntityID := relationEntityID(memory.CanonicalEntityDriver, driverLabel)
		targetEntityID := relationEntityID(memory.CanonicalEntityTarget, targetLabel)
		addCanonicalEntity(entityStates, driverEntityID, memory.CanonicalEntityDriver, driverLabel, now)
		addCanonicalEntity(entityStates, targetEntityID, memory.CanonicalEntityTarget, targetLabel, now)

		relationID := thesis.ThesisID + "-relation"
		relation := memory.Relation{
			RelationID:     relationID,
			DriverEntityID: driverEntityID,
			TargetEntityID: targetEntityID,
			Status:         memory.RelationActive,
			CreatedAt:      now,
			UpdatedAt:      now,
		}
		relationIndex[relationID] = relation

		mechanismID := thesis.CausalThesisID + "-mechanism"
		traceabilityStatus := traceabilityStatusForThesis(thesis)
		mechanism := memory.Mechanism{
			MechanismID:        mechanismID,
			RelationID:         relationID,
			AsOf:               now,
			Confidence:         boundedConfidence(thesis.CompletenessScore),
			Status:             memory.MechanismActive,
			SourceRefs:         append([]string(nil), thesis.SourceRefs...),
			TraceabilityStatus: traceabilityStatus,
			CreatedAt:          now,
			UpdatedAt:          now,
		}
		mechanisms = append(mechanisms, mechanism)

		nodeIDMap := make(map[string]string, len(thesis.CorePathNodeIDs))
		for idx, globalNodeID := range thesis.CorePathNodeIDs {
			label := relationNodeLabel(globalNodeID, compiledNodes)
			nodeType := normalizedTransmissionNodeType(thesis.NodeRoles[globalNodeID], idx, len(thesis.CorePathNodeIDs))
			mechanismNodeID := fmt.Sprintf("%s-node-%d", mechanismID, idx+1)
			nodeIDMap[globalNodeID] = mechanismNodeID
			mechanismNodes = append(mechanismNodes, memory.MechanismNode{
				MechanismNodeID:        mechanismNodeID,
				MechanismID:            mechanismID,
				NodeType:               nodeType,
				Label:                  label,
				BackingAcceptedNodeIDs: []string{globalNodeID},
				SortOrder:              idx + 1,
				CreatedAt:              now,
			})
		}

		edgeIDs := make([]string, 0, len(thesis.CorePathNodeIDs))
		for idx := 0; idx+1 < len(thesis.CorePathNodeIDs); idx++ {
			fromGlobal := thesis.CorePathNodeIDs[idx]
			toGlobal := thesis.CorePathNodeIDs[idx+1]
			edgeID := fmt.Sprintf("%s-edge-%d", mechanismID, idx+1)
			edgeIDs = append(edgeIDs, edgeID)
			mechanismEdges = append(mechanismEdges, memory.MechanismEdge{
				MechanismEdgeID: edgeID,
				MechanismID:     mechanismID,
				FromNodeID:      nodeIDMap[fromGlobal],
				ToNodeID:        nodeIDMap[toGlobal],
				EdgeType:        normalizedTransmissionEdgeType(thesis.NodeRoles[fromGlobal], thesis.NodeRoles[toGlobal]),
				PathOrder:       idx + 1,
				CreatedAt:       now,
			})
		}

		nodePath := make([]string, 0, len(thesis.CorePathNodeIDs))
		for _, globalNodeID := range thesis.CorePathNodeIDs {
			nodePath = append(nodePath, nodeIDMap[globalNodeID])
		}
		predictionNodeIDs, predictionStartAt, predictionDueAt := predictionContractForPath(thesis.CorePathNodeIDs, nodeIDMap, compiledNodes)
		pathOutcomes = append(pathOutcomes, memory.PathOutcome{
			PathOutcomeID:     mechanismID + "-path-1",
			MechanismID:       mechanismID,
			NodePath:          nodePath,
			EdgePath:          edgeIDs,
			OutcomePolarity:   inferOutcomePolarity(targetLabel, relationConditionScope(thesis, compiledNodes)),
			OutcomeLabel:      targetLabel,
			ConditionScope:    relationConditionScope(thesis, compiledNodes),
			PredictionNodeIDs: predictionNodeIDs,
			PredictionStartAt: predictionStartAt,
			PredictionDueAt:   predictionDueAt,
			Confidence:        boundedConfidence(thesis.CompletenessScore),
			CreatedAt:         now,
		})

		driverState := ensureAggregateState(driverStates, driverEntityID)
		driverState.relationIDs = append(driverState.relationIDs, relationID)
		driverState.neighborEntityIDs = append(driverState.neighborEntityIDs, targetEntityID)
		driverState.mechanismLabels = append(driverState.mechanismLabels, targetLabel)
		driverState.coverageScore = maxFloat(driverState.coverageScore, boundedConfidence(thesis.CompletenessScore))
		driverState.traceabilityStatus = strongerTraceability(driverState.traceabilityStatus, traceabilityStatus)
		for _, conclusion := range conclusionByThesis[thesis.CausalThesisID] {
			driverState.activeConclusionIDs = append(driverState.activeConclusionIDs, conclusion.ConclusionID)
		}

		targetState := ensureAggregateState(targetStates, targetEntityID)
		targetState.relationIDs = append(targetState.relationIDs, relationID)
		targetState.neighborEntityIDs = append(targetState.neighborEntityIDs, driverEntityID)
		targetState.mechanismLabels = append(targetState.mechanismLabels, driverLabel)
		targetState.coverageScore = maxFloat(targetState.coverageScore, boundedConfidence(thesis.CompletenessScore))
		targetState.traceabilityStatus = strongerTraceability(targetState.traceabilityStatus, traceabilityStatus)
		for _, conclusion := range conclusionByThesis[thesis.CausalThesisID] {
			targetState.activeConclusionIDs = append(targetState.activeConclusionIDs, conclusion.ConclusionID)
		}
	}

	canonicalEntities := make([]memory.CanonicalEntity, 0, len(entityStates))
	for _, entity := range entityStates {
		entity.Aliases = normalizeCanonicalAliases(entity.CanonicalName, entity.Aliases)
		canonicalEntities = append(canonicalEntities, *entity)
	}
	sort.Slice(canonicalEntities, func(i, j int) bool { return canonicalEntities[i].EntityID < canonicalEntities[j].EntityID })

	relations := make([]memory.Relation, 0, len(relationIndex))
	for _, relation := range relationIndex {
		relations = append(relations, relation)
	}
	sort.Slice(relations, func(i, j int) bool { return relations[i].RelationID < relations[j].RelationID })
	sort.Slice(mechanisms, func(i, j int) bool { return mechanisms[i].MechanismID < mechanisms[j].MechanismID })
	sort.Slice(mechanismNodes, func(i, j int) bool {
		if mechanismNodes[i].MechanismID != mechanismNodes[j].MechanismID {
			return mechanismNodes[i].MechanismID < mechanismNodes[j].MechanismID
		}
		return mechanismNodes[i].SortOrder < mechanismNodes[j].SortOrder
	})
	sort.Slice(mechanismEdges, func(i, j int) bool {
		if mechanismEdges[i].MechanismID != mechanismEdges[j].MechanismID {
			return mechanismEdges[i].MechanismID < mechanismEdges[j].MechanismID
		}
		return mechanismEdges[i].PathOrder < mechanismEdges[j].PathOrder
	})
	sort.Slice(pathOutcomes, func(i, j int) bool { return pathOutcomes[i].PathOutcomeID < pathOutcomes[j].PathOutcomeID })

	return relationFirstProjection{
		canonicalEntities: canonicalEntities,
		relations:         relations,
		mechanisms:        mechanisms,
		mechanismNodes:    mechanismNodes,
		mechanismEdges:    mechanismEdges,
		pathOutcomes:      pathOutcomes,
		driverAggregates:  buildDriverAggregatesFromState(driverStates, now),
		targetAggregates:  buildTargetAggregatesFromState(targetStates, now),
	}, nil
}

func (s *SQLiteStore) compiledNodesForTheses(ctx context.Context, theses []memory.CausalThesis) (map[string]compile.GraphNode, error) {
	sourceRefs := map[string]struct{}{}
	for _, thesis := range theses {
		for _, ref := range thesis.SourceRefs {
			ref = strings.TrimSpace(ref)
			if ref != "" {
				sourceRefs[ref] = struct{}{}
			}
		}
		for _, nodeID := range thesis.CorePathNodeIDs {
			platform, externalID, _, ok := splitGlobalNodeRef(nodeID)
			if ok {
				sourceRefs[platform+":"+externalID] = struct{}{}
			}
		}
	}

	out := make(map[string]compile.GraphNode)
	for ref := range sourceRefs {
		parts := strings.SplitN(ref, ":", 2)
		if len(parts) != 2 {
			continue
		}
		record, err := s.GetCompiledOutput(ctx, parts[0], parts[1])
		if err != nil {
			continue
		}
		for _, node := range record.Output.Graph.Nodes {
			out[parts[0]+":"+parts[1]+":"+node.ID] = node
		}
	}
	return out, nil
}

func relationBoundaryLabels(thesis memory.CausalThesis, compiledNodes map[string]compile.GraphNode) (driverLabel, targetLabel string, ok bool) {
	if len(thesis.CorePathNodeIDs) == 0 {
		return "", "", false
	}
	driverLabel = relationNodeLabel(thesis.CorePathNodeIDs[0], compiledNodes)
	targetLabel = relationNodeLabel(thesis.CorePathNodeIDs[len(thesis.CorePathNodeIDs)-1], compiledNodes)
	return strings.TrimSpace(driverLabel), strings.TrimSpace(targetLabel), strings.TrimSpace(driverLabel) != "" && strings.TrimSpace(targetLabel) != ""
}

func relationNodeLabel(globalNodeID string, compiledNodes map[string]compile.GraphNode) string {
	if node, ok := compiledNodes[globalNodeID]; ok {
		return normalizeCanonicalDisplay(node.Text)
	}
	return normalizeCanonicalDisplay(extractNodeTextFromGlobalRef(globalNodeID))
}

func extractNodeTextFromGlobalRef(globalNodeID string) string {
	parts := strings.Split(globalNodeID, ":")
	if len(parts) == 0 {
		return globalNodeID
	}
	return parts[len(parts)-1]
}

func addCanonicalEntity(states map[string]*memory.CanonicalEntity, entityID string, entityType memory.CanonicalEntityType, canonicalName string, now time.Time) {
	canonicalName = normalizeCanonicalDisplay(canonicalName)
	if canonicalName == "" || strings.TrimSpace(entityID) == "" {
		return
	}
	if existing, ok := states[entityID]; ok {
		existing.EntityType = combineEntityTypes(existing.EntityType, entityType)
		existing.UpdatedAt = now
		return
	}
	states[entityID] = &memory.CanonicalEntity{
		EntityID:      entityID,
		EntityType:    entityType,
		CanonicalName: canonicalName,
		Status:        memory.CanonicalEntityActive,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
}

func relationEntityID(entityType memory.CanonicalEntityType, label string) string {
	label = normalizeCanonicalAlias(label)
	if label == "" {
		return ""
	}
	return fmt.Sprintf("%s:%s", entityType, label)
}

func combineEntityTypes(left, right memory.CanonicalEntityType) memory.CanonicalEntityType {
	if left == "" {
		return right
	}
	if right == "" || left == right {
		return left
	}
	return memory.CanonicalEntityBoth
}

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

func strongerTraceability(left, right memory.TraceabilityStatus) memory.TraceabilityStatus {
	order := map[memory.TraceabilityStatus]int{
		memory.TraceabilityWeak:     0,
		memory.TraceabilityPartial:  1,
		memory.TraceabilityComplete: 2,
	}
	if order[right] > order[left] {
		return right
	}
	if left == "" {
		return right
	}
	return left
}

func ensureAggregateState(states map[string]*aggregateState, key string) *aggregateState {
	if state, ok := states[key]; ok {
		return state
	}
	state := &aggregateState{traceabilityStatus: memory.TraceabilityWeak}
	states[key] = state
	return state
}

type aggregateState struct {
	relationIDs         []string
	neighborEntityIDs   []string
	mechanismLabels     []string
	activeConclusionIDs []string
	traceabilityStatus  memory.TraceabilityStatus
	coverageScore       float64
	conflictCount       int
}

func buildDriverAggregatesFromState(states map[string]*aggregateState, now time.Time) []memory.DriverAggregate {
	out := make([]memory.DriverAggregate, 0, len(states))
	for driverEntityID, state := range states {
		out = append(out, memory.DriverAggregate{
			AggregateID:         driverEntityID + "-aggregate",
			DriverEntityID:      driverEntityID,
			RelationIDs:         uniquePreservingOrder(state.relationIDs),
			TargetEntityIDs:     uniquePreservingOrder(state.neighborEntityIDs),
			MechanismLabels:     uniquePreservingOrder(state.mechanismLabels),
			CoverageScore:       state.coverageScore,
			ConflictCount:       state.conflictCount,
			ActiveConclusionIDs: uniquePreservingOrder(state.activeConclusionIDs),
			TraceabilityStatus:  state.traceabilityStatus,
			AsOf:                now,
			CreatedAt:           now,
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].AggregateID < out[j].AggregateID })
	return out
}

func buildTargetAggregatesFromState(states map[string]*aggregateState, now time.Time) []memory.TargetAggregate {
	out := make([]memory.TargetAggregate, 0, len(states))
	for targetEntityID, state := range states {
		out = append(out, memory.TargetAggregate{
			AggregateID:         targetEntityID + "-aggregate",
			TargetEntityID:      targetEntityID,
			RelationIDs:         uniquePreservingOrder(state.relationIDs),
			DriverEntityIDs:     uniquePreservingOrder(state.neighborEntityIDs),
			MechanismLabels:     uniquePreservingOrder(state.mechanismLabels),
			CoverageScore:       state.coverageScore,
			ConflictCount:       state.conflictCount,
			ActiveConclusionIDs: uniquePreservingOrder(state.activeConclusionIDs),
			TraceabilityStatus:  state.traceabilityStatus,
			AsOf:                now,
			CreatedAt:           now,
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].AggregateID < out[j].AggregateID })
	return out
}

func maxFloat(left, right float64) float64 {
	if right > left {
		return right
	}
	return left
}
