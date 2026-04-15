package contentstore

import (
	"testing"
	"time"

	"github.com/kumaloha/VariX/varix/compile"
	"github.com/kumaloha/VariX/varix/memory"
)

func TestDetectThesisConflict_BlocksConclusionConflict(t *testing.T) {
	now := time.Date(2026, 4, 15, 0, 0, 0, 0, time.UTC)
	thesis := memory.CandidateThesis{
		ThesisID: "thesis-1",
		NodeIDs:  []string{"weibo:S1:n1", "twitter:S2:n1"},
	}
	nodesByID := map[string]memory.AcceptedNode{
		"weibo:S1:n1": {
			NodeID:   "weibo:S1:n1",
			NodeKind: string(compile.NodeConclusion),
			NodeText: "油价会上升",
		},
		"twitter:S2:n1": {
			NodeID:   "twitter:S2:n1",
			NodeKind: string(compile.NodeConclusion),
			NodeText: "油价会下降",
		},
	}

	got := detectThesisConflict(thesis, nodesByID, now)
	if !got.Blocked {
		t.Fatalf("Blocked = false, want true")
	}
	if got.Conflict == nil || got.Conflict.ConflictStatus != "blocked" {
		t.Fatalf("Conflict = %#v, want blocked conflict set", got.Conflict)
	}
	if got.Conflict.SideASummary == "" || got.Conflict.SideBSummary == "" {
		t.Fatalf("Conflict side summaries = %#v, want readable side summaries", got.Conflict)
	}
	if len(got.Conflict.SideASourceRefs) == 0 || len(got.Conflict.SideBSourceRefs) == 0 {
		t.Fatalf("Conflict side source refs = %#v, want source refs on both sides", got.Conflict)
	}
}

func TestDetectThesisConflict_DoesNotFlagSupportingNodes(t *testing.T) {
	now := time.Date(2026, 4, 15, 0, 0, 0, 0, time.UTC)
	thesis := memory.CandidateThesis{
		ThesisID: "thesis-1",
		NodeIDs:  []string{"weibo:S1:n1", "weibo:S1:n2"},
	}
	nodesByID := map[string]memory.AcceptedNode{
		"weibo:S1:n1": {
			NodeID:   "weibo:S1:n1",
			NodeKind: string(compile.NodeFact),
			NodeText: "油价上涨",
		},
		"weibo:S1:n2": {
			NodeID:   "weibo:S1:n2",
			NodeKind: string(compile.NodePrediction),
			NodeText: "未来能源企业利润可能改善",
		},
	}

	got := detectThesisConflict(thesis, nodesByID, now)
	if got.Blocked {
		t.Fatalf("Blocked = true, want false for supporting/non-contradictory nodes")
	}
	if got.Conflict != nil {
		t.Fatalf("Conflict = %#v, want nil", got.Conflict)
	}
}

func TestDetectThesisConflict_BlocksConditionConflict(t *testing.T) {
	now := time.Date(2026, 4, 15, 0, 0, 0, 0, time.UTC)
	thesis := memory.CandidateThesis{
		ThesisID:   "thesis-1",
		TopicLabel: "关于「油价路径」的判断",
		NodeIDs:    []string{"weibo:S1:n1", "twitter:S2:n1"},
	}
	nodesByID := map[string]memory.AcceptedNode{
		"weibo:S1:n1": {
			NodeID:   "weibo:S1:n1",
			NodeKind: string(compile.NodeExplicitCondition),
			NodeText: "若供给收缩，油价会上升",
		},
		"twitter:S2:n1": {
			NodeID:   "twitter:S2:n1",
			NodeKind: string(compile.NodeExplicitCondition),
			NodeText: "若供给收缩，油价会下降",
		},
	}

	got := detectThesisConflict(thesis, nodesByID, now)
	if !got.Blocked {
		t.Fatalf("Blocked = false, want true for condition conflict")
	}
	if got.Conflict == nil || got.Conflict.ConflictReason == "" {
		t.Fatalf("Conflict = %#v, want reasoned blocked conflict", got.Conflict)
	}
}

func TestDetectThesisConflict_BlocksMechanismConflict(t *testing.T) {
	now := time.Date(2026, 4, 15, 0, 0, 0, 0, time.UTC)
	thesis := memory.CandidateThesis{
		ThesisID:   "thesis-1",
		TopicLabel: "关于「银行监管与系统安全」的判断",
		NodeIDs:    []string{"weibo:S1:n1", "twitter:S2:n1"},
	}
	nodesByID := map[string]memory.AcceptedNode{
		"weibo:S1:n1": {
			NodeID:   "weibo:S1:n1",
			NodeKind: string(compile.NodeImplicitCondition),
			NodeText: "银行去监管会提升金融体系安全性",
		},
		"twitter:S2:n1": {
			NodeID:   "twitter:S2:n1",
			NodeKind: string(compile.NodeImplicitCondition),
			NodeText: "银行去监管会削弱金融体系安全性",
		},
	}

	got := detectThesisConflict(thesis, nodesByID, now)
	if !got.Blocked {
		t.Fatalf("Blocked = false, want true for mechanism conflict")
	}
}
