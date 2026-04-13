package provenance

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/kumaloha/VariX/varix/ingest/sources/web"
	"github.com/kumaloha/VariX/varix/ingest/types"
)

func TestCrossReview_RuleFinderUsesWebCollectorYouTubeRedirect(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(`<!doctype html>
<html>
  <head>
    <title>Wrapper Page</title>
  </head>
  <body>
    <article>
      <p>Short recap text.</p>
      <iframe src="https://www.youtube.com/embed/dQw4w9WgXcQ"></iframe>
    </article>
  </body>
</html>`))
	}))
	defer srv.Close()

	items, err := web.New(srv.Client()).Fetch(context.Background(), types.ParsedURL{
		Platform:     types.PlatformWeb,
		ContentType:  types.ContentTypePost,
		PlatformID:   "web-1",
		CanonicalURL: srv.URL + "/post",
	})
	if err != nil {
		t.Fatalf("Fetch() error = %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("len(items) = %d, want 1", len(items))
	}
	if items[0].Metadata.Web == nil || items[0].Metadata.Web.YouTubeRedirect == "" {
		t.Fatalf("web metadata missing youtube redirect: %#v", items[0].Metadata.Web)
	}

	got, err := NewRuleFinderWithResolver(nil).FindCandidates(context.Background(), items[0])
	if err != nil {
		t.Fatalf("FindCandidates() error = %v", err)
	}
	if len(got) == 0 {
		t.Fatal("FindCandidates() returned no candidates")
	}
	if got[0].URL != "https://www.youtube.com/watch?v=dQw4w9WgXcQ" {
		t.Fatalf("first candidate = %#v, want embedded youtube redirect first", got[0])
	}
	if got[0].Kind != "embedded_link" {
		t.Fatalf("first candidate kind = %q, want embedded_link", got[0].Kind)
	}
}

func TestCrossReview_ClassifyUsesWebCollectorYouTubeRedirectAsSourceEvidence(t *testing.T) {
	raw := types.RawContent{
		Source:     "web",
		ExternalID: "web-1",
		URL:        "https://example.com/post",
		Content:    "Short recap text.",
		Metadata: types.RawMetadata{
			Web: &types.WebMetadata{
				Title:           "Wrapper Page",
				SourceURL:       "https://example.com/post",
				YouTubeRedirect: "https://www.youtube.com/watch?v=dQw4w9WgXcQ",
			},
		},
	}

	got := Classify(raw)
	if len(got.SourceCandidates) != 1 {
		t.Fatalf("len(SourceCandidates) = %d, want 1", len(got.SourceCandidates))
	}
	if got.SourceCandidates[0].URL != "https://www.youtube.com/watch?v=dQw4w9WgXcQ" {
		t.Fatalf("SourceCandidates[0] = %#v, want youtube redirect", got.SourceCandidates[0])
	}
	if len(got.Evidence) == 0 || got.Evidence[0].Kind != "source_link" {
		t.Fatalf("Evidence = %#v, want source_link evidence", got.Evidence)
	}
}
