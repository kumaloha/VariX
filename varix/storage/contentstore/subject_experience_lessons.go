package contentstore

import (
	"fmt"
	"sort"
	"strings"

	"github.com/kumaloha/VariX/varix/memory"
)

type subjectDriverExperience struct {
	subject    string
	count      int
	horizons   []string
	changes    []string
	paths      []string
	sourceRefs []string
}

func deriveSubjectExperienceLessons(subject string, horizonMemories []memory.SubjectHorizonMemory) []memory.SubjectExperienceLesson {
	lessons := make([]memory.SubjectExperienceLesson, 0)
	drivers := map[string]*subjectDriverExperience{}
	trends := map[string][]string{}
	for _, item := range horizonMemories {
		if strings.TrimSpace(item.TrendDirection) != "" {
			trends[item.TrendDirection] = append(trends[item.TrendDirection], item.Horizon)
		}
		for _, driver := range item.DriverClusters {
			driverSubject := strings.TrimSpace(driver.Subject)
			if driverSubject == "" {
				continue
			}
			entry := drivers[driverSubject]
			if entry == nil {
				entry = &subjectDriverExperience{subject: driverSubject}
				drivers[driverSubject] = entry
			}
			entry.count += driver.Count
			entry.horizons = orderedUniqueStrings(append(entry.horizons, item.Horizon))
			entry.changes = uniqueStrings(append(entry.changes, driver.Changes...))
			entry.paths = uniqueStrings(append(entry.paths, driver.RelationPaths...))
			entry.sourceRefs = uniqueStrings(append(entry.sourceRefs, driver.SourceRefs...))
		}
	}
	driverList := make([]*subjectDriverExperience, 0, len(drivers))
	for _, driver := range drivers {
		driverList = append(driverList, driver)
	}
	sort.SliceStable(driverList, func(i, j int) bool {
		if driverList[i].count != driverList[j].count {
			return driverList[i].count > driverList[j].count
		}
		return driverList[i].subject < driverList[j].subject
	})
	for _, driver := range driverList {
		if driver.count < 2 && len(driver.horizons) < 2 {
			continue
		}
		lesson := memory.SubjectExperienceLesson{
			ID:                 "driver:" + driver.subject,
			Kind:               "driver-pattern",
			Statement:          subjectExperienceStatement(subject, driver),
			Trigger:            subjectExperienceTrigger(subject, driver),
			Mechanism:          subjectExperienceMechanism(subject, driver),
			Implication:        subjectExperienceImplication(subject, driver),
			Boundary:           subjectExperienceBoundary(subject, driver),
			TransferRule:       subjectExperienceTransferRule(subject, driver),
			TimeScaleMeaning:   subjectExperienceTimeScaleMeaning(driver.horizons),
			Confidence:         subjectExperienceConfidence(driver.count, len(driver.horizons)),
			SupportCount:       driver.count,
			Horizons:           append([]string(nil), driver.horizons...),
			DriverSubjects:     []string{driver.subject},
			EvidenceSourceRefs: append([]string(nil), driver.sourceRefs...),
		}
		lessons = append(lessons, lesson)
	}
	if len(trends) > 1 {
		parts := make([]string, 0, len(trends))
		for trend, trendHorizons := range trends {
			parts = append(parts, trend+"@"+strings.Join(trendHorizons, "/"))
		}
		sort.Strings(parts)
		lessons = append(lessons, memory.SubjectExperienceLesson{
			ID:           "horizon:trend-shift",
			Kind:         "horizon-shift",
			Statement:    fmt.Sprintf("%s 的经验会随时间尺度改变：%s。短窗口结论不能直接外推到长窗口。", subject, strings.Join(parts, ", ")),
			Implication:  "读取经验时必须带 horizon；否则会把阶段性波动误认为长期规律。",
			Confidence:   0.6,
			SupportCount: len(horizonMemories),
			Horizons:     subjectExperienceAllHorizons(horizonMemories),
		})
	}
	return lessons
}
func subjectExperienceStatement(subject string, driver *subjectDriverExperience) string {
	return fmt.Sprintf("%s 会改变市场解释 %s 的状态，而不只是单次涨跌原因。", driver.subject, subject)
}
func subjectExperienceTrigger(subject string, driver *subjectDriverExperience) string {
	return fmt.Sprintf("%s 方向变化或预期差扩大时。", driver.subject)
}
func subjectExperienceMechanism(subject string, driver *subjectDriverExperience) string {
	if len(driver.paths) > 0 {
		path := driver.paths[0]
		if subjectExperiencePathNodeCount(path) <= 2 {
			return "记忆路径: " + path + "；中间机制未展开。"
		}
		return "记忆路径: " + path + "。"
	}
	return fmt.Sprintf("记忆显示 %s 与 %s 变化反复共同出现；它和其他因素的关系未建立。", driver.subject, subject)
}
func subjectExperienceImplication(subject string, driver *subjectDriverExperience) string {
	return fmt.Sprintf("观察 %s 时，先判断 %s 是短期噪声、阶段性约束，还是跨尺度状态变量；只有第三种才值得写成可迁移经验。", subject, driver.subject)
}
func subjectExperienceBoundary(subject string, driver *subjectDriverExperience) string {
	if len(driver.paths) == 0 {
		return "没有路径证据时，只能当作观察线索，不能当作因果解释。"
	}
	return "当前只按记忆路径解释；路径之外的中间因素不能自动补全。"
}
func subjectExperienceTransferRule(subject string, driver *subjectDriverExperience) string {
	return fmt.Sprintf("下次先找 %s -> ... -> %s 的路径证据；没有就只保留为线索。", driver.subject, subject)
}
func subjectExperiencePathNodeCount(path string) int {
	if strings.TrimSpace(path) == "" {
		return 0
	}
	count := 0
	for _, part := range strings.Split(path, "->") {
		if strings.TrimSpace(part) != "" {
			count++
		}
	}
	return count
}
func subjectExperienceTimeScaleMeaning(horizons []string) string {
	switch {
	case containsOrderedHorizon(horizons, "1w") && containsOrderedHorizon(horizons, "1m"):
		return "时间尺度含义：1w 说明它正在解释近期波动，1m 说明它不是单日噪声；若继续进入 1q/1y，才可升级为结构性经验。"
	case containsOrderedHorizon(horizons, "1q") || containsOrderedHorizon(horizons, "1y") || containsOrderedHorizon(horizons, "2y") || containsOrderedHorizon(horizons, "5y"):
		return "时间尺度含义：它已跨过短期噪声窗口，更可能是状态变量；更新频率可以降低，但反例权重应该提高。"
	default:
		return "时间尺度含义：当前主要是短窗口经验，只能指导近期观察，不能直接外推。"
	}
}
func subjectExperienceConfidence(count, horizonCount int) float64 {
	confidence := 0.45 + float64(count)*0.08 + float64(horizonCount)*0.08
	if confidence > 0.9 {
		return 0.9
	}
	return confidence
}
