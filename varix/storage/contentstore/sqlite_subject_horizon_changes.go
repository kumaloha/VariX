package contentstore

import (
	"github.com/kumaloha/VariX/varix/graphmodel"
	"github.com/kumaloha/VariX/varix/memory"
	"strings"
	"time"
)

func classifySubjectHorizonChangeRelation(prior []memory.SubjectHorizonChange, current memory.SubjectHorizonChange) memory.SubjectChangeRelation {
	entries := make([]memory.SubjectChangeEntry, 0, len(prior))
	for _, item := range prior {
		entries = append(entries, memory.SubjectChangeEntry{ChangeText: item.ChangeText, SubjectText: item.Subject})
	}
	return classifySubjectChangeRelation(entries, memory.SubjectChangeEntry{SubjectText: current.Subject, ChangeText: current.ChangeText})
}

func subjectHorizonEntryTime(graph graphmodel.ContentSubgraph, node graphmodel.GraphNode) (time.Time, bool) {
	for _, value := range []string{node.TimeStart, node.TimeEnd, node.VerificationAsOf, graph.CompiledAt, graph.UpdatedAt} {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if parsed, err := time.Parse(time.RFC3339, value); err == nil {
			return parsed.UTC(), true
		}
	}
	return time.Time{}, false
}

func primaryGraphDrivers(graph graphmodel.ContentSubgraph) []graphmodel.GraphNode {
	drivers := make([]graphmodel.GraphNode, 0)
	for _, node := range graph.Nodes {
		if node.IsPrimary && node.GraphRole == graphmodel.GraphRoleDriver {
			drivers = append(drivers, node)
		}
	}
	return drivers
}
