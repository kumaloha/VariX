package provenance

import "github.com/kumaloha/VariX/varix/ingest/types"

type Enricher struct{}

func (Enricher) Annotate(items []types.RawContent) []types.RawContent {
	out := make([]types.RawContent, 0, len(items))
	for _, item := range items {
		if item.Provenance == nil {
			prov := Classify(item)
			item.Provenance = &prov
		}
		out = append(out, item)
	}
	return out
}
