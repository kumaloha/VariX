package model

// Ledger is the preserved compile fact inventory. Reader-facing views such as
// Brief and Mainline may omit items, but should keep source IDs back to it.
type Ledger struct {
	Items []LedgerItem `json:"items,omitempty"`
}

type LedgerItem struct {
	ID        string   `json:"id,omitempty"`
	Kind      string   `json:"kind,omitempty"`
	Category  string   `json:"category,omitempty"`
	Claim     string   `json:"claim,omitempty"`
	Entities  []string `json:"entities,omitempty"`
	Numbers   []string `json:"numbers,omitempty"`
	Quote     string   `json:"quote,omitempty"`
	SourceIDs []string `json:"sourceIds,omitempty"`
	Salience  float64  `json:"salience,omitempty"`
}
