package contentstore

import (
	"context"
	"fmt"
	"time"

	"github.com/kumaloha/VariX/varix/graphmodel"
)

func (s *SQLiteStore) RunVerifyQueueSweepFromContentGraphState(ctx context.Context, now time.Time, limit int) (VerifyQueueSweepResult, error) {
	return s.RunVerifyQueueSweep(ctx, now, limit, func(item graphmodel.VerifyQueueItem) (graphmodel.VerifyVerdict, error) {
		subgraph, err := s.GetContentSubgraphByArticleID(ctx, item.SourceArticleID)
		if err != nil {
			return graphmodel.VerifyVerdict{}, err
		}
		switch item.ObjectType {
		case graphmodel.VerifyQueueObjectNode:
			for _, node := range subgraph.Nodes {
				if node.ID != item.ObjectID {
					continue
				}
				return graphmodel.VerifyVerdict{ObjectType: item.ObjectType, ObjectID: item.ObjectID, Verdict: node.VerificationStatus, Reason: node.VerificationReason, AsOf: firstTrimmed(node.VerificationAsOf, now.Format(time.RFC3339)), NextVerifyAt: node.NextVerifyAt}, nil
			}
		case graphmodel.VerifyQueueObjectEdge:
			for _, edge := range subgraph.Edges {
				if edge.ID != item.ObjectID {
					continue
				}
				return graphmodel.VerifyVerdict{ObjectType: item.ObjectType, ObjectID: item.ObjectID, Verdict: edge.VerificationStatus, Reason: edge.VerificationReason, AsOf: firstTrimmed(edge.VerificationAsOf, now.Format(time.RFC3339)), NextVerifyAt: edge.NextVerifyAt}, nil
			}
		}
		return graphmodel.VerifyVerdict{}, fmt.Errorf("queue item object not found in content graph: %s", item.ObjectID)
	})
}
