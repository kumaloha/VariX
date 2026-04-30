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
	node.BlockedByNodeIDs = cloneStringSlice(posterior.BlockedByNodeIDs)
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

func enqueuePosteriorRefreshesTx(
	ctx context.Context,
	tx *sql.Tx,
	scope sourceScopeKey,
	record compile.Record,
	mutated []memory.PosteriorStateRecord,
	now time.Time,
) ([]memory.PosteriorRefreshTrigger, error) {
	if len(mutated) == 0 {
		return nil, errors.New("posterior refresh requires mutated states")
	}
	nodeIDs := make([]string, 0, len(mutated))
	seenNodeIDs := make(map[string]struct{}, len(mutated))
	for _, state := range mutated {
		if _, ok := seenNodeIDs[state.NodeID]; ok {
			continue
		}
		seenNodeIDs[state.NodeID] = struct{}{}
		nodeIDs = append(nodeIDs, state.NodeID)
	}
	sort.Strings(nodeIDs)

	placeholders := make([]string, len(nodeIDs))
	args := make([]any, 0, len(nodeIDs)+2)
	args = append(args, scope.sourcePlatform, scope.sourceExternalID)
	for i, nodeID := range nodeIDs {
		placeholders[i] = "?"
		args = append(args, nodeID)
	}

	rows, err := tx.QueryContext(
		ctx,
		fmt.Sprintf(`SELECT user_id, memory_id, node_id
			FROM user_memory_nodes
			WHERE source_platform = ? AND source_external_id = ? AND node_id IN (%s)
			ORDER BY user_id ASC, memory_id ASC`, strings.Join(placeholders, ",")),
		args...,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	memoryIDsByScope := make(map[acceptedScopeKey][]int64)
	nodeIDsByScope := make(map[acceptedScopeKey][]string)
	scopeOrder := make([]acceptedScopeKey, 0)
	for rows.Next() {
		var userID string
		var memoryID int64
		var nodeID string
		if err := rows.Scan(&userID, &memoryID, &nodeID); err != nil {
			return nil, err
		}
		targetScope := acceptedScopeKey{
			userID:           userID,
			sourcePlatform:   scope.sourcePlatform,
			sourceExternalID: scope.sourceExternalID,
		}
		if _, ok := memoryIDsByScope[targetScope]; !ok {
			scopeOrder = append(scopeOrder, targetScope)
		}
		memoryIDsByScope[targetScope] = append(memoryIDsByScope[targetScope], memoryID)
		nodeIDsByScope[targetScope] = append(nodeIDsByScope[targetScope], nodeID)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	refreshes := make([]memory.PosteriorRefreshTrigger, 0, len(scopeOrder))
	for _, targetScope := range scopeOrder {
		memoryIDs := append([]int64(nil), memoryIDsByScope[targetScope]...)
		nodeIDsForScope := uniquePreservingOrder(nodeIDsByScope[targetScope])
		sort.Slice(memoryIDs, func(i, j int) bool { return memoryIDs[i] < memoryIDs[j] })
		sort.Strings(nodeIDsForScope)

		payload, err := json.Marshal(map[string]any{
			"reason":               "posterior_state_changed",
			"posterior_memory_ids": memoryIDs,
			"posterior_node_ids":   nodeIDsForScope,
		})
		if err != nil {
			return nil, err
		}

		eventResult, err := tx.ExecContext(
			ctx,
			`INSERT INTO memory_acceptance_events(user_id, trigger_type, source_platform, source_external_id, root_external_id, source_model, source_compiled_at, payload_json, accepted_count, accepted_at)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			targetScope.userID,
			"posterior_refresh",
			targetScope.sourcePlatform,
			targetScope.sourceExternalID,
			record.RootExternalID,
			record.Model,
			record.CompiledAt.UTC().Format(time.RFC3339Nano),
			string(payload),
			0,
			now.Format(time.RFC3339Nano),
		)
		if err != nil {
			return nil, err
		}
		eventID, err := eventResult.LastInsertId()
		if err != nil {
			return nil, err
		}

		jobResult, err := tx.ExecContext(
			ctx,
			`INSERT INTO memory_organization_jobs(trigger_event_id, user_id, source_platform, source_external_id, status, created_at, started_at, finished_at)
			 VALUES (?, ?, ?, ?, ?, ?, NULL, NULL)`,
			eventID,
			targetScope.userID,
			targetScope.sourcePlatform,
			targetScope.sourceExternalID,
			"queued",
			now.Format(time.RFC3339Nano),
		)
		if err != nil {
			return nil, err
		}
		jobID, err := jobResult.LastInsertId()
		if err != nil {
			return nil, err
		}

		refreshes = append(refreshes, memory.PosteriorRefreshTrigger{
			EventID:           eventID,
			JobID:             jobID,
			UserID:            targetScope.userID,
			SourcePlatform:    targetScope.sourcePlatform,
			SourceExternalID:  targetScope.sourceExternalID,
			RootExternalID:    record.RootExternalID,
			SourceModel:       record.Model,
			SourceCompiledAt:  record.CompiledAt.UTC(),
			AffectedMemoryIDs: memoryIDs,
			AffectedNodeIDs:   nodeIDsForScope,
			Reason:            "posterior_state_changed",
			CreatedAt:         now,
		})
	}

	return refreshes, nil
}
