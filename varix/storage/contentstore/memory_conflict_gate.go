package contentstore

import (
	"time"

	"github.com/kumaloha/VariX/varix/compile"
	"github.com/kumaloha/VariX/varix/memory"
	"strings"
)

type thesisConflictResult struct {
	Blocked  bool
	Conflict *memory.ConflictSet
}

func detectThesisConflict(thesis memory.CandidateThesis, nodesByID map[string]memory.AcceptedNode, now time.Time) thesisConflictResult {
	for i := 0; i < len(thesis.NodeIDs); i++ {
		left, ok := nodesByID[thesis.NodeIDs[i]]
		if !ok {
			continue
		}
		for j := i + 1; j < len(thesis.NodeIDs); j++ {
			right, ok := nodesByID[thesis.NodeIDs[j]]
			if !ok {
				continue
			}
			if !sameGlobalClusterFamily(left, right) {
				continue
			}
			reason, ok := thesisConflictReason(left, right)
			if !ok {
				continue
			}
			conflict := buildConflictSet(
				thesis,
				[]string{left.NodeID},
				[]string{right.NodeID},
				[]string{left.SourcePlatform + ":" + left.SourceExternalID},
				[]string{right.SourcePlatform + ":" + right.SourceExternalID},
				left.NodeText,
				right.NodeText,
				reason,
				now,
			)
			return thesisConflictResult{Blocked: true, Conflict: &conflict}
		}
	}
	return thesisConflictResult{}
}

func thesisConflictReason(left, right memory.AcceptedNode) (string, bool) {
	if reason, ok := contradictionReason(left.NodeText, right.NodeText); ok {
		return reason, true
	}
	switch {
	case left.NodeKind == string(compile.NodeImplicitCondition) && right.NodeKind == string(compile.NodeImplicitCondition):
		if reason, ok := mechanismConflictReason(left.NodeText, right.NodeText); ok {
			return reason, true
		}
	case left.NodeKind == string(compile.NodeExplicitCondition) && right.NodeKind == string(compile.NodeExplicitCondition):
		if reason, ok := conditionConflictReason(left.NodeText, right.NodeText); ok {
			return reason, true
		}
	}
	return "", false
}

func mechanismConflictReason(a, b string) (string, bool) {
	a = canonicalNodeText(a)
	b = canonicalNodeText(b)
	for _, pair := range [][2]string{
		{"提升", "削弱"},
		{"增强", "削弱"},
		{"更安全", "更脆弱"},
		{"安全性", "脆弱性"},
	} {
		if strings.ReplaceAll(a, pair[0], pair[1]) == b || strings.ReplaceAll(a, pair[1], pair[0]) == b {
			return "mechanism contradiction", true
		}
		if strings.ReplaceAll(b, pair[0], pair[1]) == a || strings.ReplaceAll(b, pair[1], pair[0]) == a {
			return "mechanism contradiction", true
		}
	}
	return "", false
}

func conditionConflictReason(a, b string) (string, bool) {
	if reason, ok := contradictionReason(a, b); ok {
		return "condition " + reason, true
	}
	return "", false
}

func buildConflictSet(thesis memory.CandidateThesis, leftIDs, rightIDs, leftSourceRefs, rightSourceRefs []string, leftSummary, rightSummary, reason string, now time.Time) memory.ConflictSet {
	return memory.ConflictSet{
		ConflictID:      thesis.ThesisID + "-conflict",
		ThesisID:        thesis.ThesisID,
		ConflictStatus:  "blocked",
		ConflictTopic:   thesis.TopicLabel,
		SideANodeIDs:    append([]string(nil), leftIDs...),
		SideBNodeIDs:    append([]string(nil), rightIDs...),
		SideASourceRefs: append([]string(nil), leftSourceRefs...),
		SideBSourceRefs: append([]string(nil), rightSourceRefs...),
		SideASummary:    strings.TrimSpace(leftSummary),
		SideBSummary:    strings.TrimSpace(rightSummary),
		ConflictReason:  reason,
		SharedQuestion:  thesis.TopicLabel,
		CreatedAt:       now,
		UpdatedAt:       now,
	}
}
