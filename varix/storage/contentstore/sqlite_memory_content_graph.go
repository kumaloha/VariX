package contentstore

import (
	"context"
	"database/sql"
	"strings"
	"time"

	"github.com/kumaloha/VariX/varix/model"
)

func persistMemoryContentGraphTx(ctx context.Context, tx *sql.Tx, userID string, record model.Record, acceptedAt time.Time) (model.ContentSubgraph, error) {
	subgraph, err := getContentSubgraph(ctx, tx, record.Source, record.ExternalID)
	if err == sql.ErrNoRows {
		subgraph, err = model.FromCompileRecord(record)
	}
	if err != nil {
		return model.ContentSubgraph{}, err
	}
	if err := persistMemoryContentGraphSubgraphTx(ctx, tx, userID, subgraph, acceptedAt); err != nil {
		return model.ContentSubgraph{}, err
	}
	return subgraph, nil
}

func (s *SQLiteStore) PersistMemoryContentGraphFromCompiledOutput(ctx context.Context, userID, sourcePlatform, sourceExternalID string, acceptedAt time.Time) error {
	record, err := s.GetCompiledOutput(ctx, strings.TrimSpace(sourcePlatform), strings.TrimSpace(sourceExternalID))
	if err != nil {
		return err
	}
	subgraph, err := model.FromCompileRecord(record)
	if err != nil {
		return err
	}
	return s.PersistMemoryContentGraph(ctx, strings.TrimSpace(userID), subgraph, acceptedAt)
}
