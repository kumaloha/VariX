package compile

import (
	"regexp"
	"strings"
)

func normalizeAuthorValidation(validation AuthorValidation, claims []authorClaimCandidate, inferences []authorInferenceCandidate, model string) AuthorValidation {
	return normalizeAuthorValidationWithHints(validation, claims, inferences, model, nil)
}

func normalizeAuthorValidationWithHints(validation AuthorValidation, claims []authorClaimCandidate, inferences []authorInferenceCandidate, model string, hints []authorExternalEvidenceHint) AuthorValidation {
	if validation.ValidatedAt.IsZero() {
		validation.ValidatedAt = NowUTC()
	}
	if strings.TrimSpace(validation.Model) == "" {
		validation.Model = strings.TrimSpace(model)
	}
	validation.Version = authorValidationVersion

	hintsByClaimID := authorExternalEvidenceHintsByClaimID(hints)
	claimByID := make(map[string]AuthorClaimCheck, len(validation.ClaimChecks))
	for _, check := range validation.ClaimChecks {
		check.ClaimID = strings.TrimSpace(check.ClaimID)
		if check.ClaimID == "" {
			continue
		}
		check.Status = normalizeAuthorClaimStatus(check.Status)
		check.RequiredEvidence = normalizeAuthorEvidenceRequirements(check.RequiredEvidence)
		check.Subclaims = normalizeAuthorSubclaims(check.ClaimID, check.Subclaims)
		claimByID[check.ClaimID] = check
	}
	normalizedClaims := make([]AuthorClaimCheck, 0, len(claims))
	usedClaimIDs := make(map[string]struct{}, len(claims))
	for _, candidate := range claims {
		usedClaimIDs[candidate.ClaimID] = struct{}{}
		check, ok := claimByID[candidate.ClaimID]
		if !ok {
			check = defaultMissingAuthorClaimCheck(candidate)
		}
		if strings.TrimSpace(check.Text) == "" {
			check.Text = candidate.Text
		}
		check.Status = normalizeAuthorClaimStatus(check.Status)
		check.RequiredEvidence = normalizeAuthorEvidenceRequirements(check.RequiredEvidence)
		check.Subclaims = normalizeAuthorSubclaims(check.ClaimID, check.Subclaims)
		check.Status = aggregateClaimStatusFromSubclaims(check.Status, check.Subclaims)
		check = applyExternalEvidenceHintToClaim(check, hintsByClaimID[check.ClaimID])
		check = enforceExternalEvidenceForSupportedClaim(check)
		check = applyExternalEvidenceHintToClaim(check, hintsByClaimID[check.ClaimID])
		check = enforceLegalClaimScope(check)
		check = fillAuthorClaimDecisionNote(check)
		normalizedClaims = append(normalizedClaims, check)
	}
	for _, check := range validation.ClaimChecks {
		if _, ok := usedClaimIDs[check.ClaimID]; ok {
			continue
		}
		check.Status = normalizeAuthorClaimStatus(check.Status)
		check.RequiredEvidence = normalizeAuthorEvidenceRequirements(check.RequiredEvidence)
		check.Subclaims = normalizeAuthorSubclaims(check.ClaimID, check.Subclaims)
		check.Status = aggregateClaimStatusFromSubclaims(check.Status, check.Subclaims)
		check = applyExternalEvidenceHintToClaim(check, hintsByClaimID[check.ClaimID])
		check = enforceExternalEvidenceForSupportedClaim(check)
		check = applyExternalEvidenceHintToClaim(check, hintsByClaimID[check.ClaimID])
		check = enforceLegalClaimScope(check)
		check = fillAuthorClaimDecisionNote(check)
		normalizedClaims = append(normalizedClaims, check)
	}
	validation.ClaimChecks = normalizedClaims

	inferenceByID := make(map[string]AuthorInferenceCheck, len(validation.InferenceChecks))
	for _, check := range validation.InferenceChecks {
		check.InferenceID = strings.TrimSpace(check.InferenceID)
		if check.InferenceID == "" {
			continue
		}
		check.Status = normalizeAuthorInferenceStatus(check.Status)
		check.RequiredEvidence = normalizeAuthorEvidenceRequirements(check.RequiredEvidence)
		inferenceByID[check.InferenceID] = check
	}
	normalizedInferences := make([]AuthorInferenceCheck, 0, len(inferences))
	seenInferencePaths := make(map[string]struct{}, len(inferences))
	for _, candidate := range inferences {
		check, ok := inferenceByID[candidate.InferenceID]
		if !ok {
			check = AuthorInferenceCheck{
				InferenceID: candidate.InferenceID,
				From:        candidate.From,
				To:          candidate.To,
				Steps:       cloneStrings(candidate.Steps),
				Status:      AuthorInferenceWeak,
				Reason:      "validator did not return this candidate",
			}
		}
		if strings.TrimSpace(check.From) == "" {
			check.From = candidate.From
		}
		if strings.TrimSpace(check.To) == "" {
			check.To = candidate.To
		}
		if len(check.Steps) == 0 {
			check.Steps = cloneStrings(candidate.Steps)
		}
		check.Status = normalizeAuthorInferenceStatus(check.Status)
		check.RequiredEvidence = normalizeAuthorEvidenceRequirements(check.RequiredEvidence)
		check = enforceExternalEvidenceForSoundInference(check)
		check = fillAuthorInferenceDecisionNote(check)
		pathKey := normalizedAuthorInferencePathKey(check.From, check.Steps, check.To)
		if _, ok := seenInferencePaths[pathKey]; ok {
			continue
		}
		seenInferencePaths[pathKey] = struct{}{}
		normalizedInferences = append(normalizedInferences, check)
	}
	validation.InferenceChecks = normalizedInferences
	validation.Summary = summarizeAuthorValidation(validation)
	return validation
}

