package main

import (
	"bytes"
	"context"
	"encoding/json"
	c "github.com/kumaloha/VariX/varix/compile"
	"github.com/kumaloha/VariX/varix/ingest"
	varixllm "github.com/kumaloha/VariX/varix/llm"
	"github.com/kumaloha/VariX/varix/memory"
	"github.com/kumaloha/VariX/varix/model"
	"github.com/kumaloha/VariX/varix/storage/contentstore"
	"strings"
	"testing"
	"time"
)

func TestRunMemoryEventGraphsSupportsScopeFilter(t *testing.T) {
	prevNewIngestRuntime := newIngestRuntime
	prevOpenSQLiteStore := openSQLiteStore
	t.Cleanup(func() {
		newIngestRuntime = prevNewIngestRuntime
		openSQLiteStore = prevOpenSQLiteStore
	})

	tmp := t.TempDir()
	newIngestRuntime = func(projectRoot string) (*ingest.Runtime, error) {
		app := &ingest.Runtime{}
		app.Settings.ContentDBPath = tmp + "/content.db"
		return app, nil
	}
	openSQLiteStore = func(path string) (*contentstore.SQLiteStore, error) {
		store, err := contentstore.NewSQLiteStore(path)
		if err != nil {
			return nil, err
		}
		sg := testDriverTargetSubgraph("filter-eg", time.Now().UTC())
		if err := store.PersistMemoryContentGraph(context.Background(), "u-event-filter", sg, time.Now().UTC()); err != nil {
			return nil, err
		}
		return store, nil
	}
	var stdout, stderr bytes.Buffer
	code := run([]string{"memory", "event-graphs", "--user", "u-event-filter", "--scope", "target"}, "/tmp/project", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("event-graphs --scope target code = %d, stderr = %s", code, stderr.String())
	}
	var out []contentstore.EventGraphRecord
	if err := json.Unmarshal(stdout.Bytes(), &out); err != nil {
		t.Fatalf("json.Unmarshal(event-graphs --scope) error = %v", err)
	}
	if len(out) != 1 || out[0].Scope != "target" {
		t.Fatalf("event-graphs filtered output = %#v, want one target graph", out)
	}
}

func TestRunMemoryParadigmsSupportsSubjectFilter(t *testing.T) {
	prevNewIngestRuntime := newIngestRuntime
	prevOpenSQLiteStore := openSQLiteStore
	t.Cleanup(func() {
		newIngestRuntime = prevNewIngestRuntime
		openSQLiteStore = prevOpenSQLiteStore
	})

	tmp := t.TempDir()
	newIngestRuntime = func(projectRoot string) (*ingest.Runtime, error) {
		app := &ingest.Runtime{}
		app.Settings.ContentDBPath = tmp + "/content.db"
		return app, nil
	}
	openSQLiteStore = func(path string) (*contentstore.SQLiteStore, error) {
		store, err := contentstore.NewSQLiteStore(path)
		if err != nil {
			return nil, err
		}
		sg := testDriverTargetSubgraph("filter-pg", time.Now().UTC())
		if err := store.PersistMemoryContentGraph(context.Background(), "u-paradigm-filter", sg, time.Now().UTC()); err != nil {
			return nil, err
		}
		return store, nil
	}
	var stdout, stderr bytes.Buffer
	code := run([]string{"memory", "paradigms", "--user", "u-paradigm-filter", "--subject", "美联储"}, "/tmp/project", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("paradigms --subject 美联储 code = %d, stderr = %s", code, stderr.String())
	}
	var out []contentstore.ParadigmRecord
	if err := json.Unmarshal(stdout.Bytes(), &out); err != nil {
		t.Fatalf("json.Unmarshal(paradigms --subject) error = %v", err)
	}
	if len(out) != 1 || out[0].DriverSubject != "美联储" {
		t.Fatalf("paradigms filtered output = %#v, want 美联储 paradigm", out)
	}
}

func TestRunMemoryContentGraphsSupportsSourceFilter(t *testing.T) {
	prevNewIngestRuntime := newIngestRuntime
	prevOpenSQLiteStore := openSQLiteStore
	t.Cleanup(func() {
		newIngestRuntime = prevNewIngestRuntime
		openSQLiteStore = prevOpenSQLiteStore
	})

	tmp := t.TempDir()
	newIngestRuntime = func(projectRoot string) (*ingest.Runtime, error) {
		app := &ingest.Runtime{}
		app.Settings.ContentDBPath = tmp + "/content.db"
		return app, nil
	}
	openSQLiteStore = func(path string) (*contentstore.SQLiteStore, error) {
		store, err := contentstore.NewSQLiteStore(path)
		if err != nil {
			return nil, err
		}
		for _, rec := range []c.Record{
			{UnitID: "twitter:CF1", Source: "twitter", ExternalID: "CF1", RootExternalID: "CF1", Model: varixllm.Qwen36PlusModel, Output: c.Output{Summary: "一句话", Graph: c.ReasoningGraph{Nodes: []c.GraphNode{{ID: "n1", Kind: c.NodeFact, Text: "A", OccurredAt: time.Now().UTC()}, {ID: "n2", Kind: c.NodeConclusion, Text: "B", ValidFrom: time.Now().UTC(), ValidTo: time.Now().UTC().Add(24 * time.Hour)}}, Edges: []c.GraphEdge{{From: "n1", To: "n2", Kind: c.EdgePositive}}}, Details: c.HiddenDetails{Caveats: []string{"说明"}}}, CompiledAt: time.Now().UTC()},
			{UnitID: "twitter:CF2", Source: "twitter", ExternalID: "CF2", RootExternalID: "CF2", Model: varixllm.Qwen36PlusModel, Output: c.Output{Summary: "一句话", Graph: c.ReasoningGraph{Nodes: []c.GraphNode{{ID: "n1", Kind: c.NodeFact, Text: "X", OccurredAt: time.Now().UTC()}, {ID: "n2", Kind: c.NodeConclusion, Text: "Y", ValidFrom: time.Now().UTC(), ValidTo: time.Now().UTC().Add(24 * time.Hour)}}, Edges: []c.GraphEdge{{From: "n1", To: "n2", Kind: c.EdgePositive}}}, Details: c.HiddenDetails{Caveats: []string{"说明"}}}, CompiledAt: time.Now().UTC()},
		} {
			if err := store.UpsertCompiledOutput(context.Background(), rec); err != nil {
				return nil, err
			}
			if err := store.PersistMemoryContentGraphFromCompiledOutput(context.Background(), "u-content-filter", rec.Source, rec.ExternalID, time.Now().UTC()); err != nil {
				return nil, err
			}
		}
		return store, nil
	}
	var stdout, stderr bytes.Buffer
	code := run([]string{"memory", "content-graphs", "--user", "u-content-filter", "--platform", "twitter", "--id", "CF2"}, "/tmp/project", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("content-graphs filtered code = %d, stderr = %s", code, stderr.String())
	}
	var out []model.ContentSubgraph
	if err := json.Unmarshal(stdout.Bytes(), &out); err != nil {
		t.Fatalf("json.Unmarshal(content-graphs filter) error = %v", err)
	}
	if len(out) != 1 || out[0].SourceExternalID != "CF2" {
		t.Fatalf("content-graphs filtered output = %#v, want CF2 only", out)
	}
}

