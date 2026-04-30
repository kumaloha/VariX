package contentstore

import (
	"context"
	"testing"
	"time"

	"github.com/kumaloha/VariX/varix/graphmodel"
)

func TestSQLiteStore_GetSubjectHorizonMemoryBuildsRollingWindowAbstraction(t *testing.T) {
	store := newSubjectTimelineTestStore(t)
	now := time.Date(2026, 4, 30, 12, 0, 0, 0, time.UTC)
	old := now.AddDate(0, 0, -10)
	recent := now.AddDate(0, 0, -2)
	graphs := []graphmodel.ContentSubgraph{
		subjectTimelineSubgraph("old", old, []graphmodel.GraphNode{
			subjectHorizonNode("old-driver", "old", "科技股", "财报强劲", old, graphmodel.GraphRoleDriver),
			subjectHorizonNode("old-target", "old", "美股", "刷新历史高位", old, graphmodel.GraphRoleTarget),
		}),
		subjectTimelineSubgraph("recent", recent, []graphmodel.GraphNode{
			subjectHorizonNode("recent-driver", "recent", "油价", "油价继续上涨", recent, graphmodel.GraphRoleDriver),
			subjectHorizonNode("recent-target", "recent", "美股", "从纪录高位回落", recent, graphmodel.GraphRoleTarget),
		}),
	}
	for _, graph := range graphs {
		if err := store.PersistMemoryContentGraph(context.Background(), "u-horizon", graph, now); err != nil {
			t.Fatalf("PersistMemoryContentGraph(%s) error = %v", graph.ID, err)
		}
	}

	out, err := store.GetSubjectHorizonMemory(context.Background(), "u-horizon", "美股", "1w", now, true)
	if err != nil {
		t.Fatalf("GetSubjectHorizonMemory() error = %v", err)
	}
	if out.Horizon != "1w" || out.RefreshPolicy != "daily" {
		t.Fatalf("horizon/policy = %q/%q, want 1w/daily", out.Horizon, out.RefreshPolicy)
	}
	if out.WindowStart != now.AddDate(0, 0, -7).Format(time.RFC3339) || out.WindowEnd != now.Format(time.RFC3339) {
		t.Fatalf("window = %s..%s, want rolling 1w ending at now", out.WindowStart, out.WindowEnd)
	}
	if out.SampleCount != 1 || len(out.KeyChanges) != 1 || out.KeyChanges[0].ChangeText != "从纪录高位回落" {
		t.Fatalf("key changes = %#v sample=%d, want only recent target change", out.KeyChanges, out.SampleCount)
	}
	if len(out.DriverClusters) != 1 || out.DriverClusters[0].Subject != "油价" {
		t.Fatalf("drivers = %#v, want oil driver cluster", out.DriverClusters)
	}
	if out.NextRefreshAt != now.AddDate(0, 0, 1).Format(time.RFC3339) {
		t.Fatalf("NextRefreshAt = %s, want next day", out.NextRefreshAt)
	}
}

func TestSQLiteStore_GetSubjectHorizonMemoryReusesFreshCache(t *testing.T) {
	store := newSubjectTimelineTestStore(t)
	now := time.Date(2026, 4, 30, 12, 0, 0, 0, time.UTC)
	graph := subjectTimelineSubgraph("cached", now, []graphmodel.GraphNode{
		subjectHorizonNode("driver", "cached", "科技股", "科技股走强", now, graphmodel.GraphRoleDriver),
		subjectHorizonNode("target", "cached", "美股", "继续创新高", now, graphmodel.GraphRoleTarget),
	})
	if err := store.PersistMemoryContentGraph(context.Background(), "u-horizon-cache", graph, now); err != nil {
		t.Fatalf("PersistMemoryContentGraph() error = %v", err)
	}

	first, err := store.GetSubjectHorizonMemory(context.Background(), "u-horizon-cache", "美股", "1m", now, true)
	if err != nil {
		t.Fatalf("first GetSubjectHorizonMemory() error = %v", err)
	}
	second, err := store.GetSubjectHorizonMemory(context.Background(), "u-horizon-cache", "美股", "1m", now.AddDate(0, 0, 2), false)
	if err != nil {
		t.Fatalf("second GetSubjectHorizonMemory() error = %v", err)
	}
	if second.GeneratedAt != first.GeneratedAt || second.CacheStatus != "fresh" {
		t.Fatalf("cache result generated/status = %s/%s, want reused %s/fresh", second.GeneratedAt, second.CacheStatus, first.GeneratedAt)
	}
	if second.RefreshPolicy != "weekly" {
		t.Fatalf("RefreshPolicy = %q, want weekly for 1m", second.RefreshPolicy)
	}
}

