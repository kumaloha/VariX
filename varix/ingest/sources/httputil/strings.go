package httputil

import "strings"

// FirstString returns the first non-empty string-like value after TrimSpace.
// Supports string and fmt.Stringer inputs used across ingest collectors.
func FirstString(values ...any) string {
	for _, value := range values {
		switch typed := value.(type) {
		case string:
			if trimmed := strings.TrimSpace(typed); trimmed != "" {
				return trimmed
			}
		case interface{ String() string }:
			if trimmed := strings.TrimSpace(typed.String()); trimmed != "" {
				return trimmed
			}
		}
	}
	return ""
}
