package assemble

import (
	"testing"
	"time"

	"github.com/kumaloha/VariX/varix/ingest/types"
)

func TestFormatQuotePlaceholder_Full(t *testing.T) {
	got := FormatQuotePlaceholder(1, types.Quote{
		AuthorName: "张三",
		PostedAt:   time.Date(2026, 4, 10, 15, 30, 0, 0, time.UTC),
	})
	want := "[引用#1 @张三 · 2026-04-10]"
	if got != want {
		t.Fatalf("FormatQuotePlaceholder() = %q, want %q", got, want)
	}
}

func TestFormatQuotePlaceholder_WithoutMetadata(t *testing.T) {
	got := FormatQuotePlaceholder(2, types.Quote{})
	want := "[引用#2]"
	if got != want {
		t.Fatalf("FormatQuotePlaceholder() = %q, want %q", got, want)
	}
}

func TestFormatAttachmentPlaceholder_ByType(t *testing.T) {
	tests := []struct {
		name       string
		index      int
		attachment types.Attachment
		want       string
	}{
		{name: "video", index: 1, attachment: types.Attachment{Type: "video"}, want: "[附件#1 视频]"},
		{name: "image", index: 2, attachment: types.Attachment{Type: "image"}, want: "[附件#2 图片]"},
		{name: "unknown", index: 3, attachment: types.Attachment{Type: "pdf"}, want: "[附件#3 附件]"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := FormatAttachmentPlaceholder(tt.index, tt.attachment); got != tt.want {
				t.Fatalf("FormatAttachmentPlaceholder() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestFormatReferencePlaceholder(t *testing.T) {
	got := FormatReferencePlaceholder(1, types.Reference{Platform: "twitter"})
	want := "[参考#1 X帖子]"
	if got != want {
		t.Fatalf("FormatReferencePlaceholder() = %q, want %q", got, want)
	}
}

func TestAssembleStructuredContent(t *testing.T) {
	got := AssembleStructuredContent(
		"main text",
		[]types.Quote{{AuthorName: "a", PostedAt: time.Date(2026, 4, 10, 0, 0, 0, 0, time.UTC)}},
		[]types.Reference{{Platform: "twitter"}},
		[]types.Attachment{{Type: "video"}, {Type: "image"}},
	)
	want := "main text\n\n[引用#1 @a · 2026-04-10]\n\n[参考#1 X帖子]\n\n[附件#1 视频]\n\n[附件#2 图片]"
	if got != want {
		t.Fatalf("AssembleStructuredContent() = %q, want %q", got, want)
	}
}

func TestAssembleContent_WhitespaceHandling(t *testing.T) {
	got := AssembleContent("  main  ", []string{"  block  ", "", "   "})
	if got != "main\n\nblock" {
		t.Fatalf("AssembleContent() = %q, want %q", got, "main\n\nblock")
	}
}
