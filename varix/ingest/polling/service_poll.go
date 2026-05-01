package polling

import (
	"context"
	"fmt"
	"github.com/kumaloha/VariX/varix/ingest/types"
	"github.com/kumaloha/VariX/varix/storage/contentstore"
	"strings"
)

func (s *Service) FetchURL(ctx context.Context, rawURL string) ([]types.RawContent, error) {
	parsed, err := s.dispatcher.ParseURL(ctx, rawURL)
	if err != nil {
		return nil, err
	}
	items, err := s.dispatcher.FetchByParsedURL(ctx, parsed)
	if err != nil {
		return nil, err
	}
	items = s.hydrateReferences(ctx, items)
	items = s.localize(ctx, items)
	items = s.annotate(items)
	items = s.preserveStoredCaptureQuality(ctx, items)
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
			rawItems = s.hydrateReferences(ctx, rawItems)
			rawItems = s.localize(ctx, rawItems)
			rawItems = s.annotate(rawItems)
			rawItems = s.preserveStoredCaptureQuality(ctx, rawItems)
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
