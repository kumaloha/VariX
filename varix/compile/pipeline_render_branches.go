package compile

import (
	"strings"
)

func renderBranchesFromSpines(spines []PreviewSpine, paths []renderedPath, cn func(string, string) string) []Branch {
	if len(spines) == 0 || len(paths) == 0 {
		return nil
	}
	pathsByBranch := map[string][]renderedPath{}
	for _, path := range paths {
		branchID := strings.TrimSpace(path.branchID)
		if branchID == "" {
			continue
		}
		pathsByBranch[branchID] = append(pathsByBranch[branchID], path)
	}
	if len(pathsByBranch) == 0 {
		return nil
	}
	commonDrivers := commonRenderedDrivers(paths, cn)
	out := make([]Branch, 0, len(spines))
	for _, spine := range spines {
		branchID := strings.TrimSpace(spine.ID)
		if branchID == "" {
			continue
		}
		branchPaths := pathsByBranch[branchID]
		if len(branchPaths) == 0 {
			continue
		}
		branchThesis := compactBranchTopic(cn(spineTranslationNodeID(branchID), spine.Thesis))
		branch := Branch{
			ID:     branchID,
			Level:  strings.TrimSpace(spine.Level),
			Policy: normalizePreviewSpinePolicy(spine.Policy),
			Thesis: branchThesis,
		}
		for _, path := range branchPaths {
			driver := cn(path.driver.ID, path.driver.Text)
			target := cn(path.target.ID, path.target.Text)
			if _, ok := commonDrivers[driver]; ok {
				branch.Anchors = appendUniqueString(branch.Anchors, driver)
			}
			if branchDriver := renderBranchDriver(path, commonDrivers, cn); branchDriver != "" {
				branch.BranchDrivers = appendUniqueString(branch.BranchDrivers, branchDriver)
			}
			branch.Drivers = appendUniqueString(branch.Drivers, driver)
			branch.Targets = appendUniqueString(branch.Targets, target)
			branch.TransmissionPaths = append(branch.TransmissionPaths, renderPathToTransmission(path, cn))
		}
		out = append(out, branch)
	}
	return out
}

func spineTranslationNodes(spines []PreviewSpine) []graphNode {
	out := make([]graphNode, 0, len(spines))
	for _, spine := range spines {
		id := spineTranslationNodeID(spine.ID)
		text := strings.TrimSpace(spine.Thesis)
		if id == "" || text == "" {
			continue
		}
		out = append(out, graphNode{ID: id, Text: text})
	}
	return out
}

func spineTranslationNodeID(spineID string) string {
	spineID = strings.TrimSpace(spineID)
	if spineID == "" {
		return ""
	}
	return "spine:" + spineID
}

func renderBranchTopics(branches []Branch) []string {
	topics := make([]string, 0, len(branches))
	for _, branch := range branches {
		topic := compactBranchTopic(branch.Thesis)
		if topic == "" {
			topic = compactBranchTopic(firstNonEmptyBranchText(branch))
		}
		if topic == "" {
			continue
		}
		topics = appendUniqueString(topics, topic)
	}
	return topics
}

func compactBranchTopic(topic string) string {
	topic = strings.TrimSpace(strings.TrimRight(topic, "。.!！"))
	if topic == "" {
		return topic
	}
	if compact := compactImportantSecondClauseTopic(topic); compact != "" {
		return compact
	}
	for _, sep := range []string{"，", "；", ";", "。", ".", "，并", "，推动", "，带动", "，导致", "，使"} {
		if before, _, ok := strings.Cut(topic, sep); ok {
			before = strings.TrimSpace(strings.TrimRight(before, "。.!！"))
			if len([]rune(before)) >= 8 && len([]rune(before)) <= 32 {
				return before
			}
		}
	}
	if len([]rune(topic)) <= summaryMaxRunes {
		return topic
	}
	return truncateRunes(topic, 32)
}

func compactImportantSecondClauseTopic(topic string) string {
	for _, sep := range []string{"，并", "，且", "，同时"} {
		before, after, ok := strings.Cut(topic, sep)
		if !ok {
			continue
		}
		after = strings.TrimSpace(strings.TrimRight(after, "。.!！"))
		if !containsAnyText(after, []string{"降息", "门槛", "收益率", "美元", "石油", "OPEC", "通胀", "风险", "资产", "现金", "资本", "流动性"}) {
			continue
		}
		subject := compactTopicSubject(before)
		if subject == "" || after == "" {
			continue
		}
		candidate := subject + after
		if len([]rune(candidate)) <= 32 {
			return candidate
		}
	}
	return ""
}

func compactTopicSubject(text string) string {
	text = strings.TrimSpace(text)
	for _, marker := range []string{"迫使", "推动", "带动", "削弱", "触发", "压倒", "导致", "使得", "使"} {
		if before, _, ok := strings.Cut(text, marker); ok {
			before = strings.TrimSpace(before)
			if before != "" {
				return before
			}
		}
	}
	return text
}

func firstNonEmptyBranchText(branch Branch) string {
	for _, values := range [][]string{branch.BranchDrivers, branch.Drivers, branch.Targets, branch.Anchors} {
		for _, value := range values {
			if strings.TrimSpace(value) != "" {
				return value
			}
		}
	}
	return ""
}

func commonRenderedDrivers(paths []renderedPath, cn func(string, string) string) map[string]struct{} {
	driverBranches := map[string]map[string]struct{}{}
	for _, path := range paths {
		branchID := strings.TrimSpace(path.branchID)
		if branchID == "" {
			continue
		}
		driver := cn(path.driver.ID, path.driver.Text)
		if strings.TrimSpace(driver) == "" {
			continue
		}
		if driverBranches[driver] == nil {
			driverBranches[driver] = map[string]struct{}{}
		}
		driverBranches[driver][branchID] = struct{}{}
	}
	common := map[string]struct{}{}
	for driver, branches := range driverBranches {
		if len(branches) > 1 {
			common[driver] = struct{}{}
		}
	}
	return common
}

func renderBranchDriver(path renderedPath, commonDrivers map[string]struct{}, cn func(string, string) string) string {
	target := cn(path.target.ID, path.target.Text)
	candidates := make([]string, 0, len(path.steps)+1)
	candidates = append(candidates, cn(path.driver.ID, path.driver.Text))
	for _, step := range path.steps {
		candidates = append(candidates, cn(step.ID, step.Text))
	}
	for _, candidate := range candidates {
		candidate = strings.TrimSpace(candidate)
		if candidate == "" || candidate == target {
			continue
		}
		if _, ok := commonDrivers[candidate]; ok {
			continue
		}
		return candidate
	}
	return ""
}

func appendUniqueString(values []string, value string) []string {
	value = strings.TrimSpace(value)
	if value == "" {
		return values
	}
	for _, existing := range values {
		if existing == value {
			return values
		}
	}
	return append(values, value)
}
