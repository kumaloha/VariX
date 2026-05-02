package weibo

import (
	"context"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/kumaloha/VariX/varix/config"
	"github.com/kumaloha/VariX/varix/ingest/assemble"
	"github.com/kumaloha/VariX/varix/ingest/normalize"
	"github.com/kumaloha/VariX/varix/ingest/sources/audioutil"
	"github.com/kumaloha/VariX/varix/ingest/sources/httputil"
	"github.com/kumaloha/VariX/varix/ingest/types"
)

var bodyWeiboPost = regexp.MustCompile(`weibo\.com/\d+/(\w+)|m\.weibo\.cn/(?:status|detail)/(\w+)`)

type HTTPClient interface {
	FetchPost(ctx context.Context, postID string) (weiboStatus, error)
	FetchLongText(ctx context.Context, id string) (string, error)
	DiscoverTimeline(ctx context.Context, target types.FollowTarget) ([]types.DiscoveryItem, error)
}

type weiboUser struct {
	IDStr      string              `json:"idstr"`
	ID         httputil.FlexString `json:"id"`
	ScreenName string              `json:"screen_name"`
	Name       string              `json:"name"`
}

type weiboPic struct {
	Large struct {
		URL string `json:"url"`
	} `json:"large"`
}

type weiboPlayback struct {
	PlayInfo struct {
		URL string `json:"url"`
	} `json:"play_info"`
}

type weiboMediaInfo struct {
	StreamURL    string          `json:"stream_url"`
	StreamURLHD  string          `json:"stream_url_hd"`
	MP4HDURL     string          `json:"mp4_hd_url"`
	MP4SDURL     string          `json:"mp4_sd_url"`
	H5URL        string          `json:"h5_url"`
	MediaType    string          `json:"media_type"`
	PlaybackList []weiboPlayback `json:"playback_list"`
}

type weiboPageInfo struct {
	Type       httputil.FlexString `json:"type"`
	ObjectType string              `json:"object_type"`
	PagePic    string              `json:"page_pic"`
	PageTitle  string              `json:"page_title"`
	MediaInfo  weiboMediaInfo      `json:"media_info"`
}

type weiboStatus struct {
	IDStr           string              `json:"idstr"`
	ID              httputil.FlexString `json:"id"`
	Mid             string              `json:"mid"`
	Bid             string              `json:"bid"`
	TextRaw         string              `json:"text_raw"`
	Text            string              `json:"text"`
	IsLongText      bool                `json:"isLongText"`
	CreatedAt       string              `json:"created_at"`
	User            weiboUser           `json:"user"`
	RetweetedStatus *weiboStatus        `json:"retweeted_status,omitempty"`
	PicInfos        map[string]weiboPic `json:"pic_infos"`
	PicIDs          []string            `json:"pic_ids"`
	PageInfo        *weiboPageInfo      `json:"page_info,omitempty"`
}

type Collector struct {
	http                  HTTPClient
	transcribeRemoteVideo func(ctx context.Context, mediaURL string) (audioutil.RemoteVideoResult, error)
}

type HTTPJSONClient struct {
	client *http.Client
	cookie string
}

func New(httpClient HTTPClient) *Collector {
	return &Collector{http: httpClient}
}

func NewHTTPJSONClient(client *http.Client, cookie string) *HTTPJSONClient {
	if client == nil {
		client = http.DefaultClient
	}
	return &HTTPJSONClient{
		client: client,
		cookie: cookie,
	}
}

func NewDefault(projectRoot string, httpClient *http.Client) *Collector {
	cookie, _ := config.Get(projectRoot, "WEIBO_COOKIE")
	c := New(NewHTTPJSONClient(httpClient, cookie))
	asr := audioutil.NewClientFromConfig(projectRoot, httpClient)
	if asr != nil && httpClient != nil {
		c.transcribeRemoteVideo = func(ctx context.Context, mediaURL string) (audioutil.RemoteVideoResult, error) {
			return audioutil.TranscribeRemoteVideo(ctx, httpClient, mediaURL, asr, audioutil.RemoteVideoOptions{})
		}
	}
	return c
}

func (c *Collector) Platform() types.Platform {
	return types.PlatformWeibo
}

