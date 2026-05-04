package compile

import "testing"

func TestCoverageAuditReportsLedgerCategoriesMissingFromBrief(t *testing.T) {
	ledger := Ledger{Items: []LedgerItem{{
		ID:       "ledger-001",
		Category: "portfolio",
		Kind:     "list",
		Claim:    "Apple and American Express remain core holdings.",
		Entities: []string{"Apple", "American Express"},
	}}}
	brief := []BriefItem{{
		ID:       "brief-001",
		Category: "capital",
		Claim:    "Keep cash ready.",
	}}

	audit := auditBriefCoverage(ledger, brief)
	if len(audit.MissingCategories) != 1 || audit.MissingCategories[0] != "portfolio" {
		t.Fatalf("missing categories = %#v, want portfolio", audit.MissingCategories)
	}
	if len(audit.MissingListItems) != 1 || audit.MissingListItems[0] != "ledger-001" {
		t.Fatalf("missing list items = %#v, want ledger-001", audit.MissingListItems)
	}
}

func TestCoverageAuditPassesWhenBriefReferencesLedgerItem(t *testing.T) {
	ledger := Ledger{Items: []LedgerItem{{
		ID:        "ledger-001",
		Category:  "portfolio",
		Kind:      "list",
		Claim:     "Apple and American Express remain core holdings.",
		Entities:  []string{"Apple", "American Express"},
		SourceIDs: []string{"semantic-001"},
	}}}
	brief := []BriefItem{{
		ID:        "brief-001",
		Category:  "portfolio",
		Kind:      "list",
		Claim:     "Apple and American Express remain core holdings.",
		SourceIDs: []string{"semantic-001"},
	}}

	audit := auditBriefCoverage(ledger, brief)
	if !audit.IsZero() {
		t.Fatalf("audit = %#v, want zero when brief covers ledger item", audit)
	}
}
