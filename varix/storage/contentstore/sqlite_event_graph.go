package contentstore

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/kumaloha/VariX/varix/graphmodel"
)

type EventGraphEvidenceLink struct {
	EventGraphID string `json:"event_graph_id"`
	SubgraphID   string `json:"subgraph_id"`
	NodeID       string `json:"node_id"`
}

type EventGraphRecord struct {
	EventGraphID          string                                `json:"event_graph_id"`
	UserID                string                                `json:"user_id"`
	Scope                 string                                `json:"scope"`
	AnchorSubject         string                                `json:"anchor_subject"`
	TimeBucket            string                                `json:"time_bucket"`
	SourceSubgraphIDs     []string                              `json:"source_subgraph_ids,omitempty"`
	SourceArticleIDs      []string                              `json:"source_article_ids,omitempty"`
	NodeIDs               []string                              `json:"node_ids,omitempty"`
	RepresentativeChanges []string                              `json:"representative_changes,omitempty"`
	TraceabilityMap       map[string][]string                   `json:"traceability_map,omitempty"`
	SourceSubgraphCount   int                                   `json:"source_subgraph_count"`
	PrimaryNodeCount      int                                   `json:"primary_node_count"`
	VerificationSummary   map[graphmodel.VerificationStatus]int `json:"verification_summary,omitempty"`
	GeneratedAt           string                                `json:"generated_at"`
	UpdatedAt             string                                `json:"updated_at"`
}

func (s *SQLiteStore) RunEventGraphProjection(ctx context.Context, userID string, now time.Time) ([]EventGraphRecord, error) {
	if strings.TrimSpace(userID) == "" {
		return nil, fmt.Errorf("user id is required")
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	candidates, err := s.BuildEventInputCandidates(ctx, userID)
	if err != nil {
		return nil, err
	}
	graphs := make([]EventGraphRecord, 0, len(candidates))
	verificationByNodeID, err := s.eventVerificationStatusIndex(ctx, strings.TrimSpace(userID))
	if err != nil {
		return nil, err
	}
	nodeChangesByID, err := s.eventNodeChangeIndex(ctx, strings.TrimSpace(userID))
	if err != nil {
		return nil, err
	}
	traceabilityByNodeID, err := s.eventNodeTraceabilityIndex(ctx, strings.TrimSpace(userID))
	if err != nil {
		return nil, err
	}
	for _, candidate := range candidates {
		graph := EventGraphRecord{
			EventGraphID:          buildEventGraphID(strings.TrimSpace(userID), candidate.Scope, candidate.AnchorSubject, candidate.TimeBucket),
			UserID:                strings.TrimSpace(userID),
			Scope:                 candidate.Scope,
			AnchorSubject:         candidate.AnchorSubject,
			TimeBucket:            candidate.TimeBucket,
			SourceSubgraphIDs:     uniqueStrings(candidate.SourceSubgraphIDs),
			SourceArticleIDs:      uniqueStrings(candidate.SourceArticleIDs),
			NodeIDs:               uniqueStrings(candidate.NodeIDs),
			RepresentativeChanges: eventRepresentativeChanges(candidate.NodeIDs, nodeChangesByID),
			TraceabilityMap:       eventTraceabilityMap(candidate.NodeIDs, traceabilityByNodeID),
			SourceSubgraphCount:   len(uniqueStrings(candidate.SourceSubgraphIDs)),
			PrimaryNodeCount:      len(uniqueStrings(candidate.NodeIDs)),
			VerificationSummary:   summarizeVerificationStatuses(candidate.NodeIDs, verificationByNodeID),
			GeneratedAt:           now.UTC().Format(time.RFC3339),
			UpdatedAt:             now.UTC().Format(time.RFC3339),
		}
		if err := s.upsertEventGraph(ctx, graph); err != nil {
			return nil, err
		}
		if err := s.replaceEventGraphEvidenceLinks(ctx, graph); err != nil {
			return nil, err
		}
		graphs = append(graphs, graph)
	}
	sort.Slice(graphs, func(i, j int) bool {
		if graphs[i].AnchorSubject != graphs[j].AnchorSubject {
			return graphs[i].AnchorSubject < graphs[j].AnchorSubject
		}
		if graphs[i].Scope != graphs[j].Scope {
			return graphs[i].Scope < graphs[j].Scope
		}
		return graphs[i].TimeBucket < graphs[j].TimeBucket
	})
	return graphs, nil
}

func (s *SQLiteStore) ListEventGraphsBySubject(ctx context.Context, userID, subject string) ([]EventGraphRecord, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT payload_json FROM event_graphs WHERE user_id = ? AND anchor_subject = ? ORDER BY anchor_subject ASC, scope ASC, time_bucket ASC`, strings.TrimSpace(userID), strings.TrimSpace(subject))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]EventGraphRecord, 0)
	for rows.Next() {
		var payload string
		if err := rows.Scan(&payload); err != nil {
			return nil, err
		}
		var graph EventGraphRecord
		if err := json.Unmarshal([]byte(payload), &graph); err != nil {
			return nil, fmt.Errorf("decode event graph payload: %w", err)
		}
		out = append(out, graph)
	}
	return out, rows.Err()
}

func (s *SQLiteStore) ListEventGraphsByScope(ctx context.Context, userID, scope string) ([]EventGraphRecord, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT payload_json FROM event_graphs WHERE user_id = ? AND scope = ? ORDER BY anchor_subject ASC, scope ASC, time_bucket ASC`, strings.TrimSpace(userID), strings.TrimSpace(scope))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]EventGraphRecord, 0)
	for rows.Next() {
		var payload string
		if err := rows.Scan(&payload); err != nil {
			return nil, err
		}
		var graph EventGraphRecord
		if err := json.Unmarshal([]byte(payload), &graph); err != nil {
			return nil, fmt.Errorf("decode event graph payload: %w", err)
		}
		out = append(out, graph)
	}
	return out, rows.Err()
}

