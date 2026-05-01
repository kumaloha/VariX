package polling

import (
	"context"
	"fmt"
	"github.com/kumaloha/VariX/varix/ingest/provenance"
	"github.com/kumaloha/VariX/varix/ingest/types"
	"strings"
)

func (s *Service) persistRawCaptures(ctx context.Context, items []types.RawContent) error {
	for _, item := range items {
		if err := s.store.UpsertRawCapture(ctx, item); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) preserveStoredCaptureQuality(ctx context.Context, items []types.RawContent) []types.RawContent {
	if !s.reuseStoredCaptureQuality {
		return items
	}
	out := make([]types.RawContent, 0, len(items))
	for _, item := range items {
		existing, err := s.store.GetRawCapture(ctx, item.Source, item.ExternalID)
		if err == nil && shouldReuseStoredCapture(existing, item) {
			item = mergeStoredCaptureQuality(existing, item)
			item.Provenance = provenance.AppendEvidence(item.Provenance, types.ProvenanceEvidence{
				Kind: "stored_capture_reused",
				Value: fmt.Sprintf(
					"source=%s external_id=%s kept=%s",
					item.Source,
					item.ExternalID,
					captureMethod(existing),
				),
				Weight: string(types.ConfidenceHigh),
			})
		}
		out = append(out, item)
	}
	return out
}

func shouldReuseStoredCapture(existing, current types.RawContent) bool {
	return captureQualityScore(existing) > captureQualityScore(current)
}

func captureQualityScore(raw types.RawContent) int {
	switch raw.Source {
	case "youtube":
		if raw.Metadata.YouTube == nil || strings.TrimSpace(raw.Content) == "" {
			return 0
		}
		switch raw.Metadata.YouTube.TranscriptMethod {
		case "subtitle_vtt":
			return 3
		case "whisper":
			return 2
		case "title_only":
			return 0
		default:
			return 1
		}
	case "bilibili":
		if raw.Metadata.Bilibili == nil || strings.TrimSpace(raw.Content) == "" {
			return 0
		}
		switch raw.Metadata.Bilibili.TranscriptMethod {
		case "whisper":
			return 2
		case "title_only":
			return 0
		default:
			return 1
		}
	default:
		return 0
	}
}

func captureMethod(raw types.RawContent) string {
	switch raw.Source {
	case "youtube":
		if raw.Metadata.YouTube != nil {
			return raw.Metadata.YouTube.TranscriptMethod
		}
	case "bilibili":
		if raw.Metadata.Bilibili != nil {
			return raw.Metadata.Bilibili.TranscriptMethod
		}
	}
	return ""
}

func mergeStoredCaptureQuality(existing, current types.RawContent) types.RawContent {
	current.Content = existing.Content
	switch current.Source {
	case "youtube":
		if current.Metadata.YouTube != nil && existing.Metadata.YouTube != nil {
			current.Metadata.YouTube.TranscriptMethod = existing.Metadata.YouTube.TranscriptMethod
			current.Metadata.YouTube.TranscriptDiagnostics = append([]types.TranscriptDiagnostic(nil), existing.Metadata.YouTube.TranscriptDiagnostics...)
		}
	case "bilibili":
		if current.Metadata.Bilibili != nil && existing.Metadata.Bilibili != nil {
			current.Metadata.Bilibili.TranscriptMethod = existing.Metadata.Bilibili.TranscriptMethod
			current.Metadata.Bilibili.TranscriptDiagnostics = append([]types.TranscriptDiagnostic(nil), existing.Metadata.Bilibili.TranscriptDiagnostics...)
		}
	}
	return current
}

func (s *Service) preserveStoredProvenance(ctx context.Context, items []types.RawContent) []types.RawContent {
	out := make([]types.RawContent, 0, len(items))
	for _, item := range items {
		existing, err := s.store.GetRawCapture(ctx, item.Source, item.ExternalID)
		if err == nil && hasResolvedSourceLookup(existing.Provenance) && shouldPreserveStoredProvenance(item.Provenance) {
			item.Provenance = mergePreservedProvenance(existing.Provenance, item.Provenance)
		}
		out = append(out, item)
	}
	return out
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

func shouldPreserveStoredProvenance(prov *types.Provenance) bool {
	if prov == nil {
		return false
	}
	if !prov.NeedsSourceLookup {
		return false
	}
	return len(prov.SourceCandidates) > 0
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

func mergePreservedProvenance(existing, current *types.Provenance) *types.Provenance {
	merged := cloneProvenance(existing)
	if current == nil {
		return merged
	}
	for _, evidence := range current.Evidence {
		merged = provenance.AppendEvidence(merged, evidence)
	}
	return merged
}
