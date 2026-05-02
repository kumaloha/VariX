package provenance

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/kumaloha/VariX/varix/ingest/types"
)

func TestRuleFinder_UsesStructuredSourceCandidates(t *testing.T) {
	raw := types.RawContent{
		URL: "https://www.youtube.com/watch?v=abc123",
		Metadata: types.RawMetadata{
			YouTube: &types.YouTubeMetadata{
				Title:       "巴菲特访谈中字解读",
				SourceLinks: []string{"https://www.cnbc.com/interview"},
			},
		},
		Provenance: &types.Provenance{
			SourceCandidates: []types.SourceCandidate{{
				URL:        "https://www.cnbc.com/interview",
				Host:       "www.cnbc.com",
				Kind:       "source_link",
				Confidence: "high",
			}},
		},
	}

	got, err := NewRuleFinder().FindCandidates(context.Background(), raw)
	if err != nil {
		t.Fatalf("FindCandidates() error = %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("len(FindCandidates()) = %d, want 1", len(got))
	}
	if got[0].URL != "https://www.cnbc.com/interview" || got[0].Kind != "source_link" {
		t.Fatalf("candidate = %#v, want structured source link", got[0])
	}
}

func TestRuleFinder_AddsNoSyntheticFallbackWhenNoStructuredLinks(t *testing.T) {
	raw := types.RawContent{
		URL: "https://www.bilibili.com/video/BV1ABCDEF123",
		Metadata: types.RawMetadata{
			Bilibili: &types.BilibiliMetadata{
				Title: "巴菲特访谈翻译整理",
			},
		},
	}

	got, err := NewRuleFinder().FindCandidates(context.Background(), raw)
	if err != nil {
		t.Fatalf("FindCandidates() error = %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("len(FindCandidates()) = %d, want 0", len(got))
	}
}

func TestRuleFinder_FiltersPlatformSelfLinksForYouTube(t *testing.T) {
	raw := types.RawContent{
		URL: "https://www.youtube.com/watch?v=MLhbaA7XW1M",
		Metadata: types.RawMetadata{
			YouTube: &types.YouTubeMetadata{
				Title: "巴菲特访谈中字解读",
				SourceLinks: []string{
					"https://www.youtube.com/channel/UC1Xm-VhWUqZcPCCN5R2MniA/join",
					"https://www.cnbc.com/video/buffett-interview",
				},
			},
		},
	}

	got, err := NewRuleFinder().FindCandidates(context.Background(), raw)
	if err != nil {
		t.Fatalf("FindCandidates() error = %v", err)
	}
	if len(got) == 0 {
		t.Fatal("FindCandidates() returned no candidates")
	}
	if got[0].URL != "https://www.cnbc.com/video/buffett-interview" {
		t.Fatalf("first candidate = %#v, want external source link first", got[0])
	}
	for _, candidate := range got {
		if candidate.URL == "https://www.youtube.com/channel/UC1Xm-VhWUqZcPCCN5R2MniA/join" {
			t.Fatalf("candidates = %#v, want join link filtered out", got)
		}
	}
}

func TestRuleFinder_ResolvesTCoLinks(t *testing.T) {
	raw := types.RawContent{
		URL: "https://x.com/alice/status/123",
		Metadata: types.RawMetadata{
			Twitter: &types.TwitterMetadata{
				SourceLinks: []string{"https://t.co/abc123"},
			},
		},
	}

	got, err := NewRuleFinderWithResolver(fakeResolver{
		out: map[string]string{
			"https://t.co/abc123": "https://www.wsj.com/articles/oil-market-outlook",
		},
	}).FindCandidates(context.Background(), raw)
	if err != nil {
		t.Fatalf("FindCandidates() error = %v", err)
	}
	if len(got) == 0 {
		t.Fatal("FindCandidates() returned no candidates")
	}
	if got[0].URL != "https://www.wsj.com/articles/oil-market-outlook" {
		t.Fatalf("first candidate = %#v, want resolved external url", got[0])
	}
}

func TestRuleFinder_KeepsOriginalLinkWhenTCoResolveFails(t *testing.T) {
	raw := types.RawContent{
		URL: "https://x.com/alice/status/123",
		Metadata: types.RawMetadata{
			Twitter: &types.TwitterMetadata{
				SourceLinks: []string{"https://t.co/abc123"},
			},
		},
	}

	got, err := NewRuleFinderWithResolver(fakeResolver{
		errs: map[string]error{
			"https://t.co/abc123": errors.New("resolve failed"),
		},
	}).FindCandidates(context.Background(), raw)
	if err != nil {
		t.Fatalf("FindCandidates() error = %v", err)
	}
	if len(got) == 0 {
		t.Fatal("FindCandidates() returned no candidates")
	}
	if got[0].URL != "https://t.co/abc123" {
		t.Fatalf("first candidate = %#v, want original t.co url", got[0])
	}
}

func TestRuleFinder_IncludesWeiboOriginalURL(t *testing.T) {
	raw := types.RawContent{
		Source:  "weibo",
		URL:     "https://weibo.com/111/repost_bid",
		Content: "转发微博",
		Metadata: types.RawMetadata{
			Weibo: &types.WeiboMetadata{
				IsRepost:    true,
				OriginalURL: "https://weibo.com/222/original_bid",
			},
		},
	}

	got, err := NewRuleFinder().FindCandidates(context.Background(), raw)
	if err != nil {
		t.Fatalf("FindCandidates() error = %v", err)
	}
	for _, candidate := range got {
		if candidate.URL == "https://weibo.com/222/original_bid" {
			return
		}
	}
	t.Fatalf("candidates = %#v, want OriginalURL source candidate", got)
}

func TestRuleFinder_UsesStructuredQuoteAndReferenceURLs(t *testing.T) {
	raw := types.RawContent{
		URL:     "https://weibo.com/111/repost_bid",
		Content: "主帖正文",
		Quotes: []types.Quote{{
			Relation: "quote_tweet",
			URL:      "https://x.com/source/status/123",
		}},
		References: []types.Reference{{
			URL: "https://example.com/source-article",
		}},
	}

	got, err := NewRuleFinderWithResolver(nil).FindCandidates(context.Background(), raw)
	if err != nil {
		t.Fatalf("FindCandidates() error = %v", err)
	}
	seen := map[string]bool{}
	for _, candidate := range got {
		seen[candidate.URL] = true
	}
	if !seen["https://x.com/source/status/123"] {
		t.Fatalf("candidates = %#v, want quote url", got)
	}
	if !seen["https://example.com/source-article"] {
		t.Fatalf("candidates = %#v, want reference url", got)
	}
	for _, candidate := range got {
		if candidate.URL == "https://example.com/source-article" && candidate.Kind != "reference_link" {
			t.Fatalf("reference candidate = %#v, want reference_link", candidate)
		}
	}
}

func TestHTTPResolver_ResolveResolvesAnyURL(t *testing.T) {
	client := &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			if req.Method != http.MethodHead {
				t.Fatalf("request method = %q, want %q", req.Method, http.MethodHead)
			}
			resolvedURL, err := url.Parse("https://example.com/dest")
			if err != nil {
				t.Fatalf("url.Parse() error = %v", err)
			}
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(http.NoBody),
				Request: &http.Request{
					Method: req.Method,
					URL:    resolvedURL,
				},
			}, nil
		}),
	}

	got, err := NewHTTPResolver(client).Resolve(context.Background(), "https://example.com/short")
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if got != "https://example.com/dest" {
		t.Fatalf("Resolve() = %q, want %q", got, "https://example.com/dest")
	}
}

