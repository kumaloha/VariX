package provenance

import (
	"context"
	"testing"

	"github.com/kumaloha/VariX/varix/ingest/types"
)

func TestDeterministicJudge_JudgesSameSource(t *testing.T) {
	raw := types.RawContent{
		URL: "https://www.cnbc.com/interview",
		Provenance: &types.Provenance{
			BaseRelation: types.BaseRelationUnknown,
		},
	}

	got, err := DeterministicJudge{}.Judge(context.Background(), raw, []types.SourceCandidate{
		{URL: "https://www.cnbc.com/interview", Host: "www.cnbc.com", Kind: "embedded_link", Confidence: "high"},
	})
	if err != nil {
		t.Fatalf("Judge() error = %v", err)
	}
	if got.Lookup.Status != types.SourceLookupStatusFound || got.Lookup.MatchKind != types.SourceMatchSameSource {
		t.Fatalf("state = %#v, want found/same_source", got)
	}
}

func TestDeterministicJudge_JudgesLikelyDerived(t *testing.T) {
	raw := types.RawContent{
		URL: "https://www.youtube.com/watch?v=abc123",
		Provenance: &types.Provenance{
			BaseRelation: types.BaseRelationTranslation,
		},
	}

	got, err := DeterministicJudge{}.Judge(context.Background(), raw, []types.SourceCandidate{
		{URL: "https://www.cnbc.com/interview", Host: "www.cnbc.com", Kind: "embedded_link", Confidence: "high"},
	})
	if err != nil {
		t.Fatalf("Judge() error = %v", err)
	}
	if got.Lookup.Status != types.SourceLookupStatusFound || got.Lookup.MatchKind != types.SourceMatchLikelyDerived {
		t.Fatalf("state = %#v, want found/likely_derived", got)
	}
	if got.BaseRelation != types.BaseRelationTranslation {
		t.Fatalf("BaseRelation = %q, want translation", got.BaseRelation)
	}
	if got.Fidelity != types.FidelityLikelyAdapted {
		t.Fatalf("Fidelity = %q, want likely_adapted", got.Fidelity)
	}
}

func TestDeterministicJudge_JudgesUnrelatedWhenOnlySyntheticSearchExists(t *testing.T) {
	raw := types.RawContent{
		URL: "https://www.youtube.com/watch?v=abc123",
		Provenance: &types.Provenance{
			BaseRelation: types.BaseRelationUnknown,
		},
	}

	got, err := DeterministicJudge{}.Judge(context.Background(), raw, []types.SourceCandidate{
		{URL: "search://title/buffett-interview", Kind: "title_search", Confidence: "medium"},
	})
	if err != nil {
		t.Fatalf("Judge() error = %v", err)
	}
	if got.Lookup.Status != types.SourceLookupStatusNotFound || got.Lookup.MatchKind != types.SourceMatchUnrelated {
		t.Fatalf("state = %#v, want not_found/unrelated", got)
	}
}

func TestDeterministicJudge_DoesNotTreatPlatformSelfLinkAsSameSource(t *testing.T) {
	raw := types.RawContent{
		URL: "https://www.youtube.com/watch?v=MLhbaA7XW1M",
		Provenance: &types.Provenance{
			BaseRelation: types.BaseRelationTranslation,
		},
	}

	got, err := DeterministicJudge{}.Judge(context.Background(), raw, []types.SourceCandidate{
		{URL: "https://www.youtube.com/channel/UC1Xm-VhWUqZcPCCN5R2MniA/join", Host: "www.youtube.com", Kind: "embedded_link", Confidence: "high"},
	})
	if err != nil {
		t.Fatalf("Judge() error = %v", err)
	}
	if got.Lookup.MatchKind == types.SourceMatchSameSource {
		t.Fatalf("state = %#v, want not same_source for platform self-link", got)
	}
}

