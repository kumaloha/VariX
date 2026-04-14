package provenance

import (
	"context"
	"strings"

	"github.com/kumaloha/VariX/varix/ingest/types"
)

type MatchResult struct {
	Lookup         types.SourceLookupState
	BaseRelation   types.BaseRelation
	EditorialLayer types.EditorialLayer
	Fidelity       types.Fidelity
}

type DeterministicJudge struct{}

func (DeterministicJudge) Judge(_ context.Context, raw types.RawContent, candidates []types.SourceCandidate) (MatchResult, error) {
	concrete := concreteCandidates(candidates)
	if len(concrete) == 0 {
		return MatchResult{
			Lookup: types.SourceLookupState{
				Status:     types.SourceLookupStatusNotFound,
				ResolvedBy: "deterministic_judge",
				MatchKind:  types.SourceMatchUnrelated,
			},
		}, nil
	}

	for _, candidate := range concrete {
		if sameCanonicalTarget(raw.URL, candidate.URL) || isSameSourceCandidate(raw.URL, candidate.URL) {
			return MatchResult{
				Lookup: types.SourceLookupState{
					Status:             types.SourceLookupStatusFound,
					CanonicalSourceURL: candidate.URL,
					ResolvedBy:         "deterministic_judge",
					MatchKind:          types.SourceMatchSameSource,
				},
			}, nil
		}
	}

	for _, candidate := range concrete {
		if isExplicitRelationCandidate(candidate) && indicatesDerivation(raw) {
			return MatchResult{
				Lookup: types.SourceLookupState{
					Status:             types.SourceLookupStatusFound,
					CanonicalSourceURL: candidate.URL,
					ResolvedBy:         "deterministic_judge",
					MatchKind:          types.SourceMatchLikelyDerived,
				},
				BaseRelation:   raw.Provenance.BaseRelation,
				EditorialLayer: raw.Provenance.EditorialLayer,
				Fidelity:       defaultFidelity(raw.Provenance.BaseRelation),
			}, nil
		}
	}

	rawHost := hostFromURL(raw.URL)
	for _, candidate := range concrete {
		candidateHost := hostFromURL(candidate.URL)
		if isCrossPlatformCandidate(rawHost, candidateHost) && (indicatesDerivation(raw) || isHighConfidenceEmbeddedLink(candidate)) {
			return MatchResult{
				Lookup: types.SourceLookupState{
					Status:             types.SourceLookupStatusFound,
					CanonicalSourceURL: candidate.URL,
					ResolvedBy:         "deterministic_judge",
					MatchKind:          types.SourceMatchLikelyDerived,
				},
				BaseRelation:   raw.Provenance.BaseRelation,
				EditorialLayer: raw.Provenance.EditorialLayer,
				Fidelity:       defaultFidelity(raw.Provenance.BaseRelation),
			}, nil
		}
	}

	return MatchResult{
		Lookup: types.SourceLookupState{
			Status:     types.SourceLookupStatusNotFound,
			ResolvedBy: "deterministic_judge",
			MatchKind:  types.SourceMatchUnrelated,
		},
	}, nil
}

func concreteCandidates(candidates []types.SourceCandidate) []types.SourceCandidate {
	concrete := make([]types.SourceCandidate, 0, len(candidates))
	for _, candidate := range candidates {
		if strings.HasPrefix(candidate.URL, "https://") || strings.HasPrefix(candidate.URL, "http://") {
			concrete = append(concrete, candidate)
		}
	}
	return concrete
}

func sameCanonicalTarget(left, right string) bool {
	left = strings.TrimSuffix(strings.TrimSpace(left), "/")
	right = strings.TrimSuffix(strings.TrimSpace(right), "/")
	return left != "" && left == right
}

func isSameSourceCandidate(rawURL, candidateURL string) bool {
	rawHost := hostFromURL(rawURL)
	candidateHost := hostFromURL(candidateURL)
	if rawHost == "" || candidateHost == "" || rawHost != candidateHost {
		return false
	}
	rawLower := strings.ToLower(rawURL)
	candidateLower := strings.ToLower(candidateURL)
	if strings.Contains(candidateLower, "/channel/") || strings.Contains(candidateLower, "/playlist") || strings.Contains(candidateLower, "/join") {
		return false
	}
	return sameCanonicalTarget(rawLower, candidateLower)
}

func isCrossPlatformCandidate(rawHost, candidateHost string) bool {
	return rawHost != "" && candidateHost != "" && rawHost != candidateHost
}

func isHighConfidenceEmbeddedLink(candidate types.SourceCandidate) bool {
	return candidate.Kind == "source_link" && candidate.Confidence == string(types.ConfidenceHigh)
}

func isExplicitRelationCandidate(candidate types.SourceCandidate) bool {
	switch candidate.Kind {
	case "native_quote", "native_repost":
		return true
	default:
		return false
	}
}

func indicatesDerivation(raw types.RawContent) bool {
	if raw.Provenance == nil {
		return false
	}
	if raw.Provenance.BaseRelation != "" && raw.Provenance.BaseRelation != types.BaseRelationUnknown {
		return true
	}
	return raw.Provenance.EditorialLayer != "" && raw.Provenance.EditorialLayer != types.EditorialLayerUnknown
}

func defaultFidelity(relation types.BaseRelation) types.Fidelity {
	switch relation {
	case types.BaseRelationTranslation, types.BaseRelationExcerpt, types.BaseRelationSummary, types.BaseRelationCompilation, types.BaseRelationInterviewRecut:
		return types.FidelityLikelyAdapted
	default:
		return ""
	}
}
