package twitter

import (
	"context"
	"errors"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/kumaloha/VariX/varix/ingest/sources/audioutil"
	"github.com/kumaloha/VariX/varix/ingest/types"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestSyndicationTokenIsDeterministic(t *testing.T) {
	a := SyndicationToken("1234567890123456789")
	b := SyndicationToken("1234567890123456789")
	if a != b {
		t.Fatalf("SyndicationToken() mismatch: %q != %q", a, b)
	}
	if a == "" {
		t.Fatal("SyndicationToken() returned empty string")
	}
}

func TestParseTwitterTimeSupportsRFC1123AndRFC3339(t *testing.T) {
	rfc1123, err := parseTwitterTime("Tue, 07 Apr 2026 10:00:00 GMT")
	if err != nil {
		t.Fatalf("parseTwitterTime(RFC1123) error = %v", err)
	}
	if rfc1123.UTC() != time.Date(2026, 4, 7, 10, 0, 0, 0, time.UTC) {
		t.Fatalf("RFC1123 parse = %v", rfc1123.UTC())
	}

	rfc3339, err := parseTwitterTime("2026-04-07T10:00:00Z")
	if err != nil {
		t.Fatalf("parseTwitterTime(RFC3339) error = %v", err)
	}
	if rfc3339.UTC() != time.Date(2026, 4, 7, 10, 0, 0, 0, time.UTC) {
		t.Fatalf("RFC3339 parse = %v", rfc3339.UTC())
	}
}

func TestParseSyndicationDataUsesPreviewWhenFullArticleMissing(t *testing.T) {
	raw := syndicationPayload{
		IDStr:     "123",
		CreatedAt: "Tue, 07 Apr 2026 10:00:00 GMT",
		User:      syndicationUser{ScreenName: "alice", Name: "Alice", IDStr: "u1"},
		Article:   &syndicationArticle{Title: "Long Article", PreviewText: "Preview body", RestID: "article-1"},
	}

	got, err := ParseSyndicationData(raw, "")
	if err != nil {
		t.Fatalf("ParseSyndicationData() error = %v", err)
	}
	if got.Metadata.Twitter == nil {
		t.Fatal("Metadata.Twitter is nil")
	}
	if got.Metadata.Twitter.IsArticle != true {
		t.Fatalf("is_article = %#v, want true", got.Metadata.Twitter.IsArticle)
	}
	if got.Metadata.Twitter.ArticleURL != "https://x.com/i/article/article-1" {
		t.Fatalf("article_url = %#v", got.Metadata.Twitter.ArticleURL)
	}
	if len(got.Metadata.Twitter.SourceLinks) != 1 {
		t.Fatalf("len(source_links) = %d, want 1", len(got.Metadata.Twitter.SourceLinks))
	}
	if got.Metadata.Twitter.SourceLinks[0] != "https://x.com/i/article/article-1" {
		t.Fatalf("source_links[0] = %#v", got.Metadata.Twitter.SourceLinks[0])
	}
	wantContent := "[X长文·仅预览] Long Article\n\nPreview body\n\n[注：以上为系统自动截取的预览摘要，X长文全文需登录后查看，原文链接：https://x.com/i/article/article-1]"
	if got.Content != wantContent {
		t.Fatalf("Content = %q, want %q", got.Content, wantContent)
	}
}

func TestParseSyndicationDataPrefersFullArticleContentWhenPresent(t *testing.T) {
	raw := syndicationPayload{
		IDStr:     "123",
		CreatedAt: "Tue, 07 Apr 2026 10:00:00 GMT",
		User:      syndicationUser{ScreenName: "alice", Name: "Alice", IDStr: "u1"},
		Article:   &syndicationArticle{Title: "Long Article", PreviewText: "Preview body", RestID: "article-1"},
	}

	got, err := ParseSyndicationData(raw, "Full article body")
	if err != nil {
		t.Fatalf("ParseSyndicationData() error = %v", err)
	}
	if got.Content != "Full article body" {
		t.Fatalf("Content = %q, want %q", got.Content, "Full article body")
	}
}

func TestParseSyndicationDataIgnoresUnsupportedArticleShellAndFallsBackToPreview(t *testing.T) {
	raw := syndicationPayload{
		IDStr:     "123",
		CreatedAt: "Tue, 07 Apr 2026 10:00:00 GMT",
		User:      syndicationUser{ScreenName: "alice", Name: "Alice", IDStr: "u1"},
		Article:   &syndicationArticle{Title: "Long Article", PreviewText: "Preview body", RestID: "article-1"},
	}

	got, err := ParseSyndicationData(raw, "This page is not supported")
	if err != nil {
		t.Fatalf("ParseSyndicationData() error = %v", err)
	}
	want := "[X长文·仅预览] Long Article\n\nPreview body\n\n[注：以上为系统自动截取的预览摘要，X长文全文需登录后查看，原文链接：https://x.com/i/article/article-1]"
	if got.Content != want {
		t.Fatalf("Content = %q, want %q", got.Content, want)
	}
}

func TestExtractGraphQLNoteText(t *testing.T) {
	payload := map[string]any{
		"data": map[string]any{
			"tweetResult": map[string]any{
				"result": map[string]any{
					"note_tweet": map[string]any{
						"note_tweet_results": map[string]any{
							"result": map[string]any{
								"text": "full longform body",
							},
						},
					},
				},
			},
		},
	}
	if got := extractGraphQLNoteText(payload); got != "full longform body" {
		t.Fatalf("extractGraphQLNoteText() = %q, want full longform body", got)
	}
}

func TestSyndicationHTTPClientFetchByID_HydratesNoteTweetTextFromGraphQL(t *testing.T) {
	client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		switch {
		case strings.Contains(req.URL.Host, "cdn.syndication.twimg.com"):
			body := `{
				"id_str":"2043148422224785477",
				"text":"short text",
				"created_at":"2026-04-12T02:05:18Z",
				"user":{"screen_name":"qinbafrank","name":"qinbafrank","id_str":"1338075202798809089"},
				"note_tweet":{"id":"Tm90ZVR3ZWV0UmVzdWx0czoyMDQzMTQ4NDIyMDg2MzA3ODQw"}
			}`
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Content-Type": []string{"application/json"}},
				Body:       io.NopCloser(strings.NewReader(body)),
			}, nil
		case strings.Contains(req.URL.Path, "/TweetResultByRestId"):
			body := `{
				"data":{"tweetResult":{"result":{"note_tweet":{"note_tweet_results":{"result":{"text":"full longform body with extra paragraph"}}}}}}
			}`
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Content-Type": []string{"application/json"}},
				Body:       io.NopCloser(strings.NewReader(body)),
			}, nil
		case strings.Contains(req.URL.Path, "/TweetDetail"):
			body := `{"data":{"threaded_conversation_with_injections_v2":{"instructions":[]}}}`
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Content-Type": []string{"application/json"}},
				Body:       io.NopCloser(strings.NewReader(body)),
			}, nil
		default:
			t.Fatalf("unexpected request: %s", req.URL.String())
			return nil, nil
		}
	})}

	s := NewSyndicationHTTPClient(client)
	s.authToken = "auth-token"
	s.ct0 = "ct0-token"

	items, err := s.FetchByID(context.Background(), "2043148422224785477")
	if err != nil {
		t.Fatalf("FetchByID() error = %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("len(items) = %d, want 1", len(items))
	}
	if !strings.Contains(items[0].Content, "full longform body with extra paragraph") {
		t.Fatalf("Content = %q, want hydrated longform text", items[0].Content)
	}
}

