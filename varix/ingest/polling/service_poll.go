package polling

import (
	"context"
	"fmt"
	"hash/fnv"
	"strings"
	"time"

	"github.com/kumaloha/VariX/varix/ingest/types"
	"github.com/kumaloha/VariX/varix/storage/contentstore"
)

const (
	pollSchedulerSlot    = 15 * time.Minute
	searchFollowCadence  = 3 * time.Hour
	defaultFollowCadence = 15 * time.Minute
)

type FetchURLAndFollowAuthorResult struct {
	Items  []types.RawContent        `json:"items"`
	Author *types.AuthorFollowResult `json:"author,omitempty"`
}

type FollowSchedule struct {
	Cadence    time.Duration `json:"cadence"`
	Due        bool          `json:"due"`
	NextPollAt time.Time     `json:"next_poll_at"`
	SlotIndex  int           `json:"slot_index,omitempty"`
	SlotCount  int           `json:"slot_count,omitempty"`
}

func (s *Service) FetchURL(ctx context.Context, rawURL string) ([]types.RawContent, error) {
	parsed, err := s.dispatcher.ParseURL(ctx, rawURL)
	if err != nil {
		return nil, err
	}
	return s.fetchParsedURL(ctx, parsed)
}

func (s *Service) FetchURLAndFollowAuthor(ctx context.Context, rawURL string) (FetchURLAndFollowAuthorResult, error) {
	parsed, err := s.dispatcher.ParseURL(ctx, rawURL)
	if err != nil {
		return FetchURLAndFollowAuthorResult{}, err
	}
	items, err := s.fetchParsedURL(ctx, parsed)
	if err != nil {
		return FetchURLAndFollowAuthorResult{}, err
	}
	result := FetchURLAndFollowAuthorResult{Items: items}
	req, ok := authorFollowRequestFromParsedURL(parsed)
	if !ok {
		return result, nil
	}
	author, err := s.FollowAuthor(ctx, req)
	if err != nil {
		return FetchURLAndFollowAuthorResult{}, err
	}
	result.Author = &author
	return result, nil
}

func (s *Service) fetchParsedURL(ctx context.Context, parsed types.ParsedURL) ([]types.RawContent, error) {
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

func authorFollowRequestFromParsedURL(parsed types.ParsedURL) (types.AuthorFollowRequest, bool) {
	switch {
	case parsed.ContentType == types.ContentTypeProfile:
		return types.AuthorFollowRequest{
			Platform:   parsed.Platform,
			PlatformID: parsed.PlatformID,
			ProfileURL: parsed.CanonicalURL,
		}, parsed.Platform != "" && parsed.PlatformID != ""
	case parsed.Platform == types.PlatformTwitter && parsed.ContentType == types.ContentTypePost && strings.TrimSpace(parsed.AuthorID) != "":
		authorID := strings.TrimPrefix(strings.TrimSpace(parsed.AuthorID), "@")
		return types.AuthorFollowRequest{
			Platform:   types.PlatformTwitter,
			PlatformID: authorID,
			ProfileURL: "https://twitter.com/" + authorID,
		}, true
	default:
		return types.AuthorFollowRequest{}, false
	}
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
		if !isFollowDue(target, report.StartedAt) {
			continue
		}
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
			if updateErr := s.store.UpdateFollowPolled(ctx, target.Kind, target.Platform, target.Locator, s.now()); updateErr != nil {
				pollWarnings = append(pollWarnings, PollWarning{
					Kind:   WarningKindUpdateFollow,
					Target: targetKey,
					Detail: updateErr.Error(),
				})
			}
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

func isFollowDue(target types.FollowTarget, now time.Time) bool {
	return ScheduleForFollow(target, now).Due
}

func ScheduleForFollow(target types.FollowTarget, now time.Time) FollowSchedule {
	if now.IsZero() {
		now = time.Now().UTC()
	}
	now = now.UTC()
	cadence := followCadence(target)
	schedule := FollowSchedule{
		Cadence: cadence,
	}
	if target.LastPolledAt.IsZero() {
		schedule.Due = true
		schedule.NextPollAt = now
		fillSlotFields(target, &schedule)
		return schedule
	}
	if cadence <= 0 {
		schedule.Due = true
		schedule.NextPollAt = now
		return schedule
	}
	if cadence <= pollSchedulerSlot {
		next := target.LastPolledAt.Add(cadence).UTC()
		if !now.Before(next) {
			schedule.Due = true
			schedule.NextPollAt = now
			return schedule
		}
		schedule.NextPollAt = next
		return schedule
	}
	slotCount := int(cadence / pollSchedulerSlot)
	if slotCount <= 1 {
		next := target.LastPolledAt.Add(cadence).UTC()
		if !now.Before(next) {
			schedule.Due = true
			schedule.NextPollAt = now
			return schedule
		}
		schedule.NextPollAt = next
		return schedule
	}
	schedule.SlotIndex = assignedPollSlot(target, slotCount)
	schedule.SlotCount = slotCount
	slotStart := currentSlotStart(now)
	if currentPollSlot(now, slotCount) == schedule.SlotIndex && target.LastPolledAt.Before(slotStart) {
		schedule.Due = true
		schedule.NextPollAt = now
		return schedule
	}
	schedule.NextPollAt = nextAssignedPollSlotAfter(target, now, slotCount)
	return schedule
}

func followCadence(target types.FollowTarget) time.Duration {
	if target.Kind == types.KindSearch {
		return searchFollowCadence
	}
	return defaultFollowCadence
}

func currentPollSlot(now time.Time, slotCount int) int {
	minutes := now.UTC().Hour()*60 + now.UTC().Minute()
	return (minutes / int(pollSchedulerSlot/time.Minute)) % slotCount
}

func currentSlotStart(now time.Time) time.Time {
	now = now.UTC()
	slotMinutes := int(pollSchedulerSlot / time.Minute)
	minute := (now.Minute() / slotMinutes) * slotMinutes
	return time.Date(now.Year(), now.Month(), now.Day(), now.Hour(), minute, 0, 0, time.UTC)
}

func assignedPollSlot(target types.FollowTarget, slotCount int) int {
	if slotCount <= 1 {
		return 0
	}
	hash := fnv.New32a()
	_, _ = hash.Write([]byte(followTargetKey(target)))
	return int(hash.Sum32() % uint32(slotCount))
}

func nextAssignedPollSlotAfter(target types.FollowTarget, now time.Time, slotCount int) time.Time {
	start := currentSlotStart(now)
	for i := 0; i <= slotCount*2; i++ {
		candidate := start.Add(time.Duration(i) * pollSchedulerSlot)
		if currentPollSlot(candidate, slotCount) != assignedPollSlot(target, slotCount) {
			continue
		}
		if target.LastPolledAt.Before(currentSlotStart(candidate)) {
			return candidate
		}
	}
	return start.Add(time.Duration(slotCount) * pollSchedulerSlot)
}

func fillSlotFields(target types.FollowTarget, schedule *FollowSchedule) {
	if schedule == nil || schedule.Cadence <= pollSchedulerSlot {
		return
	}
	slotCount := int(schedule.Cadence / pollSchedulerSlot)
	if slotCount <= 1 {
		return
	}
	schedule.SlotIndex = assignedPollSlot(target, slotCount)
	schedule.SlotCount = slotCount
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
