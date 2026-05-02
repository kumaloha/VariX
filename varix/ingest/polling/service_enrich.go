package polling

import (
	"context"
	"github.com/kumaloha/VariX/varix/ingest/types"
	"strings"
)

func (s *Service) annotate(items []types.RawContent) []types.RawContent {
	if s.enricher == nil {
		return items
	}
	return s.enricher.Annotate(items)
}

func (s *Service) localize(ctx context.Context, items []types.RawContent) []types.RawContent {
	if s.localizer == nil {
		return items
	}
	return s.localizer.Localize(ctx, items)
}

func (s *Service) hydrateReferences(ctx context.Context, items []types.RawContent) []types.RawContent {
	if s.dispatcher == nil || len(items) == 0 {
		return items
	}
	out := make([]types.RawContent, 0, len(items))
	for _, item := range items {
		item.References = s.hydrateItemReferences(ctx, item.References)
		out = append(out, item)
	}
	return out
}

func (s *Service) hydrateItemReferences(ctx context.Context, refs []types.Reference) []types.Reference {
	if s.dispatcher == nil || len(refs) == 0 {
		return refs
	}
	out := make([]types.Reference, 0, len(refs))
	for _, ref := range refs {
		out = append(out, s.hydrateReference(ctx, ref))
	}
	return out
}

func (s *Service) hydrateReference(ctx context.Context, ref types.Reference) types.Reference {
	if strings.TrimSpace(ref.URL) == "" {
		return ref
	}
	parsed, err := s.dispatcher.ParseURL(ctx, ref.URL)
	if err != nil {
		return ref
	}
	items, err := s.dispatcher.FetchByParsedURL(ctx, parsed)
	if err != nil || len(items) == 0 {
		return ref
	}
	nested := items[0]
	if strings.TrimSpace(nested.Source) != "" {
		ref.Source = nested.Source
	}
	if strings.TrimSpace(ref.Platform) == "" {
		ref.Platform = nested.Source
	}
	if strings.TrimSpace(nested.ExternalID) != "" {
		ref.ExternalID = nested.ExternalID
	}
	if strings.TrimSpace(nested.Content) != "" {
		ref.Content = nested.Content
	}
	if strings.TrimSpace(nested.AuthorName) != "" {
		ref.AuthorName = nested.AuthorName
	}
	if strings.TrimSpace(nested.AuthorID) != "" {
		ref.AuthorID = nested.AuthorID
	}
	if strings.TrimSpace(nested.URL) != "" {
		ref.URL = nested.URL
	}
	if !nested.PostedAt.IsZero() {
		ref.PostedAt = nested.PostedAt
	}
	if len(nested.Attachments) > 0 {
		ref.Attachments = nested.Attachments
	}
	ref.QuoteURLs = collectQuoteURLs(nested.Quotes)
	ref.ReferenceURLs = collectReferenceURLs(nested.References)
	return ref
}

func collectQuoteURLs(quotes []types.Quote) []string {
	if len(quotes) == 0 {
		return nil
	}
	out := make([]string, 0, len(quotes))
	seen := make(map[string]struct{}, len(quotes))
	for _, quote := range quotes {
		url := strings.TrimSpace(quote.URL)
		if url == "" {
			continue
		}
		if _, ok := seen[url]; ok {
			continue
		}
		seen[url] = struct{}{}
		out = append(out, url)
	}
	return out
}

func collectReferenceURLs(refs []types.Reference) []string {
	if len(refs) == 0 {
		return nil
	}
	out := make([]string, 0, len(refs))
	seen := make(map[string]struct{}, len(refs))
	for _, ref := range refs {
		url := strings.TrimSpace(ref.URL)
		if url == "" {
			continue
		}
		if _, ok := seen[url]; ok {
			continue
		}
		seen[url] = struct{}{}
		out = append(out, url)
	}
	return out
}
