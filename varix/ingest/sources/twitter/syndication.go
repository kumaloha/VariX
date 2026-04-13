package twitter

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/kumaloha/VariX/varix/ingest/assemble"
	"github.com/kumaloha/VariX/varix/ingest/normalize"
	"github.com/kumaloha/VariX/varix/ingest/provenance"
	"github.com/kumaloha/VariX/varix/ingest/sources/httputil"
	"github.com/kumaloha/VariX/varix/ingest/types"
)

const syndicationEndpoint = "https://cdn.syndication.twimg.com/tweet-result"

var (
	bodyTwitterPost = regexp.MustCompile(`(?:twitter\.com|x\.com)/[\w]+/status/(\d+)`)
	bodyWeiboPost   = regexp.MustCompile(`weibo\.com/\d+/(\w+)|m\.weibo\.cn/(?:status|detail)/(\w+)`)
)

// --- Typed payload structs for syndication JSON decoding ---

type syndicationUser struct {
	ScreenName string `json:"screen_name"`
	Name       string `json:"name"`
	IDStr      string `json:"id_str"`
}

type syndicationNote struct {
	NoteResults struct {
		Result struct {
			Text string `json:"text"`
		} `json:"result"`
	} `json:"note_tweet_results"`
}

type syndicationArticle struct {
	Title       string `json:"title"`
	PreviewText string `json:"preview_text"`
	RestID      string `json:"rest_id"`
}

type syndicationQuote struct {
	IDStr     string           `json:"id_str"`
	Text      string           `json:"text"`
	CreatedAt string           `json:"created_at"`
	User      syndicationUser  `json:"user"`
	NoteTweet *syndicationNote `json:"note_tweet,omitempty"`
}

type syndicationThread struct {
	IDStr string `json:"id_str"`
}

type syndicationVariant struct {
	URL         string `json:"url"`
	ContentType string `json:"content_type"`
	Bitrate     *int   `json:"bitrate,omitempty"`
}

type syndicationVideoInfo struct {
	Variants []syndicationVariant `json:"variants"`
}

type syndicationMedia struct {
	Type          string                `json:"type"`
	MediaURLHTTPS string                `json:"media_url_https"`
	MediaURL      string                `json:"media_url"`
	VideoInfo     *syndicationVideoInfo `json:"video_info,omitempty"`
}

type syndicationPayload struct {
	IDStr       string              `json:"id_str"`
	Text        string              `json:"text"`
	CreatedAt   string              `json:"created_at"`
	User        syndicationUser     `json:"user"`
	Article     *syndicationArticle `json:"article,omitempty"`
	NoteTweet   *syndicationNote    `json:"note_tweet,omitempty"`
	QuotedTweet *syndicationQuote   `json:"quoted_tweet,omitempty"`
	SelfThread  *syndicationThread  `json:"self_thread,omitempty"`
	Media       []syndicationMedia  `json:"mediaDetails,omitempty"`
	Likes       int                 `json:"favorite_count"`
	Retweets    int                 `json:"retweet_count"`
	Replies     int                 `json:"conversation_count"`
	Tombstone   any                 `json:"tombstone,omitempty"`
	NotFound    bool                `json:"notFound,omitempty"`
}

// selectPreferredMP4 picks a mid-bitrate MP4 variant from the list.
// It filters to video/mp4, sorts by bitrate ascending, and returns the
// middle entry. Returns "" if no MP4 variants are available.
func selectPreferredMP4(variants []syndicationVariant) string {
	var mp4s []syndicationVariant
	for _, v := range variants {
		if v.ContentType == "video/mp4" {
			mp4s = append(mp4s, v)
		}
	}
	if len(mp4s) == 0 {
		return ""
	}
	sort.Slice(mp4s, func(i, j int) bool {
		bi, bj := 0, 0
		if mp4s[i].Bitrate != nil {
			bi = *mp4s[i].Bitrate
		}
		if mp4s[j].Bitrate != nil {
			bj = *mp4s[j].Bitrate
		}
		return bi < bj
	})
	return mp4s[len(mp4s)/2].URL
}

