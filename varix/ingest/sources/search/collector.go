package search

import (
	"context"
	"fmt"
	"html"
	"net/http"
	"net/url"
	"regexp"
	"strings"

	"github.com/kumaloha/VariX/varix/ingest/sources/httputil"
	"github.com/kumaloha/VariX/varix/ingest/types"
)

var anchorHref = regexp.MustCompile(`(?i)href=["']([^"'#]+)["']`)

const maxResponseBytes int64 = 2 << 20 // 2 MB

type Searcher interface {
	Search(ctx context.Context, query string) (string, error)
}

type Collector struct {
	platform   types.Platform
	siteFilter string
	searcher   Searcher
}

type GoogleSearcher struct {
	client *http.Client
}

func New(platform types.Platform, siteFilter string, searcher Searcher) *Collector {
	return &Collector{
		platform:   platform,
		siteFilter: strings.TrimSpace(siteFilter),
		searcher:   searcher,
	}
}

func NewGoogle(platform types.Platform, siteFilter string, client *http.Client) *Collector {
	return New(platform, siteFilter, NewGoogleSearcher(client))
}

func NewGoogleSearcher(client *http.Client) *GoogleSearcher {
	if client == nil {
		client = http.DefaultClient
	}
	return &GoogleSearcher{client: client}
}

func (c *Collector) Kind() types.Kind {
	return types.KindSearch
}

func (c *Collector) Platform() types.Platform {
	return c.platform
}

func (c *Collector) Discover(ctx context.Context, target types.FollowTarget) ([]types.DiscoveryItem, error) {
	if c.searcher == nil {
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

	body, err := c.searcher.Search(ctx, query)
	if err != nil {
		return nil, err
	}

	urls := extractResultURLs(body)
	if len(urls) == 0 {
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

func (s *GoogleSearcher) Search(ctx context.Context, query string) (string, error) {
	endpoint := "https://www.google.com/search?hl=en&num=10&gbv=1&q=" + url.QueryEscape(query)
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

func extractResultURLs(body string) []string {
	matches := anchorHref.FindAllStringSubmatch(body, -1)
	if len(matches) == 0 {
		return nil
	}

	out := make([]string, 0, len(matches))
	seen := make(map[string]struct{}, len(matches))
	for _, match := range matches {
		if len(match) < 2 {
			continue
		}
		itemURL := normalizeResultURL(match[1])
		if itemURL == "" {
			continue
		}
		if _, ok := seen[itemURL]; ok {
			continue
		}
		seen[itemURL] = struct{}{}
		out = append(out, itemURL)
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
	if host == "google.com" || strings.HasSuffix(host, ".google.com") {
		return ""
	}
	u.Fragment = ""
	return u.String()
}
