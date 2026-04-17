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

type CanonicalEntityType string

type CanonicalEntityStatus string

type RelationStatus string

type MechanismStatus string

type TraceabilityStatus string

type MechanismNodeType string

type MechanismEdgeType string

type OutcomePolarity string

type ConflictScopeType string

type ConflictStatus string

type ConfidenceLabel string

type TopMemoryItemType string

type SignalStrength string

const (
	CanonicalEntityDriver CanonicalEntityType = "driver"
	CanonicalEntityTarget CanonicalEntityType = "target"
	CanonicalEntityBoth   CanonicalEntityType = "both"
)

const (
	CanonicalEntityActive  CanonicalEntityStatus = "active"
	CanonicalEntityMerged  CanonicalEntityStatus = "merged"
	CanonicalEntitySplit   CanonicalEntityStatus = "split"
	CanonicalEntityRetired CanonicalEntityStatus = "retired"
)

const (
	RelationActive     RelationStatus = "active"
	RelationInactive   RelationStatus = "inactive"
	RelationRetired    RelationStatus = "retired"
	RelationMerged     RelationStatus = "merged"
	RelationSplit      RelationStatus = "split"
	RelationSuperseded RelationStatus = "superseded"
)

const (
	MechanismActive      MechanismStatus = "active"
	MechanismHistorical  MechanismStatus = "historical"
	MechanismInvalidated MechanismStatus = "invalidated"
)

const (
	TraceabilityComplete TraceabilityStatus = "complete"
	TraceabilityPartial  TraceabilityStatus = "partial"
	TraceabilityWeak     TraceabilityStatus = "weak"
)

const (
	MechanismNodeDriver         MechanismNodeType = "driver"
	MechanismNodeMacroEvent     MechanismNodeType = "macro_event"
	MechanismNodePolicyState    MechanismNodeType = "policy_state"
	MechanismNodeLiquidityState MechanismNodeType = "liquidity_state"
	MechanismNodeMarketBehavior MechanismNodeType = "market_behavior"
	MechanismNodeAssetFlow      MechanismNodeType = "asset_flow"
	MechanismNodeCondition      MechanismNodeType = "condition"
	MechanismNodeBoundary       MechanismNodeType = "boundary"
	MechanismNodeTargetEffect   MechanismNodeType = "target_effect"
)

const (
	MechanismEdgeCauses        MechanismEdgeType = "causes"
	MechanismEdgeAmplifies     MechanismEdgeType = "amplifies"
	MechanismEdgeSuppresses    MechanismEdgeType = "suppresses"
	MechanismEdgeTransmits     MechanismEdgeType = "transmits"
	MechanismEdgeRequires      MechanismEdgeType = "requires"
	MechanismEdgePresets       MechanismEdgeType = "presets"
	MechanismEdgeConflictsWith MechanismEdgeType = "conflicts_with"
)

const (
	OutcomeBullish     OutcomePolarity = "bullish"
	OutcomeBearish     OutcomePolarity = "bearish"
	OutcomeMixed       OutcomePolarity = "mixed"
	OutcomeConditional OutcomePolarity = "conditional"
	OutcomeUnresolved  OutcomePolarity = "unresolved"
)

const (
	ConflictScopeRelation        ConflictScopeType = "relation"
	ConflictScopeDriverAggregate ConflictScopeType = "driver_aggregate"
	ConflictScopeTargetAggregate ConflictScopeType = "target_aggregate"
)

const (
	ConflictActive     ConflictStatus = "active"
	ConflictDowngraded ConflictStatus = "downgraded"
	ConflictResolved   ConflictStatus = "resolved"
)

const (
	ConfidenceWeak   ConfidenceLabel = "weak"
	ConfidenceMedium ConfidenceLabel = "medium"
	ConfidenceStrong ConfidenceLabel = "strong"
)

const (
	TopMemoryItemDriverAggregate TopMemoryItemType = "driver_aggregate"
	TopMemoryItemTargetAggregate TopMemoryItemType = "target_aggregate"
	TopMemoryItemCard            TopMemoryItemType = "card"
	TopMemoryItemConclusion      TopMemoryItemType = "conclusion"
	TopMemoryItemConflict        TopMemoryItemType = "conflict"
)

const (
	SignalLow    SignalStrength = "low"
	SignalMedium SignalStrength = "medium"
	SignalHigh   SignalStrength = "high"
)

type CanonicalEntity struct {
	EntityID      string                `json:"entity_id"`
	EntityType    CanonicalEntityType   `json:"entity_type"`
	CanonicalName string                `json:"canonical_name"`
	Aliases       []string              `json:"aliases,omitempty"`
	Status        CanonicalEntityStatus `json:"status"`
	MergeHistory  []string              `json:"merge_history,omitempty"`
	SplitHistory  []string              `json:"split_history,omitempty"`
	CreatedAt     time.Time             `json:"created_at"`
	UpdatedAt     time.Time             `json:"updated_at"`
}

