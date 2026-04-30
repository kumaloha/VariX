package contentstore

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
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

type ProjectionDirtySweepResult struct {
	UserID    string                      `json:"user_id,omitempty"`
	Limit     int                         `json:"limit"`
	Scanned   int                         `json:"scanned"`
	Completed int                         `json:"completed"`
	Failed    int                         `json:"failed"`
	Remaining int                         `json:"remaining"`
	Layers    map[string]int              `json:"layers,omitempty"`
	Errors    []ProjectionDirtySweepError `json:"errors,omitempty"`
}

type ProjectionDirtySweepError struct {
	DirtyID int64  `json:"dirty_id,omitempty"`
	Layer   string `json:"layer"`
	Subject string `json:"subject,omitempty"`
	Horizon string `json:"horizon,omitempty"`
	Error   string `json:"error"`
}

type projectionDirtyExecer interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
}

type projectionDirtyUserState struct {
	eventRefreshed    bool
	paradigmRefreshed bool
}

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

func (s *SQLiteStore) RunProjectionDirtySweep(ctx context.Context, userID string, limit int, now time.Time) (ProjectionDirtySweepResult, error) {
	userID = strings.TrimSpace(userID)
	if limit <= 0 {
		limit = 100
	}
	now = normalizeRecordedTime(now)
	result := ProjectionDirtySweepResult{
		UserID: userID,
		Limit:  limit,
		Layers: map[string]int{},
	}
	marks, err := s.ListProjectionDirtyMarks(ctx, userID, limit)
	if err != nil {
		return result, err
	}
	result.Scanned = len(marks)
	states := map[string]*projectionDirtyUserState{}
	for _, mark := range orderProjectionDirtyMarksForSweep(marks) {
		state := projectionDirtyStateForMark(states, mark)
		if err := s.runProjectionDirtyMark(ctx, mark, now, state); err != nil {
			result.Failed++
			result.Errors = append(result.Errors, projectionDirtySweepError(mark, err))
			continue
		}
		if err := s.ClearProjectionDirtyMark(ctx, mark); err != nil {
			result.Failed++
			result.Errors = append(result.Errors, projectionDirtySweepError(mark, err))
			continue
		}
		result.Completed++
		result.Layers[strings.TrimSpace(mark.Layer)]++
	}
	remaining, err := s.countProjectionDirtyMarks(ctx, userID)
	if err != nil {
		return result, err
	}
	result.Remaining = remaining
	if result.Completed == 0 {
		result.Layers = nil
	}
	if result.Failed > 0 {
		return result, fmt.Errorf("projection dirty sweep failed for %d mark(s)", result.Failed)
	}
	return result, nil
}

func projectionDirtyStateForMark(states map[string]*projectionDirtyUserState, mark ProjectionDirtyMark) *projectionDirtyUserState {
	userID := strings.TrimSpace(mark.UserID)
	state := states[userID]
	if state == nil {
		state = &projectionDirtyUserState{}
		states[userID] = state
	}
	return state
}

func orderProjectionDirtyMarksForSweep(marks []ProjectionDirtyMark) []ProjectionDirtyMark {
	out := append([]ProjectionDirtyMark(nil), marks...)
	sort.SliceStable(out, func(i, j int) bool {
		return projectionDirtyLayerPriority(out[i].Layer) < projectionDirtyLayerPriority(out[j].Layer)
	})
	return out
}

func projectionDirtyLayerPriority(layer string) int {
	switch strings.TrimSpace(layer) {
	case "event", "paradigm":
		return 0
	case "global-v2":
		return 1
	default:
		return 2
	}
}

func (s *SQLiteStore) runProjectionDirtyMark(ctx context.Context, mark ProjectionDirtyMark, now time.Time, state *projectionDirtyUserState) error {
	userID, err := normalizeRequiredUserID(mark.UserID)
	if err != nil {
		return err
	}
	switch strings.TrimSpace(mark.Layer) {
	case "event":
		_, err = s.RunEventGraphProjection(ctx, userID, now)
		if err == nil && state != nil {
			state.eventRefreshed = true
		}
	case "paradigm":
		_, err = s.RunParadigmProjection(ctx, userID, now)
		if err == nil && state != nil {
			state.paradigmRefreshed = true
		}
	case "global-v2":
		refreshProjections := state == nil || !state.eventRefreshed || !state.paradigmRefreshed
		_, err = s.runGlobalMemoryOrganizationV2(ctx, userID, now, refreshProjections)
		if err == nil && refreshProjections && state != nil {
			state.eventRefreshed = true
			state.paradigmRefreshed = true
		}
	case "subject-timeline":
		if strings.TrimSpace(mark.Subject) == "" {
			return fmt.Errorf("subject-timeline mark requires subject")
		}
		_, err = s.BuildSubjectTimeline(ctx, userID, mark.Subject, now)
	case "subject-horizon":
		if strings.TrimSpace(mark.Subject) == "" || strings.TrimSpace(mark.Horizon) == "" {
			return fmt.Errorf("subject-horizon mark requires subject and horizon")
		}
		_, err = s.GetSubjectHorizonMemory(ctx, userID, mark.Subject, mark.Horizon, now, true)
	case "subject-experience":
		if strings.TrimSpace(mark.Subject) == "" {
			return fmt.Errorf("subject-experience mark requires subject")
		}
		_, err = s.GetSubjectExperienceMemory(ctx, userID, mark.Subject, defaultSubjectExperienceHorizons, now, false)
	default:
		err = fmt.Errorf("unsupported projection layer %q", mark.Layer)
	}
	return err
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

func projectionDirtySweepError(mark ProjectionDirtyMark, err error) ProjectionDirtySweepError {
	return ProjectionDirtySweepError{
		DirtyID: mark.ID,
		Layer:   strings.TrimSpace(mark.Layer),
		Subject: strings.TrimSpace(mark.Subject),
		Horizon: strings.TrimSpace(mark.Horizon),
		Error:   err.Error(),
	}
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