func buildSyndicationThreadContext(data syndicationPayload) *types.ThreadContext {
	if data.SelfThread == nil || data.SelfThread.IDStr == "" {
		return nil
	}
	return &types.ThreadContext{
		ThreadID:         data.SelfThread.IDStr,
		ThreadScope:      types.ThreadScopeSelfThread,
		RootExternalID:   data.SelfThread.IDStr,
		IsSelfThread:     true,
		ThreadIncomplete: true,
	}
}

func ParseSyndicationData(data syndicationPayload, fullArticleContent string) (types.RawContent, error) {
	tweetID := data.IDStr
	if tweetID == "" {
		return types.RawContent{}, fmt.Errorf("missing tweet id")
	}

	screenName := data.User.ScreenName
	authorName := httputil.FirstString(data.User.Name, screenName)
	authorID := data.User.IDStr

	body := data.Text
	metadata := &types.TwitterMetadata{
		Likes:    data.Likes,
		Retweets: data.Retweets,
		Replies:  data.Replies,
	}

	if data.Article != nil {
		metadata.IsArticle = true
		restID := data.Article.RestID
		articleURL := ""
		if restID != "" {
			articleURL = "https://x.com/i/article/" + restID
			metadata.ArticleURL = articleURL
			metadata.SourceLinks = []string{articleURL}
		}
		if fullArticleContent = sanitizeArticleContent(fullArticleContent); fullArticleContent != "" {
			body = fullArticleContent
		} else {
			title := data.Article.Title
			preview := data.Article.PreviewText
			body = "[X长文·仅预览] " + title + "\n\n" + preview + "\n\n[注：以上为系统自动截取的预览摘要，X长文全文需登录后查看"
			if articleURL != "" {
				body += "，原文链接：" + articleURL
			}
			body += "]"
		}
	}

	if data.NoteTweet != nil && fullArticleContent == "" {
		if text := data.NoteTweet.NoteResults.Result.Text; text != "" {
			body = text
		}
	}

	var quotes []types.Quote

	if qt := data.QuotedTweet; qt != nil {
		qtAuthor := httputil.FirstString(qt.User.Name, qt.User.ScreenName)
		qtScreenName := qt.User.ScreenName
		qtID := qt.IDStr
		qtText := qt.Text
		if qt.NoteTweet != nil {
			if longText := qt.NoteTweet.NoteResults.Result.Text; longText != "" {
				qtText = longText
			}
		}
		qtPostedAt, _ := parseTwitterTime(qt.CreatedAt)

		if qtID != "" || qtText != "" {
			quotes = append(quotes, types.Quote{
				Relation:   "quote_tweet",
				AuthorName: qtAuthor,
				AuthorID:   qt.User.IDStr,
				Platform:   "twitter",
				ExternalID: qtID,
				URL:        fmt.Sprintf("https://x.com/%s/status/%s", qtScreenName, qtID),
				Content:    qtText,
				PostedAt:   qtPostedAt,
			})
		}
	}

	references := extractBodyReferences(body, quotes, metadata.SourceLinks)

	var attachments []types.Attachment
	for _, media := range data.Media {
		posterURL := httputil.FirstString(media.MediaURLHTTPS, media.MediaURL)
		if posterURL == "" {
			continue
		}
		if media.Type == "video" || media.Type == "animated_gif" {
			mediaURL := ""
			if media.VideoInfo != nil {
				mediaURL = selectPreferredMP4(media.VideoInfo.Variants)
			}
			if mediaURL == "" {
				mediaURL = posterURL
			}
			attachments = append(attachments, types.Attachment{
				Type:      "video",
				URL:       mediaURL,
				PosterURL: posterURL,
			})
		} else {
			attachments = append(attachments, types.Attachment{Type: "image", URL: posterURL})
		}
	}

	postedAt, err := parseTwitterTime(data.CreatedAt)
	if err != nil {
		return types.RawContent{}, err
	}

	finalContent := assemble.AssembleStructuredContent(body, quotes, references, attachments)

	rawMeta := types.RawMetadata{Twitter: metadata}

	rawMeta.Thread = buildSyndicationThreadContext(data)

	return types.RawContent{
		Source:      "twitter",
		ExternalID:  tweetID,
		Content:     finalContent,
		AuthorName:  authorName,
		AuthorID:    authorID,
		URL:         fmt.Sprintf("https://x.com/%s/status/%s", screenName, tweetID),
		PostedAt:    postedAt,
		Quotes:      quotes,
		References:  references,
		Attachments: attachments,
		Metadata:    rawMeta,
	}, nil
}

