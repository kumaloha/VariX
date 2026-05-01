package router

import (
	"testing"

	"github.com/kumaloha/VariX/varix/ingest/types"
)

func TestParse_TwitterPost(t *testing.T) {
	got, err := Parse("https://x.com/elonmusk/status/1234567890")
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if got.Platform != types.PlatformTwitter {
		t.Fatalf("Platform = %q, want %q", got.Platform, types.PlatformTwitter)
	}
	if got.ContentType != types.ContentTypePost {
		t.Fatalf("ContentType = %q, want %q", got.ContentType, types.ContentTypePost)
	}
	if got.PlatformID != "1234567890" {
		t.Fatalf("PlatformID = %q, want %q", got.PlatformID, "1234567890")
	}
}

func TestParse_TwitterPostCapturesAuthorHandle(t *testing.T) {
	got, err := Parse("https://x.com/robin_j_brooks/status/2049570595277300120?s=20")
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if got.Platform != types.PlatformTwitter || got.ContentType != types.ContentTypePost {
		t.Fatalf("Parse() = %#v, want twitter post", got)
	}
	if got.PlatformID != "2049570595277300120" {
		t.Fatalf("PlatformID = %q, want status id", got.PlatformID)
	}
	if got.AuthorID != "robin_j_brooks" {
		t.Fatalf("AuthorID = %q, want robin_j_brooks", got.AuthorID)
	}
}

func TestParse_TwitterProfile(t *testing.T) {
	got, err := Parse("twitter.com/elonmusk")
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if got.Platform != types.PlatformTwitter {
		t.Fatalf("Platform = %q, want %q", got.Platform, types.PlatformTwitter)
	}
	if got.ContentType != types.ContentTypeProfile {
		t.Fatalf("ContentType = %q, want %q", got.ContentType, types.ContentTypeProfile)
	}
	if got.PlatformID != "elonmusk" {
		t.Fatalf("PlatformID = %q, want %q", got.PlatformID, "elonmusk")
	}
}

func TestParse_WeiboPost(t *testing.T) {
	got, err := Parse("https://weibo.com/1234567/AbCdEfG")
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if got.Platform != types.PlatformWeibo {
		t.Fatalf("Platform = %q, want %q", got.Platform, types.PlatformWeibo)
	}
	if got.ContentType != types.ContentTypePost {
		t.Fatalf("ContentType = %q, want %q", got.ContentType, types.ContentTypePost)
	}
}

func TestParse_YoutubeVideo(t *testing.T) {
	got, err := Parse("https://www.youtube.com/watch?v=dQw4w9WgXcQ")
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if got.Platform != types.PlatformYouTube {
		t.Fatalf("Platform = %q, want %q", got.Platform, types.PlatformYouTube)
	}
	if got.PlatformID != "dQw4w9WgXcQ" {
		t.Fatalf("PlatformID = %q, want %q", got.PlatformID, "dQw4w9WgXcQ")
	}
}

func TestParse_YoutubeProfiles(t *testing.T) {
	tests := []struct {
		raw string
		id  string
	}{
		{raw: "https://www.youtube.com/channel/UCabc123_XYZ", id: "UCabc123_XYZ"},
		{raw: "https://www.youtube.com/@Acme.Channel", id: "Acme.Channel"},
	}
	for _, tt := range tests {
		got, err := Parse(tt.raw)
		if err != nil {
			t.Fatalf("Parse(%q) error = %v", tt.raw, err)
		}
		if got.Platform != types.PlatformYouTube || got.ContentType != types.ContentTypeProfile {
			t.Fatalf("Parse(%q) = %#v, want youtube profile", tt.raw, got)
		}
		if got.PlatformID != tt.id {
			t.Fatalf("PlatformID = %q, want %q", got.PlatformID, tt.id)
		}
	}
}

func TestParse_BilibiliVideo(t *testing.T) {
	got, err := Parse("https://www.bilibili.com/video/BV1234567890")
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if got.Platform != types.PlatformBilibili {
		t.Fatalf("Platform = %q, want %q", got.Platform, types.PlatformBilibili)
	}
	if got.PlatformID != "BV1234567890" {
		t.Fatalf("PlatformID = %q, want %q", got.PlatformID, "BV1234567890")
	}
}

