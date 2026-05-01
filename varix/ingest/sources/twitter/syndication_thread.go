package twitter

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"github.com/kumaloha/VariX/varix/ingest/assemble"
	"github.com/kumaloha/VariX/varix/ingest/sources/httputil"
	"github.com/kumaloha/VariX/varix/ingest/types"
)

func (c *SyndicationHTTPClient) hydrateThread(ctx context.Context, item *types.RawContent) {
	if item == nil || !c.hasAuthTokens() {
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

func (c *SyndicationHTTPClient) hasAuthTokens() bool {
	authToken := strings.TrimSpace(c.authToken)
	ct0 := strings.TrimSpace(c.ct0)
	return authToken != "" && ct0 != ""
}

func (c *SyndicationHTTPClient) applyAuthHeaders(req *http.Request, bearerToken string) {
	if req == nil {
		return
	}
	authToken := strings.TrimSpace(c.authToken)
	ct0 := strings.TrimSpace(c.ct0)
	req.Header.Set("authorization", "Bearer "+bearerToken)
	req.Header.Set("x-csrf-token", ct0)
	req.Header.Set("x-twitter-active-user", "yes")
	req.Header.Set("x-twitter-auth-type", "OAuth2Session")
	req.Header.Set("x-twitter-client-language", "en")
	req.Header.Set("cookie", "auth_token="+authToken+"; ct0="+ct0)
	req.Header.Set("user-agent", "Mozilla/5.0")
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
