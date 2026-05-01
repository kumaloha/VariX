package types

import "time"

type SubscriptionStrategy string

const (
	SubscriptionStrategyRSS    SubscriptionStrategy = "rss"
	SubscriptionStrategySearch SubscriptionStrategy = "search"
)

type AuthorSubscription struct {
	ID            int64                `json:"id"`
	Platform      Platform             `json:"platform"`
	AuthorName    string               `json:"author_name,omitempty"`
	PlatformID    string               `json:"platform_id,omitempty"`
	ProfileURL    string               `json:"profile_url,omitempty"`
	Strategy      SubscriptionStrategy `json:"strategy"`
	RSSURL        string               `json:"rss_url,omitempty"`
	Status        string               `json:"status"`
	CreatedAt     time.Time            `json:"created_at"`
	UpdatedAt     time.Time            `json:"updated_at"`
	LastCheckedAt time.Time            `json:"last_checked_at,omitempty"`
}

type SubscriptionQuery struct {
	ID             int64     `json:"id"`
	SubscriptionID int64     `json:"subscription_id"`
	Provider       string    `json:"provider"`
	Query          string    `json:"query"`
	SiteFilter     string    `json:"site_filter,omitempty"`
	RecencyWindow  string    `json:"recency_window,omitempty"`
	Priority       int       `json:"priority"`
	CreatedAt      time.Time `json:"created_at"`
}

type AuthorFollowRequest struct {
	Platform   Platform `json:"platform,omitempty"`
	AuthorName string   `json:"author_name,omitempty"`
	PlatformID string   `json:"platform_id,omitempty"`
	ProfileURL string   `json:"profile_url,omitempty"`
}

type AuthorFollowResult struct {
	Subscription AuthorSubscription  `json:"subscription"`
	Follows      []FollowTarget      `json:"follows"`
	Queries      []SubscriptionQuery `json:"queries,omitempty"`
}
