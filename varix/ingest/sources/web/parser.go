package web

import (
	"crypto/md5"
	"encoding/hex"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/kumaloha/VariX/varix/ingest/assemble"
	"github.com/kumaloha/VariX/varix/ingest/normalize"
	"github.com/kumaloha/VariX/varix/ingest/types"
)

var (
	arxivPattern               = regexp.MustCompile(`(?i)^https?://(?:www\.)?arxiv\.org/(?:abs|pdf)/(\d{4}\.\d{4,5}(?:v\d+)?)`)
	readerTitle                = regexp.MustCompile(`(?m)^Title:\s*(.+)$`)
	readerPublished            = regexp.MustCompile(`(?m)^Published Time:\s*(.+)$`)
	readerContent              = regexp.MustCompile(`(?s)Markdown Content:\s*\n(.*)$`)
	htmlTitle                  = regexp.MustCompile(`(?is)<title[^>]*>(.*?)</title>`)
	htmlAuthorMeta             = regexp.MustCompile(`(?is)<meta[^>]+(?:name|property)=["'](?:author|article:author)["'][^>]+content=["'](.*?)["']`)
	htmlPublishedMeta          = regexp.MustCompile(`(?is)<meta[^>]+(?:name|property)=["'](?:article:published_time|og:published_time|pubdate)["'][^>]+content=["'](.*?)["']`)
	htmlCanonical              = regexp.MustCompile(`(?is)<link[^>]+rel=["'][^"']*canonical[^"']*["'][^>]+href=["'](.*?)["']`)
	htmlShareholderLetter      = regexp.MustCompile(`(?is)<h2[^>]*>\s*(?:<span[^>]*>)?\s*Dear Fellow Shareholders,\s*(?:</span>)?\s*</h2>(.*?)(?:<article\b[^>]*class=["'][^"']*jpmc-infographic[^"']*["']|<div\b[^>]*class=["'][^"']*infographic[^"']*["']|</body>)`)
	htmlArticle                = regexp.MustCompile(`(?is)<article\b[^>]*>(.*?)</article>`)
	htmlBody                   = regexp.MustCompile(`(?is)<body\b[^>]*>(.*?)</body>`)
	htmlImage                  = regexp.MustCompile(`(?is)<img[^>]+src=["'](.*?)["']`)
	authorLabel                = regexp.MustCompile(`(?mi)^(?:Author|By|Source)[：:\s]+(.+)$`)
	youtubeURL                 = regexp.MustCompile(`(?i)https?://(?:www\.)?(?:youtube\.com/(?:watch\?(?:.*&)?v=|embed/|shorts/)|youtu\.be/)([A-Za-z0-9_-]{11})`)
	youtubeEmbed               = regexp.MustCompile(`(?i)https?://(?:www\.)?youtube\.com/embed/([A-Za-z0-9_-]{11})`)
	trackingQueryParamPrefixes = []string{"utm_"}
)

func arxivToAr5iv(raw string) string {
	match := arxivPattern.FindStringSubmatch(strings.TrimSpace(raw))
	if len(match) < 2 {
		return raw
	}
	return "https://ar5iv.labs.arxiv.org/html/" + match[1]
}

func parseReaderMarkdown(rawURL, externalID, body string) types.RawContent {
	title := firstMatch(readerTitle, body)
	postedAt := parseTime(firstMatch(readerPublished, body))
	authorName := extractAuthor(body, rawURL)
	contentRaw := body
	if match := readerContent.FindStringSubmatch(body); len(match) > 1 {
		contentRaw = strings.TrimSpace(match[1])
	}
	contentText := normalize.JoinParagraphs(strings.Split(contentRaw, "\n"))
	youtubeRedirect := extractYouTubeRedirect(body)
	if contentText == "" {
		contentText = title
	}

	raw := types.RawContent{
		Source:     "web",
		ExternalID: fallbackExternalID(externalID, rawURL),
		Content:    contentText,
		AuthorName: authorName,
		AuthorID:   authorName,
		URL:        rawURL,
		PostedAt:   postedAt,
		Metadata: types.RawMetadata{
			Web: &types.WebMetadata{
				Title:     title,
				SourceURL: rawURL,
			},
		},
	}
	if youtubeRedirect != "" {
		raw.Metadata.Web.YouTubeRedirect = youtubeRedirect
	}
	return raw
}

