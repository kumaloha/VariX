// Package assemble provides helpers for formatting compact quote/attachment
// placeholders into RawContent.Content while leaving full detail in structured fields.
package assemble

import (
	"fmt"
	"strings"

	"github.com/kumaloha/VariX/varix/ingest/types"
)

// FormatQuotePlaceholder returns a compact quote placeholder that points to
// the corresponding entry in RawContent.Quotes.
func FormatQuotePlaceholder(index int, quote types.Quote) string {
	header := fmt.Sprintf("[引用#%d", index)
	if authorName := strings.TrimSpace(quote.AuthorName); authorName != "" {
		header += " @" + authorName
	}
	if !quote.PostedAt.IsZero() {
		header += " · " + quote.PostedAt.UTC().Format("2006-01-02")
	}
	return header + "]"
}

// FormatAttachmentPlaceholder returns a compact attachment placeholder that
// points to the corresponding entry in RawContent.Attachments.
func FormatAttachmentPlaceholder(index int, attachment types.Attachment) string {
	label := attachmentPlaceholderLabel(attachment.Type)
	if label == "" {
		return fmt.Sprintf("[附件#%d]", index)
	}
	return fmt.Sprintf("[附件#%d %s]", index, label)
}

// FormatReferencePlaceholder returns a compact reference placeholder that
// points to the corresponding entry in RawContent.References.
func FormatReferencePlaceholder(index int, reference types.Reference) string {
	label := strings.TrimSpace(reference.Label)
	if label == "" {
		label = referencePlaceholderLabel(reference)
	}
	return fmt.Sprintf("[参考#%d %s]", index, label)
}

// AssembleStructuredContent joins main text with compact quote and attachment
// placeholders. Full quote bodies/transcripts remain in Quotes/Attachments.
func AssembleStructuredContent(mainText string, quotes []types.Quote, references []types.Reference, attachments []types.Attachment) string {
	blocks := make([]string, 0, len(quotes)+len(references)+len(attachments))
	for i, quote := range quotes {
		blocks = append(blocks, FormatQuotePlaceholder(i+1, quote))
	}
	for i, reference := range references {
		blocks = append(blocks, FormatReferencePlaceholder(i+1, reference))
	}
	for i, attachment := range attachments {
		blocks = append(blocks, FormatAttachmentPlaceholder(i+1, attachment))
	}
	return AssembleContent(mainText, blocks)
}

// AssembleContent joins a main text body with zero or more formatted blocks,
// separated by blank lines. Leading and trailing whitespace on each part is
// trimmed, and empty parts are skipped.
func AssembleContent(mainText string, blocks []string) string {
	parts := make([]string, 0, 1+len(blocks))
	if trimmed := strings.TrimSpace(mainText); trimmed != "" {
		parts = append(parts, trimmed)
	}
	for _, b := range blocks {
		if trimmed := strings.TrimSpace(b); trimmed != "" {
			parts = append(parts, trimmed)
		}
	}
	return strings.Join(parts, "\n\n")
}

func attachmentPlaceholderLabel(rawType string) string {
	switch strings.ToLower(strings.TrimSpace(rawType)) {
	case "image", "photo":
		return "图片"
	case "video":
		return "视频"
	case "audio":
		return "音频"
	default:
		return "附件"
	}
}

func referencePlaceholderLabel(reference types.Reference) string {
	switch strings.ToLower(strings.TrimSpace(reference.Platform)) {
	case "twitter":
		return "X帖子"
	case "weibo":
		return "微博"
	case "youtube":
		return "YouTube"
	case "bilibili":
		return "B站"
	case "web":
		return "网页"
	default:
		if strings.TrimSpace(reference.Kind) == "post_link" {
			return "帖子"
		}
		return "链接"
	}
}
