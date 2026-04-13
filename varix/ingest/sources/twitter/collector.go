package twitter

import (
	"context"
	"net/http"
	"strings"

	"github.com/kumaloha/VariX/varix/config"
	"github.com/kumaloha/VariX/varix/ingest/assemble"
	"github.com/kumaloha/VariX/varix/ingest/sources/audioutil"
	"github.com/kumaloha/VariX/varix/ingest/types"
)

type APIClient interface {
	FetchByID(ctx context.Context, tweetID string) ([]types.RawContent, error)
}

type SyndicationClient interface {
	FetchByID(ctx context.Context, tweetID string) ([]types.RawContent, error)
}

type Collector struct {
	api         APIClient
	syndication SyndicationClient
	asr         *audioutil.Client // nil = skip video transcription
	httpClient  *http.Client      // for video download
}

func New(api APIClient, syndication SyndicationClient) *Collector {
	return &Collector{
		api:         api,
		syndication: syndication,
	}
}

func NewDefault(projectRoot string, httpClient *http.Client) *Collector {
	var api APIClient
	if token, ok := config.Get(projectRoot, "TWITTER_BEARER_TOKEN"); ok && token != "" {
		api = NewAPIHTTPClient(httpClient, token)
	}

	syndication := NewSyndicationHTTPClient(httpClient)
	if authToken, ok := config.Get(projectRoot, "TWITTER_AUTH_TOKEN"); ok && strings.TrimSpace(authToken) != "" {
		if ct0, ok := config.Get(projectRoot, "TWITTER_CT0"); ok && strings.TrimSpace(ct0) != "" {
			syndication.authToken = strings.TrimSpace(authToken)
			syndication.ct0 = strings.TrimSpace(ct0)
		}
	}

	c := New(api, syndication)
	c.httpClient = httpClient

	// Enable video transcription when ASR credentials are available.
	c.asr = audioutil.NewClientFromConfig(projectRoot, httpClient)

	return c
}

func (c *Collector) Platform() types.Platform {
	return types.PlatformTwitter
}

func (c *Collector) Fetch(ctx context.Context, parsed types.ParsedURL) ([]types.RawContent, error) {
	var items []types.RawContent
	var err error

	if c.api != nil {
		items, err = c.api.FetchByID(ctx, parsed.PlatformID)
		if err != nil || len(items) == 0 {
			items, err = c.syndication.FetchByID(ctx, parsed.PlatformID)
		}
	} else {
		items, err = c.syndication.FetchByID(ctx, parsed.PlatformID)
	}
	if err != nil {
		return nil, err
	}

	if c.asr != nil && c.httpClient != nil {
		for i := range items {
			c.transcribeVideoAttachments(ctx, &items[i])
		}
	}
	for i := range items {
		if strings.Contains(items[i].Content, "[引用#") || strings.Contains(items[i].Content, "[参考#") || strings.Contains(items[i].Content, "[附件#") {
			continue
		}
		items[i].Content = assemble.AssembleStructuredContent(items[i].Content, items[i].Quotes, items[i].References, items[i].Attachments)
	}
	return items, nil
}

// transcribeVideoAttachments finds video attachments with real media URLs,
// runs the remote video transcription pipeline, and stores transcript detail
// on the attachment while leaving Content as compact placeholders.
func (c *Collector) transcribeVideoAttachments(ctx context.Context, rc *types.RawContent) {
	for i := range rc.Attachments {
		att := &rc.Attachments[i]
		if att.Type != "video" || att.URL == "" {
			continue
		}
		// Skip if URL is actually a poster image (no real video to transcribe).
		if att.URL == att.PosterURL {
			continue
		}

		result, err := audioutil.TranscribeRemoteVideo(ctx, c.httpClient, att.URL, c.asr, audioutil.RemoteVideoOptions{})
		if err != nil {
			att.TranscriptDiagnostics = result.TranscriptDiagnostics
			continue
		}
		att.Transcript = result.Transcript
		att.TranscriptMethod = result.TranscriptMethod
		att.TranscriptDiagnostics = result.TranscriptDiagnostics
	}
}
