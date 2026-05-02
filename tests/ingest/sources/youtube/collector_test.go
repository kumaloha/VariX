package youtube

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/kumaloha/VariX/varix/ingest/types"
)

type fakeMeta struct {
	value Metadata
	err   error
}

func (f fakeMeta) Fetch(_ context.Context, _ string) (Metadata, error) {
	return f.value, f.err
}

type fakeSubtitle struct {
	text   string
	method string
	err    error
	calls  int
}

func (f *fakeSubtitle) Fetch(_ context.Context, _ string) (string, string, error) {
	f.calls++
	return f.text, f.method, f.err
}

type fakeAudio struct {
	text   string
	method string
	err    error
	calls  int
}

func (f *fakeAudio) Transcribe(_ context.Context, _ string) (string, string, error) {
	f.calls++
	return f.text, f.method, f.err
}

func TestCollector_PrefersSubtitleWhenAvailable(t *testing.T) {
	subtitles := &fakeSubtitle{text: "subtitle text", method: "subtitle_zh"}
	audio := &fakeAudio{text: "audio text", method: "whisper"}
	c := New(fakeMeta{
		value: Metadata{
			Title:       "Example Title",
			ChannelName: "Example Channel",
			ChannelID:   "chan-1",
			PublishedAt: time.Date(2026, 4, 7, 10, 0, 0, 0, time.UTC),
		},
	}, subtitles, audio)

	got, err := c.Fetch(context.Background(), types.ParsedURL{
		Platform:     types.PlatformYouTube,
		ContentType:  types.ContentTypePost,
		PlatformID:   "video-1",
		CanonicalURL: "https://www.youtube.com/watch?v=video-1",
	})
	if err != nil {
		t.Fatalf("Fetch() error = %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("len(Fetch()) = %d, want 1", len(got))
	}
	if audio.calls != 0 {
		t.Fatalf("audio calls = %d, want 0", audio.calls)
	}
	if got[0].Metadata.YouTube == nil {
		t.Fatal("Metadata.YouTube is nil")
	}
	if got[0].Metadata.YouTube.TranscriptMethod != "subtitle_zh" {
		t.Fatalf("transcript_method = %#v, want %q", got[0].Metadata.YouTube.TranscriptMethod, "subtitle_zh")
	}
	if got[0].Content != "subtitle text" {
		t.Fatalf("Content = %q, want %q", got[0].Content, "subtitle text")
	}
}

func TestCollector_FallsBackToAudio(t *testing.T) {
	subtitles := &fakeSubtitle{err: errors.New("no subtitle")}
	audio := &fakeAudio{text: "audio transcript", method: "whisper"}
	c := New(fakeMeta{
		value: Metadata{
			Title:       "Example Title",
			ChannelName: "Example Channel",
			ChannelID:   "chan-1",
			PublishedAt: time.Date(2026, 4, 7, 10, 0, 0, 0, time.UTC),
		},
	}, subtitles, audio)

	got, err := c.Fetch(context.Background(), types.ParsedURL{
		Platform:     types.PlatformYouTube,
		ContentType:  types.ContentTypePost,
		PlatformID:   "video-2",
		CanonicalURL: "https://www.youtube.com/watch?v=video-2",
	})
	if err != nil {
		t.Fatalf("Fetch() error = %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("len(Fetch()) = %d, want 1", len(got))
	}
	if audio.calls != 1 {
		t.Fatalf("audio calls = %d, want 1", audio.calls)
	}
	if got[0].Metadata.YouTube == nil {
		t.Fatal("Metadata.YouTube is nil")
	}
	if got[0].Metadata.YouTube.TranscriptMethod != "whisper" {
		t.Fatalf("transcript_method = %#v, want %q", got[0].Metadata.YouTube.TranscriptMethod, "whisper")
	}
	if got[0].Content != "audio transcript" {
		t.Fatalf("Content = %q, want %q", got[0].Content, "audio transcript")
	}
}

func TestCollector_FallsBackToTitleOnlyAndUsesSpeakerFromTitle(t *testing.T) {
	subtitles := &fakeSubtitle{err: errors.New("no subtitle")}
	audio := &fakeAudio{err: errors.New("asr key missing")}
	c := New(fakeMeta{
		value: Metadata{
			Title:       "付鹏：2026 大类资产怎么看",
			ChannelName: "Example Channel",
			ChannelID:   "chan-1",
			PublishedAt: time.Date(2026, 4, 7, 10, 0, 0, 0, time.UTC),
		},
	}, subtitles, audio)

	got, err := c.Fetch(context.Background(), types.ParsedURL{
		Platform:     types.PlatformYouTube,
		ContentType:  types.ContentTypePost,
		PlatformID:   "video-3",
		CanonicalURL: "https://www.youtube.com/watch?v=video-3",
	})
	if err != nil {
		t.Fatalf("Fetch() error = %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("len(Fetch()) = %d, want 1", len(got))
	}
	if got[0].Source != "youtube" {
		t.Fatalf("Source = %q, want %q", got[0].Source, "youtube")
	}
	if got[0].ExternalID != "video-3" {
		t.Fatalf("ExternalID = %q, want %q", got[0].ExternalID, "video-3")
	}
	if got[0].AuthorName != "付鹏" {
		t.Fatalf("AuthorName = %q, want %q", got[0].AuthorName, "付鹏")
	}
	if got[0].AuthorID != "chan-1" {
		t.Fatalf("AuthorID = %q, want %q", got[0].AuthorID, "chan-1")
	}
	if got[0].URL != "https://www.youtube.com/watch?v=video-3" {
		t.Fatalf("URL = %q, want %q", got[0].URL, "https://www.youtube.com/watch?v=video-3")
	}
	if got[0].Metadata.YouTube == nil {
		t.Fatal("Metadata.YouTube is nil")
	}
	if got[0].Metadata.YouTube.TranscriptMethod != "title_only" {
		t.Fatalf("transcript_method = %#v, want title_only", got[0].Metadata.YouTube.TranscriptMethod)
	}
	if len(got[0].Metadata.YouTube.TranscriptDiagnostics) != 2 {
		t.Fatalf("len(transcript_diagnostics) = %d, want 2", len(got[0].Metadata.YouTube.TranscriptDiagnostics))
	}
	if got[0].Metadata.YouTube.TranscriptDiagnostics[0].Stage != "subtitle" {
		t.Fatalf("diagnostics[0].stage = %q, want subtitle", got[0].Metadata.YouTube.TranscriptDiagnostics[0].Stage)
	}
	if got[0].Metadata.YouTube.TranscriptDiagnostics[0].Code != "fetch_failed" {
		t.Fatalf("diagnostics[0].code = %q, want fetch_failed", got[0].Metadata.YouTube.TranscriptDiagnostics[0].Code)
	}
	if got[0].Metadata.YouTube.TranscriptDiagnostics[1].Stage != "audio" {
		t.Fatalf("diagnostics[1].stage = %q, want audio", got[0].Metadata.YouTube.TranscriptDiagnostics[1].Stage)
	}
	if got[0].Metadata.YouTube.TranscriptDiagnostics[1].Code != "asr_key_missing" {
		t.Fatalf("diagnostics[1].code = %q, want asr_key_missing", got[0].Metadata.YouTube.TranscriptDiagnostics[1].Code)
	}
	if got[0].Metadata.YouTube.Title != "付鹏：2026 大类资产怎么看" {
		t.Fatalf("title = %#v", got[0].Metadata.YouTube.Title)
	}
	if got[0].Metadata.YouTube.ChannelName != "Example Channel" {
		t.Fatalf("channel_name = %#v", got[0].Metadata.YouTube.ChannelName)
	}
	if got[0].Metadata.YouTube.ChannelID != "chan-1" {
		t.Fatalf("channel_id = %#v", got[0].Metadata.YouTube.ChannelID)
	}
	if got[0].Content != "# 付鹏：2026 大类资产怎么看\n\n（无法获取视频内容）" {
		t.Fatalf("Content = %q", got[0].Content)
	}
}

func TestCollector_FallbackContentIncludesDescriptionWhenTranscriptUnavailable(t *testing.T) {
	subtitles := &fakeSubtitle{err: errors.New("no subtitle")}
	audio := &fakeAudio{err: errors.New("status 429")}
	c := New(fakeMeta{
		value: Metadata{
			Title:       "Long Interview",
			ChannelName: "Example Channel",
			ChannelID:   "chan-1",
			Description: "Detailed show notes and chapter summary.",
			PublishedAt: time.Date(2026, 4, 7, 10, 0, 0, 0, time.UTC),
		},
	}, subtitles, audio)

	got, err := c.Fetch(context.Background(), types.ParsedURL{
		Platform:     types.PlatformYouTube,
		ContentType:  types.ContentTypePost,
		PlatformID:   "video-desc-fallback",
		CanonicalURL: "https://www.youtube.com/watch?v=video-desc-fallback",
	})
	if err != nil {
		t.Fatalf("Fetch() error = %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("len(Fetch()) = %d, want 1", len(got))
	}
	if got[0].Metadata.YouTube == nil {
		t.Fatal("Metadata.YouTube is nil")
	}
	if got[0].Metadata.YouTube.TranscriptMethod != "title_only" {
		t.Fatalf("transcript_method = %#v, want title_only", got[0].Metadata.YouTube.TranscriptMethod)
	}
	if !strings.Contains(got[0].Content, "# Long Interview") {
		t.Fatalf("Content = %q, want title", got[0].Content)
	}
	if !strings.Contains(got[0].Content, "Detailed show notes and chapter summary.") {
		t.Fatalf("Content = %q, want description", got[0].Content)
	}
}

func TestCollector_PreservesDescriptionAndSourceLinks(t *testing.T) {
	subtitles := &fakeSubtitle{text: "subtitle text", method: "subtitle_zh"}
	audio := &fakeAudio{text: "audio text", method: "whisper"}
	c := New(fakeMeta{
		value: Metadata{
			Title:       "巴菲特访谈中字解读",
			ChannelName: "Example Channel",
			ChannelID:   "chan-1",
			Description: "原视频：https://www.cnbc.com/interview\n中字整理",
			SourceLinks: []string{"https://www.cnbc.com/interview"},
			PublishedAt: time.Date(2026, 4, 7, 10, 0, 0, 0, time.UTC),
		},
	}, subtitles, audio)

	got, err := c.Fetch(context.Background(), types.ParsedURL{
		Platform:     types.PlatformYouTube,
		ContentType:  types.ContentTypePost,
		PlatformID:   "video-4",
		CanonicalURL: "https://www.youtube.com/watch?v=video-4",
	})
	if err != nil {
		t.Fatalf("Fetch() error = %v", err)
	}
	if got[0].Metadata.YouTube == nil {
		t.Fatal("Metadata.YouTube is nil")
	}
	if got[0].Metadata.YouTube.Description != "原视频：https://www.cnbc.com/interview\n中字整理" {
		t.Fatalf("description = %#v", got[0].Metadata.YouTube.Description)
	}
	if len(got[0].Metadata.YouTube.SourceLinks) != 1 {
		t.Fatalf("len(source_links) = %d, want 1", len(got[0].Metadata.YouTube.SourceLinks))
	}
	if got[0].Metadata.YouTube.SourceLinks[0] != "https://www.cnbc.com/interview" {
		t.Fatalf("source_links[0] = %#v", got[0].Metadata.YouTube.SourceLinks[0])
	}
}
