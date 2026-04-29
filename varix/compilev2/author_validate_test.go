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
				{"claim_id":"claim-001","text":"Rates fall","claim_type":"fact","status":"supported","evidence":["source says rates fell"],"reason":"The author states this and it matches available evidence.","subclaims":[{"text":"Rates fall","subject":"policy rate","metric":"change","status":"supported","evidence":["source says rates fell"],"reason":"Matched source."}]},
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
			Summary:            "Rates falling supports a stock rally forecast.",
			Drivers:            []string{"Rates fall"},
			Targets:            []string{"Stocks will rise"},
			EvidenceNodes:      []string{"Policy rate declined by 25 bps"},
			SupplementaryNodes: []string{"Central bank statement confirms the cut"},
			Details: compile.HiddenDetails{
				QuoteHighlights:     []string{"The bank cut rates by 25 bps"},
				ReferenceHighlights: []string{"Central bank release"},
			},
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
	if len(result.Render.AuthorValidation.ClaimChecks[0].Subclaims) != 1 {
		t.Fatalf("subclaims = %#v, want preserved proof subclaim", result.Render.AuthorValidation.ClaimChecks[0].Subclaims)
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
	userPrompt := req.UserParts[len(req.UserParts)-1].Text
	for _, want := range []string{"proof_point", "supplementary_proof", "source_quote", "reference_proof"} {
		if !strings.Contains(userPrompt, want) {
			t.Fatalf("user prompt missing proof candidate kind %q: %s", want, userPrompt)
		}
	}
	for _, want := range []string{"Split compound proof points", "normalize units", "range_covered", "attribution_ok"} {
		if !strings.Contains(req.System, want) {
			t.Fatalf("system prompt missing %q: %s", want, req.System)
		}
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

func TestNormalizeAuthorValidationPreservesModelSplitClaims(t *testing.T) {
	validation := normalizeAuthorValidation(compile.AuthorValidation{
		ClaimChecks: []compile.AuthorClaimCheck{
			{
				ClaimID: "claim-001",
				Text:    "NVL72 has 5000+ copper cables and weighs 1.36 tons",
				Status:  compile.AuthorClaimSupported,
				Subclaims: []compile.AuthorSubclaim{
					{
						Text:          "NVL72 uses more than 5000 copper cables",
						Subject:       "NVL72",
						Metric:        "copper cable count",
						OriginalValue: "5000+",
						EvidenceValue: "5184",
						Status:        compile.AuthorClaimSupported,
						Reason:        "Public sources report 5184 or more than 5000 cables.",
					},
					{
						Text:          "NVL72 copper cables weigh 1.36 tons",
						Subject:       "copper cables",
						Metric:        "weight",
						OriginalValue: "1.36 tons",
						EvidenceValue: "1.36 tons rack weight",
						AttributionOK: false,
						Status:        compile.AuthorClaimContradicted,
						Reason:        "The matched value describes rack weight, not cable weight.",
					},
				},
			},
			{
				ClaimID: "claim-001-a",
				Text:    "NVL72 rack weighs 1.36 tons",
				Status:  compile.AuthorClaimSupported,
				Subclaims: []compile.AuthorSubclaim{{
					Text:          "NVL72 rack weighs 1.36 tons",
					Subject:       "NVL72 rack",
					Metric:        "weight",
					OriginalValue: "1.36 tons",
					EvidenceValue: "1.36 metric tons",
					Status:        compile.AuthorClaimSupported,
				}},
			},
		},
	}, []authorClaimCandidate{{ClaimID: "claim-001", Text: "NVL72 has 5000+ copper cables and weighs 1.36 tons"}}, nil, "model")

	if len(validation.ClaimChecks) != 2 {
		t.Fatalf("claim checks = %#v, want original plus model-split claim preserved", validation.ClaimChecks)
	}
	if validation.ClaimChecks[0].Status != compile.AuthorClaimContradicted {
		t.Fatalf("compound claim status = %q, want contradicted due misattributed subclaim", validation.ClaimChecks[0].Status)
	}
	if len(validation.ClaimChecks[0].Subclaims) != 2 {
		t.Fatalf("subclaims = %#v, want two split proof subclaims", validation.ClaimChecks[0].Subclaims)
	}
	if validation.ClaimChecks[0].Subclaims[1].AttributionOK {
		t.Fatalf("misattributed subclaim attribution_ok = true, want false")
	}
	if validation.ClaimChecks[1].ClaimID != "claim-001-a" {
		t.Fatalf("extra split claim = %#v, want preserved model-created split claim", validation.ClaimChecks[1])
	}
}

func TestNormalizeAuthorValidationAggregatesRangeCoveredSubclaims(t *testing.T) {
	validation := normalizeAuthorValidation(compile.AuthorValidation{
		ClaimChecks: []compile.AuthorClaimCheck{{
			ClaimID: "claim-001",
			Text:    "Transformer delivery lead times stretched to 100 weeks",
			Status:  compile.AuthorClaimUnverified,
			Subclaims: []compile.AuthorSubclaim{{
				Text:            "Transformer delivery lead times stretched to 100 weeks",
				Subject:         "large power transformer",
				Metric:          "delivery lead time",
				OriginalValue:   "100 weeks",
				NormalizedValue: "about 23 months",
				EvidenceRange:   "18-36 months",
				UnitNormalized:  true,
				RangeCovered:    true,
				AttributionOK:   true,
				Status:          compile.AuthorClaimSupported,
				Reason:          "100 weeks is inside the public 18-36 month range.",
			}},
		}},
	}, []authorClaimCandidate{{ClaimID: "claim-001", Text: "Transformer delivery lead times stretched to 100 weeks"}}, nil, "model")

	if validation.ClaimChecks[0].Status != compile.AuthorClaimSupported {
		t.Fatalf("claim status = %q, want supported from range-covered subclaim", validation.ClaimChecks[0].Status)
	}
	subclaim := validation.ClaimChecks[0].Subclaims[0]
	if !subclaim.UnitNormalized || !subclaim.RangeCovered {
		t.Fatalf("subclaim = %#v, want unit_normalized and range_covered", subclaim)
	}
	if validation.Summary.Verdict != "credible" {
		t.Fatalf("verdict = %q, want credible", validation.Summary.Verdict)
	}
}
