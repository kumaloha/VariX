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
	AcceptedAt       time.Time `json:"accepted_at"`
}

type AcceptanceNodeSnapshot struct {
	NodeID   string `json:"node_id"`
	NodeKind string `json:"node_kind"`
	NodeText string `json:"node_text"`
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
