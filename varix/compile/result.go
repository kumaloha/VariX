package compile

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

var openEndedNodeTime = time.Date(9999, 12, 31, 23, 59, 59, 0, time.UTC)

type NodeKind string

const (
	NodeFact              NodeKind = "事实"
	NodeExplicitCondition NodeKind = "显式条件"
	NodeImplicitCondition NodeKind = "隐含条件"
	NodeMechanism         NodeKind = "机制"
	NodeAssumption        NodeKind = NodeImplicitCondition // backward-compatible alias
	NodeConclusion        NodeKind = "结论"
	NodePrediction        NodeKind = "预测"
)

type NodeForm string

const (
	NodeFormObservation NodeForm = "observation"
	NodeFormCondition   NodeForm = "condition"
	NodeFormJudgment    NodeForm = "judgment"
	NodeFormForecast    NodeForm = "forecast"
)

type NodeFunction string

const (
	NodeFunctionSupport      NodeFunction = "support"
	NodeFunctionTransmission NodeFunction = "transmission"
	NodeFunctionClaim        NodeFunction = "claim"
)

type EdgeKind string

const (
	EdgePositive EdgeKind = "drives"
	EdgeDerives  EdgeKind = "substantiates"
	EdgePresets  EdgeKind = "gates"
	EdgeExplains EdgeKind = "explains"
)

type GraphNode struct {
	ID                string       `json:"id"`
	Kind              NodeKind     `json:"kind"`
	Form              NodeForm     `json:"form,omitempty"`
	Function          NodeFunction `json:"function,omitempty"`
	Text              string       `json:"text"`
	ValidFrom         time.Time    `json:"valid_from,omitempty"`
	ValidTo           time.Time    `json:"valid_to,omitempty"`
	OccurredAt        time.Time    `json:"occurred_at,omitempty"`
	PredictionStartAt time.Time    `json:"prediction_start_at,omitempty"`
	PredictionDueAt   time.Time    `json:"prediction_due_at,omitempty"`
}

func (n GraphNode) MarshalJSON() ([]byte, error) {
	type graphNodePayload struct {
		ID                string       `json:"id"`
		Kind              NodeKind     `json:"kind"`
		Form              NodeForm     `json:"form,omitempty"`
		Function          NodeFunction `json:"function,omitempty"`
		Text              string       `json:"text"`
		ValidFrom         *time.Time   `json:"valid_from,omitempty"`
		ValidTo           *time.Time   `json:"valid_to,omitempty"`
		OccurredAt        *time.Time   `json:"occurred_at,omitempty"`
		PredictionStartAt *time.Time   `json:"prediction_start_at,omitempty"`
		PredictionDueAt   *time.Time   `json:"prediction_due_at,omitempty"`
	}
	normalized, err := n.normalizedSchema()
	if err == nil {
		n = normalized
	}
	payload := graphNodePayload{
		ID:       n.ID,
		Kind:     n.Kind,
		Form:     n.Form,
		Function: n.Function,
		Text:     n.Text,
	}
	switch n.Kind {
	case NodeFact, NodeImplicitCondition, NodeMechanism:
		if !n.OccurredAt.IsZero() {
			t := n.OccurredAt
			payload.OccurredAt = &t
			break
		}
		if !n.ValidFrom.IsZero() {
			t := n.ValidFrom
			payload.ValidFrom = &t
		}
		if !n.ValidTo.IsZero() {
			t := n.ValidTo
			payload.ValidTo = &t
		}
	case NodePrediction:
		if !n.PredictionStartAt.IsZero() {
			t := n.PredictionStartAt
			payload.PredictionStartAt = &t
		} else if !n.ValidFrom.IsZero() {
			t := n.ValidFrom
			payload.ValidFrom = &t
		}
		if !n.PredictionDueAt.IsZero() {
			t := n.PredictionDueAt
			payload.PredictionDueAt = &t
		} else if !n.ValidTo.IsZero() {
			t := n.ValidTo
			payload.ValidTo = &t
		}
	default:
		if !n.ValidFrom.IsZero() {
			t := n.ValidFrom
			payload.ValidFrom = &t
		}
		if !n.ValidTo.IsZero() {
			t := n.ValidTo
			payload.ValidTo = &t
		}
	}
	return json.Marshal(payload)
}

func (n GraphNode) LegacyValidityWindow() (time.Time, time.Time) {
	if !n.ValidFrom.IsZero() && !n.ValidTo.IsZero() {
		return n.ValidFrom, n.ValidTo
	}
	switch n.Kind {
	case NodeFact, NodeImplicitCondition, NodeMechanism:
		if !n.OccurredAt.IsZero() {
			return n.OccurredAt, openEndedNodeTime
		}
	case NodePrediction:
		if !n.PredictionStartAt.IsZero() {
			if !n.PredictionDueAt.IsZero() {
				return n.PredictionStartAt, n.PredictionDueAt
			}
			return n.PredictionStartAt, openEndedNodeTime
		}
	}
	return time.Time{}, time.Time{}
}

