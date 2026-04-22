package contentstore

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/kumaloha/VariX/varix/ingest/types"
)

func (s *SQLiteStore) UpsertRawCapture(ctx context.Context, raw types.RawContent) error {
	raw, err := normalizeRawCapture(raw)
	if err != nil {
		return err
	}
	payload, err := json.Marshal(raw)
	if err != nil {
		return err
	}
	now := currentSQLiteTimestamp()

	_, err = s.db.ExecContext(
		ctx,
		`INSERT INTO raw_captures(platform, external_id, url, source, author_name, author_id, posted_at, payload_json, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(platform, external_id) DO UPDATE SET
		   url = excluded.url,
		   source = excluded.source,
		   author_name = excluded.author_name,
		   author_id = excluded.author_id,
		   posted_at = excluded.posted_at,
		   payload_json = excluded.payload_json,
		   updated_at = excluded.updated_at`,
		raw.Source,
		raw.ExternalID,
		raw.URL,
		raw.Source,
		raw.AuthorName,
		raw.AuthorID,
		formatSQLiteTime(raw.PostedAt),
		string(payload),
		now,
		now,
	)
	if err != nil {
		return err
	}
	if !shouldQueueSourceLookup(raw) {
		return nil
	}
	_, err = s.db.ExecContext(
		ctx,
		`INSERT INTO source_lookup_jobs(platform, external_id, status, attempt_count, last_error, created_at, updated_at)
		 VALUES (?, ?, ?, 0, '', ?, ?)
		 ON CONFLICT(platform, external_id) DO UPDATE SET
		   status = CASE WHEN source_lookup_jobs.status = 'pending' THEN excluded.status ELSE source_lookup_jobs.status END,
		   updated_at = excluded.updated_at`,
		raw.Source,
		raw.ExternalID,
		string(raw.Provenance.SourceLookup.Status),
		now,
		now,
	)
	return err
}

func (s *SQLiteStore) GetRawCapture(ctx context.Context, platform, externalID string) (types.RawContent, error) {
	var payload string
	err := s.db.QueryRowContext(
		ctx,
		`SELECT payload_json FROM raw_captures WHERE platform = ? AND external_id = ?`,
		platform,
		externalID,
	).Scan(&payload)
	if err != nil {
		return types.RawContent{}, err
	}
	var raw types.RawContent
	if err := json.Unmarshal([]byte(payload), &raw); err != nil {
		return types.RawContent{}, err
	}
	return raw, nil
}

func (s *SQLiteStore) ListPendingSourceLookups(ctx context.Context, limit int) ([]types.RawContent, error) {
	if limit <= 0 {
		limit = 100
	}
	rows, err := s.db.QueryContext(
		ctx,
		`SELECT r.payload_json
		 FROM source_lookup_jobs j
		 JOIN raw_captures r
		   ON r.platform = j.platform AND r.external_id = j.external_id
		 WHERE j.status = ?
		 ORDER BY j.updated_at ASC
		 LIMIT ?`,
		string(types.SourceLookupStatusPending),
		limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]types.RawContent, 0)
	for rows.Next() {
		var payload string
		if err := rows.Scan(&payload); err != nil {
			return nil, err
		}
		var raw types.RawContent
		if err := json.Unmarshal([]byte(payload), &raw); err != nil {
			return nil, err
		}
		items = append(items, raw)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return items, nil
}

func (s *SQLiteStore) MarkSourceLookupResult(ctx context.Context, raw types.RawContent, status types.SourceLookupStatus, errDetail string) error {
	raw, err := normalizeRawCapture(raw)
	if err != nil {
		return err
	}
	if raw.Provenance == nil {
		raw.Provenance = &types.Provenance{}
	}
	raw.Provenance.SourceLookup.Status = status
	payload, err := json.Marshal(raw)
	if err != nil {
		return err
	}
	now := currentSQLiteTimestamp()

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(
		ctx,
		`UPDATE raw_captures
		 SET url = ?, source = ?, author_name = ?, author_id = ?, posted_at = ?, payload_json = ?, updated_at = ?
		 WHERE platform = ? AND external_id = ?`,
		raw.URL,
		raw.Source,
		raw.AuthorName,
		raw.AuthorID,
		formatSQLiteTime(raw.PostedAt),
		string(payload),
		now,
		raw.Source,
		raw.ExternalID,
	); err != nil {
		return err
	}
	if _, err := tx.ExecContext(
		ctx,
		`INSERT INTO source_lookup_jobs(platform, external_id, status, attempt_count, last_error, created_at, updated_at)
		 VALUES (?, ?, ?, CASE WHEN ? = '' THEN 0 ELSE 1 END, ?, ?, ?)
		 ON CONFLICT(platform, external_id) DO UPDATE SET
		   status = excluded.status,
		   attempt_count = source_lookup_jobs.attempt_count + 1,
		   last_error = excluded.last_error,
		   updated_at = excluded.updated_at`,
		raw.Source,
		raw.ExternalID,
		string(status),
		errDetail,
		errDetail,
		now,
		now,
	); err != nil {
		return err
	}
	return tx.Commit()
}

func normalizeRawCapture(raw types.RawContent) (types.RawContent, error) {
	raw.Source = strings.TrimSpace(raw.Source)
	raw.ExternalID = strings.TrimSpace(raw.ExternalID)
	if raw.Source == "" || raw.ExternalID == "" {
		return types.RawContent{}, fmt.Errorf("invalid raw capture")
	}
	if raw.Provenance != nil && raw.Provenance.NeedsSourceLookup && raw.Provenance.SourceLookup.Status == "" {
		raw.Provenance.SourceLookup.Status = types.SourceLookupStatusPending
	}
	return raw, nil
}

func shouldQueueSourceLookup(raw types.RawContent) bool {
	return raw.Provenance != nil &&
		raw.Provenance.NeedsSourceLookup &&
		raw.Provenance.SourceLookup.Status == types.SourceLookupStatusPending
}
