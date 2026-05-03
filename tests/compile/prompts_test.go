package compile

import "testing"

func TestPromptTemplatesPresentCoreInstructions(t *testing.T) {
	loader := newPromptLoader("")
	for _, tc := range []struct {
		name string
		file string
		want []string
	}{
		{
			name: "stage1",
			file: "extract_system.tmpl",
			want: []string{"single-sided", "off_graph", "Keep node text in the article's original language"},
		},
		{
			name: "stage3",
			file: "classify_system.tmpl",
			want: []string{"market outcome", "price", "flow", "decision"},
		},
		{
			name: "translate",
			file: "translate_system.tmpl",
			want: []string{"financial-Chinese translator", "already-Chinese", "translations"},
		},
		{
			name: "summary",
			file: "summary_system.tmpl",
			want: []string{"satirical_financial_commentary", "do not call it `论文`", "mapped critique mechanism"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body, err := loader.render(tc.file, nil)
			if err != nil {
				t.Fatalf("render(%q) error = %v", tc.file, err)
			}
			for _, want := range tc.want {
				if !contains(body, want) {
					t.Fatalf("prompt missing %q", want)
				}
			}
		})
	}
}
func TestPromptTemplatesIncludeBoundaryFewShot(t *testing.T) {
	loader := newPromptLoader("")
	for _, tc := range []struct {
		name string
		file string
		want []string
	}{
		{
			name: "classify",
			file: "classify_system.tmpl",
			want: []string{"Boundary few-shot examples", "Example 1", "Input:", "Output:"},
		},
		{
			name: "aggregate",
			file: "aggregate_system.tmpl",
			want: []string{"Boundary few-shot examples", "Example 1", "Input:", "Output:"},
		},
		{
			name: "support",
			file: "support_system.tmpl",
			want: []string{"Boundary few-shot examples", "Example 1", "Input:", "Output:"},
		},
		{
			name: "mainline",
			file: "mainline_system.tmpl",
			want: []string{"Boundary few-shot examples", "Example 1", "Input:", "Output:"},
		},
		{
			name: "stage4",
			file: "validate_system.tmpl",
			want: []string{"Boundary few-shot examples", "Example 1", "Input:", "Output:"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body, err := loader.render(tc.file, nil)
			if err != nil {
				t.Fatalf("render(%q) error = %v", tc.file, err)
			}
			for _, want := range tc.want {
				if !contains(body, want) {
					t.Fatalf("prompt missing %q", want)
				}
			}
		})
	}
}
func TestMainlineUpstreamUserPromptsIncludeArticleContext(t *testing.T) {
	loader := newPromptLoader("")
	for _, tc := range []struct {
		name string
		file string
		data map[string]any
	}{
		{
			name: "extract",
			file: "extract_user.tmpl",
			data: map[string]any{"Article": "article context sentinel"},
		},
		{
			name: "refine",
			file: "refine_user.tmpl",
			data: map[string]any{"Article": "article context sentinel", "Nodes": "n1 | node"},
		},
		{
			name: "aggregate",
			file: "aggregate_user.tmpl",
			data: map[string]any{"Article": "article context sentinel", "Nodes": "Group 1 quote: q\n- n1: node"},
		},
		{
			name: "support",
			file: "support_user.tmpl",
			data: map[string]any{"Article": "article context sentinel", "Nodes": "n1 | node | role= | ontology= | quote=q"},
		},
		{
			name: "mainline",
			file: "mainline_user.tmpl",
			data: map[string]any{
				"Article":        "article context sentinel",
				"ArticleForm":    "risk_list",
				"SpinePolicy":    "risk_list policy sentinel",
				"Nodes":          "n1 | node | role= | ontology= | quote=q",
				"BranchHeads":    "n1 | node",
				"SemanticUnits":  "(none)",
				"CandidateEdges": "- (none)",
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body, err := loader.render(tc.file, tc.data)
			if err != nil {
				t.Fatalf("render(%s) error = %v", tc.file, err)
			}
			if !contains(body, "article context sentinel") {
				t.Fatalf("%s prompt missing article context:\n%s", tc.name, body)
			}
		})
	}
}
