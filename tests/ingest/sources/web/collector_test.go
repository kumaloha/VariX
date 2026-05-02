package web

import (
	"context"
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

func TestArxivToAr5iv(t *testing.T) {
	got := arxivToAr5iv("https://arxiv.org/abs/2401.12345")
	want := "https://ar5iv.labs.arxiv.org/html/2401.12345"
	if got != want {
		t.Fatalf("arxivToAr5iv() = %q, want %q", got, want)
	}
}

func TestParseReaderMarkdownExtractsYoutubeRedirect(t *testing.T) {
	input := `Title: Example Page
Published Time: 2026-04-07T10:00:00Z
Markdown Content:

# Example

Watch here:
https://www.youtube.com/watch?v=dQw4w9WgXcQ
`
	got := parseReaderMarkdown("https://example.com/post", "external-1", input)
	if got.Metadata.Web == nil {
		t.Fatal("Metadata.Web is nil")
	}
	if got.Metadata.Web.YouTubeRedirect != "https://www.youtube.com/watch?v=dQw4w9WgXcQ" {
		t.Fatalf("youtube_redirect = %#v", got.Metadata.Web.YouTubeRedirect)
	}
	if got.Metadata.Web.Title != "Example Page" {
		t.Fatalf("title = %#v, want %q", got.Metadata.Web.Title, "Example Page")
	}
}

func TestCollectorFetchParsesHTMLArticle(t *testing.T) {
	client := &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			if req.URL.String() != "https://example.test/post" {
				t.Fatalf("request URL = %q, want %q", req.URL.String(), "https://example.test/post")
			}
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Content-Type": []string{"text/html; charset=utf-8"}},
				Body: io.NopCloser(strings.NewReader(`<!doctype html>
<html>
  <head>
    <title>Example Article</title>
    <meta name="author" content="Jane Doe">
    <meta property="article:published_time" content="2026-04-07T10:00:00Z">
    <link rel="canonical" href="https://example.com/canonical">
  </head>
  <body>
    <article>
      <p>First paragraph with enough text to survive simple normalization.</p>
      <p>Second paragraph with a little more substance for the collector.</p>
      <img src="https://cdn.example.test/image-1.jpg">
      <iframe src="https://www.youtube.com/embed/dQw4w9WgXcQ"></iframe>
    </article>
  </body>
</html>`)),
			}, nil
		}),
	}

	c := New(client)
	got, err := c.Fetch(context.Background(), types.ParsedURL{
		Platform:     types.PlatformWeb,
		ContentType:  types.ContentTypePost,
		PlatformID:   "web-1",
		CanonicalURL: "https://example.test/post",
	})
	if err != nil {
		t.Fatalf("Fetch() error = %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("len(Fetch()) = %d, want 1", len(got))
	}
	if got[0].Source != "web" {
		t.Fatalf("Source = %q, want %q", got[0].Source, "web")
	}
	if got[0].ExternalID != "web-1" {
		t.Fatalf("ExternalID = %q, want %q", got[0].ExternalID, "web-1")
	}
	if got[0].AuthorName != "Jane Doe" {
		t.Fatalf("AuthorName = %q, want %q", got[0].AuthorName, "Jane Doe")
	}
	if got[0].URL != "https://example.com/canonical" {
		t.Fatalf("URL = %q, want %q", got[0].URL, "https://example.com/canonical")
	}
	if !strings.Contains(got[0].Content, "First paragraph") {
		t.Fatalf("Content = %q, want first paragraph text", got[0].Content)
	}
	if !strings.Contains(got[0].Content, "[附件#1 图片]") {
		t.Fatalf("Content = %q, want image attachment placeholder", got[0].Content)
	}
	if got[0].Metadata.Web == nil {
		t.Fatal("Metadata.Web is nil")
	}
	if got[0].Metadata.Web.YouTubeRedirect != "https://www.youtube.com/watch?v=dQw4w9WgXcQ" {
		t.Fatalf("youtube_redirect = %#v", got[0].Metadata.Web.YouTubeRedirect)
	}
	if got[0].Metadata.Web.CanonicalURL != "https://example.com/canonical" {
		t.Fatalf("canonical_url = %#v", got[0].Metadata.Web.CanonicalURL)
	}
	if len(got[0].Attachments) != 1 {
		t.Fatalf("len(Attachments) = %d, want 1", len(got[0].Attachments))
	}
	if got[0].Attachments[0].Type != "image" {
		t.Fatalf("Attachments[0].Type = %q, want %q", got[0].Attachments[0].Type, "image")
	}
	if got[0].Attachments[0].URL != "https://cdn.example.test/image-1.jpg" {
		t.Fatalf("Attachments[0].URL = %q, want %q", got[0].Attachments[0].URL, "https://cdn.example.test/image-1.jpg")
	}
}

