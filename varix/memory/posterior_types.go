package memory

import "time"

type PosteriorState string

type PosteriorDiagnosisCode string

const (
	PosteriorStatePending   PosteriorState = "pending"
	PosteriorStateVerified  PosteriorState = "verified"
	PosteriorStateFalsified PosteriorState = "falsified"
	PosteriorStateBlocked   PosteriorState = "blocked"
)

const (
	PosteriorDiagnosisFactError  PosteriorDiagnosisCode = "fact_error"
	PosteriorDiagnosisLogicError PosteriorDiagnosisCode = "logic_error"
)

type PosteriorStateRecord struct {
	MemoryID         int64                  `json:"memory_id,omitempty"`
	SourcePlatform   string                 `json:"source_platform,omitempty"`
	SourceExternalID string                 `json:"source_external_id,omitempty"`
	NodeID           string                 `json:"node_id"`
	NodeKind         string                 `json:"node_kind"`
	State            PosteriorState         `json:"state"`
	DiagnosisCode    PosteriorDiagnosisCode `json:"diagnosis_code,omitempty"`
	Reason           string                 `json:"reason,omitempty"`
	BlockedByNodeIDs []string               `json:"blocked_by_node_ids,omitempty"`
	LastEvaluatedAt  time.Time              `json:"last_evaluated_at,omitempty"`
	LastEvidenceAt   time.Time              `json:"last_evidence_at,omitempty"`
	UpdatedAt        time.Time              `json:"updated_at"`
}

type PosteriorRunRequest struct {
	UserID           string `json:"user_id"`
	SourcePlatform   string `json:"source_platform,omitempty"`
	SourceExternalID string `json:"source_external_id,omitempty"`
}

type PosteriorRefreshTrigger struct {
	EventID           int64     `json:"event_id"`
	JobID             int64     `json:"job_id"`
	UserID            string    `json:"user_id"`
	SourcePlatform    string    `json:"source_platform"`
	SourceExternalID  string    `json:"source_external_id"`
	RootExternalID    string    `json:"root_external_id,omitempty"`
	SourceModel       string    `json:"source_model,omitempty"`
	SourceCompiledAt  time.Time `json:"source_compiled_at,omitempty"`
	AffectedMemoryIDs []int64   `json:"affected_memory_ids,omitempty"`
	AffectedNodeIDs   []string  `json:"affected_node_ids,omitempty"`
	Reason            string    `json:"reason,omitempty"`
	CreatedAt         time.Time `json:"created_at"`
}

type PosteriorRunResult struct {
	RanAt     time.Time                 `json:"ran_at"`
	Evaluated []PosteriorStateRecord    `json:"evaluated,omitempty"`
	Mutated   []PosteriorStateRecord    `json:"mutated,omitempty"`
	Refreshes []PosteriorRefreshTrigger `json:"refreshes,omitempty"`
}
