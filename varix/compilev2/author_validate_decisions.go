package compilev2

import (
	"github.com/kumaloha/VariX/varix/compile"
	"strings"
)

func enforceExternalEvidenceForSupportedClaim(check compile.AuthorClaimCheck) compile.AuthorClaimCheck {
	if check.Status != compile.AuthorClaimSupported || hasExternalClaimSupport(check) {
		return check
	}
	check.Status = compile.AuthorClaimUnverified
	for i := range check.RequiredEvidence {
		if check.RequiredEvidence[i].Status == compile.AuthorClaimSupported && !requirementHasExternalSupport(check.RequiredEvidence[i]) {
			check.RequiredEvidence[i].Status = compile.AuthorClaimUnverified
		}
	}
	check.Reason = appendAuthorValidationReason(check.Reason, "Supported checkable claims require external evidence; the returned evidence only restates or cites the author.")
	return check
}

func enforceLegalClaimScope(check compile.AuthorClaimCheck) compile.AuthorClaimCheck {
	if check.Status != compile.AuthorClaimSupported || !legalClaimNeedsSpecificAllegationSupport(check) {
		return check
	}
	externalText := strings.ToLower(strings.Join(externalEvidenceStringsForClaim(check), " "))
	if externalText == "" || legalEvidenceSupportsSpecificMethod(externalText) {
		return check
	}
	check.Status = compile.AuthorClaimUnverified
	for i := range check.RequiredEvidence {
		check.RequiredEvidence[i].Status = compile.AuthorClaimUnverified
		check.RequiredEvidence[i].Reason = "External legal evidence confirms lawsuit/allegation existence but does not verify the specific trading-method detail."
	}
	check.Reason = "External legal evidence confirms lawsuit/allegation existence but does not verify the specific trading-method detail."
	return check
}

func enforceExternalEvidenceForSoundInference(check compile.AuthorInferenceCheck) compile.AuthorInferenceCheck {
	if check.Status != compile.AuthorInferenceSound || hasExternalInferenceSupport(check) {
		return check
	}
	check.Status = compile.AuthorInferenceWeak
	for i := range check.RequiredEvidence {
		if check.RequiredEvidence[i].Status == compile.AuthorClaimSupported && !requirementHasExternalSupport(check.RequiredEvidence[i]) {
			check.RequiredEvidence[i].Status = compile.AuthorClaimUnverified
			check.RequiredEvidence[i].Reason = "Sound inference requires external support for the necessary factual premise, not only author provenance."
		}
	}
	check.Reason = "Sound inference requires external support for the necessary factual premises, not only author provenance."
	check.MissingLinks = appendAuthorValidationUniqueString(check.MissingLinks, "external evidence for the factual premises needed by this inference")
	return check
}