func (c *Collector) Kind() types.Kind {
	return types.KindNative
}

func (c *Collector) Fetch(ctx context.Context, parsed types.ParsedURL) ([]types.RawContent, error) {
	status, err := c.http.FetchPost(ctx, parsed.PlatformID)
	if err != nil {
		return nil, err
	}

	text := httputil.FirstString(status.TextRaw, status.Text)
	if status.IsLongText {
		longID := httputil.FirstString(status.Bid, status.Mid, status.IDStr, status.ID)
		if full, err := c.http.FetchLongText(ctx, longID); err == nil && strings.TrimSpace(full) != "" {
			text = stripHTML(full)
		}
	}

	postedAt := parseWeiboTime(status.CreatedAt)

	var quotes []types.Quote
	var weiboMeta *types.WeiboMetadata

	if repost := status.RetweetedStatus; repost != nil {
		repostAuthor := httputil.FirstString(repost.User.ScreenName, repost.User.Name)
		repostText := httputil.FirstString(repost.TextRaw, repost.Text)

		if repost.IsLongText {
			longID := httputil.FirstString(repost.Bid, repost.Mid, repost.IDStr, repost.ID)
			if full, err := c.http.FetchLongText(ctx, longID); err == nil && strings.TrimSpace(full) != "" {
				repostText = stripHTML(full)
			}
		}

		repostPostedAt := parseWeiboTime(repost.CreatedAt)
		repostID := httputil.FirstString(repost.Bid, repost.Mid)
		repostUID := httputil.FirstString(repost.User.IDStr, repost.User.ID)

		if repostID != "" || repostText != "" {
			quotes = append(quotes, types.Quote{
				Relation:   "repost",
				AuthorName: repostAuthor,
				AuthorID:   httputil.FirstString(repost.User.IDStr, repost.User.ID),
				Platform:   "weibo",
				ExternalID: repostID,
				URL:        "https://weibo.com/" + repostUID + "/" + repostID,
				Content:    repostText,
				PostedAt:   repostPostedAt,
			})

			weiboMeta = &types.WeiboMetadata{
				IsRepost:    true,
				OriginalURL: "https://weibo.com/" + repostUID + "/" + repostID,
			}
		}
	}

	// Parse image/video attachments, then optionally transcribe videos.
	attachments := extractWeiboAttachments(status)
	c.transcribeVideoAttachments(ctx, attachments)
	references := extractWeiboReferences(text, quotes)
	finalContent := assemble.AssembleStructuredContent(text, quotes, references, attachments)
	if strings.TrimSpace(finalContent) == "" &&
		httputil.FirstString(status.User.ScreenName, status.User.Name) == "" &&
		len(attachments) == 0 &&
		len(quotes) == 0 {
		return nil, fmt.Errorf("empty weibo status payload: %s", parsed.PlatformID)
	}

	return []types.RawContent{{
		Source:      "weibo",
		ExternalID:  parsed.PlatformID,
		Content:     finalContent,
		AuthorName:  httputil.FirstString(status.User.ScreenName, status.User.Name),
		AuthorID:    httputil.FirstString(status.User.IDStr, status.User.ID),
		URL:         parsed.CanonicalURL,
		PostedAt:    postedAt,
		Quotes:      quotes,
		References:  references,
		Attachments: attachments,
		Metadata:    types.RawMetadata{Weibo: weiboMeta},
	}}, nil
}

func (c *Collector) transcribeVideoAttachments(ctx context.Context, attachments []types.Attachment) {
	if c.transcribeRemoteVideo == nil {
		return
	}

	for i := range attachments {
		att := &attachments[i]
		if att.Type != "video" || att.URL == "" {
			continue
		}
		result, err := c.transcribeRemoteVideo(ctx, att.URL)
		if err != nil {
			att.TranscriptDiagnostics = result.TranscriptDiagnostics
			continue
		}
		att.Transcript = result.Transcript
		att.TranscriptMethod = result.TranscriptMethod
		att.TranscriptDiagnostics = result.TranscriptDiagnostics
	}
}

