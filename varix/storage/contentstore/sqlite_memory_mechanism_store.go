package contentstore

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/kumaloha/VariX/varix/memory"
)

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
	now := normalizeNow(time.Time{})
	if mechanism.AsOf.IsZero() {
		mechanism.AsOf = now
	}
	if mechanism.CreatedAt.IsZero() {
		mechanism.CreatedAt = mechanism.AsOf
	}
	normalizeCreatedUpdatedTimes(&mechanism.CreatedAt, &mechanism.UpdatedAt)
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
	asOf = normalizeNow(asOf)
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
