package compile

import (
	"encoding/json"
	"time"
)

type HiddenDetails struct {
	QuoteHighlights     []string         `json:"quote_highlights,omitempty"`
	ReferenceHighlights []string         `json:"reference_highlights,omitempty"`
	AttachmentNotes     []string         `json:"attachment_notes,omitempty"`
	Caveats             []string         `json:"caveats,omitempty"`
	Items               []map[string]any `json:"items,omitempty"`
}

func (d HiddenDetails) IsEmpty() bool {
	return len(d.QuoteHighlights) == 0 &&
		len(d.ReferenceHighlights) == 0 &&
		len(d.AttachmentNotes) == 0 &&
		len(d.Caveats) == 0 &&
		len(d.Items) == 0
}

type Output struct {
	Summary            string             `json:"summary,omitempty"`
	Drivers            []string           `json:"drivers,omitempty"`
	Targets            []string           `json:"targets,omitempty"`
	TransmissionPaths  []TransmissionPath `json:"transmission_paths,omitempty"`
	Branches           []Branch           `json:"branches,omitempty"`
	EvidenceNodes      []string           `json:"evidence_nodes,omitempty"`
	ExplanationNodes   []string           `json:"explanation_nodes,omitempty"`
	SupplementaryNodes []string           `json:"supplementary_nodes,omitempty"`
	Graph              ReasoningGraph     `json:"graph,omitempty"`
	Details            HiddenDetails      `json:"details,omitempty"`
	Topics             []string           `json:"topics,omitempty"`
	Confidence         string             `json:"confidence,omitempty"`
	Verification       Verification       `json:"verification,omitempty"`
	AuthorValidation   AuthorValidation   `json:"author_validation,omitempty"`
}

func (o Output) MarshalJSON() ([]byte, error) {
	type publicOutput struct {
		Summary            string             `json:"summary,omitempty"`
		Drivers            []string           `json:"drivers,omitempty"`
		Targets            []string           `json:"targets,omitempty"`
		TransmissionPaths  []TransmissionPath `json:"transmission_paths,omitempty"`
		Branches           []Branch           `json:"branches,omitempty"`
		EvidenceNodes      []string           `json:"evidence_nodes,omitempty"`
		ExplanationNodes   []string           `json:"explanation_nodes,omitempty"`
		SupplementaryNodes []string           `json:"supplementary_nodes,omitempty"`
		Details            HiddenDetails      `json:"details,omitempty"`
		Topics             []string           `json:"topics,omitempty"`
		Confidence         string             `json:"confidence,omitempty"`
		Verification       *Verification      `json:"verification,omitempty"`
		AuthorValidation   *AuthorValidation  `json:"author_validation,omitempty"`
	}
	var verification *Verification
	if !o.Verification.IsZero() {
		verification = &o.Verification
	}
	var authorValidation *AuthorValidation
	if !o.AuthorValidation.IsZero() {
		authorValidation = &o.AuthorValidation
	}
	return json.Marshal(publicOutput{
		Summary:            o.Summary,
		Drivers:            o.Drivers,
		Targets:            o.Targets,
		TransmissionPaths:  o.TransmissionPaths,
		Branches:           o.Branches,
		EvidenceNodes:      o.EvidenceNodes,
		ExplanationNodes:   o.ExplanationNodes,
		SupplementaryNodes: o.SupplementaryNodes,
		Details:            o.Details,
		Topics:             o.Topics,
		Confidence:         o.Confidence,
		Verification:       verification,
		AuthorValidation:   authorValidation,
	})
}

type Record struct {
	UnitID         string        `json:"unit_id"`
	Source         string        `json:"source"`
	ExternalID     string        `json:"external_id"`
	RootExternalID string        `json:"root_external_id,omitempty"`
	Model          string        `json:"model"`
	Metrics        RecordMetrics `json:"metrics,omitempty"`
	Output         Output        `json:"output"`
	CompiledAt     time.Time     `json:"compiled_at"`
}

type RecordMetrics struct {
	CompileElapsedMS      int64            `json:"compile_elapsed_ms,omitempty"`
	CompileStageElapsedMS map[string]int64 `json:"compile_stage_elapsed_ms,omitempty"`
}