func TestHTTPResolver_ResolveFallsBackToGetOnHeadStatus(t *testing.T) {
	for _, status := range []int{http.StatusMethodNotAllowed, http.StatusNotImplemented} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			var methods []string
			client := &http.Client{
				Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
					methods = append(methods, req.Method)
					if req.Method == http.MethodHead {
						return &http.Response{
							StatusCode: status,
							Body:       io.NopCloser(http.NoBody),
							Request:    req,
						}, nil
					}
					resolvedURL, err := url.Parse("https://example.com/dest")
					if err != nil {
						t.Fatalf("url.Parse() error = %v", err)
					}
					return &http.Response{
						StatusCode: http.StatusOK,
						Body:       io.NopCloser(http.NoBody),
						Request: &http.Request{
							Method: req.Method,
							URL:    resolvedURL,
						},
					}, nil
				}),
			}

			got, err := NewHTTPResolver(client).Resolve(context.Background(), "https://example.com/short")
			if err != nil {
				t.Fatalf("Resolve() error = %v", err)
			}
			if got != "https://example.com/dest" {
				t.Fatalf("Resolve() = %q, want %q", got, "https://example.com/dest")
			}
			if len(methods) != 2 || methods[0] != http.MethodHead || methods[1] != http.MethodGet {
				t.Fatalf("methods = %#v, want HEAD then GET", methods)
			}
		})
	}
}

func TestHTTPResolver_ResolveFallsBackToGetOnHeadError(t *testing.T) {
	var methods []string
	client := &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			methods = append(methods, req.Method)
			if req.Method == http.MethodHead {
				return nil, errors.New("head failed")
			}
			resolvedURL, err := url.Parse("https://example.com/dest")
			if err != nil {
				t.Fatalf("url.Parse() error = %v", err)
			}
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(http.NoBody),
				Request:    &http.Request{Method: req.Method, URL: resolvedURL},
			}, nil
		}),
	}

	got, err := NewHTTPResolver(client).Resolve(context.Background(), "https://example.com/short")
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if got != "https://example.com/dest" {
		t.Fatalf("Resolve() = %q, want %q", got, "https://example.com/dest")
	}
	if len(methods) != 2 || methods[0] != http.MethodHead || methods[1] != http.MethodGet {
		t.Fatalf("methods = %#v, want HEAD then GET", methods)
	}
}

type fakeResolver struct {
	out  map[string]string
	errs map[string]error
}

func (r fakeResolver) Resolve(_ context.Context, raw string) (string, error) {
	if err, ok := r.errs[raw]; ok {
		return "", err
	}
	if out, ok := r.out[raw]; ok {
		return out, nil
	}
	return raw, nil
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return fn(req)
}

func TestHTTPResolver_DefaultTimeout(t *testing.T) {
	resolver := NewHTTPResolver(nil)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	_, _ = resolver.Resolve(ctx, "http://127.0.0.1:1")
}
