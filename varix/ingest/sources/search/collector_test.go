package search

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/kumaloha/VariX/varix/ingest/types"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

type fakeSearcher struct {
	query   string
	options []SearchOptions
	bodies  []string
	body    string
	err     error
	calls   int
}

func (f *fakeSearcher) Search(_ context.Context, query string, options SearchOptions) (string, error) {
	f.calls++
	f.query = query
	f.options = append(f.options, options)
	if len(f.bodies) > 0 {
		body := f.bodies[0]
		f.bodies = f.bodies[1:]
		return body, f.err
	}
	return f.body, f.err
}

func TestCollector_DiscoverPrefixesSiteFilterAndExtractsURLs(t *testing.T) {
	searcher := &fakeSearcher{
		body: `
<html>
  <body>
    <a href="/url?q=https://x.com/a/status/123&sa=U">tweet</a>
    <a href="/url?q=https://x.com/a/status/123&sa=U">duplicate</a>
    <a href="/url?q=https://x.com/b/status/456&sa=U">tweet2</a>
    <a href="https://www.google.com/search?q=ignored">ignored</a>
  </body>
</html>`,
	}
	c := New(types.PlatformTwitter, "x.com", searcher)

	got, err := c.Discover(context.Background(), types.FollowTarget{
		Kind:     types.KindSearch,
		Platform: "twitter",
		Query:    "nvda",
		Locator:  "nvda",
	})
	if err != nil {
		t.Fatalf("Discover() error = %v", err)
	}
	if searcher.query != "site:x.com nvda" {
		t.Fatalf("query = %q, want %q", searcher.query, "site:x.com nvda")
	}
	if len(got) != 2 {
		t.Fatalf("len(got) = %d, want 2", len(got))
	}
	if got[0].URL != "https://x.com/a/status/123" {
		t.Fatalf("got[0].URL = %q, want first direct result", got[0].URL)
	}
	if got[0].Platform != types.PlatformTwitter {
		t.Fatalf("got[0].Platform = %q, want twitter", got[0].Platform)
	}
	if got[0].HydrationHint != "twitter" {
		t.Fatalf("got[0].HydrationHint = %q, want twitter", got[0].HydrationHint)
	}
}

func TestCollector_DiscoverUsesLocatorWhenQueryMissing(t *testing.T) {
	searcher := &fakeSearcher{
		body: `<a href="/url?q=https://www.youtube.com/watch?v=dQw4w9WgXcQ&sa=U">video</a>`,
	}
	c := New(types.PlatformYouTube, "youtube.com", searcher)

	got, err := c.Discover(context.Background(), types.FollowTarget{
		Kind:     types.KindSearch,
		Platform: "youtube",
		Locator:  "semis macro",
	})
	if err != nil {
		t.Fatalf("Discover() error = %v", err)
	}
	if searcher.query != "site:youtube.com semis macro" {
		t.Fatalf("query = %q", searcher.query)
	}
	if len(got) != 1 {
		t.Fatalf("len(got) = %d, want 1", len(got))
	}
	if got[0].URL != "https://www.youtube.com/watch?v=dQw4w9WgXcQ" {
		t.Fatalf("URL = %q", got[0].URL)
	}
}

func TestCollector_DiscoverReturnsErrorWhenNoUsableLinksExtracted(t *testing.T) {
	searcher := &fakeSearcher{
		body: `<html><body><a href="https://www.google.com/search?q=still-google">ignored</a></body></html>`,
	}
	c := New(types.PlatformTwitter, "x.com", searcher)

	_, err := c.Discover(context.Background(), types.FollowTarget{
		Kind:     types.KindSearch,
		Platform: "twitter",
		Query:    "nvda",
	})
	if err == nil {
		t.Fatal("Discover() error = nil, want no usable links error")
	}
}