type Relation struct {
	RelationID             string         `json:"relation_id"`
	DriverEntityID         string         `json:"driver_entity_id"`
	TargetEntityID         string         `json:"target_entity_id"`
	Status                 RelationStatus `json:"status"`
	RetiredAt              time.Time      `json:"retired_at,omitempty"`
	SupersededByRelationID string         `json:"superseded_by_relation_id,omitempty"`
	MergeHistory           []string       `json:"merge_history,omitempty"`
	SplitHistory           []string       `json:"split_history,omitempty"`
	LifecycleReason        string         `json:"lifecycle_reason,omitempty"`
	CreatedAt              time.Time      `json:"created_at"`
	UpdatedAt              time.Time      `json:"updated_at"`
}

func (r Relation) IsActive() bool {
	return r.Status == RelationActive || r.Status == RelationInactive
}

type Mechanism struct {
	MechanismID        string             `json:"mechanism_id"`
	RelationID         string             `json:"relation_id"`
	AsOf               time.Time          `json:"as_of"`
	ValidFrom          time.Time          `json:"valid_from,omitempty"`
	ValidTo            time.Time          `json:"valid_to,omitempty"`
	Confidence         float64            `json:"confidence"`
	Status             MechanismStatus    `json:"status"`
	SourceRefs         []string           `json:"source_refs,omitempty"`
	TraceabilityStatus TraceabilityStatus `json:"traceability_status"`
	CreatedAt          time.Time          `json:"created_at"`
	UpdatedAt          time.Time          `json:"updated_at"`
}

func (m Mechanism) IsHistorical() bool {
	return m.Status == MechanismHistorical
}

type MechanismNode struct {
	MechanismNodeID        string            `json:"mechanism_node_id"`
	MechanismID            string            `json:"mechanism_id"`
	NodeType               MechanismNodeType `json:"node_type"`
	Label                  string            `json:"label"`
	BackingAcceptedNodeIDs []string          `json:"backing_accepted_node_ids,omitempty"`
	SortOrder              int               `json:"sort_order,omitempty"`
	CreatedAt              time.Time         `json:"created_at"`
}

type MechanismEdge struct {
	MechanismEdgeID string            `json:"mechanism_edge_id"`
	MechanismID     string            `json:"mechanism_id"`
	FromNodeID      string            `json:"from_node_id"`
	ToNodeID        string            `json:"to_node_id"`
	EdgeType        MechanismEdgeType `json:"edge_type"`
	CreatedAt       time.Time         `json:"created_at"`
}

type PathOutcome struct {
	PathOutcomeID   string          `json:"path_outcome_id"`
	MechanismID     string          `json:"mechanism_id"`
	NodePath        []string        `json:"node_path,omitempty"`
	OutcomePolarity OutcomePolarity `json:"outcome_polarity"`
	OutcomeLabel    string          `json:"outcome_label"`
	ConditionScope  string          `json:"condition_scope,omitempty"`
	Confidence      float64         `json:"confidence"`
	CreatedAt       time.Time       `json:"created_at"`
}

type DriverAggregate struct {
	AggregateID         string             `json:"aggregate_id"`
	DriverEntityID      string             `json:"driver_entity_id"`
	RelationIDs         []string           `json:"relation_ids,omitempty"`
	TargetEntityIDs     []string           `json:"target_entity_ids,omitempty"`
	MechanismLabels     []string           `json:"mechanism_labels,omitempty"`
	CoverageScore       float64            `json:"coverage_score"`
	ConflictCount       int                `json:"conflict_count"`
	ActiveConclusionIDs []string           `json:"active_conclusion_ids,omitempty"`
	TraceabilityStatus  TraceabilityStatus `json:"traceability_status"`
	AsOf                time.Time          `json:"as_of"`
	CreatedAt           time.Time          `json:"created_at"`
}

type TargetAggregate struct {
	AggregateID         string             `json:"aggregate_id"`
	TargetEntityID      string             `json:"target_entity_id"`
	RelationIDs         []string           `json:"relation_ids,omitempty"`
	DriverEntityIDs     []string           `json:"driver_entity_ids,omitempty"`
	MechanismLabels     []string           `json:"mechanism_labels,omitempty"`
	CoverageScore       float64            `json:"coverage_score"`
	ConflictCount       int                `json:"conflict_count"`
	ActiveConclusionIDs []string           `json:"active_conclusion_ids,omitempty"`
	TraceabilityStatus  TraceabilityStatus `json:"traceability_status"`
	AsOf                time.Time          `json:"as_of"`
	CreatedAt           time.Time          `json:"created_at"`
}

type ConflictView struct {
	ConflictID          string              `json:"conflict_id"`
	ScopeType           ConflictScopeType   `json:"scope_type"`
	ScopeID             string              `json:"scope_id"`
	LeftPathOutcomeIDs  []string            `json:"left_path_outcome_ids,omitempty"`
	RightPathOutcomeIDs []string            `json:"right_path_outcome_ids,omitempty"`
	ConflictReason      string              `json:"conflict_reason"`
	ConflictTopic       string              `json:"conflict_topic,omitempty"`
	Status              ConflictStatus      `json:"status"`
	AsOf                time.Time           `json:"as_of"`
	TraceabilityMap     map[string][]string `json:"traceability_map,omitempty"`
	CreatedAt           time.Time           `json:"created_at"`
}

