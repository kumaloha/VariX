package compilev2

import (
	"context"
	"strings"
	"testing"

	"github.com/kumaloha/VariX/varix/compile"
	"github.com/kumaloha/forge/llm"
)

func TestAuthorValidatePreviewResultValidatesAuthorOnlyWithSearch(t *testing.T) {
	rt := &fakeRuntime{responses: []llm.Response{{
		Text: `{
			"summary":{"verdict":"mixed"},
			"claim_checks":[
				{"claim_id":"claim-001","text":"Rates fall","claim_type":"fact","status":"supported","evidence":["source says rates fell"],"reason":"The author states this and it matches available evidence."},
				{"claim_id":"claim-002","text":"Stocks will rise","claim_type":"forecast","status":"interpretive","reason":"This is an unresolved forecast."}
			],
			"inference_checks":[
				{"inference_id":"inference-001","from":"Rates fall","to":"Stocks will rise","steps":["Liquidity improves"],"status":"weak","reason":"The mechanism is plausible but not established."}
			]
		}`,
	}}}
	client := &Client{runtime: rt, model: "author-model"}
	result, err := client.AuthorValidatePreviewResult(context.Background(), compile.Bundle{
		UnitID:     "twitter:author-validate",
		Source:     "twitter",
		ExternalID: "author-validate",
		URL:        "https://x.com/example/status/1",
		Content:    "The author says rates fell, so liquidity improves and stocks will rise.",
	}, FlowPreviewResult{
		ArticleForm: "forecast_thread",
		Render: compile.Output{
			Summary:           "Rates falling supports a stock rally forecast.",
			Drivers:           []string{"Rates fall"},
			Targets:           []string{"Stocks will rise"},
			TransmissionPaths: []compile.TransmissionPath{{Driver: "Rates fall", Steps: []string{"Liquidity improves"}, Target: "Stocks will rise"}},
		},
	})
	if err != nil {
		t.Fatalf("AuthorValidatePreviewResult() error = %v", err)
	}
	if result.AuthorValidation == nil || result.Render.AuthorValidation.IsZero() {
		t.Fatalf("author validation missing from result: %#v", result)
	}
	if result.Render.AuthorValidation.Summary.Verdict != "mixed" {
		t.Fatalf("verdict = %q, want mixed", result.Render.AuthorValidation.Summary.Verdict)
	}
	if result.Render.AuthorValidation.Summary.SupportedClaims != 1 || result.Render.AuthorValidation.Summary.InterpretiveClaims != 1 {
		t.Fatalf("summary = %#v, want supported and interpretive claim counts", result.Render.AuthorValidation.Summary)
	}
	if result.Render.AuthorValidation.Summary.WeakInferences != 1 {
		t.Fatalf("summary = %#v, want weak inference count", result.Render.AuthorValidation.Summary)
	}
	if got := result.Metrics["author_validate_ms"]; got < 0 {
		t.Fatalf("author_validate_ms = %d", got)
	}
	if len(rt.requests) != 1 {
		t.Fatalf("requests = %d, want one author validation request", len(rt.requests))
	}
	req := rt.requests[0]
	if !req.Search {
		t.Fatal("author validation request Search = false, want true for author fact checks")
	}
	if !strings.Contains(req.System, "Validate ONLY what the author claims") || !strings.Contains(req.System, "Do not critique the extraction pipeline") {
		t.Fatalf("system prompt = %q, want author-only validation boundary", req.System)
	}
	if len(req.UserParts) == 0 || !strings.Contains(req.UserParts[len(req.UserParts)-1].Text, "claim_candidates") || !strings.Contains(req.UserParts[len(req.UserParts)-1].Text, "inference_candidates") {
		t.Fatalf("user prompt missing candidates: %#v", req.UserParts)
	}
}

func TestNormalizeAuthorValidationBackfillsMissingCandidates(t *testing.T) {
	validation := normalizeAuthorValidation(compile.AuthorValidation{
		ClaimChecks: []compile.AuthorClaimCheck{{ClaimID: "claim-001", Status: compile.AuthorClaimSupported}},
	}, []authorClaimCandidate{
		{ClaimID: "claim-001", Text: "Fact A"},
		{ClaimID: "claim-002", Text: "Fact B"},
	}, []authorInferenceCandidate{
		{InferenceID: "inference-001", From: "Fact A", To: "Fact B"},
	}, "model")
	if len(validation.ClaimChecks) != 2 {
		t.Fatalf("claim checks = %#v, want backfilled missing candidate", validation.ClaimChecks)
	}
	if validation.ClaimChecks[1].Status != compile.AuthorClaimUnverified {
		t.Fatalf("missing claim status = %q, want unverified", validation.ClaimChecks[1].Status)
	}
	if len(validation.InferenceChecks) != 1 || validation.InferenceChecks[0].Status != compile.AuthorInferenceWeak {
		t.Fatalf("inference checks = %#v, want weak backfill", validation.InferenceChecks)
	}
	if validation.Summary.Verdict != "mixed" {
		t.Fatalf("verdict = %q, want mixed", validation.Summary.Verdict)
	}
}
