package contentstore

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/kumaloha/VariX/varix/memory"
	"strings"
)

func summarizeSubjectHorizonPattern(changes []memory.SubjectHorizonChange) (trend, volatility, pattern string) {
	up, down := 0, 0
	for _, change := range changes {
		text := normalizeSubjectChangeText(change.ChangeText)
		if containsAny(text, "上涨", "创新高", "走强", "反弹", "上升") {
			up++
		}
		if containsAny(text, "下跌", "回落", "承压", "走弱", "下降") {
			down++
		}
	}
	switch {
	case up > 0 && down > 0:
		trend = "mixed"
	case up > 0:
		trend = "up"
	case down > 0:
		trend = "down"
	default:
		trend = "unclear"
	}
	if up > 0 && down > 0 || len(changes) >= 3 {
		volatility = "active"
	} else {
		volatility = "quiet"
	}
	if len(changes) == 0 {
		return trend, volatility, "no saved changes in window"
	}
	return trend, volatility, fmt.Sprintf("%d changes over the %s trend window", len(changes), trend)
}

func summarizeSubjectHorizonAbstraction(out memory.SubjectHorizonMemory) string {
	subject := firstTrimmed(out.CanonicalSubject, out.Subject, "subject")
	if len(out.KeyChanges) == 0 {
		return fmt.Sprintf("%s has no saved changes in the %s horizon.", subject, out.Horizon)
	}
	latest := out.KeyChanges[len(out.KeyChanges)-1].ChangeText
	if len(out.DriverClusters) == 0 {
		return fmt.Sprintf("%s %s horizon has %d saved changes; latest: %s.", subject, out.Horizon, len(out.KeyChanges), latest)
	}
	topDriver := out.DriverClusters[0].Subject
	return fmt.Sprintf("%s %s horizon has %d saved changes; latest: %s. Top driver: %s.", subject, out.Horizon, len(out.KeyChanges), latest, topDriver)
}

func subjectHorizonInputHash(out memory.SubjectHorizonMemory) string {
	payload, _ := json.Marshal(struct {
		Horizon string
		Start   string
		End     string
		Changes []memory.SubjectHorizonChange
		Drivers []memory.SubjectHorizonDriver
	}{out.Horizon, out.WindowStart, out.WindowEnd, out.KeyChanges, out.DriverClusters})
	sum := sha256.Sum256(payload)
	return hex.EncodeToString(sum[:])
}

func containsAny(value string, needles ...string) bool {
	for _, needle := range needles {
		if strings.Contains(value, needle) {
			return true
		}
	}
	return false
}
