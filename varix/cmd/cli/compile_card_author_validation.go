package main

import (
	"fmt"
	"strings"

	"github.com/kumaloha/VariX/varix/model"
)

func authorValidationSummaryLines(validation model.AuthorValidation) []string {
	if validation.IsZero() {
		return nil
	}
	summary := validation.Summary
	lines := []string{
		"Verdict: " + firstNonEmpty(summary.Verdict, "insufficient_evidence"),
		fmt.Sprintf("Claims: supported %d, contradicted %d, unverified %d, interpretive %d", summary.SupportedClaims, summary.ContradictedClaims, summary.UnverifiedClaims, summary.InterpretiveClaims),
		fmt.Sprintf("Inferences: sound %d, weak %d, unsupported %d", summary.SoundInferences, summary.WeakInferences, summary.UnsupportedInferences),
	}
	if summary.NotAuthorClaims > 0 || summary.NotAuthorInferences > 0 {
		lines = append(lines, fmt.Sprintf("Not author claims/inferences: %d/%d", summary.NotAuthorClaims, summary.NotAuthorInferences))
	}
	for _, check := range validation.ClaimChecks {
		if check.Status != model.AuthorClaimContradicted && check.Status != model.AuthorClaimUnverified && check.Status != model.AuthorClaimNotAuthorClaim {
			continue
		}
		lines = append(lines, fmt.Sprintf("Claim %s: %s%s", truncate(check.Text, 42), check.Status, authorValidationNoteSuffix(firstNonEmpty(check.DecisionNote, check.Reason))))
		if len(lines) >= 6 {
			break
		}
	}
	for _, check := range validation.InferenceChecks {
		if check.Status != model.AuthorInferenceWeak && check.Status != model.AuthorInferenceUnsupportedJump && check.Status != model.AuthorInferenceNotAuthorInference {
			continue
		}
		lines = append(lines, fmt.Sprintf("Path %s -> %s: %s%s", truncate(check.From, 24), truncate(check.To, 24), check.Status, authorValidationNoteSuffix(firstNonEmpty(check.DecisionNote, check.Reason))))
		if len(lines) >= 6 {
			break
		}
	}
	return lines
}

func authorValidationNoteSuffix(note string) string {
	note = strings.TrimSpace(note)
	if note == "" {
		return ""
	}
	return " — 说明: " + truncate(note, 160)
}
