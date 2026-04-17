package contentstore

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/kumaloha/VariX/varix/compile"
	"github.com/kumaloha/VariX/varix/memory"
)

type sourceScopeKey struct {
	sourcePlatform   string
	sourceExternalID string
}

type acceptedScopeKey struct {
	userID           string
	sourcePlatform   string
	sourceExternalID string
}

type posteriorNodeState struct {
	node    memory.AcceptedNode
	current memory.PosteriorStateRecord
}

type rowScanner interface {
	Scan(dest ...any) error
}

type rowsQuerier interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}

func (s *SQLiteStore) GetPosteriorState(ctx context.Context, memoryID int64) (memory.PosteriorStateRecord, error) {
	return getPosteriorState(ctx, s.db, memoryID)
}

func (s *SQLiteStore) ListPosteriorStates(ctx context.Context, userID, sourcePlatform, sourceExternalID string) ([]memory.PosteriorStateRecord, error) {
	query := `SELECT umn.memory_id, ps.source_platform, ps.source_external_id, ps.node_id, ps.node_kind, ps.state, ps.diagnosis_code, ps.reason, ps.blocked_by_node_ids_json, ps.last_evaluated_at, ps.last_evidence_at, ps.updated_at
		FROM user_memory_nodes umn
		JOIN memory_posterior_states ps
		  ON ps.source_platform = umn.source_platform
		 AND ps.source_external_id = umn.source_external_id
		 AND ps.node_id = umn.node_id
		WHERE umn.user_id = ?`
	args := []any{strings.TrimSpace(userID)}
	if strings.TrimSpace(sourcePlatform) != "" {
		query += ` AND umn.source_platform = ?`
		args = append(args, strings.TrimSpace(sourcePlatform))
	}
	if strings.TrimSpace(sourceExternalID) != "" {
		query += ` AND umn.source_external_id = ?`
		args = append(args, strings.TrimSpace(sourceExternalID))
	}
	query += ` ORDER BY umn.accepted_at ASC, umn.memory_id ASC`
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanProjectedPosteriorStates(rows)
}

