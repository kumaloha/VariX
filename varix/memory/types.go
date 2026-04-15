package memory

import "time"

type AcceptedNode struct {
	MemoryID         int64     `json:"memory_id"`
	UserID           string    `json:"user_id"`
	SourcePlatform   string    `json:"source_platform"`
	SourceExternalID string    `json:"source_external_id"`
	RootExternalID   string    `json:"root_external_id,omitempty"`
	NodeID           string    `json:"node_id"`
	NodeKind         string    `json:"node_kind"`
	NodeText         string    `json:"node_text"`
	SourceModel      string    `json:"source_model"`
	SourceCompiledAt time.Time `json:"source_compiled_at"`
	ValidFrom        time.Time `json:"valid_from"`
	ValidTo          time.Time `json:"valid_to"`
	AcceptedAt       time.Time `json:"accepted_at"`
}

type AcceptanceNodeSnapshot struct {
	NodeID    string    `json:"node_id"`
	NodeKind  string    `json:"node_kind"`
	NodeText  string    `json:"node_text"`
	ValidFrom time.Time `json:"valid_from"`
	ValidTo   time.Time `json:"valid_to"`
}

type AcceptanceEvent struct {
	EventID           int64                    `json:"event_id"`
	UserID            string                   `json:"user_id"`
	TriggerType       string                   `json:"trigger_type"`
	SourcePlatform    string                   `json:"source_platform"`
	SourceExternalID  string                   `json:"source_external_id"`
	RootExternalID    string                   `json:"root_external_id,omitempty"`
	SourceModel       string                   `json:"source_model"`
	SourceCompiledAt  time.Time                `json:"source_compiled_at"`
	AcceptedCount     int                      `json:"accepted_count"`
	AcceptedAt        time.Time                `json:"accepted_at"`
	AcceptedNodeState []AcceptanceNodeSnapshot `json:"accepted_node_state"`
}

type OrganizationJob struct {
	JobID            int64     `json:"job_id"`
	TriggerEventID   int64     `json:"trigger_event_id"`
	UserID           string    `json:"user_id"`
	SourcePlatform   string    `json:"source_platform"`
	SourceExternalID string    `json:"source_external_id"`
	Status           string    `json:"status"`
	CreatedAt        time.Time `json:"created_at"`
	StartedAt        time.Time `json:"started_at,omitempty"`
	FinishedAt       time.Time `json:"finished_at,omitempty"`
}

type DedupeGroup struct {
	NodeIDs              []string `json:"node_ids"`
	RepresentativeNodeID string   `json:"representative_node_id,omitempty"`
	CanonicalText        string   `json:"canonical_text,omitempty"`
	Reason               string   `json:"reason,omitempty"`
	Hint                 string   `json:"hint,omitempty"`
}

type ContradictionGroup struct {
	NodeIDs    []string `json:"node_ids"`
	Reason     string   `json:"reason,omitempty"`
	ReasonCode string   `json:"reason_code,omitempty"`
}

type HierarchyLink struct {
	ParentNodeID string `json:"parent_node_id"`
	ParentKind   string `json:"parent_kind,omitempty"`
	ChildNodeID  string `json:"child_node_id"`
	ChildKind    string `json:"child_kind,omitempty"`
	Kind         string `json:"kind"`
	Source       string `json:"source,omitempty"`
	Hint         string `json:"hint,omitempty"`
}

type NodeHint struct {
	NodeID               string   `json:"node_id"`
	State                string   `json:"state,omitempty"`
	PreferredForDisplay  bool     `json:"preferred_for_display,omitempty"`
	VerificationStatus   string   `json:"verification_status,omitempty"`
	ConditionProbability string   `json:"condition_probability,omitempty"`
	PredictionStatus     string   `json:"prediction_status,omitempty"`
	DedupePeerNodeIDs    []string `json:"dedupe_peer_node_ids,omitempty"`
	ContradictionNodeIDs []string `json:"contradiction_node_ids,omitempty"`
	ParentNodeIDs        []string `json:"parent_node_ids,omitempty"`
	ChildNodeIDs         []string `json:"child_node_ids,omitempty"`
	HierarchyRole        string   `json:"hierarchy_role,omitempty"`
}

