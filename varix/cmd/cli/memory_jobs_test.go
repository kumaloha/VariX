package main

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/kumaloha/VariX/varix/bootstrap"
	"github.com/kumaloha/VariX/varix/memory"
	"github.com/kumaloha/VariX/varix/storage/contentstore"
)

func TestRunMemoryJobsSupportsStatusFilter(t *testing.T) {
	prevBuildApp := buildApp
	prevOpenSQLiteStore := openSQLiteStore
	t.Cleanup(func() {
		buildApp = prevBuildApp
		openSQLiteStore = prevOpenSQLiteStore
	})

	tmp := t.TempDir()
	buildApp = func(projectRoot string) (*bootstrap.App, error) {
		app := &bootstrap.App{}
		app.Settings.ContentDBPath = tmp + "/content.db"
		return app, nil
	}
	openSQLiteStore = func(path string) (*contentstore.SQLiteStore, error) {
		store, err := contentstore.NewSQLiteStore(path)
		if err != nil {
			return nil, err
		}
		rawDB, err := sql.Open("sqlite", path)
		if err != nil {
			return nil, err
		}
		defer rawDB.Close()
		now := time.Now().UTC()
		for _, stmt := range []string{
			fmt.Sprintf(`INSERT INTO memory_organization_jobs(trigger_event_id, user_id, source_platform, source_external_id, status, created_at) VALUES (1, 'u-jobs-filter', 'twitter', 'queued-job', 'queued', '%s')`, now.Format(time.RFC3339Nano)),
			fmt.Sprintf(`INSERT INTO memory_organization_jobs(trigger_event_id, user_id, source_platform, source_external_id, status, created_at, started_at) VALUES (2, 'u-jobs-filter', 'twitter', 'running-job', 'running', '%s', '%s')`, now.Format(time.RFC3339Nano), now.Format(time.RFC3339Nano)),
			fmt.Sprintf(`INSERT INTO memory_organization_jobs(trigger_event_id, user_id, source_platform, source_external_id, status, created_at, finished_at) VALUES (3, 'u-jobs-filter', 'twitter', 'done-job', 'done', '%s', '%s')`, now.Format(time.RFC3339Nano), now.Format(time.RFC3339Nano)),
		} {
			if _, err := rawDB.Exec(stmt); err != nil {
				return nil, err
			}
		}
		return store, nil
	}
	var stdout, stderr bytes.Buffer
	code := run([]string{"memory", "jobs", "--user", "u-jobs-filter", "--status", "running"}, "/tmp/project", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("memory jobs --status code = %d, stderr = %s", code, stderr.String())
	}
	var out []memory.OrganizationJob
	if err := json.Unmarshal(stdout.Bytes(), &out); err != nil {
		t.Fatalf("json.Unmarshal(memory jobs --status) error = %v", err)
	}
	if len(out) != 1 || out[0].Status != "running" {
		t.Fatalf("out = %#v, want only running job", out)
	}
}

func TestRunMemoryJobsSupportsPlatformFilter(t *testing.T) {
	prevBuildApp := buildApp
	prevOpenSQLiteStore := openSQLiteStore
	t.Cleanup(func() {
		buildApp = prevBuildApp
		openSQLiteStore = prevOpenSQLiteStore
	})

	tmp := t.TempDir()
	buildApp = func(projectRoot string) (*bootstrap.App, error) {
		app := &bootstrap.App{}
		app.Settings.ContentDBPath = tmp + "/content.db"
		return app, nil
	}
	openSQLiteStore = func(path string) (*contentstore.SQLiteStore, error) {
		store, err := contentstore.NewSQLiteStore(path)
		if err != nil {
			return nil, err
		}
		rawDB, err := sql.Open("sqlite", path)
		if err != nil {
			return nil, err
		}
		defer rawDB.Close()
		now := time.Now().UTC()
		for _, stmt := range []string{
			fmt.Sprintf(`INSERT INTO memory_organization_jobs(trigger_event_id, user_id, source_platform, source_external_id, status, created_at) VALUES (1, 'u-jobs-platform', 'twitter', 'twitter-job', 'queued', '%s')`, now.Format(time.RFC3339Nano)),
			fmt.Sprintf(`INSERT INTO memory_organization_jobs(trigger_event_id, user_id, source_platform, source_external_id, status, created_at) VALUES (2, 'u-jobs-platform', 'weibo', 'weibo-job', 'queued', '%s')`, now.Format(time.RFC3339Nano)),
		} {
			if _, err := rawDB.Exec(stmt); err != nil {
				return nil, err
			}
		}
		return store, nil
	}
	var stdout, stderr bytes.Buffer
	code := run([]string{"memory", "jobs", "--user", "u-jobs-platform", "--platform", "twitter"}, "/tmp/project", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("memory jobs --platform code = %d, stderr = %s", code, stderr.String())
	}
	var out []memory.OrganizationJob
	if err := json.Unmarshal(stdout.Bytes(), &out); err != nil {
		t.Fatalf("json.Unmarshal(memory jobs --platform) error = %v", err)
	}
	if len(out) != 1 || out[0].SourcePlatform != "twitter" {
		t.Fatalf("out = %#v, want only twitter job", out)
	}
}