func TestRunMemoryEventGraphsSupportsSubjectFilter(t *testing.T) {
	prevNewIngestRuntime := newIngestRuntime
	prevOpenSQLiteStore := openSQLiteStore
	t.Cleanup(func() {
		newIngestRuntime = prevNewIngestRuntime
		openSQLiteStore = prevOpenSQLiteStore
	})

	tmp := t.TempDir()
	newIngestRuntime = func(projectRoot string) (*ingest.Runtime, error) {
		app := &ingest.Runtime{}
		app.Settings.ContentDBPath = tmp + "/content.db"
		return app, nil
	}
	openSQLiteStore = func(path string) (*contentstore.SQLiteStore, error) {
		store, err := contentstore.NewSQLiteStore(path)
		if err != nil {
			return nil, err
		}
		sg := testDriverTargetSubgraph("subject-eg-cli", time.Now().UTC())
		if err := store.PersistMemoryContentGraph(context.Background(), "u-event-subject", sg, time.Now().UTC()); err != nil {
			return nil, err
		}
		return store, nil
	}
	var stdout, stderr bytes.Buffer
	code := run([]string{"memory", "event-graphs", "--user", "u-event-subject", "--subject", "美联储"}, "/tmp/project", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("event-graphs --subject 美联储 code = %d, stderr = %s", code, stderr.String())
	}
	var out []contentstore.EventGraphRecord
	if err := json.Unmarshal(stdout.Bytes(), &out); err != nil {
		t.Fatalf("json.Unmarshal(event-graphs --subject) error = %v", err)
	}
	if len(out) != 1 || out[0].AnchorSubject != "美联储" {
		t.Fatalf("event-graphs filtered output = %#v, want 美联储 graph", out)
	}
}

func TestRunMemoryContentGraphsSupportsSubjectFilter(t *testing.T) {
	prevNewIngestRuntime := newIngestRuntime
	prevOpenSQLiteStore := openSQLiteStore
	t.Cleanup(func() {
		newIngestRuntime = prevNewIngestRuntime
		openSQLiteStore = prevOpenSQLiteStore
	})

	tmp := t.TempDir()
	newIngestRuntime = func(projectRoot string) (*ingest.Runtime, error) {
		app := &ingest.Runtime{}
		app.Settings.ContentDBPath = tmp + "/content.db"
		return app, nil
	}
	openSQLiteStore = func(path string) (*contentstore.SQLiteStore, error) {
		store, err := contentstore.NewSQLiteStore(path)
		if err != nil {
			return nil, err
		}
		now := time.Now().UTC()
		for _, sg := range []model.ContentSubgraph{
			{ID: "subject-cg-cli-1", ArticleID: "subject-cg-cli-1", SourcePlatform: "twitter", SourceExternalID: "subject-cg-cli-1", CompileVersion: model.CompileBridgeVersion, CompiledAt: now.Format(time.RFC3339), UpdatedAt: now.Format(time.RFC3339), Nodes: []model.ContentNode{{ID: "n1", SourceArticleID: "subject-cg-cli-1", SourcePlatform: "twitter", SourceExternalID: "subject-cg-cli-1", RawText: "美联储加息0.25%", SubjectText: "美联储", ChangeText: "加息0.25%", Kind: model.NodeKindObservation, IsPrimary: true, VerificationStatus: model.VerificationPending}}},
			{ID: "subject-cg-cli-2", ArticleID: "subject-cg-cli-2", SourcePlatform: "twitter", SourceExternalID: "subject-cg-cli-2", CompileVersion: model.CompileBridgeVersion, CompiledAt: now.Format(time.RFC3339), UpdatedAt: now.Format(time.RFC3339), Nodes: []model.ContentNode{{ID: "n1", SourceArticleID: "subject-cg-cli-2", SourcePlatform: "twitter", SourceExternalID: "subject-cg-cli-2", RawText: "欧洲央行放缓缩表", SubjectText: "欧洲央行", ChangeText: "放缓缩表", Kind: model.NodeKindObservation, IsPrimary: true, VerificationStatus: model.VerificationPending}}},
		} {
			if err := store.PersistMemoryContentGraph(context.Background(), "u-content-subject", sg, now); err != nil {
				return nil, err
			}
		}
		return store, nil
	}
	var stdout, stderr bytes.Buffer
	code := run([]string{"memory", "content-graphs", "--user", "u-content-subject", "--subject", "美联储"}, "/tmp/project", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("content-graphs --subject 美联储 code = %d, stderr = %s", code, stderr.String())
	}
	var out []model.ContentSubgraph
	if err := json.Unmarshal(stdout.Bytes(), &out); err != nil {
		t.Fatalf("json.Unmarshal(content-graphs --subject) error = %v", err)
	}
	if len(out) != 1 || out[0].SourceExternalID != "subject-cg-cli-1" {
		t.Fatalf("content-graphs filtered output = %#v, want 美联储 snapshot only", out)
	}
}