func normalizeAuthorVerificationPlan(plan authorVerificationPlan) authorVerificationPlan {
	normalizedClaims := make([]authorClaimVerificationPlan, 0, len(plan.ClaimPlans))
	seenClaims := map[string]struct{}{}
	for _, claimPlan := range plan.ClaimPlans {
		claimPlan.ClaimID = strings.TrimSpace(claimPlan.ClaimID)
		if claimPlan.ClaimID == "" {
			continue
		}
		if _, ok := seenClaims[claimPlan.ClaimID]; ok {
			continue
		}
		seenClaims[claimPlan.ClaimID] = struct{}{}
		claimPlan.Text = strings.TrimSpace(claimPlan.Text)
		claimPlan.ClaimKind = strings.TrimSpace(claimPlan.ClaimKind)
		claimPlan.AtomicClaims = normalizeAuthorAtomicEvidenceSpecs(claimPlan.AtomicClaims, claimPlan.Text)
		claimPlan.RequiredEvidence = normalizeAuthorEvidenceRequirements(claimPlan.RequiredEvidence)
		for i := range claimPlan.RequiredEvidence {
			claimPlan.RequiredEvidence[i] = enrichAuthorEvidenceRequirementSpec(claimPlan.RequiredEvidence[i], claimPlan.Text)
		}
		claimPlan.PreferredSources = trimmedStringSlice(claimPlan.PreferredSources)
		claimPlan.Queries = trimmedStringSlice(claimPlan.Queries)
		claimPlan.ScopeCaveat = strings.TrimSpace(claimPlan.ScopeCaveat)
		normalizedClaims = append(normalizedClaims, claimPlan)
	}
	normalizedInferences := make([]authorInferenceVerificationPlan, 0, len(plan.InferencePlans))
	seenInferences := map[string]struct{}{}
	for _, inferencePlan := range plan.InferencePlans {
		inferencePlan.InferenceID = strings.TrimSpace(inferencePlan.InferenceID)
		if inferencePlan.InferenceID == "" {
			continue
		}
		if _, ok := seenInferences[inferencePlan.InferenceID]; ok {
			continue
		}
		seenInferences[inferencePlan.InferenceID] = struct{}{}
		inferencePlan.From = strings.TrimSpace(inferencePlan.From)
		inferencePlan.To = strings.TrimSpace(inferencePlan.To)
		inferencePlan.Steps = trimmedStringSlice(inferencePlan.Steps)
		inferencePlan.RequiredEvidence = normalizeAuthorEvidenceRequirements(inferencePlan.RequiredEvidence)
		inferenceContext := strings.TrimSpace(inferencePlan.From + " " + strings.Join(inferencePlan.Steps, " ") + " " + inferencePlan.To)
		for i := range inferencePlan.RequiredEvidence {
			inferencePlan.RequiredEvidence[i] = enrichAuthorEvidenceRequirementSpec(inferencePlan.RequiredEvidence[i], inferenceContext)
		}
		inferencePlan.Queries = trimmedStringSlice(inferencePlan.Queries)
		inferencePlan.MissingPremises = trimmedStringSlice(inferencePlan.MissingPremises)
		normalizedInferences = append(normalizedInferences, inferencePlan)
	}
	plan.ClaimPlans = normalizedClaims
	plan.InferencePlans = normalizedInferences
	return plan
}

