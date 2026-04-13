package weibo

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/kumaloha/VariX/varix/ingest/sources/audioutil"
	"github.com/kumaloha/VariX/varix/ingest/types"
)

func TestParseGenVisitorJSONPExtractsTID(t *testing.T) {
	raw := `gen_callback({"data":{"tid":"abc123","confidence":100,"new_tid":true}})`
	got, err := parseGenVisitorJSONP(raw)
	if err != nil {
		t.Fatalf("parseGenVisitorJSONP() error = %v", err)
	}
	if got.TID != "abc123" {
		t.Fatalf("TID = %q, want %q", got.TID, "abc123")
	}
	if got.Confidence != 100 {
		t.Fatalf("Confidence = %d, want %d", got.Confidence, 100)
	}
	if !got.NewTID {
		t.Fatal("NewTID = false, want true")
	}
}

func TestStripHTMLRemovesTags(t *testing.T) {
	got := stripHTML(`<div>hello<br/>world <a href="https://x.com">x</a></div>`)
	if got != "hello world x" {
		t.Fatalf("stripHTML() = %q, want %q", got, "hello world x")
	}
}

type fakeHTTPClient struct {
	status         weiboStatus
	statusErr      error
	longText       string
	longTextErr    error
	discoveryItems []types.DiscoveryItem
	discoveryErr   error
}

func (f *fakeHTTPClient) FetchPost(_ context.Context, _ string) (weiboStatus, error) {
	return f.status, f.statusErr
}

func (f *fakeHTTPClient) FetchLongText(_ context.Context, _ string) (string, error) {
	return f.longText, f.longTextErr
}

func (f *fakeHTTPClient) DiscoverTimeline(_ context.Context, _ types.FollowTarget) ([]types.DiscoveryItem, error) {
	return f.discoveryItems, f.discoveryErr
}