// extractWeiboAttachments parses pic_infos/pic_ids and page_info.media_info
// into image/video attachments.
func extractWeiboAttachments(status weiboStatus) []types.Attachment {
	attachments := make([]types.Attachment, 0, len(status.PicIDs)+1)
	for _, id := range status.PicIDs {
		info, ok := status.PicInfos[id]
		if !ok || info.Large.URL == "" {
			continue
		}
		attachments = append(attachments, types.Attachment{Type: "image", URL: info.Large.URL})
	}
	if video, ok := extractWeiboVideoAttachment(status.PageInfo); ok {
		attachments = append(attachments, video)
	}
	if len(attachments) == 0 {
		return nil
	}
	return attachments
}

func extractWeiboVideoAttachment(pageInfo *weiboPageInfo) (types.Attachment, bool) {
	if pageInfo == nil {
		return types.Attachment{}, false
	}
	if pageInfo.ObjectType != "" && !strings.EqualFold(pageInfo.ObjectType, "video") {
		return types.Attachment{}, false
	}
	url := bestWeiboVideoURL(pageInfo.MediaInfo)
	if url == "" {
		return types.Attachment{}, false
	}
	return types.Attachment{
		Type:      "video",
		URL:       normalizeWeiboMediaURL(url),
		PosterURL: normalizeWeiboMediaURL(pageInfo.PagePic),
	}, true
}

func bestWeiboVideoURL(media weiboMediaInfo) string {
	for _, playback := range media.PlaybackList {
		if strings.TrimSpace(playback.PlayInfo.URL) != "" {
			return playback.PlayInfo.URL
		}
	}
	for _, candidate := range []string{media.StreamURLHD, media.StreamURL, media.MP4HDURL, media.MP4SDURL} {
		if strings.TrimSpace(candidate) != "" {
			return candidate
		}
	}
	return ""
}

func normalizeWeiboMediaURL(raw string) string {
	raw = strings.TrimSpace(raw)
	raw = strings.TrimPrefix(raw, "http://")
	if raw == "" {
		return ""
	}
	if strings.HasPrefix(raw, "https://") {
		return raw
	}
	return "https://" + raw
}

