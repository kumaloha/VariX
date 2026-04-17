package memory

import "time"

// Legacy thesis-first v2 structs remain isolated here during the current coexistence period with the relation-first model.

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
	CardID          string          `json:"card_id"`
	RelationID      string          `json:"relation_id,omitempty"`
	AsOf            time.Time       `json:"as_of,omitempty"`
	CausalThesisID  string          `json:"causal_thesis_id,omitempty"`
	CardType        string          `json:"card_type,omitempty"`
	Title           string          `json:"title"`
	Summary         string          `json:"summary,omitempty"`
	MechanismChain  []string        `json:"mechanism_chain,omitempty"`
	CausalChain     []CardChainStep `json:"causal_chain,omitempty"`
	KeyEvidence     []string        `json:"key_evidence,omitempty"`
	Conditions      []string        `json:"conditions,omitempty"`
	Predictions     []string        `json:"predictions,omitempty"`
	SourceRefs      []string        `json:"source_refs,omitempty"`
	ConfidenceLabel ConfidenceLabel `json:"confidence_label,omitempty"`
	ConflictFlag    bool            `json:"conflict_flag,omitempty"`
	TraceEntry      []string        `json:"trace_entry,omitempty"`
	CreatedAt       time.Time       `json:"created_at,omitempty"`
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