func TestRunMemoryEventGraphsCombinesScopeAndSubjectFilters(t *testing.T) {
	prevNewIngestRuntime := newIngestRuntime
	prevOpenSQLiteStore := openSQLiteStore
	t.Cleanup(func() {
		newIngestRuntime = prevNewIngestRuntime
		openSQLiteStore = prevOpenSQLiteStore
	})

	tmp := t.TempDir()
	newIngestRuntime = func(projectRoot string) (*ingest.Runtime, error) {
		app := &ingest.Runtime{}
		app.Settings.ContentDBPath = tmp + "/content.db"
		return app, nil
	}
	openSQLiteStore = func(path string) (*contentstore.SQLiteStore, error) {
		store, err := contentstore.NewSQLiteStore(path)
		if err != nil {
			return nil, err
		}
		sg := testDriverTargetSubgraph("combo-eg", time.Now().UTC())
		if err := store.PersistMemoryContentGraph(context.Background(), "u-combo-eg", sg, time.Now().UTC()); err != nil {
			return nil, err
		}
		return store, nil
	}
	var stdout, stderr bytes.Buffer
	code := run([]string{"memory", "event-graphs", "--user", "u-combo-eg", "--scope", "driver", "--subject", "美股"}, "/tmp/project", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("event-graphs combined filter code = %d, stderr = %s", code, stderr.String())
	}
	var out []contentstore.EventGraphRecord
	if err := json.Unmarshal(stdout.Bytes(), &out); err != nil {
		t.Fatalf("json.Unmarshal(event-graphs combined) error = %v", err)
	}
	if len(out) != 0 {
		t.Fatalf("event-graphs combined output = %#v, want empty intersection", out)
	}
}

func TestRunMemoryContentGraphsCombinesSourceAndSubjectFilters(t *testing.T) {
	prevNewIngestRuntime := newIngestRuntime
	prevOpenSQLiteStore := openSQLiteStore
	t.Cleanup(func() {
		newIngestRuntime = prevNewIngestRuntime
		openSQLiteStore = prevOpenSQLiteStore
	})

	tmp := t.TempDir()
	newIngestRuntime = func(projectRoot string) (*ingest.Runtime, error) {
		app := &ingest.Runtime{}
		app.Settings.ContentDBPath = tmp + "/content.db"
		return app, nil
	}
	openSQLiteStore = func(path string) (*contentstore.SQLiteStore, error) {
		store, err := contentstore.NewSQLiteStore(path)
		if err != nil {
			return nil, err
		}
		now := time.Now().UTC()
		for _, sg := range []model.ContentSubgraph{
			{ID: "combo-cg-1", ArticleID: "combo-cg-1", SourcePlatform: "twitter", SourceExternalID: "combo-cg-1", CompileVersion: model.CompileBridgeVersion, CompiledAt: now.Format(time.RFC3339), UpdatedAt: now.Format(time.RFC3339), Nodes: []model.ContentNode{{ID: "n1", SourceArticleID: "combo-cg-1", SourcePlatform: "twitter", SourceExternalID: "combo-cg-1", RawText: "美联储加息0.25%", SubjectText: "美联储", ChangeText: "加息0.25%", Kind: model.NodeKindObservation, IsPrimary: true, VerificationStatus: model.VerificationPending}}},
			{ID: "combo-cg-2", ArticleID: "combo-cg-2", SourcePlatform: "twitter", SourceExternalID: "combo-cg-2", CompileVersion: model.CompileBridgeVersion, CompiledAt: now.Format(time.RFC3339), UpdatedAt: now.Format(time.RFC3339), Nodes: []model.ContentNode{{ID: "n1", SourceArticleID: "combo-cg-2", SourcePlatform: "twitter", SourceExternalID: "combo-cg-2", RawText: "欧洲央行放缓缩表", SubjectText: "欧洲央行", ChangeText: "放缓缩表", Kind: model.NodeKindObservation, IsPrimary: true, VerificationStatus: model.VerificationPending}}},
		} {
			if err := store.PersistMemoryContentGraph(context.Background(), "u-combo-cg", sg, now); err != nil {
				return nil, err
			}
		}
		return store, nil
	}
	var stdout, stderr bytes.Buffer
	code := run([]string{"memory", "content-graphs", "--user", "u-combo-cg", "--platform", "twitter", "--id", "combo-cg-2", "--subject", "美联储"}, "/tmp/project", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("content-graphs combined filter code = %d, stderr = %s", code, stderr.String())
	}
	var out []model.ContentSubgraph
	if err := json.Unmarshal(stdout.Bytes(), &out); err != nil {
		t.Fatalf("json.Unmarshal(content-graphs combined) error = %v", err)
	}
	if len(out) != 0 {
		t.Fatalf("content-graphs combined output = %#v, want empty intersection", out)
	}
}

