package twitter

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/kumaloha/VariX/varix/ingest/sources/httputil"
	"github.com/kumaloha/VariX/varix/ingest/types"
)

type SyndicationHTTPClient struct {
	client    *http.Client
	resolve   func(ctx context.Context, raw string) (string, error)
	authToken string
	ct0       string
}

func NewSyndicationHTTPClient(client *http.Client) *SyndicationHTTPClient {
	if client == nil {
		client = http.DefaultClient
	}
	return &SyndicationHTTPClient{
		client:  client,
		resolve: newReferenceResolver(client),
	}
}

func (c *SyndicationHTTPClient) FetchByID(ctx context.Context, tweetID string) ([]types.RawContent, error) {
	item, err := c.fetchItemByID(ctx, tweetID)
	if err != nil || item == nil {
		if err != nil {
			return nil, err
		}
		return nil, nil
	}
	c.hydrateThread(ctx, item)
	return []types.RawContent{*item}, nil
}

func (c *SyndicationHTTPClient) fetchItemByID(ctx context.Context, tweetID string) (*types.RawContent, error) {
	endpoint, err := url.Parse(syndicationEndpoint)
	if err != nil {
		return nil, err
	}
	query := endpoint.Query()
	query.Set("id", tweetID)
	query.Set("lang", "en")
	query.Set("token", SyndicationToken(tweetID))
	endpoint.RawQuery = query.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0")
	req.Header.Set("Referer", "https://platform.twitter.com/")
	req.Header.Set("Origin", "https://platform.twitter.com")

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return nil, nil
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("twitter syndication fetch failed: status %d", resp.StatusCode)
	}

	if err := httputil.CheckContentLength(resp, 2<<20); err != nil {
		return nil, err
	}
	var decoded syndicationPayload
	if err := httputil.DecodeJSONLimited(resp.Body, 2<<20, &decoded); err != nil {
		return nil, err
	}
	if decoded.Tombstone != nil || decoded.NotFound {
		return nil, nil
	}

	c.hydrateLongformTexts(ctx, &decoded)

	fullArticle := ""
	if decoded.Article != nil {
		if text, err := c.fetchArticlePlainText(ctx, tweetID); err == nil && strings.TrimSpace(text) != "" {
			fullArticle = text
		} else if decoded.Article.RestID != "" {
			fullArticle, _ = c.fetchArticle(ctx, "https://x.com/i/article/"+decoded.Article.RestID)
		}
	}

	item, err := ParseSyndicationData(decoded, fullArticle)
	if err != nil {
		return nil, err
	}
	resolveReferences(ctx, c.resolve, &item)
	return &item, nil
}

func (c *SyndicationHTTPClient) fetchArticle(ctx context.Context, articleURL string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://r.jina.ai/"+articleURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "text/plain")
	req.Header.Set("X-No-Cache", "true")
	req.Header.Set("X-Timeout", "30")

	resp, err := c.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("twitter article fetch failed: status %d", resp.StatusCode)
	}
	if err := httputil.CheckContentLength(resp, 5<<20); err != nil {
		return "", err
	}
	body, err := httputil.LimitedReadAll(resp.Body, 5<<20)
	if err != nil {
		return "", err
	}
	return string(body), nil
}