func (s *SQLiteStore) ListEventGraphs(ctx context.Context, userID string) ([]EventGraphRecord, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT payload_json FROM event_graphs WHERE user_id = ? ORDER BY anchor_subject ASC, scope ASC, time_bucket ASC`, strings.TrimSpace(userID))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]EventGraphRecord, 0)
	for rows.Next() {
		var payload string
		if err := rows.Scan(&payload); err != nil {
			return nil, err
		}
		var graph EventGraphRecord
		if err := json.Unmarshal([]byte(payload), &graph); err != nil {
			return nil, fmt.Errorf("decode event graph payload: %w", err)
		}
		out = append(out, graph)
	}
	return out, rows.Err()
}

func (s *SQLiteStore) upsertEventGraph(ctx context.Context, graph EventGraphRecord) error {
	payload, err := json.Marshal(graph)
	if err != nil {
		return err
	}
	generatedAt, err := time.Parse(time.RFC3339, graph.GeneratedAt)
	if err != nil {
		return err
	}
	updatedAt, err := time.Parse(time.RFC3339, graph.UpdatedAt)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `INSERT INTO event_graphs(event_graph_id, user_id, scope, anchor_subject, time_bucket, payload_json, generated_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(event_graph_id) DO UPDATE SET
		  payload_json = excluded.payload_json,
		  generated_at = excluded.generated_at,
		  updated_at = excluded.updated_at`,
		graph.EventGraphID,
		graph.UserID,
		graph.Scope,
		graph.AnchorSubject,
		graph.TimeBucket,
		string(payload),
		generatedAt.UTC().Format(time.RFC3339Nano),
		updatedAt.UTC().Format(time.RFC3339Nano),
	)
	return err
}

func buildEventGraphID(userID, scope, anchorSubject, timeBucket string) string {
	parts := []string{strings.TrimSpace(userID), strings.TrimSpace(scope), normalizeCanonicalAlias(anchorSubject), strings.TrimSpace(timeBucket)}
	return strings.Join(parts, ":")
}

func getEventGraph(ctx context.Context, q interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}, eventGraphID string) (EventGraphRecord, error) {
	var payload string
	if err := q.QueryRowContext(ctx, `SELECT payload_json FROM event_graphs WHERE event_graph_id = ?`, strings.TrimSpace(eventGraphID)).Scan(&payload); err != nil {
		return EventGraphRecord{}, err
	}
	var graph EventGraphRecord
	if err := json.Unmarshal([]byte(payload), &graph); err != nil {
		return EventGraphRecord{}, fmt.Errorf("decode event graph payload: %w", err)
	}
	return graph, nil
}

func (s *SQLiteStore) eventVerificationStatusIndex(ctx context.Context, userID string) (map[string]graphmodel.VerificationStatus, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT payload_json FROM memory_content_graphs WHERE user_id = ?`, strings.TrimSpace(userID))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]graphmodel.VerificationStatus{}
	for rows.Next() {
		var payload string
		if err := rows.Scan(&payload); err != nil {
			return nil, err
		}
		var subgraph graphmodel.ContentSubgraph
		if err := json.Unmarshal([]byte(payload), &subgraph); err != nil {
			return nil, fmt.Errorf("decode memory_content_graph payload: %w", err)
		}
		for _, node := range subgraph.Nodes {
			out[node.ID] = node.VerificationStatus
		}
	}
	return out, rows.Err()
}

func summarizeVerificationStatuses(nodeIDs []string, byNodeID map[string]graphmodel.VerificationStatus) map[graphmodel.VerificationStatus]int {
	out := map[graphmodel.VerificationStatus]int{}
	for _, nodeID := range nodeIDs {
		status := byNodeID[nodeID]
		if status == "" {
			status = graphmodel.VerificationPending
		}
		out[status]++
	}
	return out
}

