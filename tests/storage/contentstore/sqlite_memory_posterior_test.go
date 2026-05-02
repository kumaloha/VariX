package contentstore

import (
	"context"
	"errors"
	"github.com/kumaloha/VariX/varix/memory"
	"github.com/kumaloha/VariX/varix/model"
	"path/filepath"
	"slices"
	"testing"
	"time"
)

func TestSQLiteStore_OrganizationPreservesPosteriorStateAndDiagnosis(t *testing.T) {
	root := t.TempDir()
	store, err := NewSQLiteStore(filepath.Join(root, "data", "content.db"))
	if err != nil {
		t.Fatalf("NewSQLiteStore() error = %v", err)
	}
	defer store.Close()

	seedCompiledRecordForMemory(t, store)
	got, err := store.AcceptMemoryNodes(context.Background(), memory.AcceptRequest{
		UserID:           "u-posterior",
		SourcePlatform:   "weibo",
		SourceExternalID: "Q1",
		NodeIDs:          []string{"n1", "n2"},
	})
	if err != nil {
		t.Fatalf("AcceptMemoryNodes() error = %v", err)
	}

	var conclusion memory.AcceptedNode
	for _, node := range got.Nodes {
		if node.NodeID == "n2" {
			conclusion = node
			break
		}
	}
	if conclusion.MemoryID == 0 {
		t.Fatalf("accepted nodes = %#v, want conclusion node with memory id", got.Nodes)
	}

	now := time.Date(2026, 4, 15, 0, 0, 0, 0, time.UTC)
	if _, err := store.db.Exec(
		`INSERT INTO memory_posterior_states(source_platform, source_external_id, node_id, node_kind, state, diagnosis_code, reason, blocked_by_node_ids_json, last_evaluated_at, last_evidence_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(source_platform, source_external_id, node_id) DO UPDATE SET state = excluded.state, diagnosis_code = excluded.diagnosis_code, reason = excluded.reason, blocked_by_node_ids_json = excluded.blocked_by_node_ids_json, last_evaluated_at = excluded.last_evaluated_at, last_evidence_at = excluded.last_evidence_at, updated_at = excluded.updated_at`,
		conclusion.SourcePlatform,
		conclusion.SourceExternalID,
		conclusion.NodeID,
		conclusion.NodeKind,
		string(memory.PosteriorStateFalsified),
		string(memory.PosteriorDiagnosisLogicError),
		"conclusion contradicted by stronger evidence",
		`["n1"]`,
		now.Format(time.RFC3339Nano),
		now.Format(time.RFC3339Nano),
		now.Format(time.RFC3339Nano),
	); err != nil {
		t.Fatalf("seed memory_posterior_states error = %v", err)
	}

	out, err := store.RunNextMemoryOrganizationJob(context.Background(), "u-posterior", now)
	if err != nil {
		t.Fatalf("RunNextMemoryOrganizationJob() error = %v", err)
	}

	foundActiveNode := false
	for _, node := range out.ActiveNodes {
		if node.NodeID != "n2" {
			continue
		}
		foundActiveNode = true
		if node.PosteriorState != memory.PosteriorStateFalsified {
			t.Fatalf("PosteriorState = %q, want %q", node.PosteriorState, memory.PosteriorStateFalsified)
		}
		if node.PosteriorDiagnosis != memory.PosteriorDiagnosisLogicError {
			t.Fatalf("PosteriorDiagnosis = %q, want %q", node.PosteriorDiagnosis, memory.PosteriorDiagnosisLogicError)
		}
		if node.PosteriorReason != "conclusion contradicted by stronger evidence" {
			t.Fatalf("PosteriorReason = %q, want seeded reason", node.PosteriorReason)
		}
		if !slices.Equal(node.BlockedByNodeIDs, []string{"n1"}) {
			t.Fatalf("BlockedByNodeIDs = %#v, want [n1]", node.BlockedByNodeIDs)
		}
	}
	if !foundActiveNode {
		t.Fatalf("ActiveNodes = %#v, want posterior-tagged conclusion node", out.ActiveNodes)
	}

	foundHint := false
	for _, hint := range out.NodeHints {
		if hint.NodeID != "n2" {
			continue
		}
		foundHint = true
		if hint.PosteriorState != memory.PosteriorStateFalsified {
			t.Fatalf("NodeHint PosteriorState = %q, want %q", hint.PosteriorState, memory.PosteriorStateFalsified)
		}
		if hint.PosteriorDiagnosis != memory.PosteriorDiagnosisLogicError {
			t.Fatalf("NodeHint PosteriorDiagnosis = %q, want %q", hint.PosteriorDiagnosis, memory.PosteriorDiagnosisLogicError)
		}
		if !slices.Equal(hint.BlockedByNodeIDs, []string{"n1"}) {
			t.Fatalf("NodeHint BlockedByNodeIDs = %#v, want [n1]", hint.BlockedByNodeIDs)
		}
	}
	if !foundHint {
		t.Fatalf("NodeHints = %#v, want posterior-tagged hint for n2", out.NodeHints)
	}
}

