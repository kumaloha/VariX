package main

import (
	"github.com/kumaloha/VariX/varix/model"
	"strings"
)

type compileCardProjection struct {
	Summary             string
	Mainline            []string
	Topics              []string
	KeyPoints           []string
	Confidence          string
	Drivers             []string
	Targets             []string
	Declarations        []model.Declaration
	SemanticUnits       []model.SemanticUnit
	Brief               []model.BriefItem
	Branches            []model.Branch
	Evidence            []string
	Explanations        []string
	LogicHeading        string
	LogicChains         []string
	VerificationSummary []string
	AuthorValidation    []string
}

func buildCompileCardProjection(record model.Record, subgraph *model.ContentSubgraph) compileCardProjection {
	mainline := compileRecordMainline(record)
	logicChains, logicHeading := compileRecordSecondaryLogic(record, len(mainline) > 0)
	projection := compileCardProjection{
		Summary:          record.Output.Summary,
		Mainline:         mainline,
		Topics:           primaryFirstTopics(record.Output.Branches, record.Output.Topics),
		KeyPoints:        compileRecordKeyPoints(record.Output.Brief, record.Output.SemanticUnits, 12),
		Confidence:       record.Output.Confidence,
		Drivers:          cloneStringSlice(record.Output.Drivers),
		Targets:          cloneStringSlice(record.Output.Targets),
		Declarations:     cloneDeclarations(record.Output.Declarations),
		SemanticUnits:    cloneSemanticUnits(record.Output.SemanticUnits),
		Brief:            cloneBriefItems(record.Output.Brief),
		Branches:         primaryFirstBranches(record.Output.Branches),
		Evidence:         cloneStringSlice(record.Output.EvidenceNodes),
		Explanations:     cloneStringSlice(record.Output.ExplanationNodes),
		LogicHeading:     logicHeading,
		LogicChains:      logicChains,
		AuthorValidation: authorValidationSummaryLines(record.Output.AuthorValidation),
	}
	if subgraph == nil {
		return projection
	}
	if drivers := graphFirstNodeSection(*subgraph, func(node model.ContentNode) bool {
		return node.IsPrimary && node.GraphRole == model.GraphRoleDriver
	}); len(drivers) > 0 {
		projection.Drivers = preferGraphFirstSection(projection.Drivers, drivers)
	}
	if targets := graphFirstNodeSection(*subgraph, func(node model.ContentNode) bool {
		return node.IsPrimary && node.GraphRole == model.GraphRoleTarget
	}); len(targets) > 0 {
		projection.Targets = preferGraphFirstSection(projection.Targets, targets)
	}
	if evidence := graphFirstEvidenceSection(*subgraph); len(evidence) > 0 {
		projection.Evidence = preferGraphFirstSection(projection.Evidence, evidence)
	}
	if explanations := graphFirstExplanationSection(*subgraph); len(explanations) > 0 {
		projection.Explanations = preferGraphFirstSection(projection.Explanations, explanations)
	}
	if chains := graphFirstLogicChains(*subgraph); len(chains) > 0 {
		if len(projection.Mainline) > 0 {
			chains = filterPrimaryLogicChains(chains, record.Output.Branches)
		}
		if len(chains) > 0 {
			projection.LogicChains = preferGraphFirstLogicChains(projection.LogicChains, chains)
			if len(projection.Mainline) > 0 {
				projection.LogicHeading = "Side logic"
			} else {
				projection.LogicHeading = "Logic chain"
			}
		}
	}
	if verification := graphFirstVerificationSummary(*subgraph); len(verification) > 0 {
		projection.VerificationSummary = verification
	}
	return projection
}

func compileRecordKeyPoints(brief []model.BriefItem, units []model.SemanticUnit, limit int) []string {
	if points := briefKeyPoints(brief, limit); len(points) > 0 {
		return points
	}
	return semanticKeyPoints(units, limit)
}

func briefKeyPoints(items []model.BriefItem, limit int) []string {
	if len(items) == 0 || limit <= 0 {
		return nil
	}
	out := make([]string, 0, limit)
	for _, item := range items {
		claim := strings.TrimSpace(item.Claim)
		if claim == "" {
			continue
		}
		prefix := strings.TrimSpace(item.Category)
		if len(item.Entities) > 0 {
			prefix = strings.Join(item.Entities, ", ")
		}
		if prefix != "" {
			claim = trimCardSentence(prefix) + ": " + claim
		}
		out = append(out, trimCardSentence(truncate(claim, 160)))
		if len(out) == limit {
			break
		}
	}
	return out
}

