package contentstore

import (
	"fmt"
	"sort"
	"strings"

	"github.com/kumaloha/VariX/varix/memory"
)

type subjectAttributionFactorState struct {
	subject  string
	count    int
	refs     []string
	sources  []string
	horizons []string
}
type subjectAttributionChangeState struct {
	when       string
	change     string
	externalID string
	sourceBase string
	factors    []string
}
type subjectAttributionRelationState struct {
	factors []string
	sources []string
}

func buildSubjectAttributionSummary(horizonMemories []memory.SubjectHorizonMemory) memory.SubjectAttributionSummary {
	factors := map[string]*subjectAttributionFactorState{}
	changes := map[string]*subjectAttributionChangeState{}
	sourceFactors := map[string][]string{}
	relations := map[string]*subjectAttributionRelationState{}
	for _, horizon := range horizonMemories {
		for _, driver := range horizon.DriverClusters {
			subject := strings.TrimSpace(driver.Subject)
			if subject == "" {
				continue
			}
			state := factors[subject]
			if state == nil {
				state = &subjectAttributionFactorState{subject: subject}
				factors[subject] = state
			}
			state.refs = uniqueStrings(append(state.refs, driver.SourceRefs...))
			state.count = len(state.refs)
			state.horizons = orderedUniqueStrings(append(state.horizons, horizon.Horizon))
			for _, ref := range driver.SourceRefs {
				source := sourceRefBase(ref)
				if source == "" {
					continue
				}
				state.sources = orderedUniqueStrings(append(state.sources, source))
				sourceFactors[source] = orderedUniqueStrings(append(sourceFactors[source], subject))
			}
		}
		for _, change := range horizon.KeyChanges {
			sourceBase := change.SourcePlatform + ":" + change.SourceExternalID
			key := sourceBase + "#" + change.NodeID
			if _, ok := changes[key]; ok {
				continue
			}
			changes[key] = &subjectAttributionChangeState{
				when:       change.When,
				change:     change.ChangeText,
				externalID: change.SourceExternalID,
				sourceBase: sourceBase,
			}
		}
	}
	for _, change := range changes {
		change.factors = rankSubjectFactors(sourceFactors[change.sourceBase], factors)
		if len(change.factors) > 1 {
			keyFactors := append([]string(nil), change.factors...)
			sort.Strings(keyFactors)
			key := strings.Join(keyFactors, "\x00")
			state := relations[key]
			if state == nil {
				state = &subjectAttributionRelationState{factors: keyFactors}
				relations[key] = state
			}
			state.sources = orderedUniqueStrings(append(state.sources, change.sourceBase))
		}
	}
	changeList := make([]memory.SubjectChangeAttribution, 0, len(changes))
	for _, change := range changes {
		changeList = append(changeList, memory.SubjectChangeAttribution{
			When:             change.when,
			ChangeText:       change.change,
			Factors:          change.factors,
			SourceExternalID: change.externalID,
		})
	}
	sort.SliceStable(changeList, func(i, j int) bool {
		if changeList[i].When != changeList[j].When {
			return changeList[i].When < changeList[j].When
		}
		return changeList[i].SourceExternalID < changeList[j].SourceExternalID
	})
	primary := subjectAttributionPrimaryFactor(factors)
	relationList := make([]memory.SubjectFactorRelationship, 0, len(relations))
	for _, relation := range relations {
		relationList = append(relationList, memory.SubjectFactorRelationship{
			Factors:     append([]string(nil), relation.factors...),
			Relation:    subjectAttributionRelationLabel(relation.factors, primary.Subject),
			SourceCount: len(relation.sources),
			SourceRefs:  relation.sources,
		})
	}
	sort.SliceStable(relationList, func(i, j int) bool {
		if relationList[i].SourceCount != relationList[j].SourceCount {
			return relationList[i].SourceCount > relationList[j].SourceCount
		}
		leftKey := strings.Join(relationList[i].Factors, "\x00")
		rightKey := strings.Join(relationList[j].Factors, "\x00")
		if leftKey != rightKey {
			return leftKey < rightKey
		}
		return relationList[i].Relation < relationList[j].Relation
	})
	if len(relationList) > 6 {
		relationList = relationList[:6]
	}
	return memory.SubjectAttributionSummary{
		ChangeCount:        len(changeList),
		FactorCount:        len(factors),
		PrimaryFactor:      primary,
		ChangeAttributions: changeList,
		FactorRelations:    relationList,
	}
}
func subjectAttributionRelationLabel(factors []string, primary string) string {
	primary = strings.TrimSpace(primary)
	if primary == "" || !containsOrderedString(factors, primary) {
		return "共同归因同一变化；当前按并列因素处理"
	}
	others := make([]string, 0, len(factors))
	for _, factor := range factors {
		if factor != primary {
			others = append(others, factor)
		}
	}
	return fmt.Sprintf("共同归因同一变化；%s是当前主导因素，%s是伴随因素", primary, strings.Join(others, "、"))
}
func subjectAttributionPrimaryFactor(factors map[string]*subjectAttributionFactorState) memory.SubjectPrimaryFactor {
	list := make([]*subjectAttributionFactorState, 0, len(factors))
	for _, factor := range factors {
		list = append(list, factor)
	}
	sort.SliceStable(list, func(i, j int) bool {
		if list[i].count != list[j].count {
			return list[i].count > list[j].count
		}
		if len(list[i].sources) != len(list[j].sources) {
			return len(list[i].sources) > len(list[j].sources)
		}
		return list[i].subject < list[j].subject
	})
	if len(list) == 0 {
		return memory.SubjectPrimaryFactor{}
	}
	top := list[0]
	return memory.SubjectPrimaryFactor{
		Subject:     top.subject,
		Reason:      fmt.Sprintf("在当前样本中出现次数最多，覆盖 %d 个来源、%d 个时间尺度。", len(top.sources), len(top.horizons)),
		Support:     top.count,
		SourceCount: len(top.sources),
		Horizons:    append([]string(nil), top.horizons...),
	}
}
func rankSubjectFactors(items []string, factors map[string]*subjectAttributionFactorState) []string {
	out := orderedUniqueStrings(items)
	sort.SliceStable(out, func(i, j int) bool {
		left := factors[out[i]]
		right := factors[out[j]]
		leftCount, rightCount := 0, 0
		if left != nil {
			leftCount = left.count
		}
		if right != nil {
			rightCount = right.count
		}
		if leftCount != rightCount {
			return leftCount > rightCount
		}
		return out[i] < out[j]
	})
	return out
}
func containsOrderedString(items []string, want string) bool {
	for _, item := range items {
		if item == want {
			return true
		}
	}
	return false
}
func sourceRefBase(ref string) string {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return ""
	}
	if before, _, ok := strings.Cut(ref, "#"); ok {
		return before
	}
	return ref
}
