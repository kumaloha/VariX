package provenance

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/kumaloha/VariX/varix/ingest/types"
)

func TestServiceRunOnceMarksFoundWhenJudgeFindsSource(t *testing.T) {
	raw := types.RawContent{
		Source:     "youtube",
		ExternalID: "abc123",
		Content:    "translated interview",
		Provenance: &types.Provenance{
			NeedsSourceLookup: true,
			SourceLookup: types.SourceLookupState{
				Status: types.SourceLookupStatusPending,
			},
		},
	}

	store := &fakeStore{pending: []types.RawContent{raw}}
	service := NewService(
		store,
		fakeFinder{
			candidates: []types.SourceCandidate{
				{URL: "https://www.cnbc.com/interview", Host: "www.cnbc.com", Kind: "embedded_link", Confidence: "high"},
			},
		},
		fakeJudge{
			state: types.SourceLookupState{
				Status:             types.SourceLookupStatusFound,
				CanonicalSourceURL: "https://www.cnbc.com/interview",
				ResolvedBy:         "fake_judge",
				MatchKind:          types.SourceMatchSameSource,
			},
		},
	)

	report, err := service.RunOnce(context.Background(), 10)
	if err != nil {
		t.Fatalf("RunOnce() error = %v", err)
	}
	if report.ProcessedCount != 1 || report.FoundCount != 1 {
		t.Fatalf("report = %#v, want processed=1 found=1", report)
	}
	if len(store.marked) != 1 {
		t.Fatalf("len(marked) = %d, want 1", len(store.marked))
	}
	if store.marked[0].status != types.SourceLookupStatusFound {
		t.Fatalf("status = %q, want found", store.marked[0].status)
	}
	if got := store.marked[0].raw.Provenance.SourceLookup.CanonicalSourceURL; got != "https://www.cnbc.com/interview" {
		t.Fatalf("CanonicalSourceURL = %q, want https://www.cnbc.com/interview", got)
	}
	if len(store.marked[0].raw.Provenance.SourceCandidates) != 1 {
		t.Fatalf("SourceCandidates = %#v, want 1 candidate", store.marked[0].raw.Provenance.SourceCandidates)
	}
}

func TestServiceRunOnceMarksFailedWhenFinderErrors(t *testing.T) {
	raw := types.RawContent{
		Source:     "youtube",
		ExternalID: "abc123",
		Provenance: &types.Provenance{
			NeedsSourceLookup: true,
			SourceLookup: types.SourceLookupState{
				Status: types.SourceLookupStatusPending,
			},
		},
	}

	store := &fakeStore{pending: []types.RawContent{raw}}
	service := NewService(
		store,
		fakeFinder{err: errors.New("finder failed")},
		fakeJudge{},
	)

	report, err := service.RunOnce(context.Background(), 10)
	if err != nil {
		t.Fatalf("RunOnce() error = %v", err)
	}
	if report.ProcessedCount != 1 || report.FailedCount != 1 {
		t.Fatalf("report = %#v, want processed=1 failed=1", report)
	}
	if len(store.marked) != 1 {
		t.Fatalf("len(marked) = %d, want 1", len(store.marked))
	}
	if store.marked[0].status != types.SourceLookupStatusFailed {
		t.Fatalf("status = %q, want failed", store.marked[0].status)
	}
	if store.marked[0].errDetail != "finder failed" {
		t.Fatalf("errDetail = %q, want finder failed", store.marked[0].errDetail)
	}
}