func TestCollectorFetchPrefersShareholderLetterBodyOverInfographicArticle(t *testing.T) {
	client := &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Content-Type": []string{"text/html; charset=utf-8"}},
				Body: io.NopCloser(strings.NewReader(`<!doctype html>
<html>
  <head>
    <title>Shareholder Letter</title>
  </head>
  <body>
    <div class="cmp-text">
      <h2><span class="title-medium">Dear Fellow Shareholders,</span></h2>
      <p>First substantive paragraph about the economy and the company.</p>
      <p>Second substantive paragraph about long-term strategy and risks.</p>
    </div>
    <article class="jpmc-infographic">
      <h2>Earnings 2005-2025</h2>
      <img alt="Chart image" src="https://cdn.example.test/chart.png">
      <p>See Text Version</p>
    </article>
  </body>
</html>`)),
			}, nil
		}),
	}

	c := New(client)
	got, err := c.Fetch(context.Background(), types.ParsedURL{
		Platform:     types.PlatformWeb,
		ContentType:  types.ContentTypePost,
		PlatformID:   "web-letter",
		CanonicalURL: "https://example.test/letter",
	})
	if err != nil {
		t.Fatalf("Fetch() error = %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("len(Fetch()) = %d, want 1", len(got))
	}
	if !strings.Contains(got[0].Content, "First substantive paragraph") {
		t.Fatalf("Content = %q, want shareholder letter body", got[0].Content)
	}
	if strings.Contains(got[0].Content, "See Text Version") {
		t.Fatalf("Content = %q, should not prioritize infographic text", got[0].Content)
	}
}

func TestCollectorFetchRejectsPDFBinary(t *testing.T) {
	client := &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Content-Type": []string{"application/pdf"}},
				Body:       io.NopCloser(strings.NewReader("%PDF-1.6 fake payload")),
			}, nil
		}),
	}

	c := New(client)
	_, err := c.Fetch(context.Background(), types.ParsedURL{
		Platform:     types.PlatformWeb,
		ContentType:  types.ContentTypePost,
		PlatformID:   "pdf-1",
		CanonicalURL: "https://example.test/report.pdf",
	})
	if err == nil {
		t.Fatal("Fetch() error = nil, want binary-document rejection")
	}
	if !strings.Contains(err.Error(), "unsupported binary document") {
		t.Fatalf("Fetch() error = %v, want unsupported binary document", err)
	}
}

func TestCollectorFetchRejectsSpreadsheetByExtension(t *testing.T) {
	client := &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Content-Type": []string{"application/octet-stream"}},
				Body:       io.NopCloser(strings.NewReader("PK fake xlsx payload")),
			}, nil
		}),
	}

	c := New(client)
	_, err := c.Fetch(context.Background(), types.ParsedURL{
		Platform:     types.PlatformWeb,
		ContentType:  types.ContentTypePost,
		PlatformID:   "xlsx-1",
		CanonicalURL: "https://example.test/data.xlsx",
	})
	if err == nil {
		t.Fatal("Fetch() error = nil, want binary-document rejection")
	}
	if !strings.Contains(err.Error(), "unsupported binary document") {
		t.Fatalf("Fetch() error = %v, want unsupported binary document", err)
	}
}

