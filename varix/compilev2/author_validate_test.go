package compilev2

import (
	"context"
	"net/http"
	"os"
	"strings"
	"testing"

	"github.com/kumaloha/VariX/varix/compile"
	"github.com/kumaloha/forge/llm"
)

func TestAuthorValidatePreviewResultValidatesAuthorOnlyWithSearch(t *testing.T) {
	rt := &fakeRuntime{responses: []llm.Response{
		{Text: `{
			"claim_plans":[
				{"claim_id":"claim-001","text":"Rates fall","claim_kind":"number_trend","needs_validation":true,"atomic_claims":[{"text":"Policy rate fell","subject":"policy rate","metric":"rate change","original_value":"downward move","unit":"bps","time_window":"article period","source_type":"official","series":"central-bank-policy-rate","preferred_sources":["central bank release"],"queries":["policy rate cut official release"],"comparison_rule":"official policy-rate change is negative"}],"required_evidence":[{"description":"Confirm the policy rate changed downward","subject":"policy rate","metric":"rate change","time_window":"article period","source_type":"official","status":"unverified","reason":"Need official rate data.","series":"central-bank-policy-rate","preferred_sources":["central bank release"],"queries":["policy rate cut official release"],"comparison_rule":"official policy-rate change is negative"}],"preferred_sources":["central bank release"],"queries":["policy rate cut official release"]},
				{"claim_id":"claim-004","text":"Policy rate declined by 25 bps","needs_validation":true,"required_evidence":[{"description":"Confirm the policy rate declined by 25 bps","subject":"policy rate","metric":"basis-point change","time_window":"article period","source_type":"official","status":"unverified","reason":"Need official basis-point change."}],"preferred_sources":["central bank release"],"queries":["policy rate declined 25 bps central bank release"]}
			],
			"inference_plans":[
				{"inference_id":"inference-001","from":"Rates fall","to":"Stocks will rise","steps":["Liquidity improves"],"required_evidence":[{"description":"Check whether market liquidity proxies improved after the rate move","subject":"risk assets","metric":"liquidity proxy","time_window":"after rate move","source_type":"market_data","status":"unverified","reason":"Need market liquidity data."}],"queries":["liquidity proxy after rate cut"]}
			]
		}`},
		{Text: `{
			"summary":{"verdict":"mixed"},
			"claim_checks":[
				{"claim_id":"claim-001","text":"Rates fall","claim_type":"fact","status":"supported","required_evidence":[{"description":"Confirm the policy rate changed downward","subject":"policy rate","metric":"rate change","time_window":"article period","source_type":"official","status":"supported","evidence":["source says rates fell"],"reason":"The official source reports a rate cut."}],"evidence":["source says rates fell"],"reason":"The author states this and it matches available evidence.","subclaims":[{"text":"Rates fall","subject":"policy rate","metric":"change","status":"supported","evidence":["source says rates fell"],"reason":"Matched source."}]},
				{"claim_id":"claim-002","text":"Stocks will rise","claim_type":"forecast","status":"interpretive","reason":"This is an unresolved forecast."},
				{"claim_id":"claim-003","text":"Liquidity improves","claim_type":"interpretation","status":"interpretive","reason":"This is a rendered mechanism node."},
				{"claim_id":"claim-004","text":"Policy rate declined by 25 bps","claim_type":"number","status":"supported","required_evidence":[{"description":"Confirm the policy rate declined by 25 bps","subject":"policy rate","metric":"basis-point change","time_window":"article period","source_type":"official","status":"supported","evidence":["Central bank release says the policy rate declined by 25 bps"],"reason":"The official release matches the evidence point."}],"evidence":["Central bank release says the policy rate declined by 25 bps"],"reason":"The concrete evidence point is supported."}
			],
			"inference_checks":[
				{"inference_id":"inference-001","from":"Rates fall","to":"Stocks will rise","steps":["Liquidity improves"],"status":"weak","required_evidence":[{"description":"Check whether market liquidity proxies improved after the rate move","subject":"risk assets","metric":"liquidity proxy","time_window":"after rate move","source_type":"market_data","status":"unverified","reason":"No market liquidity data was returned."}],"reason":"The mechanism is plausible but not established."}
			]
		}`,
		},
	}}
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
	if result.Render.AuthorValidation.Summary.SupportedClaims != 2 || result.Render.AuthorValidation.Summary.InterpretiveClaims != 2 {
		t.Fatalf("summary = %#v, want supported and interpretive render/proof claim counts", result.Render.AuthorValidation.Summary)
	}
	if result.Render.AuthorValidation.Summary.WeakInferences != 1 {
		t.Fatalf("summary = %#v, want weak inference count", result.Render.AuthorValidation.Summary)
	}
	if len(result.Render.AuthorValidation.ClaimChecks[0].Subclaims) != 1 {
		t.Fatalf("subclaims = %#v, want preserved proof subclaim", result.Render.AuthorValidation.ClaimChecks[0].Subclaims)
	}
	if got := result.Render.AuthorValidation.ClaimChecks[0].RequiredEvidence; len(got) != 1 || got[0].Metric != "rate change" || got[0].Status != compile.AuthorClaimSupported {
		t.Fatalf("claim required evidence = %#v, want supported rate-change requirement", got)
	}
	if got := result.Render.AuthorValidation.InferenceChecks[0].RequiredEvidence; len(got) != 1 || got[0].SourceType != "market_data" || got[0].Status != compile.AuthorClaimUnverified {
		t.Fatalf("inference required evidence = %#v, want unverified market-data requirement", got)
	}
	if got := result.Render.AuthorValidation.VerificationPlan.ClaimPlans; len(got) == 0 || got[0].AtomicClaims[0].Series != "central-bank-policy-rate" || got[0].AtomicClaims[0].ComparisonRule == "" {
		t.Fatalf("verification plan = %#v, want persisted executable atomic data spec", result.Render.AuthorValidation.VerificationPlan)
	}
	if got := result.Metrics["author_validate_ms"]; got < 0 {
		t.Fatalf("author_validate_ms = %d", got)
	}
	if len(rt.requests) != 2 {
		t.Fatalf("requests = %d, want plan plus judgment requests", len(rt.requests))
	}
	planReq := rt.requests[0]
	if planReq.Search {
		t.Fatal("author validation plan request Search = true, want false")
	}
	if !strings.Contains(planReq.System, "verification plan") || strings.Contains(planReq.System, `"claim_checks"`) {
		t.Fatalf("plan system prompt = %q, want verification-plan-only prompt", planReq.System)
	}
	req := rt.requests[1]
	if !req.Search {
		t.Fatal("author validation judgment request Search = false, want true for author fact checks")
	}
	if !strings.Contains(req.System, "Validate ONLY what the author claims") || !strings.Contains(req.System, "Do not critique the extraction pipeline") {
		t.Fatalf("system prompt = %q, want author-only validation boundary", req.System)
	}
	if len(req.UserParts) == 0 || !strings.Contains(req.UserParts[len(req.UserParts)-1].Text, "claim_candidates") || !strings.Contains(req.UserParts[len(req.UserParts)-1].Text, "inference_candidates") || !strings.Contains(req.UserParts[len(req.UserParts)-1].Text, "verification_plan") {
		t.Fatalf("user prompt missing candidates: %#v", req.UserParts)
	}
	userPrompt := req.UserParts[len(req.UserParts)-1].Text
	for _, want := range []string{"render_node", "proof_point"} {
		if !strings.Contains(userPrompt, want) {
			t.Fatalf("user prompt missing claim candidate kind %q: %s", want, userPrompt)
		}
	}
	for _, unwanted := range []string{`"kind": "supplementary_proof"`, `"kind": "source_quote"`, `"kind": "reference_proof"`} {
		if strings.Contains(userPrompt, unwanted) {
			t.Fatalf("user prompt contains non-render/non-evidence candidate %q: %s", unwanted, userPrompt)
		}
	}
	if !strings.Contains(userPrompt, `"source_quote": "The bank cut rates by 25 bps"`) {
		t.Fatalf("user prompt missing proof provenance source_quote: %s", userPrompt)
	}
	if !strings.Contains(userPrompt, `"current_date"`) {
		t.Fatalf("user prompt missing current_date: %s", userPrompt)
	}
	for _, want := range []string{"Validate only externally checkable point claims", "required_evidence", "metric, date/window, denominator/base", "current_date", "not future-dated", "search the cited source name plus the metric and value", "do not substitute YTD for MTD", "post date", "nearest close used", "data requirements needed to support the jump", "deferred to inference validation", "Do not use \"unverified\" for abstract claims", "Split compound proof points", "normalize units", "range_covered", "attribution_ok", "comparison_base", "scope_status", "denominator", "do not silently rewrite the subject", "edge_evidence", "implicit premises are externally supported", "missing_links for unverified or contradicted intermediate premises"} {
		if !strings.Contains(req.System, want) {
			t.Fatalf("system prompt missing %q: %s", want, req.System)
		}
	}
	for _, want := range []string{"original_value", "unit", "series", "comparison_rule", "preferred_sources", "FRED:WRESBAL"} {
		if !strings.Contains(planReq.System, want) {
			t.Fatalf("plan system prompt missing %q: %s", want, planReq.System)
		}
	}
}

