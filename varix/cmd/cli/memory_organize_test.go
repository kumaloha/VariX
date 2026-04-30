package main

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/kumaloha/VariX/varix/bootstrap"
	c "github.com/kumaloha/VariX/varix/compile"
	"github.com/kumaloha/VariX/varix/memory"
	"github.com/kumaloha/VariX/varix/storage/contentstore"
)

func TestRunMemoryPosteriorRunRequiresUser(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{"memory", "posterior-run"}, "/tmp/project", &stdout, &stderr)
	if code != 2 {
		t.Fatalf("run() code = %d, want 2", code)
	}
	if !strings.Contains(stderr.String(), "usage: varix memory posterior-run") {
		t.Fatalf("stderr = %q, want usage", stderr.String())
	}
}

func TestRunMemoryOrganizeRunAndShow(t *testing.T) {
	prevBuildApp := buildApp
	prevOpenSQLiteStore := openSQLiteStore
	t.Cleanup(func() {
		buildApp = prevBuildApp
		openSQLiteStore = prevOpenSQLiteStore
	})

	tmp := t.TempDir()
	buildApp = func(projectRoot string) (*bootstrap.App, error) {
		app := &bootstrap.App{}
		app.Settings.ContentDBPath = tmp + "/content.db"
		return app, nil
	}
	openSQLiteStore = func(path string) (*contentstore.SQLiteStore, error) {
		store, err := contentstore.NewSQLiteStore(path)
		if err != nil {
			return nil, err
		}
		record := c.Record{
			UnitID:         "weibo:Q1",
			Source:         "weibo",
			ExternalID:     "Q1",
			RootExternalID: "Q1",
			Model:          c.Qwen36PlusModel,
			Output: c.Output{
				Summary: "一句话",
				Graph: c.ReasoningGraph{
					Nodes: []c.GraphNode{
						testGraphNode("n1", c.NodeFact, "通胀下降"),
						testGraphNode("n2", c.NodePrediction, "三个月内降息"),
					},
					Edges: []c.GraphEdge{{From: "n1", To: "n2", Kind: c.EdgeDerives}},
				},
				Details: c.HiddenDetails{Caveats: []string{"说明"}},
				Verification: c.Verification{
					VerifiedAt: time.Now().UTC(),
					Model:      c.Qwen36PlusModel,
					FactChecks: []c.FactCheck{{NodeID: "n1", Status: c.FactStatusClearlyTrue, Reason: "supported"}},
					PredictionChecks: []c.PredictionCheck{{
						NodeID: "n2", Status: c.PredictionStatusUnresolved, Reason: "still active", AsOf: time.Now().UTC(),
					}},
				},
			},
			CompiledAt: time.Now().UTC(),
		}
		if err := store.UpsertCompiledOutput(context.Background(), record); err != nil {
			return nil, err
		}
		if _, err := store.AcceptMemoryNodes(context.Background(), memory.AcceptRequest{
			UserID:           "u1",
			SourcePlatform:   "weibo",
			SourceExternalID: "Q1",
			NodeIDs:          []string{"n1", "n2"},
		}); err != nil {
			return nil, err
		}
		return store, nil
	}

	var stdout, stderr bytes.Buffer
	code := run([]string{"memory", "organize-run", "--user", "u1"}, "/tmp/project", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("organize-run code = %d, stderr = %s", code, stderr.String())
	}
	var out memory.OrganizationOutput
	if err := json.Unmarshal(stdout.Bytes(), &out); err != nil {
		t.Fatalf("json.Unmarshal(organize-run) error = %v", err)
	}
	if out.JobID == 0 {
		t.Fatalf("output = %#v", out)
	}

	stdout.Reset()
	stderr.Reset()
	code = run([]string{"memory", "organized", "--user", "u1", "--platform", "weibo", "--id", "Q1"}, "/tmp/project", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("organized code = %d, stderr = %s", code, stderr.String())
	}
	var got memory.OrganizationOutput
	if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
		t.Fatalf("json.Unmarshal(organized) error = %v", err)
	}
	if got.JobID != out.JobID {
		t.Fatalf("JobID = %d, want %d", got.JobID, out.JobID)
	}
}

