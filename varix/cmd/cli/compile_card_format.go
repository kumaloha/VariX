package main

import (
	"fmt"
	c "github.com/kumaloha/VariX/varix/compile"
	"strings"
)

func formatCompileCard(projection compileCardProjection) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Summary\n%s\n\n", projection.Summary)
	if len(projection.Topics) > 0 {
		fmt.Fprintf(&b, "Topics\n- %s\n\n", strings.Join(projection.Topics, "\n- "))
	}
	writeCompactNodeSection(&b, "Drivers", truncateList(projection.Drivers, 5))
	writeCompactNodeSection(&b, "Targets", truncateList(projection.Targets, 5))
	writeCompactNodeSection(&b, "Evidence", truncateList(projection.Evidence, 5))
	writeCompactNodeSection(&b, "Explanations", truncateList(projection.Explanations, 5))
	writeBranchSection(&b, projection.Branches, 5)
	if len(projection.LogicChains) > 0 {
		fmt.Fprintf(&b, "Logic chain\n")
		for _, chain := range projection.LogicChains {
			fmt.Fprintf(&b, "- %s\n", chain)
		}
		b.WriteString("\n")
	}
	if len(projection.VerificationSummary) > 0 {
		fmt.Fprintf(&b, "Verification\n")
		for _, line := range projection.VerificationSummary {
			fmt.Fprintf(&b, "- %s\n", line)
		}
		b.WriteString("\n")
	}
	if len(projection.AuthorValidation) > 0 {
		fmt.Fprintf(&b, "Author validation\n")
		for _, line := range projection.AuthorValidation {
			fmt.Fprintf(&b, "- %s\n", line)
		}
		b.WriteString("\n")
	}
	fmt.Fprintf(&b, "Confidence\n%s\n", projection.Confidence)
	return b.String()
}

func formatCompactCompileCard(projection compileCardProjection) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Summary\n%s\n\n", projection.Summary)
	writeCompactNodeSection(&b, "Drivers", truncateList(projection.Drivers, 3))
	writeCompactNodeSection(&b, "Targets", truncateList(projection.Targets, 3))
	writeCompactNodeSection(&b, "Evidence", truncateList(projection.Evidence, 3))
	writeCompactNodeSection(&b, "Explanations", truncateList(projection.Explanations, 2))
	writeBranchSection(&b, projection.Branches, 2)
	if len(projection.LogicChains) > 0 {
		fmt.Fprintf(&b, "Main logic\n- %s\n\n", projection.LogicChains[0])
	}
	if len(projection.AuthorValidation) > 0 {
		fmt.Fprintf(&b, "Author validation\n")
		for _, line := range truncateList(projection.AuthorValidation, 3) {
			fmt.Fprintf(&b, "- %s\n", line)
		}
		b.WriteString("\n")
	}
	fmt.Fprintf(&b, "Confidence\n%s\n", projection.Confidence)
	return b.String()
}

func writeBranchSection(b *strings.Builder, branches []c.Branch, limit int) {
	if len(branches) == 0 || limit <= 0 {
		return
	}
	if len(branches) < limit {
		limit = len(branches)
	}
	fmt.Fprintf(b, "Branches\n")
	for _, branch := range branches[:limit] {
		label := strings.TrimSpace(branch.Thesis)
		if label == "" {
			label = strings.TrimSpace(branch.ID)
		}
		if label == "" {
			label = "branch"
		}
		fmt.Fprintf(b, "- %s\n", label)
		if len(branch.Anchors) > 0 {
			fmt.Fprintf(b, "  - Anchor: %s\n", strings.Join(truncateList(branch.Anchors, 3), " / "))
		}
		if len(branch.BranchDrivers) > 0 {
			fmt.Fprintf(b, "  - Branch driver: %s\n", strings.Join(truncateList(branch.BranchDrivers, 3), " / "))
		}
		for _, chain := range branchLogicChains(branch) {
			fmt.Fprintf(b, "  - %s\n", chain)
		}
	}
	b.WriteString("\n")
}

func branchLogicChains(branch c.Branch) []string {
	if len(branch.TransmissionPaths) == 0 {
		return nil
	}
	chains := make([]string, 0, len(branch.TransmissionPaths))
	for _, path := range branch.TransmissionPaths {
		parts := []string{}
		parts = appendChainPart(parts, path.Driver)
		for _, step := range path.Steps {
			parts = appendChainPart(parts, step)
		}
		parts = appendChainPart(parts, path.Target)
		if len(parts) > 0 {
			chains = append(chains, strings.Join(parts, " -> "))
		}
	}
	return chains
}

func writeCompactNodeSection(b *strings.Builder, title string, items []string) {
	if len(items) == 0 {
		return
	}
	fmt.Fprintf(b, "%s\n", title)
	for _, item := range items {
		fmt.Fprintf(b, "- %s\n", item)
	}
	b.WriteString("\n")
}

func appendChainPart(parts []string, value string) []string {
	value = strings.TrimSpace(value)
	if value == "" {
		return parts
	}
	value = truncate(value, 50)
	if len(parts) > 0 && parts[len(parts)-1] == value {
		return parts
	}
	return append(parts, value)
}
