package main

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/kumaloha/VariX/varix/ingest"
	"github.com/kumaloha/VariX/varix/model"
	"github.com/kumaloha/VariX/varix/storage/contentstore"
	"strings"
	"testing"
	"time"
)

func TestRunMemoryEventEvidencePrintsPersistedLinks(t *testing.T) {
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
		sg := testDriverTargetSubgraph("ev-cli", time.Now().UTC())
		if err := store.PersistMemoryContentGraph(context.Background(), "u-ev-cli", sg, time.Now().UTC()); err != nil {
			return nil, err
		}
		graphs, err := store.ListEventGraphs(context.Background(), "u-ev-cli")
		if err != nil {
			return nil, err
		}
		if len(graphs) == 0 {
			return nil, nil
		}
		return store, nil
	}
	var stdout, stderr bytes.Buffer
	code := run([]string{"memory", "event-evidence", "--user", "u-ev-cli"}, "/tmp/project", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("event-evidence code = %d, stderr = %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "event_graph_id") {
		t.Fatalf("stdout = %q, want event evidence payload", stdout.String())
	}
}

func TestRunMemoryParadigmEvidencePrintsPersistedLinks(t *testing.T) {
	prevNewIngestRuntime := newIngestRuntime
	prevOpenSQLiteStore := openSQLiteStore
	t.Cleanup(func() { newIngestRuntime = prevNewIngestRuntime; openSQLiteStore = prevOpenSQLiteStore })
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
		sg := testDriverTargetSubgraph("pev-cli", time.Now().UTC())
		if err := store.PersistMemoryContentGraph(context.Background(), "u-pev-cli", sg, time.Now().UTC()); err != nil {
			return nil, err
		}
		return store, nil
	}
	var stdout, stderr bytes.Buffer
	code := run([]string{"memory", "paradigm-evidence", "--user", "u-pev-cli"}, "/tmp/project", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("paradigm-evidence code = %d, stderr = %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "paradigm_id") {
		t.Fatalf("stdout = %q, want paradigm evidence payload", stdout.String())
	}
}

func TestRunMemoryEventEvidenceSupportsEventGraphIDFilter(t *testing.T) {
	prevNewIngestRuntime := newIngestRuntime
	prevOpenSQLiteStore := openSQLiteStore
	t.Cleanup(func() { newIngestRuntime = prevNewIngestRuntime; openSQLiteStore = prevOpenSQLiteStore })
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
			testDriverTargetSubgraph("ee-filter-1", now),
			{ID: "ee-filter-2", ArticleID: "ee-filter-2", SourcePlatform: "twitter", SourceExternalID: "ee-filter-2", CompileVersion: model.CompileBridgeVersion, CompiledAt: now.Format(time.RFC3339), UpdatedAt: now.Format(time.RFC3339), Nodes: []model.ContentNode{{ID: "n1", SourceArticleID: "ee-filter-2", SourcePlatform: "twitter", SourceExternalID: "ee-filter-2", RawText: "欧洲央行放缓缩表", SubjectText: "欧洲央行", ChangeText: "放缓缩表", Kind: model.NodeKindObservation, GraphRole: model.GraphRoleDriver, IsPrimary: true, VerificationStatus: model.VerificationPending, TimeBucket: "1w"}, {ID: "n2", SourceArticleID: "ee-filter-2", SourcePlatform: "twitter", SourceExternalID: "ee-filter-2", RawText: "欧股承压", SubjectText: "欧股", ChangeText: "承压", Kind: model.NodeKindPrediction, GraphRole: model.GraphRoleTarget, IsPrimary: true, VerificationStatus: model.VerificationProved, TimeBucket: "1w"}}},
		} {
			if err := store.PersistMemoryContentGraph(context.Background(), "u-ee-filter", sg, now); err != nil {
				return nil, err
			}
		}
		return store, nil
	}
	var stdout, stderr bytes.Buffer
	code := run([]string{"memory", "event-evidence", "--user", "u-ee-filter", "--event-graph-id", "u-ee-filter:driver:美联储:1w"}, "/tmp/project", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("event-evidence filter code = %d, stderr = %s", code, stderr.String())
	}
	var out []contentstore.EventGraphEvidenceLink
	if err := json.Unmarshal(stdout.Bytes(), &out); err != nil {
		t.Fatalf("json.Unmarshal(event-evidence filter) error = %v", err)
	}
	for _, item := range out {
		if item.EventGraphID != "u-ee-filter:driver:美联储:1w" {
			t.Fatalf("filtered links = %#v, want only target event graph id", out)
		}
	}
}

