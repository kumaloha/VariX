package provenance

import (
	"strings"

	"github.com/kumaloha/VariX/varix/ingest/types"
)

var (
	translationMarkers = []string{"中字", "翻译", "translated", "translation", "熟肉"}
	commentaryMarkers  = []string{"解读", "点评", "评论", "analysis", "commentary", "reaction", "怎么看"}
	summaryMarkers     = []string{"总结", "梳理", "要点", "summary"}
	excerptMarkers     = []string{"节选", "精华", "剪辑", "highlights", "excerpt"}
)

func Classify(raw types.RawContent) types.Provenance {
	title, description, links := sourceEvidence(raw)
	content := raw.ExpandedText()

	hasTranslation := hasAnyMarker(title, description, content, translationMarkers)
	hasCommentary := hasAnyMarker(title, description, content, commentaryMarkers)
	hasSummary := hasAnyMarker(title, description, content, summaryMarkers)
	hasExcerpt := hasAnyMarker(title, description, content, excerptMarkers)
	hasLinks := len(links) > 0

	prov := types.Provenance{
		BaseRelation:      types.BaseRelationUnknown,
		EditorialLayer:    types.EditorialLayerUnknown,
		Confidence:        types.ConfidenceLow,
		NeedsSourceLookup: true,
		SourceLookup: types.SourceLookupState{
			Status: types.SourceLookupStatusPending,
		},
		SourceCandidates: sourceCandidates(links),
	}

	switch {
	case hasTranslation:
		prov.BaseRelation = types.BaseRelationTranslation
		prov.EditorialLayer = types.EditorialLayerNone
		prov.Confidence = types.ConfidenceMedium
		prov.Evidence = append(prov.Evidence, markerEvidence("translation", firstMatchedMarker(title, description, content, translationMarkers), "strong"))
	case hasExcerpt:
		prov.BaseRelation = types.BaseRelationExcerpt
		prov.EditorialLayer = types.EditorialLayerNone
		prov.Confidence = types.ConfidenceMedium
		prov.Evidence = append(prov.Evidence, markerEvidence("excerpt", firstMatchedMarker(title, description, content, excerptMarkers), "strong"))
	case hasSummary:
		prov.BaseRelation = types.BaseRelationSummary
		prov.EditorialLayer = types.EditorialLayerNone
		prov.Confidence = types.ConfidenceMedium
		prov.Evidence = append(prov.Evidence, markerEvidence("summary", firstMatchedMarker(title, description, content, summaryMarkers), "strong"))
	}

	if hasCommentary {
		prov.EditorialLayer = types.EditorialLayerCommentary
		prov.Evidence = append(prov.Evidence, markerEvidence("commentary", firstMatchedMarker(title, description, content, commentaryMarkers), "strong"))
	}

	if hasLinks {
		prov.Evidence = append(prov.Evidence, types.ProvenanceEvidence{
			Kind:   "source_link",
			Value:  links[0],
			Weight: "strong",
		})
		if prov.BaseRelation != types.BaseRelationUnknown {
			prov.Confidence = types.ConfidenceHigh
		}
	}

	if prov.BaseRelation == types.BaseRelationUnknown && prov.EditorialLayer == types.EditorialLayerCommentary {
		prov.EditorialLayer = types.EditorialLayerUnknown
	}

	return prov
}

func sourceEvidence(raw types.RawContent) (title string, description string, links []string) {
	switch {
	case raw.Metadata.YouTube != nil:
		return raw.Metadata.YouTube.Title, raw.Metadata.YouTube.Description, raw.Metadata.YouTube.SourceLinks
	case raw.Metadata.Bilibili != nil:
		return raw.Metadata.Bilibili.Title, raw.Metadata.Bilibili.Description, raw.Metadata.Bilibili.SourceLinks
	case raw.Metadata.Twitter != nil:
		return "", "", raw.Metadata.Twitter.SourceLinks
	case raw.Metadata.Web != nil:
		links := make([]string, 0, 1)
		if raw.Metadata.Web.YouTubeRedirect != "" {
			links = append(links, raw.Metadata.Web.YouTubeRedirect)
		}
		return raw.Metadata.Web.Title, "", links
	default:
		return "", "", nil
	}
}

func hasAnyMarker(values ...any) bool {
	if len(values) == 0 {
		return false
	}
	parts := make([]string, 0, len(values)-1)
	var markers []string
	for i, value := range values {
		if i == len(values)-1 {
			if typed, ok := value.([]string); ok {
				markers = typed
			}
			continue
		}
		if typed, ok := value.(string); ok {
			parts = append(parts, strings.ToLower(typed))
		}
	}
	for _, marker := range markers {
		marker = strings.ToLower(marker)
		for _, part := range parts {
			if strings.Contains(part, marker) {
				return true
			}
		}
	}
	return false
}

func firstMatchedMarker(title, description, content string, markers []string) string {
	parts := []string{
		strings.ToLower(title),
		strings.ToLower(description),
		strings.ToLower(content),
	}
	for _, marker := range markers {
		lower := strings.ToLower(marker)
		for _, part := range parts {
			if strings.Contains(part, lower) {
				return marker
			}
		}
	}
	return ""
}

func markerEvidence(kind, marker, weight string) types.ProvenanceEvidence {
	return types.ProvenanceEvidence{
		Kind:   kind,
		Value:  marker,
		Weight: weight,
	}
}

func sourceCandidates(links []string) []types.SourceCandidate {
	if len(links) == 0 {
		return nil
	}
	out := make([]types.SourceCandidate, 0, len(links))
	for _, link := range links {
		out = append(out, types.SourceCandidate{
			URL:        link,
			Host:       hostFromURL(link),
			Kind:       "embedded_link",
			Confidence: string(types.ConfidenceHigh),
		})
	}
	return out
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