func TestSQLiteStore_RunPosteriorVerificationPersistsRoundTripAndStalesPriorOutput(t *testing.T) {
	root := t.TempDir()
	store, err := NewSQLiteStore(filepath.Join(root, "data", "content.db"))
	if err != nil {
		t.Fatalf("NewSQLiteStore() error = %v", err)
	}
	defer store.Close()

	now := time.Now().UTC()
	record := model.Record{
		UnitID:         "weibo:Q-posterior-store",
		Source:         "weibo",
		ExternalID:     "Q-posterior-store",
		RootExternalID: "Q-posterior-store",
		Model:          "qwen3.6-plus",
		Output: model.Output{
			Summary: "summary",
			Graph: model.ReasoningGraph{
				Nodes: []model.GraphNode{
					{ID: "n1", Kind: model.NodeFact, Text: "事实A", OccurredAt: now.Add(-72 * time.Hour)},
					{ID: "n2", Kind: model.NodePrediction, Text: "预测B", PredictionStartAt: now.Add(-48 * time.Hour), PredictionDueAt: now.Add(-24 * time.Hour)},
				},
				Edges: []model.GraphEdge{{From: "n1", To: "n2", Kind: model.EdgeDerives}},
			},
			Details: model.HiddenDetails{Caveats: []string{"detail"}},
			Verification: model.Verification{
				PredictionChecks: []model.PredictionCheck{{
					NodeID: "n2", Status: model.PredictionStatusStaleUnresolved, Reason: "window passed", AsOf: now.Add(-12 * time.Hour),
				}},
			},
			Confidence: "medium",
		},
		CompiledAt: now.Add(-6 * time.Hour),
	}
	if err := store.UpsertCompiledOutput(context.Background(), record); err != nil {
		t.Fatalf("UpsertCompiledOutput() error = %v", err)
	}

	accepted, err := store.AcceptMemoryNodes(context.Background(), memory.AcceptRequest{
		UserID:           "u-posterior-store",
		SourcePlatform:   "weibo",
		SourceExternalID: "Q-posterior-store",
		NodeIDs:          []string{"n1", "n2"},
	})
	if err != nil {
		t.Fatalf("AcceptMemoryNodes() error = %v", err)
	}

	var prediction memory.AcceptedNode
	for _, node := range accepted.Nodes {
		if node.NodeID == "n2" {
			prediction = node
			break
		}
	}
	if prediction.MemoryID == 0 {
		t.Fatalf("accepted nodes = %#v, want prediction memory node", accepted.Nodes)
	}

	states, err := store.ListPosteriorStates(context.Background(), "u-posterior-store", "weibo", "Q-posterior-store")
	if err != nil {
		t.Fatalf("ListPosteriorStates() error = %v", err)
	}
	if len(states) != 1 || states[0].NodeID != "n2" {
		t.Fatalf("initial posterior states = %#v, want only prediction node", states)
	}
	if states[0].State != memory.PosteriorStatePending {
		t.Fatalf("initial state = %q, want pending", states[0].State)
	}

	firstOutput, err := store.RunNextMemoryOrganizationJob(context.Background(), "u-posterior-store", now)
	if err != nil {
		t.Fatalf("RunNextMemoryOrganizationJob() initial error = %v", err)
	}

	result, err := store.RunPosteriorVerification(context.Background(), memory.PosteriorRunRequest{
		UserID:           "u-posterior-store",
		SourcePlatform:   "weibo",
		SourceExternalID: "Q-posterior-store",
	}, now)
	if err != nil {
		t.Fatalf("RunPosteriorVerification() error = %v", err)
	}
	if len(result.Mutated) != 1 || result.Mutated[0].NodeID != "n2" {
		t.Fatalf("posterior mutated = %#v, want prediction node", result.Mutated)
	}
	if len(result.Refreshes) != 1 || result.Refreshes[0].JobID == 0 {
		t.Fatalf("posterior refreshes = %#v, want queued refresh job", result.Refreshes)
	}

	persisted, err := store.GetPosteriorState(context.Background(), prediction.MemoryID)
	if err != nil {
		t.Fatalf("GetPosteriorState() error = %v", err)
	}
	if persisted.State != memory.PosteriorStatePending {
		t.Fatalf("persisted state = %q, want pending", persisted.State)
	}
	if persisted.Reason != "prediction still unresolved after due time" {
		t.Fatalf("persisted reason = %q, want stale unresolved reason", persisted.Reason)
	}
	if persisted.UpdatedAt.IsZero() || persisted.LastEvaluatedAt.IsZero() {
		t.Fatalf("persisted timestamps = %#v, want evaluation/update timestamps", persisted)
	}

	states, err = store.ListPosteriorStates(context.Background(), "u-posterior-store", "weibo", "Q-posterior-store")
	if err != nil {
		t.Fatalf("ListPosteriorStates() after run error = %v", err)
	}
	if len(states) != 1 || states[0].Reason != persisted.Reason {
		t.Fatalf("posterior round-trip states = %#v, want persisted stale reason", states)
	}

	_, err = store.GetLatestMemoryOrganizationOutput(context.Background(), "u-posterior-store", "weibo", "Q-posterior-store")
	if !errors.Is(err, ErrMemoryOrganizationOutputStale) {
		t.Fatalf("GetLatestMemoryOrganizationOutput() error = %v, want stale error", err)
	}

	refreshed, err := store.RunNextMemoryOrganizationJob(context.Background(), "u-posterior-store", now.Add(time.Minute))
	if err != nil {
		t.Fatalf("RunNextMemoryOrganizationJob() refresh error = %v", err)
	}
	if refreshed.JobID == firstOutput.JobID {
		t.Fatalf("refresh job id = %d, want new job after posterior refresh", refreshed.JobID)
	}

	latest, err := store.GetLatestMemoryOrganizationOutput(context.Background(), "u-posterior-store", "weibo", "Q-posterior-store")
	if err != nil {
		t.Fatalf("GetLatestMemoryOrganizationOutput() refreshed error = %v", err)
	}
	foundPosteriorHint := false
	for _, hint := range latest.NodeHints {
		if hint.NodeID == "n2" && hint.PosteriorState == memory.PosteriorStatePending {
			foundPosteriorHint = true
			break
		}
	}
	if !foundPosteriorHint {
		t.Fatalf("refreshed NodeHints = %#v, want posterior pending hint", latest.NodeHints)
	}
}

