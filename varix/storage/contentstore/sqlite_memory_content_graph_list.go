package contentstore

import (
	"context"
	"strings"

	"github.com/kumaloha/VariX/varix/model"
)

func (s *SQLiteStore) ListMemoryContentGraphs(ctx context.Context, userID string) ([]model.ContentSubgraph, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT payload_json FROM memory_content_graphs WHERE user_id = ? ORDER BY source_platform ASC, source_external_id ASC`, strings.TrimSpace(userID))
	if err != nil {
		return nil, err
	}
	return decodePayloadRows[model.ContentSubgraph](rows, "memory content graph")
}

func (s *SQLiteStore) ListMemoryContentGraphsBySource(ctx context.Context, userID, sourcePlatform, sourceExternalID string) ([]model.ContentSubgraph, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT payload_json FROM memory_content_graphs WHERE user_id = ? AND source_platform = ? AND source_external_id = ? ORDER BY source_platform ASC, source_external_id ASC`, strings.TrimSpace(userID), strings.TrimSpace(sourcePlatform), strings.TrimSpace(sourceExternalID))
	if err != nil {
		return nil, err
	}
	return decodePayloadRows[model.ContentSubgraph](rows, "memory content graph")
}

func (s *SQLiteStore) ListMemoryContentGraphsBySubject(ctx context.Context, userID, subject string) ([]model.ContentSubgraph, error) {
	subject, err := s.resolveCanonicalListSubject(ctx, subject)
	if err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx, `SELECT g.payload_json
		FROM memory_content_graph_subjects s
		JOIN memory_content_graphs g
			ON g.user_id = s.user_id
			AND g.source_platform = s.source_platform
			AND g.source_external_id = s.source_external_id
		WHERE s.user_id = ? AND s.subject = ?
		ORDER BY g.source_platform ASC, g.source_external_id ASC`, strings.TrimSpace(userID), strings.TrimSpace(subject))
	if err != nil {
		return nil, err
	}
	return decodePayloadRows[model.ContentSubgraph](rows, "memory content graph")
}

func (s *SQLiteStore) ListMemoryContentGraphsBySourceAndSubject(ctx context.Context, userID, sourcePlatform, sourceExternalID, subject string) ([]model.ContentSubgraph, error) {
	subject, err := s.resolveCanonicalListSubject(ctx, subject)
	if err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx, `SELECT g.payload_json
		FROM memory_content_graph_subjects s
		JOIN memory_content_graphs g
			ON g.user_id = s.user_id
			AND g.source_platform = s.source_platform
			AND g.source_external_id = s.source_external_id
		WHERE s.user_id = ? AND s.source_platform = ? AND s.source_external_id = ? AND s.subject = ?
		ORDER BY g.source_platform ASC, g.source_external_id ASC`,
		strings.TrimSpace(userID), strings.TrimSpace(sourcePlatform), strings.TrimSpace(sourceExternalID), strings.TrimSpace(subject))
	if err != nil {
		return nil, err
	}
	return decodePayloadRows[model.ContentSubgraph](rows, "memory content graph")
}
