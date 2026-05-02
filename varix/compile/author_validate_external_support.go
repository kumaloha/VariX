package compile

import (
	"regexp"
	"strings"
)

func externalEvidenceStringsForClaim(check AuthorClaimCheck) []string {
	out := make([]string, 0, len(check.Evidence)+len(check.RequiredEvidence))
	for _, evidence := range check.Evidence {
		if isExternalEvidenceString(evidence) {
			out = append(out, evidence)
		}
	}
	for _, requirement := range check.RequiredEvidence {
		for _, evidence := range requirement.Evidence {
			if isExternalEvidenceString(evidence) {
				out = append(out, evidence)
			}
		}
	}
	return out
}

func hasExternalClaimSupport(check AuthorClaimCheck) bool {
	if hasExternalEvidenceStrings(check.Evidence) {
		return true
	}
	for _, requirement := range check.RequiredEvidence {
		if requirementHasExternalSupport(requirement) {
			return true
		}
	}
	for _, subclaim := range check.Subclaims {
		if subclaim.Status != AuthorClaimSupported {
			continue
		}
		if hasExternalEvidenceStrings(subclaim.Evidence) ||
			strings.TrimSpace(subclaim.EvidenceValue) != "" ||
			strings.TrimSpace(subclaim.EvidenceRange) != "" {
			return true
		}
	}
	return false
}

func hasExternalInferenceSupport(check AuthorInferenceCheck) bool {
	if hasExternalEvidenceStrings(check.Evidence) {
		return true
	}
	for _, requirement := range check.RequiredEvidence {
		if requirementHasExternalSupport(requirement) {
			return true
		}
	}
	return false
}

func requirementHasExternalSupport(requirement AuthorEvidenceRequirement) bool {
	if requirement.Status != AuthorClaimSupported {
		return false
	}
	if strings.EqualFold(strings.TrimSpace(requirement.SourceType), "author_source") {
		return false
	}
	return hasExternalEvidenceStrings(requirement.Evidence)
}

func hasExternalEvidenceStrings(values []string) bool {
	for _, value := range values {
		if isExternalEvidenceString(value) {
			return true
		}
	}
	return false
}

func isExternalEvidenceString(value string) bool {
	normalized := strings.ToLower(strings.TrimSpace(value))
	if isAuthorOnlyEvidenceString(normalized) {
		return false
	}
	if isVagueExternalEvidenceString(normalized) {
		return false
	}
	externalMarkers := []string{
		"http://",
		"https://",
		"www.",
		".gov",
		"source says",
		"official source",
		"official data",
		"official report",
		"official release",
		"central bank release",
		"company filing",
		"market data",
		"reports",
		"report:",
		"release:",
		"filing:",
		"eia ",
		"eia:",
		"steo",
		"iea ",
		"iea:",
		"fred",
		"defillama",
		"sosovalue",
		"courtlistener",
		"pacer",
		"treasury",
		"world gold council",
		"wgc",
		"bloomberg",
		"reuters",
		"s&p global",
		"federal reserve",
		"cbo",
		"bea",
		"bls",
		"sec ",
		"sec:",
	}
	for _, marker := range externalMarkers {
		if strings.Contains(normalized, marker) {
			return true
		}
	}
	return false
}

func isVagueExternalEvidenceString(normalized string) bool {
	vagueMarkers := []string{
		"industry reports",
		"market data shows",
		"data shows",
		"reports show",
		"research shows",
		"often",
		"typically",
		"e.g.",
		"for example",
		"generally",
	}
	hasVagueMarker := false
	for _, marker := range vagueMarkers {
		if strings.Contains(normalized, marker) {
			hasVagueMarker = true
			break
		}
	}
	if !hasVagueMarker {
		return false
	}
	concreteMarkers := []string{
		"http://",
		"https://",
		".gov",
		"fred:",
		"fred ",
		"steo",
		"courtlistener",
		"pacer",
	}
	for _, marker := range concreteMarkers {
		if strings.Contains(normalized, marker) {
			return false
		}
	}
	hasNumber := regexp.MustCompile(`\d`).MatchString(normalized)
	hasNamedDatedSource := regexp.MustCompile(`(?i)(jan|feb|mar|apr|may|jun|jul|aug|sep|oct|nov|dec|20\d{2}|q[1-4])`).MatchString(normalized)
	if hasNumber && hasNamedDatedSource {
		return false
	}
	return true
}