func TestParse_BilibiliProfile(t *testing.T) {
	got, err := Parse("https://space.bilibili.com/123456")
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if got.Platform != types.PlatformBilibili || got.ContentType != types.ContentTypeProfile {
		t.Fatalf("Parse() = %#v, want bilibili profile", got)
	}
	if got.PlatformID != "123456" {
		t.Fatalf("PlatformID = %q, want 123456", got.PlatformID)
	}
}

func TestParse_RSSFeed(t *testing.T) {
	got, err := Parse("https://example.com/feed.xml")
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if got.Platform != types.PlatformRSS {
		t.Fatalf("Platform = %q, want %q", got.Platform, types.PlatformRSS)
	}
	if got.ContentType != types.ContentTypeFeed {
		t.Fatalf("ContentType = %q, want %q", got.ContentType, types.ContentTypeFeed)
	}
	if got.PlatformID == "" {
		t.Fatal("PlatformID is empty")
	}
}

func TestParse_GenericWebFallback(t *testing.T) {
	got, err := Parse("https://www.cnbc.com/2026/04/07/market-news.html")
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if got.Platform != types.PlatformWeb {
		t.Fatalf("Platform = %q, want %q", got.Platform, types.PlatformWeb)
	}
	if got.ContentType != types.ContentTypePost {
		t.Fatalf("ContentType = %q, want %q", got.ContentType, types.ContentTypePost)
	}
	if got.PlatformID == "" {
		t.Fatal("PlatformID is empty")
	}
}

func TestParse_DoesNotMatchLookalikeDomains(t *testing.T) {
	tests := []string{
		"https://nottwitter.com/elonmusk/status/1234567890",
		"https://x.com.evil.tld/elonmusk/status/1234567890",
		"https://bilibili.com.evil.tld/video/BV1234567890",
		"https://youtube.com.evil.tld/watch?v=dQw4w9WgXcQ",
	}

	for _, raw := range tests {
		got, err := Parse(raw)
		if err != nil {
			t.Fatalf("Parse(%q) error = %v", raw, err)
		}
		if got.Platform != types.PlatformWeb {
			t.Fatalf("Parse(%q) platform = %q, want %q", raw, got.Platform, types.PlatformWeb)
		}
	}
}

func TestParse_DoesNotTreatReservedTwitterPathsAsProfiles(t *testing.T) {
	tests := []string{
		"https://x.com/home",
		"https://twitter.com/search",
		"https://x.com/explore",
	}

	for _, raw := range tests {
		got, err := Parse(raw)
		if err != nil {
			t.Fatalf("Parse(%q) error = %v", raw, err)
		}
		if got.Platform != types.PlatformWeb {
			t.Fatalf("Parse(%q) platform = %q, want %q", raw, got.Platform, types.PlatformWeb)
		}
	}
}

func TestParse_DoesNotTreatArticlePathsContainingFeedAsRSS(t *testing.T) {
	got, err := Parse("https://example.com/features/feed-the-world.html")
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if got.Platform != types.PlatformWeb {
		t.Fatalf("Platform = %q, want %q", got.Platform, types.PlatformWeb)
	}
	if got.ContentType != types.ContentTypePost {
		t.Fatalf("ContentType = %q, want %q", got.ContentType, types.ContentTypePost)
	}
}

func TestParse_GenericWebCanonicalizesTrackingParams(t *testing.T) {
	first, err := Parse("https://example.com/post?id=1&utm_source=twitter&utm_medium=social#top")
	if err != nil {
		t.Fatalf("Parse(first) error = %v", err)
	}
	second, err := Parse("https://EXAMPLE.com/post?id=1")
	if err != nil {
		t.Fatalf("Parse(second) error = %v", err)
	}

	if first.Platform != types.PlatformWeb || second.Platform != types.PlatformWeb {
		t.Fatalf("platforms = %q/%q, want both %q", first.Platform, second.Platform, types.PlatformWeb)
	}
	if first.PlatformID != second.PlatformID {
		t.Fatalf("PlatformID = %q/%q, want equal", first.PlatformID, second.PlatformID)
	}
	if first.CanonicalURL != "https://example.com/post?id=1" {
		t.Fatalf("first CanonicalURL = %q, want %q", first.CanonicalURL, "https://example.com/post?id=1")
	}
	if second.CanonicalURL != "https://example.com/post?id=1" {
		t.Fatalf("second CanonicalURL = %q, want %q", second.CanonicalURL, "https://example.com/post?id=1")
	}
}