func TestRunMemoryJobsSupportsPlatformAndIDFilter(t *testing.T) {
	prevBuildApp := buildApp
	prevOpenSQLiteStore := openSQLiteStore
	t.Cleanup(func() {
		buildApp = prevBuildApp
		openSQLiteStore = prevOpenSQLiteStore
	})

	tmp := t.TempDir()
	buildApp = func(projectRoot string) (*bootstrap.App, error) {
		app := &bootstrap.App{}
		app.Settings.ContentDBPath = tmp + "/content.db"
		return app, nil
	}
	openSQLiteStore = func(path string) (*contentstore.SQLiteStore, error) {
		store, err := contentstore.NewSQLiteStore(path)
		if err != nil {
			return nil, err
		}
		rawDB, err := sql.Open("sqlite", path)
		if err != nil {
			return nil, err
		}
		defer rawDB.Close()
		now := time.Now().UTC()
		for _, stmt := range []string{
			fmt.Sprintf(`INSERT INTO memory_organization_jobs(trigger_event_id, user_id, source_platform, source_external_id, status, created_at) VALUES (1, 'u-jobs-source', 'twitter', 'keep-me', 'queued', '%s')`, now.Format(time.RFC3339Nano)),
			fmt.Sprintf(`INSERT INTO memory_organization_jobs(trigger_event_id, user_id, source_platform, source_external_id, status, created_at) VALUES (2, 'u-jobs-source', 'twitter', 'drop-me', 'queued', '%s')`, now.Format(time.RFC3339Nano)),
		} {
			if _, err := rawDB.Exec(stmt); err != nil {
				return nil, err
			}
		}
		return store, nil
	}
	var stdout, stderr bytes.Buffer
	code := run([]string{"memory", "jobs", "--user", "u-jobs-source", "--platform", "twitter", "--id", "keep-me"}, "/tmp/project", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("memory jobs --platform --id code = %d, stderr = %s", code, stderr.String())
	}
	var out []memory.OrganizationJob
	if err := json.Unmarshal(stdout.Bytes(), &out); err != nil {
		t.Fatalf("json.Unmarshal(memory jobs --platform --id) error = %v", err)
	}
	if len(out) != 1 || out[0].SourceExternalID != "keep-me" {
		t.Fatalf("out = %#v, want only keep-me job", out)
	}
}

func TestRunMemoryJobsDefaultStillReturnsFullJobList(t *testing.T) {
	prevBuildApp := buildApp
	prevOpenSQLiteStore := openSQLiteStore
	t.Cleanup(func() {
		buildApp = prevBuildApp
		openSQLiteStore = prevOpenSQLiteStore
	})

	tmp := t.TempDir()
	buildApp = func(projectRoot string) (*bootstrap.App, error) {
		app := &bootstrap.App{}
		app.Settings.ContentDBPath = tmp + "/content.db"
		return app, nil
	}
	openSQLiteStore = func(path string) (*contentstore.SQLiteStore, error) {
		store, err := contentstore.NewSQLiteStore(path)
		if err != nil {
			return nil, err
		}
		rawDB, err := sql.Open("sqlite", path)
		if err != nil {
			return nil, err
		}
		defer rawDB.Close()
		now := time.Now().UTC()
		for _, stmt := range []string{
			fmt.Sprintf(`INSERT INTO memory_organization_jobs(trigger_event_id, user_id, source_platform, source_external_id, status, created_at) VALUES (1, 'u-jobs-default', 'twitter', 'queued-job', 'queued', '%s')`, now.Format(time.RFC3339Nano)),
			fmt.Sprintf(`INSERT INTO memory_organization_jobs(trigger_event_id, user_id, source_platform, source_external_id, status, created_at, finished_at) VALUES (2, 'u-jobs-default', 'twitter', 'done-job', 'done', '%s', '%s')`, now.Format(time.RFC3339Nano), now.Format(time.RFC3339Nano)),
		} {
			if _, err := rawDB.Exec(stmt); err != nil {
				return nil, err
			}
		}
		return store, nil
	}
	var stdout, stderr bytes.Buffer
	code := run([]string{"memory", "jobs", "--user", "u-jobs-default"}, "/tmp/project", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("memory jobs default code = %d, stderr = %s", code, stderr.String())
	}
	var out []memory.OrganizationJob
	if err := json.Unmarshal(stdout.Bytes(), &out); err != nil {
		t.Fatalf("json.Unmarshal(memory jobs default) error = %v", err)
	}
	if len(out) != 2 {
		t.Fatalf("out = %#v, want full unfiltered job list", out)
	}
}

