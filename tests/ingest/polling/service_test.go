package polling

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/kumaloha/VariX/varix/ingest/types"
	"github.com/kumaloha/VariX/varix/storage/contentstore"
)

type fakeStore struct {
	processed   map[string]bool
	follows     []types.FollowTarget
	warnings    []contentstore.ScanWarning
	polledKeys  []string
	removedKeys []string
	markedKeys  []string
	reports     []types.PollReport
	raws        []types.RawContent
	authors     []types.AuthorSubscription
	queries     []types.SubscriptionQuery
	ops         []string
}

type fakeEnricher struct{}

func (fakeEnricher) Annotate(items []types.RawContent) []types.RawContent {
	out := make([]types.RawContent, 0, len(items))
	for _, item := range items {
		item.Provenance = &types.Provenance{
			BaseRelation:      types.BaseRelationUnknown,
			EditorialLayer:    types.EditorialLayerUnknown,
			Confidence:        types.ConfidenceLow,
			NeedsSourceLookup: true,
			SourceLookup: types.SourceLookupState{
				Status: types.SourceLookupStatusPending,
			},
		}
		out = append(out, item)
	}
	return out
}

func (s *fakeStore) IsProcessed(_ context.Context, platform, externalID string) (bool, error) {
	return s.processed[platform+":"+externalID], nil
}

func (s *fakeStore) MarkProcessed(_ context.Context, record types.ProcessedRecord) error {
	key := record.Platform + ":" + record.ExternalID
	s.processed[key] = true
	s.markedKeys = append(s.markedKeys, key)
	s.ops = append(s.ops, "mark:"+key)
	return nil
}

func (s *fakeStore) UpsertRawCapture(_ context.Context, raw types.RawContent) error {
	s.raws = append(s.raws, raw)
	s.ops = append(s.ops, "raw:"+raw.Source+":"+raw.ExternalID)
	return nil
}

func (s *fakeStore) GetRawCapture(_ context.Context, platform, externalID string) (types.RawContent, error) {
	for _, raw := range s.raws {
		if raw.Source == platform && raw.ExternalID == externalID {
			return raw, nil
		}
	}
	return types.RawContent{}, errors.New("raw capture not found")
}

func (s *fakeStore) ListPendingSourceLookups(_ context.Context, _ int) ([]types.RawContent, error) {
	return nil, nil
}

func (s *fakeStore) MarkSourceLookupResult(_ context.Context, raw types.RawContent, _ types.SourceLookupStatus, _ string) error {
	s.raws = append(s.raws, raw)
	return nil
}

func (s *fakeStore) RegisterFollow(_ context.Context, target types.FollowTarget) error {
	s.follows = append(s.follows, target)
	return nil
}

func (s *fakeStore) ListFollows(_ context.Context) ([]types.FollowTarget, []contentstore.ScanWarning, error) {
	return s.follows, s.warnings, nil
}

func (s *fakeStore) RemoveFollow(_ context.Context, kind types.Kind, platform string, locator string) error {
	s.removedKeys = append(s.removedKeys, string(kind)+":"+platform+":"+locator)
	return nil
}

func (s *fakeStore) UpdateFollowPolled(_ context.Context, kind types.Kind, platform string, locator string, _ time.Time) error {
	s.polledKeys = append(s.polledKeys, string(kind)+":"+platform+":"+locator)
	return nil
}

func (s *fakeStore) RecordPollReport(_ context.Context, report types.PollReport) error {
	s.reports = append(s.reports, report)
	return nil
}

func (s *fakeStore) RegisterAuthorSubscription(_ context.Context, sub types.AuthorSubscription, queries []types.SubscriptionQuery) (types.AuthorSubscription, error) {
	if sub.ID == 0 {
		sub.ID = int64(len(s.authors) + 1)
	}
	s.authors = append(s.authors, sub)
	s.queries = append(s.queries, queries...)
	return sub, nil
}

func (s *fakeStore) ListAuthorSubscriptions(_ context.Context) ([]types.AuthorSubscription, []contentstore.ScanWarning, error) {
	return s.authors, nil, nil
}

type replaceStore struct {
	fakeStore
}

func (s *replaceStore) UpsertRawCapture(_ context.Context, raw types.RawContent) error {
	for i, existing := range s.raws {
		if existing.Source == raw.Source && existing.ExternalID == raw.ExternalID {
			s.raws[i] = raw
			s.ops = append(s.ops, "raw:"+raw.Source+":"+raw.ExternalID)
			return nil
		}
	}
	s.raws = append(s.raws, raw)
	s.ops = append(s.ops, "raw:"+raw.Source+":"+raw.ExternalID)
	return nil
}

type candidateEnricher struct{}

func (candidateEnricher) Annotate(items []types.RawContent) []types.RawContent {
	out := make([]types.RawContent, 0, len(items))
	for _, item := range items {
		item.Provenance = &types.Provenance{
			Confidence:        types.ConfidenceMedium,
			NeedsSourceLookup: true,
			SourceCandidates: []types.SourceCandidate{{
				URL:        "https://example.com/source",
				Kind:       "source_link",
				Confidence: string(types.ConfidenceHigh),
			}},
			SourceLookup: types.SourceLookupState{Status: types.SourceLookupStatusPending},
		}
		out = append(out, item)
	}
	return out
}

type fakeDispatcher struct {
	discovered     []types.DiscoveryItem
	discoverErrFor map[string]error
}

type fetchErrorDispatcher struct {
	fakeDispatcher
	err error
}

func (f fetchErrorDispatcher) FetchByParsedURL(_ context.Context, _ types.ParsedURL) ([]types.RawContent, error) {
	return nil, f.err
}

func (fakeDispatcher) SupportsFollow(kind types.Kind, platform types.Platform) bool {
	switch kind {
	case types.KindRSS:
		return platform == types.PlatformRSS
	case types.KindSearch:
		return platform == types.PlatformTwitter || platform == types.PlatformWeibo || platform == types.PlatformYouTube || platform == types.PlatformBilibili || platform == types.PlatformWeb
	case types.KindNative:
		return platform == types.PlatformWeibo
	default:
		return false
	}
}

func (f fakeDispatcher) DiscoverFollowedTarget(_ context.Context, target types.FollowTarget) ([]types.DiscoveryItem, error) {
	if err := f.discoverErrFor[string(target.Kind)+":"+target.Platform+":"+target.Locator]; err != nil {
		return nil, err
	}
	if len(f.discovered) > 0 {
		return f.discovered, nil
	}
	return []types.DiscoveryItem{{
		Platform:   types.PlatformTwitter,
		ExternalID: "123",
		URL:        "https://x.com/a/status/123",
	}}, nil
}

func (fakeDispatcher) ParseURL(_ context.Context, rawURL string) (types.ParsedURL, error) {
	switch rawURL {
	case "https://weibo.com/u/123456":
		return types.ParsedURL{
			Platform:     types.PlatformWeibo,
			ContentType:  types.ContentTypeProfile,
			PlatformID:   "123456",
			CanonicalURL: rawURL,
		}, nil
	case "https://twitter.com/elonmusk", "https://twitter.com/ElonMusk":
		return types.ParsedURL{
			Platform:     types.PlatformTwitter,
			ContentType:  types.ContentTypeProfile,
			PlatformID:   "ElonMusk",
			CanonicalURL: rawURL,
		}, nil
	case "https://feeds.example.test/feed.xml":
		return types.ParsedURL{
			Platform:     types.PlatformRSS,
			ContentType:  types.ContentTypeFeed,
			PlatformID:   "feed-1",
			CanonicalURL: rawURL,
		}, nil
	case "https://x.com/a/status/123":
		return types.ParsedURL{
			Platform:     types.PlatformTwitter,
			ContentType:  types.ContentTypePost,
			PlatformID:   "123",
			AuthorID:     "a",
			CanonicalURL: rawURL,
		}, nil
	case "https://x.com/robin_j_brooks/status/2049570595277300120?s=20":
		return types.ParsedURL{
			Platform:     types.PlatformTwitter,
			ContentType:  types.ContentTypePost,
			PlatformID:   "2049570595277300120",
			AuthorID:     "robin_j_brooks",
			CanonicalURL: rawURL,
		}, nil
	default:
		return types.ParsedURL{
			Platform:     types.PlatformWeb,
			ContentType:  types.ContentTypePost,
			PlatformID:   "web-1",
			CanonicalURL: rawURL,
		}, nil
	}
}

func (fakeDispatcher) FetchByParsedURL(_ context.Context, parsed types.ParsedURL) ([]types.RawContent, error) {
	return []types.RawContent{{
		Source:     string(parsed.Platform),
		ExternalID: parsed.PlatformID,
		URL:        parsed.CanonicalURL,
		PostedAt:   time.Now().UTC(),
	}}, nil
}

func (fakeDispatcher) FetchDiscoveryItem(_ context.Context, item types.DiscoveryItem) ([]types.RawContent, error) {
	platform := item.Platform
	if platform == "" {
		platform = types.PlatformTwitter
	}
	externalID := item.ExternalID
	if externalID == "" {
		externalID = "123"
	}
	return []types.RawContent{{
		Source:     string(platform),
		ExternalID: externalID,
		URL:        item.URL,
		PostedAt:   time.Now().UTC(),
	}}, nil
}