func TestRunMemoryOrganizedIncludesFrontendHints(t *testing.T) {
	prevBuildApp := buildApp
	prevOpenSQLiteStore := openSQLiteStore
	t.Cleanup(func() {
		buildApp = prevBuildApp
		openSQLiteStore = prevOpenSQLiteStore
	})

	tmp := t.TempDir()
	buildApp = func(projectRoot string) (*bootstrap.App, error) {
		app := &bootstrap.App{}
		app.Settings.ContentDBPath = tmp + "/content.db"
		return app, nil
	}
	openSQLiteStore = func(path string) (*contentstore.SQLiteStore, error) {
		store, err := contentstore.NewSQLiteStore(path)
		if err != nil {
			return nil, err
		}
		record := c.Record{
			UnitID:         "weibo:Q2",
			Source:         "weibo",
			ExternalID:     "Q2",
			RootExternalID: "Q2",
			Model:          c.Qwen36PlusModel,
			Output: c.Output{
				Summary: "一句话",
				Graph: c.ReasoningGraph{
					Nodes: []c.GraphNode{
						testGraphNode("n1", c.NodeFact, "油价会上升"),
						testGraphNode("n2", c.NodeFact, "油价将上行"),
						testGraphNode("n3", c.NodeConclusion, "能源股受益"),
					},
					Edges: []c.GraphEdge{{From: "n1", To: "n3", Kind: c.EdgePositive}},
				},
				Details: c.HiddenDetails{Caveats: []string{"说明"}},
				Verification: c.Verification{
					VerifiedAt: time.Now().UTC(),
					Model:      c.Qwen36PlusModel,
					FactChecks: []c.FactCheck{{NodeID: "n1", Status: c.FactStatusClearlyTrue, Reason: "supported"}},
				},
			},
			CompiledAt: time.Now().UTC(),
		}
		if err := store.UpsertCompiledOutput(context.Background(), record); err != nil {
			return nil, err
		}
		if _, err := store.AcceptMemoryNodes(context.Background(), memory.AcceptRequest{
			UserID:           "u2",
			SourcePlatform:   "weibo",
			SourceExternalID: "Q2",
			NodeIDs:          []string{"n1", "n2", "n3"},
		}); err != nil {
			return nil, err
		}
		return store, nil
	}

	var stdout, stderr bytes.Buffer
	code := run([]string{"memory", "organize-run", "--user", "u2"}, "/tmp/project", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("organize-run code = %d, stderr = %s", code, stderr.String())
	}

	var payload map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
		t.Fatalf("json.Unmarshal(organize-run payload) error = %v", err)
	}

	dedupeGroups, ok := payload["dedupe_groups"].([]any)
	if !ok || len(dedupeGroups) != 1 {
		t.Fatalf("dedupe_groups = %#v, want one frontend-ready group", payload["dedupe_groups"])
	}
	firstDedupe, ok := dedupeGroups[0].(map[string]any)
	if !ok {
		t.Fatalf("dedupe_groups[0] = %#v, want object", dedupeGroups[0])
	}
	if strings.TrimSpace(stringValue(firstDedupe["canonical_text"])) == "" {
		t.Fatalf("dedupe group missing canonical_text: %#v", firstDedupe)
	}
	if strings.TrimSpace(stringValue(firstDedupe["hint"])) == "" {
		t.Fatalf("dedupe group missing hint: %#v", firstDedupe)
	}

	hierarchy, ok := payload["hierarchy"].([]any)
	if !ok || len(hierarchy) == 0 {
		t.Fatalf("hierarchy = %#v, want frontend-ready link entries", payload["hierarchy"])
	}
	firstLink, ok := hierarchy[0].(map[string]any)
	if !ok {
		t.Fatalf("hierarchy[0] = %#v, want object", hierarchy[0])
	}
	for _, key := range []string{"parent_kind", "child_kind", "source", "hint"} {
		if strings.TrimSpace(stringValue(firstLink[key])) == "" {
			t.Fatalf("hierarchy link missing %s: %#v", key, firstLink)
		}
	}
}

