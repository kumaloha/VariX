package main

import (
	"fmt"
	"strings"

	"github.com/kumaloha/VariX/varix/model"
)

const fullSpeakerClaimLimit = 12

func formatCompileCard(projection compileCardProjection) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Summary\n%s\n\n", projection.Summary)
	writeMainlineSection(&b, projection.Mainline)
	writeTopicsSection(&b, projection.Topics, len(projection.Topics))
	writeKeyPointSection(&b, projection.KeyPoints, len(projection.KeyPoints))
	writeReadableSpeakerClaimSection(&b, projection.SemanticUnits)
	writeDeclarationSection(&b, projection.Declarations, 5)
	writeCompactNodeSection(&b, "Drivers", truncateList(projection.Drivers, 5))
	writeCompactNodeSection(&b, "Targets", truncateList(projection.Targets, 5))
	writeCompactNodeSection(&b, "Evidence", truncateList(projection.Evidence, 5))
	writeCompactNodeSection(&b, "Explanations", truncateList(projection.Explanations, 5))
	writeBranchSection(&b, projection.Branches, 5)
	if len(projection.LogicChains) > 0 {
		fmt.Fprintf(&b, "%s\n", projection.LogicHeading)
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
	writeMainlineSection(&b, truncateList(projection.Mainline, 3))
	writeTopicsSection(&b, projection.Topics, 3)
	writeKeyPointSection(&b, projection.KeyPoints, 3)
	writeDeclarationSection(&b, truncateDeclarations(projection.Declarations, 2), 2)
	writeCompactNodeSection(&b, "Drivers", truncateList(projection.Drivers, 3))
	writeCompactNodeSection(&b, "Targets", truncateList(projection.Targets, 3))
	writeCompactNodeSection(&b, "Evidence", truncateList(projection.Evidence, 3))
	writeCompactNodeSection(&b, "Explanations", truncateList(projection.Explanations, 2))
	writeBranchSection(&b, projection.Branches, 2)
	if len(projection.LogicChains) > 0 {
		heading := projection.LogicHeading
		if heading == "Logic chain" {
			heading = "Main logic"
		}
		fmt.Fprintf(&b, "%s\n- %s\n\n", heading, projection.LogicChains[0])
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

func writeMainlineSection(b *strings.Builder, lines []string) {
	if len(lines) == 0 {
		return
	}
	fmt.Fprintf(b, "Mainline\n")
	for _, line := range lines {
		fmt.Fprintf(b, "- %s\n", line)
	}
	b.WriteString("\n")
}

func writeKeyPointSection(b *strings.Builder, points []string, limit int) {
	if len(points) == 0 || limit <= 0 {
		return
	}
	if len(points) < limit {
		limit = len(points)
	}
	fmt.Fprintf(b, "Key points\n")
	for _, point := range points[:limit] {
		fmt.Fprintf(b, "- %s\n", point)
	}
	b.WriteString("\n")
}

func writeReadableSpeakerClaimSection(b *strings.Builder, units []model.SemanticUnit) {
	if len(units) == 0 {
		return
	}
	if len(units) > fullSpeakerClaimLimit {
		fmt.Fprintf(b, "Speaker claims\n")
		fmt.Fprintf(b, "- %d total claims stored in the compile output; use `compile show` for the full inventory.\n\n", len(units))
		return
	}
	writeSemanticUnitSection(b, units, len(units))
}

func writeTopicsSection(b *strings.Builder, topics []string, limit int) {
	if len(topics) == 0 || limit <= 0 {
		return
	}
	if len(topics) > limit {
		topics = topics[:limit]
	}
	fmt.Fprintf(b, "Topics\n- %s\n\n", strings.Join(topics, "\n- "))
}

func writeSemanticUnitSection(b *strings.Builder, units []model.SemanticUnit, limit int) {
	if len(units) == 0 || limit <= 0 {
		return
	}
	if len(units) < limit {
		limit = len(units)
	}
	fmt.Fprintf(b, "Speaker claims\n")
	for _, unit := range units[:limit] {
		header := semanticUnitHeader(unit)
		if header != "" {
			fmt.Fprintf(b, "- %s\n", header)
		}
		if context := strings.TrimSpace(unit.PromptContext); context != "" {
			if strings.EqualFold(strings.TrimSpace(unit.Force), "answer") {
				fmt.Fprintf(b, "  - Question: %s\n", context)
			} else {
				fmt.Fprintf(b, "  - Context: %s\n", context)
			}
		}
		if claim := strings.TrimSpace(unit.Claim); claim != "" {
			label := "Claim"
			if strings.EqualFold(strings.TrimSpace(unit.Force), "answer") {
				label = "Answer"
			}
			fmt.Fprintf(b, "  - %s: %s\n", label, claim)
		}
		if reason := strings.TrimSpace(unit.ImportanceReason); reason != "" {
			fmt.Fprintf(b, "  - Why it matters: %s\n", reason)
		}
	}
	b.WriteString("\n")
}

func semanticUnitHeader(unit model.SemanticUnit) string {
	parts := make([]string, 0, 4)
	if speaker := strings.TrimSpace(unit.Speaker); speaker != "" {
		parts = append(parts, speaker)
	}
	if subject := strings.TrimSpace(unit.Subject); subject != "" {
		parts = append(parts, subject)
	}
	if force := strings.TrimSpace(unit.Force); force != "" {
		parts = append(parts, force)
	}
	return strings.Join(parts, " / ")
}

func truncateSemanticUnits(values []model.SemanticUnit, max int) []model.SemanticUnit {
	if max <= 0 || len(values) <= max {
		return values
	}
	return values[:max]
}

func writeBranchSection(b *strings.Builder, branches []model.Branch, limit int) {
	if len(branches) == 0 || limit <= 0 {
		return
	}
	if len(branches) < limit {
		limit = len(branches)
	}
	fmt.Fprintf(b, "Branches\n")
	for _, branch := range branches[:limit] {
		label := branchCardLabel(branch)
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
		for _, declaration := range branch.Declarations {
			if statement := strings.TrimSpace(declaration.Statement); statement != "" {
				fmt.Fprintf(b, "  - Declaration: %s\n", statement)
			}
		}
	}
	b.WriteString("\n")
}

func writeDeclarationSection(b *strings.Builder, declarations []model.Declaration, limit int) {
	if len(declarations) == 0 || limit <= 0 {
		return
	}
	if len(declarations) < limit {
		limit = len(declarations)
	}
	fmt.Fprintf(b, "Management declarations\n")
	for _, declaration := range declarations[:limit] {
		header := declarationHeader(declaration)
		if header != "" {
			fmt.Fprintf(b, "- %s\n", header)
		}
		if statement := strings.TrimSpace(declaration.Statement); statement != "" {
			fmt.Fprintf(b, "  - Statement: %s\n", statement)
		}
		if read := declarationReading(declaration); read != "" {
			fmt.Fprintf(b, "  - Read: %s\n", read)
		}
		for _, condition := range truncateList(declaration.Conditions, 3) {
			fmt.Fprintf(b, "  - Condition: %s\n", condition)
		}
		for _, action := range truncateList(declaration.Actions, 3) {
			if sameCardText(action, declaration.Statement) {
				continue
			}
			fmt.Fprintf(b, "  - Action: %s\n", action)
		}
		if scale := strings.TrimSpace(declaration.Scale); scale != "" {
			fmt.Fprintf(b, "  - Scale: %s\n", scale)
		}
		for _, boundary := range truncateList(declaration.Constraints, 3) {
			fmt.Fprintf(b, "  - Boundary: %s\n", boundary)
		}
		for _, nonAction := range truncateList(declaration.NonActions, 3) {
			fmt.Fprintf(b, "  - Non-action: %s\n", nonAction)
		}
		for _, evidence := range truncateList(declaration.Evidence, 2) {
			fmt.Fprintf(b, "  - Evidence: %s\n", evidence)
		}
	}
	b.WriteString("\n")
}

func declarationHeader(declaration model.Declaration) string {
	parts := make([]string, 0, 3)
	if speaker := strings.TrimSpace(declaration.Speaker); speaker != "" {
		parts = append(parts, speaker)
	}
	if topic := strings.TrimSpace(declaration.Topic); topic != "" {
		parts = append(parts, topic)
	}
	if kind := strings.TrimSpace(declaration.Kind); kind != "" && kind != declaration.Topic {
		parts = append(parts, kind)
	}
	return strings.Join(parts, " / ")
}

func truncateDeclarations(values []model.Declaration, max int) []model.Declaration {
	if max <= 0 || len(values) <= max {
		return values
	}
	return values[:max]
}

func branchCardLabel(branch model.Branch) string {
	for _, declaration := range branch.Declarations {
		if statement := strings.TrimSpace(declaration.Statement); statement != "" {
			return truncate(statement, 120)
		}
	}
	label := strings.TrimSpace(branch.Thesis)
	if label != "" && containsCJK(label) {
		return label
	}
	if len(branch.BranchDrivers) > 0 || len(branch.Targets) > 0 {
		parts := make([]string, 0, 2)
		if len(branch.BranchDrivers) > 0 {
			parts = append(parts, strings.Join(truncateList(branch.BranchDrivers, 2), " / "))
		}
		if len(branch.Targets) > 0 {
			parts = append(parts, strings.Join(truncateList(branch.Targets, 2), " / "))
		}
		if len(parts) > 0 {
			return strings.Join(parts, " -> ")
		}
	}
	if label != "" {
		return truncate(label, 120)
	}
	if id := strings.TrimSpace(branch.ID); id != "" {
		return id
	}
	return "branch"
}

func declarationReading(declaration model.Declaration) string {
	if !isCapitalAllocationCardDeclaration(declaration) {
		return genericDeclarationReading(declaration)
	}
	clauses := []string{fmt.Sprintf("这是%s的资本配置纪律", capitalAllocationSubject(declaration))}
	if len(declaration.Constraints) > 0 {
		boundary := joinCardPhrases(declaration.Constraints, 2)
		if strings.HasPrefix(boundary, "保持") {
			clauses = append(clauses, "平时"+boundary)
		} else {
			clauses = append(clauses, "平时以"+boundary+"为边界")
		}
	} else {
		clauses = append(clauses, "平时不被现金规模逼着出手")
	}
	if len(declaration.Conditions) > 0 {
		clauses = append(clauses, "触发条件是"+joinCardPhrases(declaration.Conditions, 2))
	}
	action := joinCardPhrases(declaration.Actions, 2)
	scale := trimCardSentence(declaration.Scale)
	switch {
	case action != "" && scale != "":
		clauses = append(clauses, "条件满足后"+action+"，规模是"+scale)
	case action != "":
		clauses = append(clauses, "条件满足后"+action)
	case scale != "":
		clauses = append(clauses, "条件满足后可以"+scale)
	}
	return strings.Join(clauses, "；") + "。"
}

func capitalAllocationSubject(declaration model.Declaration) string {
	for _, text := range []string{
		declaration.Statement,
		declaration.Topic,
		declaration.SourceQuote,
		strings.Join(declaration.Conditions, " "),
		strings.Join(declaration.Actions, " "),
		strings.Join(declaration.Constraints, " "),
	} {
		lower := strings.ToLower(strings.TrimSpace(text))
		if strings.Contains(text, "伯克希尔") || strings.Contains(lower, "berkshire") {
			return "伯克希尔"
		}
	}
	if speaker := strings.TrimSpace(declaration.Speaker); speaker != "" {
		return speaker
	}
	return "组织"
}

func genericDeclarationReading(declaration model.Declaration) string {
	hasSlots := len(declaration.Conditions) > 0 ||
		len(declaration.Actions) > 0 ||
		strings.TrimSpace(declaration.Scale) != "" ||
		len(declaration.Constraints) > 0 ||
		len(declaration.NonActions) > 0
	if !hasSlots {
		return ""
	}
	parts := []string{"这是管理层的明确表述，不是外部推测"}
	if len(declaration.Conditions) > 0 {
		parts = append(parts, "条件是"+joinCardPhrases(declaration.Conditions, 2))
	}
	actions := nonDuplicateCardTexts(declaration.Actions, declaration.Statement)
	if len(actions) > 0 {
		parts = append(parts, "动作是"+joinCardPhrases(actions, 2))
	}
	if scale := trimCardSentence(declaration.Scale); scale != "" {
		parts = append(parts, "规模是"+scale)
	}
	if len(declaration.Constraints) > 0 {
		parts = append(parts, "边界是"+joinCardPhrases(declaration.Constraints, 2))
	}
	if len(parts) == 1 {
		return ""
	}
	return strings.Join(parts, "；") + "。"
}

func joinCardPhrases(values []string, limit int) string {
	out := make([]string, 0, limit)
	for _, value := range values {
		value = trimCardSentence(truncate(value, 100))
		if value == "" {
			continue
		}
		out = append(out, value)
		if len(out) == limit {
			break
		}
	}
	return strings.Join(out, "、")
}

func trimCardSentence(value string) string {
	return strings.TrimRight(strings.TrimSpace(value), "。.;； ")
}

func nonDuplicateCardTexts(values []string, reference string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		if sameCardText(value, reference) {
			continue
		}
		out = append(out, value)
	}
	return out
}

func isCapitalAllocationCardDeclaration(declaration model.Declaration) bool {
	text := strings.ToLower(strings.Join([]string{declaration.Kind, declaration.Topic, declaration.Statement}, " "))
	return strings.Contains(text, "capital_allocation") ||
		strings.Contains(text, "capital allocation") ||
		strings.Contains(text, "资本配置") ||
		strings.Contains(text, "配置资本")
}

func sameCardText(a, b string) bool {
	return normalizeCardText(a) == normalizeCardText(b)
}

func normalizeCardText(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	return strings.Trim(value, "。.;； ")
}

func containsCJK(value string) bool {
	for _, r := range value {
		if r >= 0x4e00 && r <= 0x9fff {
			return true
		}
	}
	return false
}

func branchLogicChains(branch model.Branch) []string {
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