func TestAuthorValidatePreviewResultRetriesInvalidJSON(t *testing.T) {
	rt := &fakeRuntime{responses: []llm.Response{
		{Text: `{"claim_plans":[{"claim_id":"claim-001","text":"Policy rate declined by 25 bps","needs_validation":true}],"inference_plans":[]}`},
		{Text: `{"summary":{"verdict":"mixed"},"claim_checks":[}}`},
		{Text: `{
			"summary":{"verdict":"mixed"},
			"claim_checks":[
				{"claim_id":"claim-001","text":"Policy rate declined by 25 bps","claim_type":"number","status":"supported","required_evidence":[{"description":"Confirm policy rate change","subject":"policy rate","metric":"basis-point change","time_window":"article period","source_type":"official","status":"supported","evidence":["Central bank release says the policy rate declined by 25 bps"],"reason":"Official release matches."}],"reason":"Official source supports it."}
			],
			"inference_checks":[]
		}`},
	}}
	client := &Client{runtime: rt, model: "author-model"}
	result, err := client.AuthorValidatePreviewResult(context.Background(), compile.Bundle{
		UnitID:     "twitter:retry-author-validate",
		Source:     "twitter",
		ExternalID: "retry-author-validate",
		Content:    "The author says the policy rate declined by 25 bps.",
	}, FlowPreviewResult{
		Render: compile.Output{
			EvidenceNodes: []string{"Policy rate declined by 25 bps"},
		},
	})
	if err != nil {
		t.Fatalf("AuthorValidatePreviewResult() error = %v", err)
	}
	if len(rt.requests) != 3 {
		t.Fatalf("requests = %d, want parse retry", len(rt.requests))
	}
	if !strings.Contains(rt.requests[2].System, "previous response was invalid JSON") {
		t.Fatalf("retry system prompt = %q, want strict JSON retry instruction", rt.requests[2].System)
	}
	if result.AuthorValidation == nil || len(result.AuthorValidation.ClaimChecks) != 1 {
		t.Fatalf("author validation = %#v, want parsed retry result", result.AuthorValidation)
	}
}

func TestAuthorValidatePreviewResultIncludesExternalEvidenceHints(t *testing.T) {
	originalBuilder := buildAuthorExternalEvidenceHints
	buildAuthorExternalEvidenceHints = func(_ context.Context, claims []authorClaimCandidate, plan authorVerificationPlan) ([]authorExternalEvidenceHint, error) {
		if len(claims) == 0 {
			t.Fatal("claims missing from evidence hint builder")
		}
		if len(plan.ClaimPlans) == 0 || plan.ClaimPlans[0].RequiredEvidence[0].Metric != "production shut-ins" {
			t.Fatalf("plan = %#v, want EIA production shut-ins plan", plan)
		}
		return []authorExternalEvidenceHint{{
			ClaimID: claims[0].ClaimID,
			Query:   "site:eia.gov STEO April 2026 production shut-ins 9.1 million b/d",
			Results: []authorExternalEvidenceResult{{
				URL:     "https://www.eia.gov/outlooks/steo/report/global_oil.php/",
				Title:   "EIA Short-Term Energy Outlook - Global Oil Markets",
				Excerpt: "We assess that production shut-ins will rise to 9.1 million b/d in April.",
			}},
		}}, nil
	}
	defer func() { buildAuthorExternalEvidenceHints = originalBuilder }()

	rt := &fakeRuntime{responses: []llm.Response{
		{Text: `{
			"claim_plans":[
				{"claim_id":"claim-001","text":"中东石油减产量达每天910万桶","needs_validation":true,"required_evidence":[{"description":"Check EIA oil production shut-ins","subject":"Middle East oil supply","metric":"production shut-ins","time_window":"April 2026","source_type":"official","status":"unverified","reason":"Need EIA STEO value."}],"preferred_sources":["EIA STEO"],"queries":["EIA STEO April 2026 production shut-ins 9.1 million b/d"]}
			],
			"inference_plans":[]
		}`},
		{Text: `{
			"summary":{"verdict":"mixed"},
			"claim_checks":[
				{"claim_id":"claim-001","text":"中东石油减产量达每天910万桶","claim_type":"number","status":"unverified","required_evidence":[{"description":"Check EIA oil production shut-ins","subject":"Middle East oil supply","metric":"production shut-ins","time_window":"April 2026","source_type":"official","status":"unverified","reason":"Model failed to use the exact EIA hint."}],"reason":"Unverified."}
			],
			"inference_checks":[]
		}`,
		},
	}}
	client := &Client{runtime: rt, model: "author-model"}
	result, err := client.AuthorValidatePreviewResult(context.Background(), compile.Bundle{
		UnitID:     "youtube:eia-hint",
		Source:     "youtube",
		ExternalID: "eia-hint",
		Content:    "美国能源信息署的数据，中东减产量到四月份已经达到了每天910万桶。",
	}, FlowPreviewResult{
		Render: compile.Output{
			EvidenceNodes: []string{"中东石油减产量达每天910万桶"},
			Details: compile.HiddenDetails{Items: []map[string]any{{
				"kind":         "proof_point",
				"text":         "中东石油减产量达每天910万桶",
				"source_quote": "美国能源信息署的数据，中东减产量到四月份已经达到了每天910万桶",
			}}},
		},
	})
	if err != nil {
		t.Fatalf("AuthorValidatePreviewResult() error = %v", err)
	}
	if got := result.AuthorValidation.ClaimChecks[0].Status; got != compile.AuthorClaimSupported {
		t.Fatalf("claim status = %q, want supported from exact external evidence hint", got)
	}
	if evidence := strings.Join(result.AuthorValidation.ClaimChecks[0].Evidence, " "); !strings.Contains(evidence, "9.1 million b/d") || !strings.Contains(evidence, "eia.gov") {
		t.Fatalf("claim evidence = %#v, want exact EIA hint carried into supported judgment", result.AuthorValidation.ClaimChecks[0].Evidence)
	}
	if len(rt.requests) != 2 {
		t.Fatalf("requests = %d, want plan plus judgment", len(rt.requests))
	}
	userPrompt := rt.requests[1].UserParts[len(rt.requests[1].UserParts)-1].Text
	for _, want := range []string{"external_evidence_hints", "eia.gov/outlooks/steo", "9.1 million b/d"} {
		if !strings.Contains(userPrompt, want) {
			t.Fatalf("user prompt missing %q: %s", want, userPrompt)
		}
	}
}

