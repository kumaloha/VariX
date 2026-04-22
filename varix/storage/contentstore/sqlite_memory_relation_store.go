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

func (s *SQLiteStore) UpsertMechanismGraph(ctx context.Context, graph memory.MechanismGraph) error {
	mechanism := graph.Mechanism
	mechanism.MechanismID = strings.TrimSpace(mechanism.MechanismID)
	mechanism.RelationID = strings.TrimSpace(mechanism.RelationID)
	if mechanism.MechanismID == "" {
		return fmt.Errorf("mechanism id is required")
	}
	if mechanism.RelationID == "" {
		return fmt.Errorf("relation id is required")
	}
	if mechanism.Status == "" {
		mechanism.Status = memory.MechanismActive
	}
	if mechanism.TraceabilityStatus == "" {
		mechanism.TraceabilityStatus = memory.TraceabilityPartial
	}
	now := time.Now().UTC()
	if mechanism.AsOf.IsZero() {
		mechanism.AsOf = now
	}
	if mechanism.CreatedAt.IsZero() {
		mechanism.CreatedAt = mechanism.AsOf
	}
	if mechanism.UpdatedAt.IsZero() {
		mechanism.UpdatedAt = maxTime(mechanism.CreatedAt, now)
	}
	sourceRefs, err := marshalJSONStringSlice(mechanism.SourceRefs)
	if err != nil {
		return err
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(
		ctx,
		`INSERT INTO memory_mechanisms(mechanism_id, relation_id, as_of, valid_from, valid_to, confidence, status, source_refs_json, traceability_status, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(mechanism_id) DO UPDATE SET
		   relation_id = excluded.relation_id,
		   as_of = excluded.as_of,
		   valid_from = excluded.valid_from,
		   valid_to = excluded.valid_to,
		   confidence = excluded.confidence,
		   status = excluded.status,
		   source_refs_json = excluded.source_refs_json,
		   traceability_status = excluded.traceability_status,
		   created_at = excluded.created_at,
		   updated_at = excluded.updated_at`,
		mechanism.MechanismID,
		mechanism.RelationID,
		mechanism.AsOf.UTC().Format(time.RFC3339Nano),
		formatSQLiteTime(mechanism.ValidFrom),
		formatSQLiteTime(mechanism.ValidTo),
		mechanism.Confidence,
		string(mechanism.Status),
		sourceRefs,
		string(mechanism.TraceabilityStatus),
		mechanism.CreatedAt.UTC().Format(time.RFC3339Nano),
		mechanism.UpdatedAt.UTC().Format(time.RFC3339Nano),
	); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM memory_mechanism_nodes WHERE mechanism_id = ?`, mechanism.MechanismID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM memory_mechanism_edges WHERE mechanism_id = ?`, mechanism.MechanismID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM memory_path_outcomes WHERE mechanism_id = ?`, mechanism.MechanismID); err != nil {
		return err
	}
	for _, node := range graph.Nodes {
		if err := insertMechanismNode(ctx, tx, mechanism.MechanismID, mechanism.UpdatedAt, node); err != nil {
			return err
		}
	}
	for _, edge := range graph.Edges {
		if err := insertMechanismEdge(ctx, tx, mechanism.MechanismID, mechanism.UpdatedAt, edge); err != nil {
			return err
		}
	}
	for _, outcome := range graph.PathOutcomes {
		if err := insertPathOutcome(ctx, tx, mechanism.MechanismID, mechanism.UpdatedAt, outcome); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *SQLiteStore) GetMechanismGraph(ctx context.Context, mechanismID string) (memory.MechanismGraph, error) {
	mechanism, err := s.getMechanism(ctx, mechanismID)
	if err != nil {
		return memory.MechanismGraph{}, err
	}
	nodes, err := s.listMechanismNodes(ctx, mechanism.MechanismID)
	if err != nil {
		return memory.MechanismGraph{}, err
	}
	edges, err := s.listMechanismEdges(ctx, mechanism.MechanismID)
	if err != nil {
		return memory.MechanismGraph{}, err
	}
	outcomes, err := s.listPathOutcomes(ctx, mechanism.MechanismID)
	if err != nil {
		return memory.MechanismGraph{}, err
	}
	return memory.MechanismGraph{
		Mechanism:    mechanism,
		Nodes:        nodes,
		Edges:        edges,
		PathOutcomes: outcomes,
	}, nil
}

func (s *SQLiteStore) ListMechanismGraphsByRelation(ctx context.Context, relationID string) ([]memory.MechanismGraph, error) {
	relationID = strings.TrimSpace(relationID)
	if relationID == "" {
		return nil, fmt.Errorf("relation id is required")
	}
	rows, err := s.db.QueryContext(
		ctx,
		`SELECT mechanism_id
		 FROM memory_mechanisms
		 WHERE relation_id = ?
		 ORDER BY as_of DESC, created_at DESC, mechanism_id ASC`,
		relationID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	graphs := make([]memory.MechanismGraph, 0)
	for rows.Next() {
		var mechanismID string
		if err := rows.Scan(&mechanismID); err != nil {
			return nil, err
		}
		graph, err := s.GetMechanismGraph(ctx, mechanismID)
		if err != nil {
			return nil, err
		}
		graphs = append(graphs, graph)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return graphs, nil
}

func (s *SQLiteStore) GetCurrentMechanismGraph(ctx context.Context, relationID string, asOf time.Time) (memory.MechanismGraph, error) {
	relationID = strings.TrimSpace(relationID)
	if relationID == "" {
		return memory.MechanismGraph{}, fmt.Errorf("relation id is required")
	}
	if asOf.IsZero() {
		asOf = time.Now().UTC()
	}
	var mechanismID string
	if err := s.db.QueryRowContext(
		ctx,
		`SELECT mechanism_id
		 FROM memory_mechanisms
		 WHERE relation_id = ? AND as_of <= ?
		 ORDER BY as_of DESC, created_at DESC, mechanism_id ASC
		 LIMIT 1`,
		relationID,
		asOf.UTC().Format(time.RFC3339Nano),
	).Scan(&mechanismID); err != nil {
		return memory.MechanismGraph{}, err
	}
	return s.GetMechanismGraph(ctx, mechanismID)
}

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
	now := time.Now().UTC()
	if mapping.CreatedAt.IsZero() {
		mapping.CreatedAt = now
	}
	if mapping.UpdatedAt.IsZero() {
		mapping.UpdatedAt = maxTime(mapping.CreatedAt, now)
	}
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

func (s *SQLiteStore) getMechanism(ctx context.Context, mechanismID string) (memory.Mechanism, error) {
	mechanismID = strings.TrimSpace(mechanismID)
	if mechanismID == "" {
		return memory.Mechanism{}, fmt.Errorf("mechanism id is required")
	}
	var mechanism memory.Mechanism
	var asOf string
	var status string
	var validFrom sql.NullString
	var validTo sql.NullString
	var sourceRefsJSON string
	var traceabilityStatus string
	var createdAt string
	var updatedAt string
	if err := s.db.QueryRowContext(
		ctx,
		`SELECT mechanism_id, relation_id, as_of, valid_from, valid_to, confidence, status, source_refs_json, traceability_status, created_at, updated_at
		 FROM memory_mechanisms
		 WHERE mechanism_id = ?`,
		mechanismID,
	).Scan(
		&mechanism.MechanismID,
		&mechanism.RelationID,
		&asOf,
		&validFrom,
		&validTo,
		&mechanism.Confidence,
		&status,
		&sourceRefsJSON,
		&traceabilityStatus,
		&createdAt,
		&updatedAt,
	); err != nil {
		return memory.Mechanism{}, err
	}
	mechanism.AsOf = parseSQLiteTime(asOf)
	if validFrom.Valid {
		mechanism.ValidFrom = parseSQLiteTime(validFrom.String)
	}
	if validTo.Valid {
		mechanism.ValidTo = parseSQLiteTime(validTo.String)
	}
	mechanism.Status = memory.MechanismStatus(status)
	mechanism.SourceRefs = unmarshalOptionalJSONStringSlice(sourceRefsJSON)
	mechanism.TraceabilityStatus = memory.TraceabilityStatus(traceabilityStatus)
	mechanism.CreatedAt = parseSQLiteTime(createdAt)
	mechanism.UpdatedAt = parseSQLiteTime(updatedAt)
	return mechanism, nil
}

func (s *SQLiteStore) listMechanismNodes(ctx context.Context, mechanismID string) ([]memory.MechanismNode, error) {
	rows, err := s.db.QueryContext(
		ctx,
		`SELECT mechanism_node_id, mechanism_id, node_type, label, backing_accepted_node_ids_json, sort_order, created_at
		 FROM memory_mechanism_nodes
		 WHERE mechanism_id = ?
		 ORDER BY sort_order ASC, created_at ASC, mechanism_node_id ASC`,
		mechanismID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var nodes []memory.MechanismNode
	for rows.Next() {
		var node memory.MechanismNode
		var nodeType string
		var backingJSON string
		var sortOrder sql.NullInt64
		var createdAt string
		if err := rows.Scan(
			&node.MechanismNodeID,
			&node.MechanismID,
			&nodeType,
			&node.Label,
			&backingJSON,
			&sortOrder,
			&createdAt,
		); err != nil {
			return nil, err
		}
		node.NodeType = memory.MechanismNodeType(nodeType)
		node.BackingAcceptedNodeIDs = unmarshalOptionalJSONStringSlice(backingJSON)
		if sortOrder.Valid {
			node.SortOrder = int(sortOrder.Int64)
		}
		node.CreatedAt = parseSQLiteTime(createdAt)
		nodes = append(nodes, node)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return nodes, nil
}

func (s *SQLiteStore) listMechanismEdges(ctx context.Context, mechanismID string) ([]memory.MechanismEdge, error) {
	rows, err := s.db.QueryContext(
		ctx,
		`SELECT mechanism_edge_id, mechanism_id, from_node_id, to_node_id, edge_type, created_at
		 FROM memory_mechanism_edges
		 WHERE mechanism_id = ?
		 ORDER BY created_at ASC, mechanism_edge_id ASC`,
		mechanismID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var edges []memory.MechanismEdge
	for rows.Next() {
		var edge memory.MechanismEdge
		var edgeType string
		var createdAt string
		if err := rows.Scan(
			&edge.MechanismEdgeID,
			&edge.MechanismID,
			&edge.FromNodeID,
			&edge.ToNodeID,
			&edgeType,
			&createdAt,
		); err != nil {
			return nil, err
		}
		edge.EdgeType = memory.MechanismEdgeType(edgeType)
		edge.CreatedAt = parseSQLiteTime(createdAt)
		edges = append(edges, edge)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return edges, nil
}

func (s *SQLiteStore) listPathOutcomes(ctx context.Context, mechanismID string) ([]memory.PathOutcome, error) {
	rows, err := s.db.QueryContext(
		ctx,
		`SELECT path_outcome_id, mechanism_id, node_path_json, outcome_polarity, outcome_label, condition_scope, confidence, created_at
		 FROM memory_path_outcomes
		 WHERE mechanism_id = ?
		 ORDER BY created_at ASC, path_outcome_id ASC`,
		mechanismID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var outcomes []memory.PathOutcome
	for rows.Next() {
		var outcome memory.PathOutcome
		var nodePathJSON string
		var polarity string
		var conditionScope sql.NullString
		var createdAt string
		if err := rows.Scan(
			&outcome.PathOutcomeID,
			&outcome.MechanismID,
			&nodePathJSON,
			&polarity,
			&outcome.OutcomeLabel,
			&conditionScope,
			&outcome.Confidence,
			&createdAt,
		); err != nil {
			return nil, err
		}
		outcome.NodePath = unmarshalOptionalJSONStringSlice(nodePathJSON)
		outcome.OutcomePolarity = memory.OutcomePolarity(polarity)
		if conditionScope.Valid {
			outcome.ConditionScope = conditionScope.String
		}
		outcome.CreatedAt = parseSQLiteTime(createdAt)
		outcomes = append(outcomes, outcome)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return outcomes, nil
}

func insertMechanismNode(ctx context.Context, tx *sql.Tx, mechanismID string, fallbackTime time.Time, node memory.MechanismNode) error {
	node.MechanismNodeID = strings.TrimSpace(node.MechanismNodeID)
	node.MechanismID = strings.TrimSpace(node.MechanismID)
	node.Label = strings.TrimSpace(node.Label)
	if node.MechanismNodeID == "" {
		return fmt.Errorf("mechanism node id is required")
	}
	if node.NodeType == "" {
		return fmt.Errorf("mechanism node type is required")
	}
	if node.Label == "" {
		return fmt.Errorf("mechanism node label is required")
	}
	if node.MechanismID == "" {
		node.MechanismID = mechanismID
	}
	if node.MechanismID != mechanismID {
		return fmt.Errorf("mechanism node %s has mismatched mechanism id %s", node.MechanismNodeID, node.MechanismID)
	}
	if node.CreatedAt.IsZero() {
		node.CreatedAt = fallbackTime
	}
	backingJSON, err := marshalJSONStringSlice(node.BackingAcceptedNodeIDs)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(
		ctx,
		`INSERT INTO memory_mechanism_nodes(mechanism_node_id, mechanism_id, node_type, label, backing_accepted_node_ids_json, sort_order, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		node.MechanismNodeID,
		node.MechanismID,
		string(node.NodeType),
		node.Label,
		backingJSON,
		node.SortOrder,
		node.CreatedAt.UTC().Format(time.RFC3339Nano),
	)
	return err
}

func insertMechanismEdge(ctx context.Context, tx *sql.Tx, mechanismID string, fallbackTime time.Time, edge memory.MechanismEdge) error {
	edge.MechanismEdgeID = strings.TrimSpace(edge.MechanismEdgeID)
	edge.MechanismID = strings.TrimSpace(edge.MechanismID)
	edge.FromNodeID = strings.TrimSpace(edge.FromNodeID)
	edge.ToNodeID = strings.TrimSpace(edge.ToNodeID)
	if edge.MechanismEdgeID == "" {
		return fmt.Errorf("mechanism edge id is required")
	}
	if edge.EdgeType == "" {
		return fmt.Errorf("mechanism edge type is required")
	}
	if edge.FromNodeID == "" || edge.ToNodeID == "" {
		return fmt.Errorf("mechanism edge endpoints are required")
	}
	if edge.MechanismID == "" {
		edge.MechanismID = mechanismID
	}
	if edge.MechanismID != mechanismID {
		return fmt.Errorf("mechanism edge %s has mismatched mechanism id %s", edge.MechanismEdgeID, edge.MechanismID)
	}
	if edge.CreatedAt.IsZero() {
		edge.CreatedAt = fallbackTime
	}
	_, err := tx.ExecContext(
		ctx,
		`INSERT INTO memory_mechanism_edges(mechanism_edge_id, mechanism_id, from_node_id, to_node_id, edge_type, created_at)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		edge.MechanismEdgeID,
		edge.MechanismID,
		edge.FromNodeID,
		edge.ToNodeID,
		string(edge.EdgeType),
		edge.CreatedAt.UTC().Format(time.RFC3339Nano),
	)
	return err
}

func insertPathOutcome(ctx context.Context, tx *sql.Tx, mechanismID string, fallbackTime time.Time, outcome memory.PathOutcome) error {
	outcome.PathOutcomeID = strings.TrimSpace(outcome.PathOutcomeID)
	outcome.MechanismID = strings.TrimSpace(outcome.MechanismID)
	outcome.OutcomeLabel = strings.TrimSpace(outcome.OutcomeLabel)
	outcome.ConditionScope = strings.TrimSpace(outcome.ConditionScope)
	if outcome.PathOutcomeID == "" {
		return fmt.Errorf("path outcome id is required")
	}
	if outcome.OutcomePolarity == "" {
		return fmt.Errorf("path outcome polarity is required")
	}
	if outcome.OutcomeLabel == "" {
		return fmt.Errorf("path outcome label is required")
	}
	if outcome.MechanismID == "" {
		outcome.MechanismID = mechanismID
	}
	if outcome.MechanismID != mechanismID {
		return fmt.Errorf("path outcome %s has mismatched mechanism id %s", outcome.PathOutcomeID, outcome.MechanismID)
	}
	if outcome.CreatedAt.IsZero() {
		outcome.CreatedAt = fallbackTime
	}
	nodePathJSON, err := marshalJSONStringSlice(outcome.NodePath)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(
		ctx,
		`INSERT INTO memory_path_outcomes(path_outcome_id, mechanism_id, node_path_json, outcome_polarity, outcome_label, condition_scope, confidence, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		outcome.PathOutcomeID,
		outcome.MechanismID,
		nodePathJSON,
		string(outcome.OutcomePolarity),
		outcome.OutcomeLabel,
		nullIfBlank(outcome.ConditionScope),
		outcome.Confidence,
		outcome.CreatedAt.UTC().Format(time.RFC3339Nano),
	)
	return err
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

func unmarshalOptionalJSONStringSlice(raw string) []string {
	out := unmarshalJSONStringSlice(raw)
	if len(out) == 0 {
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
