# VariX Memory v2 Go Types 草案

## 1. Goal

这份文档把 relation-first schema 继续落到 **Go types 草案**。

目标是：
- 为 `varix/memory/types.go` 提供清晰候选结构
- 与现有风格保持一致（简单 struct + json tag + `time.Time`）
- 为后续实现减少命名摇摆

---

## 2. Design Conventions

1. 所有时间字段使用 `time.Time`
2. 状态 / 类型字段优先用 `string` alias type，而不是裸字符串
3. JSON tag 与 schema 字段名保持一致
4. 列表字段默认 `omitempty`
5. 不在 types 层过早塞复杂行为方法，只保留轻量 helper 空间

---

## 3. Proposed enum-like aliases

```go
package memory

type CanonicalEntityType string

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
	RelationActive     RelationStatus = "active"
	RelationInactive   RelationStatus = "inactive"
	RelationRetired    RelationStatus = "retired"
	RelationMerged     RelationStatus = "merged"
	RelationSplit      RelationStatus = "split"
	RelationSuperseded RelationStatus = "superseded"
)

const (
	MechanismActive     MechanismStatus = "active"
	MechanismHistorical MechanismStatus = "historical"
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
	MechanismEdgeCauses      MechanismEdgeType = "causes"
	MechanismEdgeAmplifies   MechanismEdgeType = "amplifies"
	MechanismEdgeSuppresses  MechanismEdgeType = "suppresses"
	MechanismEdgeTransmits   MechanismEdgeType = "transmits"
	MechanismEdgeRequires    MechanismEdgeType = "requires"
	MechanismEdgePresets     MechanismEdgeType = "presets"
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
```

---

## 4. Proposed structs

### 4.1 CanonicalEntity

```go
type CanonicalEntity struct {
	EntityID      string              `json:"entity_id"`
	EntityType    CanonicalEntityType `json:"entity_type"`
	CanonicalName string              `json:"canonical_name"`
	Aliases       []string            `json:"aliases,omitempty"`
	Status        string              `json:"status"`
	MergeHistory  []string            `json:"merge_history,omitempty"`
	SplitHistory  []string            `json:"split_history,omitempty"`
	CreatedAt     time.Time           `json:"created_at"`
	UpdatedAt     time.Time           `json:"updated_at"`
}
```

### 4.2 Relation

```go
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
```

### 4.3 Mechanism

```go
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
```

### 4.4 MechanismNode

```go
type MechanismNode struct {
	MechanismNodeID       string            `json:"mechanism_node_id"`
	MechanismID           string            `json:"mechanism_id"`
	NodeType              MechanismNodeType `json:"node_type"`
	Label                 string            `json:"label"`
	BackingAcceptedNodeIDs []string         `json:"backing_accepted_node_ids,omitempty"`
	SortOrder             int               `json:"sort_order,omitempty"`
	CreatedAt             time.Time         `json:"created_at"`
}
```

### 4.5 MechanismEdge

```go
type MechanismEdge struct {
	MechanismEdgeID string            `json:"mechanism_edge_id"`
	MechanismID     string            `json:"mechanism_id"`
	FromNodeID      string            `json:"from_node_id"`
	ToNodeID        string            `json:"to_node_id"`
	EdgeType        MechanismEdgeType `json:"edge_type"`
	CreatedAt       time.Time         `json:"created_at"`
}
```

### 4.6 PathOutcome

```go
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
```

### 4.7 DriverAggregate

```go
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
```

### 4.8 TargetAggregate

```go
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
```

### 4.9 ConflictView

```go
type ConflictView struct {
	ConflictID          string            `json:"conflict_id"`
	ScopeType           ConflictScopeType `json:"scope_type"`
	ScopeID             string            `json:"scope_id"`
	LeftPathOutcomeIDs  []string          `json:"left_path_outcome_ids,omitempty"`
	RightPathOutcomeIDs []string          `json:"right_path_outcome_ids,omitempty"`
	ConflictReason      string            `json:"conflict_reason"`
	ConflictTopic       string            `json:"conflict_topic,omitempty"`
	Status              ConflictStatus    `json:"status"`
	AsOf                time.Time         `json:"as_of"`
	TraceabilityMap     map[string][]string `json:"traceability_map,omitempty"`
	CreatedAt           time.Time         `json:"created_at"`
}
```

### 4.10 CognitiveCard

