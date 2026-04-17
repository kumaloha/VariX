package contentstore

import (
	"context"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/kumaloha/VariX/varix/memory"
)

func TestSQLiteStore_UpsertMechanismGraphRoundTripAndReplaceChildren(t *testing.T) {
	store := newRelationStoreTestSQLiteStore(t)
	ctx := context.Background()

	seedRelationStoreTestRelation(t, store, "rel-1")

	first := relationStoreTestMechanismGraph(
		"mech-1",
		"rel-1",
		time.Date(2026, 4, 10, 0, 0, 0, 0, time.UTC),
		[]memory.MechanismNode{
			{MechanismNodeID: "node-driver", MechanismID: "mech-1", NodeType: memory.MechanismNodeDriver, Label: "Liquidity shock", BackingAcceptedNodeIDs: []string{"n1"}, SortOrder: 0, CreatedAt: time.Date(2026, 4, 10, 0, 0, 0, 0, time.UTC)},
			{MechanismNodeID: "node-mid", MechanismID: "mech-1", NodeType: memory.MechanismNodeMarketBehavior, Label: "Risk appetite falls", BackingAcceptedNodeIDs: []string{"n2"}, SortOrder: 1, CreatedAt: time.Date(2026, 4, 10, 0, 1, 0, 0, time.UTC)},
			{MechanismNodeID: "node-target", MechanismID: "mech-1", NodeType: memory.MechanismNodeTargetEffect, Label: "Equities sell off", BackingAcceptedNodeIDs: []string{"n3"}, SortOrder: 2, CreatedAt: time.Date(2026, 4, 10, 0, 2, 0, 0, time.UTC)},
		},
		[]memory.MechanismEdge{
			{MechanismEdgeID: "edge-1", MechanismID: "mech-1", FromNodeID: "node-driver", ToNodeID: "node-mid", EdgeType: memory.MechanismEdgeCauses, CreatedAt: time.Date(2026, 4, 10, 0, 3, 0, 0, time.UTC)},
			{MechanismEdgeID: "edge-2", MechanismID: "mech-1", FromNodeID: "node-mid", ToNodeID: "node-target", EdgeType: memory.MechanismEdgeTransmits, CreatedAt: time.Date(2026, 4, 10, 0, 4, 0, 0, time.UTC)},
		},
		[]memory.PathOutcome{
			{PathOutcomeID: "path-1", MechanismID: "mech-1", NodePath: []string{"node-driver", "node-mid", "node-target"}, OutcomePolarity: memory.OutcomeBearish, OutcomeLabel: "near-term downside", ConditionScope: "if rates stay restrictive", Confidence: 0.61, CreatedAt: time.Date(2026, 4, 10, 0, 5, 0, 0, time.UTC)},
		},
	)
	if err := store.UpsertMechanismGraph(ctx, first); err != nil {
		t.Fatalf("UpsertMechanismGraph(first) error = %v", err)
	}

	updated := relationStoreTestMechanismGraph(
		"mech-1",
		"rel-1",
		time.Date(2026, 4, 10, 12, 0, 0, 0, time.UTC),
		[]memory.MechanismNode{
			{MechanismNodeID: "node-driver", MechanismID: "mech-1", NodeType: memory.MechanismNodeDriver, Label: "Liquidity shock", BackingAcceptedNodeIDs: []string{"n1"}, SortOrder: 0, CreatedAt: time.Date(2026, 4, 10, 12, 0, 0, 0, time.UTC)},
			{MechanismNodeID: "node-target", MechanismID: "mech-1", NodeType: memory.MechanismNodeTargetEffect, Label: "Equities sell off harder", BackingAcceptedNodeIDs: []string{"n3", "n4"}, SortOrder: 1, CreatedAt: time.Date(2026, 4, 10, 12, 1, 0, 0, time.UTC)},
		},
		[]memory.MechanismEdge{
			{MechanismEdgeID: "edge-9", MechanismID: "mech-1", FromNodeID: "node-driver", ToNodeID: "node-target", EdgeType: memory.MechanismEdgeAmplifies, CreatedAt: time.Date(2026, 4, 10, 12, 2, 0, 0, time.UTC)},
		},
		[]memory.PathOutcome{
			{PathOutcomeID: "path-9", MechanismID: "mech-1", NodePath: []string{"node-driver", "node-target"}, OutcomePolarity: memory.OutcomeBearish, OutcomeLabel: "faster downside", Confidence: 0.84, CreatedAt: time.Date(2026, 4, 10, 12, 3, 0, 0, time.UTC)},
		},
	)
	updated.Mechanism.Confidence = 0.84
	updated.Mechanism.SourceRefs = []string{"weibo:Q1", "twitter:Q2"}
	updated.Mechanism.TraceabilityStatus = memory.TraceabilityComplete

	if err := store.UpsertMechanismGraph(ctx, updated); err != nil {
		t.Fatalf("UpsertMechanismGraph(updated) error = %v", err)
	}

	got, err := store.GetMechanismGraph(ctx, "mech-1")
	if err != nil {
		t.Fatalf("GetMechanismGraph() error = %v", err)
	}
	if !reflect.DeepEqual(got, updated) {
		t.Fatalf("mechanism graph mismatch\n got=%#v\nwant=%#v", got, updated)
	}

	graphs, err := store.ListMechanismGraphsByRelation(ctx, "rel-1")
	if err != nil {
		t.Fatalf("ListMechanismGraphsByRelation() error = %v", err)
	}
	if len(graphs) != 1 {
		t.Fatalf("len(graphs) = %d, want 1", len(graphs))
	}
	if !reflect.DeepEqual(graphs[0], updated) {
		t.Fatalf("listed graph mismatch\n got=%#v\nwant=%#v", graphs[0], updated)
	}
}

