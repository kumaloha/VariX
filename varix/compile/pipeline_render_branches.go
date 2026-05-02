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
		branch := Branch{
			ID:     branchID,
			Level:  strings.TrimSpace(spine.Level),
			Policy: normalizePreviewSpinePolicy(spine.Policy),
			Thesis: strings.TrimSpace(spine.Thesis),
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
