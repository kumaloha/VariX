package provenance

import (
	"testing"

	"github.com/kumaloha/VariX/varix/ingest/types"
)

func TestClassifier_TranslationCommentaryHigh(t *testing.T) {
	raw := types.RawContent{
		Source:     "youtube",
		Content:    "翻译整理内容",
		AuthorName: "channel",
		URL:        "https://www.youtube.com/watch?v=abc123",
		Metadata: types.RawMetadata{
			YouTube: &types.YouTubeMetadata{
				Title:       "巴菲特访谈中字解读",
				Description: "原视频：https://www.cnbc.com/interview",
				SourceLinks: []string{"https://www.cnbc.com/interview"},
			},
		},
	}

	got := Classify(raw)
	if got.BaseRelation != types.BaseRelationTranslation {
		t.Fatalf("BaseRelation = %q, want %q", got.BaseRelation, types.BaseRelationTranslation)
	}
	if got.EditorialLayer != types.EditorialLayerCommentary {
		t.Fatalf("EditorialLayer = %q, want %q", got.EditorialLayer, types.EditorialLayerCommentary)
	}
	if got.Confidence != types.ConfidenceHigh {
		t.Fatalf("Confidence = %q, want %q", got.Confidence, types.ConfidenceHigh)
	}
	if !got.NeedsSourceLookup {
		t.Fatal("NeedsSourceLookup = false, want true")
	}
	if got.SourceLookup.Status != types.SourceLookupStatusPending {
		t.Fatalf("SourceLookup.Status = %q, want %q", got.SourceLookup.Status, types.SourceLookupStatusPending)
	}
	if len(got.SourceCandidates) != 1 || got.SourceCandidates[0].URL != "https://www.cnbc.com/interview" {
		t.Fatalf("SourceCandidates = %#v", got.SourceCandidates)
	}
}

func TestClassifier_TranslationMediumWithoutSourceLink(t *testing.T) {
	raw := types.RawContent{
		Source:     "bilibili",
		Content:    "翻译整理内容",
		AuthorName: "channel",
		URL:        "https://www.bilibili.com/video/BV1ABCDEF123",
		Metadata: types.RawMetadata{
			Bilibili: &types.BilibiliMetadata{
				Title:       "巴菲特访谈翻译整理",
				Description: "熟肉版本",
			},
		},
	}

	got := Classify(raw)
	if got.BaseRelation != types.BaseRelationTranslation {
		t.Fatalf("BaseRelation = %q, want %q", got.BaseRelation, types.BaseRelationTranslation)
	}
	if got.EditorialLayer != types.EditorialLayerNone {
		t.Fatalf("EditorialLayer = %q, want %q", got.EditorialLayer, types.EditorialLayerNone)
	}
	if got.Confidence != types.ConfidenceMedium {
		t.Fatalf("Confidence = %q, want %q", got.Confidence, types.ConfidenceMedium)
	}
	if !got.NeedsSourceLookup {
		t.Fatal("NeedsSourceLookup = false, want true")
	}
}

func TestClassifier_WeakSignalsRemainUnknownLow(t *testing.T) {
	raw := types.RawContent{
		Source:     "youtube",
		Content:    "这是一个访谈。",
		AuthorName: "channel",
		URL:        "https://www.youtube.com/watch?v=abc123",
		Metadata: types.RawMetadata{
			YouTube: &types.YouTubeMetadata{
				Title:       "巴菲特访谈",
				Description: "访谈节目录制",
			},
		},
	}

	got := Classify(raw)
	if got.BaseRelation != types.BaseRelationUnknown {
		t.Fatalf("BaseRelation = %q, want %q", got.BaseRelation, types.BaseRelationUnknown)
	}
	if got.EditorialLayer != types.EditorialLayerUnknown {
		t.Fatalf("EditorialLayer = %q, want %q", got.EditorialLayer, types.EditorialLayerUnknown)
	}
	if got.Confidence != types.ConfidenceLow {
		t.Fatalf("Confidence = %q, want %q", got.Confidence, types.ConfidenceLow)
	}
	if !got.NeedsSourceLookup {
		t.Fatal("NeedsSourceLookup = false, want true")
	}
	if got.SourceLookup.Status != types.SourceLookupStatusPending {
		t.Fatalf("SourceLookup.Status = %q, want %q", got.SourceLookup.Status, types.SourceLookupStatusPending)
	}
}

func TestClassifier_UsesExpandedQuoteAndTranscriptText(t *testing.T) {
	raw := types.RawContent{
		Source:  "weibo",
		Content: "主帖正文\n\n[引用#1 @来源]\n\n[附件#1 视频]",
		URL:     "https://weibo.com/123/abc",
		Quotes: []types.Quote{{
			Content: "这是翻译整理的引用内容",
		}},
		Attachments: []types.Attachment{{
			Type:       "video",
			Transcript: "这里是 commentary 分析补充",
		}},
	}

	got := Classify(raw)
	if got.BaseRelation != types.BaseRelationTranslation {
		t.Fatalf("BaseRelation = %q, want %q", got.BaseRelation, types.BaseRelationTranslation)
	}
	if got.EditorialLayer != types.EditorialLayerCommentary {
		t.Fatalf("EditorialLayer = %q, want %q", got.EditorialLayer, types.EditorialLayerCommentary)
	}
}