func semanticKeyPoints(units []model.SemanticUnit, limit int) []string {
	if len(units) == 0 || limit <= 0 {
		return nil
	}
	out := make([]string, 0, limit)
	seen := map[string]struct{}{}
	categoryCounts := map[string]int{}
	for _, unit := range units {
		subject := strings.TrimSpace(unit.Subject)
		claim := strings.TrimSpace(unit.Claim)
		if subject == "" || claim == "" {
			continue
		}
		category := cardSemanticCategory(unit)
		if categoryCounts[category] >= 2 {
			continue
		}
		line := trimCardSentence(subject) + ": " + trimCardSentence(truncate(claim, 140))
		key := normalizeCardTopic(line)
		if key == "" {
			continue
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		categoryCounts[category]++
		out = append(out, line)
		if len(out) == limit {
			break
		}
	}
	return out
}

func cardSemanticCategory(unit model.SemanticUnit) string {
	text := strings.ToLower(strings.Join([]string{unit.Subject, unit.Force, unit.Claim, unit.PromptContext}, " "))
	switch {
	case strings.Contains(text, "回购") || strings.Contains(text, "buyback") || strings.Contains(text, "repurchase"):
		return "buyback"
	case strings.Contains(text, "资本配置") || strings.Contains(text, "现金") || strings.Contains(text, "美债") || strings.Contains(text, "capital allocation") || strings.Contains(text, "treasury"):
		return "capital"
	case strings.Contains(text, "持仓") || strings.Contains(text, "组合") || strings.Contains(text, "apple") || strings.Contains(text, "能力圈") || strings.Contains(text, "portfolio"):
		return "portfolio"
	case strings.Contains(text, "保险") || strings.Contains(text, "承保") || strings.Contains(text, "网络") || strings.Contains(text, "geico") || strings.Contains(text, "insurance") || strings.Contains(text, "cyber"):
		return "insurance"
	case cardContainsAIReference(text) || strings.Contains(text, "人工智能") || strings.Contains(text, "technology"):
		return "ai"
	case strings.Contains(text, "数据中心") || strings.Contains(text, "能源") || strings.Contains(text, "电力") || strings.Contains(text, "公用事业") || strings.Contains(text, "energy") || strings.Contains(text, "utility"):
		return "energy"
	case strings.Contains(text, "继任") || strings.Contains(text, "接班") || strings.Contains(text, "succession"):
		return "succession"
	case strings.Contains(text, "文化") || strings.Contains(text, "价值观") || strings.Contains(text, "culture") || strings.Contains(text, "values"):
		return "culture"
	default:
		return "other"
	}
}

func cardContainsAIReference(text string) bool {
	return strings.Contains(" "+text+" ", " ai ") ||
		strings.Contains(text, "ai应用") ||
		strings.Contains(text, "ai在") ||
		strings.Contains(text, "ai算力") ||
		strings.Contains(text, "ai数据")
}

func cloneBriefItems(values []model.BriefItem) []model.BriefItem {
	out := make([]model.BriefItem, 0, len(values))
	for _, item := range values {
		item.Entities = cloneStringSlice(item.Entities)
		item.Numbers = cloneStringSlice(item.Numbers)
		item.SourceIDs = cloneStringSlice(item.SourceIDs)
		out = append(out, item)
	}
	return out
}

func compileRecordMainline(record model.Record) []string {
	branch, ok := primaryBranch(record.Output.Branches)
	if !ok {
		return nil
	}
	lines := make([]string, 0, 4)
	for _, declaration := range branch.Declarations {
		if statement := strings.TrimSpace(declaration.Statement); statement != "" {
			lines = append(lines, "Declaration: "+statement)
		}
		if read := declarationReading(declaration); read != "" {
			lines = append(lines, "Read: "+read)
		}
	}
	if thesis := strings.TrimSpace(branch.Thesis); thesis != "" && len(branch.Declarations) == 0 {
		lines = append(lines, "Thesis: "+thesis)
	}
	for _, chain := range branchLogicChains(branch) {
		lines = append(lines, "Path: "+chain)
	}
	return lines
}

func compileRecordSecondaryLogic(record model.Record, hasMainline bool) ([]string, string) {
	chains := compileRecordLogicChains(record)
	if len(chains) == 0 {
		return nil, ""
	}
	if !hasMainline {
		return chains, "Logic chain"
	}
	out := filterPrimaryLogicChains(chains, record.Output.Branches)
	if len(out) == 0 {
		return nil, ""
	}
	return out, "Side logic"
}

func filterPrimaryLogicChains(chains []string, branches []model.Branch) []string {
	primary, ok := primaryBranch(branches)
	if !ok {
		return chains
	}
	primaryChains := map[string]struct{}{}
	for _, chain := range branchLogicChains(primary) {
		primaryChains[normalizeCardTopic(chain)] = struct{}{}
	}
	out := make([]string, 0, len(chains))
	for _, chain := range chains {
		if _, ok := primaryChains[normalizeCardTopic(chain)]; ok {
			continue
		}
		out = append(out, chain)
	}
	return out
}

func primaryBranch(branches []model.Branch) (model.Branch, bool) {
	for _, branch := range branches {
		if strings.EqualFold(strings.TrimSpace(branch.Level), "primary") {
			return branch, true
		}
	}
	return model.Branch{}, false
}

func primaryFirstBranches(branches []model.Branch) []model.Branch {
	out := cloneBranches(branches)
	primaryIndex := -1
	for i, branch := range out {
		if strings.EqualFold(strings.TrimSpace(branch.Level), "primary") {
			primaryIndex = i
			break
		}
	}
	if primaryIndex <= 0 {
		return out
	}
	primary := out[primaryIndex]
	copy(out[1:primaryIndex+1], out[:primaryIndex])
	out[0] = primary
	return out
}

func primaryFirstTopics(branches []model.Branch, topics []string) []string {
	out := cloneStringSlice(topics)
	branch, ok := primaryBranch(branches)
	if !ok {
		return out
	}
	primaryTopic := branchCardLabel(branch)
	primaryTopic = strings.TrimSpace(primaryTopic)
	if primaryTopic == "" {
		return out
	}
	filtered := make([]string, 0, len(out)+1)
	filtered = append(filtered, primaryTopic)
	seen := map[string]struct{}{normalizeCardTopic(primaryTopic): {}}
	for _, topic := range out {
		key := normalizeCardTopic(topic)
		if key == "" {
			continue
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		filtered = append(filtered, topic)
	}
	return filtered
}

func normalizeCardTopic(topic string) string {
	return strings.Join(strings.Fields(strings.TrimSpace(topic)), " ")
}

func cloneSemanticUnits(values []model.SemanticUnit) []model.SemanticUnit {
	out := make([]model.SemanticUnit, 0, len(values))
	for _, unit := range values {
		out = append(out, unit)
	}
	return out
}

func cloneBranches(values []model.Branch) []model.Branch {
	out := make([]model.Branch, 0, len(values))
	for _, branch := range values {
		branch.Anchors = cloneStringSlice(branch.Anchors)
		branch.BranchDrivers = cloneStringSlice(branch.BranchDrivers)
		branch.Drivers = cloneStringSlice(branch.Drivers)
		branch.Targets = cloneStringSlice(branch.Targets)
		branch.Declarations = cloneDeclarations(branch.Declarations)
		branch.TransmissionPaths = cloneTransmissionPaths(branch.TransmissionPaths)
		out = append(out, branch)
	}
	return out
}

func cloneDeclarations(values []model.Declaration) []model.Declaration {
	out := make([]model.Declaration, 0, len(values))
	for _, declaration := range values {
		declaration.Conditions = cloneStringSlice(declaration.Conditions)
		declaration.Actions = cloneStringSlice(declaration.Actions)
		declaration.Constraints = cloneStringSlice(declaration.Constraints)
		declaration.NonActions = cloneStringSlice(declaration.NonActions)
		declaration.Evidence = cloneStringSlice(declaration.Evidence)
		out = append(out, declaration)
	}
	return out
}

func cloneTransmissionPaths(values []model.TransmissionPath) []model.TransmissionPath {
	out := make([]model.TransmissionPath, 0, len(values))
	for _, path := range values {
		path.Steps = cloneStringSlice(path.Steps)
		out = append(out, path)
	}
	return out
}

func compileRecordLogicChains(record model.Record) []string {
	if len(record.Output.TransmissionPaths) == 0 {
		return nil
	}
	chains := make([]string, 0, len(record.Output.TransmissionPaths))
	for _, path := range record.Output.TransmissionPaths {
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
