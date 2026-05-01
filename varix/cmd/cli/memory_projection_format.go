package main

import (
	"fmt"
	"strings"

	"github.com/kumaloha/VariX/varix/graphmodel"
	"github.com/kumaloha/VariX/varix/storage/contentstore"
)

func formatContentGraphCards(items []graphmodel.ContentSubgraph) string {
	var b strings.Builder
	for _, item := range items {
		primaryCount := 0
		primarySubjects := make([]string, 0)
		for _, node := range item.Nodes {
			if node.IsPrimary {
				primaryCount++
				if strings.TrimSpace(node.SubjectText) != "" {
					primarySubjects = append(primarySubjects, node.SubjectText)
				}
			}
		}
		fmt.Fprintf(&b, "Content Graph\n- Platform: %s\n- External ID: %s\n- Article ID: %s\n- Primary nodes: %d/%d\n", item.SourcePlatform, item.SourceExternalID, item.ArticleID, primaryCount, len(item.Nodes))
		if len(primarySubjects) > 0 {
			seen := map[string]struct{}{}
			uniq := make([]string, 0, len(primarySubjects))
			for _, subject := range primarySubjects {
				if _, ok := seen[subject]; ok {
					continue
				}
				seen[subject] = struct{}{}
				uniq = append(uniq, subject)
			}
			fmt.Fprintf(&b, "- Subjects: %s\n", strings.Join(uniq, ", "))
		}
		b.WriteString("\n")
	}
	return b.String()
}
func formatEventGraphCards(items []contentstore.EventGraphRecord) string {
	var b strings.Builder
	for _, item := range items {
		fmt.Fprintf(&b, "Event Graph\n- Scope: %s\n- Anchor: %s\n- Time bucket: %s\n", item.Scope, item.AnchorSubject, item.TimeBucket)
		if timeRange := formatEventGraphTimeRange(item); timeRange != "" {
			fmt.Fprintf(&b, "- Time: %s\n", timeRange)
		}
		if len(item.RepresentativeChanges) > 0 {
			fmt.Fprintf(&b, "- Representative changes: %s\n", strings.Join(item.RepresentativeChanges, ", "))
		}
		fmt.Fprintf(&b, "- Verification: %v\n\n", item.VerificationSummary)
	}
	return b.String()
}
func formatEventGraphTimeRange(item contentstore.EventGraphRecord) string {
	start := strings.TrimSpace(item.TimeStart)
	end := strings.TrimSpace(item.TimeEnd)
	switch {
	case start == "" && end == "":
		return ""
	case start == "" || start == end:
		return firstNonEmpty(end, start)
	case end == "":
		return start
	default:
		return start + " -> " + end
	}
}
func formatParadigmCards(items []contentstore.ParadigmRecord) string {
	var b strings.Builder
	for _, item := range items {
		fmt.Fprintf(&b, "Paradigm\n- Driver: %s\n- Target: %s\n- Time bucket: %s\n- Credibility: %s (%.1f)\n", item.DriverSubject, item.TargetSubject, item.TimeBucket, item.CredibilityState, item.CredibilityScore)
		if len(item.RepresentativeChanges) > 0 {
			fmt.Fprintf(&b, "- Representative changes: %s\n", strings.Join(item.RepresentativeChanges, ", "))
		}
		fmt.Fprintf(&b, "- Success/Failure: %d/%d\n\n", item.SuccessCount, item.FailureCount)
	}
	return b.String()
}
func formatEventEvidenceCards(items []contentstore.EventGraphEvidenceLink) string {
	var b strings.Builder
	for _, item := range items {
		fmt.Fprintf(&b, "Event Evidence\n- event_graph_id: %s\n- subgraph_id: %s\n- node_id: %s\n\n", item.EventGraphID, item.SubgraphID, item.NodeID)
	}
	return b.String()
}
func formatParadigmEvidenceCards(items []contentstore.ParadigmEvidenceLink) string {
	var b strings.Builder
	for _, item := range items {
		fmt.Fprintf(&b, "Paradigm Evidence\n- paradigm_id: %s\n- event_graph_id: %s\n- subgraph_id: %s\n\n", item.ParadigmID, item.EventGraphID, item.SubgraphID)
	}
	return b.String()
}
