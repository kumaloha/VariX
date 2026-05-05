package model

import (
	"fmt"
	"strings"
	"time"

	"github.com/kumaloha/VariX/varix/ingest/types"
)

type Bundle struct {
	UnitID          string                `json:"unit_id"`
	Source          string                `json:"source"`
	ExternalID      string                `json:"external_id"`
	RootExternalID  string                `json:"root_external_id"`
	AuthorName      string                `json:"author_name,omitempty"`
	AuthorID        string                `json:"author_id,omitempty"`
	URL             string                `json:"url,omitempty"`
	PostedAt        time.Time             `json:"posted_at,omitempty"`
	Content         string                `json:"content"`
	Quotes          []types.Quote         `json:"quotes,omitempty"`
	References      []types.Reference     `json:"references,omitempty"`
	ThreadSegments  []types.ThreadSegment `json:"thread_segments,omitempty"`
	Attachments     []types.Attachment    `json:"attachments,omitempty"`
	LocalImagePaths []string              `json:"local_image_paths,omitempty"`
}

func BuildBundle(raw types.RawContent) Bundle {
	rootExternalID := raw.ExternalID
	if raw.Metadata.Thread != nil && strings.TrimSpace(raw.Metadata.Thread.RootExternalID) != "" {
		rootExternalID = raw.Metadata.Thread.RootExternalID
	}
	localImagePaths := collectLocalImagePaths(raw)
	if shouldSuppressLocalImages(raw) {
		localImagePaths = nil
	}
	return Bundle{
		UnitID:          fmt.Sprintf("%s:%s", raw.Source, raw.ExternalID),
		Source:          raw.Source,
		ExternalID:      raw.ExternalID,
		RootExternalID:  rootExternalID,
		AuthorName:      raw.AuthorName,
		AuthorID:        raw.AuthorID,
		URL:             raw.URL,
		PostedAt:        raw.PostedAt,
		Content:         raw.Content,
		Quotes:          raw.Quotes,
		References:      raw.References,
		ThreadSegments:  raw.ThreadSegments,
		Attachments:     raw.Attachments,
		LocalImagePaths: localImagePaths,
	}
}

func BuildMergedBundle(primary types.RawContent, includes []types.RawContent) Bundle {
	bundle := BuildBundle(primary)
	seenSources := map[string]struct{}{
		sourceKey(primary.Source, primary.ExternalID): {},
	}
	seenImages := map[string]struct{}{}
	for _, path := range bundle.LocalImagePaths {
		seenImages[path] = struct{}{}
	}
	for _, raw := range includes {
		key := sourceKey(raw.Source, raw.ExternalID)
		if key == "" {
			continue
		}
		if _, ok := seenSources[key]; ok {
			continue
		}
		seenSources[key] = struct{}{}
		bundle.References = append(bundle.References, sourceSetReference(raw))
		for _, path := range collectLocalImagePaths(raw) {
			if _, ok := seenImages[path]; ok {
				continue
			}
			seenImages[path] = struct{}{}
			bundle.LocalImagePaths = append(bundle.LocalImagePaths, path)
		}
	}
	return bundle
}

func sourceSetReference(raw types.RawContent) types.Reference {
	return types.Reference{
		Kind:          "source_set",
		Label:         sourceSetLabel(raw),
		Source:        raw.Source,
		Platform:      raw.Source,
		ExternalID:    raw.ExternalID,
		Content:       sourceSetContent(raw),
		AuthorName:    raw.AuthorName,
		AuthorID:      raw.AuthorID,
		URL:           raw.URL,
		PostedAt:      raw.PostedAt,
		Attachments:   raw.Attachments,
		QuoteURLs:     rawQuoteURLs(raw),
		ReferenceURLs: rawReferenceURLs(raw),
	}
}

func sourceSetLabel(raw types.RawContent) string {
	if source := strings.TrimSpace(raw.Source); source != "" {
		if externalID := strings.TrimSpace(raw.ExternalID); externalID != "" {
			return source + ":" + externalID
		}
		return source
	}
	if url := strings.TrimSpace(raw.URL); url != "" {
		return url
	}
	return "included source"
}

func sourceSetContent(raw types.RawContent) string {
	parts := make([]string, 0, 1+len(raw.ThreadSegments))
	if trimmed := strings.TrimSpace(raw.ExpandedText()); trimmed != "" {
		parts = append(parts, trimmed)
	}
	for i, seg := range raw.ThreadSegments {
		if trimmed := strings.TrimSpace(seg.Content); trimmed != "" {
			position := seg.Position
			if position <= 0 {
				position = i + 1
			}
			parts = append(parts, fmt.Sprintf("[线程正文#%d]\n%s", position, trimmed))
		}
	}
	return strings.Join(parts, "\n\n")
}

func sourceKey(source, externalID string) string {
	source = strings.TrimSpace(source)
	externalID = strings.TrimSpace(externalID)
	if source == "" || externalID == "" {
		return ""
	}
	return source + "\x00" + externalID
}

