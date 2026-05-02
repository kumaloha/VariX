package compile

import (
	"strings"
)

func legalClaimNeedsSpecificAllegationSupport(check AuthorClaimCheck) bool {
	text := strings.ToLower(check.Text + " " + check.Reason)
	for _, requirement := range check.RequiredEvidence {
		text += " " + strings.ToLower(strings.Join([]string{
			requirement.Description,
			requirement.Subject,
			requirement.Metric,
			requirement.OriginalValue,
			requirement.SourceType,
			requirement.ComparisonRule,
			requirement.ScopeCaveat,
			strings.Join(requirement.PreferredSources, " "),
		}, " "))
	}
	hasLegalSubject := containsAny(text, "lawsuit", "class action", "courtlistener", "pacer", "legal", "诉讼", "集体诉讼", "jane street")
	hasSpecificMethod := containsAny(text,
		"timed selling",
		"daily",
		"large sell",
		"sell order",
		"liquidation",
		"forced liquidation",
		"buy back",
		"low-level buying",
		"每日",
		"定时",
		"大额抛售",
		"抛售",
		"爆仓",
		"低位",
		"补仓",
	)
	return hasLegalSubject && hasSpecificMethod
}

func legalEvidenceSupportsSpecificMethod(externalText string) bool {
	return containsAny(externalText,
		"timed selling",
		"daily sell",
		"large sell",
		"sell order",
		"liquidation",
		"forced liquidation",
		"buy back",
		"low-level buying",
		"每日",
		"定时",
		"大额抛售",
		"爆仓",
		"低位补仓",
	)
}
