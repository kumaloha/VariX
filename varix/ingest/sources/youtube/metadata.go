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
	youtubeTitleRE         = regexp.MustCompile(`"title"\s*:\s*"([^"]+)"`)
	youtubeAuthorRE        = regexp.MustCompile(`"author"\s*:\s*"([^"]+)"`)
	youtubeChannelIDRE     = regexp.MustCompile(`"channelId"\s*:\s*"([^"]+)"`)
	youtubePublishDateRE   = regexp.MustCompile(`"publishDate"\s*:\s*"([^"]+)"`)
	youtubeDescRE          = regexp.MustCompile(`"shortDescription"\s*:\s*"((?:\\.|[^"])*)"`)
	youtubeMetaTitleRE     = regexp.MustCompile(`(?is)<meta[^>]+property=["']og:title["'][^>]+content=["'](.*?)["']`)
	youtubeMetaAuthorRE    = regexp.MustCompile(`(?is)<link[^>]+itemprop=["']name["'][^>]+content=["'](.*?)["']`)
	youtubeMetaPublishedRE = regexp.MustCompile(`(?is)<meta[^>]+itemprop=["'](?:datePublished|uploadDate)["'][^>]+content=["'](.*?)["']`)
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

	publishedAt := parsePublishedAt(
		firstMatch(youtubePublishDateRE, html),
		firstMatch(youtubeMetaPublishedRE, html),
	)
	rawDescription := unescapeJSONText(firstMatch(youtubeDescRE, html))
	description := normalize.CollapseWhitespace(rawDescription)

	return Metadata{
		Title:       unescapeJSONText(title),
		ChannelName: unescapeJSONText(channelName),
		ChannelID:   channelID,
		Description: description,
		SourceLinks: extractYouTubeSourceLinks(rawDescription),
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
	replacer := strings.NewReplacer(`\u0026`, "&", `\"`, `"`, `\n`, "\n", `\/`, "/")
	return replacer.Replace(input)
}

func parsePublishedAt(values ...string) time.Time {
	layouts := []string{time.RFC3339, time.RFC3339Nano, "2006-01-02"}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		for _, layout := range layouts {
			if parsed, err := time.Parse(layout, value); err == nil {
				return parsed.UTC()
			}
		}
	}
	return time.Time{}
}

func extractYouTubeSourceLinks(description string) []string {
	return extractVideoSourceLinks(description)
}

func extractVideoSourceLinks(description string) []string {
	lines := strings.Split(description, "\n")
	out := make([]string, 0)
	seen := make(map[string]struct{})
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || !looksLikeSourceLine(line) {
			continue
		}
		for _, link := range normalize.ExtractURLs(line) {
			if _, ok := seen[link]; ok {
				continue
			}
			seen[link] = struct{}{}
			out = append(out, link)
		}
	}
	return out
}

func looksLikeSourceLine(line string) bool {
	clean := strings.TrimSpace(line)
	for _, url := range normalize.ExtractURLs(clean) {
		clean = strings.ReplaceAll(clean, url, "")
	}
	lower := strings.ToLower(normalize.CollapseWhitespace(clean))
	markers := []string{
		"原视频", "原影片", "原視頻", "原片", "原文", "原帖", "原始链接", "原始連結", "來源", "来源", "資料來源", "资料来源",
		"source", "original video", "original post", "original article", "full video", "full interview", "reference",
	}
	for _, marker := range markers {
		if strings.Contains(lower, strings.ToLower(marker)) {
			return true
		}
	}
	return false
}