func normalizeAuthorAtomicEvidenceSpecs(specs []AuthorAtomicEvidenceSpec, context string) []AuthorAtomicEvidenceSpec {
	if len(specs) == 0 {
		return nil
	}
	out := make([]AuthorAtomicEvidenceSpec, 0, len(specs))
	for _, spec := range specs {
		spec.Text = strings.TrimSpace(spec.Text)
		spec.Subject = strings.TrimSpace(spec.Subject)
		spec.Metric = strings.TrimSpace(spec.Metric)
		spec.OriginalValue = strings.TrimSpace(spec.OriginalValue)
		spec.Unit = strings.TrimSpace(spec.Unit)
		spec.TimeWindow = strings.TrimSpace(spec.TimeWindow)
		spec.SourceType = strings.TrimSpace(spec.SourceType)
		spec.Series = strings.TrimSpace(spec.Series)
		spec.Entity = strings.TrimSpace(spec.Entity)
		spec.Geography = strings.TrimSpace(spec.Geography)
		spec.Denominator = strings.TrimSpace(spec.Denominator)
		spec.PreferredSources = trimmedStringSlice(spec.PreferredSources)
		spec.Queries = trimmedStringSlice(spec.Queries)
		spec.ComparisonRule = strings.TrimSpace(spec.ComparisonRule)
		spec.ScopeCaveat = strings.TrimSpace(spec.ScopeCaveat)
		if spec.Text == "" && spec.Subject == "" && spec.Metric == "" {
			continue
		}
		enriched := enrichAuthorEvidenceRequirementSpec(AuthorEvidenceRequirement{
			Description:      spec.Text,
			Subject:          spec.Subject,
			Metric:           spec.Metric,
			OriginalValue:    spec.OriginalValue,
			Unit:             spec.Unit,
			TimeWindow:       spec.TimeWindow,
			SourceType:       spec.SourceType,
			Series:           spec.Series,
			Entity:           spec.Entity,
			Geography:        spec.Geography,
			Denominator:      spec.Denominator,
			PreferredSources: spec.PreferredSources,
			Queries:          spec.Queries,
			ComparisonRule:   spec.ComparisonRule,
			ScopeCaveat:      spec.ScopeCaveat,
		}, context+" "+spec.Text)
		spec.Series = enriched.Series
		spec.PreferredSources = enriched.PreferredSources
		spec.Queries = enriched.Queries
		spec.ComparisonRule = enriched.ComparisonRule
		spec.ScopeCaveat = firstNonEmpty(spec.ScopeCaveat, enriched.ScopeCaveat)
		if spec.OriginalValue == "" {
			spec.OriginalValue = enriched.OriginalValue
		}
		out = append(out, spec)
	}
	return out
}

