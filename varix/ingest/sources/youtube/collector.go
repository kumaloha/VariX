package youtube

import (
	"context"
	"errors"
	"net/http"
	"os/exec"
	"strings"
	"time"

	"github.com/kumaloha/VariX/varix/ingest/normalize"
	"github.com/kumaloha/VariX/varix/ingest/types"
)

type Metadata struct {
	Title       string
	ChannelName string
	ChannelID   string
	Description string
	SourceLinks []string
	PublishedAt time.Time
}

type MetadataFetcher interface {
	Fetch(ctx context.Context, videoID string) (Metadata, error)
}

type SubtitleFetcher interface {
	Fetch(ctx context.Context, videoID string) (text string, method string, err error)
}

type AudioTranscriber interface {
	Transcribe(ctx context.Context, videoID string) (text string, method string, err error)
}

type Collector struct {
	meta      MetadataFetcher
	subtitles SubtitleFetcher
	audio     AudioTranscriber
}

var errASRKeyMissing = errors.New("asr key missing")

func New(meta MetadataFetcher, subtitles SubtitleFetcher, audio AudioTranscriber) *Collector {
	return &Collector{
		meta:      meta,
		subtitles: subtitles,
		audio:     audio,
	}
}

func NewDefault(projectRoot string, httpClient *http.Client) *Collector {
	return New(
		NewHTTPMetadataFetcher(httpClient),
		NewYTDLPSubtitleFetcher(),
		NewWhisperAudioTranscriber(projectRoot, httpClient),
	)
}

func (c *Collector) Platform() types.Platform {
	return types.PlatformYouTube
}

func (c *Collector) Fetch(ctx context.Context, parsed types.ParsedURL) ([]types.RawContent, error) {
	metadata, err := c.meta.Fetch(ctx, parsed.PlatformID)
	if err != nil {
		return nil, err
	}

	authorName := metadata.ChannelName
	if speaker := normalize.ExtractSpeakerFromTitle(metadata.Title); speaker != "" {
		authorName = speaker
	}

	diagnostics := make([]types.TranscriptDiagnostic, 0, 2)
	text, method := "", ""
	if c.subtitles != nil {
		text, method, err = c.subtitles.Fetch(ctx, parsed.PlatformID)
		if diagnostic, ok := classifyTranscriptDiagnostic("subtitle", text, err); ok {
			diagnostics = append(diagnostics, diagnostic)
		}
	}
	if text == "" && c.audio != nil {
		text, method, err = c.audio.Transcribe(ctx, parsed.PlatformID)
		if diagnostic, ok := classifyTranscriptDiagnostic("audio", text, err); ok {
			diagnostics = append(diagnostics, diagnostic)
		}
	}
	if text == "" {
		text = unavailableVideoContent(metadata.Title, metadata.Description)
		method = "title_only"
	}

	return []types.RawContent{{
		Source:     "youtube",
		ExternalID: parsed.PlatformID,
		Content:    text,
		AuthorName: authorName,
		AuthorID:   metadata.ChannelID,
		URL:        parsed.CanonicalURL,
		PostedAt:   metadata.PublishedAt,
		Metadata: types.RawMetadata{
			YouTube: &types.YouTubeMetadata{
				Title:                 metadata.Title,
				ChannelName:           metadata.ChannelName,
				ChannelID:             metadata.ChannelID,
				Description:           metadata.Description,
				SourceLinks:           metadata.SourceLinks,
				TranscriptMethod:      method,
				TranscriptDiagnostics: diagnostics,
			},
		},
	}}, nil
}

func classifyTranscriptDiagnostic(stage, text string, err error) (types.TranscriptDiagnostic, bool) {
	if strings.TrimSpace(text) != "" {
		return types.TranscriptDiagnostic{}, false
	}
	if err == nil {
		return types.TranscriptDiagnostic{Stage: stage, Code: "unavailable"}, true
	}

	code := "fetch_failed"
	switch {
	case errors.Is(err, errASRKeyMissing) || strings.Contains(strings.ToLower(err.Error()), "asr key missing"):
		code = "asr_key_missing"
	case errors.Is(err, exec.ErrNotFound) || strings.Contains(strings.ToLower(err.Error()), "tool missing"):
		code = "tool_missing"
	case strings.Contains(strings.ToLower(err.Error()), "status 429") || strings.Contains(strings.ToLower(err.Error()), "rate limit"):
		code = "rate_limited"
	case stage == "audio":
		code = "transcription_failed"
	}
	return types.TranscriptDiagnostic{
		Stage:  stage,
		Code:   code,
		Detail: err.Error(),
	}, true
}

func unavailableVideoContent(title, description string) string {
	title = strings.TrimSpace(title)
	description = strings.TrimSpace(description)
	if description == "" {
		return "# " + title + "\n\n（无法获取视频内容）"
	}
	return "# " + title + "\n\n（无法获取视频内容，以下为视频简介）\n\n" + description
}
