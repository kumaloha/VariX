package dispatcher

import (
	"context"
	"testing"
	"time"

	"github.com/kumaloha/VariX/varix/ingest/provenance"
	"github.com/kumaloha/VariX/varix/ingest/types"
)

type fakeItemSource struct {
	platform types.Platform
}

func (f fakeItemSource) Platform() types.Platform { return f.platform }
func (f fakeItemSource) Fetch(_ context.Context, parsed types.ParsedURL) ([]types.RawContent, error) {
	return []types.RawContent{{Source: string(f.platform), ExternalID: parsed.PlatformID, URL: parsed.CanonicalURL, PostedAt: time.Now().UTC()}}, nil
}

type fakeDiscoverer struct {
	kind     types.Kind
	platform types.Platform
}

func (f fakeDiscoverer) Kind() types.Kind         { return f.kind }
func (f fakeDiscoverer) Platform() types.Platform { return f.platform }
func (f fakeDiscoverer) Discover(_ context.Context, _ types.FollowTarget) ([]types.DiscoveryItem, error) {
	return []types.DiscoveryItem{{ExternalID: "abc", URL: "https://example.com/item", HydrationHint: "web"}}, nil
}

type capturingItemSource struct {
	platform types.Platform
	seen     []types.ParsedURL
}

func (c *capturingItemSource) Platform() types.Platform { return c.platform }
func (c *capturingItemSource) Fetch(_ context.Context, parsed types.ParsedURL) ([]types.RawContent, error) {
	c.seen = append(c.seen, parsed)
	return []types.RawContent{{Source: string(c.platform), ExternalID: parsed.PlatformID, URL: parsed.CanonicalURL, PostedAt: time.Now().UTC()}}, nil
}

