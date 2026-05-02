package contentstore

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/kumaloha/VariX/varix/memory"
	"github.com/kumaloha/VariX/varix/model"
)

var ErrMemoryOrganizationOutputStale = errors.New("memory organization output is stale")

type posteriorStateRow struct {
	State            memory.PosteriorState
	Diagnosis        memory.PosteriorDiagnosisCode
	Reason           string
	BlockedByNodeIDs []string
	UpdatedAt        *time.Time
}

type organizationJobSourceData struct {
	record                        model.Record
	verification                  model.Verification
	nodes                         []memory.AcceptedNode
	posteriorByMemoryID           map[int64]posteriorStateRow
	graphFirstSubgraph            model.ContentSubgraph
	hasGraphFirstSubgraph         bool
	graphNodesByID                map[string]model.GraphNode
	graphFirstNodesByID           map[string]model.ContentNode
	factStatusByNode              map[string]model.FactStatus
	explicitConditionStatusByNode map[string]model.ExplicitConditionStatus
	predictionStatusByNode        map[string]model.PredictionStatus
}

type organizationNodeSets struct {
	derived  []memory.AcceptedNode
	active   []memory.AcceptedNode
	inactive []memory.AcceptedNode
}

func (s *SQLiteStore) RunNextMemoryOrganizationJob(ctx context.Context, userID string, now time.Time) (memory.OrganizationOutput, error) {
	var job memory.OrganizationJob
	var createdAt string
	query := `SELECT job_id, trigger_event_id, user_id, source_platform, source_external_id, status, created_at, started_at, finished_at
		FROM memory_organization_jobs
		WHERE status = 'queued'`
	args := []any{}
	if strings.TrimSpace(userID) != "" {
		query += ` AND user_id = ?`
		args = append(args, strings.TrimSpace(userID))
	}
	query += ` ORDER BY created_at ASC, job_id ASC LIMIT 1`
	err := s.db.QueryRowContext(ctx, query, args...).Scan(&job.JobID, &job.TriggerEventID, &job.UserID, &job.SourcePlatform, &job.SourceExternalID, &job.Status, &createdAt, new(sql.NullString), new(sql.NullString))
	if err != nil {
		return memory.OrganizationOutput{}, err
	}
	job.CreatedAt = parseSQLiteTime(createdAt)
	now = normalizeNow(now)

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return memory.OrganizationOutput{}, err
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, `UPDATE memory_organization_jobs SET status = ?, started_at = ? WHERE job_id = ?`, "running", now.Format(time.RFC3339Nano), job.JobID); err != nil {
		return memory.OrganizationOutput{}, err
	}

	sourceData, err := loadOrganizationJobSourceData(ctx, tx, job)
	if err != nil {
		return memory.OrganizationOutput{}, err
	}
	nodeSets := deriveOrganizationNodeSets(sourceData, now)
	dedupeGroups := buildDedupeGroups(nodeSets.active, sourceData.factStatusByNode, sourceData.predictionStatusByNode)
	contradictionGroups := buildContradictionGroups(nodeSets.active)
	hierarchy := buildHierarchy(nodeSets.active, sourceData.record, sourceData.verification, sourceData.graphFirstSubgraph, sourceData.hasGraphFirstSubgraph)
	nodeHints := buildNodeHints(nodeSets.derived, nodeSets.active, dedupeGroups, contradictionGroups, hierarchy, sourceData.factStatusByNode, sourceData.explicitConditionStatusByNode, sourceData.predictionStatusByNode)
	dominantDriver := buildDominantDriverSummary(nodeSets.active, nodeHints)
	nodeHints = applyDominantDriverRoles(nodeHints, dominantDriver)
	feedback := buildOrganizationFeedback(nodeSets.derived, nodeHints)

	output := memory.OrganizationOutput{
		JobID:               job.JobID,
		UserID:              job.UserID,
		SourcePlatform:      job.SourcePlatform,
		SourceExternalID:    job.SourceExternalID,
		GeneratedAt:         now,
		ActiveNodes:         nodeSets.active,
		InactiveNodes:       nodeSets.inactive,
		DedupeGroups:        dedupeGroups,
		ContradictionGroups: contradictionGroups,
		Hierarchy:           hierarchy,
		PredictionStatuses:  extractPredictionStatuses(sourceData.nodes, sourceData.verification),
		FactVerifications:   extractFactVerifications(nodeSets.active, sourceData.verification),
		OpenQuestions:       buildOpenQuestions(nodeSets.active, sourceData.verification),
		NodeHints:           nodeHints,
		DominantDriver:      dominantDriver,
		Feedback:            feedback,
	}

	res, err := tx.ExecContext(ctx, `INSERT INTO memory_organization_outputs(job_id, user_id, source_platform, source_external_id, payload_json, created_at)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(job_id) DO UPDATE SET created_at = excluded.created_at`,
		job.JobID, job.UserID, job.SourcePlatform, job.SourceExternalID, "{}", now.Format(time.RFC3339Nano))
	if err != nil {
		return memory.OrganizationOutput{}, err
	}
	outputID, _ := res.LastInsertId()
	if outputID == 0 {
		_ = tx.QueryRowContext(ctx, `SELECT output_id FROM memory_organization_outputs WHERE job_id = ?`, job.JobID).Scan(&outputID)
	}
	output.OutputID = outputID
	payload, err := json.Marshal(output)
	if err != nil {
		return memory.OrganizationOutput{}, err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE memory_organization_outputs SET payload_json = ?, created_at = ? WHERE output_id = ?`, string(payload), now.Format(time.RFC3339Nano), outputID); err != nil {
		return memory.OrganizationOutput{}, err
	}

	if _, err := tx.ExecContext(ctx, `UPDATE memory_organization_jobs SET status = ?, finished_at = ? WHERE job_id = ?`, "done", now.Format(time.RFC3339Nano), job.JobID); err != nil {
		return memory.OrganizationOutput{}, err
	}

	if err := tx.Commit(); err != nil {
		return memory.OrganizationOutput{}, err
	}
	return output, nil
}
