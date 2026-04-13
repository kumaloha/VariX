package provenance

import (
	"context"
	"fmt"
	"strings"

	"github.com/kumaloha/VariX/varix/ingest/types"
	"github.com/kumaloha/forge/llm"
)

const promptSourceMatchJudge = "ingest/provenance/source_match_judge"

type LLMJudge struct {
	runner *llm.FacetRunner[sourceMatchJudgeResult]
}

type sourceMatchJudgeResult struct {
	Status             string `json:"status"`
	CanonicalSourceURL string `json:"canonical_source_url,omitempty"`
	MatchKind          string `json:"match_kind"`
	BaseRelation       string `json:"base_relation,omitempty"`
	EditorialLayer     string `json:"editorial_layer,omitempty"`
	Fidelity           string `json:"fidelity,omitempty"`
	Reasoning          string `json:"reasoning"`
}

func (r sourceMatchJudgeResult) Validate() error {
	switch r.Status {
	case string(types.SourceLookupStatusFound), string(types.SourceLookupStatusNotFound), string(types.SourceLookupStatusFailed):
	default:
		return fmt.Errorf("invalid status %q", r.Status)
	}
	switch r.MatchKind {
	case string(types.SourceMatchSameSource), string(types.SourceMatchLikelyDerived), string(types.SourceMatchUnrelated):
	default:
		return fmt.Errorf("invalid match_kind %q", r.MatchKind)
	}
	if !validBaseRelation(r.BaseRelation) {
		return fmt.Errorf("invalid base_relation %q", r.BaseRelation)
	}
	if !validEditorialLayer(r.EditorialLayer) {
		return fmt.Errorf("invalid editorial_layer %q", r.EditorialLayer)
	}
	if !validFidelity(r.Fidelity) {
		return fmt.Errorf("invalid fidelity %q", r.Fidelity)
	}
	if strings.TrimSpace(r.Reasoning) == "" {
		return fmt.Errorf("reasoning must not be empty")
	}
	if r.Status == string(types.SourceLookupStatusFound) && strings.TrimSpace(r.CanonicalSourceURL) == "" {
		return fmt.Errorf("canonical_source_url required when status=found")
	}
	if r.Status != string(types.SourceLookupStatusFound) && strings.TrimSpace(r.CanonicalSourceURL) != "" {
		return fmt.Errorf("canonical_source_url must be empty unless status=found")
	}
	switch r.Status {
	case string(types.SourceLookupStatusFound):
		if r.MatchKind == string(types.SourceMatchUnrelated) {
			return fmt.Errorf("match_kind unrelated is incompatible with status=found")
		}
	case string(types.SourceLookupStatusNotFound), string(types.SourceLookupStatusFailed):
		if r.MatchKind != string(types.SourceMatchUnrelated) {
			return fmt.Errorf("status %q requires match_kind=unrelated", r.Status)
		}
		if r.BaseRelation != "" || r.EditorialLayer != "" || r.Fidelity != "" {
			return fmt.Errorf("status %q must not carry provenance fields", r.Status)
		}
	}
	return nil
}

func NewLLMJudge(rt *llm.Runtime, loader *llm.PromptLoader) (*LLMJudge, error) {
	runner, err := llm.NewFacetRunner[sourceMatchJudgeResult](rt, loader, promptSourceMatchJudge)
	if err != nil {
		return nil, fmt.Errorf("provenance: new llm judge: %w", err)
	}
	return &LLMJudge{runner: runner}, nil
}

