package twitter

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strconv"
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

const (
	webBearerToken             = "AAAAAAAAAAAAAAAAAAAAANRILgAAAAAAnNwIzUejRCOuH5E6I8xnZz4puTs%3D1Zv7ttfk8LF81IUq16cHjhLTvJu4FA33AGWWjCpTnA"
	tweetResultByRestIDQueryID = "tmhPpO5sDermwYmq3h034A"
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

	body = inlineReferencePlaceholders(body, references)
	finalContent := assemble.AssembleContent(body, appendQuoteAndAttachmentPlaceholders(quotes, attachments))

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

func inlineReferencePlaceholders(body string, references []types.Reference) string {
	out := body
	for i, ref := range references {
		rawURL := strings.TrimSpace(ref.URL)
		if rawURL == "" {
			continue
		}
		out = strings.ReplaceAll(out, rawURL, assemble.FormatReferencePlaceholder(i+1, ref))
	}
	return out
}

func appendQuoteAndAttachmentPlaceholders(quotes []types.Quote, attachments []types.Attachment) []string {
	blocks := make([]string, 0, len(quotes)+len(attachments))
	for i, quote := range quotes {
		blocks = append(blocks, assemble.FormatQuotePlaceholder(i+1, quote))
	}
	for i, attachment := range attachments {
		blocks = append(blocks, assemble.FormatAttachmentPlaceholder(i+1, attachment))
	}
	return blocks
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
	client    *http.Client
	resolve   func(ctx context.Context, raw string) (string, error)
	authToken string
	ct0       string
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
	item, err := c.fetchItemByID(ctx, tweetID)
	if err != nil || item == nil {
		if err != nil {
			return nil, err
		}
		return nil, nil
	}
	c.hydrateThread(ctx, item)
	return []types.RawContent{*item}, nil
}

func (c *SyndicationHTTPClient) fetchItemByID(ctx context.Context, tweetID string) (*types.RawContent, error) {
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

	c.hydrateLongformTexts(ctx, &decoded)

	fullArticle := ""
	if decoded.Article != nil {
		if text, err := c.fetchArticlePlainText(ctx, tweetID); err == nil && strings.TrimSpace(text) != "" {
			fullArticle = text
		} else if decoded.Article.RestID != "" {
			fullArticle, _ = c.fetchArticle(ctx, "https://x.com/i/article/"+decoded.Article.RestID)
		}
	}

	item, err := ParseSyndicationData(decoded, fullArticle)
	if err != nil {
		return nil, err
	}
	resolveReferences(ctx, c.resolve, &item)
	return &item, nil
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

func (c *SyndicationHTTPClient) hydrateLongformTexts(ctx context.Context, payload *syndicationPayload) {
	if payload == nil {
		return
	}
	if needsLongformFallback(payload.NoteTweet) && payload.IDStr != "" {
		if text, err := c.fetchLongformText(ctx, payload.IDStr); err == nil && strings.TrimSpace(text) != "" {
			if payload.NoteTweet == nil {
				payload.NoteTweet = &syndicationNote{}
			}
			payload.NoteTweet.NoteResults.Result.Text = text
		}
	}
	if payload.QuotedTweet != nil && payload.QuotedTweet.IDStr != "" {
		if text, err := c.fetchLongformText(ctx, payload.QuotedTweet.IDStr); err == nil && strings.TrimSpace(text) != "" {
			if payload.QuotedTweet.NoteTweet == nil {
				payload.QuotedTweet.NoteTweet = &syndicationNote{}
			}
			payload.QuotedTweet.NoteTweet.NoteResults.Result.Text = text
		}
	}
}

func needsLongformFallback(note *syndicationNote) bool {
	return note != nil && strings.TrimSpace(note.NoteResults.Result.Text) == ""
}

func (c *SyndicationHTTPClient) fetchLongformText(ctx context.Context, tweetID string) (string, error) {
	if strings.TrimSpace(c.authToken) == "" || strings.TrimSpace(c.ct0) == "" {
		return "", fmt.Errorf("twitter longform auth missing")
	}
	endpoint, err := url.Parse("https://x.com/i/api/graphql/" + tweetResultByRestIDQueryID + "/TweetResultByRestId")
	if err != nil {
		return "", err
	}
	query := endpoint.Query()
	query.Set("variables", compactJSON(map[string]any{
		"tweetId":                                tweetID,
		"with_rux_injections":                    false,
		"includePromotedContent":                 true,
		"withCommunity":                          true,
		"withQuickPromoteEligibilityTweetFields": true,
		"withBirdwatchNotes":                     true,
		"withVoice":                              true,
	}))
	query.Set("features", compactJSON(longformFeatureFlags()))
	query.Set("fieldToggles", compactJSON(longformFieldToggles()))
	endpoint.RawQuery = query.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("authorization", "Bearer "+webBearerToken)
	req.Header.Set("x-csrf-token", c.ct0)
	req.Header.Set("x-twitter-active-user", "yes")
	req.Header.Set("x-twitter-auth-type", "OAuth2Session")
	req.Header.Set("x-twitter-client-language", "en")
	req.Header.Set("cookie", "auth_token="+c.authToken+"; ct0="+c.ct0)
	req.Header.Set("user-agent", "Mozilla/5.0")

	resp, err := c.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("twitter longform fetch failed: status %d", resp.StatusCode)
	}
	if err := httputil.CheckContentLength(resp, 4<<20); err != nil {
		return "", err
	}
	var decoded map[string]any
	if err := httputil.DecodeJSONLimited(resp.Body, 4<<20, &decoded); err != nil {
		return "", err
	}
	return extractGraphQLNoteText(decoded), nil
}

func (c *SyndicationHTTPClient) fetchArticlePlainText(ctx context.Context, tweetID string) (string, error) {
	payload, err := c.fetchTweetResultByRestIDGraphQL(ctx, tweetID)
	if err != nil {
		return "", err
	}
	return extractGraphQLArticlePlainText(payload), nil
}

func (c *SyndicationHTTPClient) fetchTweetResultByRestIDGraphQL(ctx context.Context, tweetID string) (map[string]any, error) {
	if strings.TrimSpace(c.authToken) == "" || strings.TrimSpace(c.ct0) == "" {
		return nil, fmt.Errorf("twitter graphql auth missing")
	}
	endpoint, err := url.Parse("https://x.com/i/api/graphql/" + tweetResultByRestIDQueryID + "/TweetResultByRestId")
	if err != nil {
		return nil, err
	}
	query := endpoint.Query()
	query.Set("variables", compactJSON(map[string]any{
		"tweetId":                                tweetID,
		"with_rux_injections":                    false,
		"includePromotedContent":                 true,
		"withCommunity":                          true,
		"withQuickPromoteEligibilityTweetFields": true,
		"withBirdwatchNotes":                     true,
		"withVoice":                              true,
	}))
	query.Set("features", compactJSON(longformFeatureFlags()))
	query.Set("fieldToggles", compactJSON(longformFieldToggles()))
	endpoint.RawQuery = query.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("authorization", "Bearer "+webBearerToken)
	req.Header.Set("x-csrf-token", c.ct0)
	req.Header.Set("x-twitter-active-user", "yes")
	req.Header.Set("x-twitter-auth-type", "OAuth2Session")
	req.Header.Set("x-twitter-client-language", "en")
	req.Header.Set("cookie", "auth_token="+c.authToken+"; ct0="+c.ct0)
	req.Header.Set("user-agent", "Mozilla/5.0")

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("twitter graphql fetch failed: status %d", resp.StatusCode)
	}
	if err := httputil.CheckContentLength(resp, 4<<20); err != nil {
		return nil, err
	}
	var decoded map[string]any
	if err := httputil.DecodeJSONLimited(resp.Body, 4<<20, &decoded); err != nil {
		return nil, err
	}
	return decoded, nil
}

