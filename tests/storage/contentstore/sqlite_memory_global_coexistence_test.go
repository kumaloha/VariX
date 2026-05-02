package contentstore

import (
	"context"
	"path/filepath"
	"reflect"
	"sort"
	"testing"
	"time"

	"github.com/kumaloha/VariX/varix/memory"
	"github.com/kumaloha/VariX/varix/model"
)

func TestSQLiteStore_GlobalMemoryClusterAndSynthesisCoexistWithoutMutatingAcceptedState(t *testing.T) {
	root := t.TempDir()
	store, err := NewSQLiteStore(filepath.Join(root, "data", "content.db"))
	if err != nil {
		t.Fatalf("NewSQLiteStore() error = %v", err)
	}
	defer store.Close()

	predictionStart := time.Date(2026, 4, 12, 0, 0, 0, 0, time.UTC)
	predictionDue := time.Date(2026, 4, 20, 0, 0, 0, 0, time.UTC)
	record := model.Record{
		UnitID:         "weibo:CO1",
		Source:         "weibo",
		ExternalID:     "CO1",
		RootExternalID: "CO1",
		Model:          "qwen3.6-plus",
		Output: model.Output{
			Summary: "summary",
			Graph: model.ReasoningGraph{
				Nodes: []model.GraphNode{
					{ID: "n1", Kind: model.NodeFact, Text: "流动性收紧", OccurredAt: time.Date(2026, 4, 10, 0, 0, 0, 0, time.UTC)},
					{ID: "n2", Kind: model.NodeConclusion, Text: "风险资产承压", OccurredAt: time.Date(2026, 4, 10, 0, 0, 0, 0, time.UTC)},
					{ID: "n3", Kind: model.NodePrediction, Text: "未来数月波动加大", PredictionStartAt: predictionStart, PredictionDueAt: predictionDue},
				},
				Edges: []model.GraphEdge{
					{From: "n1", To: "n2", Kind: model.EdgeDerives},
					{From: "n2", To: "n3", Kind: model.EdgeDerives},
				},
			},
			Details:    model.HiddenDetails{Caveats: []string{"detail"}},
			Confidence: "medium",
		},
		CompiledAt: time.Date(2026, 4, 14, 8, 0, 0, 0, time.UTC),
	}
	if err := store.UpsertCompiledOutput(context.Background(), record); err != nil {
		t.Fatalf("UpsertCompiledOutput() error = %v", err)
	}
	if _, err := store.AcceptMemoryNodes(context.Background(), memory.AcceptRequest{
		UserID:           "u-coexist",
		SourcePlatform:   "weibo",
		SourceExternalID: "CO1",
		NodeIDs:          []string{"n1", "n2", "n3"},
	}); err != nil {
		t.Fatalf("AcceptMemoryNodes() error = %v", err)
	}

	acceptedBefore, err := store.ListUserMemory(context.Background(), "u-coexist")
	if err != nil {
		t.Fatalf("ListUserMemory(before) error = %v", err)
	}
	beforeMemoryCount := tableCount(t, store, "user_memory_nodes")
	beforeEventCount := tableCount(t, store, "memory_acceptance_events")
	beforeJobCount := tableCount(t, store, "memory_organization_jobs")

	now := time.Date(2026, 4, 25, 0, 0, 0, 0, time.UTC)
	cluster, err := store.RunGlobalMemoryOrganization(context.Background(), "u-coexist", now)
	if err != nil {
		t.Fatalf("RunGlobalMemoryOrganization() error = %v", err)
	}
	synthesis, err := store.RunGlobalMemorySynthesis(context.Background(), "u-coexist", now)
	if err != nil {
		t.Fatalf("RunGlobalMemorySynthesis() error = %v", err)
	}

	acceptedAfter, err := store.ListUserMemory(context.Background(), "u-coexist")
	if err != nil {
		t.Fatalf("ListUserMemory(after) error = %v", err)
	}
	if got := tableCount(t, store, "user_memory_nodes"); got != beforeMemoryCount {
		t.Fatalf("user_memory_nodes count = %d, want unchanged %d", got, beforeMemoryCount)
	}
	if got := tableCount(t, store, "memory_acceptance_events"); got != beforeEventCount {
		t.Fatalf("memory_acceptance_events count = %d, want unchanged %d", got, beforeEventCount)
	}
	if got := tableCount(t, store, "memory_organization_jobs"); got != beforeJobCount {
		t.Fatalf("memory_organization_jobs count = %d, want unchanged %d", got, beforeJobCount)
	}
	if got := tableCount(t, store, "global_memory_organization_outputs"); got != 1 {
		t.Fatalf("global_memory_organization_outputs count = %d, want 1", got)
	}
	if got := tableCount(t, store, "global_memory_synthesis_outputs"); got != 1 {
		t.Fatalf("global_memory_synthesis_outputs count = %d, want 1", got)
	}
	if !reflect.DeepEqual(acceptedBefore, acceptedAfter) {
		t.Fatalf("accepted memory mutated by global organizers\nbefore=%#v\nafter=%#v", acceptedBefore, acceptedAfter)
	}

	gotCluster, err := store.GetLatestGlobalMemoryOrganizationOutput(context.Background(), "u-coexist")
	if err != nil {
		t.Fatalf("GetLatestGlobalMemoryOrganizationOutput() error = %v", err)
	}
	gotSynthesis, err := store.GetLatestGlobalMemorySynthesisOutput(context.Background(), "u-coexist")
	if err != nil {
		t.Fatalf("GetLatestGlobalMemorySynthesisOutput() error = %v", err)
	}

	wantActive := []string{"weibo:CO1:n1", "weibo:CO1:n2"}
	wantInactive := []string{"weibo:CO1:n3"}
	for name, got := range map[string][]string{
		"cluster run active":        globalAcceptedNodeRefs(cluster.ActiveNodes),
		"cluster run inactive":      globalAcceptedNodeRefs(cluster.InactiveNodes),
		"synthesis run active":      candidateThesisNodeRefs(synthesis.CandidateTheses),
		"synthesis run inactive":    diffNodeRefs(globalAcceptedNodeRefs(acceptedAfter), candidateThesisNodeRefs(synthesis.CandidateTheses)),
		"latest cluster active":     globalAcceptedNodeRefs(gotCluster.ActiveNodes),
		"latest cluster inactive":   globalAcceptedNodeRefs(gotCluster.InactiveNodes),
		"latest synthesis active":   candidateThesisNodeRefs(gotSynthesis.CandidateTheses),
		"latest synthesis inactive": diffNodeRefs(globalAcceptedNodeRefs(acceptedAfter), candidateThesisNodeRefs(gotSynthesis.CandidateTheses)),
	} {
		switch name {
		case "cluster run active", "synthesis run active", "latest cluster active", "latest synthesis active":
			if !reflect.DeepEqual(got, wantActive) {
				t.Fatalf("%s = %#v, want %#v", name, got, wantActive)
			}
		case "cluster run inactive", "synthesis run inactive", "latest cluster inactive", "latest synthesis inactive":
			if !reflect.DeepEqual(got, wantInactive) {
				t.Fatalf("%s = %#v, want %#v", name, got, wantInactive)
			}
		}
	}

	if gotCluster.OutputID == 0 || gotSynthesis.OutputID == 0 {
		t.Fatalf("output ids = %d / %d, want both persisted", gotCluster.OutputID, gotSynthesis.OutputID)
	}
}

