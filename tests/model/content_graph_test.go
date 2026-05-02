package model

import "testing"

func TestGraphNodeValidateRequiresCoreFields(t *testing.T) {
	node := ContentNode{}
	if err := node.Validate(); err == nil {
		t.Fatal("Validate() error = nil, want missing core field error")
	}

	node = ContentNode{
		ID:                 "n1",
		SourceArticleID:    "u1",
		SourcePlatform:     "twitter",
		SourceExternalID:   "123",
		RawText:            "美联储加息0.25%",
		SubjectText:        "美联储",
		ChangeText:         "加息0.25%",
		Kind:               NodeKindObservation,
		IsPrimary:          true,
		VerificationStatus: VerificationPending,
	}
	if err := node.Validate(); err != nil {
		t.Fatalf("Validate() error = %v, want nil", err)
	}
}

func TestContentSubgraphValidateRejectsUnknownEdgeNode(t *testing.T) {
	subgraph := ContentSubgraph{
		ID:               "sg1",
		ArticleID:        "u1",
		SourcePlatform:   "twitter",
		SourceExternalID: "123",
		CompileVersion:   CompileBridgeVersion,
		CompiledAt:       "2026-04-21T00:00:00Z",
		UpdatedAt:        "2026-04-21T00:00:00Z",
		Nodes: []ContentNode{{
			ID:                 "n1",
			SourceArticleID:    "u1",
			SourcePlatform:     "twitter",
			SourceExternalID:   "123",
			RawText:            "美联储加息0.25%",
			SubjectText:        "美联储",
			ChangeText:         "加息0.25%",
			Kind:               NodeKindObservation,
			IsPrimary:          true,
			VerificationStatus: VerificationPending,
		}},
		Edges: []ContentEdge{{
			ID:                 "e1",
			From:               "n1",
			To:                 "missing",
			Type:               EdgeTypeDrives,
			IsPrimary:          true,
			VerificationStatus: VerificationPending,
		}},
	}

	if err := subgraph.Validate(); err == nil {
		t.Fatal("Validate() error = nil, want unknown edge endpoint error")
	}
}