func TestCollector_DiscoverFallsBackAcrossDateWindows(t *testing.T) {
	searcher := &fakeSearcher{
		bodies: []string{
			`<html><body>no result today</body></html>`,
			`<a href="/url?q=https://x.com/a/status/456&sa=U">tweet</a>`,
		},
	}
	c := New(types.PlatformTwitter, "x.com", searcher)

	got, err := c.Discover(context.Background(), types.FollowTarget{
		Kind:     types.KindSearch,
		Platform: "twitter",
		Query:    "nvda",
	})
	if err != nil {
		t.Fatalf("Discover() error = %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("len(got) = %d, want 1", len(got))
	}
	if len(searcher.options) != 2 {
		t.Fatalf("search calls = %d, want 2", len(searcher.options))
	}
	if searcher.options[0].TBS != "qdr:d,sbd:1" || searcher.options[1].TBS != "qdr:w,sbd:1" {
		t.Fatalf("windows = %+v", searcher.options)
	}
}

func TestCollector_DiscoverFallsBackAcrossProviders(t *testing.T) {
	google := &fakeSearcher{err: errBoom}
	bing := &fakeSearcher{
		body: `<rss><channel><item><link>https://x.com/a/status/789</link></item></channel></rss>`,
	}
	c := New(types.PlatformTwitter, "x.com", google, bing)

	got, err := c.Discover(context.Background(), types.FollowTarget{
		Kind:     types.KindSearch,
		Platform: "twitter",
		Query:    "nvda",
	})
	if err != nil {
		t.Fatalf("Discover() error = %v", err)
	}
	if len(got) != 1 || got[0].URL != "https://x.com/a/status/789" {
		t.Fatalf("got = %#v", got)
	}
	if google.calls != 1 || bing.calls != 1 {
		t.Fatalf("provider calls google=%d bing=%d, want 1/1", google.calls, bing.calls)
	}
}

func TestCollector_DiscoverFiltersProviderResultsToSiteOperators(t *testing.T) {
	searcher := &fakeSearcher{
		body: `<rss><channel>
<item><link>https://irrelevant.example/post</link></item>
<item><link>https://x.com/a/status/789</link></item>
</channel></rss>`,
	}
	c := New(types.PlatformTwitter, "x.com", searcher)

	got, err := c.Discover(context.Background(), types.FollowTarget{
		Kind:     types.KindSearch,
		Platform: "twitter",
		Query:    "site:x.com/a/status nvda",
	})
	if err != nil {
		t.Fatalf("Discover() error = %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("len(got) = %d, want 1", len(got))
	}
	if got[0].URL != "https://x.com/a/status/789" {
		t.Fatalf("URL = %q", got[0].URL)
	}
}

func TestCollector_DiscoverFiltersGenericResultsToArticleLikeURLs(t *testing.T) {
	searcher := &fakeSearcher{
		body: `<rss><channel>
<item><link>https://stratechery.com/</link></item>
<item><link>https://stratechery.com/about/</link></item>
<item><link>https://stratechery.com/category/daily-update/</link></item>
<item><link>https://stratechery.com/2026/the-ai-bundling-shift/</link></item>
</channel></rss>`,
	}
	c := New(types.PlatformWeb, "stratechery.com", searcher)

	got, err := c.Discover(context.Background(), types.FollowTarget{
		Kind:       types.KindSearch,
		Platform:   "web",
		Query:      `site:stratechery.com "Ben Thompson"`,
		AuthorName: "Ben Thompson",
	})
	if err != nil {
		t.Fatalf("Discover() error = %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("len(got) = %d, want 1: %#v", len(got), got)
	}
	if got[0].URL != "https://stratechery.com/2026/the-ai-bundling-shift/" {
		t.Fatalf("URL = %q, want article URL", got[0].URL)
	}
	if got[0].AuthorName != "Ben Thompson" {
		t.Fatalf("AuthorName = %q, want Ben Thompson", got[0].AuthorName)
	}
}

func TestGoogleSearcher_SearchRequestsDateSortedResults(t *testing.T) {
	var got *http.Request
	client := &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			got = req
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     make(http.Header),
				Body:       io.NopCloser(strings.NewReader(`<a href="/url?q=https://example.com/post&sa=U">post</a>`)),
			}, nil
		}),
	}

	searcher := NewGoogleSearcher(client)
	if _, err := searcher.Search(context.Background(), `"Torsten Slok" site:apolloacademy.com`, SearchOptions{}); err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	if got == nil {
		t.Fatal("Search() did not issue a request")
	}
	q := got.URL.Query()
	if q.Get("tbs") != "sbd:1" {
		t.Fatalf("tbs = %q, want date sort", q.Get("tbs"))
	}
	if q.Get("num") != "20" {
		t.Fatalf("num = %q, want 20", q.Get("num"))
	}
	if q.Get("q") != `"Torsten Slok" site:apolloacademy.com` {
		t.Fatalf("q = %q", q.Get("q"))
	}
}

func TestGoogleSearcher_SearchUsesRequestedTimeWindow(t *testing.T) {
	var got *http.Request
	client := &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			got = req
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     make(http.Header),
				Body:       io.NopCloser(strings.NewReader(`ok`)),
			}, nil
		}),
	}

	searcher := NewGoogleSearcher(client)
	if _, err := searcher.Search(context.Background(), `site:x.com/a/status`, SearchOptions{TBS: "qdr:d,sbd:1"}); err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	if got.URL.Query().Get("tbs") != "qdr:d,sbd:1" {
		t.Fatalf("tbs = %q", got.URL.Query().Get("tbs"))
	}
}

func TestBingRSSSearcher_SearchRequestsRSSWithFreshness(t *testing.T) {
	var got *http.Request
	client := &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			got = req
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     make(http.Header),
				Body:       io.NopCloser(strings.NewReader(`<rss><channel></channel></rss>`)),
			}, nil
		}),
	}

	searcher := NewBingRSSSearcher(client)
	if _, err := searcher.Search(context.Background(), `site:x.com/a/status`, SearchOptions{TBS: "qdr:d,sbd:1"}); err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	q := got.URL.Query()
	if q.Get("format") != "rss" {
		t.Fatalf("format = %q", q.Get("format"))
	}
	if q.Get("freshness") != "Day" {
		t.Fatalf("freshness = %q", q.Get("freshness"))
	}
	if q.Get("q") != "site:x.com/a/status" {
		t.Fatalf("q = %q", q.Get("q"))
	}
}

var errBoom = errors.New("boom")
