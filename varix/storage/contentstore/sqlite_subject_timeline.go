package contentstore

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/kumaloha/VariX/varix/memory"
	"github.com/kumaloha/VariX/varix/model"
)

func (s *SQLiteStore) BuildSubjectTimeline(ctx context.Context, userID, subject string, now time.Time) (memory.SubjectTimeline, error) {
	userID, err := normalizeRequiredUserID(userID)
	if err != nil {
		return memory.SubjectTimeline{}, err
	}
	subject = strings.TrimSpace(subject)
	if subject == "" {
		return memory.SubjectTimeline{}, fmt.Errorf("subject is required")
	}
	now = normalizeNow(now)
	canonicalSubject, err := s.resolveCanonicalListSubject(ctx, subject)
	if err != nil {
		return memory.SubjectTimeline{}, err
	}
	graphs, err := s.ListMemoryContentGraphsBySubject(ctx, userID, canonicalSubject)
	if err != nil {
		return memory.SubjectTimeline{}, err
	}
	cache := map[string]string{}
	entries := make([]memory.SubjectChangeEntry, 0)
	for _, graph := range graphs {
		for _, node := range graph.Nodes {
			if !isSubjectTimelineNode(node) {
				continue
			}
			nodeSubject, err := s.resolveCanonicalGraphNodeSubject(ctx, node, cache)
			if err != nil {
				return memory.SubjectTimeline{}, err
			}
			if !subjectMatchesTimelineQuery(node, nodeSubject, canonicalSubject) {
				continue
			}
			entries = append(entries, subjectTimelineEntry(graph, node))
		}
	}
	sort.SliceStable(entries, func(i, j int) bool {
		left := subjectTimelineSortKey(entries[i])
		right := subjectTimelineSortKey(entries[j])
		if left != right {
			return left < right
		}
		if entries[i].SourcePlatform != entries[j].SourcePlatform {
			return entries[i].SourcePlatform < entries[j].SourcePlatform
		}
		if entries[i].SourceExternalID != entries[j].SourceExternalID {
			return entries[i].SourceExternalID < entries[j].SourceExternalID
		}
		return entries[i].NodeID < entries[j].NodeID
	})
	for i := range entries {
		entries[i].Structure = classifySubjectChangeStructure(entries[i])
		entries[i].RelationToPrior = classifySubjectChangeRelation(entries[:i], entries[i])
	}
	return memory.SubjectTimeline{
		UserID:           userID,
		Subject:          subject,
		CanonicalSubject: canonicalSubject,
		GeneratedAt:      now,
		Entries:          entries,
		Summary:          summarizeSubjectTimeline(canonicalSubject, entries),
	}, nil
}

func subjectTimelineEntry(graph model.ContentSubgraph, node model.ContentNode) memory.SubjectChangeEntry {
	return memory.SubjectChangeEntry{
		SourcePlatform:     strings.TrimSpace(graph.SourcePlatform),
		SourceExternalID:   strings.TrimSpace(graph.SourceExternalID),
		SourceArticleID:    firstTrimmed(node.SourceArticleID, graph.ArticleID),
		SourceCompiledAt:   strings.TrimSpace(graph.CompiledAt),
		SourceUpdatedAt:    strings.TrimSpace(graph.UpdatedAt),
		NodeID:             strings.TrimSpace(node.ID),
		RawText:            strings.TrimSpace(node.RawText),
		SubjectText:        strings.TrimSpace(node.SubjectText),
		SubjectCanonical:   strings.TrimSpace(node.SubjectCanonical),
		ChangeText:         strings.TrimSpace(node.ChangeText),
		ChangeKind:         strings.TrimSpace(node.ChangeKind),
		ChangeDirection:    strings.TrimSpace(node.ChangeDirection),
		ChangeValue:        node.ChangeValue,
		ChangeUnit:         strings.TrimSpace(node.ChangeUnit),
		TimeText:           strings.TrimSpace(node.TimeText),
		TimeStart:          strings.TrimSpace(node.TimeStart),
		TimeEnd:            strings.TrimSpace(node.TimeEnd),
		TimeBucket:         strings.TrimSpace(node.TimeBucket),
		GraphRole:          string(node.GraphRole),
		IsPrimary:          node.IsPrimary,
		VerificationStatus: string(node.VerificationStatus),
		VerificationReason: strings.TrimSpace(node.VerificationReason),
		VerificationAsOf:   strings.TrimSpace(node.VerificationAsOf),
	}
}

