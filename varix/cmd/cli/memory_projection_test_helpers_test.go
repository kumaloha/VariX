package main

import (
	"github.com/kumaloha/VariX/varix/graphmodel"
	"time"
)

func testDriverTargetSubgraph(id string, at time.Time) graphmodel.ContentSubgraph {
	stamp := at.Format(time.RFC3339)
	node := func(nodeID, raw, subject, change string, kind graphmodel.NodeKind, role graphmodel.GraphRole, status graphmodel.VerificationStatus) graphmodel.GraphNode {
		return graphmodel.GraphNode{ID: nodeID, SourceArticleID: id, SourcePlatform: "twitter", SourceExternalID: id, RawText: raw, SubjectText: subject, ChangeText: change, Kind: kind, GraphRole: role, IsPrimary: true, VerificationStatus: status, TimeBucket: "1w"}
	}
	return graphmodel.ContentSubgraph{
		ID:               id,
		ArticleID:        id,
		SourcePlatform:   "twitter",
		SourceExternalID: id,
		CompileVersion:   graphmodel.CompileBridgeVersion,
		CompiledAt:       stamp,
		UpdatedAt:        stamp,
		Nodes: []graphmodel.GraphNode{
			node("n1", "美联储加息0.25%", "美联储", "加息0.25%", graphmodel.NodeKindObservation, graphmodel.GraphRoleDriver, graphmodel.VerificationPending),
			node("n2", "未来一周美股承压", "美股", "承压", graphmodel.NodeKindPrediction, graphmodel.GraphRoleTarget, graphmodel.VerificationProved),
		},
	}
}
