package main

import (
	"github.com/kumaloha/VariX/varix/model"
	"strings"
)

type compileCardProjection struct {
	Summary             string
	Topics              []string
	Confidence          string
	Drivers             []string
	Targets             []string
	Declarations        []model.Declaration
	SemanticUnits       []model.SemanticUnit
	Branches            []model.Branch
	Evidence            []string
	Explanations        []string
	LogicChains         []string
	VerificationSummary []string
	AuthorValidation    []string
}

func buildCompileCardProjection(record model.Record, subgraph *model.ContentSubgraph) compileCardProjection {
	projection := compileCardProjection{
		Summary:          record.Output.Summary,
		Topics:           cloneStringSlice(record.Output.Topics),
		Confidence:       record.Output.Confidence,
		Drivers:          cloneStringSlice(record.Output.Drivers),
		Targets:          cloneStringSlice(record.Output.Targets),
		Declarations:     cloneDeclarations(record.Output.Declarations),
		SemanticUnits:    cloneSemanticUnits(record.Output.SemanticUnits),
		Branches:         cloneBranches(record.Output.Branches),
		Evidence:         cloneStringSlice(record.Output.EvidenceNodes),
		Explanations:     cloneStringSlice(record.Output.ExplanationNodes),
		LogicChains:      compileRecordLogicChains(record),
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
		projection.LogicChains = preferGraphFirstLogicChains(projection.LogicChains, chains)
	}
	if verification := graphFirstVerificationSummary(*subgraph); len(verification) > 0 {
		projection.VerificationSummary = verification
	}
	return projection
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