func TestRunMemoryJobsSupportsSummary(t *testing.T) {
	prevBuildApp := buildApp
	prevOpenSQLiteStore := openSQLiteStore
	t.Cleanup(func() {
		buildApp = prevBuildApp
		openSQLiteStore = prevOpenSQLiteStore
	})

	tmp := t.TempDir()
	buildApp = func(projectRoot string) (*bootstrap.App, error) {
		app := &bootstrap.App{}
		app.Settings.ContentDBPath = tmp + "/content.db"
		return app, nil
	}
	openSQLiteStore = func(path string) (*contentstore.SQLiteStore, error) {
		store, err := contentstore.NewSQLiteStore(path)
		if err != nil {
			return nil, err
		}
		rawDB, err := sql.Open("sqlite", path)
		if err != nil {
			return nil, err
		}
		defer rawDB.Close()
		now := time.Now().UTC()
		for _, stmt := range []string{
			fmt.Sprintf(`INSERT INTO memory_organization_jobs(trigger_event_id, user_id, source_platform, source_external_id, status, created_at) VALUES (1, 'u-jobs-summary', 'twitter', 'queued-job', 'queued', '%s')`, now.Add(-48*time.Hour).Format(time.RFC3339Nano)),
			fmt.Sprintf(`INSERT INTO memory_organization_jobs(trigger_event_id, user_id, source_platform, source_external_id, status, created_at, started_at) VALUES (2, 'u-jobs-summary', 'twitter', 'running-job', 'running', '%s', '%s')`, now.Add(-48*time.Hour).Format(time.RFC3339Nano), now.Add(-2*time.Hour).Format(time.RFC3339Nano)),
			fmt.Sprintf(`INSERT INTO memory_organization_jobs(trigger_event_id, user_id, source_platform, source_external_id, status, created_at, finished_at) VALUES (3, 'u-jobs-summary', 'twitter', 'done-job', 'done', '%s', '%s')`, now.Format(time.RFC3339Nano), now.Format(time.RFC3339Nano)),
		} {
			if _, err := rawDB.Exec(stmt); err != nil {
				return nil, err
			}
		}
		return store, nil
	}
	var stdout, stderr bytes.Buffer
	code := run([]string{"memory", "jobs", "--user", "u-jobs-summary", "--summary"}, "/tmp/project", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("memory jobs --summary code = %d, stderr = %s", code, stderr.String())
	}
	var out struct {
		User            string         `json:"user"`
		Counts          map[string]int `json:"counts"`
		OldestQueuedAt  string         `json:"oldest_queued_at"`
		OldestRunningAt string         `json:"oldest_running_at"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &out); err != nil {
		t.Fatalf("json.Unmarshal(memory jobs --summary) error = %v", err)
	}
	if out.User != "u-jobs-summary" {
		t.Fatalf("summary user = %q, want u-jobs-summary", out.User)
	}
	if out.Counts["queued"] != 1 || out.Counts["running"] != 1 || out.Counts["done"] != 1 {
		t.Fatalf("summary counts = %#v, want queued/running/done = 1/1/1", out.Counts)
	}
	if out.Counts["stale_candidates"] == 0 {
		t.Fatalf("summary counts = %#v, want stale_candidates visibility", out.Counts)
	}
	if out.Counts["stale_queued"] != 1 || out.Counts["stale_running"] != 0 {
		t.Fatalf("summary counts = %#v, want stale_queued=1 and stale_running=0", out.Counts)
	}
	if out.OldestQueuedAt == "" {
		t.Fatalf("summary output = %#v, want oldest_queued_at string", out)
	}
	if out.OldestRunningAt == "" {
		t.Fatalf("summary output = %#v, want oldest_running_at string", out)
	}
}

func TestRunMemoryJobsSummaryComposesWithStatusFilter(t *testing.T) {
	prevBuildApp := buildApp
	prevOpenSQLiteStore := openSQLiteStore
	t.Cleanup(func() {
		buildApp = prevBuildApp
		openSQLiteStore = prevOpenSQLiteStore
	})

	tmp := t.TempDir()
	buildApp = func(projectRoot string) (*bootstrap.App, error) {
		app := &bootstrap.App{}
		app.Settings.ContentDBPath = tmp + "/content.db"
		return app, nil
	}
	openSQLiteStore = func(path string) (*contentstore.SQLiteStore, error) {
		store, err := contentstore.NewSQLiteStore(path)
		if err != nil {
			return nil, err
		}
		rawDB, err := sql.Open("sqlite", path)
		if err != nil {
			return nil, err
		}
		defer rawDB.Close()
		now := time.Now().UTC()
		for _, stmt := range []string{
			fmt.Sprintf(`INSERT INTO memory_organization_jobs(trigger_event_id, user_id, source_platform, source_external_id, status, created_at) VALUES (1, 'u-jobs-summary-filter', 'twitter', 'queued-job', 'queued', '%s')`, now.Add(-48*time.Hour).Format(time.RFC3339Nano)),
			fmt.Sprintf(`INSERT INTO memory_organization_jobs(trigger_event_id, user_id, source_platform, source_external_id, status, created_at, started_at) VALUES (2, 'u-jobs-summary-filter', 'twitter', 'running-job', 'running', '%s', '%s')`, now.Add(-48*time.Hour).Format(time.RFC3339Nano), now.Add(-2*time.Hour).Format(time.RFC3339Nano)),
		} {
			if _, err := rawDB.Exec(stmt); err != nil {
				return nil, err
			}
		}
		return store, nil
	}
	var stdout, stderr bytes.Buffer
	code := run([]string{"memory", "jobs", "--user", "u-jobs-summary-filter", "--status", "queued", "--summary"}, "/tmp/project", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("memory jobs --status queued --summary code = %d, stderr = %s", code, stderr.String())
	}
	var out struct {
		User            string         `json:"user"`
		Counts          map[string]int `json:"counts"`
		OldestQueuedAt  string         `json:"oldest_queued_at"`
		OldestRunningAt string         `json:"oldest_running_at"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &out); err != nil {
		t.Fatalf("json.Unmarshal(memory jobs status+summary) error = %v", err)
	}
	if out.Counts["queued"] != 1 || len(out.Counts) == 0 {
		t.Fatalf("summary counts = %#v, want only queued count", out.Counts)
	}
	if out.Counts["running"] != 0 {
		t.Fatalf("summary counts = %#v, did not want running count under queued filter", out.Counts)
	}
	if out.OldestQueuedAt == "" || out.OldestRunningAt != "" {
		t.Fatalf("summary output = %#v, want only oldest_queued_at populated", out)
	}
}

