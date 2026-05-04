package model

type NodeExtractionOutput struct {
	Graph      ReasoningGraph `json:"graph,omitempty"`
	Details    HiddenDetails  `json:"details,omitempty"`
	Topics     []string       `json:"topics,omitempty"`
	Confidence string         `json:"confidence,omitempty"`
}

type DriverTargetOutput struct {
	Drivers    []string      `json:"drivers,omitempty"`
	Targets    []string      `json:"targets,omitempty"`
	Details    HiddenDetails `json:"details,omitempty"`
	Topics     []string      `json:"topics,omitempty"`
	Confidence string        `json:"confidence,omitempty"`
}

type FullGraphOutput struct {
	Graph      ReasoningGraph `json:"graph,omitempty"`
	Details    HiddenDetails  `json:"details,omitempty"`
	Topics     []string       `json:"topics,omitempty"`
	Confidence string         `json:"confidence,omitempty"`
}

type TransmissionPathOutput struct {
	TransmissionPaths []TransmissionPath `json:"transmission_paths,omitempty"`
	Details           HiddenDetails      `json:"details,omitempty"`
	Topics            []string           `json:"topics,omitempty"`
	Confidence        string             `json:"confidence,omitempty"`
}

type EvidenceExplanationOutput struct {
	EvidenceNodes      []string      `json:"evidence_nodes,omitempty"`
	ExplanationNodes   []string      `json:"explanation_nodes,omitempty"`
	SupplementaryNodes []string      `json:"supplementary_nodes,omitempty"`
	Details            HiddenDetails `json:"details,omitempty"`
	Topics             []string      `json:"topics,omitempty"`
	Confidence         string        `json:"confidence,omitempty"`
}

type UnifiedCompileOutput struct {
	Summary            string             `json:"summary,omitempty"`
	Drivers            []string           `json:"drivers,omitempty"`
	Targets            []string           `json:"targets,omitempty"`
	Declarations       []Declaration      `json:"declarations,omitempty"`
	SemanticUnits      []SemanticUnit     `json:"semantic_units,omitempty"`
	Ledger             Ledger             `json:"ledger,omitempty"`
	Brief              []BriefItem        `json:"brief,omitempty"`
	TransmissionPaths  []TransmissionPath `json:"transmission_paths,omitempty"`
	EvidenceNodes      []string           `json:"evidence_nodes,omitempty"`
	ExplanationNodes   []string           `json:"explanation_nodes,omitempty"`
	SupplementaryNodes []string           `json:"supplementary_nodes,omitempty"`
	Details            HiddenDetails      `json:"details,omitempty"`
	Topics             []string           `json:"topics,omitempty"`
	Confidence         string             `json:"confidence,omitempty"`
}

type ThesisOutput struct {
	Summary    string        `json:"summary,omitempty"`
	Drivers    []string      `json:"drivers,omitempty"`
	Targets    []string      `json:"targets,omitempty"`
	Details    HiddenDetails `json:"details,omitempty"`
	Topics     []string      `json:"topics,omitempty"`
	Confidence string        `json:"confidence,omitempty"`
}

type stringListField struct {
	name   string
	values []string
}
