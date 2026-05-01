package contentstore

import (
	"github.com/kumaloha/VariX/varix/graphmodel"
	"github.com/kumaloha/VariX/varix/memory"
	"sort"
	"strings"
)

func addSubjectHorizonDrivers(clusters map[string]*memory.SubjectHorizonDriver, graph graphmodel.ContentSubgraph, drivers []graphmodel.GraphNode, target graphmodel.GraphNode, sourceRef string) {
	for _, driver := range drivers {
		subject := firstTrimmed(driver.SubjectCanonical, driver.SubjectText)
		if subject == "" {
			continue
		}
		cluster := clusters[subject]
		if cluster == nil {
			cluster = &memory.SubjectHorizonDriver{Subject: subject}
			clusters[subject] = cluster
		}
		if change := strings.TrimSpace(driver.ChangeText); change != "" {
			cluster.Changes = uniqueStrings(append(cluster.Changes, change))
		}
		if path := subjectHorizonRelationPath(graph, driver.ID, target.ID); path != "" {
			cluster.RelationPaths = uniqueStrings(append(cluster.RelationPaths, path))
		}
		cluster.SourceRefs = uniqueStrings(append(cluster.SourceRefs, sourceRef+"#"+driver.ID))
		cluster.Count++
	}
}

func subjectHorizonRelationPath(graph graphmodel.ContentSubgraph, fromID, toID string) string {
	fromID = strings.TrimSpace(fromID)
	toID = strings.TrimSpace(toID)
	if fromID == "" || toID == "" || fromID == toID {
		return ""
	}
	nodes := map[string]graphmodel.GraphNode{}
	for _, node := range graph.Nodes {
		nodes[node.ID] = node
	}
	if _, ok := nodes[fromID]; !ok {
		return ""
	}
	if _, ok := nodes[toID]; !ok {
		return ""
	}
	adj := map[string][]string{}
	for _, edge := range graph.Edges {
		switch edge.Type {
		case graphmodel.EdgeTypeDrives, graphmodel.EdgeTypeExplains, graphmodel.EdgeTypeSupports:
		default:
			continue
		}
		if strings.TrimSpace(edge.From) == "" || strings.TrimSpace(edge.To) == "" {
			continue
		}
		adj[edge.From] = append(adj[edge.From], edge.To)
	}
	type queueItem struct {
		id   string
		path []string
	}
	queue := []queueItem{{id: fromID, path: []string{fromID}}}
	seen := map[string]struct{}{fromID: {}}
	for len(queue) > 0 {
		item := queue[0]
		queue = queue[1:]
		if len(item.path) > 6 {
			continue
		}
		for _, next := range adj[item.id] {
			if _, ok := seen[next]; ok {
				continue
			}
			nextPath := append(append([]string(nil), item.path...), next)
			if next == toID {
				return subjectHorizonPathLabel(nodes, nextPath)
			}
			seen[next] = struct{}{}
			queue = append(queue, queueItem{id: next, path: nextPath})
		}
	}
	return ""
}

func subjectHorizonPathLabel(nodes map[string]graphmodel.GraphNode, path []string) string {
	labels := make([]string, 0, len(path))
	for _, id := range path {
		node := nodes[id]
		label := firstTrimmed(node.SubjectCanonical, node.SubjectText, node.ChangeText)
		if label == "" {
			return ""
		}
		labels = append(labels, label)
	}
	return strings.Join(labels, " -> ")
}

func subjectHorizonDriverList(clusters map[string]*memory.SubjectHorizonDriver) []memory.SubjectHorizonDriver {
	out := make([]memory.SubjectHorizonDriver, 0, len(clusters))
	for _, cluster := range clusters {
		out = append(out, *cluster)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Count != out[j].Count {
			return out[i].Count > out[j].Count
		}
		return out[i].Subject < out[j].Subject
	})
	return out
}
