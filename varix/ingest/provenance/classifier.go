package provenance

import (
	"strings"

	"github.com/kumaloha/VariX/varix/ingest/types"
)

func Classify(raw types.RawContent) types.Provenance {
	candidates := structuredSourceCandidates(raw)
	baseRelation, relationEvidence := inferStructuredRelation(raw)
	confidence := inferProvenanceConfidence(baseRelation, len(candidates) > 0)
	status := types.SourceLookupStatusNotNeeded
	needsLookup := false
	if len(candidates) > 0 {
		status = types.SourceLookupStatusPending
		needsLookup = true
	}

	evidence := append([]types.ProvenanceEvidence(nil), relationEvidence...)
	for _, candidate := range candidates {
		evidence = append(evidence, types.ProvenanceEvidence{
			Kind:   candidate.Kind,
			Value:  candidate.URL,
			Weight: candidate.Confidence,
		})
	}

	return types.Provenance{
		BaseRelation:      baseRelation,
		EditorialLayer:    types.EditorialLayerUnknown,
		Confidence:        confidence,
		NeedsSourceLookup: needsLookup,
		SourceCandidates:  candidates,
		Evidence:          evidence,
		SourceLookup: types.SourceLookupState{
			Status: status,
		},
	}
}

func structuredSourceCandidates(raw types.RawContent) []types.SourceCandidate {
	var candidates []types.SourceCandidate

	appendCandidate := func(url string, kind string, confidence types.Confidence) {
		url = strings.TrimSpace(url)
		if url == "" {
			return
		}
		candidates = mergeCandidates(candidates, []types.SourceCandidate{{
			URL:        url,
			Host:       hostFromURL(url),
			Kind:       kind,
			Confidence: string(confidence),
		}})
	}

	for _, link := range sourceLinks(raw) {
		appendCandidate(link, "source_link", types.ConfidenceHigh)
	}
	for _, quote := range raw.Quotes {
		kind := structuredQuoteCandidateKind(quote.Relation)
		if kind == "" {
			continue
		}
		appendCandidate(quote.URL, kind, types.ConfidenceHigh)
	}
	for _, reference := range raw.References {
		appendCandidate(reference.URL, "reference_link", types.ConfidenceMedium)
	}

	return candidates
}

func inferStructuredRelation(raw types.RawContent) (types.BaseRelation, []types.ProvenanceEvidence) {
	if raw.Metadata.Weibo != nil && raw.Metadata.Weibo.IsRepost && strings.TrimSpace(raw.Metadata.Weibo.OriginalURL) != "" {
		return types.BaseRelationRepost, []types.ProvenanceEvidence{{
			Kind:   "native_repost",
			Value:  raw.Metadata.Weibo.OriginalURL,
			Weight: string(types.ConfidenceHigh),
		}}
	}
	for _, quote := range raw.Quotes {
		switch structuredQuoteCandidateKind(quote.Relation) {
		case "native_repost":
			return types.BaseRelationRepost, []types.ProvenanceEvidence{{
				Kind:   "native_repost",
				Value:  quote.URL,
				Weight: string(types.ConfidenceHigh),
			}}
		case "native_quote":
			return types.BaseRelationQuote, []types.ProvenanceEvidence{{
				Kind:   "native_quote",
				Value:  quote.URL,
				Weight: string(types.ConfidenceHigh),
			}}
		}
	}
	return types.BaseRelationUnknown, nil
}

func structuredQuoteCandidateKind(relation string) string {
	switch strings.ToLower(strings.TrimSpace(relation)) {
	case "repost":
		return "native_repost"
	case "quote", "quote_tweet":
		return "native_quote"
	default:
		return ""
	}
}

func inferProvenanceConfidence(baseRelation types.BaseRelation, hasCandidates bool) types.Confidence {
	if baseRelation != "" && baseRelation != types.BaseRelationUnknown {
		return types.ConfidenceHigh
	}
	if hasCandidates {
		return types.ConfidenceMedium
	}
	return types.ConfidenceLow
}

func hostFromURL(raw string) string {
	raw = strings.TrimSpace(raw)
	raw = strings.TrimPrefix(raw, "https://")
	raw = strings.TrimPrefix(raw, "http://")
	if raw == "" {
		return ""
	}
	if idx := strings.IndexRune(raw, '/'); idx >= 0 {
		raw = raw[:idx]
	}
	return raw
}