func TestService_FollowURLRegistersNativeTarget(t *testing.T) {
	store := &fakeStore{processed: map[string]bool{}}
	svc := New(store, fakeDispatcher{}, candidateEnricher{})

	target, err := svc.FollowURL(context.Background(), "https://weibo.com/u/123456")
	if err != nil {
		t.Fatalf("FollowURL() error = %v", err)
	}
	if len(store.follows) != 1 {
		t.Fatalf("len(follows) = %d, want 1", len(store.follows))
	}
	if target.Kind != types.KindNative {
		t.Fatalf("Kind = %q, want %q", target.Kind, types.KindNative)
	}
	if target.Platform != "weibo" || target.PlatformID != "123456" {
		t.Fatalf("FollowURL() = %#v, want weibo/123456 target", target)
	}
	if target.Locator != "https://weibo.com/u/123456" {
		t.Fatalf("Locator = %q, want canonical profile URL", target.Locator)
	}
}

type transcriptPreservingDispatcher struct{}

func (transcriptPreservingDispatcher) SupportsFollow(kind types.Kind, platform types.Platform) bool {
	return kind == types.KindNative && platform == types.PlatformYouTube
}
func (transcriptPreservingDispatcher) DiscoverFollowedTarget(context.Context, types.FollowTarget) ([]types.DiscoveryItem, error) {
	return []types.DiscoveryItem{{
		Platform:   types.PlatformYouTube,
		ExternalID: "yt-1",
		URL:        "https://www.youtube.com/watch?v=yt-1",
	}}, nil
}
func (transcriptPreservingDispatcher) ParseURL(_ context.Context, rawURL string) (types.ParsedURL, error) {
	return types.ParsedURL{Platform: types.PlatformYouTube, ContentType: types.ContentTypePost, PlatformID: "yt-1", CanonicalURL: rawURL}, nil
}
func (transcriptPreservingDispatcher) FetchByParsedURL(_ context.Context, parsed types.ParsedURL) ([]types.RawContent, error) {
	return []types.RawContent{{
		Source:     "youtube",
		ExternalID: parsed.PlatformID,
		URL:        parsed.CanonicalURL,
		Content:    "# title\n\n（无法获取视频内容，以下为视频简介）\n\nnew fallback",
		Metadata: types.RawMetadata{YouTube: &types.YouTubeMetadata{
			Title:                 "title",
			TranscriptMethod:      "title_only",
			TranscriptDiagnostics: []types.TranscriptDiagnostic{{Stage: "audio", Code: "rate_limited"}},
		}},
	}}, nil
}
func (transcriptPreservingDispatcher) FetchDiscoveryItem(context.Context, types.DiscoveryItem) ([]types.RawContent, error) {
	return []types.RawContent{{
		Source:     "youtube",
		ExternalID: "yt-1",
		URL:        "https://www.youtube.com/watch?v=yt-1",
		Content:    "# title\n\n（无法获取视频内容，以下为视频简介）\n\nnew fallback",
		Metadata: types.RawMetadata{YouTube: &types.YouTubeMetadata{
			Title:                 "title",
			TranscriptMethod:      "title_only",
			TranscriptDiagnostics: []types.TranscriptDiagnostic{{Stage: "audio", Code: "rate_limited"}},
		}},
	}}, nil
}

type referenceHydratingDispatcher struct{}

func (referenceHydratingDispatcher) SupportsFollow(kind types.Kind, platform types.Platform) bool {
	return false
}

func (referenceHydratingDispatcher) ParseURL(_ context.Context, rawURL string) (types.ParsedURL, error) {
	if rawURL == "https://x.com/root/status/1" {
		return types.ParsedURL{Platform: types.PlatformTwitter, ContentType: types.ContentTypePost, PlatformID: "1", CanonicalURL: rawURL}, nil
	}
	return types.ParsedURL{Platform: types.PlatformTwitter, ContentType: types.ContentTypePost, PlatformID: "2", CanonicalURL: rawURL}, nil
}

func (referenceHydratingDispatcher) DiscoverFollowedTarget(context.Context, types.FollowTarget) ([]types.DiscoveryItem, error) {
	return nil, nil
}

func (referenceHydratingDispatcher) FetchByParsedURL(_ context.Context, parsed types.ParsedURL) ([]types.RawContent, error) {
	switch parsed.PlatformID {
	case "1":
		return []types.RawContent{{
			Source:     "twitter",
			ExternalID: "1",
			Content:    "root body",
			URL:        parsed.CanonicalURL,
			References: []types.Reference{{
				Kind: "post_link",
				URL:  "https://x.com/ref/status/2",
			}},
		}}, nil
	default:
		return []types.RawContent{{
			Source:     "twitter",
			ExternalID: "2",
			Content:    "reference full body",
			AuthorName: "ref-author",
			URL:        parsed.CanonicalURL,
			Quotes: []types.Quote{{
				URL: "https://x.com/quote/status/3",
			}},
			References: []types.Reference{{
				URL: "https://x.com/ref/status/4",
			}},
			Attachments: []types.Attachment{{
				Type: "image",
				URL:  "https://img.test/a.jpg",
			}},
		}}, nil
	}
}

func (referenceHydratingDispatcher) FetchDiscoveryItem(context.Context, types.DiscoveryItem) ([]types.RawContent, error) {
	return nil, nil
}

