package model

import "time"

type AuthorClaimStatus string

const (
	AuthorClaimSupported      AuthorClaimStatus = "supported"
	AuthorClaimContradicted   AuthorClaimStatus = "contradicted"
	AuthorClaimUnverified     AuthorClaimStatus = "unverified"
	AuthorClaimInterpretive   AuthorClaimStatus = "interpretive"
	AuthorClaimNotAuthorClaim AuthorClaimStatus = "not_author_claim"
)

type AuthorClaimCheck struct {
	ClaimID          string                      `json:"claim_id"`
	Text             string                      `json:"text"`
	ClaimType        string                      `json:"claim_type,omitempty"`
	Status           AuthorClaimStatus           `json:"status"`
	DecisionNote     string                      `json:"decision_note,omitempty"`
	RequiredEvidence []AuthorEvidenceRequirement `json:"required_evidence,omitempty"`
	Evidence         []string                    `json:"evidence,omitempty"`
	Reason           string                      `json:"reason,omitempty"`
	Subclaims        []AuthorSubclaim            `json:"subclaims,omitempty"`
}

type AuthorEvidenceRequirement struct {
	Description      string            `json:"description"`
	Subject          string            `json:"subject,omitempty"`
	Metric           string            `json:"metric,omitempty"`
	OriginalValue    string            `json:"original_value,omitempty"`
	Unit             string            `json:"unit,omitempty"`
	TimeWindow       string            `json:"time_window,omitempty"`
	SourceType       string            `json:"source_type,omitempty"`
	Series           string            `json:"series,omitempty"`
	Entity           string            `json:"entity,omitempty"`
	Geography        string            `json:"geography,omitempty"`
	Denominator      string            `json:"denominator,omitempty"`
	PreferredSources []string          `json:"preferred_sources,omitempty"`
	Queries          []string          `json:"queries,omitempty"`
	ComparisonRule   string            `json:"comparison_rule,omitempty"`
	ScopeCaveat      string            `json:"scope_caveat,omitempty"`
	Status           AuthorClaimStatus `json:"status"`
	Evidence         []string          `json:"evidence,omitempty"`
	Reason           string            `json:"reason,omitempty"`
}

type AuthorSubclaim struct {
	SubclaimID      string            `json:"subclaim_id,omitempty"`
	ParentClaimID   string            `json:"parent_claim_id,omitempty"`
	Text            string            `json:"text"`
	Subject         string            `json:"subject,omitempty"`
	Metric          string            `json:"metric,omitempty"`
	OriginalValue   string            `json:"original_value,omitempty"`
	NormalizedValue string            `json:"normalized_value,omitempty"`
	EvidenceValue   string            `json:"evidence_value,omitempty"`
	EvidenceRange   string            `json:"evidence_range,omitempty"`
	ComparisonBase  string            `json:"comparison_base,omitempty"`
	EvidenceBase    string            `json:"evidence_base,omitempty"`
	ScopeStatus     string            `json:"scope_status,omitempty"`
	UnitNormalized  bool              `json:"unit_normalized,omitempty"`
	RangeCovered    bool              `json:"range_covered,omitempty"`
	AttributionOK   bool              `json:"attribution_ok,omitempty"`
	Status          AuthorClaimStatus `json:"status"`
	DecisionNote    string            `json:"decision_note,omitempty"`
	Evidence        []string          `json:"evidence,omitempty"`
	Reason          string            `json:"reason,omitempty"`
}

type AuthorInferenceStatus string

const (
	AuthorInferenceSound              AuthorInferenceStatus = "sound"
	AuthorInferenceWeak               AuthorInferenceStatus = "weak"
	AuthorInferenceUnsupportedJump    AuthorInferenceStatus = "unsupported_jump"
	AuthorInferenceNotAuthorInference AuthorInferenceStatus = "not_author_inference"
)

type AuthorInferenceCheck struct {
	InferenceID      string                      `json:"inference_id"`
	From             string                      `json:"from"`
	To               string                      `json:"to"`
	Steps            []string                    `json:"steps,omitempty"`
	Status           AuthorInferenceStatus       `json:"status"`
	DecisionNote     string                      `json:"decision_note,omitempty"`
	RequiredEvidence []AuthorEvidenceRequirement `json:"required_evidence,omitempty"`
	Evidence         []string                    `json:"evidence,omitempty"`
	Reason           string                      `json:"reason,omitempty"`
	MissingLinks     []string                    `json:"missing_links,omitempty"`
}

