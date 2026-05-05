package compile

import "strings"

const (
	renderPrimaryViewMainline = "mainline"
	renderPrimaryViewDigest   = "digest"
)

func primaryRenderView(state graphState) string {
	if len(renderDigestItems(state)) > 0 {
		return renderPrimaryViewDigest
	}
	return renderPrimaryViewMainline
}

func renderDigestItems(state graphState) []BriefItem {
	if !isReaderInterestSummaryForm(state.ArticleForm) || len(state.Brief) == 0 {
		return nil
	}
	return cloneBriefForDigest(state.Brief)
}

func cloneBriefForDigest(items []BriefItem) []BriefItem {
	out := make([]BriefItem, 0, len(items))
	for _, item := range items {
		if strings.TrimSpace(item.Category) == "" || strings.TrimSpace(item.Claim) == "" {
			continue
		}
		out = append(out, BriefItem{
			ID:        strings.TrimSpace(item.ID),
			Category:  strings.TrimSpace(item.Category),
			Kind:      strings.TrimSpace(item.Kind),
			Claim:     strings.TrimSpace(item.Claim),
			Entities:  append([]string(nil), item.Entities...),
			Numbers:   append([]string(nil), item.Numbers...),
			Quote:     strings.TrimSpace(item.Quote),
			Salience:  item.Salience,
			SourceIDs: append([]string(nil), item.SourceIDs...),
		})
	}
	return out
}