func TestService_FetchURLHydratesReferenceContentOneLevel(t *testing.T) {
	store := &fakeStore{processed: map[string]bool{}}
	svc := New(store, referenceHydratingDispatcher{}, fakeEnricher{})

	items, err := svc.FetchURL(context.Background(), "https://x.com/root/status/1")
	if err != nil {
		t.Fatalf("FetchURL() error = %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("len(items) = %d, want 1", len(items))
	}
	if len(items[0].References) != 1 {
		t.Fatalf("len(References) = %d, want 1", len(items[0].References))
	}
	ref := items[0].References[0]
	if ref.Content != "reference full body" {
		t.Fatalf("Reference.Content = %q, want hydrated content", ref.Content)
	}
	if ref.AuthorName != "ref-author" {
		t.Fatalf("Reference.AuthorName = %q, want ref-author", ref.AuthorName)
	}
	if len(ref.Attachments) != 1 {
		t.Fatalf("len(Reference.Attachments) = %d, want 1", len(ref.Attachments))
	}
	if len(ref.QuoteURLs) != 1 || ref.QuoteURLs[0] != "https://x.com/quote/status/3" {
		t.Fatalf("QuoteURLs = %#v", ref.QuoteURLs)
	}
	if len(ref.ReferenceURLs) != 1 || ref.ReferenceURLs[0] != "https://x.com/ref/status/4" {
		t.Fatalf("ReferenceURLs = %#v", ref.ReferenceURLs)
	}
}

type feedFetchDispatcher struct{}

func (feedFetchDispatcher) SupportsFollow(kind types.Kind, platform types.Platform) bool {
	return kind == types.KindRSS && platform == types.PlatformRSS
}

func (feedFetchDispatcher) ParseURL(_ context.Context, rawURL string) (types.ParsedURL, error) {
	switch rawURL {
	case "https://feeds.example.test/feed.xml":
		return types.ParsedURL{
			Platform:     types.PlatformRSS,
			ContentType:  types.ContentTypeFeed,
			PlatformID:   "feed-1",
			CanonicalURL: rawURL,
		}, nil
	default:
		return types.ParsedURL{
			Platform:     types.PlatformWeb,
			ContentType:  types.ContentTypePost,
			PlatformID:   "post-1",
			CanonicalURL: rawURL,
		}, nil
	}
}

func (feedFetchDispatcher) DiscoverFollowedTarget(_ context.Context, target types.FollowTarget) ([]types.DiscoveryItem, error) {
	if target.Kind != types.KindRSS || target.URL != "https://feeds.example.test/feed.xml" {
		return nil, errors.New("unexpected feed target")
	}
	return []types.DiscoveryItem{{
		Platform:   types.PlatformRSS,
		ExternalID: "item-guid-1",
		URL:        "https://example.com/post-1",
		AuthorName: "Example Feed",
	}}, nil
}

func (feedFetchDispatcher) FetchByParsedURL(_ context.Context, parsed types.ParsedURL) ([]types.RawContent, error) {
	return []types.RawContent{{
		Source:     string(parsed.Platform),
		ExternalID: parsed.PlatformID,
		URL:        parsed.CanonicalURL,
		Content:    "post body",
	}}, nil
}

func (d feedFetchDispatcher) FetchDiscoveryItem(ctx context.Context, item types.DiscoveryItem) ([]types.RawContent, error) {
	parsed, err := d.ParseURL(ctx, item.URL)
	if err != nil {
		return nil, err
	}
	return d.FetchByParsedURL(ctx, parsed)
}

func TestService_FetchURLExpandsRSSFeedIntoItems(t *testing.T) {
	store := &fakeStore{processed: map[string]bool{}}
	svc := New(store, feedFetchDispatcher{}, fakeEnricher{})

	items, err := svc.FetchURL(context.Background(), "https://feeds.example.test/feed.xml")
	if err != nil {
		t.Fatalf("FetchURL() error = %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("len(items) = %d, want 1 feed item", len(items))
	}
	if items[0].Source != "web" || items[0].ExternalID != "post-1" {
		t.Fatalf("item = %#v, want hydrated web feed item", items[0])
	}
	if len(store.raws) != 1 || store.raws[0].Source != "web" {
		t.Fatalf("raws = %#v, want item raw capture only", store.raws)
	}
	if store.processed["rss:feed-1"] {
		t.Fatal("feed container was marked processed; want item identities only")
	}
	if !store.processed["web:post-1"] {
		t.Fatal("feed item was not marked processed")
	}
}

func TestService_FetchURLAndFollowAuthorExpandsRSSFeedIntoItems(t *testing.T) {
	store := &fakeStore{processed: map[string]bool{}}
	svc := New(store, feedFetchDispatcher{}, fakeEnricher{})

	result, err := svc.FetchURLAndFollowAuthor(context.Background(), "https://feeds.example.test/feed.xml")
	if err != nil {
		t.Fatalf("FetchURLAndFollowAuthor() error = %v", err)
	}
	if result.Author != nil {
		t.Fatalf("Author = %#v, want nil for feed URL", result.Author)
	}
	if len(result.Items) != 1 {
		t.Fatalf("len(Items) = %d, want 1 feed item", len(result.Items))
	}
	if result.Items[0].Source != "web" || result.Items[0].ExternalID != "post-1" {
		t.Fatalf("item = %#v, want hydrated web feed item", result.Items[0])
	}
	if store.processed["rss:feed-1"] {
		t.Fatal("feed container was marked processed; want item identities only")
	}
	if !store.processed["web:post-1"] {
		t.Fatal("feed item was not marked processed")
	}
}

type partialFeedFetchDispatcher struct{}

func (partialFeedFetchDispatcher) SupportsFollow(kind types.Kind, platform types.Platform) bool {
	return kind == types.KindRSS && platform == types.PlatformRSS
}

func (partialFeedFetchDispatcher) ParseURL(_ context.Context, rawURL string) (types.ParsedURL, error) {
	if rawURL == "https://feeds.example.test/feed.xml" {
		return types.ParsedURL{Platform: types.PlatformRSS, ContentType: types.ContentTypeFeed, PlatformID: "feed-1", CanonicalURL: rawURL}, nil
	}
	id := strings.TrimPrefix(rawURL, "https://example.com/")
	return types.ParsedURL{Platform: types.PlatformWeb, ContentType: types.ContentTypePost, PlatformID: id, CanonicalURL: rawURL}, nil
}

func (partialFeedFetchDispatcher) DiscoverFollowedTarget(_ context.Context, _ types.FollowTarget) ([]types.DiscoveryItem, error) {
	return []types.DiscoveryItem{
		{Platform: types.PlatformRSS, ExternalID: "good", URL: "https://example.com/good"},
		{Platform: types.PlatformRSS, ExternalID: "bad", URL: "https://example.com/bad"},
	}, nil
}

func (partialFeedFetchDispatcher) FetchByParsedURL(_ context.Context, parsed types.ParsedURL) ([]types.RawContent, error) {
	if parsed.PlatformID == "bad" {
		return nil, errors.New("origin 503")
	}
	return []types.RawContent{{Source: string(parsed.Platform), ExternalID: parsed.PlatformID, URL: parsed.CanonicalURL, Content: "ok"}}, nil
}

func (d partialFeedFetchDispatcher) FetchDiscoveryItem(ctx context.Context, item types.DiscoveryItem) ([]types.RawContent, error) {
	parsed, err := d.ParseURL(ctx, item.URL)
	if err != nil {
		return nil, err
	}
	return d.FetchByParsedURL(ctx, parsed)
}

func TestService_FetchURLExpandsRSSFeedWithPartialItemFailures(t *testing.T) {
	store := &fakeStore{processed: map[string]bool{}}
	svc := New(store, partialFeedFetchDispatcher{}, fakeEnricher{})

	items, err := svc.FetchURL(context.Background(), "https://feeds.example.test/feed.xml")
	if err != nil {
		t.Fatalf("FetchURL() error = %v, want partial success", err)
	}
	if len(items) != 1 || items[0].ExternalID != "good" {
		t.Fatalf("items = %#v, want only successful item", items)
	}
	if !store.processed["web:good"] {
		t.Fatal("successful feed item was not marked processed")
	}
	if store.processed["web:bad"] {
		t.Fatal("failed feed item was marked processed")
	}
}

func TestService_FollowURLRejectsUnsupportedNativeTarget(t *testing.T) {
	store := &fakeStore{processed: map[string]bool{}}
	svc := New(store, fakeDispatcher{}, fakeEnricher{})

	_, err := svc.FollowURL(context.Background(), "https://twitter.com/elonmusk")
	if err == nil {
		t.Fatal("FollowURL() error = nil, want unsupported native target error")
	}
}

func TestService_FollowURLRegistersRSSStrategy(t *testing.T) {
	store := &fakeStore{processed: map[string]bool{}}
	svc := New(store, fakeDispatcher{}, fakeEnricher{})

	target, err := svc.FollowURL(context.Background(), "https://feeds.example.test/feed.xml")
	if err != nil {
		t.Fatalf("FollowURL() error = %v", err)
	}
	if target.Kind != types.KindRSS {
		t.Fatalf("Kind = %q, want %q", target.Kind, types.KindRSS)
	}
	if target.Platform != "rss" {
		t.Fatalf("Platform = %q, want rss", target.Platform)
	}
	if target.Locator != "https://feeds.example.test/feed.xml" {
		t.Fatalf("Locator = %q, want feed URL", target.Locator)
	}
}

func TestService_FollowSearchRegistersSearchTarget(t *testing.T) {
	store := &fakeStore{processed: map[string]bool{}}
	svc := New(store, fakeDispatcher{}, fakeEnricher{})

	target, err := svc.FollowSearch(context.Background(), types.PlatformTwitter, "  nvda semis  ")
	if err != nil {
		t.Fatalf("FollowSearch() error = %v", err)
	}
	if target.Kind != types.KindSearch {
		t.Fatalf("Kind = %q, want %q", target.Kind, types.KindSearch)
	}
	if target.Platform != "twitter" {
		t.Fatalf("Platform = %q, want twitter", target.Platform)
	}
	if target.Query != "nvda semis" || target.Locator != "nvda semis" {
		t.Fatalf("target = %#v, want trimmed query+locator", target)
	}
}

func TestService_FollowAuthorUsesYouTubeRSSWhenChannelIDKnown(t *testing.T) {
	store := &fakeStore{processed: map[string]bool{}}
	svc := New(store, fakeDispatcher{}, candidateEnricher{})

	result, err := svc.FollowAuthor(context.Background(), types.AuthorFollowRequest{
		Platform:   types.PlatformYouTube,
		AuthorName: "Acme Channel",
		PlatformID: "UCabc123",
	})
	if err != nil {
		t.Fatalf("FollowAuthor() error = %v", err)
	}
	if result.Subscription.Strategy != types.SubscriptionStrategyRSS {
		t.Fatalf("strategy = %q, want rss", result.Subscription.Strategy)
	}
	if len(result.Follows) != 1 {
		t.Fatalf("len(follows) = %d, want 1", len(result.Follows))
	}
	if result.Follows[0].Kind != types.KindRSS {
		t.Fatalf("follow kind = %q, want rss", result.Follows[0].Kind)
	}
	if !strings.Contains(result.Follows[0].URL, "youtube.com/feeds/videos.xml?channel_id=UCabc123") {
		t.Fatalf("rss url = %q", result.Follows[0].URL)
	}
}

func TestService_FollowAuthorUsesNativeWeiboTimelineWhenUIDKnown(t *testing.T) {
	store := &fakeStore{processed: map[string]bool{}}
	svc := New(store, fakeDispatcher{}, candidateEnricher{})

	result, err := svc.FollowAuthor(context.Background(), types.AuthorFollowRequest{
		Platform:   types.PlatformWeibo,
		AuthorName: "Weibo User",
		PlatformID: "123456",
	})
	if err != nil {
		t.Fatalf("FollowAuthor() error = %v", err)
	}
	if result.Subscription.Strategy != types.SubscriptionStrategyNative {
		t.Fatalf("strategy = %q, want native", result.Subscription.Strategy)
	}
	if result.Subscription.ProfileURL != "https://weibo.com/123456" {
		t.Fatalf("profile_url = %q, want canonical weibo profile", result.Subscription.ProfileURL)
	}
	if len(result.Follows) != 1 {
		t.Fatalf("len(follows) = %d, want 1", len(result.Follows))
	}
	if result.Follows[0].Kind != types.KindNative {
		t.Fatalf("follow kind = %q, want native", result.Follows[0].Kind)
	}
	if result.Follows[0].Platform != "weibo" || result.Follows[0].PlatformID != "123456" {
		t.Fatalf("follow = %#v, want weibo/123456", result.Follows[0])
	}
	if len(store.queries) != 0 {
		t.Fatalf("len(queries) = %d, want no search queries for native weibo", len(store.queries))
	}
}

func TestService_FollowAuthorGeneratesSearchFollowsForTwitterProfile(t *testing.T) {
	store := &fakeStore{processed: map[string]bool{}}
	svc := New(store, fakeDispatcher{}, candidateEnricher{})

	result, err := svc.FollowAuthor(context.Background(), types.AuthorFollowRequest{
		ProfileURL: "https://twitter.com/ElonMusk",
		AuthorName: "Elon Musk",
	})
	if err != nil {
		t.Fatalf("FollowAuthor() error = %v", err)
	}
	if result.Subscription.Strategy != types.SubscriptionStrategySearch {
		t.Fatalf("strategy = %q, want search", result.Subscription.Strategy)
	}
	if result.Subscription.PlatformID != "elonmusk" {
		t.Fatalf("platform_id = %q", result.Subscription.PlatformID)
	}
	if result.Subscription.ProfileURL != "https://twitter.com/elonmusk" {
		t.Fatalf("profile_url = %q, want canonical lowercase profile", result.Subscription.ProfileURL)
	}
	if len(result.Follows) != 2 {
		t.Fatalf("len(follows) = %d, want 2", len(result.Follows))
	}
	if result.Follows[0].Query != "site:x.com/elonmusk/status" {
		t.Fatalf("first query = %q", result.Follows[0].Query)
	}
	if len(store.queries) != 2 {
		t.Fatalf("len(queries) = %d, want 2", len(store.queries))
	}
	if store.queries[0].Query != "site:x.com/elonmusk/status" {
		t.Fatalf("stored query = %q, want canonical lowercase query", store.queries[0].Query)
	}
}

func TestService_FollowAuthorDerivesTwitterAuthorFromPostURL(t *testing.T) {
	store := &fakeStore{processed: map[string]bool{}}
	svc := New(store, fakeDispatcher{}, candidateEnricher{})

	result, err := svc.FollowAuthor(context.Background(), types.AuthorFollowRequest{
		ProfileURL: "https://x.com/robin_j_brooks/status/2049570595277300120?s=20",
	})
	if err != nil {
		t.Fatalf("FollowAuthor() error = %v", err)
	}
	if result.Subscription.PlatformID != "robin_j_brooks" {
		t.Fatalf("platform_id = %q, want robin_j_brooks", result.Subscription.PlatformID)
	}
	if result.Subscription.ProfileURL != "https://twitter.com/robin_j_brooks" {
		t.Fatalf("profile_url = %q, want twitter profile", result.Subscription.ProfileURL)
	}
	if len(result.Follows) != 2 {
		t.Fatalf("len(follows) = %d, want 2", len(result.Follows))
	}
	if result.Follows[0].Query != "site:x.com/robin_j_brooks/status" {
		t.Fatalf("first query = %q", result.Follows[0].Query)
	}
}

func TestAuthorSearchQueriesUsePlatformIDWhenNameMissing(t *testing.T) {
	got := AuthorSearchQueries(types.AuthorFollowRequest{
		Platform:   types.PlatformYouTube,
		PlatformID: "AcmeChannel",
	})
	if len(got) != 2 {
		t.Fatalf("len(queries) = %d, want 2", len(got))
	}
	if got[0].Query != `site:youtube.com/watch "AcmeChannel"` {
		t.Fatalf("first query = %q", got[0].Query)
	}
}

func TestService_RemoveFollowURLCanonicalizesRSS(t *testing.T) {
	store := &fakeStore{processed: map[string]bool{}}
	svc := New(store, fakeDispatcher{}, fakeEnricher{})

	target, err := svc.FollowURL(context.Background(), "https://feeds.example.test/feed.xml")
	if err != nil {
		t.Fatalf("FollowURL() error = %v", err)
	}
	if err := svc.RemoveFollowURL(context.Background(), "https://feeds.example.test/feed.xml"); err != nil {
		t.Fatalf("RemoveFollowURL() error = %v", err)
	}
	if len(store.removedKeys) != 1 {
		t.Fatalf("len(removedKeys) = %d, want 1", len(store.removedKeys))
	}
	want := string(target.Kind) + ":" + target.Platform + ":" + target.Locator
	if store.removedKeys[0] != want {
		t.Fatalf("removed key = %q, want %q", store.removedKeys[0], want)
	}
}

func TestService_RemoveFollowSearchNormalizesQuery(t *testing.T) {
	store := &fakeStore{processed: map[string]bool{}}
	svc := New(store, fakeDispatcher{}, fakeEnricher{})

	if err := svc.RemoveFollowSearch(context.Background(), types.PlatformTwitter, "  nvda   semis "); err != nil {
		t.Fatalf("RemoveFollowSearch() error = %v", err)
	}
	if len(store.removedKeys) != 1 {
		t.Fatalf("len(removedKeys) = %d, want 1", len(store.removedKeys))
	}
	if store.removedKeys[0] != "search:twitter:nvda semis" {
		t.Fatalf("removed key = %q, want %q", store.removedKeys[0], "search:twitter:nvda semis")
	}
}

func TestService_ListFollowsReturnsTargetsAndWarnings(t *testing.T) {
	store := &fakeStore{
		processed: map[string]bool{},
		follows: []types.FollowTarget{{
			Kind:       types.KindRSS,
			Platform:   "rss",
			Locator:    "https://feeds.example.test/feed.xml",
			URL:        "https://feeds.example.test/feed.xml",
			FollowedAt: time.Now().UTC(),
		}},
		warnings: []contentstore.ScanWarning{{
			Path: "/tmp/follow_bad.json",
			Kind: contentstore.WarningKindCorruptJSON,
		}},
	}
	svc := New(store, fakeDispatcher{}, fakeEnricher{})

	items, warnings, err := svc.ListFollows(context.Background())
	if err != nil {
		t.Fatalf("ListFollows() error = %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("len(items) = %d, want 1", len(items))
	}
	if len(warnings) != 1 {
		t.Fatalf("len(warnings) = %d, want 1", len(warnings))
	}
}

func TestService_PollSkipsProcessedMarksNewAndUpdatesFollowState(t *testing.T) {
	store := &fakeStore{
		processed: map[string]bool{"twitter:done": true},
		follows: []types.FollowTarget{{
			Kind:       types.KindSearch,
			Platform:   "twitter",
			Locator:    "nvda",
			Query:      "nvda",
			FollowedAt: time.Now().UTC(),
		}},
	}
	svc := New(store, fakeDispatcher{
		discovered: []types.DiscoveryItem{
			{Platform: types.PlatformTwitter, ExternalID: "done", URL: "https://x.com/a/status/done"},
			{Platform: types.PlatformTwitter, ExternalID: "new", URL: "https://x.com/a/status/new"},
		},
	}, nil)

	report, items, warnings, pollWarnings, err := svc.Poll(context.Background())
	if err != nil {
		t.Fatalf("Poll() error = %v", err)
	}
	if len(warnings) != 0 {
		t.Fatalf("len(warnings) = %d, want 0", len(warnings))
	}
	if len(pollWarnings) != 0 {
		t.Fatalf("len(pollWarnings) = %d, want 0", len(pollWarnings))
	}
	if len(items) != 1 {
		t.Fatalf("len(items) = %d, want 1", len(items))
	}
	if !store.processed["twitter:new"] {
		t.Fatal("new item was not marked processed")
	}
	if len(store.polledKeys) != 1 || store.polledKeys[0] != "search:twitter:nvda" {
		t.Fatalf("polledKeys = %#v, want search:twitter:nvda", store.polledKeys)
	}
	if report.TargetCount != 1 || report.DiscoveredCount != 2 || report.FetchedCount != 1 || report.SkippedCount != 1 {
		t.Fatalf("report = %#v", report)
	}
	if len(store.reports) != 1 {
		t.Fatalf("len(store.reports) = %d, want 1", len(store.reports))
	}
}

func TestService_PollSkipsSearchTargetsUntilAssignedTimeSlot(t *testing.T) {
	base := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)
	target := types.FollowTarget{
		Kind:         types.KindSearch,
		Platform:     "twitter",
		Locator:      "site:x.com/robin_j_brooks/status",
		Query:        "site:x.com/robin_j_brooks/status",
		FollowedAt:   base.Add(-24 * time.Hour),
		LastPolledAt: base.Add(-4 * time.Hour),
	}
	slotCount := int(searchFollowCadence / pollSchedulerSlot)
	now := base.Add(time.Duration((assignedPollSlot(target, slotCount)+1)%slotCount) * pollSchedulerSlot)
	target.LastPolledAt = now.Add(-4 * time.Hour)

	store := &fakeStore{
		processed: map[string]bool{},
		follows:   []types.FollowTarget{target},
	}
	svc := New(store, fakeDispatcher{}, fakeEnricher{})
	svc.now = func() time.Time { return now }

	report, items, _, warnings, err := svc.Poll(context.Background())
	if err != nil {
		t.Fatalf("Poll() error = %v", err)
	}
	if len(warnings) != 0 {
		t.Fatalf("len(warnings) = %d, want 0", len(warnings))
	}
	if len(items) != 0 {
		t.Fatalf("len(items) = %d, want 0", len(items))
	}
	if len(store.polledKeys) != 0 {
		t.Fatalf("polledKeys = %#v, want no poll", store.polledKeys)
	}
	if report.TargetCount != 1 || report.DiscoveredCount != 0 {
		t.Fatalf("report = %#v, want configured target but no discovery", report)
	}
}

func TestService_PollChecksSearchTargetInAssignedTimeSlot(t *testing.T) {
	base := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)
	target := types.FollowTarget{
		Kind:         types.KindSearch,
		Platform:     "twitter",
		Locator:      "site:x.com/robin_j_brooks/status",
		Query:        "site:x.com/robin_j_brooks/status",
		FollowedAt:   base.Add(-24 * time.Hour),
		LastPolledAt: base.Add(-4 * time.Hour),
	}
	slotCount := int(searchFollowCadence / pollSchedulerSlot)
	now := base.Add(time.Duration(assignedPollSlot(target, slotCount)) * pollSchedulerSlot)
	target.LastPolledAt = now.Add(-4 * time.Hour)

	store := &fakeStore{
		processed: map[string]bool{},
		follows:   []types.FollowTarget{target},
	}
	svc := New(store, fakeDispatcher{
		discovered: []types.DiscoveryItem{{
			Platform:   types.PlatformTwitter,
			ExternalID: "new-slot",
			URL:        "https://x.com/a/status/new-slot",
		}},
	}, fakeEnricher{})
	svc.now = func() time.Time { return now }

	report, items, _, warnings, err := svc.Poll(context.Background())
	if err != nil {
		t.Fatalf("Poll() error = %v", err)
	}
	if len(warnings) != 0 {
		t.Fatalf("len(warnings) = %d, want 0", len(warnings))
	}
	if len(items) != 1 {
		t.Fatalf("len(items) = %d, want 1", len(items))
	}
	if len(store.polledKeys) != 1 {
		t.Fatalf("polledKeys = %#v, want one poll", store.polledKeys)
	}
	if report.DiscoveredCount != 1 || report.FetchedCount != 1 {
		t.Fatalf("report = %#v, want discovered and fetched", report)
	}
}

func TestService_PollDoesNotMissNextSearchSlotWhenPreviousPollWasInsideSlot(t *testing.T) {
	base := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)
	target := types.FollowTarget{
		Kind:       types.KindSearch,
		Platform:   "twitter",
		Locator:    "site:x.com/robin_j_brooks/status",
		Query:      "site:x.com/robin_j_brooks/status",
		FollowedAt: base.Add(-24 * time.Hour),
	}
	slotCount := int(searchFollowCadence / pollSchedulerSlot)
	firstSlotStart := base.Add(time.Duration(assignedPollSlot(target, slotCount)) * pollSchedulerSlot)
	nextSlotStart := firstSlotStart.Add(searchFollowCadence)
	target.LastPolledAt = firstSlotStart.Add(7 * time.Minute)

	if !isFollowDue(target, nextSlotStart) {
		t.Fatal("target should be due at the next assigned slot even if the prior poll finished inside its slot")
	}
}

