package verify

import "testing"

func TestUnifiedPromptTemplatesIncludeBoundaryFewShot(t *testing.T) {
	registry := newPromptRegistry("")
	for _, tc := range []struct {
		name string
		file string
		want []string
	}{
		{
			name: "generator",
			file: "compile/unified_generator_system.tmpl",
			want: []string{"Boundary few-shot examples", "Example 1", "Input signal", "Expected handling"},
		},
		{
			name: "challenge",
			file: "compile/unified_challenge_system.tmpl",
			want: []string{"Boundary few-shot examples", "Example 1", "Failure pattern", "Challenge action"},
		},
		{
			name: "judge",
			file: "compile/unified_judge_system.tmpl",
			want: []string{"Boundary few-shot examples", "Example 1", "Draft problem", "Expected finalization"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body, err := registry.render(tc.file, nil)
			if err != nil {
				t.Fatalf("render(%q) error = %v", tc.file, err)
			}
			for _, want := range tc.want {
				if !containsPromptText(body, want) {
					t.Fatalf("prompt %q missing %q", tc.file, want)
				}
			}
		})
	}
}

func containsPromptText(s, sub string) bool {
	return len(sub) == 0 || (len(s) >= len(sub) && promptTextIndex(s, sub) >= 0)
}

func promptTextIndex(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
