package model

import "time"

type FactStatus string

const (
	FactStatusClearlyTrue  FactStatus = "clearly_true"
	FactStatusClearlyFalse FactStatus = "clearly_false"
	FactStatusUnverifiable FactStatus = "unverifiable"
)

type ExplicitConditionStatus string

const (
	ExplicitConditionStatusHigh    ExplicitConditionStatus = "high"
	ExplicitConditionStatusMedium  ExplicitConditionStatus = "medium"
	ExplicitConditionStatusLow     ExplicitConditionStatus = "low"
	ExplicitConditionStatusUnknown ExplicitConditionStatus = "unknown"
)

type PredictionStatus string

const (
	PredictionStatusUnresolved      PredictionStatus = "unresolved"
	PredictionStatusResolvedTrue    PredictionStatus = "resolved_true"
	PredictionStatusResolvedFalse   PredictionStatus = "resolved_false"
	PredictionStatusStaleUnresolved PredictionStatus = "stale_unresolved"
)

type FactCheck struct {
	NodeID string     `json:"node_id"`
	Status FactStatus `json:"status"`
	Reason string     `json:"reason,omitempty"`
}

type ExplicitConditionCheck struct {
	NodeID string                  `json:"node_id"`
	Status ExplicitConditionStatus `json:"status"`
	Reason string                  `json:"reason,omitempty"`
}

type ImplicitConditionCheck struct {
	NodeID string     `json:"node_id"`
	Status FactStatus `json:"status"`
	Reason string     `json:"reason,omitempty"`
}

type PredictionCheck struct {
	NodeID string           `json:"node_id"`
	Status PredictionStatus `json:"status"`
	Reason string           `json:"reason,omitempty"`
	AsOf   time.Time        `json:"as_of,omitempty"`
}

type RealizedCheck struct {
	NodeID string     `json:"node_id"`
	Status FactStatus `json:"status"`
	Reason string     `json:"reason,omitempty"`
}

type FutureConditionCheck struct {
	NodeID string    `json:"node_id"`
	Status string    `json:"status,omitempty"`
	Reason string    `json:"reason,omitempty"`
	AsOf   time.Time `json:"as_of,omitempty"`
}

type NodeVerificationStatus string

const (
	NodeVerificationProved    NodeVerificationStatus = "proved"
	NodeVerificationFalsified NodeVerificationStatus = "falsified"
	NodeVerificationWaiting   NodeVerificationStatus = "waiting"
)

type NodeVerification struct {
	NodeID   string                 `json:"node_id"`
	Status   NodeVerificationStatus `json:"status"`
	Evidence []string               `json:"evidence,omitempty"`
	Reason   string                 `json:"reason,omitempty"`
	AsOf     time.Time              `json:"as_of,omitempty"`
	NodeText string                 `json:"node_text,omitempty"`
	NodeKind string                 `json:"node_kind,omitempty"`
}

type PathVerificationStatus string

const (
	PathVerificationSound   PathVerificationStatus = "sound"
	PathVerificationProblem PathVerificationStatus = "problem"
)

type PathVerification struct {
	Driver       string                 `json:"driver"`
	Target       string                 `json:"target"`
	Steps        []string               `json:"steps,omitempty"`
	Status       PathVerificationStatus `json:"status"`
	Complete     bool                   `json:"complete"`
	Rigorous     bool                   `json:"rigorous"`
	Reason       string                 `json:"reason,omitempty"`
	MissingLinks []string               `json:"missing_links,omitempty"`
}

type DeclarationVerificationStatus string

const (
	DeclarationVerificationProved          DeclarationVerificationStatus = "proved"
	DeclarationVerificationOverclaimed     DeclarationVerificationStatus = "overclaimed"
	DeclarationVerificationInferredOnly    DeclarationVerificationStatus = "inferred_only"
	DeclarationVerificationSpeakerMismatch DeclarationVerificationStatus = "speaker_mismatch"
	DeclarationVerificationConditionLost   DeclarationVerificationStatus = "condition_lost"
	DeclarationVerificationScopeMismatch   DeclarationVerificationStatus = "scope_mismatch"
)

type DeclarationVerification struct {
	DeclarationID     string                        `json:"declaration_id,omitempty"`
	Statement         string                        `json:"statement,omitempty"`
	Speaker           string                        `json:"speaker,omitempty"`
	Status            DeclarationVerificationStatus `json:"status"`
	Reason            string                        `json:"reason,omitempty"`
	Evidence          []string                      `json:"evidence,omitempty"`
	MissingConditions []string                      `json:"missing_conditions,omitempty"`
}

type VerificationPassKind string

