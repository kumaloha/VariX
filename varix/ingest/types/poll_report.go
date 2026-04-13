package types

import "time"

type PollReport struct {
	StartedAt         time.Time          `json:"started_at"`
	FinishedAt        time.Time          `json:"finished_at"`
	TargetCount       int                `json:"target_count"`
	DiscoveredCount   int                `json:"discovered_count"`
	FetchedCount      int                `json:"fetched_count"`
	SkippedCount      int                `json:"skipped_count"`
	StoreWarningCount int                `json:"store_warning_count"`
	PollWarningCount  int                `json:"poll_warning_count"`
	Targets           []TargetPollReport `json:"targets,omitempty"`
}

type TargetPollReport struct {
	Target          string `json:"target"`
	DiscoveredCount int    `json:"discovered_count"`
	FetchedCount    int    `json:"fetched_count"`
	SkippedCount    int    `json:"skipped_count"`
	WarningCount    int    `json:"warning_count"`
	Status          string `json:"status"`
	ErrorDetail     string `json:"error_detail,omitempty"`
}
