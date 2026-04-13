package types

type ParsedURL struct {
	Platform     Platform    `json:"platform"`
	ContentType  ContentType `json:"content_type"`
	PlatformID   string      `json:"platform_id"`
	CanonicalURL string      `json:"canonical_url"`
}