func extractWeiboReferences(body string, quotes []types.Quote) []types.Reference {
	urls := normalize.ExtractURLs(body)
	if len(urls) == 0 {
		return nil
	}
	quotedURLs := make(map[string]struct{}, len(quotes))
	for _, quote := range quotes {
		if quote.URL != "" {
			quotedURLs[strings.TrimSpace(quote.URL)] = struct{}{}
		}
	}

	refs := make([]types.Reference, 0, len(urls))
	seen := make(map[string]struct{}, len(urls))
	for _, rawURL := range urls {
		canonical := strings.TrimSpace(rawURL)
		if canonical == "" {
			continue
		}
		if _, ok := quotedURLs[canonical]; ok {
			continue
		}
		if _, ok := seen[canonical]; ok {
			continue
		}
		seen[canonical] = struct{}{}

		ref := types.Reference{Kind: "link", URL: canonical}
		if m := bodyWeiboPost.FindStringSubmatch(canonical); len(m) > 2 {
			if id := firstNonEmptyString(m[1], m[2]); id != "" {
				ref.Kind = "post_link"
				ref.Platform = string(types.PlatformWeibo)
				ref.ExternalID = id
				ref.Label = "微博"
			}
		}
		refs = append(refs, ref)
	}
	return refs
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func (c *Collector) Discover(ctx context.Context, target types.FollowTarget) ([]types.DiscoveryItem, error) {
	return c.http.DiscoverTimeline(ctx, target)
}

const weiboMaxBytes = 2 << 20 // 2 MiB

func (c *HTTPJSONClient) FetchPost(ctx context.Context, postID string) (weiboStatus, error) {
	session, err := ensureWeiboHomepageSession(ctx, c.client, c.cookie)
	if err != nil {
		return weiboStatus{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://weibo.com/ajax/statuses/show?id="+postID, nil)
	if err != nil {
		return weiboStatus{}, err
	}
	if session.Cookie != "" {
		req.Header.Set("Cookie", session.Cookie)
	}
	req.Header.Set("Accept", "application/json, text/plain, */*")
	req.Header.Set("Referer", session.BootstrapURL)
	if session.XSRFToken != "" {
		req.Header.Set("x-xsrf-token", session.XSRFToken)
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return weiboStatus{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return weiboStatus{}, fmt.Errorf("weibo post fetch failed: status %d", resp.StatusCode)
	}

	if err := httputil.CheckContentLength(resp, weiboMaxBytes); err != nil {
		return weiboStatus{}, err
	}
	var status weiboStatus
	if err := httputil.DecodeJSONLimited(resp.Body, weiboMaxBytes, &status); err != nil {
		return weiboStatus{}, err
	}
	return status, nil
}

func (c *HTTPJSONClient) FetchLongText(ctx context.Context, id string) (string, error) {
	session, err := ensureWeiboHomepageSession(ctx, c.client, c.cookie)
	if err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://weibo.com/ajax/statuses/longtext?id="+id, nil)
	if err != nil {
		return "", err
	}
	if session.Cookie != "" {
		req.Header.Set("Cookie", session.Cookie)
	}
	req.Header.Set("Accept", "application/json, text/plain, */*")
	req.Header.Set("Referer", session.BootstrapURL)
	if session.XSRFToken != "" {
		req.Header.Set("x-xsrf-token", session.XSRFToken)
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("weibo longtext fetch failed: status %d", resp.StatusCode)
	}

	if err := httputil.CheckContentLength(resp, weiboMaxBytes); err != nil {
		return "", err
	}
	var payload struct {
		Data struct {
			LongTextContent string `json:"longTextContent"`
		} `json:"data"`
	}
	if err := httputil.DecodeJSONLimited(resp.Body, weiboMaxBytes, &payload); err != nil {
		return "", err
	}
	return payload.Data.LongTextContent, nil
}

func (c *HTTPJSONClient) DiscoverTimeline(ctx context.Context, target types.FollowTarget) ([]types.DiscoveryItem, error) {
	session, err := ensureWeiboTimelineSession(ctx, c.client, c.cookie, weiboProfileURL(target.PlatformID))
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodGet,
		"https://weibo.com/ajax/profile/getWaterFallContent?uid="+target.PlatformID,
		nil,
	)
	if err != nil {
		return nil, err
	}
	if session.Cookie != "" {
		req.Header.Set("Cookie", session.Cookie)
	}
	req.Header.Set("Accept", "application/json, text/plain, */*")
	req.Header.Set("Referer", session.BootstrapURL)
	if session.XSRFToken != "" {
		req.Header.Set("x-xsrf-token", session.XSRFToken)
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("weibo timeline fetch failed: status %d", resp.StatusCode)
	}

	if err := httputil.CheckContentLength(resp, weiboMaxBytes); err != nil {
		return nil, err
	}
	var payload struct {
		Data struct {
			List []struct {
				ID        httputil.FlexString `json:"id"`
				IDStr     string              `json:"idstr"`
				Mid       string              `json:"mid"`
				MblogID   string              `json:"mblogid"`
				Bid       string              `json:"bid"`
				CreatedAt string              `json:"created_at"`
			} `json:"list"`
		} `json:"data"`
	}
	if err := httputil.DecodeJSONLimited(resp.Body, weiboMaxBytes, &payload); err != nil {
		return nil, err
	}

	items := make([]types.DiscoveryItem, 0, len(payload.Data.List))
	for _, item := range payload.Data.List {
		externalID := httputil.FirstString(item.MblogID, item.Bid, item.IDStr, item.Mid, item.ID)
		items = append(items, types.DiscoveryItem{
			Platform:   types.PlatformWeibo,
			ExternalID: externalID,
			URL:        "https://weibo.com/" + target.PlatformID + "/" + externalID,
			AuthorName: target.AuthorName,
			PostedAt:   parseWeiboTime(item.CreatedAt),
		})
	}
	return items, nil
}

func weiboProfileURL(uid string) string {
	return "https://weibo.com/u/" + uid
}

func parseWeiboTime(raw string) time.Time {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return time.Time{}
	}
	layouts := []string{
		"Mon Jan 02 15:04:05 -0700 2006",
		time.RFC3339,
		time.RFC1123,
		time.RFC1123Z,
	}
	for _, layout := range layouts {
		if parsed, err := time.Parse(layout, raw); err == nil {
			return parsed.UTC()
		}
	}
	return time.Time{}
}
