package compilev2

import "testing"

func TestStage2SupportPromptFindsSingleDirectionAuxiliaryLinks(t *testing.T) {
	loader := newPromptLoader("")
	body, err := loader.render("support_system.tmpl", nil)
	if err != nil {
		t.Fatalf("render(support_system.tmpl) error = %v", err)
	}
	for _, want := range []string{
		"Your job is only to find single-direction auxiliary links:",
		"`from` = the auxiliary/supporting node.",
		"`to` = the node being served",
		"Allowed kinds:",
		"`evidence`: A proves, documents, or gives factual support for B.",
		"`inference`: A is a premise, precedent, indicator, or policy clue",
		"`explanation`: A explains why/how B is true without being the next downstream outcome",
		"`supplementary`: A is a local side, symptom, numeric face, rule, threshold, case detail, or concrete manifestation of B.",
		"Do not output mainline drive edges here.",
		"Do not choose branch heads here.",
		"Do not merge nodes here.",
		"If A naturally reads as \"therefore / then / which leads to B\", do not output it as support.",
		"How to orient inference:",
		"1946至1974年美国通过负实际利率压低债务率",
		"沃什主张大幅降息",
		"Return JSON only:",
		"support_edges",
	} {
		if !contains(body, want) {
			t.Fatalf("support prompt missing %q", want)
		}
	}
}
func TestStage2SupportUserPromptIncludesArticleContext(t *testing.T) {
	loader := newPromptLoader("")
	body, err := loader.render("support_user.tmpl", map[string]any{
		"Nodes":   "n1 | A | role= | ontology= | quote=q1",
		"Article": "full article text",
	})
	if err != nil {
		t.Fatalf("render(support_user.tmpl) error = %v", err)
	}
	for _, want := range []string{
		"n1 | A | role= | ontology= | quote=q1",
		"Full article (for discourse context only; do not invent new nodes):",
		"full article text",
	} {
		if !contains(body, want) {
			t.Fatalf("support user prompt missing %q", want)
		}
	}
	if contains(body, "Extract candidate edges") {
		t.Fatal("support user prompt unexpectedly contains candidate edges section")
	}
}
func TestSupportFormationChainIsReservedForMainline(t *testing.T) {
	edge := auxEdge{
		From:        "n1",
		To:          "n2",
		Kind:        "supplementary",
		SourceQuote: "油价、通胀、利率、利息形成财政紧缩螺旋",
	}
	from := graphNode{ID: "n1", Text: "油价上涨", SourceQuote: edge.SourceQuote}
	to := graphNode{ID: "n2", Text: "财政紧缩螺旋形成", SourceQuote: edge.SourceQuote}

	if !isLikelyMainlineAuxEdge(edge, from, to) {
		t.Fatal("formation chain should be reserved for mainline instead of collapsed as auxiliary support")
	}
}
func TestSupportBetweenTwoOutcomeLikeNodesFallsBackToSupplementHeuristic(t *testing.T) {
	from := graphNode{ID: "n1", Text: "Foreign portfolio inflows into US assets remain huge", Role: roleTransmission, Ontology: "flow", IsTarget: true}
	to := graphNode{ID: "n2", Text: "\"Sell America\" trade never existed", Role: roleTransmission, Ontology: "flow", IsTarget: true}
	if !shouldDemoteSupportToSupplement(from, to) {
		t.Fatal("expected support link to be demoted to supplement for two outcome-like nodes")
	}
	primary, secondary := chooseSupplementPrimary(from, to)
	if primary != "n1" || secondary.ID != "n2" {
		t.Fatalf("got primary=%s secondary=%s, want primary=n1 secondary=n2", primary, secondary.ID)
	}
}
