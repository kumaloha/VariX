package model

import "time"

type VerificationRecord struct {
	UnitID         string       `json:"unit_id"`
	Source         string       `json:"source"`
	ExternalID     string       `json:"external_id"`
	RootExternalID string       `json:"root_external_id,omitempty"`
	Model          string       `json:"model"`
	Verification   Verification `json:"verification"`
	VerifiedAt     time.Time    `json:"verified_at"`
}