func TestRunMemoryParadigmEvidenceSupportsParadigmIDFilter(t *testing.T) {
	prevNewIngestRuntime := newIngestRuntime
	prevOpenSQLiteStore := openSQLiteStore
	t.Cleanup(func() { newIngestRuntime = prevNewIngestRuntime; openSQLiteStore = prevOpenSQLiteStore })
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
			testDriverTargetSubgraph("pe-filter-1", now),
			{ID: "pe-filter-2", ArticleID: "pe-filter-2", SourcePlatform: "twitter", SourceExternalID: "pe-filter-2", CompileVersion: model.CompileBridgeVersion, CompiledAt: now.Format(time.RFC3339), UpdatedAt: now.Format(time.RFC3339), Nodes: []model.ContentNode{{ID: "n1", SourceArticleID: "pe-filter-2", SourcePlatform: "twitter", SourceExternalID: "pe-filter-2", RawText: "欧洲央行放缓缩表", SubjectText: "欧洲央行", ChangeText: "放缓缩表", Kind: model.NodeKindObservation, GraphRole: model.GraphRoleDriver, IsPrimary: true, VerificationStatus: model.VerificationPending, TimeBucket: "1w"}, {ID: "n2", SourceArticleID: "pe-filter-2", SourcePlatform: "twitter", SourceExternalID: "pe-filter-2", RawText: "欧股承压", SubjectText: "欧股", ChangeText: "承压", Kind: model.NodeKindPrediction, GraphRole: model.GraphRoleTarget, IsPrimary: true, VerificationStatus: model.VerificationProved, TimeBucket: "1w"}}},
		} {
			if err := store.PersistMemoryContentGraph(context.Background(), "u-pe-filter", sg, now); err != nil {
				return nil, err
			}
		}
		return store, nil
	}
	var stdout, stderr bytes.Buffer
	code := run([]string{"memory", "paradigm-evidence", "--user", "u-pe-filter", "--paradigm-id", "u-pe-filter:美联储:美股:1w"}, "/tmp/project", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("paradigm-evidence filter code = %d, stderr = %s", code, stderr.String())
	}
	var out []contentstore.ParadigmEvidenceLink
	if err := json.Unmarshal(stdout.Bytes(), &out); err != nil {
		t.Fatalf("json.Unmarshal(paradigm-evidence filter) error = %v", err)
	}
	for _, item := range out {
		if item.ParadigmID != "u-pe-filter:美联储:美股:1w" {
			t.Fatalf("filtered links = %#v, want only target paradigm id", out)
		}
	}
}

func TestRunMemoryEventEvidenceShowsNoMatchMessageForUnknownID(t *testing.T) {
	prevNewIngestRuntime := newIngestRuntime
	prevOpenSQLiteStore := openSQLiteStore
	t.Cleanup(func() { newIngestRuntime = prevNewIngestRuntime; openSQLiteStore = prevOpenSQLiteStore })
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
		sg := testDriverTargetSubgraph("empty-ee", now)
		if err := store.PersistMemoryContentGraph(context.Background(), "u-empty-ee", sg, now); err != nil {
			return nil, err
		}
		return store, nil
	}
	var stdout, stderr bytes.Buffer
	code := run([]string{"memory", "event-evidence", "--user", "u-empty-ee", "--event-graph-id", "missing"}, "/tmp/project", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("event-evidence no-match code = %d, stderr = %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "No event evidence matched") {
		t.Fatalf("stdout = %q, want no-match message", stdout.String())
	}
}

func TestRunMemoryParadigmEvidenceShowsNoMatchMessageForUnknownID(t *testing.T) {
	prevNewIngestRuntime := newIngestRuntime
	prevOpenSQLiteStore := openSQLiteStore
	t.Cleanup(func() { newIngestRuntime = prevNewIngestRuntime; openSQLiteStore = prevOpenSQLiteStore })
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
		sg := testDriverTargetSubgraph("empty-pe", now)
		if err := store.PersistMemoryContentGraph(context.Background(), "u-empty-pe", sg, now); err != nil {
			return nil, err
		}
		return store, nil
	}
	var stdout, stderr bytes.Buffer
	code := run([]string{"memory", "paradigm-evidence", "--user", "u-empty-pe", "--paradigm-id", "missing"}, "/tmp/project", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("paradigm-evidence no-match code = %d, stderr = %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "No paradigm evidence matched") {
		t.Fatalf("stdout = %q, want no-match message", stdout.String())
	}
}

func TestRunMemoryEventEvidenceCardPrintsReadableSections(t *testing.T) {
	prevNewIngestRuntime := newIngestRuntime
	prevOpenSQLiteStore := openSQLiteStore
	t.Cleanup(func() { newIngestRuntime = prevNewIngestRuntime; openSQLiteStore = prevOpenSQLiteStore })
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
		sg := testDriverTargetSubgraph("event-evi-card", now)
		if err := store.PersistMemoryContentGraph(context.Background(), "u-event-evi-card", sg, now); err != nil {
			return nil, err
		}
		return store, nil
	}
	var stdout, stderr bytes.Buffer
	code := run([]string{"memory", "event-evidence", "--card", "--user", "u-event-evi-card"}, "/tmp/project", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("event-evidence --card code = %d, stderr = %s", code, stderr.String())
	}
	for _, want := range []string{"Event Evidence", "event_graph_id", "subgraph_id"} {
		if !strings.Contains(stdout.String(), want) {
			t.Fatalf("stdout = %q, want substring %q", stdout.String(), want)
		}
	}
}

func TestRunMemoryParadigmEvidenceCardPrintsReadableSections(t *testing.T) {
	prevNewIngestRuntime := newIngestRuntime
	prevOpenSQLiteStore := openSQLiteStore
	t.Cleanup(func() { newIngestRuntime = prevNewIngestRuntime; openSQLiteStore = prevOpenSQLiteStore })
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
		sg := testDriverTargetSubgraph("paradigm-evi-card", now)
		if err := store.PersistMemoryContentGraph(context.Background(), "u-paradigm-evi-card", sg, now); err != nil {
			return nil, err
		}
		return store, nil
	}
	var stdout, stderr bytes.Buffer
	code := run([]string{"memory", "paradigm-evidence", "--card", "--user", "u-paradigm-evi-card"}, "/tmp/project", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("paradigm-evidence --card code = %d, stderr = %s", code, stderr.String())
	}
	for _, want := range []string{"Paradigm Evidence", "paradigm_id", "subgraph_id"} {
		if !strings.Contains(stdout.String(), want) {
			t.Fatalf("stdout = %q, want substring %q", stdout.String(), want)
		}
	}
}
