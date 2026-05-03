package model

// SemanticUnit captures an important speaker-centered claim that may not be a
// causal graph edge, such as an answer, boundary, operating update, or rule.
type SemanticUnit struct {
	ID               string  `json:"id"`
	Span             string  `json:"span,omitempty"`
	Speaker          string  `json:"speaker,omitempty"`
	SpeakerRole      string  `json:"speaker_role,omitempty"`
	Subject          string  `json:"subject"`
	Force            string  `json:"force,omitempty"`
	Claim            string  `json:"claim"`
	PromptContext    string  `json:"prompt_context,omitempty"`
	ImportanceReason string  `json:"importance_reason,omitempty"`
	SourceQuote      string  `json:"source_quote,omitempty"`
	Salience         float64 `json:"salience,omitempty"`
	Confidence       string  `json:"confidence,omitempty"`
}