const (
	VerificationPassFact              VerificationPassKind = "fact"
	VerificationPassExplicitCondition VerificationPassKind = "explicit_condition"
	VerificationPassImplicitCondition VerificationPassKind = "implicit_condition"
	VerificationPassPrediction        VerificationPassKind = "prediction"
)

type VerificationStageSummary struct {
	Model         string    `json:"model,omitempty"`
	CompletedAt   time.Time `json:"completed_at,omitempty"`
	ParseOK       bool      `json:"parse_ok"`
	OutputNodeIDs []string  `json:"output_node_ids,omitempty"`
}

type VerificationPassCoverage struct {
	ExpectedNodeIDs   []string `json:"expected_node_ids,omitempty"`
	ReturnedNodeIDs   []string `json:"returned_node_ids,omitempty"`
	MissingNodeIDs    []string `json:"missing_node_ids,omitempty"`
	DuplicateNodeIDs  []string `json:"duplicate_node_ids,omitempty"`
	UnexpectedNodeIDs []string `json:"unexpected_node_ids,omitempty"`
	Valid             bool     `json:"valid"`
}

type VerificationRetrievalSummary struct {
	RetrievedNodeIDs     []string `json:"retrieved_node_ids,omitempty"`
	NoResultNodeIDs      []string `json:"no_result_node_ids,omitempty"`
	BudgetLimitedNodeIDs []string `json:"budget_limited_node_ids,omitempty"`
	PromptContextReduced bool     `json:"prompt_context_reduced"`
	ExcerptTruncated     bool     `json:"excerpt_truncated"`
}

type VerificationPass struct {
	Kind             VerificationPassKind          `json:"kind,omitempty"`
	NodeIDs          []string                      `json:"node_ids,omitempty"`
	Coverage         VerificationPassCoverage      `json:"coverage"`
	RetrievalSummary *VerificationRetrievalSummary `json:"retrieval_summary,omitempty"`
	Claim            *VerificationStageSummary     `json:"claim,omitempty"`
	Challenge        *VerificationStageSummary     `json:"challenge,omitempty"`
	Adjudication     *VerificationStageSummary     `json:"adjudication,omitempty"`
}

type VerificationCoverageSummary struct {
	TotalExpectedNodes  int      `json:"total_expected_nodes"`
	TotalFinalizedNodes int      `json:"total_finalized_nodes"`
	MissingNodeIDs      []string `json:"missing_node_ids,omitempty"`
	DuplicateNodeIDs    []string `json:"duplicate_node_ids,omitempty"`
	UnexpectedNodeIDs   []string `json:"unexpected_node_ids,omitempty"`
	Valid               bool     `json:"valid"`
}

type Verification struct {
	VerifiedAt               time.Time                    `json:"verified_at,omitempty"`
	Model                    string                       `json:"model,omitempty"`
	Version                  string                       `json:"version,omitempty"`
	RolloutStage             string                       `json:"rollout_stage,omitempty"`
	NodeVerifications        []NodeVerification           `json:"node_verifications,omitempty"`
	PathVerifications        []PathVerification           `json:"path_verifications,omitempty"`
	DeclarationVerifications []DeclarationVerification    `json:"declaration_verifications,omitempty"`
	RealizedChecks           []RealizedCheck              `json:"realized_checks,omitempty"`
	FutureConditionChecks    []FutureConditionCheck       `json:"future_condition_checks,omitempty"`
	FactChecks               []FactCheck                  `json:"fact_checks,omitempty"`
	ExplicitConditionChecks  []ExplicitConditionCheck     `json:"explicit_condition_checks,omitempty"`
	ImplicitConditionChecks  []ImplicitConditionCheck     `json:"implicit_condition_checks,omitempty"`
	PredictionChecks         []PredictionCheck            `json:"prediction_checks,omitempty"`
	Passes                   []VerificationPass           `json:"passes,omitempty"`
	CoverageSummary          *VerificationCoverageSummary `json:"coverage_summary,omitempty"`
}

func (v Verification) IsZero() bool {
	return v.VerifiedAt.IsZero() &&
		v.Model == "" &&
		v.Version == "" &&
		v.RolloutStage == "" &&
		len(v.NodeVerifications) == 0 &&
		len(v.PathVerifications) == 0 &&
		len(v.DeclarationVerifications) == 0 &&
		len(v.RealizedChecks) == 0 &&
		len(v.FutureConditionChecks) == 0 &&
		len(v.FactChecks) == 0 &&
		len(v.ExplicitConditionChecks) == 0 &&
		len(v.ImplicitConditionChecks) == 0 &&
		len(v.PredictionChecks) == 0 &&
		len(v.Passes) == 0 &&
		v.CoverageSummary == nil
}