func enrichAuthorEvidenceRequirementSpec(requirement AuthorEvidenceRequirement, context string) AuthorEvidenceRequirement {
	joined := strings.ToLower(strings.TrimSpace(strings.Join([]string{
		context,
		requirement.Description,
		requirement.Subject,
		requirement.Metric,
		requirement.TimeWindow,
	}, " ")))
	if requirement.OriginalValue == "" {
		requirement.OriginalValue = firstAuthorNumericValue(context)
	}
	switch {
	case containsAny(joined, "bank reserves", "reserve balances", "银行准备金", "商业银行准备金"):
		requirement.Series = firstNonEmpty(requirement.Series, "FRED:WRESBAL")
		requirement.PreferredSources = appendAuthorValidationUniqueStrings(requirement.PreferredSources, "Federal Reserve H.4.1", "FRED WRESBAL")
		requirement.Queries = appendAuthorValidationUniqueStrings(requirement.Queries, "FRED WRESBAL reserve balances "+requirement.TimeWindow, "Federal Reserve H.4.1 reserve balances "+requirement.TimeWindow)
		requirement.ComparisonRule = firstNonEmpty(requirement.ComparisonRule, "Compare nearest weekly reserve-balance values in the stated window against the author's value or threshold; preserve whether the claim is a level, trend, peak, trough, or threshold breach.")
		requirement.ScopeCaveat = firstNonEmpty(requirement.ScopeCaveat, "This verifies bank reserve balances, not Fed total assets, M2, or a broad liquidity proxy.")
	case containsAny(joined, "fed balance sheet", "federal reserve balance sheet", "total assets", "美联储资产负债表", "美联储总资产"):
		requirement.Series = firstNonEmpty(requirement.Series, "FRED:WALCL")
		requirement.PreferredSources = appendAuthorValidationUniqueStrings(requirement.PreferredSources, "Federal Reserve H.4.1", "FRED WALCL")
		requirement.Queries = appendAuthorValidationUniqueStrings(requirement.Queries, "FRED WALCL Fed total assets "+requirement.TimeWindow, "Federal Reserve H.4.1 total assets "+requirement.TimeWindow)
		requirement.ComparisonRule = firstNonEmpty(requirement.ComparisonRule, "Compare nearest weekly Fed total-assets values in the stated window against the author's level or change.")
	case containsAny(joined, "treasury general account", " tga", "tga ", "财政部tga", "tga余额"):
		requirement.Series = firstNonEmpty(requirement.Series, "FRED:WTREGEN")
		requirement.PreferredSources = appendAuthorValidationUniqueStrings(requirement.PreferredSources, "US Treasury Quarterly Refunding Announcement", "Daily Treasury Statement", "FRED WTREGEN")
		requirement.Queries = appendAuthorValidationUniqueStrings(requirement.Queries, "Treasury Quarterly Refunding TGA balance forecast "+requirement.TimeWindow, "FRED WTREGEN Treasury General Account "+requirement.TimeWindow)
		requirement.ComparisonRule = firstNonEmpty(requirement.ComparisonRule, "Use official Treasury forecast documents for projected balances and DTS/FRED weekly values for realized balances; compare the stated date window and peak value directly.")
	case containsAny(joined, "stablecoin", "usdt", "usdc", "稳定币"):
		requirement.PreferredSources = appendAuthorValidationUniqueStrings(requirement.PreferredSources, "DeFiLlama stablecoins", "Tether transparency", "Circle transparency")
		requirement.Queries = appendAuthorValidationUniqueStrings(requirement.Queries, "DeFiLlama stablecoins USDT USDC supply "+requirement.TimeWindow, "USDT USDC circulating supply change "+requirement.TimeWindow)
		requirement.ComparisonRule = firstNonEmpty(requirement.ComparisonRule, "Compare circulating-supply deltas for each named stablecoin over the exact window; do not combine issuers unless the author does.")
	case containsAny(joined, "bitcoin spot etf", "spot bitcoin etf", "btc etf", "比特币现货etf"):
		requirement.PreferredSources = appendAuthorValidationUniqueStrings(requirement.PreferredSources, "Farside Investors", "SoSoValue", "Bloomberg ETF flow data")
		requirement.Queries = appendAuthorValidationUniqueStrings(requirement.Queries, "Bitcoin spot ETF net flows "+requirement.TimeWindow, "Farside Bitcoin ETF flows "+requirement.TimeWindow)
		requirement.ComparisonRule = firstNonEmpty(requirement.ComparisonRule, "Compare daily or weekly net flows across the exact window; distinguish continuous net outflow from cumulative net outflow.")
	case containsAny(joined, "jane street", "lawsuit", "class action", "legal filing", "操纵", "集体诉讼"):
		requirement.PreferredSources = appendAuthorValidationUniqueStrings(requirement.PreferredSources, "CourtListener", "PACER", "Reuters", "Bloomberg", "Financial Times")
		requirement.Queries = appendAuthorValidationUniqueStrings(requirement.Queries, "Jane Street Bitcoin manipulation class action lawsuit "+requirement.TimeWindow, "CourtListener Jane Street Bitcoin manipulation "+requirement.TimeWindow)
		requirement.ComparisonRule = firstNonEmpty(requirement.ComparisonRule, "Verify that a legal filing or high-quality news source exists and that it matches both the named party and specific allegation; rumors alone are not support.")
	case containsAny(joined, "rmp", "reserve management purchase", "short-term treasury", "短债购买", "买短债"):
		requirement.PreferredSources = appendAuthorValidationUniqueStrings(requirement.PreferredSources, "FOMC statement", "New York Fed operations", "Federal Reserve press release")
		requirement.Queries = appendAuthorValidationUniqueStrings(requirement.Queries, "Federal Reserve Reserve Management Purchases December 2025 short-term Treasury purchases", "New York Fed RMP monthly purchases "+requirement.TimeWindow)
		requirement.ComparisonRule = firstNonEmpty(requirement.ComparisonRule, "Confirm the official program name, announcement date, purchase start date, asset type, and stated monthly pace.")
	}
	return requirement
}

