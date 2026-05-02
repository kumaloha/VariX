//go:build compile_manual

package compile

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"

	"github.com/kumaloha/VariX/varix/ingest/types"
	_ "modernc.org/sqlite"
)

func loadManualRawCapture(ctx context.Context, dbPath, platform, externalID string) (types.RawContent, error) {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return types.RawContent{}, err
	}
	defer db.Close()

	var payload string
	if err := db.QueryRowContext(ctx, `SELECT payload_json FROM raw_captures WHERE platform = ? AND external_id = ?`, platform, externalID).Scan(&payload); err != nil {
		return types.RawContent{}, err
	}
	var raw types.RawContent
	if err := json.Unmarshal([]byte(payload), &raw); err != nil {
		return types.RawContent{}, fmt.Errorf("parse raw capture payload: %w", err)
	}
	return raw, nil
}
