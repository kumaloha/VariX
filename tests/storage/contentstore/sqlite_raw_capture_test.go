package contentstore

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/kumaloha/VariX/varix/ingest/types"
	"github.com/kumaloha/VariX/varix/model"
)

func TestSQLiteStore_UpsertRawCaptureAndQueueLookup(t *testing.T) {
	root := t.TempDir()
	store, err := NewSQLiteStore(filepath.Join(root, "data", "content.db"))
	if err != nil {
		t.Fatalf("NewSQLiteStore() error = %v", err)
	}
	defer store.Close()

	raw := types.RawContent{
		Source:     "youtube",
		ExternalID: "abc123",
		Content:    "translated excerpt",
		AuthorName: "channel-name",
		URL:        "https://www.youtube.com/watch?v=abc123",
		Provenance: &types.Provenance{
			BaseRelation:      types.BaseRelationUnknown,
			Confidence:        types.ConfidenceLow,
			NeedsSourceLookup: true,
			SourceLookup: types.SourceLookupState{
				Status: types.SourceLookupStatusPending,
			},
		},
	}

	if err := store.UpsertRawCapture(context.Background(), raw); err != nil {
		t.Fatalf("UpsertRawCapture() error = %v", err)
	}

	got, err := store.GetRawCapture(context.Background(), "youtube", "abc123")
	if err != nil {
		t.Fatalf("GetRawCapture() error = %v", err)
	}
	if got.ExternalID != "abc123" {
		t.Fatalf("ExternalID = %q, want abc123", got.ExternalID)
	}
	if got.Provenance == nil || got.Provenance.SourceLookup.Status != types.SourceLookupStatusPending {
		t.Fatalf("SourceLookup.Status = %#v, want pending", got.Provenance)
	}

	items, err := store.ListPendingSourceLookups(context.Background(), 10)
	if err != nil {
		t.Fatalf("ListPendingSourceLookups() error = %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("len(ListPendingSourceLookups()) = %d, want 1", len(items))
	}
	if items[0].ExternalID != "abc123" {
		t.Fatalf("pending ExternalID = %q, want abc123", items[0].ExternalID)
	}
}

func TestSQLiteStore_ListUncompiledRawCaptureRefs(t *testing.T) {
	root := t.TempDir()
	store, err := NewSQLiteStore(filepath.Join(root, "data", "content.db"))
	if err != nil {
		t.Fatalf("NewSQLiteStore() error = %v", err)
	}
	defer store.Close()

	for _, raw := range []types.RawContent{
		{Source: "twitter", ExternalID: "newer", Content: "newer raw", URL: "https://x.com/a/status/newer"},
		{Source: "twitter", ExternalID: "compiled", Content: "compiled raw", URL: "https://x.com/a/status/compiled"},
		{Source: "weibo", ExternalID: "other-platform", Content: "other raw", URL: "https://weibo.com/1/other-platform"},
	} {
		if err := store.UpsertRawCapture(context.Background(), raw); err != nil {
			t.Fatalf("UpsertRawCapture(%s) error = %v", raw.ExternalID, err)
		}
	}
	if err := store.UpsertCompiledOutput(context.Background(), model.Record{
		UnitID:         "twitter:compiled",
		Source:         "twitter",
		ExternalID:     "compiled",
		RootExternalID: "compiled",
		Model:          "test-model",
		Output: model.Output{
			Summary: "already compiled",
			Graph: model.ReasoningGraph{
				Nodes: []model.GraphNode{
					{ID: "n1", Kind: model.NodeFact, Text: "compiled raw", ValidFrom: time.Now().UTC(), ValidTo: time.Now().UTC().Add(time.Hour)},
					{ID: "n2", Kind: model.NodeConclusion, Text: "compiled output", ValidFrom: time.Now().UTC(), ValidTo: time.Now().UTC().Add(time.Hour)},
				},
				Edges: []model.GraphEdge{{From: "n1", To: "n2", Kind: model.EdgePositive}},
			},
			Details: model.HiddenDetails{Caveats: []string{"compiled caveat"}},
		},
		CompiledAt: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("UpsertCompiledOutput() error = %v", err)
	}

	got, err := store.ListUncompiledRawCaptureRefs(context.Background(), 0, "")
	if err != nil {
		t.Fatalf("ListUncompiledRawCaptureRefs() error = %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("len(ListUncompiledRawCaptureRefs) = %d, want 2: %#v", len(got), got)
	}
	for _, ref := range got {
		if ref.ExternalID == "compiled" {
			t.Fatalf("compiled ref was returned: %#v", got)
		}
	}

	got, err = store.ListUncompiledRawCaptureRefs(context.Background(), 10, "twitter")
	if err != nil {
		t.Fatalf("ListUncompiledRawCaptureRefs(twitter) error = %v", err)
	}
	if len(got) != 1 || got[0].ExternalID != "newer" {
		t.Fatalf("twitter refs = %#v, want only newer", got)
	}
}

func TestSQLiteStore_MarkSourceLookupResultUpdatesStatus(t *testing.T) {
	root := t.TempDir()
	store, err := NewSQLiteStore(filepath.Join(root, "data", "content.db"))
	if err != nil {
		t.Fatalf("NewSQLiteStore() error = %v", err)
	}
	defer store.Close()

	raw := types.RawContent{
		Source:     "youtube",
		ExternalID: "abc123",
		Content:    "translated excerpt",
		AuthorName: "channel-name",
		URL:        "https://www.youtube.com/watch?v=abc123",
		Provenance: &types.Provenance{
			NeedsSourceLookup: true,
			SourceLookup: types.SourceLookupState{
				Status: types.SourceLookupStatusPending,
			},
		},
	}
	if err := store.UpsertRawCapture(context.Background(), raw); err != nil {
		t.Fatalf("UpsertRawCapture() error = %v", err)
	}

	raw.Provenance.SourceLookup.Status = types.SourceLookupStatusFound
	raw.Provenance.SourceLookup.CanonicalSourceURL = "https://www.cnbc.com/interview"
	if err := store.MarkSourceLookupResult(context.Background(), raw, types.SourceLookupStatusFound, ""); err != nil {
		t.Fatalf("MarkSourceLookupResult() error = %v", err)
	}

	got, err := store.GetRawCapture(context.Background(), "youtube", "abc123")
	if err != nil {
		t.Fatalf("GetRawCapture() error = %v", err)
	}
	if got.Provenance == nil || got.Provenance.SourceLookup.Status != types.SourceLookupStatusFound {
		t.Fatalf("SourceLookup.Status = %#v, want found", got.Provenance)
	}

	items, err := store.ListPendingSourceLookups(context.Background(), 10)
	if err != nil {
		t.Fatalf("ListPendingSourceLookups() error = %v", err)
	}
	if len(items) != 0 {
		t.Fatalf("len(ListPendingSourceLookups()) = %d, want 0", len(items))
	}
}

func TestSQLiteStore_MarkSourceLookupResultPersistsDerivedProvenance(t *testing.T) {
	root := t.TempDir()
	store, err := NewSQLiteStore(filepath.Join(root, "data", "content.db"))
	if err != nil {
		t.Fatalf("NewSQLiteStore() error = %v", err)
	}
	defer store.Close()

	raw := types.RawContent{
		Source:     "youtube",
		ExternalID: "abc123",
		Content:    "translated excerpt",
		AuthorName: "channel-name",
		URL:        "https://www.youtube.com/watch?v=abc123",
		Provenance: &types.Provenance{
			BaseRelation:      types.BaseRelationUnknown,
			EditorialLayer:    types.EditorialLayerUnknown,
			Confidence:        types.ConfidenceLow,
			NeedsSourceLookup: true,
			SourceLookup: types.SourceLookupState{
				Status: types.SourceLookupStatusPending,
			},
		},
	}
	if err := store.UpsertRawCapture(context.Background(), raw); err != nil {
		t.Fatalf("UpsertRawCapture() error = %v", err)
	}

	raw.Provenance.BaseRelation = types.BaseRelationTranslation
	raw.Provenance.EditorialLayer = types.EditorialLayerCommentary
	raw.Provenance.Fidelity = types.FidelityLikelyAdapted
	raw.Provenance.SourceLookup = types.SourceLookupState{
		Status:             types.SourceLookupStatusFound,
		CanonicalSourceURL: "https://www.cnbc.com/interview",
		ResolvedBy:         "fake_judge",
		MatchKind:          types.SourceMatchLikelyDerived,
	}
	if err := store.MarkSourceLookupResult(context.Background(), raw, types.SourceLookupStatusFound, ""); err != nil {
		t.Fatalf("MarkSourceLookupResult() error = %v", err)
	}

	got, err := store.GetRawCapture(context.Background(), "youtube", "abc123")
	if err != nil {
		t.Fatalf("GetRawCapture() error = %v", err)
	}
	if got.Provenance.BaseRelation != types.BaseRelationTranslation {
		t.Fatalf("BaseRelation = %q, want translation", got.Provenance.BaseRelation)
	}
	if got.Provenance.EditorialLayer != types.EditorialLayerCommentary {
		t.Fatalf("EditorialLayer = %q, want commentary", got.Provenance.EditorialLayer)
	}
	if got.Provenance.Fidelity != types.FidelityLikelyAdapted {
		t.Fatalf("Fidelity = %q, want likely_adapted", got.Provenance.Fidelity)
	}
}
