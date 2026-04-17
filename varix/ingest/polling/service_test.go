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
	case "https://twitter.com/elonmusk":
		return types.ParsedURL{
			Platform:     types.PlatformTwitter,
			ContentType:  types.ContentTypeProfile,
			PlatformID:   "elonmusk",
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
	if len(store.polledKeys) != 1 || store.polledKeys[0] != "search:twitter:good" {
		t.Fatalf("polledKeys = %#v, want only successful target update", store.polledKeys)
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
