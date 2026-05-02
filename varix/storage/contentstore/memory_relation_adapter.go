package contentstore

import (
	"context"
	"fmt"
	"github.com/kumaloha/VariX/varix/memory"
	"sort"
	"time"
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
	state := newRelationProjectionState(causalTheses, conclusions)

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
		addCanonicalEntity(state.entityStates, driverEntityID, memory.CanonicalEntityDriver, driverLabel, now)
		addCanonicalEntity(state.entityStates, targetEntityID, memory.CanonicalEntityTarget, targetLabel, now)

		relationID := thesis.ThesisID + "-relation"
		relation := memory.Relation{
			RelationID:     relationID,
			DriverEntityID: driverEntityID,
			TargetEntityID: targetEntityID,
			Status:         memory.RelationActive,
			CreatedAt:      now,
			UpdatedAt:      now,
		}
		state.relationIndex[relationID] = relation

		mechanismID := thesis.CausalThesisID + "-mechanism"
		traceabilityStatus := traceabilityStatusForThesis(thesis)
		mechanism := memory.Mechanism{
			MechanismID:        mechanismID,
			RelationID:         relationID,
			AsOf:               now,
			Confidence:         boundedConfidence(thesis.CompletenessScore),
			Status:             memory.MechanismActive,
			SourceRefs:         cloneStringSlice(thesis.SourceRefs),
			TraceabilityStatus: traceabilityStatus,
			CreatedAt:          now,
			UpdatedAt:          now,
		}
		state.mechanisms = append(state.mechanisms, mechanism)

		nodeIDMap := make(map[string]string, len(thesis.CorePathNodeIDs))
		for idx, globalNodeID := range thesis.CorePathNodeIDs {
			label := relationNodeLabel(globalNodeID, compiledNodes)
			nodeType := normalizedTransmissionNodeType(thesis.NodeRoles[globalNodeID], idx, len(thesis.CorePathNodeIDs))
			mechanismNodeID := fmt.Sprintf("%s-node-%d", mechanismID, idx+1)
			nodeIDMap[globalNodeID] = mechanismNodeID
			state.mechanismNodes = append(state.mechanismNodes, memory.MechanismNode{
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
			state.mechanismEdges = append(state.mechanismEdges, memory.MechanismEdge{
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
		state.pathOutcomes = append(state.pathOutcomes, memory.PathOutcome{
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

		conclusionIDs := conclusionIDsForThesis(state.conclusionByThesis[thesis.CausalThesisID])
		updateAggregateState(
			ensureAggregateState(state.driverStates, driverEntityID),
			relationID,
			targetEntityID,
			targetLabel,
			boundedConfidence(thesis.CompletenessScore),
			traceabilityStatus,
			conclusionIDs,
		)
		updateAggregateState(
			ensureAggregateState(state.targetStates, targetEntityID),
			relationID,
			driverEntityID,
			driverLabel,
			boundedConfidence(thesis.CompletenessScore),
			traceabilityStatus,
			conclusionIDs,
		)
	}

	canonicalEntities := make([]memory.CanonicalEntity, 0, len(state.entityStates))
	for _, entity := range state.entityStates {
		entity.Aliases = normalizeCanonicalAliases(entity.CanonicalName, entity.Aliases)
		canonicalEntities = append(canonicalEntities, *entity)
	}
	sort.Slice(canonicalEntities, func(i, j int) bool { return canonicalEntities[i].EntityID < canonicalEntities[j].EntityID })

	relations := make([]memory.Relation, 0, len(state.relationIndex))
	for _, relation := range state.relationIndex {
		relations = append(relations, relation)
	}
	sort.Slice(relations, func(i, j int) bool { return relations[i].RelationID < relations[j].RelationID })
	sort.Slice(state.mechanisms, func(i, j int) bool { return state.mechanisms[i].MechanismID < state.mechanisms[j].MechanismID })
	sort.Slice(state.mechanismNodes, func(i, j int) bool {
		if state.mechanismNodes[i].MechanismID != state.mechanismNodes[j].MechanismID {
			return state.mechanismNodes[i].MechanismID < state.mechanismNodes[j].MechanismID
		}
		return state.mechanismNodes[i].SortOrder < state.mechanismNodes[j].SortOrder
	})
	sort.Slice(state.mechanismEdges, func(i, j int) bool {
		if state.mechanismEdges[i].MechanismID != state.mechanismEdges[j].MechanismID {
			return state.mechanismEdges[i].MechanismID < state.mechanismEdges[j].MechanismID
		}
		return state.mechanismEdges[i].PathOrder < state.mechanismEdges[j].PathOrder
	})
	sort.Slice(state.pathOutcomes, func(i, j int) bool { return state.pathOutcomes[i].PathOutcomeID < state.pathOutcomes[j].PathOutcomeID })

	return relationFirstProjection{
		canonicalEntities: canonicalEntities,
		relations:         relations,
		mechanisms:        state.mechanisms,
		mechanismNodes:    state.mechanismNodes,
		mechanismEdges:    state.mechanismEdges,
		pathOutcomes:      state.pathOutcomes,
		driverAggregates:  buildDriverAggregatesFromState(state.driverStates, now),
		targetAggregates:  buildTargetAggregatesFromState(state.targetStates, now),
	}, nil
}
