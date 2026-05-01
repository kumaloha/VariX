package contentstore

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/kumaloha/VariX/varix/memory"
)

func (s *SQLiteStore) GetSubjectExperienceMemory(ctx context.Context, userID, subject string, horizons []string, now time.Time, refresh bool) (memory.SubjectExperienceMemory, error) {
	return s.getSubjectExperienceMemoryWithHorizonInputs(ctx, userID, subject, horizons, now, refresh, nil)
}
func (s *SQLiteStore) getSubjectExperienceMemoryWithHorizonInputs(ctx context.Context, userID, subject string, horizons []string, now time.Time, refresh bool, preloaded map[string]memory.SubjectHorizonMemory) (memory.SubjectExperienceMemory, error) {
	userID, err := normalizeRequiredUserID(userID)
	if err != nil {
		return memory.SubjectExperienceMemory{}, err
	}
	subject = strings.TrimSpace(subject)
	if subject == "" {
		return memory.SubjectExperienceMemory{}, fmt.Errorf("subject is required")
	}
	horizons, err = normalizeSubjectExperienceHorizons(horizons)
	if err != nil {
		return memory.SubjectExperienceMemory{}, err
	}
	now = normalizeNow(now)
	canonicalSubject, err := s.resolveCanonicalListSubject(ctx, subject)
	if err != nil {
		return memory.SubjectExperienceMemory{}, err
	}
	horizonMemories := make([]memory.SubjectHorizonMemory, 0, len(horizons))
	for _, horizon := range horizons {
		if item, ok := preloaded[strings.TrimSpace(horizon)]; ok {
			horizonMemories = append(horizonMemories, item)
			continue
		}
		item, err := s.GetSubjectHorizonMemory(ctx, userID, subject, horizon, now, refresh)
		if err != nil {
			return memory.SubjectExperienceMemory{}, err
		}
		horizonMemories = append(horizonMemories, item)
	}
	inputHash := subjectExperienceInputHash(horizonMemories)
	horizonSet := strings.Join(horizons, ",")
	if !refresh {
		cached, ok, err := s.getCachedSubjectExperienceMemory(ctx, userID, canonicalSubject, horizonSet, inputHash)
		if err != nil {
			return memory.SubjectExperienceMemory{}, err
		}
		if ok {
			return cached, nil
		}
	}
	out := buildSubjectExperienceMemory(userID, subject, canonicalSubject, horizons, horizonMemories, inputHash, now)
	if err := s.upsertSubjectExperienceMemory(ctx, horizonSet, out); err != nil {
		return memory.SubjectExperienceMemory{}, err
	}
	return out, nil
}