type ConclusionJudgeMetadata struct {
	JudgeModel         string             `json:"judge_model,omitempty"`
	JudgePromptVersion string             `json:"judge_prompt_version,omitempty"`
	JudgeScores        map[string]float64 `json:"judge_scores,omitempty"`
	JudgePassed        bool               `json:"judge_passed,omitempty"`
	JudgedAt           time.Time          `json:"judged_at,omitempty"`
}

// Legacy thesis-first v2 structs remain below for coexistence during the
// relation-first rollout.
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
	SideAWhy        []string  `json:"side_a_why,omitempty"`
	SideBWhy        []string  `json:"side_b_why,omitempty"`
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
	CardID           string          `json:"card_id"`
	RelationID       string          `json:"relation_id,omitempty"`
	AsOf             time.Time       `json:"as_of,omitempty"`
	CausalThesisID   string          `json:"causal_thesis_id,omitempty"`
	CardType         string          `json:"card_type,omitempty"`
	Title            string          `json:"title"`
	Summary          string          `json:"summary,omitempty"`
	MechanismChain   []string        `json:"mechanism_chain,omitempty"`
	CausalChain      []CardChainStep `json:"causal_chain,omitempty"`
	KeyEvidence      []string        `json:"key_evidence,omitempty"`
	Conditions       []string        `json:"conditions,omitempty"`
	Predictions      []string        `json:"predictions,omitempty"`
	SourceRefs       []string        `json:"source_refs,omitempty"`
	ConfidenceLabel  ConfidenceLabel `json:"confidence_label,omitempty"`
	ConflictFlag     bool            `json:"conflict_flag,omitempty"`
	TraceEntry       []string        `json:"trace_entry,omitempty"`
	CreatedAt        time.Time       `json:"created_at,omitempty"`
}

type CognitiveConclusion struct {
	ConclusionID       string                  `json:"conclusion_id"`
	SourceType         string                  `json:"source_type,omitempty"`
	SourceID           string                  `json:"source_id,omitempty"`
	CausalThesisID     string                  `json:"causal_thesis_id,omitempty"`
	Headline           string                  `json:"headline"`
	Subheadline        string                  `json:"subheadline,omitempty"`
	ConclusionType     string                  `json:"conclusion_type,omitempty"`
	BackingCardIDs     []string                `json:"backing_card_ids,omitempty"`
	CoreClaims         []string                `json:"core_claims,omitempty"`
	WhyItExists        string                  `json:"why_it_exists,omitempty"`
	AbstractionLevel   string                  `json:"abstraction_level,omitempty"`
	TraceabilityStatus TraceabilityStatus      `json:"traceability_status,omitempty"`
	BlockedByConflict  bool                    `json:"blocked_by_conflict,omitempty"`
	AsOf               time.Time               `json:"as_of,omitempty"`
	Judge              ConclusionJudgeMetadata `json:"judge,omitempty"`
	Freshness          string                  `json:"freshness,omitempty"`
	CreatedAt          time.Time               `json:"created_at,omitempty"`
}

func (c CognitiveConclusion) JudgePassed() bool {
	return c.Judge.JudgePassed
}

type TopMemoryItem struct {
	ItemID          string            `json:"item_id"`
	ItemType        TopMemoryItemType `json:"item_type"`
	Headline        string            `json:"headline"`
	Subheadline     string            `json:"subheadline,omitempty"`
	BackingObjectID string            `json:"backing_object_id"`
	SignalStrength  SignalStrength    `json:"signal_strength,omitempty"`
	AsOf            time.Time         `json:"as_of,omitempty"`
	UpdatedAt       time.Time         `json:"updated_at"`
}

type GlobalMemoryV2Output struct {
	OutputID             int64                 `json:"output_id"`
	UserID               string                `json:"user_id"`
	GeneratedAt          time.Time             `json:"generated_at"`
	CanonicalEntities    []CanonicalEntity     `json:"canonical_entities,omitempty"`
	Relations            []Relation            `json:"relations,omitempty"`
	Mechanisms           []Mechanism           `json:"mechanisms,omitempty"`
	MechanismNodes       []MechanismNode       `json:"mechanism_nodes,omitempty"`
	MechanismEdges       []MechanismEdge       `json:"mechanism_edges,omitempty"`
	PathOutcomes         []PathOutcome         `json:"path_outcomes,omitempty"`
	DriverAggregates     []DriverAggregate     `json:"driver_aggregates,omitempty"`
	TargetAggregates     []TargetAggregate     `json:"target_aggregates,omitempty"`
	ConflictViews        []ConflictView        `json:"conflict_views,omitempty"`
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
