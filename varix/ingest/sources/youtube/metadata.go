package youtube

import (
	"context"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/kumaloha/VariX/varix/ingest/normalize"
	"github.com/kumaloha/VariX/varix/ingest/sources/httputil"
)

type HTTPMetadataFetcher struct {
	client *http.Client
}

func NewHTTPMetadataFetcher(client *http.Client) *HTTPMetadataFetcher {
	if client == nil {
		client = http.DefaultClient
	}
	return &HTTPMetadataFetcher{client: client}
}

var (
	youtubeTitleRE      = regexp.MustCompile(`"title"\s*:\s*"([^"]+)"`)
	youtubeAuthorRE     = regexp.MustCompile(`"author"\s*:\s*"([^"]+)"`)
	youtubeChannelIDRE  = regexp.MustCompile(`"channelId"\s*:\s*"([^"]+)"`)
	youtubePublishDate  = regexp.MustCompile(`"publishDate"\s*:\s*"(\d{4}-\d{2}-\d{2})"`)
	youtubeDescRE       = regexp.MustCompile(`"shortDescription"\s*:\s*"((?:\\.|[^"])*)"`)
	youtubeMetaTitleRE  = regexp.MustCompile(`(?is)<meta[^>]+property=["']og:title["'][^>]+content=["'](.*?)["']`)
	youtubeMetaAuthorRE = regexp.MustCompile(`(?is)<link[^>]+itemprop=["']name["'][^>]+content=["'](.*?)["']`)
)

func (f *HTTPMetadataFetcher) Fetch(ctx context.Context, videoID string) (Metadata, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://www.youtube.com/watch?v="+videoID, nil)
	if err != nil {
		return Metadata{}, err
	}

	resp, err := f.client.Do(req)
	if err != nil {
		return Metadata{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return Metadata{}, fmt.Errorf("youtube metadata fetch failed: status %d", resp.StatusCode)
	}

	if err := httputil.CheckContentLength(resp, 5<<20); err != nil {
		return Metadata{}, err
	}
	body, err := httputil.LimitedReadAll(resp.Body, 5<<20)
	if err != nil {
		return Metadata{}, err
	}
	html := string(body)

	title := firstMatch(youtubeTitleRE, html)
	if title == "" {
		title = firstMatch(youtubeMetaTitleRE, html)
	}
	if title == "" {
		return Metadata{}, fmt.Errorf("youtube metadata title missing for %s", videoID)
	}

	channelName := firstMatch(youtubeAuthorRE, html)
	if channelName == "" {
		channelName = firstMatch(youtubeMetaAuthorRE, html)
	}
	channelID := firstNonEmpty(firstMatch(youtubeChannelIDRE, html), videoID)

	publishedAt := time.Time{}
	if rawDate := firstMatch(youtubePublishDate, html); rawDate != "" {
		if parsed, err := time.Parse("2006-01-02", rawDate); err == nil {
			publishedAt = parsed.UTC()
		}
	}
	description := normalize.CollapseWhitespace(unescapeJSONText(firstMatch(youtubeDescRE, html)))

	return Metadata{
		Title:       unescapeJSONText(title),
		ChannelName: unescapeJSONText(channelName),
		ChannelID:   channelID,
		Description: description,
		SourceLinks: normalize.ExtractURLs(description),
		PublishedAt: publishedAt,
	}, nil
}

func firstMatch(pattern *regexp.Regexp, input string) string {
	match := pattern.FindStringSubmatch(input)
	if len(match) < 2 {
		return ""
	}
	return strings.TrimSpace(match[1])
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func unescapeJSONText(input string) string {
	replacer := strings.NewReplacer(`\u0026`, "&", `\"`, `"`, `\n`, " ", `\/`, "/")
	return replacer.Replace(input)
}
