package youtube

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
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
