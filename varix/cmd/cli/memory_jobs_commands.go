package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/kumaloha/VariX/varix/memory"
)

func runMemoryJobs(args []string, projectRoot string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("memory jobs", flag.ContinueOnError)
	fs.SetOutput(stderr)
	userID := fs.String("user", "", "user id")
	status := fs.String("status", "", "optional filter: queued, running, done")
	summary := fs.Bool("summary", false, "print status counts instead of full jobs")
	platform := fs.String("platform", "", "optional source platform filter")
	externalID := fs.String("id", "", "optional source external id filter (requires --platform)")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if invalidScopedMemorySourceRequest(*userID, *platform, *externalID) {
		fmt.Fprintln(stderr, "usage: varix memory jobs --user <user_id>")
		return 2
	}
	store, err := openStore(projectRoot)
	if err != nil {
		writeErr(stderr, err)
		return 1
	}
	defer store.Close()
	jobs, err := store.ListMemoryJobs(context.Background(), strings.TrimSpace(*userID))
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	if strings.TrimSpace(*platform) != "" {
		filtered := make([]memory.OrganizationJob, 0, len(jobs))
		for _, job := range jobs {
			if job.SourcePlatform != strings.TrimSpace(*platform) {
				continue
			}
			if strings.TrimSpace(*externalID) != "" && job.SourceExternalID != strings.TrimSpace(*externalID) {
				continue
			}
			filtered = append(filtered, job)
		}
		jobs = filtered
	}
	if strings.TrimSpace(*status) != "" {
		filtered := make([]memory.OrganizationJob, 0, len(jobs))
		for _, job := range jobs {
			if job.Status == strings.TrimSpace(*status) {
				filtered = append(filtered, job)
			}
		}
		jobs = filtered
	}
	if *summary {
		counts := map[string]int{}
		now := currentUTC()
		var oldestQueuedAt time.Time
		var oldestRunningAt time.Time
		for _, job := range jobs {
			counts[job.Status]++
			switch job.Status {
			case "queued":
				if oldestQueuedAt.IsZero() || (!job.CreatedAt.IsZero() && job.CreatedAt.Before(oldestQueuedAt)) {
					oldestQueuedAt = job.CreatedAt
				}
			case "running":
				staleAt := job.CreatedAt
				if !job.StartedAt.IsZero() {
					staleAt = job.StartedAt
				}
				if oldestRunningAt.IsZero() || (!staleAt.IsZero() && staleAt.Before(oldestRunningAt)) {
					oldestRunningAt = staleAt
				}
			}
			switch job.Status {
			case "queued":
				if !job.CreatedAt.IsZero() && now.Sub(job.CreatedAt) > 24*time.Hour {
					counts["stale_candidates"]++
					counts["stale_queued"]++
				}
			case "running":
				staleAt := job.CreatedAt
				if !job.StartedAt.IsZero() {
					staleAt = job.StartedAt
				}
				if !staleAt.IsZero() && now.Sub(staleAt) > 24*time.Hour {
					counts["stale_candidates"]++
					counts["stale_running"]++
				}
			}
		}
		payload, err := json.MarshalIndent(map[string]any{
			"user":              strings.TrimSpace(*userID),
			"counts":            counts,
			"oldest_queued_at":  formatMaybeRFC3339(oldestQueuedAt),
			"oldest_running_at": formatMaybeRFC3339(oldestRunningAt),
		}, "", "  ")
		if err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		fmt.Fprintln(stdout, string(payload))
		return 0
	}
	payload, err := json.MarshalIndent(jobs, "", "  ")
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	fmt.Fprintln(stdout, string(payload))
	return 0
}

func formatMaybeRFC3339(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return value.UTC().Format(time.RFC3339)
}

func runMemoryCleanupStale(args []string, projectRoot string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("memory cleanup-stale", flag.ContinueOnError)
	fs.SetOutput(stderr)
	userID := fs.String("user", "", "user id")
	olderThan := fs.Duration("older-than", 24*time.Hour, "delete queued/running jobs older than this duration")
	platform := fs.String("platform", "", "optional source platform filter")
	externalID := fs.String("id", "", "optional source external id filter (requires --platform)")
	dryRun := fs.Bool("dry-run", false, "report stale jobs without deleting them")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if invalidScopedMemorySourceRequest(*userID, *platform, *externalID) {
		fmt.Fprintln(stderr, "usage: varix memory cleanup-stale --user <user_id> [--older-than 24h] [--platform <platform> --id <external_id>]")
		return 2
	}
	if *olderThan <= 0 {
		fmt.Fprintln(stderr, "--older-than must be positive")
		return 2
	}
	store, err := openStore(projectRoot)
	if err != nil {
		writeErr(stderr, err)
		return 1
	}
	defer store.Close()
	cutoff := currentUTC().Add(-*olderThan)
	var deleted int64
	if *dryRun {
		deleted, err = store.CountStaleMemoryJobs(context.Background(), strings.TrimSpace(*userID), strings.TrimSpace(*platform), strings.TrimSpace(*externalID), cutoff)
		if err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
	} else {
		deleted, err = store.CleanupStaleMemoryJobs(context.Background(), strings.TrimSpace(*userID), strings.TrimSpace(*platform), strings.TrimSpace(*externalID), cutoff)
		if err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
	}
	payload, err := json.MarshalIndent(map[string]any{
		"ok":            true,
		"user":          strings.TrimSpace(*userID),
		"deleted_jobs":  deleted,
		"dry_run":       *dryRun,
		"cutoff_before": cutoff.Format(time.RFC3339),
	}, "", "  ")
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	fmt.Fprintln(stdout, string(payload))
	return 0
}