func (s *SQLiteStore) eventNodeChangeIndex(ctx context.Context, userID string) (map[string]string, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT payload_json FROM memory_content_graphs WHERE user_id = ?`, strings.TrimSpace(userID))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]string{}
	for rows.Next() {
		var payload string
		if err := rows.Scan(&payload); err != nil {
			return nil, err
		}
		var subgraph graphmodel.ContentSubgraph
		if err := json.Unmarshal([]byte(payload), &subgraph); err != nil {
			return nil, fmt.Errorf("decode memory_content_graph payload: %w", err)
		}
		for _, node := range subgraph.Nodes {
			out[node.ID] = strings.TrimSpace(node.ChangeText)
		}
	}
	return out, rows.Err()
}

func eventRepresentativeChanges(nodeIDs []string, changeByNodeID map[string]string) []string {
	changes := make([]string, 0, len(nodeIDs))
	for _, nodeID := range nodeIDs {
		change := strings.TrimSpace(changeByNodeID[nodeID])
		if change == "" {
			continue
		}
		changes = append(changes, change)
	}
	return uniqueStrings(changes)
}

func (s *SQLiteStore) eventNodeTraceabilityIndex(ctx context.Context, userID string) (map[string]string, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT payload_json FROM memory_content_graphs WHERE user_id = ?`, strings.TrimSpace(userID))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]string{}
	for rows.Next() {
		var payload string
		if err := rows.Scan(&payload); err != nil {
			return nil, err
		}
		var subgraph graphmodel.ContentSubgraph
		if err := json.Unmarshal([]byte(payload), &subgraph); err != nil {
			return nil, fmt.Errorf("decode memory_content_graph payload: %w", err)
		}
		for _, node := range subgraph.Nodes {
			out[node.ID] = subgraph.ID
		}
	}
	return out, rows.Err()
}

func eventTraceabilityMap(nodeIDs []string, sourceByNodeID map[string]string) map[string][]string {
	out := map[string][]string{}
	for _, nodeID := range nodeIDs {
		sourceID := strings.TrimSpace(sourceByNodeID[nodeID])
		if sourceID == "" {
			continue
		}
		out[sourceID] = append(out[sourceID], nodeID)
	}
	for sourceID, ids := range out {
		out[sourceID] = uniqueStrings(ids)
	}
	return out
}

func (s *SQLiteStore) replaceEventGraphEvidenceLinks(ctx context.Context, graph EventGraphRecord) error {
	if strings.TrimSpace(graph.EventGraphID) == "" {
		return fmt.Errorf("event graph id is required")
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `DELETE FROM event_graph_evidence_links WHERE event_graph_id = ?`, graph.EventGraphID); err != nil {
		return err
	}
	for subgraphID, nodeIDs := range graph.TraceabilityMap {
		for _, nodeID := range uniqueStrings(nodeIDs) {
			if _, err := tx.ExecContext(ctx, `INSERT INTO event_graph_evidence_links(event_graph_id, subgraph_id, node_id, created_at) VALUES (?, ?, ?, ?)`, graph.EventGraphID, subgraphID, nodeID, now); err != nil {
				return err
			}
		}
	}
	return tx.Commit()
}

func (s *SQLiteStore) ListEventGraphEvidenceLinks(ctx context.Context, eventGraphID string) ([]EventGraphEvidenceLink, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT event_graph_id, subgraph_id, node_id FROM event_graph_evidence_links WHERE event_graph_id = ? ORDER BY subgraph_id ASC, node_id ASC`, strings.TrimSpace(eventGraphID))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]EventGraphEvidenceLink, 0)
	for rows.Next() {
		var link EventGraphEvidenceLink
		if err := rows.Scan(&link.EventGraphID, &link.SubgraphID, &link.NodeID); err != nil {
			return nil, err
		}
		out = append(out, link)
	}
	return out, rows.Err()
}

func (s *SQLiteStore) ListEventGraphEvidenceLinksByUser(ctx context.Context, userID string) ([]EventGraphEvidenceLink, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT l.event_graph_id, l.subgraph_id, l.node_id
		FROM event_graph_evidence_links l
		INNER JOIN event_graphs g ON g.event_graph_id = l.event_graph_id
		WHERE g.user_id = ?
		ORDER BY l.event_graph_id ASC, l.subgraph_id ASC, l.node_id ASC`, strings.TrimSpace(userID))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]EventGraphEvidenceLink, 0)
	for rows.Next() {
		var link EventGraphEvidenceLink
		if err := rows.Scan(&link.EventGraphID, &link.SubgraphID, &link.NodeID); err != nil {
			return nil, err
		}
		out = append(out, link)
	}
	return out, rows.Err()
}
