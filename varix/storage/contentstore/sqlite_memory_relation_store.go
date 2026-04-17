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
)

func (s *SQLiteStore) UpsertCanonicalEntity(ctx context.Context, entity memory.CanonicalEntity) error {
	entity.EntityID = strings.TrimSpace(entity.EntityID)
	entity.CanonicalName = normalizeCanonicalDisplay(entity.CanonicalName)
	if entity.EntityID == "" {
		return fmt.Errorf("canonical entity id is required")
	}
	if entity.CanonicalName == "" {
		return fmt.Errorf("canonical entity name is required")
	}
	if entity.EntityType == "" {
		return fmt.Errorf("canonical entity type is required")
	}
	if entity.Status == "" {
		entity.Status = memory.CanonicalEntityActive
	}
	now := time.Now().UTC()
	if entity.CreatedAt.IsZero() {
		entity.CreatedAt = now
	}
	if entity.UpdatedAt.IsZero() {
		entity.UpdatedAt = maxTime(entity.CreatedAt, now)
	}

	mergeHistory, err := marshalJSONStringSlice(entity.MergeHistory)
	if err != nil {
		return err
	}
	splitHistory, err := marshalJSONStringSlice(entity.SplitHistory)
	if err != nil {
		return err
	}
	aliases := normalizeCanonicalAliases(entity.CanonicalName, entity.Aliases)

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(
		ctx,
		`INSERT INTO memory_canonical_entities(entity_id, entity_type, canonical_name, status, merge_history_json, split_history_json, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(entity_id) DO UPDATE SET
		   entity_type = excluded.entity_type,
		   canonical_name = excluded.canonical_name,
		   status = excluded.status,
		   merge_history_json = excluded.merge_history_json,
		   split_history_json = excluded.split_history_json,
		   updated_at = excluded.updated_at`,
		entity.EntityID,
		string(entity.EntityType),
		entity.CanonicalName,
		string(entity.Status),
		mergeHistory,
		splitHistory,
		entity.CreatedAt.UTC().Format(time.RFC3339Nano),
		entity.UpdatedAt.UTC().Format(time.RFC3339Nano),
	); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM memory_canonical_entity_aliases WHERE entity_id = ?`, entity.EntityID); err != nil {
		return err
	}
	for _, alias := range aliases {
		if _, err := tx.ExecContext(
			ctx,
			`INSERT INTO memory_canonical_entity_aliases(entity_id, alias_text, created_at) VALUES (?, ?, ?)`,
			entity.EntityID,
			alias,
			entity.UpdatedAt.UTC().Format(time.RFC3339Nano),
		); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *SQLiteStore) GetCanonicalEntity(ctx context.Context, entityID string) (memory.CanonicalEntity, error) {
	var entity memory.CanonicalEntity
	var entityType string
	var status string
	var mergeHistoryJSON string
	var splitHistoryJSON string
	var createdAt string
	var updatedAt string
	entityID = strings.TrimSpace(entityID)
	if entityID == "" {
		return memory.CanonicalEntity{}, fmt.Errorf("canonical entity id is required")
	}
	if err := s.db.QueryRowContext(
		ctx,
		`SELECT entity_id, entity_type, canonical_name, status, merge_history_json, split_history_json, created_at, updated_at
		 FROM memory_canonical_entities
		 WHERE entity_id = ?`,
		entityID,
	).Scan(
		&entity.EntityID,
		&entityType,
		&entity.CanonicalName,
		&status,
		&mergeHistoryJSON,
		&splitHistoryJSON,
		&createdAt,
		&updatedAt,
	); err != nil {
		return memory.CanonicalEntity{}, err
	}
	entity.EntityType = memory.CanonicalEntityType(entityType)
	entity.Status = memory.CanonicalEntityStatus(status)
	entity.MergeHistory = unmarshalJSONStringSlice(mergeHistoryJSON)
	entity.SplitHistory = unmarshalJSONStringSlice(splitHistoryJSON)
	entity.CreatedAt = parseSQLiteTime(createdAt)
	entity.UpdatedAt = parseSQLiteTime(updatedAt)
	aliases, err := s.listCanonicalEntityAliases(ctx, entity.EntityID)
	if err != nil {
		return memory.CanonicalEntity{}, err
	}
	entity.Aliases = aliases
	return entity, nil
}

func (s *SQLiteStore) FindCanonicalEntityByAlias(ctx context.Context, alias string) (memory.CanonicalEntity, error) {
	alias = normalizeCanonicalAlias(alias)
	if alias == "" {
		return memory.CanonicalEntity{}, fmt.Errorf("canonical entity alias is required")
	}
	var entityID string
	if err := s.db.QueryRowContext(
		ctx,
		`SELECT entity_id FROM memory_canonical_entity_aliases WHERE alias_text = ?`,
		alias,
	).Scan(&entityID); err != nil {
		return memory.CanonicalEntity{}, err
	}
	return s.GetCanonicalEntity(ctx, entityID)
}

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
	now := time.Now().UTC()
	if relation.CreatedAt.IsZero() {
		relation.CreatedAt = now
	}
	if relation.UpdatedAt.IsZero() {
		relation.UpdatedAt = maxTime(relation.CreatedAt, now)
	}

	mergeHistory, err := marshalJSONStringSlice(relation.MergeHistory)
	if err != nil {
		return err
	}
	splitHistory, err := marshalJSONStringSlice(relation.SplitHistory)
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

func (s *SQLiteStore) listCanonicalEntityAliases(ctx context.Context, entityID string) ([]string, error) {
	rows, err := s.db.QueryContext(
		ctx,
		`SELECT alias_text FROM memory_canonical_entity_aliases WHERE entity_id = ? ORDER BY alias_text ASC`,
		entityID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	aliases := make([]string, 0)
	for rows.Next() {
		var alias string
		if err := rows.Scan(&alias); err != nil {
			return nil, err
		}
		aliases = append(aliases, alias)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return aliases, nil
}

func marshalJSONStringSlice(values []string) (string, error) {
	normalized := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		normalized = append(normalized, value)
	}
	payload, err := json.Marshal(normalized)
	if err != nil {
		return "", err
	}
	return string(payload), nil
}

func unmarshalJSONStringSlice(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	var out []string
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return nil
	}
	return out
}

func normalizeCanonicalAliases(canonicalName string, aliases []string) []string {
	set := map[string]struct{}{}
	out := make([]string, 0, len(aliases)+1)
	for _, alias := range append([]string{canonicalName}, aliases...) {
		normalized := normalizeCanonicalAlias(alias)
		if normalized == "" {
			continue
		}
		if _, ok := set[normalized]; ok {
			continue
		}
		set[normalized] = struct{}{}
		out = append(out, normalized)
	}
	sort.Strings(out)
	return out
}

func normalizeCanonicalAlias(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return ""
	}
	return strings.Join(strings.Fields(value), " ")
}

func normalizeCanonicalDisplay(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	return strings.Join(strings.Fields(value), " ")
}

func nullIfBlank(value string) any {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	return value
}

func maxTime(left, right time.Time) time.Time {
	if right.After(left) {
		return right
	}
	return left
}