func TestRunMemoryCleanupStaleRequiresUser(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{"memory", "cleanup-stale"}, "/tmp/project", &stdout, &stderr)
	if code != 2 {
		t.Fatalf("run() code = %d, want 2", code)
	}
	if !strings.Contains(stderr.String(), "usage: varix memory cleanup-stale") {
		t.Fatalf("stderr = %q, want usage", stderr.String())
	}
}

func TestRunMemoryCleanupStaleRejectsNonPositiveOlderThan(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{"memory", "cleanup-stale", "--user", "u-clean", "--older-than", "-1h"}, "/tmp/project", &stdout, &stderr)
	if code != 2 {
		t.Fatalf("run() code = %d, want 2", code)
	}
	if !strings.Contains(stderr.String(), "--older-than must be positive") {
		t.Fatalf("stderr = %q, want positive older-than validation", stderr.String())
	}
}

func TestRunMemoryCleanupStaleDeletesOnlyOldQueuedOrRunningJobs(t *testing.T) {
	prevBuildApp := buildApp
	prevOpenSQLiteStore := openSQLiteStore
	t.Cleanup(func() {
		buildApp = prevBuildApp
		openSQLiteStore = prevOpenSQLiteStore
	})

	tmp := t.TempDir()
	buildApp = func(projectRoot string) (*bootstrap.App, error) {
		app := &bootstrap.App{}
		app.Settings.ContentDBPath = tmp + "/content.db"
		return app, nil
	}
	openSQLiteStore = func(path string) (*contentstore.SQLiteStore, error) {
		store, err := contentstore.NewSQLiteStore(path)
		if err != nil {
			return nil, err
		}
		rawDB, err := sql.Open("sqlite", path)
		if err != nil {
			return nil, err
		}
		defer rawDB.Close()
		now := time.Now().UTC()
		for _, stmt := range []string{
			fmt.Sprintf(`INSERT INTO memory_organization_jobs(trigger_event_id, user_id, source_platform, source_external_id, status, created_at) VALUES (1, 'u-clean', 'twitter', 'old-queued', 'queued', '%s')`, now.Add(-48*time.Hour).Format(time.RFC3339Nano)),
			fmt.Sprintf(`INSERT INTO memory_organization_jobs(trigger_event_id, user_id, source_platform, source_external_id, status, created_at, started_at) VALUES (2, 'u-clean', 'twitter', 'old-running', 'running', '%s', '%s')`, now.Add(-36*time.Hour).Format(time.RFC3339Nano), now.Add(-35*time.Hour).Format(time.RFC3339Nano)),
			fmt.Sprintf(`INSERT INTO memory_organization_jobs(trigger_event_id, user_id, source_platform, source_external_id, status, created_at) VALUES (3, 'u-clean', 'twitter', 'fresh-queued', 'queued', '%s')`, now.Add(-2*time.Hour).Format(time.RFC3339Nano)),
			fmt.Sprintf(`INSERT INTO memory_organization_jobs(trigger_event_id, user_id, source_platform, source_external_id, status, created_at, finished_at) VALUES (4, 'u-clean', 'twitter', 'done-old', 'done', '%s', '%s')`, now.Add(-72*time.Hour).Format(time.RFC3339Nano), now.Add(-71*time.Hour).Format(time.RFC3339Nano)),
		} {
			if _, err := rawDB.Exec(stmt); err != nil {
				return nil, err
			}
		}
		return store, nil
	}
	var stdout, stderr bytes.Buffer
	code := run([]string{"memory", "cleanup-stale", "--user", "u-clean", "--older-than", "24h"}, "/tmp/project", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("memory cleanup-stale code = %d, stderr = %s", code, stderr.String())
	}
	var out map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &out); err != nil {
		t.Fatalf("json.Unmarshal(cleanup-stale) error = %v", err)
	}
	if out["deleted_jobs"] != float64(2) {
		t.Fatalf("cleanup-stale output = %#v, want deleted_jobs=2", out)
	}
	// Reopen and verify only fresh queued + done remain.
	store, err := contentstore.NewSQLiteStore(tmp + "/content.db")
	if err != nil {
		t.Fatalf("NewSQLiteStore(reopen) error = %v", err)
	}
	defer store.Close()
	jobs, err := store.ListMemoryJobs(context.Background(), "u-clean")
	if err != nil {
		t.Fatalf("ListMemoryJobs() error = %v", err)
	}
	if len(jobs) != 2 {
		t.Fatalf("len(ListMemoryJobs()) = %d, want 2 after cleanup", len(jobs))
	}
	for _, job := range jobs {
		if job.SourceExternalID == "old-queued" || job.SourceExternalID == "old-running" {
			t.Fatalf("jobs = %#v, did not want stale queued/running jobs to remain", jobs)
		}
	}
}

