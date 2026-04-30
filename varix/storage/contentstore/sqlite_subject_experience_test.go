package contentstore

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/kumaloha/VariX/varix/graphmodel"
	"github.com/kumaloha/VariX/varix/memory"
)

func TestSQLiteStore_GetSubjectExperienceMemoryDerivesReusableDriverLessonsAcrossHorizons(t *testing.T) {
	store := newSubjectTimelineTestStore(t)
	now := time.Date(2026, 4, 30, 12, 0, 0, 0, time.UTC)
	recent := now.AddDate(0, 0, -2)
	older := now.AddDate(0, 0, -20)
	for _, graph := range []graphmodel.ContentSubgraph{
		subjectTimelineSubgraph("experience-recent", recent, []graphmodel.GraphNode{
			subjectHorizonNode("recent-driver", "experience-recent", "油价", "油价上行压制风险偏好", recent, graphmodel.GraphRoleDriver),
			subjectHorizonNode("recent-target", "experience-recent", "美股", "从纪录高位回落", recent, graphmodel.GraphRoleTarget),
		}),
		subjectTimelineSubgraph("experience-older", older, []graphmodel.GraphNode{
			subjectHorizonNode("older-driver", "experience-older", "油价", "能源价格冲击估值", older, graphmodel.GraphRoleDriver),
			subjectHorizonNode("older-target", "experience-older", "美股", "估值承压", older, graphmodel.GraphRoleTarget),
		}),
	} {
		if err := store.PersistMemoryContentGraph(context.Background(), "u-experience", graph, now); err != nil {
			t.Fatalf("PersistMemoryContentGraph(%s) error = %v", graph.ID, err)
		}
	}

	out, err := store.GetSubjectExperienceMemory(context.Background(), "u-experience", "美股", []string{"1w", "1m"}, now, true)
	if err != nil {
		t.Fatalf("GetSubjectExperienceMemory() error = %v", err)
	}
	if out.Subject != "美股" || out.CanonicalSubject != "美股" {
		t.Fatalf("subject = %q canonical=%q, want 美股", out.Subject, out.CanonicalSubject)
	}
	if len(out.Horizons) != 2 || out.Horizons[0] != "1w" || out.Horizons[1] != "1m" {
		t.Fatalf("horizons = %#v, want 1w,1m", out.Horizons)
	}
	if out.LessonCount == 0 || len(out.Lessons) == 0 {
		t.Fatalf("lessons = %#v, want at least one reusable experience lesson", out.Lessons)
	}
	if out.AttributionSummary.PrimaryFactor.Subject != "油价" {
		t.Fatalf("primary factor = %#v, want oil", out.AttributionSummary.PrimaryFactor)
	}
	if out.AttributionSummary.ChangeCount != 2 || len(out.AttributionSummary.ChangeAttributions) != 2 {
		t.Fatalf("change attribution = %#v, want two attributed changes", out.AttributionSummary.ChangeAttributions)
	}
	var found bool
	for _, lesson := range out.Lessons {
		if lesson.Kind == "driver-pattern" && strings.Contains(lesson.Statement, "油价") && containsString(lesson.Horizons, "1w") && containsString(lesson.Horizons, "1m") {
			if lesson.Mechanism == "" || lesson.Boundary == "" || lesson.TransferRule == "" || lesson.TimeScaleMeaning == "" {
				t.Fatalf("oil lesson = %#v, want mechanism, boundary, transfer rule, and time-scale meaning", lesson)
			}
			if strings.Contains(lesson.Mechanism, "CPI") || strings.Contains(lesson.Mechanism, "利率") {
				t.Fatalf("oil lesson mechanism = %q, should not invent CPI/rate chain without graph path evidence", lesson.Mechanism)
			}
			if !strings.Contains(lesson.Mechanism, "关系未建立") || !strings.Contains(lesson.Boundary, "路径证据") {
				t.Fatalf("oil lesson mechanism/boundary = %q/%q, want conservative relation status with boundary", lesson.Mechanism, lesson.Boundary)
			}
			found = true
		}
	}
	if !found {
		t.Fatalf("lessons = %#v, want driver-pattern lesson for oil across 1w and 1m", out.Lessons)
	}
	if out.CacheStatus != "refreshed" || out.InputHash == "" {
		t.Fatalf("cache/hash = %s/%s, want refreshed with input hash", out.CacheStatus, out.InputHash)
	}
}

