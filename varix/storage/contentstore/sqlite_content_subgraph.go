package contentstore

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/kumaloha/VariX/varix/graphmodel"
)

func (s *SQLiteStore) UpsertContentSubgraph(ctx context.Context, subgraph graphmodel.ContentSubgraph) error {
	if err := subgraph.Validate(); err != nil {
		return err
	}
	payload, err := json.Marshal(subgraph)
	if err != nil {
		return err
	}
	compiledAt, err := time.Parse(time.RFC3339, subgraph.CompiledAt)
	if err != nil {
		return err
	}
	updatedAt, err := time.Parse(time.RFC3339, subgraph.UpdatedAt)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(
		ctx,
		`INSERT INTO content_subgraphs(subgraph_id, platform, external_id, root_external_id, compile_version, payload_json, compiled_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(platform, external_id) DO UPDATE SET
		   subgraph_id = excluded.subgraph_id,
		   root_external_id = excluded.root_external_id,
		   compile_version = excluded.compile_version,
		   payload_json = excluded.payload_json,
		   compiled_at = excluded.compiled_at,
		   updated_at = excluded.updated_at`,
		subgraph.ID,
		subgraph.SourcePlatform,
		subgraph.SourceExternalID,
		subgraph.RootExternalID,
		subgraph.CompileVersion,
		string(payload),
		compiledAt.UTC().Format(time.RFC3339Nano),
		updatedAt.UTC().Format(time.RFC3339Nano),
	)
	if err != nil {
		return err
	}
	return s.enqueuePendingVerifyItemsFromSubgraph(ctx, subgraph)
}

func (s *SQLiteStore) GetContentSubgraph(ctx context.Context, platform, externalID string) (graphmodel.ContentSubgraph, error) {
	return getContentSubgraph(ctx, s.db, platform, externalID)
}

func getContentSubgraph(ctx context.Context, q interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}, platform, externalID string) (graphmodel.ContentSubgraph, error) {
	var payload string
	if err := q.QueryRowContext(ctx, `SELECT payload_json FROM content_subgraphs WHERE platform = ? AND external_id = ?`, platform, externalID).Scan(&payload); err != nil {
		return graphmodel.ContentSubgraph{}, err
	}
	var subgraph graphmodel.ContentSubgraph
	if err := json.Unmarshal([]byte(payload), &subgraph); err != nil {
		return graphmodel.ContentSubgraph{}, fmt.Errorf("decode content subgraph: %w", err)
	}
	return subgraph, nil
}

func (s *SQLiteStore) enqueuePendingVerifyItemsFromSubgraph(ctx context.Context, subgraph graphmodel.ContentSubgraph) error {
	baseSchedule := strings.TrimSpace(subgraph.CompiledAt)
	if baseSchedule == "" {
		baseSchedule = time.Now().UTC().Format(time.RFC3339)
	}
	for _, node := range subgraph.Nodes {
		if node.VerificationStatus != graphmodel.VerificationPending {
			continue
		}
		scheduledAt := firstNonEmpty(strings.TrimSpace(node.NextVerifyAt), baseSchedule)
		if err := s.EnqueueVerifyQueueItem(ctx, graphmodel.VerifyQueueItem{
			ID:              verifyQueueID(subgraph.ID, "node", node.ID),
			ObjectType:      graphmodel.VerifyQueueObjectNode,
			ObjectID:        node.ID,
			SourceArticleID: subgraph.ArticleID,
			Priority:        verifyPriorityForNode(node),
			ScheduledAt:     scheduledAt,
			Status:          graphmodel.VerifyQueueStatusQueued,
		}); err != nil {
			return err
		}
	}
	for _, edge := range subgraph.Edges {
		if edge.VerificationStatus != graphmodel.VerificationPending {
			continue
		}
		scheduledAt := firstNonEmpty(strings.TrimSpace(edge.NextVerifyAt), baseSchedule)
		if err := s.EnqueueVerifyQueueItem(ctx, graphmodel.VerifyQueueItem{
			ID:              verifyQueueID(subgraph.ID, "edge", edge.ID),
			ObjectType:      graphmodel.VerifyQueueObjectEdge,
			ObjectID:        edge.ID,
			SourceArticleID: subgraph.ArticleID,
			Priority:        verifyPriorityForEdge(edge),
			ScheduledAt:     scheduledAt,
			Status:          graphmodel.VerifyQueueStatusQueued,
		}); err != nil {
			return err
		}
	}
	return nil
}

