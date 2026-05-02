package contentstore

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/kumaloha/VariX/varix/model"
)

func replaceMemoryContentGraphSubjectsTx(ctx context.Context, tx *sql.Tx, userID string, subgraph model.ContentSubgraph, updatedAt time.Time) error {
	userID = strings.TrimSpace(userID)
	sourcePlatform := strings.TrimSpace(subgraph.SourcePlatform)
	sourceExternalID := strings.TrimSpace(subgraph.SourceExternalID)
	if _, err := tx.ExecContext(ctx, `DELETE FROM memory_content_graph_subjects WHERE user_id = ? AND source_platform = ? AND source_external_id = ?`, userID, sourcePlatform, sourceExternalID); err != nil {
		return fmt.Errorf("clear memory content graph subjects: %w", err)
	}
	subjects := memoryContentGraphSubjectCounts(subgraph)
	if len(subjects) == 0 {
		return nil
	}
	updated := normalizeRecordedTime(updatedAt).UTC().Format(time.RFC3339Nano)
	for subject, count := range subjects {
		if _, err := tx.ExecContext(ctx, `INSERT INTO memory_content_graph_subjects(user_id, subject, source_platform, source_external_id, node_count, updated_at)
			VALUES (?, ?, ?, ?, ?, ?)
			ON CONFLICT(user_id, subject, source_platform, source_external_id) DO UPDATE SET
				node_count = excluded.node_count,
				updated_at = excluded.updated_at`,
			userID, subject, sourcePlatform, sourceExternalID, count, updated); err != nil {
			return fmt.Errorf("upsert memory content graph subject: %w", err)
		}
	}
	return nil
}

func memoryContentGraphSubjectCounts(subgraph model.ContentSubgraph) map[string]int {
	out := map[string]int{}
	for _, node := range subgraph.Nodes {
		for _, subject := range []string{node.SubjectCanonical, node.SubjectText} {
			subject = normalizeDirtyDimension(subject)
			if subject == "" {
				continue
			}
			out[subject]++
		}
	}
	return out
}

func (s *SQLiteStore) backfillMemoryContentGraphSubjects() error {
	rows, err := s.db.Query(`SELECT g.user_id, g.payload_json, g.accepted_at
		FROM memory_content_graphs g
		WHERE NOT EXISTS (
			SELECT 1 FROM memory_content_graph_subjects s
			WHERE s.user_id = g.user_id
				AND s.source_platform = g.source_platform
				AND s.source_external_id = g.source_external_id
			LIMIT 1
		)
		ORDER BY g.user_id ASC, g.source_platform ASC, g.source_external_id ASC`)
	if err != nil {
		return err
	}
	defer rows.Close()
	type backfillItem struct {
		userID     string
		payload    string
		acceptedAt string
	}
	items := make([]backfillItem, 0)
	for rows.Next() {
		var item backfillItem
		if err := rows.Scan(&item.userID, &item.payload, &item.acceptedAt); err != nil {
			return err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if len(items) == 0 {
		return nil
	}
	tx, err := s.db.BeginTx(context.Background(), nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, item := range items {
		var subgraph model.ContentSubgraph
		if err := json.Unmarshal([]byte(item.payload), &subgraph); err != nil {
			return fmt.Errorf("decode memory content graph for subject backfill: %w", err)
		}
		acceptedAt := parseSQLiteTime(item.acceptedAt)
		if err := replaceMemoryContentGraphSubjectsTx(context.Background(), tx, item.userID, subgraph, acceptedAt); err != nil {
			return err
		}
	}
	return tx.Commit()
}
