package main

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/kumaloha/VariX/varix/bootstrap"
	c "github.com/kumaloha/VariX/varix/compile"
	"github.com/kumaloha/VariX/varix/graphmodel"
	"github.com/kumaloha/VariX/varix/ingest/types"
	"github.com/kumaloha/VariX/varix/storage/contentstore"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestRunVerifyRunAndShowUseSeparateVerificationStore(t *testing.T) {
	prevBuildApp := buildApp
	prevBuildCompileClient := buildCompileClient
	prevOpenSQLiteStore := openSQLiteStore
	t.Cleanup(func() {
		buildApp = prevBuildApp
		buildCompileClient = prevBuildCompileClient
		openSQLiteStore = prevOpenSQLiteStore
	})

	tmp := t.TempDir()
	buildApp = func(projectRoot string) (*bootstrap.App, error) {
		app := &bootstrap.App{}
		app.Settings.ContentDBPath = tmp + "/content.db"
		return app, nil
	}
	buildCompileClient = func(projectRoot string) compileClient {
		return fakeCompileClient{
			verifyFn: func(ctx context.Context, bundle c.Bundle, output c.Output) (c.Verification, error) {
				return c.Verification{
					Model: "verify-model",
					FactChecks: []c.FactCheck{{
						NodeID: "n1",
						Status: c.FactStatusClearlyTrue,
						Reason: "supported",
					}},
					VerifiedAt: time.Now().UTC(),
				}, nil
			},
		}
	}
	openSQLiteStore = func(path string) (*contentstore.SQLiteStore, error) {
		store, err := contentstore.NewSQLiteStore(path)
		if err != nil {
			return nil, err
		}
		raw := types.RawContent{
			Source:     "weibo",
			ExternalID: "Q-verify",
			URL:        "https://weibo.com/123/Q-verify",
			Content:    "root body",
			AuthorName: "alice",
			PostedAt:   time.Now().UTC(),
		}
		if err := store.UpsertRawCapture(context.Background(), raw); err != nil {
			return nil, err
		}
		record := c.Record{
			UnitID:         "weibo:Q-verify",
			Source:         "weibo",
			ExternalID:     "Q-verify",
			RootExternalID: "Q-verify",
			Model:          c.Qwen36PlusModel,
			Output: c.Output{
				Summary: "一句话",
				Graph: c.ReasoningGraph{
					Nodes: []c.GraphNode{testGraphNode("n1", c.NodeFact, "事实A"), testGraphNode("n2", c.NodeConclusion, "结论B")},
					Edges: []c.GraphEdge{{From: "n1", To: "n2", Kind: c.EdgeDerives}},
				},
				Details: c.HiddenDetails{Caveats: []string{"说明"}},
			},
			CompiledAt: time.Now().UTC(),
		}
		if err := store.UpsertCompiledOutput(context.Background(), record); err != nil {
			return nil, err
		}
		return store, nil
	}

	var stdout, stderr bytes.Buffer
	code := run([]string{"verify", "run", "--platform", "weibo", "--id", "Q-verify"}, "/tmp/project", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("verify run code = %d, stderr = %s", code, stderr.String())
	}
	var verifyRecord c.VerificationRecord
	if err := json.Unmarshal(stdout.Bytes(), &verifyRecord); err != nil {
		t.Fatalf("json.Unmarshal(verify run) error = %v", err)
	}
	if verifyRecord.Model != "qwen3.6-plus" {
		t.Fatalf("verify record model = %q, want compile model persistence surface", verifyRecord.Model)
	}
	if len(verifyRecord.Verification.FactChecks) != 1 || verifyRecord.Verification.FactChecks[0].NodeID != "n1" {
		t.Fatalf("verify record = %#v", verifyRecord)
	}

	stdout.Reset()
	stderr.Reset()
	code = run([]string{"verify", "show", "--platform", "weibo", "--id", "Q-verify"}, "/tmp/project", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("verify show code = %d, stderr = %s", code, stderr.String())
	}
	var shown c.VerificationRecord
	if err := json.Unmarshal(stdout.Bytes(), &shown); err != nil {
		t.Fatalf("json.Unmarshal(verify show) error = %v", err)
	}
	if len(shown.Verification.FactChecks) != 1 || shown.Verification.FactChecks[0].Reason != "supported" {
		t.Fatalf("shown verification = %#v", shown)
	}
}