func tableCount(t *testing.T, store *SQLiteStore, table string) int {
	t.Helper()
	var count int
	if err := store.db.QueryRow(`SELECT COUNT(*) FROM ` + table).Scan(&count); err != nil {
		t.Fatalf("COUNT(%s) error = %v", table, err)
	}
	return count
}

func globalAcceptedNodeRefs(nodes []memory.AcceptedNode) []string {
	refs := make([]string, 0, len(nodes))
	for _, node := range nodes {
		if _, _, _, ok := splitGlobalNodeRef(node.NodeID); ok {
			refs = append(refs, node.NodeID)
			continue
		}
		refs = append(refs, globalMemoryNodeRef(node))
	}
	sort.Strings(refs)
	return refs
}

func candidateThesisNodeRefs(theses []memory.CandidateThesis) []string {
	seen := make(map[string]struct{})
	for _, thesis := range theses {
		for _, ref := range thesis.NodeIDs {
			seen[ref] = struct{}{}
		}
	}
	refs := make([]string, 0, len(seen))
	for ref := range seen {
		refs = append(refs, ref)
	}
	sort.Strings(refs)
	return refs
}

func diffNodeRefs(all, subset []string) []string {
	subsetSet := make(map[string]struct{}, len(subset))
	for _, ref := range subset {
		subsetSet[ref] = struct{}{}
	}
	out := make([]string, 0, len(all))
	for _, ref := range all {
		if _, ok := subsetSet[ref]; ok {
			continue
		}
		out = append(out, ref)
	}
	sort.Strings(out)
	return out
}
