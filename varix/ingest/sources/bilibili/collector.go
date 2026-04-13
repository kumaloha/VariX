package bilibili

import (
	"context"
	"errors"
	"os/exec"
	"strings"
	"time"

	"github.com/kumaloha/VariX/varix/ingest/normalize"
	"github.com/kumaloha/VariX/varix/ingest/types"
)

type Metadata struct {
	Title       string
	Uploader    string
	Description string
	SourceLinks []string
	PublishedAt time.Time
}

type MetadataFetcher interface {
	Fetch(ctx context.Context, videoID string) (Metadata, error)
}

type AudioTranscriber interface {
	Transcribe(ctx context.Context, videoID string) (text string, method string, err error)
}

type Collector struct {
	meta  MetadataFetcher
	audio AudioTranscriber
}

var errASRKeyMissing = errors.New("asr key missing")

func New(meta MetadataFetcher, audio AudioTranscriber) *Collector {
	return &Collector{
		meta:  meta,
		audio: audio,
	}
}

func NewDefault(projectRoot string) *Collector {
	return New(
		NewYTDLPMetadataFetcher(),
		NewWhisperAudioTranscriber(projectRoot),
	)
}

func (c *Collector) Platform() types.Platform {
	return types.PlatformBilibili
}

func (c *Collector) Fetch(ctx context.Context, parsed types.ParsedURL) ([]types.RawContent, error) {
	metadata, err := c.meta.Fetch(ctx, parsed.PlatformID)
	if err != nil {
		return nil, err
	}
	title, uploader, postedAt := metadata.Title, metadata.Uploader, metadata.PublishedAt

	authorName := uploader
	if speaker := normalize.ExtractSpeakerFromTitle(title); speaker != "" {
		authorName = speaker
	}

	diagnostics := make([]types.TranscriptDiagnostic, 0, 1)
	text, method := "", ""
	if c.audio != nil {
		text, method, err = c.audio.Transcribe(ctx, parsed.PlatformID)
		if diagnostic, ok := classifyTranscriptDiagnostic("audio", text, err); ok {
			diagnostics = append(diagnostics, diagnostic)
		}
	}
	if text == "" {
		text = "# " + title + "\n\n（无法获取视频内容）"
		method = "title_only"
	}

	return []types.RawContent{{
		Source:     "bilibili",
		ExternalID: parsed.PlatformID,
		Content:    text,
		AuthorName: authorName,
		AuthorID:   uploader,
		URL:        parsed.CanonicalURL,
		PostedAt:   postedAt,
		Metadata: types.RawMetadata{
			Bilibili: &types.BilibiliMetadata{
				Title:                 title,
				Uploader:              uploader,
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

	code := "transcription_failed"
	switch {
	case errors.Is(err, errASRKeyMissing) || strings.Contains(strings.ToLower(err.Error()), "asr key missing"):
		code = "asr_key_missing"
	case errors.Is(err, exec.ErrNotFound) || strings.Contains(strings.ToLower(err.Error()), "tool missing"):
		code = "tool_missing"
	}
	return types.TranscriptDiagnostic{
		Stage:  stage,
		Code:   code,
		Detail: err.Error(),
	}, true
}