func firstAuthorNumericValue(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	patterns := []*regexp.Regexp{
		regexp.MustCompile(`(?i)(?:\$|usd\s*)?\d+(?:\.\d+)?\s*(?:万亿|亿|trillion|billion|million|tn|bn|mm|万桶|b/d|bps|%)`),
		regexp.MustCompile(`\d+(?:\.\d+)?\s*-\s*\d+(?:\.\d+)?\s*(?:%|天|日|days|bps)`),
	}
	for _, pattern := range patterns {
		if match := strings.TrimSpace(pattern.FindString(value)); match != "" {
			return match
		}
	}
	return ""
}

func authorExternalEvidenceHintsByClaimID(hints []authorExternalEvidenceHint) map[string][]authorExternalEvidenceHint {
	out := make(map[string][]authorExternalEvidenceHint, len(hints))
	for _, hint := range hints {
		id := strings.TrimSpace(hint.ClaimID)
		if id == "" || len(hint.Results) == 0 {
			continue
		}
		out[id] = append(out[id], hint)
	}
	return out
}

func applyExternalEvidenceHintToClaim(check AuthorClaimCheck, hints []authorExternalEvidenceHint) AuthorClaimCheck {
	if len(hints) == 0 {
		return check
	}
	if result, contradicted := numericExternalEvidenceHintContradictsClaim(check, hints); contradicted {
		return applyExternalEvidenceContradictionToClaim(check, result, "External evidence hint contains comparable numeric data that does not match the author's value.")
	}
	if result, contradicted := qualitativeExternalEvidenceHintContradictsClaim(check, hints); contradicted {
		return applyExternalEvidenceContradictionToClaim(check, result, "External evidence hint contradicts the claim's qualitative condition.")
	}
	if check.Status != AuthorClaimUnverified {
		return check
	}
	result, ok := exactExternalEvidenceHintForClaim(check, hints)
	if !ok {
		check = appendExternalEvidenceHintsToClaim(check, hints)
		result, ok = numericExternalEvidenceHintForClaim(check, hints)
		if !ok {
			return check
		}
	}
	check.Status = AuthorClaimSupported
	evidence := authorExternalEvidenceResultString(result)
	check.Evidence = appendAuthorValidationUniqueString(check.Evidence, evidence)
	for i := range check.RequiredEvidence {
		if check.RequiredEvidence[i].Status == AuthorClaimUnverified {
			check.RequiredEvidence[i].Status = AuthorClaimSupported
		}
		check.RequiredEvidence[i].Evidence = appendAuthorValidationUniqueString(check.RequiredEvidence[i].Evidence, evidence)
		check.RequiredEvidence[i].Reason = "External evidence hint contains the matching official value."
	}
	check.Reason = "External evidence hint contains the matching official value."
	return check
}

func applyExternalEvidenceContradictionToClaim(check AuthorClaimCheck, result authorExternalEvidenceResult, reason string) AuthorClaimCheck {
	check.Status = AuthorClaimContradicted
	evidence := authorExternalEvidenceResultString(result)
	check.Evidence = appendAuthorValidationUniqueString(check.Evidence, evidence)
	for i := range check.RequiredEvidence {
		check.RequiredEvidence[i].Status = AuthorClaimContradicted
		check.RequiredEvidence[i].Evidence = appendAuthorValidationUniqueString(check.RequiredEvidence[i].Evidence, evidence)
		check.RequiredEvidence[i].Reason = reason
	}
	check.Reason = reason
	return check
}

