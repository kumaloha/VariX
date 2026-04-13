package web

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"path"
	"strings"

	"github.com/kumaloha/VariX/varix/ingest/sources/httputil"
	"github.com/kumaloha/VariX/varix/ingest/types"
)

const maxResponseBytes int64 = 10 << 20 // 10 MB

type Collector struct {
	client *http.Client
}

func New(client *http.Client) *Collector {
	if client == nil {
		client = http.DefaultClient
	}
	return &Collector{client: client}
}

func (c *Collector) Platform() types.Platform {
	return types.PlatformWeb
}

func (c *Collector) Fetch(ctx context.Context, parsed types.ParsedURL) ([]types.RawContent, error) {
	if parsed.CanonicalURL == "" {
		return nil, fmt.Errorf("missing canonical url")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, arxivToAr5iv(parsed.CanonicalURL), nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("web fetch failed: status %d", resp.StatusCode)
	}
	if err := httputil.CheckContentLength(resp, maxResponseBytes); err != nil {
		return nil, err
	}
	body, err := httputil.LimitedReadAll(resp.Body, maxResponseBytes)
	if err != nil {
		return nil, err
	}

	var item types.RawContent
	contentType := strings.ToLower(resp.Header.Get("Content-Type"))
	if isBinaryDocument(parsed.CanonicalURL, contentType) {
		return nil, fmt.Errorf("unsupported binary document: %s", parsed.CanonicalURL)
	}

	raw := string(body)
	switch {
	case strings.Contains(contentType, "text/plain"),
		strings.Contains(raw, "Markdown Content:"),
		strings.HasPrefix(strings.TrimSpace(raw), "Title:"):
		item = parseReaderMarkdown(parsed.CanonicalURL, parsed.PlatformID, raw)
	default:
		item = parseHTMLDocument(parsed.CanonicalURL, parsed.PlatformID, raw)
	}
	return []types.RawContent{item}, nil
}

func isBinaryDocument(rawURL, contentType string) bool {
	contentType = strings.ToLower(strings.TrimSpace(contentType))
	if strings.Contains(contentType, "application/pdf") {
		return true
	}
	if strings.Contains(contentType, "application/vnd.ms-excel") ||
		strings.Contains(contentType, "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet") ||
		strings.Contains(contentType, "application/vnd.openxmlformats-officedocument.wordprocessingml.document") ||
		strings.Contains(contentType, "application/msword") ||
		strings.Contains(contentType, "application/vnd.ms-powerpoint") ||
		strings.Contains(contentType, "application/vnd.openxmlformats-officedocument.presentationml.presentation") {
		return true
	}

	parsed, err := url.Parse(rawURL)
	if err != nil {
		return false
	}
	ext := strings.ToLower(path.Ext(parsed.Path))
	switch ext {
	case ".pdf", ".xls", ".xlsx", ".doc", ".docx", ".ppt", ".pptx":
		return true
	default:
		return false
	}
}