func TestRunMemoryEventGraphsCardSupportsSubjectFilter(t *testing.T) {
	prevNewIngestRuntime := newIngestRuntime
	prevOpenSQLiteStore := openSQLiteStore
	t.Cleanup(func() {
		newIngestRuntime = prevNewIngestRuntime
		openSQLiteStore = prevOpenSQLiteStore
	})

	tmp := t.TempDir()
	newIngestRuntime = func(projectRoot string) (*ingest.Runtime, error) {
		app := &ingest.Runtime{}
		app.Settings.ContentDBPath = tmp + "/content.db"
		return app, nil
	}
	openSQLiteStore = func(path string) (*contentstore.SQLiteStore, error) {
		store, err := contentstore.NewSQLiteStore(path)
		if err != nil {
			return nil, err
		}
		sg := testDriverTargetSubgraph("card-subject-eg", time.Now().UTC())
		if err := store.PersistMemoryContentGraph(context.Background(), "u-event-card-subject", sg, time.Now().UTC()); err != nil {
			return nil, err
		}
		return store, nil
	}
	var stdout, stderr bytes.Buffer
	code := run([]string{"memory", "event-graphs", "--card", "--user", "u-event-card-subject", "--subject", "美联储"}, "/tmp/project", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("event-graphs --card --subject code = %d, stderr = %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "美联储") || strings.Contains(stdout.String(), "欧洲央行") {
		t.Fatalf("stdout = %q, want filtered event card output", stdout.String())
	}
}

func TestRunMemoryEventGraphsSupportsAliasSubjectFilter(t *testing.T) {
	prevNewIngestRuntime := newIngestRuntime
	prevOpenSQLiteStore := openSQLiteStore
	t.Cleanup(func() {
		newIngestRuntime = prevNewIngestRuntime
		openSQLiteStore = prevOpenSQLiteStore
	})

	tmp := t.TempDir()
	newIngestRuntime = func(projectRoot string) (*ingest.Runtime, error) {
		app := &ingest.Runtime{}
		app.Settings.ContentDBPath = tmp + "/content.db"
		return app, nil
	}
	openSQLiteStore = func(path string) (*contentstore.SQLiteStore, error) {
		store, err := contentstore.NewSQLiteStore(path)
		if err != nil {
			return nil, err
		}
		now := time.Now().UTC()
		if err := store.UpsertCanonicalEntity(context.Background(), memory.CanonicalEntity{
			EntityID:      "driver-fed",
			EntityType:    memory.CanonicalEntityDriver,
			CanonicalName: "美联储",
			Aliases:       []string{"联储"},
			Status:        memory.CanonicalEntityActive,
			CreatedAt:     now,
			UpdatedAt:     now,
		}); err != nil {
			return nil, err
		}
		sg := model.ContentSubgraph{ID: "alias-eg", ArticleID: "alias-eg", SourcePlatform: "twitter", SourceExternalID: "alias-eg", CompileVersion: model.CompileBridgeVersion, CompiledAt: now.Format(time.RFC3339), UpdatedAt: now.Format(time.RFC3339), Nodes: []model.ContentNode{{ID: "n1", SourceArticleID: "alias-eg", SourcePlatform: "twitter", SourceExternalID: "alias-eg", RawText: "联储加息0.25%", SubjectText: "联储", ChangeText: "加息0.25%", Kind: model.NodeKindObservation, GraphRole: model.GraphRoleDriver, IsPrimary: true, VerificationStatus: model.VerificationPending, TimeBucket: "1w"}}}
		if err := store.PersistMemoryContentGraph(context.Background(), "u-event-alias-filter", sg, now); err != nil {
			return nil, err
		}
		return store, nil
	}
	var stdout, stderr bytes.Buffer
	code := run([]string{"memory", "event-graphs", "--user", "u-event-alias-filter", "--subject", "联储"}, "/tmp/project", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("event-graphs alias subject code = %d, stderr = %s", code, stderr.String())
	}
	var out []contentstore.EventGraphRecord
	if err := json.Unmarshal(stdout.Bytes(), &out); err != nil {
		t.Fatalf("json.Unmarshal(event-graphs alias) error = %v", err)
	}
	if len(out) != 1 || out[0].AnchorSubject != "美联储" {
		t.Fatalf("out = %#v, want alias lookup to return canonical 美联储 event graph", out)
	}
}

func TestRunMemoryContentGraphsCardSupportsSubjectFilter(t *testing.T) {
	prevNewIngestRuntime := newIngestRuntime
	prevOpenSQLiteStore := openSQLiteStore
	t.Cleanup(func() {
		newIngestRuntime = prevNewIngestRuntime
		openSQLiteStore = prevOpenSQLiteStore
	})

	tmp := t.TempDir()
	newIngestRuntime = func(projectRoot string) (*ingest.Runtime, error) {
		app := &ingest.Runtime{}
		app.Settings.ContentDBPath = tmp + "/content.db"
		return app, nil
	}
	openSQLiteStore = func(path string) (*contentstore.SQLiteStore, error) {
		store, err := contentstore.NewSQLiteStore(path)
		if err != nil {
			return nil, err
		}
		now := time.Now().UTC()
		for _, sg := range []model.ContentSubgraph{
			{ID: "card-subject-cg-1", ArticleID: "card-subject-cg-1", SourcePlatform: "twitter", SourceExternalID: "card-subject-cg-1", CompileVersion: model.CompileBridgeVersion, CompiledAt: now.Format(time.RFC3339), UpdatedAt: now.Format(time.RFC3339), Nodes: []model.ContentNode{{ID: "n1", SourceArticleID: "card-subject-cg-1", SourcePlatform: "twitter", SourceExternalID: "card-subject-cg-1", RawText: "美联储加息0.25%", SubjectText: "美联储", ChangeText: "加息0.25%", Kind: model.NodeKindObservation, IsPrimary: true, VerificationStatus: model.VerificationPending}}},
			{ID: "card-subject-cg-2", ArticleID: "card-subject-cg-2", SourcePlatform: "twitter", SourceExternalID: "card-subject-cg-2", CompileVersion: model.CompileBridgeVersion, CompiledAt: now.Format(time.RFC3339), UpdatedAt: now.Format(time.RFC3339), Nodes: []model.ContentNode{{ID: "n1", SourceArticleID: "card-subject-cg-2", SourcePlatform: "twitter", SourceExternalID: "card-subject-cg-2", RawText: "欧洲央行放缓缩表", SubjectText: "欧洲央行", ChangeText: "放缓缩表", Kind: model.NodeKindObservation, IsPrimary: true, VerificationStatus: model.VerificationPending}}},
		} {
			if err := store.PersistMemoryContentGraph(context.Background(), "u-content-card-subject", sg, now); err != nil {
				return nil, err
			}
		}
		return store, nil
	}
	var stdout, stderr bytes.Buffer
	code := run([]string{"memory", "content-graphs", "--card", "--user", "u-content-card-subject", "--subject", "美联储"}, "/tmp/project", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("content-graphs --card --subject code = %d, stderr = %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "美联储") || strings.Contains(stdout.String(), "欧洲央行") {
		t.Fatalf("stdout = %q, want filtered content graph card output", stdout.String())
	}
}

func TestRunMemoryContentGraphsSupportsAliasSubjectFilter(t *testing.T) {
	prevNewIngestRuntime := newIngestRuntime
	prevOpenSQLiteStore := openSQLiteStore
	t.Cleanup(func() {
		newIngestRuntime = prevNewIngestRuntime
		openSQLiteStore = prevOpenSQLiteStore
	})

	tmp := t.TempDir()
	newIngestRuntime = func(projectRoot string) (*ingest.Runtime, error) {
		app := &ingest.Runtime{}
		app.Settings.ContentDBPath = tmp + "/content.db"
		return app, nil
	}
	openSQLiteStore = func(path string) (*contentstore.SQLiteStore, error) {
		store, err := contentstore.NewSQLiteStore(path)
		if err != nil {
			return nil, err
		}
		now := time.Now().UTC()
		if err := store.UpsertCanonicalEntity(context.Background(), memory.CanonicalEntity{
			EntityID:      "driver-fed",
			EntityType:    memory.CanonicalEntityDriver,
			CanonicalName: "美联储",
			Aliases:       []string{"联储"},
			Status:        memory.CanonicalEntityActive,
			CreatedAt:     now,
			UpdatedAt:     now,
		}); err != nil {
			return nil, err
		}
		sg := model.ContentSubgraph{ID: "alias-cg", ArticleID: "alias-cg", SourcePlatform: "twitter", SourceExternalID: "alias-cg", CompileVersion: model.CompileBridgeVersion, CompiledAt: now.Format(time.RFC3339), UpdatedAt: now.Format(time.RFC3339), Nodes: []model.ContentNode{{ID: "n1", SourceArticleID: "alias-cg", SourcePlatform: "twitter", SourceExternalID: "alias-cg", RawText: "联储加息0.25%", SubjectText: "联储", ChangeText: "加息0.25%", SubjectCanonical: "美联储", Kind: model.NodeKindObservation, IsPrimary: true, VerificationStatus: model.VerificationPending}}}
		if err := store.PersistMemoryContentGraph(context.Background(), "u-content-alias-filter", sg, now); err != nil {
			return nil, err
		}
		return store, nil
	}
	var stdout, stderr bytes.Buffer
	code := run([]string{"memory", "content-graphs", "--user", "u-content-alias-filter", "--subject", "联储"}, "/tmp/project", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("content-graphs alias subject code = %d, stderr = %s", code, stderr.String())
	}
	var out []model.ContentSubgraph
	if err := json.Unmarshal(stdout.Bytes(), &out); err != nil {
		t.Fatalf("json.Unmarshal(content-graphs alias) error = %v", err)
	}
	if len(out) != 1 {
		t.Fatalf("out = %#v, want one content graph for alias lookup", out)
	}
}

func TestRunMemoryContentGraphsResolvesAliasToCanonicalSubjectFilter(t *testing.T) {
	prevNewIngestRuntime := newIngestRuntime
	prevOpenSQLiteStore := openSQLiteStore
	t.Cleanup(func() {
		newIngestRuntime = prevNewIngestRuntime
		openSQLiteStore = prevOpenSQLiteStore
	})

	tmp := t.TempDir()
	newIngestRuntime = func(projectRoot string) (*ingest.Runtime, error) {
		app := &ingest.Runtime{}
		app.Settings.ContentDBPath = tmp + "/content.db"
		return app, nil
	}
	openSQLiteStore = func(path string) (*contentstore.SQLiteStore, error) {
		store, err := contentstore.NewSQLiteStore(path)
		if err != nil {
			return nil, err
		}
		now := time.Now().UTC()
		if err := store.UpsertCanonicalEntity(context.Background(), memory.CanonicalEntity{
			EntityID:      "driver-fed",
			EntityType:    memory.CanonicalEntityDriver,
			CanonicalName: "美联储",
			Aliases:       []string{"联储"},
			Status:        memory.CanonicalEntityActive,
			CreatedAt:     now,
			UpdatedAt:     now,
		}); err != nil {
			return nil, err
		}
		sg := model.ContentSubgraph{ID: "canonical-cg", ArticleID: "canonical-cg", SourcePlatform: "twitter", SourceExternalID: "canonical-cg", CompileVersion: model.CompileBridgeVersion, CompiledAt: now.Format(time.RFC3339), UpdatedAt: now.Format(time.RFC3339), Nodes: []model.ContentNode{{ID: "n1", SourceArticleID: "canonical-cg", SourcePlatform: "twitter", SourceExternalID: "canonical-cg", RawText: "美联储加息0.25%", SubjectText: "美联储", ChangeText: "加息0.25%", Kind: model.NodeKindObservation, IsPrimary: true, VerificationStatus: model.VerificationPending}}}
		if err := store.PersistMemoryContentGraph(context.Background(), "u-content-canonical-filter", sg, now); err != nil {
			return nil, err
		}
		return store, nil
	}
	var stdout, stderr bytes.Buffer
	code := run([]string{"memory", "content-graphs", "--user", "u-content-canonical-filter", "--subject", "联储"}, "/tmp/project", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("content-graphs canonical alias filter code = %d, stderr = %s", code, stderr.String())
	}
	var out []model.ContentSubgraph
	if err := json.Unmarshal(stdout.Bytes(), &out); err != nil {
		t.Fatalf("json.Unmarshal(content-graphs canonical alias) error = %v", err)
	}
	if len(out) != 1 {
		t.Fatalf("out = %#v, want one content graph for canonical alias lookup", out)
	}
	if out[0].Nodes[0].SubjectText != "美联储" {
		t.Fatalf("out = %#v, want canonical subject presentation in returned payload", out)
	}
}

func TestRunMemoryContentGraphsSupportsSourceAndAliasSubjectFilter(t *testing.T) {
	prevNewIngestRuntime := newIngestRuntime
	prevOpenSQLiteStore := openSQLiteStore
	t.Cleanup(func() {
		newIngestRuntime = prevNewIngestRuntime
		openSQLiteStore = prevOpenSQLiteStore
	})

	tmp := t.TempDir()
	newIngestRuntime = func(projectRoot string) (*ingest.Runtime, error) {
		app := &ingest.Runtime{}
		app.Settings.ContentDBPath = tmp + "/content.db"
		return app, nil
	}
	openSQLiteStore = func(path string) (*contentstore.SQLiteStore, error) {
		store, err := contentstore.NewSQLiteStore(path)
		if err != nil {
			return nil, err
		}
		now := time.Now().UTC()
		if err := store.UpsertCanonicalEntity(context.Background(), memory.CanonicalEntity{
			EntityID:      "driver-fed",
			EntityType:    memory.CanonicalEntityDriver,
			CanonicalName: "美联储",
			Aliases:       []string{"联储"},
			Status:        memory.CanonicalEntityActive,
			CreatedAt:     now,
			UpdatedAt:     now,
		}); err != nil {
			return nil, err
		}
		for _, sg := range []model.ContentSubgraph{
			{ID: "source-alias-cg-1", ArticleID: "source-alias-cg-1", SourcePlatform: "twitter", SourceExternalID: "source-alias-cg-1", CompileVersion: model.CompileBridgeVersion, CompiledAt: now.Format(time.RFC3339), UpdatedAt: now.Format(time.RFC3339), Nodes: []model.ContentNode{{ID: "n1", SourceArticleID: "source-alias-cg-1", SourcePlatform: "twitter", SourceExternalID: "source-alias-cg-1", RawText: "美联储加息0.25%", SubjectText: "美联储", ChangeText: "加息0.25%", Kind: model.NodeKindObservation, IsPrimary: true, VerificationStatus: model.VerificationPending}}},
			{ID: "source-alias-cg-2", ArticleID: "source-alias-cg-2", SourcePlatform: "twitter", SourceExternalID: "source-alias-cg-2", CompileVersion: model.CompileBridgeVersion, CompiledAt: now.Format(time.RFC3339), UpdatedAt: now.Format(time.RFC3339), Nodes: []model.ContentNode{{ID: "n1", SourceArticleID: "source-alias-cg-2", SourcePlatform: "twitter", SourceExternalID: "source-alias-cg-2", RawText: "欧洲央行放缓缩表", SubjectText: "欧洲央行", ChangeText: "放缓缩表", Kind: model.NodeKindObservation, IsPrimary: true, VerificationStatus: model.VerificationPending}}},
		} {
			if err := store.PersistMemoryContentGraph(context.Background(), "u-content-source-alias-filter", sg, now); err != nil {
				return nil, err
			}
		}
		return store, nil
	}
	var stdout, stderr bytes.Buffer
	code := run([]string{"memory", "content-graphs", "--user", "u-content-source-alias-filter", "--platform", "twitter", "--id", "source-alias-cg-1", "--subject", "联储"}, "/tmp/project", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("content-graphs source+alias subject code = %d, stderr = %s", code, stderr.String())
	}
	var out []model.ContentSubgraph
	if err := json.Unmarshal(stdout.Bytes(), &out); err != nil {
		t.Fatalf("json.Unmarshal(content-graphs source+alias) error = %v", err)
	}
	if len(out) != 1 || out[0].SourceExternalID != "source-alias-cg-1" {
		t.Fatalf("out = %#v, want one source-filtered alias match", out)
	}
}

func TestRunMemoryParadigmsCardSupportsSubjectFilter(t *testing.T) {
	prevNewIngestRuntime := newIngestRuntime
	prevOpenSQLiteStore := openSQLiteStore
	t.Cleanup(func() {
		newIngestRuntime = prevNewIngestRuntime
		openSQLiteStore = prevOpenSQLiteStore
	})

	tmp := t.TempDir()
	newIngestRuntime = func(projectRoot string) (*ingest.Runtime, error) {
		app := &ingest.Runtime{}
		app.Settings.ContentDBPath = tmp + "/content.db"
		return app, nil
	}
	openSQLiteStore = func(path string) (*contentstore.SQLiteStore, error) {
		store, err := contentstore.NewSQLiteStore(path)
		if err != nil {
			return nil, err
		}
		now := time.Now().UTC()
		for _, sg := range []model.ContentSubgraph{
			testDriverTargetSubgraph("card-subject-pg-1", now),
			{ID: "card-subject-pg-2", ArticleID: "card-subject-pg-2", SourcePlatform: "twitter", SourceExternalID: "card-subject-pg-2", CompileVersion: model.CompileBridgeVersion, CompiledAt: now.Format(time.RFC3339), UpdatedAt: now.Format(time.RFC3339), Nodes: []model.ContentNode{{ID: "n1", SourceArticleID: "card-subject-pg-2", SourcePlatform: "twitter", SourceExternalID: "card-subject-pg-2", RawText: "欧洲央行放缓缩表", SubjectText: "欧洲央行", ChangeText: "放缓缩表", Kind: model.NodeKindObservation, GraphRole: model.GraphRoleDriver, IsPrimary: true, VerificationStatus: model.VerificationPending, TimeBucket: "1w"}, {ID: "n2", SourceArticleID: "card-subject-pg-2", SourcePlatform: "twitter", SourceExternalID: "card-subject-pg-2", RawText: "欧股承压", SubjectText: "欧股", ChangeText: "承压", Kind: model.NodeKindPrediction, GraphRole: model.GraphRoleTarget, IsPrimary: true, VerificationStatus: model.VerificationProved, TimeBucket: "1w"}}},
		} {
			if err := store.PersistMemoryContentGraph(context.Background(), "u-paradigm-card-subject", sg, now); err != nil {
				return nil, err
			}
		}
		return store, nil
	}
	var stdout, stderr bytes.Buffer
	code := run([]string{"memory", "paradigms", "--card", "--user", "u-paradigm-card-subject", "--subject", "美联储"}, "/tmp/project", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("paradigms --card --subject code = %d, stderr = %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "美联储") || strings.Contains(stdout.String(), "欧洲央行") {
		t.Fatalf("stdout = %q, want filtered paradigm card output", stdout.String())
	}
}

func TestRunMemoryParadigmsSupportsAliasSubjectFilter(t *testing.T) {
	prevNewIngestRuntime := newIngestRuntime
	prevOpenSQLiteStore := openSQLiteStore
	t.Cleanup(func() {
		newIngestRuntime = prevNewIngestRuntime
		openSQLiteStore = prevOpenSQLiteStore
	})

	tmp := t.TempDir()
	newIngestRuntime = func(projectRoot string) (*ingest.Runtime, error) {
		app := &ingest.Runtime{}
		app.Settings.ContentDBPath = tmp + "/content.db"
		return app, nil
	}
	openSQLiteStore = func(path string) (*contentstore.SQLiteStore, error) {
		store, err := contentstore.NewSQLiteStore(path)
		if err != nil {
			return nil, err
		}
		now := time.Now().UTC()
		if err := store.UpsertCanonicalEntity(context.Background(), memory.CanonicalEntity{
			EntityID:      "driver-fed",
			EntityType:    memory.CanonicalEntityDriver,
			CanonicalName: "美联储",
			Aliases:       []string{"联储"},
			Status:        memory.CanonicalEntityActive,
			CreatedAt:     now,
			UpdatedAt:     now,
		}); err != nil {
			return nil, err
		}
		sg := model.ContentSubgraph{ID: "alias-pg", ArticleID: "alias-pg", SourcePlatform: "twitter", SourceExternalID: "alias-pg", CompileVersion: model.CompileBridgeVersion, CompiledAt: now.Format(time.RFC3339), UpdatedAt: now.Format(time.RFC3339), Nodes: []model.ContentNode{{ID: "n1", SourceArticleID: "alias-pg", SourcePlatform: "twitter", SourceExternalID: "alias-pg", RawText: "联储加息0.25%", SubjectText: "联储", ChangeText: "加息0.25%", Kind: model.NodeKindObservation, GraphRole: model.GraphRoleDriver, IsPrimary: true, VerificationStatus: model.VerificationPending, TimeBucket: "1w"}, {ID: "n2", SourceArticleID: "alias-pg", SourcePlatform: "twitter", SourceExternalID: "alias-pg", RawText: "未来一周美股承压", SubjectText: "美股", ChangeText: "承压", Kind: model.NodeKindPrediction, GraphRole: model.GraphRoleTarget, IsPrimary: true, VerificationStatus: model.VerificationProved, TimeBucket: "1w"}}}
		if err := store.PersistMemoryContentGraph(context.Background(), "u-paradigm-alias-filter", sg, now); err != nil {
			return nil, err
		}
		return store, nil
	}
	var stdout, stderr bytes.Buffer
	code := run([]string{"memory", "paradigms", "--user", "u-paradigm-alias-filter", "--subject", "联储"}, "/tmp/project", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("paradigms alias subject code = %d, stderr = %s", code, stderr.String())
	}
	var out []contentstore.ParadigmRecord
	if err := json.Unmarshal(stdout.Bytes(), &out); err != nil {
		t.Fatalf("json.Unmarshal(paradigms alias) error = %v", err)
	}
	if len(out) != 1 || out[0].DriverSubject != "美联储" {
		t.Fatalf("out = %#v, want alias lookup to return canonical 美联储 paradigm", out)
	}
}

func TestRunMemoryEventGraphsCardSupportsScopeFilter(t *testing.T) {
	prevNewIngestRuntime := newIngestRuntime
	prevOpenSQLiteStore := openSQLiteStore
	t.Cleanup(func() {
		newIngestRuntime = prevNewIngestRuntime
		openSQLiteStore = prevOpenSQLiteStore
	})

	tmp := t.TempDir()
	newIngestRuntime = func(projectRoot string) (*ingest.Runtime, error) {
		app := &ingest.Runtime{}
		app.Settings.ContentDBPath = tmp + "/content.db"
		return app, nil
	}
	openSQLiteStore = func(path string) (*contentstore.SQLiteStore, error) {
		store, err := contentstore.NewSQLiteStore(path)
		if err != nil {
			return nil, err
		}
		sg := testDriverTargetSubgraph("card-scope-eg", time.Now().UTC())
		if err := store.PersistMemoryContentGraph(context.Background(), "u-event-card-scope", sg, time.Now().UTC()); err != nil {
			return nil, err
		}
		return store, nil
	}
	var stdout, stderr bytes.Buffer
	code := run([]string{"memory", "event-graphs", "--card", "--user", "u-event-card-scope", "--scope", "driver"}, "/tmp/project", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("event-graphs --card --scope code = %d, stderr = %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "Scope: driver") || strings.Contains(stdout.String(), "Scope: target") {
		t.Fatalf("stdout = %q, want only driver card output", stdout.String())
	}
}

func TestRunMemoryEventGraphsCardShowsNoMatchMessageForEmptyFilter(t *testing.T) {
	prevNewIngestRuntime := newIngestRuntime
	prevOpenSQLiteStore := openSQLiteStore
	t.Cleanup(func() {
		newIngestRuntime = prevNewIngestRuntime
		openSQLiteStore = prevOpenSQLiteStore
	})

	tmp := t.TempDir()
	newIngestRuntime = func(projectRoot string) (*ingest.Runtime, error) {
		app := &ingest.Runtime{}
		app.Settings.ContentDBPath = tmp + "/content.db"
		return app, nil
	}
	openSQLiteStore = func(path string) (*contentstore.SQLiteStore, error) {
		store, err := contentstore.NewSQLiteStore(path)
		if err != nil {
			return nil, err
		}
		sg := testDriverTargetSubgraph("empty-eg", time.Now().UTC())
		if err := store.PersistMemoryContentGraph(context.Background(), "u-empty-eg", sg, time.Now().UTC()); err != nil {
			return nil, err
		}
		return store, nil
	}
	var stdout, stderr bytes.Buffer
	code := run([]string{"memory", "event-graphs", "--card", "--user", "u-empty-eg", "--subject", "不存在"}, "/tmp/project", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("event-graphs empty card code = %d, stderr = %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "No event graphs matched") {
		t.Fatalf("stdout = %q, want no-match message", stdout.String())
	}
}

func TestRunMemoryParadigmsCardShowsNoMatchMessageForEmptyFilter(t *testing.T) {
	prevNewIngestRuntime := newIngestRuntime
	prevOpenSQLiteStore := openSQLiteStore
	t.Cleanup(func() {
		newIngestRuntime = prevNewIngestRuntime
		openSQLiteStore = prevOpenSQLiteStore
	})

	tmp := t.TempDir()
	newIngestRuntime = func(projectRoot string) (*ingest.Runtime, error) {
		app := &ingest.Runtime{}
		app.Settings.ContentDBPath = tmp + "/content.db"
		return app, nil
	}
	openSQLiteStore = func(path string) (*contentstore.SQLiteStore, error) {
		store, err := contentstore.NewSQLiteStore(path)
		if err != nil {
			return nil, err
		}
		sg := testDriverTargetSubgraph("empty-pg", time.Now().UTC())
		if err := store.PersistMemoryContentGraph(context.Background(), "u-empty-pg", sg, time.Now().UTC()); err != nil {
			return nil, err
		}
		return store, nil
	}
	var stdout, stderr bytes.Buffer
	code := run([]string{"memory", "paradigms", "--card", "--user", "u-empty-pg", "--subject", "不存在"}, "/tmp/project", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("paradigms empty card code = %d, stderr = %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "No paradigms matched") {
		t.Fatalf("stdout = %q, want no-match message", stdout.String())
	}
}

func TestRunMemoryContentGraphsCardShowsNoMatchMessageForEmptyFilter(t *testing.T) {
	prevNewIngestRuntime := newIngestRuntime
	prevOpenSQLiteStore := openSQLiteStore
	t.Cleanup(func() {
		newIngestRuntime = prevNewIngestRuntime
		openSQLiteStore = prevOpenSQLiteStore
	})

	tmp := t.TempDir()
	newIngestRuntime = func(projectRoot string) (*ingest.Runtime, error) {
		app := &ingest.Runtime{}
		app.Settings.ContentDBPath = tmp + "/content.db"
		return app, nil
	}
	openSQLiteStore = func(path string) (*contentstore.SQLiteStore, error) {
		store, err := contentstore.NewSQLiteStore(path)
		if err != nil {
			return nil, err
		}
		now := time.Now().UTC()
		sg := model.ContentSubgraph{ID: "empty-cg-card", ArticleID: "empty-cg-card", SourcePlatform: "twitter", SourceExternalID: "empty-cg-card", CompileVersion: model.CompileBridgeVersion, CompiledAt: now.Format(time.RFC3339), UpdatedAt: now.Format(time.RFC3339), Nodes: []model.ContentNode{{ID: "n1", SourceArticleID: "empty-cg-card", SourcePlatform: "twitter", SourceExternalID: "empty-cg-card", RawText: "美联储加息0.25%", SubjectText: "美联储", ChangeText: "加息0.25%", Kind: model.NodeKindObservation, IsPrimary: true, VerificationStatus: model.VerificationPending}}}
		if err := store.PersistMemoryContentGraph(context.Background(), "u-empty-cg-card", sg, now); err != nil {
			return nil, err
		}
		return store, nil
	}
	var stdout, stderr bytes.Buffer
	code := run([]string{"memory", "content-graphs", "--card", "--user", "u-empty-cg-card", "--subject", "不存在"}, "/tmp/project", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("content-graphs empty card code = %d, stderr = %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "No content graphs matched") {
		t.Fatalf("stdout = %q, want no-match message", stdout.String())
	}
}
