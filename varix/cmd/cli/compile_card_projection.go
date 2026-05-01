package main

import (
	c "github.com/kumaloha/VariX/varix/compile"
	"github.com/kumaloha/VariX/varix/graphmodel"
	"strings"
)

type compileCardProjection struct {
	Summary             string
	Topics              []string
	Confidence          string
	Drivers             []string
	Targets             []string
	Branches            []c.Branch
	Evidence            []string
	Explanations        []string
	LogicChains         []string
	VerificationSummary []string
	AuthorValidation    []string
}

func buildCompileCardProjection(record c.Record, subgraph *graphmodel.ContentSubgraph) compileCardProjection {
	projection := compileCardProjection{
		Summary:          record.Output.Summary,
		Topics:           cloneStringSlice(record.Output.Topics),
		Confidence:       record.Output.Confidence,
		Drivers:          cloneStringSlice(record.Output.Drivers),
		Targets:          cloneStringSlice(record.Output.Targets),
		Branches:         cloneBranches(record.Output.Branches),
		Evidence:         cloneStringSlice(record.Output.EvidenceNodes),
		Explanations:     cloneStringSlice(record.Output.ExplanationNodes),
		LogicChains:      legacyLogicChains(record),
		AuthorValidation: authorValidationSummaryLines(record.Output.AuthorValidation),
	}
	if subgraph == nil {
		return projection
	}
	if drivers := graphFirstNodeSection(*subgraph, func(node graphmodel.GraphNode) bool {
		return node.IsPrimary && node.GraphRole == graphmodel.GraphRoleDriver
	}); len(drivers) > 0 {
		projection.Drivers = preferGraphFirstSection(projection.Drivers, drivers)
	}
	if targets := graphFirstNodeSection(*subgraph, func(node graphmodel.GraphNode) bool {
		return node.IsPrimary && node.GraphRole == graphmodel.GraphRoleTarget
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

func cloneBranches(values []c.Branch) []c.Branch {
	out := make([]c.Branch, 0, len(values))
	for _, branch := range values {
		branch.Anchors = cloneStringSlice(branch.Anchors)
		branch.BranchDrivers = cloneStringSlice(branch.BranchDrivers)
		branch.Drivers = cloneStringSlice(branch.Drivers)
		branch.Targets = cloneStringSlice(branch.Targets)
		branch.TransmissionPaths = cloneTransmissionPaths(branch.TransmissionPaths)
		out = append(out, branch)
	}
	return out
}

func cloneTransmissionPaths(values []c.TransmissionPath) []c.TransmissionPath {
	out := make([]c.TransmissionPath, 0, len(values))
	for _, path := range values {
		path.Steps = cloneStringSlice(path.Steps)
		out = append(out, path)
	}
	return out
}

func legacyLogicChains(record c.Record) []string {
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
