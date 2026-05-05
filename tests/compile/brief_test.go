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
	if countBriefCategory(got, "insurance") > meetingBriefCategoryLimit {
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

func TestMeetingBriefKeepsMultipleSpecificAgendaItemsPerCategory(t *testing.T) {
	state := graphState{
		ArticleForm: "shareholder_meeting",
		Ledger: Ledger{Items: []LedgerItem{
			{ID: "capital", Category: "capital", Kind: "commitment", Claim: "Hold cash and wait for rare opportunities.", Salience: 0.99},
			{ID: "portfolio", Category: "portfolio", Kind: "list", Claim: "Portfolio holdings remain concentrated.", Salience: 0.98},
			{ID: "ai", Category: "ai", Kind: "boundary", Claim: "AI must retain human oversight.", Salience: 0.97},
			{ID: "culture", Category: "culture", Kind: "commitment", Claim: "Culture stays unchanged.", Salience: 0.96},
			{ID: "succession", Category: "succession", Kind: "commitment", Claim: "The board has succession plans for Greg Abel and Ajit Jain.", Salience: 0.95},
			{ID: "governance", Category: "governance", Kind: "risk", Claim: "Single-day options make the market more casino-like.", Salience: 0.94},
			{ID: "buyback", Category: "buyback", Kind: "boundary", Claim: "Repurchase only below intrinsic value.", Salience: 0.93},
			{ID: "shareholder", Category: "shareholder", Kind: "disclosure", Claim: "Tokyo Marine partnership has three parts.", Salience: 0.92},
			{ID: "insurance-soft", Category: "insurance", Kind: "boundary", Claim: "Insurance market softening means Berkshire writes less premium.", Salience: 0.91},
			{ID: "insurance-geico", Category: "insurance", Kind: "commitment", Claim: "GEICO must balance risk pricing, retention, and growth.", Salience: 0.9},
			{ID: "insurance-underwriting", Category: "insurance", Kind: "boundary", Claim: "Underwriters default to saying no unless value screams.", Salience: 0.89},
			{ID: "insurance-cyber", Category: "insurance", Kind: "boundary", Claim: "Cyber insurance is avoided while aggregate exposure cannot be modeled.", Salience: 0.88},
			{ID: "energy-compact", Category: "energy", Kind: "boundary", Claim: "Utilities need a balanced regulatory compact.", Salience: 0.91},
			{ID: "energy-inflation", Category: "energy", Kind: "risk", Claim: "Runaway inflation is something Berkshire can only avoid.", Salience: 0.9},
			{ID: "energy-data-center", Category: "energy", Kind: "boundary", Claim: "Data centers and hyperscalers must bear their full infrastructure cost.", Salience: 0.89},
			{ID: "operations-bnsf", Category: "operations", Kind: "commitment", Claim: "BNSF must improve operating efficiency toward leading railroad margins.", Salience: 0.87},
		}},
	}

	got := stageBrief(state).Brief
	for _, want := range []string{
		"Cyber insurance is avoided",
		"Data centers and hyperscalers",
		"BNSF must improve",
	} {
		if !briefContainsClaim(got, want) {
			t.Fatalf("brief = %#v, missing agenda item containing %q", got, want)
		}
	}
}

func TestEarningsCallBriefKeepsFinancialMetricInventory(t *testing.T) {
	state := graphState{
		ArticleForm: "earnings_call",
		Ledger: Ledger{Items: []LedgerItem{
			{ID: "revenue", Category: "financials", Kind: "number", Claim: "FY 2025 Alphabet consolidated revenues reached $403 billion, up 15%.", Salience: 0.99},
			{ID: "q4-revenue", Category: "financials", Kind: "number", Claim: "Q4 2025 Alphabet consolidated revenues reached $113.8 billion, up 18%.", Salience: 0.98},
			{ID: "search", Category: "financials", Kind: "number", Claim: "Q4 Google Search and Other advertising revenues increased 17% to $63.1 billion.", Salience: 0.97},
			{ID: "youtube", Category: "financials", Kind: "number", Claim: "Q4 YouTube advertising revenues increased 9% to $11.4 billion.", Salience: 0.96},
			{ID: "cloud", Category: "financials", Kind: "number", Claim: "Q4 Google Cloud revenues increased 48% to $17.7 billion.", Salience: 0.95},
			{ID: "backlog", Category: "financials", Kind: "number", Claim: "Google Cloud backlog increased 55% sequentially to $240 billion.", Salience: 0.94},
			{ID: "operating-income", Category: "financials", Kind: "number", Claim: "Q4 Alphabet operating income increased 16% to $35.9 billion.", Salience: 0.93},
			{ID: "net-income", Category: "financials", Kind: "number", Claim: "Q4 Alphabet net income increased 30% to $34.5 billion.", Salience: 0.92},
			{ID: "capex", Category: "financials", Kind: "number", Claim: "Q4 Alphabet capital expenditures were $27.9 billion.", Salience: 0.91},
			{ID: "ai", Category: "ai", Kind: "commitment", Claim: "2026 capital expenditures support AI compute and Cloud demand.", Salience: 0.9},
		}},
	}

	got := stageBrief(state).Brief
	if countBriefCategory(got, "financials") < 8 {
		t.Fatalf("brief = %#v, want earnings call financial metric inventory preserved", got)
	}
	for _, want := range []string{"Google Cloud revenues", "Cloud backlog", "net income"} {
		if !briefContainsClaim(got, want) {
			t.Fatalf("brief = %#v, missing metric containing %q", got, want)
		}
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

func briefContainsClaim(items []BriefItem, needle string) bool {
	for _, item := range items {
		if strings.Contains(item.Claim, needle) {
			return true
		}
	}
	return false
}