```go
type CognitiveCard struct {
	CardID           string          `json:"card_id"`
	RelationID       string          `json:"relation_id"`
	AsOf             time.Time       `json:"as_of"`
	Title            string          `json:"title"`
	Summary          string          `json:"summary"`
	MechanismChain   []string        `json:"mechanism_chain,omitempty"`
	KeyEvidence      []string        `json:"key_evidence,omitempty"`
	Conditions       []string        `json:"conditions,omitempty"`
	Predictions      []string        `json:"predictions,omitempty"`
	SourceRefs       []string        `json:"source_refs,omitempty"`
	ConfidenceLabel  ConfidenceLabel `json:"confidence_label"`
	TraceEntry       []string        `json:"trace_entry,omitempty"`
	CreatedAt        time.Time       `json:"created_at"`
}
```

### 4.11 Judge metadata helper

```go
type ConclusionJudgeMetadata struct {
	JudgeModel         string             `json:"judge_model,omitempty"`
	JudgePromptVersion string             `json:"judge_prompt_version,omitempty"`
	JudgeScores        map[string]float64 `json:"judge_scores,omitempty"`
	JudgePassed        bool               `json:"judge_passed,omitempty"`
	JudgedAt           time.Time          `json:"judged_at,omitempty"`
}
```

### 4.12 CognitiveConclusion

```go
type CognitiveConclusion struct {
	ConclusionID       string                  `json:"conclusion_id"`
	SourceType         string                  `json:"source_type"`
	SourceID           string                  `json:"source_id"`
	Headline           string                  `json:"headline"`
	Subheadline        string                  `json:"subheadline,omitempty"`
	BackingCardIDs     []string                `json:"backing_card_ids,omitempty"`
	CoreClaims         []string                `json:"core_claims,omitempty"`
	TraceabilityStatus TraceabilityStatus      `json:"traceability_status"`
	BlockedByConflict  bool                    `json:"blocked_by_conflict"`
	AsOf               time.Time               `json:"as_of"`
	Judge              ConclusionJudgeMetadata `json:"judge,omitempty"`
	CreatedAt          time.Time               `json:"created_at"`
}
```

### 4.13 TopMemoryItem

```go
type TopMemoryItem struct {
	ItemID          string            `json:"item_id"`
	ItemType        TopMemoryItemType `json:"item_type"`
	Headline        string            `json:"headline"`
	Subheadline     string            `json:"subheadline,omitempty"`
	BackingObjectID string            `json:"backing_object_id"`
	SignalStrength  SignalStrength    `json:"signal_strength"`
	AsOf            time.Time         `json:"as_of"`
	UpdatedAt       time.Time         `json:"updated_at"`
}
```

### 4.14 GlobalMemoryV2Output

```go
type GlobalMemoryV2Output struct {
	OutputID              int64                 `json:"output_id"`
	UserID                string                `json:"user_id"`
	GeneratedAt           time.Time             `json:"generated_at"`
	CanonicalEntities     []CanonicalEntity     `json:"canonical_entities,omitempty"`
	Relations             []Relation            `json:"relations,omitempty"`
	Mechanisms            []Mechanism           `json:"mechanisms,omitempty"`
	MechanismNodes        []MechanismNode       `json:"mechanism_nodes,omitempty"`
	MechanismEdges        []MechanismEdge       `json:"mechanism_edges,omitempty"`
	PathOutcomes          []PathOutcome         `json:"path_outcomes,omitempty"`
	DriverAggregates      []DriverAggregate     `json:"driver_aggregates,omitempty"`
	TargetAggregates      []TargetAggregate     `json:"target_aggregates,omitempty"`
	ConflictViews         []ConflictView        `json:"conflict_views,omitempty"`
	CognitiveCards        []CognitiveCard       `json:"cognitive_cards,omitempty"`
	CognitiveConclusions  []CognitiveConclusion `json:"cognitive_conclusions,omitempty"`
	TopMemoryItems        []TopMemoryItem       `json:"top_memory_items,omitempty"`
}
```

---

## 5. Optional lightweight helpers

These are acceptable in `varix/memory/types.go` if they reduce repeated string logic without adding domain-heavy behavior.

```go
func (r Relation) IsActive() bool {
	return r.Status == RelationActive || r.Status == RelationInactive
}

func (m Mechanism) IsHistorical() bool {
	return m.Status == MechanismHistorical
}

func (c CognitiveConclusion) JudgePassed() bool {
	return c.Judge.JudgePassed
}
```

---

## 6. Notes for implementation

1. Keep existing v1 types intact during migration.
2. Add v2 types additively before deleting old thesis-first types.
3. If coexistence pressure is high, old thesis-first types can remain for a while even after new types land.
4. JSON-heavy fields in SQLite can map cleanly to these structs, with marshaling at store boundaries.
5. Avoid embedding rendering-only concepts such as `primary_path` into persisted structs.
