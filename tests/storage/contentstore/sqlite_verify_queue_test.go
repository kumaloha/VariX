package contentstore

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/kumaloha/VariX/varix/model"
)

func TestSQLiteStore_EnqueueAndListDueVerifyQueueItems(t *testing.T) {
	root := t.TempDir()
	store, err := NewSQLiteStore(filepath.Join(root, "data", "content.db"))
	if err != nil {
		t.Fatalf("NewSQLiteStore() error = %v", err)
	}
	defer store.Close()

	now := time.Date(2026, 4, 21, 0, 0, 0, 0, time.UTC)
	future := now.Add(time.Hour)
	if err := store.EnqueueVerifyQueueItem(context.Background(), model.VerifyQueueItem{
		ID:              "q-due",
		ObjectType:      model.VerifyQueueObjectNode,
		ObjectID:        "n1",
		SourceArticleID: "unit-1",
		Priority:        10,
		ScheduledAt:     now.Format(time.RFC3339),
		Status:          model.VerifyQueueStatusQueued,
	}); err != nil {
		t.Fatalf("EnqueueVerifyQueueItem(due) error = %v", err)
	}
	if err := store.EnqueueVerifyQueueItem(context.Background(), model.VerifyQueueItem{
		ID:              "q-future",
		ObjectType:      model.VerifyQueueObjectEdge,
		ObjectID:        "e1",
		SourceArticleID: "unit-1",
		Priority:        1,
		ScheduledAt:     future.Format(time.RFC3339),
		Status:          model.VerifyQueueStatusQueued,
	}); err != nil {
		t.Fatalf("EnqueueVerifyQueueItem(future) error = %v", err)
	}

	items, err := store.ListDueVerifyQueueItems(context.Background(), now, 10)
	if err != nil {
		t.Fatalf("ListDueVerifyQueueItems() error = %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("len(ListDueVerifyQueueItems()) = %d, want 1", len(items))
	}
	if items[0].ID != "q-due" {
		t.Fatalf("due queue item id = %q, want q-due", items[0].ID)
	}
}

func TestSQLiteStore_MarkFinishAndRetryVerifyQueueItem(t *testing.T) {
	root := t.TempDir()
	store, err := NewSQLiteStore(filepath.Join(root, "data", "content.db"))
	if err != nil {
		t.Fatalf("NewSQLiteStore() error = %v", err)
	}
	defer store.Close()

	now := time.Date(2026, 4, 21, 0, 0, 0, 0, time.UTC)
	queue := model.VerifyQueueItem{
		ID:              "q1",
		ObjectType:      model.VerifyQueueObjectNode,
		ObjectID:        "n1",
		SourceArticleID: "unit-1",
		Priority:        5,
		ScheduledAt:     now.Format(time.RFC3339),
		Status:          model.VerifyQueueStatusQueued,
	}
	if err := store.EnqueueVerifyQueueItem(context.Background(), queue); err != nil {
		t.Fatalf("EnqueueVerifyQueueItem() error = %v", err)
	}
	if err := store.MarkVerifyQueueItemRunning(context.Background(), "q1", now); err != nil {
		t.Fatalf("MarkVerifyQueueItemRunning() error = %v", err)
	}
	items, err := store.ListDueVerifyQueueItems(context.Background(), now, 10)
	if err != nil {
		t.Fatalf("ListDueVerifyQueueItems() error = %v", err)
	}
	if len(items) != 0 {
		t.Fatalf("len(ListDueVerifyQueueItems() after running) = %d, want 0", len(items))
	}
	verdict := model.VerifyVerdict{
		ObjectType: model.VerifyQueueObjectNode,
		ObjectID:   "n1",
		Verdict:    model.VerificationProved,
		Reason:     "matched observed outcome",
		AsOf:       now.Format(time.RFC3339),
	}
	if err := store.FinishVerifyQueueItem(context.Background(), "q1", verdict, now); err != nil {
		t.Fatalf("FinishVerifyQueueItem() error = %v", err)
	}

	var queueStatus string
	if err := store.db.QueryRow(`SELECT status FROM verify_queue WHERE queue_id = ?`, "q1").Scan(&queueStatus); err != nil {
		t.Fatalf("QueryRow(verify_queue) error = %v", err)
	}
	if queueStatus != string(model.VerifyQueueStatusDone) {
		t.Fatalf("queue status = %q, want done", queueStatus)
	}
	var verdictCount int
	if err := store.db.QueryRow(`SELECT COUNT(*) FROM verify_verdict_history WHERE object_id = ?`, "n1").Scan(&verdictCount); err != nil {
		t.Fatalf("QueryRow(verify_verdict_history) error = %v", err)
	}
	if verdictCount != 1 {
		t.Fatalf("verdict history count = %d, want 1", verdictCount)
	}

	if err := store.EnqueueVerifyQueueItem(context.Background(), model.VerifyQueueItem{
		ID:              "q2",
		ObjectType:      model.VerifyQueueObjectNode,
		ObjectID:        "n2",
		SourceArticleID: "unit-1",
		Priority:        1,
		ScheduledAt:     now.Format(time.RFC3339),
		Status:          model.VerifyQueueStatusQueued,
	}); err != nil {
		t.Fatalf("EnqueueVerifyQueueItem(q2) error = %v", err)
	}
	retryAt := now.Add(2 * time.Hour)
	if err := store.RetryVerifyQueueItem(context.Background(), "q2", retryAt, "still pending", now); err != nil {
		t.Fatalf("RetryVerifyQueueItem() error = %v", err)
	}
	var status, scheduledAt string
	if err := store.db.QueryRow(`SELECT status, scheduled_at FROM verify_queue WHERE queue_id = ?`, "q2").Scan(&status, &scheduledAt); err != nil {
		t.Fatalf("QueryRow(verify_queue q2) error = %v", err)
	}
	if status != string(model.VerifyQueueStatusRetry) {
		t.Fatalf("q2 status = %q, want retry", status)
	}
	if scheduledAt != retryAt.Format(time.RFC3339Nano) {
		t.Fatalf("q2 scheduled_at = %q, want %q", scheduledAt, retryAt.Format(time.RFC3339Nano))
	}
}

func TestSQLiteStore_ClaimDueVerifyQueueItemsMarksItemsRunning(t *testing.T) {
	root := t.TempDir()
	store, err := NewSQLiteStore(filepath.Join(root, "data", "content.db"))
	if err != nil {
		t.Fatalf("NewSQLiteStore() error = %v", err)
	}
	defer store.Close()

	now := time.Date(2026, 4, 21, 0, 0, 0, 0, time.UTC)
	for _, item := range []model.VerifyQueueItem{
		{ID: "q-claim-1", ObjectType: model.VerifyQueueObjectNode, ObjectID: "n1", SourceArticleID: "u1", Priority: 5, ScheduledAt: now.Format(time.RFC3339), Status: model.VerifyQueueStatusQueued},
		{ID: "q-claim-2", ObjectType: model.VerifyQueueObjectEdge, ObjectID: "e1", SourceArticleID: "u1", Priority: 10, ScheduledAt: now.Format(time.RFC3339), Status: model.VerifyQueueStatusQueued},
		{ID: "q-claim-3", ObjectType: model.VerifyQueueObjectNode, ObjectID: "n2", SourceArticleID: "u1", Priority: 1, ScheduledAt: now.Add(time.Hour).Format(time.RFC3339), Status: model.VerifyQueueStatusQueued},
	} {
		if err := store.EnqueueVerifyQueueItem(context.Background(), item); err != nil {
			t.Fatalf("EnqueueVerifyQueueItem(%s) error = %v", item.ID, err)
		}
	}
	claimed, err := store.ClaimDueVerifyQueueItems(context.Background(), now, 2)
	if err != nil {
		t.Fatalf("ClaimDueVerifyQueueItems() error = %v", err)
	}
	if len(claimed) != 2 {
		t.Fatalf("len(ClaimDueVerifyQueueItems()) = %d, want 2", len(claimed))
	}
	if claimed[0].ID != "q-claim-2" || claimed[1].ID != "q-claim-1" {
		t.Fatalf("claimed order = %#v, want priority-desc due items", claimed)
	}
	for _, id := range []string{"q-claim-1", "q-claim-2"} {
		item, err := getVerifyQueueItem(context.Background(), store.db, id)
		if err != nil {
			t.Fatalf("getVerifyQueueItem(%s) error = %v", id, err)
		}
		if item.Status != model.VerifyQueueStatusRunning {
			t.Fatalf("item %s status = %q, want running", id, item.Status)
		}
		if item.Attempts != 1 {
			t.Fatalf("item %s attempts = %d, want 1", id, item.Attempts)
		}
	}
	future, err := getVerifyQueueItem(context.Background(), store.db, "q-claim-3")
	if err != nil {
		t.Fatalf("getVerifyQueueItem(q-claim-3) error = %v", err)
	}
	if future.Status != model.VerifyQueueStatusQueued {
		t.Fatalf("future item status = %q, want queued", future.Status)
	}
}

func TestSQLiteStore_RunVerifyQueueSweepProcessesClaimsAndRetries(t *testing.T) {
	root := t.TempDir()
	store, err := NewSQLiteStore(filepath.Join(root, "data", "content.db"))
	if err != nil {
		t.Fatalf("NewSQLiteStore() error = %v", err)
	}
	defer store.Close()

	now := time.Date(2026, 4, 21, 0, 0, 0, 0, time.UTC)
	items := []model.VerifyQueueItem{
		{ID: "q-sweep-1", ObjectType: model.VerifyQueueObjectNode, ObjectID: "n1", SourceArticleID: "u1", Priority: 10, ScheduledAt: now.Format(time.RFC3339), Status: model.VerifyQueueStatusQueued},
		{ID: "q-sweep-2", ObjectType: model.VerifyQueueObjectEdge, ObjectID: "e1", SourceArticleID: "u1", Priority: 5, ScheduledAt: now.Format(time.RFC3339), Status: model.VerifyQueueStatusQueued},
	}
	for _, item := range items {
		if err := store.EnqueueVerifyQueueItem(context.Background(), item); err != nil {
			t.Fatalf("EnqueueVerifyQueueItem(%s) error = %v", item.ID, err)
		}
	}

	result, err := store.RunVerifyQueueSweep(context.Background(), now, 10, func(item model.VerifyQueueItem) (model.VerifyVerdict, error) {
		switch item.ID {
		case "q-sweep-1":
			return model.VerifyVerdict{ObjectType: item.ObjectType, ObjectID: item.ObjectID, Verdict: model.VerificationProved, Reason: "matched", AsOf: now.Format(time.RFC3339)}, nil
		case "q-sweep-2":
			return model.VerifyVerdict{ObjectType: item.ObjectType, ObjectID: item.ObjectID, Verdict: model.VerificationPending, Reason: "still pending", AsOf: now.Format(time.RFC3339), NextVerifyAt: now.Add(time.Hour).Format(time.RFC3339)}, nil
		default:
			return model.VerifyVerdict{}, nil
		}
	})
	if err != nil {
		t.Fatalf("RunVerifyQueueSweep() error = %v", err)
	}
	if result.Claimed != 2 || result.Finished != 1 || result.Retried != 1 {
		t.Fatalf("result = %#v, want claimed=2 finished=1 retried=1", result)
	}
	first, err := getVerifyQueueItem(context.Background(), store.db, "q-sweep-1")
	if err != nil {
		t.Fatalf("getVerifyQueueItem(q-sweep-1) error = %v", err)
	}
	if first.Status != model.VerifyQueueStatusDone {
		t.Fatalf("q-sweep-1 status = %q, want done", first.Status)
	}
	second, err := getVerifyQueueItem(context.Background(), store.db, "q-sweep-2")
	if err != nil {
		t.Fatalf("getVerifyQueueItem(q-sweep-2) error = %v", err)
	}
	if second.Status != model.VerifyQueueStatusRetry {
		t.Fatalf("q-sweep-2 status = %q, want retry", second.Status)
	}
}

func TestSQLiteStore_RunVerifyQueueSweepAlsoAppliesVerdictToContentGraph(t *testing.T) {
	root := t.TempDir()
	store, err := NewSQLiteStore(filepath.Join(root, "data", "content.db"))
	if err != nil {
		t.Fatalf("NewSQLiteStore() error = %v", err)
	}
	defer store.Close()

	now := time.Date(2026, 4, 21, 0, 0, 0, 0, time.UTC)
	subgraph := model.ContentSubgraph{
		ID:               "unit-sweep-graph",
		ArticleID:        "unit-sweep-graph",
		SourcePlatform:   "twitter",
		SourceExternalID: "sweep-graph",
		CompileVersion:   model.CompileBridgeVersion,
		CompiledAt:       now.Format(time.RFC3339),
		UpdatedAt:        now.Format(time.RFC3339),
		Nodes: []model.ContentNode{{
			ID:                 "n1",
			SourceArticleID:    "unit-sweep-graph",
			SourcePlatform:     "twitter",
			SourceExternalID:   "sweep-graph",
			RawText:            "未来一周美股承压",
			SubjectText:        "美股",
			ChangeText:         "承压",
			Kind:               model.NodeKindPrediction,
			GraphRole:          model.GraphRoleTarget,
			IsPrimary:          true,
			VerificationStatus: model.VerificationPending,
			TimeBucket:         "1w",
		}},
	}
	if err := store.UpsertContentSubgraph(context.Background(), subgraph); err != nil {
		t.Fatalf("UpsertContentSubgraph() error = %v", err)
	}
	if err := store.EnqueueVerifyQueueItem(context.Background(), model.VerifyQueueItem{ID: "q-sweep-graph", ObjectType: model.VerifyQueueObjectNode, ObjectID: "n1", SourceArticleID: "unit-sweep-graph", Priority: 10, ScheduledAt: now.Format(time.RFC3339), Status: model.VerifyQueueStatusQueued}); err != nil {
		t.Fatalf("EnqueueVerifyQueueItem() error = %v", err)
	}
	if _, err := store.RunVerifyQueueSweep(context.Background(), now, 10, func(item model.VerifyQueueItem) (model.VerifyVerdict, error) {
		return model.VerifyVerdict{ObjectType: item.ObjectType, ObjectID: item.ObjectID, Verdict: model.VerificationProved, Reason: "resolved", AsOf: now.Format(time.RFC3339)}, nil
	}); err != nil {
		t.Fatalf("RunVerifyQueueSweep() error = %v", err)
	}
	got, err := store.GetContentSubgraph(context.Background(), "twitter", "sweep-graph")
	if err != nil {
		t.Fatalf("GetContentSubgraph() error = %v", err)
	}
	if got.Nodes[0].VerificationStatus != model.VerificationProved {
		t.Fatalf("node verification_status = %q, want proved", got.Nodes[0].VerificationStatus)
	}
}

func TestSQLiteStore_RunVerifyQueueSweepUpdatesContentGraphForPendingRetryVerdict(t *testing.T) {
	root := t.TempDir()
	store, err := NewSQLiteStore(filepath.Join(root, "data", "content.db"))
	if err != nil {
		t.Fatalf("NewSQLiteStore() error = %v", err)
	}
	defer store.Close()

	now := time.Date(2026, 4, 21, 0, 0, 0, 0, time.UTC)
	nextAt := now.Add(2 * time.Hour)
	subgraph := model.ContentSubgraph{
		ID:               "unit-sweep-pending",
		ArticleID:        "unit-sweep-pending",
		SourcePlatform:   "twitter",
		SourceExternalID: "sweep-pending",
		CompileVersion:   model.CompileBridgeVersion,
		CompiledAt:       now.Format(time.RFC3339),
		UpdatedAt:        now.Format(time.RFC3339),
		Nodes: []model.ContentNode{{
			ID:                 "n1",
			SourceArticleID:    "unit-sweep-pending",
			SourcePlatform:     "twitter",
			SourceExternalID:   "sweep-pending",
			RawText:            "未来一周美股承压",
			SubjectText:        "美股",
			ChangeText:         "承压",
			Kind:               model.NodeKindPrediction,
			GraphRole:          model.GraphRoleTarget,
			IsPrimary:          true,
			VerificationStatus: model.VerificationPending,
			TimeBucket:         "1w",
		}},
	}
	if err := store.UpsertContentSubgraph(context.Background(), subgraph); err != nil {
		t.Fatalf("UpsertContentSubgraph() error = %v", err)
	}
	if err := store.EnqueueVerifyQueueItem(context.Background(), model.VerifyQueueItem{ID: "q-sweep-pending", ObjectType: model.VerifyQueueObjectNode, ObjectID: "n1", SourceArticleID: "unit-sweep-pending", Priority: 10, ScheduledAt: now.Format(time.RFC3339), Status: model.VerifyQueueStatusQueued}); err != nil {
		t.Fatalf("EnqueueVerifyQueueItem() error = %v", err)
	}
	if _, err := store.RunVerifyQueueSweep(context.Background(), now, 10, func(item model.VerifyQueueItem) (model.VerifyVerdict, error) {
		return model.VerifyVerdict{ObjectType: item.ObjectType, ObjectID: item.ObjectID, Verdict: model.VerificationPending, Reason: "waiting for weekly close", AsOf: now.Format(time.RFC3339), NextVerifyAt: nextAt.Format(time.RFC3339)}, nil
	}); err != nil {
		t.Fatalf("RunVerifyQueueSweep() error = %v", err)
	}
	got, err := store.GetContentSubgraph(context.Background(), "twitter", "sweep-pending")
	if err != nil {
		t.Fatalf("GetContentSubgraph() error = %v", err)
	}
	if got.Nodes[0].VerificationStatus != model.VerificationPending {
		t.Fatalf("node verification_status = %q, want pending", got.Nodes[0].VerificationStatus)
	}
	if got.Nodes[0].VerificationReason != "waiting for weekly close" {
		t.Fatalf("node verification_reason = %q, want propagated pending reason", got.Nodes[0].VerificationReason)
	}
	if got.Nodes[0].NextVerifyAt != nextAt.Format(time.RFC3339) {
		t.Fatalf("node next_verify_at = %q, want %q", got.Nodes[0].NextVerifyAt, nextAt.Format(time.RFC3339))
	}
}

func TestSQLiteStore_RunVerifyQueueSweepUsesRetryBackoffPolicyOnEvaluatorError(t *testing.T) {
	root := t.TempDir()
	store, err := NewSQLiteStore(filepath.Join(root, "data", "content.db"))
	if err != nil {
		t.Fatalf("NewSQLiteStore() error = %v", err)
	}
	defer store.Close()

	now := time.Date(2026, 4, 21, 0, 0, 0, 0, time.UTC)
	if err := store.EnqueueVerifyQueueItem(context.Background(), model.VerifyQueueItem{
		ID:              "q-retry-error",
		ObjectType:      model.VerifyQueueObjectNode,
		ObjectID:        "n1",
		SourceArticleID: "u1",
		Priority:        10,
		ScheduledAt:     now.Format(time.RFC3339),
		Attempts:        1,
		Status:          model.VerifyQueueStatusRetry,
	}); err != nil {
		t.Fatalf("EnqueueVerifyQueueItem() error = %v", err)
	}
	if _, err := store.RunVerifyQueueSweep(context.Background(), now, 10, func(item model.VerifyQueueItem) (model.VerifyVerdict, error) {
		return model.VerifyVerdict{}, errors.New("boom")
	}); err != nil {
		t.Fatalf("RunVerifyQueueSweep() error = %v", err)
	}
	item, err := getVerifyQueueItem(context.Background(), store.db, "q-retry-error")
	if err != nil {
		t.Fatalf("getVerifyQueueItem() error = %v", err)
	}
	if item.Status != model.VerifyQueueStatusRetry {
		t.Fatalf("Status = %q, want retry", item.Status)
	}
	if item.ScheduledAt != now.Add(5*time.Minute).Format(time.RFC3339) {
		t.Fatalf("ScheduledAt = %q, want %q", item.ScheduledAt, now.Add(5*time.Minute).Format(time.RFC3339))
	}
}

func TestSQLiteStore_RunVerifyQueueSweepUsesRetryBackoffPolicyOnFirstFailure(t *testing.T) {
	root := t.TempDir()
	store, err := NewSQLiteStore(filepath.Join(root, "data", "content.db"))
	if err != nil {
		t.Fatalf("NewSQLiteStore() error = %v", err)
	}
	defer store.Close()

	now := time.Date(2026, 4, 21, 0, 0, 0, 0, time.UTC)
	if err := store.EnqueueVerifyQueueItem(context.Background(), model.VerifyQueueItem{
		ID:              "q-retry-first",
		ObjectType:      model.VerifyQueueObjectNode,
		ObjectID:        "n1",
		SourceArticleID: "u1",
		Priority:        10,
		ScheduledAt:     now.Format(time.RFC3339),
		Status:          model.VerifyQueueStatusQueued,
	}); err != nil {
		t.Fatalf("EnqueueVerifyQueueItem() error = %v", err)
	}
	if _, err := store.RunVerifyQueueSweep(context.Background(), now, 10, func(item model.VerifyQueueItem) (model.VerifyVerdict, error) {
		return model.VerifyVerdict{}, errors.New("boom")
	}); err != nil {
		t.Fatalf("RunVerifyQueueSweep() error = %v", err)
	}
	item, err := getVerifyQueueItem(context.Background(), store.db, "q-retry-first")
	if err != nil {
		t.Fatalf("getVerifyQueueItem() error = %v", err)
	}
	if item.ScheduledAt != now.Add(time.Minute).Format(time.RFC3339) {
		t.Fatalf("ScheduledAt = %q, want %q", item.ScheduledAt, now.Add(time.Minute).Format(time.RFC3339))
	}
}

func TestSQLiteStore_RunVerifyQueueSweepUsesRetryBackoffPolicyForPendingWithoutNextVerifyAt(t *testing.T) {
	root := t.TempDir()
	store, err := NewSQLiteStore(filepath.Join(root, "data", "content.db"))
	if err != nil {
		t.Fatalf("NewSQLiteStore() error = %v", err)
	}
	defer store.Close()

	now := time.Date(2026, 4, 21, 0, 0, 0, 0, time.UTC)
	subgraph := model.ContentSubgraph{
		ID:               "retry-policy-pending",
		ArticleID:        "retry-policy-pending",
		SourcePlatform:   "twitter",
		SourceExternalID: "retry-policy-pending",
		CompileVersion:   model.CompileBridgeVersion,
		CompiledAt:       now.Format(time.RFC3339),
		UpdatedAt:        now.Format(time.RFC3339),
		Nodes: []model.ContentNode{{
			ID:                 "n1",
			SourceArticleID:    "retry-policy-pending",
			SourcePlatform:     "twitter",
			SourceExternalID:   "retry-policy-pending",
			RawText:            "未来一周美股承压",
			SubjectText:        "美股",
			ChangeText:         "承压",
			Kind:               model.NodeKindPrediction,
			GraphRole:          model.GraphRoleTarget,
			IsPrimary:          true,
			VerificationStatus: model.VerificationPending,
			TimeBucket:         "1w",
		}},
	}
	if err := store.UpsertContentSubgraph(context.Background(), subgraph); err != nil {
		t.Fatalf("UpsertContentSubgraph() error = %v", err)
	}
	if err := store.EnqueueVerifyQueueItem(context.Background(), model.VerifyQueueItem{
		ID:              "q-retry-pending",
		ObjectType:      model.VerifyQueueObjectNode,
		ObjectID:        "n1",
		SourceArticleID: "retry-policy-pending",
		Priority:        10,
		ScheduledAt:     now.Format(time.RFC3339),
		Attempts:        2,
		Status:          model.VerifyQueueStatusRetry,
	}); err != nil {
		t.Fatalf("EnqueueVerifyQueueItem() error = %v", err)
	}
	if _, err := store.RunVerifyQueueSweep(context.Background(), now, 10, func(item model.VerifyQueueItem) (model.VerifyVerdict, error) {
		return model.VerifyVerdict{ObjectType: item.ObjectType, ObjectID: item.ObjectID, Verdict: model.VerificationPending, Reason: "still waiting", AsOf: now.Format(time.RFC3339)}, nil
	}); err != nil {
		t.Fatalf("RunVerifyQueueSweep() error = %v", err)
	}
	item, err := getVerifyQueueItem(context.Background(), store.db, "q-retry-pending")
	if err != nil {
		t.Fatalf("getVerifyQueueItem() error = %v", err)
	}
	if item.ScheduledAt != now.Add(15*time.Minute).Format(time.RFC3339) {
		t.Fatalf("ScheduledAt = %q, want %q", item.ScheduledAt, now.Add(15*time.Minute).Format(time.RFC3339))
	}
}

func TestSQLiteStore_RunVerifyQueueSweepUsesRetryBackoffPolicyAtOneHourCeiling(t *testing.T) {
	root := t.TempDir()
	store, err := NewSQLiteStore(filepath.Join(root, "data", "content.db"))
	if err != nil {
		t.Fatalf("NewSQLiteStore() error = %v", err)
	}
	defer store.Close()

	now := time.Date(2026, 4, 21, 0, 0, 0, 0, time.UTC)
	if err := store.EnqueueVerifyQueueItem(context.Background(), model.VerifyQueueItem{
		ID:              "q-retry-ceiling",
		ObjectType:      model.VerifyQueueObjectNode,
		ObjectID:        "n1",
		SourceArticleID: "u1",
		Priority:        10,
		ScheduledAt:     now.Format(time.RFC3339),
		Attempts:        4,
		Status:          model.VerifyQueueStatusRetry,
	}); err != nil {
		t.Fatalf("EnqueueVerifyQueueItem() error = %v", err)
	}
	if _, err := store.RunVerifyQueueSweep(context.Background(), now, 10, func(item model.VerifyQueueItem) (model.VerifyVerdict, error) {
		return model.VerifyVerdict{}, errors.New("boom")
	}); err != nil {
		t.Fatalf("RunVerifyQueueSweep() error = %v", err)
	}
	item, err := getVerifyQueueItem(context.Background(), store.db, "q-retry-ceiling")
	if err != nil {
		t.Fatalf("getVerifyQueueItem() error = %v", err)
	}
	if item.ScheduledAt != now.Add(time.Hour).Format(time.RFC3339) {
		t.Fatalf("ScheduledAt = %q, want %q", item.ScheduledAt, now.Add(time.Hour).Format(time.RFC3339))
	}
}

func TestSQLiteStore_ListVerifyQueueItemsReturnsAllStatuses(t *testing.T) {
	root := t.TempDir()
	store, err := NewSQLiteStore(filepath.Join(root, "data", "content.db"))
	if err != nil {
		t.Fatalf("NewSQLiteStore() error = %v", err)
	}
	defer store.Close()

	now := time.Date(2026, 4, 21, 0, 0, 0, 0, time.UTC)
	items := []model.VerifyQueueItem{
		{ID: "q-all-queued", ObjectType: model.VerifyQueueObjectNode, ObjectID: "n1", SourceArticleID: "u1", Priority: 10, ScheduledAt: now.Format(time.RFC3339), Status: model.VerifyQueueStatusQueued},
		{ID: "q-all-running", ObjectType: model.VerifyQueueObjectNode, ObjectID: "n2", SourceArticleID: "u1", Priority: 5, ScheduledAt: now.Format(time.RFC3339), Status: model.VerifyQueueStatusQueued},
		{ID: "q-all-retry", ObjectType: model.VerifyQueueObjectEdge, ObjectID: "e1", SourceArticleID: "u1", Priority: 1, ScheduledAt: now.Add(time.Hour).Format(time.RFC3339), Status: model.VerifyQueueStatusQueued},
	}
	for _, item := range items {
		if err := store.EnqueueVerifyQueueItem(context.Background(), item); err != nil {
			t.Fatalf("EnqueueVerifyQueueItem(%s) error = %v", item.ID, err)
		}
	}
	if err := store.MarkVerifyQueueItemRunning(context.Background(), "q-all-running", now); err != nil {
		t.Fatalf("MarkVerifyQueueItemRunning() error = %v", err)
	}
	if err := store.RetryVerifyQueueItem(context.Background(), "q-all-retry", now.Add(2*time.Hour), "still pending", now); err != nil {
		t.Fatalf("RetryVerifyQueueItem() error = %v", err)
	}
	all, err := store.ListVerifyQueueItems(context.Background(), 10)
	if err != nil {
		t.Fatalf("ListVerifyQueueItems() error = %v", err)
	}
	if len(all) != 3 {
		t.Fatalf("len(ListVerifyQueueItems()) = %d, want 3", len(all))
	}
	if all[0].ID != "q-all-queued" || all[1].ID != "q-all-running" || all[2].ID != "q-all-retry" {
		t.Fatalf("queue order = %#v, want queued/running/retry order by status+priority", all)
	}
}

func TestSQLiteStore_RunVerifyQueueSweepRefreshesDownstreamProjections(t *testing.T) {
	root := t.TempDir()
	store, err := NewSQLiteStore(filepath.Join(root, "data", "content.db"))
	if err != nil {
		t.Fatalf("NewSQLiteStore() error = %v", err)
	}
	defer store.Close()

	now := time.Date(2026, 4, 21, 0, 0, 0, 0, time.UTC)
	subgraph := model.ContentSubgraph{
		ID:               "sweep-downstream",
		ArticleID:        "sweep-downstream",
		SourcePlatform:   "twitter",
		SourceExternalID: "sweep-downstream",
		CompileVersion:   model.CompileBridgeVersion,
		CompiledAt:       now.Format(time.RFC3339),
		UpdatedAt:        now.Format(time.RFC3339),
		Nodes: []model.ContentNode{
			{ID: "n1", SourceArticleID: "sweep-downstream", SourcePlatform: "twitter", SourceExternalID: "sweep-downstream", RawText: "美联储加息0.25%", SubjectText: "美联储", ChangeText: "加息0.25%", Kind: model.NodeKindObservation, GraphRole: model.GraphRoleDriver, IsPrimary: true, VerificationStatus: model.VerificationPending, TimeBucket: "1w"},
			{ID: "n2", SourceArticleID: "sweep-downstream", SourcePlatform: "twitter", SourceExternalID: "sweep-downstream", RawText: "未来一周美股承压", SubjectText: "美股", ChangeText: "承压", Kind: model.NodeKindPrediction, GraphRole: model.GraphRoleTarget, IsPrimary: true, VerificationStatus: model.VerificationPending, TimeBucket: "1w"},
		},
		Edges: []model.ContentEdge{{ID: "e1", From: "n1", To: "n2", Type: model.EdgeTypeDrives, IsPrimary: true, VerificationStatus: model.VerificationPending}},
	}
	if err := store.UpsertContentSubgraph(context.Background(), subgraph); err != nil {
		t.Fatalf("UpsertContentSubgraph() error = %v", err)
	}
	if err := store.PersistMemoryContentGraph(context.Background(), "u-sweep-downstream", subgraph, now); err != nil {
		t.Fatalf("PersistMemoryContentGraph() error = %v", err)
	}
	if err := store.EnqueueVerifyQueueItem(context.Background(), model.VerifyQueueItem{ID: "q-sweep-downstream", ObjectType: model.VerifyQueueObjectNode, ObjectID: "n2", SourceArticleID: "sweep-downstream", Priority: 10, ScheduledAt: now.Add(-time.Minute).Format(time.RFC3339), Status: model.VerifyQueueStatusQueued}); err != nil {
		t.Fatalf("EnqueueVerifyQueueItem() error = %v", err)
	}
	if _, err := store.RunVerifyQueueSweep(context.Background(), now, 10, func(item model.VerifyQueueItem) (model.VerifyVerdict, error) {
		return model.VerifyVerdict{ObjectType: item.ObjectType, ObjectID: item.ObjectID, Verdict: model.VerificationProved, Reason: "resolved in sweep", AsOf: now.Format(time.RFC3339)}, nil
	}); err != nil {
		t.Fatalf("RunVerifyQueueSweep() error = %v", err)
	}
	memGraphs, err := store.ListMemoryContentGraphs(context.Background(), "u-sweep-downstream")
	if err != nil {
		t.Fatalf("ListMemoryContentGraphs() error = %v", err)
	}
	if len(memGraphs) != 1 || memGraphs[0].Nodes[1].VerificationStatus != model.VerificationProved {
		t.Fatalf("memory content graphs = %#v, want n2 proved", memGraphs)
	}
	events, err := store.ListEventGraphs(context.Background(), "u-sweep-downstream")
	if err != nil {
		t.Fatalf("ListEventGraphs() error = %v", err)
	}
	if len(events) != 2 {
		t.Fatalf("len(ListEventGraphs()) = %d, want 2", len(events))
	}
	foundTarget := false
	for _, event := range events {
		if event.Scope == "target" {
			foundTarget = true
			if event.VerificationSummary[model.VerificationProved] != 1 {
				t.Fatalf("target event verification summary = %#v, want proved=1", event.VerificationSummary)
			}
		}
	}
	if !foundTarget {
		t.Fatal("target event graph missing")
	}
	paradigms, err := store.ListParadigms(context.Background(), "u-sweep-downstream")
	if err != nil {
		t.Fatalf("ListParadigms() error = %v", err)
	}
	if len(paradigms) != 1 || paradigms[0].SuccessCount != 1 {
		t.Fatalf("paradigms = %#v, want success_count=1", paradigms)
	}
}

func TestSQLiteStore_RunVerifyQueueSweepPropagatesPendingReasonToMemoryGraph(t *testing.T) {
	root := t.TempDir()
	store, err := NewSQLiteStore(filepath.Join(root, "data", "content.db"))
	if err != nil {
		t.Fatalf("NewSQLiteStore() error = %v", err)
	}
	defer store.Close()

	now := time.Date(2026, 4, 21, 0, 0, 0, 0, time.UTC)
	nextAt := now.Add(2 * time.Hour)
	subgraph := model.ContentSubgraph{
		ID:               "sweep-pending-downstream",
		ArticleID:        "sweep-pending-downstream",
		SourcePlatform:   "twitter",
		SourceExternalID: "sweep-pending-downstream",
		CompileVersion:   model.CompileBridgeVersion,
		CompiledAt:       now.Format(time.RFC3339),
		UpdatedAt:        now.Format(time.RFC3339),
		Nodes: []model.ContentNode{{
			ID:                 "n1",
			SourceArticleID:    "sweep-pending-downstream",
			SourcePlatform:     "twitter",
			SourceExternalID:   "sweep-pending-downstream",
			RawText:            "未来一周美股承压",
			SubjectText:        "美股",
			ChangeText:         "承压",
			Kind:               model.NodeKindPrediction,
			GraphRole:          model.GraphRoleTarget,
			IsPrimary:          true,
			VerificationStatus: model.VerificationPending,
			TimeBucket:         "1w",
		}},
	}
	if err := store.UpsertContentSubgraph(context.Background(), subgraph); err != nil {
		t.Fatalf("UpsertContentSubgraph() error = %v", err)
	}
	if err := store.PersistMemoryContentGraph(context.Background(), "u-sweep-pending-downstream", subgraph, now); err != nil {
		t.Fatalf("PersistMemoryContentGraph() error = %v", err)
	}
	if err := store.EnqueueVerifyQueueItem(context.Background(), model.VerifyQueueItem{ID: "q-sweep-pending-downstream", ObjectType: model.VerifyQueueObjectNode, ObjectID: "n1", SourceArticleID: "sweep-pending-downstream", Priority: 10, ScheduledAt: now.Add(-time.Minute).Format(time.RFC3339), Status: model.VerifyQueueStatusQueued}); err != nil {
		t.Fatalf("EnqueueVerifyQueueItem() error = %v", err)
	}
	if _, err := store.RunVerifyQueueSweep(context.Background(), now, 10, func(item model.VerifyQueueItem) (model.VerifyVerdict, error) {
		return model.VerifyVerdict{ObjectType: item.ObjectType, ObjectID: item.ObjectID, Verdict: model.VerificationPending, Reason: "awaiting close", AsOf: now.Format(time.RFC3339), NextVerifyAt: nextAt.Format(time.RFC3339)}, nil
	}); err != nil {
		t.Fatalf("RunVerifyQueueSweep() error = %v", err)
	}
	memGraphs, err := store.ListMemoryContentGraphs(context.Background(), "u-sweep-pending-downstream")
	if err != nil {
		t.Fatalf("ListMemoryContentGraphs() error = %v", err)
	}
	if len(memGraphs) != 1 || memGraphs[0].Nodes[0].VerificationReason != "awaiting close" || memGraphs[0].Nodes[0].NextVerifyAt != nextAt.Format(time.RFC3339) {
		t.Fatalf("memory content graphs = %#v, want propagated pending reason/next_verify_at", memGraphs)
	}
}

func TestSQLiteStore_RunVerifyQueueSweepPropagatesPendingReasonToEventAndParadigmLayers(t *testing.T) {
	root := t.TempDir()
	store, err := NewSQLiteStore(filepath.Join(root, "data", "content.db"))
	if err != nil {
		t.Fatalf("NewSQLiteStore() error = %v", err)
	}
	defer store.Close()

	now := time.Date(2026, 4, 21, 0, 0, 0, 0, time.UTC)
	nextAt := now.Add(2 * time.Hour)
	subgraph := model.ContentSubgraph{
		ID:               "sweep-pending-full",
		ArticleID:        "sweep-pending-full",
		SourcePlatform:   "twitter",
		SourceExternalID: "sweep-pending-full",
		CompileVersion:   model.CompileBridgeVersion,
		CompiledAt:       now.Format(time.RFC3339),
		UpdatedAt:        now.Format(time.RFC3339),
		Nodes: []model.ContentNode{
			{ID: "n1", SourceArticleID: "sweep-pending-full", SourcePlatform: "twitter", SourceExternalID: "sweep-pending-full", RawText: "美联储加息0.25%", SubjectText: "美联储", ChangeText: "加息0.25%", Kind: model.NodeKindObservation, GraphRole: model.GraphRoleDriver, IsPrimary: true, VerificationStatus: model.VerificationPending, TimeBucket: "1w"},
			{ID: "n2", SourceArticleID: "sweep-pending-full", SourcePlatform: "twitter", SourceExternalID: "sweep-pending-full", RawText: "未来一周美股承压", SubjectText: "美股", ChangeText: "承压", Kind: model.NodeKindPrediction, GraphRole: model.GraphRoleTarget, IsPrimary: true, VerificationStatus: model.VerificationPending, TimeBucket: "1w"},
		},
		Edges: []model.ContentEdge{{ID: "e1", From: "n1", To: "n2", Type: model.EdgeTypeDrives, IsPrimary: true, VerificationStatus: model.VerificationPending}},
	}
	if err := store.UpsertContentSubgraph(context.Background(), subgraph); err != nil {
		t.Fatalf("UpsertContentSubgraph() error = %v", err)
	}
	if err := store.PersistMemoryContentGraph(context.Background(), "u-sweep-pending-full", subgraph, now); err != nil {
		t.Fatalf("PersistMemoryContentGraph() error = %v", err)
	}
	if err := store.EnqueueVerifyQueueItem(context.Background(), model.VerifyQueueItem{ID: "q-sweep-pending-full", ObjectType: model.VerifyQueueObjectNode, ObjectID: "n2", SourceArticleID: "sweep-pending-full", Priority: 10, ScheduledAt: now.Add(-time.Minute).Format(time.RFC3339), Status: model.VerifyQueueStatusQueued}); err != nil {
		t.Fatalf("EnqueueVerifyQueueItem() error = %v", err)
	}
	if _, err := store.RunVerifyQueueSweep(context.Background(), now, 10, func(item model.VerifyQueueItem) (model.VerifyVerdict, error) {
		return model.VerifyVerdict{ObjectType: item.ObjectType, ObjectID: item.ObjectID, Verdict: model.VerificationPending, Reason: "awaiting close", AsOf: now.Format(time.RFC3339), NextVerifyAt: nextAt.Format(time.RFC3339)}, nil
	}); err != nil {
		t.Fatalf("RunVerifyQueueSweep() error = %v", err)
	}
	events, err := store.ListEventGraphs(context.Background(), "u-sweep-pending-full")
	if err != nil {
		t.Fatalf("ListEventGraphs() error = %v", err)
	}
	if len(events) != 2 {
		t.Fatalf("len(ListEventGraphs()) = %d, want 2", len(events))
	}
	foundTarget := false
	for _, event := range events {
		if event.Scope == "target" {
			foundTarget = true
			if event.VerificationSummary[model.VerificationPending] != 1 {
				t.Fatalf("target event summary = %#v, want pending=1", event.VerificationSummary)
			}
		}
	}
	if !foundTarget {
		t.Fatal("target event graph missing")
	}
	paradigms, err := store.ListParadigms(context.Background(), "u-sweep-pending-full")
	if err != nil {
		t.Fatalf("ListParadigms() error = %v", err)
	}
	if len(paradigms) != 1 {
		t.Fatalf("len(ListParadigms()) = %d, want 1", len(paradigms))
	}
	if paradigms[0].SuccessCount != 0 || paradigms[0].FailureCount != 0 {
		t.Fatalf("paradigm = %#v, want no resolved counts under pending verdict", paradigms[0])
	}
}

func TestSQLiteStore_GetVerifyQueueSummaryCountsStatuses(t *testing.T) {
	root := t.TempDir()
	store, err := NewSQLiteStore(filepath.Join(root, "data", "content.db"))
	if err != nil {
		t.Fatalf("NewSQLiteStore() error = %v", err)
	}
	defer store.Close()

	now := time.Now().UTC()
	for _, item := range []model.VerifyQueueItem{
		{ID: "q-sum-1", ObjectType: model.VerifyQueueObjectNode, ObjectID: "n1", SourceArticleID: "u1", Priority: 10, ScheduledAt: now.Format(time.RFC3339), Status: model.VerifyQueueStatusQueued},
		{ID: "q-sum-2", ObjectType: model.VerifyQueueObjectNode, ObjectID: "n2", SourceArticleID: "u1", Priority: 5, ScheduledAt: now.Format(time.RFC3339), Status: model.VerifyQueueStatusQueued},
		{ID: "q-sum-3", ObjectType: model.VerifyQueueObjectEdge, ObjectID: "e1", SourceArticleID: "u1", Priority: 1, ScheduledAt: now.Format(time.RFC3339), Status: model.VerifyQueueStatusQueued},
	} {
		if err := store.EnqueueVerifyQueueItem(context.Background(), item); err != nil {
			t.Fatalf("EnqueueVerifyQueueItem(%s) error = %v", item.ID, err)
		}
	}
	if err := store.MarkVerifyQueueItemRunning(context.Background(), "q-sum-2", now); err != nil {
		t.Fatalf("MarkVerifyQueueItemRunning() error = %v", err)
	}
	if err := store.RetryVerifyQueueItem(context.Background(), "q-sum-3", now.Add(time.Hour), "still pending", now); err != nil {
		t.Fatalf("RetryVerifyQueueItem() error = %v", err)
	}
	summary, err := store.GetVerifyQueueSummary(context.Background())
	if err != nil {
		t.Fatalf("GetVerifyQueueSummary() error = %v", err)
	}
	if summary[model.VerifyQueueStatusQueued] != 1 || summary[model.VerifyQueueStatusRunning] != 1 || summary[model.VerifyQueueStatusRetry] != 1 {
		t.Fatalf("summary = %#v, want queued/running/retry = 1/1/1", summary)
	}
}

func TestSQLiteStore_GetVerifyQueueSummaryIncludesDueAndOldestScheduledAt(t *testing.T) {
	root := t.TempDir()
	store, err := NewSQLiteStore(filepath.Join(root, "data", "content.db"))
	if err != nil {
		t.Fatalf("NewSQLiteStore() error = %v", err)
	}
	defer store.Close()
	now := time.Date(2026, 4, 21, 12, 0, 0, 0, time.UTC)
	if err := store.EnqueueVerifyQueueItem(context.Background(), model.VerifyQueueItem{ID: "q-sum-meta-1", ObjectType: model.VerifyQueueObjectNode, ObjectID: "n1", SourceArticleID: "u1", Priority: 10, ScheduledAt: now.Add(-time.Hour).Format(time.RFC3339), Status: model.VerifyQueueStatusQueued}); err != nil {
		t.Fatalf("EnqueueVerifyQueueItem() error = %v", err)
	}
	if err := store.EnqueueVerifyQueueItem(context.Background(), model.VerifyQueueItem{ID: "q-sum-meta-2", ObjectType: model.VerifyQueueObjectEdge, ObjectID: "e1", SourceArticleID: "u1", Priority: 5, ScheduledAt: now.Add(time.Hour).Format(time.RFC3339), Status: model.VerifyQueueStatusRetry}); err != nil {
		t.Fatalf("EnqueueVerifyQueueItem() error = %v", err)
	}
	summary, err := store.GetVerifyQueueSummaryDetailed(context.Background(), now)
	if err != nil {
		t.Fatalf("GetVerifyQueueSummaryDetailed() error = %v", err)
	}
	if summary.DueCount != 1 {
		t.Fatalf("DueCount = %d, want 1", summary.DueCount)
	}
	if summary.OldestScheduledAt != now.Add(-time.Hour).Format(time.RFC3339) {
		t.Fatalf("OldestScheduledAt = %q, want %q", summary.OldestScheduledAt, now.Add(-time.Hour).Format(time.RFC3339))
	}
}

func TestSQLiteStore_GetVerifyQueueSummaryIncludesObjectTypeBreakdown(t *testing.T) {
	root := t.TempDir()
	store, err := NewSQLiteStore(filepath.Join(root, "data", "content.db"))
	if err != nil {
		t.Fatalf("NewSQLiteStore() error = %v", err)
	}
	defer store.Close()
	now := time.Now().UTC()
	if err := store.EnqueueVerifyQueueItem(context.Background(), model.VerifyQueueItem{ID: "q-type-1", ObjectType: model.VerifyQueueObjectNode, ObjectID: "n1", SourceArticleID: "u1", Priority: 10, ScheduledAt: now.Format(time.RFC3339), Status: model.VerifyQueueStatusQueued}); err != nil {
		t.Fatalf("EnqueueVerifyQueueItem() error = %v", err)
	}
	if err := store.EnqueueVerifyQueueItem(context.Background(), model.VerifyQueueItem{ID: "q-type-2", ObjectType: model.VerifyQueueObjectEdge, ObjectID: "e1", SourceArticleID: "u1", Priority: 10, ScheduledAt: now.Format(time.RFC3339), Status: model.VerifyQueueStatusQueued}); err != nil {
		t.Fatalf("EnqueueVerifyQueueItem() error = %v", err)
	}
	summary, err := store.GetVerifyQueueSummaryDetailed(context.Background(), now)
	if err != nil {
		t.Fatalf("GetVerifyQueueSummaryDetailed() error = %v", err)
	}
	if summary.ObjectTypes[model.VerifyQueueObjectNode] != 1 || summary.ObjectTypes[model.VerifyQueueObjectEdge] != 1 {
		t.Fatalf("summary object types = %#v, want node=1 edge=1", summary.ObjectTypes)
	}
	if summary.TotalCount != 2 {
		t.Fatalf("TotalCount = %d, want 2", summary.TotalCount)
	}
}

func TestSQLiteStore_GetVerifyQueueSummaryIncludesPendingAgeBuckets(t *testing.T) {
	root := t.TempDir()
	store, err := NewSQLiteStore(filepath.Join(root, "data", "content.db"))
	if err != nil {
		t.Fatalf("NewSQLiteStore() error = %v", err)
	}
	defer store.Close()
	now := time.Date(2026, 4, 21, 12, 0, 0, 0, time.UTC)
	for _, item := range []model.VerifyQueueItem{
		{ID: "q-age-future", ObjectType: model.VerifyQueueObjectNode, ObjectID: "n1", SourceArticleID: "u1", Priority: 10, ScheduledAt: now.Add(30 * time.Minute).Format(time.RFC3339), Status: model.VerifyQueueStatusQueued},
		{ID: "q-age-short", ObjectType: model.VerifyQueueObjectNode, ObjectID: "n2", SourceArticleID: "u1", Priority: 10, ScheduledAt: now.Add(-30 * time.Minute).Format(time.RFC3339), Status: model.VerifyQueueStatusQueued},
		{ID: "q-age-mid", ObjectType: model.VerifyQueueObjectEdge, ObjectID: "e1", SourceArticleID: "u1", Priority: 10, ScheduledAt: now.Add(-2 * time.Hour).Format(time.RFC3339), Status: model.VerifyQueueStatusRetry},
		{ID: "q-age-long", ObjectType: model.VerifyQueueObjectEdge, ObjectID: "e2", SourceArticleID: "u1", Priority: 10, ScheduledAt: now.Add(-48 * time.Hour).Format(time.RFC3339), Status: model.VerifyQueueStatusRetry},
	} {
		if err := store.EnqueueVerifyQueueItem(context.Background(), item); err != nil {
			t.Fatalf("EnqueueVerifyQueueItem(%s) error = %v", item.ID, err)
		}
	}
	summary, err := store.GetVerifyQueueSummaryDetailed(context.Background(), now)
	if err != nil {
		t.Fatalf("GetVerifyQueueSummaryDetailed() error = %v", err)
	}
	if summary.PendingAgeBuckets["future"] != 1 || summary.PendingAgeBuckets["overdue_lt_1h"] != 1 || summary.PendingAgeBuckets["overdue_1h_to_24h"] != 1 || summary.PendingAgeBuckets["overdue_gt_24h"] != 1 {
		t.Fatalf("PendingAgeBuckets = %#v, want one item in each bucket", summary.PendingAgeBuckets)
	}
}