func TestCollectorFetchUsesLongTextWhenPresent(t *testing.T) {
	c := New(&fakeHTTPClient{
		status: weiboStatus{
			ID:         "123",
			Mid:        "123",
			Bid:        "AbCdEf",
			TextRaw:    "short preview",
			IsLongText: true,
			CreatedAt:  "Tue Apr 07 10:00:00 +0800 2026",
			User:       weiboUser{IDStr: "u1", ScreenName: "Alice"},
		},
		longText: `<div>full<br/>text <a href="https://weibo.com">link</a></div>`,
	})

	got, err := c.Fetch(context.Background(), types.ParsedURL{
		Platform:     types.PlatformWeibo,
		ContentType:  types.ContentTypePost,
		PlatformID:   "AbCdEf",
		CanonicalURL: "https://weibo.com/u1/AbCdEf",
	})
	if err != nil {
		t.Fatalf("Fetch() error = %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("len(Fetch()) = %d, want 1", len(got))
	}
	if got[0].Source != "weibo" {
		t.Fatalf("Source = %q, want %q", got[0].Source, "weibo")
	}
	if got[0].Content != "full text link" {
		t.Fatalf("Content = %q, want %q", got[0].Content, "full text link")
	}
	if got[0].AuthorName != "Alice" {
		t.Fatalf("AuthorName = %q, want %q", got[0].AuthorName, "Alice")
	}
	if got[0].AuthorID != "u1" {
		t.Fatalf("AuthorID = %q, want %q", got[0].AuthorID, "u1")
	}
}

func TestCollectorFetchFallsBackToTextRaw(t *testing.T) {
	c := New(&fakeHTTPClient{
		status: weiboStatus{
			ID:         "124",
			Mid:        "124",
			Bid:        "XyZ987",
			TextRaw:    "plain text https://weibo.com/1234567/AbCdEf",
			IsLongText: false,
			CreatedAt:  "Tue Apr 07 10:00:00 +0800 2026",
			User:       weiboUser{IDStr: "u2", ScreenName: "Bob"},
		},
	})

	got, err := c.Fetch(context.Background(), types.ParsedURL{
		Platform:     types.PlatformWeibo,
		ContentType:  types.ContentTypePost,
		PlatformID:   "XyZ987",
		CanonicalURL: "https://weibo.com/u2/XyZ987",
	})
	if err != nil {
		t.Fatalf("Fetch() error = %v", err)
	}
	if !strings.Contains(got[0].Content, "plain text https://weibo.com/1234567/AbCdEf") {
		t.Fatalf("Content = %q, want main text", got[0].Content)
	}
	if !strings.Contains(got[0].Content, "[参考#1 微博]") {
		t.Fatalf("Content = %q, want reference placeholder", got[0].Content)
	}
	if len(got[0].References) != 1 {
		t.Fatalf("len(References) = %d, want 1", len(got[0].References))
	}
	if got[0].References[0].ExternalID != "AbCdEf" {
		t.Fatalf("References[0] = %#v", got[0].References[0])
	}
}

func TestCollectorDiscoverTimelinePassesThroughItems(t *testing.T) {
	items := []types.DiscoveryItem{{
		Platform:   types.PlatformWeibo,
		ExternalID: "AbCdEf",
		URL:        "https://weibo.com/u1/AbCdEf",
		AuthorName: "Alice",
		PostedAt:   time.Date(2026, 4, 7, 10, 0, 0, 0, time.UTC),
	}}
	c := New(&fakeHTTPClient{discoveryItems: items})

	got, err := c.Discover(context.Background(), types.FollowTarget{
		Kind:       types.KindNative,
		Platform:   "weibo",
		PlatformID: "u1",
		Locator:    "u1",
		AuthorName: "Alice",
	})
	if err != nil {
		t.Fatalf("Discover() error = %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("len(Discover()) = %d, want 1", len(got))
	}
	if got[0].ExternalID != "AbCdEf" {
		t.Fatalf("ExternalID = %q, want %q", got[0].ExternalID, "AbCdEf")
	}
}

func TestHTTPJSONClientDiscoverTimelineParsesLiveIdentityFields(t *testing.T) {
	httpClient := NewHTTPJSONClient(&http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			switch {
			case req.URL.Host == "passport.weibo.com" && req.URL.Path == "/visitor/genvisitor":
				return textResponse(http.StatusOK, `window.gen_callback && gen_callback({"retcode":20000000,"msg":"succ","data":{"tid":"abc123","new_tid":true}});`, nil), nil
			case req.URL.Host == "passport.weibo.com" && req.URL.Path == "/visitor/visitor":
				return textResponse(http.StatusOK, "", map[string]string{
					"Set-Cookie": "SUB=subcookie; Path=/; Domain=.weibo.com",
				}), nil
			case req.URL.Host == "weibo.com" && req.URL.Path == "/u/6315229624":
				return textResponse(http.StatusOK, "<html></html>", map[string]string{
					"Set-Cookie": "XSRF-TOKEN=xsrf-token; Path=/",
				}), nil
			case req.URL.Host == "weibo.com" && req.URL.Path == "/ajax/profile/getWaterFallContent":
				payload := map[string]any{
					"data": map[string]any{
						"list": []map[string]any{
							{
								"id":         4858157115641330,
								"idstr":      "4858157115641330",
								"mid":        "4858157115641330",
								"mblogid":    "MooVNnFzc",
								"created_at": "Sun Jan 15 12:52:59 +0800 2023",
							},
						},
					},
				}
				body, err := json.Marshal(payload)
				if err != nil {
					t.Fatalf("json.Marshal() error = %v", err)
				}
				return textResponse(http.StatusOK, string(body), map[string]string{
					"Content-Type": "application/json",
				}), nil
			default:
				t.Fatalf("unexpected request: %s %s", req.Method, req.URL.String())
				return nil, nil
			}
		}),
	}, "")

	got, err := httpClient.DiscoverTimeline(context.Background(), types.FollowTarget{
		Kind:       types.KindNative,
		Platform:   "weibo",
		PlatformID: "6315229624",
		Locator:    "https://weibo.com/6315229624",
		URL:        "https://weibo.com/6315229624",
		AuthorName: "鲨鱼菲特",
	})
	if err != nil {
		t.Fatalf("DiscoverTimeline() error = %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("len(DiscoverTimeline()) = %d, want 1", len(got))
	}
	if got[0].ExternalID != "MooVNnFzc" {
		t.Fatalf("ExternalID = %q, want %q", got[0].ExternalID, "MooVNnFzc")
	}
	if got[0].URL != "https://weibo.com/6315229624/MooVNnFzc" {
		t.Fatalf("URL = %q, want %q", got[0].URL, "https://weibo.com/6315229624/MooVNnFzc")
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return fn(req)
}

func TestHTTPJSONClientDiscoverTimelineBootstrapsProfileSession(t *testing.T) {
	t.Helper()

	var requests []*http.Request
	client := &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			requests = append(requests, req.Clone(req.Context()))

			switch {
			case req.URL.Host == "passport.weibo.com" && req.URL.Path == "/visitor/genvisitor":
				return textResponse(http.StatusOK, `window.gen_callback && gen_callback({"retcode":20000000,"msg":"succ","data":{"tid":"abc123","new_tid":true}});`, nil), nil
			case req.URL.Host == "passport.weibo.com" && req.URL.Path == "/visitor/visitor":
				return textResponse(http.StatusOK, "", map[string]string{
					"Set-Cookie": "SUB=subcookie; Path=/; Domain=.weibo.com",
				}), nil
			case req.URL.Host == "weibo.com" && req.URL.Path == "/u/6315229624":
				return textResponse(http.StatusOK, "<html></html>", map[string]string{
					"Set-Cookie": "XSRF-TOKEN=xsrf-token; Path=/",
				}), nil
			case req.URL.Host == "weibo.com" && req.URL.Path == "/ajax/profile/getWaterFallContent":
				if got := req.Header.Get("Referer"); got != "https://weibo.com/u/6315229624" {
					t.Fatalf("Referer = %q, want %q", got, "https://weibo.com/u/6315229624")
				}
				if got := req.Header.Get("x-xsrf-token"); got != "xsrf-token" {
					t.Fatalf("x-xsrf-token = %q, want %q", got, "xsrf-token")
				}
				if got := req.Header.Get("Cookie"); !strings.Contains(got, "SUB=subcookie") {
					t.Fatalf("Cookie = %q, want SUB cookie present", got)
				}
				if got := req.Header.Get("Cookie"); !strings.Contains(got, "XSRF-TOKEN=xsrf-token") {
					t.Fatalf("Cookie = %q, want XSRF cookie present", got)
				}
				return textResponse(http.StatusOK, `{"data":{"list":[]}}`, map[string]string{
					"Content-Type": "application/json",
				}), nil
			default:
				t.Fatalf("unexpected request: %s %s", req.Method, req.URL.String())
				return nil, nil
			}
		}),
	}

	httpClient := NewHTTPJSONClient(client, "")
	_, err := httpClient.DiscoverTimeline(context.Background(), types.FollowTarget{
		Kind:       types.KindNative,
		Platform:   "weibo",
		PlatformID: "6315229624",
		Locator:    "https://weibo.com/6315229624",
		URL:        "https://weibo.com/6315229624",
	})
	if err != nil {
		t.Fatalf("DiscoverTimeline() error = %v", err)
	}

	if len(requests) != 4 {
		t.Fatalf("request count = %d, want %d", len(requests), 4)
	}
}

