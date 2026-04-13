package types

import "time"

type ProcessedRecord struct {
	Platform    string    `json:"platform"`
	ExternalID  string    `json:"external_id"`
	URL         string    `json:"url,omitempty"`
	Author      string    `json:"author,omitempty"`
	ProcessedAt time.Time `json:"processed_at"`
}
