package types

type ParsedURL struct {
	Platform     Platform    `json:"platform"`
	ContentType  ContentType `json:"content_type"`
	PlatformID   string      `json:"platform_id"`
	AuthorID     string      `json:"author_id,omitempty"`
	CanonicalURL string      `json:"canonical_url"`
}
