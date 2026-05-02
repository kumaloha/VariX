package contentstore

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/kumaloha/VariX/varix/ingest/types"
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