func compactJSON(value any) string {
	payload, _ := json.Marshal(value)
	return string(payload)
}

func longformFeatureFlags() map[string]bool {
	return map[string]bool{
		"creator_subscriptions_tweet_preview_api_enabled":                         true,
		"premium_content_api_read_enabled":                                        true,
		"communities_web_enable_tweet_community_results_fetch":                    true,
		"c9s_tweet_anatomy_moderator_badge_enabled":                               true,
		"responsive_web_grok_analyze_button_fetch_trends_enabled":                 true,
		"responsive_web_grok_analyze_post_followups_enabled":                      true,
		"responsive_web_jetfuel_frame":                                            true,
		"responsive_web_grok_share_attachment_enabled":                            true,
		"responsive_web_grok_annotations_enabled":                                 true,
		"articles_preview_enabled":                                                true,
		"responsive_web_edit_tweet_api_enabled":                                   true,
		"graphql_is_translatable_rweb_tweet_is_translatable_enabled":              true,
		"view_counts_everywhere_api_enabled":                                      true,
		"longform_notetweets_consumption_enabled":                                 true,
		"responsive_web_twitter_article_tweet_consumption_enabled":                true,
		"content_disclosure_indicator_enabled":                                    true,
		"content_disclosure_ai_generated_indicator_enabled":                       true,
		"responsive_web_grok_show_grok_translated_post":                           false,
		"responsive_web_grok_analysis_button_from_backend":                        true,
		"post_ctas_fetch_enabled":                                                 true,
		"freedom_of_speech_not_reach_fetch_enabled":                               true,
		"standardized_nudges_misinfo":                                             true,
		"tweet_with_visibility_results_prefer_gql_limited_actions_policy_enabled": true,
		"longform_notetweets_rich_text_read_enabled":                              true,
		"longform_notetweets_inline_media_enabled":                                true,
		"profile_label_improvements_pcf_label_in_post_enabled":                    true,
		"responsive_web_profile_redirect_enabled":                                 false,
		"rweb_tipjar_consumption_enabled":                                         true,
		"verified_phone_label_enabled":                                            false,
		"responsive_web_grok_image_annotation_enabled":                            true,
		"responsive_web_grok_imagine_annotation_enabled":                          true,
		"responsive_web_grok_community_note_auto_translation_is_enabled":          false,
		"responsive_web_graphql_skip_user_profile_image_extensions_enabled":       false,
		"responsive_web_graphql_timeline_navigation_enabled":                      true,
		"responsive_web_enhance_cards_enabled":                                    false,
	}
}

