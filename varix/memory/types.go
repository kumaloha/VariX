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