func TestNormalizeAuthorVerificationPlanEnrichesExecutableMarketDataSpecs(t *testing.T) {
	plan := normalizeAuthorVerificationPlan(authorVerificationPlan{
		ClaimPlans: []authorClaimVerificationPlan{
			{
				ClaimID:         "claim-reserves",
				Text:            "美国商业银行准备金规模从2025年8月触顶后持续下滑",
				NeedsValidation: true,
				RequiredEvidence: []compile.AuthorEvidenceRequirement{{
					Description: "Bank reserves peak and trough data",
					Subject:     "US Bank Reserves",
					Metric:      "Reserve balances",
					TimeWindow:  "August 2025 to November 2025",
					SourceType:  "official",
					Status:      compile.AuthorClaimUnverified,
				}},
			},
			{
				ClaimID:         "claim-tga",
				Text:            "美国财政部TGA余额预计在2026年4月中下旬达到峰值1.05万亿美元",
				NeedsValidation: true,
				RequiredEvidence: []compile.AuthorEvidenceRequirement{{
					Description: "US Treasury Quarterly Refunding Announcement forecast",
					Subject:     "US Treasury General Account (TGA)",
					Metric:      "Balance forecast",
					TimeWindow:  "April 2026",
					SourceType:  "official",
					Status:      compile.AuthorClaimUnverified,
				}},
			},
			{
				ClaimID:         "claim-stablecoin",
				Text:            "稳定币发行量在2026年初至2月中旬累计减少100亿美元",
				NeedsValidation: true,
				RequiredEvidence: []compile.AuthorEvidenceRequirement{{
					Description: "Stablecoin supply change data",
					Subject:     "USDT, USDC",
					Metric:      "Circulating supply decrease",
					TimeWindow:  "January 2026 to mid-February 2026",
					SourceType:  "market_data",
					Status:      compile.AuthorClaimUnverified,
				}},
			},
			{
				ClaimID:         "claim-etf",
				Text:            "比特币现货ETF从2025年10月开始出现持续性净流出",
				NeedsValidation: true,
				RequiredEvidence: []compile.AuthorEvidenceRequirement{{
					Description: "Bitcoin spot ETF flow data",
					Subject:     "Bitcoin Spot ETFs",
					Metric:      "Net flows",
					TimeWindow:  "October 2025 to March 2026",
					SourceType:  "market_data",
					Status:      compile.AuthorClaimUnverified,
				}},
			},
			{
				ClaimID:         "claim-legal",
				Text:            "Jane Street涉嫌长期操纵比特币市场并已引发集体诉讼",
				NeedsValidation: true,
				RequiredEvidence: []compile.AuthorEvidenceRequirement{{
					Description: "Legal filing or credible financial news report",
					Subject:     "Jane Street",
					Metric:      "Manipulation allegations status",
					TimeWindow:  "February 2026",
					SourceType:  "news",
					Status:      compile.AuthorClaimUnverified,
				}},
			},
		},
	})

	byID := map[string]compile.AuthorEvidenceRequirement{}
	for _, claimPlan := range plan.ClaimPlans {
		if len(claimPlan.RequiredEvidence) > 0 {
			byID[claimPlan.ClaimID] = claimPlan.RequiredEvidence[0]
		}
	}
	if got := byID["claim-reserves"]; got.Series != "FRED:WRESBAL" || !containsString(got.PreferredSources, "Federal Reserve H.4.1") || got.ComparisonRule == "" {
		t.Fatalf("reserve requirement = %#v, want FRED/H.4.1 executable spec", got)
	}
	if got := byID["claim-tga"]; got.Series != "FRED:WTREGEN" || !containsString(got.PreferredSources, "US Treasury Quarterly Refunding Announcement") || got.ComparisonRule == "" {
		t.Fatalf("tga requirement = %#v, want Treasury/FRED executable spec", got)
	}
	if got := byID["claim-stablecoin"]; !containsString(got.PreferredSources, "DeFiLlama stablecoins") || got.ComparisonRule == "" {
		t.Fatalf("stablecoin requirement = %#v, want stablecoin data vendors", got)
	}
	if got := byID["claim-etf"]; !containsString(got.PreferredSources, "Farside Investors") || got.ComparisonRule == "" {
		t.Fatalf("etf requirement = %#v, want ETF flow data vendors", got)
	}
	if got := byID["claim-legal"]; !containsString(got.PreferredSources, "CourtListener") || got.ComparisonRule == "" {
		t.Fatalf("legal requirement = %#v, want legal/news data vendors", got)
	}
}

func TestBuildFREDEvidenceHintFromCSV(t *testing.T) {
	result, ok := buildFREDEvidenceResultFromCSV("WRESBAL", "2025-08 to 2025-11", "3.4T to 2.85T", "Verify peak and drop", `observation_date,WRESBAL
2025-07-30,3300.0
2025-08-06,3400.0
2025-09-03,3200.0
2025-11-26,2850.0
2025-12-03,2900.0
`)
	if !ok {
		t.Fatal("buildFREDEvidenceResultFromCSV() ok = false, want true")
	}
	if result.URL == "" || !strings.Contains(result.Title, "FRED WRESBAL") {
		t.Fatalf("result = %#v, want FRED title/url", result)
	}
	for _, want := range []string{"2025-08-06=3400", "2025-11-26=2850", "min 2025-11-26=2850", "max 2025-08-06=3400", "author value 3.4T to 2.85T"} {
		if !strings.Contains(result.Excerpt, want) {
			t.Fatalf("excerpt = %q, missing %q", result.Excerpt, want)
		}
	}
}

