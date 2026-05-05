package compile

import "strings"

func auditBriefCoverage(ledger Ledger, brief []BriefItem) CoverageAudit {
	if len(ledger.Items) == 0 || len(brief) == 0 {
		return CoverageAudit{}
	}
	view := newCoverageAuditView()
	for _, item := range brief {
		view.addBriefItem(item)
	}
	return auditLedgerCoverage(ledger, view)
}

func auditRenderedCoverage(ledger Ledger, brief []BriefItem, drivers, targets []graphNode, paths []renderedPath, offGraph []offGraphItem) CoverageAudit {
	if len(ledger.Items) == 0 {
		return CoverageAudit{}
	}
	view := newCoverageAuditView()
	for _, item := range brief {
		view.addBriefItem(item)
	}
	for _, node := range drivers {
		view.addGraphNode(node)
	}
	for _, node := range targets {
		view.addGraphNode(node)
	}
	for _, path := range paths {
		view.addGraphNode(path.driver)
		view.addGraphNode(path.target)
		for _, premise := range path.premises {
			view.addGraphNode(premise)
		}
		for _, step := range path.steps {
			view.addGraphNode(step)
		}
	}
	for _, item := range offGraph {
		view.addOffGraphItem(item)
	}
	return auditLedgerCoverage(ledger, view)
}

type coverageAuditView struct {
	categories map[string]struct{}
	sourceIDs  map[string]struct{}
	texts      map[string]struct{}
}

func newCoverageAuditView() coverageAuditView {
	return coverageAuditView{
		categories: map[string]struct{}{},
		sourceIDs:  map[string]struct{}{},
		texts:      map[string]struct{}{},
	}
}

func (v coverageAuditView) addBriefItem(item BriefItem) {
	if category := strings.TrimSpace(item.Category); category != "" {
		v.categories[category] = struct{}{}
	}
	if claim := normalizeText(item.Claim); claim != "" {
		v.texts[claim] = struct{}{}
	}
	for _, sourceID := range item.SourceIDs {
		v.addSourceID(sourceID)
	}
}

func (v coverageAuditView) addGraphNode(node graphNode) {
	v.addSourceID(node.ID)
	if text := normalizeText(node.Text); text != "" {
		v.texts[text] = struct{}{}
	}
}

func (v coverageAuditView) addOffGraphItem(item offGraphItem) {
	v.addSourceID(item.ID)
	if text := normalizeText(item.Text); text != "" {
		v.texts[text] = struct{}{}
	}
}

func (v coverageAuditView) addSourceID(sourceID string) {
	sourceID = strings.TrimSpace(sourceID)
	if sourceID != "" {
		v.sourceIDs[sourceID] = struct{}{}
	}
}

func auditLedgerCoverage(ledger Ledger, view coverageAuditView) CoverageAudit {
	var audit CoverageAudit
	missingCategories := map[string]struct{}{}
	var missingCategoryOrder []string
	for _, item := range ledger.Items {
		category := strings.TrimSpace(item.Category)
		covered := ledgerItemCoveredByView(item, view)
		if covered && category != "" {
			view.categories[category] = struct{}{}
		}
		if category != "" {
			if _, ok := view.categories[category]; !ok {
				if _, seen := missingCategories[category]; !seen {
					missingCategories[category] = struct{}{}
					missingCategoryOrder = append(missingCategoryOrder, category)
				}
			}
		}
		if !covered {
			id := strings.TrimSpace(item.ID)
			if id != "" {
				audit.OmittedLedgerIDs = append(audit.OmittedLedgerIDs, id)
				if item.Kind == "list" {
					audit.MissingListItems = append(audit.MissingListItems, id)
				}
			}
		}
	}
	for _, category := range missingCategoryOrder {
		if _, covered := view.categories[category]; !covered {
			audit.MissingCategories = append(audit.MissingCategories, category)
		}
	}
	return audit
}

func ledgerItemCoveredByView(item LedgerItem, view coverageAuditView) bool {
	for _, sourceID := range item.SourceIDs {
		sourceID = strings.TrimSpace(sourceID)
		if sourceID == "" {
			continue
		}
		if _, ok := view.sourceIDs[sourceID]; ok {
			return true
		}
	}
	if claim := normalizeText(item.Claim); claim != "" {
		if _, ok := view.texts[claim]; ok {
			return true
		}
	}
	return false
}
