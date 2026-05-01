package search

import (
	"context"
	"encoding/xml"
	"fmt"
	"html"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"github.com/kumaloha/VariX/varix/ingest/sources/httputil"
	"github.com/kumaloha/VariX/varix/ingest/types"
)

var anchorHref = regexp.MustCompile(`(?i)href=["']([^"'#]+)["']`)
var siteOperator = regexp.MustCompile(`(?i)(?:^|\s)site:([^\s"']+)`)

const maxResponseBytes int64 = 2 << 20 // 2 MB

type Searcher interface {
	Search(ctx context.Context, query string, options SearchOptions) (string, error)
}

type SearchOptions struct {
	TBS string
}

type Collector struct {
	platform   types.Platform
	siteFilter string
	searchers  []Searcher
	windows    []SearchOptions
}

type GoogleSearcher struct {
	client      *http.Client
	resultCount int
	dateSort    bool
}

type BingRSSSearcher struct {
	client      *http.Client
	resultCount int
}

func New(platform types.Platform, siteFilter string, searchers ...Searcher) *Collector {
	return &Collector{
		platform:   platform,
		siteFilter: strings.TrimSpace(siteFilter),
		searchers:  compactSearchers(searchers),
		windows: []SearchOptions{
			{TBS: "qdr:d,sbd:1"},
			{TBS: "qdr:w,sbd:1"},
			{TBS: "sbd:1"},
		},
	}
}

func NewGoogle(platform types.Platform, siteFilter string, client *http.Client) *Collector {
	return New(platform, siteFilter, NewGoogleSearcher(client), NewBingRSSSearcher(client))
}

func NewGoogleSearcher(client *http.Client) *GoogleSearcher {
	if client == nil {
		client = http.DefaultClient
	}
	return &GoogleSearcher{
		client:      client,
		resultCount: 20,
		dateSort:    true,
	}
}

func NewBingRSSSearcher(client *http.Client) *BingRSSSearcher {
	if client == nil {
		client = http.DefaultClient
	}
	return &BingRSSSearcher{
		client:      client,
		resultCount: 20,
	}
}

func (c *Collector) Kind() types.Kind {
	return types.KindSearch
}

func (c *Collector) Platform() types.Platform {
	return c.platform
}

func (c *Collector) Discover(ctx context.Context, target types.FollowTarget) ([]types.DiscoveryItem, error) {
	if len(c.searchers) == 0 {
		return nil, fmt.Errorf("no searcher configured")
	}

	query := strings.TrimSpace(target.Query)
	if query == "" {
		query = strings.TrimSpace(target.Locator)
	}
	if query == "" {
		return nil, fmt.Errorf("search target missing query")
	}
	if c.siteFilter != "" && !strings.Contains(query, "site:") {
		query = "site:" + c.siteFilter + " " + query
	}
	allowedHosts := allowedResultHosts(query, c.siteFilter)

	var lastErr error
	var urls []string
	for _, window := range c.windows {
		for _, searcher := range c.searchers {
			body, err := searcher.Search(ctx, query, window)
			if err != nil {
				lastErr = err
				continue
			}
			urls = filterResultURLs(extractResultURLs(body), allowedHosts, c.platform)
			if len(urls) > 0 {
				break
			}
		}
		if len(urls) > 0 {
			break
		}
	}
	if len(urls) == 0 {
		if lastErr != nil {
			return nil, lastErr
		}
		return nil, fmt.Errorf("search produced no usable result urls")
	}
	items := make([]types.DiscoveryItem, 0, len(urls))
	for _, itemURL := range urls {
		items = append(items, types.DiscoveryItem{
			Platform:      c.platform,
			URL:           itemURL,
			AuthorName:    target.AuthorName,
			HydrationHint: string(c.platform),
		})
	}
	return items, nil
}

