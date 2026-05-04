package compile

import (
	"slices"
	"testing"
)

func TestLedgerPreservesPortfolioListFacts(t *testing.T) {
	state := graphState{
		SemanticUnits: []SemanticUnit{{
			ID:       "semantic-014",
			Subject:  "existing portfolio / circle of competence",
			Force:    "answer",
			Claim:    "Greg Abel said Berkshire remains comfortable with Apple, American Express, Coca-Cola, and Bank of America because the businesses remain understandable.",
			Salience: 0.77,
		}},
	}

	got := buildLedger(state)
	item := ledgerItemByCategory(got.Items, "portfolio")
	if item == nil {
		t.Fatalf("ledger items = %#v, want portfolio item", got.Items)
	}
	for _, entity := range []string{"Apple", "American Express", "Coca-Cola", "Bank of America"} {
		if !slices.Contains(item.Entities, entity) {
			t.Fatalf("portfolio entities = %#v, want %q", item.Entities, entity)
		}
	}
	if !slices.Contains(item.SourceIDs, "semantic-014") {
		t.Fatalf("source IDs = %#v, want semantic-014", item.SourceIDs)
	}
}

func TestLedgerClassifiesOperationsBeforeAI(t *testing.T) {
	state := graphState{
		SemanticUnits: []SemanticUnit{{
			ID:       "semantic-017",
			Subject:  "BNSF operating plan",
			Force:    "commit",
			Claim:    "BNSF plans to improve margins through cost reduction and technology application.",
			Salience: 0.7,
		}},
	}

	got := buildLedger(state)
	if item := ledgerItemByCategory(got.Items, "operations"); item == nil {
		t.Fatalf("ledger items = %#v, want BNSF item classified as operations", got.Items)
	}
	if item := ledgerItemByCategory(got.Items, "ai"); item != nil {
		t.Fatalf("ledger items = %#v, did not want BNSF technology application classified as ai", got.Items)
	}
}

func ledgerItemByCategory(items []LedgerItem, category string) *LedgerItem {
	for i := range items {
		if items[i].Category == category {
			return &items[i]
		}
	}
	return nil
}