func TestSQLiteStore_GetSubjectExperienceMemoryUsesGraphPathWhenMechanismEvidenceExists(t *testing.T) {
	store := newSubjectTimelineTestStore(t)
	now := time.Date(2026, 4, 30, 12, 0, 0, 0, time.UTC)
	graph := subjectTimelineSubgraph("experience-path", now, []graphmodel.GraphNode{
		subjectHorizonNode("oil", "experience-path", "油价", "上涨", now, graphmodel.GraphRoleDriver),
		subjectHorizonNode("cpi", "experience-path", "CPI", "通胀预期升温", now, graphmodel.GraphRoleIntermediate),
		subjectHorizonNode("rate", "experience-path", "利率", "降息预期下降", now, graphmodel.GraphRoleIntermediate),
		subjectHorizonNode("target", "experience-path", "美股", "估值承压", now, graphmodel.GraphRoleTarget),
	})
	graph.Edges = []graphmodel.GraphEdge{
		{ID: "e1", From: "oil", To: "cpi", Type: graphmodel.EdgeTypeDrives, IsPrimary: true},
		{ID: "e2", From: "cpi", To: "rate", Type: graphmodel.EdgeTypeDrives, IsPrimary: true},
		{ID: "e3", From: "rate", To: "target", Type: graphmodel.EdgeTypeDrives, IsPrimary: true},
	}
	if err := store.PersistMemoryContentGraph(context.Background(), "u-experience-path", graph, now); err != nil {
		t.Fatalf("PersistMemoryContentGraph() error = %v", err)
	}

	out, err := store.GetSubjectExperienceMemory(context.Background(), "u-experience-path", "美股", []string{"1w", "1m"}, now, true)
	if err != nil {
		t.Fatalf("GetSubjectExperienceMemory() error = %v", err)
	}
	var found bool
	for _, lesson := range out.Lessons {
		if lesson.Kind == "driver-pattern" && strings.Contains(lesson.Statement, "油价") {
			if !strings.Contains(lesson.Mechanism, "记忆路径") || !strings.Contains(lesson.Mechanism, "CPI") || !strings.Contains(lesson.Mechanism, "利率") {
				t.Fatalf("oil mechanism = %q, want evidence-backed graph path with CPI and rate", lesson.Mechanism)
			}
			found = true
		}
	}
	if !found {
		t.Fatalf("lessons = %#v, want oil path lesson", out.Lessons)
	}
}

func TestSQLiteStore_GetSubjectExperienceMemoryReusesFreshCacheWhenHorizonInputsUnchanged(t *testing.T) {
	store := newSubjectTimelineTestStore(t)
	now := time.Date(2026, 4, 30, 12, 0, 0, 0, time.UTC)
	graph := subjectTimelineSubgraph("experience-cache", now, []graphmodel.GraphNode{
		subjectHorizonNode("driver", "experience-cache", "科技股", "科技股走强", now, graphmodel.GraphRoleDriver),
		subjectHorizonNode("target", "experience-cache", "美股", "继续创新高", now, graphmodel.GraphRoleTarget),
	})
	if err := store.PersistMemoryContentGraph(context.Background(), "u-experience-cache", graph, now); err != nil {
		t.Fatalf("PersistMemoryContentGraph() error = %v", err)
	}

	first, err := store.GetSubjectExperienceMemory(context.Background(), "u-experience-cache", "美股", []string{"1w"}, now, true)
	if err != nil {
		t.Fatalf("first GetSubjectExperienceMemory() error = %v", err)
	}
	second, err := store.GetSubjectExperienceMemory(context.Background(), "u-experience-cache", "美股", []string{"1w"}, now.Add(2*time.Hour), false)
	if err != nil {
		t.Fatalf("second GetSubjectExperienceMemory() error = %v", err)
	}
	if second.GeneratedAt != first.GeneratedAt || second.CacheStatus != "fresh" {
		t.Fatalf("cache result generated/status = %s/%s, want reused %s/fresh", second.GeneratedAt, second.CacheStatus, first.GeneratedAt)
	}
}

func TestSQLiteStore_GetSubjectExperienceMemoryUsesPreloadedHorizonInputs(t *testing.T) {
	store := newSubjectTimelineTestStore(t)
	ctx := context.Background()
	now := time.Date(2026, 4, 30, 12, 0, 0, 0, time.UTC)
	preloaded := memory.SubjectHorizonMemory{
		UserID:           "u-experience-preload",
		Subject:          "美股",
		CanonicalSubject: "美股",
		Horizon:          "1w",
		GeneratedAt:      now.Format(time.RFC3339),
		InputHash:        "preloaded-hash",
		SampleCount:      7,
		KeyChanges:       []memory.SubjectHorizonChange{{When: now.Format(time.RFC3339), Subject: "美股", ChangeText: "继续走强"}},
	}

	out, err := store.getSubjectExperienceMemoryWithHorizonInputs(ctx, "u-experience-preload", "美股", []string{"1w"}, now, false, map[string]memory.SubjectHorizonMemory{"1w": preloaded})
	if err != nil {
		t.Fatalf("getSubjectExperienceMemoryWithHorizonInputs() error = %v", err)
	}
	if len(out.HorizonSummaries) != 1 || out.HorizonSummaries[0].SampleCount != 7 {
		t.Fatalf("HorizonSummaries = %#v, want preloaded sample count", out.HorizonSummaries)
	}
	var horizonRows int
	if err := store.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM subject_horizon_memories WHERE user_id = ?`, "u-experience-preload").Scan(&horizonRows); err != nil {
		t.Fatalf("subject_horizon_memories count query error = %v", err)
	}
	if horizonRows != 0 {
		t.Fatalf("subject_horizon_memories rows = %d, want no horizon DB build/read path for preloaded horizon", horizonRows)
	}
}

func TestSQLiteStore_GetSubjectExperienceMemoryRejectsUnsupportedHorizon(t *testing.T) {
	store := newSubjectTimelineTestStore(t)
	_, err := store.GetSubjectExperienceMemory(context.Background(), "u-experience-bad", "美股", []string{"10y"}, time.Date(2026, 4, 30, 0, 0, 0, 0, time.UTC), false)
	if err == nil {
		t.Fatal("GetSubjectExperienceMemory(10y) error = nil, want unsupported horizon error")
	}
}

func containsString(items []string, want string) bool {
	for _, item := range items {
		if item == want {
			return true
		}
	}
	return false
}