func TestSyndicationHTTPClientFetchByID_HydratesQuotedTweetLongformTextFromGraphQL(t *testing.T) {
	client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		switch {
		case strings.Contains(req.URL.Host, "cdn.syndication.twimg.com"):
			body := `{
				"id_str":"2043148422224785477",
				"text":"short root text",
				"created_at":"2026-04-12T02:05:18Z",
				"user":{"screen_name":"qinbafrank","name":"qinbafrank","id_str":"1338075202798809089"},
				"quoted_tweet":{
					"id_str":"2042807337417871695",
					"text":"short quoted text",
					"created_at":"2026-04-11T03:29:57Z",
					"user":{"screen_name":"qinbafrank","name":"qinbafrank","id_str":"1338075202798809089"}
				}
			}`
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Content-Type": []string{"application/json"}},
				Body:       io.NopCloser(strings.NewReader(body)),
			}, nil
		case strings.Contains(req.URL.Path, "/TweetResultByRestId") && strings.Contains(req.URL.RawQuery, "2042807337417871695"):
			body := `{
				"data":{"tweetResult":{"result":{"note_tweet":{"note_tweet_results":{"result":{"text":"full quoted longform body"}}}}}}
			}`
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Content-Type": []string{"application/json"}},
				Body:       io.NopCloser(strings.NewReader(body)),
			}, nil
		case strings.Contains(req.URL.Path, "/TweetResultByRestId"):
			body := `{"data":{"tweetResult":{"result":{}}}}`
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Content-Type": []string{"application/json"}},
				Body:       io.NopCloser(strings.NewReader(body)),
			}, nil
		case strings.Contains(req.URL.Path, "/TweetDetail"):
			body := `{"data":{"threaded_conversation_with_injections_v2":{"instructions":[]}}}`
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Content-Type": []string{"application/json"}},
				Body:       io.NopCloser(strings.NewReader(body)),
			}, nil
		default:
			t.Fatalf("unexpected request: %s", req.URL.String())
			return nil, nil
		}
	})}

	s := NewSyndicationHTTPClient(client)
	s.authToken = "auth-token"
	s.ct0 = "ct0-token"

	items, err := s.FetchByID(context.Background(), "2043148422224785477")
	if err != nil {
		t.Fatalf("FetchByID() error = %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("len(items) = %d, want 1", len(items))
	}
	if len(items[0].Quotes) != 1 {
		t.Fatalf("len(Quotes) = %d, want 1", len(items[0].Quotes))
	}
	if items[0].Quotes[0].Content != "full quoted longform body" {
		t.Fatalf("Quote.Content = %q, want hydrated quoted longform body", items[0].Quotes[0].Content)
	}
}

func TestExtractGraphQLArticlePlainText(t *testing.T) {
	payload := map[string]any{
		"data": map[string]any{
			"tweetResult": map[string]any{
				"result": map[string]any{
					"article": map[string]any{
						"article_results": map[string]any{
							"result": map[string]any{
								"plain_text": "full article body",
							},
						},
					},
				},
			},
		},
	}
	if got := extractGraphQLArticlePlainText(payload); got != "full article body" {
		t.Fatalf("extractGraphQLArticlePlainText() = %q, want full article body", got)
	}
}

func TestSyndicationHTTPClientFetchByID_PrefersGraphQLArticlePlainText(t *testing.T) {
	client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		switch {
		case strings.Contains(req.URL.Host, "cdn.syndication.twimg.com"):
			body := `{
				"id_str":"2026305745872998803",
				"text":"preview text",
				"created_at":"2026-02-24T14:38:31Z",
				"user":{"screen_name":"RayDalio","name":"Ray Dalio","id_str":"62603893"},
				"article":{"title":"Investing In Light Of The Big Cycle","preview_text":"Preview body","rest_id":"2026295690318454784"}
			}`
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Content-Type": []string{"application/json"}},
				Body:       io.NopCloser(strings.NewReader(body)),
			}, nil
		case strings.Contains(req.URL.Path, "/TweetResultByRestId"):
			body := `{"data":{"tweetResult":{"result":{"article":{"article_results":{"result":{"plain_text":"full article body from graphql"}}}}}}}`
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Content-Type": []string{"application/json"}},
				Body:       io.NopCloser(strings.NewReader(body)),
			}, nil
		case strings.Contains(req.URL.Path, "/TweetDetail"):
			body := `{"data":{"threaded_conversation_with_injections_v2":{"instructions":[]}}}`
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Content-Type": []string{"application/json"}},
				Body:       io.NopCloser(strings.NewReader(body)),
			}, nil
		default:
			t.Fatalf("unexpected request: %s", req.URL.String())
			return nil, nil
		}
	})}

	s := NewSyndicationHTTPClient(client)
	s.authToken = "auth-token"
	s.ct0 = "ct0-token"

	items, err := s.FetchByID(context.Background(), "2026305745872998803")
	if err != nil {
		t.Fatalf("FetchByID() error = %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("len(items) = %d, want 1", len(items))
	}
	if items[0].Content != "full article body from graphql" {
		t.Fatalf("Content = %q, want graphql article plaintext", items[0].Content)
	}
}

