package compilev2

import (
	"fmt"
	"github.com/kumaloha/VariX/varix/compile"
	"strings"
)

func isAuthorOnlyEvidenceString(value string) bool {
	normalized := strings.ToLower(strings.TrimSpace(value))
	if normalized == "" {
		return true
	}
	authorPhrases := []string{
		"author states",
		"author states:",
		"author cites",
		"author says",
		"author explicitly",
		"author provides",
		"author attributes",
		"author links",
		"author's stated",
		"the author states",
		"the author says",
		"the author explicitly",
		"direct author claim",
		"directly stated",
		"same as inference",
		"identical logical path",
		"作者",
		"文中",
		"原文",
	}
	for _, phrase := range authorPhrases {
		if strings.Contains(normalized, phrase) {
			return true
		}
	}
	return false
}

func appendAuthorValidationReason(reason, addition string) string {
	reason = strings.TrimSpace(reason)
	addition = strings.TrimSpace(addition)
	if reason == "" {
		return addition
	}
	if addition == "" || strings.Contains(reason, addition) {
		return reason
	}
	return reason + " " + addition
}

func appendAuthorValidationUniqueString(values []string, value string) []string {
	value = strings.TrimSpace(value)
	if value == "" {
		return values
	}
	for _, existing := range values {
		if strings.TrimSpace(existing) == value {
			return values
		}
	}
	return append(values, value)
}

