package main

import (
	"strings"
)

func invalidScopedMemorySourceRequest(userID, platform, externalID string) bool {
	return strings.TrimSpace(userID) == "" || (strings.TrimSpace(externalID) != "" && strings.TrimSpace(platform) == "")
}

func invalidRequiredMemorySource(userID, platform, externalID string) bool {
	return strings.TrimSpace(userID) == "" || !hasContentTarget(platform, externalID)
}

func uniqueStringSlice(in []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(in))
	for _, item := range in {
		if _, ok := seen[item]; ok {
			continue
		}
		seen[item] = struct{}{}
		out = append(out, item)
	}
	return out
}

func uniqueCLIStrings(items []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(items))
	for _, item := range items {
		if _, ok := seen[item]; ok {
			continue
		}
		seen[item] = struct{}{}
		out = append(out, item)
	}
	return out
}