func TestExtractSelfThreadIDsFromDetail(t *testing.T) {
	payload := map[string]any{
		"data": map[string]any{
			"threaded_conversation_with_injections_v2": map[string]any{
				"instructions": []any{
					map[string]any{"entry": map[string]any{"content": map[string]any{"itemContent": map[string]any{"tweet_results": map[string]any{"result": map[string]any{
						"__typename": "Tweet",
						"rest_id":    "1",
						"legacy": map[string]any{
							"user_id_str":         "u1",
							"conversation_id_str": "1",
						},
					}}}}}},
					map[string]any{"entry": map[string]any{"content": map[string]any{"itemContent": map[string]any{"tweet_results": map[string]any{"result": map[string]any{
						"__typename": "Tweet",
						"rest_id":    "2",
						"legacy": map[string]any{
							"user_id_str":               "u1",
							"conversation_id_str":       "1",
							"in_reply_to_status_id_str": "1",
						},
					}}}}}},
					map[string]any{"entry": map[string]any{"content": map[string]any{"itemContent": map[string]any{"tweet_results": map[string]any{"result": map[string]any{
						"__typename": "Tweet",
						"rest_id":    "x",
						"legacy": map[string]any{
							"user_id_str":         "u2",
							"conversation_id_str": "1",
						},
					}}}}}},
				},
			},
		},
	}
	got := extractSelfThreadIDsFromDetail(payload, "1")
	if len(got) != 2 || got[0] != "1" || got[1] != "2" {
		t.Fatalf("extractSelfThreadIDsFromDetail() = %#v, want [1 2]", got)
	}
}

func TestExtractSelfThreadIDsFromDetail_SkipsAuthorReplyBranch(t *testing.T) {
	payload := map[string]any{
		"data": map[string]any{
			"threaded_conversation_with_injections_v2": map[string]any{
				"instructions": []any{
					map[string]any{"entry": map[string]any{"content": map[string]any{"itemContent": map[string]any{"tweet_results": map[string]any{"result": map[string]any{
						"__typename": "Tweet", "rest_id": "1", "legacy": map[string]any{"user_id_str": "u1", "conversation_id_str": "1"},
					}}}}}},
					map[string]any{"entry": map[string]any{"content": map[string]any{"itemContent": map[string]any{"tweet_results": map[string]any{"result": map[string]any{
						"__typename": "Tweet", "rest_id": "2", "legacy": map[string]any{"user_id_str": "u1", "conversation_id_str": "1", "in_reply_to_status_id_str": "1"},
					}}}}}},
					map[string]any{"entry": map[string]any{"content": map[string]any{"itemContent": map[string]any{"tweet_results": map[string]any{"result": map[string]any{
						"__typename": "Tweet", "rest_id": "3", "legacy": map[string]any{"user_id_str": "u1", "conversation_id_str": "1", "in_reply_to_status_id_str": "2"},
					}}}}}},
					map[string]any{"entry": map[string]any{"content": map[string]any{"itemContent": map[string]any{"tweet_results": map[string]any{"result": map[string]any{
						"__typename": "Tweet", "rest_id": "9", "legacy": map[string]any{"user_id_str": "u1", "conversation_id_str": "1", "in_reply_to_status_id_str": "x-other-user-reply"},
					}}}}}},
				},
			},
		},
	}
	got := extractSelfThreadIDsFromDetail(payload, "1")
	if len(got) != 3 || got[0] != "1" || got[1] != "2" || got[2] != "3" {
		t.Fatalf("extractSelfThreadIDsFromDetail() = %#v, want [1 2 3]", got)
	}
}

func TestHydrateThreadMergesOrderedSegments(t *testing.T) {
	client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		switch {
		case strings.Contains(req.URL.Path, "/TweetDetail"):
			body := `{"data":{"threaded_conversation_with_injections_v2":{"instructions":[
				{"entry":{"content":{"itemContent":{"tweet_results":{"result":{"__typename":"Tweet","rest_id":"1","legacy":{"user_id_str":"u1","conversation_id_str":"1"}}}}}}},
				{"entry":{"content":{"itemContent":{"tweet_results":{"result":{"__typename":"Tweet","rest_id":"2","legacy":{"user_id_str":"u1","conversation_id_str":"1","in_reply_to_status_id_str":"1"}}}}}}}
			]}}}`
			return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(body))}, nil
		case strings.Contains(req.URL.Host, "cdn.syndication.twimg.com") && strings.Contains(req.URL.RawQuery, "id=2"):
			body := `{"id_str":"2","text":"segment two","created_at":"2026-04-12T02:06:18Z","user":{"screen_name":"alice","name":"Alice","id_str":"u1"},"self_thread":{"id_str":"1"},"mediaDetails":[{"type":"photo","media_url_https":"https://img.test/seg2.jpg"}]}`
			return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(body))}, nil
		default:
			t.Fatalf("unexpected request: %s", req.URL.String())
			return nil, nil
		}
	})}

	s := NewSyndicationHTTPClient(client)
	s.authToken = "auth"
	s.ct0 = "ct0"

	item := &types.RawContent{
		Source:     "twitter",
		ExternalID: "1",
		Content:    "segment one\n\n[附件#1 图片]",
		AuthorName: "Alice",
		AuthorID:   "u1",
		URL:        "https://x.com/alice/status/1",
		Metadata:   types.RawMetadata{Thread: &types.ThreadContext{IsSelfThread: true}},
		Attachments: []types.Attachment{{
			Type: "image",
			URL:  "https://img.test/root.jpg",
		}},
	}

	s.hydrateThread(context.Background(), item)
	if item.Content != "segment one\n\n[附件#1 图片]\n\nsegment two\n\n[附件#2 图片]" {
		t.Fatalf("Content = %q", item.Content)
	}
	if len(item.ThreadSegments) != 2 {
		t.Fatalf("len(ThreadSegments) = %d, want 2", len(item.ThreadSegments))
	}
	if len(item.Attachments) != 2 {
		t.Fatalf("len(Attachments) = %d, want 2", len(item.Attachments))
	}
	if item.ThreadSegments[1].ExternalID != "2" {
		t.Fatalf("ThreadSegments[1] = %#v", item.ThreadSegments[1])
	}
	if item.Metadata.Thread.RootExternalID != "1" {
		t.Fatalf("RootExternalID = %q", item.Metadata.Thread.RootExternalID)
	}
	if item.Metadata.Thread.ThreadPosition == nil || *item.Metadata.Thread.ThreadPosition != 1 {
		t.Fatalf("ThreadPosition = %#v", item.Metadata.Thread.ThreadPosition)
	}
}

