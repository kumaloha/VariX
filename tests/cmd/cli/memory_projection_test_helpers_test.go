package main

import (
	"github.com/kumaloha/VariX/varix/model"
	"time"
)

func testDriverTargetSubgraph(id string, at time.Time) model.ContentSubgraph {
	stamp := at.Format(time.RFC3339)
	node := func(nodeID, raw, subject, change string, kind model.ContentNodeKind, role model.GraphRole, status model.VerificationStatus) model.ContentNode {
		return model.ContentNode{ID: nodeID, SourceArticleID: id, SourcePlatform: "twitter", SourceExternalID: id, RawText: raw, SubjectText: subject, ChangeText: change, Kind: kind, GraphRole: role, IsPrimary: true, VerificationStatus: status, TimeBucket: "1w"}
	}
	return model.ContentSubgraph{
		ID:               id,
		ArticleID:        id,
		SourcePlatform:   "twitter",
		SourceExternalID: id,
		CompileVersion:   model.CompileBridgeVersion,
		CompiledAt:       stamp,
		UpdatedAt:        stamp,
		Nodes: []model.ContentNode{
			node("n1", "美联储加息0.25%", "美联储", "加息0.25%", model.NodeKindObservation, model.GraphRoleDriver, model.VerificationPending),
			node("n2", "未来一周美股承压", "美股", "承压", model.NodeKindPrediction, model.GraphRoleTarget, model.VerificationProved),
		},
	}
}
