package contentstore

import (
	"context"
	"database/sql"
	"strings"
	"time"

	"github.com/kumaloha/VariX/varix/compile"
	"github.com/kumaloha/VariX/varix/graphmodel"
)

func persistMemoryContentGraphTx(ctx context.Context, tx *sql.Tx, userID string, record compile.Record, acceptedAt time.Time) (graphmodel.ContentSubgraph, error) {
	subgraph, err := getContentSubgraph(ctx, tx, record.Source, record.ExternalID)
	if err == sql.ErrNoRows {
		subgraph, err = graphmodel.FromCompileRecord(record)
	}
	if err != nil {
		return graphmodel.ContentSubgraph{}, err
	}
	if err := persistMemoryContentGraphSubgraphTx(ctx, tx, userID, subgraph, acceptedAt); err != nil {
		return graphmodel.ContentSubgraph{}, err
	}
	return subgraph, nil
}

func (s *SQLiteStore) PersistMemoryContentGraphFromCompiledOutput(ctx context.Context, userID, sourcePlatform, sourceExternalID string, acceptedAt time.Time) error {
	record, err := s.GetCompiledOutput(ctx, strings.TrimSpace(sourcePlatform), strings.TrimSpace(sourceExternalID))
	if err != nil {
		return err
	}
	subgraph, err := graphmodel.FromCompileRecord(record)
	if err != nil {
		return err
	}
	return s.PersistMemoryContentGraph(ctx, strings.TrimSpace(userID), subgraph, acceptedAt)
}
