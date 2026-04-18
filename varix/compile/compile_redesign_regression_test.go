package compile

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kumaloha/forge/llm"
)

func TestCompileRedesignPromptCorpusIncludesThreeStageContracts(t *testing.T) {
	promptsDir := newPromptRegistry("").promptsDir
	root := filepath.Join(promptsDir, "compile")

	var corpus strings.Builder
	if err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || filepath.Ext(path) != ".tmpl" {
			return nil
		}
		body, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		corpus.WriteByte('\n')
		corpus.Write(body)
		return nil
	}); err != nil {
		t.Fatalf("walk prompt corpus: %v", err)
	}

	normalized := strings.ToLower(corpus.String())
	for _, want := range []string{
		"driver / target",
		"transmission path",
		"evidence / explanation",
		"`transmission_paths`",
		"`evidence_nodes`",
		"`explanation_nodes`",
		"main causal spine",
		"auxiliary layer",
	} {
		if !strings.Contains(normalized, strings.ToLower(want)) {
			t.Fatalf("compile prompt corpus missing %q", want)
		}
	}
}

func TestClientCompileRedesignRunsThreeGeneratorChallengeJudgeStagesBeforeVerification(t *testing.T) {
	provider := &compileMockProvider{responses: append(compileStageResponses(t,
		`{"summary":"一句话","graph":{"nodes":[{"id":"n1","kind":"事实","text":"事实A","occurred_at":"2026-04-14T00:00:00Z"},{"id":"n2","kind":"结论","text":"结论B"}],"edges":[{"from":"n1","to":"n2","kind":"推出"}]},"details":{"caveats":["待确认"]},"topics":["topic"],"confidence":"medium"}`, "compile-model"), []llm.ProviderResponse{
		{Text: `{"fact_checks":[{"node_id":"n1","status":"clearly_true","reason":"supported"}]}`, Model: "fact-claim-model"},
		{Text: `{"challenges":[{"node_id":"n1","assessment":"supported","reason":"claim seems grounded"}]}`, Model: "fact-challenge-model"},
		{Text: `{"fact_checks":[{"node_id":"n1","status":"clearly_true","reason":"supported"}]}`, Model: "fact-judge-model"},
	}...)}
	client := NewClientWithRuntime(newTestRuntime(provider, "compile-model"), "compile-model")

	if _, err := client.Compile(context.Background(), Bundle{
		UnitID:     "web:redesign",
		Source:     "web",
		ExternalID: "redesign",
		Content:    "root body",
	}); err != nil {
		t.Fatalf("Compile() error = %v", err)
	}

	if got, want := len(provider.requests), 12; got != want {
		t.Fatalf("provider calls = %d, want %d for 9 compile stages + 3 verifier stages", got, want)
	}

	stageChecks := []struct {
		index    int
		keywords []string
	}{
		{0, []string{"driver", "target", "generator"}},
		{1, []string{"driver", "target", "challenge"}},
		{2, []string{"driver", "target", "judge"}},
		{3, []string{"transmission path", "generator"}},
		{4, []string{"transmission path", "challenge"}},
		{5, []string{"transmission path", "judge"}},
		{6, []string{"evidence", "explanation", "generator"}},
		{7, []string{"evidence", "explanation", "challenge"}},
		{8, []string{"evidence", "explanation", "judge"}},
	}
	for _, check := range stageChecks {
		system := strings.ToLower(provider.requests[check.index].System)
		for _, keyword := range check.keywords {
			if !strings.Contains(system, keyword) {
				t.Fatalf("compile stage %d system prompt missing %q in %q", check.index+1, keyword, provider.requests[check.index].System)
			}
		}
	}
}
