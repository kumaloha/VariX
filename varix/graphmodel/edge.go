package graphmodel

import (
	"fmt"
)

type EdgeType string

const (
	EdgeTypeDrives   EdgeType = "drives"
	EdgeTypeSupports EdgeType = "supports"
	EdgeTypeExplains EdgeType = "explains"
	EdgeTypeContext  EdgeType = "context"
)

type GraphEdge struct {
	ID                 string             `json:"id"`
	From               string             `json:"from"`
	To                 string             `json:"to"`
	Type               EdgeType           `json:"type"`
	IsPrimary          bool               `json:"is_primary"`
	Confidence         *float64           `json:"confidence,omitempty"`
	VerificationStatus VerificationStatus `json:"verification_status,omitempty"`
	VerificationReason string             `json:"verification_reason,omitempty"`
	VerificationAsOf   string             `json:"verification_as_of,omitempty"`
	NextVerifyAt       string             `json:"next_verify_at,omitempty"`
	LastVerifiedAt     string             `json:"last_verified_at,omitempty"`
}

func (e GraphEdge) Validate(nodeIDs map[string]struct{}) error {
	if err := requireTrimmed("graph edge id", e.ID); err != nil {
		return err
	}
	if err := requireTrimmed("graph edge source", e.From); err != nil {
		return fmt.Errorf("graph edge endpoints are required")
	}
	if err := requireTrimmed("graph edge target", e.To); err != nil {
		return fmt.Errorf("graph edge endpoints are required")
	}
	if e.From == e.To {
		return fmt.Errorf("graph edge must not self-reference")
	}
	if _, ok := nodeIDs[e.From]; !ok {
		return fmt.Errorf("graph edge source %q is unknown", e.From)
	}
	if _, ok := nodeIDs[e.To]; !ok {
		return fmt.Errorf("graph edge target %q is unknown", e.To)
	}
	switch e.Type {
	case EdgeTypeDrives, EdgeTypeSupports, EdgeTypeExplains, EdgeTypeContext:
	default:
		return fmt.Errorf("graph edge type %q is unsupported", e.Type)
	}
	if e.VerificationStatus != "" {
		switch e.VerificationStatus {
		case VerificationPending, VerificationProved, VerificationDisproved, VerificationUnverifiable:
		default:
			return fmt.Errorf("graph edge verification_status %q is unsupported", e.VerificationStatus)
		}
	}
	if err := validateOptionalRFC3339("verification_as_of", e.VerificationAsOf); err != nil {
		return err
	}
	if err := validateOptionalRFC3339("next_verify_at", e.NextVerifyAt); err != nil {
		return err
	}
	if err := validateOptionalRFC3339("last_verified_at", e.LastVerifiedAt); err != nil {
		return err
	}
	return nil
}
