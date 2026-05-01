package twitter

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/kumaloha/VariX/varix/ingest/sources/httputil"
)

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
	if !c.hasAuthTokens() {
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
	c.applyAuthHeaders(req, webBearerToken)

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
	if !c.hasAuthTokens() {
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
	c.applyAuthHeaders(req, webBearerToken)

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
