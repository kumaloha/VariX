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

func (s *SQLiteStore) EnqueueVerifyQueueItem(ctx context.Context, item graphmodel.VerifyQueueItem) error {
	if strings.TrimSpace(item.ID) == "" {
		return fmt.Errorf("verify queue item id is required")
	}
	if item.ObjectType != graphmodel.VerifyQueueObjectNode && item.ObjectType != graphmodel.VerifyQueueObjectEdge {
		return fmt.Errorf("verify queue item object_type is unsupported")
	}
	if strings.TrimSpace(item.ObjectID) == "" {
		return fmt.Errorf("verify queue item object_id is required")
	}
	if strings.TrimSpace(item.SourceArticleID) == "" {
		return fmt.Errorf("verify queue item source_article_id is required")
	}
	if item.Status == "" {
		item.Status = graphmodel.VerifyQueueStatusQueued
	}
	if item.Status != graphmodel.VerifyQueueStatusQueued && item.Status != graphmodel.VerifyQueueStatusRetry {
		return fmt.Errorf("verify queue item status must be queued or retry on enqueue")
	}
	scheduledAt, err := time.Parse(time.RFC3339, item.ScheduledAt)
	if err != nil {
		return fmt.Errorf("verify queue item scheduled_at must be RFC3339: %w", err)
	}
	now := currentSQLiteTimestamp()
	_, err = s.db.ExecContext(ctx, `INSERT INTO verify_queue(queue_id, object_type, object_id, source_article_id, priority, scheduled_at, attempts, last_error, status, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(queue_id) DO UPDATE SET
		  object_type = excluded.object_type,
		  object_id = excluded.object_id,
		  source_article_id = excluded.source_article_id,
		  priority = excluded.priority,
		  scheduled_at = excluded.scheduled_at,
		  last_error = excluded.last_error,
		  status = excluded.status,
		  updated_at = excluded.updated_at`,
		item.ID,
		string(item.ObjectType),
		item.ObjectID,
		item.SourceArticleID,
		item.Priority,
		scheduledAt.UTC().Format(time.RFC3339Nano),
		item.Attempts,
		item.LastError,
		string(item.Status),
		now,
		now,
	)
	return err
}

func (s *SQLiteStore) ListDueVerifyQueueItems(ctx context.Context, now time.Time, limit int) ([]graphmodel.VerifyQueueItem, error) {
	now = normalizeNow(now)
	if limit <= 0 {
		limit = 100
	}
	rows, err := s.db.QueryContext(ctx, `SELECT queue_id, object_type, object_id, source_article_id, priority, scheduled_at, attempts, last_error, status
		FROM verify_queue
		WHERE status IN ('queued', 'retry') AND scheduled_at <= ?
		ORDER BY priority DESC, scheduled_at ASC, queue_id ASC
		LIMIT ?`, now.UTC().Format(time.RFC3339Nano), limit)
	if err != nil {
		return nil, err
	}
	return scanVerifyQueueItems(rows)
}

