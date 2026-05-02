package main

import (
	"strings"
)

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func truncate(value string, max int) string {
	value = strings.TrimSpace(value)
	runes := []rune(value)
	if len(runes) <= max {
		return value
	}
	return string(runes[:max]) + "…"
}

func truncateList(values []string, max int) []string {
	out := make([]string, 0, max)
	for _, value := range values {
		if strings.TrimSpace(value) == "" {
			continue
		}
		out = append(out, truncate(value, 100))
		if len(out) == max {
			break
		}
	}
	return out
}
