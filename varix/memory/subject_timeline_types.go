package memory

import "time"

type SubjectChangeStructure string

const (
	SubjectChangeStructured   SubjectChangeStructure = "structured"
	SubjectChangeLowStructure SubjectChangeStructure = "low_structure"
)

type SubjectChangeRelation string

const (
	SubjectChangeNew                  SubjectChangeRelation = "new"
	SubjectChangeReinforces           SubjectChangeRelation = "reinforces"
	SubjectChangeUpdates              SubjectChangeRelation = "updates"
	SubjectChangeContradicts          SubjectChangeRelation = "contradicts"
	SubjectChangeBranches             SubjectChangeRelation = "branches"
	SubjectChangeRelationLowStructure SubjectChangeRelation = "low_structure"
)

type SubjectTimeline struct {
	UserID           string               `json:"user_id"`
	Subject          string               `json:"subject"`
	CanonicalSubject string               `json:"canonical_subject,omitempty"`
	GeneratedAt      time.Time            `json:"generated_at"`
	Entries          []SubjectChangeEntry `json:"entries"`
	Summary          string               `json:"summary,omitempty"`
}

type SubjectChangeEntry struct {
	SourcePlatform     string                 `json:"source_platform"`
	SourceExternalID   string                 `json:"source_external_id"`
	SourceArticleID    string                 `json:"source_article_id,omitempty"`
	SourceCompiledAt   string                 `json:"source_compiled_at,omitempty"`
	SourceUpdatedAt    string                 `json:"source_updated_at,omitempty"`
	NodeID             string                 `json:"node_id"`
	RawText            string                 `json:"raw_text,omitempty"`
	SubjectText        string                 `json:"subject_text"`
	SubjectCanonical   string                 `json:"subject_canonical,omitempty"`
	ChangeText         string                 `json:"change_text"`
	ChangeKind         string                 `json:"change_kind,omitempty"`
	ChangeDirection    string                 `json:"change_direction,omitempty"`
	ChangeValue        *float64               `json:"change_value,omitempty"`
	ChangeUnit         string                 `json:"change_unit,omitempty"`
	TimeText           string                 `json:"time_text,omitempty"`
	TimeStart          string                 `json:"time_start,omitempty"`
	TimeEnd            string                 `json:"time_end,omitempty"`
	TimeBucket         string                 `json:"time_bucket,omitempty"`
	GraphRole          string                 `json:"graph_role,omitempty"`
	IsPrimary          bool                   `json:"is_primary,omitempty"`
	VerificationStatus string                 `json:"verification_status,omitempty"`
	VerificationReason string                 `json:"verification_reason,omitempty"`
	VerificationAsOf   string                 `json:"verification_as_of,omitempty"`
	Structure          SubjectChangeStructure `json:"structure"`
	RelationToPrior    SubjectChangeRelation  `json:"relation_to_prior"`
}
