package contentstore

import (
	"fmt"
	"strings"
	"time"

	"github.com/kumaloha/VariX/varix/ingest/types"
)

type WarningKind string

const (
	WarningKindCorruptJSON   WarningKind = "corrupt_json"
	WarningKindInvalidRecord WarningKind = "invalid_record"
)

type ScanWarning struct {
	Path   string      `json:"path"`
	Kind   WarningKind `json:"kind"`
	Detail string      `json:"detail"`
}

func isValidProcessedRecord(record types.ProcessedRecord) bool {
	return strings.TrimSpace(record.Platform) != "" &&
		strings.TrimSpace(record.ExternalID) != ""
}

func isValidFollowTarget(target types.FollowTarget) bool {
	return strings.TrimSpace(string(target.Kind)) != "" &&
		strings.TrimSpace(target.Platform) != "" &&
		strings.TrimSpace(target.Locator) != ""
}

func normalizeRequiredUserID(userID string) (string, error) {
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return "", fmt.Errorf("user id is required")
	}
	return userID, nil
}

func normalizeNow(now time.Time) time.Time {
	if now.IsZero() {
		return time.Now().UTC()
	}
	return now
}
