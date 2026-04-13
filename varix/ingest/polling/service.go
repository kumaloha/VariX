package polling

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/kumaloha/VariX/varix/ingest/types"
	"github.com/kumaloha/VariX/varix/storage/contentstore"
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

type Service struct {
	store      Store
	dispatcher Dispatcher
	enricher   Enricher
	now        func() time.Time
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

func New(store Store, dispatcher Dispatcher, enricher Enricher) *Service {
	return &Service{
		store:      store,
		dispatcher: dispatcher,
		enricher:   enricher,
		now: func() time.Time {
			return time.Now().UTC()
		},
	}
}

func (s *Service) Follow(ctx context.Context, target types.FollowTarget) (types.FollowTarget, error) {
	target, err := normalizeFollowTarget(target)
	if err != nil {
		return types.FollowTarget{}, err
	}
	if target.FollowedAt.IsZero() {
		target.FollowedAt = s.now()
	}
	return target, s.store.RegisterFollow(ctx, target)
}

func (s *Service) FollowURL(ctx context.Context, rawURL string) (types.FollowTarget, error) {
	parsed, err := s.dispatcher.ParseURL(ctx, rawURL)
	if err != nil {
		return types.FollowTarget{}, err
	}

	switch {
	case parsed.Platform == types.PlatformRSS || parsed.ContentType == types.ContentTypeFeed:
		if !s.dispatcher.SupportsFollow(types.KindRSS, types.PlatformRSS) {
			return types.FollowTarget{}, fmt.Errorf("follow strategy not supported: rss/rss")
		}
		return s.Follow(ctx, types.FollowTarget{
			Kind:       types.KindRSS,
			Platform:   string(types.PlatformRSS),
			PlatformID: parsed.PlatformID,
			Locator:    parsed.CanonicalURL,
			URL:        parsed.CanonicalURL,
		})
	case parsed.ContentType == types.ContentTypeProfile:
		if !s.dispatcher.SupportsFollow(types.KindNative, parsed.Platform) {
			return types.FollowTarget{}, fmt.Errorf("follow strategy not supported: native/%s", parsed.Platform)
		}
		return s.Follow(ctx, types.FollowTarget{
			Kind:       types.KindNative,
			Platform:   string(parsed.Platform),
			PlatformID: parsed.PlatformID,
			Locator:    parsed.CanonicalURL,
			URL:        parsed.CanonicalURL,
		})
	default:
		return types.FollowTarget{}, fmt.Errorf("url is not followable: %s", rawURL)
	}
}

func (s *Service) FollowSearch(ctx context.Context, platform types.Platform, query string) (types.FollowTarget, error) {
	clean := strings.Join(strings.Fields(query), " ")
	if strings.TrimSpace(string(platform)) == "" {
		return types.FollowTarget{}, fmt.Errorf("search follow requires platform")
	}
	if !s.dispatcher.SupportsFollow(types.KindSearch, platform) {
		return types.FollowTarget{}, fmt.Errorf("follow strategy not supported: search/%s", platform)
	}
	if clean == "" {
		return types.FollowTarget{}, fmt.Errorf("search follow requires query")
	}

	return s.Follow(ctx, types.FollowTarget{
		Kind:          types.KindSearch,
		Platform:      string(platform),
		Locator:       clean,
		Query:         clean,
		HydrationHint: string(platform),
	})
}

func (s *Service) RemoveFollowURL(ctx context.Context, rawURL string) error {
	parsed, err := s.dispatcher.ParseURL(ctx, rawURL)
	if err != nil {
		return err
	}

	switch {
	case parsed.Platform == types.PlatformRSS || parsed.ContentType == types.ContentTypeFeed:
		if !s.dispatcher.SupportsFollow(types.KindRSS, types.PlatformRSS) {
			return fmt.Errorf("follow strategy not supported: rss/rss")
		}
		return s.removeFollow(ctx, types.FollowTarget{
			Kind:       types.KindRSS,
			Platform:   string(types.PlatformRSS),
			PlatformID: parsed.PlatformID,
			Locator:    parsed.CanonicalURL,
			URL:        parsed.CanonicalURL,
		})
	case parsed.ContentType == types.ContentTypeProfile:
		if !s.dispatcher.SupportsFollow(types.KindNative, parsed.Platform) {
			return fmt.Errorf("follow strategy not supported: native/%s", parsed.Platform)
		}
		return s.removeFollow(ctx, types.FollowTarget{
			Kind:       types.KindNative,
			Platform:   string(parsed.Platform),
			PlatformID: parsed.PlatformID,
			Locator:    parsed.CanonicalURL,
			URL:        parsed.CanonicalURL,
		})
	default:
		return fmt.Errorf("url is not followable: %s", rawURL)
	}
}

func (s *Service) RemoveFollowSearch(ctx context.Context, platform types.Platform, query string) error {
	clean := strings.Join(strings.Fields(query), " ")
	if strings.TrimSpace(string(platform)) == "" {
		return fmt.Errorf("search follow requires platform")
	}
	if !s.dispatcher.SupportsFollow(types.KindSearch, platform) {
		return fmt.Errorf("follow strategy not supported: search/%s", platform)
	}
	if clean == "" {
		return fmt.Errorf("search follow requires query")
	}

	return s.removeFollow(ctx, types.FollowTarget{
		Kind:          types.KindSearch,
		Platform:      string(platform),
		Locator:       clean,
		Query:         clean,
		HydrationHint: string(platform),
	})
}

func (s *Service) ListFollows(ctx context.Context) ([]types.FollowTarget, []contentstore.ScanWarning, error) {
	return s.store.ListFollows(ctx)
}

func (s *Service) FetchURL(ctx context.Context, rawURL string) ([]types.RawContent, error) {
	parsed, err := s.dispatcher.ParseURL(ctx, rawURL)
	if err != nil {
		return nil, err
	}
	items, err := s.dispatcher.FetchByParsedURL(ctx, parsed)
	if err != nil {
		return nil, err
	}
	items = s.annotate(items)
	items = s.preserveStoredProvenance(ctx, items)
	if err := s.persistRawCaptures(ctx, items); err != nil {
		return nil, err
	}
	if err := s.markProcessed(ctx, items); err != nil {
		return nil, err
	}
	return items, nil
}

func (s *Service) Poll(ctx context.Context) (types.PollReport, []types.RawContent, []contentstore.ScanWarning, []PollWarning, error) {
	targets, warnings, err := s.store.ListFollows(ctx)
	if err != nil {
		return types.PollReport{}, nil, nil, nil, err
	}

	report := types.PollReport{
		StartedAt:         s.now(),
		TargetCount:       len(targets),
		StoreWarningCount: len(warnings),
		Targets:           make([]types.TargetPollReport, 0, len(targets)),
	}
	out := make([]types.RawContent, 0)
	pollWarnings := make([]PollWarning, 0)
	for _, target := range targets {
		targetKey := followTargetKey(target)
		targetReport := types.TargetPollReport{
			Target: targetKey,
			Status: "ok",
		}
		items, err := s.dispatcher.DiscoverFollowedTarget(ctx, target)
		if err != nil {
			pollWarnings = append(pollWarnings, PollWarning{
				Kind:   WarningKindDiscover,
				Target: targetKey,
				Detail: err.Error(),
			})
			targetReport.WarningCount++
			targetReport.Status = "warning"
			targetReport.ErrorDetail = err.Error()
			report.Targets = append(report.Targets, targetReport)
			continue
		}
		targetReport.DiscoveredCount = len(items)
		report.DiscoveredCount += len(items)
		for _, item := range items {
			platform, externalID, err := s.discoveryIdentity(ctx, item)
			if err != nil {
				pollWarnings = append(pollWarnings, PollWarning{
					Kind:    WarningKindIdentity,
					Target:  targetKey,
					ItemURL: item.URL,
					Detail:  err.Error(),
				})
				targetReport.WarningCount++
				targetReport.Status = "warning"
				if targetReport.ErrorDetail == "" {
					targetReport.ErrorDetail = err.Error()
				}
				continue
			}
			seen, err := s.store.IsProcessed(ctx, platform, externalID)
			if err != nil {
				pollWarnings = append(pollWarnings, PollWarning{
					Kind:    WarningKindProcessedCheck,
					Target:  targetKey,
					ItemURL: item.URL,
					Detail:  err.Error(),
				})
				targetReport.WarningCount++
				targetReport.Status = "warning"
				if targetReport.ErrorDetail == "" {
					targetReport.ErrorDetail = err.Error()
				}
				continue
			}
			if seen {
				targetReport.SkippedCount++
				report.SkippedCount++
				continue
			}

			rawItems, err := s.dispatcher.FetchDiscoveryItem(ctx, item)
			if err != nil {
				pollWarnings = append(pollWarnings, PollWarning{
					Kind:    WarningKindHydrate,
					Target:  targetKey,
					ItemURL: item.URL,
					Detail:  err.Error(),
				})
				targetReport.WarningCount++
				targetReport.Status = "warning"
				if targetReport.ErrorDetail == "" {
					targetReport.ErrorDetail = err.Error()
				}
				continue
			}
			rawItems = s.annotate(rawItems)
			// When a source returns empty results without error (e.g., Twitter 404/tombstone),
			// mark the discovery identity as processed to avoid retrying permanently gone items.
			// Network errors and rate limits return as errors, not (nil, nil).
			if len(rawItems) == 0 {
				if err := s.store.MarkProcessed(ctx, types.ProcessedRecord{
					Platform:    platform,
					ExternalID:  externalID,
					URL:         item.URL,
					ProcessedAt: s.now(),
				}); err != nil {
					pollWarnings = append(pollWarnings, PollWarning{
						Kind:    WarningKindMarkProcessed,
						Target:  targetKey,
						ItemURL: item.URL,
						Detail:  err.Error(),
					})
					targetReport.WarningCount++
					targetReport.Status = "warning"
					if targetReport.ErrorDetail == "" {
						targetReport.ErrorDetail = err.Error()
					}
				}
				continue
			}
			if err := s.persistRawCaptures(ctx, rawItems); err != nil {
				pollWarnings = append(pollWarnings, PollWarning{
					Kind:    WarningKindRawCapture,
					Target:  targetKey,
					ItemURL: item.URL,
					Detail:  err.Error(),
				})
				targetReport.WarningCount++
				targetReport.Status = "warning"
				if targetReport.ErrorDetail == "" {
					targetReport.ErrorDetail = err.Error()
				}
				continue
			}
			if err := s.markProcessed(ctx, rawItems); err != nil {
				pollWarnings = append(pollWarnings, PollWarning{
					Kind:    WarningKindMarkProcessed,
					Target:  targetKey,
					ItemURL: item.URL,
					Detail:  err.Error(),
				})
				targetReport.WarningCount++
				targetReport.Status = "warning"
				if targetReport.ErrorDetail == "" {
					targetReport.ErrorDetail = err.Error()
				}
				continue
			}
			targetReport.FetchedCount += len(rawItems)
			report.FetchedCount += len(rawItems)
			out = append(out, rawItems...)
		}

		if err := s.store.UpdateFollowPolled(ctx, target.Kind, target.Platform, target.Locator, s.now()); err != nil {
			pollWarnings = append(pollWarnings, PollWarning{
				Kind:   WarningKindUpdateFollow,
				Target: targetKey,
				Detail: err.Error(),
			})
			targetReport.WarningCount++
			targetReport.Status = "warning"
			if targetReport.ErrorDetail == "" {
				targetReport.ErrorDetail = err.Error()
			}
		}
		report.Targets = append(report.Targets, targetReport)
	}

	report.FinishedAt = s.now()
	report.PollWarningCount = len(pollWarnings)
	if err := s.store.RecordPollReport(ctx, report); err != nil {
		pollWarnings = append(pollWarnings, PollWarning{
			Kind:   WarningKindRecordReport,
			Target: "poll",
			Detail: err.Error(),
		})
		report.PollWarningCount = len(pollWarnings)
	}
	return report, out, warnings, pollWarnings, nil
}

func (s *Service) persistRawCaptures(ctx context.Context, items []types.RawContent) error {
	for _, item := range items {
		if err := s.store.UpsertRawCapture(ctx, item); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) preserveStoredProvenance(ctx context.Context, items []types.RawContent) []types.RawContent {
	out := make([]types.RawContent, 0, len(items))
	for _, item := range items {
		existing, err := s.store.GetRawCapture(ctx, item.Source, item.ExternalID)
		if err == nil && hasResolvedSourceLookup(existing.Provenance) {
			item.Provenance = cloneProvenance(existing.Provenance)
		}
		out = append(out, item)
	}
	return out
}

func (s *Service) annotate(items []types.RawContent) []types.RawContent {
	if s.enricher == nil {
		return items
	}
	return s.enricher.Annotate(items)
}

func (s *Service) discoveryIdentity(ctx context.Context, item types.DiscoveryItem) (string, string, error) {
	if item.Platform != "" && item.Platform != types.PlatformRSS && item.ExternalID != "" {
		return string(item.Platform), item.ExternalID, nil
	}
	if strings.TrimSpace(item.URL) != "" {
		parsed, err := s.dispatcher.ParseURL(ctx, item.URL)
		if err == nil && parsed.ContentType == types.ContentTypePost && parsed.PlatformID != "" {
			return string(parsed.Platform), parsed.PlatformID, nil
		}
	}
	if item.Platform != "" && item.ExternalID != "" {
		return string(item.Platform), item.ExternalID, nil
	}
	return "", "", fmt.Errorf("discovery item has no dedupe identity: %+v", item)
}

func (s *Service) markProcessed(ctx context.Context, items []types.RawContent) error {
	for _, raw := range items {
		if err := s.store.MarkProcessed(ctx, types.ProcessedRecord{
			Platform:    raw.Source,
			ExternalID:  raw.ExternalID,
			URL:         raw.URL,
			Author:      raw.AuthorName,
			ProcessedAt: s.now(),
		}); err != nil {
			return err
		}
	}
	return nil
}

func hasResolvedSourceLookup(prov *types.Provenance) bool {
	if prov == nil {
		return false
	}
	switch prov.SourceLookup.Status {
	case types.SourceLookupStatusFound, types.SourceLookupStatusNotFound, types.SourceLookupStatusFailed:
		return true
	default:
		return false
	}
}

func cloneProvenance(prov *types.Provenance) *types.Provenance {
	if prov == nil {
		return nil
	}
	copyProv := *prov
	if len(prov.ClaimedSpeakers) > 0 {
		copyProv.ClaimedSpeakers = append([]string(nil), prov.ClaimedSpeakers...)
	}
	if len(prov.SourceCandidates) > 0 {
		copyProv.SourceCandidates = append([]types.SourceCandidate(nil), prov.SourceCandidates...)
	}
	if len(prov.Evidence) > 0 {
		copyProv.Evidence = append([]types.ProvenanceEvidence(nil), prov.Evidence...)
	}
	return &copyProv
}

func (s *Service) removeFollow(ctx context.Context, target types.FollowTarget) error {
	target, err := normalizeFollowTarget(target)
	if err != nil {
		return err
	}
	return s.store.RemoveFollow(ctx, target.Kind, target.Platform, target.Locator)
}

func normalizeFollowTarget(target types.FollowTarget) (types.FollowTarget, error) {
	target.Platform = strings.TrimSpace(target.Platform)
	target.PlatformID = strings.TrimSpace(target.PlatformID)
	target.URL = strings.TrimSpace(target.URL)
	target.Query = strings.Join(strings.Fields(target.Query), " ")
	target.Locator = strings.Join(strings.Fields(target.Locator), " ")
	target.HydrationHint = strings.TrimSpace(target.HydrationHint)

	switch target.Kind {
	case types.KindRSS:
		target.Platform = string(types.PlatformRSS)
		if target.Locator == "" {
			target.Locator = target.URL
		}
		if target.URL == "" {
			target.URL = target.Locator
		}
	case types.KindNative:
		if target.Locator == "" {
			target.Locator = target.URL
		}
	case types.KindSearch:
		if target.Query == "" {
			target.Query = target.Locator
		}
		if target.Locator == "" {
			target.Locator = target.Query
		}
		if target.HydrationHint == "" {
			target.HydrationHint = target.Platform
		}
	default:
		return types.FollowTarget{}, fmt.Errorf("unsupported follow kind: %s", target.Kind)
	}

	if strings.TrimSpace(string(target.Kind)) == "" || target.Platform == "" || target.Locator == "" {
		return types.FollowTarget{}, fmt.Errorf("invalid follow target")
	}
	return target, nil
}

func followTargetKey(target types.FollowTarget) string {
	return string(target.Kind) + ":" + target.Platform + ":" + target.Locator
}
