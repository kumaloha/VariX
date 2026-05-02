package dispatcher

import (
	"context"
	"testing"

	"github.com/kumaloha/VariX/varix/ingest/types"
)

func TestCrossReview_FetchDiscoveryItemKeepsParsedWebPlatformForGenericPlatformDomainURL(t *testing.T) {
	webSource := &capturingItemSource{platform: types.PlatformWeb}
	twitterSource := &capturingItemSource{platform: types.PlatformTwitter}

	svc := New(
		func(raw string) (types.ParsedURL, error) {
			return types.ParsedURL{
				Platform:     types.PlatformWeb,
				ContentType:  types.ContentTypePost,
				PlatformID:   "web-hash",
				CanonicalURL: raw,
			}, nil
		},
		[]ItemSource{webSource, twitterSource},
		nil,
		nil,
	)

	_, err := svc.FetchDiscoveryItem(context.Background(), types.DiscoveryItem{
		Platform:      types.PlatformTwitter,
		HydrationHint: string(types.PlatformTwitter),
		URL:           "https://x.com/i/article/1234567890",
	})
	if err != nil {
		t.Fatalf("FetchDiscoveryItem() error = %v", err)
	}
	if len(webSource.seen) != 1 {
		t.Fatalf("len(webSource.seen) = %d, want 1", len(webSource.seen))
	}
	if len(twitterSource.seen) != 0 {
		t.Fatalf("len(twitterSource.seen) = %d, want 0", len(twitterSource.seen))
	}
	if webSource.seen[0].Platform != types.PlatformWeb {
		t.Fatalf("Platform = %q, want %q", webSource.seen[0].Platform, types.PlatformWeb)
	}
	if webSource.seen[0].PlatformID != "web-hash" {
		t.Fatalf("PlatformID = %q, want %q", webSource.seen[0].PlatformID, "web-hash")
	}
}

func TestCrossReview_SearchWebDiscoveryDoesNotDowngradeRecognizedNativeURL(t *testing.T) {
	webSource := &capturingItemSource{platform: types.PlatformWeb}
	youtubeSource := &capturingItemSource{platform: types.PlatformYouTube}

	svc := New(
		func(raw string) (types.ParsedURL, error) {
			return types.ParsedURL{
				Platform:     types.PlatformYouTube,
				ContentType:  types.ContentTypePost,
				PlatformID:   "dQw4w9WgXcQ",
				CanonicalURL: raw,
			}, nil
		},
		[]ItemSource{webSource, youtubeSource},
		nil,
		nil,
	)

	_, err := svc.FetchDiscoveryItem(context.Background(), types.DiscoveryItem{
		Platform:      types.PlatformWeb,
		HydrationHint: string(types.PlatformWeb),
		URL:           "https://www.youtube.com/watch?v=dQw4w9WgXcQ",
	})
	if err != nil {
		t.Fatalf("FetchDiscoveryItem() error = %v", err)
	}
	if len(youtubeSource.seen) != 1 {
		t.Fatalf("len(youtubeSource.seen) = %d, want 1", len(youtubeSource.seen))
	}
	if len(webSource.seen) != 0 {
		t.Fatalf("len(webSource.seen) = %d, want 0", len(webSource.seen))
	}
	if youtubeSource.seen[0].Platform != types.PlatformYouTube {
		t.Fatalf("Platform = %q, want %q", youtubeSource.seen[0].Platform, types.PlatformYouTube)
	}
	if youtubeSource.seen[0].PlatformID != "dQw4w9WgXcQ" {
		t.Fatalf("PlatformID = %q, want %q", youtubeSource.seen[0].PlatformID, "dQw4w9WgXcQ")
	}
}
