package types

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestRawContent_ProvenanceJSONRoundTrip(t *testing.T) {
	raw := RawContent{
		Source:     "youtube",
		ExternalID: "abc123",
		Content:    "translated excerpt",
		AuthorName: "channel-name",
		URL:        "https://www.youtube.com/watch?v=abc123",
		Provenance: &Provenance{
			BaseRelation:      BaseRelationTranslation,
			EditorialLayer:    EditorialLayerCommentary,
			Confidence:        ConfidenceHigh,
			NeedsSourceLookup: true,
			ClaimedSpeakers:   []string{"Warren Buffett"},
			SourceLookup: SourceLookupState{
				Status: SourceLookupStatusPending,
			},
		},
	}

	payload, err := json.Marshal(raw)
	if err != nil {
		t.Fatal(err)
	}

	var decoded RawContent
	if err := json.Unmarshal(payload, &decoded); err != nil {
		t.Fatal(err)
	}

	if decoded.Provenance == nil {
		t.Fatal("Provenance is nil")
	}
	if decoded.Provenance.BaseRelation != BaseRelationTranslation {
		t.Fatalf("BaseRelation = %q, want %q", decoded.Provenance.BaseRelation, BaseRelationTranslation)
	}
	if decoded.Provenance.EditorialLayer != EditorialLayerCommentary {
		t.Fatalf("EditorialLayer = %q, want %q", decoded.Provenance.EditorialLayer, EditorialLayerCommentary)
	}
	if decoded.Provenance.Confidence != ConfidenceHigh {
		t.Fatalf("Confidence = %q, want %q", decoded.Provenance.Confidence, ConfidenceHigh)
	}
	if decoded.AuthorName != "channel-name" {
		t.Fatalf("AuthorName = %q, want %q", decoded.AuthorName, "channel-name")
	}
}

func TestRawContent_ZeroValueProvenanceOmittedFromJSON(t *testing.T) {
	raw := RawContent{
		Source:     "web",
		ExternalID: "abc123",
		Content:    "body",
		AuthorName: "publisher",
		URL:        "https://example.com/post",
	}

	payload, err := json.Marshal(raw)
	if err != nil {
		t.Fatal(err)
	}

	if strings.Contains(string(payload), `"provenance"`) {
		t.Fatalf("zero-value provenance should be omitted, got %s", payload)
	}
}

func TestRawContent_QuotesAndAttachmentsJSONRoundTrip(t *testing.T) {
	posted := time.Date(2026, 4, 10, 12, 0, 0, 0, time.UTC)
	raw := RawContent{
		Source:     "weibo",
		ExternalID: "weibo-123",
		Content:    "main text",
		AuthorName: "test-user",
		URL:        "https://weibo.com/detail/123",
		PostedAt:   posted,
		Quotes: []Quote{
			{
				Relation:   "repost",
				AuthorName: "original-author",
				AuthorID:   "uid-456",
				Platform:   "weibo",
				ExternalID: "weibo-original",
				URL:        "https://weibo.com/detail/original",
				Content:    "original content",
				PostedAt:   posted.Add(-24 * time.Hour),
			},
		},
		Attachments: []Attachment{
			{
				Type: "photo",
				URL:  "https://cdn.example.com/img.jpg",
			},
			{
				Type:       "video",
				URL:        "https://cdn.example.com/vid.mp4",
				Transcript: "hello world",
				Method:     "whisper",
			},
		},
		Metadata: RawMetadata{
			Weibo: &WeiboMetadata{
				IsRepost:    true,
				OriginalURL: "https://weibo.com/detail/original",
			},
		},
	}

	payload, err := json.Marshal(raw)
	if err != nil {
		t.Fatal(err)
	}

	var decoded RawContent
	if err := json.Unmarshal(payload, &decoded); err != nil {
		t.Fatal(err)
	}

	// Verify quotes
	if len(decoded.Quotes) != 1 {
		t.Fatalf("len(Quotes) = %d, want 1", len(decoded.Quotes))
	}
	q := decoded.Quotes[0]
	if q.Relation != "repost" {
		t.Errorf("Quote.Relation = %q, want %q", q.Relation, "repost")
	}
	if q.AuthorName != "original-author" {
		t.Errorf("Quote.AuthorName = %q, want %q", q.AuthorName, "original-author")
	}
	if q.Content != "original content" {
		t.Errorf("Quote.Content = %q, want %q", q.Content, "original content")
	}

	// Verify attachments
	if len(decoded.Attachments) != 2 {
		t.Fatalf("len(Attachments) = %d, want 2", len(decoded.Attachments))
	}
	if decoded.Attachments[0].Type != "photo" {
		t.Errorf("Attachments[0].Type = %q, want %q", decoded.Attachments[0].Type, "photo")
	}
	if decoded.Attachments[1].Transcript != "hello world" {
		t.Errorf("Attachments[1].Transcript = %q, want %q", decoded.Attachments[1].Transcript, "hello world")
	}
	if decoded.Attachments[1].Method != "whisper" {
		t.Errorf("Attachments[1].Method = %q, want %q", decoded.Attachments[1].Method, "whisper")
	}

	// Verify WeiboMetadata
	if decoded.Metadata.Weibo == nil {
		t.Fatal("WeiboMetadata is nil")
	}
	if !decoded.Metadata.Weibo.IsRepost {
		t.Error("WeiboMetadata.IsRepost should be true")
	}
	if decoded.Metadata.Weibo.OriginalURL != "https://weibo.com/detail/original" {
		t.Errorf("WeiboMetadata.OriginalURL = %q, want %q", decoded.Metadata.Weibo.OriginalURL, "https://weibo.com/detail/original")
	}
}

