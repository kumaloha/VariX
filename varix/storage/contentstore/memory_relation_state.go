package contentstore

import (
	"context"
	"github.com/kumaloha/VariX/varix/memory"
	"github.com/kumaloha/VariX/varix/model"
	"strings"
)

type relationProjectionState struct {
	entityStates       map[string]*memory.CanonicalEntity
	relationIndex      map[string]memory.Relation
	mechanisms         []memory.Mechanism
	mechanismNodes     []memory.MechanismNode
	mechanismEdges     []memory.MechanismEdge
	pathOutcomes       []memory.PathOutcome
	driverStates       map[string]*aggregateState
	targetStates       map[string]*aggregateState
	conclusionByThesis map[string][]memory.CognitiveConclusion
}

func newRelationProjectionState(causalTheses []memory.CausalThesis, conclusions []memory.CognitiveConclusion) relationProjectionState {
	state := relationProjectionState{
		entityStates:       map[string]*memory.CanonicalEntity{},
		relationIndex:      map[string]memory.Relation{},
		mechanisms:         make([]memory.Mechanism, 0, len(causalTheses)),
		mechanismNodes:     make([]memory.MechanismNode, 0),
		mechanismEdges:     make([]memory.MechanismEdge, 0),
		pathOutcomes:       make([]memory.PathOutcome, 0, len(causalTheses)),
		driverStates:       map[string]*aggregateState{},
		targetStates:       map[string]*aggregateState{},
		conclusionByThesis: make(map[string][]memory.CognitiveConclusion),
	}
	for _, conclusion := range conclusions {
		if strings.TrimSpace(conclusion.CausalThesisID) == "" {
			continue
		}
		state.conclusionByThesis[conclusion.CausalThesisID] = append(state.conclusionByThesis[conclusion.CausalThesisID], conclusion)
	}
	return state
}

func (s *SQLiteStore) compiledNodesForTheses(ctx context.Context, theses []memory.CausalThesis) (map[string]model.GraphNode, error) {
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

	out := make(map[string]model.GraphNode)
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
