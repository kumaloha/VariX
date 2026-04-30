package contentstore

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/kumaloha/VariX/varix/memory"
)

func (s *SQLiteStore) UpsertRawCanonicalMapping(ctx context.Context, mapping memory.RawCanonicalMapping) error {
	mapping.CanonicalObjectID = strings.TrimSpace(mapping.CanonicalObjectID)
	mapping.SourcePlatform = strings.TrimSpace(mapping.SourcePlatform)
	mapping.SourceExternalID = strings.TrimSpace(mapping.SourceExternalID)
	mapping.RawNodeID = strings.TrimSpace(mapping.RawNodeID)
	mapping.RawEdgeKey = strings.TrimSpace(mapping.RawEdgeKey)
	if mapping.CanonicalObjectType == "" {
		return fmt.Errorf("canonical object type is required")
	}
	if mapping.CanonicalObjectID == "" {
		return fmt.Errorf("canonical object id is required")
	}
	if mapping.SourcePlatform == "" || mapping.SourceExternalID == "" {
		return fmt.Errorf("source platform and external id are required")
	}
	if mapping.RawNodeID == "" && mapping.RawEdgeKey == "" {
		return fmt.Errorf("raw node id or raw edge key is required")
	}
	normalizeCreatedUpdatedTimes(&mapping.CreatedAt, &mapping.UpdatedAt)
	_, err := s.db.ExecContext(
		ctx,
		`INSERT INTO memory_raw_canonical_mappings(canonical_object_type, canonical_object_id, source_platform, source_external_id, raw_node_id, raw_edge_key, mapping_confidence, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(canonical_object_type, canonical_object_id, source_platform, source_external_id, raw_node_id, raw_edge_key) DO UPDATE SET
		   mapping_confidence = excluded.mapping_confidence,
		   created_at = excluded.created_at,
		   updated_at = excluded.updated_at`,
		string(mapping.CanonicalObjectType),
		mapping.CanonicalObjectID,
		mapping.SourcePlatform,
		mapping.SourceExternalID,
		mapping.RawNodeID,
		mapping.RawEdgeKey,
		mapping.MappingConfidence,
		mapping.CreatedAt.UTC().Format(time.RFC3339Nano),
		mapping.UpdatedAt.UTC().Format(time.RFC3339Nano),
	)
	return err
}

func (s *SQLiteStore) ListRawCanonicalMappingsForCanonicalObject(ctx context.Context, objectType memory.CanonicalObjectType, objectID string) ([]memory.RawCanonicalMapping, error) {
	objectID = strings.TrimSpace(objectID)
	if objectType == "" {
		return nil, fmt.Errorf("canonical object type is required")
	}
	if objectID == "" {
		return nil, fmt.Errorf("canonical object id is required")
	}
	rows, err := s.db.QueryContext(
		ctx,
		`SELECT canonical_object_type, canonical_object_id, source_platform, source_external_id, raw_node_id, raw_edge_key, mapping_confidence, created_at, updated_at
		 FROM memory_raw_canonical_mappings
		 WHERE canonical_object_type = ? AND canonical_object_id = ?
		 ORDER BY source_platform ASC, source_external_id ASC, raw_node_id ASC, raw_edge_key ASC`,
		string(objectType),
		objectID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanRawCanonicalMappings(rows)
}

func (s *SQLiteStore) ListRawCanonicalMappingsForSource(ctx context.Context, sourcePlatform, sourceExternalID string) ([]memory.RawCanonicalMapping, error) {
	sourcePlatform = strings.TrimSpace(sourcePlatform)
	sourceExternalID = strings.TrimSpace(sourceExternalID)
	if sourcePlatform == "" || sourceExternalID == "" {
		return nil, fmt.Errorf("source platform and external id are required")
	}
	rows, err := s.db.QueryContext(
		ctx,
		`SELECT canonical_object_type, canonical_object_id, source_platform, source_external_id, raw_node_id, raw_edge_key, mapping_confidence, created_at, updated_at
		 FROM memory_raw_canonical_mappings
		 WHERE source_platform = ? AND source_external_id = ?
		 ORDER BY canonical_object_type ASC, canonical_object_id ASC, raw_node_id ASC, raw_edge_key ASC`,
		sourcePlatform,
		sourceExternalID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanRawCanonicalMappings(rows)
}

func scanRawCanonicalMappings(rows *sql.Rows) ([]memory.RawCanonicalMapping, error) {
	mappings := make([]memory.RawCanonicalMapping, 0)
	for rows.Next() {
		var mapping memory.RawCanonicalMapping
		var objectType string
		var createdAt string
		var updatedAt string
		if err := rows.Scan(
			&objectType,
			&mapping.CanonicalObjectID,
			&mapping.SourcePlatform,
			&mapping.SourceExternalID,
			&mapping.RawNodeID,
			&mapping.RawEdgeKey,
			&mapping.MappingConfidence,
			&createdAt,
			&updatedAt,
		); err != nil {
			return nil, err
		}
		mapping.CanonicalObjectType = memory.CanonicalObjectType(objectType)
		mapping.CreatedAt = parseSQLiteTime(createdAt)
		mapping.UpdatedAt = parseSQLiteTime(updatedAt)
		mappings = append(mappings, mapping)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return mappings, nil
}