func longformFieldToggles() map[string]bool {
	return map[string]bool{
		"withArticleRichContentState": true,
		"withArticlePlainText":        true,
		"withArticleSummaryText":      true,
		"withArticleVoiceOver":        false,
		"withGrokAnalyze":             false,
		"withDisallowedReplyControls": false,
		"withPayments":                false,
		"withAuxiliaryUserLabels":     false,
	}
}

func extractGraphQLNoteText(payload map[string]any) string {
	path := []string{"data", "tweetResult", "result", "note_tweet", "note_tweet_results", "result", "text"}
	current := any(payload)
	for _, key := range path {
		asMap, ok := current.(map[string]any)
		if !ok {
			return ""
		}
		current, ok = asMap[key]
		if !ok {
			return ""
		}
	}
	text, _ := current.(string)
	return strings.TrimSpace(text)
}

func extractGraphQLArticlePlainText(payload map[string]any) string {
	for _, path := range [][]string{
		{"data", "tweetResult", "result", "article", "article_results", "result", "plain_text"},
		{"data", "tweetResult", "result", "article", "article_results", "result", "summary_text"},
	} {
		current := any(payload)
		ok := true
		for _, key := range path {
			asMap, isMap := current.(map[string]any)
			if !isMap {
				ok = false
				break
			}
			current, isMap = asMap[key]
			if !isMap {
				ok = false
				break
			}
		}
		if !ok {
			continue
		}
		if text, isString := current.(string); isString && strings.TrimSpace(text) != "" {
			return strings.TrimSpace(text)
		}
	}
	return ""
}