func rawQuoteURLs(raw types.RawContent) []string {
	return collectUniqueURLs(func(appendURL func(string)) {
		for _, quote := range raw.Quotes {
			appendURL(quote.URL)
		}
		for _, ref := range raw.References {
			for _, url := range ref.QuoteURLs {
				appendURL(url)
			}
		}
	})
}

func rawReferenceURLs(raw types.RawContent) []string {
	return collectUniqueURLs(func(appendURL func(string)) {
		for _, ref := range raw.References {
			appendURL(ref.URL)
			for _, url := range ref.ReferenceURLs {
				appendURL(url)
			}
		}
	})
}

func collectUniqueURLs(visit func(func(string))) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0)
	visit(func(url string) {
		url = strings.TrimSpace(url)
		if url == "" {
			return
		}
		if _, ok := seen[url]; ok {
			return
		}
		seen[url] = struct{}{}
		out = append(out, url)
	})
	return out
}

func shouldSuppressLocalImages(raw types.RawContent) bool {
	if strings.TrimSpace(strings.ToLower(raw.Source)) != "web" {
		return false
	}
	if len(raw.Attachments) < 4 {
		return false
	}
	return len([]rune(strings.TrimSpace(raw.Content))) >= 2000
}

func (b Bundle) ApproxTextLength() int {
	return len([]rune(b.TextContext()))
}

func (b Bundle) TextContext() string {
	sections := make([]string, 0, 5)
	if trimmed := strings.TrimSpace(b.Content); trimmed != "" {
		sections = append(sections, "[ROOT CONTENT]\n"+trimmed)
	}
	if len(b.Quotes) > 0 {
		parts := make([]string, 0, len(b.Quotes))
		for i, quote := range b.Quotes {
			parts = append(parts, fmt.Sprintf("[QUOTE %d]\n%s", i+1, strings.TrimSpace(quote.Content)))
		}
		sections = append(sections, strings.Join(parts, "\n\n"))
	}
	if len(b.References) > 0 {
		parts := make([]string, 0, len(b.References))
		for i, ref := range b.References {
			if strings.EqualFold(strings.TrimSpace(ref.Kind), "source_set") {
				parts = append(parts, formatIncludedSourceSection(i+1, ref))
				continue
			}
			if strings.TrimSpace(ref.Content) == "" {
				parts = append(parts, fmt.Sprintf("[REFERENCE %d URL]\n%s", i+1, strings.TrimSpace(ref.URL)))
				continue
			}
			parts = append(parts, fmt.Sprintf("[REFERENCE %d]\n%s", i+1, strings.TrimSpace(ref.Content)))
		}
		sections = append(sections, strings.Join(parts, "\n\n"))
	}
	if len(b.ThreadSegments) > 0 {
		parts := make([]string, 0, len(b.ThreadSegments))
		for _, seg := range b.ThreadSegments {
			if strings.TrimSpace(seg.Content) == "" {
				continue
			}
			parts = append(parts, fmt.Sprintf("[THREAD %d]\n%s", seg.Position, strings.TrimSpace(seg.Content)))
		}
		if len(parts) > 0 {
			sections = append(sections, strings.Join(parts, "\n\n"))
		}
	}
	if transcripts := attachmentTranscriptSection(b.Attachments); transcripts != "" {
		sections = append(sections, transcripts)
	}
	return strings.Join(sections, "\n\n")
}

func formatIncludedSourceSection(index int, ref types.Reference) string {
	label := strings.TrimSpace(ref.Label)
	if label == "" {
		label = strings.TrimSpace(ref.Source)
	}
	if label != "" {
		label = " " + label
	}
	if content := strings.TrimSpace(ref.Content); content != "" {
		return fmt.Sprintf("[INCLUDED SOURCE %d%s]\n%s", index, label, content)
	}
	return fmt.Sprintf("[INCLUDED SOURCE %d%s URL]\n%s", index, label, strings.TrimSpace(ref.URL))
}

func collectLocalImagePaths(raw types.RawContent) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0)
	appendPaths := func(attachments []types.Attachment) {
		for _, att := range attachments {
			if strings.ToLower(strings.TrimSpace(att.Type)) != "image" {
				continue
			}
			path := strings.TrimSpace(att.StoredPath)
			if path == "" {
				continue
			}
			if _, ok := seen[path]; ok {
				continue
			}
			seen[path] = struct{}{}
			out = append(out, path)
		}
	}
	appendPaths(raw.Attachments)
	for _, ref := range raw.References {
		appendPaths(ref.Attachments)
	}
	for _, seg := range raw.ThreadSegments {
		appendPaths(seg.Attachments)
	}
	return out
}

func attachmentTranscriptSection(attachments []types.Attachment) string {
	parts := make([]string, 0)
	for i, att := range attachments {
		if strings.TrimSpace(att.Transcript) == "" {
			continue
		}
		parts = append(parts, fmt.Sprintf("[ATTACHMENT TRANSCRIPT %d]\n%s", i+1, strings.TrimSpace(att.Transcript)))
	}
	return strings.Join(parts, "\n\n")
}
