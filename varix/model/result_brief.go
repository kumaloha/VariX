package model

// BriefItem is a reader-facing meeting digest item derived from the fuller
// salience inventory. It preserves the compact facts a skim reader expects.
type BriefItem struct {
	ID        string   `json:"id,omitempty"`
	Category  string   `json:"category"`
	Kind      string   `json:"kind,omitempty"`
	Claim     string   `json:"claim"`
	Entities  []string `json:"entities,omitempty"`
	Numbers   []string `json:"numbers,omitempty"`
	Quote     string   `json:"quote,omitempty"`
	Salience  float64  `json:"salience,omitempty"`
	SourceIDs []string `json:"sourceIds,omitempty"`
}
