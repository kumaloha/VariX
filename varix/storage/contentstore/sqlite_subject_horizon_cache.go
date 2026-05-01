package contentstore

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"github.com/kumaloha/VariX/varix/memory"
	"strings"
	"time"
)

func (s *SQLiteStore) getCachedSubjectHorizonMemory(ctx context.Context, userID, canonicalSubject, horizon string, now time.Time) (memory.SubjectHorizonMemory, bool, error) {
	var payload, nextRefreshAt string
	err := s.db.QueryRowContext(ctx, `SELECT payload_json, next_refresh_at FROM subject_horizon_memories WHERE user_id = ? AND canonical_subject = ? AND horizon = ?`, userID, canonicalSubject, horizon).Scan(&payload, &nextRefreshAt)
	if err == sql.ErrNoRows {
		return memory.SubjectHorizonMemory{}, false, nil
	}
	if err != nil {
		return memory.SubjectHorizonMemory{}, false, err
	}
	next, err := time.Parse(time.RFC3339, strings.TrimSpace(nextRefreshAt))
	if err != nil {
		return memory.SubjectHorizonMemory{}, false, nil
	}
	if !now.Before(next) {
		return memory.SubjectHorizonMemory{}, false, nil
	}
	var out memory.SubjectHorizonMemory
	if err := json.Unmarshal([]byte(payload), &out); err != nil {
		return memory.SubjectHorizonMemory{}, false, fmt.Errorf("decode subject horizon memory: %w", err)
	}
	out.CacheStatus = "fresh"
	return out, true, nil
}

func (s *SQLiteStore) upsertSubjectHorizonMemory(ctx context.Context, out memory.SubjectHorizonMemory) error {
	payload, err := json.Marshal(out)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `INSERT INTO subject_horizon_memories(user_id, subject, canonical_subject, horizon, window_start, window_end, refresh_policy, next_refresh_at, input_hash, payload_json, generated_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(user_id, canonical_subject, horizon) DO UPDATE SET
		  subject = excluded.subject,
		  window_start = excluded.window_start,
		  window_end = excluded.window_end,
		  refresh_policy = excluded.refresh_policy,
		  next_refresh_at = excluded.next_refresh_at,
		  input_hash = excluded.input_hash,
		  payload_json = excluded.payload_json,
		  generated_at = excluded.generated_at,
		  updated_at = excluded.updated_at`,
		out.UserID,
		out.Subject,
		out.CanonicalSubject,
		out.Horizon,
		out.WindowStart,
		out.WindowEnd,
		out.RefreshPolicy,
		out.NextRefreshAt,
		out.InputHash,
		string(payload),
		out.GeneratedAt,
		out.GeneratedAt,
	)
	return err
}
