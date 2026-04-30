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

func TestRunMemoryAcceptRequiresFields(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{"memory", "accept"}, "/tmp/project", &stdout, &stderr)
	if code != 2 {
		t.Fatalf("run() code = %d, want 2", code)
	}
	if !strings.Contains(stderr.String(), "usage: varix memory accept") {
		t.Fatalf("stderr = %q, want usage", stderr.String())
	}
}

func TestRunMemoryCanonicalEntityUpsertRequiresFields(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{"memory", "canonical-entity-upsert"}, "/tmp/project", &stdout, &stderr)
	if code != 2 {
		t.Fatalf("run() code = %d, want 2", code)
	}
	if !strings.Contains(stderr.String(), "usage: varix memory canonical-entity-upsert") {
		t.Fatalf("stderr = %q, want usage", stderr.String())
	}
}

func TestRunMemoryAcceptPersistsNodeAndJob(t *testing.T) {
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
	code := run([]string{"memory", "accept", "--user", "u1", "--platform", "weibo", "--id", "Q1", "--node", "n1"}, "/tmp/project", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("run() code = %d, stderr = %s", code, stderr.String())
	}
	var got memory.AcceptResult
	if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
		t.Fatalf("json.Unmarshal(stdout) error = %v", err)
	}
	if len(got.Nodes) != 1 || got.Nodes[0].NodeID != "n1" {
		t.Fatalf("got = %#v", got)
	}
	if got.Job.JobID == 0 || got.Event.EventID == 0 {
		t.Fatalf("job/event = %#v / %#v", got.Job, got.Event)
	}
}

func TestRunMemoryAcceptBatchAndList(t *testing.T) {
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
	code := run([]string{"memory", "accept-batch", "--user", "u1", "--platform", "weibo", "--id", "Q1", "--nodes", "n1,n2"}, "/tmp/project", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("accept-batch code = %d, stderr = %s", code, stderr.String())
	}
	stdout.Reset()
	stderr.Reset()
	code = run([]string{"memory", "list", "--user", "u1"}, "/tmp/project", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("list code = %d, stderr = %s", code, stderr.String())
	}
	var got []memory.AcceptedNode
	if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
		t.Fatalf("json.Unmarshal(list) error = %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("len(list) = %d, want 2", len(got))
	}
}