func (c *SyndicationHTTPClient) hydrateThread(ctx context.Context, item *types.RawContent) {
	if item == nil || strings.TrimSpace(c.authToken) == "" || strings.TrimSpace(c.ct0) == "" {
		return
	}
	ids, err := c.fetchSelfThreadIDs(ctx, item.ExternalID)
	if err != nil || len(ids) <= 1 {
		return
	}

	segments := make([]types.ThreadSegment, 0, len(ids))
	contents := make([]string, 0, len(ids))
	mergedQuotes := make([]types.Quote, 0)
	mergedReferences := make([]types.Reference, 0)
	mergedAttachments := make([]types.Attachment, 0)
	for idx, id := range ids {
		var segItem *types.RawContent
		if id == item.ExternalID {
			segItem = item
		} else {
			fetched, err := c.fetchItemByID(ctx, id)
			if err != nil || fetched == nil {
				continue
			}
			segItem = fetched
		}
		segments = append(segments, types.ThreadSegment{
			ExternalID:  segItem.ExternalID,
			URL:         segItem.URL,
			AuthorName:  segItem.AuthorName,
			AuthorID:    segItem.AuthorID,
			PostedAt:    segItem.PostedAt,
			Position:    idx + 1,
			Content:     segItem.Content,
			Attachments: segItem.Attachments,
		})
		rebased := rebaseStructuredPlaceholders(segItem.Content, segItem.Quotes, segItem.References, segItem.Attachments, len(mergedQuotes), len(mergedReferences), len(mergedAttachments))
		if strings.TrimSpace(rebased) != "" {
			contents = append(contents, rebased)
		}
		mergedQuotes = append(mergedQuotes, segItem.Quotes...)
		mergedReferences = append(mergedReferences, segItem.References...)
		mergedAttachments = append(mergedAttachments, segItem.Attachments...)
		if segItem.ExternalID == item.ExternalID {
			pos := idx + 1
			if item.Metadata.Thread == nil {
				item.Metadata.Thread = &types.ThreadContext{}
			}
			item.Metadata.Thread.ThreadID = ids[0]
			item.Metadata.Thread.ThreadScope = types.ThreadScopeSelfThread
			item.Metadata.Thread.ThreadPosition = &pos
			item.Metadata.Thread.RootExternalID = ids[0]
			item.Metadata.Thread.IsSelfThread = true
		}
	}
	if len(segments) <= 1 {
		return
	}
	item.ThreadSegments = segments
	item.Quotes = mergedQuotes
	item.References = mergedReferences
	item.Attachments = mergedAttachments
	item.Content = strings.Join(contents, "\n\n")
}

func (c *SyndicationHTTPClient) fetchSelfThreadIDs(ctx context.Context, focalTweetID string) ([]string, error) {
	payload, err := c.fetchTweetDetailGraphQL(ctx, focalTweetID)
	if err != nil {
		return nil, err
	}
	return extractSelfThreadIDsFromDetail(payload, focalTweetID), nil
}

