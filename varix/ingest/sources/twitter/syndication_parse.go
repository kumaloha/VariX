package twitter

import (
	"fmt"
	"strings"
	"time"

	"github.com/kumaloha/VariX/varix/ingest/assemble"
	"github.com/kumaloha/VariX/varix/ingest/normalize"
	"github.com/kumaloha/VariX/varix/ingest/sources/httputil"
	"github.com/kumaloha/VariX/varix/ingest/types"
)

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
