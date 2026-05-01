package contentstore

import (
	"context"
	"github.com/kumaloha/VariX/varix/graphmodel"
	"github.com/kumaloha/VariX/varix/memory"
	"sort"
	"time"
)

func (s *SQLiteStore) buildSubjectHorizonMemoryFromGraphs(ctx context.Context, userID, subject, canonicalSubject string, spec subjectHorizonSpec, now time.Time, graphs []graphmodel.ContentSubgraph, cache map[string]string, resolveSubject projectionSubjectResolver) (memory.SubjectHorizonMemory, error) {
	windowStart := spec.WindowStart(now).UTC()
	windowEnd := now.UTC()
	if cache == nil {
		cache = map[string]string{}
	}
	if resolveSubject == nil {
		resolveSubject = func(ctx context.Context, node graphmodel.GraphNode) (string, error) {
			return s.resolveCanonicalGraphNodeSubject(ctx, node, cache)
		}
	}
	keyChanges := make([]memory.SubjectHorizonChange, 0)
	driverClusters := map[string]*memory.SubjectHorizonDriver{}
	evidenceRefs := make([]string, 0)
	contradictions := make([]memory.SubjectHorizonConflict, 0)
	sourceSet := map[string]struct{}{}
	for _, graph := range graphs {
		graphDrivers := primaryGraphDrivers(graph)
		for _, node := range graph.Nodes {
			if !isSubjectTimelineNode(node) {
				continue
			}
			nodeSubject, err := resolveSubject(ctx, node)
			if err != nil {
				return memory.SubjectHorizonMemory{}, err
			}
			if !subjectMatchesTimelineQuery(node, nodeSubject, canonicalSubject) {
				continue
			}
			when, ok := subjectHorizonEntryTime(graph, node)
			if !ok || when.Before(windowStart) || when.After(windowEnd) {
				continue
			}
			entry := subjectTimelineEntry(graph, node)
			change := memory.SubjectHorizonChange{
				When:             when.UTC().Format(time.RFC3339),
				Subject:          firstTrimmed(entry.SubjectCanonical, entry.SubjectText),
				ChangeText:       entry.ChangeText,
				SourcePlatform:   entry.SourcePlatform,
				SourceExternalID: entry.SourceExternalID,
				NodeID:           entry.NodeID,
			}
			keyChanges = append(keyChanges, change)
			ref := entry.SourcePlatform + ":" + entry.SourceExternalID + "#" + entry.NodeID
			evidenceRefs = append(evidenceRefs, ref)
			sourceSet[entry.SourcePlatform+":"+entry.SourceExternalID] = struct{}{}
			addSubjectHorizonDrivers(driverClusters, graph, graphDrivers, node, entry.SourcePlatform+":"+entry.SourceExternalID)
		}
	}
	sort.SliceStable(keyChanges, func(i, j int) bool {
		if keyChanges[i].When != keyChanges[j].When {
			return keyChanges[i].When < keyChanges[j].When
		}
		return keyChanges[i].SourceExternalID < keyChanges[j].SourceExternalID
	})
	for i := range keyChanges {
		keyChanges[i].RelationToPrior = classifySubjectHorizonChangeRelation(keyChanges[:i], keyChanges[i])
		if keyChanges[i].RelationToPrior == memory.SubjectChangeContradicts && i > 0 {
			contradictions = append(contradictions, memory.SubjectHorizonConflict{PreviousChange: keyChanges[i-1].ChangeText, CurrentChange: keyChanges[i].ChangeText, At: keyChanges[i].When})
		}
	}
	drivers := subjectHorizonDriverList(driverClusters)
	generatedAt := now.UTC().Format(time.RFC3339)
	out := memory.SubjectHorizonMemory{
		UserID:             userID,
		Subject:            subject,
		CanonicalSubject:   canonicalSubject,
		Horizon:            spec.Horizon,
		RefreshPolicy:      spec.RefreshPolicy,
		WindowStart:        windowStart.Format(time.RFC3339),
		WindowEnd:          windowEnd.Format(time.RFC3339),
		GeneratedAt:        generatedAt,
		LastRefreshedAt:    generatedAt,
		NextRefreshAt:      spec.NextRefresh(now).UTC().Format(time.RFC3339),
		CacheStatus:        "refreshed",
		SampleCount:        len(keyChanges),
		SourceCount:        len(sourceSet),
		KeyChanges:         keyChanges,
		DriverClusters:     drivers,
		Contradictions:     contradictions,
		EvidenceSourceRefs: uniqueStrings(evidenceRefs),
	}
	out.TrendDirection, out.VolatilityState, out.DominantPattern = summarizeSubjectHorizonPattern(keyChanges)
	out.Abstraction = summarizeSubjectHorizonAbstraction(out)
	out.InputHash = subjectHorizonInputHash(out)
	return out, nil
}
