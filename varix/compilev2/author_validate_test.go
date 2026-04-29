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
				Items: []map[string]any{{
					"kind":         "proof_point",
					"text":         "Policy rate declined by 25 bps",
					"source_text":  "Policy rate declined by 25 bps",
					"source_quote": "The bank cut rates by 25 bps",
					"role":         "evidence",
				}},
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
	if !strings.Contains(userPrompt, `"source_quote": "The bank cut rates by 25 bps"`) {
		t.Fatalf("user prompt missing proof provenance source_quote: %s", userPrompt)
	}
	for _, want := range []string{"Split compound proof points", "normalize units", "range_covered", "attribution_ok", "comparison_base", "scope_status", "denominator", "do not silently rewrite the subject", "edge_evidence"} {
		if !strings.Contains(req.System, want) {
			t.Fatalf("system prompt missing %q: %s", want, req.System)
		}
	}
}

func TestCollectAuthorClaimCandidatesCarriesProofProvenance(t *testing.T) {
	claims := collectAuthorClaimCandidates(compile.Output{
		EvidenceNodes: []string{"NVL72机柜铜缆总重1.36吨"},
		Details: compile.HiddenDetails{Items: []map[string]any{{
			"kind":         "proof_point",
			"text":         "NVL72机柜铜缆总重1.36吨",
			"source_text":  "NVL72机柜铜缆总重1.36吨",
			"source_quote": "The GB200 NVL72 rack weighs 1.36 metric tons and uses more than 5000 copper cables.",
			"role":         "evidence",
			"attaches_to":  "nvl72",
		}}},
	})
	if len(claims) != 1 {
		t.Fatalf("claims = %#v, want one proof candidate", claims)
	}
	claim := claims[0]
	if claim.Kind != "proof_point" || claim.Text != "NVL72机柜铜缆总重1.36吨" {
		t.Fatalf("claim = %#v, want proof point text preserved", claim)
	}
	if !strings.Contains(claim.SourceQuote, "rack weighs 1.36 metric tons") {
		t.Fatalf("source_quote = %q, want rack-weight provenance", claim.SourceQuote)
	}
	if claim.Role != "evidence" || claim.AttachesTo != "nvl72" {
		t.Fatalf("claim provenance = %#v, want role and attaches_to", claim)
	}
}

func TestRenderOffGraphDetailsPreservesSourceQuote(t *testing.T) {
	details := renderOffGraphDetails([]offGraphItem{{
		ID:          "off1",
		Text:        "NVL72 copper-cable total weight is 1.36 tons",
		Role:        "evidence",
		AttachesTo:  "n1",
		SourceQuote: "The GB200 NVL72 rack weighs 1.36 metric tons.",
	}}, func(id, fallback string) string {
		if id == "off1" {
			return "NVL72机柜铜缆总重1.36吨"
		}
		return fallback
	})
	if len(details) != 1 {
		t.Fatalf("details = %#v, want one item", details)
	}
	item := details[0]
	if item["kind"] != "proof_point" || item["text"] != "NVL72机柜铜缆总重1.36吨" {
		t.Fatalf("item = %#v, want rendered proof detail", item)
	}
	if item["source_quote"] != "The GB200 NVL72 rack weighs 1.36 metric tons." {
		t.Fatalf("source_quote = %#v, want preserved source quote", item["source_quote"])
	}
}

func TestCollectAuthorInferenceCandidatesCarriesEdgeProvenance(t *testing.T) {
	candidates := collectAuthorInferenceCandidates(compile.Output{
		TransmissionPaths: []compile.TransmissionPath{{
			Driver: "Rates fall",
			Steps:  []string{"Liquidity improves"},
			Target: "Stocks rise",
		}},
		Details: compile.HiddenDetails{Items: []map[string]any{{
			"kind":         "inference_path",
			"from":         "Rates fall",
			"steps":        []string{"Liquidity improves"},
			"to":           "Stocks rise",
			"source_quote": "The author says rate cuts improve liquidity and can lift stocks.",
			"edge_evidence": []map[string]any{{
				"from":         "n1",
				"to":           "n2",
				"from_text":    "Rates fall",
				"to_text":      "Liquidity improves",
				"source_quote": "rate cuts improve liquidity",
				"reason":       "lower rates loosen financial conditions",
			}},
		}}},
	})
	if len(candidates) != 1 {
		t.Fatalf("candidates = %#v, want one inference candidate", candidates)
	}
	candidate := candidates[0]
	if !strings.Contains(candidate.SourceQuote, "rate cuts improve liquidity") {
		t.Fatalf("source_quote = %q, want path provenance", candidate.SourceQuote)
	}
	if len(candidate.EdgeEvidence) != 1 {
		t.Fatalf("edge evidence = %#v, want one edge item", candidate.EdgeEvidence)
	}
	if candidate.EdgeEvidence[0].FromText != "Rates fall" || candidate.EdgeEvidence[0].ToText != "Liquidity improves" {
		t.Fatalf("edge evidence = %#v, want rendered endpoint text", candidate.EdgeEvidence[0])
	}
}