func (s *SQLiteStore) RunPosteriorVerification(ctx context.Context, req memory.PosteriorRunRequest, now time.Time) (memory.PosteriorRunResult, error) {
	if strings.TrimSpace(req.UserID) == "" {
		return memory.PosteriorRunResult{}, fmt.Errorf("posterior run user id is required")
	}
	if strings.TrimSpace(req.SourceExternalID) != "" && strings.TrimSpace(req.SourcePlatform) == "" {
		return memory.PosteriorRunResult{}, fmt.Errorf("source platform is required when source external id is provided")
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	now = now.UTC()

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
		graphNodesByID := make(map[string]compile.GraphNode, len(record.Output.Graph.Nodes))
		for _, node := range record.Output.Graph.Nodes {
			graphNodesByID[node.ID] = node
		}
		predecessors := graphPredecessorIndex(record.Output.Graph.Edges)

		mutatedForScope := make([]memory.PosteriorStateRecord, 0, len(scopedStates))
		for _, state := range scopedStates {
			next := evaluatePosteriorState(state.node, state.current, graphNodesByID, predecessors, record, allUserNodes, now)
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

func listPosteriorEligibleNodesTx(ctx context.Context, tx *sql.Tx, req memory.PosteriorRunRequest) ([]memory.AcceptedNode, error) {
	query := `SELECT memory_id, user_id, source_platform, source_external_id, root_external_id, node_id, node_kind, node_text, source_model, source_compiled_at, valid_from, valid_to, accepted_at
		FROM user_memory_nodes
		WHERE user_id = ?`
	args := []any{strings.TrimSpace(req.UserID)}
	if strings.TrimSpace(req.SourcePlatform) != "" {
		query += ` AND source_platform = ?`
		args = append(args, strings.TrimSpace(req.SourcePlatform))
	}
	if strings.TrimSpace(req.SourceExternalID) != "" {
		query += ` AND source_external_id = ?`
		args = append(args, strings.TrimSpace(req.SourceExternalID))
	}
	query += ` ORDER BY accepted_at ASC, memory_id ASC`
	rows, err := tx.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	nodes, err := scanMemoryNodes(rows)
	if err != nil {
		return nil, err
	}
	out := make([]memory.AcceptedNode, 0, len(nodes))
	for _, node := range nodes {
		if isPosteriorEligibleNodeKind(node.NodeKind) {
			out = append(out, node)
		}
	}
	return out, nil
}

func isPosteriorEligibleNodeKind(kind string) bool {
	switch strings.TrimSpace(kind) {
	case string(compile.NodeConclusion), string(compile.NodePrediction):
		return true
	default:
		return false
	}
}

func seedPosteriorStateTx(ctx context.Context, tx *sql.Tx, node memory.AcceptedNode, now time.Time) (bool, error) {
	if !isPosteriorEligibleNodeKind(node.NodeKind) {
		return false, nil
	}
	res, err := tx.ExecContext(
		ctx,
		`INSERT INTO memory_posterior_states(source_platform, source_external_id, node_id, node_kind, state, diagnosis_code, reason, blocked_by_node_ids_json, last_evaluated_at, last_evidence_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, NULL, NULL, '[]', NULL, NULL, ?)
		 ON CONFLICT(source_platform, source_external_id, node_id) DO NOTHING`,
		node.SourcePlatform,
		node.SourceExternalID,
		node.NodeID,
		node.NodeKind,
		string(memory.PosteriorStatePending),
		now.UTC().Format(time.RFC3339Nano),
	)
	if err != nil {
		return false, err
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return false, err
	}
	return rows > 0, nil
}

func getPosteriorStateTx(ctx context.Context, tx *sql.Tx, memoryID int64) (memory.PosteriorStateRecord, error) {
	return getPosteriorState(ctx, tx, memoryID)
}

func getPosteriorState(ctx context.Context, q interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}, memoryID int64) (memory.PosteriorStateRecord, error) {
	row := q.QueryRowContext(ctx, `SELECT u.memory_id, p.source_platform, p.source_external_id, p.node_id, p.node_kind, p.state, p.diagnosis_code, p.reason, p.blocked_by_node_ids_json, p.last_evaluated_at, p.last_evidence_at, p.updated_at
		FROM user_memory_nodes u
		INNER JOIN memory_posterior_states p
		  ON p.source_platform = u.source_platform
		 AND p.source_external_id = u.source_external_id
		 AND p.node_id = u.node_id
		WHERE u.memory_id = ?`, memoryID)
	return scanProjectedPosteriorState(row)
}

func scanProjectedPosteriorState(scanner rowScanner) (memory.PosteriorStateRecord, error) {
	var state memory.PosteriorStateRecord
	var diagnosis sql.NullString
	var reason sql.NullString
	var blockedJSON string
	var lastEvaluated sql.NullString
	var lastEvidence sql.NullString
	var updatedAt string
	if err := scanner.Scan(&state.MemoryID, &state.SourcePlatform, &state.SourceExternalID, &state.NodeID, &state.NodeKind, &state.State, &diagnosis, &reason, &blockedJSON, &lastEvaluated, &lastEvidence, &updatedAt); err != nil {
		return memory.PosteriorStateRecord{}, err
	}
	return hydratePosteriorStateRecord(state, diagnosis, reason, blockedJSON, lastEvaluated, lastEvidence, updatedAt)
}

func scanProjectedPosteriorStates(rows *sql.Rows) ([]memory.PosteriorStateRecord, error) {
	out := make([]memory.PosteriorStateRecord, 0)
	for rows.Next() {
		state, err := scanProjectedPosteriorState(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, state)
	}
	return out, rows.Err()
}

func projectPosteriorStatesOntoNodes(ctx context.Context, q rowsQuerier, nodes []memory.AcceptedNode) error {
	if len(nodes) == 0 {
		return nil
	}
	indexByMemoryID := make(map[int64][]int, len(nodes))
	memoryIDs := make([]int64, 0, len(nodes))
	for i, node := range nodes {
		if node.MemoryID == 0 {
			continue
		}
		if _, ok := indexByMemoryID[node.MemoryID]; !ok {
			memoryIDs = append(memoryIDs, node.MemoryID)
		}
		indexByMemoryID[node.MemoryID] = append(indexByMemoryID[node.MemoryID], i)
	}
	if len(memoryIDs) == 0 {
		return nil
	}
	posteriorByMemoryID, err := loadPosteriorStatesByMemoryID(ctx, q, memoryIDs)
	if err != nil {
		return err
	}
	for memoryID, posterior := range posteriorByMemoryID {
		indexes := indexByMemoryID[memoryID]
		for _, idx := range indexes {
			applyPosteriorStateRow(&nodes[idx], posterior)
		}
	}
	return nil
}

func applyPosteriorStateRecord(node *memory.AcceptedNode, posterior memory.PosteriorStateRecord) {
	if node == nil {
		return
	}
	applyPosteriorStateRow(node, posteriorStateRow{
		State:            posterior.State,
		Diagnosis:        posterior.DiagnosisCode,
		Reason:           posterior.Reason,
		BlockedByNodeIDs: posterior.BlockedByNodeIDs,
		UpdatedAt:        timePointer(posterior.UpdatedAt),
	})
}

func applyPosteriorStateRow(node *memory.AcceptedNode, posterior posteriorStateRow) {
	if node == nil {
		return
	}
	node.PosteriorState = posterior.State
	node.PosteriorDiagnosis = posterior.Diagnosis
	node.PosteriorReason = posterior.Reason
	node.BlockedByNodeIDs = append([]string(nil), posterior.BlockedByNodeIDs...)
	node.PosteriorUpdatedAt = posterior.UpdatedAt
}

func timePointer(value time.Time) *time.Time {
	if value.IsZero() {
		return nil
	}
	utc := value.UTC()
	return &utc
}

func loadPosteriorStatesByMemoryID(ctx context.Context, q rowsQuerier, memoryIDs []int64) (map[int64]posteriorStateRow, error) {
	placeholders := make([]string, 0, len(memoryIDs))
	args := make([]any, 0, len(memoryIDs))
	seen := make(map[int64]struct{}, len(memoryIDs))
	for _, memoryID := range memoryIDs {
		if memoryID == 0 {
			continue
		}
		if _, ok := seen[memoryID]; ok {
			continue
		}
		seen[memoryID] = struct{}{}
		placeholders = append(placeholders, "?")
		args = append(args, memoryID)
	}
	if len(placeholders) == 0 {
		return map[int64]posteriorStateRow{}, nil
	}
	rows, err := q.QueryContext(
		ctx,
		fmt.Sprintf(`SELECT u.memory_id, p.state, p.diagnosis_code, p.reason, p.blocked_by_node_ids_json, p.updated_at
			FROM user_memory_nodes u
			INNER JOIN memory_posterior_states p
			  ON p.source_platform = u.source_platform
			 AND p.source_external_id = u.source_external_id
			 AND p.node_id = u.node_id
			WHERE u.memory_id IN (%s)`, strings.Join(placeholders, ",")),
		args...,
	)
	if err != nil {
		if isMissingPosteriorStateTableErr(err) {
			return map[int64]posteriorStateRow{}, nil
		}
		return nil, err
	}
	defer rows.Close()

	out := make(map[int64]posteriorStateRow)
	for rows.Next() {
		var memoryID int64
		var state sql.NullString
		var diagnosis sql.NullString
		var reason sql.NullString
		var blockedByNodeIDsJSON sql.NullString
		var updatedAt sql.NullString
		if err := rows.Scan(&memoryID, &state, &diagnosis, &reason, &blockedByNodeIDsJSON, &updatedAt); err != nil {
			if isMissingPosteriorStateTableErr(err) {
				return map[int64]posteriorStateRow{}, nil
			}
			return nil, err
		}
		row := posteriorStateRow{
			State:     memory.PosteriorState(strings.TrimSpace(state.String)),
			Diagnosis: memory.PosteriorDiagnosisCode(strings.TrimSpace(diagnosis.String)),
			Reason:    strings.TrimSpace(reason.String),
		}
		if strings.TrimSpace(blockedByNodeIDsJSON.String) != "" {
			if err := json.Unmarshal([]byte(blockedByNodeIDsJSON.String), &row.BlockedByNodeIDs); err != nil {
				return nil, fmt.Errorf("decode posterior blocked_by_node_ids_json for memory_id %d: %w", memoryID, err)
			}
			sort.Strings(row.BlockedByNodeIDs)
		}
		if updatedAt.Valid && strings.TrimSpace(updatedAt.String) != "" {
			parsed := parseSQLiteTime(updatedAt.String)
			row.UpdatedAt = &parsed
		}
		out[memoryID] = row
	}
	if err := rows.Err(); err != nil {
		if isMissingPosteriorStateTableErr(err) {
			return map[int64]posteriorStateRow{}, nil
		}
		return nil, err
	}
	return out, nil
}

func upsertPosteriorStateTx(ctx context.Context, tx *sql.Tx, state memory.PosteriorStateRecord) error {
	blockedJSON, err := marshalJSONStringSlice(state.BlockedByNodeIDs)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(
		ctx,
		`INSERT INTO memory_posterior_states(source_platform, source_external_id, node_id, node_kind, state, diagnosis_code, reason, blocked_by_node_ids_json, last_evaluated_at, last_evidence_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(source_platform, source_external_id, node_id) DO UPDATE SET
		   node_kind = excluded.node_kind,
		   state = excluded.state,
		   diagnosis_code = excluded.diagnosis_code,
		   reason = excluded.reason,
		   blocked_by_node_ids_json = excluded.blocked_by_node_ids_json,
		   last_evaluated_at = excluded.last_evaluated_at,
		   last_evidence_at = excluded.last_evidence_at,
		   updated_at = excluded.updated_at`,
		state.SourcePlatform,
		state.SourceExternalID,
		state.NodeID,
		state.NodeKind,
		string(state.State),
		nullIfBlank(string(state.DiagnosisCode)),
		nullIfBlank(state.Reason),
		blockedJSON,
		formatSQLiteTime(state.LastEvaluatedAt),
		formatSQLiteTime(state.LastEvidenceAt),
		state.UpdatedAt.UTC().Format(time.RFC3339Nano),
	)
	return err
}

func hydratePosteriorStateRecord(
	state memory.PosteriorStateRecord,
	diagnosis sql.NullString,
	reason sql.NullString,
	blockedJSON string,
	lastEvaluated sql.NullString,
	lastEvidence sql.NullString,
	updatedAt string,
) (memory.PosteriorStateRecord, error) {
	if diagnosis.Valid {
		state.DiagnosisCode = memory.PosteriorDiagnosisCode(diagnosis.String)
	}
	if reason.Valid {
		state.Reason = reason.String
	}
	state.BlockedByNodeIDs = unmarshalJSONStringSlice(blockedJSON)
	if lastEvaluated.Valid {
		state.LastEvaluatedAt = parseSQLiteTime(lastEvaluated.String)
	}
	if lastEvidence.Valid {
		state.LastEvidenceAt = parseSQLiteTime(lastEvidence.String)
	}
	state.UpdatedAt = parseSQLiteTime(updatedAt)
	return state, nil
}

func evaluatePosteriorState(
	node memory.AcceptedNode,
	current memory.PosteriorStateRecord,
	graphNodesByID map[string]compile.GraphNode,
	predecessors map[string][]string,
	record compile.Record,
	allUserNodes []memory.AcceptedNode,
	now time.Time,
) memory.PosteriorStateRecord {
	next := current
	next.MemoryID = node.MemoryID
	next.NodeID = node.NodeID
	next.NodeKind = node.NodeKind
	next.LastEvaluatedAt = now
	if next.State == "" {
		next.State = memory.PosteriorStatePending
	}

	blockedBy, blockedReason := blockedByConditions(node.NodeID, graphNodesByID, predecessors, record)
	if len(blockedBy) > 0 {
		next.State = memory.PosteriorStateBlocked
		next.DiagnosisCode = ""
		next.Reason = blockedReason
		next.BlockedByNodeIDs = blockedBy
		next.LastEvidenceAt = time.Time{}
		return finalizePosteriorTransition(current, next, now)
	}

	next.BlockedByNodeIDs = nil
	switch strings.TrimSpace(node.NodeKind) {
	case string(compile.NodePrediction):
		evaluatePredictionPosterior(node, &next, graphNodesByID[node.NodeID], record, now)
	case string(compile.NodeConclusion):
		evaluateConclusionPosterior(node, &next, graphNodesByID, predecessors, record, allUserNodes, now)
	default:
		next.State = memory.PosteriorStatePending
		next.DiagnosisCode = ""
		next.Reason = ""
		next.LastEvidenceAt = time.Time{}
	}
	return finalizePosteriorTransition(current, next, now)
}

func finalizePosteriorTransition(current, next memory.PosteriorStateRecord, now time.Time) memory.PosteriorStateRecord {
	if next.UpdatedAt.IsZero() {
		if posteriorMateriallyChanged(current, next) || current.UpdatedAt.IsZero() {
			next.UpdatedAt = now
		} else {
			next.UpdatedAt = current.UpdatedAt
		}
	}
	return next
}

func evaluatePredictionPosterior(node memory.AcceptedNode, state *memory.PosteriorStateRecord, graphNode compile.GraphNode, record compile.Record, now time.Time) {
	checks := predictionStatusMap(record)
	for _, check := range record.Output.Verification.PredictionChecks {
		if check.NodeID != node.NodeID {
			continue
		}
		switch check.Status {
		case compile.PredictionStatusResolvedTrue:
			state.State = memory.PosteriorStateVerified
			state.DiagnosisCode = ""
			state.Reason = strings.TrimSpace(check.Reason)
			state.LastEvidenceAt = maxTime(record.CompiledAt.UTC(), check.AsOf.UTC())
			return
		case compile.PredictionStatusResolvedFalse:
			state.State = memory.PosteriorStateFalsified
			state.DiagnosisCode = memory.PosteriorDiagnosisFactError
			state.Reason = firstNonBlank(check.Reason, "prediction resolved false")
			state.LastEvidenceAt = maxTime(record.CompiledAt.UTC(), check.AsOf.UTC())
			return
		}
	}

	due := graphNode.PredictionDueAt
	if due.IsZero() {
		due = node.ValidTo
	}
	if !due.IsZero() && now.Before(due) {
		state.State = memory.PosteriorStatePending
		state.DiagnosisCode = ""
		state.Reason = "prediction due time not reached"
		state.LastEvidenceAt = time.Time{}
		return
	}

	if status, ok := checks[node.NodeID]; ok && status == compile.PredictionStatusStaleUnresolved {
		state.State = memory.PosteriorStatePending
		state.DiagnosisCode = ""
		state.Reason = "prediction still unresolved after due time"
		state.LastEvidenceAt = time.Time{}
		return
	}

	state.State = memory.PosteriorStatePending
	state.DiagnosisCode = ""
	state.Reason = "awaiting fresh posterior evidence"
	state.LastEvidenceAt = time.Time{}
}

func evaluateConclusionPosterior(
	node memory.AcceptedNode,
	state *memory.PosteriorStateRecord,
	graphNodesByID map[string]compile.GraphNode,
	predecessors map[string][]string,
	record compile.Record,
	allUserNodes []memory.AcceptedNode,
	now time.Time,
) {
	factStatuses := factStatusMap(record)
	for _, ancestorID := range collectAncestorNodeIDs(node.NodeID, predecessors) {
		ancestor, ok := graphNodesByID[ancestorID]
		if !ok {
			continue
		}
		switch ancestor.Kind {
		case compile.NodeFact, compile.NodeImplicitCondition:
			if factStatuses[ancestorID] == compile.FactStatusClearlyFalse {
				state.State = memory.PosteriorStateFalsified
				state.DiagnosisCode = memory.PosteriorDiagnosisFactError
				state.Reason = firstNonBlank(ancestor.Text, "supporting evidence was later contradicted")
				state.LastEvidenceAt = record.CompiledAt.UTC()
				return
			}
		}
	}

	if conflictingNode, reason, ok := fresherContradictingNode(node, allUserNodes, now); ok {
		state.State = memory.PosteriorStateFalsified
		state.DiagnosisCode = memory.PosteriorDiagnosisLogicError
		state.Reason = firstNonBlank(reason, conflictingNode.NodeText, "fresher contradiction detected")
		state.LastEvidenceAt = conflictingNode.SourceCompiledAt.UTC()
		return
	}

	state.State = memory.PosteriorStatePending
	state.DiagnosisCode = ""
	state.Reason = "insufficient deterministic posterior evidence"
	state.LastEvidenceAt = time.Time{}
}

func blockedByConditions(nodeID string, graphNodesByID map[string]compile.GraphNode, predecessors map[string][]string, record compile.Record) ([]string, string) {
	explicitStatuses := explicitConditionStatusMap(record)
	blockedBy := make([]string, 0)
	reasons := make([]string, 0)
	for _, ancestorID := range collectAncestorNodeIDs(nodeID, predecessors) {
		ancestor, ok := graphNodesByID[ancestorID]
		if !ok || ancestor.Kind != compile.NodeExplicitCondition {
			continue
		}
		status, ok := explicitStatuses[ancestorID]
		if !ok {
			blockedBy = append(blockedBy, ancestorID)
			reasons = append(reasons, "condition unresolved")
			continue
		}
		switch status {
		case compile.ExplicitConditionStatusHigh, compile.ExplicitConditionStatusMedium:
			continue
		default:
			blockedBy = append(blockedBy, ancestorID)
			reasons = append(reasons, strings.TrimSpace(ancestor.Text))
		}
	}
	if len(blockedBy) == 0 {
		return nil, ""
	}
	sort.Strings(blockedBy)
	return blockedBy, firstNonBlank(strings.Join(uniquePreservingOrder(reasons), "; "), "required condition unresolved")
}

func fresherContradictingNode(node memory.AcceptedNode, allUserNodes []memory.AcceptedNode, now time.Time) (memory.AcceptedNode, string, bool) {
	var best memory.AcceptedNode
	var bestReason string
	found := false
	for _, candidate := range allUserNodes {
		if candidate.MemoryID == node.MemoryID {
			continue
		}
		if !isPosteriorEligibleNodeKind(candidate.NodeKind) {
			continue
		}
		if !candidate.SourceCompiledAt.After(node.SourceCompiledAt) {
			continue
		}
		if !isAcceptedNodeActiveAt(candidate, now) {
			continue
		}
		reason, ok := contradictionReason(node.NodeText, candidate.NodeText)
		if !ok {
			continue
		}
		if !found || candidate.SourceCompiledAt.After(best.SourceCompiledAt) {
			best = candidate
			bestReason = reason
			found = true
		}
	}
	return best, bestReason, found
}

func collectAncestorNodeIDs(nodeID string, predecessors map[string][]string) []string {
	seen := map[string]struct{}{}
	queue := append([]string(nil), predecessors[nodeID]...)
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		if _, ok := seen[current]; ok {
			continue
		}
		seen[current] = struct{}{}
		queue = append(queue, predecessors[current]...)
	}
	out := make([]string, 0, len(seen))
	for id := range seen {
		out = append(out, id)
	}
	sort.Strings(out)
	return out
}

func graphPredecessorIndex(edges []compile.GraphEdge) map[string][]string {
	out := make(map[string][]string)
	for _, edge := range edges {
		out[edge.To] = append(out[edge.To], edge.From)
	}
	for nodeID := range out {
		sort.Strings(out[nodeID])
	}
	return out
}

func posteriorMateriallyChanged(current, next memory.PosteriorStateRecord) bool {
	if current.State != next.State {
		return true
	}
	if current.DiagnosisCode != next.DiagnosisCode {
		return true
	}
	if current.Reason != next.Reason {
		return true
	}
	if !sameStringSlice(current.BlockedByNodeIDs, next.BlockedByNodeIDs) {
		return true
	}
	if !current.LastEvidenceAt.Equal(next.LastEvidenceAt) {
		return true
	}
	if current.MemoryID == 0 || current.UpdatedAt.IsZero() {
		return true
	}
	return false
}

func sameStringSlice(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}

func enqueuePosteriorRefreshTx(
	ctx context.Context,
	tx *sql.Tx,
	node memory.AcceptedNode,
	record compile.Record,
	mutated []memory.PosteriorStateRecord,
	now time.Time,
) (memory.PosteriorRefreshTrigger, error) {
	if len(mutated) == 0 {
		return memory.PosteriorRefreshTrigger{}, errors.New("posterior refresh requires mutated states")
	}
	memoryIDs := make([]int64, 0, len(mutated))
	nodeIDs := make([]string, 0, len(mutated))
	for _, state := range mutated {
		memoryIDs = append(memoryIDs, state.MemoryID)
		nodeIDs = append(nodeIDs, state.NodeID)
	}
	sort.Slice(memoryIDs, func(i, j int) bool { return memoryIDs[i] < memoryIDs[j] })
	sort.Strings(nodeIDs)

	payload, err := json.Marshal(map[string]any{
		"reason":               "posterior_state_changed",
		"posterior_memory_ids": memoryIDs,
		"posterior_node_ids":   nodeIDs,
	})
	if err != nil {
		return memory.PosteriorRefreshTrigger{}, err
	}

	eventResult, err := tx.ExecContext(
		ctx,
		`INSERT INTO memory_acceptance_events(user_id, trigger_type, source_platform, source_external_id, root_external_id, source_model, source_compiled_at, payload_json, accepted_count, accepted_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		node.UserID,
		"posterior_refresh",
		node.SourcePlatform,
		node.SourceExternalID,
		record.RootExternalID,
		record.Model,
		record.CompiledAt.UTC().Format(time.RFC3339Nano),
		string(payload),
		0,
		now.Format(time.RFC3339Nano),
	)
	if err != nil {
		return memory.PosteriorRefreshTrigger{}, err
	}
	eventID, err := eventResult.LastInsertId()
	if err != nil {
		return memory.PosteriorRefreshTrigger{}, err
	}

	jobResult, err := tx.ExecContext(
		ctx,
		`INSERT INTO memory_organization_jobs(trigger_event_id, user_id, source_platform, source_external_id, status, created_at, started_at, finished_at)
		 VALUES (?, ?, ?, ?, ?, ?, NULL, NULL)`,
		eventID,
		node.UserID,
		node.SourcePlatform,
		node.SourceExternalID,
		"queued",
		now.Format(time.RFC3339Nano),
	)
	if err != nil {
		return memory.PosteriorRefreshTrigger{}, err
	}
	jobID, err := jobResult.LastInsertId()
	if err != nil {
		return memory.PosteriorRefreshTrigger{}, err
	}

	return memory.PosteriorRefreshTrigger{
		EventID:           eventID,
		JobID:             jobID,
		UserID:            node.UserID,
		SourcePlatform:    node.SourcePlatform,
		SourceExternalID:  node.SourceExternalID,
		RootExternalID:    record.RootExternalID,
		SourceModel:       record.Model,
		SourceCompiledAt:  record.CompiledAt.UTC(),
		AffectedMemoryIDs: memoryIDs,
		AffectedNodeIDs:   nodeIDs,
		Reason:            "posterior_state_changed",
		CreatedAt:         now,
	}, nil
}

func firstNonBlank(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}
