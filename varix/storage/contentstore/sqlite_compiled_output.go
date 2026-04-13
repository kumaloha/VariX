package contentstore

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/kumaloha/VariX/varix/compile"
)

func (s *SQLiteStore) UpsertCompiledOutput(ctx context.Context, record compile.Record) error {
	if record.Source == "" || record.ExternalID == "" || record.Model == "" {
		return fmt.Errorf("invalid compiled output")
	}
	if err := record.Output.Validate(); err != nil {
		return err
	}
	if record.CompiledAt.IsZero() {
		record.CompiledAt = time.Now().UTC()
	}
	payload, err := json.Marshal(record)
	if err != nil {
		return err
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err = s.db.ExecContext(
		ctx,
		`INSERT INTO compiled_outputs(platform, external_id, root_external_id, model, payload_json, compiled_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(platform, external_id) DO UPDATE SET
		   root_external_id = excluded.root_external_id,
		   model = excluded.model,
		   payload_json = excluded.payload_json,
		   compiled_at = excluded.compiled_at,
		   updated_at = excluded.updated_at`,
		record.Source,
		record.ExternalID,
		record.RootExternalID,
		record.Model,
		string(payload),
		record.CompiledAt.UTC().Format(time.RFC3339Nano),
		now,
	)
	return err
}

func (s *SQLiteStore) GetCompiledOutput(ctx context.Context, platform, externalID string) (compile.Record, error) {
	var payload string
	err := s.db.QueryRowContext(
		ctx,
		`SELECT payload_json FROM compiled_outputs WHERE platform = ? AND external_id = ?`,
		platform,
		externalID,
	).Scan(&payload)
	if err != nil {
		return compile.Record{}, err
	}
	var record compile.Record
	if err := json.Unmarshal([]byte(payload), &record); err != nil {
		return compile.Record{}, err
	}
	return record, nil
}
