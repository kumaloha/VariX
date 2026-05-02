package contentstore

import (
	"encoding/json"
	"sort"
	"strings"
	"time"
)

func marshalJSONStringSlice(values []string) (string, error) {
	normalized := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		normalized = append(normalized, value)
	}
	payload, err := json.Marshal(normalized)
	if err != nil {
		return "", err
	}
	return string(payload), nil
}

func marshalLifecycleHistory(mergeHistory, splitHistory []string) (string, string, error) {
	mergeJSON, err := marshalJSONStringSlice(mergeHistory)
	if err != nil {
		return "", "", err
	}
	splitJSON, err := marshalJSONStringSlice(splitHistory)
	if err != nil {
		return "", "", err
	}
	return mergeJSON, splitJSON, nil
}

func normalizeCreatedUpdatedTimes(createdAt, updatedAt *time.Time) {
	now := normalizeNow(time.Time{})
	if createdAt != nil && createdAt.IsZero() {
		*createdAt = now
	}
	if createdAt == nil || updatedAt == nil || !updatedAt.IsZero() {
		return
	}
	*updatedAt = maxTime(*createdAt, now)
}

func unmarshalJSONStringSlice(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	var out []string
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return nil
	}
	return out
}

func unmarshalOptionalJSONStringSlice(raw string) []string {
	out := unmarshalJSONStringSlice(raw)
	if len(out) == 0 {
		return nil
	}
	return out
}

func normalizeCanonicalAliases(canonicalName string, aliases []string) []string {
	set := map[string]struct{}{}
	out := make([]string, 0, len(aliases)+1)
	for _, alias := range append([]string{canonicalName}, aliases...) {
		normalized := normalizeCanonicalAlias(alias)
		if normalized == "" {
			continue
		}
		if _, ok := set[normalized]; ok {
			continue
		}
		set[normalized] = struct{}{}
		out = append(out, normalized)
	}
	sort.Strings(out)
	return out
}

func normalizeCanonicalAlias(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return ""
	}
	return strings.Join(strings.Fields(value), " ")
}

func normalizeCanonicalDisplay(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	return strings.Join(strings.Fields(value), " ")
}

func nullIfBlank(value string) any {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	return value
}

func maxTime(left, right time.Time) time.Time {
	if right.After(left) {
		return right
	}
	return left
}