func extractBodyReferences(body string, quotes []types.Quote, excludeURLs []string) []types.Reference {
	urls := normalize.ExtractURLs(body)
	if len(urls) == 0 {
		return nil
	}
	quotedURLs := make(map[string]struct{}, len(quotes))
	for _, quote := range quotes {
		if quote.URL != "" {
			quotedURLs[strings.TrimSpace(quote.URL)] = struct{}{}
		}
	}
	for _, rawURL := range excludeURLs {
		if trimmed := strings.TrimSpace(rawURL); trimmed != "" {
			quotedURLs[trimmed] = struct{}{}
		}
	}

	refs := make([]types.Reference, 0, len(urls))
	seen := make(map[string]struct{}, len(urls))
	for _, rawURL := range urls {
		canonical := strings.TrimSpace(rawURL)
		if canonical == "" {
			continue
		}
		if _, ok := quotedURLs[canonical]; ok {
			continue
		}
		if _, ok := seen[canonical]; ok {
			continue
		}
		seen[canonical] = struct{}{}

		ref := types.Reference{Kind: "link", URL: canonical}
		if platform, externalID, label, ok := detectPostReference(canonical); ok {
			ref.Kind = "post_link"
			ref.Platform = platform
			ref.ExternalID = externalID
			ref.Label = label
		}
		refs = append(refs, ref)
	}
	return refs
}

func detectPostReference(rawURL string) (platform string, externalID string, label string, ok bool) {
	switch {
	case bodyTwitterPost.MatchString(rawURL):
		m := bodyTwitterPost.FindStringSubmatch(rawURL)
		if len(m) > 1 {
			return string(types.PlatformTwitter), m[1], "X帖子", true
		}
	case bodyWeiboPost.MatchString(rawURL):
		m := bodyWeiboPost.FindStringSubmatch(rawURL)
		if len(m) > 2 {
			id := firstNonEmptyString(m[1], m[2])
			return string(types.PlatformWeibo), id, "微博", id != ""
		}
	}
	return "", "", "", false
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func sanitizeArticleContent(content string) string {
	trimmed := httputil.FirstString(content)
	if trimmed == "" {
		return ""
	}
	lower := strings.ToLower(trimmed)
	if strings.Contains(lower, "this page is not supported") {
		return ""
	}
	return trimmed
}

func parseTwitterTime(raw string) (time.Time, error) {
	if raw == "" {
		return time.Time{}, fmt.Errorf("empty twitter time")
	}
	if parsed, err := time.Parse(time.RFC1123, raw); err == nil {
		return parsed.UTC(), nil
	}
	if parsed, err := time.Parse(time.RFC1123Z, raw); err == nil {
		return parsed.UTC(), nil
	}
	parsed, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		return time.Time{}, err
	}
	return parsed.UTC(), nil
}

type SyndicationHTTPClient struct {
	client  *http.Client
	resolve func(ctx context.Context, raw string) (string, error)
}

func NewSyndicationHTTPClient(client *http.Client) *SyndicationHTTPClient {
	if client == nil {
		client = http.DefaultClient
	}
	return &SyndicationHTTPClient{
		client:  client,
		resolve: newReferenceResolver(client),
	}
}