func TestRunVerifyRunAlsoAppliesVerificationToGraphFirstContentSubgraph(t *testing.T) {
	prevBuildApp := buildApp
	prevBuildCompileClient := buildCompileClient
	prevOpenSQLiteStore := openSQLiteStore
	t.Cleanup(func() {
		buildApp = prevBuildApp
		buildCompileClient = prevBuildCompileClient
		openSQLiteStore = prevOpenSQLiteStore
	})

	var dbPath string
	buildApp = func(projectRoot string) (*bootstrap.App, error) {
		app := &bootstrap.App{}
		dbPath = filepath.Join(t.TempDir(), "content.db")
		app.Settings.ContentDBPath = dbPath
		return app, nil
	}
	buildCompileClient = func(projectRoot string) compileClient {
		return compileClientStub{
			verifyDetailed: func(ctx context.Context, bundle c.Bundle, output c.Output) (c.Verification, error) {
				return c.Verification{
					FactChecks:       []c.FactCheck{{NodeID: "n1", Status: c.FactStatusClearlyTrue, Reason: "supported"}},
					PredictionChecks: []c.PredictionCheck{{NodeID: "n2", Status: c.PredictionStatusResolvedTrue, Reason: "resolved", AsOf: time.Now().UTC()}},
					VerifiedAt:       time.Now().UTC(),
				}, nil
			},
		}
	}
	openSQLiteStore = func(path string) (*contentstore.SQLiteStore, error) {
		store, err := contentstore.NewSQLiteStore(path)
		if err != nil {
			return nil, err
		}
		raw := types.RawContent{Source: "weibo", ExternalID: "Q-verify-graph", URL: "https://weibo.com/123/Q-verify-graph", Content: "root body", AuthorName: "alice", PostedAt: time.Now().UTC()}
		if err := store.UpsertRawCapture(context.Background(), raw); err != nil {
			return nil, err
		}
		record := c.Record{UnitID: "weibo:Q-verify-graph", Source: "weibo", ExternalID: "Q-verify-graph", RootExternalID: "Q-verify-graph", Model: c.Qwen36PlusModel, Output: c.Output{Summary: "一句话", Graph: c.ReasoningGraph{Nodes: []c.GraphNode{testGraphNode("n1", c.NodeFact, "事实A"), testGraphNode("n2", c.NodePrediction, "预测B")}, Edges: []c.GraphEdge{{From: "n1", To: "n2", Kind: c.EdgePositive}}}, Details: c.HiddenDetails{Caveats: []string{"说明"}}}, CompiledAt: time.Now().UTC()}
		if err := store.UpsertCompiledOutput(context.Background(), record); err != nil {
			return nil, err
		}
		return store, nil
	}
	var stdout, stderr bytes.Buffer
	code := run([]string{"verify", "run", "--platform", "weibo", "--id", "Q-verify-graph", "--force"}, "/tmp/project", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("verify run graph-first code = %d, stderr = %s", code, stderr.String())
	}
	reopen, err := contentstore.NewSQLiteStore(dbPath)
	if err != nil {
		t.Fatalf("NewSQLiteStore(reopen) error = %v", err)
	}
	defer reopen.Close()
	got, err := reopen.GetContentSubgraph(context.Background(), "weibo", "Q-verify-graph")
	if err != nil {
		t.Fatalf("GetContentSubgraph() error = %v", err)
	}
	statuses := map[string]string{}
	for _, node := range got.Nodes {
		statuses[node.ID] = string(node.VerificationStatus) + ":" + node.VerificationReason
	}
	if statuses["n1"] != "proved:supported" {
		t.Fatalf("n1 status = %q, want proved:supported", statuses["n1"])
	}
	if statuses["n2"] != "proved:resolved" {
		t.Fatalf("n2 status = %q, want proved:resolved", statuses["n2"])
	}
}

func TestRunVerifyQueueListPrintsQueuedItems(t *testing.T) {
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
		if err := store.EnqueueVerifyQueueItem(context.Background(), graphmodel.VerifyQueueItem{ID: "q-cli-1", ObjectType: graphmodel.VerifyQueueObjectNode, ObjectID: "n1", SourceArticleID: "unit-cli", Priority: 10, ScheduledAt: now.Format(time.RFC3339), Status: graphmodel.VerifyQueueStatusQueued}); err != nil {
			return nil, err
		}
		if err := store.EnqueueVerifyQueueItem(context.Background(), graphmodel.VerifyQueueItem{ID: "q-cli-2", ObjectType: graphmodel.VerifyQueueObjectEdge, ObjectID: "e1", SourceArticleID: "unit-cli", Priority: 5, ScheduledAt: now.Format(time.RFC3339), Status: graphmodel.VerifyQueueStatusQueued}); err != nil {
			return nil, err
		}
		if err := store.MarkVerifyQueueItemRunning(context.Background(), "q-cli-2", now); err != nil {
			return nil, err
		}
		return store, nil
	}

	var stdout, stderr bytes.Buffer
	code := run([]string{"verify", "queue", "--limit", "10"}, "/tmp/project", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("verify queue code = %d, stderr = %s", code, stderr.String())
	}
	var out []graphmodel.VerifyQueueItem
	if err := json.Unmarshal(stdout.Bytes(), &out); err != nil {
		t.Fatalf("json.Unmarshal(verify queue) error = %v", err)
	}
	if len(out) != 2 || out[0].ID != "q-cli-1" || out[1].ID != "q-cli-2" {
		t.Fatalf("verify queue output = %#v, want queued then running items", out)
	}
}

