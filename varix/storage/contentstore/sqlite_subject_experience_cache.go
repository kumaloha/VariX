package contentstore

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"

	"github.com/kumaloha/VariX/varix/memory"
)

func (s *SQLiteStore) getCachedSubjectExperienceMemory(ctx context.Context, userID, canonicalSubject, horizonSet, inputHash string) (memory.SubjectExperienceMemory, bool, error) {
	var payload, storedHash string
	err := s.db.QueryRowContext(ctx, `SELECT input_hash, payload_json FROM subject_experience_memories WHERE user_id = ? AND canonical_subject = ? AND horizon_set = ?`, userID, canonicalSubject, horizonSet).Scan(&storedHash, &payload)
	if err == sql.ErrNoRows {
		return memory.SubjectExperienceMemory{}, false, nil
	}
	if err != nil {
		return memory.SubjectExperienceMemory{}, false, err
	}
	if storedHash != inputHash {
		return memory.SubjectExperienceMemory{}, false, nil
	}
	var out memory.SubjectExperienceMemory
	if err := json.Unmarshal([]byte(payload), &out); err != nil {
		return memory.SubjectExperienceMemory{}, false, fmt.Errorf("decode subject experience memory: %w", err)
	}
	out.CacheStatus = "fresh"
	return out, true, nil
}
func (s *SQLiteStore) upsertSubjectExperienceMemory(ctx context.Context, horizonSet string, out memory.SubjectExperienceMemory) error {
	payload, err := json.Marshal(out)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `INSERT INTO subject_experience_memories(user_id, subject, canonical_subject, horizon_set, input_hash, payload_json, generated_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(user_id, canonical_subject, horizon_set) DO UPDATE SET
		  subject = excluded.subject,
		  input_hash = excluded.input_hash,
		  payload_json = excluded.payload_json,
		  generated_at = excluded.generated_at,
		  updated_at = excluded.updated_at`,
		out.UserID,
		out.Subject,
		out.CanonicalSubject,
		horizonSet,
		out.InputHash,
		string(payload),
		out.GeneratedAt,
		out.GeneratedAt,
	)
	return err
}
