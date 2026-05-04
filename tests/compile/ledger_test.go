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

func TestLedgerFallsBackToGraphItemsForArticleForms(t *testing.T) {
	state := graphState{
		ArticleForm: "market_update",
		Nodes: []graphNode{{
			ID:          "n-ai-capex",
			Text:        "四大超巨云端服务商AI基建投资额较去年增长77%",
			SourceQuote: "AI capex up 77%",
			Role:        roleDriver,
		}, {
			ID:       "n-spx",
			Text:     "S&P500指数四月上涨10%",
			Role:     roleTransmission,
			IsTarget: true,
		}},
		OffGraph: []offGraphItem{{
			ID:   "off-opec",
			Text: "阿联酋退出标志OPEC内部凝聚力出现重大裂痕",
			Role: "evidence",
		}},
	}

	got := buildLedger(state)
	if len(got.Items) < 3 {
		t.Fatalf("ledger items = %#v, want graph and off-graph fallback items", got.Items)
	}
	if item := ledgerItemBySourceID(got.Items, "n-ai-capex"); item == nil || item.Category != "ai" || !slices.Contains(item.Numbers, "77%") {
		t.Fatalf("AI capex ledger item = %#v, want ai item preserving 77%%", item)
	}
	if item := ledgerItemBySourceID(got.Items, "off-opec"); item == nil || item.Category != "governance" {
		t.Fatalf("off-graph ledger item = %#v, want persisted evidence fallback", item)
	}
}

func TestLedgerClassifiesCultureAndGEICOBeforeOperations(t *testing.T) {
	culture := ledgerCategory("management transition operations Berkshire culture and values remain unchanged and continue as the operating foundation")
	if culture != "culture" {
		t.Fatalf("culture ledger category = %q, want culture", culture)
	}
	geico := ledgerCategory("GEICO核心运营目标是在精准风险定价、客户留存与保单增长之间取得平衡")
	if geico != "insurance" {
		t.Fatalf("GEICO ledger category = %q, want insurance", geico)
	}
	ai := ledgerCategory("AI目前仅作为降低人工成本的生产力工具，短期内无法替代人类在定价、理赔等核心承保决策中的判断力")
	if ai != "ai" {
		t.Fatalf("AI underwriting tool ledger category = %q, want ai", ai)
	}
	succession := ledgerCategory("culture and succession 董事会已为Ajit Jain和Greg Abel制定并持续讨论正式的继任计划")
	if succession != "succession" {
		t.Fatalf("succession ledger category = %q, want succession", succession)
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

func ledgerItemBySourceID(items []LedgerItem, sourceID string) *LedgerItem {
	for i := range items {
		for _, existing := range items[i].SourceIDs {
			if existing == sourceID {
				return &items[i]
			}
		}
	}
	return nil
}

func ledgerItemByClaim(items []LedgerItem, claim string) *LedgerItem {
	for i := range items {
		if items[i].Claim == claim {
			return &items[i]
		}
	}
	return nil
}