func verifyQueueID(subgraphID, objectType, objectID string) string {
	return strings.TrimSpace(subgraphID) + ":" + strings.TrimSpace(objectType) + ":" + strings.TrimSpace(objectID)
}

func verifyPriorityForNode(node graphmodel.GraphNode) int {
	if node.Kind == graphmodel.NodeKindPrediction {
		return 20
	}
	return 10
}

func verifyPriorityForEdge(edge graphmodel.GraphEdge) int {
	if edge.Type == graphmodel.EdgeTypeDrives {
		return 15
	}
	return 5
}

func (s *SQLiteStore) ApplyVerifyVerdictToContentSubgraph(ctx context.Context, platform, externalID string, verdict graphmodel.VerifyVerdict) error {
	subgraph, err := s.GetContentSubgraph(ctx, platform, externalID)
	if err != nil {
		return err
	}
	switch verdict.ObjectType {
	case graphmodel.VerifyQueueObjectNode:
		updated := false
		for i := range subgraph.Nodes {
			if subgraph.Nodes[i].ID != verdict.ObjectID {
				continue
			}
			subgraph.Nodes[i].VerificationStatus = verdict.Verdict
			subgraph.Nodes[i].VerificationReason = strings.TrimSpace(verdict.Reason)
			subgraph.Nodes[i].VerificationAsOf = strings.TrimSpace(verdict.AsOf)
			if strings.TrimSpace(verdict.NextVerifyAt) != "" {
				subgraph.Nodes[i].NextVerifyAt = strings.TrimSpace(verdict.NextVerifyAt)
			} else {
				subgraph.Nodes[i].NextVerifyAt = ""
			}
			updated = true
			break
		}
		if !updated {
			return fmt.Errorf("verify verdict node %q not found in content subgraph", verdict.ObjectID)
		}
	case graphmodel.VerifyQueueObjectEdge:
		updated := false
		for i := range subgraph.Edges {
			if subgraph.Edges[i].ID != verdict.ObjectID {
				continue
			}
			subgraph.Edges[i].VerificationStatus = verdict.Verdict
			subgraph.Edges[i].VerificationReason = strings.TrimSpace(verdict.Reason)
			subgraph.Edges[i].VerificationAsOf = strings.TrimSpace(verdict.AsOf)
			if strings.TrimSpace(verdict.NextVerifyAt) != "" {
				subgraph.Edges[i].NextVerifyAt = strings.TrimSpace(verdict.NextVerifyAt)
			} else {
				subgraph.Edges[i].NextVerifyAt = ""
			}
			updated = true
			break
		}
		if !updated {
			return fmt.Errorf("verify verdict edge %q not found in content subgraph", verdict.ObjectID)
		}
	default:
		return fmt.Errorf("verify verdict object_type %q is unsupported", verdict.ObjectType)
	}
	now := time.Now().UTC().Format(time.RFC3339)
	subgraph.UpdatedAt = now
	if err := s.UpsertContentSubgraph(ctx, subgraph); err != nil {
		return err
	}
	if err := s.refreshMemoryContentGraphsFromSubgraph(ctx, subgraph); err != nil {
		return err
	}
	if err := s.refreshEventGraphsForSubgraph(ctx, subgraph); err != nil {
		return err
	}
	return s.refreshParadigmsForSubgraph(ctx, subgraph)
}