func TestRunMemoryOrganizedIncludesDominantDriverFeedbackAndVerdicts(t *testing.T) {
	prevBuildApp := buildApp
	prevOpenSQLiteStore := openSQLiteStore
	t.Cleanup(func() {
		buildApp = prevBuildApp
		openSQLiteStore = prevOpenSQLiteStore
	})

	tmp := t.TempDir()
	buildApp = func(projectRoot string) (*bootstrap.App, error) {
		app := &bootstrap.App{}
		app.Settings.ContentDBPath = tmp + "/content.db"
		return app, nil
	}
	openSQLiteStore = func(path string) (*contentstore.SQLiteStore, error) {
		store, err := contentstore.NewSQLiteStore(path)
		if err != nil {
			return nil, err
		}
		record := c.Record{
			UnitID:         "weibo:Q-driver-cli",
			Source:         "weibo",
			ExternalID:     "Q-driver-cli",
			RootExternalID: "Q-driver-cli",
			Model:          c.Qwen36PlusModel,
			Output: c.Output{
				Summary: "一句话",
				Graph: c.ReasoningGraph{
					Nodes: []c.GraphNode{
						testGraphNode("n1", c.NodeFact, "美元走弱"),
						testGraphNode("n2", c.NodeFact, "风险偏好回升"),
						testGraphNode("n3", c.NodeConclusion, "黄金获得支撑"),
						testGraphNode("n4", c.NodePrediction, "金价继续走高"),
					},
					Edges: []c.GraphEdge{
						{From: "n1", To: "n3", Kind: c.EdgePositive},
						{From: "n2", To: "n3", Kind: c.EdgePositive},
						{From: "n3", To: "n4", Kind: c.EdgeDerives},
					},
				},
				Details: c.HiddenDetails{Caveats: []string{"说明"}},
				Verification: c.Verification{
					VerifiedAt: time.Now().UTC(),
					Model:      c.Qwen36PlusModel,
					FactChecks: []c.FactCheck{
						{NodeID: "n1", Status: c.FactStatusClearlyTrue, Reason: "confirmed by price action"},
						{NodeID: "n2", Status: c.FactStatusUnverifiable, Reason: "support remains thin"},
					},
					PredictionChecks: []c.PredictionCheck{
						{NodeID: "n4", Status: c.PredictionStatusResolvedFalse, Reason: "price broke lower", AsOf: time.Now().UTC()},
					},
				},
			},
			CompiledAt: time.Now().UTC(),
		}
		if err := store.UpsertCompiledOutput(context.Background(), record); err != nil {
			return nil, err
		}
		if _, err := store.AcceptMemoryNodes(context.Background(), memory.AcceptRequest{
			UserID:           "u-driver-cli",
			SourcePlatform:   "weibo",
			SourceExternalID: "Q-driver-cli",
			NodeIDs:          []string{"n1", "n2", "n3", "n4"},
		}); err != nil {
			return nil, err
		}
		return store, nil
	}

	var stdout, stderr bytes.Buffer
	code := run([]string{"memory", "organize-run", "--user", "u-driver-cli"}, "/tmp/project", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("organize-run code = %d, stderr = %s", code, stderr.String())
	}

	stdout.Reset()
	stderr.Reset()
	code = run([]string{"memory", "organized", "--user", "u-driver-cli", "--platform", "weibo", "--id", "Q-driver-cli"}, "/tmp/project", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("organized code = %d, stderr = %s", code, stderr.String())
	}

	var payload map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
		t.Fatalf("json.Unmarshal(organized payload) error = %v", err)
	}

	dominantDriver, ok := payload["dominant_driver"].(map[string]any)
	if !ok {
		t.Fatalf("dominant_driver = %#v, want object", payload["dominant_driver"])
	}
	if stringValue(dominantDriver["node_id"]) != "n1" {
		t.Fatalf("dominant_driver.node_id = %#v, want n1", dominantDriver["node_id"])
	}
	if !strings.Contains(stringValue(dominantDriver["explanation"]), "primary") || !strings.Contains(stringValue(dominantDriver["explanation"]), "supporting") {
		t.Fatalf("dominant_driver.explanation = %#v, want primary vs supporting explanation", dominantDriver["explanation"])
	}

	feedback, ok := payload["feedback"].([]any)
	if !ok || len(feedback) == 0 {
		t.Fatalf("feedback = %#v, want strongest-error-first list", payload["feedback"])
	}
	firstFeedback, ok := feedback[0].(map[string]any)
	if !ok {
		t.Fatalf("feedback[0] = %#v, want object", feedback[0])
	}
	if stringValue(firstFeedback["node_id"]) != "n4" || stringValue(firstFeedback["severity"]) != "error" {
		t.Fatalf("feedback[0] = %#v, want error-ranked prediction failure", firstFeedback)
	}

	nodeHints, ok := payload["node_hints"].([]any)
	if !ok {
		t.Fatalf("node_hints = %#v, want array", payload["node_hints"])
	}
	hintsByID := map[string]map[string]any{}
	for _, raw := range nodeHints {
		hint, ok := raw.(map[string]any)
		if !ok {
			t.Fatalf("node_hints entry = %#v, want object", raw)
		}
		hintsByID[stringValue(hint["node_id"])] = hint
	}
	if got := hintsByID["n1"]; stringValue(got["node_verdict"]) != "supported" || stringValue(got["driver_role"]) != "primary" {
		t.Fatalf("hint[n1] = %#v, want supported primary driver", got)
	}
	if got := hintsByID["n2"]; stringValue(got["node_verdict"]) != "needs_review" || stringValue(got["driver_role"]) != "supporting" {
		t.Fatalf("hint[n2] = %#v, want needs_review supporting driver", got)
	}
}

