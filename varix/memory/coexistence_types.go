package memory

import "time"

// GlobalMemoryV2Output intentionally references both relation-first and legacy thesis-first structs while the coexistence strategy remains active.

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
