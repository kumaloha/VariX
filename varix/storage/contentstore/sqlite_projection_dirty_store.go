package contentstore

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"
)

func (s *SQLiteStore) MarkProjectionDirty(ctx context.Context, mark ProjectionDirtyMark, at time.Time) error {
	return markProjectionDirty(ctx, s.db, mark, at)
}

func markProjectionDirty(ctx context.Context, execer projectionDirtyExecer, mark ProjectionDirtyMark, at time.Time) error {
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
	_, err = execer.ExecContext(ctx, `INSERT INTO projection_dirty_marks(user_id, layer, subject, ticker, horizon, reason, source_ref, status, dirty_at, updated_at)
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

func (s *SQLiteStore) HasProjectionDirtyMark(ctx context.Context, userID, layer, subject, horizon string) (bool, error) {
	userID = strings.TrimSpace(userID)
	layer = strings.TrimSpace(layer)
	if userID == "" || layer == "" {
		return false, nil
	}
	query := `SELECT 1 FROM projection_dirty_marks WHERE status = ? AND user_id = ? AND layer = ?`
	args := []any{ProjectionDirtyPending, userID, layer}
	if strings.TrimSpace(subject) != "" {
		query += ` AND subject = ?`
		args = append(args, normalizeDirtyDimension(subject))
	}
	if strings.TrimSpace(horizon) != "" {
		query += ` AND horizon = ?`
		args = append(args, normalizeDirtyDimension(horizon))
	}
	query += ` LIMIT 1`
	var one int
	err := s.db.QueryRowContext(ctx, query, args...).Scan(&one)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

func (s *SQLiteStore) countProjectionDirtyMarks(ctx context.Context, userID string) (int, error) {
	query := `SELECT COUNT(*) FROM projection_dirty_marks WHERE status = ?`
	args := []any{ProjectionDirtyPending}
	if strings.TrimSpace(userID) != "" {
		query += ` AND user_id = ?`
		args = append(args, strings.TrimSpace(userID))
	}
	var count int
	if err := s.db.QueryRowContext(ctx, query, args...).Scan(&count); err != nil {
		return 0, err
	}
	return count, nil
}

func (s *SQLiteStore) ClearProjectionDirtyMark(ctx context.Context, mark ProjectionDirtyMark) error {
	return s.ClearProjectionDirtyMarks(ctx, []ProjectionDirtyMark{mark})
}

func (s *SQLiteStore) ClearProjectionDirtyMarks(ctx context.Context, marks []ProjectionDirtyMark) error {
	if len(marks) == 0 {
		return nil
	}
	if len(marks) == 1 {
		return s.clearProjectionDirtyMark(ctx, marks[0])
	}
	ids := make([]int64, 0, len(marks))
	for _, mark := range marks {
		if mark.ID <= 0 {
			return s.clearProjectionDirtyMarksIndividually(ctx, marks)
		}
		ids = append(ids, mark.ID)
	}
	placeholders := strings.TrimRight(strings.Repeat("?,", len(ids)), ",")
	args := make([]any, 0, len(ids))
	for _, id := range ids {
		args = append(args, id)
	}
	result, err := s.db.ExecContext(ctx, `DELETE FROM projection_dirty_marks WHERE dirty_id IN (`+placeholders+`)`, args...)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected != int64(len(ids)) {
		return sql.ErrNoRows
	}
	return nil
}

func (s *SQLiteStore) clearProjectionDirtyMarksIndividually(ctx context.Context, marks []ProjectionDirtyMark) error {
	for _, mark := range marks {
		if err := s.clearProjectionDirtyMark(ctx, mark); err != nil {
			return err
		}
	}
	return nil
}

func (s *SQLiteStore) clearProjectionDirtyMark(ctx context.Context, mark ProjectionDirtyMark) error {
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