func TestBuildStablecoinEvidenceResultFromJSON(t *testing.T) {
	result, ok := buildStablecoinEvidenceResultFromJSON("USDT", "2026-01-01 to 2026-02-15", "-4B", `{
		"symbol":"USDT",
		"chainBalances":{
			"Ethereum":{"tokens":[
				{"date":1767225600,"circulating":{"peggedUSD":100000000000}},
				{"date":1771113600,"circulating":{"peggedUSD":96000000000}}
			]},
			"Tron":{"tokens":[
				{"date":1767225600,"circulating":{"peggedUSD":50000000000}},
				{"date":1771113600,"circulating":{"peggedUSD":50000000000}}
			]}
		}
	}`)
	if !ok {
		t.Fatal("buildStablecoinEvidenceResultFromJSON() ok = false, want true")
	}
	for _, want := range []string{"USDT", "2026-01-01=150000000000", "2026-02-15=146000000000", "delta=-4000000000", "author value -4B"} {
		if !strings.Contains(result.Excerpt, want) {
			t.Fatalf("excerpt = %q, missing %q", result.Excerpt, want)
		}
	}
}

func TestBuildBitcoinETFEvidenceResultFromSoSoValueJSON(t *testing.T) {
	result, ok := buildBitcoinETFEvidenceResultFromSoSoValueJSON("2026-01 to 2026-03", `{
		"code":0,
		"data":[
			{"date":"2026-03-03","totalNetInflow":100000000},
			{"date":"2026-03-04","totalNetInflow":-50000000},
			{"date":"2026-03-05","totalNetInflow":-25000000}
		]
	}`)
	if !ok {
		t.Fatal("buildBitcoinETFEvidenceResultFromSoSoValueJSON() ok = false, want true")
	}
	for _, want := range []string{"SoSoValue BTC spot ETF flows", "positive_days=1", "negative_days=2", "sum=25000000", "continuous_outflow=false"} {
		if !strings.Contains(result.Excerpt, want) {
			t.Fatalf("excerpt = %q, missing %q", result.Excerpt, want)
		}
	}
}

func TestBuildCourtListenerEvidenceResultFromJSON(t *testing.T) {
	result, ok := buildCourtListenerEvidenceResultFromJSON("Jane Street Bitcoin manipulation", `{
		"count": 70,
		"results": [
			{"caseName":"Snyder v. Jane Street Group, LLC","court":"District Court, S.D. New York","dateFiled":"2026-02-24","docketNumber":"1:26-cv-01536","cause":"15:78m(a) Securities Exchange Act","docket_absolute_url":"/docket/72321910/snyder-v-jane-street-group-llc/"}
		]
	}`)
	if !ok {
		t.Fatal("buildCourtListenerEvidenceResultFromJSON() ok = false, want true")
	}
	for _, want := range []string{"CourtListener search", "count=70", "Snyder v. Jane Street Group, LLC", "2026-02-24", "Securities Exchange Act"} {
		if !strings.Contains(result.Excerpt, want) {
			t.Fatalf("excerpt = %q, missing %q", result.Excerpt, want)
		}
	}
}

func TestParseAuthorComparableNumbersPreservesStablecoinDecrease(t *testing.T) {
	got := parseAuthorComparableNumbers("USDT发行量减少40亿美元，USDC decreased -6 billion USD, total -10B")
	if len(got) < 3 {
		t.Fatalf("numbers = %#v, want three decrease values", got)
	}
	wants := []float64{-4, -6, -10}
	for i, want := range wants {
		if got[i].Value != want || got[i].Unit != "billion" {
			t.Fatalf("numbers[%d] = %#v, want %v billion", i, got[i], want)
		}
	}
}

func TestLiveAuthorEvidenceFetchesBitcoinETFWhenEnabled(t *testing.T) {
	if os.Getenv("RUN_LIVE_AUTHOR_EVIDENCE") != "1" {
		t.Skip("set RUN_LIVE_AUTHOR_EVIDENCE=1 to exercise live ETF evidence fetching")
	}
	result, ok := fetchBitcoinETFEvidenceResult(context.Background(), &http.Client{}, "2026-01 to 2026-03")
	if !ok {
		t.Fatal("fetchBitcoinETFEvidenceResult() ok = false, want true")
	}
	if !strings.Contains(result.Excerpt, "SoSoValue BTC spot ETF flows") || !strings.Contains(result.Excerpt, "positive_days=") {
		t.Fatalf("result = %#v, want SoSoValue ETF flow excerpt", result)
	}
}

func TestLiveAuthorEvidenceFetchesCourtListenerWhenEnabled(t *testing.T) {
	if os.Getenv("RUN_LIVE_AUTHOR_EVIDENCE") != "1" {
		t.Skip("set RUN_LIVE_AUTHOR_EVIDENCE=1 to exercise live CourtListener evidence fetching")
	}
	result, ok := fetchCourtListenerEvidenceResult(context.Background(), &http.Client{}, "Jane Street Bitcoin manipulation")
	if !ok {
		t.Fatal("fetchCourtListenerEvidenceResult() ok = false, want true")
	}
	if !strings.Contains(result.Excerpt, "CourtListener search") || !strings.Contains(result.Excerpt, "Jane Street") {
		t.Fatalf("result = %#v, want CourtListener Jane Street excerpt", result)
	}
}

func TestLiveAuthorEvidenceFetchesFREDWhenEnabled(t *testing.T) {
	if os.Getenv("RUN_LIVE_AUTHOR_EVIDENCE") != "1" {
		t.Skip("set RUN_LIVE_AUTHOR_EVIDENCE=1 to exercise live FRED evidence fetching")
	}
	result, ok := fetchFREDEvidenceResult(context.Background(), &http.Client{}, "WRESBAL", "2025-08 to 2025-11", "3.4T to 2.85T", "trend check")
	if !ok {
		t.Fatal("fetchFREDEvidenceResult() ok = false, want true")
	}
	if !strings.Contains(result.Excerpt, "FRED WRESBAL") || !strings.Contains(result.Excerpt, "2025-08") {
		t.Fatalf("result = %#v, want WRESBAL excerpt", result)
	}
}

func TestLiveAuthorEvidenceFetchesStablecoinWhenEnabled(t *testing.T) {
	if os.Getenv("RUN_LIVE_AUTHOR_EVIDENCE") != "1" {
		t.Skip("set RUN_LIVE_AUTHOR_EVIDENCE=1 to exercise live stablecoin evidence fetching")
	}
	result, ok := fetchStablecoinEvidenceResult(context.Background(), &http.Client{}, "USDT", "2026-01 to 2026-02-15", "-4B")
	if !ok {
		t.Fatal("fetchStablecoinEvidenceResult() ok = false, want true")
	}
	if !strings.Contains(result.Excerpt, "DeFiLlama stablecoin USDT") || !strings.Contains(result.Excerpt, "delta=") {
		t.Fatalf("result = %#v, want DeFiLlama stablecoin excerpt with delta", result)
	}
}