func (s *GoogleSearcher) Search(ctx context.Context, query string, options SearchOptions) (string, error) {
	params := url.Values{}
	params.Set("hl", "en")
	params.Set("num", strconv.Itoa(s.resultCount))
	params.Set("gbv", "1")
	params.Set("q", query)
	switch {
	case options.TBS != "":
		params.Set("tbs", options.TBS)
	case s.dateSort:
		params.Set("tbs", "sbd:1")
	}
	endpoint := "https://www.google.com/search?" + params.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0")

	resp, err := s.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("google search failed: status %d", resp.StatusCode)
	}

	if err := httputil.CheckContentLength(resp, maxResponseBytes); err != nil {
		return "", err
	}
	body, err := httputil.LimitedReadAll(resp.Body, maxResponseBytes)
	if err != nil {
		return "", err
	}
	return string(body), nil
}

func (s *BingRSSSearcher) Search(ctx context.Context, query string, options SearchOptions) (string, error) {
	params := url.Values{}
	params.Set("format", "rss")
	params.Set("cc", "us")
	params.Set("mkt", "en-US")
	params.Set("setlang", "en-US")
	params.Set("count", strconv.Itoa(s.resultCount))
	params.Set("q", query)
	switch {
	case strings.Contains(options.TBS, "qdr:d"):
		params.Set("freshness", "Day")
	case strings.Contains(options.TBS, "qdr:w"):
		params.Set("freshness", "Week")
	}
	endpoint := "https://www.bing.com/search?" + params.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0")

	resp, err := s.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("bing rss search failed: status %d", resp.StatusCode)
	}

	if err := httputil.CheckContentLength(resp, maxResponseBytes); err != nil {
		return "", err
	}
	body, err := httputil.LimitedReadAll(resp.Body, maxResponseBytes)
	if err != nil {
		return "", err
	}
	return string(body), nil
}

func extractResultURLs(body string) []string {
	out := make([]string, 0)
	seen := make(map[string]struct{})
	appendURL := func(raw string) {
		itemURL := normalizeResultURL(raw)
		if itemURL == "" {
			return
		}
		if _, ok := seen[itemURL]; ok {
			return
		}
		seen[itemURL] = struct{}{}
		out = append(out, itemURL)
	}

	matches := anchorHref.FindAllStringSubmatch(body, -1)
	for _, match := range matches {
		if len(match) < 2 {
			continue
		}
		appendURL(match[1])
	}

	for _, link := range extractRSSLinks(body) {
		appendURL(link)
	}
	return out
}

func extractRSSLinks(body string) []string {
	type rss struct {
		Channel struct {
			Items []struct {
				Link string `xml:"link"`
			} `xml:"item"`
		} `xml:"channel"`
	}
	var feed rss
	if err := xml.Unmarshal([]byte(body), &feed); err != nil {
		return nil
	}
	out := make([]string, 0, len(feed.Channel.Items))
	for _, item := range feed.Channel.Items {
		if strings.TrimSpace(item.Link) != "" {
			out = append(out, item.Link)
		}
	}
	return out
}

func normalizeResultURL(raw string) string {
	raw = html.UnescapeString(strings.TrimSpace(raw))
	if raw == "" {
		return ""
	}

	if strings.HasPrefix(raw, "/url?") {
		u, err := url.Parse(raw)
		if err != nil {
			return ""
		}
		raw = u.Query().Get("q")
	}

	if !strings.HasPrefix(raw, "http://") && !strings.HasPrefix(raw, "https://") {
		return ""
	}

	u, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	host := strings.ToLower(u.Hostname())
	if isSearchEngineHost(host) {
		return ""
	}
	u.Fragment = ""
	return u.String()
}

func compactSearchers(searchers []Searcher) []Searcher {
	out := make([]Searcher, 0, len(searchers))
	for _, searcher := range searchers {
		if searcher != nil {
			out = append(out, searcher)
		}
	}
	return out
}