func TestRunVerifySweepProcessesQueueFromCurrentContentGraphState(t *testing.T) {
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
		subgraph := graphmodel.ContentSubgraph{ID: "sweep-cli", ArticleID: "sweep-cli", SourcePlatform: "twitter", SourceExternalID: "sweep-cli", CompileVersion: graphmodel.CompileBridgeVersion, CompiledAt: now.Format(time.RFC3339), UpdatedAt: now.Format(time.RFC3339), Nodes: []graphmodel.GraphNode{{ID: "n1", SourceArticleID: "sweep-cli", SourcePlatform: "twitter", SourceExternalID: "sweep-cli", RawText: "未来一周美股承压", SubjectText: "美股", ChangeText: "承压", Kind: graphmodel.NodeKindPrediction, GraphRole: graphmodel.GraphRoleTarget, IsPrimary: true, VerificationStatus: graphmodel.VerificationProved, VerificationReason: "resolved", VerificationAsOf: now.Format(time.RFC3339), TimeBucket: "1w"}}}
		if err := store.UpsertContentSubgraph(context.Background(), subgraph); err != nil {
			return nil, err
		}
		if err := store.EnqueueVerifyQueueItem(context.Background(), graphmodel.VerifyQueueItem{ID: "q-sweep-cli", ObjectType: graphmodel.VerifyQueueObjectNode, ObjectID: "n1", SourceArticleID: "sweep-cli", Priority: 10, ScheduledAt: now.Add(-time.Hour).Format(time.RFC3339), Status: graphmodel.VerifyQueueStatusQueued}); err != nil {
			return nil, err
		}
		return store, nil
	}

	var stdout, stderr bytes.Buffer
	code := run([]string{"verify", "sweep", "--limit", "10"}, "/tmp/project", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("verify sweep code = %d, stderr = %s", code, stderr.String())
	}
	var out contentstore.VerifyQueueSweepResult
	if err := json.Unmarshal(stdout.Bytes(), &out); err != nil {
		t.Fatalf("json.Unmarshal(verify sweep) error = %v", err)
	}
	if out.Claimed != 1 || out.Finished != 1 {
		t.Fatalf("verify sweep output = %#v, want claimed=1 finished=1", out)
	}
}

func TestRunVerifyQueueSupportsStatusFilter(t *testing.T) {
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
		if err := store.EnqueueVerifyQueueItem(context.Background(), graphmodel.VerifyQueueItem{ID: "q-filter-1", ObjectType: graphmodel.VerifyQueueObjectNode, ObjectID: "n1", SourceArticleID: "unit-cli", Priority: 10, ScheduledAt: now.Format(time.RFC3339), Status: graphmodel.VerifyQueueStatusQueued}); err != nil {
			return nil, err
		}
		if err := store.EnqueueVerifyQueueItem(context.Background(), graphmodel.VerifyQueueItem{ID: "q-filter-2", ObjectType: graphmodel.VerifyQueueObjectEdge, ObjectID: "e1", SourceArticleID: "unit-cli", Priority: 5, ScheduledAt: now.Format(time.RFC3339), Status: graphmodel.VerifyQueueStatusQueued}); err != nil {
			return nil, err
		}
		if err := store.MarkVerifyQueueItemRunning(context.Background(), "q-filter-2", now); err != nil {
			return nil, err
		}
		return store, nil
	}

	var stdout, stderr bytes.Buffer
	code := run([]string{"verify", "queue", "--limit", "10", "--status", "running"}, "/tmp/project", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("verify queue --status running code = %d, stderr = %s", code, stderr.String())
	}
	var out []graphmodel.VerifyQueueItem
	if err := json.Unmarshal(stdout.Bytes(), &out); err != nil {
		t.Fatalf("json.Unmarshal(verify queue --status) error = %v", err)
	}
	if len(out) != 1 || out[0].ID != "q-filter-2" {
		t.Fatalf("verify queue filtered output = %#v, want q-filter-2 only", out)
	}
}