func TestHTTPJSONClientFetchPostBootstrapsHomepageSession(t *testing.T) {
	httpClient := NewHTTPJSONClient(&http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			switch {
			case req.URL.Host == "passport.weibo.com" && req.URL.Path == "/visitor/genvisitor":
				return textResponse(http.StatusOK, `window.gen_callback && gen_callback({"retcode":20000000,"msg":"succ","data":{"tid":"abc123","new_tid":true}});`, nil), nil
			case req.URL.Host == "passport.weibo.com" && req.URL.Path == "/visitor/visitor":
				return textResponse(http.StatusOK, "", map[string]string{
					"Set-Cookie": "SUB=subcookie; Path=/; Domain=.weibo.com",
				}), nil
			case req.URL.Host == "weibo.com" && req.URL.Path == "/":
				return textResponse(http.StatusOK, "<html></html>", map[string]string{
					"Set-Cookie": "XSRF-TOKEN=xsrf-token; Path=/",
				}), nil
			case req.URL.Host == "weibo.com" && req.URL.Path == "/ajax/statuses/show":
				if got := req.Header.Get("Referer"); got != "https://weibo.com/" {
					t.Fatalf("Referer = %q, want %q", got, "https://weibo.com/")
				}
				if got := req.Header.Get("x-xsrf-token"); got != "xsrf-token" {
					t.Fatalf("x-xsrf-token = %q, want %q", got, "xsrf-token")
				}
				if got := req.Header.Get("Cookie"); !strings.Contains(got, "SUB=subcookie") {
					t.Fatalf("Cookie = %q, want SUB cookie present", got)
				}
				if got := req.Header.Get("Cookie"); !strings.Contains(got, "XSRF-TOKEN=xsrf-token") {
					t.Fatalf("Cookie = %q, want XSRF cookie present", got)
				}
				return textResponse(http.StatusOK, `{"idstr":"4858157115641330","bid":"MooVNnFzc","text_raw":"hello"}`, map[string]string{
					"Content-Type": "application/json",
				}), nil
			default:
				t.Fatalf("unexpected request: %s %s", req.Method, req.URL.String())
				return nil, nil
			}
		}),
	}, "")

	got, err := httpClient.FetchPost(context.Background(), "MooVNnFzc")
	if err != nil {
		t.Fatalf("FetchPost() error = %v", err)
	}
	if got.Bid != "MooVNnFzc" {
		t.Fatalf("Bid = %v, want %q", got.Bid, "MooVNnFzc")
	}
}

