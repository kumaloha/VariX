package memory

import "time"

// Relation-first synthesis types live here so canonical entities, relations, mechanisms, and aggregates stay reviewable as one boundary.

type CanonicalEntityType string

type CanonicalEntityStatus string

type CanonicalObjectType string

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
	CanonicalObjectDriver           CanonicalObjectType = "driver"
	CanonicalObjectTarget           CanonicalObjectType = "target"
	CanonicalObjectTransmission     CanonicalObjectType = "transmission"
	CanonicalObjectTransmissionPath CanonicalObjectType = "transmission_path"
	CanonicalObjectPathNode         CanonicalObjectType = "path_node"
	CanonicalObjectPathEdge         CanonicalObjectType = "path_edge"
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

type MechanismGraph struct {
	Mechanism    Mechanism       `json:"mechanism"`
	Nodes        []MechanismNode `json:"nodes,omitempty"`
	Edges        []MechanismEdge `json:"edges,omitempty"`
	PathOutcomes []PathOutcome   `json:"path_outcomes,omitempty"`
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
	PathOrder       int               `json:"path_order,omitempty"`
	CreatedAt       time.Time         `json:"created_at"`
}

type PathOutcome struct {
	PathOutcomeID     string          `json:"path_outcome_id"`
	MechanismID       string          `json:"mechanism_id"`
	NodePath          []string        `json:"node_path,omitempty"`
	EdgePath          []string        `json:"edge_path,omitempty"`
	OutcomePolarity   OutcomePolarity `json:"outcome_polarity"`
	OutcomeLabel      string          `json:"outcome_label"`
	ConditionScope    string          `json:"condition_scope,omitempty"`
	PredictionNodeIDs []string        `json:"prediction_node_ids,omitempty"`
	PredictionStartAt time.Time       `json:"prediction_start_at,omitempty"`
	PredictionDueAt   time.Time       `json:"prediction_due_at,omitempty"`
	Confidence        float64         `json:"confidence"`
	CreatedAt         time.Time       `json:"created_at"`
}

type RawCanonicalMapping struct {
	CanonicalObjectType CanonicalObjectType `json:"canonical_object_type"`
	CanonicalObjectID   string              `json:"canonical_object_id"`
	SourcePlatform      string              `json:"source_platform"`
	SourceExternalID    string              `json:"source_external_id"`
	RawNodeID           string              `json:"raw_node_id,omitempty"`
	RawEdgeKey          string              `json:"raw_edge_key,omitempty"`
	MappingConfidence   float64             `json:"mapping_confidence"`
	CreatedAt           time.Time           `json:"created_at"`
	UpdatedAt           time.Time           `json:"updated_at"`
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
