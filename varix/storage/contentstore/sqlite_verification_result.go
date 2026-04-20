package contentstore

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/kumaloha/VariX/varix/compile"
)

func (s *SQLiteStore) UpsertVerificationResult(ctx context.Context, record compile.VerificationRecord) error {
	if record.Source == "" || record.ExternalID == "" || record.Model == "" {
		return fmt.Errorf("invalid verification result")
	}
	if record.VerifiedAt.IsZero() {
		record.VerifiedAt = record.Verification.VerifiedAt
	}
	if record.VerifiedAt.IsZero() {
		record.VerifiedAt = time.Now().UTC()
	}
	payload, err := json.Marshal(record)
	if err != nil {
		return err
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err = s.db.ExecContext(
		ctx,
		`INSERT INTO verification_results(platform, external_id, root_external_id, model, payload_json, verified_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(platform, external_id) DO UPDATE SET
		   root_external_id = excluded.root_external_id,
		   model = excluded.model,
		   payload_json = excluded.payload_json,
		   verified_at = excluded.verified_at,
		   updated_at = excluded.updated_at`,
		record.Source,
		record.ExternalID,
		record.RootExternalID,
		record.Model,
		string(payload),
		record.VerifiedAt.UTC().Format(time.RFC3339Nano),
		now,
	)
	return err
}

func (s *SQLiteStore) GetVerificationResult(ctx context.Context, platform, externalID string) (compile.VerificationRecord, error) {
	return getVerificationResult(ctx, s.db, platform, externalID)
}

func getVerificationResult(ctx context.Context, q interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}, platform, externalID string) (compile.VerificationRecord, error) {
	var payload string
	if err := q.QueryRowContext(ctx, `SELECT payload_json FROM verification_results WHERE platform = ? AND external_id = ?`, platform, externalID).Scan(&payload); err != nil {
		return compile.VerificationRecord{}, err
	}
	var record compile.VerificationRecord
	if err := json.Unmarshal([]byte(payload), &record); err != nil {
		return compile.VerificationRecord{}, err
	}
	return record, nil
}

func getVerificationResultTx(ctx context.Context, tx interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}, platform, externalID string) (compile.VerificationRecord, error) {
	return getVerificationResult(ctx, tx, platform, externalID)
}
