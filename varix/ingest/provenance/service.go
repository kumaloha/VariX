package provenance

import (
	"context"
	"strings"

	"github.com/kumaloha/VariX/varix/ingest/types"
)

type RawCaptureStore interface {
	ListPendingSourceLookups(ctx context.Context, limit int) ([]types.RawContent, error)
	MarkSourceLookupResult(ctx context.Context, raw types.RawContent, status types.SourceLookupStatus, errDetail string) error
}

type Finder interface {
	FindCandidates(ctx context.Context, raw types.RawContent) ([]types.SourceCandidate, error)
}

type Judge interface {
	Judge(ctx context.Context, raw types.RawContent, candidates []types.SourceCandidate) (MatchResult, error)
}

type Report struct {
	ProcessedCount int
	FoundCount     int
	NotFoundCount  int
	FailedCount    int
}

type Service struct {
	store  RawCaptureStore
	finder Finder
	judge  Judge
}

func NewService(store RawCaptureStore, finder Finder, judge Judge) *Service {
	return &Service{
		store:  store,
		finder: finder,
		judge:  judge,
	}
}

func (s *Service) RunOnce(ctx context.Context, limit int) (Report, error) {
	items, err := s.store.ListPendingSourceLookups(ctx, limit)
	if err != nil {
		return Report{}, err
	}

	var report Report
	for _, raw := range items {
		report.ProcessedCount++
		if raw.Provenance == nil {
			raw.Provenance = &types.Provenance{}
		}

		candidates := sanitizeCandidates(raw, raw.Provenance.SourceCandidates)
		if s.finder != nil {
			found, err := s.finder.FindCandidates(ctx, raw)
			if err != nil {
				report.FailedCount++
				raw.Provenance.SourceLookup.Status = types.SourceLookupStatusFailed
				if err := s.store.MarkSourceLookupResult(ctx, raw, types.SourceLookupStatusFailed, err.Error()); err != nil {
					return report, err
				}
				continue
			}
			candidates = mergeCandidates(candidates, found)
		}
		raw.Provenance.SourceCandidates = candidates

		result, err := s.judge.Judge(ctx, raw, candidates)
		if err != nil {
			report.FailedCount++
			raw.Provenance.SourceLookup.Status = types.SourceLookupStatusFailed
			if err := s.store.MarkSourceLookupResult(ctx, raw, types.SourceLookupStatusFailed, err.Error()); err != nil {
				return report, err
			}
			continue
		}
		state := result.Lookup
		if state.Status == "" {
			state.Status = types.SourceLookupStatusNotFound
		}
		applyMatchResult(raw.Provenance, result)
		raw.Provenance.SourceLookup = state
		if err := s.store.MarkSourceLookupResult(ctx, raw, state.Status, ""); err != nil {
			return report, err
		}
		switch state.Status {
		case types.SourceLookupStatusFound:
			report.FoundCount++
		case types.SourceLookupStatusNotFound:
			report.NotFoundCount++
		case types.SourceLookupStatusFailed:
			report.FailedCount++
		}
	}
	return report, nil
}

func sanitizeCandidates(raw types.RawContent, candidates []types.SourceCandidate) []types.SourceCandidate {
	if len(candidates) == 0 {
		return nil
	}
	out := make([]types.SourceCandidate, 0, len(candidates))
	rawHost := hostFromURL(raw.URL)
	for _, candidate := range candidates {
		if shouldFilterSelfLink(rawHost, candidate.URL) {
			continue
		}
		out = append(out, candidate)
	}
	return out
}

func applyMatchResult(prov *types.Provenance, result MatchResult) {
	if prov == nil || result.Lookup.Status != types.SourceLookupStatusFound {
		return
	}
	if result.Lookup.MatchKind != types.SourceMatchLikelyDerived {
		return
	}
	if result.BaseRelation != "" && result.BaseRelation != types.BaseRelationUnknown {
		prov.BaseRelation = result.BaseRelation
	}
	if result.EditorialLayer != "" && result.EditorialLayer != types.EditorialLayerUnknown {
		prov.EditorialLayer = result.EditorialLayer
	}
	if result.Fidelity != "" {
		prov.Fidelity = result.Fidelity
	}
}

func mergeCandidates(existing []types.SourceCandidate, incoming []types.SourceCandidate) []types.SourceCandidate {
	if len(incoming) == 0 {
		return existing
	}
	out := append([]types.SourceCandidate(nil), existing...)
	seen := make(map[string]struct{}, len(existing))
	for _, candidate := range existing {
		key := candidateKey(candidate)
		if key != "" {
			seen[key] = struct{}{}
		}
	}
	for _, candidate := range incoming {
		key := candidateKey(candidate)
		if key == "" {
			continue
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, candidate)
	}
	return out
}

func candidateKey(candidate types.SourceCandidate) string {
	if trimmed := strings.TrimSpace(candidate.URL); trimmed != "" {
		return trimmed
	}
	if trimmed := strings.TrimSpace(candidate.Host); trimmed != "" {
		return trimmed + "|" + candidate.Kind
	}
	return ""
}
