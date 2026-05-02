package model

type VerifyQueueObjectType string

type VerifyQueueStatus string

const (
	VerifyQueueObjectNode VerifyQueueObjectType = "node"
	VerifyQueueObjectEdge VerifyQueueObjectType = "edge"

	VerifyQueueStatusQueued  VerifyQueueStatus = "queued"
	VerifyQueueStatusRunning VerifyQueueStatus = "running"
	VerifyQueueStatusDone    VerifyQueueStatus = "done"
	VerifyQueueStatusRetry   VerifyQueueStatus = "retry"
)

type VerifyQueueItem struct {
	ID              string                `json:"id"`
	ObjectType      VerifyQueueObjectType `json:"object_type"`
	ObjectID        string                `json:"object_id"`
	SourceArticleID string                `json:"source_article_id"`
	Priority        int                   `json:"priority"`
	ScheduledAt     string                `json:"scheduled_at"`
	Attempts        int                   `json:"attempts"`
	LastError       string                `json:"last_error,omitempty"`
	Status          VerifyQueueStatus     `json:"status"`
}

type VerifyVerdict struct {
	ObjectType   VerifyQueueObjectType `json:"object_type"`
	ObjectID     string                `json:"object_id"`
	Verdict      VerificationStatus    `json:"verdict"`
	Reason       string                `json:"reason,omitempty"`
	EvidenceRefs []string              `json:"evidence_refs,omitempty"`
	AsOf         string                `json:"as_of"`
	NextVerifyAt string                `json:"next_verify_at,omitempty"`
}