func (c *SyndicationHTTPClient) fetchTweetDetailGraphQL(ctx context.Context, focalTweetID string) (map[string]any, error) {
	endpoint, err := url.Parse("https://x.com/i/api/graphql/rU08O-YiXdr0IZfE7qaUMg/TweetDetail")
	if err != nil {
		return nil, err
	}
	query := endpoint.Query()
	query.Set("variables", compactJSON(map[string]any{
		"focalTweetId":                           focalTweetID,
		"with_rux_injections":                    false,
		"includePromotedContent":                 true,
		"withCommunity":                          true,
		"withQuickPromoteEligibilityTweetFields": true,
		"withBirdwatchNotes":                     true,
		"withVoice":                              true,
	}))
	query.Set("features", compactJSON(map[string]bool{
		"rweb_video_screen_enabled":                                               true,
		"profile_label_improvements_pcf_label_in_post_enabled":                    true,
		"responsive_web_profile_redirect_enabled":                                 false,
		"rweb_tipjar_consumption_enabled":                                         true,
		"verified_phone_label_enabled":                                            false,
		"creator_subscriptions_tweet_preview_api_enabled":                         true,
		"responsive_web_graphql_timeline_navigation_enabled":                      true,
		"responsive_web_graphql_skip_user_profile_image_extensions_enabled":       false,
		"premium_content_api_read_enabled":                                        true,
		"communities_web_enable_tweet_community_results_fetch":                    true,
		"c9s_tweet_anatomy_moderator_badge_enabled":                               true,
		"responsive_web_grok_analyze_button_fetch_trends_enabled":                 true,
		"responsive_web_grok_analyze_post_followups_enabled":                      true,
		"responsive_web_jetfuel_frame":                                            true,
		"responsive_web_grok_share_attachment_enabled":                            true,
		"responsive_web_grok_annotations_enabled":                                 true,
		"articles_preview_enabled":                                                true,
		"responsive_web_edit_tweet_api_enabled":                                   true,
		"graphql_is_translatable_rweb_tweet_is_translatable_enabled":              true,
		"view_counts_everywhere_api_enabled":                                      true,
		"longform_notetweets_consumption_enabled":                                 true,
		"responsive_web_twitter_article_tweet_consumption_enabled":                true,
		"content_disclosure_indicator_enabled":                                    true,
		"content_disclosure_ai_generated_indicator_enabled":                       true,
		"responsive_web_grok_show_grok_translated_post":                           false,
		"responsive_web_grok_analysis_button_from_backend":                        true,
		"post_ctas_fetch_enabled":                                                 true,
		"freedom_of_speech_not_reach_fetch_enabled":                               true,
		"standardized_nudges_misinfo":                                             true,
		"tweet_with_visibility_results_prefer_gql_limited_actions_policy_enabled": true,
		"longform_notetweets_rich_text_read_enabled":                              true,
		"longform_notetweets_inline_media_enabled":                                true,
		"responsive_web_grok_image_annotation_enabled":                            true,
		"responsive_web_grok_imagine_annotation_enabled":                          true,
		"responsive_web_grok_community_note_auto_translation_is_enabled":          false,
		"responsive_web_enhance_cards_enabled":                                    false,
	}))
	query.Set("fieldToggles", compactJSON(longformFieldToggles()))
	endpoint.RawQuery = query.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("authorization", "Bearer "+webBearerToken)
	req.Header.Set("x-csrf-token", c.ct0)
	req.Header.Set("x-twitter-active-user", "yes")
	req.Header.Set("x-twitter-auth-type", "OAuth2Session")
	req.Header.Set("x-twitter-client-language", "en")
	req.Header.Set("cookie", "auth_token="+c.authToken+"; ct0="+c.ct0)
	req.Header.Set("user-agent", "Mozilla/5.0")

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("twitter thread detail fetch failed: status %d", resp.StatusCode)
	}
	if err := httputil.CheckContentLength(resp, 6<<20); err != nil {
		return nil, err
	}
	var decoded map[string]any
	if err := httputil.DecodeJSONLimited(resp.Body, 6<<20, &decoded); err != nil {
		return nil, err
	}
	return decoded, nil
}

