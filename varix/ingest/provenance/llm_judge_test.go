package provenance

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/kumaloha/VariX/varix/ingest/types"
	"github.com/kumaloha/forge/llm"
)

type llmJudgeProvider struct{ text string }

func (p *llmJudgeProvider) Name() string { return "llm-judge-test" }

func (p *llmJudgeProvider) Call(_ context.Context, req llm.ProviderRequest) (llm.ProviderResponse, error) {
	return llm.ProviderResponse{Text: p.text, Model: req.Model}, nil
}

func TestLLMJudge_Judge(t *testing.T) {
	judge := newTestLLMJudge(t, `{"status":"found","canonical_source_url":"https://source.example/original","match_kind":"likely_derived","base_relation":"translation","editorial_layer":"commentary","fidelity":"likely_adapted","reasoning":"Cross-platform candidate with derivation markers is likely the source."}`)
	result, err := judge.Judge(context.Background(), types.RawContent{URL: "https://mirror.example/post", Content: "translated commentary"}, []types.SourceCandidate{{URL: "https://source.example/original", Host: "source.example", Kind: "embedded_link", Confidence: "high"}})
	if err != nil {
		t.Fatalf("Judge() error = %v", err)
	}
	if result.Lookup.ResolvedBy != "llm_judge" {
		t.Fatalf("ResolvedBy = %q, want llm_judge", result.Lookup.ResolvedBy)
	}
	if result.Lookup.MatchKind != types.SourceMatchLikelyDerived {
		t.Fatalf("MatchKind = %q, want likely_derived", result.Lookup.MatchKind)
	}
	if result.BaseRelation != types.BaseRelationTranslation {
		t.Fatalf("BaseRelation = %q, want translation", result.BaseRelation)
	}
}

func TestLLMJudge_RejectsCanonicalURLOutsideCandidates(t *testing.T) {
	judge := newTestLLMJudge(t, `{"status":"found","canonical_source_url":"https://hallucinated.example/source","match_kind":"likely_derived","base_relation":"translation","editorial_layer":"commentary","fidelity":"likely_adapted","reasoning":"wrong source"}`)
	_, err := judge.Judge(context.Background(), types.RawContent{URL: "https://mirror.example/post", Content: "translated commentary"}, []types.SourceCandidate{{URL: "https://source.example/original", Host: "source.example", Kind: "embedded_link", Confidence: "high"}})
	if err == nil {
		t.Fatal("Judge() error = nil, want candidate-membership failure")
	}
}

func TestLLMJudge_RejectsInvalidEnums(t *testing.T) {
	judge := newTestLLMJudge(t, `{"status":"found","canonical_source_url":"https://source.example/original","match_kind":"likely_derived","base_relation":"made_up","editorial_layer":"commentary","fidelity":"likely_adapted","reasoning":"bad enum"}`)
	_, err := judge.Judge(context.Background(), types.RawContent{URL: "https://mirror.example/post", Content: "translated commentary"}, []types.SourceCandidate{{URL: "https://source.example/original", Host: "source.example", Kind: "embedded_link", Confidence: "high"}})
	if err == nil {
		t.Fatal("Judge() error = nil, want validation failure")
	}
}

func newTestLLMJudge(t *testing.T, response string) *LLMJudge {
	t.Helper()
	root := t.TempDir()
	promptPath := filepath.Join(root, "prompts", "ingest", "provenance", "source_match_judge.yaml")
	if err := os.MkdirAll(filepath.Dir(promptPath), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(promptPath, []byte(`id: ingest/provenance/source_match_judge
version: v1
role: Judge source provenance matches.
output_format: |
  Output JSON:
  {
    "status": "found|not_found|failed",
    "canonical_source_url": "...",
    "match_kind": "same_source|likely_derived|unrelated",
    "base_relation": "translation|excerpt|summary|compilation|interview_recut|original|repost|quote|unknown",
    "editorial_layer": "none|commentary|analysis|reaction|framing|unknown",
    "fidelity": "unknown|partial|likely_faithful|likely_adapted",
    "reasoning": "..."
  }
`), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	rt := llm.NewRuntime(llm.RuntimeConfig{Provider: &llmJudgeProvider{text: response}, LLMConfig: llm.LLMConfig{Default: llm.DefaultConfig{Model: "test-model"}}})
	loader := llm.NewPromptLoaderFromDir(filepath.Join(root, "prompts"))
	judge, err := NewLLMJudge(rt, loader)
	if err != nil {
		t.Fatalf("NewLLMJudge() error = %v", err)
	}
	return judge
}
