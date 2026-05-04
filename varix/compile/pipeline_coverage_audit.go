package compile

import "strings"

func auditBriefCoverage(ledger Ledger, brief []BriefItem) CoverageAudit {
	if len(ledger.Items) == 0 || len(brief) == 0 {
		return CoverageAudit{}
	}
	briefCategories := map[string]struct{}{}
	briefSources := map[string]struct{}{}
	for _, item := range brief {
		if category := strings.TrimSpace(item.Category); category != "" {
			briefCategories[category] = struct{}{}
		}
		for _, sourceID := range item.SourceIDs {
			sourceID = strings.TrimSpace(sourceID)
			if sourceID != "" {
				briefSources[sourceID] = struct{}{}
			}
		}
	}

	var audit CoverageAudit
	missingCategories := map[string]struct{}{}
	for _, item := range ledger.Items {
		category := strings.TrimSpace(item.Category)
		if category != "" {
			if _, ok := briefCategories[category]; !ok {
				if _, seen := missingCategories[category]; !seen {
					missingCategories[category] = struct{}{}
					audit.MissingCategories = append(audit.MissingCategories, category)
				}
			}
		}
		if !ledgerItemCoveredByBrief(item, briefSources) {
			id := strings.TrimSpace(item.ID)
			if id != "" {
				audit.OmittedLedgerIDs = append(audit.OmittedLedgerIDs, id)
				if item.Kind == "list" {
					audit.MissingListItems = append(audit.MissingListItems, id)
				}
			}
		}
	}
	return audit
}

func ledgerItemCoveredByBrief(item LedgerItem, briefSources map[string]struct{}) bool {
	for _, sourceID := range item.SourceIDs {
		sourceID = strings.TrimSpace(sourceID)
		if sourceID == "" {
			continue
		}
		if _, ok := briefSources[sourceID]; ok {
			return true
		}
	}
	return false
}
