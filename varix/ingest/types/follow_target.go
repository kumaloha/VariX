package types

import "time"

type Kind string

const (
	KindNative Kind = "native"
	KindRSS    Kind = "rss"
	KindSearch Kind = "search"
)

type FollowTarget struct {
	Kind          Kind      `json:"kind"`
	Platform      string    `json:"platform,omitempty"`
	PlatformID    string    `json:"platform_id,omitempty"`
	Locator       string    `json:"locator"`
	URL           string    `json:"url,omitempty"`
	Query         string    `json:"query,omitempty"`
	HydrationHint string    `json:"hydration_hint,omitempty"`
	AuthorName    string    `json:"author_name,omitempty"`
	FollowedAt    time.Time `json:"followed_at"`
	LastPolledAt  time.Time `json:"last_polled_at,omitempty"`
}
