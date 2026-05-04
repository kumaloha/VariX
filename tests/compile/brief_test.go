package compile

import (
	"strings"
	"testing"
)

func TestBriefBalancesMeetingCategories(t *testing.T) {
	state := graphState{
		ArticleForm: "shareholder_meeting",
		SemanticUnits: []SemanticUnit{
			{ID: "i1", Subject: "网络保险风险", Claim: "网络保险风险一", Salience: 0.99},
			{ID: "i2", Subject: "保险市场软化", Claim: "保险市场软化", Salience: 0.98},
			{ID: "i3", Subject: "GEICO定价", Claim: "GEICO定价", Salience: 0.97},
			{ID: "a1", Subject: "AI应用治理", Claim: "AI必须保留人工介入", Salience: 0.9},
			{ID: "e1", Subject: "数据中心能源成本", Claim: "数据中心承担全部电力成本", Salience: 0.86},
			{ID: "p1", Subject: "现有投资组合评估框架", Claim: "Apple说明能力圈看产品价值与消费者依赖", Salience: 0.74},
			{ID: "c1", Subject: "culture and values", Claim: "Berkshire culture and values remain unchanged", Salience: 0.72},
			{ID: "s1", Subject: "succession plan", Claim: "董事会已有Greg Abel与Ajit Jain继任计划", Salience: 0.7},
		},
	}
	got := stageBrief(state).Brief
	if len(got) == 0 {
		t.Fatalf("Brief is empty")
	}
	for _, want := range []string{"insurance", "ai", "energy", "portfolio", "culture", "succession"} {
		if briefItemByCategory(got, want) == nil {
			t.Fatalf("Brief = %#v, missing category %q", got, want)
		}
	}
	if countBriefCategory(got, "insurance") > 2 {
		t.Fatalf("Brief = %#v, want insurance capped so it cannot crowd out meeting topics", got)
	}
}

func TestBriefInfersListsAndNumbers(t *testing.T) {
	state := graphState{
		ArticleForm: "shareholder_meeting",
		SemanticUnits: []SemanticUnit{{
			ID:       "portfolio",
			Subject:  "portfolio holdings",
			Claim:    "Berkshire highlighted Apple, American Express, Coca-Cola, Bank of America, and five Japanese trading houses.",
			Salience: 0.8,
		}, {
			ID:       "energy",
			Subject:  "data center energy load",
			Claim:    "Data centers already represent 8% of peak load and may grow 50% over five years.",
			Salience: 0.79,
		}},
	}
	got := stageBrief(state).Brief
	portfolio := briefItemByCategory(got, "portfolio")
	if portfolio == nil || portfolio.Kind != "list" {
		t.Fatalf("portfolio item = %#v, want list brief item", portfolio)
	}
	if !containsBriefEntity(portfolio.Entities, "Apple") || !containsBriefEntity(portfolio.Entities, "Coca-Cola") {
		t.Fatalf("portfolio entities = %#v, want named holdings preserved", portfolio.Entities)
	}
	energy := briefItemByCategory(got, "energy")
	if energy == nil || len(energy.Numbers) < 2 {
		t.Fatalf("energy item = %#v, want numeric facts preserved", energy)
	}
}

func TestBriefKeepsMandatoryMeetingCategoriesFromLedger(t *testing.T) {
	state := graphState{
		ArticleForm: "shareholder_meeting",
		Ledger: Ledger{Items: []LedgerItem{
			{ID: "ledger-001", Category: "capital", Kind: "commitment", Claim: "Hold cash until the right opportunity appears.", Salience: 0.98},
			{ID: "ledger-002", Category: "insurance", Kind: "boundary", Claim: "Do not write cyber risk when aggregation cannot be modeled.", Salience: 0.96},
			{ID: "ledger-003", Category: "ai", Kind: "boundary", Claim: "AI must remain additive and supervised.", Salience: 0.94},
			{ID: "ledger-004", Category: "portfolio", Kind: "list", Claim: "The portfolio includes Apple, American Express, Coca-Cola, and Bank of America.", Entities: []string{"Apple", "American Express", "Coca-Cola", "Bank of America"}, Salience: 0.7},
			{ID: "ledger-005", Category: "succession", Kind: "commitment", Claim: "The board has succession plans for Greg Abel and Ajit Jain.", Salience: 0.68},
		}},
	}

	got := stageBrief(state).Brief
	if briefItemByCategory(got, "portfolio") == nil {
		t.Fatalf("brief = %#v, want portfolio", got)
	}
	if briefItemByCategory(got, "succession") == nil {
		t.Fatalf("brief = %#v, want succession", got)
	}
}

func briefItemByCategory(items []BriefItem, category string) *BriefItem {
	for i := range items {
		if items[i].Category == category {
			return &items[i]
		}
	}
	return nil
}

func countBriefCategory(items []BriefItem, category string) int {
	count := 0
	for _, item := range items {
		if item.Category == category {
			count++
		}
	}
	return count
}

func containsBriefEntity(values []string, needle string) bool {
	for _, value := range values {
		if strings.EqualFold(value, needle) {
			return true
		}
	}
	return false
}