func TestRunMemoryPosteriorRunMarksOrganizedOutputStaleUntilRefreshRun(t *testing.T) {
	prevBuildApp := buildApp
	prevOpenSQLiteStore := openSQLiteStore
	t.Cleanup(func() {
		buildApp = prevBuildApp
		openSQLiteStore = prevOpenSQLiteStore
	})

	tmp := t.TempDir()
	buildApp = func(projectRoot string) (*bootstrap.App, error) {
		app := &bootstrap.App{}
		app.Settings.ContentDBPath = tmp + "/content.db"
		return app, nil
	}
	openSQLiteStore = func(path string) (*contentstore.SQLiteStore, error) {
		store, err := contentstore.NewSQLiteStore(path)
		if err != nil {
			return nil, err
		}
		now := time.Now().UTC()
		record := c.Record{
			UnitID:         "weibo:Q-posterior-cli",
			Source:         "weibo",
			ExternalID:     "Q-posterior-cli",
			RootExternalID: "Q-posterior-cli",
			Model:          c.Qwen36PlusModel,
			Output: c.Output{
				Summary: "一句话",
				Graph: c.ReasoningGraph{
					Nodes: []c.GraphNode{
						{ID: "n1", Kind: c.NodeFact, Text: "事实A", OccurredAt: now.Add(-72 * time.Hour)},
						{ID: "n2", Kind: c.NodePrediction, Text: "预测B", PredictionStartAt: now.Add(-48 * time.Hour), PredictionDueAt: now.Add(-24 * time.Hour)},
					},
					Edges: []c.GraphEdge{{From: "n1", To: "n2", Kind: c.EdgeDerives}},
				},
				Details: c.HiddenDetails{Caveats: []string{"说明"}},
				Verification: c.Verification{
					PredictionChecks: []c.PredictionCheck{{
						NodeID: "n2", Status: c.PredictionStatusStaleUnresolved, Reason: "window passed", AsOf: now.Add(-12 * time.Hour),
					}},
				},
			},
			CompiledAt: now.Add(-6 * time.Hour),
		}
		if err := store.UpsertCompiledOutput(context.Background(), record); err != nil {
			return nil, err
		}
		if _, err := store.AcceptMemoryNodes(context.Background(), memory.AcceptRequest{
			UserID:           "u-posterior-cli",
			SourcePlatform:   "weibo",
			SourceExternalID: "Q-posterior-cli",
			NodeIDs:          []string{"n1", "n2"},
		}); err != nil {
			return nil, err
		}
		return store, nil
	}

	var stdout, stderr bytes.Buffer
	code := run([]string{"memory", "organize-run", "--user", "u-posterior-cli"}, "/tmp/project", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("organize-run code = %d, stderr = %s", code, stderr.String())
	}

	stdout.Reset()
	stderr.Reset()
	code = run([]string{"memory", "posterior-run", "--user", "u-posterior-cli", "--platform", "weibo", "--id", "Q-posterior-cli"}, "/tmp/project", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("posterior-run code = %d, stderr = %s", code, stderr.String())
	}
	var posterior memory.PosteriorRunResult
	if err := json.Unmarshal(stdout.Bytes(), &posterior); err != nil {
		t.Fatalf("json.Unmarshal(posterior-run) error = %v", err)
	}
	if len(posterior.Mutated) != 1 || posterior.Mutated[0].NodeID != "n2" {
		t.Fatalf("posterior mutated = %#v, want one mutated prediction node", posterior.Mutated)
	}
	if posterior.Mutated[0].State != memory.PosteriorStatePending {
		t.Fatalf("posterior state = %q, want pending", posterior.Mutated[0].State)
	}
	if len(posterior.Refreshes) != 1 || posterior.Refreshes[0].JobID == 0 {
		t.Fatalf("posterior refreshes = %#v, want one queued refresh job", posterior.Refreshes)
	}

	stdout.Reset()
	stderr.Reset()
	code = run([]string{"memory", "organized", "--user", "u-posterior-cli", "--platform", "weibo", "--id", "Q-posterior-cli"}, "/tmp/project", &stdout, &stderr)
	if code != 1 {
		t.Fatalf("organized stale code = %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), "memory organization output is stale") {
		t.Fatalf("stderr = %q, want stale output error", stderr.String())
	}
	if !strings.Contains(stderr.String(), "memory organize-run --user u-posterior-cli") {
		t.Fatalf("stderr = %q, want rerun guidance", stderr.String())
	}

	stdout.Reset()
	stderr.Reset()
	code = run([]string{"memory", "organize-run", "--user", "u-posterior-cli"}, "/tmp/project", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("refresh organize-run code = %d, stderr = %s", code, stderr.String())
	}

	stdout.Reset()
	stderr.Reset()
	code = run([]string{"memory", "organized", "--user", "u-posterior-cli", "--platform", "weibo", "--id", "Q-posterior-cli"}, "/tmp/project", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("organized refreshed code = %d, stderr = %s", code, stderr.String())
	}
	var organized memory.OrganizationOutput
	if err := json.Unmarshal(stdout.Bytes(), &organized); err != nil {
		t.Fatalf("json.Unmarshal(refreshed organized) error = %v", err)
	}
	foundPosteriorHint := false
	for _, hint := range organized.NodeHints {
		if hint.NodeID == "n2" && hint.PosteriorState == memory.PosteriorStatePending {
			foundPosteriorHint = true
			break
		}
	}
	if !foundPosteriorHint {
		t.Fatalf("NodeHints = %#v, want posterior pending hint for n2", organized.NodeHints)
	}
}

func TestRunMemoryOrganizedWithoutOutputShowsRunGuidance(t *testing.T) {
	prevBuildApp := buildApp
	prevOpenSQLiteStore := openSQLiteStore
	t.Cleanup(func() {
		buildApp = prevBuildApp
		openSQLiteStore = prevOpenSQLiteStore
	})

	tmp := t.TempDir()
	buildApp = func(projectRoot string) (*bootstrap.App, error) {
		app := &bootstrap.App{}
		app.Settings.ContentDBPath = tmp + "/content.db"
		return app, nil
	}
	openSQLiteStore = func(path string) (*contentstore.SQLiteStore, error) {
		return contentstore.NewSQLiteStore(path)
	}

	var stdout, stderr bytes.Buffer
	code := run([]string{"memory", "organized", "--user", "u-empty-memory", "--platform", "weibo", "--id", "Q-empty"}, "/tmp/project", &stdout, &stderr)
	if code != 1 {
		t.Fatalf("organized code = %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), "memory organize-run --user u-empty-memory") {
		t.Fatalf("stderr = %q, want organize-run guidance", stderr.String())
	}
}