func TestScheduleForFollowReportsSearchCadenceAndNextPoll(t *testing.T) {
	base := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)
	target := types.FollowTarget{
		Kind:         types.KindSearch,
		Platform:     "twitter",
		Locator:      "site:x.com/robin_j_brooks/status",
		Query:        "site:x.com/robin_j_brooks/status",
		FollowedAt:   base.Add(-24 * time.Hour),
		LastPolledAt: base.Add(-4 * time.Hour),
	}
	slotCount := int(searchFollowCadence / pollSchedulerSlot)
	assigned := assignedPollSlot(target, slotCount)
	now := base.Add(time.Duration((assigned+1)%slotCount) * pollSchedulerSlot)
	target.LastPolledAt = now.Add(-4 * time.Hour)

	got := ScheduleForFollow(target, now)
	if got.Cadence != searchFollowCadence {
		t.Fatalf("Cadence = %v, want %v", got.Cadence, searchFollowCadence)
	}
	if got.Due {
		t.Fatal("Due = true, want false outside assigned slot")
	}
	if got.SlotIndex != assigned || got.SlotCount != slotCount {
		t.Fatalf("slot = %d/%d, want %d/%d", got.SlotIndex, got.SlotCount, assigned, slotCount)
	}
	if got.NextPollAt.IsZero() || currentPollSlot(got.NextPollAt, slotCount) != assigned {
		t.Fatalf("NextPollAt = %v, want assigned slot", got.NextPollAt)
	}
}

