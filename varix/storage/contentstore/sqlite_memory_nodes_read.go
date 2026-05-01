package contentstore

import (
	"context"
	"database/sql"

	"github.com/kumaloha/VariX/varix/memory"
)

func listAllUserMemoryTx(ctx context.Context, tx *sql.Tx, userID string) ([]memory.AcceptedNode, error) {
	rows, err := tx.QueryContext(ctx, `SELECT memory_id, user_id, source_platform, source_external_id, root_external_id, node_id, node_kind, node_text, source_model, source_compiled_at, valid_from, valid_to, accepted_at
		FROM user_memory_nodes
		WHERE user_id = ?
		ORDER BY accepted_at ASC, memory_id ASC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanMemoryNodes(rows)
}
