package search

import (
	"context"
	"encoding/xml"
	"fmt"
	"html"
	"net/http"
	"net/url"
	"reflect"
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

type DuckDuckGoHTMLSearcher struct {
	client *http.Client
}

type NitterAuthorRSSSearcher struct {
	client *http.Client
}

type BingRSSSearcher struct {
	client      *http.Client
	resultCount int
}

type resultConstraint struct {
	host       string
	pathPrefix string
}

type NoUsableResultsError struct {
	Query    string
	Attempts []ProviderAttempt
	LastErr  error
}

type ProviderAttempt struct {
	Provider                  string
	Window                    string
	ExtractedCount            int
	RejectedByConstraintCount int
	RejectedByUsabilityCount  int
	UsableCount               int
	BlockedOrUnparseable      bool
	Error                     string
}

func (e *NoUsableResultsError) Error() string {
	if e == nil {
		return ""
	}
	classification := e.classification()
	parts := make([]string, 0, len(e.Attempts))
	for _, attempt := range e.Attempts {
		detail := fmt.Sprintf("%s/%s extracted=%d usable=%d constraint_rejected=%d usability_rejected=%d",
			attempt.Provider,
			attempt.Window,
			attempt.ExtractedCount,
			attempt.UsableCount,
			attempt.RejectedByConstraintCount,
			attempt.RejectedByUsabilityCount,
		)
		if attempt.BlockedOrUnparseable {
			detail += " blocked_or_unparseable=true"
		}
		if attempt.Error != "" {
			detail += " error=" + attempt.Error
		}
		parts = append(parts, detail)
	}
	message := fmt.Sprintf("search %s: query=%q", classification, e.Query)
	if len(parts) > 0 {
		message += " attempts=[" + strings.Join(parts, "; ") + "]"
	}
	if e.LastErr != nil {
		message += " last_error=" + e.LastErr.Error()
	}
	return message
}

func (e *NoUsableResultsError) classification() string {
	hasProviderError := false
	hasBlockedOrUnparseable := false
	hasFiltered := false
	for _, attempt := range e.Attempts {
		if attempt.Error != "" {
			hasProviderError = true
		}
		if attempt.BlockedOrUnparseable {
			hasBlockedOrUnparseable = true
		}
		if attempt.RejectedByConstraintCount > 0 || attempt.RejectedByUsabilityCount > 0 {
			hasFiltered = true
		}
	}
	switch {
	case hasBlockedOrUnparseable:
		return "provider_blocked_or_unparseable"
	case hasFiltered:
		return "provider_results_filtered"
	case hasProviderError:
		return "provider_error"
	default:
		return "provider_empty_result"
	}
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
	searchers := []Searcher{NewGoogleSearcher(client)}
	if platform == types.PlatformTwitter {
		searchers = append(searchers, NewNitterAuthorRSSSearcher(client))
	}
	searchers = append(searchers, NewDuckDuckGoHTMLSearcher(client), NewBingRSSSearcher(client))
	return New(platform, siteFilter, searchers...)
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

func NewDuckDuckGoHTMLSearcher(client *http.Client) *DuckDuckGoHTMLSearcher {
	if client == nil {
		client = http.DefaultClient
	}
	return &DuckDuckGoHTMLSearcher{client: client}
}

func NewNitterAuthorRSSSearcher(client *http.Client) *NitterAuthorRSSSearcher {
	if client == nil {
		client = http.DefaultClient
	}
	return &NitterAuthorRSSSearcher{client: client}
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
	resultConstraints := allowedResultConstraints(query, c.siteFilter)

	var lastErr error
	var urls []string
	attempts := make([]ProviderAttempt, 0, len(c.windows)*len(c.searchers))
	for _, window := range c.windows {
		for _, searcher := range c.searchers {
			attempt := ProviderAttempt{
				Provider: searcherName(searcher),
				Window:   searchWindowLabel(window),
			}
			body, err := searcher.Search(ctx, query, window)
			if err != nil {
				lastErr = err
				attempt.Error = err.Error()
				attempts = append(attempts, attempt)
				continue
			}
			extracted := extractResultURLs(body)
			attempt.ExtractedCount = len(extracted)
			attempt.BlockedOrUnparseable = isBlockedOrUnparseableSearchBody(body, extracted)
			var rejectedByConstraint int
			var rejectedByUsability int
			urls, rejectedByConstraint, rejectedByUsability = filterResultURLsWithStats(extracted, resultConstraints, c.platform)
			attempt.RejectedByConstraintCount = rejectedByConstraint
			attempt.RejectedByUsabilityCount = rejectedByUsability
			attempt.UsableCount = len(urls)
			attempts = append(attempts, attempt)
			if len(urls) > 0 {
				break
			}
		}
		if len(urls) > 0 {
			break
		}
	}
	if len(urls) == 0 {
		return nil, &NoUsableResultsError{
			Query:    query,
			Attempts: attempts,
			LastErr:  lastErr,
		}
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

func searcherName(searcher Searcher) string {
	if searcher == nil {
		return "nil"
	}
	t := reflect.TypeOf(searcher)
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	if t.Name() == "" {
		return t.String()
	}
	return t.Name()
}

func searchWindowLabel(options SearchOptions) string {
	if strings.TrimSpace(options.TBS) == "" {
		return "default"
	}
	return options.TBS
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

func (s *DuckDuckGoHTMLSearcher) Search(ctx context.Context, query string, options SearchOptions) (string, error) {
	params := url.Values{}
	params.Set("q", query)
	endpoint := "https://html.duckduckgo.com/html/?" + params.Encode()
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
		return "", fmt.Errorf("duckduckgo html search failed: status %d", resp.StatusCode)
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

func (s *NitterAuthorRSSSearcher) Search(ctx context.Context, query string, options SearchOptions) (string, error) {
	author := twitterAuthorFromSiteQuery(query)
	if author == "" {
		return `<rss><channel></channel></rss>`, nil
	}
	endpoint := "https://nitter.net/" + url.PathEscape(author) + "/rss"
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
		return "", fmt.Errorf("nitter author rss failed: status %d", resp.StatusCode)
	}

	if err := httputil.CheckContentLength(resp, maxResponseBytes); err != nil {
		return "", err
	}
	body, err := httputil.LimitedReadAll(resp.Body, maxResponseBytes)
	if err != nil {
		return "", err
	}
	return rewriteNitterStatusLinksToX(string(body)), nil
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

func twitterAuthorFromSiteQuery(query string) string {
	for _, constraint := range allowedResultConstraints(query, "") {
		if constraint.host != "x.com" && constraint.host != "twitter.com" {
			continue
		}
		segments := cleanPathSegments(constraint.pathPrefix)
		if len(segments) >= 2 && segments[1] == "status" && isLikelyTwitterAuthorID(segments[0]) {
			return segments[0]
		}
	}
	return ""
}

func isLikelyTwitterAuthorID(value string) bool {
	if value == "" || len(value) > 15 {
		return false
	}
	for _, r := range value {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '_' {
			continue
		}
		return false
	}
	return true
}

func rewriteNitterStatusLinksToX(body string) string {
	links := extractRSSLinks(body)
	if len(links) == 0 {
		return body
	}
	var b strings.Builder
	b.WriteString("<rss><channel>")
	for _, link := range links {
		rewritten := nitterStatusLinkToX(link)
		if rewritten == "" {
			continue
		}
		b.WriteString("<item><link>")
		b.WriteString(html.EscapeString(rewritten))
		b.WriteString("</link></item>")
	}
	b.WriteString("</channel></rss>")
	return b.String()
}

func nitterStatusLinkToX(raw string) string {
	u, err := url.Parse(html.UnescapeString(strings.TrimSpace(raw)))
	if err != nil {
		return ""
	}
	host := strings.TrimPrefix(strings.ToLower(u.Hostname()), "www.")
	if host != "nitter.net" && !strings.HasSuffix(host, ".nitter.net") {
		return ""
	}
	segments := cleanPathSegments(u.EscapedPath())
	if len(segments) < 3 || segments[1] != "status" || !isLikelyTwitterAuthorID(segments[0]) {
		return ""
	}
	if _, err := strconv.ParseInt(segments[2], 10, 64); err != nil {
		return ""
	}
	return "https://x.com/" + segments[0] + "/status/" + segments[2]
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
	if strings.HasPrefix(raw, "/l/?") || strings.HasPrefix(raw, "/l?") {
		u, err := url.Parse(raw)
		if err != nil {
			return ""
		}
		raw = u.Query().Get("uddg")
	}
	if strings.HasPrefix(raw, "//") {
		raw = "https:" + raw
	}

	if !strings.HasPrefix(raw, "http://") && !strings.HasPrefix(raw, "https://") {
		return ""
	}

	u, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	if isDuckDuckGoRedirectURL(u) {
		return normalizeResultURL(u.Query().Get("uddg"))
	}
	host := strings.ToLower(u.Hostname())
	if isSearchEngineHost(host) {
		return ""
	}
	u.Fragment = ""
	return u.String()
}

func isDuckDuckGoRedirectURL(u *url.URL) bool {
	host := strings.TrimPrefix(strings.ToLower(u.Hostname()), "www.")
	return (host == "duckduckgo.com" || host == "html.duckduckgo.com") &&
		strings.HasPrefix(strings.ToLower(u.EscapedPath()), "/l/") &&
		strings.TrimSpace(u.Query().Get("uddg")) != ""
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

func allowedResultConstraints(query string, siteFilter string) []resultConstraint {
	out := make([]resultConstraint, 0, 2)
	seen := make(map[string]struct{})
	add := func(raw string) {
		constraint := siteOperatorConstraint(raw)
		if constraint.host == "" {
			return
		}
		key := constraint.host + constraint.pathPrefix
		if _, ok := seen[key]; ok {
			return
		}
		seen[key] = struct{}{}
		out = append(out, constraint)
	}
	matches := siteOperator.FindAllStringSubmatch(query, -1)
	if len(matches) == 0 {
		add(siteFilter)
	}
	for _, match := range matches {
		if len(match) > 1 {
			add(match[1])
		}
	}
	return out
}

func siteOperatorConstraint(raw string) resultConstraint {
	raw = strings.Trim(strings.TrimSpace(raw), `"'`)
	raw = strings.TrimPrefix(raw, "http://")
	raw = strings.TrimPrefix(raw, "https://")
	raw = strings.TrimPrefix(raw, "www.")
	var pathPrefix string
	if idx := strings.Index(raw, "/"); idx >= 0 {
		pathPrefix = "/" + strings.Trim(raw[idx+1:], "/")
		raw = raw[:idx]
	}
	host := strings.ToLower(strings.TrimSpace(raw))
	if pathPrefix != "" {
		pathPrefix = strings.ToLower(pathPrefix)
	}
	return resultConstraint{host: host, pathPrefix: pathPrefix}
}

func filterResultURLs(urls []string, constraints []resultConstraint, platform types.Platform) []string {
	out, _, _ := filterResultURLsWithStats(urls, constraints, platform)
	return out
}

func filterResultURLsWithStats(urls []string, constraints []resultConstraint, platform types.Platform) ([]string, int, int) {
	out := make([]string, 0, len(urls))
	rejectedByConstraint := 0
	rejectedByUsability := 0
	for _, raw := range urls {
		u, err := url.Parse(raw)
		if err != nil {
			rejectedByUsability++
			continue
		}
		if len(constraints) > 0 && !matchesResultConstraints(u, constraints, platform) {
			rejectedByConstraint++
			continue
		}
		if !isUsableSearchResultURL(u, platform) {
			rejectedByUsability++
			continue
		}
		out = append(out, raw)
	}
	return out, rejectedByConstraint, rejectedByUsability
}

func isBlockedOrUnparseableSearchBody(body string, extracted []string) bool {
	if len(extracted) > 0 {
		return false
	}
	normalized := strings.ToLower(body)
	return strings.Contains(normalized, "trouble accessing google search") ||
		strings.Contains(normalized, "emsg=sg_rel") ||
		strings.Contains(normalized, "our systems have detected unusual traffic") ||
		strings.Contains(normalized, "challenge-form") ||
		strings.Contains(normalized, "verifying your browser") ||
		strings.Contains(normalized, "making sure you're not a bot")
}

func matchesResultConstraints(u *url.URL, constraints []resultConstraint, platform types.Platform) bool {
	host := strings.TrimPrefix(strings.ToLower(u.Hostname()), "www.")
	path := strings.ToLower(strings.TrimRight(u.EscapedPath(), "/"))
	for _, constraint := range constraints {
		if !matchesConstraintHost(host, constraint.host, platform) {
			continue
		}
		prefix := strings.TrimRight(constraint.pathPrefix, "/")
		if prefix == "" || path == prefix || strings.HasPrefix(path, prefix+"/") {
			return true
		}
	}
	return false
}

func matchesConstraintHost(host string, constraintHost string, platform types.Platform) bool {
	if host == constraintHost || strings.HasSuffix(host, "."+constraintHost) {
		return true
	}
	if platform == types.PlatformTwitter && isTwitterEquivalentHost(host) && isTwitterEquivalentHost(constraintHost) {
		return true
	}
	return false
}

func isTwitterEquivalentHost(host string) bool {
	host = strings.TrimPrefix(strings.ToLower(host), "www.")
	return host == "x.com" || host == "twitter.com"
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