type OrganizationOutput struct {
	OutputID            int64                `json:"output_id"`
	JobID               int64                `json:"job_id"`
	UserID              string               `json:"user_id"`
	SourcePlatform      string               `json:"source_platform"`
	SourceExternalID    string               `json:"source_external_id"`
	GeneratedAt         time.Time            `json:"generated_at"`
	ActiveNodes         []AcceptedNode       `json:"active_nodes"`
	InactiveNodes       []AcceptedNode       `json:"inactive_nodes"`
	DedupeGroups        []DedupeGroup        `json:"dedupe_groups,omitempty"`
	ContradictionGroups []ContradictionGroup `json:"contradiction_groups,omitempty"`
	Hierarchy           []HierarchyLink      `json:"hierarchy,omitempty"`
	PredictionStatuses  []PredictionStatus   `json:"prediction_statuses,omitempty"`
	FactVerifications   []FactVerification   `json:"fact_verifications,omitempty"`
	OpenQuestions       []string             `json:"open_questions,omitempty"`
	NodeHints           []NodeHint           `json:"node_hints,omitempty"`
}

type GlobalCluster struct {
	ClusterID              string              `json:"cluster_id"`
	CanonicalProposition   string              `json:"canonical_proposition"`
	Summary                string              `json:"summary,omitempty"`
	RepresentativeNodeID   string              `json:"representative_node_id,omitempty"`
	SupportingNodeIDs      []string            `json:"supporting_node_ids,omitempty"`
	ConflictingNodeIDs     []string            `json:"conflicting_node_ids,omitempty"`
	ConditionalNodeIDs     []string            `json:"conditional_node_ids,omitempty"`
	PredictiveNodeIDs      []string            `json:"predictive_node_ids,omitempty"`
	CoreSupportingNodeIDs  []string            `json:"core_supporting_node_ids,omitempty"`
	CoreConditionalNodeIDs []string            `json:"core_conditional_node_ids,omitempty"`
	CoreConclusionNodeIDs  []string            `json:"core_conclusion_node_ids,omitempty"`
	CorePredictiveNodeIDs  []string            `json:"core_predictive_node_ids,omitempty"`
	ExpandedNodeIDs        []string            `json:"expanded_node_ids,omitempty"`
	SynthesizedEdges       []GlobalClusterEdge `json:"synthesized_edges,omitempty"`
	Active                 bool                `json:"active"`
	UpdatedAt              time.Time           `json:"updated_at"`
}

type GlobalClusterEdge struct {
	From string `json:"from"`
	To   string `json:"to"`
	Kind string `json:"kind"`
}

type GlobalOrganizationOutput struct {
	OutputID            int64                `json:"output_id"`
	UserID              string               `json:"user_id"`
	GeneratedAt         time.Time            `json:"generated_at"`
	ActiveNodes         []AcceptedNode       `json:"active_nodes"`
	InactiveNodes       []AcceptedNode       `json:"inactive_nodes"`
	DedupeGroups        []DedupeGroup        `json:"dedupe_groups,omitempty"`
	ContradictionGroups []ContradictionGroup `json:"contradiction_groups,omitempty"`
	Clusters            []GlobalCluster      `json:"clusters,omitempty"`
	OpenQuestions       []string             `json:"open_questions,omitempty"`
}

