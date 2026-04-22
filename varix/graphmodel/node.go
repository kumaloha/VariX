package graphmodel

import (
	"fmt"
	"time"
)

type NodeKind string

const (
	NodeKindObservation NodeKind = "observation"
	NodeKindPrediction  NodeKind = "prediction"
)

type VerificationStatus string

const (
	VerificationPending      VerificationStatus = "pending"
	VerificationProved       VerificationStatus = "proved"
	VerificationDisproved    VerificationStatus = "disproved"
	VerificationUnverifiable VerificationStatus = "unverifiable"
)

type GraphRole string

const (
	GraphRoleDriver       GraphRole = "driver"
	GraphRoleTarget       GraphRole = "target"
	GraphRoleIntermediate GraphRole = "intermediate"
	GraphRoleEvidence     GraphRole = "evidence"
	GraphRoleContext      GraphRole = "context"
)

type VerificationTarget struct {
	Metric                string   `json:"metric,omitempty"`
	Subject               string   `json:"subject,omitempty"`
	Comparator            string   `json:"comparator,omitempty"`
	ExpectedValueText     string   `json:"expected_value_text,omitempty"`
	ExpectedValueNum      *float64 `json:"expected_value_num,omitempty"`
	ExpectedUnit          string   `json:"expected_unit,omitempty"`
	EvaluationWindowStart string   `json:"evaluation_window_start,omitempty"`
	EvaluationWindowEnd   string   `json:"evaluation_window_end,omitempty"`
}

type GraphNode struct {
	ID                 string              `json:"id"`
	SourceArticleID    string              `json:"source_article_id"`
	SourcePlatform     string              `json:"source_platform"`
	SourceExternalID   string              `json:"source_external_id"`
	SourceQuote        string              `json:"source_quote,omitempty"`
	SourceTextSpan     string              `json:"source_text_span,omitempty"`
	RawText            string              `json:"raw_text,omitempty"`
	SubjectText        string              `json:"subject_text"`
	SubjectCanonical   string              `json:"subject_canonical,omitempty"`
	ChangeText         string              `json:"change_text"`
	ChangeKind         string              `json:"change_kind,omitempty"`
	ChangeDirection    string              `json:"change_direction,omitempty"`
	ChangeValue        *float64            `json:"change_value,omitempty"`
	ChangeUnit         string              `json:"change_unit,omitempty"`
	TimeText           string              `json:"time_text,omitempty"`
	TimeStart          string              `json:"time_start,omitempty"`
	TimeEnd            string              `json:"time_end,omitempty"`
	TimeBucket         string              `json:"time_bucket,omitempty"`
	Kind               NodeKind            `json:"kind"`
	GraphRole          GraphRole           `json:"graph_role,omitempty"`
	IsPrimary          bool                `json:"is_primary"`
	VerificationStatus VerificationStatus  `json:"verification_status"`
	VerificationReason string              `json:"verification_reason,omitempty"`
	VerificationAsOf   string              `json:"verification_as_of,omitempty"`
	NextVerifyAt       string              `json:"next_verify_at,omitempty"`
	LastVerifiedAt     string              `json:"last_verified_at,omitempty"`
	VerificationTarget *VerificationTarget `json:"verification_target,omitempty"`
	CompileConfidence  *float64            `json:"compile_confidence,omitempty"`
	VerifyConfidence   *float64            `json:"verify_confidence,omitempty"`
}

func (n GraphNode) Validate() error {
	if err := requireTrimmed("graph node id", n.ID); err != nil {
		return err
	}
	if err := requireTrimmed("graph node source_article_id", n.SourceArticleID); err != nil {
		return err
	}
	if err := requireTrimmed("graph node source_platform", n.SourcePlatform); err != nil {
		return err
	}
	if err := requireTrimmed("graph node source_external_id", n.SourceExternalID); err != nil {
		return err
	}
	if err := requireTrimmed("graph node subject_text", n.SubjectText); err != nil {
		return err
	}
	if err := requireTrimmed("graph node change_text", n.ChangeText); err != nil {
		return err
	}
	if err := requireTrimmed("graph node raw_text", n.RawText); err != nil {
		return err
	}
	switch n.Kind {
	case NodeKindObservation, NodeKindPrediction:
	default:
		return fmt.Errorf("graph node kind %q is unsupported", n.Kind)
	}
	if err := validateVerificationStatus("graph node", n.VerificationStatus); err != nil {
		return err
	}
	if err := validateOptionalRFC3339("time_start", n.TimeStart); err != nil {
		return err
	}
	if err := validateOptionalRFC3339("time_end", n.TimeEnd); err != nil {
		return err
	}
	if err := validateOptionalRFC3339("verification_as_of", n.VerificationAsOf); err != nil {
		return err
	}
	if err := validateOptionalRFC3339("next_verify_at", n.NextVerifyAt); err != nil {
		return err
	}
	if err := validateOptionalRFC3339("last_verified_at", n.LastVerifiedAt); err != nil {
		return err
	}
	if n.TimeStart != "" && n.TimeEnd != "" {
		start, _ := time.Parse(time.RFC3339, n.TimeStart)
		end, _ := time.Parse(time.RFC3339, n.TimeEnd)
		if end.Before(start) {
			return fmt.Errorf("graph node time window is inverted")
		}
	}
	if n.VerificationTarget != nil {
		if err := validateOptionalRFC3339("verification_target.evaluation_window_start", n.VerificationTarget.EvaluationWindowStart); err != nil {
			return err
		}
		if err := validateOptionalRFC3339("verification_target.evaluation_window_end", n.VerificationTarget.EvaluationWindowEnd); err != nil {
			return err
		}
	}
	return nil
}
