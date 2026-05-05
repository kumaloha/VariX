package compile

import (
	"context"
	"github.com/kumaloha/forge/llm"
	"slices"
	"testing"
)

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

func TestCoverageAuditReportsArticleLedgerItemsMissingFromRenderedMainline(t *testing.T) {
	rt := &fakeRuntime{responses: []llm.Response{
		{Text: `{"translations":[{"id":"n-ai","text":"AI资本开支激增"},{"id":"n-spx","text":"美股反弹"},{"id":"spine:s1","text":"AI资本开支推动美股反弹"}]}`},
		{Text: `{"summary":"AI资本开支推动美股反弹。"}`},
	}}
	out, err := stage5Render(context.Background(), rt, "compile-model", Bundle{
		UnitID:     "weibo:coverage-audit-article",
		Source:     "weibo",
		ExternalID: "coverage-audit-article",
		Content:    "AI capex drives stocks, while OPEC cohesion is another separate point.",
	}, graphState{
		ArticleForm: "market_update",
		Nodes: []graphNode{
			{ID: "n-ai", Text: "AI资本开支激增"},
			{ID: "n-spx", Text: "美股反弹", IsTarget: true},
			{ID: "n-opec", Text: "OPEC内部凝聚力出现裂痕"},
		},
		Edges: []graphEdge{{From: "n-ai", To: "n-spx"}},
		Ledger: Ledger{Items: []LedgerItem{{
			ID:        "ledger-001",
			Category:  "ai",
			Claim:     "AI资本开支激增",
			SourceIDs: []string{"n-ai"},
		}, {
			ID:        "ledger-002",
			Category:  "market",
			Claim:     "美股反弹",
			SourceIDs: []string{"n-spx"},
		}, {
			ID:        "ledger-003",
			Category:  "macro",
			Claim:     "OPEC内部凝聚力出现裂痕",
			SourceIDs: []string{"n-opec"},
		}}},
		Spines: []PreviewSpine{{
			ID:       "s1",
			Level:    "primary",
			Priority: 1,
			Thesis:   "AI资本开支推动美股反弹",
			NodeIDs:  []string{"n-ai", "n-spx"},
			Edges:    []PreviewEdge{{From: "n-ai", To: "n-spx"}},
			Scope:    "article",
		}},
	})
	if err != nil {
		t.Fatalf("stage5Render() error = %v", err)
	}
	if slices.Contains(out.CoverageAudit.OmittedLedgerIDs, "ledger-001") || slices.Contains(out.CoverageAudit.OmittedLedgerIDs, "ledger-002") {
		t.Fatalf("CoverageAudit = %#v, did not want rendered path ledger items omitted", out.CoverageAudit)
	}
	if !slices.Contains(out.CoverageAudit.OmittedLedgerIDs, "ledger-003") {
		t.Fatalf("CoverageAudit = %#v, want off-mainline ledger item omitted", out.CoverageAudit)
	}
	if !slices.Contains(out.CoverageAudit.MissingCategories, "macro") {
		t.Fatalf("missing categories = %#v, want macro", out.CoverageAudit.MissingCategories)
	}
}

func TestCoverageAuditCountsRenderedEvidenceAsCovered(t *testing.T) {
	rt := &fakeRuntime{responses: []llm.Response{
		{Text: `{"translations":[{"id":"n-ai","text":"AI资本开支激增"},{"id":"n-spx","text":"美股反弹"},{"id":"off-fed","text":"联储独立性争议升温"},{"id":"spine:s1","text":"AI资本开支推动美股反弹"}]}`},
		{Text: `{"summary":"AI资本开支推动美股反弹。"}`},
	}}
	out, err := stage5Render(context.Background(), rt, "compile-model", Bundle{
		UnitID:     "weibo:coverage-audit-evidence",
		Source:     "weibo",
		ExternalID: "coverage-audit-evidence",
		Content:    "AI capex drives stocks. Fed independence is visible supporting evidence.",
	}, graphState{
		ArticleForm: "market_update",
		Nodes: []graphNode{
			{ID: "n-ai", Text: "AI资本开支激增"},
			{ID: "n-spx", Text: "美股反弹", IsTarget: true},
		},
		Edges: []graphEdge{{From: "n-ai", To: "n-spx"}},
		OffGraph: []offGraphItem{{
			ID:   "off-fed",
			Text: "联储独立性争议升温",
			Role: "evidence",
		}},
		Ledger: Ledger{Items: []LedgerItem{{
			ID:        "ledger-001",
			Category:  "ai",
			Claim:     "AI资本开支激增",
			SourceIDs: []string{"n-ai"},
		}, {
			ID:        "ledger-002",
			Category:  "market",
			Claim:     "美股反弹",
			SourceIDs: []string{"n-spx"},
		}, {
			ID:        "ledger-003",
			Category:  "macro",
			Claim:     "联储独立性争议升温",
			SourceIDs: []string{"off-fed"},
		}}},
		Spines: []PreviewSpine{{
			ID:       "s1",
			Level:    "primary",
			Priority: 1,
			Thesis:   "AI资本开支推动美股反弹",
			NodeIDs:  []string{"n-ai", "n-spx"},
			Edges:    []PreviewEdge{{From: "n-ai", To: "n-spx"}},
			Scope:    "article",
		}},
	})
	if err != nil {
		t.Fatalf("stage5Render() error = %v", err)
	}
	if !slices.Contains(out.EvidenceNodes, "联储独立性争议升温") {
		t.Fatalf("EvidenceNodes = %#v, want rendered off-graph evidence", out.EvidenceNodes)
	}
	if slices.Contains(out.CoverageAudit.OmittedLedgerIDs, "ledger-003") {
		t.Fatalf("CoverageAudit = %#v, did not want rendered evidence ledger item omitted", out.CoverageAudit)
	}
}