func TestParseHTMLDocument_ResolvesRelativeCanonical(t *testing.T) {
	html := `<html><head><title>Test</title><link rel="canonical" href="/post"></head><body>Hello</body></html>`
	rc := parseHTMLDocument("https://site-a.example/post?id=1", "ext1", html)
	if rc.URL != "https://site-a.example/post" {
		t.Errorf("URL = %q, want resolved canonical", rc.URL)
	}
	// Different site with same relative canonical should get different ExternalID
	rc2 := parseHTMLDocument("https://site-b.example/post?id=2", "ext2", html)
	if rc.ExternalID == rc2.ExternalID {
		t.Errorf("different sites with same relative canonical should have different ExternalIDs: %s vs %s", rc.ExternalID, rc2.ExternalID)
	}
}

func TestParseHTMLDocument_AbsoluteCanonicalUnchanged(t *testing.T) {
	html := `<html><head><title>Test</title><link rel="canonical" href="https://example.com/canonical"></head><body>Hello</body></html>`
	rc := parseHTMLDocument("https://example.com/page?utm=1", "ext1", html)
	if rc.URL != "https://example.com/canonical" {
		t.Errorf("URL = %q, want absolute canonical", rc.URL)
	}
}

func TestParseHTMLDocument_ResolvesRelativeAttachmentsAndDedupesAfterResolution(t *testing.T) {
	html := `<!doctype html>
<html>
  <head>
    <title>Test</title>
    <link rel="canonical" href="https://example.com/posts/alpha">
  </head>
  <body>
    <img src="images/chart.png">
    <img src="/posts/images/chart.png">
    <img src="https://cdn.example.com/cover.png">
    <img src="data:image/png;base64,abc">
  </body>
</html>`

	rc := parseHTMLDocument("https://example.com/posts/alpha?utm=1", "ext1", html)
	if len(rc.Attachments) != 2 {
		t.Fatalf("len(Attachments) = %d, want 2", len(rc.Attachments))
	}
	if rc.Attachments[0].URL != "https://example.com/posts/images/chart.png" {
		t.Fatalf("Attachments[0].URL = %q, want resolved relative URL", rc.Attachments[0].URL)
	}
	if rc.Attachments[1].URL != "https://cdn.example.com/cover.png" {
		t.Fatalf("Attachments[1].URL = %q, want absolute URL preserved", rc.Attachments[1].URL)
	}
	if !strings.Contains(rc.Content, "[附件#1 图片]") || !strings.Contains(rc.Content, "[附件#2 图片]") {
		t.Fatalf("Content = %q, want attachment placeholders", rc.Content)
	}
}

func TestParseHTMLDocument_ResolvesAttachmentsAgainstFetchedURLWhenCanonicalPathDiffers(t *testing.T) {
	html := `<!doctype html>
<html>
  <head>
    <title>Test</title>
    <link rel="canonical" href="https://example.com/canonical/alpha">
  </head>
  <body>
    <img src="foo.png">
  </body>
</html>`

	rc := parseHTMLDocument("https://example.com/fetched/path/index.html?utm=1", "ext1", html)
	if rc.URL != "https://example.com/canonical/alpha" {
		t.Fatalf("URL = %q, want canonical URL", rc.URL)
	}
	if len(rc.Attachments) != 1 {
		t.Fatalf("len(Attachments) = %d, want 1", len(rc.Attachments))
	}
	if rc.Attachments[0].URL != "https://example.com/fetched/path/foo.png" {
		t.Fatalf("Attachments[0].URL = %q, want fetched-path-resolved URL", rc.Attachments[0].URL)
	}
	if !strings.Contains(rc.Content, "[附件#1 图片]") {
		t.Fatalf("Content = %q, want attachment placeholder", rc.Content)
	}
}
