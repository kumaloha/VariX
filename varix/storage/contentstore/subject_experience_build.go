package contentstore

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/kumaloha/VariX/varix/memory"
)

func buildSubjectExperienceMemory(userID, subject, canonicalSubject string, horizons []string, horizonMemories []memory.SubjectHorizonMemory, inputHash string, now time.Time) memory.SubjectExperienceMemory {
	generatedAt := now.UTC().Format(time.RFC3339)
	out := memory.SubjectExperienceMemory{
		UserID:           userID,
		Subject:          subject,
		CanonicalSubject: canonicalSubject,
		Horizons:         append([]string(nil), horizons...),
		GeneratedAt:      generatedAt,
		CacheStatus:      "refreshed",
		InputHash:        inputHash,
	}
	out.HorizonSummaries = subjectExperienceHorizonSummaries(horizonMemories)
	out.EvidenceSourceRefs = subjectExperienceEvidenceRefs(horizonMemories)
	out.Lessons = deriveSubjectExperienceLessons(firstTrimmed(canonicalSubject, subject), horizonMemories)
	out.AttributionSummary = buildSubjectAttributionSummary(horizonMemories)
	out.LessonCount = len(out.Lessons)
	out.Abstraction = summarizeSubjectExperience(out)
	return out
}
func subjectExperienceHorizonSummaries(horizonMemories []memory.SubjectHorizonMemory) []memory.SubjectExperienceHorizon {
	out := make([]memory.SubjectExperienceHorizon, 0, len(horizonMemories))
	for _, item := range horizonMemories {
		topDrivers := make([]string, 0, 3)
		for i, driver := range item.DriverClusters {
			if i >= 3 {
				break
			}
			topDrivers = append(topDrivers, driver.Subject)
		}
		out = append(out, memory.SubjectExperienceHorizon{
			Horizon:         item.Horizon,
			SampleCount:     item.SampleCount,
			TrendDirection:  item.TrendDirection,
			VolatilityState: item.VolatilityState,
			TopDrivers:      topDrivers,
		})
	}
	return out
}
func subjectExperienceEvidenceRefs(horizonMemories []memory.SubjectHorizonMemory) []string {
	refs := make([]string, 0)
	for _, item := range horizonMemories {
		refs = append(refs, item.EvidenceSourceRefs...)
	}
	return uniqueStrings(refs)
}
func summarizeSubjectExperience(out memory.SubjectExperienceMemory) string {
	subject := firstTrimmed(out.CanonicalSubject, out.Subject, "subject")
	if len(out.Lessons) == 0 {
		return fmt.Sprintf("%s 在 %s 时间尺度下还没有可复用经验。", subject, strings.Join(out.Horizons, "/"))
	}
	top := out.Lessons[0]
	return fmt.Sprintf("%s 在 %s 时间尺度下形成了 %d 条可复用经验；最强经验：%s", subject, strings.Join(out.Horizons, "/"), len(out.Lessons), top.Statement)
}
func subjectExperienceInputHash(horizonMemories []memory.SubjectHorizonMemory) string {
	inputs := make([]struct {
		Horizon   string
		InputHash string
		WindowEnd string
	}, 0, len(horizonMemories))
	for _, item := range horizonMemories {
		inputs = append(inputs, struct {
			Horizon   string
			InputHash string
			WindowEnd string
		}{item.Horizon, item.InputHash, item.WindowEnd})
	}
	payload, _ := json.Marshal(inputs)
	sum := sha256.Sum256(payload)
	return hex.EncodeToString(sum[:])
}