func TestSQLiteStore_GetSubjectHorizonMemoryClassifiesRelationsAfterSortingByTime(t *testing.T) {
	store := newSubjectTimelineTestStore(t)
	now := time.Date(2026, 4, 30, 12, 0, 0, 0, time.UTC)
	early := now.AddDate(0, 0, -3)
	late := now.AddDate(0, 0, -2)
	for _, graph := range []graphmodel.ContentSubgraph{
		subjectTimelineSubgraph("z-early", early, []graphmodel.GraphNode{
			subjectHorizonNode("target", "z-early", "美股", "上涨并刷新高位", early, graphmodel.GraphRoleTarget),
		}),
		subjectTimelineSubgraph("a-late", late, []graphmodel.GraphNode{
			subjectHorizonNode("target", "a-late", "美股", "下跌并从高位回落", late, graphmodel.GraphRoleTarget),
		}),
	} {
		if err := store.PersistMemoryContentGraph(context.Background(), "u-horizon-relation", graph, now); err != nil {
			t.Fatalf("PersistMemoryContentGraph(%s) error = %v", graph.ID, err)
		}
	}

	out, err := store.GetSubjectHorizonMemory(context.Background(), "u-horizon-relation", "美股", "1w", now, true)
	if err != nil {
		t.Fatalf("GetSubjectHorizonMemory() error = %v", err)
	}
	if len(out.KeyChanges) != 2 {
		t.Fatalf("KeyChanges = %#v, want 2", out.KeyChanges)
	}
	if out.KeyChanges[0].ChangeText != "上涨并刷新高位" || out.KeyChanges[0].RelationToPrior != "new" {
		t.Fatalf("first change = %#v, want early new change", out.KeyChanges[0])
	}
	if out.KeyChanges[1].ChangeText != "下跌并从高位回落" || out.KeyChanges[1].RelationToPrior != "contradicts" {
		t.Fatalf("second change = %#v, want late contradicts change", out.KeyChanges[1])
	}
}

func TestSQLiteStore_GetSubjectHorizonMemoryRejectsUnsupportedHorizon(t *testing.T) {
	store := newSubjectTimelineTestStore(t)
	_, err := store.GetSubjectHorizonMemory(context.Background(), "u-horizon-bad", "美股", "10y", time.Date(2026, 4, 30, 0, 0, 0, 0, time.UTC), false)
	if err == nil {
		t.Fatal("GetSubjectHorizonMemory(10y) error = nil, want unsupported horizon error")
	}
}

func TestSQLiteStore_GetSubjectHorizonMemorySupportsDefaultRefreshCadence(t *testing.T) {
	store := newSubjectTimelineTestStore(t)
	now := time.Date(2026, 4, 30, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		horizon string
		policy  string
		next    time.Time
	}{
		{"1w", "daily", now.AddDate(0, 0, 1)},
		{"1m", "weekly", now.AddDate(0, 0, 7)},
		{"1q", "monthly", now.AddDate(0, 1, 0)},
		{"1y", "quarterly", now.AddDate(0, 3, 0)},
		{"2y", "semiannual", now.AddDate(0, 6, 0)},
		{"5y", "annual", now.AddDate(1, 0, 0)},
	}
	for _, tt := range tests {
		out, err := store.GetSubjectHorizonMemory(context.Background(), "u-horizon-cadence", "美股", tt.horizon, now, true)
		if err != nil {
			t.Fatalf("GetSubjectHorizonMemory(%s) error = %v", tt.horizon, err)
		}
		if out.RefreshPolicy != tt.policy || out.NextRefreshAt != tt.next.Format(time.RFC3339) {
			t.Fatalf("%s policy/next = %s/%s, want %s/%s", tt.horizon, out.RefreshPolicy, out.NextRefreshAt, tt.policy, tt.next.Format(time.RFC3339))
		}
	}
}

func subjectHorizonNode(id, sourceID, subject, change string, at time.Time, role graphmodel.GraphRole) graphmodel.GraphNode {
	return graphmodel.GraphNode{
		ID:                 id,
		SourceArticleID:    sourceID,
		SourcePlatform:     "twitter",
		SourceExternalID:   sourceID,
		RawText:            subject + "：" + change,
		SubjectText:        subject,
		ChangeText:         change,
		TimeStart:          at.Format(time.RFC3339),
		Kind:               graphmodel.NodeKindObservation,
		GraphRole:          role,
		IsPrimary:          true,
		VerificationStatus: graphmodel.VerificationProved,
	}
}
