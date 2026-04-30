package contentstore

import (
	"context"
	"database/sql"
)

const (
	ProjectionDirtyPending        = "pending"
	projectionDirtySubjectWorkers = 4
)

type ProjectionDirtyMark struct {
	ID        int64  `json:"dirty_id,omitempty"`
	UserID    string `json:"user_id"`
	Layer     string `json:"layer"`
	Subject   string `json:"subject,omitempty"`
	Ticker    string `json:"ticker,omitempty"`
	Horizon   string `json:"horizon,omitempty"`
	Reason    string `json:"reason,omitempty"`
	SourceRef string `json:"source_ref,omitempty"`
	Status    string `json:"status"`
	DirtyAt   string `json:"dirty_at"`
	UpdatedAt string `json:"updated_at,omitempty"`
}

type ProjectionDirtySweepResult struct {
	UserID    string                      `json:"user_id,omitempty"`
	Limit     int                         `json:"limit"`
	Workers   int                         `json:"workers"`
	Scanned   int                         `json:"scanned"`
	Completed int                         `json:"completed"`
	Failed    int                         `json:"failed"`
	Remaining int                         `json:"remaining"`
	Layers    map[string]int              `json:"layers,omitempty"`
	Errors    []ProjectionDirtySweepError `json:"errors,omitempty"`
}

type ProjectionDirtySweepError struct {
	DirtyID int64  `json:"dirty_id,omitempty"`
	Layer   string `json:"layer"`
	Subject string `json:"subject,omitempty"`
	Horizon string `json:"horizon,omitempty"`
	Error   string `json:"error"`
}

type projectionDirtyExecer interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
}

type projectionDirtyMarkRunner func(context.Context, ProjectionDirtyMark, *projectionDirtyUserState) error

type projectionDirtyMarkClearer func(context.Context, []ProjectionDirtyMark) error