func (j *LLMJudge) Judge(ctx context.Context, raw types.RawContent, candidates []types.SourceCandidate) (MatchResult, error) {
	if j == nil || j.runner == nil {
		return MatchResult{}, fmt.Errorf("provenance: llm judge is not initialized")
	}
	if len(candidates) == 0 {
		return MatchResult{
			Lookup: types.SourceLookupState{
				Status:     types.SourceLookupStatusNotFound,
				ResolvedBy: "llm_judge",
				MatchKind:  types.SourceMatchUnrelated,
			},
		}, nil
	}

	promptInput := map[string]any{
		"input": buildSourceMatchJudgePromptInput(raw, candidates),
	}
	resp, err := j.runner.Run(ctx, promptInput, llm.ExtractOptions[sourceMatchJudgeResult]{})
	if err != nil {
		return MatchResult{}, fmt.Errorf("provenance: llm judge: %w", err)
	}
	if err := ensureCandidateURL(resp.Value.CanonicalSourceURL, candidates, resp.Value.Status); err != nil {
		return MatchResult{}, fmt.Errorf("provenance: llm judge: %w", err)
	}
	return matchResultFromLLM(resp.Value), nil
}

func buildSourceMatchJudgePromptInput(raw types.RawContent, candidates []types.SourceCandidate) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Raw URL: %s\n", raw.URL)
	fmt.Fprintf(&b, "Raw source: %s\n", raw.Source)
	fmt.Fprintf(&b, "Expanded text:\n%s\n", raw.ExpandedText())
	if raw.Provenance != nil {
		fmt.Fprintf(&b, "Current provenance: base_relation=%s editorial_layer=%s fidelity=%s\n",
			raw.Provenance.BaseRelation, raw.Provenance.EditorialLayer, raw.Provenance.Fidelity)
	}
	fmt.Fprintf(&b, "Candidates:\n")
	for i, candidate := range candidates {
		fmt.Fprintf(&b, "%d. url=%s host=%s kind=%s confidence=%s\n", i+1, candidate.URL, candidate.Host, candidate.Kind, candidate.Confidence)
	}
	fmt.Fprintf(&b, "If you return status=found, canonical_source_url must exactly match one candidate URL from the list above.\n")
	return b.String()
}

func matchResultFromLLM(result sourceMatchJudgeResult) MatchResult {
	return MatchResult{
		Lookup: types.SourceLookupState{
			Status:             types.SourceLookupStatus(result.Status),
			CanonicalSourceURL: result.CanonicalSourceURL,
			ResolvedBy:         "llm_judge",
			MatchKind:          types.SourceMatchKind(result.MatchKind),
		},
		BaseRelation:   types.BaseRelation(result.BaseRelation),
		EditorialLayer: types.EditorialLayer(result.EditorialLayer),
		Fidelity:       types.Fidelity(result.Fidelity),
	}
}

func ensureCandidateURL(canonicalSourceURL string, candidates []types.SourceCandidate, status string) error {
	canonicalSourceURL = strings.TrimSpace(canonicalSourceURL)
	if status != string(types.SourceLookupStatusFound) {
		return nil
	}
	for _, candidate := range candidates {
		if strings.TrimSpace(candidate.URL) == canonicalSourceURL {
			return nil
		}
	}
	return fmt.Errorf("canonical_source_url %q is not one of the provided candidates", canonicalSourceURL)
}

func validBaseRelation(value string) bool {
	switch types.BaseRelation(value) {
	case "", types.BaseRelationOriginal, types.BaseRelationRepost, types.BaseRelationQuote, types.BaseRelationExcerpt,
		types.BaseRelationTranslation, types.BaseRelationSummary, types.BaseRelationCompilation,
		types.BaseRelationInterviewRecut, types.BaseRelationUnknown:
		return true
	default:
		return false
	}
}

func validEditorialLayer(value string) bool {
	switch types.EditorialLayer(value) {
	case "", types.EditorialLayerNone, types.EditorialLayerCommentary, types.EditorialLayerAnalysis,
		types.EditorialLayerReaction, types.EditorialLayerFraming, types.EditorialLayerUnknown:
		return true
	default:
		return false
	}
}

func validFidelity(value string) bool {
	switch types.Fidelity(value) {
	case "", types.FidelityUnknown, types.FidelityPartial, types.FidelityLikelyFaithful, types.FidelityLikelyAdapted:
		return true
	default:
		return false
	}
}
