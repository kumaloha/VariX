package contentstore

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/kumaloha/VariX/varix/memory"
	"github.com/kumaloha/VariX/varix/model"
)

func (s *SQLiteStore) GetLatestMemoryOrganizationOutput(ctx context.Context, userID, sourcePlatform, sourceExternalID string) (memory.OrganizationOutput, error) {
	var payload string
	var latestOutputCreatedAt string
	var latestOutputJobID int64
	err := s.db.QueryRowContext(
		ctx,
		`SELECT payload_json, created_at, job_id FROM memory_organization_outputs
		 WHERE user_id = ? AND source_platform = ? AND source_external_id = ?
		 ORDER BY created_at DESC, output_id DESC
		 LIMIT 1`,
		userID, sourcePlatform, sourceExternalID,
	).Scan(&payload, &latestOutputCreatedAt, &latestOutputJobID)
	if err != nil {
		return memory.OrganizationOutput{}, err
	}
	stale, staleJobID, staleJobStatus, staleJobCreatedAt, err := s.hasNewerInFlightOrganizationJob(ctx, userID, sourcePlatform, sourceExternalID, parseSQLiteTime(latestOutputCreatedAt), latestOutputJobID)
	if err != nil {
		return memory.OrganizationOutput{}, err
	}
	if stale {
		return memory.OrganizationOutput{}, fmt.Errorf("%w: source %s/%s for user %s has newer %s job %d created at %s", ErrMemoryOrganizationOutputStale, sourcePlatform, sourceExternalID, userID, staleJobStatus, staleJobID, staleJobCreatedAt.Format(time.RFC3339Nano))
	}
	var out memory.OrganizationOutput
	if err := json.Unmarshal([]byte(payload), &out); err != nil {
		return memory.OrganizationOutput{}, err
	}
	return out, nil
}

func (s *SQLiteStore) hasNewerInFlightOrganizationJob(ctx context.Context, userID, sourcePlatform, sourceExternalID string, latestOutputCreatedAt time.Time, latestOutputJobID int64) (bool, int64, string, time.Time, error) {
	var jobID int64
	var status string
	var createdAt string
	err := s.db.QueryRowContext(
		ctx,
		`SELECT j.job_id, j.status, j.created_at
		 FROM memory_organization_jobs j
		 INNER JOIN memory_acceptance_events e ON e.event_id = j.trigger_event_id
		 WHERE j.user_id = ? AND j.source_platform = ? AND j.source_external_id = ? AND j.status IN ('queued', 'running') AND e.trigger_type = 'posterior_refresh'
		 ORDER BY created_at DESC, job_id DESC
		 LIMIT 1`,
		userID, sourcePlatform, sourceExternalID,
	).Scan(&jobID, &status, &createdAt)
	if err == sql.ErrNoRows {
		return false, 0, "", time.Time{}, nil
	}
	if err != nil {
		return false, 0, "", time.Time{}, err
	}
	jobCreatedAt := parseSQLiteTime(createdAt)
	if jobCreatedAt.After(latestOutputCreatedAt) || (jobCreatedAt.Equal(latestOutputCreatedAt) && jobID > latestOutputJobID) {
		return true, jobID, status, jobCreatedAt, nil
	}
	return false, 0, "", time.Time{}, nil
}

func listUserMemoryBySourceTx(ctx context.Context, tx *sql.Tx, userID, sourcePlatform, sourceExternalID string) ([]memory.AcceptedNode, error) {
	rows, err := tx.QueryContext(ctx, `SELECT memory_id, user_id, source_platform, source_external_id, root_external_id, node_id, node_kind, node_text, source_model, source_compiled_at, valid_from, valid_to, accepted_at
		FROM user_memory_nodes
		WHERE user_id = ? AND source_platform = ? AND source_external_id = ?
		ORDER BY accepted_at ASC, memory_id ASC`, userID, sourcePlatform, sourceExternalID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanMemoryNodes(rows)
}

func getCompiledOutputTx(ctx context.Context, tx *sql.Tx, platform, externalID string) (model.Record, error) {
	var payload string
	if err := tx.QueryRowContext(ctx, `SELECT payload_json FROM compiled_outputs WHERE platform = ? AND external_id = ?`, platform, externalID).Scan(&payload); err != nil {
		return model.Record{}, err
	}
	var record model.Record
	if err := json.Unmarshal([]byte(payload), &record); err != nil {
		return model.Record{}, err
	}
	return record, nil
}

func loadPosteriorStatesBySourceTx(ctx context.Context, tx *sql.Tx, userID, sourcePlatform, sourceExternalID string) (map[int64]posteriorStateRow, error) {
	rows, err := tx.QueryContext(ctx, `SELECT u.memory_id, p.state, p.diagnosis_code, p.reason, p.blocked_by_node_ids_json, p.updated_at
		FROM user_memory_nodes u
		INNER JOIN memory_posterior_states p
		  ON p.source_platform = u.source_platform
		 AND p.source_external_id = u.source_external_id
		 AND p.node_id = u.node_id
		WHERE u.user_id = ? AND u.source_platform = ? AND u.source_external_id = ?`,
		userID, sourcePlatform, sourceExternalID,
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

func isMissingPosteriorStateTableErr(err error) bool {
	return err != nil && strings.Contains(err.Error(), "no such table: memory_posterior_states")
}

func getMemoryContentGraphBySourceTx(ctx context.Context, tx *sql.Tx, userID, sourcePlatform, sourceExternalID string) (model.ContentSubgraph, error) {
	var payload string
	if err := tx.QueryRowContext(ctx, `SELECT payload_json FROM memory_content_graphs WHERE user_id = ? AND source_platform = ? AND source_external_id = ?`, userID, sourcePlatform, sourceExternalID).Scan(&payload); err != nil {
		return model.ContentSubgraph{}, err
	}
	var subgraph model.ContentSubgraph
	if err := json.Unmarshal([]byte(payload), &subgraph); err != nil {
		return model.ContentSubgraph{}, err
	}
	return subgraph, nil
}
