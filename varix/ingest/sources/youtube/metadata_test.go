package youtube

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestHTTPMetadataFetcher_ExtractsDescriptionAndSourceLinks(t *testing.T) {
	html := `
<html>
  <head>
    <meta property="og:title" content="巴菲特访谈中字解读" />
    <link itemprop="name" content="Example Channel" />
    <script>
      var ytInitialPlayerResponse = {
        "channelId":"chan-1",
        "publishDate":"2026-04-07",
        "shortDescription":"原视频：https://www.cnbc.com/interview\n中字整理"
      };
    </script>
  </head>
</html>`

	client := &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader(html)),
				Header:     make(http.Header),
			}, nil
		}),
	}

	got, err := NewHTTPMetadataFetcher(client).Fetch(context.Background(), "abc123")
	if err != nil {
		t.Fatalf("Fetch() error = %v", err)
	}
	if got.Description != "原视频：https://www.cnbc.com/interview 中字整理" {
		t.Fatalf("Description = %#v", got.Description)
	}
	if len(got.SourceLinks) != 1 {
		t.Fatalf("len(SourceLinks) = %d, want 1", len(got.SourceLinks))
	}
	if got.SourceLinks[0] != "https://www.cnbc.com/interview" {
		t.Fatalf("SourceLinks[0] = %#v", got.SourceLinks[0])
	}
}

func TestHTTPMetadataFetcher_ParsesRFC3339PublishDate(t *testing.T) {
	html := `
<html>
  <head>
    <meta property="og:title" content="Sample Title" />
    <link itemprop="name" content="Example Channel" />
    <meta itemprop="datePublished" content="2026-04-12T03:00:06-07:00" />
    <script>
      var ytInitialPlayerResponse = {
        "channelId":"chan-1",
        "publishDate":"2026-04-12T03:00:06-07:00",
        "shortDescription":"无来源链接"
      };
    </script>
  </head>
</html>`
	client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(html)), Header: make(http.Header)}, nil
	})}
	got, err := NewHTTPMetadataFetcher(client).Fetch(context.Background(), "abc123")
	if err != nil {
		t.Fatalf("Fetch() error = %v", err)
	}
	want := time.Date(2026, 4, 12, 10, 0, 6, 0, time.UTC)
	if !got.PublishedAt.Equal(want) {
		t.Fatalf("PublishedAt = %v, want %v", got.PublishedAt, want)
	}
}

func TestHTTPMetadataFetcher_FiltersPromoLinksFromDescription(t *testing.T) {
	html := `
<html>
  <head>
    <meta property="og:title" content="Sample Title" />
    <link itemprop="name" content="Example Channel" />
    <script>
      var ytInitialPlayerResponse = {
        "channelId":"chan-1",
        "publishDate":"2026-04-12",
        "shortDescription":"加入會員：https://reurl.cc/pgZL9d\n商品連結：https://pse.is/AR569c\n原視頻：https://www.example.com/original"
      };
    </script>
  </head>
</html>`
	client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(html)), Header: make(http.Header)}, nil
	})}
	got, err := NewHTTPMetadataFetcher(client).Fetch(context.Background(), "abc123")
	if err != nil {
		t.Fatalf("Fetch() error = %v", err)
	}
	if len(got.SourceLinks) != 1 {
		t.Fatalf("len(SourceLinks) = %d, want 1 (%#v)", len(got.SourceLinks), got.SourceLinks)
	}
	if got.SourceLinks[0] != "https://www.example.com/original" {
		t.Fatalf("SourceLinks[0] = %q, want original source", got.SourceLinks[0])
	}
}

func TestPreferredSubtitleFile_PrefersExactEnglishThenChineseThenOthers(t *testing.T) {
	matches := []string{
		"/tmp/video.fr.vtt",
		"/tmp/video.zh-TW.vtt",
		"/tmp/video.en.vtt",
	}
	got := preferredSubtitleFile(matches)
	if got != "/tmp/video.en.vtt" {
		t.Fatalf("preferredSubtitleFile() = %q, want en file", got)
	}
}

func TestPreferredSubtitleFile_PrefersExactChineseOverTranslatedEnglish(t *testing.T) {
	matches := []string{
		"/tmp/video.en-zh-TW.vtt",
		"/tmp/video.zh-TW.vtt",
	}
	got := preferredSubtitleFile(matches)
	if got != "/tmp/video.zh-TW.vtt" {
		t.Fatalf("preferredSubtitleFile() = %q, want zh-TW file", got)
	}
}
