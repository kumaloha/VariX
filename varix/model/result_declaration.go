package model

// Declaration captures an explicit speech act from an authorized speaker.
// It is parallel to causal transmission paths: declarations answer "what did
// management say it will do, under which conditions, and with what boundary?"
type Declaration struct {
	ID          string   `json:"id,omitempty"`
	Speaker     string   `json:"speaker,omitempty"`
	Kind        string   `json:"kind,omitempty"`
	Topic       string   `json:"topic,omitempty"`
	Statement   string   `json:"statement"`
	Conditions  []string `json:"conditions,omitempty"`
	Actions     []string `json:"actions,omitempty"`
	Scale       string   `json:"scale,omitempty"`
	Constraints []string `json:"constraints,omitempty"`
	NonActions  []string `json:"non_actions,omitempty"`
	Evidence    []string `json:"evidence,omitempty"`
	SourceQuote string   `json:"source_quote,omitempty"`
	Confidence  string   `json:"confidence,omitempty"`
}
