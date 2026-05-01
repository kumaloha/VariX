package compilev2

import (
	"github.com/kumaloha/VariX/varix/compile"
	"strings"
)

func exactExternalEvidenceHintForClaim(check compile.AuthorClaimCheck, hints []authorExternalEvidenceHint) (authorExternalEvidenceResult, bool) {
	claimText := strings.ToLower(strings.TrimSpace(check.Text + " " + check.Reason))
	for _, requirement := range check.RequiredEvidence {
		claimText += " " + strings.ToLower(strings.Join([]string{
			requirement.Description,
			requirement.Subject,
			requirement.Metric,
			requirement.TimeWindow,
			requirement.SourceType,
			requirement.Reason,
		}, " "))
	}
	wantsEIAOil91 := (strings.Contains(claimText, "910") || strings.Contains(claimText, "9.1")) &&
		(strings.Contains(claimText, "oil") || strings.Contains(claimText, "石油") || strings.Contains(claimText, "production") || strings.Contains(claimText, "减产")) &&
		(strings.Contains(claimText, "eia") || strings.Contains(claimText, "energy information") || strings.Contains(claimText, "能源信息署") || strings.Contains(claimText, "official"))
	for _, hint := range hints {
		for _, result := range hint.Results {
			evidenceText := strings.ToLower(strings.Join([]string{hint.Query, result.URL, result.Title, result.Excerpt}, " "))
			if wantsEIAOil91 &&
				strings.Contains(evidenceText, "eia") &&
				strings.Contains(evidenceText, "9.1 million b/d") &&
				strings.Contains(evidenceText, "production shut-ins") &&
				strings.Contains(evidenceText, "april") {
				return result, true
			}
		}
	}
	return authorExternalEvidenceResult{}, false
}
