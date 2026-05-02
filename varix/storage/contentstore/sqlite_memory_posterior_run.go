package contentstore

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/kumaloha/VariX/varix/memory"
	"github.com/kumaloha/VariX/varix/model"
)

func (s *SQLiteStore) RunPosteriorVerification(ctx context.Context, req memory.PosteriorRunRequest, now time.Time) (memory.PosteriorRunResult, error) {
	if strings.TrimSpace(req.UserID) == "" {
		return memory.PosteriorRunResult{}, fmt.Errorf("posterior run user id is required")
	}
	if strings.TrimSpace(req.SourceExternalID) != "" && strings.TrimSpace(req.SourcePlatform) == "" {
		return memory.PosteriorRunResult{}, fmt.Errorf("source platform is required when source external id is provided")
	}
	now = normalizeNow(now)

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return memory.PosteriorRunResult{}, err
	}
	defer tx.Rollback()

	nodes, err := listPosteriorEligibleNodesTx(ctx, tx, req)
	if err != nil {
		return memory.PosteriorRunResult{}, err
	}
	if len(nodes) == 0 {
		return memory.PosteriorRunResult{RanAt: now}, tx.Commit()
	}

	allUserNodes, err := listAllUserMemoryTx(ctx, tx, strings.TrimSpace(req.UserID))
	if err != nil {
		return memory.PosteriorRunResult{}, err
	}

	statesByScope := make(map[sourceScopeKey][]posteriorNodeState)
	for _, node := range nodes {
		created, err := seedPosteriorStateTx(ctx, tx, node, now)
		if err != nil {
			return memory.PosteriorRunResult{}, err
		}
		current, err := getPosteriorStateTx(ctx, tx, node.MemoryID)
		if err != nil {
			return memory.PosteriorRunResult{}, err
		}
		if created && current.UpdatedAt.IsZero() {
			current.UpdatedAt = now
		}
		scope := sourceScopeKey{
			sourcePlatform:   node.SourcePlatform,
			sourceExternalID: node.SourceExternalID,
		}
		if containsPosteriorNodeState(statesByScope[scope], node.NodeID) {
			continue
		}
		statesByScope[scope] = append(statesByScope[scope], posteriorNodeState{
			node:    node,
			current: current,
		})
	}

	result := memory.PosteriorRunResult{
		RanAt:     now,
		Evaluated: make([]memory.PosteriorStateRecord, 0, len(nodes)),
		Mutated:   make([]memory.PosteriorStateRecord, 0, len(nodes)),
		Refreshes: make([]memory.PosteriorRefreshTrigger, 0),
	}

	for scope, scopedStates := range statesByScope {
		record, err := getCompiledOutputTx(ctx, tx, scope.sourcePlatform, scope.sourceExternalID)
		if err != nil {
			return memory.PosteriorRunResult{}, err
		}
		verifyRecord, err := getVerificationResultTx(ctx, tx, scope.sourcePlatform, scope.sourceExternalID)
		if err != nil && err != sql.ErrNoRows {
			return memory.PosteriorRunResult{}, err
		}
		verification := effectiveVerification(record, verifyRecord)
		graphNodesByID := make(map[string]model.GraphNode, len(record.Output.Graph.Nodes))
		for _, node := range record.Output.Graph.Nodes {
			graphNodesByID[node.ID] = node
		}
		predecessors := graphPredecessorIndex(record.Output.Graph.Edges)

		mutatedForScope := make([]memory.PosteriorStateRecord, 0, len(scopedStates))
		for _, state := range scopedStates {
			next := evaluatePosteriorState(state.node, state.current, graphNodesByID, predecessors, verification, record, allUserNodes, now)
			materiallyChanged := posteriorMateriallyChanged(state.current, next)
			evaluationChanged := !state.current.LastEvaluatedAt.Equal(next.LastEvaluatedAt)
			if materiallyChanged || evaluationChanged {
				if err := upsertPosteriorStateTx(ctx, tx, next); err != nil {
					return memory.PosteriorRunResult{}, err
				}
			}
			result.Evaluated = append(result.Evaluated, next)
			if materiallyChanged {
				result.Mutated = append(result.Mutated, next)
				mutatedForScope = append(mutatedForScope, next)
			}
		}
		if len(mutatedForScope) == 0 {
			continue
		}
		refreshes, err := enqueuePosteriorRefreshesTx(ctx, tx, scope, record, mutatedForScope, now)
		if err != nil {
			return memory.PosteriorRunResult{}, err
		}
		result.Refreshes = append(result.Refreshes, refreshes...)
	}

	if err := tx.Commit(); err != nil {
		return memory.PosteriorRunResult{}, err
	}
	return result, nil
}
