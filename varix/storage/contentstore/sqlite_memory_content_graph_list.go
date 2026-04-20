package contentstore

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/kumaloha/VariX/varix/graphmodel"
)

func (s *SQLiteStore) ListMemoryContentGraphs(ctx context.Context, userID string) ([]graphmodel.ContentSubgraph, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT payload_json FROM memory_content_graphs WHERE user_id = ? ORDER BY source_platform ASC, source_external_id ASC`, strings.TrimSpace(userID))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]graphmodel.ContentSubgraph, 0)
	for rows.Next() {
		var payload string
		if err := rows.Scan(&payload); err != nil {
			return nil, err
		}
		var subgraph graphmodel.ContentSubgraph
		if err := json.Unmarshal([]byte(payload), &subgraph); err != nil {
			return nil, fmt.Errorf("decode memory content graph payload: %w", err)
		}
		out = append(out, subgraph)
	}
	return out, rows.Err()
}

func (s *SQLiteStore) ListMemoryContentGraphsBySource(ctx context.Context, userID, sourcePlatform, sourceExternalID string) ([]graphmodel.ContentSubgraph, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT payload_json FROM memory_content_graphs WHERE user_id = ? AND source_platform = ? AND source_external_id = ? ORDER BY source_platform ASC, source_external_id ASC`, strings.TrimSpace(userID), strings.TrimSpace(sourcePlatform), strings.TrimSpace(sourceExternalID))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]graphmodel.ContentSubgraph, 0)
	for rows.Next() {
		var payload string
		if err := rows.Scan(&payload); err != nil {
			return nil, err
		}
		var subgraph graphmodel.ContentSubgraph
		if err := json.Unmarshal([]byte(payload), &subgraph); err != nil {
			return nil, fmt.Errorf("decode memory content graph payload: %w", err)
		}
		out = append(out, subgraph)
	}
	return out, rows.Err()
}

func (s *SQLiteStore) ListMemoryContentGraphsBySubject(ctx context.Context, userID, subject string) ([]graphmodel.ContentSubgraph, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT payload_json FROM memory_content_graphs WHERE user_id = ? ORDER BY source_platform ASC, source_external_id ASC`, strings.TrimSpace(userID))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]graphmodel.ContentSubgraph, 0)
	for rows.Next() {
		var payload string
		if err := rows.Scan(&payload); err != nil {
			return nil, err
		}
		var subgraph graphmodel.ContentSubgraph
		if err := json.Unmarshal([]byte(payload), &subgraph); err != nil {
			return nil, fmt.Errorf("decode memory content graph payload: %w", err)
		}
		matched := false
		for _, node := range subgraph.Nodes {
			if node.SubjectText == strings.TrimSpace(subject) || node.SubjectCanonical == strings.TrimSpace(subject) {
				matched = true
				break
			}
		}
		if matched {
			out = append(out, subgraph)
		}
	}
	return out, rows.Err()
}
