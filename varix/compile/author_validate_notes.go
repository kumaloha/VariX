package compile

import (
	"strings"
)

func fillAuthorClaimDecisionNote(check AuthorClaimCheck) AuthorClaimCheck {
	for i := range check.Subclaims {
		check.Subclaims[i].DecisionNote = authorSubclaimDecisionNote(check.Subclaims[i])
	}
	check.DecisionNote = authorClaimDecisionNote(check)
	return check
}

func authorClaimDecisionNote(check AuthorClaimCheck) string {
	basis := authorClaimBasisSummary(check)
	reason := authorClaimReasonSummary(check)
	parts := make([]string, 0, 2)
	if basis != "" {
		parts = append(parts, "口径: "+basis)
	}
	parts = append(parts, "判定: "+firstNonEmpty(reason, string(check.Status)))
	return truncateAuthorEvidenceExcerpt(strings.Join(parts, "；"), 480)
}

func authorClaimBasisSummary(check AuthorClaimCheck) string {
	for _, requirement := range check.RequiredEvidence {
		if basis := authorRequirementBasisSummary(requirement); basis != "" {
			return basis
		}
	}
	for _, subclaim := range check.Subclaims {
		parts := make([]string, 0, 5)
		if subject := strings.TrimSpace(subclaim.Subject); subject != "" {
			parts = append(parts, subject)
		}
		if metric := strings.TrimSpace(subclaim.Metric); metric != "" {
			parts = append(parts, metric)
		}
		if value := strings.TrimSpace(subclaim.OriginalValue); value != "" {
			parts = append(parts, "作者值 "+value)
		}
		if base := strings.TrimSpace(subclaim.ComparisonBase); base != "" {
			parts = append(parts, "分母/对象 "+base)
		}
		if evidenceBase := strings.TrimSpace(subclaim.EvidenceBase); evidenceBase != "" {
			parts = append(parts, "证据对象 "+evidenceBase)
		}
		if len(parts) > 0 {
			return strings.Join(parts, "；")
		}
	}
	return ""
}

func authorRequirementBasisSummary(requirement AuthorEvidenceRequirement) string {
	parts := make([]string, 0, 8)
	if subject := strings.TrimSpace(requirement.Subject); subject != "" {
		parts = append(parts, subject)
	}
	if metric := strings.TrimSpace(requirement.Metric); metric != "" {
		parts = append(parts, metric)
	}
	if value := strings.TrimSpace(requirement.OriginalValue); value != "" {
		parts = append(parts, "作者值 "+value)
	}
	if unit := strings.TrimSpace(requirement.Unit); unit != "" {
		parts = append(parts, "单位 "+unit)
	}
	if window := strings.TrimSpace(requirement.TimeWindow); window != "" {
		parts = append(parts, "窗口 "+window)
	}
	if source := firstNonEmpty(requirement.Series, requirement.SourceType, firstString(requirement.PreferredSources)); source != "" {
		parts = append(parts, "来源 "+source)
	}
	if denominator := strings.TrimSpace(requirement.Denominator); denominator != "" {
		parts = append(parts, "分母 "+denominator)
	}
	if caveat := strings.TrimSpace(requirement.ScopeCaveat); caveat != "" {
		parts = append(parts, "范围 "+caveat)
	}
	if len(parts) == 0 {
		return ""
	}
	return strings.Join(parts, "；")
}

func authorClaimReasonSummary(check AuthorClaimCheck) string {
	if reason := strings.TrimSpace(check.Reason); reason != "" {
		return reason
	}
	for _, subclaim := range check.Subclaims {
		if subclaim.Status == check.Status {
			if reason := strings.TrimSpace(subclaim.Reason); reason != "" {
				return reason
			}
		}
	}
	for _, requirement := range check.RequiredEvidence {
		if requirement.Status == check.Status {
			if reason := strings.TrimSpace(requirement.Reason); reason != "" {
				return reason
			}
		}
	}
	for _, requirement := range check.RequiredEvidence {
		if reason := strings.TrimSpace(requirement.Reason); reason != "" {
			return reason
		}
	}
	return ""
}

func authorSubclaimDecisionNote(subclaim AuthorSubclaim) string {
	basisParts := make([]string, 0, 6)
	if subject := strings.TrimSpace(subclaim.Subject); subject != "" {
		basisParts = append(basisParts, subject)
	}
	if metric := strings.TrimSpace(subclaim.Metric); metric != "" {
		basisParts = append(basisParts, metric)
	}
	if value := strings.TrimSpace(subclaim.OriginalValue); value != "" {
		basisParts = append(basisParts, "作者值 "+value)
	}
	if evidenceValue := strings.TrimSpace(firstNonEmpty(subclaim.EvidenceValue, subclaim.EvidenceRange)); evidenceValue != "" {
		basisParts = append(basisParts, "证据值 "+evidenceValue)
	}
	if scope := strings.TrimSpace(subclaim.ScopeStatus); scope != "" {
		basisParts = append(basisParts, "范围 "+scope)
	}
	parts := make([]string, 0, 2)
	if len(basisParts) > 0 {
		parts = append(parts, "口径: "+strings.Join(basisParts, "；"))
	}
	parts = append(parts, "判定: "+firstNonEmpty(subclaim.Reason, string(subclaim.Status)))
	return truncateAuthorEvidenceExcerpt(strings.Join(parts, "；"), 480)
}

func fillAuthorInferenceDecisionNote(check AuthorInferenceCheck) AuthorInferenceCheck {
	check.DecisionNote = authorInferenceDecisionNote(check)
	return check
}

func authorInferenceDecisionNote(check AuthorInferenceCheck) string {
	basisParts := make([]string, 0, 2)
	path := authorInferencePathText(check)
	if path != "" {
		basisParts = append(basisParts, "路径 "+path)
	}
	for _, requirement := range check.RequiredEvidence {
		if basis := authorRequirementBasisSummary(requirement); basis != "" {
			basisParts = append(basisParts, "所需证据 "+basis)
			break
		}
	}
	parts := make([]string, 0, 2)
	if len(basisParts) > 0 {
		parts = append(parts, "口径: "+strings.Join(basisParts, "；"))
	}
	parts = append(parts, "判定: "+firstNonEmpty(check.Reason, string(check.Status)))
	if len(check.MissingLinks) > 0 {
		parts = append(parts, "缺口: "+strings.Join(check.MissingLinks, "；"))
	}
	return truncateAuthorEvidenceExcerpt(strings.Join(parts, "；"), 480)
}

func authorInferencePathText(check AuthorInferenceCheck) string {
	parts := make([]string, 0, len(check.Steps)+2)
	if from := strings.TrimSpace(check.From); from != "" {
		parts = append(parts, from)
	}
	parts = append(parts, trimmedStringSlice(check.Steps)...)
	if to := strings.TrimSpace(check.To); to != "" {
		parts = append(parts, to)
	}
	return strings.Join(parts, " -> ")
}
