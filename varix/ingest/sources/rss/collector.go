package rss

import (
	"context"
	"encoding/xml"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/kumaloha/VariX/varix/ingest/internal/textutil"
	"github.com/kumaloha/VariX/varix/ingest/sources/httputil"
	"github.com/kumaloha/VariX/varix/ingest/types"
)

const maxResponseBytes int64 = 5 << 20 // 5 MB

type Collector struct {
	client *http.Client
}

func New(client *http.Client) *Collector {
	if client == nil {
		client = http.DefaultClient
	}
	return &Collector{client: client}
}

func (c *Collector) Kind() types.Kind {
	return types.KindRSS
}

func (c *Collector) Platform() types.Platform {
	return types.PlatformRSS
}

func (c *Collector) Discover(ctx context.Context, target types.FollowTarget) ([]types.DiscoveryItem, error) {
	feedURL := strings.TrimSpace(target.URL)
	if feedURL == "" {
		feedURL = strings.TrimSpace(target.Locator)
	}
	if feedURL == "" {
		return nil, fmt.Errorf("missing feed url")
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, feedURL, nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("rss fetch failed: status %d", resp.StatusCode)
	}

	if err := httputil.CheckContentLength(resp, maxResponseBytes); err != nil {
		return nil, err
	}
	body, err := httputil.LimitedReadAll(resp.Body, maxResponseBytes)
	if err != nil {
		return nil, err
	}

	var doc feedDocument
	if err := xml.Unmarshal(body, &doc); err != nil {
		return nil, err
	}

	items := make([]types.DiscoveryItem, 0, len(doc.Channel.Items)+len(doc.Entries))
	for _, item := range doc.Channel.Items {
		link := strings.TrimSpace(item.Link)
		if link == "" {
			continue
		}
		items = append(items, types.DiscoveryItem{
			Platform:   types.PlatformRSS,
			ExternalID: textutil.FirstNonEmpty(item.GUID, link),
			URL:        link,
			AuthorName: target.AuthorName,
			PostedAt:   parseTime(item.PubDate),
			Metadata: types.DiscoveryMetadata{
				RSS: &types.RSSDiscoveryMetadata{
					Title: strings.TrimSpace(item.Title),
					Feed:  feedURL,
				},
			},
		})
	}
	for _, entry := range doc.Entries {
		link := entry.link()
		if link == "" {
			continue
		}
		items = append(items, types.DiscoveryItem{
			Platform:   types.PlatformRSS,
			ExternalID: textutil.FirstNonEmpty(entry.ID, link),
			URL:        link,
			AuthorName: target.AuthorName,
			PostedAt:   parseTime(textutil.FirstNonEmpty(entry.Published, entry.Updated)),
			Metadata: types.DiscoveryMetadata{
				RSS: &types.RSSDiscoveryMetadata{
					Title: strings.TrimSpace(entry.Title),
					Feed:  feedURL,
				},
			},
		})
	}
	return items, nil
}

type feedDocument struct {
	Channel rssChannel  `xml:"channel"`
	Entries []atomEntry `xml:"entry"`
}

type rssChannel struct {
	Items []rssItem `xml:"item"`
}

type rssItem struct {
	Title   string `xml:"title"`
	Link    string `xml:"link"`
	GUID    string `xml:"guid"`
	PubDate string `xml:"pubDate"`
}

type atomEntry struct {
	Title     string     `xml:"title"`
	ID        string     `xml:"id"`
	Updated   string     `xml:"updated"`
	Published string     `xml:"published"`
	Links     []atomLink `xml:"link"`
}

type atomLink struct {
	Href string `xml:"href,attr"`
	Rel  string `xml:"rel,attr"`
}

func (e atomEntry) link() string {
	for _, link := range e.Links {
		if link.Href == "" {
			continue
		}
		if link.Rel == "" || link.Rel == "alternate" {
			return strings.TrimSpace(link.Href)
		}
	}
	return ""
}

func parseTime(raw string) time.Time {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return time.Time{}
	}
	layouts := []string{
		time.RFC3339,
		time.RFC3339Nano,
		time.RFC1123Z,
		time.RFC1123,
		time.RFC822Z,
		time.RFC822,
		time.RubyDate,
	}
	for _, layout := range layouts {
		if parsed, err := time.Parse(layout, raw); err == nil {
			return parsed.UTC()
		}
	}
	return time.Time{}
}