func TestHTTPJSONClientFetchLongTextBootstrapsHomepageSession(t *testing.T) {
	httpClient := NewHTTPJSONClient(&http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			switch {
			case req.URL.Host == "passport.weibo.com" && req.URL.Path == "/visitor/genvisitor":
				return textResponse(http.StatusOK, `window.gen_callback && gen_callback({"retcode":20000000,"msg":"succ","data":{"tid":"abc123","new_tid":true}});`, nil), nil
			case req.URL.Host == "passport.weibo.com" && req.URL.Path == "/visitor/visitor":
				return textResponse(http.StatusOK, "", map[string]string{
					"Set-Cookie": "SUB=subcookie; Path=/; Domain=.weibo.com",
				}), nil
			case req.URL.Host == "weibo.com" && req.URL.Path == "/":
				return textResponse(http.StatusOK, "<html></html>", map[string]string{
					"Set-Cookie": "XSRF-TOKEN=xsrf-token; Path=/",
				}), nil
			case req.URL.Host == "weibo.com" && req.URL.Path == "/ajax/statuses/longtext":
				if got := req.Header.Get("Referer"); got != "https://weibo.com/" {
					t.Fatalf("Referer = %q, want %q", got, "https://weibo.com/")
				}
				if got := req.Header.Get("x-xsrf-token"); got != "xsrf-token" {
					t.Fatalf("x-xsrf-token = %q, want %q", got, "xsrf-token")
				}
				return textResponse(http.StatusOK, `{"data":{"longTextContent":"<div>full text</div>"}}`, map[string]string{
					"Content-Type": "application/json",
				}), nil
			default:
				t.Fatalf("unexpected request: %s %s", req.Method, req.URL.String())
				return nil, nil
			}
		}),
	}, "")

	got, err := httpClient.FetchLongText(context.Background(), "4858157115641330")
	if err != nil {
		t.Fatalf("FetchLongText() error = %v", err)
	}
	if got != "<div>full text</div>" {
		t.Fatalf("FetchLongText() = %q, want %q", got, "<div>full text</div>")
	}
}

func textResponse(status int, body string, headers map[string]string) *http.Response {
	resp := &http.Response{
		StatusCode: status,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(body)),
	}
	for key, value := range headers {
		resp.Header.Set(key, value)
	}
	return resp
}

