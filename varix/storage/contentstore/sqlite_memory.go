package contentstore

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/kumaloha/VariX/varix/compile"
	"github.com/kumaloha/VariX/varix/memory"
)

func (s *SQLiteStore) AcceptMemoryNodes(ctx context.Context, req memory.AcceptRequest) (memory.AcceptResult, error) {
	req.UserID = strings.TrimSpace(req.UserID)
	req.SourcePlatform = strings.TrimSpace(req.SourcePlatform)
	req.SourceExternalID = strings.TrimSpace(req.SourceExternalID)
	if req.UserID == "" || req.SourcePlatform == "" || req.SourceExternalID == "" || len(req.NodeIDs) == 0 {
		return memory.AcceptResult{}, fmt.Errorf("invalid memory accept request")
	}

	record, err := s.GetCompiledOutput(ctx, req.SourcePlatform, req.SourceExternalID)
	if err != nil {
		return memory.AcceptResult{}, fmt.Errorf("compiled output not found: %w", err)
	}

	uniqueNodeIDs := make([]string, 0, len(req.NodeIDs))
	seen := map[string]struct{}{}
	for _, nodeID := range req.NodeIDs {
		nodeID = strings.TrimSpace(nodeID)
		if nodeID == "" {
			continue
		}
		if _, ok := seen[nodeID]; ok {
			continue
		}
		seen[nodeID] = struct{}{}
		uniqueNodeIDs = append(uniqueNodeIDs, nodeID)
	}
	if len(uniqueNodeIDs) == 0 {
		return memory.AcceptResult{}, fmt.Errorf("no node ids provided")
	}

	graphNodes := map[string]compile.GraphNode{}
	for _, node := range record.Output.Graph.Nodes {
		graphNodes[node.ID] = node
	}

	snapshots := make([]memory.AcceptanceNodeSnapshot, 0, len(uniqueNodeIDs))
	for _, nodeID := range uniqueNodeIDs {
		node, ok := graphNodes[nodeID]
		if !ok {
			return memory.AcceptResult{}, fmt.Errorf("node id not found in compiled graph: %s", nodeID)
		}
		snapshots = append(snapshots, memory.AcceptanceNodeSnapshot{
			NodeID:   node.ID,
			NodeKind: string(node.Kind),
			NodeText: node.Text,
			ValidFrom: func() time.Time {
				start, _ := node.LegacyValidityWindow()
				return start
			}(),
			ValidTo: func() time.Time {
				_, end := node.LegacyValidityWindow()
				return end
			}(),
		})
	}

	now := normalizeNow(time.Time{})
	triggerType := "accept_single"
	if len(snapshots) > 1 {
		triggerType = "accept_batch"
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return memory.AcceptResult{}, err
	}
	defer tx.Rollback()

	nodes := make([]memory.AcceptedNode, 0, len(snapshots))
	for _, snap := range snapshots {
		_, err := tx.ExecContext(
			ctx,
			`INSERT INTO user_memory_nodes(user_id, source_platform, source_external_id, root_external_id, node_id, node_kind, node_text, source_model, source_compiled_at, valid_from, valid_to, accepted_at)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
			 ON CONFLICT(user_id, source_platform, source_external_id, node_id) DO NOTHING`,
			req.UserID,
			req.SourcePlatform,
			req.SourceExternalID,
			record.RootExternalID,
			snap.NodeID,
			snap.NodeKind,
			snap.NodeText,
			record.Model,
			record.CompiledAt.UTC().Format(time.RFC3339Nano),
			snap.ValidFrom.UTC().Format(time.RFC3339Nano),
			snap.ValidTo.UTC().Format(time.RFC3339Nano),
			now.Format(time.RFC3339Nano),
		)
		if err != nil {
			return memory.AcceptResult{}, err
		}
		node, err := queryMemoryNodeTx(ctx, tx, req.UserID, req.SourcePlatform, req.SourceExternalID, snap.NodeID)
		if err != nil {
			return memory.AcceptResult{}, err
		}
		if _, err := seedPosteriorStateTx(ctx, tx, node, now); err != nil {
			return memory.AcceptResult{}, err
		}
		if isPosteriorEligibleNodeKind(node.NodeKind) {
			posterior, err := getPosteriorStateTx(ctx, tx, node.MemoryID)
			if err != nil {
				return memory.AcceptResult{}, err
			}
			applyPosteriorStateRecord(&node, posterior)
		}
		nodes = append(nodes, node)
	}

	payload, err := json.Marshal(snapshots)
	if err != nil {
		return memory.AcceptResult{}, err
	}

	result, err := tx.ExecContext(
		ctx,
		`INSERT INTO memory_acceptance_events(user_id, trigger_type, source_platform, source_external_id, root_external_id, source_model, source_compiled_at, payload_json, accepted_count, accepted_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		req.UserID,
		triggerType,
		req.SourcePlatform,
		req.SourceExternalID,
		record.RootExternalID,
		record.Model,
		record.CompiledAt.UTC().Format(time.RFC3339Nano),
		string(payload),
		len(snapshots),
		now.Format(time.RFC3339Nano),
	)
	if err != nil {
		return memory.AcceptResult{}, err
	}
	eventID, err := result.LastInsertId()
	if err != nil {
		return memory.AcceptResult{}, err
	}
	subgraph, err := persistMemoryContentGraphTx(ctx, tx, req.UserID, record, now)
	if err != nil {
		return memory.AcceptResult{}, err
	}
	if err := markContentGraphProjectionDirty(ctx, tx, req.UserID, subgraph, now); err != nil {
		return memory.AcceptResult{}, err
	}

	event := memory.AcceptanceEvent{
		EventID:           eventID,
		UserID:            req.UserID,
		TriggerType:       triggerType,
		SourcePlatform:    req.SourcePlatform,
		SourceExternalID:  req.SourceExternalID,
		RootExternalID:    record.RootExternalID,
		SourceModel:       record.Model,
		SourceCompiledAt:  record.CompiledAt.UTC(),
		AcceptedCount:     len(snapshots),
		AcceptedAt:        now,
		AcceptedNodeState: snapshots,
	}

	jobResult, err := tx.ExecContext(
		ctx,
		`INSERT INTO memory_organization_jobs(trigger_event_id, user_id, source_platform, source_external_id, status, created_at, started_at, finished_at)
		 VALUES (?, ?, ?, ?, ?, ?, NULL, NULL)`,
		eventID,
		req.UserID,
		req.SourcePlatform,
		req.SourceExternalID,
		"queued",
		now.Format(time.RFC3339Nano),
	)
	if err != nil {
		return memory.AcceptResult{}, err
	}
	jobID, err := jobResult.LastInsertId()
	if err != nil {
		return memory.AcceptResult{}, err
	}
	job := memory.OrganizationJob{
		JobID:            jobID,
		TriggerEventID:   eventID,
		UserID:           req.UserID,
		SourcePlatform:   req.SourcePlatform,
		SourceExternalID: req.SourceExternalID,
		Status:           "queued",
		CreatedAt:        now,
	}

	if err := tx.Commit(); err != nil {
		return memory.AcceptResult{}, err
	}

	return memory.AcceptResult{
		Nodes: nodes,
		Event: event,
		Job:   job,
	}, nil
}

func (s *SQLiteStore) ListUserMemory(ctx context.Context, userID string) ([]memory.AcceptedNode, error) {
	return s.listUserMemoryNodes(ctx, `SELECT memory_id, user_id, source_platform, source_external_id, root_external_id, node_id, node_kind, node_text, source_model, source_compiled_at, valid_from, valid_to, accepted_at
		 FROM user_memory_nodes
		 WHERE user_id = ?
		 ORDER BY accepted_at ASC, memory_id ASC`, userID)
}

func (s *SQLiteStore) ListUserMemoryBySource(ctx context.Context, userID, sourcePlatform, sourceExternalID string) ([]memory.AcceptedNode, error) {
	return s.listUserMemoryNodes(ctx, `SELECT memory_id, user_id, source_platform, source_external_id, root_external_id, node_id, node_kind, node_text, source_model, source_compiled_at, valid_from, valid_to, accepted_at
		 FROM user_memory_nodes
		 WHERE user_id = ? AND source_platform = ? AND source_external_id = ?
		 ORDER BY accepted_at ASC, memory_id ASC`, userID, sourcePlatform, sourceExternalID)
}

func (s *SQLiteStore) listUserMemoryNodes(ctx context.Context, query string, args ...any) ([]memory.AcceptedNode, error) {
	rows, err := s.db.QueryContext(
		ctx,
		query,
		args...,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	nodes, err := scanMemoryNodes(rows)
	if err != nil {
		return nil, err
	}
	if err := projectPosteriorStatesOntoNodes(ctx, s.db, nodes); err != nil {
		return nil, err
	}
	return nodes, nil
}

func (s *SQLiteStore) ListMemoryJobs(ctx context.Context, userID string) ([]memory.OrganizationJob, error) {
	rows, err := s.db.QueryContext(
		ctx,
		`SELECT job_id, trigger_event_id, user_id, source_platform, source_external_id, status, created_at, started_at, finished_at
		 FROM memory_organization_jobs
		 WHERE user_id = ?
		 ORDER BY created_at ASC, job_id ASC`,
		userID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]memory.OrganizationJob, 0)
	for rows.Next() {
		var job memory.OrganizationJob
		var createdAt string
		var startedAt sql.NullString
		var finishedAt sql.NullString
		if err := rows.Scan(&job.JobID, &job.TriggerEventID, &job.UserID, &job.SourcePlatform, &job.SourceExternalID, &job.Status, &createdAt, &startedAt, &finishedAt); err != nil {
			return nil, err
		}
		job.CreatedAt = parseSQLiteTime(createdAt)
		if startedAt.Valid {
			job.StartedAt = parseSQLiteTime(startedAt.String)
		}
		if finishedAt.Valid {
			job.FinishedAt = parseSQLiteTime(finishedAt.String)
		}
		out = append(out, job)
	}
	return out, rows.Err()
}

func (s *SQLiteStore) CleanupStaleMemoryJobs(ctx context.Context, userID, sourcePlatform, sourceExternalID string, cutoff time.Time) (int64, error) {
	var err error
	userID, err = normalizeRequiredUserID(userID)
	if err != nil {
		return 0, err
	}
	if cutoff.IsZero() {
		return 0, fmt.Errorf("cutoff is required")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	jobIDs, err := listStaleMemoryJobIDs(ctx, tx, userID, sourcePlatform, sourceExternalID, cutoff)
	if err != nil {
		return 0, err
	}
	var deleted int64
	for _, jobID := range jobIDs {
		result, err := tx.ExecContext(ctx, `DELETE FROM memory_organization_jobs WHERE job_id = ?`, jobID)
		if err != nil {
			return deleted, err
		}
		affected, err := result.RowsAffected()
		if err != nil {
			return deleted, err
		}
		deleted += affected
	}
	if err := tx.Commit(); err != nil {
		return deleted, err
	}
	return deleted, nil
}

func (s *SQLiteStore) CountStaleMemoryJobs(ctx context.Context, userID, sourcePlatform, sourceExternalID string, cutoff time.Time) (int64, error) {
	var err error
	userID, err = normalizeRequiredUserID(userID)
	if err != nil {
		return 0, err
	}
	if cutoff.IsZero() {
		return 0, fmt.Errorf("cutoff is required")
	}
	jobIDs, err := listStaleMemoryJobIDs(ctx, s.db, userID, sourcePlatform, sourceExternalID, cutoff)
	if err != nil {
		return 0, err
	}
	return int64(len(jobIDs)), nil
}

func listStaleMemoryJobIDs(ctx context.Context, q interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}, userID, sourcePlatform, sourceExternalID string, cutoff time.Time) ([]int64, error) {
	userID = strings.TrimSpace(userID)
	sourcePlatform = strings.TrimSpace(sourcePlatform)
	sourceExternalID = strings.TrimSpace(sourceExternalID)
	query := `SELECT job_id, status, created_at, started_at
		FROM memory_organization_jobs
		WHERE user_id = ?
		  AND status IN ('queued', 'running')`
	args := []any{userID}
	if sourcePlatform != "" {
		query += ` AND source_platform = ?`
		args = append(args, sourcePlatform)
	}
	if sourceExternalID != "" {
		query += ` AND source_external_id = ?`
		args = append(args, sourceExternalID)
	}
	rows, err := q.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	jobIDs := make([]int64, 0)
	for rows.Next() {
		var jobID int64
		var status string
		var createdAt string
		var startedAt sql.NullString
		if err := rows.Scan(&jobID, &status, &createdAt, &startedAt); err != nil {
			return nil, err
		}
		staleAt := parseSQLiteTime(createdAt)
		if status == "running" && startedAt.Valid && strings.TrimSpace(startedAt.String) != "" {
			staleAt = parseSQLiteTime(startedAt.String)
		}
		if staleAt.Before(cutoff) {
			jobIDs = append(jobIDs, jobID)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return jobIDs, nil
}

func queryMemoryNodeTx(ctx context.Context, tx *sql.Tx, userID, sourcePlatform, sourceExternalID, nodeID string) (memory.AcceptedNode, error) {
	var node memory.AcceptedNode
	var sourceCompiledAt string
	var validFrom string
	var validTo string
	var acceptedAt string
	err := tx.QueryRowContext(
		ctx,
		`SELECT memory_id, user_id, source_platform, source_external_id, root_external_id, node_id, node_kind, node_text, source_model, source_compiled_at, valid_from, valid_to, accepted_at
		 FROM user_memory_nodes
		 WHERE user_id = ? AND source_platform = ? AND source_external_id = ? AND node_id = ?`,
		userID,
		sourcePlatform,
		sourceExternalID,
		nodeID,
	).Scan(&node.MemoryID, &node.UserID, &node.SourcePlatform, &node.SourceExternalID, &node.RootExternalID, &node.NodeID, &node.NodeKind, &node.NodeText, &node.SourceModel, &sourceCompiledAt, &validFrom, &validTo, &acceptedAt)
	if err != nil {
		return memory.AcceptedNode{}, err
	}
	node.SourceCompiledAt = parseSQLiteTime(sourceCompiledAt)
	node.ValidFrom = parseSQLiteTime(validFrom)
	node.ValidTo = parseSQLiteTime(validTo)
	node.AcceptedAt = parseSQLiteTime(acceptedAt)
	return node, nil
}

func scanMemoryNodes(rows *sql.Rows) ([]memory.AcceptedNode, error) {
	out := make([]memory.AcceptedNode, 0)
	for rows.Next() {
		var node memory.AcceptedNode
		var sourceCompiledAt string
		var validFrom string
		var validTo string
		var acceptedAt string
		if err := rows.Scan(&node.MemoryID, &node.UserID, &node.SourcePlatform, &node.SourceExternalID, &node.RootExternalID, &node.NodeID, &node.NodeKind, &node.NodeText, &node.SourceModel, &sourceCompiledAt, &validFrom, &validTo, &acceptedAt); err != nil {
			return nil, err
		}
		node.SourceCompiledAt = parseSQLiteTime(sourceCompiledAt)
		node.ValidFrom = parseSQLiteTime(validFrom)
		node.ValidTo = parseSQLiteTime(validTo)
		node.AcceptedAt = parseSQLiteTime(acceptedAt)
		out = append(out, node)
	}
	return out, rows.Err()
}
