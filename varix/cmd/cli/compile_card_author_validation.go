package main

import (
	"fmt"
	c "github.com/kumaloha/VariX/varix/compile"
	"strings"
)

func authorValidationSummaryLines(validation c.AuthorValidation) []string {
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
		if check.Status != c.AuthorClaimContradicted && check.Status != c.AuthorClaimUnverified && check.Status != c.AuthorClaimNotAuthorClaim {
			continue
		}
		lines = append(lines, fmt.Sprintf("Claim %s: %s%s", truncate(check.Text, 42), check.Status, authorValidationNoteSuffix(firstNonEmpty(check.DecisionNote, check.Reason))))
		if len(lines) >= 6 {
			break
		}
	}
	for _, check := range validation.InferenceChecks {
		if check.Status != c.AuthorInferenceWeak && check.Status != c.AuthorInferenceUnsupportedJump && check.Status != c.AuthorInferenceNotAuthorInference {
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