func parseHTMLDocument(rawURL, externalID, body string) types.RawContent {
	title := normalize.CollapseWhitespace(firstMatch(htmlTitle, body))
	authorName := normalize.CollapseWhitespace(firstMatch(htmlAuthorMeta, body))
	if authorName == "" {
		authorName = extractAuthor(body, rawURL)
	}
	postedAt := parseTime(firstMatch(htmlPublishedMeta, body))

	canonicalURL := rawURL
	if candidate := strings.TrimSpace(firstMatch(htmlCanonical, body)); candidate != "" {
		if resolved := resolveCanonical(rawURL, candidate); resolved != "" {
			canonicalURL = resolved
		}
	}

	articleHTML := firstMatch(htmlShareholderLetter, body)
	if articleHTML == "" {
		articleHTML = firstMatch(htmlArticle, body)
	}
	if articleHTML == "" {
		articleHTML = firstMatch(htmlBody, body)
	}
	if articleHTML == "" {
		articleHTML = body
	}
	text := normalize.HTMLToText(articleHTML)
	if text == "" {
		text = title
	}

	youtubeRedirect := extractYouTubeRedirect(body)
	metadata := types.RawMetadata{
		Web: &types.WebMetadata{
			Title:        title,
			SourceURL:    rawURL,
			CanonicalURL: canonicalURL,
		},
	}
	if youtubeRedirect != "" {
		metadata.Web.YouTubeRedirect = youtubeRedirect
	}

	attachments := extractAttachments(rawURL, body)

	return types.RawContent{
		Source:      "web",
		ExternalID:  fallbackExternalID(externalID, canonicalURL),
		Content:     assemble.AssembleStructuredContent(text, nil, nil, attachments),
		AuthorName:  authorName,
		AuthorID:    authorName,
		URL:         canonicalURL,
		PostedAt:    postedAt,
		Metadata:    metadata,
		Attachments: attachments,
	}
}

func extractAuthor(body, rawURL string) string {
	if match := authorLabel.FindStringSubmatch(body); len(match) > 1 {
		return normalize.CollapseWhitespace(match[1])
	}
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}
	host := strings.TrimPrefix(strings.ToLower(parsed.Hostname()), "www.")
	if host == "" {
		return ""
	}
	parts := strings.Split(host, ".")
	return parts[0]
}

func extractYouTubeRedirect(body string) string {
	if match := youtubeURL.FindStringSubmatch(body); len(match) > 1 {
		return "https://www.youtube.com/watch?v=" + match[1]
	}
	if match := youtubeEmbed.FindStringSubmatch(body); len(match) > 1 {
		return "https://www.youtube.com/watch?v=" + match[1]
	}
	return ""
}

func extractAttachments(baseURL, body string) []types.Attachment {
	matches := htmlImage.FindAllStringSubmatch(body, -1)
	if len(matches) == 0 {
		return nil
	}
	items := make([]types.Attachment, 0, len(matches))
	seen := make(map[string]struct{}, len(matches))
	for _, match := range matches {
		if len(match) < 2 {
			continue
		}
		src := strings.TrimSpace(match[1])
		if src == "" || strings.HasPrefix(strings.ToLower(src), "data:") {
			continue
		}
		resolved := resolveCanonical(baseURL, src)
		if resolved == "" {
			continue
		}
		if _, ok := seen[resolved]; ok {
			continue
		}
		seen[resolved] = struct{}{}
		items = append(items, types.Attachment{Type: "image", URL: resolved})
	}
	return items
}

func parseTime(raw string) time.Time {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return time.Time{}
	}
	layouts := []string{
		time.RFC3339,
		time.RFC3339Nano,
		time.RFC1123Z,
		time.RFC1123,
		time.RFC822Z,
		time.RFC822,
		"2006-01-02 15:04:05",
	}
	for _, layout := range layouts {
		if parsed, err := time.Parse(layout, raw); err == nil {
			return parsed.UTC()
		}
	}
	return time.Time{}
}

func firstMatch(pattern *regexp.Regexp, body string) string {
	match := pattern.FindStringSubmatch(body)
	if len(match) < 2 {
		return ""
	}
	return strings.TrimSpace(match[1])
}

func resolveCanonical(baseURL, candidate string) string {
	parsed, err := url.Parse(candidate)
	if err != nil {
		return ""
	}
	// Already absolute
	if parsed.IsAbs() {
		if parsed.Scheme == "http" || parsed.Scheme == "https" {
			return candidate
		}
		return "" // reject non-http schemes
	}
	// Relative — resolve against base
	base, err := url.Parse(baseURL)
	if err != nil {
		return ""
	}
	resolved := base.ResolveReference(parsed)
	return resolved.String()
}

func fallbackExternalID(externalID, rawURL string) string {
	if externalID != "" {
		return externalID
	}
	sum := md5.Sum([]byte(strings.TrimSpace(rawURL)))
	return hex.EncodeToString(sum[:])[:16]
}