func appendAuthorValidationUniqueStrings(values []string, additions ...string) []string {
	for _, addition := range additions {
		values = appendAuthorValidationUniqueString(values, addition)
	}
	return values
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func firstString(values []string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func containsAny(value string, markers ...string) bool {
	for _, marker := range markers {
		if strings.Contains(value, strings.ToLower(strings.TrimSpace(marker))) {
			return true
		}
	}
	return false
}

func normalizedAuthorInferencePathKey(from string, steps []string, to string) string {
	return strings.TrimSpace(from) + "\x00" + strings.Join(trimmedStringSlice(steps), "\x00") + "\x00" + strings.TrimSpace(to)
}

func defaultMissingAuthorClaimCheck(candidate authorClaimCandidate) compile.AuthorClaimCheck {
	check := compile.AuthorClaimCheck{
		ClaimID: candidate.ClaimID,
		Text:    candidate.Text,
		Status:  compile.AuthorClaimUnverified,
		Reason:  "validator did not return this concrete proof candidate",
	}
	if isAuthorNarrativeClaimKind(candidate.Kind) {
		check.Status = compile.AuthorClaimInterpretive
		check.Reason = "validator did not return this narrative candidate; defer abstract point validation to inference checks unless a concrete subclaim is explicitly checked"
	}
	return check
}

func isAuthorNarrativeClaimKind(kind string) bool {
	switch strings.TrimSpace(kind) {
	case "render_node", "driver", "target", "explanation", "branch_thesis", "branch_anchor", "branch_driver", "branch_target":
		return true
	default:
		return false
	}
}

func normalizeAuthorClaimStatus(status compile.AuthorClaimStatus) compile.AuthorClaimStatus {
	switch status {
	case compile.AuthorClaimSupported, compile.AuthorClaimContradicted, compile.AuthorClaimUnverified, compile.AuthorClaimInterpretive, compile.AuthorClaimNotAuthorClaim:
		return status
	default:
		return compile.AuthorClaimUnverified
	}
}

func normalizeAuthorEvidenceRequirements(requirements []compile.AuthorEvidenceRequirement) []compile.AuthorEvidenceRequirement {
	if len(requirements) == 0 {
		return nil
	}
	out := make([]compile.AuthorEvidenceRequirement, 0, len(requirements))
	for _, requirement := range requirements {
		requirement.Description = strings.TrimSpace(requirement.Description)
		requirement.Subject = strings.TrimSpace(requirement.Subject)
		requirement.Metric = strings.TrimSpace(requirement.Metric)
		requirement.OriginalValue = strings.TrimSpace(requirement.OriginalValue)
		requirement.Unit = strings.TrimSpace(requirement.Unit)
		requirement.TimeWindow = strings.TrimSpace(requirement.TimeWindow)
		requirement.SourceType = strings.TrimSpace(requirement.SourceType)
		requirement.Series = strings.TrimSpace(requirement.Series)
		requirement.Entity = strings.TrimSpace(requirement.Entity)
		requirement.Geography = strings.TrimSpace(requirement.Geography)
		requirement.Denominator = strings.TrimSpace(requirement.Denominator)
		requirement.PreferredSources = trimmedStringSlice(requirement.PreferredSources)
		requirement.Queries = trimmedStringSlice(requirement.Queries)
		requirement.ComparisonRule = strings.TrimSpace(requirement.ComparisonRule)
		requirement.ScopeCaveat = strings.TrimSpace(requirement.ScopeCaveat)
		requirement.Status = normalizeAuthorClaimStatus(requirement.Status)
		if requirement.Description == "" && requirement.Subject == "" && requirement.Metric == "" {
			continue
		}
		out = append(out, requirement)
	}
	return out
}

func normalizeAuthorSubclaims(parentID string, subclaims []compile.AuthorSubclaim) []compile.AuthorSubclaim {
	if len(subclaims) == 0 {
		return nil
	}
	out := make([]compile.AuthorSubclaim, 0, len(subclaims))
	for i, subclaim := range subclaims {
		subclaim.SubclaimID = strings.TrimSpace(subclaim.SubclaimID)
		if subclaim.SubclaimID == "" {
			subclaim.SubclaimID = fmt.Sprintf("%s.%d", parentID, i+1)
		}
		if strings.TrimSpace(subclaim.ParentClaimID) == "" {
			subclaim.ParentClaimID = parentID
		}
		subclaim.ScopeStatus = normalizeAuthorScopeStatus(subclaim.ScopeStatus)
		subclaim.Status = normalizeAuthorClaimStatus(subclaim.Status)
		if subclaim.ScopeStatus == "mismatch" && subclaim.Status == compile.AuthorClaimSupported {
			subclaim.Status = compile.AuthorClaimContradicted
		}
		out = append(out, subclaim)
	}
	return out
}

func normalizeAuthorScopeStatus(status string) string {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "exact_match", "related_scope", "mismatch", "unknown":
		return strings.ToLower(strings.TrimSpace(status))
	default:
		return ""
	}
}

func aggregateClaimStatusFromSubclaims(status compile.AuthorClaimStatus, subclaims []compile.AuthorSubclaim) compile.AuthorClaimStatus {
	if len(subclaims) == 0 {
		return status
	}
	hasContradicted := false
	hasUnverified := false
	hasInterpretive := false
	hasNotAuthor := false
	allSupported := true
	for _, subclaim := range subclaims {
		switch subclaim.Status {
		case compile.AuthorClaimContradicted:
			hasContradicted = true
			allSupported = false
		case compile.AuthorClaimUnverified:
			hasUnverified = true
			allSupported = false
		case compile.AuthorClaimInterpretive:
			hasInterpretive = true
			allSupported = false
		case compile.AuthorClaimNotAuthorClaim:
			hasNotAuthor = true
			allSupported = false
		case compile.AuthorClaimSupported:
		default:
			hasUnverified = true
			allSupported = false
		}
	}
	switch {
	case hasContradicted:
		return compile.AuthorClaimContradicted
	case hasUnverified:
		return compile.AuthorClaimUnverified
	case hasNotAuthor:
		return compile.AuthorClaimNotAuthorClaim
	case hasInterpretive:
		return compile.AuthorClaimInterpretive
	case allSupported:
		if status == compile.AuthorClaimInterpretive {
			return compile.AuthorClaimInterpretive
		}
		return compile.AuthorClaimSupported
	default:
		return status
	}
}

func normalizeAuthorInferenceStatus(status compile.AuthorInferenceStatus) compile.AuthorInferenceStatus {
	switch status {
	case compile.AuthorInferenceSound, compile.AuthorInferenceWeak, compile.AuthorInferenceUnsupportedJump, compile.AuthorInferenceNotAuthorInference:
		return status
	default:
		return compile.AuthorInferenceWeak
	}
}

func summarizeAuthorValidation(validation compile.AuthorValidation) compile.AuthorValidationSummary {
	var summary compile.AuthorValidationSummary
	for _, check := range validation.ClaimChecks {
		switch check.Status {
		case compile.AuthorClaimSupported:
			summary.SupportedClaims++
		case compile.AuthorClaimContradicted:
			summary.ContradictedClaims++
		case compile.AuthorClaimUnverified:
			summary.UnverifiedClaims++
		case compile.AuthorClaimInterpretive:
			summary.InterpretiveClaims++
		case compile.AuthorClaimNotAuthorClaim:
			summary.NotAuthorClaims++
		}
	}
	for _, check := range validation.InferenceChecks {
		switch check.Status {
		case compile.AuthorInferenceSound:
			summary.SoundInferences++
		case compile.AuthorInferenceWeak:
			summary.WeakInferences++
		case compile.AuthorInferenceUnsupportedJump:
			summary.UnsupportedInferences++
		case compile.AuthorInferenceNotAuthorInference:
			summary.NotAuthorInferences++
		}
	}
	switch {
	case len(validation.ClaimChecks) == 0 && len(validation.InferenceChecks) == 0:
		summary.Verdict = "insufficient_evidence"
	case summary.ContradictedClaims > 0 || summary.UnsupportedInferences > 0:
		summary.Verdict = "high_risk"
	case summary.UnverifiedClaims > 0 || summary.WeakInferences > 0 || summary.NotAuthorClaims > 0 || summary.NotAuthorInferences > 0:
		summary.Verdict = "mixed"
	default:
		summary.Verdict = "credible"
	}
	return summary
}

func firstTrimmed(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func cloneStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	return append([]string(nil), values...)
}