func (s *SQLiteStore) ListVerifyQueueItems(ctx context.Context, limit int) ([]graphmodel.VerifyQueueItem, error) {
	if limit <= 0 {
		limit = 100
	}
	rows, err := s.db.QueryContext(ctx, `SELECT queue_id, object_type, object_id, source_article_id, priority, scheduled_at, attempts, last_error, status
		FROM verify_queue
		ORDER BY CASE status WHEN 'queued' THEN 0 WHEN 'running' THEN 1 WHEN 'retry' THEN 2 WHEN 'done' THEN 3 ELSE 4 END ASC,
		priority DESC, scheduled_at ASC, queue_id ASC
		LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	return scanVerifyQueueItems(rows)
}

func (s *SQLiteStore) ClaimDueVerifyQueueItems(ctx context.Context, now time.Time, limit int) ([]graphmodel.VerifyQueueItem, error) {
	now = normalizeNow(now)
	if limit <= 0 {
		limit = 100
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	rows, err := tx.QueryContext(ctx, `SELECT queue_id, object_type, object_id, source_article_id, priority, scheduled_at, attempts, last_error, status
		FROM verify_queue
		WHERE status IN ('queued', 'retry') AND scheduled_at <= ?
		ORDER BY priority DESC, scheduled_at ASC, queue_id ASC
		LIMIT ?`, now.UTC().Format(time.RFC3339Nano), limit)
	if err != nil {
		return nil, err
	}
	items, err := scanVerifyQueueItems(rows)
	if err != nil {
		return nil, err
	}
	for i := range items {
		if _, err := tx.ExecContext(ctx, `UPDATE verify_queue SET status = ?, attempts = attempts + 1, updated_at = ? WHERE queue_id = ?`, string(graphmodel.VerifyQueueStatusRunning), now.UTC().Format(time.RFC3339Nano), items[i].ID); err != nil {
			return nil, err
		}
		items[i].Status = graphmodel.VerifyQueueStatusRunning
		items[i].Attempts++
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return items, nil
}

func (s *SQLiteStore) RunVerifyQueueSweep(ctx context.Context, now time.Time, limit int, evaluator func(graphmodel.VerifyQueueItem) (graphmodel.VerifyVerdict, error)) (VerifyQueueSweepResult, error) {
	if evaluator == nil {
		return VerifyQueueSweepResult{}, fmt.Errorf("verify queue evaluator is required")
	}
	claimed, err := s.ClaimDueVerifyQueueItems(ctx, now, limit)
	if err != nil {
		return VerifyQueueSweepResult{}, err
	}
	result := VerifyQueueSweepResult{Claimed: len(claimed)}
	for _, item := range claimed {
		verdict, err := evaluator(item)
		if err != nil {
			result.Failed++
			if retryErr := s.RetryVerifyQueueItem(ctx, item.ID, verifyQueueRetryAt(now, item.Attempts), err.Error(), now); retryErr != nil {
				return result, retryErr
			}
			continue
		}
		if verdict.Verdict == graphmodel.VerificationPending {
			result.Retried++
			nextAt := verifyQueueRetryAt(now, item.Attempts)
			if strings.TrimSpace(verdict.NextVerifyAt) != "" {
				parsed, parseErr := time.Parse(time.RFC3339, verdict.NextVerifyAt)
				if parseErr != nil {
					return result, parseErr
				}
				nextAt = parsed
			}
			if _, lookupErr := s.GetContentSubgraphByArticleID(ctx, item.SourceArticleID); lookupErr == nil {
				if applyErr := s.ApplyVerifyVerdictToContentSubgraphByArticleID(ctx, item.SourceArticleID, verdict); applyErr != nil {
					return result, applyErr
				}
			}
			if err := s.RetryVerifyQueueItem(ctx, item.ID, nextAt, verdict.Reason, now); err != nil {
				return result, err
			}
			continue
		}
		result.Finished++
		if err := s.FinishVerifyQueueItem(ctx, item.ID, verdict, now); err != nil {
			return result, err
		}
		if _, lookupErr := s.GetContentSubgraphByArticleID(ctx, item.SourceArticleID); lookupErr == nil {
			if applyErr := s.ApplyVerifyVerdictToContentSubgraphByArticleID(ctx, item.SourceArticleID, verdict); applyErr != nil {
				return result, applyErr
			}
		}
	}
	return result, nil
}

func verifyQueueRetryAt(now time.Time, attempts int) time.Time {
	now = normalizeNow(now)
	return now.Add(verifyQueueRetryDelay(attempts))
}

func verifyQueueRetryDelay(attempts int) time.Duration {
	switch {
	case attempts <= 1:
		return time.Minute
	case attempts == 2:
		return 5 * time.Minute
	case attempts == 3:
		return 15 * time.Minute
	default:
		return time.Hour
	}
}

func (s *SQLiteStore) MarkVerifyQueueItemRunning(ctx context.Context, queueID string, now time.Time) error {
	now = normalizeNow(now)
	_, err := s.db.ExecContext(ctx, `UPDATE verify_queue
		SET status = ?, attempts = attempts + 1, updated_at = ?
		WHERE queue_id = ?`, string(graphmodel.VerifyQueueStatusRunning), now.UTC().Format(time.RFC3339Nano), strings.TrimSpace(queueID))
	return err
}

func (s *SQLiteStore) FinishVerifyQueueItem(ctx context.Context, queueID string, verdict graphmodel.VerifyVerdict, now time.Time) error {
	if strings.TrimSpace(queueID) == "" {
		return fmt.Errorf("verify queue queueID is required")
	}
	if verdict.ObjectType != graphmodel.VerifyQueueObjectNode && verdict.ObjectType != graphmodel.VerifyQueueObjectEdge {
		return fmt.Errorf("verify verdict object_type is unsupported")
	}
	if verdict.Verdict != graphmodel.VerificationPending && verdict.Verdict != graphmodel.VerificationProved && verdict.Verdict != graphmodel.VerificationDisproved && verdict.Verdict != graphmodel.VerificationUnverifiable {
		return fmt.Errorf("verify verdict status is unsupported")
	}
	now = normalizeNow(now)
	asOf, err := time.Parse(time.RFC3339, verdict.AsOf)
	if err != nil {
		return fmt.Errorf("verify verdict as_of must be RFC3339: %w", err)
	}
	evidenceJSON, err := json.Marshal(verdict.EvidenceRefs)
	if err != nil {
		return err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `INSERT INTO verify_verdict_history(object_type, object_id, verdict, reason, evidence_refs_json, as_of, next_verify_at, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		string(verdict.ObjectType), verdict.ObjectID, string(verdict.Verdict), nullIfBlank(verdict.Reason), string(evidenceJSON), asOf.UTC().Format(time.RFC3339Nano), nullIfBlank(verdict.NextVerifyAt), now.UTC().Format(time.RFC3339Nano)); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE verify_queue SET status = ?, updated_at = ?, last_error = '' WHERE queue_id = ?`, string(graphmodel.VerifyQueueStatusDone), now.UTC().Format(time.RFC3339Nano), strings.TrimSpace(queueID)); err != nil {
		return err
	}
	return tx.Commit()
}

type VerifyQueueSweepResult struct {
	Claimed  int `json:"claimed"`
	Finished int `json:"finished"`
	Retried  int `json:"retried"`
	Failed   int `json:"failed"`
}

type VerifyQueueSummaryDetailed struct {
	Counts            map[graphmodel.VerifyQueueStatus]int     `json:"counts"`
	ObjectTypes       map[graphmodel.VerifyQueueObjectType]int `json:"object_types"`
	TotalCount        int                                      `json:"total_count"`
	DueCount          int                                      `json:"due_count"`
	OldestScheduledAt string                                   `json:"oldest_scheduled_at,omitempty"`
	PendingAgeBuckets map[string]int                           `json:"pending_age_buckets"`
}

func (s *SQLiteStore) RetryVerifyQueueItem(ctx context.Context, queueID string, nextAt time.Time, lastError string, now time.Time) error {
	if nextAt.IsZero() {
		return fmt.Errorf("verify queue retry nextAt is required")
	}
	now = normalizeNow(now)
	_, err := s.db.ExecContext(ctx, `UPDATE verify_queue
		SET status = ?, scheduled_at = ?, last_error = ?, updated_at = ?
		WHERE queue_id = ?`, string(graphmodel.VerifyQueueStatusRetry), nextAt.UTC().Format(time.RFC3339Nano), strings.TrimSpace(lastError), now.UTC().Format(time.RFC3339Nano), strings.TrimSpace(queueID))
	return err
}

func getVerifyQueueItem(ctx context.Context, q interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}, queueID string) (graphmodel.VerifyQueueItem, error) {
	var item graphmodel.VerifyQueueItem
	var objectType, status, scheduledAt string
	if err := q.QueryRowContext(ctx, `SELECT queue_id, object_type, object_id, source_article_id, priority, scheduled_at, attempts, last_error, status FROM verify_queue WHERE queue_id = ?`, strings.TrimSpace(queueID)).Scan(&item.ID, &objectType, &item.ObjectID, &item.SourceArticleID, &item.Priority, &scheduledAt, &item.Attempts, &item.LastError, &status); err != nil {
		return graphmodel.VerifyQueueItem{}, err
	}
	item.ObjectType = graphmodel.VerifyQueueObjectType(objectType)
	item.Status = graphmodel.VerifyQueueStatus(status)
	item.ScheduledAt = parseSQLiteTime(scheduledAt).UTC().Format(time.RFC3339)
	return item, nil
}

func (s *SQLiteStore) ListVerifyQueueItemsByStatus(ctx context.Context, status string, limit int) ([]graphmodel.VerifyQueueItem, error) {
	status = strings.TrimSpace(status)
	if status == "" {
		return s.ListVerifyQueueItems(ctx, limit)
	}
	if limit <= 0 {
		limit = 100
	}
	rows, err := s.db.QueryContext(ctx, `SELECT queue_id, object_type, object_id, source_article_id, priority, scheduled_at, attempts, last_error, status
		FROM verify_queue
		WHERE status = ?
		ORDER BY priority DESC, scheduled_at ASC, queue_id ASC
		LIMIT ?`, status, limit)
	if err != nil {
		return nil, err
	}
	return scanVerifyQueueItems(rows)
}

func scanVerifyQueueItems(rows *sql.Rows) ([]graphmodel.VerifyQueueItem, error) {
	defer rows.Close()
	items := make([]graphmodel.VerifyQueueItem, 0)
	for rows.Next() {
		item, err := scanVerifyQueueItem(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func scanVerifyQueueItem(rows *sql.Rows) (graphmodel.VerifyQueueItem, error) {
	var item graphmodel.VerifyQueueItem
	var objectType, status, scheduledAt string
	if err := rows.Scan(&item.ID, &objectType, &item.ObjectID, &item.SourceArticleID, &item.Priority, &scheduledAt, &item.Attempts, &item.LastError, &status); err != nil {
		return graphmodel.VerifyQueueItem{}, err
	}
	item.ObjectType = graphmodel.VerifyQueueObjectType(objectType)
	item.Status = graphmodel.VerifyQueueStatus(status)
	item.ScheduledAt = parseSQLiteTime(scheduledAt).UTC().Format(time.RFC3339)
	return item, nil
}

func (s *SQLiteStore) GetVerifyQueueSummary(ctx context.Context) (map[graphmodel.VerifyQueueStatus]int, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT status, COUNT(*) FROM verify_queue GROUP BY status`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[graphmodel.VerifyQueueStatus]int{}
	for rows.Next() {
		var status string
		var count int
		if err := rows.Scan(&status, &count); err != nil {
			return nil, err
		}
		out[graphmodel.VerifyQueueStatus(status)] = count
	}
	return out, rows.Err()
}

func (s *SQLiteStore) GetVerifyQueueSummaryDetailed(ctx context.Context, now time.Time) (VerifyQueueSummaryDetailed, error) {
	now = normalizeNow(now)
	counts, err := s.GetVerifyQueueSummary(ctx)
	if err != nil {
		return VerifyQueueSummaryDetailed{}, err
	}
	var dueCount int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM verify_queue WHERE status IN ('queued','retry') AND scheduled_at <= ?`, now.UTC().Format(time.RFC3339Nano)).Scan(&dueCount); err != nil {
		return VerifyQueueSummaryDetailed{}, err
	}
	var oldest sql.NullString
	if err := s.db.QueryRowContext(ctx, `SELECT scheduled_at FROM verify_queue ORDER BY scheduled_at ASC LIMIT 1`).Scan(&oldest); err != nil && err != sql.ErrNoRows {
		return VerifyQueueSummaryDetailed{}, err
	}
	typeRows, err := s.db.QueryContext(ctx, `SELECT object_type, COUNT(*) FROM verify_queue GROUP BY object_type`)
	if err != nil {
		return VerifyQueueSummaryDetailed{}, err
	}
	defer typeRows.Close()
	objectTypes := map[graphmodel.VerifyQueueObjectType]int{}
	for typeRows.Next() {
		var objectType string
		var count int
		if err := typeRows.Scan(&objectType, &count); err != nil {
			return VerifyQueueSummaryDetailed{}, err
		}
		objectTypes[graphmodel.VerifyQueueObjectType(objectType)] = count
	}
	if err := typeRows.Err(); err != nil {
		return VerifyQueueSummaryDetailed{}, err
	}
	ageRows, err := s.db.QueryContext(ctx, `SELECT scheduled_at FROM verify_queue WHERE status IN ('queued','retry')`)
	if err != nil {
		return VerifyQueueSummaryDetailed{}, err
	}
	defer ageRows.Close()
	pendingAgeBuckets := map[string]int{}
	for ageRows.Next() {
		var scheduledAt string
		if err := ageRows.Scan(&scheduledAt); err != nil {
			return VerifyQueueSummaryDetailed{}, err
		}
		parsed := parseSQLiteTime(scheduledAt)
		switch age := now.Sub(parsed); {
		case age < 0:
			pendingAgeBuckets["future"]++
		case age < time.Hour:
			pendingAgeBuckets["overdue_lt_1h"]++
		case age <= 24*time.Hour:
			pendingAgeBuckets["overdue_1h_to_24h"]++
		default:
			pendingAgeBuckets["overdue_gt_24h"]++
		}
	}
	if err := ageRows.Err(); err != nil {
		return VerifyQueueSummaryDetailed{}, err
	}
	if pendingAgeBuckets == nil {
		pendingAgeBuckets = map[string]int{}
	}
	totalCount := 0
	for _, count := range counts {
		totalCount += count
	}
	out := VerifyQueueSummaryDetailed{Counts: counts, ObjectTypes: objectTypes, TotalCount: totalCount, DueCount: dueCount, PendingAgeBuckets: pendingAgeBuckets}
	if oldest.Valid {
		out.OldestScheduledAt = parseSQLiteTime(oldest.String).UTC().Format(time.RFC3339)
	}
	return out, nil
}