type AuthorValidationSummary struct {
	Verdict               string `json:"verdict,omitempty"`
	SupportedClaims       int    `json:"supported_claims,omitempty"`
	ContradictedClaims    int    `json:"contradicted_claims,omitempty"`
	UnverifiedClaims      int    `json:"unverified_claims,omitempty"`
	InterpretiveClaims    int    `json:"interpretive_claims,omitempty"`
	NotAuthorClaims       int    `json:"not_author_claims,omitempty"`
	SoundInferences       int    `json:"sound_inferences,omitempty"`
	WeakInferences        int    `json:"weak_inferences,omitempty"`
	UnsupportedInferences int    `json:"unsupported_inferences,omitempty"`
	NotAuthorInferences   int    `json:"not_author_inferences,omitempty"`
}

type AuthorVerificationPlan struct {
	ClaimPlans     []AuthorClaimVerificationPlan     `json:"claim_plans,omitempty"`
	InferencePlans []AuthorInferenceVerificationPlan `json:"inference_plans,omitempty"`
}

type AuthorClaimVerificationPlan struct {
	ClaimID          string                      `json:"claim_id"`
	Text             string                      `json:"text,omitempty"`
	ClaimKind        string                      `json:"claim_kind,omitempty"`
	NeedsValidation  bool                        `json:"needs_validation"`
	AtomicClaims     []AuthorAtomicEvidenceSpec  `json:"atomic_claims,omitempty"`
	RequiredEvidence []AuthorEvidenceRequirement `json:"required_evidence,omitempty"`
	PreferredSources []string                    `json:"preferred_sources,omitempty"`
	Queries          []string                    `json:"queries,omitempty"`
	ScopeCaveat      string                      `json:"scope_caveat,omitempty"`
}

type AuthorAtomicEvidenceSpec struct {
	Text             string   `json:"text,omitempty"`
	Subject          string   `json:"subject,omitempty"`
	Metric           string   `json:"metric,omitempty"`
	OriginalValue    string   `json:"original_value,omitempty"`
	Unit             string   `json:"unit,omitempty"`
	TimeWindow       string   `json:"time_window,omitempty"`
	SourceType       string   `json:"source_type,omitempty"`
	Series           string   `json:"series,omitempty"`
	Entity           string   `json:"entity,omitempty"`
	Geography        string   `json:"geography,omitempty"`
	Denominator      string   `json:"denominator,omitempty"`
	PreferredSources []string `json:"preferred_sources,omitempty"`
	Queries          []string `json:"queries,omitempty"`
	ComparisonRule   string   `json:"comparison_rule,omitempty"`
	ScopeCaveat      string   `json:"scope_caveat,omitempty"`
}

type AuthorInferenceVerificationPlan struct {
	InferenceID      string                      `json:"inference_id"`
	From             string                      `json:"from,omitempty"`
	To               string                      `json:"to,omitempty"`
	Steps            []string                    `json:"steps,omitempty"`
	RequiredEvidence []AuthorEvidenceRequirement `json:"required_evidence,omitempty"`
	Queries          []string                    `json:"queries,omitempty"`
	MissingPremises  []string                    `json:"missing_premises,omitempty"`
}

type AuthorValidation struct {
	ValidatedAt      time.Time               `json:"validated_at,omitempty"`
	Model            string                  `json:"model,omitempty"`
	Version          string                  `json:"version,omitempty"`
	Summary          AuthorValidationSummary `json:"summary,omitempty"`
	VerificationPlan AuthorVerificationPlan  `json:"verification_plan,omitempty"`
	ClaimChecks      []AuthorClaimCheck      `json:"claim_checks,omitempty"`
	InferenceChecks  []AuthorInferenceCheck  `json:"inference_checks,omitempty"`
}

func (v AuthorValidation) IsZero() bool {
	return v.ValidatedAt.IsZero() &&
		v.Model == "" &&
		v.Version == "" &&
		v.Summary == (AuthorValidationSummary{}) &&
		len(v.VerificationPlan.ClaimPlans) == 0 &&
		len(v.VerificationPlan.InferencePlans) == 0 &&
		len(v.ClaimChecks) == 0 &&
		len(v.InferenceChecks) == 0
}