func TestRunVerifyQueueSummaryPrintsCounts(t *testing.T) {
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
		if err := store.EnqueueVerifyQueueItem(context.Background(), graphmodel.VerifyQueueItem{ID: "q-summary-1", ObjectType: graphmodel.VerifyQueueObjectNode, ObjectID: "n1", SourceArticleID: "unit-cli", Priority: 10, ScheduledAt: now.Format(time.RFC3339), Status: graphmodel.VerifyQueueStatusQueued}); err != nil {
			return nil, err
		}
		if err := store.EnqueueVerifyQueueItem(context.Background(), graphmodel.VerifyQueueItem{ID: "q-summary-2", ObjectType: graphmodel.VerifyQueueObjectEdge, ObjectID: "e1", SourceArticleID: "unit-cli", Priority: 5, ScheduledAt: now.Format(time.RFC3339), Status: graphmodel.VerifyQueueStatusQueued}); err != nil {
			return nil, err
		}
		if err := store.MarkVerifyQueueItemRunning(context.Background(), "q-summary-2", now); err != nil {
			return nil, err
		}
		return store, nil
	}
	var stdout, stderr bytes.Buffer
	code := run([]string{"verify", "queue", "--summary"}, "/tmp/project", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("verify queue --summary code = %d, stderr = %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "queued") || !strings.Contains(stdout.String(), "running") || !strings.Contains(stdout.String(), "due_count") || !strings.Contains(stdout.String(), "object_types") {
		t.Fatalf("stdout = %q, want queued/running summary with due_count and object_types", stdout.String())
	}
	if !strings.Contains(stdout.String(), "total_count") {
		t.Fatalf("stdout = %q, want total_count in summary", stdout.String())
	}
	if !strings.Contains(stdout.String(), "pending_age_buckets") {
		t.Fatalf("stdout = %q, want pending_age_buckets in summary", stdout.String())
	}
}

func TestRunVerifyQueueSummaryIncludesEmptyPendingAgeBucketsWhenQueueIsEmpty(t *testing.T) {
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
	code := run([]string{"verify", "queue", "--summary"}, "/tmp/project", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("verify queue --summary code = %d, stderr = %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "pending_age_buckets") {
		t.Fatalf("stdout = %q, want empty pending_age_buckets object", stdout.String())
	}
}

func TestRunVerifyShowFallsBackToGraphFirstVerificationState(t *testing.T) {
	prevBuildApp := buildApp
	prevOpenSQLiteStore := openSQLiteStore
	t.Cleanup(func() { buildApp = prevBuildApp; openSQLiteStore = prevOpenSQLiteStore })

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
		subgraph := graphmodel.ContentSubgraph{ID: "verify-show-fallback", ArticleID: "verify-show-fallback", SourcePlatform: "twitter", SourceExternalID: "verify-show-fallback", CompileVersion: graphmodel.CompileBridgeVersion, CompiledAt: time.Now().UTC().Format(time.RFC3339), UpdatedAt: time.Now().UTC().Format(time.RFC3339), Nodes: []graphmodel.GraphNode{{ID: "n1", SourceArticleID: "verify-show-fallback", SourcePlatform: "twitter", SourceExternalID: "verify-show-fallback", RawText: "事实A", SubjectText: "事实A", ChangeText: "事实A", Kind: graphmodel.NodeKindObservation, IsPrimary: true, VerificationStatus: graphmodel.VerificationProved, VerificationReason: "supported", VerificationAsOf: time.Now().UTC().Format(time.RFC3339)}, {ID: "n2", SourceArticleID: "verify-show-fallback", SourcePlatform: "twitter", SourceExternalID: "verify-show-fallback", RawText: "未来一周结论B", SubjectText: "结论B", ChangeText: "未来一周结论B", Kind: graphmodel.NodeKindPrediction, IsPrimary: true, VerificationStatus: graphmodel.VerificationPending, VerificationReason: "waiting", VerificationAsOf: time.Now().UTC().Format(time.RFC3339), NextVerifyAt: time.Now().UTC().Add(time.Hour).Format(time.RFC3339)}}}
		if err := store.UpsertContentSubgraph(context.Background(), subgraph); err != nil {
			return nil, err
		}
		return store, nil
	}
	var stdout, stderr bytes.Buffer
	code := run([]string{"verify", "show", "--platform", "twitter", "--id", "verify-show-fallback"}, "/tmp/project", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("verify show fallback code = %d, stderr = %s", code, stderr.String())
	}
	var shown c.VerificationRecord
	if err := json.Unmarshal(stdout.Bytes(), &shown); err != nil {
		t.Fatalf("json.Unmarshal(verify show fallback) error = %v", err)
	}
	if len(shown.Verification.FactChecks) != 1 || shown.Verification.FactChecks[0].Reason != "supported" {
		t.Fatalf("fallback shown verification = %#v", shown)
	}
	if len(shown.Verification.PredictionChecks) != 1 || shown.Verification.PredictionChecks[0].Reason != "waiting" {
		t.Fatalf("fallback prediction checks = %#v", shown)
	}
}