func TestRunMemoryAcceptBatchAndListDerivesLegacyValidityFromNodeTiming(t *testing.T) {
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
		occurredAt := time.Date(2026, 4, 10, 9, 0, 0, 0, time.UTC)
		predictionStart := time.Date(2026, 4, 12, 0, 0, 0, 0, time.UTC)
		predictionDue := time.Date(2026, 4, 20, 0, 0, 0, 0, time.UTC)
		record := c.Record{
			UnitID:         "weibo:Q-time",
			Source:         "weibo",
			ExternalID:     "Q-time",
			RootExternalID: "Q-time",
			Model:          c.Qwen36PlusModel,
			Output: c.Output{
				Summary: "一句话",
				Graph: c.ReasoningGraph{
					Nodes: []c.GraphNode{
						{ID: "n1", Kind: c.NodeFact, Text: "事实A", OccurredAt: occurredAt},
						{ID: "n2", Kind: c.NodePrediction, Text: "预测B", PredictionStartAt: predictionStart, PredictionDueAt: predictionDue},
						{ID: "n3", Kind: c.NodeConclusion, Text: "结论C"},
					},
					Edges: []c.GraphEdge{{From: "n1", To: "n3", Kind: c.EdgeDerives}, {From: "n3", To: "n2", Kind: c.EdgeDerives}},
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
	code := run([]string{"memory", "accept-batch", "--user", "u1", "--platform", "weibo", "--id", "Q-time", "--nodes", "n1,n2"}, "/tmp/project", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("accept-batch code = %d, stderr = %s", code, stderr.String())
	}
	stdout.Reset()
	stderr.Reset()
	code = run([]string{"memory", "list", "--user", "u1"}, "/tmp/project", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("list code = %d, stderr = %s", code, stderr.String())
	}
	var got []memory.AcceptedNode
	if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
		t.Fatalf("json.Unmarshal(list) error = %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("len(list) = %d, want 2", len(got))
	}
	if !got[0].ValidFrom.Equal(time.Date(2026, 4, 10, 9, 0, 0, 0, time.UTC)) {
		t.Fatalf("fact ValidFrom = %s, want occurred_at-derived timestamp", got[0].ValidFrom)
	}
	if got[0].ValidTo.Year() != 9999 {
		t.Fatalf("fact ValidTo = %s, want open-ended year 9999", got[0].ValidTo)
	}
	if !got[1].ValidFrom.Equal(time.Date(2026, 4, 12, 0, 0, 0, 0, time.UTC)) || !got[1].ValidTo.Equal(time.Date(2026, 4, 20, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("prediction validity = %s..%s, want prediction_start_at/prediction_due_at-derived window", got[1].ValidFrom, got[1].ValidTo)
	}
}

func TestRunMemoryCanonicalEntityUpsertAndList(t *testing.T) {
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
	code := run([]string{"memory", "canonical-entity-upsert", "--id", "driver-fed", "--type", "driver", "--name", "美联储", "--aliases", "联储, Federal Reserve"}, "/tmp/project", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("canonical-entity-upsert code = %d, stderr = %s", code, stderr.String())
	}
	var upsertOut map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &upsertOut); err != nil {
		t.Fatalf("json.Unmarshal(canonical-entity-upsert) error = %v", err)
	}
	if ok, _ := upsertOut["ok"].(bool); !ok {
		t.Fatalf("upsert output = %#v, want ok=true", upsertOut)
	}

	stdout.Reset()
	stderr.Reset()
	code = run([]string{"memory", "canonical-entities"}, "/tmp/project", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("canonical-entities code = %d, stderr = %s", code, stderr.String())
	}
	var out []memory.CanonicalEntity
	if err := json.Unmarshal(stdout.Bytes(), &out); err != nil {
		t.Fatalf("json.Unmarshal(canonical-entities) error = %v", err)
	}
	if len(out) != 1 || out[0].CanonicalName != "美联储" {
		t.Fatalf("out = %#v, want one canonical entity named 美联储", out)
	}
	if len(out[0].Aliases) < 2 {
		t.Fatalf("aliases = %#v, want normalized aliases persisted", out[0].Aliases)
	}
}

func TestRunMemoryCanonicalEntityUpsertSupportsExplicitStatus(t *testing.T) {
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
	code := run([]string{"memory", "canonical-entity-upsert", "--id", "driver-fed", "--type", "driver", "--name", "美联储", "--status", "retired"}, "/tmp/project", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("canonical-entity-upsert --status code = %d, stderr = %s", code, stderr.String())
	}
	stdout.Reset()
	stderr.Reset()
	code = run([]string{"memory", "canonical-entities"}, "/tmp/project", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("canonical-entities code = %d, stderr = %s", code, stderr.String())
	}
	var out []memory.CanonicalEntity
	if err := json.Unmarshal(stdout.Bytes(), &out); err != nil {
		t.Fatalf("json.Unmarshal(canonical-entities) error = %v", err)
	}
	if len(out) != 1 || out[0].Status != memory.CanonicalEntityRetired {
		t.Fatalf("out = %#v, want retired canonical entity", out)
	}
}

func TestRunMemoryCanonicalEntitiesSupportsAliasFilter(t *testing.T) {
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
		if err := store.UpsertCanonicalEntity(context.Background(), memory.CanonicalEntity{
			EntityID:      "driver-fed",
			EntityType:    memory.CanonicalEntityDriver,
			CanonicalName: "美联储",
			Aliases:       []string{"联储"},
			Status:        memory.CanonicalEntityActive,
		}); err != nil {
			return nil, err
		}
		if err := store.UpsertCanonicalEntity(context.Background(), memory.CanonicalEntity{
			EntityID:      "target-us-equity",
			EntityType:    memory.CanonicalEntityTarget,
			CanonicalName: "美股",
			Aliases:       []string{"美国股市"},
			Status:        memory.CanonicalEntityActive,
		}); err != nil {
			return nil, err
		}
		return store, nil
	}

	var stdout, stderr bytes.Buffer
	code := run([]string{"memory", "canonical-entities", "--alias", "联储"}, "/tmp/project", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("canonical-entities --alias code = %d, stderr = %s", code, stderr.String())
	}
	var out []memory.CanonicalEntity
	if err := json.Unmarshal(stdout.Bytes(), &out); err != nil {
		t.Fatalf("json.Unmarshal(canonical-entities --alias) error = %v", err)
	}
	if len(out) != 1 || out[0].CanonicalName != "美联储" {
		t.Fatalf("out = %#v, want only 美联储 under alias filter", out)
	}
}

func TestRunMemoryCanonicalEntitiesSupportsTypeAndStatusFilters(t *testing.T) {
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
		if err := store.UpsertCanonicalEntity(context.Background(), memory.CanonicalEntity{
			EntityID:      "driver-fed",
			EntityType:    memory.CanonicalEntityDriver,
			CanonicalName: "美联储",
			Status:        memory.CanonicalEntityActive,
		}); err != nil {
			return nil, err
		}
		if err := store.UpsertCanonicalEntity(context.Background(), memory.CanonicalEntity{
			EntityID:      "target-us-equity",
			EntityType:    memory.CanonicalEntityTarget,
			CanonicalName: "美股",
			Status:        memory.CanonicalEntityRetired,
		}); err != nil {
			return nil, err
		}
		return store, nil
	}

	var stdout, stderr bytes.Buffer
	code := run([]string{"memory", "canonical-entities", "--type", "target", "--status", "retired"}, "/tmp/project", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("canonical-entities filter code = %d, stderr = %s", code, stderr.String())
	}
	var out []memory.CanonicalEntity
	if err := json.Unmarshal(stdout.Bytes(), &out); err != nil {
		t.Fatalf("json.Unmarshal(canonical-entities filter) error = %v", err)
	}
	if len(out) != 1 || out[0].CanonicalName != "美股" {
		t.Fatalf("out = %#v, want only retired target canonical entity", out)
	}
}

func TestRunMemoryCanonicalEntitiesSupportsSummary(t *testing.T) {
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
		if err := store.UpsertCanonicalEntity(context.Background(), memory.CanonicalEntity{
			EntityID:      "driver-fed",
			EntityType:    memory.CanonicalEntityDriver,
			CanonicalName: "美联储",
			Aliases:       []string{"联储", "Federal Reserve"},
			Status:        memory.CanonicalEntityActive,
		}); err != nil {
			return nil, err
		}
		if err := store.UpsertCanonicalEntity(context.Background(), memory.CanonicalEntity{
			EntityID:      "target-us-equity",
			EntityType:    memory.CanonicalEntityTarget,
			CanonicalName: "美股",
			Aliases:       []string{"美国股市"},
			Status:        memory.CanonicalEntityRetired,
		}); err != nil {
			return nil, err
		}
		return store, nil
	}
	var stdout, stderr bytes.Buffer
	code := run([]string{"memory", "canonical-entities", "--summary"}, "/tmp/project", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("canonical-entities --summary code = %d, stderr = %s", code, stderr.String())
	}
	var out struct {
		TotalEntities int            `json:"total_entities"`
		TotalAliases  int            `json:"total_aliases"`
		ByType        map[string]int `json:"by_type"`
		ByStatus      map[string]int `json:"by_status"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &out); err != nil {
		t.Fatalf("json.Unmarshal(canonical-entities --summary) error = %v", err)
	}
	if out.TotalEntities != 2 || out.TotalAliases < 3 {
		t.Fatalf("summary = %#v, want 2 entities and at least 3 aliases", out)
	}
	if out.ByType["driver"] != 1 || out.ByType["target"] != 1 {
		t.Fatalf("summary by_type = %#v, want driver=1 target=1", out.ByType)
	}
	if out.ByStatus["active"] != 1 || out.ByStatus["retired"] != 1 {
		t.Fatalf("summary by_status = %#v, want active=1 retired=1", out.ByStatus)
	}
}

func TestRunMemoryCanonicalEntitiesSupportsIDFilter(t *testing.T) {
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
		if err := store.UpsertCanonicalEntity(context.Background(), memory.CanonicalEntity{
			EntityID:      "driver-fed",
			EntityType:    memory.CanonicalEntityDriver,
			CanonicalName: "美联储",
			Status:        memory.CanonicalEntityActive,
		}); err != nil {
			return nil, err
		}
		if err := store.UpsertCanonicalEntity(context.Background(), memory.CanonicalEntity{
			EntityID:      "target-us-equity",
			EntityType:    memory.CanonicalEntityTarget,
			CanonicalName: "美股",
			Status:        memory.CanonicalEntityActive,
		}); err != nil {
			return nil, err
		}
		return store, nil
	}
	var stdout, stderr bytes.Buffer
	code := run([]string{"memory", "canonical-entities", "--id", "driver-fed"}, "/tmp/project", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("canonical-entities --id code = %d, stderr = %s", code, stderr.String())
	}
	var out []memory.CanonicalEntity
	if err := json.Unmarshal(stdout.Bytes(), &out); err != nil {
		t.Fatalf("json.Unmarshal(canonical-entities --id) error = %v", err)
	}
	if len(out) != 1 || out[0].EntityID != "driver-fed" {
		t.Fatalf("out = %#v, want only driver-fed under id filter", out)
	}
}

func TestRunMemoryCanonicalEntitiesCardRendersReadableOutput(t *testing.T) {
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
		if err := store.UpsertCanonicalEntity(context.Background(), memory.CanonicalEntity{
			EntityID:      "driver-fed",
			EntityType:    memory.CanonicalEntityDriver,
			CanonicalName: "美联储",
			Aliases:       []string{"联储", "Federal Reserve"},
			Status:        memory.CanonicalEntityActive,
		}); err != nil {
			return nil, err
		}
		return store, nil
	}
	var stdout, stderr bytes.Buffer
	code := run([]string{"memory", "canonical-entities", "--card"}, "/tmp/project", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("canonical-entities --card code = %d, stderr = %s", code, stderr.String())
	}
	for _, want := range []string{"Canonical Entity", "driver-fed", "美联储", "driver", "active", "联储"} {
		if !strings.Contains(stdout.String(), want) {
			t.Fatalf("stdout = %q, want substring %q", stdout.String(), want)
		}
	}
}