func TestRenderTransmissionPathDetailsPreservesEdgeQuotes(t *testing.T) {
	details := renderTransmissionPathDetails([]renderedPath{{
		branchID: "branch-1",
		driver:   graphNode{ID: "n1", Text: "Rates fall"},
		steps:    []graphNode{{ID: "n2", Text: "Liquidity improves"}},
		target:   graphNode{ID: "n3", Text: "Stocks rise"},
		edges: []PreviewEdge{
			{From: "n1", To: "n2", SourceQuote: "lower rates improve liquidity", Reason: "funding costs fall"},
			{From: "n2", To: "n3", SourceQuote: "liquidity can lift stocks"},
		},
	}}, func(id, fallback string) string {
		return fallback
	})
	if len(details) != 1 {
		t.Fatalf("details = %#v, want one inference path detail", details)
	}
	item := details[0]
	if item["kind"] != "inference_path" || item["branch"] != "branch-1" {
		t.Fatalf("item = %#v, want branch inference detail", item)
	}
	evidence, ok := item["edge_evidence"].([]map[string]any)
	if !ok || len(evidence) != 2 {
		t.Fatalf("edge_evidence = %#v, want two edge evidence items", item["edge_evidence"])
	}
	if evidence[0]["source_quote"] != "lower rates improve liquidity" || evidence[1]["source_quote"] != "liquidity can lift stocks" {
		t.Fatalf("edge evidence = %#v, want preserved source quotes", evidence)
	}
	if !strings.Contains(item["context"].(string), "funding costs fall") {
		t.Fatalf("context = %#v, want edge reason included", item["context"])
	}
}

func TestAuthorValidationBackfillsPreviewGraphProvenanceForOldRender(t *testing.T) {
	result := FlowPreviewResult{
		ArticleForm: "analysis",
		Classify: PreviewGraph{
			Nodes: []PreviewNode{
				{ID: "n1", Text: "AI硬件瓶颈扩散", SourceQuote: "AI硬件瓶颈从GPU扩散到电力等五个维度"},
				{ID: "n2", Text: "光互连加速落地", SourceQuote: "2026年处于光互连加速落地阶段", IsTarget: true},
			},
			OffGraph: []PreviewOffGraph{{
				ID:          "off1",
				Text:        "NVL72机柜铜缆总重1.36吨",
				Role:        "evidence",
				SourceQuote: "NVL72机柜铜缆超5,000根、总重1.36吨",
			}},
		},
		Spines: []PreviewSpine{{
			ID:      "spine-1",
			Level:   "primary",
			Thesis:  "AI hardware constraints force optical interconnect",
			NodeIDs: []string{"n1", "n2"},
			Edges: []PreviewEdge{{
				From:        "n1",
				To:          "n2",
				SourceQuote: "AI硬件瓶颈扩散后，目前处于光互连加速落地阶段",
				Reason:      "The author links bottleneck diffusion to optical interconnect adoption.",
			}},
		}},
		Render: compile.Output{
			EvidenceNodes: []string{"NVL72机柜铜缆总重1.36吨"},
			TransmissionPaths: []compile.TransmissionPath{{
				Driver: "AI硬件瓶颈扩散",
				Target: "光互连加速落地",
				Steps:  []string{"AI硬件瓶颈扩散"},
			}},
			Details: compile.HiddenDetails{Caveats: []string{"old render without details.items"}},
		},
	}

	enriched := enrichAuthorValidationRenderDetails(result)
	claims := collectAuthorClaimCandidates(enriched)
	inferences := collectAuthorInferenceCandidates(enriched)
	if len(claims) != 1 || !strings.Contains(claims[0].SourceQuote, "5,000根") {
		t.Fatalf("claims = %#v, want off-graph source quote backfilled", claims)
	}
	if len(inferences) != 1 || len(inferences[0].EdgeEvidence) != 1 {
		t.Fatalf("inferences = %#v, want edge evidence backfilled", inferences)
	}
	if !strings.Contains(inferences[0].EdgeEvidence[0].SourceQuote, "光互连加速落地") {
		t.Fatalf("edge evidence = %#v, want spine edge quote", inferences[0].EdgeEvidence)
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

func TestNormalizeAuthorValidationContradictsScopeMismatch(t *testing.T) {
	validation := normalizeAuthorValidation(compile.AuthorValidation{
		ClaimChecks: []compile.AuthorClaimCheck{{
			ClaimID: "claim-001",
			Text:    "China consumes 80-90% of Iran's oil production",
			Status:  compile.AuthorClaimSupported,
			Subclaims: []compile.AuthorSubclaim{{
				Text:           "China consumes 80-90% of Iran's oil production",
				Subject:        "China",
				Metric:         "share of Iran oil",
				OriginalValue:  "80-90%",
				EvidenceValue:  "80-90%",
				ComparisonBase: "Iran total oil production",
				EvidenceBase:   "Iran seaborne oil exports",
				ScopeStatus:    "mismatch",
				AttributionOK:  false,
				Status:         compile.AuthorClaimSupported,
				Reason:         "The public number applies to exports, not total production.",
			}},
		}},
	}, []authorClaimCandidate{{ClaimID: "claim-001", Text: "China consumes 80-90% of Iran's oil production"}}, nil, "model")

	if validation.ClaimChecks[0].Status != compile.AuthorClaimContradicted {
		t.Fatalf("claim status = %q, want contradicted for denominator mismatch", validation.ClaimChecks[0].Status)
	}
	subclaim := validation.ClaimChecks[0].Subclaims[0]
	if subclaim.ScopeStatus != "mismatch" {
		t.Fatalf("scope_status = %q, want mismatch", subclaim.ScopeStatus)
	}
	if subclaim.ComparisonBase != "Iran total oil production" || subclaim.EvidenceBase != "Iran seaborne oil exports" {
		t.Fatalf("subclaim bases = %#v, want author/evidence bases preserved", subclaim)
	}
}
