package contentstore

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/kumaloha/VariX/varix/memory"
)

func (s *SQLiteStore) UpsertRelation(ctx context.Context, relation memory.Relation) error {
	relation.RelationID = strings.TrimSpace(relation.RelationID)
	relation.DriverEntityID = strings.TrimSpace(relation.DriverEntityID)
	relation.TargetEntityID = strings.TrimSpace(relation.TargetEntityID)
	relation.LifecycleReason = strings.TrimSpace(relation.LifecycleReason)
	if relation.RelationID == "" {
		return fmt.Errorf("relation id is required")
	}
	if relation.DriverEntityID == "" || relation.TargetEntityID == "" {
		return fmt.Errorf("relation endpoints are required")
	}
	if relation.Status == "" {
		relation.Status = memory.RelationActive
	}
	normalizeCreatedUpdatedTimes(&relation.CreatedAt, &relation.UpdatedAt)

	mergeHistory, splitHistory, err := marshalLifecycleHistory(relation.MergeHistory, relation.SplitHistory)
	if err != nil {
		return err
	}

	_, err = s.db.ExecContext(
		ctx,
		`INSERT INTO memory_relations(relation_id, driver_entity_id, target_entity_id, status, retired_at, superseded_by_relation_id, merge_history_json, split_history_json, lifecycle_reason, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(relation_id) DO UPDATE SET
		   driver_entity_id = excluded.driver_entity_id,
		   target_entity_id = excluded.target_entity_id,
		   status = excluded.status,
		   retired_at = excluded.retired_at,
		   superseded_by_relation_id = excluded.superseded_by_relation_id,
		   merge_history_json = excluded.merge_history_json,
		   split_history_json = excluded.split_history_json,
		   lifecycle_reason = excluded.lifecycle_reason,
		   updated_at = excluded.updated_at`,
		relation.RelationID,
		relation.DriverEntityID,
		relation.TargetEntityID,
		string(relation.Status),
		formatSQLiteTime(relation.RetiredAt),
		nullIfBlank(relation.SupersededByRelationID),
		mergeHistory,
		splitHistory,
		nullIfBlank(relation.LifecycleReason),
		relation.CreatedAt.UTC().Format(time.RFC3339Nano),
		relation.UpdatedAt.UTC().Format(time.RFC3339Nano),
	)
	return err
}

func (s *SQLiteStore) GetRelation(ctx context.Context, relationID string) (memory.Relation, error) {
	relationID = strings.TrimSpace(relationID)
	if relationID == "" {
		return memory.Relation{}, fmt.Errorf("relation id is required")
	}
	var relation memory.Relation
	var status string
	var retiredAt sql.NullString
	var supersededBy sql.NullString
	var mergeHistoryJSON string
	var splitHistoryJSON string
	var lifecycleReason sql.NullString
	var createdAt string
	var updatedAt string
	if err := s.db.QueryRowContext(
		ctx,
		`SELECT relation_id, driver_entity_id, target_entity_id, status, retired_at, superseded_by_relation_id, merge_history_json, split_history_json, lifecycle_reason, created_at, updated_at
		 FROM memory_relations
		 WHERE relation_id = ?`,
		relationID,
	).Scan(
		&relation.RelationID,
		&relation.DriverEntityID,
		&relation.TargetEntityID,
		&status,
		&retiredAt,
		&supersededBy,
		&mergeHistoryJSON,
		&splitHistoryJSON,
		&lifecycleReason,
		&createdAt,
		&updatedAt,
	); err != nil {
		return memory.Relation{}, err
	}
	relation.Status = memory.RelationStatus(status)
	if retiredAt.Valid {
		relation.RetiredAt = parseSQLiteTime(retiredAt.String)
	}
	if supersededBy.Valid {
		relation.SupersededByRelationID = supersededBy.String
	}
	relation.MergeHistory = unmarshalJSONStringSlice(mergeHistoryJSON)
	relation.SplitHistory = unmarshalJSONStringSlice(splitHistoryJSON)
	if lifecycleReason.Valid {
		relation.LifecycleReason = lifecycleReason.String
	}
	relation.CreatedAt = parseSQLiteTime(createdAt)
	relation.UpdatedAt = parseSQLiteTime(updatedAt)
	return relation, nil
}

func (s *SQLiteStore) FindActiveRelationByEntities(ctx context.Context, driverEntityID, targetEntityID string) (memory.Relation, error) {
	driverEntityID = strings.TrimSpace(driverEntityID)
	targetEntityID = strings.TrimSpace(targetEntityID)
	if driverEntityID == "" || targetEntityID == "" {
		return memory.Relation{}, fmt.Errorf("relation endpoints are required")
	}
	var relationID string
	if err := s.db.QueryRowContext(
		ctx,
		`SELECT relation_id
		 FROM memory_relations
		 WHERE driver_entity_id = ? AND target_entity_id = ? AND status IN (?, ?)
		 ORDER BY updated_at DESC, created_at DESC
		 LIMIT 1`,
		driverEntityID,
		targetEntityID,
		string(memory.RelationActive),
		string(memory.RelationInactive),
	).Scan(&relationID); err != nil {
		return memory.Relation{}, err
	}
	return s.GetRelation(ctx, relationID)
}