func TestSQLiteStore_GetCurrentMechanismGraphUsesLatestMechanismAtAsOf(t *testing.T) {
	store := newRelationStoreTestSQLiteStore(t)
	ctx := context.Background()

	seedRelationStoreTestRelation(t, store, "rel-current")

	earlier := relationStoreTestMechanismGraph(
		"mech-early",
		"rel-current",
		time.Date(2026, 4, 10, 0, 0, 0, 0, time.UTC),
		[]memory.MechanismNode{{MechanismNodeID: "node-early", MechanismID: "mech-early", NodeType: memory.MechanismNodeDriver, Label: "Early driver", SortOrder: 0, CreatedAt: time.Date(2026, 4, 10, 0, 0, 0, 0, time.UTC)}},
		nil,
		[]memory.PathOutcome{{PathOutcomeID: "path-early", MechanismID: "mech-early", NodePath: []string{"node-early"}, OutcomePolarity: memory.OutcomeMixed, OutcomeLabel: "early view", Confidence: 0.4, CreatedAt: time.Date(2026, 4, 10, 0, 1, 0, 0, time.UTC)}},
	)
	later := relationStoreTestMechanismGraph(
		"mech-late",
		"rel-current",
		time.Date(2026, 4, 20, 0, 0, 0, 0, time.UTC),
		[]memory.MechanismNode{{MechanismNodeID: "node-late", MechanismID: "mech-late", NodeType: memory.MechanismNodeDriver, Label: "Late driver", SortOrder: 0, CreatedAt: time.Date(2026, 4, 20, 0, 0, 0, 0, time.UTC)}},
		nil,
		[]memory.PathOutcome{{PathOutcomeID: "path-late", MechanismID: "mech-late", NodePath: []string{"node-late"}, OutcomePolarity: memory.OutcomeBullish, OutcomeLabel: "late view", Confidence: 0.7, CreatedAt: time.Date(2026, 4, 20, 0, 1, 0, 0, time.UTC)}},
	)

	for _, graph := range []memory.MechanismGraph{earlier, later} {
		if err := store.UpsertMechanismGraph(ctx, graph); err != nil {
			t.Fatalf("UpsertMechanismGraph(%s) error = %v", graph.Mechanism.MechanismID, err)
		}
	}

	gotEarly, err := store.GetCurrentMechanismGraph(ctx, "rel-current", time.Date(2026, 4, 15, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("GetCurrentMechanismGraph(mid) error = %v", err)
	}
	if !reflect.DeepEqual(gotEarly, earlier) {
		t.Fatalf("current(mid) mismatch\n got=%#v\nwant=%#v", gotEarly, earlier)
	}

	gotLate, err := store.GetCurrentMechanismGraph(ctx, "rel-current", time.Date(2026, 4, 25, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("GetCurrentMechanismGraph(late) error = %v", err)
	}
	if !reflect.DeepEqual(gotLate, later) {
		t.Fatalf("current(late) mismatch\n got=%#v\nwant=%#v", gotLate, later)
	}
}

func TestSQLiteStore_RawCanonicalMappingsRoundTripByCanonicalObjectAndSource(t *testing.T) {
	store := newRelationStoreTestSQLiteStore(t)
	ctx := context.Background()

	mappingA := memory.RawCanonicalMapping{
		CanonicalObjectType: memory.CanonicalObjectTransmission,
		CanonicalObjectID:   "rel-1",
		SourcePlatform:      "weibo",
		SourceExternalID:    "Q1",
		RawNodeID:           "n-driver",
		MappingConfidence:   0.61,
		CreatedAt:           time.Date(2026, 4, 10, 0, 0, 0, 0, time.UTC),
		UpdatedAt:           time.Date(2026, 4, 10, 0, 0, 0, 0, time.UTC),
	}
	mappingB := memory.RawCanonicalMapping{
		CanonicalObjectType: memory.CanonicalObjectPathEdge,
		CanonicalObjectID:   "edge-1",
		SourcePlatform:      "weibo",
		SourceExternalID:    "Q1",
		RawNodeID:           "n-target",
		RawEdgeKey:          "n-driver->n-target",
		MappingConfidence:   0.88,
		CreatedAt:           time.Date(2026, 4, 10, 0, 1, 0, 0, time.UTC),
		UpdatedAt:           time.Date(2026, 4, 10, 0, 1, 0, 0, time.UTC),
	}
	mappingBUpdated := mappingB
	mappingBUpdated.MappingConfidence = 0.93
	mappingBUpdated.UpdatedAt = time.Date(2026, 4, 10, 0, 2, 0, 0, time.UTC)

	for _, mapping := range []memory.RawCanonicalMapping{mappingA, mappingB, mappingBUpdated} {
		if err := store.UpsertRawCanonicalMapping(ctx, mapping); err != nil {
			t.Fatalf("UpsertRawCanonicalMapping(%s/%s) error = %v", mapping.CanonicalObjectType, mapping.CanonicalObjectID, err)
		}
	}

	gotSource, err := store.ListRawCanonicalMappingsForSource(ctx, "weibo", "Q1")
	if err != nil {
		t.Fatalf("ListRawCanonicalMappingsForSource() error = %v", err)
	}
	wantSource := []memory.RawCanonicalMapping{mappingBUpdated, mappingA}
	if !reflect.DeepEqual(gotSource, wantSource) {
		t.Fatalf("source mappings mismatch\n got=%#v\nwant=%#v", gotSource, wantSource)
	}

	gotObject, err := store.ListRawCanonicalMappingsForCanonicalObject(ctx, memory.CanonicalObjectPathEdge, "edge-1")
	if err != nil {
		t.Fatalf("ListRawCanonicalMappingsForCanonicalObject() error = %v", err)
	}
	if !reflect.DeepEqual(gotObject, []memory.RawCanonicalMapping{mappingBUpdated}) {
		t.Fatalf("canonical-object mappings mismatch\n got=%#v\nwant=%#v", gotObject, []memory.RawCanonicalMapping{mappingBUpdated})
	}
}

func newRelationStoreTestSQLiteStore(t *testing.T) *SQLiteStore {
	t.Helper()
	store, err := NewSQLiteStore(filepath.Join(t.TempDir(), "data", "content.db"))
	if err != nil {
		t.Fatalf("NewSQLiteStore() error = %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return store
}

func seedRelationStoreTestRelation(t *testing.T, store *SQLiteStore, relationID string) {
	t.Helper()
	ctx := context.Background()
	if err := store.UpsertCanonicalEntity(ctx, memory.CanonicalEntity{
		EntityID:      relationID + "-driver",
		EntityType:    memory.CanonicalEntityDriver,
		CanonicalName: "Fed liquidity",
		Status:        memory.CanonicalEntityActive,
		CreatedAt:     time.Date(2026, 4, 10, 0, 0, 0, 0, time.UTC),
		UpdatedAt:     time.Date(2026, 4, 10, 0, 0, 0, 0, time.UTC),
	}); err != nil {
		t.Fatalf("UpsertCanonicalEntity(driver) error = %v", err)
	}
	if err := store.UpsertCanonicalEntity(ctx, memory.CanonicalEntity{
		EntityID:      relationID + "-target",
		EntityType:    memory.CanonicalEntityTarget,
		CanonicalName: "US equities",
		Status:        memory.CanonicalEntityActive,
		CreatedAt:     time.Date(2026, 4, 10, 0, 0, 0, 0, time.UTC),
		UpdatedAt:     time.Date(2026, 4, 10, 0, 0, 0, 0, time.UTC),
	}); err != nil {
		t.Fatalf("UpsertCanonicalEntity(target) error = %v", err)
	}
	if err := store.UpsertRelation(ctx, memory.Relation{
		RelationID:     relationID,
		DriverEntityID: relationID + "-driver",
		TargetEntityID: relationID + "-target",
		Status:         memory.RelationActive,
		CreatedAt:      time.Date(2026, 4, 10, 0, 0, 0, 0, time.UTC),
		UpdatedAt:      time.Date(2026, 4, 10, 0, 0, 0, 0, time.UTC),
	}); err != nil {
		t.Fatalf("UpsertRelation() error = %v", err)
	}
}

func relationStoreTestMechanismGraph(mechanismID, relationID string, asOf time.Time, nodes []memory.MechanismNode, edges []memory.MechanismEdge, outcomes []memory.PathOutcome) memory.MechanismGraph {
	if nodes == nil {
		nodes = []memory.MechanismNode{}
	}
	for i := range nodes {
		if nodes[i].BackingAcceptedNodeIDs == nil {
			nodes[i].BackingAcceptedNodeIDs = []string{}
		}
	}
	if edges == nil {
		edges = []memory.MechanismEdge{}
	}
	if outcomes == nil {
		outcomes = []memory.PathOutcome{}
	}
	return memory.MechanismGraph{
		Mechanism: memory.Mechanism{
			MechanismID:        mechanismID,
			RelationID:         relationID,
			AsOf:               asOf,
			ValidFrom:          asOf,
			Confidence:         0.61,
			Status:             memory.MechanismActive,
			SourceRefs:         []string{"weibo:Q1"},
			TraceabilityStatus: memory.TraceabilityPartial,
			CreatedAt:          asOf,
			UpdatedAt:          asOf,
		},
		Nodes:        nodes,
		Edges:        edges,
		PathOutcomes: outcomes,
	}
}
