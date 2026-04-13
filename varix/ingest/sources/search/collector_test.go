package search

import (
	"context"
	"testing"

	"github.com/kumaloha/VariX/varix/ingest/types"
)

type fakeSearcher struct {
	query string
	body  string
	err   error
}

func (f *fakeSearcher) Search(_ context.Context, query string) (string, error) {
	f.query = query
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
