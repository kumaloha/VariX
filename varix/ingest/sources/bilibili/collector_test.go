package bilibili

import (
	"context"
	"errors"
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

func TestCollector_UsesSpeakerFromTitleWhenPresent(t *testing.T) {
	audio := &fakeAudio{text: "audio transcript", method: "whisper"}
	c := New(fakeMeta{
		value: Metadata{
			Title:       "付鹏：2026 大类资产怎么看",
			Uploader:    "Uploader Name",
			PublishedAt: time.Date(2026, 4, 7, 10, 0, 0, 0, time.UTC),
		},
	}, audio)

	got, err := c.Fetch(context.Background(), types.ParsedURL{
		Platform:     types.PlatformBilibili,
		ContentType:  types.ContentTypePost,
		PlatformID:   "BV1ABCDEF123",
		CanonicalURL: "https://www.bilibili.com/video/BV1ABCDEF123",
	})
	if err != nil {
		t.Fatalf("Fetch() error = %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("len(Fetch()) = %d, want 1", len(got))
	}
	if got[0].AuthorName != "付鹏" {
		t.Fatalf("AuthorName = %q, want %q", got[0].AuthorName, "付鹏")
	}
	if got[0].AuthorID != "Uploader Name" {
		t.Fatalf("AuthorID = %q, want %q", got[0].AuthorID, "Uploader Name")
	}
	if got[0].Metadata.Bilibili == nil {
		t.Fatal("Metadata.Bilibili is nil")
	}
	if got[0].Metadata.Bilibili.TranscriptMethod != "whisper" {
		t.Fatalf("transcript_method = %#v, want %q", got[0].Metadata.Bilibili.TranscriptMethod, "whisper")
	}
}

func TestCollector_FallsBackToTitleOnlyWhenAudioUnavailable(t *testing.T) {
	audio := &fakeAudio{err: errors.New("tool missing")}
	c := New(fakeMeta{
		value: Metadata{
			Title:       "Example Bilibili Video",
			Uploader:    "Uploader Name",
			PublishedAt: time.Date(2026, 4, 7, 10, 0, 0, 0, time.UTC),
		},
	}, audio)

	got, err := c.Fetch(context.Background(), types.ParsedURL{
		Platform:     types.PlatformBilibili,
		ContentType:  types.ContentTypePost,
		PlatformID:   "BV1ABCDEF124",
		CanonicalURL: "https://www.bilibili.com/video/BV1ABCDEF124",
	})
	if err != nil {
		t.Fatalf("Fetch() error = %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("len(Fetch()) = %d, want 1", len(got))
	}
	if got[0].Content != "# Example Bilibili Video\n\n（无法获取视频内容）" {
		t.Fatalf("Content = %q", got[0].Content)
	}
	if got[0].Metadata.Bilibili == nil {
		t.Fatal("Metadata.Bilibili is nil")
	}
	if got[0].Metadata.Bilibili.TranscriptMethod != "title_only" {
		t.Fatalf("transcript_method = %#v, want title_only", got[0].Metadata.Bilibili.TranscriptMethod)
	}
	if len(got[0].Metadata.Bilibili.TranscriptDiagnostics) != 1 {
		t.Fatalf("len(transcript_diagnostics) = %d, want 1", len(got[0].Metadata.Bilibili.TranscriptDiagnostics))
	}
	if got[0].Metadata.Bilibili.TranscriptDiagnostics[0].Stage != "audio" {
		t.Fatalf("diagnostics[0].stage = %q, want audio", got[0].Metadata.Bilibili.TranscriptDiagnostics[0].Stage)
	}
	if got[0].Metadata.Bilibili.TranscriptDiagnostics[0].Code != "tool_missing" {
		t.Fatalf("diagnostics[0].code = %q, want tool_missing", got[0].Metadata.Bilibili.TranscriptDiagnostics[0].Code)
	}
}

func TestCollector_EmitsRawContentShape(t *testing.T) {
	audio := &fakeAudio{text: "audio transcript", method: "whisper"}
	c := New(fakeMeta{
		value: Metadata{
			Title:       "Example Bilibili Video",
			Uploader:    "Uploader Name",
			PublishedAt: time.Date(2026, 4, 7, 10, 0, 0, 0, time.UTC),
		},
	}, audio)

	got, err := c.Fetch(context.Background(), types.ParsedURL{
		Platform:     types.PlatformBilibili,
		ContentType:  types.ContentTypePost,
		PlatformID:   "BV1ABCDEF125",
		CanonicalURL: "https://www.bilibili.com/video/BV1ABCDEF125",
	})
	if err != nil {
		t.Fatalf("Fetch() error = %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("len(Fetch()) = %d, want 1", len(got))
	}
	if got[0].Source != "bilibili" {
		t.Fatalf("Source = %q, want %q", got[0].Source, "bilibili")
	}
	if got[0].ExternalID != "BV1ABCDEF125" {
		t.Fatalf("ExternalID = %q, want %q", got[0].ExternalID, "BV1ABCDEF125")
	}
	if got[0].URL != "https://www.bilibili.com/video/BV1ABCDEF125" {
		t.Fatalf("URL = %q, want %q", got[0].URL, "https://www.bilibili.com/video/BV1ABCDEF125")
	}
	if got[0].Metadata.Bilibili == nil {
		t.Fatal("Metadata.Bilibili is nil")
	}
	if got[0].Metadata.Bilibili.Title != "Example Bilibili Video" {
		t.Fatalf("title = %#v", got[0].Metadata.Bilibili.Title)
	}
	if got[0].Metadata.Bilibili.Uploader != "Uploader Name" {
		t.Fatalf("uploader = %#v", got[0].Metadata.Bilibili.Uploader)
	}
}

func TestCollector_PreservesDescriptionAndSourceLinks(t *testing.T) {
	audio := &fakeAudio{text: "audio transcript", method: "whisper"}
	c := New(fakeMeta{
		value: Metadata{
			Title:       "巴菲特访谈熟肉",
			Uploader:    "Uploader Name",
			Description: "原视频：https://www.cnbc.com/interview",
			SourceLinks: []string{"https://www.cnbc.com/interview"},
			PublishedAt: time.Date(2026, 4, 7, 10, 0, 0, 0, time.UTC),
		},
	}, audio)

	got, err := c.Fetch(context.Background(), types.ParsedURL{
		Platform:     types.PlatformBilibili,
		ContentType:  types.ContentTypePost,
		PlatformID:   "BV1ABCDEF126",
		CanonicalURL: "https://www.bilibili.com/video/BV1ABCDEF126",
	})
	if err != nil {
		t.Fatalf("Fetch() error = %v", err)
	}
	if got[0].Metadata.Bilibili == nil {
		t.Fatal("Metadata.Bilibili is nil")
	}
	if got[0].Metadata.Bilibili.Description != "原视频：https://www.cnbc.com/interview" {
		t.Fatalf("description = %#v", got[0].Metadata.Bilibili.Description)
	}
	if len(got[0].Metadata.Bilibili.SourceLinks) != 1 {
		t.Fatalf("len(source_links) = %d, want 1", len(got[0].Metadata.Bilibili.SourceLinks))
	}
	if got[0].Metadata.Bilibili.SourceLinks[0] != "https://www.cnbc.com/interview" {
		t.Fatalf("source_links[0] = %#v", got[0].Metadata.Bilibili.SourceLinks[0])
	}
}
