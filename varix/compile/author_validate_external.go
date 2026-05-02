package compile

import (
	"context"
	"net/http"
	"strings"
	"time"
)

func defaultAuthorExternalEvidenceHints(ctx context.Context, claims []authorClaimCandidate, plan authorVerificationPlan) ([]authorExternalEvidenceHint, error) {
	if len(claims) == 0 {
		return nil, nil
	}
	client := &http.Client{Timeout: 12 * time.Second}
	out := make([]authorExternalEvidenceHint, 0)
	plans := authorClaimPlansByID(plan)
	for _, claim := range claims {
		claimPlan := plans[claim.ClaimID]
		out = append(out, buildEIAOilEvidenceHint(ctx, client, claim, claimPlan)...)
		out = append(out, buildPlannedExternalEvidenceHints(ctx, client, claim.ClaimID, claimPlan)...)
	}
	return out, nil
}

func buildEIAOilEvidenceHint(ctx context.Context, client *http.Client, claim authorClaimCandidate, claimPlan authorClaimVerificationPlan) []authorExternalEvidenceHint {
	if !needsEIAOilEvidenceHint(claim, claimPlan) {
		return nil
	}
	hint := authorExternalEvidenceHint{
		ClaimID: claim.ClaimID,
		Query:   `site:eia.gov STEO April 2026 production shut-ins 9.1 million b/d`,
	}
	for _, source := range []struct {
		url   string
		title string
	}{
		{
			url:   "https://www.eia.gov/outlooks/steo/report/global_oil.php/",
			title: "EIA Short-Term Energy Outlook - Global Oil Markets",
		},
		{
			url:   "https://www.eia.gov/pressroom/releases/press586.php",
			title: "EIA press release on Hormuz closure and production outages",
		},
	} {
		excerpt, err := fetchAuthorEvidenceExcerpt(ctx, client, source.url, []string{"9.1 million", "production shut-ins", "April"})
		if err != nil || strings.TrimSpace(excerpt) == "" {
			continue
		}
		hint.Results = append(hint.Results, authorExternalEvidenceResult{
			URL:     source.url,
			Title:   source.title,
			Excerpt: excerpt,
		})
	}
	if len(hint.Results) == 0 {
		return nil
	}
	return []authorExternalEvidenceHint{hint}
}

func buildPlannedExternalEvidenceHints(ctx context.Context, client *http.Client, claimID string, claimPlan authorClaimVerificationPlan) []authorExternalEvidenceHint {
	requirements := authorEvidenceRequirementsForPlan(claimPlan)
	out := make([]authorExternalEvidenceHint, 0)
	seen := map[string]struct{}{}
	for _, requirement := range requirements {
		for _, hint := range buildExternalEvidenceHintsForRequirement(ctx, client, claimID, requirement) {
			key := hint.Query + "\x00" + strings.TrimSpace(claimID)
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			if len(hint.Results) > 0 {
				out = append(out, hint)
			}
		}
	}
	return out
}

func authorEvidenceRequirementsForPlan(claimPlan authorClaimVerificationPlan) []AuthorEvidenceRequirement {
	out := make([]AuthorEvidenceRequirement, 0, len(claimPlan.RequiredEvidence)+len(claimPlan.AtomicClaims))
	out = append(out, claimPlan.RequiredEvidence...)
	for _, atomic := range claimPlan.AtomicClaims {
		out = append(out, AuthorEvidenceRequirement{
			Description:      atomic.Text,
			Subject:          atomic.Subject,
			Metric:           atomic.Metric,
			OriginalValue:    atomic.OriginalValue,
			Unit:             atomic.Unit,
			TimeWindow:       atomic.TimeWindow,
			SourceType:       atomic.SourceType,
			Series:           atomic.Series,
			Entity:           atomic.Entity,
			Geography:        atomic.Geography,
			Denominator:      atomic.Denominator,
			PreferredSources: atomic.PreferredSources,
			Queries:          atomic.Queries,
			ComparisonRule:   atomic.ComparisonRule,
			ScopeCaveat:      atomic.ScopeCaveat,
		})
	}
	return out
}