func TestSQLiteStore_RunPosteriorVerificationSharesCanonicalStateAcrossAcceptedScopes(t *testing.T) {
	root := t.TempDir()
	store, err := NewSQLiteStore(filepath.Join(root, "data", "content.db"))
	if err != nil {
		t.Fatalf("NewSQLiteStore() error = %v", err)
	}
	defer store.Close()

	now := time.Now().UTC()
	record := model.Record{
		UnitID:         "weibo:Q-shared-posterior",
		Source:         "weibo",
		ExternalID:     "Q-shared-posterior",
		RootExternalID: "Q-shared-posterior",
		Model:          "qwen3.6-plus",
		Output: model.Output{
			Summary: "summary",
			Graph: model.ReasoningGraph{
				Nodes: []model.GraphNode{
					{ID: "n1", Kind: model.NodeFact, Text: "事实A", OccurredAt: now.Add(-72 * time.Hour)},
					{ID: "n2", Kind: model.NodePrediction, Text: "预测B", PredictionStartAt: now.Add(-48 * time.Hour), PredictionDueAt: now.Add(-24 * time.Hour)},
				},
				Edges: []model.GraphEdge{{From: "n1", To: "n2", Kind: model.EdgeDerives}},
			},
			Details: model.HiddenDetails{Caveats: []string{"detail"}},
			Verification: model.Verification{
				PredictionChecks: []model.PredictionCheck{{
					NodeID: "n2", Status: model.PredictionStatusStaleUnresolved, Reason: "window passed", AsOf: now.Add(-12 * time.Hour),
				}},
			},
			Confidence: "medium",
		},
		CompiledAt: now.Add(-6 * time.Hour),
	}
	if err := store.UpsertCompiledOutput(context.Background(), record); err != nil {
		t.Fatalf("UpsertCompiledOutput() error = %v", err)
	}

	userA, err := store.AcceptMemoryNodes(context.Background(), memory.AcceptRequest{
		UserID:           "u-shared-a",
		SourcePlatform:   "weibo",
		SourceExternalID: "Q-shared-posterior",
		NodeIDs:          []string{"n1", "n2"},
	})
	if err != nil {
		t.Fatalf("AcceptMemoryNodes(userA) error = %v", err)
	}
	userB, err := store.AcceptMemoryNodes(context.Background(), memory.AcceptRequest{
		UserID:           "u-shared-b",
		SourcePlatform:   "weibo",
		SourceExternalID: "Q-shared-posterior",
		NodeIDs:          []string{"n1", "n2"},
	})
	if err != nil {
		t.Fatalf("AcceptMemoryNodes(userB) error = %v", err)
	}

	firstA, err := store.RunNextMemoryOrganizationJob(context.Background(), "u-shared-a", now)
	if err != nil {
		t.Fatalf("RunNextMemoryOrganizationJob(userA) initial error = %v", err)
	}
	firstB, err := store.RunNextMemoryOrganizationJob(context.Background(), "u-shared-b", now)
	if err != nil {
		t.Fatalf("RunNextMemoryOrganizationJob(userB) initial error = %v", err)
	}

	result, err := store.RunPosteriorVerification(context.Background(), memory.PosteriorRunRequest{
		UserID:           "u-shared-a",
		SourcePlatform:   "weibo",
		SourceExternalID: "Q-shared-posterior",
	}, now)
	if err != nil {
		t.Fatalf("RunPosteriorVerification() error = %v", err)
	}
	if len(result.Mutated) != 1 || result.Mutated[0].NodeID != "n2" {
		t.Fatalf("Mutated = %#v, want one canonical prediction state", result.Mutated)
	}
	if len(result.Refreshes) != 2 {
		t.Fatalf("Refreshes = %#v, want one refresh per accepted user scope", result.Refreshes)
	}

	refreshByUser := make(map[string]memory.PosteriorRefreshTrigger, len(result.Refreshes))
	for _, refresh := range result.Refreshes {
		refreshByUser[refresh.UserID] = refresh
	}
	for _, userID := range []string{"u-shared-a", "u-shared-b"} {
		refresh, ok := refreshByUser[userID]
		if !ok {
			t.Fatalf("Refreshes = %#v, missing refresh for %s", result.Refreshes, userID)
		}
		if !slices.Equal(refresh.AffectedNodeIDs, []string{"n2"}) {
			t.Fatalf("refresh[%s].AffectedNodeIDs = %#v, want [n2]", userID, refresh.AffectedNodeIDs)
		}
		if len(refresh.AffectedMemoryIDs) != 1 || refresh.AffectedMemoryIDs[0] == 0 {
			t.Fatalf("refresh[%s].AffectedMemoryIDs = %#v, want projected memory id", userID, refresh.AffectedMemoryIDs)
		}
	}

	var predictionA, predictionB memory.AcceptedNode
	for _, node := range userA.Nodes {
		if node.NodeID == "n2" {
			predictionA = node
		}
	}
	for _, node := range userB.Nodes {
		if node.NodeID == "n2" {
			predictionB = node
		}
	}
	if predictionA.MemoryID == 0 || predictionB.MemoryID == 0 {
		t.Fatalf("accepted prediction nodes = %#v / %#v, want memory ids", userA.Nodes, userB.Nodes)
	}

	stateA, err := store.GetPosteriorState(context.Background(), predictionA.MemoryID)
	if err != nil {
		t.Fatalf("GetPosteriorState(userA) error = %v", err)
	}
	stateB, err := store.GetPosteriorState(context.Background(), predictionB.MemoryID)
	if err != nil {
		t.Fatalf("GetPosteriorState(userB) error = %v", err)
	}
	if stateA.State != memory.PosteriorStatePending || stateB.State != memory.PosteriorStatePending {
		t.Fatalf("shared states = %#v / %#v, want pending", stateA, stateB)
	}
	if stateA.Reason != "prediction still unresolved after due time" || stateB.Reason != stateA.Reason {
		t.Fatalf("shared reasons = %q / %q, want shared canonical stale reason", stateA.Reason, stateB.Reason)
	}
	if !stateA.UpdatedAt.Equal(stateB.UpdatedAt) {
		t.Fatalf("shared UpdatedAt = %s / %s, want canonical timestamp", stateA.UpdatedAt, stateB.UpdatedAt)
	}

	statesB, err := store.ListPosteriorStates(context.Background(), "u-shared-b", "weibo", "Q-shared-posterior")
	if err != nil {
		t.Fatalf("ListPosteriorStates(userB) error = %v", err)
	}
	if len(statesB) != 1 || statesB[0].NodeID != "n2" || statesB[0].Reason != stateA.Reason {
		t.Fatalf("ListPosteriorStates(userB) = %#v, want projected shared posterior state", statesB)
	}

	if _, err := store.GetLatestMemoryOrganizationOutput(context.Background(), "u-shared-a", "weibo", "Q-shared-posterior"); !errors.Is(err, ErrMemoryOrganizationOutputStale) {
		t.Fatalf("GetLatestMemoryOrganizationOutput(userA) error = %v, want stale", err)
	}
	if _, err := store.GetLatestMemoryOrganizationOutput(context.Background(), "u-shared-b", "weibo", "Q-shared-posterior"); !errors.Is(err, ErrMemoryOrganizationOutputStale) {
		t.Fatalf("GetLatestMemoryOrganizationOutput(userB) error = %v, want stale", err)
	}

	refreshedA, err := store.RunNextMemoryOrganizationJob(context.Background(), "u-shared-a", now.Add(time.Minute))
	if err != nil {
		t.Fatalf("RunNextMemoryOrganizationJob(userA) refresh error = %v", err)
	}
	refreshedB, err := store.RunNextMemoryOrganizationJob(context.Background(), "u-shared-b", now.Add(time.Minute))
	if err != nil {
		t.Fatalf("RunNextMemoryOrganizationJob(userB) refresh error = %v", err)
	}
	if refreshedA.JobID == firstA.JobID || refreshedB.JobID == firstB.JobID {
		t.Fatalf("refresh job ids = %d / %d, want new jobs after shared posterior refresh", refreshedA.JobID, refreshedB.JobID)
	}
}