func TestParse_RecognizesCommonRSSURLs(t *testing.T) {
	tests := []string{
		"https://hnrss.org/newest?q=golang",
		"https://www.reddit.com/r/golang/.rss",
		"https://example.com/rss/latest",
	}

	for _, raw := range tests {
		got, err := Parse(raw)
		if err != nil {
			t.Fatalf("Parse(%q) error = %v", raw, err)
		}
		if got.Platform != types.PlatformRSS {
			t.Fatalf("Parse(%q) platform = %q, want %q", raw, got.Platform, types.PlatformRSS)
		}
		if got.ContentType != types.ContentTypeFeed {
			t.Fatalf("Parse(%q) content type = %q, want %q", raw, got.ContentType, types.ContentTypeFeed)
		}
	}
}

func TestParse_TwitterProfileWithQueryStillParsesAsProfile(t *testing.T) {
	tests := []string{
		"https://x.com/elonmusk?lang=en",
		"https://twitter.com/elonmusk?ref_src=twsrc%5Egoogle%7Ctwcamp%5Eserp%7Ctwgr%5Eauthor",
	}

	for _, raw := range tests {
		got, err := Parse(raw)
		if err != nil {
			t.Fatalf("Parse(%q) error = %v", raw, err)
		}
		if got.Platform != types.PlatformTwitter {
			t.Fatalf("Parse(%q) platform = %q, want %q", raw, got.Platform, types.PlatformTwitter)
		}
		if got.ContentType != types.ContentTypeProfile {
			t.Fatalf("Parse(%q) content type = %q, want %q", raw, got.ContentType, types.ContentTypeProfile)
		}
		if got.PlatformID != "elonmusk" {
			t.Fatalf("Parse(%q) platform id = %q, want %q", raw, got.PlatformID, "elonmusk")
		}
	}
}

func TestParse_DoesNotTreatAdditionalReservedTwitterPathsAsProfiles(t *testing.T) {
	tests := []string{
		"https://x.com/about",
		"https://x.com/tos",
		"https://twitter.com/privacy",
	}

	for _, raw := range tests {
		got, err := Parse(raw)
		if err != nil {
			t.Fatalf("Parse(%q) error = %v", raw, err)
		}
		if got.Platform != types.PlatformWeb {
			t.Fatalf("Parse(%q) platform = %q, want %q", raw, got.Platform, types.PlatformWeb)
		}
	}
}

func TestParse_DoesNotTreatWeiboScreenNameAsNativeProfile(t *testing.T) {
	got, err := Parse("https://weibo.com/alice")
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if got.Platform != types.PlatformWeb {
		t.Fatalf("Platform = %q, want %q", got.Platform, types.PlatformWeb)
	}
	if got.ContentType != types.ContentTypePost {
		t.Fatalf("ContentType = %q, want %q", got.ContentType, types.ContentTypePost)
	}
}

func TestParse_MatchesOfficialHostsCaseInsensitively(t *testing.T) {
	tests := []struct {
		raw      string
		platform types.Platform
	}{
		{raw: "https://X.com/elonmusk/status/1234567890", platform: types.PlatformTwitter},
		{raw: "https://YouTube.com/watch?v=dQw4w9WgXcQ", platform: types.PlatformYouTube},
		{raw: "https://Weibo.com/1234567/AbCdEfG", platform: types.PlatformWeibo},
	}

	for _, tt := range tests {
		got, err := Parse(tt.raw)
		if err != nil {
			t.Fatalf("Parse(%q) error = %v", tt.raw, err)
		}
		if got.Platform != tt.platform {
			t.Fatalf("Parse(%q) platform = %q, want %q", tt.raw, got.Platform, tt.platform)
		}
	}
}
