package contentstore

import (
	"context"
	"fmt"
	"time"

	"github.com/kumaloha/VariX/varix/model"
)

func (s *SQLiteStore) RunVerifyQueueSweepFromContentGraphState(ctx context.Context, now time.Time, limit int) (VerifyQueueSweepResult, error) {
	return s.RunVerifyQueueSweep(ctx, now, limit, func(item model.VerifyQueueItem) (model.VerifyVerdict, error) {
		subgraph, err := s.GetContentSubgraphByArticleID(ctx, item.SourceArticleID)
		if err != nil {
			return model.VerifyVerdict{}, err
		}
		switch item.ObjectType {
		case model.VerifyQueueObjectNode:
			for _, node := range subgraph.Nodes {
				if node.ID != item.ObjectID {
					continue
				}
				return model.VerifyVerdict{ObjectType: item.ObjectType, ObjectID: item.ObjectID, Verdict: node.VerificationStatus, Reason: node.VerificationReason, AsOf: firstTrimmed(node.VerificationAsOf, now.Format(time.RFC3339)), NextVerifyAt: node.NextVerifyAt}, nil
			}
		case model.VerifyQueueObjectEdge:
			for _, edge := range subgraph.Edges {
				if edge.ID != item.ObjectID {
					continue
				}
				return model.VerifyVerdict{ObjectType: item.ObjectType, ObjectID: item.ObjectID, Verdict: edge.VerificationStatus, Reason: edge.VerificationReason, AsOf: firstTrimmed(edge.VerificationAsOf, now.Format(time.RFC3339)), NextVerifyAt: edge.NextVerifyAt}, nil
			}
		}
		return model.VerifyVerdict{}, fmt.Errorf("queue item object not found in content graph: %s", item.ObjectID)
	})
}