func TestRawContent_MediaItemsBackwardCompat(t *testing.T) {
	// Simulates deserializing an old record that only has media_items.
	oldJSON := `{
		"source": "web",
		"external_id": "old-123",
		"content": "old post",
		"author_name": "author",
		"url": "https://example.com/old",
		"posted_at": "2026-01-01T00:00:00Z",
		"media_items": [
			{"type": "photo", "url": "https://cdn.example.com/old.jpg"}
		]
	}`

	var rc RawContent
	if err := json.Unmarshal([]byte(oldJSON), &rc); err != nil {
		t.Fatal(err)
	}

	if len(rc.MediaItems) != 1 {
		t.Errorf("expected 1 media item, got %d", len(rc.MediaItems))
	}
	if rc.MediaItems[0].Type != "photo" {
		t.Errorf("MediaItems[0].Type = %q, want %q", rc.MediaItems[0].Type, "photo")
	}

	// New fields should be nil/empty
	if len(rc.Quotes) != 0 {
		t.Errorf("expected 0 quotes, got %d", len(rc.Quotes))
	}
	if len(rc.Attachments) != 0 {
		t.Errorf("expected 0 attachments, got %d", len(rc.Attachments))
	}
}

func TestRawContent_ThreadContextJSONRoundTrip(t *testing.T) {
	pos := 3
	raw := RawContent{
		Source:     "twitter",
		ExternalID: "tweet-123",
		Content:    "thread post",
		AuthorName: "author",
		URL:        "https://x.com/author/status/tweet-123",
		Metadata: RawMetadata{
			Thread: &ThreadContext{
				ThreadID:         "conv-1",
				ParentExternalID: "tweet-122",
				RootExternalID:   "tweet-100",
				ThreadPosition:   &pos,
				ThreadIncomplete: true,
				IsSelfThread:     true,
			},
		},
	}

	payload, err := json.Marshal(raw)
	if err != nil {
		t.Fatal(err)
	}
	var decoded RawContent
	if err := json.Unmarshal(payload, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.Metadata.Thread == nil {
		t.Fatal("Thread is nil")
	}
	tc := decoded.Metadata.Thread
	if tc.ThreadID != "conv-1" {
		t.Errorf("ThreadID = %q", tc.ThreadID)
	}
	if tc.ParentExternalID != "tweet-122" {
		t.Errorf("ParentExternalID = %q", tc.ParentExternalID)
	}
	if tc.ThreadPosition == nil || *tc.ThreadPosition != 3 {
		t.Errorf("ThreadPosition = %v", tc.ThreadPosition)
	}
	if !tc.ThreadIncomplete {
		t.Error("ThreadIncomplete should be true")
	}
	if !tc.IsSelfThread {
		t.Error("IsSelfThread should be true")
	}
}

func TestRawContent_ThreadContextJSONRoundTripIncludesScope(t *testing.T) {
	pos := 3
	raw := RawContent{
		Source:     "twitter",
		ExternalID: "tweet-123",
		Content:    "thread post",
		AuthorName: "author",
		URL:        "https://x.com/author/status/tweet-123",
		Metadata: RawMetadata{
			Thread: &ThreadContext{
				ThreadID:         "conv-1",
				ThreadScope:      ThreadScopeConversation,
				ParentExternalID: "tweet-122",
				RootExternalID:   "tweet-100",
				ThreadPosition:   &pos,
				ThreadIncomplete: true,
				IsSelfThread:     true,
			},
		},
	}

	payload, err := json.Marshal(raw)
	if err != nil {
		t.Fatal(err)
	}
	var decoded RawContent
	if err := json.Unmarshal(payload, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.Metadata.Thread == nil {
		t.Fatal("decoded.Metadata.Thread is nil")
	}
	if decoded.Metadata.Thread.ThreadScope != ThreadScopeConversation {
		t.Fatalf("ThreadScope = %q, want %q", decoded.Metadata.Thread.ThreadScope, ThreadScopeConversation)
	}
}

func TestRawContent_ThreadContextLegacyJSONWithoutScopeStillDecodes(t *testing.T) {
	raw := []byte(`{
		"source":"twitter",
		"external_id":"tweet-123",
		"content":"thread post",
		"url":"https://x.com/author/status/tweet-123",
		"metadata":{
			"thread":{
				"thread_id":"conv-1",
				"parent_external_id":"tweet-122",
				"root_external_id":"tweet-100",
				"is_self_thread":true
			}
		}
	}`)

	var decoded RawContent
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.Metadata.Thread == nil {
		t.Fatal("decoded.Metadata.Thread is nil")
	}
	if decoded.Metadata.Thread.ThreadScope != "" {
		t.Fatalf("ThreadScope = %q, want empty string for legacy payload", decoded.Metadata.Thread.ThreadScope)
	}
}

func TestRawContent_AttachmentNewFieldsJSONRoundTrip(t *testing.T) {
	raw := RawContent{
		Source:     "twitter",
		ExternalID: "v-123",
		Content:    "video post",
		AuthorName: "author",
		URL:        "https://x.com/author/status/v-123",
		Attachments: []Attachment{
			{
				Type:             "video",
				URL:              "https://video.twimg.com/vid.mp4",
				PosterURL:        "https://pbs.twimg.com/poster.jpg",
				Transcript:       "hello world",
				TranscriptMethod: "whisper",
				TranscriptDiagnostics: []TranscriptDiagnostic{
					{Stage: "download", Code: "ok", Detail: "50MB"},
				},
			},
		},
	}

	payload, err := json.Marshal(raw)
	if err != nil {
		t.Fatal(err)
	}
	var decoded RawContent
	if err := json.Unmarshal(payload, &decoded); err != nil {
		t.Fatal(err)
	}
	if len(decoded.Attachments) != 1 {
		t.Fatalf("len(Attachments) = %d", len(decoded.Attachments))
	}
	a := decoded.Attachments[0]
	if a.PosterURL != "https://pbs.twimg.com/poster.jpg" {
		t.Errorf("PosterURL = %q", a.PosterURL)
	}
	if a.TranscriptMethod != "whisper" {
		t.Errorf("TranscriptMethod = %q", a.TranscriptMethod)
	}
	if len(a.TranscriptDiagnostics) != 1 {
		t.Fatalf("len(TranscriptDiagnostics) = %d", len(a.TranscriptDiagnostics))
	}
	if a.TranscriptDiagnostics[0].Stage != "download" {
		t.Errorf("diagnostics stage = %q", a.TranscriptDiagnostics[0].Stage)
	}
}

func TestRawContent_OldMethodFieldStillDecodes(t *testing.T) {
	// Old records only have "method", not "transcript_method"
	oldJSON := `{
		"source": "weibo",
		"external_id": "old-vid",
		"content": "video post",
		"author_name": "author",
		"url": "https://example.com",
		"posted_at": "2026-01-01T00:00:00Z",
		"attachments": [
			{"type": "video", "url": "https://example.com/v.mp4", "transcript": "hello", "method": "whisper"}
		]
	}`
	var rc RawContent
	if err := json.Unmarshal([]byte(oldJSON), &rc); err != nil {
		t.Fatal(err)
	}
	if len(rc.Attachments) != 1 {
		t.Fatalf("len(Attachments) = %d", len(rc.Attachments))
	}
	if rc.Attachments[0].Method != "whisper" {
		t.Errorf("Method = %q, want whisper", rc.Attachments[0].Method)
	}
}

func TestRawContent_ExpandedTextIncludesQuotesAndTranscripts(t *testing.T) {
	raw := RawContent{
		Content: "main body\n\n[引用#1 @alice]\n\n[附件#1 视频]",
		Quotes: []Quote{{
			Content: "quoted full body",
		}},
		References: []Reference{{
			URL: "https://example.com/ref",
		}},
		Attachments: []Attachment{{
			Type:       "video",
			Transcript: "video transcript body",
		}},
	}

	got := raw.ExpandedText()
	if !strings.Contains(got, "main body") {
		t.Fatalf("ExpandedText() missing main body: %q", got)
	}
	if !strings.Contains(got, "[引用正文#1]\nquoted full body") {
		t.Fatalf("ExpandedText() missing quote body: %q", got)
	}
	if !strings.Contains(got, "[参考链接#1]\nhttps://example.com/ref") {
		t.Fatalf("ExpandedText() missing reference url: %q", got)
	}
	if !strings.Contains(got, "[附件转写#1]\nvideo transcript body") {
		t.Fatalf("ExpandedText() missing transcript body: %q", got)
	}
}