func TestDeterministicJudge_JudgesCrossPlatformLinkAsLikelyDerived(t *testing.T) {
	raw := types.RawContent{
		URL: "https://www.bilibili.com/video/BV1kPE7zEEA2/",
		Provenance: &types.Provenance{
			BaseRelation: types.BaseRelationTranslation,
		},
	}

	got, err := DeterministicJudge{}.Judge(context.Background(), raw, []types.SourceCandidate{
		{URL: "https://www.youtube.com/watch?v=As1a2VgbdWg", Host: "www.youtube.com", Kind: "embedded_link", Confidence: "high"},
	})
	if err != nil {
		t.Fatalf("Judge() error = %v", err)
	}
	if got.Lookup.Status != types.SourceLookupStatusFound || got.Lookup.MatchKind != types.SourceMatchLikelyDerived {
		t.Fatalf("state = %#v, want found/likely_derived", got)
	}
}

func TestDeterministicJudge_JudgesHighConfidenceCrossPlatformLinkAsLikelyDerivedEvenWhenUnknown(t *testing.T) {
	raw := types.RawContent{
		URL: "https://www.bilibili.com/video/BV1kPE7zEEA2/",
		Provenance: &types.Provenance{
			BaseRelation: types.BaseRelationUnknown,
		},
	}

	got, err := DeterministicJudge{}.Judge(context.Background(), raw, []types.SourceCandidate{
		{URL: "https://www.youtube.com/watch?v=As1a2VgbdWg", Host: "www.youtube.com", Kind: "embedded_link", Confidence: "high"},
	})
	if err != nil {
		t.Fatalf("Judge() error = %v", err)
	}
	if got.Lookup.Status != types.SourceLookupStatusFound || got.Lookup.MatchKind != types.SourceMatchLikelyDerived {
		t.Fatalf("state = %#v, want found/likely_derived", got)
	}
}

func TestDeterministicJudge_PrefersLaterSameSourceCandidateOverEarlierUnrelatedConcrete(t *testing.T) {
	raw := types.RawContent{
		URL: "https://www.cnbc.com/interview",
		Provenance: &types.Provenance{
			BaseRelation: types.BaseRelationTranslation,
		},
	}

	got, err := DeterministicJudge{}.Judge(context.Background(), raw, []types.SourceCandidate{
		{URL: "https://www.youtube.com/watch?v=abc123", Host: "www.youtube.com", Kind: "embedded_link", Confidence: "high"},
		{URL: "https://www.cnbc.com/interview", Host: "www.cnbc.com", Kind: "embedded_link", Confidence: "high"},
	})
	if err != nil {
		t.Fatalf("Judge() error = %v", err)
	}
	if got.Lookup.MatchKind != types.SourceMatchSameSource {
		t.Fatalf("MatchKind = %q, want same_source", got.Lookup.MatchKind)
	}
	if got.Lookup.CanonicalSourceURL != "https://www.cnbc.com/interview" {
		t.Fatalf("CanonicalSourceURL = %q, want CNBC URL", got.Lookup.CanonicalSourceURL)
	}
}

func TestDeterministicJudge_PrefersLaterLikelyDerivedCandidateWhenEarlierConcreteIsUnrelated(t *testing.T) {
	raw := types.RawContent{
		URL: "https://www.youtube.com/watch?v=abc123",
		Provenance: &types.Provenance{
			BaseRelation: types.BaseRelationTranslation,
		},
	}

	got, err := DeterministicJudge{}.Judge(context.Background(), raw, []types.SourceCandidate{
		{URL: "https://www.youtube.com/channel/UC123/join", Host: "www.youtube.com", Kind: "embedded_link", Confidence: "high"},
		{URL: "https://www.cnbc.com/interview", Host: "www.cnbc.com", Kind: "embedded_link", Confidence: "high"},
	})
	if err != nil {
		t.Fatalf("Judge() error = %v", err)
	}
	if got.Lookup.MatchKind != types.SourceMatchLikelyDerived {
		t.Fatalf("MatchKind = %q, want likely_derived", got.Lookup.MatchKind)
	}
	if got.Lookup.CanonicalSourceURL != "https://www.cnbc.com/interview" {
		t.Fatalf("CanonicalSourceURL = %q, want CNBC URL", got.Lookup.CanonicalSourceURL)
	}
}