func TestCollectAuthorClaimCandidatesUsesOnlyRenderNodesAndEvidence(t *testing.T) {
	claims := collectAuthorClaimCandidates(compile.Output{
		Drivers:            []string{"Rates fall"},
		Targets:            []string{"Stocks rise"},
		EvidenceNodes:      []string{"Policy rate declined by 25 bps"},
		ExplanationNodes:   []string{"Central bank reaction function changed"},
		SupplementaryNodes: []string{"Model caveat"},
		TransmissionPaths: []compile.TransmissionPath{{
			Driver: "Rates fall",
			Steps:  []string{"Liquidity improves"},
			Target: "Stocks rise",
		}},
		Details: compile.HiddenDetails{
			QuoteHighlights:     []string{"The bank cut rates by 25 bps"},
			ReferenceHighlights: []string{"Central bank release"},
			Items: []map[string]any{
				{
					"kind":         "proof_point",
					"text":         "Policy rate declined by 25 bps",
					"source_quote": "The bank cut rates by 25 bps",
				},
				{"kind": "explanation", "text": "Central bank reaction function changed"},
				{"kind": "supplementary_proof", "text": "Model caveat"},
			},
		},
	})

	byText := map[string]authorClaimCandidate{}
	for _, claim := range claims {
		byText[claim.Text] = claim
	}
	for _, want := range []string{"Rates fall", "Liquidity improves", "Stocks rise", "Policy rate declined by 25 bps"} {
		if _, ok := byText[want]; !ok {
			t.Fatalf("claims = %#v, missing %q", claims, want)
		}
	}
	if byText["Rates fall"].Kind != "render_node" || byText["Policy rate declined by 25 bps"].Kind != "proof_point" {
		t.Fatalf("claims by text = %#v, want render_node plus proof_point kinds", byText)
	}
	for _, unwanted := range []string{"Central bank reaction function changed", "Model caveat", "The bank cut rates by 25 bps", "Central bank release"} {
		if _, ok := byText[unwanted]; ok {
			t.Fatalf("claims = %#v, should exclude non-render/non-evidence candidate %q", claims, unwanted)
		}
	}
	if byText["Policy rate declined by 25 bps"].SourceQuote != "The bank cut rates by 25 bps" {
		t.Fatalf("proof provenance = %#v, want source_quote preserved", byText["Policy rate declined by 25 bps"])
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
	var proofClaim *authorClaimCandidate
	var renderClaim *authorClaimCandidate
	for i := range claims {
		if claims[i].Kind == "proof_point" && claims[i].Text == "NVL72机柜铜缆总重1.36吨" {
			proofClaim = &claims[i]
		}
		if claims[i].Kind == "render_node" && claims[i].Text == "AI硬件瓶颈扩散" {
			renderClaim = &claims[i]
		}
	}
	if renderClaim == nil || !strings.Contains(renderClaim.SourceQuote, "GPU扩散到电力") {
		t.Fatalf("claims = %#v, want render node source quote backfilled", claims)
	}
	if proofClaim == nil || !strings.Contains(proofClaim.SourceQuote, "5,000根") {
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

func TestNormalizeAuthorValidationRejectsAuthorOnlySupportedClaimEvidence(t *testing.T) {
	validation := normalizeAuthorValidation(compile.AuthorValidation{
		ClaimChecks: []compile.AuthorClaimCheck{{
			ClaimID:   "claim-001",
			Text:      "中东石油减产量达每天910万桶",
			ClaimType: "number",
			Status:    compile.AuthorClaimSupported,
			RequiredEvidence: []compile.AuthorEvidenceRequirement{{
				Description: "Middle East oil production reduction volume",
				Subject:     "Middle East Oil Supply",
				Metric:      "Daily production cut (bpd)",
				TimeWindow:  "April 2026",
				SourceType:  "official",
				Status:      compile.AuthorClaimSupported,
				Evidence:    []string{"Author cites EIA data for 9.1M bpd reduction"},
				Reason:      "Specific numeric claim attributed to EIA.",
			}},
			Evidence: []string{"美国能源信息署的数据 中东减产量到四月份 已经达到了每天910万桶"},
			Reason:   "Direct numeric claim with source attribution.",
		}},
	}, []authorClaimCandidate{{ClaimID: "claim-001", Kind: "proof_point", Text: "中东石油减产量达每天910万桶"}}, nil, "model")

	check := validation.ClaimChecks[0]
	if check.Status != compile.AuthorClaimUnverified {
		t.Fatalf("claim status = %q, want unverified when supported evidence is only author citation", check.Status)
	}
	if !strings.Contains(check.Reason, "external evidence") {
		t.Fatalf("reason = %q, want external evidence downgrade reason", check.Reason)
	}
	if validation.Summary.UnverifiedClaims != 1 || validation.Summary.SupportedClaims != 0 {
		t.Fatalf("summary = %#v, want downgraded unverified claim", validation.Summary)
	}
}

func TestNormalizeAuthorValidationPreservesSupportedClaimWithExternalEvidence(t *testing.T) {
	validation := normalizeAuthorValidation(compile.AuthorValidation{
		ClaimChecks: []compile.AuthorClaimCheck{{
			ClaimID:   "claim-001",
			Text:      "中东石油减产量达每天910万桶",
			ClaimType: "number",
			Status:    compile.AuthorClaimSupported,
			RequiredEvidence: []compile.AuthorEvidenceRequirement{{
				Description: "Middle East oil production reduction volume",
				Subject:     "Middle East Oil Supply",
				Metric:      "Daily production shut-ins (bpd)",
				TimeWindow:  "April 2026",
				SourceType:  "official",
				Status:      compile.AuthorClaimSupported,
				Evidence:    []string{"EIA April 2026 STEO reports production shut-ins will rise to 9.1 million b/d in April."},
				Reason:      "Official EIA source matches the figure and time window.",
			}},
			Evidence: []string{"EIA April 2026 STEO: production shut-ins rise to 9.1 million b/d."},
			Reason:   "Official source supports the numeric claim.",
		}},
	}, []authorClaimCandidate{{ClaimID: "claim-001", Kind: "proof_point", Text: "中东石油减产量达每天910万桶"}}, nil, "model")

	if validation.ClaimChecks[0].Status != compile.AuthorClaimSupported {
		t.Fatalf("claim status = %q, want supported with external evidence", validation.ClaimChecks[0].Status)
	}
}

func TestNormalizeAuthorValidationRejectsVagueExternalSupportedEvidence(t *testing.T) {
	validation := normalizeAuthorValidation(compile.AuthorValidation{
		ClaimChecks: []compile.AuthorClaimCheck{{
			ClaimID:   "claim-001",
			Text:      "一级Crypto VC管理规模扩张",
			ClaimType: "fact",
			Status:    compile.AuthorClaimSupported,
			RequiredEvidence: []compile.AuthorEvidenceRequirement{{
				Description: "Crypto VC AUM growth",
				Subject:     "Crypto VC",
				Metric:      "AUM Growth",
				SourceType:  "market_data",
				Status:      compile.AuthorClaimSupported,
				Evidence:    []string{"Industry reports (e.g., Preqin/PitchBook) show significant growth in crypto VC AUM."},
				Reason:      "Industry reports support the trend.",
			}},
			Evidence: []string{"Market data shows VC-backed tokens often face selling pressure."},
			Reason:   "Supported by industry trends.",
		}},
	}, []authorClaimCandidate{{ClaimID: "claim-001", Kind: "render_node", Text: "一级Crypto VC管理规模扩张"}}, nil, "model")

	if validation.ClaimChecks[0].Status != compile.AuthorClaimUnverified {
		t.Fatalf("claim status = %q, want unverified when evidence names vague reports without a concrete source/value", validation.ClaimChecks[0].Status)
	}
}

func TestNormalizeAuthorValidationUsesFREDHintForNumericSupport(t *testing.T) {
	validation := normalizeAuthorValidationWithHints(compile.AuthorValidation{
		ClaimChecks: []compile.AuthorClaimCheck{{
			ClaimID:   "claim-001",
			Text:      "美国商业银行准备金规模从2025年8月触顶后持续下滑，从3.4万亿到2.85万亿",
			ClaimType: "number",
			Status:    compile.AuthorClaimUnverified,
			RequiredEvidence: []compile.AuthorEvidenceRequirement{{
				Description:   "Reserve balance trend",
				Metric:        "Reserve Balances",
				OriginalValue: "3.4T to 2.85T",
				Series:        "FRED:WRESBAL",
				Status:        compile.AuthorClaimUnverified,
			}},
		}},
	}, []authorClaimCandidate{{ClaimID: "claim-001", Kind: "proof_point", Text: "美国商业银行准备金规模从2025年8月触顶后持续下滑，从3.4万亿到2.85万亿"}}, nil, "model", []authorExternalEvidenceHint{{
		ClaimID: "claim-001",
		Query:   "FRED WRESBAL",
		Results: []authorExternalEvidenceResult{{
			URL:     "https://fred.stlouisfed.org/series/WRESBAL",
			Title:   "FRED WRESBAL",
			Excerpt: "FRED WRESBAL observations for Aug 2025 to Nov 2025: first 2025-08-06=3332492, last 2025-11-26=2896586, min 2025-11-12=2855030, max 2025-08-06=3332492. author value 3.4T to 2.85T.",
		}},
	}})

	check := validation.ClaimChecks[0]
	if check.Status != compile.AuthorClaimSupported {
		t.Fatalf("claim status = %q, want supported from numeric FRED hint", check.Status)
	}
	if !strings.Contains(strings.Join(check.Evidence, " "), "FRED WRESBAL") {
		t.Fatalf("evidence = %#v, want FRED hint appended", check.Evidence)
	}
}

func TestNormalizeAuthorValidationUsesStablecoinHintForContradiction(t *testing.T) {
	validation := normalizeAuthorValidationWithHints(compile.AuthorValidation{
		ClaimChecks: []compile.AuthorClaimCheck{{
			ClaimID:   "claim-001",
			Text:      "稳定币发行量在2026年初至2月中旬累计减少100亿美元",
			ClaimType: "number",
			Status:    compile.AuthorClaimUnverified,
			RequiredEvidence: []compile.AuthorEvidenceRequirement{{
				Description:   "Stablecoin supply decline",
				Metric:        "Supply Change",
				OriginalValue: "-10 billion USD",
				Series:        "DeFiLlama",
				Status:        compile.AuthorClaimUnverified,
			}},
		}},
	}, []authorClaimCandidate{{ClaimID: "claim-001", Kind: "proof_point", Text: "稳定币发行量在2026年初至2月中旬累计减少100亿美元"}}, nil, "model", []authorExternalEvidenceHint{{
		ClaimID: "claim-001",
		Query:   "DeFiLlama stablecoins",
		Results: []authorExternalEvidenceResult{{
			URL:     "https://stablecoins.llama.fi/stablecoin/1",
			Title:   "DeFiLlama stablecoin USDT",
			Excerpt: "DeFiLlama stablecoin USDT circulating supply for 2026-01 to 2026-02-15: first 2026-01-01=150000000000, last 2026-02-15=146300000000, delta=-3700000000. author value -10B.",
		}},
	}})

	check := validation.ClaimChecks[0]
	if check.Status != compile.AuthorClaimContradicted {
		t.Fatalf("claim status = %q, want contradicted from DeFiLlama delta mismatch", check.Status)
	}
	if !strings.Contains(check.DecisionNote, "口径:") || !strings.Contains(check.DecisionNote, "判定:") || !strings.Contains(check.DecisionNote, "Supply Change") {
		t.Fatalf("decision_note = %q, want basis and judgment for explanation column", check.DecisionNote)
	}
	if !strings.Contains(strings.Join(check.Evidence, " "), "DeFiLlama stablecoin USDT") {
		t.Fatalf("evidence = %#v, want stablecoin hint appended", check.Evidence)
	}
}

func TestNormalizeAuthorValidationUsesETFHintForContinuousOutflowContradiction(t *testing.T) {
	validation := normalizeAuthorValidationWithHints(compile.AuthorValidation{
		ClaimChecks: []compile.AuthorClaimCheck{{
			ClaimID:   "claim-001",
			Text:      "比特币现货ETF从2025年10月开始出现持续性净流出",
			ClaimType: "fact",
			Status:    compile.AuthorClaimUnverified,
			RequiredEvidence: []compile.AuthorEvidenceRequirement{{
				Description:   "Bitcoin spot ETF net flow trend",
				Metric:        "Net Flows",
				OriginalValue: "continuous net outflow",
				Series:        "SoSoValue BTC spot ETF",
				Status:        compile.AuthorClaimUnverified,
			}},
		}},
	}, []authorClaimCandidate{{ClaimID: "claim-001", Kind: "proof_point", Text: "比特币现货ETF从2025年10月开始出现持续性净流出"}}, nil, "model", []authorExternalEvidenceHint{{
		ClaimID: "claim-001",
		Query:   "SoSoValue Bitcoin ETF flows",
		Results: []authorExternalEvidenceResult{{
			URL:     "https://open.sosovalue.xyz/openapi/v1/etf/us-btc-spot/historicalInflowChart",
			Title:   "SoSoValue BTC spot ETF flows",
			Excerpt: "SoSoValue BTC spot ETF flows for 2026-01 to 2026-03: first 2026-03-03=100000000, last 2026-03-05=-25000000, sum=25000000, positive_days=1, negative_days=2, continuous_outflow=false.",
		}},
	}})

	check := validation.ClaimChecks[0]
	if check.Status != compile.AuthorClaimContradicted {
		t.Fatalf("claim status = %q, want contradicted from positive ETF flow day", check.Status)
	}
}

func TestNormalizeAuthorValidationReappliesETFHintAfterAuthorOnlyDowngrade(t *testing.T) {
	validation := normalizeAuthorValidationWithHints(compile.AuthorValidation{
		ClaimChecks: []compile.AuthorClaimCheck{{
			ClaimID:   "claim-001",
			Text:      "比特币现货ETF从2025年10月开始出现持续性净流出",
			ClaimType: "fact",
			Status:    compile.AuthorClaimSupported,
			RequiredEvidence: []compile.AuthorEvidenceRequirement{{
				Description:   "Bitcoin spot ETF net flow trend",
				Metric:        "Net Flows",
				OriginalValue: "continuous net outflow",
				Series:        "SoSoValue BTC spot ETF",
				Status:        compile.AuthorClaimSupported,
				Evidence:      []string{"Author says ETF flows were negative."},
			}},
			Evidence: []string{"Author cites ETF outflows."},
			Reason:   "The author states this trend.",
		}},
	}, []authorClaimCandidate{{ClaimID: "claim-001", Kind: "proof_point", Text: "比特币现货ETF从2025年10月开始出现持续性净流出"}}, nil, "model", []authorExternalEvidenceHint{{
		ClaimID: "claim-001",
		Query:   "SoSoValue Bitcoin ETF flows",
		Results: []authorExternalEvidenceResult{{
			URL:     "https://open.sosovalue.xyz/openapi/v1/etf/us-btc-spot/historicalInflowChart",
			Title:   "SoSoValue BTC spot ETF flows",
			Excerpt: "SoSoValue BTC spot ETF flows for 2025-10 to 2026-03: sum=-1640000000, positive_days=40, negative_days=72, continuous_outflow=false.",
		}},
	}})

	check := validation.ClaimChecks[0]
	if check.Status != compile.AuthorClaimContradicted {
		t.Fatalf("claim status = %q, want contradicted after author-only supported claim is downgraded and hint reapplied", check.Status)
	}
}

func TestNormalizeAuthorValidationETFHintOverridesSupportedContinuousOutflow(t *testing.T) {
	validation := normalizeAuthorValidationWithHints(compile.AuthorValidation{
		ClaimChecks: []compile.AuthorClaimCheck{{
			ClaimID:   "claim-001",
			Text:      "比特币现货ETF从2025年10月开始出现持续性净流出",
			ClaimType: "market_flow",
			Status:    compile.AuthorClaimSupported,
			RequiredEvidence: []compile.AuthorEvidenceRequirement{{
				Description:   "Bitcoin spot ETF net flow trend",
				Metric:        "Net Flows",
				OriginalValue: "continuous net outflow",
				Series:        "Farside Investors",
				Status:        compile.AuthorClaimSupported,
				Evidence:      []string{"Farside showed cumulative net outflows."},
				Reason:        "Cumulative net outflow trend confirmed.",
			}},
			Evidence: []string{"Farside cumulative flow data."},
			Reason:   "Net outflow trend confirmed.",
		}},
	}, []authorClaimCandidate{{ClaimID: "claim-001", Kind: "proof_point", Text: "比特币现货ETF从2025年10月开始出现持续性净流出"}}, nil, "model", []authorExternalEvidenceHint{{
		ClaimID: "claim-001",
		Query:   "SoSoValue Bitcoin ETF flows",
		Results: []authorExternalEvidenceResult{{
			URL:     "https://open.sosovalue.xyz/openapi/v1/etf/us-btc-spot/historicalInflowChart",
			Title:   "SoSoValue BTC spot ETF flows",
			Excerpt: "SoSoValue BTC spot ETF flows for 2025-10 onwards: sum=3419179392.82, positive_days=13, negative_days=10, continuous_outflow=false.",
		}},
	}})

	check := validation.ClaimChecks[0]
	if check.Status != compile.AuthorClaimContradicted {
		t.Fatalf("claim status = %q, want local continuous_outflow=false hint to override model-supported cumulative-flow judgment", check.Status)
	}
	if !strings.Contains(check.DecisionNote, "continuous") && !strings.Contains(check.DecisionNote, "qualitative") {
		t.Fatalf("decision_note = %q, want qualitative contradiction explanation", check.DecisionNote)
	}
}

func TestNormalizeAuthorValidationDowngradesSpecificLegalMethodWithOnlyDocketEvidence(t *testing.T) {
	validation := normalizeAuthorValidation(compile.AuthorValidation{
		ClaimChecks: []compile.AuthorClaimCheck{{
			ClaimID:   "claim-001",
			Text:      "Jane Street操纵手法为每日定时大额抛售触发爆仓后低位补仓，已引发集体诉讼",
			ClaimType: "fact",
			Status:    compile.AuthorClaimSupported,
			RequiredEvidence: []compile.AuthorEvidenceRequirement{{
				Description: "Court filings regarding Jane Street lawsuit",
				Subject:     "Jane Street",
				Metric:      "Legal Action",
				SourceType:  "legal",
				Status:      compile.AuthorClaimSupported,
				Evidence: []string{
					"CourtListener: Snyder v. Jane Street Group, LLC (2026-02-24, District Court, S.D. New York, docket 1:26-cv-01536)",
				},
				Reason: "Court records confirm a lawsuit exists.",
			}},
			Evidence: []string{
				"CourtListener legal search (https://www.courtlistener.com/docket/72321910/snyder-v-jane-street-group-llc/): CourtListener search \"Jane Street Bitcoin manipulation\": count=70; top: Snyder v. Jane Street Group, LLC (2026-02-24, District Court, S.D. New York, docket 1:26-cv-01536).",
			},
			Reason: "Court records confirm the lawsuit exists.",
		}},
	}, []authorClaimCandidate{{
		ClaimID: "claim-001",
		Kind:    "proof_point",
		Text:    "Jane Street操纵手法为每日定时大额抛售触发爆仓后低位补仓，已引发集体诉讼",
	}}, nil, "model")

	check := validation.ClaimChecks[0]
	if check.Status != compile.AuthorClaimUnverified {
		t.Fatalf("claim status = %q, want unverified because docket evidence does not verify trading method", check.Status)
	}
	if !strings.Contains(check.Reason, "specific trading-method detail") {
		t.Fatalf("reason = %q, want scope warning", check.Reason)
	}
	if !strings.Contains(check.DecisionNote, "口径:") || !strings.Contains(check.DecisionNote, "判定:") || !strings.Contains(check.DecisionNote, "specific trading-method detail") {
		t.Fatalf("decision_note = %q, want scope reason in explanation column", check.DecisionNote)
	}
}

func TestNormalizeAuthorValidationKeepsDeFiLlamaSupportedEvidence(t *testing.T) {
	validation := normalizeAuthorValidation(compile.AuthorValidation{
		ClaimChecks: []compile.AuthorClaimCheck{{
			ClaimID:   "claim-001",
			Text:      "USDC发行量近三周回升，USDT发行量仍无起色",
			ClaimType: "number",
			Status:    compile.AuthorClaimSupported,
			RequiredEvidence: []compile.AuthorEvidenceRequirement{{
				Description: "Stablecoin supply data for USDC and USDT",
				Subject:     "Stablecoins",
				Metric:      "Circulating Supply Change",
				SourceType:  "market_data",
				Status:      compile.AuthorClaimSupported,
				Evidence: []string{
					"DeFiLlama USDC: 2026-03-01=75.23B, 2026-03-31=77.38B (+2.16B increase); DeFiLlama USDT: 2026-03-01=183.48B, 2026-03-31=184.19B (+0.71B, relatively flat)",
				},
				Reason: "DeFiLlama data supports both trend checks.",
			}},
			Evidence: []string{
				"DeFiLlama USDC/USDT supply data supports the stated trend.",
			},
			Reason: "DeFiLlama data confirms the trend.",
		}},
	}, []authorClaimCandidate{{
		ClaimID: "claim-001",
		Kind:    "proof_point",
		Text:    "USDC发行量近三周回升，USDT发行量仍无起色",
	}}, nil, "model")

	if validation.ClaimChecks[0].Status != compile.AuthorClaimSupported {
		t.Fatalf("claim status = %q, want supported with DeFiLlama evidence", validation.ClaimChecks[0].Status)
	}
}

func TestNormalizeAuthorValidationDowngradesAuthorOnlySoundInferenceAndDedupes(t *testing.T) {
	validation := normalizeAuthorValidation(compile.AuthorValidation{
		InferenceChecks: []compile.AuthorInferenceCheck{
			{
				InferenceID: "inference-001",
				From:        "全球石油供应缺口约7-10%",
				Steps:       []string{"美国财政应对空间有限", "美联储大概率选择印钞"},
				To:          "对冲通胀资产上涨",
				Status:      compile.AuthorInferenceSound,
				RequiredEvidence: []compile.AuthorEvidenceRequirement{{
					Description: "Chain from supply shock to fiscal constraint to monetary easing",
					Subject:     "Macro policy response",
					Metric:      "Policy sequence",
					TimeWindow:  "2026",
					SourceType:  "author_source",
					Status:      compile.AuthorClaimSupported,
					Evidence:    []string{"Author explicitly maps this sequence as the core thesis."},
					Reason:      "Author explicitly constructs the transmission path.",
				}},
				Evidence: []string{"作者明确说油价冲击导致财政压力并迫使印钞"},
				Reason:   "Author explicitly constructs the transmission path.",
			},
			{
				InferenceID: "inference-002",
				From:        "全球石油供应缺口约7-10%",
				Steps:       []string{"美国财政应对空间有限", "美联储大概率选择印钞"},
				To:          "对冲通胀资产上涨",
				Status:      compile.AuthorInferenceSound,
				RequiredEvidence: []compile.AuthorEvidenceRequirement{{
					Description: "Duplicate of inference-001",
					Subject:     "Macro policy response",
					Metric:      "Policy sequence",
					TimeWindow:  "2026",
					SourceType:  "author_source",
					Status:      compile.AuthorClaimSupported,
					Evidence:    []string{"Same as inference-001"},
					Reason:      "Identical logical path.",
				}},
			},
		},
	}, nil, []authorInferenceCandidate{
		{InferenceID: "inference-001", From: "全球石油供应缺口约7-10%", Steps: []string{"美国财政应对空间有限", "美联储大概率选择印钞"}, To: "对冲通胀资产上涨"},
		{InferenceID: "inference-002", From: "全球石油供应缺口约7-10%", Steps: []string{"美国财政应对空间有限", "美联储大概率选择印钞"}, To: "对冲通胀资产上涨"},
	}, "model")

	if len(validation.InferenceChecks) != 1 {
		t.Fatalf("inference checks = %#v, want duplicate path collapsed", validation.InferenceChecks)
	}
	check := validation.InferenceChecks[0]
	if check.Status != compile.AuthorInferenceWeak {
		t.Fatalf("inference status = %q, want weak without external premise evidence", check.Status)
	}
	if len(check.MissingLinks) == 0 || !strings.Contains(strings.Join(check.MissingLinks, " "), "external") {
		t.Fatalf("missing_links = %#v, want external evidence gap", check.MissingLinks)
	}
}

func TestNormalizeAuthorValidationBackfillsNarrativeCandidatesAsInterpretive(t *testing.T) {
	validation := normalizeAuthorValidation(compile.AuthorValidation{}, []authorClaimCandidate{
		{ClaimID: "claim-001", Kind: "target", Text: "China and Russia are relative geopolitical winners"},
		{ClaimID: "claim-002", Kind: "proof_point", Text: "China has 90-120 days of oil inventory"},
	}, nil, "model")

	if validation.ClaimChecks[0].Status != compile.AuthorClaimInterpretive {
		t.Fatalf("narrative claim status = %q, want interpretive", validation.ClaimChecks[0].Status)
	}
	if validation.ClaimChecks[1].Status != compile.AuthorClaimUnverified {
		t.Fatalf("proof claim status = %q, want unverified", validation.ClaimChecks[1].Status)
	}
	if validation.Summary.InterpretiveClaims != 1 || validation.Summary.UnverifiedClaims != 1 {
		t.Fatalf("summary = %#v, want one interpretive narrative and one unverified proof", validation.Summary)
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

func TestNormalizeAuthorValidationKeepsInterpretiveParentWithSupportedSubclaims(t *testing.T) {
	validation := normalizeAuthorValidation(compile.AuthorValidation{
		ClaimChecks: []compile.AuthorClaimCheck{{
			ClaimID:   "claim-001",
			Text:      "China and Russia are relative economic and geopolitical winners from this war",
			ClaimType: "interpretation",
			Status:    compile.AuthorClaimInterpretive,
			Subclaims: []compile.AuthorSubclaim{{
				Text:           "China buys most of Iran's seaborne oil exports",
				Subject:        "China",
				Metric:         "share of Iranian seaborne oil exports",
				OriginalValue:  "most",
				EvidenceValue:  "80-90%",
				ComparisonBase: "Iran seaborne oil exports",
				EvidenceBase:   "Iran seaborne oil exports",
				ScopeStatus:    "exact_match",
				AttributionOK:  true,
				Status:         compile.AuthorClaimSupported,
				Reason:         "Public sources support the concrete oil-import premise.",
			}},
		}},
	}, []authorClaimCandidate{{ClaimID: "claim-001", Kind: "target", Text: "China and Russia are relative economic and geopolitical winners from this war"}}, nil, "model")

	if validation.ClaimChecks[0].Status != compile.AuthorClaimInterpretive {
		t.Fatalf("claim status = %q, want interpretive parent despite supported factual subclaim", validation.ClaimChecks[0].Status)
	}
	if validation.Summary.InterpretiveClaims != 1 || validation.Summary.SupportedClaims != 0 {
		t.Fatalf("summary = %#v, want interpretive parent not supported point claim", validation.Summary)
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