func (c *SyndicationHTTPClient) FetchByID(ctx context.Context, tweetID string) ([]types.RawContent, error) {
	endpoint, err := url.Parse(syndicationEndpoint)
	if err != nil {
		return nil, err
	}
	query := endpoint.Query()
	query.Set("id", tweetID)
	query.Set("lang", "en")
	query.Set("token", SyndicationToken(tweetID))
	endpoint.RawQuery = query.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0")
	req.Header.Set("Referer", "https://platform.twitter.com/")
	req.Header.Set("Origin", "https://platform.twitter.com")

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return nil, nil
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("twitter syndication fetch failed: status %d", resp.StatusCode)
	}

	if err := httputil.CheckContentLength(resp, 2<<20); err != nil {
		return nil, err
	}
	var decoded syndicationPayload
	if err := httputil.DecodeJSONLimited(resp.Body, 2<<20, &decoded); err != nil {
		return nil, err
	}
	if decoded.Tombstone != nil || decoded.NotFound {
		return nil, nil
	}

	fullArticle := ""
	if decoded.Article != nil && decoded.Article.RestID != "" {
		fullArticle, _ = c.fetchArticle(ctx, "https://x.com/i/article/"+decoded.Article.RestID)
	}

	item, err := ParseSyndicationData(decoded, fullArticle)
	if err != nil {
		return nil, err
	}
	resolveReferences(ctx, c.resolve, &item)
	return []types.RawContent{item}, nil
}

func (c *SyndicationHTTPClient) fetchArticle(ctx context.Context, articleURL string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://r.jina.ai/"+articleURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "text/plain")
	req.Header.Set("X-No-Cache", "true")
	req.Header.Set("X-Timeout", "30")

	resp, err := c.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("twitter article fetch failed: status %d", resp.StatusCode)
	}
	if err := httputil.CheckContentLength(resp, 5<<20); err != nil {
		return "", err
	}
	body, err := httputil.LimitedReadAll(resp.Body, 5<<20)
	if err != nil {
		return "", err
	}
	return string(body), nil
}

func firstInt(values ...any) int {
	for _, value := range values {
		switch typed := value.(type) {
		case int:
			return typed
		case int32:
			return int(typed)
		case int64:
			return int(typed)
		case float64:
			return int(typed)
		}
	}
	return 0
}

func newReferenceResolver(client *http.Client) func(context.Context, string) (string, error) {
	resolver := provenance.NewHTTPResolver(client)
	return resolver.Resolve
}

func resolveReferences(ctx context.Context, resolver func(context.Context, string) (string, error), raw *types.RawContent) {
	if raw == nil || len(raw.References) == 0 || resolver == nil {
		return
	}
	filtered := raw.References[:0]
	for i := range raw.References {
		ref := raw.References[i]
		resolved, err := resolver(ctx, ref.URL)
		if err != nil || strings.TrimSpace(resolved) == "" {
			filtered = append(filtered, ref)
			continue
		}
		ref.URL = strings.TrimSpace(resolved)
		if platform, externalID, label, ok := detectPostReference(ref.URL); ok {
			if platform == raw.Source && externalID == raw.ExternalID {
				continue
			}
			ref.Kind = "post_link"
			ref.Platform = platform
			ref.ExternalID = externalID
			ref.Label = label
		}
		filtered = append(filtered, ref)
	}
	raw.References = filtered
	raw.Content = assemble.AssembleStructuredContent(stripStructuredPlaceholders(raw.Content), raw.Quotes, raw.References, raw.Attachments)
}

func stripStructuredPlaceholders(content string) string {
	if strings.TrimSpace(content) == "" {
		return ""
	}
	lines := strings.Split(content, "\n")
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			out = append(out, line)
			continue
		}
		switch {
		case strings.HasPrefix(trimmed, "[引用#"),
			strings.HasPrefix(trimmed, "[参考#"),
			strings.HasPrefix(trimmed, "[附件#"):
			continue
		default:
			out = append(out, line)
		}
	}
	return strings.TrimSpace(strings.Join(out, "\n"))
}
