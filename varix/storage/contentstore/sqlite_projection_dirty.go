package contentstore

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"
)

const ProjectionDirtyPending = "pending"

type ProjectionDirtyMark struct {
	ID        int64  `json:"dirty_id,omitempty"`
	UserID    string `json:"user_id"`
	Layer     string `json:"layer"`
	Subject   string `json:"subject,omitempty"`
	Ticker    string `json:"ticker,omitempty"`
	Horizon   string `json:"horizon,omitempty"`
	Reason    string `json:"reason,omitempty"`
	SourceRef string `json:"source_ref,omitempty"`
	Status    string `json:"status"`
	DirtyAt   string `json:"dirty_at"`
	UpdatedAt string `json:"updated_at,omitempty"`
}

func (s *SQLiteStore) MarkProjectionDirty(ctx context.Context, mark ProjectionDirtyMark, at time.Time) error {
	userID, err := normalizeRequiredUserID(mark.UserID)
	if err != nil {
		return err
	}
	layer := strings.TrimSpace(mark.Layer)
	if layer == "" {
		return fmt.Errorf("projection layer is required")
	}
	at = normalizeRecordedTime(at)
	now := currentSQLiteTimestamp()
	_, err = s.db.ExecContext(ctx, `INSERT INTO projection_dirty_marks(user_id, layer, subject, ticker, horizon, reason, source_ref, status, dirty_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(user_id, layer, subject, ticker, horizon) DO UPDATE SET
			reason = excluded.reason,
			source_ref = excluded.source_ref,
			status = excluded.status,
			dirty_at = excluded.dirty_at,
			updated_at = excluded.updated_at`,
		userID,
		layer,
		normalizeDirtyDimension(mark.Subject),
		normalizeDirtyDimension(mark.Ticker),
		normalizeDirtyDimension(mark.Horizon),
		strings.TrimSpace(mark.Reason),
		strings.TrimSpace(mark.SourceRef),
		ProjectionDirtyPending,
		at.UTC().Format(time.RFC3339Nano),
		now,
	)
	return err
}

func (s *SQLiteStore) ListProjectionDirtyMarks(ctx context.Context, userID string, limit int) ([]ProjectionDirtyMark, error) {
	userID = strings.TrimSpace(userID)
	if limit <= 0 {
		limit = 100
	}
	query := `SELECT dirty_id, user_id, layer, subject, ticker, horizon, reason, source_ref, status, dirty_at, updated_at
		FROM projection_dirty_marks WHERE status = ?`
	args := []any{ProjectionDirtyPending}
	if userID != "" {
		query += ` AND user_id = ?`
		args = append(args, userID)
	}
	query += ` ORDER BY dirty_at ASC, dirty_id ASC LIMIT ?`
	args = append(args, limit)
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]ProjectionDirtyMark, 0)
	for rows.Next() {
		var mark ProjectionDirtyMark
		if err := rows.Scan(&mark.ID, &mark.UserID, &mark.Layer, &mark.Subject, &mark.Ticker, &mark.Horizon, &mark.Reason, &mark.SourceRef, &mark.Status, &mark.DirtyAt, &mark.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, mark)
	}
	return out, rows.Err()
}

func (s *SQLiteStore) ClearProjectionDirtyMark(ctx context.Context, mark ProjectionDirtyMark) error {
	if mark.ID > 0 {
		result, err := s.db.ExecContext(ctx, `DELETE FROM projection_dirty_marks WHERE dirty_id = ?`, mark.ID)
		if err != nil {
			return err
		}
		return ensureDirtyMarkDeleted(result)
	}
	userID, err := normalizeRequiredUserID(mark.UserID)
	if err != nil {
		return err
	}
	if strings.TrimSpace(mark.Layer) == "" {
		return fmt.Errorf("projection layer is required")
	}
	result, err := s.db.ExecContext(ctx, `DELETE FROM projection_dirty_marks
		WHERE user_id = ? AND layer = ? AND subject = ? AND ticker = ? AND horizon = ?`,
		userID,
		strings.TrimSpace(mark.Layer),
		normalizeDirtyDimension(mark.Subject),
		normalizeDirtyDimension(mark.Ticker),
		normalizeDirtyDimension(mark.Horizon),
	)
	if err != nil {
		return err
	}
	return ensureDirtyMarkDeleted(result)
}

func ensureDirtyMarkDeleted(result sql.Result) error {
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func normalizeDirtyDimension(value string) string {
	return strings.Join(strings.Fields(strings.TrimSpace(value)), " ")
}