func extractSelfThreadIDsFromDetail(payload map[string]any, focalTweetID string) []string {
	if payload == nil {
		return nil
	}
	type tweetLite struct {
		ID             string
		AuthorID       string
		ConversationID string
		InReplyTo      string
	}
	collected := make([]tweetLite, 0)
	seen := map[string]struct{}{}
	var walk func(any)
	walk = func(node any) {
		switch typed := node.(type) {
		case map[string]any:
			if typed["__typename"] == "Tweet" {
				id, _ := typed["rest_id"].(string)
				legacy, _ := typed["legacy"].(map[string]any)
				authorID, _ := legacy["user_id_str"].(string)
				conversationID, _ := legacy["conversation_id_str"].(string)
				inReplyTo, _ := legacy["in_reply_to_status_id_str"].(string)
				if id != "" {
					if _, ok := seen[id]; !ok {
						seen[id] = struct{}{}
						collected = append(collected, tweetLite{ID: id, AuthorID: authorID, ConversationID: conversationID, InReplyTo: inReplyTo})
					}
				}
			}
			for _, value := range typed {
				walk(value)
			}
		case []any:
			for _, value := range typed {
				walk(value)
			}
		}
	}
	walk(payload)

	var focal tweetLite
	for _, item := range collected {
		if item.ID == focalTweetID {
			focal = item
			break
		}
	}
	if focal.ID == "" || focal.AuthorID == "" || focal.ConversationID == "" {
		return nil
	}
	sameThread := make(map[string]tweetLite)
	for _, item := range collected {
		if item.AuthorID == focal.AuthorID && item.ConversationID == focal.ConversationID {
			sameThread[item.ID] = item
		}
	}
	if len(sameThread) <= 1 {
		return nil
	}

	rootID := focal.ConversationID
	root, ok := sameThread[rootID]
	if !ok {
		root = tweetLite{ID: rootID, AuthorID: focal.AuthorID, ConversationID: focal.ConversationID}
	}

	children := make(map[string][]tweetLite)
	for _, item := range sameThread {
		parent := item.InReplyTo
		children[parent] = append(children[parent], item)
	}

	ordered := make([]string, 0, len(sameThread))
	currentID := root.ID
	visited := make(map[string]struct{}, len(sameThread))
	for currentID != "" {
		if _, ok := visited[currentID]; ok {
			break
		}
		if _, ok := sameThread[currentID]; !ok && currentID != root.ID {
			break
		}
		visited[currentID] = struct{}{}
		ordered = append(ordered, currentID)

		nextChildren := children[currentID]
		if len(nextChildren) == 0 {
			break
		}
		nextID := ""
		for _, child := range nextChildren {
			if _, seen := visited[child.ID]; seen {
				continue
			}
			nextID = child.ID
			break
		}
		currentID = nextID
	}
	if len(ordered) <= 1 {
		return nil
	}
	return ordered
}

func rebaseStructuredPlaceholders(content string, quotes []types.Quote, references []types.Reference, attachments []types.Attachment, quoteOffset int, referenceOffset int, attachmentOffset int) string {
	out := content
	out = rebasePlaceholders(out, regexp.MustCompile(`\[引用#(\d+)(?: [^\]]+)?\]`), len(quotes), quoteOffset, func(globalIdx int, localIdx int) string {
		return assemble.FormatQuotePlaceholder(globalIdx, quotes[localIdx])
	})
	out = rebasePlaceholders(out, regexp.MustCompile(`\[参考#(\d+)(?: [^\]]+)?\]`), len(references), referenceOffset, func(globalIdx int, localIdx int) string {
		return assemble.FormatReferencePlaceholder(globalIdx, references[localIdx])
	})
	out = rebasePlaceholders(out, regexp.MustCompile(`\[附件#(\d+)(?: [^\]]+)?\]`), len(attachments), attachmentOffset, func(globalIdx int, localIdx int) string {
		return assemble.FormatAttachmentPlaceholder(globalIdx, attachments[localIdx])
	})
	return out
}

func rebasePlaceholders(content string, pattern *regexp.Regexp, count int, offset int, render func(globalIdx int, localIdx int) string) string {
	if count == 0 {
		return content
	}
	return pattern.ReplaceAllStringFunc(content, func(match string) string {
		sub := pattern.FindStringSubmatch(match)
		if len(sub) < 2 {
			return match
		}
		localParsed, err := strconv.Atoi(sub[1])
		if err != nil {
			return match
		}
		localNum := localParsed - 1
		if localNum < 0 || localNum >= count {
			return match
		}
		return render(offset+localNum+1, localNum)
	})
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
	raw.Content = refreshReferencePlaceholders(raw.Content, raw.References)
}

func refreshReferencePlaceholders(content string, references []types.Reference) string {
	out := content
	for i, ref := range references {
		pattern := regexp.MustCompile(`\[参考#` + fmt.Sprintf("%d", i+1) + ` [^\]]+\]`)
		out = pattern.ReplaceAllString(out, assemble.FormatReferencePlaceholder(i+1, ref))
	}
	out = regexp.MustCompile(`\[参考#\d+ [^\]]+\]`).ReplaceAllStringFunc(out, func(match string) string {
		for i := range references {
			expected := `[参考#` + fmt.Sprintf("%d", i+1)
			if strings.HasPrefix(match, expected) {
				return match
			}
		}
		return ""
	})
	out = regexp.MustCompile(`\n{3,}`).ReplaceAllString(out, "\n\n")
	out = strings.TrimSpace(out)
	return out
}
