package contentstore

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/kumaloha/VariX/varix/ingest/types"
)

type WarningKind string

const (
	WarningKindCorruptJSON   WarningKind = "corrupt_json"
	WarningKindInvalidRecord WarningKind = "invalid_record"
)

type ScanWarning struct {
	Path   string      `json:"path"`
	Kind   WarningKind `json:"kind"`
	Detail string      `json:"detail"`
}

func isValidProcessedRecord(record types.ProcessedRecord) bool {
	return strings.TrimSpace(record.Platform) != "" &&
		strings.TrimSpace(record.ExternalID) != ""
}

func isValidFollowTarget(target types.FollowTarget) bool {
	return strings.TrimSpace(string(target.Kind)) != "" &&
		strings.TrimSpace(target.Platform) != "" &&
		strings.TrimSpace(target.Locator) != ""
}

func normalizeRequiredUserID(userID string) (string, error) {
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return "", fmt.Errorf("user id is required")
	}
	return userID, nil
}

func normalizeNow(now time.Time) time.Time {
	if now.IsZero() {
		return time.Now().UTC()
	}
	return now
}

func currentSQLiteTimestamp() string {
	return time.Now().UTC().Format(time.RFC3339Nano)
}

func normalizeRecordedTime(value time.Time) time.Time {
	if value.IsZero() {
		return time.Now().UTC()
	}
	return value.UTC()
}

func latestUserScopedPayload(ctx context.Context, q interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}, table string, userID string) (string, error) {
	var payload string
	err := q.QueryRowContext(
		ctx,
		fmt.Sprintf(`SELECT payload_json FROM %s
		 WHERE user_id = ?
		 ORDER BY created_at DESC, output_id DESC
		 LIMIT 1`, table),
		strings.TrimSpace(userID),
	).Scan(&payload)
	return payload, err
}

func persistLatestUserScopedOutput(ctx context.Context, db *sql.DB, table string, userID string, createdAt time.Time, value any, setOutputID func(int64)) error {
	payload, err := json.Marshal(value)
	if err != nil {
		return err
	}
	res, err := db.ExecContext(
		ctx,
		fmt.Sprintf(`INSERT INTO %s(user_id, payload_json, created_at)
		 VALUES (?, ?, ?)
		 ON CONFLICT(user_id) DO UPDATE SET payload_json = excluded.payload_json, created_at = excluded.created_at`, table),
		userID,
		string(payload),
		createdAt.Format(time.RFC3339Nano),
	)
	if err != nil {
		return err
	}
	outputID, _ := res.LastInsertId()
	if outputID == 0 {
		_ = db.QueryRowContext(ctx, fmt.Sprintf(`SELECT output_id FROM %s WHERE user_id = ?`, table), userID).Scan(&outputID)
	}
	if setOutputID != nil {
		setOutputID(outputID)
		payload, err = json.Marshal(value)
		if err != nil {
			return err
		}
	}
	_, err = db.ExecContext(
		ctx,
		fmt.Sprintf(`UPDATE %s SET payload_json = ?, created_at = ? WHERE user_id = ?`, table),
		string(payload),
		createdAt.Format(time.RFC3339Nano),
		userID,
	)
	return err
}

func hasDistinctNonEmptyPair(left, right string) bool {
	left = strings.TrimSpace(left)
	right = strings.TrimSpace(right)
	return left != "" && right != "" && left != right
}

func decodePayloadRows[T any](rows *sql.Rows, decodeLabel string) ([]T, error) {
	defer rows.Close()
	out := make([]T, 0)
	for rows.Next() {
		var payload string
		if err := rows.Scan(&payload); err != nil {
			return nil, err
		}
		var record T
		if err := json.Unmarshal([]byte(payload), &record); err != nil {
			return nil, fmt.Errorf("decode %s payload: %w", decodeLabel, err)
		}
		out = append(out, record)
	}
	return out, rows.Err()
}
