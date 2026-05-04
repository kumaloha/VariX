package model

import (
	"encoding/json"
	"fmt"
)

func ParseOutput(raw string) (Output, error) {
	payload, err := parseCompilePayload(raw)
	if err != nil {
		return Output{}, err
	}
	var out Output
	if err := json.Unmarshal(payload["summary"], &out.Summary); err != nil {
		return Output{}, fmt.Errorf("parse compile summary: %w", err)
	}
	_ = json.Unmarshal(payload["drivers"], &out.Drivers)
	_ = json.Unmarshal(payload["targets"], &out.Targets)
	_ = json.Unmarshal(payload["declarations"], &out.Declarations)
	_ = json.Unmarshal(payload["semantic_units"], &out.SemanticUnits)
	_ = json.Unmarshal(payload["ledger"], &out.Ledger)
	_ = json.Unmarshal(payload["brief"], &out.Brief)
	_ = json.Unmarshal(payload["coverage_audit"], &out.CoverageAudit)
	_ = json.Unmarshal(payload["transmission_paths"], &out.TransmissionPaths)
	_ = json.Unmarshal(payload["evidence_nodes"], &out.EvidenceNodes)
	_ = json.Unmarshal(payload["explanation_nodes"], &out.ExplanationNodes)
	_ = json.Unmarshal(payload["supplementary_nodes"], &out.SupplementaryNodes)
	out.Drivers = splitParallelDrivers(normalizeStringList(out.Drivers))
	out.Targets = normalizeStringList(out.Targets)
	normalizeDeclarations(out.Declarations)
	normalizeSemanticUnits(out.SemanticUnits)
	out.EvidenceNodes = normalizeStringList(out.EvidenceNodes)
	out.ExplanationNodes = normalizeStringList(out.ExplanationNodes)
	out.SupplementaryNodes = normalizeStringList(out.SupplementaryNodes)
	normalizeTransmissionPaths(out.TransmissionPaths)
	_ = json.Unmarshal(payload["topics"], &out.Topics)
	_ = json.Unmarshal(payload["confidence"], &out.Confidence)
	_ = json.Unmarshal(payload["graph"], &out.Graph)
	normalizeNodeTaxonomy(&out.Graph)
	normalizeNodeTiming(&out.Graph)
	_ = json.Unmarshal(payload["verification"], &out.Verification)
	if rawDetails, ok := payload["details"]; ok {
		details, err := parseHiddenDetails(rawDetails)
		if err != nil {
			return Output{}, err
		}
		out.Details = details
	}
	if err := out.Validate(); err != nil {
		return Output{}, err
	}
	return out, nil
}
func ParseNodeExtractionOutput(raw string) (NodeExtractionOutput, error) {
	payload, err := parseCompilePayload(raw)
	if err != nil {
		return NodeExtractionOutput{}, err
	}
	var out NodeExtractionOutput
	_ = json.Unmarshal(payload["topics"], &out.Topics)
	_ = json.Unmarshal(payload["confidence"], &out.Confidence)
	_ = json.Unmarshal(payload["graph"], &out.Graph)
	normalizeNodeTaxonomy(&out.Graph)
	normalizeNodeTiming(&out.Graph)
	if rawDetails, ok := payload["details"]; ok {
		details, err := parseHiddenDetails(rawDetails)
		if err != nil {
			return NodeExtractionOutput{}, err
		}
		out.Details = details
	}
	return out, nil
}
func ParseFullGraphOutput(raw string, nodeIDs map[string]struct{}, nodeKinds map[string]NodeKind) (FullGraphOutput, error) {
	payload, err := parseCompilePayload(raw)
	if err != nil {
		return FullGraphOutput{}, err
	}
	var out FullGraphOutput
	_ = json.Unmarshal(payload["topics"], &out.Topics)
	_ = json.Unmarshal(payload["confidence"], &out.Confidence)
	_ = json.Unmarshal(payload["graph"], &out.Graph)
	if rawDetails, ok := payload["details"]; ok {
		details, err := parseHiddenDetails(rawDetails)
		if err != nil {
			return FullGraphOutput{}, err
		}
		out.Details = details
	}
	return out, nil
}
func ParseUnifiedCompileOutput(raw string) (UnifiedCompileOutput, error) {
	payload, err := parseCompilePayload(raw)
	if err != nil {
		return UnifiedCompileOutput{}, err
	}
	var out UnifiedCompileOutput
	_ = json.Unmarshal(payload["summary"], &out.Summary)
	_ = json.Unmarshal(payload["drivers"], &out.Drivers)
	_ = json.Unmarshal(payload["targets"], &out.Targets)
	_ = json.Unmarshal(payload["declarations"], &out.Declarations)
	_ = json.Unmarshal(payload["semantic_units"], &out.SemanticUnits)
	_ = json.Unmarshal(payload["ledger"], &out.Ledger)
	_ = json.Unmarshal(payload["brief"], &out.Brief)
	_ = json.Unmarshal(payload["coverage_audit"], &out.CoverageAudit)
	_ = json.Unmarshal(payload["transmission_paths"], &out.TransmissionPaths)
	_ = json.Unmarshal(payload["evidence_nodes"], &out.EvidenceNodes)
	_ = json.Unmarshal(payload["explanation_nodes"], &out.ExplanationNodes)
	_ = json.Unmarshal(payload["supplementary_nodes"], &out.SupplementaryNodes)
	out.Drivers = splitParallelDrivers(normalizeStringList(out.Drivers))
	out.Targets = normalizeStringList(out.Targets)
	normalizeDeclarations(out.Declarations)
	normalizeSemanticUnits(out.SemanticUnits)
	out.EvidenceNodes = normalizeStringList(out.EvidenceNodes)
	out.ExplanationNodes = normalizeStringList(out.ExplanationNodes)
	out.SupplementaryNodes = normalizeStringList(out.SupplementaryNodes)
	normalizeTransmissionPaths(out.TransmissionPaths)
	_ = json.Unmarshal(payload["topics"], &out.Topics)
	_ = json.Unmarshal(payload["confidence"], &out.Confidence)
	if rawDetails, ok := payload["details"]; ok {
		details, err := parseHiddenDetails(rawDetails)
		if err != nil {
			return UnifiedCompileOutput{}, err
		}
		out.Details = details
	}
	return out, nil
}
func ParseThesisOutput(raw string) (ThesisOutput, error) {
	payload, err := parseCompilePayload(raw)
	if err != nil {
		return ThesisOutput{}, err
	}
	var out ThesisOutput
	if err := json.Unmarshal(payload["summary"], &out.Summary); err != nil {
		return ThesisOutput{}, fmt.Errorf("parse compile summary: %w", err)
	}
	_ = json.Unmarshal(payload["drivers"], &out.Drivers)
	_ = json.Unmarshal(payload["targets"], &out.Targets)
	out.Drivers = normalizeStringList(out.Drivers)
	out.Targets = normalizeStringList(out.Targets)
	_ = json.Unmarshal(payload["topics"], &out.Topics)
	_ = json.Unmarshal(payload["confidence"], &out.Confidence)
	if rawDetails, ok := payload["details"]; ok {
		details, err := parseHiddenDetails(rawDetails)
		if err != nil {
			return ThesisOutput{}, err
		}
		out.Details = details
	}
	if err := out.Validate(); err != nil {
		return ThesisOutput{}, err
	}
	return out, nil
}