func (n *GraphNode) UnmarshalJSON(data []byte) error {
	type alias GraphNode
	var aux struct {
		alias
		Content string `json:"content"`
	}
	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}
	*n = GraphNode(aux.alias)
	if strings.TrimSpace(n.Text) == "" {
		n.Text = aux.Content
	}
	normalized, err := n.normalizedSchema()
	if err != nil {
		return err
	}
	*n = normalized
	return nil
}

func (n GraphNode) normalizedSchema() (GraphNode, error) {
	normalized := n
	if shouldNormalizeToExplicitCondition(normalized.Kind, normalized.Text) {
		normalized.Kind = NodeExplicitCondition
	}
	if normalized.Kind == "" {
		kind, ok := inferNodeKindFromFormFunction(normalized.Form, normalized.Function, normalized.Text)
		if !ok {
			return GraphNode{}, fmt.Errorf("unsupported node schema: form=%s function=%s", normalized.Form, normalized.Function)
		}
		normalized.Kind = kind
	}
	expectedForm, expectedFunction, ok := defaultFormFunctionForKind(normalized.Kind)
	if !ok {
		return GraphNode{}, fmt.Errorf("unsupported node kind: %s", normalized.Kind)
	}
	if normalized.Form != "" && normalized.Form != expectedForm {
		return GraphNode{}, fmt.Errorf("node form %q does not match kind %q", normalized.Form, normalized.Kind)
	}
	if normalized.Function != "" && normalized.Function != expectedFunction {
		return GraphNode{}, fmt.Errorf("node function %q does not match kind %q", normalized.Function, normalized.Kind)
	}
	normalized.Form = expectedForm
	normalized.Function = expectedFunction
	return normalized, nil
}

type GraphEdge struct {
	From string   `json:"from"`
	To   string   `json:"to"`
	Kind EdgeKind `json:"kind"`
}

func (e *GraphEdge) UnmarshalJSON(data []byte) error {
	type alias GraphEdge
	var aux struct {
		alias
		Source   string `json:"source"`
		Target   string `json:"target"`
		Relation string `json:"relation"`
	}
	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}
	*e = GraphEdge(aux.alias)
	if strings.TrimSpace(e.From) == "" {
		e.From = aux.Source
	}
	if strings.TrimSpace(e.To) == "" {
		e.To = aux.Target
	}
	if strings.TrimSpace(string(e.Kind)) == "" && strings.TrimSpace(aux.Relation) != "" {
		e.Kind = normalizeEdgeKind(EdgeKind(strings.TrimSpace(aux.Relation)))
	} else {
		e.Kind = normalizeEdgeKind(e.Kind)
	}
	return nil
}

type ReasoningGraph struct {
	Nodes []GraphNode `json:"nodes,omitempty"`
	Edges []GraphEdge `json:"edges,omitempty"`
}

type TransmissionPath struct {
	Driver string   `json:"driver"`
	Target string   `json:"target"`
	Steps  []string `json:"steps,omitempty"`
}

type Branch struct {
	ID                string             `json:"id,omitempty"`
	Level             string             `json:"level,omitempty"`
	Policy            string             `json:"policy,omitempty"`
	Thesis            string             `json:"thesis,omitempty"`
	Anchors           []string           `json:"anchors,omitempty"`
	BranchDrivers     []string           `json:"branch_drivers,omitempty"`
	Drivers           []string           `json:"drivers,omitempty"`
	Targets           []string           `json:"targets,omitempty"`
	TransmissionPaths []TransmissionPath `json:"transmission_paths,omitempty"`
}

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
	VerifiedAt              time.Time                    `json:"verified_at,omitempty"`
	Model                   string                       `json:"model,omitempty"`
	Version                 string                       `json:"version,omitempty"`
	RolloutStage            string                       `json:"rollout_stage,omitempty"`
	NodeVerifications       []NodeVerification           `json:"node_verifications,omitempty"`
	PathVerifications       []PathVerification           `json:"path_verifications,omitempty"`
	RealizedChecks          []RealizedCheck              `json:"realized_checks,omitempty"`
	FutureConditionChecks   []FutureConditionCheck       `json:"future_condition_checks,omitempty"`
	FactChecks              []FactCheck                  `json:"fact_checks,omitempty"`
	ExplicitConditionChecks []ExplicitConditionCheck     `json:"explicit_condition_checks,omitempty"`
	ImplicitConditionChecks []ImplicitConditionCheck     `json:"implicit_condition_checks,omitempty"`
	PredictionChecks        []PredictionCheck            `json:"prediction_checks,omitempty"`
	Passes                  []VerificationPass           `json:"passes,omitempty"`
	CoverageSummary         *VerificationCoverageSummary `json:"coverage_summary,omitempty"`
}

func (v Verification) IsZero() bool {
	return v.VerifiedAt.IsZero() &&
		v.Model == "" &&
		v.Version == "" &&
		v.RolloutStage == "" &&
		len(v.NodeVerifications) == 0 &&
		len(v.PathVerifications) == 0 &&
		len(v.RealizedChecks) == 0 &&
		len(v.FutureConditionChecks) == 0 &&
		len(v.FactChecks) == 0 &&
		len(v.ExplicitConditionChecks) == 0 &&
		len(v.ImplicitConditionChecks) == 0 &&
		len(v.PredictionChecks) == 0 &&
		len(v.Passes) == 0 &&
		v.CoverageSummary == nil
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
