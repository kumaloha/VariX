package contentstore

import (
	"fmt"
	"strings"

	"github.com/kumaloha/VariX/varix/memory"
)

var defaultSubjectExperienceHorizons = []string{"1w", "1m", "1q", "1y", "2y", "5y"}

func normalizeSubjectExperienceHorizons(horizons []string) ([]string, error) {
	if len(horizons) == 0 {
		return append([]string(nil), defaultSubjectExperienceHorizons...), nil
	}
	out := make([]string, 0, len(horizons))
	seen := map[string]struct{}{}
	for _, horizon := range horizons {
		horizon = strings.TrimSpace(horizon)
		if horizon == "" {
			continue
		}
		spec, err := subjectHorizonSpecFor(horizon)
		if err != nil {
			return nil, err
		}
		if _, ok := seen[spec.Horizon]; ok {
			continue
		}
		seen[spec.Horizon] = struct{}{}
		out = append(out, spec.Horizon)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("at least one horizon is required")
	}
	return out, nil
}
func containsOrderedHorizon(horizons []string, want string) bool {
	for _, horizon := range horizons {
		if horizon == want {
			return true
		}
	}
	return false
}
func subjectExperienceAllHorizons(horizonMemories []memory.SubjectHorizonMemory) []string {
	out := make([]string, 0, len(horizonMemories))
	for _, item := range horizonMemories {
		out = append(out, item.Horizon)
	}
	return orderedUniqueStrings(out)
}
