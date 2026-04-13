package types

import "time"

type DiscoveryItem struct {
	Platform      Platform          `json:"platform"`
	ExternalID    string            `json:"external_id"`
	URL           string            `json:"url"`
	AuthorName    string            `json:"author_name,omitempty"`
	HydrationHint string            `json:"hydration_hint,omitempty"`
	PostedAt      time.Time         `json:"posted_at,omitempty"`
	ThreadIDs     []string          `json:"thread_ids,omitempty"`
	Metadata      DiscoveryMetadata `json:"metadata,omitempty"`
}

type DiscoveryMetadata struct {
	RSS *RSSDiscoveryMetadata `json:"rss,omitempty"`
}

type RSSDiscoveryMetadata struct {
	Title string `json:"title,omitempty"`
	Feed  string `json:"feed,omitempty"`
}