func TestParseSyndicationDataTrimsWhitespaceUserNamesBeforeFallback(t *testing.T) {
	raw := syndicationPayload{
		IDStr:     "123",
		Text:      "hello",
		CreatedAt: "Tue, 07 Apr 2026 10:00:00 GMT",
		User:      syndicationUser{ScreenName: "alice", Name: " ", IDStr: "u1"},
	}

	got, err := ParseSyndicationData(raw, "")
	if err != nil {
		t.Fatalf("ParseSyndicationData() error = %v", err)
	}
	if got.AuthorName != "alice" {
		t.Fatalf("AuthorName = %q, want %q", got.AuthorName, "alice")
	}
}

type fakeAPIClient struct {
	items []types.RawContent
	err   error
	calls int
}

func (f *fakeAPIClient) FetchByID(_ context.Context, _ string) ([]types.RawContent, error) {
	f.calls++
	return f.items, f.err
}

func (f *fakeAPIClient) DiscoverTimeline(_ context.Context, _ string, _ string) ([]types.DiscoveryItem, error) {
	return nil, nil
}

type fakeSyndicationClient struct {
	items []types.RawContent
	err   error
	calls int
}

func (f *fakeSyndicationClient) FetchByID(_ context.Context, _ string) ([]types.RawContent, error) {
	f.calls++
	return f.items, f.err
}

func TestCollector_PrefersAPIWhenAvailable(t *testing.T) {
	api := &fakeAPIClient{
		items: []types.RawContent{{Source: "twitter", ExternalID: "123"}},
	}
	syndication := &fakeSyndicationClient{
		items: []types.RawContent{{Source: "twitter", ExternalID: "fallback"}},
	}
	c := New(api, syndication)

	got, err := c.Fetch(context.Background(), types.ParsedURL{
		Platform:     types.PlatformTwitter,
		ContentType:  types.ContentTypePost,
		PlatformID:   "123",
		CanonicalURL: "https://x.com/alice/status/123",
	})
	if err != nil {
		t.Fatalf("Fetch() error = %v", err)
	}
	if api.calls != 1 {
		t.Fatalf("api calls = %d, want 1", api.calls)
	}
	if syndication.calls != 0 {
		t.Fatalf("syndication calls = %d, want 0", syndication.calls)
	}
	if len(got) != 1 || got[0].ExternalID != "123" {
		t.Fatalf("got = %#v", got)
	}
}

func TestCollector_FallsBackToSyndicationWhenAPIUnavailable(t *testing.T) {
	api := &fakeAPIClient{
		err: errors.New("api down"),
	}
	syndication := &fakeSyndicationClient{
		items: []types.RawContent{{Source: "twitter", ExternalID: "123"}},
	}
	c := New(api, syndication)

	got, err := c.Fetch(context.Background(), types.ParsedURL{
		Platform:     types.PlatformTwitter,
		ContentType:  types.ContentTypePost,
		PlatformID:   "123",
		CanonicalURL: "https://x.com/alice/status/123",
	})
	if err != nil {
		t.Fatalf("Fetch() error = %v", err)
	}
	if api.calls != 1 {
		t.Fatalf("api calls = %d, want 1", api.calls)
	}
	if syndication.calls != 1 {
		t.Fatalf("syndication calls = %d, want 1", syndication.calls)
	}
	if len(got) != 1 || got[0].ExternalID != "123" {
		t.Fatalf("got = %#v", got)
	}
}