func TestService_FetchByParsedURL(t *testing.T) {
	svc := New(
		func(raw string) (types.ParsedURL, error) {
			return types.ParsedURL{
				Platform:     types.PlatformTwitter,
				ContentType:  types.ContentTypePost,
				PlatformID:   "123",
				CanonicalURL: raw,
			}, nil
		},
		[]ItemSource{fakeItemSource{platform: types.PlatformTwitter}},
		nil,
		nil,
	)
	got, err := svc.FetchByParsedURL(context.Background(), types.ParsedURL{
		Platform:     types.PlatformTwitter,
		ContentType:  types.ContentTypePost,
		PlatformID:   "123",
		CanonicalURL: "https://x.com/a/status/123",
	})
	if err != nil {
		t.Fatalf("FetchByParsedURL() error = %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("len(FetchByParsedURL()) = %d, want 1", len(got))
	}
}

func TestService_FetchByParsedURLFallsBackToWebForRSS(t *testing.T) {
	webSource := &capturingItemSource{platform: types.PlatformWeb}
	svc := New(
		func(raw string) (types.ParsedURL, error) {
			return types.ParsedURL{Platform: types.PlatformRSS, ContentType: types.ContentTypeFeed, PlatformID: "feed-1", CanonicalURL: raw}, nil
		},
		[]ItemSource{webSource},
		nil,
		nil,
	)

	got, err := svc.FetchByParsedURL(context.Background(), types.ParsedURL{
		Platform:     types.PlatformRSS,
		ContentType:  types.ContentTypeFeed,
		PlatformID:   "feed-1",
		CanonicalURL: "https://www.ecb.europa.eu/rss/press.html",
	})
	if err != nil {
		t.Fatalf("FetchByParsedURL() error = %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("len(FetchByParsedURL()) = %d, want 1", len(got))
	}
	if len(webSource.seen) != 1 {
		t.Fatalf("len(webSource.seen) = %d, want 1", len(webSource.seen))
	}
	if webSource.seen[0].Platform != types.PlatformWeb {
		t.Fatalf("Platform = %q, want %q", webSource.seen[0].Platform, types.PlatformWeb)
	}
	if webSource.seen[0].ContentType != types.ContentTypePost {
		t.Fatalf("ContentType = %q, want %q", webSource.seen[0].ContentType, types.ContentTypePost)
	}
}

func TestService_DiscoverFollowedTarget(t *testing.T) {
	svc := New(
		func(raw string) (types.ParsedURL, error) {
			return types.ParsedURL{Platform: types.PlatformWeb, ContentType: types.ContentTypePost, PlatformID: "item", CanonicalURL: raw}, nil
		},
		nil,
		[]Discoverer{fakeDiscoverer{kind: types.KindRSS, platform: types.PlatformRSS}},
		nil,
	)
	got, err := svc.DiscoverFollowedTarget(context.Background(), types.FollowTarget{
		Kind:     types.KindRSS,
		Platform: "rss",
		Locator:  "https://example.com/feed.xml",
		URL:      "https://example.com/feed.xml",
	})
	if err != nil {
		t.Fatalf("DiscoverFollowedTarget() error = %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("len(DiscoverFollowedTarget()) = %d, want 1", len(got))
	}
}

func TestService_FetchDiscoveryItemForcesItemContentTypeWhenHydrationOverridesParse(t *testing.T) {
	itemSource := &capturingItemSource{platform: types.PlatformWeb}
	svc := New(
		func(raw string) (types.ParsedURL, error) {
			return types.ParsedURL{
				Platform:     types.PlatformRSS,
				ContentType:  types.ContentTypeFeed,
				PlatformID:   "feed",
				CanonicalURL: raw,
			}, nil
		},
		[]ItemSource{itemSource},
		nil,
		nil,
	)

	_, err := svc.FetchDiscoveryItem(context.Background(), types.DiscoveryItem{
		URL:           "https://example.com/articles/123",
		ExternalID:    "article-123",
		HydrationHint: "web",
	})
	if err != nil {
		t.Fatalf("FetchDiscoveryItem() error = %v", err)
	}
	if len(itemSource.seen) != 1 {
		t.Fatalf("len(itemSource.seen) = %d, want 1", len(itemSource.seen))
	}
	if itemSource.seen[0].Platform != types.PlatformWeb {
		t.Fatalf("Platform = %q, want %q", itemSource.seen[0].Platform, types.PlatformWeb)
	}
	if itemSource.seen[0].ContentType != types.ContentTypePost {
		t.Fatalf("ContentType = %q, want %q", itemSource.seen[0].ContentType, types.ContentTypePost)
	}
	if itemSource.seen[0].PlatformID != "article-123" {
		t.Fatalf("PlatformID = %q, want %q", itemSource.seen[0].PlatformID, "article-123")
	}
}

func TestService_FetchDiscoveryItemKeepsNativeParsedIDWhenPresent(t *testing.T) {
	itemSource := &capturingItemSource{platform: types.PlatformYouTube}
	svc := New(
		func(raw string) (types.ParsedURL, error) {
			return types.ParsedURL{
				Platform:     types.PlatformYouTube,
				ContentType:  types.ContentTypePost,
				PlatformID:   "dQw4w9WgXcQ",
				CanonicalURL: "https://www.youtube.com/watch?v=dQw4w9WgXcQ",
			}, nil
		},
		[]ItemSource{itemSource},
		nil,
		nil,
	)

	_, err := svc.FetchDiscoveryItem(context.Background(), types.DiscoveryItem{
		Platform:   types.PlatformYouTube,
		URL:        "https://www.youtube.com/watch?v=dQw4w9WgXcQ",
		ExternalID: "yt:video:dQw4w9WgXcQ",
	})
	if err != nil {
		t.Fatalf("FetchDiscoveryItem() error = %v", err)
	}
	if len(itemSource.seen) != 1 {
		t.Fatalf("len(itemSource.seen) = %d, want 1", len(itemSource.seen))
	}
	if itemSource.seen[0].PlatformID != "dQw4w9WgXcQ" {
		t.Fatalf("PlatformID = %q, want %q", itemSource.seen[0].PlatformID, "dQw4w9WgXcQ")
	}
}

func TestService_FetchDiscoveryItemDoesNotTrustGenericFallbackIDAfterHydrationOverride(t *testing.T) {
	itemSource := &capturingItemSource{platform: types.PlatformYouTube}
	svc := New(
		func(raw string) (types.ParsedURL, error) {
			return types.ParsedURL{
				Platform:     types.PlatformWeb,
				ContentType:  types.ContentTypePost,
				PlatformID:   "web-fallback-hash",
				CanonicalURL: raw,
			}, nil
		},
		[]ItemSource{itemSource},
		nil,
		nil,
	)

	_, err := svc.FetchDiscoveryItem(context.Background(), types.DiscoveryItem{
		URL:           "https://example.com/out?to=https://www.youtube.com/watch?v=dQw4w9WgXcQ",
		ExternalID:    "dQw4w9WgXcQ",
		HydrationHint: "youtube",
	})
	if err != nil {
		t.Fatalf("FetchDiscoveryItem() error = %v", err)
	}
	if len(itemSource.seen) != 1 {
		t.Fatalf("len(itemSource.seen) = %d, want 1", len(itemSource.seen))
	}
	if itemSource.seen[0].PlatformID != "dQw4w9WgXcQ" {
		t.Fatalf("PlatformID = %q, want %q", itemSource.seen[0].PlatformID, "dQw4w9WgXcQ")
	}
}

func TestService_FetchDiscoveryItemPreservesWebIdentityForRSSDiscoveredURLs(t *testing.T) {
	// Regression test: RSS-discovered web URLs must keep the parser-derived
	// PlatformID (md5 hash of URL) so that markProcessed stores the same
	// identity that discoveryIdentity computes for dedupe.  Without this fix,
	// the RSS guid would overwrite the PlatformID, causing infinite re-fetch.
	webHash := "abc123hash" // simulates the md5 hash the router produces for a web URL
	itemSource := &capturingItemSource{platform: types.PlatformWeb}
	svc := New(
		func(raw string) (types.ParsedURL, error) {
			return types.ParsedURL{
				Platform:     types.PlatformWeb,
				ContentType:  types.ContentTypePost,
				PlatformID:   webHash,
				CanonicalURL: raw,
			}, nil
		},
		[]ItemSource{itemSource},
		nil,
		nil,
	)

	_, err := svc.FetchDiscoveryItem(context.Background(), types.DiscoveryItem{
		Platform:   types.PlatformRSS,
		URL:        "https://example.com/article",
		ExternalID: "rss_guid_that_must_not_leak",
	})
	if err != nil {
		t.Fatalf("FetchDiscoveryItem() error = %v", err)
	}
	if len(itemSource.seen) != 1 {
		t.Fatalf("len(itemSource.seen) = %d, want 1", len(itemSource.seen))
	}
	// The parser-derived web hash must survive — not the RSS guid.
	if itemSource.seen[0].PlatformID != webHash {
		t.Fatalf("PlatformID = %q, want %q (parser-derived web hash)", itemSource.seen[0].PlatformID, webHash)
	}
	if itemSource.seen[0].Platform != types.PlatformWeb {
		t.Fatalf("Platform = %q, want %q", itemSource.seen[0].Platform, types.PlatformWeb)
	}
}

func TestService_ParseURLResolvesAllowlistedHostBeforeParsing(t *testing.T) {
	resolver := &fakeLinkResolver{
		out: map[string]string{
			"https://t.co/abc123": "https://example.com/articles/123",
		},
	}
	var parsed []string
	svc := New(
		func(raw string) (types.ParsedURL, error) {
			parsed = append(parsed, raw)
			return types.ParsedURL{Platform: types.PlatformWeb, ContentType: types.ContentTypePost, CanonicalURL: raw}, nil
		},
		nil,
		nil,
		resolver,
	)

	got, err := svc.ParseURL(context.Background(), "https://t.co/abc123")
	if err != nil {
		t.Fatalf("ParseURL() error = %v", err)
	}
	if len(resolver.calls) != 1 || resolver.calls[0] != "https://t.co/abc123" {
		t.Fatalf("resolver.calls = %#v, want original short URL", resolver.calls)
	}
	if len(parsed) != 1 || parsed[0] != "https://example.com/articles/123" {
		t.Fatalf("parsed = %#v, want resolved URL", parsed)
	}
	if got.CanonicalURL != "https://example.com/articles/123" {
		t.Fatalf("CanonicalURL = %q, want resolved URL", got.CanonicalURL)
	}
}

func TestService_ParseURLUsesExactHostAllowlist(t *testing.T) {
	resolver := &fakeLinkResolver{
		out: map[string]string{
			"https://www.bit.ly/abc123": "https://example.com/articles/123",
		},
	}
	var parsed []string
	svc := New(
		func(raw string) (types.ParsedURL, error) {
			parsed = append(parsed, raw)
			return types.ParsedURL{Platform: types.PlatformWeb, ContentType: types.ContentTypePost, CanonicalURL: raw}, nil
		},
		nil,
		nil,
		resolver,
	)

	got, err := svc.ParseURL(context.Background(), "https://www.bit.ly/abc123")
	if err != nil {
		t.Fatalf("ParseURL() error = %v", err)
	}
	if len(resolver.calls) != 0 {
		t.Fatalf("resolver.calls = %#v, want no resolution for non-exact host", resolver.calls)
	}
	if len(parsed) != 1 || parsed[0] != "https://www.bit.ly/abc123" {
		t.Fatalf("parsed = %#v, want original URL", parsed)
	}
	if got.CanonicalURL != "https://www.bit.ly/abc123" {
		t.Fatalf("CanonicalURL = %q, want original URL", got.CanonicalURL)
	}
}

func TestService_FetchDiscoveryItemResolvesAllowlistedURLBeforeHydration(t *testing.T) {
	resolver := &fakeLinkResolver{
		out: map[string]string{
			"https://bit.ly/video123": "https://www.youtube.com/watch?v=dQw4w9WgXcQ",
		},
	}
	itemSource := &capturingItemSource{platform: types.PlatformYouTube}
	var parsed []string
	svc := New(
		func(raw string) (types.ParsedURL, error) {
			parsed = append(parsed, raw)
			return types.ParsedURL{
				Platform:     types.PlatformYouTube,
				ContentType:  types.ContentTypePost,
				PlatformID:   "dQw4w9WgXcQ",
				CanonicalURL: raw,
			}, nil
		},
		[]ItemSource{itemSource},
		nil,
		resolver,
	)

	_, err := svc.FetchDiscoveryItem(context.Background(), types.DiscoveryItem{
		URL:           "https://bit.ly/video123",
		ExternalID:    "yt:video:dQw4w9WgXcQ",
		HydrationHint: "youtube",
	})
	if err != nil {
		t.Fatalf("FetchDiscoveryItem() error = %v", err)
	}
	if len(resolver.calls) != 1 || resolver.calls[0] != "https://bit.ly/video123" {
		t.Fatalf("resolver.calls = %#v, want discovery item short URL", resolver.calls)
	}
	if len(parsed) != 1 || parsed[0] != "https://www.youtube.com/watch?v=dQw4w9WgXcQ" {
		t.Fatalf("parsed = %#v, want resolved YouTube URL", parsed)
	}
	if len(itemSource.seen) != 1 {
		t.Fatalf("len(itemSource.seen) = %d, want 1", len(itemSource.seen))
	}
	if itemSource.seen[0].CanonicalURL != "https://www.youtube.com/watch?v=dQw4w9WgXcQ" {
		t.Fatalf("CanonicalURL = %q, want resolved URL", itemSource.seen[0].CanonicalURL)
	}
	if itemSource.seen[0].PlatformID != "dQw4w9WgXcQ" {
		t.Fatalf("PlatformID = %q, want parser-derived native ID", itemSource.seen[0].PlatformID)
	}
}

type fakeLinkResolver struct {
	out   map[string]string
	errs  map[string]error
	calls []string
}

var _ provenance.LinkResolver = (*fakeLinkResolver)(nil)

func (f *fakeLinkResolver) Resolve(_ context.Context, raw string) (string, error) {
	f.calls = append(f.calls, raw)
	if err, ok := f.errs[raw]; ok {
		return "", err
	}
	if out, ok := f.out[raw]; ok {
		return out, nil
	}
	return raw, nil
}
