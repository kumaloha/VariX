package compilev2

import "testing"

func TestPromptConstantsPresentCoreInstructions(t *testing.T) {
	for _, tc := range []struct {
		name string
		body string
		want []string
	}{
		{
			name: "stage1",
			body: stage1SystemPrompt,
			want: []string{"single-sided", "off_graph", "Keep node text in the article's original language"},
		},
		{
			name: "stage3",
			body: stage3SystemPrompt,
			want: []string{"market outcome", "price", "flow", "decision"},
		},
		{
			name: "translate",
			body: stage5TranslateSystemPrompt,
			want: []string{"financial-Chinese translator", "already-Chinese", "translations"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, want := range tc.want {
				if !contains(tc.body, want) {
					t.Fatalf("prompt missing %q", want)
				}
			}
		})
	}
}

func contains(s, sub string) bool { return len(sub) == 0 || (len(s) >= len(sub) && stringIndex(s, sub) >= 0) }

func stringIndex(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