func TestRunMemoryCleanupStaleSupportsSourceFilter(t *testing.T) {
	prevBuildApp := buildApp
	prevOpenSQLiteStore := openSQLiteStore
	t.Cleanup(func() {
		buildApp = prevBuildApp
		openSQLiteStore = prevOpenSQLiteStore
	})

	tmp := t.TempDir()
	buildApp = func(projectRoot string) (*bootstrap.App, error) {
		app := &bootstrap.App{}
		app.Settings.ContentDBPath = tmp + "/content.db"
		return app, nil
	}
	openSQLiteStore = func(path string) (*contentstore.SQLiteStore, error) {
		store, err := contentstore.NewSQLiteStore(path)
		if err != nil {
			return nil, err
		}
		rawDB, err := sql.Open("sqlite", path)
		if err != nil {
			return nil, err
		}
		defer rawDB.Close()
		now := time.Now().UTC()
		for _, stmt := range []string{
			fmt.Sprintf(`INSERT INTO memory_organization_jobs(trigger_event_id, user_id, source_platform, source_external_id, status, created_at) VALUES (1, 'u-clean-filter', 'twitter', 'delete-me', 'queued', '%s')`, now.Add(-48*time.Hour).Format(time.RFC3339Nano)),
			fmt.Sprintf(`INSERT INTO memory_organization_jobs(trigger_event_id, user_id, source_platform, source_external_id, status, created_at) VALUES (2, 'u-clean-filter', 'twitter', 'keep-me', 'queued', '%s')`, now.Add(-48*time.Hour).Format(time.RFC3339Nano)),
		} {
			if _, err := rawDB.Exec(stmt); err != nil {
				return nil, err
			}
		}
		return store, nil
	}
	var stdout, stderr bytes.Buffer
	code := run([]string{"memory", "cleanup-stale", "--user", "u-clean-filter", "--older-than", "24h", "--platform", "twitter", "--id", "delete-me"}, "/tmp/project", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("memory cleanup-stale filter code = %d, stderr = %s", code, stderr.String())
	}
	store, err := contentstore.NewSQLiteStore(tmp + "/content.db")
	if err != nil {
		t.Fatalf("NewSQLiteStore(reopen) error = %v", err)
	}
	defer store.Close()
	jobs, err := store.ListMemoryJobs(context.Background(), "u-clean-filter")
	if err != nil {
		t.Fatalf("ListMemoryJobs() error = %v", err)
	}
	if len(jobs) != 1 || jobs[0].SourceExternalID != "keep-me" {
		t.Fatalf("jobs = %#v, want only keep-me after filtered cleanup", jobs)
	}
}