func TestService_PollChecksNewSearchTargetImmediately(t *testing.T) {
	now := time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)
	target := types.FollowTarget{
		Kind:       types.KindSearch,
		Platform:   "twitter",
		Locator:    "site:x.com/new_author/status",
		Query:      "site:x.com/new_author/status",
		FollowedAt: now,
	}

	store := &fakeStore{
		processed: map[string]bool{},
		follows:   []types.FollowTarget{target},
	}
	svc := New(store, fakeDispatcher{
		discovered: []types.DiscoveryItem{{
			Platform:   types.PlatformTwitter,
			ExternalID: "new-author",
			URL:        "https://x.com/a/status/new-author",
		}},
	}, fakeEnricher{})
	svc.now = func() time.Time { return now }

	_, items, _, warnings, err := svc.Poll(context.Background())
	if err != nil {
		t.Fatalf("Poll() error = %v", err)
	}
	if len(warnings) != 0 {
		t.Fatalf("len(warnings) = %d, want 0", len(warnings))
	}
	if len(items) != 1 {
		t.Fatalf("len(items) = %d, want immediate fetch", len(items))
	}
}

func TestService_PollContinuesAfterTargetDiscoverError(t *testing.T) {
	store := &fakeStore{
		processed: map[string]bool{},
		follows: []types.FollowTarget{
			{
				Kind:       types.KindSearch,
				Platform:   "twitter",
				Locator:    "bad",
				Query:      "bad",
				FollowedAt: time.Now().UTC(),
			},
			{
				Kind:       types.KindSearch,
				Platform:   "twitter",
				Locator:    "good",
				Query:      "good",
				FollowedAt: time.Now().UTC(),
			},
		},
	}
	svc := New(store, fakeDispatcher{
		discovered: []types.DiscoveryItem{
			{Platform: types.PlatformTwitter, ExternalID: "new", URL: "https://x.com/a/status/new"},
		},
		discoverErrFor: map[string]error{
			"search:twitter:bad": errors.New("discover failed"),
		},
	}, nil)

	report, items, storeWarnings, pollWarnings, err := svc.Poll(context.Background())
	if err != nil {
		t.Fatalf("Poll() error = %v", err)
	}
	if len(storeWarnings) != 0 {
		t.Fatalf("len(storeWarnings) = %d, want 0", len(storeWarnings))
	}
	if len(pollWarnings) != 1 {
		t.Fatalf("len(pollWarnings) = %d, want 1", len(pollWarnings))
	}
	if pollWarnings[0].Kind != WarningKindDiscover {
		t.Fatalf("pollWarnings[0].Kind = %q, want %q", pollWarnings[0].Kind, WarningKindDiscover)
	}
	if len(items) != 1 {
		t.Fatalf("len(items) = %d, want 1", len(items))
	}
	if !store.processed["twitter:new"] {
		t.Fatal("good target item was not marked processed")
	}
	if len(store.polledKeys) != 2 || store.polledKeys[0] != "search:twitter:bad" || store.polledKeys[1] != "search:twitter:good" {
		t.Fatalf("polledKeys = %#v, want failed and successful target updates", store.polledKeys)
	}
	if report.TargetCount != 2 || report.DiscoveredCount != 1 || report.FetchedCount != 1 {
		t.Fatalf("report = %#v", report)
	}
}

func TestService_PollTurnsSearchNoResultIntoWarning(t *testing.T) {
	store := &fakeStore{
		processed: map[string]bool{},
		follows: []types.FollowTarget{{
			Kind:     types.KindSearch,
			Platform: "twitter",
			Locator:  "nvda",
			Query:    "nvda",
		}},
	}
	svc := New(store, fakeDispatcher{
		discoverErrFor: map[string]error{
			"search:twitter:nvda": errors.New("search produced no usable result urls"),
		},
	}, fakeEnricher{})

	_, _, _, warnings, err := svc.Poll(context.Background())
	if err != nil {
		t.Fatalf("Poll() error = %v", err)
	}
	if len(warnings) == 0 {
		t.Fatal("len(warnings) = 0, want discover warning")
	}
	if warnings[0].Kind != WarningKindDiscover {
		t.Fatalf("warnings[0].Kind = %q, want %q", warnings[0].Kind, WarningKindDiscover)
	}
}

func TestService_PollUpdatesLastPolledAfterDiscoverErrorForBackoff(t *testing.T) {
	now := time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)
	store := &fakeStore{
		processed: map[string]bool{},
		follows: []types.FollowTarget{{
			Kind:       types.KindSearch,
			Platform:   "twitter",
			Locator:    "site:x.com/rate_limited/status",
			Query:      "site:x.com/rate_limited/status",
			FollowedAt: now.Add(-24 * time.Hour),
		}},
	}
	svc := New(store, fakeDispatcher{
		discoverErrFor: map[string]error{
			"search:twitter:site:x.com/rate_limited/status": errors.New("rate limited"),
		},
	}, fakeEnricher{})
	svc.now = func() time.Time { return now }

	_, _, _, warnings, err := svc.Poll(context.Background())
	if err != nil {
		t.Fatalf("Poll() error = %v", err)
	}
	if len(warnings) != 1 {
		t.Fatalf("len(warnings) = %d, want 1", len(warnings))
	}
	if len(store.polledKeys) != 1 {
		t.Fatalf("polledKeys = %#v, want failed target marked polled for backoff", store.polledKeys)
	}
}

