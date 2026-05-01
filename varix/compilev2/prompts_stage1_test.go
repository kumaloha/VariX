package compilev2

import "testing"

func TestStage1PromptAllowsNormalizationButRejectsSemanticUpgrade(t *testing.T) {
	loader := newPromptLoader("")
	body, err := loader.render("extract_system.tmpl", nil)
	if err != nil {
		t.Fatalf("render(extract_system.tmpl) error = %v", err)
	}
	for _, want := range []string{
		"may normalize wording into a clearer subject + change form",
		"Do not upgrade the meaning",
		"Do not add direction, intensity, certainty, or causality that is not explicit in the source quote",
		"A valid node should be interpretable as `subject + change`.",
		"The subject must be the stable object that undergoes the change, not the action word itself.",
		"Prefer subjects that are stable objects such as:",
		"If a quote fragment only states the change but not a full subject, recover the subject from the nearest local context in the same branch before writing the node text.",
		"Resolve local referential subjects such as `该基金`, `该市场`, `这笔钱`, `这些请求`, `投资者资金` back to the nearest stable container subject in the local context.",
		"Do not leave a node subject as only `该基金` / `资金` / `请求` / `赎回` when the local context already tells you whose fund / whose funds / whose requests they are.",
		"A percentage, amount, threshold, time expression, or pure action noun is not a sufficient subject by itself.",
		"If a node loses the direction of change, it is invalid and should be rewritten or split",
		"Every node must be directly grounded in a source quote from the article",
		"If you cannot point to the quote that supports the node, do not output the node",
		"For explicit `X causes Y` wording, extract X and Y as separate nodes",
		"Do classify the article form and each node's discourse role",
		"Article form:",
		"`main_narrative_plus_investment_implication`",
		"`evidence_backed_forecast`",
		"`institutional_satire`",
		`"article_form": "single_thesis|main_narrative_plus_investment_implication|evidence_backed_forecast|institutional_satire|satirical_financial_commentary|risk_list|macro_framework|market_update"`,
		"Node discourse roles:",
		"Every node must include a `role`",
		"`analogy`",
		"`satire_target`",
		`"role":"thesis|mechanism|evidence|example|implication|caveat|market_move|analogy|satire_target|implied_thesis"`,
		"do not demote the central allegory",
		"U.S. trade policy is causing a realignment of global economic relations",
		"Barings基金赎回请求仅满足44.3%",
		"Barings基金每季度最多允许5%赎回",
		"Barings基金投资者资金被锁定无法取出",
	} {
		if !contains(body, want) {
			t.Fatalf("stage1 prompt missing %q", want)
		}
	}
}
func TestExtractSchemaRequiresArticleFormAndNodeRole(t *testing.T) {
	schema := stageJSONSchema("extract")
	if schema == nil {
		t.Fatal("extract schema is nil")
	}
	if !containsString(schema.Required, "article_form") {
		t.Fatalf("extract schema required = %#v, want article_form", schema.Required)
	}
	nodes, ok := schema.Properties["nodes"].(map[string]any)
	if !ok {
		t.Fatalf("nodes schema missing: %#v", schema.Properties)
	}
	items, ok := nodes["items"].(map[string]any)
	if !ok {
		t.Fatalf("nodes items schema missing: %#v", nodes)
	}
	required, ok := items["required"].([]string)
	if !ok {
		t.Fatalf("node required schema missing: %#v", items)
	}
	if !containsString(required, "role") {
		t.Fatalf("node required = %#v, want role", required)
	}
}