func TestServiceRunOnceAppliesLikelyDerivedProvenance(t *testing.T) {
	raw := types.RawContent{
		Source:     "youtube",
		ExternalID: "abc123",
		Content:    "translated interview",
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

	store := &fakeStore{pending: []types.RawContent{raw}}
	service := NewService(
		store,
		fakeFinder{
			candidates: []types.SourceCandidate{
				{URL: "https://www.cnbc.com/interview", Host: "www.cnbc.com", Kind: "embedded_link", Confidence: "high"},
			},
		},
		fakeJudge{
			state: types.SourceLookupState{
				Status:             types.SourceLookupStatusFound,
				CanonicalSourceURL: "https://www.cnbc.com/interview",
				ResolvedBy:         "fake_judge",
				MatchKind:          types.SourceMatchLikelyDerived,
			},
			baseRelation:   types.BaseRelationTranslation,
			editorialLayer: types.EditorialLayerCommentary,
			fidelity:       types.FidelityLikelyAdapted,
		},
	)

	report, err := service.RunOnce(context.Background(), 10)
	if err != nil {
		t.Fatalf("RunOnce() error = %v", err)
	}
	if report.FoundCount != 1 {
		t.Fatalf("report = %#v, want found=1", report)
	}
	got := store.marked[0].raw.Provenance
	if got.BaseRelation != types.BaseRelationTranslation {
		t.Fatalf("BaseRelation = %q, want translation", got.BaseRelation)
	}
	if got.EditorialLayer != types.EditorialLayerCommentary {
		t.Fatalf("EditorialLayer = %q, want commentary", got.EditorialLayer)
	}
	if got.Fidelity != types.FidelityLikelyAdapted {
		t.Fatalf("Fidelity = %q, want likely_adapted", got.Fidelity)
	}
}

func TestServiceRunOnceLeavesUnknownOnUnrelated(t *testing.T) {
	raw := types.RawContent{
		Source:     "youtube",
		ExternalID: "abc123",
		Content:    "interview",
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

	store := &fakeStore{pending: []types.RawContent{raw}}
	service := NewService(
		store,
		fakeFinder{
			candidates: []types.SourceCandidate{
				{URL: "search://title/buffett-interview", Kind: "title_search", Confidence: "medium"},
			},
		},
		fakeJudge{
			state: types.SourceLookupState{
				Status:     types.SourceLookupStatusNotFound,
				ResolvedBy: "fake_judge",
				MatchKind:  types.SourceMatchUnrelated,
			},
			baseRelation:   types.BaseRelationExcerpt,
			editorialLayer: types.EditorialLayerCommentary,
			fidelity:       types.FidelityLikelyAdapted,
		},
	)

	report, err := service.RunOnce(context.Background(), 10)
	if err != nil {
		t.Fatalf("RunOnce() error = %v", err)
	}
	if report.NotFoundCount != 1 {
		t.Fatalf("report = %#v, want not_found=1", report)
	}
	got := store.marked[0].raw.Provenance
	if got.BaseRelation != types.BaseRelationUnknown {
		t.Fatalf("BaseRelation = %q, want unknown", got.BaseRelation)
	}
	if got.EditorialLayer != types.EditorialLayerUnknown {
		t.Fatalf("EditorialLayer = %q, want unknown", got.EditorialLayer)
	}
	if got.Fidelity != "" {
		t.Fatalf("Fidelity = %q, want empty", got.Fidelity)
	}
}

func TestServiceRunOnceSanitizesExistingSourceCandidates(t *testing.T) {
	raw := types.RawContent{
		Source:     "youtube",
		ExternalID: "abc123",
		URL:        "https://www.youtube.com/watch?v=MLhbaA7XW1M",
		Metadata: types.RawMetadata{
			YouTube: &types.YouTubeMetadata{
				Title: "巴菲特访谈中字解读",
			},
		},
		Provenance: &types.Provenance{
			BaseRelation:      types.BaseRelationTranslation,
			EditorialLayer:    types.EditorialLayerNone,
			Confidence:        types.ConfidenceHigh,
			NeedsSourceLookup: true,
			SourceCandidates: []types.SourceCandidate{
				{URL: "https://www.youtube.com/channel/UC1Xm-VhWUqZcPCCN5R2MniA/join", Host: "www.youtube.com", Kind: "embedded_link", Confidence: "high"},
				{URL: "https://www.cnbc.com/video/buffett-interview", Host: "www.cnbc.com", Kind: "embedded_link", Confidence: "high"},
			},
			SourceLookup: types.SourceLookupState{
				Status: types.SourceLookupStatusPending,
			},
		},
	}

	store := &fakeStore{pending: []types.RawContent{raw}}
	service := NewService(store, NewRuleFinder(), fakeJudge{
		state: types.SourceLookupState{
			Status:             types.SourceLookupStatusFound,
			CanonicalSourceURL: "https://www.cnbc.com/video/buffett-interview",
			ResolvedBy:         "fake_judge",
			MatchKind:          types.SourceMatchLikelyDerived,
		},
	})

	_, err := service.RunOnce(context.Background(), 10)
	if err != nil {
		t.Fatalf("RunOnce() error = %v", err)
	}
	got := store.marked[0].raw.Provenance.SourceCandidates
	if len(got) == 0 || got[0].URL != "https://www.cnbc.com/video/buffett-interview" {
		t.Fatalf("SourceCandidates = %#v, want sanitized external candidate first", got)
	}
	for _, candidate := range got {
		if candidate.URL == "https://www.youtube.com/channel/UC1Xm-VhWUqZcPCCN5R2MniA/join" {
			t.Fatalf("SourceCandidates = %#v, want join link removed", got)
		}
	}
}

func TestServiceRunOncePreservesMergedCandidateOrderForJudge(t *testing.T) {
	raw := types.RawContent{
		Source:     "youtube",
		ExternalID: "abc123",
		URL:        "https://www.youtube.com/watch?v=abc123",
		Provenance: &types.Provenance{
			NeedsSourceLookup: true,
			SourceCandidates: []types.SourceCandidate{
				{URL: "https://existing.example/first", Host: "existing.example", Kind: "reference", Confidence: "low"},
			},
			SourceLookup: types.SourceLookupState{
				Status: types.SourceLookupStatusPending,
			},
		},
	}

	store := &fakeStore{pending: []types.RawContent{raw}}
	judge := &capturingJudge{
		result: MatchResult{
			Lookup: types.SourceLookupState{
				Status:     types.SourceLookupStatusNotFound,
				ResolvedBy: "capturing_judge",
				MatchKind:  types.SourceMatchUnrelated,
			},
		},
	}
	service := NewService(
		store,
		fakeFinder{
			candidates: []types.SourceCandidate{
				{URL: "https://finder.example/second", Host: "finder.example", Kind: "embedded_link", Confidence: "high"},
			},
		},
		judge,
	)

	if _, err := service.RunOnce(context.Background(), 10); err != nil {
		t.Fatalf("RunOnce() error = %v", err)
	}

	if len(judge.seen) != 2 {
		t.Fatalf("judge saw %d candidates, want 2", len(judge.seen))
	}
	if judge.seen[0].URL != "https://existing.example/first" {
		t.Fatalf("first candidate = %q, want existing candidate first", judge.seen[0].URL)
	}
	if judge.seen[1].URL != "https://finder.example/second" {
		t.Fatalf("second candidate = %q, want finder candidate second", judge.seen[1].URL)
	}
}

func TestMergeCandidates_PrefersStrongerConfidenceForDuplicateURL(t *testing.T) {
	got := mergeCandidates(
		[]types.SourceCandidate{{
			URL:        "https://example.com/source",
			Host:       "example.com",
			Kind:       "reference_link",
			Confidence: string(types.ConfidenceLow),
		}},
		[]types.SourceCandidate{{
			URL:        "https://example.com/source",
			Host:       "example.com",
			Kind:       "reference_link",
			Confidence: string(types.ConfidenceHigh),
		}},
	)

	if len(got) != 1 {
		t.Fatalf("len(got) = %d, want 1", len(got))
	}
	if got[0].Confidence != string(types.ConfidenceHigh) {
		t.Fatalf("Confidence = %q, want high", got[0].Confidence)
	}
}

func TestMergeCandidates_PrefersStrongerConfidenceForDuplicateHostKind(t *testing.T) {
	got := mergeCandidates(
		[]types.SourceCandidate{{
			Host:       "example.com",
			Kind:       "reference_link",
			Confidence: string(types.ConfidenceLow),
		}},
		[]types.SourceCandidate{{
			Host:       "example.com",
			Kind:       "reference_link",
			Confidence: string(types.ConfidenceMedium),
		}},
	)

	if len(got) != 1 {
		t.Fatalf("len(got) = %d, want 1", len(got))
	}
	if got[0].Confidence != string(types.ConfidenceMedium) {
		t.Fatalf("Confidence = %q, want medium", got[0].Confidence)
	}
}

func TestMergeCandidates_IsDeterministicAcrossRepeatedMerges(t *testing.T) {
	existing := []types.SourceCandidate{{
		URL:        "https://example.com/source",
		Host:       "example.com",
		Kind:       "reference_link",
		Confidence: string(types.ConfidenceLow),
	}}
	incoming := []types.SourceCandidate{{
		URL:        "https://example.com/source",
		Host:       "example.com",
		Kind:       "reference_link",
		Confidence: string(types.ConfidenceHigh),
	}, {
		URL:        "https://example.com/other",
		Host:       "example.com",
		Kind:       "source_link",
		Confidence: string(types.ConfidenceMedium),
	}}

	first := mergeCandidates(existing, incoming)
	second := mergeCandidates(first, incoming)
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("repeated merge mismatch:\nfirst  = %#v\nsecond = %#v", first, second)
	}
}

func TestMergeCandidates_EmptyConfidenceStillBackfillsMissingFields(t *testing.T) {
	got := mergeCandidates(
		[]types.SourceCandidate{{
			URL:        "https://example.com/source",
			Confidence: "",
		}},
		[]types.SourceCandidate{{
			URL:        "https://example.com/source",
			Host:       "example.com",
			Kind:       "reference_link",
			Confidence: "",
		}},
	)

	if len(got) != 1 {
		t.Fatalf("len(got) = %d, want 1", len(got))
	}
	if got[0].Confidence != "" {
		t.Fatalf("Confidence = %q, want empty", got[0].Confidence)
	}
	if got[0].Host != "example.com" || got[0].Kind != "reference_link" {
		t.Fatalf("merged candidate = %#v, want host/kind backfilled", got[0])
	}
}

func TestMergeCandidates_UnknownConfidenceStillBackfillsMissingFields(t *testing.T) {
	got := mergeCandidates(
		[]types.SourceCandidate{{
			URL:        "https://example.com/source",
			Confidence: "foo",
		}},
		[]types.SourceCandidate{{
			URL:        "https://example.com/source",
			Host:       "example.com",
			Kind:       "reference_link",
			Confidence: "bar",
		}},
	)

	if len(got) != 1 {
		t.Fatalf("len(got) = %d, want 1", len(got))
	}
	if got[0].URL != "https://example.com/source" {
		t.Fatalf("URL = %q, want backfilled source url", got[0].URL)
	}
	if got[0].Confidence != "foo" {
		t.Fatalf("Confidence = %q, want existing tie-ranked value preserved", got[0].Confidence)
	}
}

type fakeStore struct {
	pending []types.RawContent
	marked  []markedResult
}

type markedResult struct {
	raw       types.RawContent
	status    types.SourceLookupStatus
	errDetail string
}

func (f *fakeStore) ListPendingSourceLookups(_ context.Context, _ int) ([]types.RawContent, error) {
	return f.pending, nil
}

func (f *fakeStore) MarkSourceLookupResult(_ context.Context, raw types.RawContent, status types.SourceLookupStatus, errDetail string) error {
	f.marked = append(f.marked, markedResult{raw: raw, status: status, errDetail: errDetail})
	return nil
}

type fakeFinder struct {
	candidates []types.SourceCandidate
	err        error
}

func (f fakeFinder) FindCandidates(_ context.Context, _ types.RawContent) ([]types.SourceCandidate, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.candidates, nil
}

type fakeJudge struct {
	state          types.SourceLookupState
	baseRelation   types.BaseRelation
	editorialLayer types.EditorialLayer
	fidelity       types.Fidelity
	err            error
}

func (f fakeJudge) Judge(_ context.Context, _ types.RawContent, _ []types.SourceCandidate) (MatchResult, error) {
	if f.err != nil {
		return MatchResult{}, f.err
	}
	return MatchResult{
		Lookup:         f.state,
		BaseRelation:   f.baseRelation,
		EditorialLayer: f.editorialLayer,
		Fidelity:       f.fidelity,
	}, nil
}

type capturingJudge struct {
	seen   []types.SourceCandidate
	result MatchResult
}

func (c *capturingJudge) Judge(_ context.Context, _ types.RawContent, candidates []types.SourceCandidate) (MatchResult, error) {
	c.seen = append([]types.SourceCandidate(nil), candidates...)
	return c.result, nil
}

func TestServiceRunOnce_DoesNotApplyDerivedFieldsWhenLookupNotFound(t *testing.T) {
	raw := types.RawContent{
		Source:     "youtube",
		ExternalID: "abc123",
		Content:    "translated interview",
		Provenance: &types.Provenance{
			BaseRelation:      types.BaseRelationUnknown,
			EditorialLayer:    types.EditorialLayerUnknown,
			Confidence:        types.ConfidenceLow,
			NeedsSourceLookup: true,
			SourceLookup:      types.SourceLookupState{Status: types.SourceLookupStatusPending},
		},
	}

	store := &fakeStore{pending: []types.RawContent{raw}}
	service := NewService(
		store,
		fakeFinder{candidates: []types.SourceCandidate{{URL: "https://www.cnbc.com/interview", Host: "www.cnbc.com", Kind: "embedded_link", Confidence: "high"}}},
		fakeJudge{
			state:          types.SourceLookupState{Status: types.SourceLookupStatusNotFound, ResolvedBy: "fake_judge", MatchKind: types.SourceMatchLikelyDerived},
			baseRelation:   types.BaseRelationTranslation,
			editorialLayer: types.EditorialLayerCommentary,
			fidelity:       types.FidelityLikelyAdapted,
		},
	)

	report, err := service.RunOnce(context.Background(), 10)
	if err != nil {
		t.Fatalf("RunOnce() error = %v", err)
	}
	if report.NotFoundCount != 1 {
		t.Fatalf("report = %#v, want not_found=1", report)
	}
	got := store.marked[0].raw.Provenance
	if got.BaseRelation != types.BaseRelationUnknown {
		t.Fatalf("BaseRelation = %q, want unknown", got.BaseRelation)
	}
	if got.EditorialLayer != types.EditorialLayerUnknown {
		t.Fatalf("EditorialLayer = %q, want unknown", got.EditorialLayer)
	}
	if got.Fidelity != "" {
		t.Fatalf("Fidelity = %q, want empty", got.Fidelity)
	}
}