func TestParseSyndicationData_QuoteTweet(t *testing.T) {
	data := syndicationPayload{
		IDStr:     "200",
		Text:      "I disagree with this take",
		CreatedAt: "Tue, 07 Apr 2026 10:00:00 GMT",
		User:      syndicationUser{ScreenName: "commenter", Name: "Commenter", IDStr: "u2"},
		QuotedTweet: &syndicationQuote{
			IDStr:     "100",
			Text:      "The Fed will cut rates in Q3",
			CreatedAt: "Mon, 06 Apr 2026 08:00:00 GMT",
			User:      syndicationUser{ScreenName: "analyst", Name: "Analyst", IDStr: "u1"},
		},
	}
	rc, err := ParseSyndicationData(data, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(rc.Quotes) != 1 {
		t.Fatalf("expected 1 quote, got %d", len(rc.Quotes))
	}
	q := rc.Quotes[0]
	if q.Relation != "quote_tweet" {
		t.Errorf("relation = %q, want quote_tweet", q.Relation)
	}
	if q.AuthorName != "Analyst" {
		t.Errorf("author = %q, want Analyst", q.AuthorName)
	}
	if q.Content != "The Fed will cut rates in Q3" {
		t.Errorf("content = %q", q.Content)
	}
	if !strings.Contains(rc.Content, "[引用#1 @Analyst · 2026-04-06]") {
		t.Errorf("Content should contain quote placeholder, got:\n%s", rc.Content)
	}
	if len(rc.References) != 0 {
		t.Fatalf("References = %#v, want none for pure quote tweet", rc.References)
	}
	if !strings.Contains(rc.Content, "I disagree with this take") {
		t.Errorf("Content should contain main text")
	}
}

func TestParseSyndicationData_NoQuote(t *testing.T) {
	data := syndicationPayload{
		IDStr:     "300",
		Text:      "Just a normal tweet",
		CreatedAt: "Tue, 07 Apr 2026 10:00:00 GMT",
		User:      syndicationUser{ScreenName: "user1", Name: "User One", IDStr: "u3"},
	}
	rc, err := ParseSyndicationData(data, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(rc.Quotes) != 0 {
		t.Errorf("expected 0 quotes, got %d", len(rc.Quotes))
	}
	if rc.Content != "Just a normal tweet" {
		t.Errorf("content = %q", rc.Content)
	}
}

func TestParseSyndicationData_MediaAttachments(t *testing.T) {
	data := syndicationPayload{
		IDStr:     "400",
		Text:      "Check out this chart",
		CreatedAt: "Tue, 07 Apr 2026 10:00:00 GMT",
		User:      syndicationUser{ScreenName: "trader", Name: "Trader", IDStr: "u4"},
		Media: []syndicationMedia{
			{Type: "photo", MediaURLHTTPS: "https://pbs.twimg.com/media/abc.jpg"},
			{Type: "video", MediaURLHTTPS: "https://pbs.twimg.com/ext_tw_video/xyz.mp4"},
		},
	}
	rc, err := ParseSyndicationData(data, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(rc.Attachments) != 2 {
		t.Fatalf("expected 2 attachments, got %d", len(rc.Attachments))
	}
	if rc.Attachments[0].Type != "image" {
		t.Errorf("first attachment type = %q, want image", rc.Attachments[0].Type)
	}
	if rc.Attachments[0].URL != "https://pbs.twimg.com/media/abc.jpg" {
		t.Errorf("first attachment URL = %q", rc.Attachments[0].URL)
	}
	if rc.Attachments[1].Type != "video" {
		t.Errorf("second attachment type = %q, want video", rc.Attachments[1].Type)
	}
	// Video without VideoInfo variants: URL falls back to poster URL.
	if rc.Attachments[1].URL != "https://pbs.twimg.com/ext_tw_video/xyz.mp4" {
		t.Errorf("second attachment URL = %q, want poster fallback", rc.Attachments[1].URL)
	}
	if rc.Attachments[1].PosterURL != "https://pbs.twimg.com/ext_tw_video/xyz.mp4" {
		t.Errorf("second attachment PosterURL = %q", rc.Attachments[1].PosterURL)
	}
	if !strings.Contains(rc.Content, "[附件#1 图片]") || !strings.Contains(rc.Content, "[附件#2 视频]") {
		t.Errorf("Content should contain attachment placeholders, got:\n%s", rc.Content)
	}
}

func TestParseSyndicationData_ExtractsBodyPostReferences(t *testing.T) {
	data := syndicationPayload{
		IDStr:     "401",
		Text:      "看这里 https://x.com/example/status/12345 还有这条 https://weibo.com/123456/AbCdEf",
		CreatedAt: "Tue, 07 Apr 2026 10:00:00 GMT",
		User:      syndicationUser{ScreenName: "trader", Name: "Trader", IDStr: "u4"},
	}
	rc, err := ParseSyndicationData(data, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(rc.References) != 2 {
		t.Fatalf("len(References) = %d, want 2", len(rc.References))
	}
	if rc.References[0].Platform != "twitter" || rc.References[0].ExternalID != "12345" {
		t.Fatalf("References[0] = %#v", rc.References[0])
	}
	if rc.References[1].Platform != "weibo" || rc.References[1].ExternalID != "AbCdEf" {
		t.Fatalf("References[1] = %#v", rc.References[1])
	}
	if !strings.Contains(rc.Content, "看这里 [参考#1 X帖子] 还有这条 [参考#2 微博]") {
		t.Fatalf("Content = %q, want reference placeholders", rc.Content)
	}
}

func TestResolveReferences_ResolvesShortLinksToPostReferences(t *testing.T) {
	raw := types.RawContent{
		Content: "main [参考#1 链接]",
		References: []types.Reference{{
			Kind: "link",
			URL:  "https://t.co/abc123",
		}},
	}

	resolveReferences(context.Background(), func(_ context.Context, raw string) (string, error) {
		if raw != "https://t.co/abc123" {
			t.Fatalf("raw = %q", raw)
		}
		return "https://x.com/example/status/12345", nil
	}, &raw)

	if raw.References[0].Platform != "twitter" || raw.References[0].ExternalID != "12345" {
		t.Fatalf("References[0] = %#v", raw.References[0])
	}
	if raw.Content != "main [参考#1 X帖子]" {
		t.Fatalf("Content = %q, want resolved X placeholder", raw.Content)
	}
}

func TestResolveReferences_StripsOldPlaceholdersAndSkipsSelfReference(t *testing.T) {
	raw := types.RawContent{
		Source:     "twitter",
		ExternalID: "999",
		Content:    "main body\n\n[参考#1 链接]\n\n[附件#1 图片]",
		Attachments: []types.Attachment{{
			Type: "image",
			URL:  "https://pbs.twimg.com/media/a.jpg",
		}},
		References: []types.Reference{{
			Kind: "link",
			URL:  "https://t.co/self",
		}},
	}

	resolveReferences(context.Background(), func(_ context.Context, raw string) (string, error) {
		return "https://twitter.com/example/status/999/photo/1", nil
	}, &raw)

	if len(raw.References) != 0 {
		t.Fatalf("References = %#v, want self reference filtered", raw.References)
	}
	if raw.Content != "main body\n\n[附件#1 图片]" {
		t.Fatalf("Content = %q", raw.Content)
	}
}

func TestParseSyndicationData_IgnoresEmptyQuotedTweet(t *testing.T) {
	data := syndicationPayload{
		IDStr:       "200",
		Text:        "main tweet",
		CreatedAt:   "Tue, 07 Apr 2026 10:00:00 GMT",
		User:        syndicationUser{ScreenName: "commenter", Name: "Commenter", IDStr: "u2"},
		QuotedTweet: &syndicationQuote{},
	}
	rc, err := ParseSyndicationData(data, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(rc.Quotes) != 0 {
		t.Fatalf("expected empty quoted_tweet to be ignored, got %d quotes", len(rc.Quotes))
	}
	if rc.Content != "main tweet" {
		t.Fatalf("Content = %q, want %q", rc.Content, "main tweet")
	}
}

func TestParseSyndicationData_QuotedNoteTweetUsesLongFormText(t *testing.T) {
	note := &syndicationNote{}
	note.NoteResults.Result.Text = "This is the full long quoted note tweet text"

	data := syndicationPayload{
		IDStr:     "200",
		Text:      "commentary",
		CreatedAt: "Tue, 07 Apr 2026 10:00:00 GMT",
		User:      syndicationUser{ScreenName: "commenter", Name: "Commenter", IDStr: "u2"},
		QuotedTweet: &syndicationQuote{
			IDStr:     "100",
			Text:      "short version",
			CreatedAt: "Mon, 06 Apr 2026 08:00:00 GMT",
			User:      syndicationUser{ScreenName: "analyst", Name: "Analyst", IDStr: "u1"},
			NoteTweet: note,
		},
	}
	rc, err := ParseSyndicationData(data, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(rc.Quotes) != 1 {
		t.Fatalf("expected 1 quote, got %d", len(rc.Quotes))
	}
	if rc.Quotes[0].Content != "This is the full long quoted note tweet text" {
		t.Fatalf("expected note_tweet text, got %q", rc.Quotes[0].Content)
	}
}

// --- New tests ---

func TestParseSyndicationData_SelfThread(t *testing.T) {
	data := syndicationPayload{
		IDStr:     "500",
		Text:      "This is a reply in my own thread",
		CreatedAt: "Tue, 07 Apr 2026 10:00:00 GMT",
		User:      syndicationUser{ScreenName: "alice", Name: "Alice", IDStr: "u1"},
		SelfThread: &syndicationThread{
			IDStr: "490",
		},
	}
	rc, err := ParseSyndicationData(data, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rc.Metadata.Thread == nil {
		t.Fatal("expected Thread to be set, got nil")
	}
	th := rc.Metadata.Thread
	if th.ThreadID != "490" {
		t.Errorf("ThreadID = %q, want 490", th.ThreadID)
	}
	if th.ThreadScope != types.ThreadScopeSelfThread {
		t.Errorf("ThreadScope = %q, want %q", th.ThreadScope, types.ThreadScopeSelfThread)
	}
	if th.RootExternalID != "490" {
		t.Errorf("RootExternalID = %q, want 490", th.RootExternalID)
	}
	if !th.IsSelfThread {
		t.Error("IsSelfThread = false, want true")
	}
	if !th.ThreadIncomplete {
		t.Error("ThreadIncomplete = false, want true")
	}
}

func TestParseSyndicationData_SelfThreadEmpty(t *testing.T) {
	data := syndicationPayload{
		IDStr:      "500",
		Text:       "standalone tweet",
		CreatedAt:  "Tue, 07 Apr 2026 10:00:00 GMT",
		User:       syndicationUser{ScreenName: "alice", Name: "Alice", IDStr: "u1"},
		SelfThread: &syndicationThread{IDStr: ""},
	}
	rc, err := ParseSyndicationData(data, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rc.Metadata.Thread != nil {
		t.Fatalf("expected Thread to be nil for empty SelfThread IDStr, got %+v", rc.Metadata.Thread)
	}
}

func TestParseSyndicationData_VideoWithVariants(t *testing.T) {
	br200 := 200000
	br800 := 800000
	br2000 := 2000000

	data := syndicationPayload{
		IDStr:     "600",
		Text:      "video tweet",
		CreatedAt: "Tue, 07 Apr 2026 10:00:00 GMT",
		User:      syndicationUser{ScreenName: "videographer", Name: "Videographer", IDStr: "u5"},
		Media: []syndicationMedia{
			{
				Type:          "video",
				MediaURLHTTPS: "https://pbs.twimg.com/ext_tw_video/poster.jpg",
				VideoInfo: &syndicationVideoInfo{
					Variants: []syndicationVariant{
						{URL: "https://video.twimg.com/low.mp4", ContentType: "video/mp4", Bitrate: &br200},
						{URL: "https://video.twimg.com/mid.mp4", ContentType: "video/mp4", Bitrate: &br800},
						{URL: "https://video.twimg.com/high.mp4", ContentType: "video/mp4", Bitrate: &br2000},
						{URL: "https://video.twimg.com/stream.m3u8", ContentType: "application/x-mpegURL"},
					},
				},
			},
		},
	}
	rc, err := ParseSyndicationData(data, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(rc.Attachments) != 1 {
		t.Fatalf("expected 1 attachment, got %d", len(rc.Attachments))
	}
	att := rc.Attachments[0]
	if att.Type != "video" {
		t.Errorf("type = %q, want video", att.Type)
	}
	if att.PosterURL != "https://pbs.twimg.com/ext_tw_video/poster.jpg" {
		t.Errorf("PosterURL = %q", att.PosterURL)
	}
	// 3 MP4 variants sorted by bitrate: [low, mid, high]. Middle index = 3/2 = 1 => mid.
	if att.URL != "https://video.twimg.com/mid.mp4" {
		t.Errorf("URL = %q, want mid.mp4", att.URL)
	}
}

func TestSelectPreferredMP4(t *testing.T) {
	br100 := 100000
	br500 := 500000
	br1500 := 1500000

	tests := []struct {
		name     string
		variants []syndicationVariant
		want     string
	}{
		{
			name:     "empty",
			variants: nil,
			want:     "",
		},
		{
			name: "no mp4",
			variants: []syndicationVariant{
				{URL: "https://v.com/stream.m3u8", ContentType: "application/x-mpegURL"},
			},
			want: "",
		},
		{
			name: "single mp4",
			variants: []syndicationVariant{
				{URL: "https://v.com/only.mp4", ContentType: "video/mp4", Bitrate: &br500},
			},
			want: "https://v.com/only.mp4",
		},
		{
			name: "two mp4",
			variants: []syndicationVariant{
				{URL: "https://v.com/low.mp4", ContentType: "video/mp4", Bitrate: &br100},
				{URL: "https://v.com/high.mp4", ContentType: "video/mp4", Bitrate: &br1500},
			},
			want: "https://v.com/high.mp4", // 2/2 = 1 => index 1
		},
		{
			name: "three mp4 picks middle",
			variants: []syndicationVariant{
				{URL: "https://v.com/high.mp4", ContentType: "video/mp4", Bitrate: &br1500},
				{URL: "https://v.com/low.mp4", ContentType: "video/mp4", Bitrate: &br100},
				{URL: "https://v.com/mid.mp4", ContentType: "video/mp4", Bitrate: &br500},
				{URL: "https://v.com/stream.m3u8", ContentType: "application/x-mpegURL"},
			},
			want: "https://v.com/mid.mp4", // sorted: [low, mid, high], 3/2=1 => mid
		},
		{
			name: "mp4 without bitrate treated as zero",
			variants: []syndicationVariant{
				{URL: "https://v.com/unknown.mp4", ContentType: "video/mp4"},
				{URL: "https://v.com/known.mp4", ContentType: "video/mp4", Bitrate: &br500},
			},
			want: "https://v.com/known.mp4", // sorted: [unknown(0), known(500)], 2/2=1 => known
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := selectPreferredMP4(tt.variants)
			if got != tt.want {
				t.Errorf("selectPreferredMP4() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestThreadContextMarksNestedReplyIncompleteWhenParentIncluded(t *testing.T) {
	tweet := struct {
		AuthorID         string
		ConversationID   string
		ReferencedTweets []struct {
			Type string
			ID   string
		}
	}{
		AuthorID:       "u1",
		ConversationID: "480",
		ReferencedTweets: []struct {
			Type string
			ID   string
		}{
			{Type: "replied_to", ID: "490"},
		},
	}
	includedAuthors := map[string]string{
		"490": "u1",
	}

	tc := &types.ThreadContext{
		ThreadID:       tweet.ConversationID,
		RootExternalID: tweet.ConversationID,
	}

	for _, ref := range tweet.ReferencedTweets {
		if ref.Type == "replied_to" {
			tc.ParentExternalID = ref.ID
			break
		}
	}

	if tc.ParentExternalID != "" {
		tc.ThreadIncomplete = true
		parentAuthor, found := includedAuthors[tc.ParentExternalID]
		if found {
			tc.IsSelfThread = parentAuthor == tweet.AuthorID
		}
	}

	if tc.ParentExternalID != "490" {
		t.Fatalf("ParentExternalID = %q, want 490", tc.ParentExternalID)
	}
	if tc.RootExternalID != "480" {
		t.Fatalf("RootExternalID = %q, want 480", tc.RootExternalID)
	}
	if !tc.IsSelfThread {
		t.Fatal("IsSelfThread = false, want true")
	}
	if !tc.ThreadIncomplete {
		t.Fatal("ThreadIncomplete = false, want true")
	}
}

func TestCollector_TranscribesVideoAttachments(t *testing.T) {
	// Override audioutil exec hooks to avoid needing ffmpeg/ffprobe.
	origProbe := audioutil.ExecProbe
	origExtract := audioutil.ExecExtract
	defer func() {
		audioutil.ExecProbe = origProbe
		audioutil.ExecExtract = origExtract
	}()

	audioutil.ExecProbe = func(_ context.Context, _ string) (time.Duration, error) {
		return 30 * time.Second, nil
	}
	audioutil.ExecExtract = func(_ context.Context, _, output string) error {
		return os.WriteFile(output, []byte("fake-audio"), 0o644)
	}

	// Mock HTTP: HEAD + GET for video download, then ASR upload.
	httpClient := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if strings.Contains(r.URL.Host, "asr.test") {
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Content-Type": []string{"application/json"}},
				Body:       io.NopCloser(strings.NewReader(`{"text":"video transcript here"}`)),
			}, nil
		}
		// Video download
		if r.Method == http.MethodHead {
			return &http.Response{
				StatusCode:    http.StatusOK,
				ContentLength: 100,
				Body:          io.NopCloser(strings.NewReader("")),
			}, nil
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader("fake-video-bytes")),
		}, nil
	})}

	br800 := 800000
	syndication := &fakeSyndicationClient{
		items: []types.RawContent{{
			Source:     "twitter",
			ExternalID: "999",
			Content:    "Look at this clip",
			Attachments: []types.Attachment{
				{Type: "image", URL: "https://pbs.twimg.com/media/img.jpg"},
				{Type: "video", URL: "https://video.twimg.com/mid.mp4", PosterURL: "https://pbs.twimg.com/poster.jpg"},
			},
		}},
	}
	_ = br800

	asr := audioutil.NewClient(httpClient, "https://asr.test", "test-key", "test-model")
	c := &Collector{
		syndication: syndication,
		asr:         asr,
		httpClient:  httpClient,
	}

	got, err := c.Fetch(context.Background(), types.ParsedURL{
		Platform:   types.PlatformTwitter,
		PlatformID: "999",
	})
	if err != nil {
		t.Fatalf("Fetch() error = %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("len(Fetch()) = %d, want 1", len(got))
	}

	rc := got[0]

	// Image attachment should be untouched.
	if rc.Attachments[0].Transcript != "" {
		t.Errorf("image attachment should have no transcript, got %q", rc.Attachments[0].Transcript)
	}

	// Video attachment should have transcript.
	vid := rc.Attachments[1]
	if vid.Transcript != "video transcript here" {
		t.Errorf("video Transcript = %q, want %q", vid.Transcript, "video transcript here")
	}
	if vid.TranscriptMethod != "whisper" {
		t.Errorf("video TranscriptMethod = %q, want %q", vid.TranscriptMethod, "whisper")
	}

	// Content should keep compact placeholders only.
	if !strings.Contains(rc.Content, "[附件#1 图片]") || !strings.Contains(rc.Content, "[附件#2 视频]") {
		t.Errorf("Content should contain attachment placeholders, got:\n%s", rc.Content)
	}
	if strings.Contains(rc.Content, "video transcript here") {
		t.Errorf("Content should not inline transcript text, got:\n%s", rc.Content)
	}
	if !strings.Contains(rc.Content, "Look at this clip") {
		t.Errorf("Content should still contain original text, got:\n%s", rc.Content)
	}
}

func TestCollector_TranscribesVideoAttachmentsWithoutLeadingBlankLinesWhenOriginalContentEmpty(t *testing.T) {
	origProbe := audioutil.ExecProbe
	origExtract := audioutil.ExecExtract
	defer func() {
		audioutil.ExecProbe = origProbe
		audioutil.ExecExtract = origExtract
	}()

	audioutil.ExecProbe = func(_ context.Context, _ string) (time.Duration, error) {
		return 30 * time.Second, nil
	}
	audioutil.ExecExtract = func(_ context.Context, _, output string) error {
		return os.WriteFile(output, []byte("fake-audio"), 0o644)
	}

	httpClient := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if strings.Contains(r.URL.Host, "asr.test") {
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Content-Type": []string{"application/json"}},
				Body:       io.NopCloser(strings.NewReader(`{"text":"video transcript here"}`)),
			}, nil
		}
		if r.Method == http.MethodHead {
			return &http.Response{
				StatusCode:    http.StatusOK,
				ContentLength: 100,
				Body:          io.NopCloser(strings.NewReader("")),
			}, nil
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader("fake-video-bytes")),
		}, nil
	})}

	syndication := &fakeSyndicationClient{
		items: []types.RawContent{{
			Source:     "twitter",
			ExternalID: "1000",
			Content:    "",
			Attachments: []types.Attachment{
				{Type: "video", URL: "https://video.twimg.com/mid.mp4", PosterURL: "https://pbs.twimg.com/poster.jpg"},
			},
		}},
	}

	asr := audioutil.NewClient(httpClient, "https://asr.test", "test-key", "test-model")
	c := &Collector{
		syndication: syndication,
		asr:         asr,
		httpClient:  httpClient,
	}

	got, err := c.Fetch(context.Background(), types.ParsedURL{
		Platform:   types.PlatformTwitter,
		PlatformID: "1000",
	})
	if err != nil {
		t.Fatalf("Fetch() error = %v", err)
	}
	want := "[附件#1 视频]"
	if got[0].Content != want {
		t.Fatalf("Content = %q, want %q", got[0].Content, want)
	}
}

func TestCollector_SkipsTranscriptionWhenURLEqualsPoster(t *testing.T) {
	syndication := &fakeSyndicationClient{
		items: []types.RawContent{{
			Source:     "twitter",
			ExternalID: "888",
			Content:    "video without variants",
			Attachments: []types.Attachment{
				{Type: "video", URL: "https://pbs.twimg.com/poster.jpg", PosterURL: "https://pbs.twimg.com/poster.jpg"},
			},
		}},
	}

	asr := audioutil.NewClient(http.DefaultClient, "https://asr.test", "test-key", "test-model")
	c := &Collector{
		syndication: syndication,
		asr:         asr,
		httpClient:  http.DefaultClient,
	}

	got, err := c.Fetch(context.Background(), types.ParsedURL{
		Platform:   types.PlatformTwitter,
		PlatformID: "888",
	})
	if err != nil {
		t.Fatalf("Fetch() error = %v", err)
	}
	// Video with URL == PosterURL should be skipped (it's a poster image, not a video).
	if got[0].Attachments[0].Transcript != "" {
		t.Errorf("should skip transcription when URL == PosterURL, got transcript %q", got[0].Attachments[0].Transcript)
	}
	if !strings.Contains(got[0].Content, "[附件#1 视频]") {
		t.Error("Content should contain attachment placeholder for poster-only video")
	}
}

func TestCollector_NoASRSkipsTranscription(t *testing.T) {
	syndication := &fakeSyndicationClient{
		items: []types.RawContent{{
			Source:     "twitter",
			ExternalID: "777",
			Content:    "tweet with video",
			Attachments: []types.Attachment{
				{Type: "video", URL: "https://video.twimg.com/v.mp4", PosterURL: "https://pbs.twimg.com/poster.jpg"},
			},
		}},
	}

	// No ASR configured.
	c := &Collector{syndication: syndication}

	got, err := c.Fetch(context.Background(), types.ParsedURL{
		Platform:   types.PlatformTwitter,
		PlatformID: "777",
	})
	if err != nil {
		t.Fatalf("Fetch() error = %v", err)
	}
	if got[0].Attachments[0].Transcript != "" {
		t.Error("without ASR, video should not be transcribed")
	}
}

func TestNewDefault_UsesDashscopeFallbackForASR(t *testing.T) {
	t.Setenv("ASR_API_KEY", "")
	t.Setenv("DASHSCOPE_API_KEY", "dash-key")
	t.Setenv("ASR_BASE_URL", "https://dashscope.example/v1")
	t.Setenv("ASR_MODEL", "sensevoice")

	c := NewDefault(t.TempDir(), &http.Client{})
	if c.asr == nil {
		t.Fatal("asr = nil, want client created from DASHSCOPE fallback")
	}
}
