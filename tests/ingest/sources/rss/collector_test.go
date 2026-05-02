package rss

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/kumaloha/VariX/varix/ingest/types"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestCollectorDiscoverReturnsFeedLinks(t *testing.T) {
	client := &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			if req.URL.String() != "https://feeds.example.test/feed.xml" {
				t.Fatalf("request URL = %q, want %q", req.URL.String(), "https://feeds.example.test/feed.xml")
			}
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Content-Type": []string{"application/rss+xml"}},
				Body: io.NopCloser(strings.NewReader(`<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0">
  <channel>
    <title>Example</title>
    <item>
      <title>Item 1</title>
      <link>https://example.com/post-1</link>
      <guid>item-1</guid>
      <pubDate>Tue, 07 Apr 2026 10:00:00 GMT</pubDate>
    </item>
  </channel>
</rss>`)),
			}, nil
		}),
	}

	c := New(client)
	got, err := c.Discover(context.Background(), types.FollowTarget{
		Kind:       types.KindRSS,
		Platform:   "rss",
		Locator:    "https://feeds.example.test/feed.xml",
		URL:        "https://feeds.example.test/feed.xml",
		AuthorName: "Example Feed",
		FollowedAt: time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("Discover() error = %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("len(Discover()) = %d, want 1", len(got))
	}
	if got[0].Platform != types.PlatformRSS {
		t.Fatalf("Platform = %q, want %q", got[0].Platform, types.PlatformRSS)
	}
	if got[0].ExternalID != "item-1" {
		t.Fatalf("ExternalID = %q, want %q", got[0].ExternalID, "item-1")
	}
	if got[0].URL != "https://example.com/post-1" {
		t.Fatalf("URL = %q, want %q", got[0].URL, "https://example.com/post-1")
	}
	if got[0].AuthorName != "Example Feed" {
		t.Fatalf("AuthorName = %q, want %q", got[0].AuthorName, "Example Feed")
	}
	if got[0].Metadata.RSS == nil {
		t.Fatal("Metadata.RSS is nil")
	}
	if got[0].Metadata.RSS.Title != "Item 1" {
		t.Fatalf("title = %#v, want %q", got[0].Metadata.RSS.Title, "Item 1")
	}
	if got[0].Metadata.RSS.Feed != "https://feeds.example.test/feed.xml" {
		t.Fatalf("feed = %#v, want %q", got[0].Metadata.RSS.Feed, "https://feeds.example.test/feed.xml")
	}
}
