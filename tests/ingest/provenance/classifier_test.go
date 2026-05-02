package provenance

import (
	"testing"

	"github.com/kumaloha/VariX/varix/ingest/types"
)

func TestClassifier_StructuredSourceLinksStayDeterministic(t *testing.T) {
	raw := types.RawContent{
		Source:     "youtube",
		Content:    "巴菲特访谈",
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
	if got.BaseRelation != types.BaseRelationUnknown {
		t.Fatalf("BaseRelation = %q, want unknown", got.BaseRelation)
	}
	if got.EditorialLayer != types.EditorialLayerUnknown {
		t.Fatalf("EditorialLayer = %q, want unknown", got.EditorialLayer)
	}
	if got.Confidence != types.ConfidenceMedium {
		t.Fatalf("Confidence = %q, want medium", got.Confidence)
	}
	if !got.NeedsSourceLookup {
		t.Fatal("NeedsSourceLookup = false, want true")
	}
	if got.SourceLookup.Status != types.SourceLookupStatusPending {
		t.Fatalf("SourceLookup.Status = %q, want pending", got.SourceLookup.Status)
	}
	if len(got.SourceCandidates) != 1 || got.SourceCandidates[0].URL != "https://www.cnbc.com/interview" {
		t.Fatalf("SourceCandidates = %#v", got.SourceCandidates)
	}
	if got.SourceCandidates[0].Kind != "source_link" {
		t.Fatalf("SourceCandidates[0].Kind = %q, want source_link", got.SourceCandidates[0].Kind)
	}
}

func TestClassifier_NoStructuredSourceStaysNotNeeded(t *testing.T) {
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
	if got.BaseRelation != types.BaseRelationUnknown {
		t.Fatalf("BaseRelation = %q, want unknown", got.BaseRelation)
	}
	if got.Confidence != types.ConfidenceLow {
		t.Fatalf("Confidence = %q, want low", got.Confidence)
	}
	if got.NeedsSourceLookup {
		t.Fatal("NeedsSourceLookup = true, want false")
	}
	if got.SourceLookup.Status != types.SourceLookupStatusNotNeeded {
		t.Fatalf("SourceLookup.Status = %q, want not_needed", got.SourceLookup.Status)
	}
}

func TestClassifier_WeiboRepostMarksNativeRelation(t *testing.T) {
	raw := types.RawContent{
		Source:  "weibo",
		Content: "转发微博",
		URL:     "https://weibo.com/111/repost_bid",
		Metadata: types.RawMetadata{
			Weibo: &types.WeiboMetadata{
				IsRepost:    true,
				OriginalURL: "https://weibo.com/222/original_bid",
			},
		},
	}

	got := Classify(raw)
	if got.BaseRelation != types.BaseRelationRepost {
		t.Fatalf("BaseRelation = %q, want repost", got.BaseRelation)
	}
	if got.Confidence != types.ConfidenceHigh {
		t.Fatalf("Confidence = %q, want high", got.Confidence)
	}
	if !got.NeedsSourceLookup {
		t.Fatal("NeedsSourceLookup = false, want true")
	}
	if len(got.SourceCandidates) != 1 {
		t.Fatalf("SourceCandidates = %#v, want 1", got.SourceCandidates)
	}
	if got.SourceCandidates[0].Kind != "source_link" {
		t.Fatalf("SourceCandidates[0].Kind = %q, want source_link", got.SourceCandidates[0].Kind)
	}
	if got.Evidence[0].Kind != "native_repost" {
		t.Fatalf("Evidence[0] = %#v, want native_repost evidence", got.Evidence[0])
	}
}

func TestClassifier_QuoteTweetMarksNativeQuote(t *testing.T) {
	raw := types.RawContent{
		Source:  "twitter",
		Content: "主帖正文",
		URL:     "https://x.com/alice/status/123",
		Quotes: []types.Quote{{
			Relation: "quote_tweet",
			URL:      "https://x.com/bob/status/456",
		}},
	}

	got := Classify(raw)
	if got.BaseRelation != types.BaseRelationQuote {
		t.Fatalf("BaseRelation = %q, want quote", got.BaseRelation)
	}
	if got.Confidence != types.ConfidenceHigh {
		t.Fatalf("Confidence = %q, want high", got.Confidence)
	}
	if len(got.SourceCandidates) != 1 {
		t.Fatalf("SourceCandidates = %#v, want 1", got.SourceCandidates)
	}
	if got.SourceCandidates[0].Kind != "native_quote" {
		t.Fatalf("SourceCandidates[0].Kind = %q, want native_quote", got.SourceCandidates[0].Kind)
	}
}