func TestService_FetchURLMarksProcessed(t *testing.T) {
	store := &fakeStore{processed: map[string]bool{}}
	svc := New(store, fakeDispatcher{}, candidateEnricher{})

	got, err := svc.FetchURL(context.Background(), "https://x.com/a/status/123")
	if err != nil {
		t.Fatalf("FetchURL() error = %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("len(FetchURL()) = %d, want 1", len(got))
	}
	if !store.processed["twitter:123"] {
		t.Fatal("processed state was not updated by FetchURL")
	}
}

func TestService_FetchURLAndFollowAuthorSubscribesTwitterPostAuthor(t *testing.T) {
	store := &fakeStore{processed: map[string]bool{}}
	svc := New(store, fakeDispatcher{}, candidateEnricher{})

	got, err := svc.FetchURLAndFollowAuthor(context.Background(), "https://x.com/robin_j_brooks/status/2049570595277300120?s=20")
	if err != nil {
		t.Fatalf("FetchURLAndFollowAuthor() error = %v", err)
	}
	if len(got.Items) != 1 {
		t.Fatalf("len(Items) = %d, want 1", len(got.Items))
	}
	if !store.processed["twitter:2049570595277300120"] {
		t.Fatal("tweet was not marked processed")
	}
	if got.Author == nil {
		t.Fatal("Author = nil, want subscription result")
	}
	if got.Author.Subscription.Platform != types.PlatformTwitter {
		t.Fatalf("subscription platform = %q, want twitter", got.Author.Subscription.Platform)
	}
	if got.Author.Subscription.PlatformID != "robin_j_brooks" {
		t.Fatalf("subscription platform_id = %q, want robin_j_brooks", got.Author.Subscription.PlatformID)
	}
	if len(got.Author.Follows) != 2 {
		t.Fatalf("len(author follows) = %d, want 2", len(got.Author.Follows))
	}
	if got.Author.Follows[0].Query != "site:x.com/robin_j_brooks/status" {
		t.Fatalf("first author query = %q", got.Author.Follows[0].Query)
	}
	if len(store.authors) != 1 {
		t.Fatalf("len(authors) = %d, want 1", len(store.authors))
	}
}

func TestService_FetchURLAndFollowAuthorSubscribesAuthorWhenFetchFails(t *testing.T) {
	store := &fakeStore{processed: map[string]bool{}}
	svc := New(store, fetchErrorDispatcher{
		fakeDispatcher: fakeDispatcher{},
		err:            errors.New("temporary fetch failure"),
	}, candidateEnricher{})

	_, err := svc.FetchURLAndFollowAuthor(context.Background(), "https://x.com/robin_j_brooks/status/2049570595277300120?s=20")
	if err == nil {
		t.Fatal("FetchURLAndFollowAuthor() error = nil, want fetch error")
	}
	if len(store.authors) != 1 {
		t.Fatalf("len(authors) = %d, want 1", len(store.authors))
	}
	if store.authors[0].PlatformID != "robin_j_brooks" {
		t.Fatalf("subscription platform_id = %q, want robin_j_brooks", store.authors[0].PlatformID)
	}
	if len(store.follows) != 2 {
		t.Fatalf("len(follows) = %d, want 2", len(store.follows))
	}
}

func TestService_FetchURLPersistsRawBeforeMarkProcessed(t *testing.T) {
	store := &fakeStore{processed: map[string]bool{}}
	svc := New(store, fakeDispatcher{}, fakeEnricher{})

	got, err := svc.FetchURL(context.Background(), "https://x.com/a/status/123")
	if err != nil {
		t.Fatalf("FetchURL() error = %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("len(FetchURL()) = %d, want 1", len(got))
	}
	if len(store.raws) != 1 {
		t.Fatalf("len(raws) = %d, want 1", len(store.raws))
	}
	if store.raws[0].Provenance == nil {
		t.Fatal("raw capture provenance is nil")
	}
	if got[0].Provenance == nil {
		t.Fatal("returned raw provenance is nil")
	}
	if len(store.ops) < 2 {
		t.Fatalf("ops = %#v, want at least raw then mark", store.ops)
	}
	if store.ops[0] != "raw:twitter:123" || store.ops[1] != "mark:twitter:123" {
		t.Fatalf("ops = %#v, want raw before mark", store.ops)
	}
}

func TestService_FetchURLPreservesStoredResolvedProvenance(t *testing.T) {
	store := &fakeStore{
		processed: map[string]bool{},
		raws: []types.RawContent{{
			Source:     "twitter",
			ExternalID: "123",
			URL:        "https://x.com/a/status/123",
			Provenance: &types.Provenance{
				BaseRelation:      types.BaseRelationTranslation,
				NeedsSourceLookup: true,
				SourceLookup: types.SourceLookupState{
					Status:             types.SourceLookupStatusFound,
					CanonicalSourceURL: "https://example.com/source",
				},
			},
		}},
	}
	svc := New(store, fakeDispatcher{}, candidateEnricher{})

	got, err := svc.FetchURL(context.Background(), "https://x.com/a/status/123")
	if err != nil {
		t.Fatalf("FetchURL() error = %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("len(FetchURL()) = %d, want 1", len(got))
	}
	if got[0].Provenance == nil {
		t.Fatal("returned raw provenance is nil")
	}
	if got[0].Provenance.SourceLookup.Status != types.SourceLookupStatusFound {
		t.Fatalf("SourceLookup.Status = %q, want %q", got[0].Provenance.SourceLookup.Status, types.SourceLookupStatusFound)
	}
	if got[0].Provenance.SourceLookup.CanonicalSourceURL != "https://example.com/source" {
		t.Fatalf("CanonicalSourceURL = %q, want preserved source url", got[0].Provenance.SourceLookup.CanonicalSourceURL)
	}
}

func TestService_FetchURLPreservesStoredResolvedProvenanceAndNewReuseEvidence(t *testing.T) {
	store := &fakeStore{
		processed: map[string]bool{},
		raws: []types.RawContent{{
			Source:     "youtube",
			ExternalID: "yt-1",
			URL:        "https://www.youtube.com/watch?v=yt-1",
			Content:    "stored high quality transcript",
			Metadata: types.RawMetadata{YouTube: &types.YouTubeMetadata{
				Title:            "title",
				TranscriptMethod: "subtitle_vtt",
			}},
			Provenance: &types.Provenance{
				BaseRelation:      types.BaseRelationTranslation,
				NeedsSourceLookup: true,
				SourceLookup: types.SourceLookupState{
					Status:             types.SourceLookupStatusFound,
					CanonicalSourceURL: "https://example.com/source",
				},
			},
		}},
	}
	svc := New(store, transcriptPreservingDispatcher{}, candidateEnricher{})

	got, err := svc.FetchURL(context.Background(), "https://www.youtube.com/watch?v=yt-1")
	if err != nil {
		t.Fatalf("FetchURL() error = %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("len(FetchURL()) = %d, want 1", len(got))
	}
	if got[0].Provenance == nil {
		t.Fatal("returned raw provenance is nil")
	}
	if got[0].Provenance.SourceLookup.CanonicalSourceURL != "https://example.com/source" {
		t.Fatalf("CanonicalSourceURL = %q, want preserved source url", got[0].Provenance.SourceLookup.CanonicalSourceURL)
	}
	if !hasEvidence(got[0].Provenance, "stored_capture_reused", "kept=subtitle_vtt") {
		t.Fatalf("Evidence = %#v, want stored_capture_reused entry to survive preserved provenance", got[0].Provenance)
	}
}

// tombstoneDispatcher returns (nil, nil) from FetchDiscoveryItem for items
// in the emptyFetchIDs set, simulating Twitter 404/tombstone responses.
type tombstoneDispatcher struct {
	fakeDispatcher
	emptyFetchIDs map[string]bool
	fetchCalls    int
}

func (d *tombstoneDispatcher) FetchDiscoveryItem(_ context.Context, item types.DiscoveryItem) ([]types.RawContent, error) {
	d.fetchCalls++
	if d.emptyFetchIDs[item.ExternalID] {
		return nil, nil
	}
	platform := item.Platform
	if platform == "" {
		platform = types.PlatformTwitter
	}
	externalID := item.ExternalID
	if externalID == "" {
		externalID = "123"
	}
	return []types.RawContent{{
		Source:     string(platform),
		ExternalID: externalID,
		URL:        item.URL,
		PostedAt:   time.Now().UTC(),
	}}, nil
}

type shortURLDispatcher struct {
	fakeDispatcher
	fetchCalls int
}

func (d *shortURLDispatcher) ParseURL(ctx context.Context, rawURL string) (types.ParsedURL, error) {
	if rawURL == "https://t.co/abc123" {
		return types.ParsedURL{
			Platform:     types.PlatformTwitter,
			ContentType:  types.ContentTypePost,
			PlatformID:   "123",
			CanonicalURL: "https://x.com/a/status/123",
		}, nil
	}
	return d.fakeDispatcher.ParseURL(ctx, rawURL)
}

func (d *shortURLDispatcher) FetchDiscoveryItem(_ context.Context, item types.DiscoveryItem) ([]types.RawContent, error) {
	d.fetchCalls++
	return []types.RawContent{{
		Source:     string(types.PlatformTwitter),
		ExternalID: "123",
		URL:        "https://x.com/a/status/123",
		PostedAt:   time.Now().UTC(),
	}}, nil
}

func TestService_PollMarksEmptyFetchAsProcessed(t *testing.T) {
	store := &fakeStore{
		processed: map[string]bool{},
		follows: []types.FollowTarget{{
			Kind:       types.KindSearch,
			Platform:   "twitter",
			Locator:    "nvda",
			Query:      "nvda",
			FollowedAt: time.Now().UTC(),
		}},
	}
	dispatcher := &tombstoneDispatcher{
		fakeDispatcher: fakeDispatcher{
			discovered: []types.DiscoveryItem{
				{Platform: types.PlatformTwitter, ExternalID: "dead-tweet", URL: "https://x.com/a/status/dead-tweet"},
			},
		},
		emptyFetchIDs: map[string]bool{"dead-tweet": true},
	}
	svc := New(store, dispatcher, fakeEnricher{})

	// First poll: FetchDiscoveryItem returns (nil, nil) for "dead-tweet".
	// The item should be marked as processed despite having no raw content.
	report, items, _, pollWarnings, err := svc.Poll(context.Background())
	if err != nil {
		t.Fatalf("Poll() error = %v", err)
	}
	if len(pollWarnings) != 0 {
		t.Fatalf("len(pollWarnings) = %d, want 0; warnings: %#v", len(pollWarnings), pollWarnings)
	}
	if len(items) != 0 {
		t.Fatalf("len(items) = %d, want 0 (tombstone returns no content)", len(items))
	}
	if !store.processed["twitter:dead-tweet"] {
		t.Fatal("dead tweet was not marked as processed")
	}
	if report.FetchedCount != 0 {
		t.Fatalf("report.FetchedCount = %d, want 0", report.FetchedCount)
	}
	if dispatcher.fetchCalls != 1 {
		t.Fatalf("fetchCalls = %d, want 1", dispatcher.fetchCalls)
	}

	// Second poll: the item should be skipped because it's already processed.
	report2, items2, _, pollWarnings2, err := svc.Poll(context.Background())
	if err != nil {
		t.Fatalf("Poll() #2 error = %v", err)
	}
	if len(pollWarnings2) != 0 {
		t.Fatalf("Poll() #2 len(pollWarnings) = %d, want 0", len(pollWarnings2))
	}
	if len(items2) != 0 {
		t.Fatalf("Poll() #2 len(items) = %d, want 0", len(items2))
	}
	if report2.SkippedCount != 1 {
		t.Fatalf("Poll() #2 report.SkippedCount = %d, want 1", report2.SkippedCount)
	}
	// FetchDiscoveryItem should NOT have been called again.
	if dispatcher.fetchCalls != 1 {
		t.Fatalf("fetchCalls after second poll = %d, want 1 (should not re-fetch)", dispatcher.fetchCalls)
	}
}

func TestService_PollDedupesResolvedShortURLOnNextPoll(t *testing.T) {
	store := &fakeStore{
		processed: map[string]bool{},
		follows: []types.FollowTarget{{
			Kind:       types.KindSearch,
			Platform:   "twitter",
			Locator:    "nvda",
			Query:      "nvda",
			FollowedAt: time.Now().UTC(),
		}},
	}
	dispatcher := &shortURLDispatcher{
		fakeDispatcher: fakeDispatcher{
			discovered: []types.DiscoveryItem{
				{URL: "https://t.co/abc123"},
			},
		},
	}
	svc := New(store, dispatcher, fakeEnricher{})

	report, items, _, pollWarnings, err := svc.Poll(context.Background())
	if err != nil {
		t.Fatalf("Poll() error = %v", err)
	}
	if len(pollWarnings) != 0 {
		t.Fatalf("len(pollWarnings) = %d, want 0", len(pollWarnings))
	}
	if len(items) != 1 {
		t.Fatalf("len(items) = %d, want 1", len(items))
	}
	if !store.processed["twitter:123"] {
		t.Fatal("resolved short URL item was not marked processed with resolved identity")
	}
	if dispatcher.fetchCalls != 1 {
		t.Fatalf("fetchCalls after first poll = %d, want 1", dispatcher.fetchCalls)
	}
	if report.FetchedCount != 1 || report.SkippedCount != 0 {
		t.Fatalf("report = %#v", report)
	}

	report2, items2, _, pollWarnings2, err := svc.Poll(context.Background())
	if err != nil {
		t.Fatalf("Poll() #2 error = %v", err)
	}
	if len(pollWarnings2) != 0 {
		t.Fatalf("len(pollWarnings2) = %d, want 0", len(pollWarnings2))
	}
	if len(items2) != 0 {
		t.Fatalf("len(items2) = %d, want 0", len(items2))
	}
	if dispatcher.fetchCalls != 1 {
		t.Fatalf("fetchCalls after second poll = %d, want 1", dispatcher.fetchCalls)
	}
	if report2.SkippedCount != 1 {
		t.Fatalf("report2.SkippedCount = %d, want 1", report2.SkippedCount)
	}
}

func TestShouldPreserveStoredProvenance(t *testing.T) {
	if shouldPreserveStoredProvenance(nil) {
		t.Fatal("nil provenance should not be preserved")
	}
	if shouldPreserveStoredProvenance(&types.Provenance{NeedsSourceLookup: false}) {
		t.Fatal("not_needed provenance should not be preserved")
	}
	if shouldPreserveStoredProvenance(&types.Provenance{NeedsSourceLookup: true}) {
		t.Fatal("lookup-needed provenance without candidates should not be preserved")
	}
	if !shouldPreserveStoredProvenance(&types.Provenance{NeedsSourceLookup: true, SourceCandidates: []types.SourceCandidate{{URL: "https://example.com"}}}) {
		t.Fatal("lookup-needed provenance with candidates should be preserved")
	}
}

func TestService_FetchURLReusesStoredHighQualityTranscriptOnRateLimit(t *testing.T) {
	store := &fakeStore{processed: map[string]bool{}, raws: []types.RawContent{{
		Source:     "youtube",
		ExternalID: "yt-1",
		URL:        "https://www.youtube.com/watch?v=yt-1",
		Content:    "stored high quality transcript",
		Metadata: types.RawMetadata{YouTube: &types.YouTubeMetadata{
			Title:            "title",
			TranscriptMethod: "subtitle_vtt",
		}},
	}}}
	svc := New(store, transcriptPreservingDispatcher{}, fakeEnricher{})
	items, err := svc.FetchURL(context.Background(), "https://www.youtube.com/watch?v=yt-1")
	if err != nil {
		t.Fatalf("FetchURL() error = %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("len(items) = %d, want 1", len(items))
	}
	if items[0].Content != "stored high quality transcript" {
		t.Fatalf("Content = %q, want stored transcript", items[0].Content)
	}
	if items[0].Metadata.YouTube == nil || items[0].Metadata.YouTube.TranscriptMethod != "subtitle_vtt" {
		t.Fatalf("TranscriptMethod = %#v, want subtitle_vtt", items[0].Metadata.YouTube)
	}
}

func TestService_FetchURLCanDisableStoredTranscriptReuse(t *testing.T) {
	store := &fakeStore{processed: map[string]bool{}, raws: []types.RawContent{{
		Source:     "youtube",
		ExternalID: "yt-1",
		URL:        "https://www.youtube.com/watch?v=yt-1",
		Content:    "stored high quality transcript",
		Metadata: types.RawMetadata{YouTube: &types.YouTubeMetadata{
			Title:            "title",
			TranscriptMethod: "subtitle_vtt",
		}},
	}}}
	svc := New(store, transcriptPreservingDispatcher{}, fakeEnricher{}, WithStoredCaptureReuse(false))
	items, err := svc.FetchURL(context.Background(), "https://www.youtube.com/watch?v=yt-1")
	if err != nil {
		t.Fatalf("FetchURL() error = %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("len(items) = %d, want 1", len(items))
	}
	if items[0].Content == "stored high quality transcript" {
		t.Fatalf("Content = %q, want fresh fallback content when reuse disabled", items[0].Content)
	}
	if items[0].Metadata.YouTube == nil || items[0].Metadata.YouTube.TranscriptMethod != "title_only" {
		t.Fatalf("TranscriptMethod = %#v, want title_only when reuse disabled", items[0].Metadata.YouTube)
	}
}

func TestService_PollReusesStoredHighQualityTranscriptOnRateLimit(t *testing.T) {
	store := &fakeStore{
		processed: map[string]bool{},
		follows: []types.FollowTarget{{
			Kind:     types.KindNative,
			Platform: string(types.PlatformYouTube),
			Locator:  "https://www.youtube.com/watch?v=yt-1",
			URL:      "https://www.youtube.com/watch?v=yt-1",
		}},
		raws: []types.RawContent{{
			Source:     "youtube",
			ExternalID: "yt-1",
			URL:        "https://www.youtube.com/watch?v=yt-1",
			Content:    "stored high quality transcript",
			Metadata: types.RawMetadata{YouTube: &types.YouTubeMetadata{
				Title:            "title",
				TranscriptMethod: "subtitle_vtt",
			}},
		}},
	}
	svc := New(store, transcriptPreservingDispatcher{}, fakeEnricher{})

	report, items, _, pollWarnings, err := svc.Poll(context.Background())
	if err != nil {
		t.Fatalf("Poll() error = %v", err)
	}
	if len(pollWarnings) != 0 {
		t.Fatalf("len(pollWarnings) = %d, want 0", len(pollWarnings))
	}
	if report.FetchedCount != 1 {
		t.Fatalf("report.FetchedCount = %d, want 1", report.FetchedCount)
	}
	if len(items) != 1 {
		t.Fatalf("len(items) = %d, want 1", len(items))
	}
	if items[0].Content != "stored high quality transcript" {
		t.Fatalf("Content = %q, want stored transcript", items[0].Content)
	}
	if items[0].Metadata.YouTube == nil || items[0].Metadata.YouTube.TranscriptMethod != "subtitle_vtt" {
		t.Fatalf("TranscriptMethod = %#v, want subtitle_vtt", items[0].Metadata.YouTube)
	}
	if !hasEvidence(items[0].Provenance, "stored_capture_reused", "kept=subtitle_vtt") {
		t.Fatalf("Evidence = %#v, want stored_capture_reused entry", items[0].Provenance)
	}
}

type provenanceEvidencePollingDispatcher struct {
	item types.DiscoveryItem
	raw  types.RawContent
}

func (d provenanceEvidencePollingDispatcher) SupportsFollow(kind types.Kind, platform types.Platform) bool {
	return kind == types.KindSearch && platform == types.PlatformTwitter
}

func (d provenanceEvidencePollingDispatcher) DiscoverFollowedTarget(context.Context, types.FollowTarget) ([]types.DiscoveryItem, error) {
	return []types.DiscoveryItem{d.item}, nil
}

func (d provenanceEvidencePollingDispatcher) ParseURL(_ context.Context, rawURL string) (types.ParsedURL, error) {
	return types.ParsedURL{
		Platform:     types.PlatformWeb,
		ContentType:  types.ContentTypePost,
		PlatformID:   "web-hash",
		CanonicalURL: rawURL,
	}, nil
}

func (d provenanceEvidencePollingDispatcher) FetchByParsedURL(context.Context, types.ParsedURL) ([]types.RawContent, error) {
	return nil, nil
}

func (d provenanceEvidencePollingDispatcher) FetchDiscoveryItem(context.Context, types.DiscoveryItem) ([]types.RawContent, error) {
	raw := d.raw
	raw.URL = d.item.URL
	return []types.RawContent{raw}, nil
}

type changingTranscriptDispatcher struct {
	methods []string
	call    int
}

func (changingTranscriptDispatcher) SupportsFollow(kind types.Kind, platform types.Platform) bool {
	return kind == types.KindNative && platform == types.PlatformYouTube
}

func (d *changingTranscriptDispatcher) DiscoverFollowedTarget(context.Context, types.FollowTarget) ([]types.DiscoveryItem, error) {
	return nil, nil
}

func (d *changingTranscriptDispatcher) ParseURL(_ context.Context, rawURL string) (types.ParsedURL, error) {
	return types.ParsedURL{
		Platform:     types.PlatformYouTube,
		ContentType:  types.ContentTypePost,
		PlatformID:   "yt-1",
		CanonicalURL: rawURL,
	}, nil
}

func (d *changingTranscriptDispatcher) FetchByParsedURL(_ context.Context, parsed types.ParsedURL) ([]types.RawContent, error) {
	method := "title_only"
	if d.call < len(d.methods) {
		method = d.methods[d.call]
	}
	d.call++
	return []types.RawContent{{
		Source:     string(types.PlatformYouTube),
		ExternalID: parsed.PlatformID,
		URL:        parsed.CanonicalURL,
		Content:    "fresh content " + method,
		Metadata: types.RawMetadata{YouTube: &types.YouTubeMetadata{
			Title:            "title",
			TranscriptMethod: method,
		}},
	}}, nil
}

func (d *changingTranscriptDispatcher) FetchDiscoveryItem(context.Context, types.DiscoveryItem) ([]types.RawContent, error) {
	return nil, nil
}

func TestService_PollPreservesExistingDispatcherDecisionEvidence(t *testing.T) {
	store := &fakeStore{
		processed: map[string]bool{},
		follows: []types.FollowTarget{{
			Kind:     types.KindSearch,
			Platform: "twitter",
			Locator:  "nvda",
			Query:    "nvda",
		}},
	}
	svc := New(store, provenanceEvidencePollingDispatcher{
		item: types.DiscoveryItem{
			Platform:   types.PlatformRSS,
			ExternalID: "rss-guid",
			URL:        "https://example.com/article",
		},
		raw: types.RawContent{
			Source:     "web",
			ExternalID: "web-hash",
			Content:    "article body",
			Provenance: &types.Provenance{
				Evidence: []types.ProvenanceEvidence{{
					Kind:   "discovery_identity_decision",
					Value:  "mode=retained_parsed_identity reason=stable_web_identity",
					Weight: string(types.ConfidenceHigh),
				}},
			},
		},
	}, nil)

	_, items, _, pollWarnings, err := svc.Poll(context.Background())
	if err != nil {
		t.Fatalf("Poll() error = %v", err)
	}
	if len(pollWarnings) != 0 {
		t.Fatalf("len(pollWarnings) = %d, want 0", len(pollWarnings))
	}
	if len(items) != 1 {
		t.Fatalf("len(items) = %d, want 1", len(items))
	}
	if !hasEvidence(items[0].Provenance, "discovery_identity_decision", "mode=retained_parsed_identity") {
		t.Fatalf("Evidence = %#v, want preserved dispatcher decision evidence", items[0].Provenance)
	}
}

func TestService_FetchURLStoredCaptureReuseDoesNotDriftWhenReplacedMethodChanges(t *testing.T) {
	store := &replaceStore{fakeStore: fakeStore{
		processed: map[string]bool{},
		raws: []types.RawContent{{
			Source:     "youtube",
			ExternalID: "yt-1",
			URL:        "https://www.youtube.com/watch?v=yt-1",
			Content:    "stored high quality transcript",
			Metadata: types.RawMetadata{YouTube: &types.YouTubeMetadata{
				Title:            "title",
				TranscriptMethod: "subtitle_vtt",
			}},
		}},
	}}
	dispatcher := &changingTranscriptDispatcher{methods: []string{"title_only", "whisper"}}
	svc := New(store, dispatcher, fakeEnricher{})

	first, err := svc.FetchURL(context.Background(), "https://www.youtube.com/watch?v=yt-1")
	if err != nil {
		t.Fatalf("FetchURL() first error = %v", err)
	}
	second, err := svc.FetchURL(context.Background(), "https://www.youtube.com/watch?v=yt-1")
	if err != nil {
		t.Fatalf("FetchURL() second error = %v", err)
	}

	for idx, items := range [][]types.RawContent{first, second} {
		if len(items) != 1 {
			t.Fatalf("call %d len(items) = %d, want 1", idx+1, len(items))
		}
		evidence := matchingEvidence(items[0].Provenance, "stored_capture_reused")
		if len(evidence) != 1 {
			t.Fatalf("call %d len(stored_capture_reused evidence) = %d, want 1; provenance=%#v", idx+1, len(evidence), items[0].Provenance)
		}
		if strings.Contains(evidence[0].Value, "replaced=") {
			t.Fatalf("call %d evidence value = %q, want stable dedupe key without replaced method", idx+1, evidence[0].Value)
		}
		if !strings.Contains(evidence[0].Value, "kept=subtitle_vtt") {
			t.Fatalf("call %d evidence value = %q, want kept method preserved", idx+1, evidence[0].Value)
		}
	}

	stored, err := store.GetRawCapture(context.Background(), "youtube", "yt-1")
	if err != nil {
		t.Fatalf("GetRawCapture() error = %v", err)
	}
	if evidence := matchingEvidence(stored.Provenance, "stored_capture_reused"); len(evidence) != 1 {
		t.Fatalf("stored len(stored_capture_reused evidence) = %d, want 1; provenance=%#v", len(evidence), stored.Provenance)
	}
}

func hasEvidence(prov *types.Provenance, kind, contains string) bool {
	if prov == nil {
		return false
	}
	for _, evidence := range prov.Evidence {
		if evidence.Kind == kind && strings.Contains(evidence.Value, contains) {
			return true
		}
	}
	return false
}

func matchingEvidence(prov *types.Provenance, kind string) []types.ProvenanceEvidence {
	if prov == nil {
		return nil
	}
	out := make([]types.ProvenanceEvidence, 0)
	for _, evidence := range prov.Evidence {
		if evidence.Kind == kind {
			out = append(out, evidence)
		}
	}
	return out
}