func allowedResultHosts(query string, siteFilter string) []string {
	out := make([]string, 0, 2)
	seen := make(map[string]struct{})
	add := func(raw string) {
		host := siteOperatorHost(raw)
		if host == "" {
			return
		}
		if _, ok := seen[host]; ok {
			return
		}
		seen[host] = struct{}{}
		out = append(out, host)
	}
	add(siteFilter)
	for _, match := range siteOperator.FindAllStringSubmatch(query, -1) {
		if len(match) > 1 {
			add(match[1])
		}
	}
	return out
}

func siteOperatorHost(raw string) string {
	raw = strings.Trim(strings.TrimSpace(raw), `"'`)
	raw = strings.TrimPrefix(raw, "http://")
	raw = strings.TrimPrefix(raw, "https://")
	raw = strings.TrimPrefix(raw, "www.")
	if idx := strings.Index(raw, "/"); idx >= 0 {
		raw = raw[:idx]
	}
	return strings.ToLower(strings.TrimSpace(raw))
}

func filterResultURLs(urls []string, allowedHosts []string, platform types.Platform) []string {
	out := make([]string, 0, len(urls))
	for _, raw := range urls {
		u, err := url.Parse(raw)
		if err != nil {
			continue
		}
		if len(allowedHosts) > 0 {
			host := strings.TrimPrefix(strings.ToLower(u.Hostname()), "www.")
			matched := false
			for _, allowed := range allowedHosts {
				if host == allowed || strings.HasSuffix(host, "."+allowed) {
					matched = true
					break
				}
			}
			if !matched {
				continue
			}
		}
		if !isUsableSearchResultURL(u, platform) {
			continue
		}
		out = append(out, raw)
	}
	return out
}

func isUsableSearchResultURL(u *url.URL, platform types.Platform) bool {
	if platform != types.PlatformWeb {
		return true
	}
	return isLikelyArticleURL(u)
}

func isLikelyArticleURL(u *url.URL) bool {
	segments := cleanPathSegments(u.EscapedPath())
	if len(segments) == 0 {
		return false
	}
	if isNonArticleSectionSegment(segments[0]) {
		return false
	}
	if len(segments) == 1 && isNonArticleSectionSegment(segments[0]) {
		return false
	}
	last := segments[len(segments)-1]
	if strings.HasSuffix(last, ".html") || strings.HasSuffix(last, ".htm") {
		return true
	}
	for _, segment := range segments {
		if isYearSegment(segment) {
			return true
		}
	}
	if len(segments) >= 2 && (segments[0] == "p" || segments[0] == "post" || segments[0] == "posts") {
		return true
	}
	return len(segments) >= 2 && strings.Contains(last, "-")
}

func cleanPathSegments(rawPath string) []string {
	path := strings.Trim(strings.ToLower(rawPath), "/")
	if path == "" {
		return nil
	}
	parts := strings.Split(path, "/")
	segments := parts[:0]
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			segments = append(segments, part)
		}
	}
	return segments
}

func isYearSegment(segment string) bool {
	if len(segment) != 4 {
		return false
	}
	year, err := strconv.Atoi(segment)
	return err == nil && year >= 1900 && year <= 2099
}

func isNonArticleSectionSegment(segment string) bool {
	switch segment {
	case "about", "archive", "archives", "author", "authors", "category", "categories",
		"contact", "feed", "home", "login", "newsletter", "newsletters", "page", "pages",
		"privacy", "rss", "search", "subscribe", "tag", "tags", "terms", "topic", "topics":
		return true
	default:
		return false
	}
}

func isSearchEngineHost(host string) bool {
	switch {
	case host == "google.com" || strings.HasSuffix(host, ".google.com"):
		return true
	case host == "bing.com" || strings.HasSuffix(host, ".bing.com"):
		return true
	case host == "duckduckgo.com" || strings.HasSuffix(host, ".duckduckgo.com"):
		return true
	default:
		return false
	}
}
