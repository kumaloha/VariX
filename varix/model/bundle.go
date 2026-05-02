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