func TestCollectorFetchRepost(t *testing.T) {
	client := &fakeHTTPClient{
		status: weiboStatus{
			TextRaw:   "转发微博 我觉得说得对",
			CreatedAt: "Mon Jan 06 10:00:00 +0800 2025",
			User:      weiboUser{ScreenName: "reposter", IDStr: "111"},
			RetweetedStatus: &weiboStatus{
				TextRaw:   "原始观点：通胀会持续到Q3",
				CreatedAt: "Sun Jan 05 08:00:00 +0800 2025",
				User:      weiboUser{ScreenName: "original_author", IDStr: "222"},
				Mid:       "original_mid",
				Bid:       "original_bid",
			},
		},
	}
	c := New(client)
	items, err := c.Fetch(context.Background(), types.ParsedURL{
		Platform:     types.PlatformWeibo,
		PlatformID:   "test_id",
		CanonicalURL: "https://weibo.com/111/test_id",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	rc := items[0]
	if len(rc.Quotes) != 1 {
		t.Fatalf("expected 1 quote, got %d", len(rc.Quotes))
	}
	q := rc.Quotes[0]
	if q.Relation != "repost" {
		t.Errorf("relation = %q, want repost", q.Relation)
	}
	if q.AuthorName != "original_author" {
		t.Errorf("author = %q, want original_author", q.AuthorName)
	}
	if !strings.Contains(q.Content, "通胀会持续到Q3") {
		t.Errorf("quote content missing original text")
	}
	if !strings.Contains(rc.Content, "[引用#1 @original_author · 2025-01-05]") {
		t.Errorf("Content should contain quote placeholder, got:\n%s", rc.Content)
	}
	if rc.Metadata.Weibo == nil || !rc.Metadata.Weibo.IsRepost {
		t.Error("WeiboMetadata.IsRepost should be true")
	}
}

func TestCollectorFetchWithImages(t *testing.T) {
	client := &fakeHTTPClient{
		status: weiboStatus{
			TextRaw:   "看看这个图表",
			CreatedAt: "Mon Jan 06 10:00:00 +0800 2025",
			User:      weiboUser{ScreenName: "analyst", IDStr: "333"},
			PicIDs:    []string{"pic1", "pic2"},
			PicInfos: map[string]weiboPic{
				"pic1": {Large: struct {
					URL string `json:"url"`
				}{URL: "https://wx1.sinaimg.cn/large/pic1.jpg"}},
				"pic2": {Large: struct {
					URL string `json:"url"`
				}{URL: "https://wx1.sinaimg.cn/large/pic2.jpg"}},
			},
		},
	}
	c := New(client)
	items, err := c.Fetch(context.Background(), types.ParsedURL{
		Platform:     types.PlatformWeibo,
		PlatformID:   "test_id",
		CanonicalURL: "https://weibo.com/333/test_id",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	rc := items[0]
	if len(rc.Attachments) != 2 {
		t.Fatalf("expected 2 attachments, got %d", len(rc.Attachments))
	}
	if rc.Attachments[0].Type != "image" {
		t.Errorf("type = %q, want image", rc.Attachments[0].Type)
	}
	if !strings.Contains(rc.Attachments[0].URL, "pic1.jpg") {
		t.Errorf("url = %q", rc.Attachments[0].URL)
	}
	if !strings.Contains(rc.Content, "[附件#1 图片]") || !strings.Contains(rc.Content, "[附件#2 图片]") {
		t.Fatalf("Content should contain image attachment placeholders, got:\n%s", rc.Content)
	}
}

func TestCollectorFetchWithVideoAttachment(t *testing.T) {
	client := &fakeHTTPClient{
		status: weiboStatus{
			TextRaw:   "看看这个视频",
			CreatedAt: "Mon Jan 06 10:00:00 +0800 2025",
			User:      weiboUser{ScreenName: "analyst", IDStr: "333"},
			PageInfo: &weiboPageInfo{
				ObjectType: "video",
				PagePic:    "http://wx3.sinaimg.cn/orj480/poster.jpg",
				MediaInfo: weiboMediaInfo{
					PlaybackList: []weiboPlayback{{
						PlayInfo: struct {
							URL string `json:"url"`
						}{URL: "http://f.video.weibocdn.com/o0/video.mp4"},
					}},
				},
			},
		},
	}
	c := New(client)
	items, err := c.Fetch(context.Background(), types.ParsedURL{
		Platform:     types.PlatformWeibo,
		PlatformID:   "test_id",
		CanonicalURL: "https://weibo.com/333/test_id",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	rc := items[0]
	if len(rc.Attachments) != 1 {
		t.Fatalf("expected 1 attachment, got %d", len(rc.Attachments))
	}
	if rc.Attachments[0].Type != "video" {
		t.Fatalf("attachment type = %q, want video", rc.Attachments[0].Type)
	}
	if rc.Attachments[0].URL != "https://f.video.weibocdn.com/o0/video.mp4" {
		t.Fatalf("video url = %q", rc.Attachments[0].URL)
	}
	if rc.Attachments[0].PosterURL != "https://wx3.sinaimg.cn/orj480/poster.jpg" {
		t.Fatalf("poster url = %q", rc.Attachments[0].PosterURL)
	}
	if !strings.Contains(rc.Content, "[附件#1 视频]") {
		t.Fatalf("content should contain attachment placeholder, got:\n%s", rc.Content)
	}
}

func TestCollectorFetchTranscribesVideoAttachment(t *testing.T) {
	client := &fakeHTTPClient{
		status: weiboStatus{
			TextRaw:   "看看这个视频",
			CreatedAt: "Mon Jan 06 10:00:00 +0800 2025",
			User:      weiboUser{ScreenName: "analyst", IDStr: "333"},
			PageInfo: &weiboPageInfo{
				ObjectType: "video",
				MediaInfo: weiboMediaInfo{
					StreamURLHD: "https://f.video.weibocdn.com/o0/video.mp4",
				},
			},
		},
	}
	c := New(client)
	c.transcribeRemoteVideo = func(_ context.Context, mediaURL string) (audioutil.RemoteVideoResult, error) {
		if mediaURL != "https://f.video.weibocdn.com/o0/video.mp4" {
			t.Fatalf("mediaURL = %q", mediaURL)
		}
		return audioutil.RemoteVideoResult{
			Transcript:       "视频转写内容",
			TranscriptMethod: "whisper",
		}, nil
	}

	items, err := c.Fetch(context.Background(), types.ParsedURL{
		Platform:     types.PlatformWeibo,
		PlatformID:   "test_id",
		CanonicalURL: "https://weibo.com/333/test_id",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	rc := items[0]
	if got := rc.Attachments[0].Transcript; got != "视频转写内容" {
		t.Fatalf("Transcript = %q, want 视频转写内容", got)
	}
	if got := rc.Attachments[0].TranscriptMethod; got != "whisper" {
		t.Fatalf("TranscriptMethod = %q, want whisper", got)
	}
	if !strings.Contains(rc.Content, "[附件#1 视频]") {
		t.Fatalf("content missing attachment placeholder:\n%s", rc.Content)
	}
	if strings.Contains(rc.Content, "视频转写内容") {
		t.Fatalf("content should not inline transcript text:\n%s", rc.Content)
	}
}

func TestCollectorFetchNoRepost(t *testing.T) {
	client := &fakeHTTPClient{
		status: weiboStatus{
			TextRaw:   "Just a normal weibo",
			CreatedAt: "Mon Jan 06 10:00:00 +0800 2025",
			User:      weiboUser{ScreenName: "user1", IDStr: "444"},
		},
	}
	c := New(client)
	items, err := c.Fetch(context.Background(), types.ParsedURL{
		Platform:     types.PlatformWeibo,
		PlatformID:   "test_id",
		CanonicalURL: "https://weibo.com/444/test_id",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	rc := items[0]
	if len(rc.Quotes) != 0 {
		t.Errorf("expected 0 quotes, got %d", len(rc.Quotes))
	}
	if len(rc.Attachments) != 0 {
		t.Errorf("expected 0 attachments, got %d", len(rc.Attachments))
	}
}

func TestCollectorFetch_IgnoresEmptyRetweetedStatus(t *testing.T) {
	c := New(&fakeHTTPClient{
		status: weiboStatus{
			TextRaw:         "Just a normal weibo",
			CreatedAt:       "Mon Jan 06 10:00:00 +0800 2025",
			User:            weiboUser{ScreenName: "user1", IDStr: "444"},
			RetweetedStatus: &weiboStatus{},
		},
	})
	items, err := c.Fetch(context.Background(), types.ParsedURL{
		Platform:     types.PlatformWeibo,
		PlatformID:   "test_id",
		CanonicalURL: "https://weibo.com/444/test_id",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	rc := items[0]
	if len(rc.Quotes) != 0 {
		t.Fatalf("expected empty retweeted_status to be ignored, got %d quotes", len(rc.Quotes))
	}
	if rc.Metadata.Weibo != nil && rc.Metadata.Weibo.IsRepost {
		t.Fatalf("empty retweeted_status should not mark IsRepost")
	}
}
