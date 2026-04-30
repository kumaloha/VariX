package contentstore

import (
	"context"
	"fmt"
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
	normalizeCreatedUpdatedTimes(&entity.CreatedAt, &entity.UpdatedAt)

	mergeHistory, splitHistory, err := marshalLifecycleHistory(entity.MergeHistory, entity.SplitHistory)
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

func (s *SQLiteStore) ListCanonicalEntities(ctx context.Context) ([]memory.CanonicalEntity, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT entity_id FROM memory_canonical_entities ORDER BY canonical_name ASC, entity_id ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]memory.CanonicalEntity, 0)
	for rows.Next() {
		var entityID string
		if err := rows.Scan(&entityID); err != nil {
			return nil, err
		}
		entity, err := s.GetCanonicalEntity(ctx, entityID)
		if err != nil {
			return nil, err
		}
		out = append(out, entity)
	}
	return out, rows.Err()
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