func isSubjectTimelineNode(node model.ContentNode) bool {
	if !node.IsPrimary {
		return false
	}
	switch node.GraphRole {
	case model.GraphRoleDriver, model.GraphRoleTarget:
		return true
	default:
		return false
	}
}

func subjectMatchesTimelineQuery(node model.ContentNode, resolvedNodeSubject, canonicalSubject string) bool {
	canonicalSubject = strings.TrimSpace(canonicalSubject)
	return strings.TrimSpace(resolvedNodeSubject) == canonicalSubject ||
		strings.TrimSpace(node.SubjectText) == canonicalSubject ||
		strings.TrimSpace(node.SubjectCanonical) == canonicalSubject
}

func classifySubjectChangeStructure(entry memory.SubjectChangeEntry) memory.SubjectChangeStructure {
	raw := normalizeSubjectChangeText(firstTrimmed(entry.RawText, entry.ChangeText))
	subject := normalizeSubjectChangeText(entry.SubjectText)
	change := normalizeSubjectChangeText(entry.ChangeText)
	if subject == "" || change == "" {
		return memory.SubjectChangeLowStructure
	}
	if subject == change || (raw != "" && raw == subject && raw == change) {
		return memory.SubjectChangeLowStructure
	}
	return memory.SubjectChangeStructured
}

func classifySubjectChangeRelation(prior []memory.SubjectChangeEntry, current memory.SubjectChangeEntry) memory.SubjectChangeRelation {
	if classifySubjectChangeStructure(current) == memory.SubjectChangeLowStructure {
		return memory.SubjectChangeRelationLowStructure
	}
	currentChange := normalizeSubjectChangeText(current.ChangeText)
	if currentChange == "" || len(prior) == 0 {
		return memory.SubjectChangeNew
	}
	sawStructuredPrior := false
	for i := len(prior) - 1; i >= 0; i-- {
		prev := prior[i]
		if classifySubjectChangeStructure(prev) == memory.SubjectChangeLowStructure {
			continue
		}
		sawStructuredPrior = true
		prevChange := normalizeSubjectChangeText(prev.ChangeText)
		if prevChange == "" {
			continue
		}
		if prevChange == currentChange {
			return memory.SubjectChangeReinforces
		}
		if subjectChangesContradict(prev.ChangeText, current.ChangeText) {
			return memory.SubjectChangeContradicts
		}
		if strings.Contains(currentChange, prevChange) || strings.Contains(prevChange, currentChange) {
			return memory.SubjectChangeUpdates
		}
	}
	if !sawStructuredPrior {
		return memory.SubjectChangeNew
	}
	return memory.SubjectChangeBranches
}

func subjectChangesContradict(a, b string) bool {
	left := normalizeSubjectChangeText(a)
	right := normalizeSubjectChangeText(b)
	opposites := [][2]string{
		{"承压", "反弹"},
		{"承压", "走强"},
		{"下跌", "上涨"},
		{"下降", "上升"},
		{"走弱", "走强"},
		{"流出", "流入"},
		{"收紧", "宽松"},
		{"放缓", "加速"},
		{"被证实", "被反驳"},
		{"proved", "disproved"},
		{"true", "false"},
	}
	for _, pair := range opposites {
		if (strings.Contains(left, pair[0]) && strings.Contains(right, pair[1])) || (strings.Contains(left, pair[1]) && strings.Contains(right, pair[0])) {
			return true
		}
	}
	return false
}

func subjectTimelineSortKey(entry memory.SubjectChangeEntry) string {
	for _, value := range []string{entry.TimeStart, entry.TimeEnd, entry.VerificationAsOf, entry.SourceCompiledAt, entry.SourceUpdatedAt} {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if parsed, err := time.Parse(time.RFC3339, value); err == nil {
			return parsed.UTC().Format(time.RFC3339Nano)
		}
	}
	return strings.Join([]string{entry.SourcePlatform, entry.SourceExternalID, entry.NodeID}, "\x00")
}

func summarizeSubjectTimeline(subject string, entries []memory.SubjectChangeEntry) string {
	subject = strings.TrimSpace(subject)
	if subject == "" {
		subject = "subject"
	}
	if len(entries) == 0 {
		return subject + " has no saved changes."
	}
	latest := entries[len(entries)-1]
	return fmt.Sprintf("%s has %d saved changes; latest: %s.", subject, len(entries), strings.TrimSpace(latest.ChangeText))
}

func normalizeSubjectChangeText(value string) string {
	return strings.Join(strings.Fields(strings.TrimSpace(value)), " ")
}
