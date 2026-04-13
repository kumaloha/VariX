package normalize

import (
	"html"
	"regexp"
	"strings"
)

var (
	htmlComment = regexp.MustCompile(`(?is)<!--.*?-->`)
	htmlScript  = regexp.MustCompile(`(?is)<script\b[^>]*>.*?</script>`)
	htmlStyle   = regexp.MustCompile(`(?is)<style\b[^>]*>.*?</style>`)
	htmlTags    = regexp.MustCompile(`(?is)<[^>]+>`)
	blockTags   = regexp.MustCompile(`(?is)</?(?:article|section|div|p|br|li|ul|ol|h[1-6]|blockquote|main|body|html)[^>]*>`)
)

func HTMLToText(body string) string {
	body = htmlComment.ReplaceAllString(body, " ")
	body = htmlScript.ReplaceAllString(body, " ")
	body = htmlStyle.ReplaceAllString(body, " ")
	body = blockTags.ReplaceAllString(body, "\n")
	body = htmlTags.ReplaceAllString(body, " ")
	body = html.UnescapeString(body)

	lines := strings.Split(body, "\n")
	return JoinParagraphs(lines)
}
