package weibo

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"

	"github.com/kumaloha/VariX/varix/ingest/sources/httputil"
)

type pageSession struct {
	Cookie     string
	XSRFToken  string
	BootstrapURL string
}

type visitorPayload struct {
	TID        string
	Confidence int
	NewTID     bool
}

var (
	genCallback = regexp.MustCompile(`gen_callback\((.*)\)`)
	htmlTag     = regexp.MustCompile(`<[^>]+>`)
)

func parseGenVisitorJSONP(raw string) (visitorPayload, error) {
	match := genCallback.FindStringSubmatch(strings.TrimSpace(raw))
	if len(match) < 2 {
		return visitorPayload{}, fmt.Errorf("unexpected jsonp format")
	}

	var decoded struct {
		Data struct {
			TID        string `json:"tid"`
			Confidence int    `json:"confidence"`
			NewTID     bool   `json:"new_tid"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(match[1]), &decoded); err != nil {
		return visitorPayload{}, err
	}
	return visitorPayload{
		TID:        decoded.Data.TID,
		Confidence: decoded.Data.Confidence,
		NewTID:     decoded.Data.NewTID,
	}, nil
}

func stripHTML(raw string) string {
	raw = strings.ReplaceAll(raw, "<br/>", " ")
	raw = strings.ReplaceAll(raw, "<br>", " ")
	raw = strings.ReplaceAll(raw, "<br />", " ")
	noTags := htmlTag.ReplaceAllString(raw, " ")
	return strings.Join(strings.Fields(noTags), " ")
}

func ensureWeiboCookie(ctx context.Context, client *http.Client, cookie string) (string, error) {
	cookie = strings.TrimSpace(cookie)
	if cookie != "" {
		return cookie, nil
	}
	if client == nil {
		client = http.DefaultClient
	}

	form := url.Values{
		"cb": {"gen_callback"},
		"fp": {`{"os":"1","browser":"Chrome120,0,0,0","screenInfo":"1920*1080*24"}`},
	}
	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		"https://passport.weibo.com/visitor/genvisitor",
		strings.NewReader(form.Encode()),
	)
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if err := httputil.CheckContentLength(resp, 256*1024); err != nil {
		return "", err
	}
	body, err := httputil.LimitedReadAll(resp.Body, 256*1024)
	if err != nil {
		return "", err
	}

	payload, err := parseGenVisitorJSONP(string(body))
	if err != nil {
		return "", err
	}
	if payload.TID == "" {
		return "", nil
	}

	activateURL := "https://passport.weibo.com/visitor/visitor?a=incarnate&t=" + url.QueryEscape(payload.TID)
	activateReq, err := http.NewRequestWithContext(ctx, http.MethodGet, activateURL, nil)
	if err != nil {
		return "", err
	}
	activateResp, err := client.Do(activateReq)
	if err != nil {
		return "", err
	}
	defer activateResp.Body.Close()

	cookies := activateResp.Cookies()
	if len(cookies) == 0 {
		return "", nil
	}

	parts := make([]string, 0, len(cookies))
	for _, item := range cookies {
		parts = append(parts, item.Name+"="+item.Value)
	}
	return strings.Join(parts, "; "), nil
}

func ensureWeiboPageSession(ctx context.Context, client *http.Client, cookie string, bootstrapURL string) (pageSession, error) {
	cookie, err := ensureWeiboCookie(ctx, client, cookie)
	if err != nil {
		return pageSession{}, err
	}
	if client == nil {
		client = http.DefaultClient
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, bootstrapURL, nil)
	if err != nil {
		return pageSession{}, err
	}
	if cookie != "" {
		req.Header.Set("Cookie", cookie)
	}

	resp, err := client.Do(req)
	if err != nil {
		return pageSession{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return pageSession{}, fmt.Errorf("weibo page bootstrap failed: status %d", resp.StatusCode)
	}

	session := pageSession{
		Cookie:       mergeCookieHeader(cookie, resp.Cookies()),
		BootstrapURL: bootstrapURL,
	}
	for _, item := range resp.Cookies() {
		if item.Name == "XSRF-TOKEN" {
			session.XSRFToken = item.Value
			break
		}
	}
	return session, nil
}

func ensureWeiboTimelineSession(ctx context.Context, client *http.Client, cookie string, profileURL string) (pageSession, error) {
	return ensureWeiboPageSession(ctx, client, cookie, profileURL)
}

func ensureWeiboHomepageSession(ctx context.Context, client *http.Client, cookie string) (pageSession, error) {
	return ensureWeiboPageSession(ctx, client, cookie, "https://weibo.com/")
}

func mergeCookieHeader(base string, cookies []*http.Cookie) string {
	values := map[string]string{}
	order := make([]string, 0, len(cookies))

	for _, part := range strings.Split(base, ";") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		pieces := strings.SplitN(part, "=", 2)
		if len(pieces) != 2 {
			continue
		}
		name := strings.TrimSpace(pieces[0])
		if _, ok := values[name]; !ok {
			order = append(order, name)
		}
		values[name] = strings.TrimSpace(pieces[1])
	}

	for _, item := range cookies {
		if item == nil || strings.TrimSpace(item.Name) == "" {
			continue
		}
		if _, ok := values[item.Name]; !ok {
			order = append(order, item.Name)
		}
		values[item.Name] = item.Value
	}

	parts := make([]string, 0, len(order))
	for _, name := range order {
		parts = append(parts, name+"="+values[name])
	}
	return strings.Join(parts, "; ")
}
