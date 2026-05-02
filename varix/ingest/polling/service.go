package polling

import (
	"context"
	"github.com/kumaloha/VariX/varix/ingest/types"
	"github.com/kumaloha/VariX/varix/storage/contentstore"
	"time"
)

type Store interface {
	IsProcessed(ctx context.Context, platform, externalID string) (bool, error)
	MarkProcessed(ctx context.Context, record types.ProcessedRecord) error
	UpsertRawCapture(ctx context.Context, raw types.RawContent) error
	GetRawCapture(ctx context.Context, platform, externalID string) (types.RawContent, error)
	ListPendingSourceLookups(ctx context.Context, limit int) ([]types.RawContent, error)
	MarkSourceLookupResult(ctx context.Context, raw types.RawContent, status types.SourceLookupStatus, errDetail string) error
	RegisterFollow(ctx context.Context, target types.FollowTarget) error
	ListFollows(ctx context.Context) ([]types.FollowTarget, []contentstore.ScanWarning, error)
	RemoveFollow(ctx context.Context, kind types.Kind, platform string, locator string) error
	UpdateFollowPolled(ctx context.Context, kind types.Kind, platform string, locator string, at time.Time) error
	RecordPollReport(ctx context.Context, report types.PollReport) error
	RegisterAuthorSubscription(ctx context.Context, sub types.AuthorSubscription, queries []types.SubscriptionQuery) (types.AuthorSubscription, error)
	ListAuthorSubscriptions(ctx context.Context) ([]types.AuthorSubscription, []contentstore.ScanWarning, error)
}

type Dispatcher interface {
	SupportsFollow(kind types.Kind, platform types.Platform) bool
	ParseURL(ctx context.Context, rawURL string) (types.ParsedURL, error)
	DiscoverFollowedTarget(ctx context.Context, target types.FollowTarget) ([]types.DiscoveryItem, error)
	FetchByParsedURL(ctx context.Context, parsed types.ParsedURL) ([]types.RawContent, error)
	FetchDiscoveryItem(ctx context.Context, item types.DiscoveryItem) ([]types.RawContent, error)
}

type Enricher interface {
	Annotate(items []types.RawContent) []types.RawContent
}

type AttachmentLocalizer interface {
	Localize(ctx context.Context, items []types.RawContent) []types.RawContent
}

type Service struct {
	store                     Store
	dispatcher                Dispatcher
	enricher                  Enricher
	localizer                 AttachmentLocalizer
	now                       func() time.Time
	reuseStoredCaptureQuality bool
}

type WarningKind string

const (
	WarningKindDiscover       WarningKind = "discover_error"
	WarningKindIdentity       WarningKind = "identity_error"
	WarningKindProcessedCheck WarningKind = "processed_check_error"
	WarningKindHydrate        WarningKind = "hydrate_error"
	WarningKindRawCapture     WarningKind = "raw_capture_error"
	WarningKindMarkProcessed  WarningKind = "mark_processed_error"
	WarningKindUpdateFollow   WarningKind = "update_follow_error"
	WarningKindRecordReport   WarningKind = "record_report_error"
)

type PollWarning struct {
	Kind    WarningKind `json:"kind"`
	Target  string      `json:"target"`
	ItemURL string      `json:"item_url,omitempty"`
	Detail  string      `json:"detail"`
}

type Option func(*Service)

func WithStoredCaptureReuse(enabled bool) Option {
	return func(s *Service) {
		s.reuseStoredCaptureQuality = enabled
	}
}

func WithAttachmentLocalizer(localizer AttachmentLocalizer) Option {
	return func(s *Service) {
		s.localizer = localizer
	}
}

func New(store Store, dispatcher Dispatcher, enricher Enricher, opts ...Option) *Service {
	svc := &Service{
		store:      store,
		dispatcher: dispatcher,
		enricher:   enricher,
		now: func() time.Time {
			return time.Now().UTC()
		},
		reuseStoredCaptureQuality: true,
	}
	for _, opt := range opts {
		if opt != nil {
			opt(svc)
		}
	}
	return svc
}