func appendExternalEvidenceHintsToClaim(check AuthorClaimCheck, hints []authorExternalEvidenceHint) AuthorClaimCheck {
	for _, hint := range hints {
		for _, result := range hint.Results {
			evidence := authorExternalEvidenceResultString(result)
			check.Evidence = appendAuthorValidationUniqueString(check.Evidence, evidence)
			for i := range check.RequiredEvidence {
				check.RequiredEvidence[i].Evidence = appendAuthorValidationUniqueString(check.RequiredEvidence[i].Evidence, evidence)
			}
		}
	}
	return check
}

func authorExternalEvidenceResultString(result authorExternalEvidenceResult) string {
	evidence := strings.TrimSpace(result.Title)
	if url := strings.TrimSpace(result.URL); url != "" {
		if evidence == "" {
			evidence = url
		} else {
			evidence += " (" + url + ")"
		}
	}
	if excerpt := strings.TrimSpace(result.Excerpt); excerpt != "" {
		if evidence == "" {
			evidence = excerpt
		} else {
			evidence += ": " + excerpt
		}
	}
	return evidence
}

func numericExternalEvidenceHintForClaim(check AuthorClaimCheck, hints []authorExternalEvidenceHint) (authorExternalEvidenceResult, bool) {
	authorNumbers := authorClaimComparableNumbers(check)
	if len(authorNumbers) == 0 {
		return authorExternalEvidenceResult{}, false
	}
	for _, hint := range hints {
		for _, result := range hint.Results {
			if externalEvidenceNumbersSupport(authorNumbers, result) {
				return result, true
			}
		}
	}
	return authorExternalEvidenceResult{}, false
}

func qualitativeExternalEvidenceHintContradictsClaim(check AuthorClaimCheck, hints []authorExternalEvidenceHint) (authorExternalEvidenceResult, bool) {
	claimText := strings.ToLower(check.Text + " " + check.Reason)
	for _, requirement := range check.RequiredEvidence {
		claimText += " " + strings.ToLower(strings.Join([]string{
			requirement.Description,
			requirement.OriginalValue,
			requirement.Metric,
			requirement.Series,
		}, " "))
	}
	wantsContinuousOutflow := (strings.Contains(claimText, "continuous") || strings.Contains(claimText, "持续")) &&
		(strings.Contains(claimText, "outflow") || strings.Contains(claimText, "流出"))
	if !wantsContinuousOutflow {
		return authorExternalEvidenceResult{}, false
	}
	for _, hint := range hints {
		for _, result := range hint.Results {
			evidenceText := strings.ToLower(strings.Join([]string{result.Title, result.Excerpt}, " "))
			if strings.Contains(evidenceText, "continuous_outflow=false") {
				return result, true
			}
		}
	}
	return authorExternalEvidenceResult{}, false
}

func numericExternalEvidenceHintContradictsClaim(check AuthorClaimCheck, hints []authorExternalEvidenceHint) (authorExternalEvidenceResult, bool) {
	authorNumbers := authorClaimComparableNumbers(check)
	if len(authorNumbers) == 0 {
		return authorExternalEvidenceResult{}, false
	}
	for _, hint := range hints {
		for _, result := range hint.Results {
			sourceValues := externalEvidenceComparableValues(result)
			if len(sourceValues) == 0 {
				continue
			}
			if !externalEvidenceNumbersSupport(authorNumbers, result) && isPreciseNumericEvidenceSource(result) {
				return result, true
			}
		}
	}
	return authorExternalEvidenceResult{}, false
}

func isPreciseNumericEvidenceSource(result authorExternalEvidenceResult) bool {
	text := strings.ToLower(strings.Join([]string{result.URL, result.Title, result.Excerpt}, " "))
	return strings.Contains(text, "fred ") ||
		strings.Contains(text, "fred:") ||
		strings.Contains(text, "defillama stablecoin") ||
		strings.Contains(text, "stablecoins.llama.fi")
}

type authorComparableNumber struct {
	Value      float64
	Unit       string
	Comparator string
}
