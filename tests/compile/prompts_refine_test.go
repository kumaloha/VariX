package compile

import "testing"

func TestRefinePromptSplitsCausalAndParallelNodes(t *testing.T) {
	loader := newPromptLoader("")
	body, err := loader.render("refine_system.tmpl", nil)
	if err != nil {
		t.Fatalf("render(refine_system.tmpl) error = %v", err)
	}
	for _, want := range []string{
		"Only patch nodes that are structurally inaccurate",
		"Split any node that contains more than one independently meaningful `subject + change/state` unit.",
		"Do not output any edges.",
		"导致, 引发, 触发",
		"和, 及, 以及, 与",
		"Return JSON only:",
	} {
		if !contains(body, want) {
			t.Fatalf("refine prompt missing %q", want)
		}
	}
}
func TestRefineUserPromptIncludesArticleContext(t *testing.T) {
	loader := newPromptLoader("")
	body, err := loader.render("refine_user.tmpl", map[string]any{
		"Article": "full article text for refine",
		"Nodes":   "n1 | 高利率压低股票和债券价格 | quote=高利率压低股票和债券价格",
	})
	if err != nil {
		t.Fatalf("render(refine_user.tmpl) error = %v", err)
	}
	for _, want := range []string{
		"Article:",
		"full article text for refine",
		"Candidate nodes:",
		"n1 | 高利率压低股票和债券价格",
	} {
		if !contains(body, want) {
			t.Fatalf("refine user prompt missing %q", want)
		}
	}
}