type CandidateThesis struct {
	ThesisID      string    `json:"thesis_id"`
	UserID        string    `json:"user_id"`
	TopicLabel    string    `json:"topic_label"`
	NodeIDs       []string  `json:"node_ids,omitempty"`
	SourceRefs    []string  `json:"source_refs,omitempty"`
	ClusterReason string    `json:"cluster_reason,omitempty"`
	CoverageScore float64   `json:"coverage_score,omitempty"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

type ConflictSet struct {
	ConflictID      string    `json:"conflict_id"`
	ThesisID        string    `json:"thesis_id"`
	ConflictStatus  string    `json:"conflict_status"`
	ConflictTopic   string    `json:"conflict_topic,omitempty"`
	SideANodeIDs    []string  `json:"side_a_node_ids,omitempty"`
	SideBNodeIDs    []string  `json:"side_b_node_ids,omitempty"`
	SideASourceRefs []string  `json:"side_a_source_refs,omitempty"`
	SideBSourceRefs []string  `json:"side_b_source_refs,omitempty"`
	SideASummary    string    `json:"side_a_summary,omitempty"`
	SideBSummary    string    `json:"side_b_summary,omitempty"`
	ConflictReason  string    `json:"conflict_reason,omitempty"`
	SharedQuestion  string    `json:"shared_question,omitempty"`
	UserResolution  string    `json:"user_resolution,omitempty"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

type CausalEdge struct {
	From       string  `json:"from"`
	To         string  `json:"to"`
	Kind       string  `json:"kind"`
	Confidence float64 `json:"confidence,omitempty"`
}

type CausalThesis struct {
	CausalThesisID    string              `json:"causal_thesis_id"`
	ThesisID          string              `json:"thesis_id"`
	Status            string              `json:"status"`
	CoreQuestion      string              `json:"core_question,omitempty"`
	MechanismSummary  string              `json:"mechanism_summary,omitempty"`
	NodeRoles         map[string]string   `json:"node_roles,omitempty"`
	Edges             []CausalEdge        `json:"edges,omitempty"`
	EntryNodeIDs      []string            `json:"entry_node_ids,omitempty"`
	CorePathNodeIDs   []string            `json:"core_path_node_ids,omitempty"`
	SupportingNodeIDs []string            `json:"supporting_node_ids,omitempty"`
	BoundaryNodeIDs   []string            `json:"boundary_node_ids,omitempty"`
	PredictionNodeIDs []string            `json:"prediction_node_ids,omitempty"`
	SourceRefs        []string            `json:"source_refs,omitempty"`
	TraceabilityMap   map[string][]string `json:"traceability_map,omitempty"`
	CompletenessScore float64             `json:"completeness_score,omitempty"`
	AbstractionReady  bool                `json:"abstraction_ready"`
}

type CardChainStep struct {
	Label          string   `json:"label"`
	Role           string   `json:"role"`
	BackingNodeIDs []string `json:"backing_node_ids,omitempty"`
}

type CognitiveCard struct {
	CardID          string          `json:"card_id"`
	CausalThesisID  string          `json:"causal_thesis_id"`
	CardType        string          `json:"card_type"`
	Title           string          `json:"title"`
	Summary         string          `json:"summary,omitempty"`
	CausalChain     []CardChainStep `json:"causal_chain,omitempty"`
	KeyEvidence     []string        `json:"key_evidence,omitempty"`
	Conditions      []string        `json:"conditions,omitempty"`
	Predictions     []string        `json:"predictions,omitempty"`
	SourceRefs      []string        `json:"source_refs,omitempty"`
	ConfidenceLabel string          `json:"confidence_label,omitempty"`
	ConflictFlag    bool            `json:"conflict_flag,omitempty"`
	TraceEntry      []string        `json:"trace_entry,omitempty"`
}

type CognitiveConclusion struct {
	ConclusionID       string   `json:"conclusion_id"`
	CausalThesisID     string   `json:"causal_thesis_id"`
	Headline           string   `json:"headline"`
	Subheadline        string   `json:"subheadline,omitempty"`
	ConclusionType     string   `json:"conclusion_type,omitempty"`
	BackingCardIDs     []string `json:"backing_card_ids,omitempty"`
	CoreClaims         []string `json:"core_claims,omitempty"`
	WhyItExists        string   `json:"why_it_exists,omitempty"`
	AbstractionLevel   string   `json:"abstraction_level,omitempty"`
	TraceabilityStatus string   `json:"traceability_status,omitempty"`
	BlockedByConflict  bool     `json:"blocked_by_conflict,omitempty"`
	Freshness          string   `json:"freshness,omitempty"`
}

type TopMemoryItem struct {
	ItemID          string    `json:"item_id"`
	ItemType        string    `json:"item_type"`
	Headline        string    `json:"headline"`
	Subheadline     string    `json:"subheadline,omitempty"`
	BackingObjectID string    `json:"backing_object_id"`
	SignalStrength  string    `json:"signal_strength,omitempty"`
	UpdatedAt       time.Time `json:"updated_at"`
}

type GlobalMemoryV2Output struct {
	OutputID             int64                 `json:"output_id"`
	UserID               string                `json:"user_id"`
	GeneratedAt          time.Time             `json:"generated_at"`
	CandidateTheses      []CandidateThesis     `json:"candidate_theses,omitempty"`
	ConflictSets         []ConflictSet         `json:"conflict_sets,omitempty"`
	CausalTheses         []CausalThesis        `json:"causal_theses,omitempty"`
	CognitiveCards       []CognitiveCard       `json:"cognitive_cards,omitempty"`
	CognitiveConclusions []CognitiveConclusion `json:"cognitive_conclusions,omitempty"`
	TopMemoryItems       []TopMemoryItem       `json:"top_memory_items,omitempty"`
}

type PredictionStatus struct {
	NodeID string `json:"node_id"`
	Status string `json:"status"`
	Reason string `json:"reason,omitempty"`
}

type FactVerification struct {
	NodeID string `json:"node_id"`
	Status string `json:"status"`
	Reason string `json:"reason,omitempty"`
}

type AcceptRequest struct {
	UserID           string   `json:"user_id"`
	SourcePlatform   string   `json:"source_platform"`
	SourceExternalID string   `json:"source_external_id"`
	NodeIDs          []string `json:"node_ids"`
}

type AcceptResult struct {
	Nodes []AcceptedNode  `json:"nodes"`
	Event AcceptanceEvent `json:"event"`
	Job   OrganizationJob `json:"job"`
}
