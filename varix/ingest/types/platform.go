package types

type Platform string

const (
	PlatformTwitter  Platform = "twitter"
	PlatformWeibo    Platform = "weibo"
	PlatformYouTube  Platform = "youtube"
	PlatformBilibili Platform = "bilibili"
	PlatformWeb      Platform = "web"
	PlatformRSS      Platform = "rss"
)

type ContentType string

const (
	ContentTypePost    ContentType = "post"
	ContentTypeProfile ContentType = "profile"
	ContentTypeFeed    ContentType = "feed"
)
