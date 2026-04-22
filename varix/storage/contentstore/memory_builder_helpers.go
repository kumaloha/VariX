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