func (s *SQLiteStore) refreshMemoryContentGraphsFromSubgraph(ctx context.Context, subgraph graphmodel.ContentSubgraph) error {
	payload, err := json.Marshal(subgraph)
	if err != nil {
		return err
	}
	updatedAt, err := time.Parse(time.RFC3339, subgraph.UpdatedAt)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `UPDATE memory_content_graphs
		SET payload_json = ?, subgraph_id = ?, updated_at = ?
		WHERE source_platform = ? AND source_external_id = ?`,
		string(payload),
		subgraph.ID,
		updatedAt.UTC().Format(time.RFC3339Nano),
		subgraph.SourcePlatform,
		subgraph.SourceExternalID,
	)
	return err
}

func (s *SQLiteStore) refreshEventGraphsForSubgraph(ctx context.Context, subgraph graphmodel.ContentSubgraph) error {
	return s.refreshProjectionForSubgraphUsers(ctx, subgraph, func(userID string, now time.Time) error {
		_, err := s.RunEventGraphProjection(ctx, userID, now)
		return err
	})
}

func (s *SQLiteStore) refreshParadigmsForSubgraph(ctx context.Context, subgraph graphmodel.ContentSubgraph) error {
	return s.refreshProjectionForSubgraphUsers(ctx, subgraph, func(userID string, now time.Time) error {
		_, err := s.RunParadigmProjection(ctx, userID, now)
		return err
	})
}

func (s *SQLiteStore) refreshProjectionForSubgraphUsers(ctx context.Context, subgraph graphmodel.ContentSubgraph, run func(string, time.Time) error) error {
	userIDs, err := s.userIDsForMemoryContentGraphSource(ctx, subgraph.SourcePlatform, subgraph.SourceExternalID)
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	for _, userID := range userIDs {
		if err := run(userID, now); err != nil {
			return err
		}
	}
	return nil
}

func (s *SQLiteStore) userIDsForMemoryContentGraphSource(ctx context.Context, sourcePlatform, sourceExternalID string) ([]string, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT DISTINCT user_id FROM memory_content_graphs WHERE source_platform = ? AND source_external_id = ?`, sourcePlatform, sourceExternalID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	userIDs := make([]string, 0)
	for rows.Next() {
		var userID string
		if err := rows.Scan(&userID); err != nil {
			return nil, err
		}
		userID = strings.TrimSpace(userID)
		if userID != "" {
			userIDs = append(userIDs, userID)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return userIDs, nil
}

func (s *SQLiteStore) GetContentSubgraphByArticleID(ctx context.Context, articleID string) (graphmodel.ContentSubgraph, error) {
	return getContentSubgraphByArticleID(ctx, s.db, articleID)
}

func getContentSubgraphByArticleID(ctx context.Context, q interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}, articleID string) (graphmodel.ContentSubgraph, error) {
	var payload string
	if err := q.QueryRowContext(ctx, `SELECT payload_json FROM content_subgraphs WHERE subgraph_id = ?`, strings.TrimSpace(articleID)).Scan(&payload); err != nil {
		return graphmodel.ContentSubgraph{}, err
	}
	var subgraph graphmodel.ContentSubgraph
	if err := json.Unmarshal([]byte(payload), &subgraph); err != nil {
		return graphmodel.ContentSubgraph{}, fmt.Errorf("decode content subgraph by article id: %w", err)
	}
	return subgraph, nil
}

func (s *SQLiteStore) ApplyVerifyVerdictToContentSubgraphByArticleID(ctx context.Context, articleID string, verdict graphmodel.VerifyVerdict) error {
	subgraph, err := s.GetContentSubgraphByArticleID(ctx, articleID)
	if err != nil {
		return err
	}
	return s.ApplyVerifyVerdictToContentSubgraph(ctx, subgraph.SourcePlatform, subgraph.SourceExternalID, verdict)
}
