package provenance

import (
	"testing"

	"github.com/kumaloha/VariX/varix/ingest/types"
)

func TestEnricher_AnnotateSetsProvenance(t *testing.T) {
	items := []types.RawContent{{
		Source:     "youtube",
		ExternalID: "abc123",
		Content:    "访谈内容",
		AuthorName: "channel",
		URL:        "https://www.youtube.com/watch?v=abc123",
		Metadata: types.RawMetadata{
			YouTube: &types.YouTubeMetadata{
				Title:       "巴菲特访谈中字解读",
				Description: "原视频：https://www.cnbc.com/interview",
				SourceLinks: []string{"https://www.cnbc.com/interview"},
			},
		},
	}}

	got := (Enricher{}).Annotate(items)
	if len(got) != 1 {
		t.Fatalf("len(Annotate()) = %d, want 1", len(got))
	}
	if got[0].Provenance == nil {
		t.Fatal("Provenance is nil")
	}
	if got[0].Provenance.NeedsSourceLookup != true {
		t.Fatalf("NeedsSourceLookup = %v, want true", got[0].Provenance.NeedsSourceLookup)
	}
	if got[0].Provenance.SourceLookup.Status != types.SourceLookupStatusPending {
		t.Fatalf("SourceLookup.Status = %q, want pending", got[0].Provenance.SourceLookup.Status)
	}
}

func TestEnricher_AnnotatePreservesResolvedSourceLookup(t *testing.T) {
	items := []types.RawContent{{
		Source:     "twitter",
		ExternalID: "123",
		Content:    "translated test",
		URL:        "https://x.com/a/status/123",
		Provenance: &types.Provenance{
			BaseRelation:      types.BaseRelationQuote,
			NeedsSourceLookup: true,
			SourceLookup: types.SourceLookupState{
				Status:             types.SourceLookupStatusFound,
				CanonicalSourceURL: "https://example.com/source",
			},
		},
	}}

	got := (Enricher{}).Annotate(items)
	if len(got) != 1 {
		t.Fatalf("len(Annotate()) = %d, want 1", len(got))
	}
	if got[0].Provenance == nil {
		t.Fatal("Provenance is nil")
	}
	if got[0].Provenance.SourceLookup.Status != types.SourceLookupStatusFound {
		t.Fatalf("SourceLookup.Status = %q, want %q", got[0].Provenance.SourceLookup.Status, types.SourceLookupStatusFound)
	}
	if got[0].Provenance.SourceLookup.CanonicalSourceURL != "https://example.com/source" {
		t.Fatalf("CanonicalSourceURL = %q, want preserved source url", got[0].Provenance.SourceLookup.CanonicalSourceURL)
	}
}
