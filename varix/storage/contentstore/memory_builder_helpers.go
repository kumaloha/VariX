package contentstore

import (
	"strings"
	"time"

	"github.com/kumaloha/VariX/varix/memory"
)

func cloneStringSlice(values []string) []string {
	return append([]string(nil), values...)
}

func buildCardOrConclusionSubheadline(summary string) string {
	return strings.TrimSpace(summary)
}

func newTopMemoryItem(itemID string, itemType memory.TopMemoryItemType, headline, subheadline, backingObjectID string, signal memory.SignalStrength, updatedAt time.Time) memory.TopMemoryItem {
	return memory.TopMemoryItem{
		ItemID:          itemID,
		ItemType:        itemType,
		Headline:        headline,
		Subheadline:     subheadline,
		BackingObjectID: backingObjectID,
		SignalStrength:  signal,
		UpdatedAt:       updatedAt,
	}
}

func newTopMemoryItemWithAsOf(itemID string, itemType memory.TopMemoryItemType, headline, subheadline, backingObjectID string, signal memory.SignalStrength, asOf, updatedAt time.Time) memory.TopMemoryItem {
	item := newTopMemoryItem(itemID, itemType, headline, subheadline, backingObjectID, signal, updatedAt)
	item.AsOf = asOf
	return item
}

func newCognitiveConclusion(conclusionID, sourceType, sourceID, headline, subheadline string, traceabilityStatus memory.TraceabilityStatus, asOf, createdAt time.Time) memory.CognitiveConclusion {
	return memory.CognitiveConclusion{
		ConclusionID:       conclusionID,
		SourceType:         sourceType,
		SourceID:           sourceID,
		Headline:           headline,
		Subheadline:        subheadline,
		TraceabilityStatus: traceabilityStatus,
		AsOf:               asOf,
		CreatedAt:          createdAt,
	}
}