func TestRunMemoryCleanupStaleSupportsPlatformOnlyFilter(t *testing.T) {
	prevBuildApp := buildApp
	prevOpenSQLiteStore := openSQLiteStore
	t.Cleanup(func() {
		buildApp = prevBuildApp
		openSQLiteStore = prevOpenSQLiteStore
	})

	tmp := t.TempDir()
	buildApp = func(projectRoot string) (*bootstrap.App, error) {
		app := &bootstrap.App{}
		app.Settings.ContentDBPath = tmp + "/content.db"
		return app, nil
	}
	openSQLiteStore = func(path string) (*contentstore.SQLiteStore, error) {
		store, err := contentstore.NewSQLiteStore(path)
		if err != nil {
			return nil, err
		}
		rawDB, err := sql.Open("sqlite", path)
		if err != nil {
			return nil, err
		}
		defer rawDB.Close()
		now := time.Now().UTC()
		for _, stmt := range []string{
			fmt.Sprintf(`INSERT INTO memory_organization_jobs(trigger_event_id, user_id, source_platform, source_external_id, status, created_at) VALUES (1, 'u-clean-platform', 'twitter', 'delete-twitter', 'queued', '%s')`, now.Add(-48*time.Hour).Format(time.RFC3339Nano)),
			fmt.Sprintf(`INSERT INTO memory_organization_jobs(trigger_event_id, user_id, source_platform, source_external_id, status, created_at) VALUES (2, 'u-clean-platform', 'weibo', 'keep-weibo', 'queued', '%s')`, now.Add(-48*time.Hour).Format(time.RFC3339Nano)),
		} {
			if _, err := rawDB.Exec(stmt); err != nil {
				return nil, err
			}
		}
		return store, nil
	}
	var stdout, stderr bytes.Buffer
	code := run([]string{"memory", "cleanup-stale", "--user", "u-clean-platform", "--older-than", "24h", "--platform", "twitter"}, "/tmp/project", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("memory cleanup-stale platform-only code = %d, stderr = %s", code, stderr.String())
	}
	store, err := contentstore.NewSQLiteStore(tmp + "/content.db")
	if err != nil {
		t.Fatalf("NewSQLiteStore(reopen) error = %v", err)
	}
	defer store.Close()
	jobs, err := store.ListMemoryJobs(context.Background(), "u-clean-platform")
	if err != nil {
		t.Fatalf("ListMemoryJobs() error = %v", err)
	}
	if len(jobs) != 1 || jobs[0].SourcePlatform != "weibo" {
		t.Fatalf("jobs = %#v, want only weibo job after platform-only cleanup", jobs)
	}
}