func buildExternalEvidenceHintsForRequirement(ctx context.Context, client *http.Client, claimID string, requirement AuthorEvidenceRequirement) []authorExternalEvidenceHint {
	if seriesID, ok := fredSeriesID(requirement.Series); ok {
		result, ok := fetchFREDEvidenceResult(ctx, client, seriesID, requirement.TimeWindow, requirement.OriginalValue, requirement.ComparisonRule)
		if ok {
			return []authorExternalEvidenceHint{{
				ClaimID: claimID,
				Query:   firstNonEmpty(firstString(requirement.Queries), "FRED "+seriesID+" "+requirement.TimeWindow),
				Results: []authorExternalEvidenceResult{
					result,
				},
			}}
		}
	}
	if symbols := stablecoinSymbolsForRequirement(requirement); len(symbols) > 0 {
		hint := authorExternalEvidenceHint{
			ClaimID: claimID,
			Query:   firstNonEmpty(firstString(requirement.Queries), "DeFiLlama stablecoins "+strings.Join(symbols, " ")+" "+requirement.TimeWindow),
		}
		for _, symbol := range symbols {
			result, ok := fetchStablecoinEvidenceResult(ctx, client, symbol, requirement.TimeWindow, requirement.OriginalValue)
			if ok {
				hint.Results = append(hint.Results, result)
			}
		}
		if len(hint.Results) > 0 {
			return []authorExternalEvidenceHint{hint}
		}
	}
	if isBitcoinETFRequirement(requirement) {
		result, ok := fetchBitcoinETFEvidenceResult(ctx, client, requirement.TimeWindow)
		if ok {
			return []authorExternalEvidenceHint{{
				ClaimID: claimID,
				Query:   firstNonEmpty(firstString(requirement.Queries), "SoSoValue Bitcoin spot ETF flows "+requirement.TimeWindow),
				Results: []authorExternalEvidenceResult{
					result,
				},
			}}
		}
	}
	if isLegalEvidenceRequirement(requirement) {
		query := firstNonEmpty(firstString(requirement.Queries), strings.TrimSpace(requirement.Subject+" "+requirement.Description))
		result, ok := fetchCourtListenerEvidenceResult(ctx, client, query)
		if ok {
			return []authorExternalEvidenceHint{{
				ClaimID: claimID,
				Query:   "CourtListener " + query,
				Results: []authorExternalEvidenceResult{
					result,
				},
			}}
		}
	}
	return nil
}

func authorClaimPlansByID(plan authorVerificationPlan) map[string]authorClaimVerificationPlan {
	out := make(map[string]authorClaimVerificationPlan, len(plan.ClaimPlans))
	for _, claimPlan := range plan.ClaimPlans {
		id := strings.TrimSpace(claimPlan.ClaimID)
		if id == "" {
			continue
		}
		out[id] = claimPlan
	}
	return out
}

func needsEIAOilEvidenceHint(claim authorClaimCandidate, plan authorClaimVerificationPlan) bool {
	planEvidenceParts := make([]string, 0, len(plan.RequiredEvidence)*4+len(plan.PreferredSources)+len(plan.Queries)+2)
	planEvidenceParts = append(planEvidenceParts, plan.Text, plan.ScopeCaveat)
	for _, atomicClaim := range plan.AtomicClaims {
		planEvidenceParts = append(planEvidenceParts,
			atomicClaim.Text,
			atomicClaim.Subject,
			atomicClaim.Metric,
			atomicClaim.OriginalValue,
			atomicClaim.Unit,
			atomicClaim.TimeWindow,
			atomicClaim.Series,
			atomicClaim.ComparisonRule,
			atomicClaim.ScopeCaveat,
		)
		planEvidenceParts = append(planEvidenceParts, atomicClaim.PreferredSources...)
		planEvidenceParts = append(planEvidenceParts, atomicClaim.Queries...)
	}
	planEvidenceParts = append(planEvidenceParts, plan.PreferredSources...)
	planEvidenceParts = append(planEvidenceParts, plan.Queries...)
	for _, requirement := range plan.RequiredEvidence {
		planEvidenceParts = append(planEvidenceParts, requirement.Description, requirement.Subject, requirement.Metric, requirement.TimeWindow, requirement.SourceType, requirement.Reason)
	}
	text := strings.ToLower(strings.Join([]string{
		claim.Text,
		claim.SourceText,
		claim.SourceQuote,
		claim.Context,
		strings.Join(planEvidenceParts, " "),
	}, " "))
	hasSource := strings.Contains(text, "eia") ||
		strings.Contains(text, "energy information administration") ||
		strings.Contains(text, "美国能源信息署")
	hasOil := strings.Contains(text, "oil") ||
		strings.Contains(text, "crude") ||
		strings.Contains(text, "石油") ||
		strings.Contains(text, "原油") ||
		strings.Contains(text, "减产") ||
		strings.Contains(text, "停产") ||
		strings.Contains(text, "shut-in")
	hasVolume := strings.Contains(text, "9.1") ||
		strings.Contains(text, "910") ||
		strings.Contains(text, "million b/d") ||
		strings.Contains(text, "百万桶") ||
		strings.Contains(text, "万桶")
	return hasSource && hasOil && hasVolume
}