func TestRunMemoryCleanupStaleKeepsRecentlyStartedRunningJobs(t *testing.T) {
	prevBuildApp := buildApp
	prevOpenSQLiteStore := openSQLiteStore
	t.Cleanup(func() {
		buildApp = prevBuildApp
		openSQLiteStore = prevOpenSQLiteStore
	})

	tmp := t.TempDir()
	buildApp = func(projectRoot string) (*bootstrap.App, error) {
		app := &bootstrap.App{}
		app.Settings.ContentDBPath = tmp + "/content.db"
		return app, nil
	}
	openSQLiteStore = func(path string) (*contentstore.SQLiteStore, error) {
		store, err := contentstore.NewSQLiteStore(path)
		if err != nil {
			return nil, err
		}
		rawDB, err := sql.Open("sqlite", path)
		if err != nil {
			return nil, err
		}
		defer rawDB.Close()
		now := time.Now().UTC()
		for _, stmt := range []string{
			fmt.Sprintf(`INSERT INTO memory_organization_jobs(trigger_event_id, user_id, source_platform, source_external_id, status, created_at, started_at) VALUES (1, 'u-clean-running', 'twitter', 'recent-running', 'running', '%s', '%s')`, now.Add(-72*time.Hour).Format(time.RFC3339Nano), now.Add(-30*time.Minute).Format(time.RFC3339Nano)),
			fmt.Sprintf(`INSERT INTO memory_organization_jobs(trigger_event_id, user_id, source_platform, source_external_id, status, created_at, started_at) VALUES (2, 'u-clean-running', 'twitter', 'stale-running', 'running', '%s', '%s')`, now.Add(-72*time.Hour).Format(time.RFC3339Nano), now.Add(-48*time.Hour).Format(time.RFC3339Nano)),
		} {
			if _, err := rawDB.Exec(stmt); err != nil {
				return nil, err
			}
		}
		return store, nil
	}
	var stdout, stderr bytes.Buffer
	code := run([]string{"memory", "cleanup-stale", "--user", "u-clean-running", "--older-than", "24h"}, "/tmp/project", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("memory cleanup-stale running code = %d, stderr = %s", code, stderr.String())
	}
	store, err := contentstore.NewSQLiteStore(tmp + "/content.db")
	if err != nil {
		t.Fatalf("NewSQLiteStore(reopen) error = %v", err)
	}
	defer store.Close()
	jobs, err := store.ListMemoryJobs(context.Background(), "u-clean-running")
	if err != nil {
		t.Fatalf("ListMemoryJobs() error = %v", err)
	}
	if len(jobs) != 1 || jobs[0].SourceExternalID != "recent-running" {
		t.Fatalf("jobs = %#v, want only recently started running job to remain", jobs)
	}
}

func TestRunMemoryCleanupStaleDryRunReportsButDoesNotDelete(t *testing.T) {
	prevBuildApp := buildApp
	prevOpenSQLiteStore := openSQLiteStore
	t.Cleanup(func() {
		buildApp = prevBuildApp
		openSQLiteStore = prevOpenSQLiteStore
	})

	tmp := t.TempDir()
	buildApp = func(projectRoot string) (*bootstrap.App, error) {
		app := &bootstrap.App{}
		app.Settings.ContentDBPath = tmp + "/content.db"
		return app, nil
	}
	openSQLiteStore = func(path string) (*contentstore.SQLiteStore, error) {
		store, err := contentstore.NewSQLiteStore(path)
		if err != nil {
			return nil, err
		}
		rawDB, err := sql.Open("sqlite", path)
		if err != nil {
			return nil, err
		}
		defer rawDB.Close()
		now := time.Now().UTC()
		for _, stmt := range []string{
			fmt.Sprintf(`INSERT INTO memory_organization_jobs(trigger_event_id, user_id, source_platform, source_external_id, status, created_at) VALUES (1, 'u-clean-dry', 'twitter', 'old-queued', 'queued', '%s')`, now.Add(-48*time.Hour).Format(time.RFC3339Nano)),
			fmt.Sprintf(`INSERT INTO memory_organization_jobs(trigger_event_id, user_id, source_platform, source_external_id, status, created_at) VALUES (2, 'u-clean-dry', 'twitter', 'fresh-queued', 'queued', '%s')`, now.Add(-2*time.Hour).Format(time.RFC3339Nano)),
		} {
			if _, err := rawDB.Exec(stmt); err != nil {
				return nil, err
			}
		}
		return store, nil
	}

	var stdout, stderr bytes.Buffer
	code := run([]string{"memory", "cleanup-stale", "--user", "u-clean-dry", "--older-than", "24h", "--dry-run"}, "/tmp/project", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("memory cleanup-stale --dry-run code = %d, stderr = %s", code, stderr.String())
	}
	var out map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &out); err != nil {
		t.Fatalf("json.Unmarshal(cleanup-stale dry-run) error = %v", err)
	}
	if out["deleted_jobs"] != float64(1) {
		t.Fatalf("dry-run output = %#v, want deleted_jobs=1", out)
	}
	if dryRun, ok := out["dry_run"].(bool); !ok || !dryRun {
		t.Fatalf("dry-run output = %#v, want dry_run=true", out)
	}
	store, err := contentstore.NewSQLiteStore(tmp + "/content.db")
	if err != nil {
		t.Fatalf("NewSQLiteStore(reopen) error = %v", err)
	}
	defer store.Close()
	jobs, err := store.ListMemoryJobs(context.Background(), "u-clean-dry")
	if err != nil {
		t.Fatalf("ListMemoryJobs() error = %v", err)
	}
	if len(jobs) != 2 {
		t.Fatalf("len(ListMemoryJobs()) = %d, want 2 because dry-run should not delete", len(jobs))
	}
}

func TestRunMemoryCleanupStaleDryRunHonorsSourceFilter(t *testing.T) {
	prevBuildApp := buildApp
	prevOpenSQLiteStore := openSQLiteStore
	t.Cleanup(func() {
		buildApp = prevBuildApp
		openSQLiteStore = prevOpenSQLiteStore
	})

	tmp := t.TempDir()
	buildApp = func(projectRoot string) (*bootstrap.App, error) {
		app := &bootstrap.App{}
		app.Settings.ContentDBPath = tmp + "/content.db"
		return app, nil
	}
	openSQLiteStore = func(path string) (*contentstore.SQLiteStore, error) {
		store, err := contentstore.NewSQLiteStore(path)
		if err != nil {
			return nil, err
		}
		rawDB, err := sql.Open("sqlite", path)
		if err != nil {
			return nil, err
		}
		defer rawDB.Close()
		now := time.Now().UTC()
		for _, stmt := range []string{
			fmt.Sprintf(`INSERT INTO memory_organization_jobs(trigger_event_id, user_id, source_platform, source_external_id, status, created_at) VALUES (1, 'u-clean-dry-filter', 'twitter', 'delete-me', 'queued', '%s')`, now.Add(-48*time.Hour).Format(time.RFC3339Nano)),
			fmt.Sprintf(`INSERT INTO memory_organization_jobs(trigger_event_id, user_id, source_platform, source_external_id, status, created_at) VALUES (2, 'u-clean-dry-filter', 'weibo', 'keep-me', 'queued', '%s')`, now.Add(-48*time.Hour).Format(time.RFC3339Nano)),
		} {
			if _, err := rawDB.Exec(stmt); err != nil {
				return nil, err
			}
		}
		return store, nil
	}

	var stdout, stderr bytes.Buffer
	code := run([]string{"memory", "cleanup-stale", "--user", "u-clean-dry-filter", "--older-than", "24h", "--platform", "twitter", "--dry-run"}, "/tmp/project", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("memory cleanup-stale --dry-run filter code = %d, stderr = %s", code, stderr.String())
	}
	var out map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &out); err != nil {
		t.Fatalf("json.Unmarshal(cleanup-stale dry-run filter) error = %v", err)
	}
	if out["deleted_jobs"] != float64(1) {
		t.Fatalf("dry-run filter output = %#v, want deleted_jobs=1", out)
	}
	store, err := contentstore.NewSQLiteStore(tmp + "/content.db")
	if err != nil {
		t.Fatalf("NewSQLiteStore(reopen) error = %v", err)
	}
	defer store.Close()
	jobs, err := store.ListMemoryJobs(context.Background(), "u-clean-dry-filter")
	if err != nil {
		t.Fatalf("ListMemoryJobs() error = %v", err)
	}
	if len(jobs) != 2 {
		t.Fatalf("len(ListMemoryJobs()) = %d, want 2 because dry-run filter should not delete", len(jobs))
	}
}
