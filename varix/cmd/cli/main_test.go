package main

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kumaloha/VariX/varix/bootstrap"
	c "github.com/kumaloha/VariX/varix/compile"
	"github.com/kumaloha/VariX/varix/graphmodel"
	"github.com/kumaloha/VariX/varix/ingest/dispatcher"
	"github.com/kumaloha/VariX/varix/ingest/polling"
	"github.com/kumaloha/VariX/varix/ingest/types"
	"github.com/kumaloha/VariX/varix/memory"
	"github.com/kumaloha/VariX/varix/storage/contentstore"
)

type fakeItemSource struct {
	platform types.Platform
	kind     types.Kind
	items    []types.RawContent
	fetchFn  func(context.Context, types.ParsedURL) ([]types.RawContent, error)
}

func (f fakeItemSource) Platform() types.Platform {
	if f.platform != "" {
		return f.platform
	}
	return types.PlatformWeb
}

func (f fakeItemSource) Kind() types.Kind {
	if f.kind != "" {
		return f.kind
	}
	return types.KindNative
}

func (f fakeItemSource) Fetch(ctx context.Context, parsed types.ParsedURL) ([]types.RawContent, error) {
	if f.fetchFn != nil {
		return f.fetchFn(ctx, parsed)
	}
	return f.items, nil
}

type panicItemSource struct{}

func (panicItemSource) Platform() types.Platform { return types.PlatformWeb }
func (panicItemSource) Kind() types.Kind         { return types.KindNative }
func (panicItemSource) Fetch(context.Context, types.ParsedURL) ([]types.RawContent, error) {
	panic("fetch should not be called")
}

func TestRunIngestFetchWritesJSONToStdout(t *testing.T) {
	prevBuildApp := buildApp
	prevGetwd := getwd
	t.Cleanup(func() {
		buildApp = prevBuildApp
		getwd = prevGetwd
	})

	buildApp = func(projectRoot string) (*bootstrap.App, error) {
		src := fakeItemSource{
			items: []types.RawContent{{
				Source:     "weibo",
				ExternalID: "QAzzRES0G",
				Content:    "hello",
				AuthorName: "Alice",
				URL:        "https://weibo.com/1182426800/QAzzRES0G",
			}},
		}
		return &bootstrap.App{
			Dispatcher: dispatcher.New(
				func(raw string) (types.ParsedURL, error) {
					return types.ParsedURL{
						Platform:     types.PlatformWeb,
						ContentType:  types.ContentTypePost,
						PlatformID:   "id-1",
						CanonicalURL: raw,
					}, nil
				},
				[]dispatcher.ItemSource{src},
				nil,
				nil,
			),
		}, nil
	}
	getwd = func() (string, error) { return "/tmp/project", nil }

	var stdout, stderr bytes.Buffer
	code := run([]string{"ingest", "fetch", "https://example.com/post"}, "/tmp/project", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("run() code = %d, stderr = %s", code, stderr.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q, want empty", stderr.String())
	}

	var got []types.RawContent
	if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
		t.Fatalf("json.Unmarshal(stdout) error = %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("len(stdout payload) = %d, want 1", len(got))
	}
	if got[0].ExternalID != "QAzzRES0G" {
		t.Fatalf("ExternalID = %q, want QAzzRES0G", got[0].ExternalID)
	}
}

func TestParseCompileRunIDsDedupesAndRejectsInvalidValues(t *testing.T) {
	got, err := parseCompileRunIDs("301, 302,301")
	if err != nil {
		t.Fatalf("parseCompileRunIDs() error = %v", err)
	}
	if len(got) != 2 || got[0] != 301 || got[1] != 302 {
		t.Fatalf("parseCompileRunIDs() = %#v", got)
	}
	if _, err := parseCompileRunIDs("301,nope"); err == nil {
		t.Fatalf("parseCompileRunIDs() error = nil, want invalid run id error")
	}
}

func TestRunIngestFetchRequiresURL(t *testing.T) {
	prevGetwd := getwd
	t.Cleanup(func() {
		getwd = prevGetwd
	})
	getwd = func() (string, error) { return "/tmp/project", nil }

	var stdout, stderr bytes.Buffer
	code := run([]string{"ingest", "fetch"}, "/tmp/project", &stdout, &stderr)
	if code != 2 {
		t.Fatalf("run() code = %d, want 2", code)
	}
	if !strings.Contains(stderr.String(), "usage: varix ingest fetch") {
		t.Fatalf("stderr = %q, want usage", stderr.String())
	}
}

func TestRunCompileRequiresURL(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{"compile", "run"}, "/tmp/project", &stdout, &stderr)
	if code != 2 {
		t.Fatalf("run() code = %d, want 2", code)
	}
	if !strings.Contains(stderr.String(), "usage: varix compile run") {
		t.Fatalf("stderr = %q, want usage", stderr.String())
	}
}

func TestRunCompileShowRequiresLocator(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{"compile", "show"}, "/tmp/project", &stdout, &stderr)
	if code != 2 {
		t.Fatalf("run() code = %d, want 2", code)
	}
	if !strings.Contains(stderr.String(), "usage: varix compile show") {
		t.Fatalf("stderr = %q, want usage", stderr.String())
	}
}

func TestRunCompileSummaryRequiresLocator(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{"compile", "summary"}, "/tmp/project", &stdout, &stderr)
	if code != 2 {
		t.Fatalf("run() code = %d, want 2", code)
	}
	if !strings.Contains(stderr.String(), "usage: varix compile summary") {
		t.Fatalf("stderr = %q, want usage", stderr.String())
	}
}

func TestRunCompileCompareRequiresLocator(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{"compile", "compare"}, "/tmp/project", &stdout, &stderr)
	if code != 2 {
		t.Fatalf("run() code = %d, want 2", code)
	}
	if !strings.Contains(stderr.String(), "usage: varix compile compare") {
		t.Fatalf("stderr = %q, want usage", stderr.String())
	}
}

func TestRunCompileCardRequiresLocator(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{"compile", "card"}, "/tmp/project", &stdout, &stderr)
	if code != 2 {
		t.Fatalf("run() code = %d, want 2", code)
	}
	if !strings.Contains(stderr.String(), "usage: varix compile card") {
		t.Fatalf("stderr = %q, want usage", stderr.String())
	}
}

func TestRunMemoryAcceptRequiresFields(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{"memory", "accept"}, "/tmp/project", &stdout, &stderr)
	if code != 2 {
		t.Fatalf("run() code = %d, want 2", code)
	}
	if !strings.Contains(stderr.String(), "usage: varix memory accept") {
		t.Fatalf("stderr = %q, want usage", stderr.String())
	}
}

func TestRunMemoryPosteriorRunRequiresUser(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{"memory", "posterior-run"}, "/tmp/project", &stdout, &stderr)
	if code != 2 {
		t.Fatalf("run() code = %d, want 2", code)
	}
	if !strings.Contains(stderr.String(), "usage: varix memory posterior-run") {
		t.Fatalf("stderr = %q, want usage", stderr.String())
	}
}

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

func TestRunMemoryBackfillRequiresValidLayerInputs(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{"memory", "backfill"}, "/tmp/project", &stdout, &stderr)
	if code != 2 {
		t.Fatalf("run() code = %d, want 2", code)
	}
	if !strings.Contains(stderr.String(), "usage: varix memory backfill") {
		t.Fatalf("stderr = %q, want usage", stderr.String())
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

func TestRunMemoryCanonicalEntityUpsertRequiresFields(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{"memory", "canonical-entity-upsert"}, "/tmp/project", &stdout, &stderr)
	if code != 2 {
		t.Fatalf("run() code = %d, want 2", code)
	}
	if !strings.Contains(stderr.String(), "usage: varix memory canonical-entity-upsert") {
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

func TestRunMemoryAcceptPersistsNodeAndJob(t *testing.T) {
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
		record := c.Record{
			UnitID:         "weibo:Q1",
			Source:         "weibo",
			ExternalID:     "Q1",
			RootExternalID: "Q1",
			Model:          c.Qwen36PlusModel,
			Output: c.Output{
				Summary: "一句话",
				Graph: c.ReasoningGraph{
					Nodes: []c.GraphNode{testGraphNode("n1", c.NodeFact, "事实A"), testGraphNode("n2", c.NodeConclusion, "结论B")},
					Edges: []c.GraphEdge{{From: "n1", To: "n2", Kind: c.EdgeDerives}},
				},
				Details: c.HiddenDetails{Caveats: []string{"说明"}},
			},
			CompiledAt: time.Now().UTC(),
		}
		if err := store.UpsertCompiledOutput(context.Background(), record); err != nil {
			return nil, err
		}
		return store, nil
	}

	var stdout, stderr bytes.Buffer
	code := run([]string{"memory", "accept", "--user", "u1", "--platform", "weibo", "--id", "Q1", "--node", "n1"}, "/tmp/project", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("run() code = %d, stderr = %s", code, stderr.String())
	}
	var got memory.AcceptResult
	if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
		t.Fatalf("json.Unmarshal(stdout) error = %v", err)
	}
	if len(got.Nodes) != 1 || got.Nodes[0].NodeID != "n1" {
		t.Fatalf("got = %#v", got)
	}
	if got.Job.JobID == 0 || got.Event.EventID == 0 {
		t.Fatalf("job/event = %#v / %#v", got.Job, got.Event)
	}
}

func TestRunMemoryAcceptBatchAndList(t *testing.T) {
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
		record := c.Record{
			UnitID:         "weibo:Q1",
			Source:         "weibo",
			ExternalID:     "Q1",
			RootExternalID: "Q1",
			Model:          c.Qwen36PlusModel,
			Output: c.Output{
				Summary: "一句话",
				Graph: c.ReasoningGraph{
					Nodes: []c.GraphNode{testGraphNode("n1", c.NodeFact, "事实A"), testGraphNode("n2", c.NodeConclusion, "结论B")},
					Edges: []c.GraphEdge{{From: "n1", To: "n2", Kind: c.EdgeDerives}},
				},
				Details: c.HiddenDetails{Caveats: []string{"说明"}},
			},
			CompiledAt: time.Now().UTC(),
		}
		if err := store.UpsertCompiledOutput(context.Background(), record); err != nil {
			return nil, err
		}
		return store, nil
	}

	var stdout, stderr bytes.Buffer
	code := run([]string{"memory", "accept-batch", "--user", "u1", "--platform", "weibo", "--id", "Q1", "--nodes", "n1,n2"}, "/tmp/project", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("accept-batch code = %d, stderr = %s", code, stderr.String())
	}
	stdout.Reset()
	stderr.Reset()
	code = run([]string{"memory", "list", "--user", "u1"}, "/tmp/project", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("list code = %d, stderr = %s", code, stderr.String())
	}
	var got []memory.AcceptedNode
	if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
		t.Fatalf("json.Unmarshal(list) error = %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("len(list) = %d, want 2", len(got))
	}
}

func TestRunMemoryAcceptBatchAndListDerivesLegacyValidityFromNodeTiming(t *testing.T) {
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
		occurredAt := time.Date(2026, 4, 10, 9, 0, 0, 0, time.UTC)
		predictionStart := time.Date(2026, 4, 12, 0, 0, 0, 0, time.UTC)
		predictionDue := time.Date(2026, 4, 20, 0, 0, 0, 0, time.UTC)
		record := c.Record{
			UnitID:         "weibo:Q-time",
			Source:         "weibo",
			ExternalID:     "Q-time",
			RootExternalID: "Q-time",
			Model:          c.Qwen36PlusModel,
			Output: c.Output{
				Summary: "一句话",
				Graph: c.ReasoningGraph{
					Nodes: []c.GraphNode{
						{ID: "n1", Kind: c.NodeFact, Text: "事实A", OccurredAt: occurredAt},
						{ID: "n2", Kind: c.NodePrediction, Text: "预测B", PredictionStartAt: predictionStart, PredictionDueAt: predictionDue},
						{ID: "n3", Kind: c.NodeConclusion, Text: "结论C"},
					},
					Edges: []c.GraphEdge{{From: "n1", To: "n3", Kind: c.EdgeDerives}, {From: "n3", To: "n2", Kind: c.EdgeDerives}},
				},
				Details: c.HiddenDetails{Caveats: []string{"说明"}},
			},
			CompiledAt: time.Now().UTC(),
		}
		if err := store.UpsertCompiledOutput(context.Background(), record); err != nil {
			return nil, err
		}
		return store, nil
	}

	var stdout, stderr bytes.Buffer
	code := run([]string{"memory", "accept-batch", "--user", "u1", "--platform", "weibo", "--id", "Q-time", "--nodes", "n1,n2"}, "/tmp/project", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("accept-batch code = %d, stderr = %s", code, stderr.String())
	}
	stdout.Reset()
	stderr.Reset()
	code = run([]string{"memory", "list", "--user", "u1"}, "/tmp/project", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("list code = %d, stderr = %s", code, stderr.String())
	}
	var got []memory.AcceptedNode
	if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
		t.Fatalf("json.Unmarshal(list) error = %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("len(list) = %d, want 2", len(got))
	}
	if !got[0].ValidFrom.Equal(time.Date(2026, 4, 10, 9, 0, 0, 0, time.UTC)) {
		t.Fatalf("fact ValidFrom = %s, want occurred_at-derived timestamp", got[0].ValidFrom)
	}
	if got[0].ValidTo.Year() != 9999 {
		t.Fatalf("fact ValidTo = %s, want open-ended year 9999", got[0].ValidTo)
	}
	if !got[1].ValidFrom.Equal(time.Date(2026, 4, 12, 0, 0, 0, 0, time.UTC)) || !got[1].ValidTo.Equal(time.Date(2026, 4, 20, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("prediction validity = %s..%s, want prediction_start_at/prediction_due_at-derived window", got[1].ValidFrom, got[1].ValidTo)
	}
}

func TestRunMemoryOrganizeRunAndShow(t *testing.T) {
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
		record := c.Record{
			UnitID:         "weibo:Q1",
			Source:         "weibo",
			ExternalID:     "Q1",
			RootExternalID: "Q1",
			Model:          c.Qwen36PlusModel,
			Output: c.Output{
				Summary: "一句话",
				Graph: c.ReasoningGraph{
					Nodes: []c.GraphNode{
						testGraphNode("n1", c.NodeFact, "通胀下降"),
						testGraphNode("n2", c.NodePrediction, "三个月内降息"),
					},
					Edges: []c.GraphEdge{{From: "n1", To: "n2", Kind: c.EdgeDerives}},
				},
				Details: c.HiddenDetails{Caveats: []string{"说明"}},
				Verification: c.Verification{
					VerifiedAt: time.Now().UTC(),
					Model:      c.Qwen36PlusModel,
					FactChecks: []c.FactCheck{{NodeID: "n1", Status: c.FactStatusClearlyTrue, Reason: "supported"}},
					PredictionChecks: []c.PredictionCheck{{
						NodeID: "n2", Status: c.PredictionStatusUnresolved, Reason: "still active", AsOf: time.Now().UTC(),
					}},
				},
			},
			CompiledAt: time.Now().UTC(),
		}
		if err := store.UpsertCompiledOutput(context.Background(), record); err != nil {
			return nil, err
		}
		if _, err := store.AcceptMemoryNodes(context.Background(), memory.AcceptRequest{
			UserID:           "u1",
			SourcePlatform:   "weibo",
			SourceExternalID: "Q1",
			NodeIDs:          []string{"n1", "n2"},
		}); err != nil {
			return nil, err
		}
		return store, nil
	}

	var stdout, stderr bytes.Buffer
	code := run([]string{"memory", "organize-run", "--user", "u1"}, "/tmp/project", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("organize-run code = %d, stderr = %s", code, stderr.String())
	}
	var out memory.OrganizationOutput
	if err := json.Unmarshal(stdout.Bytes(), &out); err != nil {
		t.Fatalf("json.Unmarshal(organize-run) error = %v", err)
	}
	if out.JobID == 0 {
		t.Fatalf("output = %#v", out)
	}

	stdout.Reset()
	stderr.Reset()
	code = run([]string{"memory", "organized", "--user", "u1", "--platform", "weibo", "--id", "Q1"}, "/tmp/project", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("organized code = %d, stderr = %s", code, stderr.String())
	}
	var got memory.OrganizationOutput
	if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
		t.Fatalf("json.Unmarshal(organized) error = %v", err)
	}
	if got.JobID != out.JobID {
		t.Fatalf("JobID = %d, want %d", got.JobID, out.JobID)
	}
}

func TestRunMemoryOrganizedIncludesFrontendHints(t *testing.T) {
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
		record := c.Record{
			UnitID:         "weibo:Q2",
			Source:         "weibo",
			ExternalID:     "Q2",
			RootExternalID: "Q2",
			Model:          c.Qwen36PlusModel,
			Output: c.Output{
				Summary: "一句话",
				Graph: c.ReasoningGraph{
					Nodes: []c.GraphNode{
						testGraphNode("n1", c.NodeFact, "油价会上升"),
						testGraphNode("n2", c.NodeFact, "油价将上行"),
						testGraphNode("n3", c.NodeConclusion, "能源股受益"),
					},
					Edges: []c.GraphEdge{{From: "n1", To: "n3", Kind: c.EdgePositive}},
				},
				Details: c.HiddenDetails{Caveats: []string{"说明"}},
				Verification: c.Verification{
					VerifiedAt: time.Now().UTC(),
					Model:      c.Qwen36PlusModel,
					FactChecks: []c.FactCheck{{NodeID: "n1", Status: c.FactStatusClearlyTrue, Reason: "supported"}},
				},
			},
			CompiledAt: time.Now().UTC(),
		}
		if err := store.UpsertCompiledOutput(context.Background(), record); err != nil {
			return nil, err
		}
		if _, err := store.AcceptMemoryNodes(context.Background(), memory.AcceptRequest{
			UserID:           "u2",
			SourcePlatform:   "weibo",
			SourceExternalID: "Q2",
			NodeIDs:          []string{"n1", "n2", "n3"},
		}); err != nil {
			return nil, err
		}
		return store, nil
	}

	var stdout, stderr bytes.Buffer
	code := run([]string{"memory", "organize-run", "--user", "u2"}, "/tmp/project", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("organize-run code = %d, stderr = %s", code, stderr.String())
	}

	var payload map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
		t.Fatalf("json.Unmarshal(organize-run payload) error = %v", err)
	}

	dedupeGroups, ok := payload["dedupe_groups"].([]any)
	if !ok || len(dedupeGroups) != 1 {
		t.Fatalf("dedupe_groups = %#v, want one frontend-ready group", payload["dedupe_groups"])
	}
	firstDedupe, ok := dedupeGroups[0].(map[string]any)
	if !ok {
		t.Fatalf("dedupe_groups[0] = %#v, want object", dedupeGroups[0])
	}
	if strings.TrimSpace(stringValue(firstDedupe["canonical_text"])) == "" {
		t.Fatalf("dedupe group missing canonical_text: %#v", firstDedupe)
	}
	if strings.TrimSpace(stringValue(firstDedupe["hint"])) == "" {
		t.Fatalf("dedupe group missing hint: %#v", firstDedupe)
	}

	hierarchy, ok := payload["hierarchy"].([]any)
	if !ok || len(hierarchy) == 0 {
		t.Fatalf("hierarchy = %#v, want frontend-ready link entries", payload["hierarchy"])
	}
	firstLink, ok := hierarchy[0].(map[string]any)
	if !ok {
		t.Fatalf("hierarchy[0] = %#v, want object", hierarchy[0])
	}
	for _, key := range []string{"parent_kind", "child_kind", "source", "hint"} {
		if strings.TrimSpace(stringValue(firstLink[key])) == "" {
			t.Fatalf("hierarchy link missing %s: %#v", key, firstLink)
		}
	}
}

func TestRunMemoryOrganizedIncludesDominantDriverFeedbackAndVerdicts(t *testing.T) {
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
		record := c.Record{
			UnitID:         "weibo:Q-driver-cli",
			Source:         "weibo",
			ExternalID:     "Q-driver-cli",
			RootExternalID: "Q-driver-cli",
			Model:          c.Qwen36PlusModel,
			Output: c.Output{
				Summary: "一句话",
				Graph: c.ReasoningGraph{
					Nodes: []c.GraphNode{
						testGraphNode("n1", c.NodeFact, "美元走弱"),
						testGraphNode("n2", c.NodeFact, "风险偏好回升"),
						testGraphNode("n3", c.NodeConclusion, "黄金获得支撑"),
						testGraphNode("n4", c.NodePrediction, "金价继续走高"),
					},
					Edges: []c.GraphEdge{
						{From: "n1", To: "n3", Kind: c.EdgePositive},
						{From: "n2", To: "n3", Kind: c.EdgePositive},
						{From: "n3", To: "n4", Kind: c.EdgeDerives},
					},
				},
				Details: c.HiddenDetails{Caveats: []string{"说明"}},
				Verification: c.Verification{
					VerifiedAt: time.Now().UTC(),
					Model:      c.Qwen36PlusModel,
					FactChecks: []c.FactCheck{
						{NodeID: "n1", Status: c.FactStatusClearlyTrue, Reason: "confirmed by price action"},
						{NodeID: "n2", Status: c.FactStatusUnverifiable, Reason: "support remains thin"},
					},
					PredictionChecks: []c.PredictionCheck{
						{NodeID: "n4", Status: c.PredictionStatusResolvedFalse, Reason: "price broke lower", AsOf: time.Now().UTC()},
					},
				},
			},
			CompiledAt: time.Now().UTC(),
		}
		if err := store.UpsertCompiledOutput(context.Background(), record); err != nil {
			return nil, err
		}
		if _, err := store.AcceptMemoryNodes(context.Background(), memory.AcceptRequest{
			UserID:           "u-driver-cli",
			SourcePlatform:   "weibo",
			SourceExternalID: "Q-driver-cli",
			NodeIDs:          []string{"n1", "n2", "n3", "n4"},
		}); err != nil {
			return nil, err
		}
		return store, nil
	}

	var stdout, stderr bytes.Buffer
	code := run([]string{"memory", "organize-run", "--user", "u-driver-cli"}, "/tmp/project", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("organize-run code = %d, stderr = %s", code, stderr.String())
	}

	stdout.Reset()
	stderr.Reset()
	code = run([]string{"memory", "organized", "--user", "u-driver-cli", "--platform", "weibo", "--id", "Q-driver-cli"}, "/tmp/project", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("organized code = %d, stderr = %s", code, stderr.String())
	}

	var payload map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
		t.Fatalf("json.Unmarshal(organized payload) error = %v", err)
	}

	dominantDriver, ok := payload["dominant_driver"].(map[string]any)
	if !ok {
		t.Fatalf("dominant_driver = %#v, want object", payload["dominant_driver"])
	}
	if stringValue(dominantDriver["node_id"]) != "n1" {
		t.Fatalf("dominant_driver.node_id = %#v, want n1", dominantDriver["node_id"])
	}
	if !strings.Contains(stringValue(dominantDriver["explanation"]), "primary") || !strings.Contains(stringValue(dominantDriver["explanation"]), "supporting") {
		t.Fatalf("dominant_driver.explanation = %#v, want primary vs supporting explanation", dominantDriver["explanation"])
	}

	feedback, ok := payload["feedback"].([]any)
	if !ok || len(feedback) == 0 {
		t.Fatalf("feedback = %#v, want strongest-error-first list", payload["feedback"])
	}
	firstFeedback, ok := feedback[0].(map[string]any)
	if !ok {
		t.Fatalf("feedback[0] = %#v, want object", feedback[0])
	}
	if stringValue(firstFeedback["node_id"]) != "n4" || stringValue(firstFeedback["severity"]) != "error" {
		t.Fatalf("feedback[0] = %#v, want error-ranked prediction failure", firstFeedback)
	}

	nodeHints, ok := payload["node_hints"].([]any)
	if !ok {
		t.Fatalf("node_hints = %#v, want array", payload["node_hints"])
	}
	hintsByID := map[string]map[string]any{}
	for _, raw := range nodeHints {
		hint, ok := raw.(map[string]any)
		if !ok {
			t.Fatalf("node_hints entry = %#v, want object", raw)
		}
		hintsByID[stringValue(hint["node_id"])] = hint
	}
	if got := hintsByID["n1"]; stringValue(got["node_verdict"]) != "supported" || stringValue(got["driver_role"]) != "primary" {
		t.Fatalf("hint[n1] = %#v, want supported primary driver", got)
	}
	if got := hintsByID["n2"]; stringValue(got["node_verdict"]) != "needs_review" || stringValue(got["driver_role"]) != "supporting" {
		t.Fatalf("hint[n2] = %#v, want needs_review supporting driver", got)
	}
}

func TestRunMemoryPosteriorRunMarksOrganizedOutputStaleUntilRefreshRun(t *testing.T) {
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
		now := time.Now().UTC()
		record := c.Record{
			UnitID:         "weibo:Q-posterior-cli",
			Source:         "weibo",
			ExternalID:     "Q-posterior-cli",
			RootExternalID: "Q-posterior-cli",
			Model:          c.Qwen36PlusModel,
			Output: c.Output{
				Summary: "一句话",
				Graph: c.ReasoningGraph{
					Nodes: []c.GraphNode{
						{ID: "n1", Kind: c.NodeFact, Text: "事实A", OccurredAt: now.Add(-72 * time.Hour)},
						{ID: "n2", Kind: c.NodePrediction, Text: "预测B", PredictionStartAt: now.Add(-48 * time.Hour), PredictionDueAt: now.Add(-24 * time.Hour)},
					},
					Edges: []c.GraphEdge{{From: "n1", To: "n2", Kind: c.EdgeDerives}},
				},
				Details: c.HiddenDetails{Caveats: []string{"说明"}},
				Verification: c.Verification{
					PredictionChecks: []c.PredictionCheck{{
						NodeID: "n2", Status: c.PredictionStatusStaleUnresolved, Reason: "window passed", AsOf: now.Add(-12 * time.Hour),
					}},
				},
			},
			CompiledAt: now.Add(-6 * time.Hour),
		}
		if err := store.UpsertCompiledOutput(context.Background(), record); err != nil {
			return nil, err
		}
		if _, err := store.AcceptMemoryNodes(context.Background(), memory.AcceptRequest{
			UserID:           "u-posterior-cli",
			SourcePlatform:   "weibo",
			SourceExternalID: "Q-posterior-cli",
			NodeIDs:          []string{"n1", "n2"},
		}); err != nil {
			return nil, err
		}
		return store, nil
	}

	var stdout, stderr bytes.Buffer
	code := run([]string{"memory", "organize-run", "--user", "u-posterior-cli"}, "/tmp/project", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("organize-run code = %d, stderr = %s", code, stderr.String())
	}

	stdout.Reset()
	stderr.Reset()
	code = run([]string{"memory", "posterior-run", "--user", "u-posterior-cli", "--platform", "weibo", "--id", "Q-posterior-cli"}, "/tmp/project", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("posterior-run code = %d, stderr = %s", code, stderr.String())
	}
	var posterior memory.PosteriorRunResult
	if err := json.Unmarshal(stdout.Bytes(), &posterior); err != nil {
		t.Fatalf("json.Unmarshal(posterior-run) error = %v", err)
	}
	if len(posterior.Mutated) != 1 || posterior.Mutated[0].NodeID != "n2" {
		t.Fatalf("posterior mutated = %#v, want one mutated prediction node", posterior.Mutated)
	}
	if posterior.Mutated[0].State != memory.PosteriorStatePending {
		t.Fatalf("posterior state = %q, want pending", posterior.Mutated[0].State)
	}
	if len(posterior.Refreshes) != 1 || posterior.Refreshes[0].JobID == 0 {
		t.Fatalf("posterior refreshes = %#v, want one queued refresh job", posterior.Refreshes)
	}

	stdout.Reset()
	stderr.Reset()
	code = run([]string{"memory", "organized", "--user", "u-posterior-cli", "--platform", "weibo", "--id", "Q-posterior-cli"}, "/tmp/project", &stdout, &stderr)
	if code != 1 {
		t.Fatalf("organized stale code = %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), "memory organization output is stale") {
		t.Fatalf("stderr = %q, want stale output error", stderr.String())
	}
	if !strings.Contains(stderr.String(), "memory organize-run --user u-posterior-cli") {
		t.Fatalf("stderr = %q, want rerun guidance", stderr.String())
	}

	stdout.Reset()
	stderr.Reset()
	code = run([]string{"memory", "organize-run", "--user", "u-posterior-cli"}, "/tmp/project", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("refresh organize-run code = %d, stderr = %s", code, stderr.String())
	}

	stdout.Reset()
	stderr.Reset()
	code = run([]string{"memory", "organized", "--user", "u-posterior-cli", "--platform", "weibo", "--id", "Q-posterior-cli"}, "/tmp/project", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("organized refreshed code = %d, stderr = %s", code, stderr.String())
	}
	var organized memory.OrganizationOutput
	if err := json.Unmarshal(stdout.Bytes(), &organized); err != nil {
		t.Fatalf("json.Unmarshal(refreshed organized) error = %v", err)
	}
	foundPosteriorHint := false
	for _, hint := range organized.NodeHints {
		if hint.NodeID == "n2" && hint.PosteriorState == memory.PosteriorStatePending {
			foundPosteriorHint = true
			break
		}
	}
	if !foundPosteriorHint {
		t.Fatalf("NodeHints = %#v, want posterior pending hint for n2", organized.NodeHints)
	}
}

func TestRunMemoryOrganizedWithoutOutputShowsRunGuidance(t *testing.T) {
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
		return contentstore.NewSQLiteStore(path)
	}

	var stdout, stderr bytes.Buffer
	code := run([]string{"memory", "organized", "--user", "u-empty-memory", "--platform", "weibo", "--id", "Q-empty"}, "/tmp/project", &stdout, &stderr)
	if code != 1 {
		t.Fatalf("organized code = %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), "memory organize-run --user u-empty-memory") {
		t.Fatalf("stderr = %q, want organize-run guidance", stderr.String())
	}
}

func TestRunMemoryGlobalOrganizeRunAndShow(t *testing.T) {
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
		recordA := c.Record{
			UnitID:         "weibo:G1",
			Source:         "weibo",
			ExternalID:     "G1",
			RootExternalID: "G1",
			Model:          c.Qwen36PlusModel,
			Output: c.Output{
				Summary: "一句话",
				Graph: c.ReasoningGraph{
					Nodes: []c.GraphNode{
						{ID: "n1", Kind: c.NodeFact, Text: "油价会上升", OccurredAt: time.Date(2026, 4, 10, 0, 0, 0, 0, time.UTC)},
						{ID: "n2", Kind: c.NodeExplicitCondition, Text: "若地缘冲突升级"},
					},
					Edges: []c.GraphEdge{{From: "n1", To: "n2", Kind: c.EdgePositive}},
				},
				Details: c.HiddenDetails{Caveats: []string{"说明"}},
			},
			CompiledAt: time.Now().UTC(),
		}
		recordB := c.Record{
			UnitID:         "twitter:G2",
			Source:         "twitter",
			ExternalID:     "G2",
			RootExternalID: "G2",
			Model:          c.Qwen36PlusModel,
			Output: c.Output{
				Summary: "一句话",
				Graph: c.ReasoningGraph{
					Nodes: []c.GraphNode{
						{ID: "n1", Kind: c.NodeFact, Text: "油价会下降", OccurredAt: time.Date(2026, 4, 11, 0, 0, 0, 0, time.UTC)},
						{ID: "n2", Kind: c.NodeConclusion, Text: "结论C"},
					},
					Edges: []c.GraphEdge{{From: "n1", To: "n2", Kind: c.EdgePositive}},
				},
				Details: c.HiddenDetails{Caveats: []string{"说明"}},
			},
			CompiledAt: time.Now().UTC(),
		}
		if err := store.UpsertCompiledOutput(context.Background(), recordA); err != nil {
			return nil, err
		}
		if err := store.UpsertCompiledOutput(context.Background(), recordB); err != nil {
			return nil, err
		}
		if _, err := store.AcceptMemoryNodes(context.Background(), memory.AcceptRequest{UserID: "u-global", SourcePlatform: "weibo", SourceExternalID: "G1", NodeIDs: []string{"n1", "n2"}}); err != nil {
			return nil, err
		}
		if _, err := store.AcceptMemoryNodes(context.Background(), memory.AcceptRequest{UserID: "u-global", SourcePlatform: "twitter", SourceExternalID: "G2", NodeIDs: []string{"n1"}}); err != nil {
			return nil, err
		}
		return store, nil
	}

	var stdout, stderr bytes.Buffer
	code := run([]string{"memory", "global-organize-run", "--user", "u-global"}, "/tmp/project", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("global-organize-run code = %d, stderr = %s", code, stderr.String())
	}
	var out memory.GlobalOrganizationOutput
	if err := json.Unmarshal(stdout.Bytes(), &out); err != nil {
		t.Fatalf("json.Unmarshal(global-organize-run) error = %v", err)
	}
	if len(out.Clusters) == 0 {
		t.Fatalf("output = %#v, want clusters", out)
	}

	stdout.Reset()
	stderr.Reset()
	code = run([]string{"memory", "global-organized", "--user", "u-global"}, "/tmp/project", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("global-organized code = %d, stderr = %s", code, stderr.String())
	}
	var got memory.GlobalOrganizationOutput
	if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
		t.Fatalf("json.Unmarshal(global-organized) error = %v", err)
	}
	if got.OutputID == 0 || len(got.Clusters) == 0 {
		t.Fatalf("global output = %#v", got)
	}
	foundNeutral := false
	for _, cluster := range got.Clusters {
		if strings.Contains(cluster.CanonicalProposition, "关于「") {
			foundNeutral = true
			break
		}
	}
	if !foundNeutral {
		t.Fatalf("clusters = %#v, want at least one neutral contradiction-centered proposition", got.Clusters)
	}
}

func TestRunMemoryGlobalCardPrintsClusterSections(t *testing.T) {
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
		record := c.Record{
			UnitID:         "weibo:GC1",
			Source:         "weibo",
			ExternalID:     "GC1",
			RootExternalID: "GC1",
			Model:          c.Qwen36PlusModel,
			Output: c.Output{
				Summary: "一句话",
				Graph: c.ReasoningGraph{
					Nodes: []c.GraphNode{
						{ID: "n1", Kind: c.NodeFact, Text: "事实A", OccurredAt: time.Date(2026, 4, 10, 0, 0, 0, 0, time.UTC)},
						{ID: "n2", Kind: c.NodeConclusion, Text: "结论B"},
						{ID: "n3", Kind: c.NodePrediction, Text: "预测C", PredictionStartAt: time.Date(2026, 4, 10, 0, 0, 0, 0, time.UTC)},
					},
					Edges: []c.GraphEdge{{From: "n1", To: "n2", Kind: c.EdgeDerives}, {From: "n2", To: "n3", Kind: c.EdgeDerives}},
				},
				Details: c.HiddenDetails{Caveats: []string{"说明"}},
			},
			CompiledAt: time.Now().UTC(),
		}
		if err := store.UpsertCompiledOutput(context.Background(), record); err != nil {
			return nil, err
		}
		if _, err := store.AcceptMemoryNodes(context.Background(), memory.AcceptRequest{UserID: "u-card", SourcePlatform: "weibo", SourceExternalID: "GC1", NodeIDs: []string{"n1", "n2", "n3"}}); err != nil {
			return nil, err
		}
		if _, err := store.RunGlobalMemoryOrganization(context.Background(), "u-card", time.Now().UTC()); err != nil {
			return nil, err
		}
		return store, nil
	}

	var stdout, stderr bytes.Buffer
	code := run([]string{"memory", "global-card", "--user", "u-card"}, "/tmp/project", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("global-card code = %d, stderr = %s", code, stderr.String())
	}
	out := stdout.String()
	for _, want := range []string{"Cluster", "事实A", "Current judgment", "结论B", "What next", "预测C"} {
		if !strings.Contains(out, want) {
			t.Fatalf("stdout missing %q in %q", want, out)
		}
	}
}

func TestRunMemoryGlobalV2OrganizeAndShow(t *testing.T) {
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
		record := c.Record{
			UnitID:         "weibo:GV2",
			Source:         "weibo",
			ExternalID:     "GV2",
			RootExternalID: "GV2",
			Model:          c.Qwen36PlusModel,
			Output: c.Output{
				Summary: "一句话",
				Graph: c.ReasoningGraph{
					Nodes: []c.GraphNode{
						{ID: "n1", Kind: c.NodeFact, Text: "流动性收紧", OccurredAt: time.Date(2026, 4, 10, 0, 0, 0, 0, time.UTC)},
						{ID: "n2", Kind: c.NodeConclusion, Text: "风险资产承压"},
						{ID: "n3", Kind: c.NodePrediction, Text: "未来数月波动加大", PredictionStartAt: time.Date(2026, 4, 10, 0, 0, 0, 0, time.UTC)},
					},
					Edges: []c.GraphEdge{{From: "n1", To: "n2", Kind: c.EdgeDerives}, {From: "n2", To: "n3", Kind: c.EdgeDerives}},
				},
				Details: c.HiddenDetails{Caveats: []string{"说明"}},
			},
			CompiledAt: time.Now().UTC(),
		}
		if err := store.UpsertCompiledOutput(context.Background(), record); err != nil {
			return nil, err
		}
		if _, err := store.AcceptMemoryNodes(context.Background(), memory.AcceptRequest{UserID: "u-v2-cli", SourcePlatform: "weibo", SourceExternalID: "GV2", NodeIDs: []string{"n1", "n2", "n3"}}); err != nil {
			return nil, err
		}
		return store, nil
	}

	var stdout, stderr bytes.Buffer
	code := run([]string{"memory", "global-v2-organize-run", "--user", "u-v2-cli"}, "/tmp/project", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("global-v2-organize-run code = %d, stderr = %s", code, stderr.String())
	}
	var out memory.GlobalMemoryV2Output
	if err := json.Unmarshal(stdout.Bytes(), &out); err != nil {
		t.Fatalf("json.Unmarshal(global-v2-organize-run) error = %v", err)
	}
	if len(out.CognitiveCards) == 0 || len(out.TopMemoryItems) == 0 {
		t.Fatalf("v2 output = %#v, want cards + top items", out)
	}

	stdout.Reset()
	stderr.Reset()
	code = run([]string{"memory", "global-v2-organized", "--user", "u-v2-cli"}, "/tmp/project", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("global-v2-organized code = %d, stderr = %s", code, stderr.String())
	}
	var got memory.GlobalMemoryV2Output
	if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
		t.Fatalf("json.Unmarshal(global-v2-organized) error = %v", err)
	}
	if got.OutputID == 0 || len(got.CognitiveConclusions) == 0 {
		t.Fatalf("v2 stored output = %#v, want persisted v2 result", got)
	}
}

func TestRunMemoryGlobalV2CardShowsItemCountHeader(t *testing.T) {
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
		record := c.Record{
			UnitID: "weibo:COUNT1", Source: "weibo", ExternalID: "COUNT1", RootExternalID: "COUNT1", Model: c.Qwen36PlusModel,
			Output: c.Output{Summary: "一句话", Graph: c.ReasoningGraph{Nodes: []c.GraphNode{
				{ID: "n1", Kind: c.NodeFact, Text: "流动性收紧", OccurredAt: time.Date(2026, 4, 10, 0, 0, 0, 0, time.UTC)},
				{ID: "n2", Kind: c.NodeConclusion, Text: "风险资产承压"},
				{ID: "n3", Kind: c.NodePrediction, Text: "未来数月波动加大", PredictionStartAt: time.Date(2026, 4, 10, 0, 0, 0, 0, time.UTC)},
			}, Edges: []c.GraphEdge{{From: "n1", To: "n2", Kind: c.EdgeDerives}, {From: "n2", To: "n3", Kind: c.EdgeDerives}}}, Details: c.HiddenDetails{Caveats: []string{"说明"}}},
			CompiledAt: time.Now().UTC(),
		}
		if err := store.UpsertCompiledOutput(context.Background(), record); err != nil {
			return nil, err
		}
		if _, err := store.AcceptMemoryNodes(context.Background(), memory.AcceptRequest{UserID: "u-v2-count", SourcePlatform: "weibo", SourceExternalID: "COUNT1", NodeIDs: []string{"n1", "n2", "n3"}}); err != nil {
			return nil, err
		}
		if _, err := store.RunGlobalMemoryOrganizationV2(context.Background(), "u-v2-count", time.Now().UTC()); err != nil {
			return nil, err
		}
		return store, nil
	}

	var stdout, stderr bytes.Buffer
	code := run([]string{"memory", "global-v2-card", "--user", "u-v2-count"}, "/tmp/project", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("global-v2-card code = %d, stderr = %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "Items\n1") {
		t.Fatalf("stdout = %q, want item count header", stdout.String())
	}
}

func TestRunMemoryGlobalV2CardPrintsConclusionSections(t *testing.T) {
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
		record := c.Record{
			UnitID:         "weibo:GV2C",
			Source:         "weibo",
			ExternalID:     "GV2C",
			RootExternalID: "GV2C",
			Model:          c.Qwen36PlusModel,
			Output: c.Output{
				Summary: "一句话",
				Graph: c.ReasoningGraph{
					Nodes: []c.GraphNode{
						{ID: "n1", Kind: c.NodeFact, Text: "流动性收紧", OccurredAt: time.Date(2026, 4, 10, 0, 0, 0, 0, time.UTC)},
						{ID: "n2", Kind: c.NodeConclusion, Text: "风险资产承压"},
						{ID: "n3", Kind: c.NodePrediction, Text: "未来数月波动加大", PredictionStartAt: time.Date(2026, 4, 10, 0, 0, 0, 0, time.UTC)},
						{ID: "n4", Kind: c.NodeExplicitCondition, Text: "若融资环境继续恶化"},
					},
					Edges: []c.GraphEdge{
						{From: "n1", To: "n2", Kind: c.EdgeDerives},
						{From: "n2", To: "n3", Kind: c.EdgeDerives},
						{From: "n4", To: "n3", Kind: c.EdgePresets},
					},
				},
				Details: c.HiddenDetails{Caveats: []string{"说明"}},
			},
			CompiledAt: time.Now().UTC(),
		}
		if err := store.UpsertCompiledOutput(context.Background(), record); err != nil {
			return nil, err
		}
		if _, err := store.AcceptMemoryNodes(context.Background(), memory.AcceptRequest{UserID: "u-v2-card", SourcePlatform: "weibo", SourceExternalID: "GV2C", NodeIDs: []string{"n1", "n2", "n3", "n4"}}); err != nil {
			return nil, err
		}
		if _, err := store.RunGlobalMemoryOrganizationV2(context.Background(), "u-v2-card", time.Now().UTC()); err != nil {
			return nil, err
		}
		return store, nil
	}

	var stdout, stderr bytes.Buffer
	code := run([]string{"memory", "global-v2-card", "--user", "u-v2-card"}, "/tmp/project", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("global-v2-card code = %d, stderr = %s", code, stderr.String())
	}
	out := stdout.String()
	for _, want := range []string{"Conclusion", "Signal", "Logic", "Why", "Conditions", "What next", "Sources", "weibo:GV2C", "流动性收紧", "若融资环境继续恶化", "未来数月波动加大"} {
		if !strings.Contains(out, want) {
			t.Fatalf("stdout missing %q in %q", want, out)
		}
	}
}

func TestRunMemoryGlobalV2CardPrintsMechanismSection(t *testing.T) {
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
		record := c.Record{
			UnitID: "weibo:MECH1", Source: "weibo", ExternalID: "MECH1", RootExternalID: "MECH1", Model: c.Qwen36PlusModel,
			Output: c.Output{Summary: "一句话", Graph: c.ReasoningGraph{Nodes: []c.GraphNode{
				{ID: "n1", Kind: c.NodeFact, Text: "高资产价格环境延续", OccurredAt: time.Date(2026, 4, 10, 0, 0, 0, 0, time.UTC)},
				{ID: "n2", Kind: c.NodeImplicitCondition, Text: "宏观负面冲击会放大金融系统脆弱性", OccurredAt: time.Date(2026, 4, 10, 0, 0, 0, 0, time.UTC)},
				{ID: "n3", Kind: c.NodeConclusion, Text: "风险资产承压"},
				{ID: "n4", Kind: c.NodePrediction, Text: "未来数月波动加大", PredictionStartAt: time.Date(2026, 4, 10, 0, 0, 0, 0, time.UTC)},
			}, Edges: []c.GraphEdge{{From: "n1", To: "n2", Kind: c.EdgeDerives}, {From: "n2", To: "n3", Kind: c.EdgeDerives}, {From: "n3", To: "n4", Kind: c.EdgeDerives}}}, Details: c.HiddenDetails{Caveats: []string{"说明"}}},
			CompiledAt: time.Now().UTC(),
		}
		if err := store.UpsertCompiledOutput(context.Background(), record); err != nil {
			return nil, err
		}
		if _, err := store.AcceptMemoryNodes(context.Background(), memory.AcceptRequest{UserID: "u-v2-mech", SourcePlatform: "weibo", SourceExternalID: "MECH1", NodeIDs: []string{"n1", "n2", "n3", "n4"}}); err != nil {
			return nil, err
		}
		if _, err := store.RunGlobalMemoryOrganizationV2(context.Background(), "u-v2-mech", time.Now().UTC()); err != nil {
			return nil, err
		}
		return store, nil
	}

	var stdout, stderr bytes.Buffer
	code := run([]string{"memory", "global-v2-card", "--user", "u-v2-mech"}, "/tmp/project", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("global-v2-card code = %d, stderr = %s", code, stderr.String())
	}
	out := stdout.String()
	for _, want := range []string{"Mechanism", "宏观负面冲击会放大金融系统脆弱性"} {
		if !strings.Contains(out, want) {
			t.Fatalf("stdout missing %q in %q", want, out)
		}
	}
	whyStart := strings.Index(out, "Why\n")
	if whyStart == -1 {
		t.Fatalf("stdout = %q, want Why section", out)
	}
	nextStart := strings.Index(out[whyStart+4:], "\n\nWhat next")
	if nextStart == -1 {
		t.Fatalf("stdout = %q, want What next section after Why", out)
	}
	whyBlock := out[whyStart : whyStart+4+nextStart]
	if strings.Contains(whyBlock, "宏观负面冲击会放大金融系统脆弱性") {
		t.Fatalf("Why section should not repeat mechanism text: %q", whyBlock)
	}
}

func TestRunMemoryGlobalV2CardRunFlagBuildsFreshOutput(t *testing.T) {
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
		record := c.Record{
			UnitID:         "weibo:GV2R",
			Source:         "weibo",
			ExternalID:     "GV2R",
			RootExternalID: "GV2R",
			Model:          c.Qwen36PlusModel,
			Output: c.Output{
				Summary: "一句话",
				Graph: c.ReasoningGraph{
					Nodes: []c.GraphNode{
						{ID: "n1", Kind: c.NodeFact, Text: "流动性收紧", OccurredAt: time.Date(2026, 4, 10, 0, 0, 0, 0, time.UTC)},
						{ID: "n2", Kind: c.NodeConclusion, Text: "风险资产承压"},
						{ID: "n3", Kind: c.NodePrediction, Text: "未来数月波动加大", PredictionStartAt: time.Date(2026, 4, 10, 0, 0, 0, 0, time.UTC)},
					},
					Edges: []c.GraphEdge{{From: "n1", To: "n2", Kind: c.EdgeDerives}, {From: "n2", To: "n3", Kind: c.EdgeDerives}},
				},
				Details: c.HiddenDetails{Caveats: []string{"说明"}},
			},
			CompiledAt: time.Now().UTC(),
		}
		if err := store.UpsertCompiledOutput(context.Background(), record); err != nil {
			return nil, err
		}
		if _, err := store.AcceptMemoryNodes(context.Background(), memory.AcceptRequest{UserID: "u-v2-card-run", SourcePlatform: "weibo", SourceExternalID: "GV2R", NodeIDs: []string{"n1", "n2", "n3"}}); err != nil {
			return nil, err
		}
		return store, nil
	}

	var stdout, stderr bytes.Buffer
	code := run([]string{"memory", "global-v2-card", "--run", "--user", "u-v2-card-run"}, "/tmp/project", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("global-v2-card --run code = %d, stderr = %s", code, stderr.String())
	}
	out := stdout.String()
	for _, want := range []string{"Conclusion", "Signal", "Logic", "流动性收紧", "风险资产承压"} {
		if !strings.Contains(out, want) {
			t.Fatalf("stdout missing %q in %q", want, out)
		}
	}
}

func TestRunMemoryGlobalV2CardSuggestsRunWhenNoStoredOutput(t *testing.T) {
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
		return store, nil
	}

	var stdout, stderr bytes.Buffer
	code := run([]string{"memory", "global-v2-card", "--user", "u-empty"}, "/tmp/project", &stdout, &stderr)
	if code != 1 {
		t.Fatalf("global-v2-card code = %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), "memory global-v2-card --run --user u-empty") {
		t.Fatalf("stderr = %q, want --run guidance", stderr.String())
	}
}

func TestRunMemoryGlobalV2OrganizedSuggestsOrganizeRunWhenEmpty(t *testing.T) {
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
		return store, nil
	}

	var stdout, stderr bytes.Buffer
	code := run([]string{"memory", "global-v2-organized", "--user", "u-empty"}, "/tmp/project", &stdout, &stderr)
	if code != 1 {
		t.Fatalf("global-v2-organized code = %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), "memory global-v2-organize-run --user u-empty") {
		t.Fatalf("stderr = %q, want organize-run guidance", stderr.String())
	}
}

func TestRunMemoryGlobalV2CardPrintsConflictSides(t *testing.T) {
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
		recordA := c.Record{
			UnitID:         "weibo:CF1",
			Source:         "weibo",
			ExternalID:     "CF1",
			RootExternalID: "CF1",
			Model:          c.Qwen36PlusModel,
			Output: c.Output{
				Summary: "一句话",
				Graph: c.ReasoningGraph{
					Nodes: []c.GraphNode{
						{ID: "n1", Kind: c.NodeFact, Text: "供给趋紧", OccurredAt: time.Date(2026, 4, 10, 0, 0, 0, 0, time.UTC)},
						{ID: "n2", Kind: c.NodeConclusion, Text: "油价会上升"},
					},
					Edges: []c.GraphEdge{{From: "n1", To: "n2", Kind: c.EdgeDerives}},
				},
				Details: c.HiddenDetails{Caveats: []string{"说明"}},
			},
			CompiledAt: time.Now().UTC(),
		}
		recordB := c.Record{
			UnitID:         "twitter:CF2",
			Source:         "twitter",
			ExternalID:     "CF2",
			RootExternalID: "CF2",
			Model:          c.Qwen36PlusModel,
			Output: c.Output{
				Summary: "一句话",
				Graph: c.ReasoningGraph{
					Nodes: []c.GraphNode{
						{ID: "n1", Kind: c.NodeFact, Text: "需求走弱", OccurredAt: time.Date(2026, 4, 10, 0, 0, 0, 0, time.UTC)},
						{ID: "n2", Kind: c.NodeConclusion, Text: "油价会下降"},
					},
					Edges: []c.GraphEdge{{From: "n1", To: "n2", Kind: c.EdgeDerives}},
				},
				Details: c.HiddenDetails{Caveats: []string{"说明"}},
			},
			CompiledAt: time.Now().UTC(),
		}
		if err := store.UpsertCompiledOutput(context.Background(), recordA); err != nil {
			return nil, err
		}
		if err := store.UpsertCompiledOutput(context.Background(), recordB); err != nil {
			return nil, err
		}
		if _, err := store.AcceptMemoryNodes(context.Background(), memory.AcceptRequest{UserID: "u-v2-conflict-card", SourcePlatform: "weibo", SourceExternalID: "CF1", NodeIDs: []string{"n2"}}); err != nil {
			return nil, err
		}
		if _, err := store.AcceptMemoryNodes(context.Background(), memory.AcceptRequest{UserID: "u-v2-conflict-card", SourcePlatform: "twitter", SourceExternalID: "CF2", NodeIDs: []string{"n2"}}); err != nil {
			return nil, err
		}
		if _, err := store.RunGlobalMemoryOrganizationV2(context.Background(), "u-v2-conflict-card", time.Now().UTC()); err != nil {
			return nil, err
		}
		return store, nil
	}

	var stdout, stderr bytes.Buffer
	code := run([]string{"memory", "global-v2-card", "--user", "u-v2-conflict-card"}, "/tmp/project", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("global-v2-card code = %d, stderr = %s", code, stderr.String())
	}
	out := stdout.String()
	for _, want := range []string{"Conflict", "Side A", "Side B", "Why A", "Why B", "Sources A", "Sources B", "weibo:CF1", "twitter:CF2", "油价会上升", "油价会下降"} {
		if !strings.Contains(out, want) {
			t.Fatalf("stdout missing %q in %q", want, out)
		}
	}
}

func TestRunMemoryGlobalV2CardFiltersByItemType(t *testing.T) {
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
		recordA := c.Record{
			UnitID: "weibo:F1", Source: "weibo", ExternalID: "F1", RootExternalID: "F1", Model: c.Qwen36PlusModel,
			Output: c.Output{Summary: "s", Graph: c.ReasoningGraph{Nodes: []c.GraphNode{
				{ID: "n1", Kind: c.NodeFact, Text: "流动性收紧", OccurredAt: time.Date(2026, 4, 10, 0, 0, 0, 0, time.UTC)},
				{ID: "n2", Kind: c.NodeConclusion, Text: "风险资产承压"},
				{ID: "n3", Kind: c.NodePrediction, Text: "未来数月波动加大", PredictionStartAt: time.Date(2026, 4, 10, 0, 0, 0, 0, time.UTC)},
			}, Edges: []c.GraphEdge{{From: "n1", To: "n2", Kind: c.EdgeDerives}, {From: "n2", To: "n3", Kind: c.EdgeDerives}}}, Details: c.HiddenDetails{Caveats: []string{"说明"}}},
			CompiledAt: time.Now().UTC(),
		}
		recordB := c.Record{
			UnitID: "weibo:F2", Source: "weibo", ExternalID: "F2", RootExternalID: "F2", Model: c.Qwen36PlusModel,
			Output: c.Output{Summary: "s", Graph: c.ReasoningGraph{Nodes: []c.GraphNode{
				{ID: "n1", Kind: c.NodeFact, Text: "供给趋紧", OccurredAt: time.Date(2026, 4, 10, 0, 0, 0, 0, time.UTC)},
				{ID: "n2", Kind: c.NodeConclusion, Text: "油价会上升"},
			}, Edges: []c.GraphEdge{{From: "n1", To: "n2", Kind: c.EdgeDerives}}}, Details: c.HiddenDetails{Caveats: []string{"说明"}}},
			CompiledAt: time.Now().UTC(),
		}
		recordC := c.Record{
			UnitID: "twitter:F3", Source: "twitter", ExternalID: "F3", RootExternalID: "F3", Model: c.Qwen36PlusModel,
			Output: c.Output{Summary: "s", Graph: c.ReasoningGraph{Nodes: []c.GraphNode{
				{ID: "n1", Kind: c.NodeFact, Text: "需求走弱", OccurredAt: time.Date(2026, 4, 10, 0, 0, 0, 0, time.UTC)},
				{ID: "n2", Kind: c.NodeConclusion, Text: "油价会下降"},
			}, Edges: []c.GraphEdge{{From: "n1", To: "n2", Kind: c.EdgeDerives}}}, Details: c.HiddenDetails{Caveats: []string{"说明"}}},
			CompiledAt: time.Now().UTC(),
		}
		for _, record := range []c.Record{recordA, recordB, recordC} {
			if err := store.UpsertCompiledOutput(context.Background(), record); err != nil {
				return nil, err
			}
		}
		for _, req := range []memory.AcceptRequest{
			{UserID: "u-v2-filter", SourcePlatform: "weibo", SourceExternalID: "F1", NodeIDs: []string{"n1", "n2", "n3"}},
			{UserID: "u-v2-filter", SourcePlatform: "weibo", SourceExternalID: "F2", NodeIDs: []string{"n2"}},
			{UserID: "u-v2-filter", SourcePlatform: "twitter", SourceExternalID: "F3", NodeIDs: []string{"n2"}},
		} {
			if _, err := store.AcceptMemoryNodes(context.Background(), req); err != nil {
				return nil, err
			}
		}
		if _, err := store.RunGlobalMemoryOrganizationV2(context.Background(), "u-v2-filter", time.Now().UTC()); err != nil {
			return nil, err
		}
		return store, nil
	}

	var stdout, stderr bytes.Buffer
	code := run([]string{"memory", "global-v2-card", "--user", "u-v2-filter", "--item-type", "conflict"}, "/tmp/project", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("conflict filter code = %d, stderr = %s", code, stderr.String())
	}
	if strings.Contains(stdout.String(), "Conclusion") || !strings.Contains(stdout.String(), "Conflict") {
		t.Fatalf("conflict-only stdout = %q, want only conflict items", stdout.String())
	}
	if !strings.Contains(stdout.String(), "Items (1, filter=conflict)") {
		t.Fatalf("stdout = %q, want item header with filter context", stdout.String())
	}

	stdout.Reset()
	stderr.Reset()
	code = run([]string{"memory", "global-v2-card", "--user", "u-v2-filter", "--item-type", "conclusion"}, "/tmp/project", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("conclusion filter code = %d, stderr = %s", code, stderr.String())
	}
	if strings.Contains(stdout.String(), "Conflict") || !strings.Contains(stdout.String(), "Conclusion") {
		t.Fatalf("conclusion-only stdout = %q, want only conclusion items", stdout.String())
	}
	if !strings.Contains(stdout.String(), "Items (1, filter=conclusion)") {
		t.Fatalf("stdout = %q, want item header with filter context", stdout.String())
	}
}

func TestRunMemoryGlobalV2CardRejectsInvalidItemType(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{"memory", "global-v2-card", "--user", "u-any", "--item-type", "foo"}, "/tmp/project", &stdout, &stderr)
	if code != 2 {
		t.Fatalf("invalid item-type code = %d, want 2", code)
	}
	if !strings.Contains(stderr.String(), "item-type must be one of: card, conclusion, conflict") {
		t.Fatalf("stderr = %q, want explicit item-type guidance", stderr.String())
	}
}

func TestRunMemoryGlobalV2CardPrintsStandaloneCardItems(t *testing.T) {
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
		record := c.Record{
			UnitID:         "weibo:CardOnly1",
			Source:         "weibo",
			ExternalID:     "CardOnly1",
			RootExternalID: "CardOnly1",
			Model:          c.Qwen36PlusModel,
			Output: c.Output{
				Summary: "s",
				Graph: c.ReasoningGraph{
					Nodes: []c.GraphNode{
						{ID: "n1", Kind: c.NodeFact, Text: "事实A", OccurredAt: time.Date(2026, 4, 10, 0, 0, 0, 0, time.UTC)},
						{ID: "n2", Kind: c.NodeConclusion, Text: "结论B"},
					},
					Edges: []c.GraphEdge{{From: "n1", To: "n2", Kind: c.EdgeDerives}},
				},
				Details: c.HiddenDetails{Caveats: []string{"说明"}},
			},
			CompiledAt: time.Now().UTC(),
		}
		if err := store.UpsertCompiledOutput(context.Background(), record); err != nil {
			return nil, err
		}
		if _, err := store.AcceptMemoryNodes(context.Background(), memory.AcceptRequest{
			UserID:           "u-v2-card-only",
			SourcePlatform:   "weibo",
			SourceExternalID: "CardOnly1",
			NodeIDs:          []string{"n1"},
		}); err != nil {
			return nil, err
		}
		if _, err := store.RunGlobalMemoryOrganizationV2(context.Background(), "u-v2-card-only", time.Now().UTC()); err != nil {
			return nil, err
		}
		return store, nil
	}

	var stdout, stderr bytes.Buffer
	code := run([]string{"memory", "global-v2-card", "--user", "u-v2-card-only", "--item-type", "card"}, "/tmp/project", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("global-v2-card card filter code = %d, stderr = %s", code, stderr.String())
	}
	out := stdout.String()
	for _, want := range []string{"Card", "事实A", "Logic", "Why", "Items (1, filter=card)"} {
		if !strings.Contains(out, want) {
			t.Fatalf("stdout missing %q in %q", want, out)
		}
	}
}

func TestRunMemoryGlobalV2CardReportsWhenFilterMatchesNothing(t *testing.T) {
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
		record := c.Record{
			UnitID: "weibo:N1", Source: "weibo", ExternalID: "N1", RootExternalID: "N1", Model: c.Qwen36PlusModel,
			Output: c.Output{Summary: "s", Graph: c.ReasoningGraph{Nodes: []c.GraphNode{
				{ID: "n1", Kind: c.NodeFact, Text: "流动性收紧", OccurredAt: time.Date(2026, 4, 10, 0, 0, 0, 0, time.UTC)},
				{ID: "n2", Kind: c.NodeConclusion, Text: "风险资产承压"},
			}, Edges: []c.GraphEdge{{From: "n1", To: "n2", Kind: c.EdgeDerives}}}, Details: c.HiddenDetails{Caveats: []string{"说明"}}},
			CompiledAt: time.Now().UTC(),
		}
		if err := store.UpsertCompiledOutput(context.Background(), record); err != nil {
			return nil, err
		}
		if _, err := store.AcceptMemoryNodes(context.Background(), memory.AcceptRequest{UserID: "u-v2-empty-filter", SourcePlatform: "weibo", SourceExternalID: "N1", NodeIDs: []string{"n1", "n2"}}); err != nil {
			return nil, err
		}
		if _, err := store.RunGlobalMemoryOrganizationV2(context.Background(), "u-v2-empty-filter", time.Now().UTC()); err != nil {
			return nil, err
		}
		return store, nil
	}

	var stdout, stderr bytes.Buffer
	code := run([]string{"memory", "global-v2-card", "--user", "u-v2-empty-filter", "--item-type", "conflict"}, "/tmp/project", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("empty filtered code = %d, stderr = %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "No conflict items") {
		t.Fatalf("stdout = %q, want no-match guidance", stdout.String())
	}
	if !strings.Contains(stdout.String(), "Items (0, filter=conflict)") {
		t.Fatalf("stdout = %q, want empty item header with filter context", stdout.String())
	}
}

func TestRunMemoryGlobalV2CardRespectsLimit(t *testing.T) {
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
		recordA := c.Record{
			UnitID: "weibo:L1", Source: "weibo", ExternalID: "L1", RootExternalID: "L1", Model: c.Qwen36PlusModel,
			Output: c.Output{Summary: "s", Graph: c.ReasoningGraph{Nodes: []c.GraphNode{
				{ID: "n1", Kind: c.NodeFact, Text: "流动性收紧", OccurredAt: time.Date(2026, 4, 10, 0, 0, 0, 0, time.UTC)},
				{ID: "n2", Kind: c.NodeConclusion, Text: "风险资产承压"},
			}, Edges: []c.GraphEdge{{From: "n1", To: "n2", Kind: c.EdgeDerives}}}, Details: c.HiddenDetails{Caveats: []string{"说明"}}},
			CompiledAt: time.Now().UTC(),
		}
		recordB := c.Record{
			UnitID: "weibo:L2", Source: "weibo", ExternalID: "L2", RootExternalID: "L2", Model: c.Qwen36PlusModel,
			Output: c.Output{Summary: "s", Graph: c.ReasoningGraph{Nodes: []c.GraphNode{
				{ID: "n1", Kind: c.NodeFact, Text: "供给趋紧", OccurredAt: time.Date(2026, 4, 10, 0, 0, 0, 0, time.UTC)},
				{ID: "n2", Kind: c.NodeConclusion, Text: "油价会上升"},
				{ID: "n3", Kind: c.NodePrediction, Text: "油价冲击扩大", PredictionStartAt: time.Date(2026, 4, 10, 0, 0, 0, 0, time.UTC)},
			}, Edges: []c.GraphEdge{{From: "n1", To: "n2", Kind: c.EdgeDerives}, {From: "n2", To: "n3", Kind: c.EdgeDerives}}}, Details: c.HiddenDetails{Caveats: []string{"说明"}}},
			CompiledAt: time.Now().UTC(),
		}
		for _, record := range []c.Record{recordA, recordB} {
			if err := store.UpsertCompiledOutput(context.Background(), record); err != nil {
				return nil, err
			}
		}
		for _, req := range []memory.AcceptRequest{
			{UserID: "u-v2-limit", SourcePlatform: "weibo", SourceExternalID: "L1", NodeIDs: []string{"n1", "n2"}},
			{UserID: "u-v2-limit", SourcePlatform: "weibo", SourceExternalID: "L2", NodeIDs: []string{"n1", "n2", "n3"}},
		} {
			if _, err := store.AcceptMemoryNodes(context.Background(), req); err != nil {
				return nil, err
			}
		}
		if _, err := store.RunGlobalMemoryOrganizationV2(context.Background(), "u-v2-limit", time.Now().UTC()); err != nil {
			return nil, err
		}
		return store, nil
	}

	var stdout, stderr bytes.Buffer
	code := run([]string{"memory", "global-v2-card", "--user", "u-v2-limit", "--limit", "1"}, "/tmp/project", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("limit code = %d, stderr = %s", code, stderr.String())
	}
	if strings.Count(stdout.String(), "Conclusion\n") != 1 {
		t.Fatalf("stdout = %q, want exactly one rendered card", stdout.String())
	}
}

func TestRunMemoryGlobalCompareShowsV1AndV2Sections(t *testing.T) {
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
		record := c.Record{
			UnitID: "weibo:CMP1", Source: "weibo", ExternalID: "CMP1", RootExternalID: "CMP1", Model: c.Qwen36PlusModel,
			Output: c.Output{Summary: "s", Graph: c.ReasoningGraph{Nodes: []c.GraphNode{
				{ID: "n1", Kind: c.NodeFact, Text: "流动性收紧", OccurredAt: time.Date(2026, 4, 10, 0, 0, 0, 0, time.UTC)},
				{ID: "n2", Kind: c.NodeConclusion, Text: "风险资产承压"},
				{ID: "n3", Kind: c.NodePrediction, Text: "未来数月波动加大", PredictionStartAt: time.Date(2026, 4, 10, 0, 0, 0, 0, time.UTC)},
			}, Edges: []c.GraphEdge{{From: "n1", To: "n2", Kind: c.EdgeDerives}, {From: "n2", To: "n3", Kind: c.EdgeDerives}}}, Details: c.HiddenDetails{Caveats: []string{"说明"}}},
			CompiledAt: time.Now().UTC(),
		}
		if err := store.UpsertCompiledOutput(context.Background(), record); err != nil {
			return nil, err
		}
		if _, err := store.AcceptMemoryNodes(context.Background(), memory.AcceptRequest{UserID: "u-compare", SourcePlatform: "weibo", SourceExternalID: "CMP1", NodeIDs: []string{"n1", "n2", "n3"}}); err != nil {
			return nil, err
		}
		if _, err := store.RunGlobalMemoryOrganization(context.Background(), "u-compare", time.Now().UTC()); err != nil {
			return nil, err
		}
		if _, err := store.RunGlobalMemoryOrganizationV2(context.Background(), "u-compare", time.Now().UTC()); err != nil {
			return nil, err
		}
		return store, nil
	}

	var stdout, stderr bytes.Buffer
	code := run([]string{"memory", "global-compare", "--user", "u-compare"}, "/tmp/project", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("global-compare code = %d, stderr = %s", code, stderr.String())
	}
	out := stdout.String()
	for _, want := range []string{"V1 cluster-first", "V2 thesis-first", "风险资产承压", "未来数月波动加大"} {
		if !strings.Contains(out, want) {
			t.Fatalf("stdout missing %q in %q", want, out)
		}
	}
}

func TestRunMemoryGlobalCompareRunFlagBuildsFreshOutputs(t *testing.T) {
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
		record := c.Record{
			UnitID: "weibo:CMP2", Source: "weibo", ExternalID: "CMP2", RootExternalID: "CMP2", Model: c.Qwen36PlusModel,
			Output: c.Output{Summary: "s", Graph: c.ReasoningGraph{Nodes: []c.GraphNode{
				{ID: "n1", Kind: c.NodeFact, Text: "流动性收紧", OccurredAt: time.Date(2026, 4, 10, 0, 0, 0, 0, time.UTC)},
				{ID: "n2", Kind: c.NodeConclusion, Text: "风险资产承压"},
			}, Edges: []c.GraphEdge{{From: "n1", To: "n2", Kind: c.EdgeDerives}}}, Details: c.HiddenDetails{Caveats: []string{"说明"}}},
			CompiledAt: time.Now().UTC(),
		}
		if err := store.UpsertCompiledOutput(context.Background(), record); err != nil {
			return nil, err
		}
		if _, err := store.AcceptMemoryNodes(context.Background(), memory.AcceptRequest{UserID: "u-compare-run", SourcePlatform: "weibo", SourceExternalID: "CMP2", NodeIDs: []string{"n1", "n2"}}); err != nil {
			return nil, err
		}
		return store, nil
	}

	var stdout, stderr bytes.Buffer
	code := run([]string{"memory", "global-compare", "--run", "--user", "u-compare-run"}, "/tmp/project", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("global-compare --run code = %d, stderr = %s", code, stderr.String())
	}
	out := stdout.String()
	for _, want := range []string{"V1 cluster-first", "V2 thesis-first", "风险资产承压"} {
		if !strings.Contains(out, want) {
			t.Fatalf("stdout missing %q in %q", want, out)
		}
	}
}

func TestRunMemoryGlobalCompareRespectsLimit(t *testing.T) {
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
		records := []c.Record{
			{UnitID: "weibo:CL1", Source: "weibo", ExternalID: "CL1", RootExternalID: "CL1", Model: c.Qwen36PlusModel, Output: c.Output{Summary: "s", Graph: c.ReasoningGraph{Nodes: []c.GraphNode{{ID: "n1", Kind: c.NodeFact, Text: "流动性收紧", OccurredAt: time.Date(2026, 4, 10, 0, 0, 0, 0, time.UTC)}, {ID: "n2", Kind: c.NodeConclusion, Text: "风险资产承压"}, {ID: "n3", Kind: c.NodePrediction, Text: "未来数月波动加大", PredictionStartAt: time.Date(2026, 4, 10, 0, 0, 0, 0, time.UTC)}}, Edges: []c.GraphEdge{{From: "n1", To: "n2", Kind: c.EdgeDerives}, {From: "n2", To: "n3", Kind: c.EdgeDerives}}}, Details: c.HiddenDetails{Caveats: []string{"说明"}}}, CompiledAt: time.Now().UTC()},
			{UnitID: "weibo:CL2", Source: "weibo", ExternalID: "CL2", RootExternalID: "CL2", Model: c.Qwen36PlusModel, Output: c.Output{Summary: "s", Graph: c.ReasoningGraph{Nodes: []c.GraphNode{{ID: "n1", Kind: c.NodeFact, Text: "供给趋紧", OccurredAt: time.Date(2026, 4, 10, 0, 0, 0, 0, time.UTC)}, {ID: "n2", Kind: c.NodeConclusion, Text: "油价会上升"}, {ID: "n3", Kind: c.NodePrediction, Text: "油价冲击扩大", PredictionStartAt: time.Date(2026, 4, 10, 0, 0, 0, 0, time.UTC)}}, Edges: []c.GraphEdge{{From: "n1", To: "n2", Kind: c.EdgeDerives}, {From: "n2", To: "n3", Kind: c.EdgeDerives}}}, Details: c.HiddenDetails{Caveats: []string{"说明"}}}, CompiledAt: time.Now().UTC()},
		}
		for _, record := range records {
			if err := store.UpsertCompiledOutput(context.Background(), record); err != nil {
				return nil, err
			}
		}
		for _, req := range []memory.AcceptRequest{
			{UserID: "u-compare-limit", SourcePlatform: "weibo", SourceExternalID: "CL1", NodeIDs: []string{"n1", "n2", "n3"}},
			{UserID: "u-compare-limit", SourcePlatform: "weibo", SourceExternalID: "CL2", NodeIDs: []string{"n1", "n2", "n3"}},
		} {
			if _, err := store.AcceptMemoryNodes(context.Background(), req); err != nil {
				return nil, err
			}
		}
		if _, err := store.RunGlobalMemoryOrganization(context.Background(), "u-compare-limit", time.Now().UTC()); err != nil {
			return nil, err
		}
		if _, err := store.RunGlobalMemoryOrganizationV2(context.Background(), "u-compare-limit", time.Now().UTC()); err != nil {
			return nil, err
		}
		return store, nil
	}

	var stdout, stderr bytes.Buffer
	code := run([]string{"memory", "global-compare", "--user", "u-compare-limit", "--limit", "1"}, "/tmp/project", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("global-compare limit code = %d, stderr = %s", code, stderr.String())
	}
	if strings.Count(stdout.String(), "- ") != 2 {
		t.Fatalf("stdout = %q, want one v1 item and one v2 item", stdout.String())
	}
}

func TestRunMemoryGlobalCompareFiltersV2ItemType(t *testing.T) {
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
		records := []c.Record{
			{UnitID: "weibo:CFV1", Source: "weibo", ExternalID: "CFV1", RootExternalID: "CFV1", Model: c.Qwen36PlusModel, Output: c.Output{Summary: "s", Graph: c.ReasoningGraph{Nodes: []c.GraphNode{{ID: "n1", Kind: c.NodeFact, Text: "供给趋紧", OccurredAt: time.Date(2026, 4, 10, 0, 0, 0, 0, time.UTC)}, {ID: "n2", Kind: c.NodeConclusion, Text: "油价会上升"}}, Edges: []c.GraphEdge{{From: "n1", To: "n2", Kind: c.EdgeDerives}}}, Details: c.HiddenDetails{Caveats: []string{"说明"}}}, CompiledAt: time.Now().UTC()},
			{UnitID: "twitter:CFV2", Source: "twitter", ExternalID: "CFV2", RootExternalID: "CFV2", Model: c.Qwen36PlusModel, Output: c.Output{Summary: "s", Graph: c.ReasoningGraph{Nodes: []c.GraphNode{{ID: "n1", Kind: c.NodeFact, Text: "需求走弱", OccurredAt: time.Date(2026, 4, 10, 0, 0, 0, 0, time.UTC)}, {ID: "n2", Kind: c.NodeConclusion, Text: "油价会下降"}}, Edges: []c.GraphEdge{{From: "n1", To: "n2", Kind: c.EdgeDerives}}}, Details: c.HiddenDetails{Caveats: []string{"说明"}}}, CompiledAt: time.Now().UTC()},
			{UnitID: "weibo:CFV3", Source: "weibo", ExternalID: "CFV3", RootExternalID: "CFV3", Model: c.Qwen36PlusModel, Output: c.Output{Summary: "s", Graph: c.ReasoningGraph{Nodes: []c.GraphNode{{ID: "n1", Kind: c.NodeFact, Text: "流动性收紧", OccurredAt: time.Date(2026, 4, 10, 0, 0, 0, 0, time.UTC)}, {ID: "n2", Kind: c.NodeConclusion, Text: "风险资产承压"}, {ID: "n3", Kind: c.NodePrediction, Text: "未来数月波动加大", PredictionStartAt: time.Date(2026, 4, 10, 0, 0, 0, 0, time.UTC)}}, Edges: []c.GraphEdge{{From: "n1", To: "n2", Kind: c.EdgeDerives}, {From: "n2", To: "n3", Kind: c.EdgeDerives}}}, Details: c.HiddenDetails{Caveats: []string{"说明"}}}, CompiledAt: time.Now().UTC()},
		}
		for _, record := range records {
			if err := store.UpsertCompiledOutput(context.Background(), record); err != nil {
				return nil, err
			}
		}
		for _, req := range []memory.AcceptRequest{
			{UserID: "u-compare-filter", SourcePlatform: "weibo", SourceExternalID: "CFV1", NodeIDs: []string{"n2"}},
			{UserID: "u-compare-filter", SourcePlatform: "twitter", SourceExternalID: "CFV2", NodeIDs: []string{"n2"}},
			{UserID: "u-compare-filter", SourcePlatform: "weibo", SourceExternalID: "CFV3", NodeIDs: []string{"n1", "n2", "n3"}},
		} {
			if _, err := store.AcceptMemoryNodes(context.Background(), req); err != nil {
				return nil, err
			}
		}
		if _, err := store.RunGlobalMemoryOrganization(context.Background(), "u-compare-filter", time.Now().UTC()); err != nil {
			return nil, err
		}
		if _, err := store.RunGlobalMemoryOrganizationV2(context.Background(), "u-compare-filter", time.Now().UTC()); err != nil {
			return nil, err
		}
		return store, nil
	}

	var stdout, stderr bytes.Buffer
	code := run([]string{"memory", "global-compare", "--user", "u-compare-filter", "--item-type", "conflict"}, "/tmp/project", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("global-compare conflict filter code = %d, stderr = %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "conflict:") || strings.Contains(stdout.String(), "conclusion:") {
		t.Fatalf("stdout = %q, want only v2 conflict items while keeping v1 section", stdout.String())
	}
	if !strings.Contains(stdout.String(), "V2 thesis-first (1, filter=conflict)") {
		t.Fatalf("stdout = %q, want filter annotation in V2 header", stdout.String())
	}
}

func TestRunMemoryGlobalCompareReportsWhenFilteredV2SideIsEmpty(t *testing.T) {
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
		record := c.Record{
			UnitID: "weibo:EC1", Source: "weibo", ExternalID: "EC1", RootExternalID: "EC1", Model: c.Qwen36PlusModel,
			Output: c.Output{Summary: "s", Graph: c.ReasoningGraph{Nodes: []c.GraphNode{
				{ID: "n1", Kind: c.NodeFact, Text: "流动性收紧", OccurredAt: time.Date(2026, 4, 10, 0, 0, 0, 0, time.UTC)},
				{ID: "n2", Kind: c.NodeConclusion, Text: "风险资产承压"},
				{ID: "n3", Kind: c.NodePrediction, Text: "未来数月波动加大", PredictionStartAt: time.Date(2026, 4, 10, 0, 0, 0, 0, time.UTC)},
			}, Edges: []c.GraphEdge{{From: "n1", To: "n2", Kind: c.EdgeDerives}, {From: "n2", To: "n3", Kind: c.EdgeDerives}}}, Details: c.HiddenDetails{Caveats: []string{"说明"}}},
			CompiledAt: time.Now().UTC(),
		}
		if err := store.UpsertCompiledOutput(context.Background(), record); err != nil {
			return nil, err
		}
		if _, err := store.AcceptMemoryNodes(context.Background(), memory.AcceptRequest{UserID: "u-compare-empty-filter", SourcePlatform: "weibo", SourceExternalID: "EC1", NodeIDs: []string{"n1", "n2", "n3"}}); err != nil {
			return nil, err
		}
		if _, err := store.RunGlobalMemoryOrganization(context.Background(), "u-compare-empty-filter", time.Now().UTC()); err != nil {
			return nil, err
		}
		if _, err := store.RunGlobalMemoryOrganizationV2(context.Background(), "u-compare-empty-filter", time.Now().UTC()); err != nil {
			return nil, err
		}
		return store, nil
	}

	var stdout, stderr bytes.Buffer
	code := run([]string{"memory", "global-compare", "--user", "u-compare-empty-filter", "--item-type", "conflict"}, "/tmp/project", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("global-compare empty filter code = %d, stderr = %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "No conflict items") {
		t.Fatalf("stdout = %q, want no-match guidance while keeping compare context", stdout.String())
	}
	if !strings.Contains(stdout.String(), "V2 thesis-first (0, filter=conflict)") {
		t.Fatalf("stdout = %q, want filtered count annotation even when empty", stdout.String())
	}
}

func TestRunMemoryGlobalCompareShowsSectionCounts(t *testing.T) {
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
		record := c.Record{
			UnitID: "weibo:CC1", Source: "weibo", ExternalID: "CC1", RootExternalID: "CC1", Model: c.Qwen36PlusModel,
			Output: c.Output{Summary: "s", Graph: c.ReasoningGraph{Nodes: []c.GraphNode{
				{ID: "n1", Kind: c.NodeFact, Text: "流动性收紧", OccurredAt: time.Date(2026, 4, 10, 0, 0, 0, 0, time.UTC)},
				{ID: "n2", Kind: c.NodeConclusion, Text: "风险资产承压"},
				{ID: "n3", Kind: c.NodePrediction, Text: "未来数月波动加大", PredictionStartAt: time.Date(2026, 4, 10, 0, 0, 0, 0, time.UTC)},
			}, Edges: []c.GraphEdge{{From: "n1", To: "n2", Kind: c.EdgeDerives}, {From: "n2", To: "n3", Kind: c.EdgeDerives}}}, Details: c.HiddenDetails{Caveats: []string{"说明"}}},
			CompiledAt: time.Now().UTC(),
		}
		if err := store.UpsertCompiledOutput(context.Background(), record); err != nil {
			return nil, err
		}
		if _, err := store.AcceptMemoryNodes(context.Background(), memory.AcceptRequest{UserID: "u-compare-count", SourcePlatform: "weibo", SourceExternalID: "CC1", NodeIDs: []string{"n1", "n2", "n3"}}); err != nil {
			return nil, err
		}
		if _, err := store.RunGlobalMemoryOrganization(context.Background(), "u-compare-count", time.Now().UTC()); err != nil {
			return nil, err
		}
		if _, err := store.RunGlobalMemoryOrganizationV2(context.Background(), "u-compare-count", time.Now().UTC()); err != nil {
			return nil, err
		}
		return store, nil
	}

	var stdout, stderr bytes.Buffer
	code := run([]string{"memory", "global-compare", "--user", "u-compare-count"}, "/tmp/project", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("global-compare code = %d, stderr = %s", code, stderr.String())
	}
	out := stdout.String()
	for _, want := range []string{"V1 cluster-first (", "V2 thesis-first ("} {
		if !strings.Contains(out, want) {
			t.Fatalf("stdout missing %q in %q", want, out)
		}
	}
}

func TestRunMemoryGlobalCompareSuggestsRunWhenNoStoredOutputs(t *testing.T) {
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
		return contentstore.NewSQLiteStore(path)
	}

	var stdout, stderr bytes.Buffer
	code := run([]string{"memory", "global-compare", "--user", "u-empty-compare"}, "/tmp/project", &stdout, &stderr)
	if code != 1 {
		t.Fatalf("global-compare code = %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), "memory global-compare --run --user u-empty-compare") {
		t.Fatalf("stderr = %q, want --run guidance", stderr.String())
	}
}

type fakeCompileClient struct {
	record    c.Record
	err       error
	compileFn func(context.Context, c.Bundle) (c.Record, error)
	verifyFn  func(context.Context, c.Bundle, c.Output) (c.Verification, error)
}

func (f fakeCompileClient) Compile(ctx context.Context, bundle c.Bundle) (c.Record, error) {
	if f.compileFn != nil {
		return f.compileFn(ctx, bundle)
	}
	return f.record, f.err
}

func (f fakeCompileClient) Verify(ctx context.Context, bundle c.Bundle, output c.Output) (c.Verification, error) {
	if f.verifyFn != nil {
		return f.verifyFn(ctx, bundle, output)
	}
	return c.Verification{}, f.err
}

func (f fakeCompileClient) VerifyDetailed(ctx context.Context, bundle c.Bundle, output c.Output) (c.Verification, error) {
	return f.Verify(ctx, bundle, output)
}

func testGraphNode(id string, kind c.NodeKind, text string) c.GraphNode {
	return c.GraphNode{
		ID:        id,
		Kind:      kind,
		Text:      text,
		ValidFrom: time.Date(2026, 4, 14, 0, 0, 0, 0, time.UTC),
		ValidTo:   time.Date(2026, 7, 14, 0, 0, 0, 0, time.UTC),
	}
}

func stringValue(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

func TestRunCompileWritesCompiledRecordJSON(t *testing.T) {
	prevBuildApp := buildApp
	prevBuildCompileClient := buildCompileClient
	prevBuildCompileClientNoVerify := buildCompileClientNoVerify
	prevBuildCompileClientV2 := buildCompileClientV2
	prevOpenSQLiteStore := openSQLiteStore
	t.Cleanup(func() {
		buildApp = prevBuildApp
		buildCompileClient = prevBuildCompileClient
		buildCompileClientNoVerify = prevBuildCompileClientNoVerify
		buildCompileClientV2 = prevBuildCompileClientV2
		openSQLiteStore = prevOpenSQLiteStore
	})

	tmp := t.TempDir()
	buildApp = func(projectRoot string) (*bootstrap.App, error) {
		src := fakeItemSource{
			items: []types.RawContent{{
				Source:     "weibo",
				ExternalID: "QAu4U9USk",
				Content:    "hello",
				AuthorName: "Alice",
				URL:        "https://weibo.com/1182426800/QAu4U9USk",
			}},
		}
		app := &bootstrap.App{
			Dispatcher: dispatcher.New(
				func(raw string) (types.ParsedURL, error) {
					return types.ParsedURL{
						Platform:     types.PlatformWeb,
						ContentType:  types.ContentTypePost,
						PlatformID:   "id-1",
						CanonicalURL: raw,
					}, nil
				},
				[]dispatcher.ItemSource{src},
				nil,
				nil,
			),
		}
		app.Settings.ContentDBPath = tmp + "/content.db"
		return app, nil
	}

	buildCompileClient = func(projectRoot string) compileClient {
		return fakeCompileClient{
			record: c.Record{
				UnitID:         "weibo:QAu4U9USk",
				Source:         "weibo",
				ExternalID:     "QAu4U9USk",
				RootExternalID: "QAu4U9USk",
				Model:          c.Qwen36PlusModel,
				Output: c.Output{
					Summary: "一句话",
					Graph: c.ReasoningGraph{
						Nodes: []c.GraphNode{testGraphNode("n1", c.NodeFact, "事实A"), testGraphNode("n2", c.NodeConclusion, "结论B")},
						Edges: []c.GraphEdge{{From: "n1", To: "n2", Kind: c.EdgeDerives}},
					},
					Details: c.HiddenDetails{Caveats: []string{"说明"}},
				},
				CompiledAt: time.Now().UTC(),
			},
		}
	}

	openSQLiteStore = func(path string) (*contentstore.SQLiteStore, error) {
		return contentstore.NewSQLiteStore(path)
	}

	var stdout, stderr bytes.Buffer
	code := run([]string{"compile", "run", "https://example.com/post"}, "/tmp/project", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("run() code = %d, stderr = %s", code, stderr.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q, want empty", stderr.String())
	}
	var got c.Record
	if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
		t.Fatalf("json.Unmarshal(stdout) error = %v", err)
	}
	if got.Output.Summary != "一句话" {
		t.Fatalf("Summary = %q", got.Output.Summary)
	}
}

func TestRunCompilePipelineV2UsesV2Client(t *testing.T) {
	prevBuildApp := buildApp
	prevBuildCompileClient := buildCompileClient
	prevBuildCompileClientNoVerify := buildCompileClientNoVerify
	prevBuildCompileClientV2 := buildCompileClientV2
	prevOpenSQLiteStore := openSQLiteStore
	t.Cleanup(func() {
		buildApp = prevBuildApp
		buildCompileClient = prevBuildCompileClient
		buildCompileClientNoVerify = prevBuildCompileClientNoVerify
		buildCompileClientV2 = prevBuildCompileClientV2
		openSQLiteStore = prevOpenSQLiteStore
	})

	tmp := t.TempDir()
	buildApp = func(projectRoot string) (*bootstrap.App, error) {
		app := &bootstrap.App{}
		app.Settings.ContentDBPath = tmp + "/content.db"
		src := fakeItemSource{
			items: []types.RawContent{{
				Source:     "web",
				ExternalID: "v2-id",
				Content:    "hello",
				URL:        "https://example.com/v2",
			}},
		}
		app.Dispatcher = dispatcher.New(
			func(raw string) (types.ParsedURL, error) {
				return types.ParsedURL{Platform: types.PlatformWeb, ContentType: types.ContentTypePost, PlatformID: "v2-id", CanonicalURL: raw}, nil
			},
			[]dispatcher.ItemSource{src},
			nil,
			nil,
		)
		return app, nil
	}
	buildCompileClient = func(projectRoot string) compileClient {
		t.Fatal("legacy compile client should not be used")
		return nil
	}
	buildCompileClientV2 = func(projectRoot string) compileClient {
		return fakeCompileClient{record: c.Record{
			UnitID:     "web:v2-id",
			Source:     "web",
			ExternalID: "v2-id",
			Model:      "qwen3.6-plus",
			Metrics:    c.RecordMetrics{CompileElapsedMS: 777, CompileStageElapsedMS: map[string]int64{"extract": 101, "refine": 102, "aggregate": 103, "support": 104, "collapse": 105, "relations": 106, "classify": 107, "render": 108}},
			Output: c.Output{
				Summary: "v2 summary",
				Graph: c.ReasoningGraph{
					Nodes: []c.GraphNode{testGraphNode("n1", c.NodeFact, "事实A"), testGraphNode("n2", c.NodeConclusion, "结论B")},
					Edges: []c.GraphEdge{{From: "n1", To: "n2", Kind: c.EdgeDerives}},
				},
				Details: c.HiddenDetails{Caveats: []string{"说明"}},
			},
			CompiledAt: time.Now().UTC(),
		}}
	}
	openSQLiteStore = func(path string) (*contentstore.SQLiteStore, error) {
		return contentstore.NewSQLiteStore(path)
	}

	var stdout, stderr bytes.Buffer
	code := run([]string{"compile", "run", "--pipeline", "v2", "--url", "https://example.com/v2"}, "/tmp/project", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("run() code = %d, stderr = %s", code, stderr.String())
	}
	var got c.Record
	if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
		t.Fatalf("json.Unmarshal(stdout) error = %v", err)
	}
	if got.Output.Summary != "v2 summary" {
		t.Fatalf("Summary = %q, want v2 summary", got.Output.Summary)
	}
	if got.Metrics.CompileElapsedMS != 777 {
		t.Fatalf("CompileElapsedMS = %d, want 777", got.Metrics.CompileElapsedMS)
	}
	for _, stage := range []string{"extract", "refine", "aggregate", "support", "collapse", "relations", "classify", "render"} {
		if got.Metrics.CompileStageElapsedMS[stage] <= 0 {
			t.Fatalf("CompileStageElapsedMS = %#v, want positive persisted v2 stage metric for %q", got.Metrics.CompileStageElapsedMS, stage)
		}
	}
	if _, ok := got.Metrics.CompileStageElapsedMS["validate"]; ok {
		t.Fatalf("CompileStageElapsedMS = %#v, compile metrics must not include validate", got.Metrics.CompileStageElapsedMS)
	}
	stdout.Reset()
	stderr.Reset()
	code = run([]string{"compile", "show", "--platform", "web", "--id", "v2-id"}, "/tmp/project", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("compile show code = %d, stderr = %s", code, stderr.String())
	}
	var shown c.Record
	if err := json.Unmarshal(stdout.Bytes(), &shown); err != nil {
		t.Fatalf("json.Unmarshal(show stdout) error = %v", err)
	}
	if shown.Metrics.CompileElapsedMS != 777 {
		t.Fatalf("shown CompileElapsedMS = %d, want 777", shown.Metrics.CompileElapsedMS)
	}
	for _, stage := range []string{"extract", "refine", "aggregate", "support", "collapse", "relations", "classify", "render"} {
		if shown.Metrics.CompileStageElapsedMS[stage] <= 0 {
			t.Fatalf("shown CompileStageElapsedMS = %#v, want positive persisted v2 stage metric for %q", shown.Metrics.CompileStageElapsedMS, stage)
		}
	}
	if _, ok := shown.Metrics.CompileStageElapsedMS["validate"]; ok {
		t.Fatalf("shown CompileStageElapsedMS = %#v, compile metrics must not include validate", shown.Metrics.CompileStageElapsedMS)
	}
}

func TestRunCompileRejectsUnknownPipeline(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{"compile", "run", "--pipeline", "bogus", "--platform", "weibo", "--id", "x"}, "/tmp/project", &stdout, &stderr)
	if code != 2 {
		t.Fatalf("run() code = %d, want 2", code)
	}
	if !strings.Contains(stderr.String(), "unsupported compile pipeline") {
		t.Fatalf("stderr = %q, want unsupported compile pipeline", stderr.String())
	}
}

func TestSelectCompileClientKeepsLegacyPipelineIsolated(t *testing.T) {
	prevBuildCompileClient := buildCompileClient
	prevBuildCompileClientNoVerify := buildCompileClientNoVerify
	prevBuildCompileClientV2 := buildCompileClientV2
	t.Cleanup(func() {
		buildCompileClient = prevBuildCompileClient
		buildCompileClientNoVerify = prevBuildCompileClientNoVerify
		buildCompileClientV2 = prevBuildCompileClientV2
	})

	legacyCalls := 0
	noVerifyCalls := 0
	v2Calls := 0
	buildCompileClient = func(projectRoot string) compileClient {
		legacyCalls++
		return fakeCompileClient{}
	}
	buildCompileClientNoVerify = func(projectRoot string) compileClient {
		noVerifyCalls++
		return fakeCompileClient{}
	}
	buildCompileClientV2 = func(projectRoot string) compileClient {
		v2Calls++
		return fakeCompileClient{}
	}

	if _, err := selectCompileClient("/tmp/project", "", false); err != nil {
		t.Fatalf("selectCompileClient(default legacy) error = %v", err)
	}
	if _, err := selectCompileClient("/tmp/project", "legacy", false); err != nil {
		t.Fatalf("selectCompileClient(explicit legacy) error = %v", err)
	}
	if _, err := selectCompileClient("/tmp/project", "legacy", true); err != nil {
		t.Fatalf("selectCompileClient(no verify) error = %v", err)
	}

	if legacyCalls != 2 {
		t.Fatalf("legacy builder calls = %d, want 2", legacyCalls)
	}
	if noVerifyCalls != 1 {
		t.Fatalf("no-verify builder calls = %d, want 1", noVerifyCalls)
	}
	if v2Calls != 0 {
		t.Fatalf("v2 builder calls = %d, want 0 for legacy pipeline selections", v2Calls)
	}
}

func TestSelectCompileClientUsesV2OnlyWhenRequested(t *testing.T) {
	prevBuildCompileClient := buildCompileClient
	prevBuildCompileClientNoVerify := buildCompileClientNoVerify
	prevBuildCompileClientV2 := buildCompileClientV2
	t.Cleanup(func() {
		buildCompileClient = prevBuildCompileClient
		buildCompileClientNoVerify = prevBuildCompileClientNoVerify
		buildCompileClientV2 = prevBuildCompileClientV2
	})

	legacyCalls := 0
	v2Calls := 0
	buildCompileClient = func(projectRoot string) compileClient {
		legacyCalls++
		return fakeCompileClient{}
	}
	buildCompileClientNoVerify = func(projectRoot string) compileClient {
		t.Fatal("legacy no-verify builder should not be used for v2 pipeline")
		return nil
	}
	buildCompileClientV2 = func(projectRoot string) compileClient {
		v2Calls++
		return fakeCompileClient{}
	}

	if _, err := selectCompileClient("/tmp/project", "v2", false); err != nil {
		t.Fatalf("selectCompileClient(v2) error = %v", err)
	}
	if v2Calls != 1 {
		t.Fatalf("v2 builder calls = %d, want 1", v2Calls)
	}
	if legacyCalls != 0 {
		t.Fatalf("legacy builder calls = %d, want 0 for v2 selection", legacyCalls)
	}

	if _, err := selectCompileClient("/tmp/project", "v2", true); err == nil {
		t.Fatal("selectCompileClient(v2, --no-verify) error = nil, want unsupported flag error")
	}
	if v2Calls != 1 {
		t.Fatalf("v2 builder calls after unsupported flag checks = %d, want 1", v2Calls)
	}
}

func TestRunVerifyRunAndShowUseSeparateVerificationStore(t *testing.T) {
	prevBuildApp := buildApp
	prevBuildCompileClient := buildCompileClient
	prevOpenSQLiteStore := openSQLiteStore
	t.Cleanup(func() {
		buildApp = prevBuildApp
		buildCompileClient = prevBuildCompileClient
		openSQLiteStore = prevOpenSQLiteStore
	})

	tmp := t.TempDir()
	buildApp = func(projectRoot string) (*bootstrap.App, error) {
		app := &bootstrap.App{}
		app.Settings.ContentDBPath = tmp + "/content.db"
		return app, nil
	}
	buildCompileClient = func(projectRoot string) compileClient {
		return fakeCompileClient{
			verifyFn: func(ctx context.Context, bundle c.Bundle, output c.Output) (c.Verification, error) {
				return c.Verification{
					Model: "verify-model",
					FactChecks: []c.FactCheck{{
						NodeID: "n1",
						Status: c.FactStatusClearlyTrue,
						Reason: "supported",
					}},
					VerifiedAt: time.Now().UTC(),
				}, nil
			},
		}
	}
	openSQLiteStore = func(path string) (*contentstore.SQLiteStore, error) {
		store, err := contentstore.NewSQLiteStore(path)
		if err != nil {
			return nil, err
		}
		raw := types.RawContent{
			Source:     "weibo",
			ExternalID: "Q-verify",
			URL:        "https://weibo.com/123/Q-verify",
			Content:    "root body",
			AuthorName: "alice",
			PostedAt:   time.Now().UTC(),
		}
		if err := store.UpsertRawCapture(context.Background(), raw); err != nil {
			return nil, err
		}
		record := c.Record{
			UnitID:         "weibo:Q-verify",
			Source:         "weibo",
			ExternalID:     "Q-verify",
			RootExternalID: "Q-verify",
			Model:          c.Qwen36PlusModel,
			Output: c.Output{
				Summary: "一句话",
				Graph: c.ReasoningGraph{
					Nodes: []c.GraphNode{testGraphNode("n1", c.NodeFact, "事实A"), testGraphNode("n2", c.NodeConclusion, "结论B")},
					Edges: []c.GraphEdge{{From: "n1", To: "n2", Kind: c.EdgeDerives}},
				},
				Details: c.HiddenDetails{Caveats: []string{"说明"}},
			},
			CompiledAt: time.Now().UTC(),
		}
		if err := store.UpsertCompiledOutput(context.Background(), record); err != nil {
			return nil, err
		}
		return store, nil
	}

	var stdout, stderr bytes.Buffer
	code := run([]string{"verify", "run", "--platform", "weibo", "--id", "Q-verify"}, "/tmp/project", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("verify run code = %d, stderr = %s", code, stderr.String())
	}
	var verifyRecord c.VerificationRecord
	if err := json.Unmarshal(stdout.Bytes(), &verifyRecord); err != nil {
		t.Fatalf("json.Unmarshal(verify run) error = %v", err)
	}
	if verifyRecord.Model != "qwen3.6-plus" {
		t.Fatalf("verify record model = %q, want compile model persistence surface", verifyRecord.Model)
	}
	if len(verifyRecord.Verification.FactChecks) != 1 || verifyRecord.Verification.FactChecks[0].NodeID != "n1" {
		t.Fatalf("verify record = %#v", verifyRecord)
	}

	stdout.Reset()
	stderr.Reset()
	code = run([]string{"verify", "show", "--platform", "weibo", "--id", "Q-verify"}, "/tmp/project", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("verify show code = %d, stderr = %s", code, stderr.String())
	}
	var shown c.VerificationRecord
	if err := json.Unmarshal(stdout.Bytes(), &shown); err != nil {
		t.Fatalf("json.Unmarshal(verify show) error = %v", err)
	}
	if len(shown.Verification.FactChecks) != 1 || shown.Verification.FactChecks[0].Reason != "supported" {
		t.Fatalf("shown verification = %#v", shown)
	}
}

func TestRunVerifyRunAlsoAppliesVerificationToGraphFirstContentSubgraph(t *testing.T) {
	prevBuildApp := buildApp
	prevBuildCompileClient := buildCompileClient
	prevOpenSQLiteStore := openSQLiteStore
	t.Cleanup(func() {
		buildApp = prevBuildApp
		buildCompileClient = prevBuildCompileClient
		openSQLiteStore = prevOpenSQLiteStore
	})

	var dbPath string
	buildApp = func(projectRoot string) (*bootstrap.App, error) {
		app := &bootstrap.App{}
		dbPath = filepath.Join(t.TempDir(), "content.db")
		app.Settings.ContentDBPath = dbPath
		return app, nil
	}
	buildCompileClient = func(projectRoot string) compileClient {
		return compileClientStub{
			verifyDetailed: func(ctx context.Context, bundle c.Bundle, output c.Output) (c.Verification, error) {
				return c.Verification{
					FactChecks:       []c.FactCheck{{NodeID: "n1", Status: c.FactStatusClearlyTrue, Reason: "supported"}},
					PredictionChecks: []c.PredictionCheck{{NodeID: "n2", Status: c.PredictionStatusResolvedTrue, Reason: "resolved", AsOf: time.Now().UTC()}},
					VerifiedAt:       time.Now().UTC(),
				}, nil
			},
		}
	}
	openSQLiteStore = func(path string) (*contentstore.SQLiteStore, error) {
		store, err := contentstore.NewSQLiteStore(path)
		if err != nil {
			return nil, err
		}
		raw := types.RawContent{Source: "weibo", ExternalID: "Q-verify-graph", URL: "https://weibo.com/123/Q-verify-graph", Content: "root body", AuthorName: "alice", PostedAt: time.Now().UTC()}
		if err := store.UpsertRawCapture(context.Background(), raw); err != nil {
			return nil, err
		}
		record := c.Record{UnitID: "weibo:Q-verify-graph", Source: "weibo", ExternalID: "Q-verify-graph", RootExternalID: "Q-verify-graph", Model: c.Qwen36PlusModel, Output: c.Output{Summary: "一句话", Graph: c.ReasoningGraph{Nodes: []c.GraphNode{testGraphNode("n1", c.NodeFact, "事实A"), testGraphNode("n2", c.NodePrediction, "预测B")}, Edges: []c.GraphEdge{{From: "n1", To: "n2", Kind: c.EdgePositive}}}, Details: c.HiddenDetails{Caveats: []string{"说明"}}}, CompiledAt: time.Now().UTC()}
		if err := store.UpsertCompiledOutput(context.Background(), record); err != nil {
			return nil, err
		}
		return store, nil
	}
	var stdout, stderr bytes.Buffer
	code := run([]string{"verify", "run", "--platform", "weibo", "--id", "Q-verify-graph", "--force"}, "/tmp/project", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("verify run graph-first code = %d, stderr = %s", code, stderr.String())
	}
	reopen, err := contentstore.NewSQLiteStore(dbPath)
	if err != nil {
		t.Fatalf("NewSQLiteStore(reopen) error = %v", err)
	}
	defer reopen.Close()
	got, err := reopen.GetContentSubgraph(context.Background(), "weibo", "Q-verify-graph")
	if err != nil {
		t.Fatalf("GetContentSubgraph() error = %v", err)
	}
	statuses := map[string]string{}
	for _, node := range got.Nodes {
		statuses[node.ID] = string(node.VerificationStatus) + ":" + node.VerificationReason
	}
	if statuses["n1"] != "proved:supported" {
		t.Fatalf("n1 status = %q, want proved:supported", statuses["n1"])
	}
	if statuses["n2"] != "proved:resolved" {
		t.Fatalf("n2 status = %q, want proved:resolved", statuses["n2"])
	}
}

func TestRunCompileReadsExistingRawCaptureByPlatformAndID(t *testing.T) {
	prevBuildApp := buildApp
	prevBuildCompileClient := buildCompileClient
	prevOpenSQLiteStore := openSQLiteStore
	t.Cleanup(func() {
		buildApp = prevBuildApp
		buildCompileClient = prevBuildCompileClient
		openSQLiteStore = prevOpenSQLiteStore
	})

	tmp := t.TempDir()
	buildApp = func(projectRoot string) (*bootstrap.App, error) {
		app := &bootstrap.App{}
		app.Settings.ContentDBPath = tmp + "/content.db"
		return app, nil
	}
	buildCompileClient = func(projectRoot string) compileClient {
		return fakeCompileClient{
			record: c.Record{
				UnitID:         "weibo:QAu4U9USk",
				Source:         "weibo",
				ExternalID:     "QAu4U9USk",
				RootExternalID: "QAu4U9USk",
				Model:          c.Qwen36PlusModel,
				Output: c.Output{
					Summary: "一句话",
					Graph: c.ReasoningGraph{
						Nodes: []c.GraphNode{testGraphNode("n1", c.NodeFact, "事实A"), testGraphNode("n2", c.NodeConclusion, "结论B")},
						Edges: []c.GraphEdge{{From: "n1", To: "n2", Kind: c.EdgeDerives}},
					},
					Details: c.HiddenDetails{Caveats: []string{"说明"}},
				},
				CompiledAt: time.Now().UTC(),
			},
		}
	}
	openSQLiteStore = func(path string) (*contentstore.SQLiteStore, error) {
		store, err := contentstore.NewSQLiteStore(path)
		if err != nil {
			return nil, err
		}
		if err := store.UpsertRawCapture(context.Background(), types.RawContent{
			Source:     "weibo",
			ExternalID: "QAu4U9USk",
			Content:    "hello",
			AuthorName: "Alice",
			URL:        "https://weibo.com/1182426800/QAu4U9USk",
		}); err != nil {
			return nil, err
		}
		return store, nil
	}

	var stdout, stderr bytes.Buffer
	code := run([]string{"compile", "run", "--platform", "weibo", "--id", "QAu4U9USk"}, "/tmp/project", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("run() code = %d, stderr = %s", code, stderr.String())
	}
	var got c.Record
	if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
		t.Fatalf("json.Unmarshal(stdout) error = %v", err)
	}
	if got.ExternalID != "QAu4U9USk" {
		t.Fatalf("ExternalID = %q", got.ExternalID)
	}
}

func TestRunCompileURLPrefersStoredRawCapture(t *testing.T) {
	prevBuildApp := buildApp
	prevBuildCompileClient := buildCompileClient
	prevOpenSQLiteStore := openSQLiteStore
	t.Cleanup(func() {
		buildApp = prevBuildApp
		buildCompileClient = prevBuildCompileClient
		openSQLiteStore = prevOpenSQLiteStore
	})

	tmp := t.TempDir()
	buildApp = func(projectRoot string) (*bootstrap.App, error) {
		app := &bootstrap.App{}
		app.Settings.ContentDBPath = tmp + "/content.db"
		app.Dispatcher = dispatcher.New(
			func(raw string) (types.ParsedURL, error) {
				return types.ParsedURL{Platform: types.PlatformTwitter, PlatformID: "2026305745872998803", CanonicalURL: raw}, nil
			},
			[]dispatcher.ItemSource{panicItemSource{}},
			nil,
			nil,
		)
		return app, nil
	}
	buildCompileClient = func(projectRoot string) compileClient {
		return fakeCompileClient{
			record: c.Record{
				UnitID:         "twitter:2026305745872998803",
				Source:         "twitter",
				ExternalID:     "2026305745872998803",
				RootExternalID: "2026305745872998803",
				Model:          c.Qwen36PlusModel,
				Output: c.Output{
					Summary: "Dalio summary",
					Graph: c.ReasoningGraph{
						Nodes: []c.GraphNode{testGraphNode("n1", c.NodeFact, "事实A"), testGraphNode("n2", c.NodeConclusion, "结论B")},
						Edges: []c.GraphEdge{{From: "n1", To: "n2", Kind: c.EdgeDerives}},
					},
					Details:    c.HiddenDetails{Caveats: []string{"说明"}},
					Confidence: "high",
				},
				CompiledAt: time.Now().UTC(),
			},
		}
	}
	openSQLiteStore = func(path string) (*contentstore.SQLiteStore, error) {
		store, err := contentstore.NewSQLiteStore(path)
		if err != nil {
			return nil, err
		}
		if err := store.UpsertRawCapture(context.Background(), types.RawContent{
			Source:     "twitter",
			ExternalID: "2026305745872998803",
			Content:    "stored raw body",
			AuthorName: "Ray Dalio",
			URL:        "https://x.com/RayDalio/status/2026305745872998803",
		}); err != nil {
			return nil, err
		}
		return store, nil
	}

	var stdout, stderr bytes.Buffer
	code := run([]string{"compile", "run", "--url", "https://x.com/RayDalio/status/2026305745872998803"}, "/tmp/project", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("run() code = %d, stderr = %s", code, stderr.String())
	}
	var got c.Record
	if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
		t.Fatalf("json.Unmarshal(stdout) error = %v", err)
	}
	if got.ExternalID != "2026305745872998803" {
		t.Fatalf("ExternalID = %q", got.ExternalID)
	}
}

func TestRunCompileUsesStoredCompiledOutputUnlessForced(t *testing.T) {
	prevBuildApp := buildApp
	prevBuildCompileClient := buildCompileClient
	prevOpenSQLiteStore := openSQLiteStore
	t.Cleanup(func() {
		buildApp = prevBuildApp
		buildCompileClient = prevBuildCompileClient
		openSQLiteStore = prevOpenSQLiteStore
	})

	tmp := t.TempDir()
	buildApp = func(projectRoot string) (*bootstrap.App, error) {
		app := &bootstrap.App{}
		app.Settings.ContentDBPath = tmp + "/content.db"
		app.Dispatcher = dispatcher.New(
			func(raw string) (types.ParsedURL, error) {
				return types.ParsedURL{Platform: types.PlatformTwitter, PlatformID: "2026305745872998803", CanonicalURL: raw}, nil
			},
			[]dispatcher.ItemSource{panicItemSource{}},
			nil,
			nil,
		)
		return app, nil
	}
	buildCompileClient = func(projectRoot string) compileClient {
		return fakeCompileClient{
			record: c.Record{
				UnitID:         "twitter:2026305745872998803",
				Source:         "twitter",
				ExternalID:     "2026305745872998803",
				RootExternalID: "2026305745872998803",
				Model:          c.Qwen36PlusModel,
				Output: c.Output{
					Summary: "new summary should not be used",
					Graph: c.ReasoningGraph{
						Nodes: []c.GraphNode{testGraphNode("n1", c.NodeFact, "事实A"), testGraphNode("n2", c.NodeConclusion, "结论B")},
						Edges: []c.GraphEdge{{From: "n1", To: "n2", Kind: c.EdgeDerives}},
					},
					Details: c.HiddenDetails{Caveats: []string{"说明"}},
				},
				CompiledAt: time.Now().UTC(),
			},
		}
	}
	openSQLiteStore = func(path string) (*contentstore.SQLiteStore, error) {
		store, err := contentstore.NewSQLiteStore(path)
		if err != nil {
			return nil, err
		}
		if err := store.UpsertCompiledOutput(context.Background(), c.Record{
			UnitID:         "twitter:2026305745872998803",
			Source:         "twitter",
			ExternalID:     "2026305745872998803",
			RootExternalID: "2026305745872998803",
			Model:          c.Qwen36PlusModel,
			Output: c.Output{
				Summary: "cached summary",
				Graph: c.ReasoningGraph{
					Nodes: []c.GraphNode{testGraphNode("n1", c.NodeFact, "事实A"), testGraphNode("n2", c.NodeConclusion, "结论B")},
					Edges: []c.GraphEdge{{From: "n1", To: "n2", Kind: c.EdgeDerives}},
				},
				Details: c.HiddenDetails{Caveats: []string{"说明"}},
			},
			CompiledAt: time.Now().UTC(),
		}); err != nil {
			return nil, err
		}
		return store, nil
	}

	var stdout, stderr bytes.Buffer
	code := run([]string{"compile", "run", "--platform", "twitter", "--id", "2026305745872998803"}, "/tmp/project", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("run() code = %d, stderr = %s", code, stderr.String())
	}
	var got c.Record
	if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
		t.Fatalf("json.Unmarshal(stdout) error = %v", err)
	}
	if got.Output.Summary != "cached summary" {
		t.Fatalf("Summary = %q, want cached summary", got.Output.Summary)
	}
}

func TestRunCompileForceBypassesStoredCompiledOutput(t *testing.T) {
	prevBuildApp := buildApp
	prevBuildCompileClient := buildCompileClient
	prevOpenSQLiteStore := openSQLiteStore
	t.Cleanup(func() {
		buildApp = prevBuildApp
		buildCompileClient = prevBuildCompileClient
		openSQLiteStore = prevOpenSQLiteStore
	})

	tmp := t.TempDir()
	buildApp = func(projectRoot string) (*bootstrap.App, error) {
		app := &bootstrap.App{}
		app.Settings.ContentDBPath = tmp + "/content.db"
		return app, nil
	}
	buildCompileClient = func(projectRoot string) compileClient {
		return fakeCompileClient{
			record: c.Record{
				UnitID:         "weibo:QAu4U9USk",
				Source:         "weibo",
				ExternalID:     "QAu4U9USk",
				RootExternalID: "QAu4U9USk",
				Model:          c.Qwen36PlusModel,
				Output: c.Output{
					Summary: "forced summary",
					Graph: c.ReasoningGraph{
						Nodes: []c.GraphNode{testGraphNode("n1", c.NodeFact, "事实A"), testGraphNode("n2", c.NodeConclusion, "结论B")},
						Edges: []c.GraphEdge{{From: "n1", To: "n2", Kind: c.EdgeDerives}},
					},
					Details: c.HiddenDetails{Caveats: []string{"说明"}},
				},
				CompiledAt: time.Now().UTC(),
			},
		}
	}
	openSQLiteStore = func(path string) (*contentstore.SQLiteStore, error) {
		store, err := contentstore.NewSQLiteStore(path)
		if err != nil {
			return nil, err
		}
		if err := store.UpsertRawCapture(context.Background(), types.RawContent{
			Source:     "weibo",
			ExternalID: "QAu4U9USk",
			Content:    "hello",
			AuthorName: "Alice",
			URL:        "https://weibo.com/1182426800/QAu4U9USk",
		}); err != nil {
			return nil, err
		}
		if err := store.UpsertCompiledOutput(context.Background(), c.Record{
			UnitID:         "weibo:QAu4U9USk",
			Source:         "weibo",
			ExternalID:     "QAu4U9USk",
			RootExternalID: "QAu4U9USk",
			Model:          c.Qwen36PlusModel,
			Output: c.Output{
				Summary: "cached summary",
				Graph: c.ReasoningGraph{
					Nodes: []c.GraphNode{testGraphNode("n1", c.NodeFact, "事实A"), testGraphNode("n2", c.NodeConclusion, "结论B")},
					Edges: []c.GraphEdge{{From: "n1", To: "n2", Kind: c.EdgeDerives}},
				},
				Details: c.HiddenDetails{Caveats: []string{"说明"}},
			},
			CompiledAt: time.Now().UTC(),
		}); err != nil {
			return nil, err
		}
		return store, nil
	}

	var stdout, stderr bytes.Buffer
	code := run([]string{"compile", "run", "--force", "--platform", "weibo", "--id", "QAu4U9USk"}, "/tmp/project", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("run() code = %d, stderr = %s", code, stderr.String())
	}
	var got c.Record
	if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
		t.Fatalf("json.Unmarshal(stdout) error = %v", err)
	}
	if got.Output.Summary != "forced summary" {
		t.Fatalf("Summary = %q, want forced summary", got.Output.Summary)
	}
}

func TestRunHarnessPersistsNoNetworkIngestCompileAndMemoryFlow(t *testing.T) {
	prevBuildApp := buildApp
	prevBuildCompileClient := buildCompileClient
	prevOpenSQLiteStore := openSQLiteStore
	t.Cleanup(func() {
		buildApp = prevBuildApp
		buildCompileClient = prevBuildCompileClient
		openSQLiteStore = prevOpenSQLiteStore
	})

	tmp := t.TempDir()
	dbPath := tmp + "/content.db"
	rawURL := "https://x.com/VarixHarness/status/12345"
	rawCapture := types.RawContent{
		Source:     "twitter",
		ExternalID: "12345",
		Content:    "CPI cooled again, so yields may fall and equities could re-rate.",
		AuthorName: "Macro Alice",
		URL:        rawURL,
		PostedAt:   time.Date(2026, 4, 15, 9, 30, 0, 0, time.UTC),
	}
	compiledAt := time.Date(2026, 4, 16, 8, 0, 0, 0, time.UTC)

	openStore := func(t *testing.T) *contentstore.SQLiteStore {
		t.Helper()
		store, err := contentstore.NewSQLiteStore(dbPath)
		if err != nil {
			t.Fatalf("NewSQLiteStore(%q) error = %v", dbPath, err)
		}
		return store
	}

	var fetchCount int
	buildApp = func(projectRoot string) (*bootstrap.App, error) {
		store, err := contentstore.NewSQLiteStore(dbPath)
		if err != nil {
			return nil, err
		}
		dispatch := dispatcher.New(
			func(raw string) (types.ParsedURL, error) {
				return types.ParsedURL{
					Platform:     types.PlatformTwitter,
					ContentType:  types.ContentTypePost,
					PlatformID:   rawCapture.ExternalID,
					CanonicalURL: raw,
				}, nil
			},
			[]dispatcher.ItemSource{fakeItemSource{
				platform: types.PlatformTwitter,
				kind:     types.KindNative,
				items: []types.RawContent{{
					Source:     rawCapture.Source,
					ExternalID: rawCapture.ExternalID,
					Content:    rawCapture.Content,
					AuthorName: rawCapture.AuthorName,
					URL:        rawCapture.URL,
					PostedAt:   rawCapture.PostedAt,
				}},
			}},
			nil,
			nil,
		)
		app := &bootstrap.App{
			Dispatcher: dispatch,
			Polling: polling.New(
				store,
				dispatch,
				nil,
			),
		}
		app.Settings.ContentDBPath = dbPath
		return app, nil
	}

	buildCompileClient = func(projectRoot string) compileClient {
		return fakeCompileClient{
			compileFn: func(_ context.Context, bundle c.Bundle) (c.Record, error) {
				fetchCount++
				if bundle.Source != rawCapture.Source {
					t.Fatalf("bundle.Source = %q, want %q", bundle.Source, rawCapture.Source)
				}
				if bundle.ExternalID != rawCapture.ExternalID {
					t.Fatalf("bundle.ExternalID = %q, want %q", bundle.ExternalID, rawCapture.ExternalID)
				}
				if bundle.Content != rawCapture.Content {
					t.Fatalf("bundle.Content = %q, want persisted raw capture content", bundle.Content)
				}
				return c.Record{
					UnitID:         "twitter:12345",
					Source:         rawCapture.Source,
					ExternalID:     rawCapture.ExternalID,
					RootExternalID: rawCapture.ExternalID,
					Model:          c.Qwen36PlusModel,
					Metrics:        c.RecordMetrics{CompileElapsedMS: 123, CompileStageElapsedMS: map[string]int64{"unified_generator": 11, "unified_challenge": 22, "unified_judge": 33}},
					Output: c.Output{
						Summary: "Cooling CPI points to lower yields and a bullish risk setup.",
						Graph: c.ReasoningGraph{
							Nodes: []c.GraphNode{
								{ID: "n1", Kind: c.NodeFact, Text: "CPI cooled again", OccurredAt: rawCapture.PostedAt},
								{ID: "n2", Kind: c.NodeConclusion, Text: "Yields may fall"},
								{ID: "n3", Kind: c.NodePrediction, Text: "Equities may re-rate higher", PredictionStartAt: rawCapture.PostedAt},
							},
							Edges: []c.GraphEdge{
								{From: "n1", To: "n2", Kind: c.EdgeDerives},
								{From: "n2", To: "n3", Kind: c.EdgeDerives},
							},
						},
						Details: c.HiddenDetails{Caveats: []string{"Macro path can reverse quickly."}},
					},
					CompiledAt: compiledAt,
				}, nil
			},
		}
	}

	openSQLiteStore = func(path string) (*contentstore.SQLiteStore, error) {
		return contentstore.NewSQLiteStore(path)
	}

	app, err := buildApp("/tmp/project")
	if err != nil {
		t.Fatalf("buildApp() error = %v", err)
	}
	fetched, err := app.Polling.FetchURL(context.Background(), rawURL)
	if err != nil {
		t.Fatalf("app.Polling.FetchURL() error = %v", err)
	}
	if len(fetched) != 1 || fetched[0].ExternalID != rawCapture.ExternalID {
		t.Fatalf("FetchURL() = %#v, want one persisted raw capture", fetched)
	}

	store := openStore(t)
	persistedRaw, err := store.GetRawCapture(context.Background(), rawCapture.Source, rawCapture.ExternalID)
	store.Close()
	if err != nil {
		t.Fatalf("GetRawCapture() error = %v", err)
	}
	if persistedRaw.Content != rawCapture.Content {
		t.Fatalf("persisted raw content = %q, want %q", persistedRaw.Content, rawCapture.Content)
	}

	var stdout, stderr bytes.Buffer
	code := run([]string{"compile", "run", "--platform", rawCapture.Source, "--id", rawCapture.ExternalID}, "/tmp/project", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("compile run code = %d, stderr = %s", code, stderr.String())
	}
	if fetchCount != 1 {
		t.Fatalf("compile client calls = %d, want 1", fetchCount)
	}
	var compiled c.Record
	if err := json.Unmarshal(stdout.Bytes(), &compiled); err != nil {
		t.Fatalf("json.Unmarshal(compile stdout) error = %v", err)
	}
	if compiled.Output.Summary == "" {
		t.Fatalf("compiled stdout = %#v, want summary", compiled)
	}
	if compiled.Metrics.CompileElapsedMS <= 0 {
		t.Fatalf("compiled metrics = %#v, want positive compile elapsed ms", compiled.Metrics)
	}
	for _, stage := range []string{"unified_generator", "unified_challenge", "unified_judge"} {
		if compiled.Metrics.CompileStageElapsedMS[stage] <= 0 {
			t.Fatalf("compiled stage metrics = %#v, want positive duration for %q", compiled.Metrics.CompileStageElapsedMS, stage)
		}
	}

	store = openStore(t)
	persistedCompiled, err := store.GetCompiledOutput(context.Background(), rawCapture.Source, rawCapture.ExternalID)
	store.Close()
	if err != nil {
		t.Fatalf("GetCompiledOutput() error = %v", err)
	}
	if persistedCompiled.Output.Summary != compiled.Output.Summary {
		t.Fatalf("persisted compiled summary = %q, want %q", persistedCompiled.Output.Summary, compiled.Output.Summary)
	}
	if persistedCompiled.Metrics.CompileElapsedMS <= 0 {
		t.Fatalf("persisted compiled metrics = %#v, want positive compile elapsed ms", persistedCompiled.Metrics)
	}
	for _, stage := range []string{"unified_generator", "unified_challenge", "unified_judge"} {
		if persistedCompiled.Metrics.CompileStageElapsedMS[stage] <= 0 {
			t.Fatalf("persisted stage metrics = %#v, want positive duration for %q", persistedCompiled.Metrics.CompileStageElapsedMS, stage)
		}
	}

	stdout.Reset()
	stderr.Reset()
	code = run([]string{"memory", "accept-batch", "--user", "u-harness", "--platform", rawCapture.Source, "--id", rawCapture.ExternalID, "--nodes", "n1,n2"}, "/tmp/project", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("memory accept-batch code = %d, stderr = %s", code, stderr.String())
	}
	var accepted memory.AcceptResult
	if err := json.Unmarshal(stdout.Bytes(), &accepted); err != nil {
		t.Fatalf("json.Unmarshal(accept-batch stdout) error = %v", err)
	}
	if len(accepted.Nodes) != 2 {
		t.Fatalf("accept-batch nodes = %#v, want 2", accepted.Nodes)
	}

	stdout.Reset()
	stderr.Reset()
	code = run([]string{"memory", "organize-run", "--user", "u-harness"}, "/tmp/project", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("memory organize-run code = %d, stderr = %s", code, stderr.String())
	}
	var organized memory.OrganizationOutput
	if err := json.Unmarshal(stdout.Bytes(), &organized); err != nil {
		t.Fatalf("json.Unmarshal(organize-run stdout) error = %v", err)
	}
	if organized.JobID == 0 {
		t.Fatalf("organize-run stdout = %#v, want job id", organized)
	}

	store = openStore(t)
	defer store.Close()
	nodes, err := store.ListUserMemoryBySource(context.Background(), "u-harness", rawCapture.Source, rawCapture.ExternalID)
	if err != nil {
		t.Fatalf("ListUserMemoryBySource() error = %v", err)
	}
	if len(nodes) != 2 {
		t.Fatalf("len(ListUserMemoryBySource) = %d, want 2", len(nodes))
	}
	persistedOutput, err := store.GetLatestMemoryOrganizationOutput(context.Background(), "u-harness", rawCapture.Source, rawCapture.ExternalID)
	if err != nil {
		t.Fatalf("GetLatestMemoryOrganizationOutput() error = %v", err)
	}
	if persistedOutput.JobID != organized.JobID {
		t.Fatalf("persisted JobID = %d, want %d", persistedOutput.JobID, organized.JobID)
	}
	if len(persistedOutput.ActiveNodes) != 2 {
		t.Fatalf("len(persisted active nodes) = %d, want 2", len(persistedOutput.ActiveNodes))
	}
	if len(persistedOutput.Hierarchy) == 0 {
		t.Fatalf("persisted hierarchy = %#v, want derived links", persistedOutput.Hierarchy)
	}
	if fetchCount != 1 {
		t.Fatalf("unexpected refetch/compile count = %d, want 1 total compile invocation", fetchCount)
	}
}

func TestRunCompileShowReadsCompiledRecordByPlatformAndID(t *testing.T) {
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
		app.Dispatcher = dispatcher.New(
			func(raw string) (types.ParsedURL, error) {
				return types.ParsedURL{Platform: types.PlatformWeibo, PlatformID: "QAu4U9USk", CanonicalURL: raw}, nil
			},
			[]dispatcher.ItemSource{fakeItemSource{}},
			nil,
			nil,
		)
		return app, nil
	}
	openSQLiteStore = func(path string) (*contentstore.SQLiteStore, error) {
		store, err := contentstore.NewSQLiteStore(path)
		if err != nil {
			return nil, err
		}
		record := c.Record{
			UnitID:         "weibo:QAu4U9USk",
			Source:         "weibo",
			ExternalID:     "QAu4U9USk",
			RootExternalID: "QAu4U9USk",
			Model:          c.Qwen36PlusModel,
			Metrics:        c.RecordMetrics{CompileElapsedMS: 321, CompileStageElapsedMS: map[string]int64{"unified_generator": 111, "unified_challenge": 99, "unified_judge": 88}},
			Output: c.Output{
				Summary: "一句话",
				Graph: c.ReasoningGraph{
					Nodes: []c.GraphNode{testGraphNode("n1", c.NodeFact, "事实A"), testGraphNode("n2", c.NodeConclusion, "结论B")},
					Edges: []c.GraphEdge{{From: "n1", To: "n2", Kind: c.EdgeDerives}},
				},
				Details: c.HiddenDetails{Caveats: []string{"说明"}},
			},
			CompiledAt: time.Now().UTC(),
		}
		if err := store.UpsertCompiledOutput(context.Background(), record); err != nil {
			return nil, err
		}
		return store, nil
	}

	var stdout, stderr bytes.Buffer
	code := run([]string{"compile", "show", "--platform", "weibo", "--id", "QAu4U9USk"}, "/tmp/project", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("run() code = %d, stderr = %s", code, stderr.String())
	}
	var got c.Record
	if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
		t.Fatalf("json.Unmarshal(stdout) error = %v", err)
	}
	if got.Output.Summary != "一句话" {
		t.Fatalf("Summary = %q", got.Output.Summary)
	}
	if got.Metrics.CompileElapsedMS != 321 {
		t.Fatalf("CompileElapsedMS = %d, want 321", got.Metrics.CompileElapsedMS)
	}
	if got.Metrics.CompileStageElapsedMS["unified_generator"] != 111 || got.Metrics.CompileStageElapsedMS["unified_challenge"] != 99 || got.Metrics.CompileStageElapsedMS["unified_judge"] != 88 {
		t.Fatalf("CompileStageElapsedMS = %#v, want persisted stage metrics", got.Metrics.CompileStageElapsedMS)
	}
}

func TestRunCompileShowReadsCompiledRecordByURL(t *testing.T) {
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
		app.Dispatcher = dispatcher.New(
			func(raw string) (types.ParsedURL, error) {
				return types.ParsedURL{Platform: types.PlatformTwitter, PlatformID: "2026305745872998803", CanonicalURL: raw}, nil
			},
			[]dispatcher.ItemSource{fakeItemSource{}},
			nil,
			nil,
		)
		return app, nil
	}
	openSQLiteStore = func(path string) (*contentstore.SQLiteStore, error) {
		store, err := contentstore.NewSQLiteStore(path)
		if err != nil {
			return nil, err
		}
		record := c.Record{
			UnitID:         "twitter:2026305745872998803",
			Source:         "twitter",
			ExternalID:     "2026305745872998803",
			RootExternalID: "2026305745872998803",
			Model:          c.Qwen36PlusModel,
			Metrics:        c.RecordMetrics{CompileElapsedMS: 654, CompileStageElapsedMS: map[string]int64{"unified_generator": 222, "unified_challenge": 211, "unified_judge": 201}},
			Output: c.Output{
				Summary: "Dalio summary",
				Graph: c.ReasoningGraph{
					Nodes: []c.GraphNode{testGraphNode("n1", c.NodeFact, "事实A"), testGraphNode("n2", c.NodeConclusion, "结论B")},
					Edges: []c.GraphEdge{{From: "n1", To: "n2", Kind: c.EdgeDerives}},
				},
				Details: c.HiddenDetails{Caveats: []string{"说明"}},
			},
			CompiledAt: time.Now().UTC(),
		}
		if err := store.UpsertCompiledOutput(context.Background(), record); err != nil {
			return nil, err
		}
		return store, nil
	}

	var stdout, stderr bytes.Buffer
	code := run([]string{"compile", "show", "--url", "https://x.com/RayDalio/status/2026305745872998803"}, "/tmp/project", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("run() code = %d, stderr = %s", code, stderr.String())
	}
	var got c.Record
	if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
		t.Fatalf("json.Unmarshal(stdout) error = %v", err)
	}
	if got.Output.Summary != "Dalio summary" {
		t.Fatalf("Summary = %q", got.Output.Summary)
	}
	if got.Metrics.CompileElapsedMS != 654 {
		t.Fatalf("CompileElapsedMS = %d, want 654", got.Metrics.CompileElapsedMS)
	}
	if got.Metrics.CompileStageElapsedMS["unified_generator"] != 222 || got.Metrics.CompileStageElapsedMS["unified_challenge"] != 211 || got.Metrics.CompileStageElapsedMS["unified_judge"] != 201 {
		t.Fatalf("CompileStageElapsedMS = %#v, want persisted URL stage metrics", got.Metrics.CompileStageElapsedMS)
	}
}

func TestRunCompileSummaryPrintsHumanReadableOutput(t *testing.T) {
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
		record := c.Record{
			UnitID:         "weibo:QAu4U9USk",
			Source:         "weibo",
			ExternalID:     "QAu4U9USk",
			RootExternalID: "QAu4U9USk",
			Model:          c.Qwen36PlusModel,
			Output: c.Output{
				Summary:           "一句话",
				Drivers:           []string{"驱动A"},
				Targets:           []string{"目标B"},
				TransmissionPaths: []c.TransmissionPath{{Driver: "驱动A", Target: "目标B", Steps: []string{"中间步骤"}}},
				Graph: c.ReasoningGraph{
					Nodes: []c.GraphNode{testGraphNode("n1", c.NodeMechanism, "驱动A"), testGraphNode("n2", c.NodeConclusion, "目标B")},
					Edges: []c.GraphEdge{{From: "n1", To: "n2", Kind: c.EdgePositive}},
				},
				Details:    c.HiddenDetails{Caveats: []string{"说明"}},
				Topics:     []string{"topic-a", "topic-b"},
				Confidence: "medium",
			},
			CompiledAt: time.Now().UTC(),
		}
		if err := store.UpsertCompiledOutput(context.Background(), record); err != nil {
			return nil, err
		}
		return store, nil
	}

	var stdout, stderr bytes.Buffer
	code := run([]string{"compile", "summary", "--platform", "weibo", "--id", "QAu4U9USk"}, "/tmp/project", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("run() code = %d, stderr = %s", code, stderr.String())
	}
	out := stdout.String()
	for _, want := range []string{"Summary: 一句话", "Drivers: 1", "Targets: 1", "Paths: 1", "Topics: topic-a, topic-b", "Confidence: medium"} {
		if !strings.Contains(out, want) {
			t.Fatalf("stdout missing %q in %q", want, out)
		}
	}
	if !strings.Contains(out, "Compile elapsed:") || !strings.Contains(out, "Stages:") {
		t.Fatalf("stdout = %q, want compile metrics in summary view", out)
	}
}

func TestRunCompileGoldScoreOutputsReviewItems(t *testing.T) {
	tmp := t.TempDir()
	goldPath := filepath.Join(tmp, "gold.json")
	candidatePath := filepath.Join(tmp, "candidate.json")
	gold := c.GoldDataset{
		Version: "test-v1",
		Samples: []c.GoldSample{{
			ID:      "G04",
			Summary: "海外资金继续流入美国资产，说明美国增长叙事仍然吸引全球资金",
			Drivers: []string{
				"美国增长叙事仍然吸引全球资金",
				"政治风险没有压倒市场对美国资产的增长偏好",
			},
			Targets: []string{
				"海外资金继续流入美国资产",
				"没有形成 sell America 交易",
			},
		}},
	}
	candidates := []c.GoldCandidate{{
		SampleID: "G04",
		Output: c.Output{
			Summary: "美国政治风险导致美元走弱",
			Drivers: []string{
				"美国增长叙事吸引全球资金",
				"美联储政治化压低收益率",
			},
			Targets: []string{
				"海外资金继续流入美国资产",
				"美元下跌",
			},
		},
	}}
	writeTestJSONFile(t, goldPath, gold)
	writeTestJSONFile(t, candidatePath, candidates)

	var stdout, stderr bytes.Buffer
	code := run([]string{"compile", "gold-score", "--gold", goldPath, "--candidate", candidatePath}, "/tmp/project", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("run() code = %d, stderr = %s", code, stderr.String())
	}
	var out c.GoldScorecard
	if err := json.Unmarshal(stdout.Bytes(), &out); err != nil {
		t.Fatalf("json.Unmarshal(stdout) error = %v; stdout=%s", err, stdout.String())
	}
	if out.DatasetVersion != "test-v1" || out.SampleCount != 1 {
		t.Fatalf("scorecard = %#v, want dataset metadata", out)
	}
	if len(out.Samples) != 1 || len(out.Samples[0].ReviewItems) == 0 {
		t.Fatalf("scorecard missing review items: %#v", out)
	}
}

func TestRunCompileGoldScoreReadsCandidateDir(t *testing.T) {
	tmp := t.TempDir()
	goldPath := filepath.Join(tmp, "gold.json")
	candidateDir := filepath.Join(tmp, "candidates")
	outPath := filepath.Join(tmp, "scorecard.json")
	if err := os.Mkdir(candidateDir, 0o755); err != nil {
		t.Fatalf("os.Mkdir() error = %v", err)
	}
	gold := c.GoldDataset{
		Version: "test-v1",
		Samples: []c.GoldSample{{
			ID:      "G01",
			Summary: "资金流入硬资产以对冲货币贬值",
			Drivers: []string{
				"央行扩表压低实际利率",
			},
			Targets: []string{
				"资金流入硬资产",
			},
		}},
	}
	report := struct {
		UnitID string   `json:"unit_id"`
		Output c.Output `json:"output"`
	}{
		UnitID: "twitter:1",
		Output: c.Output{
			Summary: "实际利率为负导致资金买入黄金",
			Drivers: []string{
				"实际利率下降",
			},
			Targets: []string{
				"资金买入黄金",
			},
		},
	}
	writeTestJSONFile(t, goldPath, gold)
	writeTestJSONFile(t, filepath.Join(candidateDir, "G01.json"), report)

	var stdout, stderr bytes.Buffer
	code := run([]string{"compile", "gold-score", "--gold", goldPath, "--candidate-dir", candidateDir, "--out", outPath}, "/tmp/project", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("run() code = %d, stderr = %s", code, stderr.String())
	}
	var out c.GoldScorecard
	if err := json.Unmarshal(stdout.Bytes(), &out); err != nil {
		t.Fatalf("json.Unmarshal(stdout) error = %v; stdout=%s", err, stdout.String())
	}
	if out.SampleCount != 1 || len(out.Samples) != 1 {
		t.Fatalf("scorecard = %#v, want one scored sample", out)
	}
	if out.Samples[0].ID != "G01" {
		t.Fatalf("sample id = %q, want G01", out.Samples[0].ID)
	}
	if len(out.Samples[0].ReviewItems) == 0 {
		t.Fatalf("scorecard missing review items: %#v", out)
	}
	rawOut, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("os.ReadFile(outPath) error = %v", err)
	}
	var fileOut c.GoldScorecard
	if err := json.Unmarshal(rawOut, &fileOut); err != nil {
		t.Fatalf("json.Unmarshal(outPath) error = %v; raw=%s", err, string(rawOut))
	}
	if fileOut.DatasetVersion != "test-v1" {
		t.Fatalf("file scorecard version = %q, want test-v1", fileOut.DatasetVersion)
	}
}

func TestRunCompileSummaryReadsByURL(t *testing.T) {
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
		app.Dispatcher = dispatcher.New(
			func(raw string) (types.ParsedURL, error) {
				return types.ParsedURL{Platform: types.PlatformTwitter, PlatformID: "2026305745872998803", CanonicalURL: raw}, nil
			},
			[]dispatcher.ItemSource{fakeItemSource{}},
			nil,
			nil,
		)
		return app, nil
	}
	openSQLiteStore = func(path string) (*contentstore.SQLiteStore, error) {
		store, err := contentstore.NewSQLiteStore(path)
		if err != nil {
			return nil, err
		}
		record := c.Record{
			UnitID:         "twitter:2026305745872998803",
			Source:         "twitter",
			ExternalID:     "2026305745872998803",
			RootExternalID: "2026305745872998803",
			Model:          c.Qwen36PlusModel,
			Output: c.Output{
				Summary:           "Dalio summary",
				Drivers:           []string{"驱动A"},
				Targets:           []string{"目标B"},
				TransmissionPaths: []c.TransmissionPath{{Driver: "驱动A", Target: "目标B", Steps: []string{"中间步骤"}}},
				Graph: c.ReasoningGraph{
					Nodes: []c.GraphNode{testGraphNode("n1", c.NodeMechanism, "驱动A"), testGraphNode("n2", c.NodeConclusion, "目标B")},
					Edges: []c.GraphEdge{{From: "n1", To: "n2", Kind: c.EdgePositive}},
				},
				Details:    c.HiddenDetails{Caveats: []string{"说明"}},
				Topics:     []string{"macro"},
				Confidence: "high",
			},
			CompiledAt: time.Now().UTC(),
		}
		if err := store.UpsertCompiledOutput(context.Background(), record); err != nil {
			return nil, err
		}
		return store, nil
	}

	var stdout, stderr bytes.Buffer
	code := run([]string{"compile", "summary", "--url", "https://x.com/RayDalio/status/2026305745872998803"}, "/tmp/project", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("run() code = %d, stderr = %s", code, stderr.String())
	}
	out := stdout.String()
	for _, want := range []string{"Summary: Dalio summary", "Drivers: 1", "Targets: 1", "Paths: 1", "Topics: macro", "Confidence: high"} {
		if !strings.Contains(out, want) {
			t.Fatalf("stdout missing %q in %q", want, out)
		}
	}
	if !strings.Contains(out, "Compile elapsed:") || !strings.Contains(out, "Stages:") {
		t.Fatalf("stdout = %q, want compile metrics in summary-by-url view", out)
	}
}

func TestRunCompileComparePrintsRawPreviewAndSummary(t *testing.T) {
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
		if err := store.UpsertRawCapture(context.Background(), types.RawContent{
			Source:     "weibo",
			ExternalID: "QAu4U9USk",
			Content:    "原文正文",
			AuthorName: "Alice",
			URL:        "https://weibo.com/1182426800/QAu4U9USk",
		}); err != nil {
			return nil, err
		}
		record := c.Record{
			UnitID:         "weibo:QAu4U9USk",
			Source:         "weibo",
			ExternalID:     "QAu4U9USk",
			RootExternalID: "QAu4U9USk",
			Model:          c.Qwen36PlusModel,
			Output: c.Output{
				Summary:           "一句话",
				Drivers:           []string{"驱动A"},
				Targets:           []string{"目标B"},
				TransmissionPaths: []c.TransmissionPath{{Driver: "驱动A", Target: "目标B", Steps: []string{"中间步骤"}}},
				Graph: c.ReasoningGraph{
					Nodes: []c.GraphNode{testGraphNode("n1", c.NodeMechanism, "驱动A"), testGraphNode("n2", c.NodeConclusion, "目标B")},
					Edges: []c.GraphEdge{{From: "n1", To: "n2", Kind: c.EdgePositive}},
				},
				Details:    c.HiddenDetails{Caveats: []string{"说明"}},
				Confidence: "medium",
			},
			CompiledAt: time.Now().UTC(),
		}
		if err := store.UpsertCompiledOutput(context.Background(), record); err != nil {
			return nil, err
		}
		return store, nil
	}

	var stdout, stderr bytes.Buffer
	code := run([]string{"compile", "compare", "--platform", "weibo", "--id", "QAu4U9USk"}, "/tmp/project", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("run() code = %d, stderr = %s", code, stderr.String())
	}
	out := stdout.String()
	for _, want := range []string{"Raw preview: 原文正文", "Summary: 一句话", "Drivers: 1", "Targets: 1", "Paths: 1", "Confidence: medium"} {
		if !strings.Contains(out, want) {
			t.Fatalf("stdout missing %q in %q", want, out)
		}
	}
	if !strings.Contains(out, "Compile elapsed:") || !strings.Contains(out, "Stages:") {
		t.Fatalf("stdout = %q, want compile metrics in compare view", out)
	}
}

func TestRunCompileCompareReadsByURL(t *testing.T) {
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
		app.Dispatcher = dispatcher.New(
			func(raw string) (types.ParsedURL, error) {
				return types.ParsedURL{Platform: types.PlatformTwitter, PlatformID: "2026305745872998803", CanonicalURL: raw}, nil
			},
			[]dispatcher.ItemSource{fakeItemSource{}},
			nil,
			nil,
		)
		return app, nil
	}
	openSQLiteStore = func(path string) (*contentstore.SQLiteStore, error) {
		store, err := contentstore.NewSQLiteStore(path)
		if err != nil {
			return nil, err
		}
		if err := store.UpsertRawCapture(context.Background(), types.RawContent{
			Source:     "twitter",
			ExternalID: "2026305745872998803",
			Content:    "dalio raw body",
			AuthorName: "Ray Dalio",
			URL:        "https://x.com/RayDalio/status/2026305745872998803",
		}); err != nil {
			return nil, err
		}
		record := c.Record{
			UnitID:         "twitter:2026305745872998803",
			Source:         "twitter",
			ExternalID:     "2026305745872998803",
			RootExternalID: "2026305745872998803",
			Model:          c.Qwen36PlusModel,
			Output: c.Output{
				Summary:           "Dalio summary",
				Drivers:           []string{"驱动A"},
				Targets:           []string{"目标B"},
				TransmissionPaths: []c.TransmissionPath{{Driver: "驱动A", Target: "目标B", Steps: []string{"中间步骤"}}},
				Graph: c.ReasoningGraph{
					Nodes: []c.GraphNode{testGraphNode("n1", c.NodeMechanism, "驱动A"), testGraphNode("n2", c.NodeConclusion, "目标B")},
					Edges: []c.GraphEdge{{From: "n1", To: "n2", Kind: c.EdgePositive}},
				},
				Details:    c.HiddenDetails{Caveats: []string{"说明"}},
				Confidence: "high",
			},
			CompiledAt: time.Now().UTC(),
		}
		if err := store.UpsertCompiledOutput(context.Background(), record); err != nil {
			return nil, err
		}
		return store, nil
	}

	var stdout, stderr bytes.Buffer
	code := run([]string{"compile", "compare", "--url", "https://x.com/RayDalio/status/2026305745872998803"}, "/tmp/project", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("run() code = %d, stderr = %s", code, stderr.String())
	}
	out := stdout.String()
	for _, want := range []string{"Raw preview: dalio raw body", "Summary: Dalio summary", "Drivers: 1", "Targets: 1", "Paths: 1", "Confidence: high"} {
		if !strings.Contains(out, want) {
			t.Fatalf("stdout missing %q in %q", want, out)
		}
	}
	if !strings.Contains(out, "Compile elapsed:") || !strings.Contains(out, "Stages:") {
		t.Fatalf("stdout = %q, want compile metrics in compare-by-url view", out)
	}
}

func TestRunCompileCardPrintsHumanReadableCard(t *testing.T) {
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
		record := c.Record{
			UnitID:         "twitter:1",
			Source:         "twitter",
			ExternalID:     "1",
			RootExternalID: "1",
			Model:          c.Qwen36PlusModel,
			Output: c.Output{
				Summary:           "一句话总结",
				Drivers:           []string{"驱动A"},
				Targets:           []string{"目标B"},
				TransmissionPaths: []c.TransmissionPath{{Driver: "驱动A", Target: "目标B", Steps: []string{"中间步骤"}}},
				Branches: []c.Branch{{
					ID:                "s1",
					Level:             "primary",
					Thesis:            "分支论点",
					Anchors:           []string{"总前提"},
					BranchDrivers:     []string{"分支机制"},
					Drivers:           []string{"驱动A"},
					Targets:           []string{"目标B"},
					TransmissionPaths: []c.TransmissionPath{{Driver: "驱动A", Target: "目标B", Steps: []string{"中间步骤"}}},
				}},
				EvidenceNodes:    []string{"证据A"},
				ExplanationNodes: []string{"解释B"},
				Graph: c.ReasoningGraph{
					Nodes: []c.GraphNode{testGraphNode("n1", c.NodeMechanism, "驱动A"), testGraphNode("n2", c.NodeConclusion, "目标B")},
					Edges: []c.GraphEdge{{From: "n1", To: "n2", Kind: c.EdgePositive}},
				},
				Details:    c.HiddenDetails{Caveats: []string{"说明"}},
				Topics:     []string{"topic-a", "topic-b"},
				Confidence: "high",
				AuthorValidation: c.AuthorValidation{
					Version: "author_validate_v1",
					Summary: c.AuthorValidationSummary{
						Verdict:               "mixed",
						SupportedClaims:       1,
						UnverifiedClaims:      1,
						SoundInferences:       1,
						UnsupportedInferences: 1,
					},
					ClaimChecks: []c.AuthorClaimCheck{{
						ClaimID: "claim-001",
						Text:    "目标B",
						Status:  c.AuthorClaimUnverified,
					}},
					InferenceChecks: []c.AuthorInferenceCheck{{
						InferenceID: "inference-001",
						From:        "驱动A",
						To:          "目标B",
						Status:      c.AuthorInferenceUnsupportedJump,
					}},
				},
			},
			CompiledAt: time.Now().UTC(),
		}
		if err := store.UpsertCompiledOutput(context.Background(), record); err != nil {
			return nil, err
		}
		return store, nil
	}

	var stdout, stderr bytes.Buffer
	code := run([]string{"compile", "card", "--platform", "twitter", "--id", "1"}, "/tmp/project", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("run() code = %d, stderr = %s", code, stderr.String())
	}
	out := stdout.String()
	for _, want := range []string{"Summary", "一句话总结", "Topics", "topic-a", "Branches", "分支论点", "Anchor: 总前提", "Branch driver: 分支机制", "驱动A -> 中间步骤 -> 目标B", "Logic chain", "Author validation", "Verdict: mixed", "Claims: supported 1, contradicted 0, unverified 1, interpretive 0", "Path 驱动A -> 目标B: unsupported_jump", "Confidence", "high"} {
		if !strings.Contains(out, want) {
			t.Fatalf("stdout missing %q in %q", want, out)
		}
	}
}

func TestRunCompileCardShowsGraphFirstExpandedViewAndVerificationSummary(t *testing.T) {
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
		record := c.Record{
			UnitID:         "twitter:expanded-view",
			Source:         "twitter",
			ExternalID:     "expanded-view",
			RootExternalID: "expanded-view",
			Model:          c.Qwen36PlusModel,
			Output: c.Output{
				Summary:           "一句话总结",
				Drivers:           []string{"旧驱动"},
				Targets:           []string{"旧目标"},
				TransmissionPaths: []c.TransmissionPath{{Driver: "旧驱动", Target: "旧目标", Steps: []string{"旧中间步骤"}}},
				EvidenceNodes:     []string{"旧证据"},
				ExplanationNodes:  []string{"旧解释"},
				Topics:            []string{"topic-a", "topic-b"},
				Graph: c.ReasoningGraph{
					Nodes: []c.GraphNode{testGraphNode("legacy-driver", c.NodeMechanism, "旧驱动"), testGraphNode("legacy-target", c.NodeConclusion, "旧目标")},
					Edges: []c.GraphEdge{{From: "legacy-driver", To: "legacy-target", Kind: c.EdgePositive}},
				},
				Details:    c.HiddenDetails{Caveats: []string{"说明"}},
				Confidence: "high",
			},
			CompiledAt: time.Now().UTC(),
		}
		if err := store.UpsertCompiledOutput(context.Background(), record); err != nil {
			return nil, err
		}
		subgraph := graphmodel.ContentSubgraph{
			ID:               "twitter:expanded-view",
			ArticleID:        "twitter:expanded-view",
			SourcePlatform:   "twitter",
			SourceExternalID: "expanded-view",
			RootExternalID:   "expanded-view",
			CompileVersion:   graphmodel.CompileBridgeVersion,
			CompiledAt:       time.Now().UTC().Format(time.RFC3339),
			UpdatedAt:        time.Now().UTC().Format(time.RFC3339),
			Nodes: []graphmodel.GraphNode{
				{ID: "n1", SourceArticleID: "twitter:expanded-view", SourcePlatform: "twitter", SourceExternalID: "expanded-view", RawText: "美联储加息0.25%", SubjectText: "美联储", ChangeText: "加息0.25%", Kind: graphmodel.NodeKindObservation, GraphRole: graphmodel.GraphRoleDriver, IsPrimary: true, VerificationStatus: graphmodel.VerificationProved},
				{ID: "n2", SourceArticleID: "twitter:expanded-view", SourcePlatform: "twitter", SourceExternalID: "expanded-view", RawText: "流动性收紧", SubjectText: "流动性", ChangeText: "收紧", Kind: graphmodel.NodeKindObservation, GraphRole: graphmodel.GraphRoleIntermediate, IsPrimary: true, VerificationStatus: graphmodel.VerificationPending},
				{ID: "n3", SourceArticleID: "twitter:expanded-view", SourcePlatform: "twitter", SourceExternalID: "expanded-view", RawText: "未来一周美股承压", SubjectText: "美股", ChangeText: "承压", Kind: graphmodel.NodeKindPrediction, GraphRole: graphmodel.GraphRoleTarget, IsPrimary: true, VerificationStatus: graphmodel.VerificationPending},
				{ID: "n4", SourceArticleID: "twitter:expanded-view", SourcePlatform: "twitter", SourceExternalID: "expanded-view", RawText: "CPI回落", SubjectText: "CPI", ChangeText: "回落", Kind: graphmodel.NodeKindObservation, GraphRole: graphmodel.GraphRoleEvidence, IsPrimary: false, VerificationStatus: graphmodel.VerificationProved},
				{ID: "n5", SourceArticleID: "twitter:expanded-view", SourcePlatform: "twitter", SourceExternalID: "expanded-view", RawText: "估值承压先传导到科技股", SubjectText: "科技股估值", ChangeText: "承压", Kind: graphmodel.NodeKindObservation, GraphRole: graphmodel.GraphRoleContext, IsPrimary: false, VerificationStatus: graphmodel.VerificationUnverifiable},
			},
			Edges: []graphmodel.GraphEdge{
				{ID: "n1->n2:drives", From: "n1", To: "n2", Type: graphmodel.EdgeTypeDrives, IsPrimary: true, VerificationStatus: graphmodel.VerificationProved},
				{ID: "n2->n3:drives", From: "n2", To: "n3", Type: graphmodel.EdgeTypeDrives, IsPrimary: true, VerificationStatus: graphmodel.VerificationPending},
				{ID: "n4->n1:supports", From: "n4", To: "n1", Type: graphmodel.EdgeTypeSupports, IsPrimary: false, VerificationStatus: graphmodel.VerificationProved},
				{ID: "n5->n3:explains", From: "n5", To: "n3", Type: graphmodel.EdgeTypeExplains, IsPrimary: false, VerificationStatus: graphmodel.VerificationUnverifiable},
			},
		}
		if err := store.UpsertContentSubgraph(context.Background(), subgraph); err != nil {
			return nil, err
		}
		return store, nil
	}

	var stdout, stderr bytes.Buffer
	code := run([]string{"compile", "card", "--platform", "twitter", "--id", "expanded-view"}, "/tmp/project", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("run() code = %d, stderr = %s", code, stderr.String())
	}
	out := stdout.String()
	for _, want := range []string{
		"Summary", "一句话总结",
		"Topics", "topic-a",
		"Drivers", "- 美联储加息0.25%",
		"Targets", "- 未来一周美股承压",
		"Evidence", "- CPI回落",
		"Explanations", "- 估值承压先传导到科技股",
		"Logic chain", "美联储加息0.25% -> 流动性收紧 -> 未来一周美股承压",
		"Verification",
		"Nodes: pending=2, proved=2, unverifiable=1",
		"Edges: pending=1, proved=2, unverifiable=1",
		"Confidence", "high",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("stdout missing %q in %q", want, out)
		}
	}
}

func TestRunCompileCardFallsBackToLegacyWhenGraphFirstSubgraphMissing(t *testing.T) {
	prevBuildApp := buildApp
	prevOpenSQLiteStore := openSQLiteStore
	t.Cleanup(func() {
		buildApp = prevBuildApp
		openSQLiteStore = prevOpenSQLiteStore
	})

	tmp := t.TempDir()
	dbPath := filepath.Join(tmp, "content.db")
	buildApp = func(projectRoot string) (*bootstrap.App, error) {
		app := &bootstrap.App{}
		app.Settings.ContentDBPath = dbPath
		return app, nil
	}
	openSQLiteStore = func(path string) (*contentstore.SQLiteStore, error) {
		store, err := contentstore.NewSQLiteStore(path)
		if err != nil {
			return nil, err
		}
		record := c.Record{
			UnitID:         "twitter:legacy-fallback-standard",
			Source:         "twitter",
			ExternalID:     "legacy-fallback-standard",
			RootExternalID: "legacy-fallback-standard",
			Model:          c.Qwen36PlusModel,
			Output: c.Output{
				Summary:           "一句话总结",
				Drivers:           []string{"驱动A"},
				Targets:           []string{"目标B"},
				TransmissionPaths: []c.TransmissionPath{{Driver: "驱动A", Target: "目标B", Steps: []string{"中间步骤"}}},
				EvidenceNodes:     []string{"证据A"},
				ExplanationNodes:  []string{"解释B"},
				Graph: c.ReasoningGraph{
					Nodes: []c.GraphNode{testGraphNode("legacy-driver", c.NodeMechanism, "驱动A"), testGraphNode("legacy-target", c.NodeConclusion, "目标B")},
					Edges: []c.GraphEdge{{From: "legacy-driver", To: "legacy-target", Kind: c.EdgePositive}},
				},
				Details:    c.HiddenDetails{Caveats: []string{"说明"}},
				Confidence: "high",
			},
			CompiledAt: time.Now().UTC(),
		}
		if err := store.UpsertCompiledOutput(context.Background(), record); err != nil {
			return nil, err
		}
		rawDB, err := sql.Open("sqlite", path)
		if err != nil {
			return nil, err
		}
		defer rawDB.Close()
		if _, err := rawDB.Exec(`DELETE FROM content_subgraphs WHERE platform = ? AND external_id = ?`, "twitter", "legacy-fallback-standard"); err != nil {
			return nil, err
		}
		return store, nil
	}

	var stdout, stderr bytes.Buffer
	code := run([]string{"compile", "card", "--platform", "twitter", "--id", "legacy-fallback-standard"}, "/tmp/project", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("run() code = %d, stderr = %s", code, stderr.String())
	}
	out := stdout.String()
	for _, want := range []string{"Drivers", "- 驱动A", "Targets", "- 目标B", "Evidence", "- 证据A", "Explanations", "- 解释B", "Logic chain", "驱动A -> 中间步骤 -> 目标B"} {
		if !strings.Contains(out, want) {
			t.Fatalf("stdout missing %q in %q", want, out)
		}
	}
	if strings.Contains(out, "Verification") {
		t.Fatalf("stdout = %q, did not want verification summary without subgraph", out)
	}
}

func TestRunCompileCardFallsBackToLegacyWhenGraphFirstProjectionIsLessInformative(t *testing.T) {
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
		record := c.Record{
			UnitID:         "twitter:less-informative-standard",
			Source:         "twitter",
			ExternalID:     "less-informative-standard",
			RootExternalID: "less-informative-standard",
			Model:          c.Qwen36PlusModel,
			Output: c.Output{
				Summary:           "一句话总结",
				Drivers:           []string{"旧驱动A", "旧驱动B"},
				Targets:           []string{"旧目标A", "旧目标B"},
				TransmissionPaths: []c.TransmissionPath{{Driver: "旧驱动A", Target: "旧目标A", Steps: []string{"旧中间步骤"}}},
				EvidenceNodes:     []string{"旧证据A", "旧证据B"},
				ExplanationNodes:  []string{"旧解释A", "旧解释B"},
				Graph: c.ReasoningGraph{
					Nodes: []c.GraphNode{testGraphNode("legacy-driver", c.NodeMechanism, "旧驱动A"), testGraphNode("legacy-target", c.NodeConclusion, "旧目标A")},
					Edges: []c.GraphEdge{{From: "legacy-driver", To: "legacy-target", Kind: c.EdgePositive}},
				},
				Details:    c.HiddenDetails{Caveats: []string{"说明"}},
				Confidence: "high",
			},
			CompiledAt: time.Now().UTC(),
		}
		if err := store.UpsertCompiledOutput(context.Background(), record); err != nil {
			return nil, err
		}
		subgraph := graphmodel.ContentSubgraph{
			ID:               "twitter:less-informative-standard",
			ArticleID:        "twitter:less-informative-standard",
			SourcePlatform:   "twitter",
			SourceExternalID: "less-informative-standard",
			RootExternalID:   "less-informative-standard",
			CompileVersion:   graphmodel.CompileBridgeVersion,
			CompiledAt:       time.Now().UTC().Format(time.RFC3339),
			UpdatedAt:        time.Now().UTC().Format(time.RFC3339),
			Nodes: []graphmodel.GraphNode{
				{ID: "n1", SourceArticleID: "twitter:less-informative-standard", SourcePlatform: "twitter", SourceExternalID: "less-informative-standard", RawText: "新驱动", SubjectText: "新驱动", ChangeText: "新驱动", Kind: graphmodel.NodeKindObservation, GraphRole: graphmodel.GraphRoleDriver, IsPrimary: true, VerificationStatus: graphmodel.VerificationPending},
				{ID: "n2", SourceArticleID: "twitter:less-informative-standard", SourcePlatform: "twitter", SourceExternalID: "less-informative-standard", RawText: "新目标", SubjectText: "新目标", ChangeText: "新目标", Kind: graphmodel.NodeKindPrediction, GraphRole: graphmodel.GraphRoleTarget, IsPrimary: true, VerificationStatus: graphmodel.VerificationPending},
				{ID: "n3", SourceArticleID: "twitter:less-informative-standard", SourcePlatform: "twitter", SourceExternalID: "less-informative-standard", RawText: "新证据", SubjectText: "新证据", ChangeText: "新证据", Kind: graphmodel.NodeKindObservation, GraphRole: graphmodel.GraphRoleEvidence, IsPrimary: false, VerificationStatus: graphmodel.VerificationPending},
				{ID: "n4", SourceArticleID: "twitter:less-informative-standard", SourcePlatform: "twitter", SourceExternalID: "less-informative-standard", RawText: "新解释", SubjectText: "新解释", ChangeText: "新解释", Kind: graphmodel.NodeKindObservation, GraphRole: graphmodel.GraphRoleContext, IsPrimary: false, VerificationStatus: graphmodel.VerificationPending},
			},
			Edges: []graphmodel.GraphEdge{
				{ID: "n1->n2:drives", From: "n1", To: "n2", Type: graphmodel.EdgeTypeDrives, IsPrimary: true, VerificationStatus: graphmodel.VerificationPending},
			},
		}
		if err := store.UpsertContentSubgraph(context.Background(), subgraph); err != nil {
			return nil, err
		}
		return store, nil
	}

	var stdout, stderr bytes.Buffer
	code := run([]string{"compile", "card", "--platform", "twitter", "--id", "less-informative-standard"}, "/tmp/project", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("run() code = %d, stderr = %s", code, stderr.String())
	}
	out := stdout.String()
	for _, want := range []string{"- 旧驱动A", "- 旧驱动B", "- 旧目标A", "- 旧目标B", "- 旧证据A", "- 旧证据B", "- 旧解释A", "- 旧解释B"} {
		if !strings.Contains(out, want) {
			t.Fatalf("stdout missing legacy section item %q in %q", want, out)
		}
	}
	for _, avoid := range []string{"- 新驱动", "- 新目标", "- 新证据", "- 新解释"} {
		if strings.Contains(out, avoid) {
			t.Fatalf("stdout = %q, did not want less-informative graph-first item %q", out, avoid)
		}
	}
}

func TestRunCompileCardCompactPrintsCompactView(t *testing.T) {
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
		record := c.Record{
			UnitID:         "twitter:1",
			Source:         "twitter",
			ExternalID:     "1",
			RootExternalID: "1",
			Model:          c.Qwen36PlusModel,
			Output: c.Output{
				Summary:           "一句话总结",
				Drivers:           []string{"驱动A"},
				Targets:           []string{"目标B"},
				TransmissionPaths: []c.TransmissionPath{{Driver: "驱动A", Target: "目标B", Steps: []string{"中间步骤"}}},
				EvidenceNodes:     []string{"证据A"},
				ExplanationNodes:  []string{"解释B"},
				Graph: c.ReasoningGraph{
					Nodes: []c.GraphNode{testGraphNode("n1", c.NodeMechanism, "驱动A"), testGraphNode("n2", c.NodeConclusion, "目标B")},
					Edges: []c.GraphEdge{{From: "n1", To: "n2", Kind: c.EdgePositive}},
				},
				Details:    c.HiddenDetails{Caveats: []string{"说明"}},
				Confidence: "high",
			},
			CompiledAt: time.Now().UTC(),
		}
		if err := store.UpsertCompiledOutput(context.Background(), record); err != nil {
			return nil, err
		}
		return store, nil
	}

	var stdout, stderr bytes.Buffer
	code := run([]string{"compile", "card", "--compact", "--platform", "twitter", "--id", "1"}, "/tmp/project", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("run() code = %d, stderr = %s", code, stderr.String())
	}
	out := stdout.String()
	for _, want := range []string{"Summary", "一句话总结", "Drivers", "- 驱动A", "Targets", "- 目标B", "Evidence", "- 证据A", "Explanations", "- 解释B", "Main logic", "驱动A -> 中间步骤 -> 目标B", "Confidence", "high"} {
		if !strings.Contains(out, want) {
			t.Fatalf("stdout missing %q in %q", want, out)
		}
	}
}

func TestRunCompileCardCompactReadsByURL(t *testing.T) {
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
		app.Dispatcher = dispatcher.New(
			func(raw string) (types.ParsedURL, error) {
				return types.ParsedURL{Platform: types.PlatformWeibo, PlatformID: "QAu4U9USk", CanonicalURL: raw}, nil
			},
			[]dispatcher.ItemSource{fakeItemSource{}},
			nil,
			nil,
		)
		return app, nil
	}
	openSQLiteStore = func(path string) (*contentstore.SQLiteStore, error) {
		store, err := contentstore.NewSQLiteStore(path)
		if err != nil {
			return nil, err
		}
		record := c.Record{
			UnitID:         "weibo:QAu4U9USk",
			Source:         "weibo",
			ExternalID:     "QAu4U9USk",
			RootExternalID: "QAu4U9USk",
			Model:          c.Qwen36PlusModel,
			Output: c.Output{
				Summary:           "一句话总结",
				Drivers:           []string{"驱动A"},
				Targets:           []string{"目标B"},
				TransmissionPaths: []c.TransmissionPath{{Driver: "驱动A", Target: "目标B", Steps: []string{"中间步骤"}}},
				EvidenceNodes:     []string{"证据A"},
				ExplanationNodes:  []string{"解释B"},
				Graph: c.ReasoningGraph{
					Nodes: []c.GraphNode{testGraphNode("n1", c.NodeMechanism, "驱动A"), testGraphNode("n2", c.NodeConclusion, "目标B")},
					Edges: []c.GraphEdge{{From: "n1", To: "n2", Kind: c.EdgePositive}},
				},
				Details:    c.HiddenDetails{Caveats: []string{"说明"}},
				Confidence: "high",
			},
			CompiledAt: time.Now().UTC(),
		}
		if err := store.UpsertCompiledOutput(context.Background(), record); err != nil {
			return nil, err
		}
		return store, nil
	}

	var stdout, stderr bytes.Buffer
	code := run([]string{"compile", "card", "--compact", "--url", "https://weibo.com/1182426800/QAu4U9USk"}, "/tmp/project", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("run() code = %d, stderr = %s", code, stderr.String())
	}
	out := stdout.String()
	for _, want := range []string{"Summary", "一句话总结", "Drivers", "- 驱动A", "Targets", "- 目标B", "Evidence", "- 证据A", "Explanations", "- 解释B", "Main logic", "驱动A -> 中间步骤 -> 目标B", "Confidence", "high"} {
		if !strings.Contains(out, want) {
			t.Fatalf("stdout missing %q in %q", want, out)
		}
	}
}

func TestRunCompileCardPrefersGraphFirstLogicChain(t *testing.T) {
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
		record := c.Record{
			UnitID:         "twitter:graph-first-logic",
			Source:         "twitter",
			ExternalID:     "graph-first-logic",
			RootExternalID: "graph-first-logic",
			Model:          c.Qwen36PlusModel,
			Output: c.Output{
				Summary:           "一句话总结",
				Drivers:           []string{"旧驱动"},
				Targets:           []string{"旧目标"},
				TransmissionPaths: []c.TransmissionPath{{Driver: "旧驱动", Target: "旧目标", Steps: []string{"旧中间步骤"}}},
				Graph: c.ReasoningGraph{
					Nodes: []c.GraphNode{testGraphNode("legacy-driver", c.NodeMechanism, "旧驱动"), testGraphNode("legacy-target", c.NodeConclusion, "旧目标")},
					Edges: []c.GraphEdge{{From: "legacy-driver", To: "legacy-target", Kind: c.EdgePositive}},
				},
				Details:    c.HiddenDetails{Caveats: []string{"说明"}},
				Confidence: "high",
			},
			CompiledAt: time.Now().UTC(),
		}
		if err := store.UpsertCompiledOutput(context.Background(), record); err != nil {
			return nil, err
		}
		subgraph := graphmodel.ContentSubgraph{
			ID:               "twitter:graph-first-logic",
			ArticleID:        "twitter:graph-first-logic",
			SourcePlatform:   "twitter",
			SourceExternalID: "graph-first-logic",
			RootExternalID:   "graph-first-logic",
			CompileVersion:   graphmodel.CompileBridgeVersion,
			CompiledAt:       time.Now().UTC().Format(time.RFC3339),
			UpdatedAt:        time.Now().UTC().Format(time.RFC3339),
			Nodes: []graphmodel.GraphNode{
				{ID: "n1", SourceArticleID: "twitter:graph-first-logic", SourcePlatform: "twitter", SourceExternalID: "graph-first-logic", RawText: "美联储加息0.25%", SubjectText: "美联储", ChangeText: "加息0.25%", Kind: graphmodel.NodeKindObservation, GraphRole: graphmodel.GraphRoleDriver, IsPrimary: true, VerificationStatus: graphmodel.VerificationPending},
				{ID: "n2", SourceArticleID: "twitter:graph-first-logic", SourcePlatform: "twitter", SourceExternalID: "graph-first-logic", RawText: "流动性收紧", SubjectText: "流动性", ChangeText: "收紧", Kind: graphmodel.NodeKindObservation, GraphRole: graphmodel.GraphRoleIntermediate, IsPrimary: true, VerificationStatus: graphmodel.VerificationPending},
				{ID: "n3", SourceArticleID: "twitter:graph-first-logic", SourcePlatform: "twitter", SourceExternalID: "graph-first-logic", RawText: "未来一周美股承压", SubjectText: "美股", ChangeText: "承压", Kind: graphmodel.NodeKindPrediction, GraphRole: graphmodel.GraphRoleTarget, IsPrimary: true, VerificationStatus: graphmodel.VerificationPending},
			},
			Edges: []graphmodel.GraphEdge{
				{ID: "n1->n2:drives", From: "n1", To: "n2", Type: graphmodel.EdgeTypeDrives, IsPrimary: true, VerificationStatus: graphmodel.VerificationPending},
				{ID: "n2->n3:drives", From: "n2", To: "n3", Type: graphmodel.EdgeTypeDrives, IsPrimary: true, VerificationStatus: graphmodel.VerificationPending},
			},
		}
		if err := store.UpsertContentSubgraph(context.Background(), subgraph); err != nil {
			return nil, err
		}
		return store, nil
	}

	var stdout, stderr bytes.Buffer
	code := run([]string{"compile", "card", "--platform", "twitter", "--id", "graph-first-logic"}, "/tmp/project", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("run() code = %d, stderr = %s", code, stderr.String())
	}
	out := stdout.String()
	if !strings.Contains(out, "美联储加息0.25% -> 流动性收紧 -> 未来一周美股承压") {
		t.Fatalf("stdout = %q, want graph-first logic chain", out)
	}
	if strings.Contains(out, "旧驱动 -> 旧中间步骤 -> 旧目标") {
		t.Fatalf("stdout = %q, did not want stale legacy logic chain", out)
	}
}

func TestRunCompileCardCompactPrefersGraphFirstProjection(t *testing.T) {
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
		record := c.Record{
			UnitID:         "twitter:graph-first-compact",
			Source:         "twitter",
			ExternalID:     "graph-first-compact",
			RootExternalID: "graph-first-compact",
			Model:          c.Qwen36PlusModel,
			Output: c.Output{
				Summary:           "一句话总结",
				Drivers:           []string{"旧驱动"},
				Targets:           []string{"旧目标"},
				TransmissionPaths: []c.TransmissionPath{{Driver: "旧驱动", Target: "旧目标", Steps: []string{"旧中间步骤"}}},
				EvidenceNodes:     []string{"旧证据"},
				ExplanationNodes:  []string{"旧解释"},
				Graph: c.ReasoningGraph{
					Nodes: []c.GraphNode{testGraphNode("legacy-driver", c.NodeMechanism, "旧驱动"), testGraphNode("legacy-target", c.NodeConclusion, "旧目标")},
					Edges: []c.GraphEdge{{From: "legacy-driver", To: "legacy-target", Kind: c.EdgePositive}},
				},
				Details:    c.HiddenDetails{Caveats: []string{"说明"}},
				Confidence: "high",
			},
			CompiledAt: time.Now().UTC(),
		}
		if err := store.UpsertCompiledOutput(context.Background(), record); err != nil {
			return nil, err
		}
		subgraph := graphmodel.ContentSubgraph{
			ID:               "twitter:graph-first-compact",
			ArticleID:        "twitter:graph-first-compact",
			SourcePlatform:   "twitter",
			SourceExternalID: "graph-first-compact",
			RootExternalID:   "graph-first-compact",
			CompileVersion:   graphmodel.CompileBridgeVersion,
			CompiledAt:       time.Now().UTC().Format(time.RFC3339),
			UpdatedAt:        time.Now().UTC().Format(time.RFC3339),
			Nodes: []graphmodel.GraphNode{
				{ID: "n1", SourceArticleID: "twitter:graph-first-compact", SourcePlatform: "twitter", SourceExternalID: "graph-first-compact", RawText: "美联储加息0.25%", SubjectText: "美联储", ChangeText: "加息0.25%", Kind: graphmodel.NodeKindObservation, GraphRole: graphmodel.GraphRoleDriver, IsPrimary: true, VerificationStatus: graphmodel.VerificationPending},
				{ID: "n2", SourceArticleID: "twitter:graph-first-compact", SourcePlatform: "twitter", SourceExternalID: "graph-first-compact", RawText: "流动性收紧", SubjectText: "流动性", ChangeText: "收紧", Kind: graphmodel.NodeKindObservation, GraphRole: graphmodel.GraphRoleIntermediate, IsPrimary: true, VerificationStatus: graphmodel.VerificationPending},
				{ID: "n3", SourceArticleID: "twitter:graph-first-compact", SourcePlatform: "twitter", SourceExternalID: "graph-first-compact", RawText: "未来一周美股承压", SubjectText: "美股", ChangeText: "承压", Kind: graphmodel.NodeKindPrediction, GraphRole: graphmodel.GraphRoleTarget, IsPrimary: true, VerificationStatus: graphmodel.VerificationPending},
				{ID: "n4", SourceArticleID: "twitter:graph-first-compact", SourcePlatform: "twitter", SourceExternalID: "graph-first-compact", RawText: "CPI回落", SubjectText: "CPI", ChangeText: "回落", Kind: graphmodel.NodeKindObservation, GraphRole: graphmodel.GraphRoleEvidence, IsPrimary: false, VerificationStatus: graphmodel.VerificationPending},
				{ID: "n5", SourceArticleID: "twitter:graph-first-compact", SourcePlatform: "twitter", SourceExternalID: "graph-first-compact", RawText: "估值承压先传导到科技股", SubjectText: "科技股估值", ChangeText: "承压", Kind: graphmodel.NodeKindObservation, GraphRole: graphmodel.GraphRoleContext, IsPrimary: false, VerificationStatus: graphmodel.VerificationPending},
			},
			Edges: []graphmodel.GraphEdge{
				{ID: "n1->n2:drives", From: "n1", To: "n2", Type: graphmodel.EdgeTypeDrives, IsPrimary: true, VerificationStatus: graphmodel.VerificationPending},
				{ID: "n2->n3:drives", From: "n2", To: "n3", Type: graphmodel.EdgeTypeDrives, IsPrimary: true, VerificationStatus: graphmodel.VerificationPending},
				{ID: "n4->n1:supports", From: "n4", To: "n1", Type: graphmodel.EdgeTypeSupports, IsPrimary: false, VerificationStatus: graphmodel.VerificationPending},
				{ID: "n5->n3:explains", From: "n5", To: "n3", Type: graphmodel.EdgeTypeExplains, IsPrimary: false, VerificationStatus: graphmodel.VerificationPending},
			},
		}
		if err := store.UpsertContentSubgraph(context.Background(), subgraph); err != nil {
			return nil, err
		}
		return store, nil
	}

	var stdout, stderr bytes.Buffer
	code := run([]string{"compile", "card", "--compact", "--platform", "twitter", "--id", "graph-first-compact"}, "/tmp/project", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("run() code = %d, stderr = %s", code, stderr.String())
	}
	out := stdout.String()
	for _, want := range []string{"- 美联储加息0.25%", "- 未来一周美股承压", "- CPI回落", "- 估值承压先传导到科技股", "美联储加息0.25% -> 流动性收紧 -> 未来一周美股承压"} {
		if !strings.Contains(out, want) {
			t.Fatalf("stdout missing %q in %q", want, out)
		}
	}
	for _, stale := range []string{"- 旧驱动", "- 旧目标", "- 旧证据", "- 旧解释", "旧驱动 -> 旧中间步骤 -> 旧目标"} {
		if strings.Contains(out, stale) {
			t.Fatalf("stdout = %q, did not want stale legacy projection %q", out, stale)
		}
	}
}

func TestRunCompileCardCompactFallsBackToLegacyWhenGraphFirstSubgraphMissing(t *testing.T) {
	prevBuildApp := buildApp
	prevOpenSQLiteStore := openSQLiteStore
	t.Cleanup(func() {
		buildApp = prevBuildApp
		openSQLiteStore = prevOpenSQLiteStore
	})

	tmp := t.TempDir()
	dbPath := filepath.Join(tmp, "content.db")
	buildApp = func(projectRoot string) (*bootstrap.App, error) {
		app := &bootstrap.App{}
		app.Settings.ContentDBPath = dbPath
		return app, nil
	}
	openSQLiteStore = func(path string) (*contentstore.SQLiteStore, error) {
		store, err := contentstore.NewSQLiteStore(path)
		if err != nil {
			return nil, err
		}
		record := c.Record{
			UnitID:         "twitter:legacy-fallback-card",
			Source:         "twitter",
			ExternalID:     "legacy-fallback-card",
			RootExternalID: "legacy-fallback-card",
			Model:          c.Qwen36PlusModel,
			Output: c.Output{
				Summary:           "一句话总结",
				Drivers:           []string{"驱动A"},
				Targets:           []string{"目标B"},
				TransmissionPaths: []c.TransmissionPath{{Driver: "驱动A", Target: "目标B", Steps: []string{"中间步骤"}}},
				EvidenceNodes:     []string{"证据A"},
				ExplanationNodes:  []string{"解释B"},
				Graph: c.ReasoningGraph{
					Nodes: []c.GraphNode{testGraphNode("legacy-driver", c.NodeMechanism, "驱动A"), testGraphNode("legacy-target", c.NodeConclusion, "目标B")},
					Edges: []c.GraphEdge{{From: "legacy-driver", To: "legacy-target", Kind: c.EdgePositive}},
				},
				Details:    c.HiddenDetails{Caveats: []string{"说明"}},
				Confidence: "high",
			},
			CompiledAt: time.Now().UTC(),
		}
		if err := store.UpsertCompiledOutput(context.Background(), record); err != nil {
			return nil, err
		}
		rawDB, err := sql.Open("sqlite", path)
		if err != nil {
			return nil, err
		}
		defer rawDB.Close()
		if _, err := rawDB.Exec(`DELETE FROM content_subgraphs WHERE platform = ? AND external_id = ?`, "twitter", "legacy-fallback-card"); err != nil {
			return nil, err
		}
		return store, nil
	}

	var stdout, stderr bytes.Buffer
	code := run([]string{"compile", "card", "--compact", "--platform", "twitter", "--id", "legacy-fallback-card"}, "/tmp/project", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("run() code = %d, stderr = %s", code, stderr.String())
	}
	out := stdout.String()
	for _, want := range []string{"- 驱动A", "- 目标B", "- 证据A", "- 解释B", "驱动A -> 中间步骤 -> 目标B"} {
		if !strings.Contains(out, want) {
			t.Fatalf("stdout missing %q in %q", want, out)
		}
	}
}

func TestRunCompileCardFailsWhenGraphFirstLookupReturnsUnexpectedStoreError(t *testing.T) {
	prevBuildApp := buildApp
	prevOpenSQLiteStore := openSQLiteStore
	t.Cleanup(func() {
		buildApp = prevBuildApp
		openSQLiteStore = prevOpenSQLiteStore
	})

	tmp := t.TempDir()
	dbPath := filepath.Join(tmp, "content.db")
	buildApp = func(projectRoot string) (*bootstrap.App, error) {
		app := &bootstrap.App{}
		app.Settings.ContentDBPath = dbPath
		return app, nil
	}
	openSQLiteStore = func(path string) (*contentstore.SQLiteStore, error) {
		store, err := contentstore.NewSQLiteStore(path)
		if err != nil {
			return nil, err
		}
		record := c.Record{
			UnitID:         "twitter:graph-first-store-error",
			Source:         "twitter",
			ExternalID:     "graph-first-store-error",
			RootExternalID: "graph-first-store-error",
			Model:          c.Qwen36PlusModel,
			Output: c.Output{
				Summary:           "一句话总结",
				Drivers:           []string{"驱动A"},
				Targets:           []string{"目标B"},
				TransmissionPaths: []c.TransmissionPath{{Driver: "驱动A", Target: "目标B", Steps: []string{"中间步骤"}}},
				EvidenceNodes:     []string{"证据A"},
				ExplanationNodes:  []string{"解释B"},
				Graph: c.ReasoningGraph{
					Nodes: []c.GraphNode{testGraphNode("legacy-driver", c.NodeMechanism, "驱动A"), testGraphNode("legacy-target", c.NodeConclusion, "目标B")},
					Edges: []c.GraphEdge{{From: "legacy-driver", To: "legacy-target", Kind: c.EdgePositive}},
				},
				Details:    c.HiddenDetails{Caveats: []string{"说明"}},
				Confidence: "high",
			},
			CompiledAt: time.Now().UTC(),
		}
		if err := store.UpsertCompiledOutput(context.Background(), record); err != nil {
			return nil, err
		}
		rawDB, err := sql.Open("sqlite", path)
		if err != nil {
			return nil, err
		}
		defer rawDB.Close()
		if _, err := rawDB.Exec(`DROP TABLE content_subgraphs`); err != nil {
			return nil, err
		}
		return store, nil
	}

	var stdout, stderr bytes.Buffer
	code := run([]string{"compile", "card", "--compact", "--platform", "twitter", "--id", "graph-first-store-error"}, "/tmp/project", &stdout, &stderr)
	if code != 1 {
		t.Fatalf("run() code = %d, want 1, stdout = %s, stderr = %s", code, stdout.String(), stderr.String())
	}
	if !strings.Contains(stderr.String(), "content_subgraphs") {
		t.Fatalf("stderr = %q, want surfaced content_subgraphs lookup error", stderr.String())
	}
}

func TestRunCompileCardFailsWhenExpandedGraphFirstLookupReturnsUnexpectedStoreError(t *testing.T) {
	prevBuildApp := buildApp
	prevOpenSQLiteStore := openSQLiteStore
	t.Cleanup(func() {
		buildApp = prevBuildApp
		openSQLiteStore = prevOpenSQLiteStore
	})

	tmp := t.TempDir()
	dbPath := filepath.Join(tmp, "content.db")
	buildApp = func(projectRoot string) (*bootstrap.App, error) {
		app := &bootstrap.App{}
		app.Settings.ContentDBPath = dbPath
		return app, nil
	}
	openSQLiteStore = func(path string) (*contentstore.SQLiteStore, error) {
		store, err := contentstore.NewSQLiteStore(path)
		if err != nil {
			return nil, err
		}
		record := c.Record{
			UnitID:         "twitter:graph-first-store-error-expanded",
			Source:         "twitter",
			ExternalID:     "graph-first-store-error-expanded",
			RootExternalID: "graph-first-store-error-expanded",
			Model:          c.Qwen36PlusModel,
			Output: c.Output{
				Summary:           "一句话总结",
				Drivers:           []string{"驱动A"},
				Targets:           []string{"目标B"},
				TransmissionPaths: []c.TransmissionPath{{Driver: "驱动A", Target: "目标B", Steps: []string{"中间步骤"}}},
				EvidenceNodes:     []string{"证据A"},
				ExplanationNodes:  []string{"解释B"},
				Graph: c.ReasoningGraph{
					Nodes: []c.GraphNode{testGraphNode("legacy-driver", c.NodeMechanism, "驱动A"), testGraphNode("legacy-target", c.NodeConclusion, "目标B")},
					Edges: []c.GraphEdge{{From: "legacy-driver", To: "legacy-target", Kind: c.EdgePositive}},
				},
				Details:    c.HiddenDetails{Caveats: []string{"说明"}},
				Confidence: "high",
			},
			CompiledAt: time.Now().UTC(),
		}
		if err := store.UpsertCompiledOutput(context.Background(), record); err != nil {
			return nil, err
		}
		rawDB, err := sql.Open("sqlite", path)
		if err != nil {
			return nil, err
		}
		defer rawDB.Close()
		if _, err := rawDB.Exec(`DROP TABLE content_subgraphs`); err != nil {
			return nil, err
		}
		return store, nil
	}

	var stdout, stderr bytes.Buffer
	code := run([]string{"compile", "card", "--platform", "twitter", "--id", "graph-first-store-error-expanded"}, "/tmp/project", &stdout, &stderr)
	if code != 1 {
		t.Fatalf("run() code = %d, want 1, stdout = %s, stderr = %s", code, stdout.String(), stderr.String())
	}
	if !strings.Contains(stderr.String(), "content_subgraphs") {
		t.Fatalf("stderr = %q, want surfaced content_subgraphs lookup error", stderr.String())
	}
}

func TestRunCompileCardFailsWhenURLGraphFirstLookupReturnsUnexpectedStoreError(t *testing.T) {
	prevBuildApp := buildApp
	prevOpenSQLiteStore := openSQLiteStore
	t.Cleanup(func() {
		buildApp = prevBuildApp
		openSQLiteStore = prevOpenSQLiteStore
	})

	tmp := t.TempDir()
	dbPath := filepath.Join(tmp, "content.db")
	buildApp = func(projectRoot string) (*bootstrap.App, error) {
		app := &bootstrap.App{}
		app.Settings.ContentDBPath = dbPath
		app.Dispatcher = dispatcher.New(
			func(raw string) (types.ParsedURL, error) {
				return types.ParsedURL{Platform: types.PlatformWeibo, PlatformID: "Q-store-url", CanonicalURL: raw}, nil
			},
			[]dispatcher.ItemSource{fakeItemSource{}},
			nil,
			nil,
		)
		return app, nil
	}
	openSQLiteStore = func(path string) (*contentstore.SQLiteStore, error) {
		store, err := contentstore.NewSQLiteStore(path)
		if err != nil {
			return nil, err
		}
		record := c.Record{
			UnitID:         "weibo:Q-store-url",
			Source:         "weibo",
			ExternalID:     "Q-store-url",
			RootExternalID: "Q-store-url",
			Model:          c.Qwen36PlusModel,
			Output: c.Output{
				Summary:           "一句话总结",
				Drivers:           []string{"驱动A"},
				Targets:           []string{"目标B"},
				TransmissionPaths: []c.TransmissionPath{{Driver: "驱动A", Target: "目标B", Steps: []string{"中间步骤"}}},
				EvidenceNodes:     []string{"证据A"},
				ExplanationNodes:  []string{"解释B"},
				Graph: c.ReasoningGraph{
					Nodes: []c.GraphNode{testGraphNode("legacy-driver", c.NodeMechanism, "驱动A"), testGraphNode("legacy-target", c.NodeConclusion, "目标B")},
					Edges: []c.GraphEdge{{From: "legacy-driver", To: "legacy-target", Kind: c.EdgePositive}},
				},
				Details:    c.HiddenDetails{Caveats: []string{"说明"}},
				Confidence: "high",
			},
			CompiledAt: time.Now().UTC(),
		}
		if err := store.UpsertCompiledOutput(context.Background(), record); err != nil {
			return nil, err
		}
		rawDB, err := sql.Open("sqlite", path)
		if err != nil {
			return nil, err
		}
		defer rawDB.Close()
		if _, err := rawDB.Exec(`DROP TABLE content_subgraphs`); err != nil {
			return nil, err
		}
		return store, nil
	}

	var stdout, stderr bytes.Buffer
	code := run([]string{"compile", "card", "--url", "https://weibo.com/1182426800/Q-store-url"}, "/tmp/project", &stdout, &stderr)
	if code != 1 {
		t.Fatalf("run() code = %d, want 1, stdout = %s, stderr = %s", code, stdout.String(), stderr.String())
	}
	if !strings.Contains(stderr.String(), "content_subgraphs") {
		t.Fatalf("stderr = %q, want surfaced content_subgraphs lookup error", stderr.String())
	}
}

func TestRunCompileCardCompactFailsWhenURLGraphFirstLookupReturnsUnexpectedStoreError(t *testing.T) {
	prevBuildApp := buildApp
	prevOpenSQLiteStore := openSQLiteStore
	t.Cleanup(func() {
		buildApp = prevBuildApp
		openSQLiteStore = prevOpenSQLiteStore
	})

	tmp := t.TempDir()
	dbPath := filepath.Join(tmp, "content.db")
	buildApp = func(projectRoot string) (*bootstrap.App, error) {
		app := &bootstrap.App{}
		app.Settings.ContentDBPath = dbPath
		app.Dispatcher = dispatcher.New(
			func(raw string) (types.ParsedURL, error) {
				return types.ParsedURL{Platform: types.PlatformWeibo, PlatformID: "Q-store-url-compact", CanonicalURL: raw}, nil
			},
			[]dispatcher.ItemSource{fakeItemSource{}},
			nil,
			nil,
		)
		return app, nil
	}
	openSQLiteStore = func(path string) (*contentstore.SQLiteStore, error) {
		store, err := contentstore.NewSQLiteStore(path)
		if err != nil {
			return nil, err
		}
		record := c.Record{
			UnitID:         "weibo:Q-store-url-compact",
			Source:         "weibo",
			ExternalID:     "Q-store-url-compact",
			RootExternalID: "Q-store-url-compact",
			Model:          c.Qwen36PlusModel,
			Output: c.Output{
				Summary:           "一句话总结",
				Drivers:           []string{"驱动A"},
				Targets:           []string{"目标B"},
				TransmissionPaths: []c.TransmissionPath{{Driver: "驱动A", Target: "目标B", Steps: []string{"中间步骤"}}},
				EvidenceNodes:     []string{"证据A"},
				ExplanationNodes:  []string{"解释B"},
				Graph: c.ReasoningGraph{
					Nodes: []c.GraphNode{testGraphNode("legacy-driver", c.NodeMechanism, "驱动A"), testGraphNode("legacy-target", c.NodeConclusion, "目标B")},
					Edges: []c.GraphEdge{{From: "legacy-driver", To: "legacy-target", Kind: c.EdgePositive}},
				},
				Details:    c.HiddenDetails{Caveats: []string{"说明"}},
				Confidence: "high",
			},
			CompiledAt: time.Now().UTC(),
		}
		if err := store.UpsertCompiledOutput(context.Background(), record); err != nil {
			return nil, err
		}
		rawDB, err := sql.Open("sqlite", path)
		if err != nil {
			return nil, err
		}
		defer rawDB.Close()
		if _, err := rawDB.Exec(`DROP TABLE content_subgraphs`); err != nil {
			return nil, err
		}
		return store, nil
	}

	var stdout, stderr bytes.Buffer
	code := run([]string{"compile", "card", "--compact", "--url", "https://weibo.com/1182426800/Q-store-url-compact"}, "/tmp/project", &stdout, &stderr)
	if code != 1 {
		t.Fatalf("run() code = %d, want 1, stdout = %s, stderr = %s", code, stdout.String(), stderr.String())
	}
	if !strings.Contains(stderr.String(), "content_subgraphs") {
		t.Fatalf("stderr = %q, want surfaced content_subgraphs lookup error", stderr.String())
	}
}

func TestRunCompileCardCompactFallsBackToLegacyWhenGraphFirstProjectionIsLessInformative(t *testing.T) {
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
		record := c.Record{
			UnitID:         "twitter:less-informative-card",
			Source:         "twitter",
			ExternalID:     "less-informative-card",
			RootExternalID: "less-informative-card",
			Model:          c.Qwen36PlusModel,
			Output: c.Output{
				Summary:           "一句话总结",
				Drivers:           []string{"旧驱动A", "旧驱动B"},
				Targets:           []string{"旧目标A", "旧目标B"},
				TransmissionPaths: []c.TransmissionPath{{Driver: "旧驱动A", Target: "旧目标A", Steps: []string{"旧中间步骤"}}},
				EvidenceNodes:     []string{"旧证据A", "旧证据B"},
				ExplanationNodes:  []string{"旧解释A", "旧解释B"},
				Graph: c.ReasoningGraph{
					Nodes: []c.GraphNode{testGraphNode("legacy-driver", c.NodeMechanism, "旧驱动A"), testGraphNode("legacy-target", c.NodeConclusion, "旧目标A")},
					Edges: []c.GraphEdge{{From: "legacy-driver", To: "legacy-target", Kind: c.EdgePositive}},
				},
				Details:    c.HiddenDetails{Caveats: []string{"说明"}},
				Confidence: "high",
			},
			CompiledAt: time.Now().UTC(),
		}
		if err := store.UpsertCompiledOutput(context.Background(), record); err != nil {
			return nil, err
		}
		subgraph := graphmodel.ContentSubgraph{
			ID:               "twitter:less-informative-card",
			ArticleID:        "twitter:less-informative-card",
			SourcePlatform:   "twitter",
			SourceExternalID: "less-informative-card",
			RootExternalID:   "less-informative-card",
			CompileVersion:   graphmodel.CompileBridgeVersion,
			CompiledAt:       time.Now().UTC().Format(time.RFC3339),
			UpdatedAt:        time.Now().UTC().Format(time.RFC3339),
			Nodes: []graphmodel.GraphNode{
				{ID: "n1", SourceArticleID: "twitter:less-informative-card", SourcePlatform: "twitter", SourceExternalID: "less-informative-card", RawText: "新驱动", SubjectText: "新驱动", ChangeText: "新驱动", Kind: graphmodel.NodeKindObservation, GraphRole: graphmodel.GraphRoleDriver, IsPrimary: true, VerificationStatus: graphmodel.VerificationPending},
				{ID: "n2", SourceArticleID: "twitter:less-informative-card", SourcePlatform: "twitter", SourceExternalID: "less-informative-card", RawText: "新目标", SubjectText: "新目标", ChangeText: "新目标", Kind: graphmodel.NodeKindPrediction, GraphRole: graphmodel.GraphRoleTarget, IsPrimary: true, VerificationStatus: graphmodel.VerificationPending},
				{ID: "n3", SourceArticleID: "twitter:less-informative-card", SourcePlatform: "twitter", SourceExternalID: "less-informative-card", RawText: "新证据", SubjectText: "新证据", ChangeText: "新证据", Kind: graphmodel.NodeKindObservation, GraphRole: graphmodel.GraphRoleEvidence, IsPrimary: false, VerificationStatus: graphmodel.VerificationPending},
				{ID: "n4", SourceArticleID: "twitter:less-informative-card", SourcePlatform: "twitter", SourceExternalID: "less-informative-card", RawText: "新解释", SubjectText: "新解释", ChangeText: "新解释", Kind: graphmodel.NodeKindObservation, GraphRole: graphmodel.GraphRoleContext, IsPrimary: false, VerificationStatus: graphmodel.VerificationPending},
			},
			Edges: []graphmodel.GraphEdge{
				{ID: "n1->n2:drives", From: "n1", To: "n2", Type: graphmodel.EdgeTypeDrives, IsPrimary: true, VerificationStatus: graphmodel.VerificationPending},
			},
		}
		if err := store.UpsertContentSubgraph(context.Background(), subgraph); err != nil {
			return nil, err
		}
		return store, nil
	}

	var stdout, stderr bytes.Buffer
	code := run([]string{"compile", "card", "--compact", "--platform", "twitter", "--id", "less-informative-card"}, "/tmp/project", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("run() code = %d, stderr = %s", code, stderr.String())
	}
	out := stdout.String()
	for _, want := range []string{"- 旧驱动A", "- 旧驱动B", "- 旧目标A", "- 旧目标B", "- 旧证据A", "- 旧证据B", "- 旧解释A", "- 旧解释B"} {
		if !strings.Contains(out, want) {
			t.Fatalf("stdout missing legacy section item %q in %q", want, out)
		}
	}
	for _, avoid := range []string{"- 新驱动", "- 新目标", "- 新证据", "- 新解释"} {
		if strings.Contains(out, avoid) {
			t.Fatalf("stdout = %q, did not want less-informative graph-first item %q", out, avoid)
		}
	}
}

func TestRunCompileCardCollapsesLinearChain(t *testing.T) {
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
		record := c.Record{
			UnitID:         "twitter:1",
			Source:         "twitter",
			ExternalID:     "1",
			RootExternalID: "1",
			Model:          c.Qwen36PlusModel,
			Output: c.Output{
				Summary: "一句话总结",
				Drivers: []string{"驱动A"},
				Targets: []string{"目标C"},
				TransmissionPaths: []c.TransmissionPath{{
					Driver: "驱动A",
					Target: "目标C",
					Steps:  []string{"步骤B"},
				}},
				Graph: c.ReasoningGraph{
					Nodes: []c.GraphNode{testGraphNode("n1", c.NodeMechanism, "驱动A"), testGraphNode("n2", c.NodeMechanism, "步骤B"), testGraphNode("n3", c.NodeConclusion, "目标C")},
					Edges: []c.GraphEdge{{From: "n1", To: "n2", Kind: c.EdgePositive}, {From: "n2", To: "n3", Kind: c.EdgePositive}},
				},
				Details:    c.HiddenDetails{Caveats: []string{"说明"}},
				Confidence: "high",
			},
			CompiledAt: time.Now().UTC(),
		}
		if err := store.UpsertCompiledOutput(context.Background(), record); err != nil {
			return nil, err
		}
		return store, nil
	}

	var stdout, stderr bytes.Buffer
	code := run([]string{"compile", "card", "--platform", "twitter", "--id", "1"}, "/tmp/project", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("run() code = %d, stderr = %s", code, stderr.String())
	}
	out := stdout.String()
	if !strings.Contains(out, "驱动A -> 步骤B -> 目标C") {
		t.Fatalf("stdout missing collapsed chain in %q", out)
	}
}

func TestRunMemoryEventGraphsPrintsProjectedEventGraphs(t *testing.T) {
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
		record := c.Record{
			UnitID: "twitter:EG1", Source: "twitter", ExternalID: "EG1", RootExternalID: "EG1", Model: c.Qwen36PlusModel,
			Output: c.Output{Summary: "一句话", Drivers: []string{"美联储加息0.25%"}, Targets: []string{"未来一周美股承压"}, Graph: c.ReasoningGraph{Nodes: []c.GraphNode{
				{ID: "n1", Kind: c.NodeFact, Text: "美联储加息0.25%", OccurredAt: time.Date(2026, 4, 10, 0, 0, 0, 0, time.UTC)},
				{ID: "n2", Kind: c.NodePrediction, Text: "未来一周美股承压", PredictionStartAt: time.Date(2026, 4, 10, 0, 0, 0, 0, time.UTC), PredictionDueAt: time.Date(2026, 4, 17, 0, 0, 0, 0, time.UTC)},
			}, Edges: []c.GraphEdge{{From: "n1", To: "n2", Kind: c.EdgePositive}}}, Details: c.HiddenDetails{Caveats: []string{"说明"}}},
			CompiledAt: time.Now().UTC(),
		}
		if err := store.UpsertCompiledOutput(context.Background(), record); err != nil {
			return nil, err
		}
		if _, err := store.AcceptMemoryNodes(context.Background(), memory.AcceptRequest{UserID: "u-event-cli", SourcePlatform: "twitter", SourceExternalID: "EG1", NodeIDs: []string{"n1", "n2"}}); err != nil {
			return nil, err
		}
		return store, nil
	}

	var stdout, stderr bytes.Buffer
	code := run([]string{"memory", "event-graphs", "--user", "u-event-cli"}, "/tmp/project", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("event-graphs code = %d, stderr = %s", code, stderr.String())
	}
	var out []contentstore.EventGraphRecord
	if err := json.Unmarshal(stdout.Bytes(), &out); err != nil {
		t.Fatalf("json.Unmarshal(event-graphs) error = %v", err)
	}
	if len(out) == 0 {
		t.Fatalf("event-graphs output = %#v, want non-empty", out)
	}
}

func TestRunMemoryParadigmsPrintsProjectedParadigms(t *testing.T) {
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
		record := c.Record{
			UnitID: "twitter:PG1", Source: "twitter", ExternalID: "PG1", RootExternalID: "PG1", Model: c.Qwen36PlusModel,
			Output: c.Output{Summary: "一句话", Drivers: []string{"美联储加息0.25%"}, Targets: []string{"未来一周美股承压"}, Graph: c.ReasoningGraph{Nodes: []c.GraphNode{
				{ID: "n1", Kind: c.NodeFact, Text: "美联储加息0.25%", OccurredAt: time.Date(2026, 4, 10, 0, 0, 0, 0, time.UTC)},
				{ID: "n2", Kind: c.NodePrediction, Text: "未来一周美股承压", PredictionStartAt: time.Date(2026, 4, 10, 0, 0, 0, 0, time.UTC), PredictionDueAt: time.Date(2026, 4, 17, 0, 0, 0, 0, time.UTC)},
			}, Edges: []c.GraphEdge{{From: "n1", To: "n2", Kind: c.EdgePositive}}}, Details: c.HiddenDetails{Caveats: []string{"说明"}}, Verification: c.Verification{NodeVerifications: []c.NodeVerification{{NodeID: "n2", Status: c.NodeVerificationProved}}}},
			CompiledAt: time.Now().UTC(),
		}
		if err := store.UpsertCompiledOutput(context.Background(), record); err != nil {
			return nil, err
		}
		if _, err := store.AcceptMemoryNodes(context.Background(), memory.AcceptRequest{UserID: "u-paradigm-cli", SourcePlatform: "twitter", SourceExternalID: "PG1", NodeIDs: []string{"n1", "n2"}}); err != nil {
			return nil, err
		}
		return store, nil
	}

	var stdout, stderr bytes.Buffer
	code := run([]string{"memory", "paradigms", "--user", "u-paradigm-cli"}, "/tmp/project", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("paradigms code = %d, stderr = %s", code, stderr.String())
	}
	var out []contentstore.ParadigmRecord
	if err := json.Unmarshal(stdout.Bytes(), &out); err != nil {
		t.Fatalf("json.Unmarshal(paradigms) error = %v", err)
	}
	if len(out) == 0 {
		t.Fatalf("paradigms output = %#v, want non-empty", out)
	}
}

func TestRunMemoryContentGraphsPrintsStoredContentMemoryGraphs(t *testing.T) {
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
		record := c.Record{
			UnitID: "twitter:CG1", Source: "twitter", ExternalID: "CG1", RootExternalID: "CG1", Model: c.Qwen36PlusModel,
			Output: c.Output{Summary: "一句话", Drivers: []string{"美联储加息0.25%"}, Targets: []string{"未来一周美股承压"}, Graph: c.ReasoningGraph{Nodes: []c.GraphNode{
				{ID: "n1", Kind: c.NodeFact, Text: "美联储加息0.25%", OccurredAt: time.Date(2026, 4, 10, 0, 0, 0, 0, time.UTC)},
				{ID: "n2", Kind: c.NodePrediction, Text: "未来一周美股承压", PredictionStartAt: time.Date(2026, 4, 10, 0, 0, 0, 0, time.UTC), PredictionDueAt: time.Date(2026, 4, 17, 0, 0, 0, 0, time.UTC)},
			}, Edges: []c.GraphEdge{{From: "n1", To: "n2", Kind: c.EdgePositive}}}, Details: c.HiddenDetails{Caveats: []string{"说明"}}},
			CompiledAt: time.Now().UTC(),
		}
		if err := store.UpsertCompiledOutput(context.Background(), record); err != nil {
			return nil, err
		}
		if _, err := store.AcceptMemoryNodes(context.Background(), memory.AcceptRequest{UserID: "u-content-graph-cli", SourcePlatform: "twitter", SourceExternalID: "CG1", NodeIDs: []string{"n1", "n2"}}); err != nil {
			return nil, err
		}
		return store, nil
	}

	var stdout, stderr bytes.Buffer
	code := run([]string{"memory", "content-graphs", "--user", "u-content-graph-cli"}, "/tmp/project", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("content-graphs code = %d, stderr = %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "unit-sweep") && !strings.Contains(stdout.String(), "twitter") {
		var out []map[string]any
		if err := json.Unmarshal(stdout.Bytes(), &out); err != nil {
			t.Fatalf("json.Unmarshal(content-graphs) error = %v", err)
		}
		if len(out) == 0 {
			t.Fatalf("content-graphs output = %#v, want non-empty", out)
		}
	}
}

func TestRunVerifyQueueListPrintsQueuedItems(t *testing.T) {
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
		now := time.Now().UTC()
		if err := store.EnqueueVerifyQueueItem(context.Background(), graphmodel.VerifyQueueItem{ID: "q-cli-1", ObjectType: graphmodel.VerifyQueueObjectNode, ObjectID: "n1", SourceArticleID: "unit-cli", Priority: 10, ScheduledAt: now.Format(time.RFC3339), Status: graphmodel.VerifyQueueStatusQueued}); err != nil {
			return nil, err
		}
		if err := store.EnqueueVerifyQueueItem(context.Background(), graphmodel.VerifyQueueItem{ID: "q-cli-2", ObjectType: graphmodel.VerifyQueueObjectEdge, ObjectID: "e1", SourceArticleID: "unit-cli", Priority: 5, ScheduledAt: now.Format(time.RFC3339), Status: graphmodel.VerifyQueueStatusQueued}); err != nil {
			return nil, err
		}
		if err := store.MarkVerifyQueueItemRunning(context.Background(), "q-cli-2", now); err != nil {
			return nil, err
		}
		return store, nil
	}

	var stdout, stderr bytes.Buffer
	code := run([]string{"verify", "queue", "--limit", "10"}, "/tmp/project", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("verify queue code = %d, stderr = %s", code, stderr.String())
	}
	var out []graphmodel.VerifyQueueItem
	if err := json.Unmarshal(stdout.Bytes(), &out); err != nil {
		t.Fatalf("json.Unmarshal(verify queue) error = %v", err)
	}
	if len(out) != 2 || out[0].ID != "q-cli-1" || out[1].ID != "q-cli-2" {
		t.Fatalf("verify queue output = %#v, want queued then running items", out)
	}
}

func TestRunMemoryEventGraphsRunRecomputesProjection(t *testing.T) {
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
		sg := graphmodel.ContentSubgraph{ID: "manual-eg", ArticleID: "manual-eg", SourcePlatform: "twitter", SourceExternalID: "manual-eg", CompileVersion: graphmodel.CompileBridgeVersion, CompiledAt: time.Now().UTC().Format(time.RFC3339), UpdatedAt: time.Now().UTC().Format(time.RFC3339), Nodes: []graphmodel.GraphNode{{ID: "n1", SourceArticleID: "manual-eg", SourcePlatform: "twitter", SourceExternalID: "manual-eg", RawText: "美联储加息0.25%", SubjectText: "美联储", ChangeText: "加息0.25%", Kind: graphmodel.NodeKindObservation, GraphRole: graphmodel.GraphRoleDriver, IsPrimary: true, VerificationStatus: graphmodel.VerificationPending, TimeBucket: "1w"}, {ID: "n2", SourceArticleID: "manual-eg", SourcePlatform: "twitter", SourceExternalID: "manual-eg", RawText: "未来一周美股承压", SubjectText: "美股", ChangeText: "承压", Kind: graphmodel.NodeKindPrediction, GraphRole: graphmodel.GraphRoleTarget, IsPrimary: true, VerificationStatus: graphmodel.VerificationProved, TimeBucket: "1w"}}}
		if err := store.PersistMemoryContentGraph(context.Background(), "u-event-run", sg, time.Now().UTC()); err != nil {
			return nil, err
		}
		return store, nil
	}

	var stdout, stderr bytes.Buffer
	code := run([]string{"memory", "event-graphs", "--run", "--user", "u-event-run"}, "/tmp/project", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("event-graphs --run code = %d, stderr = %s", code, stderr.String())
	}
	var out []contentstore.EventGraphRecord
	if err := json.Unmarshal(stdout.Bytes(), &out); err != nil {
		t.Fatalf("json.Unmarshal(event-graphs --run) error = %v", err)
	}
	if len(out) == 0 {
		t.Fatalf("event-graphs --run output = %#v, want non-empty", out)
	}
}

func TestRunMemoryParadigmsRunRecomputesProjection(t *testing.T) {
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
		sg := graphmodel.ContentSubgraph{ID: "manual-pg", ArticleID: "manual-pg", SourcePlatform: "twitter", SourceExternalID: "manual-pg", CompileVersion: graphmodel.CompileBridgeVersion, CompiledAt: time.Now().UTC().Format(time.RFC3339), UpdatedAt: time.Now().UTC().Format(time.RFC3339), Nodes: []graphmodel.GraphNode{{ID: "n1", SourceArticleID: "manual-pg", SourcePlatform: "twitter", SourceExternalID: "manual-pg", RawText: "美联储加息0.25%", SubjectText: "美联储", ChangeText: "加息0.25%", Kind: graphmodel.NodeKindObservation, GraphRole: graphmodel.GraphRoleDriver, IsPrimary: true, VerificationStatus: graphmodel.VerificationPending, TimeBucket: "1w"}, {ID: "n2", SourceArticleID: "manual-pg", SourcePlatform: "twitter", SourceExternalID: "manual-pg", RawText: "未来一周美股承压", SubjectText: "美股", ChangeText: "承压", Kind: graphmodel.NodeKindPrediction, GraphRole: graphmodel.GraphRoleTarget, IsPrimary: true, VerificationStatus: graphmodel.VerificationProved, TimeBucket: "1w"}}}
		if err := store.PersistMemoryContentGraph(context.Background(), "u-paradigm-run", sg, time.Now().UTC()); err != nil {
			return nil, err
		}
		return store, nil
	}

	var stdout, stderr bytes.Buffer
	code := run([]string{"memory", "paradigms", "--run", "--user", "u-paradigm-run"}, "/tmp/project", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("paradigms --run code = %d, stderr = %s", code, stderr.String())
	}
	var out []contentstore.ParadigmRecord
	if err := json.Unmarshal(stdout.Bytes(), &out); err != nil {
		t.Fatalf("json.Unmarshal(paradigms --run) error = %v", err)
	}
	if len(out) == 0 {
		t.Fatalf("paradigms --run output = %#v, want non-empty", out)
	}
}

func TestRunVerifySweepProcessesQueueFromCurrentContentGraphState(t *testing.T) {
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
		now := time.Now().UTC()
		subgraph := graphmodel.ContentSubgraph{ID: "sweep-cli", ArticleID: "sweep-cli", SourcePlatform: "twitter", SourceExternalID: "sweep-cli", CompileVersion: graphmodel.CompileBridgeVersion, CompiledAt: now.Format(time.RFC3339), UpdatedAt: now.Format(time.RFC3339), Nodes: []graphmodel.GraphNode{{ID: "n1", SourceArticleID: "sweep-cli", SourcePlatform: "twitter", SourceExternalID: "sweep-cli", RawText: "未来一周美股承压", SubjectText: "美股", ChangeText: "承压", Kind: graphmodel.NodeKindPrediction, GraphRole: graphmodel.GraphRoleTarget, IsPrimary: true, VerificationStatus: graphmodel.VerificationProved, VerificationReason: "resolved", VerificationAsOf: now.Format(time.RFC3339), TimeBucket: "1w"}}}
		if err := store.UpsertContentSubgraph(context.Background(), subgraph); err != nil {
			return nil, err
		}
		if err := store.EnqueueVerifyQueueItem(context.Background(), graphmodel.VerifyQueueItem{ID: "q-sweep-cli", ObjectType: graphmodel.VerifyQueueObjectNode, ObjectID: "n1", SourceArticleID: "sweep-cli", Priority: 10, ScheduledAt: now.Add(-time.Hour).Format(time.RFC3339), Status: graphmodel.VerifyQueueStatusQueued}); err != nil {
			return nil, err
		}
		return store, nil
	}

	var stdout, stderr bytes.Buffer
	code := run([]string{"verify", "sweep", "--limit", "10"}, "/tmp/project", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("verify sweep code = %d, stderr = %s", code, stderr.String())
	}
	var out contentstore.VerifyQueueSweepResult
	if err := json.Unmarshal(stdout.Bytes(), &out); err != nil {
		t.Fatalf("json.Unmarshal(verify sweep) error = %v", err)
	}
	if out.Claimed != 1 || out.Finished != 1 {
		t.Fatalf("verify sweep output = %#v, want claimed=1 finished=1", out)
	}
}

func TestRunHarnessGraphFirstFlowCommandsWorkTogether(t *testing.T) {
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
		now := time.Now().UTC()
		record := c.Record{UnitID: "twitter:FLOW1", Source: "twitter", ExternalID: "FLOW1", RootExternalID: "FLOW1", Model: c.Qwen36PlusModel, Output: c.Output{Summary: "一句话", Drivers: []string{"美联储加息0.25%"}, Targets: []string{"未来一周美股承压"}, Graph: c.ReasoningGraph{Nodes: []c.GraphNode{{ID: "n1", Kind: c.NodeFact, Text: "美联储加息0.25%", OccurredAt: now}, {ID: "n2", Kind: c.NodePrediction, Text: "未来一周美股承压", PredictionStartAt: now, PredictionDueAt: now.Add(24 * time.Hour)}}, Edges: []c.GraphEdge{{From: "n1", To: "n2", Kind: c.EdgePositive}}}, Details: c.HiddenDetails{Caveats: []string{"说明"}}}, CompiledAt: now}
		if err := store.UpsertCompiledOutput(context.Background(), record); err != nil {
			return nil, err
		}
		if _, err := store.AcceptMemoryNodes(context.Background(), memory.AcceptRequest{UserID: "u-flow", SourcePlatform: "twitter", SourceExternalID: "FLOW1", NodeIDs: []string{"n1", "n2"}}); err != nil {
			return nil, err
		}
		return store, nil
	}

	for _, argv := range [][]string{
		{"memory", "content-graphs", "--user", "u-flow"},
		{"memory", "event-graphs", "--run", "--user", "u-flow"},
		{"memory", "paradigms", "--run", "--user", "u-flow"},
		{"verify", "queue", "--limit", "10"},
		{"verify", "sweep", "--limit", "10"},
	} {
		var stdout, stderr bytes.Buffer
		code := run(argv, "/tmp/project", &stdout, &stderr)
		if code != 0 {
			t.Fatalf("run(%v) code = %d, stderr = %s", argv, code, stderr.String())
		}
		if strings.TrimSpace(stdout.String()) == "" {
			t.Fatalf("run(%v) stdout empty", argv)
		}
	}
}

func TestRunMemoryEventGraphsSupportsScopeFilter(t *testing.T) {
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
		sg := graphmodel.ContentSubgraph{ID: "filter-eg", ArticleID: "filter-eg", SourcePlatform: "twitter", SourceExternalID: "filter-eg", CompileVersion: graphmodel.CompileBridgeVersion, CompiledAt: time.Now().UTC().Format(time.RFC3339), UpdatedAt: time.Now().UTC().Format(time.RFC3339), Nodes: []graphmodel.GraphNode{{ID: "n1", SourceArticleID: "filter-eg", SourcePlatform: "twitter", SourceExternalID: "filter-eg", RawText: "美联储加息0.25%", SubjectText: "美联储", ChangeText: "加息0.25%", Kind: graphmodel.NodeKindObservation, GraphRole: graphmodel.GraphRoleDriver, IsPrimary: true, VerificationStatus: graphmodel.VerificationPending, TimeBucket: "1w"}, {ID: "n2", SourceArticleID: "filter-eg", SourcePlatform: "twitter", SourceExternalID: "filter-eg", RawText: "未来一周美股承压", SubjectText: "美股", ChangeText: "承压", Kind: graphmodel.NodeKindPrediction, GraphRole: graphmodel.GraphRoleTarget, IsPrimary: true, VerificationStatus: graphmodel.VerificationProved, TimeBucket: "1w"}}}
		if err := store.PersistMemoryContentGraph(context.Background(), "u-event-filter", sg, time.Now().UTC()); err != nil {
			return nil, err
		}
		return store, nil
	}
	var stdout, stderr bytes.Buffer
	code := run([]string{"memory", "event-graphs", "--user", "u-event-filter", "--scope", "target"}, "/tmp/project", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("event-graphs --scope target code = %d, stderr = %s", code, stderr.String())
	}
	var out []contentstore.EventGraphRecord
	if err := json.Unmarshal(stdout.Bytes(), &out); err != nil {
		t.Fatalf("json.Unmarshal(event-graphs --scope) error = %v", err)
	}
	if len(out) != 1 || out[0].Scope != "target" {
		t.Fatalf("event-graphs filtered output = %#v, want one target graph", out)
	}
}

func TestRunMemoryParadigmsSupportsSubjectFilter(t *testing.T) {
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
		sg := graphmodel.ContentSubgraph{ID: "filter-pg", ArticleID: "filter-pg", SourcePlatform: "twitter", SourceExternalID: "filter-pg", CompileVersion: graphmodel.CompileBridgeVersion, CompiledAt: time.Now().UTC().Format(time.RFC3339), UpdatedAt: time.Now().UTC().Format(time.RFC3339), Nodes: []graphmodel.GraphNode{{ID: "n1", SourceArticleID: "filter-pg", SourcePlatform: "twitter", SourceExternalID: "filter-pg", RawText: "美联储加息0.25%", SubjectText: "美联储", ChangeText: "加息0.25%", Kind: graphmodel.NodeKindObservation, GraphRole: graphmodel.GraphRoleDriver, IsPrimary: true, VerificationStatus: graphmodel.VerificationPending, TimeBucket: "1w"}, {ID: "n2", SourceArticleID: "filter-pg", SourcePlatform: "twitter", SourceExternalID: "filter-pg", RawText: "未来一周美股承压", SubjectText: "美股", ChangeText: "承压", Kind: graphmodel.NodeKindPrediction, GraphRole: graphmodel.GraphRoleTarget, IsPrimary: true, VerificationStatus: graphmodel.VerificationProved, TimeBucket: "1w"}}}
		if err := store.PersistMemoryContentGraph(context.Background(), "u-paradigm-filter", sg, time.Now().UTC()); err != nil {
			return nil, err
		}
		return store, nil
	}
	var stdout, stderr bytes.Buffer
	code := run([]string{"memory", "paradigms", "--user", "u-paradigm-filter", "--subject", "美联储"}, "/tmp/project", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("paradigms --subject 美联储 code = %d, stderr = %s", code, stderr.String())
	}
	var out []contentstore.ParadigmRecord
	if err := json.Unmarshal(stdout.Bytes(), &out); err != nil {
		t.Fatalf("json.Unmarshal(paradigms --subject) error = %v", err)
	}
	if len(out) != 1 || out[0].DriverSubject != "美联储" {
		t.Fatalf("paradigms filtered output = %#v, want 美联储 paradigm", out)
	}
}

func TestRunMemoryContentGraphsRunRebuildsSnapshotFromCompiledOutput(t *testing.T) {
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
		record := c.Record{UnitID: "twitter:CGRUN1", Source: "twitter", ExternalID: "CGRUN1", RootExternalID: "CGRUN1", Model: c.Qwen36PlusModel, Output: c.Output{Summary: "一句话", Drivers: []string{"美联储加息0.25%"}, Targets: []string{"未来一周美股承压"}, Graph: c.ReasoningGraph{Nodes: []c.GraphNode{{ID: "n1", Kind: c.NodeFact, Text: "美联储加息0.25%", OccurredAt: time.Date(2026, 4, 10, 0, 0, 0, 0, time.UTC)}, {ID: "n2", Kind: c.NodePrediction, Text: "未来一周美股承压", PredictionStartAt: time.Date(2026, 4, 10, 0, 0, 0, 0, time.UTC), PredictionDueAt: time.Date(2026, 4, 17, 0, 0, 0, 0, time.UTC)}}, Edges: []c.GraphEdge{{From: "n1", To: "n2", Kind: c.EdgePositive}}}, Details: c.HiddenDetails{Caveats: []string{"说明"}}}, CompiledAt: time.Now().UTC()}
		if err := store.UpsertCompiledOutput(context.Background(), record); err != nil {
			return nil, err
		}
		return store, nil
	}

	var stdout, stderr bytes.Buffer
	code := run([]string{"memory", "content-graphs", "--run", "--user", "u-content-run-cli", "--platform", "twitter", "--id", "CGRUN1"}, "/tmp/project", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("content-graphs --run code = %d, stderr = %s", code, stderr.String())
	}
	var out []map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &out); err != nil {
		t.Fatalf("json.Unmarshal(content-graphs --run) error = %v", err)
	}
	if len(out) == 0 {
		t.Fatalf("content-graphs --run output = %#v, want non-empty", out)
	}
}

func TestRunMemoryEventGraphsCardPrintsReadableSections(t *testing.T) {
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
		sg := graphmodel.ContentSubgraph{ID: "card-eg", ArticleID: "card-eg", SourcePlatform: "twitter", SourceExternalID: "card-eg", CompileVersion: graphmodel.CompileBridgeVersion, CompiledAt: time.Now().UTC().Format(time.RFC3339), UpdatedAt: time.Now().UTC().Format(time.RFC3339), Nodes: []graphmodel.GraphNode{{ID: "n1", SourceArticleID: "card-eg", SourcePlatform: "twitter", SourceExternalID: "card-eg", RawText: "美联储加息0.25%", SubjectText: "美联储", ChangeText: "加息0.25%", Kind: graphmodel.NodeKindObservation, GraphRole: graphmodel.GraphRoleDriver, IsPrimary: true, VerificationStatus: graphmodel.VerificationPending, TimeBucket: "1w"}, {ID: "n2", SourceArticleID: "card-eg", SourcePlatform: "twitter", SourceExternalID: "card-eg", RawText: "未来一周美股承压", SubjectText: "美股", ChangeText: "承压", Kind: graphmodel.NodeKindPrediction, GraphRole: graphmodel.GraphRoleTarget, IsPrimary: true, VerificationStatus: graphmodel.VerificationProved, TimeBucket: "1w"}}}
		if err := store.PersistMemoryContentGraph(context.Background(), "u-event-card", sg, time.Now().UTC()); err != nil {
			return nil, err
		}
		return store, nil
	}
	var stdout, stderr bytes.Buffer
	code := run([]string{"memory", "event-graphs", "--card", "--user", "u-event-card"}, "/tmp/project", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("event-graphs --card code = %d, stderr = %s", code, stderr.String())
	}
	for _, want := range []string{"Event Graph", "美联储", "美股", "Representative changes"} {
		if !strings.Contains(stdout.String(), want) {
			t.Fatalf("stdout = %q, want substring %q", stdout.String(), want)
		}
	}
}

func TestRunMemoryParadigmsCardPrintsReadableSections(t *testing.T) {
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
		sg := graphmodel.ContentSubgraph{ID: "card-pg", ArticleID: "card-pg", SourcePlatform: "twitter", SourceExternalID: "card-pg", CompileVersion: graphmodel.CompileBridgeVersion, CompiledAt: time.Now().UTC().Format(time.RFC3339), UpdatedAt: time.Now().UTC().Format(time.RFC3339), Nodes: []graphmodel.GraphNode{{ID: "n1", SourceArticleID: "card-pg", SourcePlatform: "twitter", SourceExternalID: "card-pg", RawText: "美联储加息0.25%", SubjectText: "美联储", ChangeText: "加息0.25%", Kind: graphmodel.NodeKindObservation, GraphRole: graphmodel.GraphRoleDriver, IsPrimary: true, VerificationStatus: graphmodel.VerificationPending, TimeBucket: "1w"}, {ID: "n2", SourceArticleID: "card-pg", SourcePlatform: "twitter", SourceExternalID: "card-pg", RawText: "未来一周美股承压", SubjectText: "美股", ChangeText: "承压", Kind: graphmodel.NodeKindPrediction, GraphRole: graphmodel.GraphRoleTarget, IsPrimary: true, VerificationStatus: graphmodel.VerificationProved, TimeBucket: "1w"}}}
		if err := store.PersistMemoryContentGraph(context.Background(), "u-paradigm-card", sg, time.Now().UTC()); err != nil {
			return nil, err
		}
		return store, nil
	}
	var stdout, stderr bytes.Buffer
	code := run([]string{"memory", "paradigms", "--card", "--user", "u-paradigm-card"}, "/tmp/project", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("paradigms --card code = %d, stderr = %s", code, stderr.String())
	}
	for _, want := range []string{"Paradigm", "美联储", "美股", "Credibility"} {
		if !strings.Contains(stdout.String(), want) {
			t.Fatalf("stdout = %q, want substring %q", stdout.String(), want)
		}
	}
}

func TestRunVerifyQueueSupportsStatusFilter(t *testing.T) {
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
		now := time.Now().UTC()
		if err := store.EnqueueVerifyQueueItem(context.Background(), graphmodel.VerifyQueueItem{ID: "q-filter-1", ObjectType: graphmodel.VerifyQueueObjectNode, ObjectID: "n1", SourceArticleID: "unit-cli", Priority: 10, ScheduledAt: now.Format(time.RFC3339), Status: graphmodel.VerifyQueueStatusQueued}); err != nil {
			return nil, err
		}
		if err := store.EnqueueVerifyQueueItem(context.Background(), graphmodel.VerifyQueueItem{ID: "q-filter-2", ObjectType: graphmodel.VerifyQueueObjectEdge, ObjectID: "e1", SourceArticleID: "unit-cli", Priority: 5, ScheduledAt: now.Format(time.RFC3339), Status: graphmodel.VerifyQueueStatusQueued}); err != nil {
			return nil, err
		}
		if err := store.MarkVerifyQueueItemRunning(context.Background(), "q-filter-2", now); err != nil {
			return nil, err
		}
		return store, nil
	}

	var stdout, stderr bytes.Buffer
	code := run([]string{"verify", "queue", "--limit", "10", "--status", "running"}, "/tmp/project", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("verify queue --status running code = %d, stderr = %s", code, stderr.String())
	}
	var out []graphmodel.VerifyQueueItem
	if err := json.Unmarshal(stdout.Bytes(), &out); err != nil {
		t.Fatalf("json.Unmarshal(verify queue --status) error = %v", err)
	}
	if len(out) != 1 || out[0].ID != "q-filter-2" {
		t.Fatalf("verify queue filtered output = %#v, want q-filter-2 only", out)
	}
}

func TestRunMemoryContentGraphsSupportsSourceFilter(t *testing.T) {
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
		for _, rec := range []c.Record{
			{UnitID: "twitter:CF1", Source: "twitter", ExternalID: "CF1", RootExternalID: "CF1", Model: c.Qwen36PlusModel, Output: c.Output{Summary: "一句话", Graph: c.ReasoningGraph{Nodes: []c.GraphNode{{ID: "n1", Kind: c.NodeFact, Text: "A", OccurredAt: time.Now().UTC()}, {ID: "n2", Kind: c.NodeConclusion, Text: "B", ValidFrom: time.Now().UTC(), ValidTo: time.Now().UTC().Add(24 * time.Hour)}}, Edges: []c.GraphEdge{{From: "n1", To: "n2", Kind: c.EdgePositive}}}, Details: c.HiddenDetails{Caveats: []string{"说明"}}}, CompiledAt: time.Now().UTC()},
			{UnitID: "twitter:CF2", Source: "twitter", ExternalID: "CF2", RootExternalID: "CF2", Model: c.Qwen36PlusModel, Output: c.Output{Summary: "一句话", Graph: c.ReasoningGraph{Nodes: []c.GraphNode{{ID: "n1", Kind: c.NodeFact, Text: "X", OccurredAt: time.Now().UTC()}, {ID: "n2", Kind: c.NodeConclusion, Text: "Y", ValidFrom: time.Now().UTC(), ValidTo: time.Now().UTC().Add(24 * time.Hour)}}, Edges: []c.GraphEdge{{From: "n1", To: "n2", Kind: c.EdgePositive}}}, Details: c.HiddenDetails{Caveats: []string{"说明"}}}, CompiledAt: time.Now().UTC()},
		} {
			if err := store.UpsertCompiledOutput(context.Background(), rec); err != nil {
				return nil, err
			}
			if err := store.PersistMemoryContentGraphFromCompiledOutput(context.Background(), "u-content-filter", rec.Source, rec.ExternalID, time.Now().UTC()); err != nil {
				return nil, err
			}
		}
		return store, nil
	}
	var stdout, stderr bytes.Buffer
	code := run([]string{"memory", "content-graphs", "--user", "u-content-filter", "--platform", "twitter", "--id", "CF2"}, "/tmp/project", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("content-graphs filtered code = %d, stderr = %s", code, stderr.String())
	}
	var out []graphmodel.ContentSubgraph
	if err := json.Unmarshal(stdout.Bytes(), &out); err != nil {
		t.Fatalf("json.Unmarshal(content-graphs filter) error = %v", err)
	}
	if len(out) != 1 || out[0].SourceExternalID != "CF2" {
		t.Fatalf("content-graphs filtered output = %#v, want CF2 only", out)
	}
}

func TestRunMemoryContentGraphsCardPrintsReadableSections(t *testing.T) {
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
		record := c.Record{UnitID: "twitter:CGCARD1", Source: "twitter", ExternalID: "CGCARD1", RootExternalID: "CGCARD1", Model: c.Qwen36PlusModel, Output: c.Output{Summary: "一句话", Drivers: []string{"美联储加息0.25%"}, Targets: []string{"未来一周美股承压"}, Graph: c.ReasoningGraph{Nodes: []c.GraphNode{{ID: "n1", Kind: c.NodeFact, Text: "美联储加息0.25%", OccurredAt: time.Date(2026, 4, 10, 0, 0, 0, 0, time.UTC)}, {ID: "n2", Kind: c.NodePrediction, Text: "未来一周美股承压", PredictionStartAt: time.Date(2026, 4, 10, 0, 0, 0, 0, time.UTC), PredictionDueAt: time.Date(2026, 4, 17, 0, 0, 0, 0, time.UTC)}}, Edges: []c.GraphEdge{{From: "n1", To: "n2", Kind: c.EdgePositive}}}, Details: c.HiddenDetails{Caveats: []string{"说明"}}}, CompiledAt: time.Now().UTC()}
		if err := store.UpsertCompiledOutput(context.Background(), record); err != nil {
			return nil, err
		}
		if err := store.PersistMemoryContentGraphFromCompiledOutput(context.Background(), "u-content-card", "twitter", "CGCARD1", time.Now().UTC()); err != nil {
			return nil, err
		}
		return store, nil
	}
	var stdout, stderr bytes.Buffer
	code := run([]string{"memory", "content-graphs", "--card", "--user", "u-content-card", "--platform", "twitter", "--id", "CGCARD1"}, "/tmp/project", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("content-graphs --card code = %d, stderr = %s", code, stderr.String())
	}
	for _, want := range []string{"Content Graph", "twitter", "CGCARD1", "Primary nodes"} {
		if !strings.Contains(stdout.String(), want) {
			t.Fatalf("stdout = %q, want substring %q", stdout.String(), want)
		}
	}
}

func TestRunMemoryEventGraphsSupportsSubjectFilter(t *testing.T) {
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
		sg := graphmodel.ContentSubgraph{ID: "subject-eg-cli", ArticleID: "subject-eg-cli", SourcePlatform: "twitter", SourceExternalID: "subject-eg-cli", CompileVersion: graphmodel.CompileBridgeVersion, CompiledAt: time.Now().UTC().Format(time.RFC3339), UpdatedAt: time.Now().UTC().Format(time.RFC3339), Nodes: []graphmodel.GraphNode{{ID: "n1", SourceArticleID: "subject-eg-cli", SourcePlatform: "twitter", SourceExternalID: "subject-eg-cli", RawText: "美联储加息0.25%", SubjectText: "美联储", ChangeText: "加息0.25%", Kind: graphmodel.NodeKindObservation, GraphRole: graphmodel.GraphRoleDriver, IsPrimary: true, VerificationStatus: graphmodel.VerificationPending, TimeBucket: "1w"}, {ID: "n2", SourceArticleID: "subject-eg-cli", SourcePlatform: "twitter", SourceExternalID: "subject-eg-cli", RawText: "未来一周美股承压", SubjectText: "美股", ChangeText: "承压", Kind: graphmodel.NodeKindPrediction, GraphRole: graphmodel.GraphRoleTarget, IsPrimary: true, VerificationStatus: graphmodel.VerificationProved, TimeBucket: "1w"}}}
		if err := store.PersistMemoryContentGraph(context.Background(), "u-event-subject", sg, time.Now().UTC()); err != nil {
			return nil, err
		}
		return store, nil
	}
	var stdout, stderr bytes.Buffer
	code := run([]string{"memory", "event-graphs", "--user", "u-event-subject", "--subject", "美联储"}, "/tmp/project", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("event-graphs --subject 美联储 code = %d, stderr = %s", code, stderr.String())
	}
	var out []contentstore.EventGraphRecord
	if err := json.Unmarshal(stdout.Bytes(), &out); err != nil {
		t.Fatalf("json.Unmarshal(event-graphs --subject) error = %v", err)
	}
	if len(out) != 1 || out[0].AnchorSubject != "美联储" {
		t.Fatalf("event-graphs filtered output = %#v, want 美联储 graph", out)
	}
}

func TestRunMemoryContentGraphsSupportsSubjectFilter(t *testing.T) {
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
		now := time.Now().UTC()
		for _, sg := range []graphmodel.ContentSubgraph{
			{ID: "subject-cg-cli-1", ArticleID: "subject-cg-cli-1", SourcePlatform: "twitter", SourceExternalID: "subject-cg-cli-1", CompileVersion: graphmodel.CompileBridgeVersion, CompiledAt: now.Format(time.RFC3339), UpdatedAt: now.Format(time.RFC3339), Nodes: []graphmodel.GraphNode{{ID: "n1", SourceArticleID: "subject-cg-cli-1", SourcePlatform: "twitter", SourceExternalID: "subject-cg-cli-1", RawText: "美联储加息0.25%", SubjectText: "美联储", ChangeText: "加息0.25%", Kind: graphmodel.NodeKindObservation, IsPrimary: true, VerificationStatus: graphmodel.VerificationPending}}},
			{ID: "subject-cg-cli-2", ArticleID: "subject-cg-cli-2", SourcePlatform: "twitter", SourceExternalID: "subject-cg-cli-2", CompileVersion: graphmodel.CompileBridgeVersion, CompiledAt: now.Format(time.RFC3339), UpdatedAt: now.Format(time.RFC3339), Nodes: []graphmodel.GraphNode{{ID: "n1", SourceArticleID: "subject-cg-cli-2", SourcePlatform: "twitter", SourceExternalID: "subject-cg-cli-2", RawText: "欧洲央行放缓缩表", SubjectText: "欧洲央行", ChangeText: "放缓缩表", Kind: graphmodel.NodeKindObservation, IsPrimary: true, VerificationStatus: graphmodel.VerificationPending}}},
		} {
			if err := store.PersistMemoryContentGraph(context.Background(), "u-content-subject", sg, now); err != nil {
				return nil, err
			}
		}
		return store, nil
	}
	var stdout, stderr bytes.Buffer
	code := run([]string{"memory", "content-graphs", "--user", "u-content-subject", "--subject", "美联储"}, "/tmp/project", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("content-graphs --subject 美联储 code = %d, stderr = %s", code, stderr.String())
	}
	var out []graphmodel.ContentSubgraph
	if err := json.Unmarshal(stdout.Bytes(), &out); err != nil {
		t.Fatalf("json.Unmarshal(content-graphs --subject) error = %v", err)
	}
	if len(out) != 1 || out[0].SourceExternalID != "subject-cg-cli-1" {
		t.Fatalf("content-graphs filtered output = %#v, want 美联储 snapshot only", out)
	}
}

func TestRunMemoryProjectAllRebuildsEventAndParadigmLayers(t *testing.T) {
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
		sg := graphmodel.ContentSubgraph{ID: "project-all", ArticleID: "project-all", SourcePlatform: "twitter", SourceExternalID: "project-all", CompileVersion: graphmodel.CompileBridgeVersion, CompiledAt: time.Now().UTC().Format(time.RFC3339), UpdatedAt: time.Now().UTC().Format(time.RFC3339), Nodes: []graphmodel.GraphNode{{ID: "n1", SourceArticleID: "project-all", SourcePlatform: "twitter", SourceExternalID: "project-all", RawText: "美联储加息0.25%", SubjectText: "美联储", ChangeText: "加息0.25%", Kind: graphmodel.NodeKindObservation, GraphRole: graphmodel.GraphRoleDriver, IsPrimary: true, VerificationStatus: graphmodel.VerificationPending, TimeBucket: "1w"}, {ID: "n2", SourceArticleID: "project-all", SourcePlatform: "twitter", SourceExternalID: "project-all", RawText: "未来一周美股承压", SubjectText: "美股", ChangeText: "承压", Kind: graphmodel.NodeKindPrediction, GraphRole: graphmodel.GraphRoleTarget, IsPrimary: true, VerificationStatus: graphmodel.VerificationProved, TimeBucket: "1w"}}}
		if err := store.PersistMemoryContentGraph(context.Background(), "u-project-all", sg, time.Now().UTC()); err != nil {
			return nil, err
		}
		return store, nil
	}
	var stdout, stderr bytes.Buffer
	code := run([]string{"memory", "project-all", "--user", "u-project-all"}, "/tmp/project", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("memory project-all code = %d, stderr = %s", code, stderr.String())
	}
	var out map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &out); err != nil {
		t.Fatalf("json.Unmarshal(project-all) error = %v", err)
	}
	if out["content_graphs"] == nil || out["event_graphs"] == nil || out["paradigms"] == nil || out["global_v2"] == nil {
		t.Fatalf("project-all output = %#v, want content_graphs/event_graphs/paradigms/global_v2 keys", out)
	}
	if ok, _ := out["ok"].(bool); !ok {
		t.Fatalf("project-all output = %#v, want ok=true", out)
	}
	metrics, ok := out["metrics"].(map[string]any)
	if !ok {
		t.Fatalf("project-all output = %#v, want metrics object", out)
	}
	for _, key := range []string{"event_graph_rebuild_ms", "paradigm_recompute_ms", "global_v2_rebuild_ms"} {
		value, ok := metrics[key]
		if !ok {
			t.Fatalf("project-all metrics = %#v, want key %q", metrics, key)
		}
		number, ok := value.(float64)
		if !ok || number < 0 {
			t.Fatalf("project-all metrics[%q] = %#v, want non-negative number", key, value)
		}
	}
}

func TestRunMemoryBackfillContentRebuildsOneContentGraphFromCompiledOutput(t *testing.T) {
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
		record := c.Record{
			UnitID:         "twitter:bf-content-1",
			Source:         "twitter",
			ExternalID:     "bf-content-1",
			RootExternalID: "bf-content-1",
			Model:          c.Qwen36PlusModel,
			Output: c.Output{
				Summary: "一句话",
				Drivers: []string{"美联储加息0.25%"},
				Targets: []string{"未来一周美股承压"},
				Graph: c.ReasoningGraph{
					Nodes: []c.GraphNode{
						{ID: "n1", Kind: c.NodeFact, Text: "美联储加息0.25%", OccurredAt: time.Now().UTC()},
						{ID: "n2", Kind: c.NodePrediction, Text: "未来一周美股承压", PredictionStartAt: time.Now().UTC(), PredictionDueAt: time.Now().UTC().Add(24 * time.Hour)},
					},
					Edges: []c.GraphEdge{{From: "n1", To: "n2", Kind: c.EdgePositive}},
				},
				Details: c.HiddenDetails{Caveats: []string{"说明"}},
			},
			CompiledAt: time.Now().UTC(),
		}
		if err := store.UpsertCompiledOutput(context.Background(), record); err != nil {
			return nil, err
		}
		return store, nil
	}
	var stdout, stderr bytes.Buffer
	code := run([]string{"memory", "backfill", "--layer", "content", "--user", "u-backfill-content", "--platform", "twitter", "--id", "bf-content-1"}, "/tmp/project", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("memory backfill content code = %d, stderr = %s", code, stderr.String())
	}
	var out map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &out); err != nil {
		t.Fatalf("json.Unmarshal(backfill content) error = %v", err)
	}
	if out["layer"] != "content" || out["content_graphs"] == nil {
		t.Fatalf("backfill content output = %#v, want layer=content and content_graphs key", out)
	}
	if got, ok := out["content_graphs"].(float64); !ok || got != 1 {
		t.Fatalf("content_graphs = %#v, want 1", out["content_graphs"])
	}
}

func TestRunMemoryCanonicalEntityUpsertAndList(t *testing.T) {
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
		return contentstore.NewSQLiteStore(path)
	}

	var stdout, stderr bytes.Buffer
	code := run([]string{"memory", "canonical-entity-upsert", "--id", "driver-fed", "--type", "driver", "--name", "美联储", "--aliases", "联储, Federal Reserve"}, "/tmp/project", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("canonical-entity-upsert code = %d, stderr = %s", code, stderr.String())
	}
	var upsertOut map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &upsertOut); err != nil {
		t.Fatalf("json.Unmarshal(canonical-entity-upsert) error = %v", err)
	}
	if ok, _ := upsertOut["ok"].(bool); !ok {
		t.Fatalf("upsert output = %#v, want ok=true", upsertOut)
	}

	stdout.Reset()
	stderr.Reset()
	code = run([]string{"memory", "canonical-entities"}, "/tmp/project", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("canonical-entities code = %d, stderr = %s", code, stderr.String())
	}
	var out []memory.CanonicalEntity
	if err := json.Unmarshal(stdout.Bytes(), &out); err != nil {
		t.Fatalf("json.Unmarshal(canonical-entities) error = %v", err)
	}
	if len(out) != 1 || out[0].CanonicalName != "美联储" {
		t.Fatalf("out = %#v, want one canonical entity named 美联储", out)
	}
	if len(out[0].Aliases) < 2 {
		t.Fatalf("aliases = %#v, want normalized aliases persisted", out[0].Aliases)
	}
}

func TestRunMemoryCanonicalEntityUpsertSupportsExplicitStatus(t *testing.T) {
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
		return contentstore.NewSQLiteStore(path)
	}

	var stdout, stderr bytes.Buffer
	code := run([]string{"memory", "canonical-entity-upsert", "--id", "driver-fed", "--type", "driver", "--name", "美联储", "--status", "retired"}, "/tmp/project", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("canonical-entity-upsert --status code = %d, stderr = %s", code, stderr.String())
	}
	stdout.Reset()
	stderr.Reset()
	code = run([]string{"memory", "canonical-entities"}, "/tmp/project", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("canonical-entities code = %d, stderr = %s", code, stderr.String())
	}
	var out []memory.CanonicalEntity
	if err := json.Unmarshal(stdout.Bytes(), &out); err != nil {
		t.Fatalf("json.Unmarshal(canonical-entities) error = %v", err)
	}
	if len(out) != 1 || out[0].Status != memory.CanonicalEntityRetired {
		t.Fatalf("out = %#v, want retired canonical entity", out)
	}
}

func TestRunMemoryCanonicalEntitiesSupportsAliasFilter(t *testing.T) {
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
		if err := store.UpsertCanonicalEntity(context.Background(), memory.CanonicalEntity{
			EntityID:      "driver-fed",
			EntityType:    memory.CanonicalEntityDriver,
			CanonicalName: "美联储",
			Aliases:       []string{"联储"},
			Status:        memory.CanonicalEntityActive,
		}); err != nil {
			return nil, err
		}
		if err := store.UpsertCanonicalEntity(context.Background(), memory.CanonicalEntity{
			EntityID:      "target-us-equity",
			EntityType:    memory.CanonicalEntityTarget,
			CanonicalName: "美股",
			Aliases:       []string{"美国股市"},
			Status:        memory.CanonicalEntityActive,
		}); err != nil {
			return nil, err
		}
		return store, nil
	}

	var stdout, stderr bytes.Buffer
	code := run([]string{"memory", "canonical-entities", "--alias", "联储"}, "/tmp/project", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("canonical-entities --alias code = %d, stderr = %s", code, stderr.String())
	}
	var out []memory.CanonicalEntity
	if err := json.Unmarshal(stdout.Bytes(), &out); err != nil {
		t.Fatalf("json.Unmarshal(canonical-entities --alias) error = %v", err)
	}
	if len(out) != 1 || out[0].CanonicalName != "美联储" {
		t.Fatalf("out = %#v, want only 美联储 under alias filter", out)
	}
}

func TestRunMemoryCanonicalEntitiesSupportsTypeAndStatusFilters(t *testing.T) {
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
		if err := store.UpsertCanonicalEntity(context.Background(), memory.CanonicalEntity{
			EntityID:      "driver-fed",
			EntityType:    memory.CanonicalEntityDriver,
			CanonicalName: "美联储",
			Status:        memory.CanonicalEntityActive,
		}); err != nil {
			return nil, err
		}
		if err := store.UpsertCanonicalEntity(context.Background(), memory.CanonicalEntity{
			EntityID:      "target-us-equity",
			EntityType:    memory.CanonicalEntityTarget,
			CanonicalName: "美股",
			Status:        memory.CanonicalEntityRetired,
		}); err != nil {
			return nil, err
		}
		return store, nil
	}

	var stdout, stderr bytes.Buffer
	code := run([]string{"memory", "canonical-entities", "--type", "target", "--status", "retired"}, "/tmp/project", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("canonical-entities filter code = %d, stderr = %s", code, stderr.String())
	}
	var out []memory.CanonicalEntity
	if err := json.Unmarshal(stdout.Bytes(), &out); err != nil {
		t.Fatalf("json.Unmarshal(canonical-entities filter) error = %v", err)
	}
	if len(out) != 1 || out[0].CanonicalName != "美股" {
		t.Fatalf("out = %#v, want only retired target canonical entity", out)
	}
}

func TestRunMemoryCanonicalEntitiesSupportsSummary(t *testing.T) {
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
		if err := store.UpsertCanonicalEntity(context.Background(), memory.CanonicalEntity{
			EntityID:      "driver-fed",
			EntityType:    memory.CanonicalEntityDriver,
			CanonicalName: "美联储",
			Aliases:       []string{"联储", "Federal Reserve"},
			Status:        memory.CanonicalEntityActive,
		}); err != nil {
			return nil, err
		}
		if err := store.UpsertCanonicalEntity(context.Background(), memory.CanonicalEntity{
			EntityID:      "target-us-equity",
			EntityType:    memory.CanonicalEntityTarget,
			CanonicalName: "美股",
			Aliases:       []string{"美国股市"},
			Status:        memory.CanonicalEntityRetired,
		}); err != nil {
			return nil, err
		}
		return store, nil
	}
	var stdout, stderr bytes.Buffer
	code := run([]string{"memory", "canonical-entities", "--summary"}, "/tmp/project", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("canonical-entities --summary code = %d, stderr = %s", code, stderr.String())
	}
	var out struct {
		TotalEntities int            `json:"total_entities"`
		TotalAliases  int            `json:"total_aliases"`
		ByType        map[string]int `json:"by_type"`
		ByStatus      map[string]int `json:"by_status"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &out); err != nil {
		t.Fatalf("json.Unmarshal(canonical-entities --summary) error = %v", err)
	}
	if out.TotalEntities != 2 || out.TotalAliases < 3 {
		t.Fatalf("summary = %#v, want 2 entities and at least 3 aliases", out)
	}
	if out.ByType["driver"] != 1 || out.ByType["target"] != 1 {
		t.Fatalf("summary by_type = %#v, want driver=1 target=1", out.ByType)
	}
	if out.ByStatus["active"] != 1 || out.ByStatus["retired"] != 1 {
		t.Fatalf("summary by_status = %#v, want active=1 retired=1", out.ByStatus)
	}
}

func TestRunMemoryCanonicalEntitiesSupportsIDFilter(t *testing.T) {
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
		if err := store.UpsertCanonicalEntity(context.Background(), memory.CanonicalEntity{
			EntityID:      "driver-fed",
			EntityType:    memory.CanonicalEntityDriver,
			CanonicalName: "美联储",
			Status:        memory.CanonicalEntityActive,
		}); err != nil {
			return nil, err
		}
		if err := store.UpsertCanonicalEntity(context.Background(), memory.CanonicalEntity{
			EntityID:      "target-us-equity",
			EntityType:    memory.CanonicalEntityTarget,
			CanonicalName: "美股",
			Status:        memory.CanonicalEntityActive,
		}); err != nil {
			return nil, err
		}
		return store, nil
	}
	var stdout, stderr bytes.Buffer
	code := run([]string{"memory", "canonical-entities", "--id", "driver-fed"}, "/tmp/project", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("canonical-entities --id code = %d, stderr = %s", code, stderr.String())
	}
	var out []memory.CanonicalEntity
	if err := json.Unmarshal(stdout.Bytes(), &out); err != nil {
		t.Fatalf("json.Unmarshal(canonical-entities --id) error = %v", err)
	}
	if len(out) != 1 || out[0].EntityID != "driver-fed" {
		t.Fatalf("out = %#v, want only driver-fed under id filter", out)
	}
}

func TestRunMemoryCanonicalEntitiesCardRendersReadableOutput(t *testing.T) {
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
		if err := store.UpsertCanonicalEntity(context.Background(), memory.CanonicalEntity{
			EntityID:      "driver-fed",
			EntityType:    memory.CanonicalEntityDriver,
			CanonicalName: "美联储",
			Aliases:       []string{"联储", "Federal Reserve"},
			Status:        memory.CanonicalEntityActive,
		}); err != nil {
			return nil, err
		}
		return store, nil
	}
	var stdout, stderr bytes.Buffer
	code := run([]string{"memory", "canonical-entities", "--card"}, "/tmp/project", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("canonical-entities --card code = %d, stderr = %s", code, stderr.String())
	}
	for _, want := range []string{"Canonical Entity", "driver-fed", "美联储", "driver", "active", "联储"} {
		if !strings.Contains(stdout.String(), want) {
			t.Fatalf("stdout = %q, want substring %q", stdout.String(), want)
		}
	}
}

func TestRunMemoryBackfillAllRebuildsAggregateLayers(t *testing.T) {
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
		sg := graphmodel.ContentSubgraph{ID: "bf-all", ArticleID: "bf-all", SourcePlatform: "twitter", SourceExternalID: "bf-all", CompileVersion: graphmodel.CompileBridgeVersion, CompiledAt: time.Now().UTC().Format(time.RFC3339), UpdatedAt: time.Now().UTC().Format(time.RFC3339), Nodes: []graphmodel.GraphNode{{ID: "n1", SourceArticleID: "bf-all", SourcePlatform: "twitter", SourceExternalID: "bf-all", RawText: "美联储加息0.25%", SubjectText: "美联储", ChangeText: "加息0.25%", Kind: graphmodel.NodeKindObservation, GraphRole: graphmodel.GraphRoleDriver, IsPrimary: true, VerificationStatus: graphmodel.VerificationPending, TimeBucket: "1w"}, {ID: "n2", SourceArticleID: "bf-all", SourcePlatform: "twitter", SourceExternalID: "bf-all", RawText: "未来一周美股承压", SubjectText: "美股", ChangeText: "承压", Kind: graphmodel.NodeKindPrediction, GraphRole: graphmodel.GraphRoleTarget, IsPrimary: true, VerificationStatus: graphmodel.VerificationProved, TimeBucket: "1w"}}}
		if err := store.PersistMemoryContentGraph(context.Background(), "u-backfill-all", sg, time.Now().UTC()); err != nil {
			return nil, err
		}
		return store, nil
	}
	var stdout, stderr bytes.Buffer
	code := run([]string{"memory", "backfill", "--layer", "all", "--user", "u-backfill-all"}, "/tmp/project", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("memory backfill all code = %d, stderr = %s", code, stderr.String())
	}
	var out map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &out); err != nil {
		t.Fatalf("json.Unmarshal(backfill all) error = %v", err)
	}
	if out["layer"] != "all" || out["event_graphs"] == nil || out["paradigms"] == nil || out["global_v2"] == nil {
		t.Fatalf("backfill all output = %#v, want aggregate keys", out)
	}
}

func TestRunMemoryBackfillEventRebuildsEventLayer(t *testing.T) {
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
		sg := graphmodel.ContentSubgraph{ID: "bf-event", ArticleID: "bf-event", SourcePlatform: "twitter", SourceExternalID: "bf-event", CompileVersion: graphmodel.CompileBridgeVersion, CompiledAt: time.Now().UTC().Format(time.RFC3339), UpdatedAt: time.Now().UTC().Format(time.RFC3339), Nodes: []graphmodel.GraphNode{{ID: "n1", SourceArticleID: "bf-event", SourcePlatform: "twitter", SourceExternalID: "bf-event", RawText: "美联储加息0.25%", SubjectText: "美联储", ChangeText: "加息0.25%", Kind: graphmodel.NodeKindObservation, GraphRole: graphmodel.GraphRoleDriver, IsPrimary: true, VerificationStatus: graphmodel.VerificationPending, TimeBucket: "1w"}, {ID: "n2", SourceArticleID: "bf-event", SourcePlatform: "twitter", SourceExternalID: "bf-event", RawText: "未来一周美股承压", SubjectText: "美股", ChangeText: "承压", Kind: graphmodel.NodeKindPrediction, GraphRole: graphmodel.GraphRoleTarget, IsPrimary: true, VerificationStatus: graphmodel.VerificationProved, TimeBucket: "1w"}}}
		if err := store.PersistMemoryContentGraph(context.Background(), "u-backfill-event", sg, time.Now().UTC()); err != nil {
			return nil, err
		}
		return store, nil
	}
	var stdout, stderr bytes.Buffer
	code := run([]string{"memory", "backfill", "--layer", "event", "--user", "u-backfill-event"}, "/tmp/project", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("memory backfill event code = %d, stderr = %s", code, stderr.String())
	}
	var out map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &out); err != nil {
		t.Fatalf("json.Unmarshal(backfill event) error = %v", err)
	}
	if out["layer"] != "event" || out["event_graphs"] == nil {
		t.Fatalf("backfill event output = %#v, want layer=event and event_graphs key", out)
	}
}

func TestRunMemoryBackfillParadigmRebuildsParadigmLayer(t *testing.T) {
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
		sg := graphmodel.ContentSubgraph{ID: "bf-paradigm", ArticleID: "bf-paradigm", SourcePlatform: "twitter", SourceExternalID: "bf-paradigm", CompileVersion: graphmodel.CompileBridgeVersion, CompiledAt: time.Now().UTC().Format(time.RFC3339), UpdatedAt: time.Now().UTC().Format(time.RFC3339), Nodes: []graphmodel.GraphNode{{ID: "n1", SourceArticleID: "bf-paradigm", SourcePlatform: "twitter", SourceExternalID: "bf-paradigm", RawText: "美联储加息0.25%", SubjectText: "美联储", ChangeText: "加息0.25%", Kind: graphmodel.NodeKindObservation, GraphRole: graphmodel.GraphRoleDriver, IsPrimary: true, VerificationStatus: graphmodel.VerificationPending, TimeBucket: "1w"}, {ID: "n2", SourceArticleID: "bf-paradigm", SourcePlatform: "twitter", SourceExternalID: "bf-paradigm", RawText: "未来一周美股承压", SubjectText: "美股", ChangeText: "承压", Kind: graphmodel.NodeKindPrediction, GraphRole: graphmodel.GraphRoleTarget, IsPrimary: true, VerificationStatus: graphmodel.VerificationProved, TimeBucket: "1w"}}}
		if err := store.PersistMemoryContentGraph(context.Background(), "u-backfill-paradigm", sg, time.Now().UTC()); err != nil {
			return nil, err
		}
		return store, nil
	}
	var stdout, stderr bytes.Buffer
	code := run([]string{"memory", "backfill", "--layer", "paradigm", "--user", "u-backfill-paradigm"}, "/tmp/project", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("memory backfill paradigm code = %d, stderr = %s", code, stderr.String())
	}
	var out map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &out); err != nil {
		t.Fatalf("json.Unmarshal(backfill paradigm) error = %v", err)
	}
	if out["layer"] != "paradigm" || out["paradigms"] == nil {
		t.Fatalf("backfill paradigm output = %#v, want layer=paradigm and paradigms key", out)
	}
}

func TestRunMemoryBackfillGlobalV2RebuildsGlobalLayer(t *testing.T) {
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
		sg := graphmodel.ContentSubgraph{ID: "bf-global-v2", ArticleID: "bf-global-v2", SourcePlatform: "twitter", SourceExternalID: "bf-global-v2", CompileVersion: graphmodel.CompileBridgeVersion, CompiledAt: time.Now().UTC().Format(time.RFC3339), UpdatedAt: time.Now().UTC().Format(time.RFC3339), Nodes: []graphmodel.GraphNode{{ID: "n1", SourceArticleID: "bf-global-v2", SourcePlatform: "twitter", SourceExternalID: "bf-global-v2", RawText: "美联储加息0.25%", SubjectText: "美联储", ChangeText: "加息0.25%", Kind: graphmodel.NodeKindObservation, GraphRole: graphmodel.GraphRoleDriver, IsPrimary: true, VerificationStatus: graphmodel.VerificationPending, TimeBucket: "1w"}, {ID: "n2", SourceArticleID: "bf-global-v2", SourcePlatform: "twitter", SourceExternalID: "bf-global-v2", RawText: "未来一周美股承压", SubjectText: "美股", ChangeText: "承压", Kind: graphmodel.NodeKindPrediction, GraphRole: graphmodel.GraphRoleTarget, IsPrimary: true, VerificationStatus: graphmodel.VerificationProved, TimeBucket: "1w"}}}
		if err := store.PersistMemoryContentGraph(context.Background(), "u-backfill-global-v2", sg, time.Now().UTC()); err != nil {
			return nil, err
		}
		return store, nil
	}
	var stdout, stderr bytes.Buffer
	code := run([]string{"memory", "backfill", "--layer", "global-v2", "--user", "u-backfill-global-v2"}, "/tmp/project", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("memory backfill global-v2 code = %d, stderr = %s", code, stderr.String())
	}
	var out map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &out); err != nil {
		t.Fatalf("json.Unmarshal(backfill global-v2) error = %v", err)
	}
	if out["layer"] != "global-v2" || out["global_v2"] == nil {
		t.Fatalf("backfill global-v2 output = %#v, want layer=global-v2 and global_v2 key", out)
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

func TestRunMemoryEventGraphsCombinesScopeAndSubjectFilters(t *testing.T) {
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
		sg := graphmodel.ContentSubgraph{ID: "combo-eg", ArticleID: "combo-eg", SourcePlatform: "twitter", SourceExternalID: "combo-eg", CompileVersion: graphmodel.CompileBridgeVersion, CompiledAt: time.Now().UTC().Format(time.RFC3339), UpdatedAt: time.Now().UTC().Format(time.RFC3339), Nodes: []graphmodel.GraphNode{{ID: "n1", SourceArticleID: "combo-eg", SourcePlatform: "twitter", SourceExternalID: "combo-eg", RawText: "美联储加息0.25%", SubjectText: "美联储", ChangeText: "加息0.25%", Kind: graphmodel.NodeKindObservation, GraphRole: graphmodel.GraphRoleDriver, IsPrimary: true, VerificationStatus: graphmodel.VerificationPending, TimeBucket: "1w"}, {ID: "n2", SourceArticleID: "combo-eg", SourcePlatform: "twitter", SourceExternalID: "combo-eg", RawText: "未来一周美股承压", SubjectText: "美股", ChangeText: "承压", Kind: graphmodel.NodeKindPrediction, GraphRole: graphmodel.GraphRoleTarget, IsPrimary: true, VerificationStatus: graphmodel.VerificationProved, TimeBucket: "1w"}}}
		if err := store.PersistMemoryContentGraph(context.Background(), "u-combo-eg", sg, time.Now().UTC()); err != nil {
			return nil, err
		}
		return store, nil
	}
	var stdout, stderr bytes.Buffer
	code := run([]string{"memory", "event-graphs", "--user", "u-combo-eg", "--scope", "driver", "--subject", "美股"}, "/tmp/project", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("event-graphs combined filter code = %d, stderr = %s", code, stderr.String())
	}
	var out []contentstore.EventGraphRecord
	if err := json.Unmarshal(stdout.Bytes(), &out); err != nil {
		t.Fatalf("json.Unmarshal(event-graphs combined) error = %v", err)
	}
	if len(out) != 0 {
		t.Fatalf("event-graphs combined output = %#v, want empty intersection", out)
	}
}

func TestRunMemoryContentGraphsCombinesSourceAndSubjectFilters(t *testing.T) {
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
		now := time.Now().UTC()
		for _, sg := range []graphmodel.ContentSubgraph{
			{ID: "combo-cg-1", ArticleID: "combo-cg-1", SourcePlatform: "twitter", SourceExternalID: "combo-cg-1", CompileVersion: graphmodel.CompileBridgeVersion, CompiledAt: now.Format(time.RFC3339), UpdatedAt: now.Format(time.RFC3339), Nodes: []graphmodel.GraphNode{{ID: "n1", SourceArticleID: "combo-cg-1", SourcePlatform: "twitter", SourceExternalID: "combo-cg-1", RawText: "美联储加息0.25%", SubjectText: "美联储", ChangeText: "加息0.25%", Kind: graphmodel.NodeKindObservation, IsPrimary: true, VerificationStatus: graphmodel.VerificationPending}}},
			{ID: "combo-cg-2", ArticleID: "combo-cg-2", SourcePlatform: "twitter", SourceExternalID: "combo-cg-2", CompileVersion: graphmodel.CompileBridgeVersion, CompiledAt: now.Format(time.RFC3339), UpdatedAt: now.Format(time.RFC3339), Nodes: []graphmodel.GraphNode{{ID: "n1", SourceArticleID: "combo-cg-2", SourcePlatform: "twitter", SourceExternalID: "combo-cg-2", RawText: "欧洲央行放缓缩表", SubjectText: "欧洲央行", ChangeText: "放缓缩表", Kind: graphmodel.NodeKindObservation, IsPrimary: true, VerificationStatus: graphmodel.VerificationPending}}},
		} {
			if err := store.PersistMemoryContentGraph(context.Background(), "u-combo-cg", sg, now); err != nil {
				return nil, err
			}
		}
		return store, nil
	}
	var stdout, stderr bytes.Buffer
	code := run([]string{"memory", "content-graphs", "--user", "u-combo-cg", "--platform", "twitter", "--id", "combo-cg-2", "--subject", "美联储"}, "/tmp/project", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("content-graphs combined filter code = %d, stderr = %s", code, stderr.String())
	}
	var out []graphmodel.ContentSubgraph
	if err := json.Unmarshal(stdout.Bytes(), &out); err != nil {
		t.Fatalf("json.Unmarshal(content-graphs combined) error = %v", err)
	}
	if len(out) != 0 {
		t.Fatalf("content-graphs combined output = %#v, want empty intersection", out)
	}
}

func TestRunMemoryEventGraphsCardSupportsSubjectFilter(t *testing.T) {
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
		sg := graphmodel.ContentSubgraph{ID: "card-subject-eg", ArticleID: "card-subject-eg", SourcePlatform: "twitter", SourceExternalID: "card-subject-eg", CompileVersion: graphmodel.CompileBridgeVersion, CompiledAt: time.Now().UTC().Format(time.RFC3339), UpdatedAt: time.Now().UTC().Format(time.RFC3339), Nodes: []graphmodel.GraphNode{{ID: "n1", SourceArticleID: "card-subject-eg", SourcePlatform: "twitter", SourceExternalID: "card-subject-eg", RawText: "美联储加息0.25%", SubjectText: "美联储", ChangeText: "加息0.25%", Kind: graphmodel.NodeKindObservation, GraphRole: graphmodel.GraphRoleDriver, IsPrimary: true, VerificationStatus: graphmodel.VerificationPending, TimeBucket: "1w"}, {ID: "n2", SourceArticleID: "card-subject-eg", SourcePlatform: "twitter", SourceExternalID: "card-subject-eg", RawText: "未来一周美股承压", SubjectText: "美股", ChangeText: "承压", Kind: graphmodel.NodeKindPrediction, GraphRole: graphmodel.GraphRoleTarget, IsPrimary: true, VerificationStatus: graphmodel.VerificationProved, TimeBucket: "1w"}}}
		if err := store.PersistMemoryContentGraph(context.Background(), "u-event-card-subject", sg, time.Now().UTC()); err != nil {
			return nil, err
		}
		return store, nil
	}
	var stdout, stderr bytes.Buffer
	code := run([]string{"memory", "event-graphs", "--card", "--user", "u-event-card-subject", "--subject", "美联储"}, "/tmp/project", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("event-graphs --card --subject code = %d, stderr = %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "美联储") || strings.Contains(stdout.String(), "欧洲央行") {
		t.Fatalf("stdout = %q, want filtered event card output", stdout.String())
	}
}

func TestRunMemoryEventGraphsSupportsAliasSubjectFilter(t *testing.T) {
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
		now := time.Now().UTC()
		if err := store.UpsertCanonicalEntity(context.Background(), memory.CanonicalEntity{
			EntityID:      "driver-fed",
			EntityType:    memory.CanonicalEntityDriver,
			CanonicalName: "美联储",
			Aliases:       []string{"联储"},
			Status:        memory.CanonicalEntityActive,
			CreatedAt:     now,
			UpdatedAt:     now,
		}); err != nil {
			return nil, err
		}
		sg := graphmodel.ContentSubgraph{ID: "alias-eg", ArticleID: "alias-eg", SourcePlatform: "twitter", SourceExternalID: "alias-eg", CompileVersion: graphmodel.CompileBridgeVersion, CompiledAt: now.Format(time.RFC3339), UpdatedAt: now.Format(time.RFC3339), Nodes: []graphmodel.GraphNode{{ID: "n1", SourceArticleID: "alias-eg", SourcePlatform: "twitter", SourceExternalID: "alias-eg", RawText: "联储加息0.25%", SubjectText: "联储", ChangeText: "加息0.25%", Kind: graphmodel.NodeKindObservation, GraphRole: graphmodel.GraphRoleDriver, IsPrimary: true, VerificationStatus: graphmodel.VerificationPending, TimeBucket: "1w"}}}
		if err := store.PersistMemoryContentGraph(context.Background(), "u-event-alias-filter", sg, now); err != nil {
			return nil, err
		}
		return store, nil
	}
	var stdout, stderr bytes.Buffer
	code := run([]string{"memory", "event-graphs", "--user", "u-event-alias-filter", "--subject", "联储"}, "/tmp/project", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("event-graphs alias subject code = %d, stderr = %s", code, stderr.String())
	}
	var out []contentstore.EventGraphRecord
	if err := json.Unmarshal(stdout.Bytes(), &out); err != nil {
		t.Fatalf("json.Unmarshal(event-graphs alias) error = %v", err)
	}
	if len(out) != 1 || out[0].AnchorSubject != "美联储" {
		t.Fatalf("out = %#v, want alias lookup to return canonical 美联储 event graph", out)
	}
}

func TestRunMemoryContentGraphsCardSupportsSubjectFilter(t *testing.T) {
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
		now := time.Now().UTC()
		for _, sg := range []graphmodel.ContentSubgraph{
			{ID: "card-subject-cg-1", ArticleID: "card-subject-cg-1", SourcePlatform: "twitter", SourceExternalID: "card-subject-cg-1", CompileVersion: graphmodel.CompileBridgeVersion, CompiledAt: now.Format(time.RFC3339), UpdatedAt: now.Format(time.RFC3339), Nodes: []graphmodel.GraphNode{{ID: "n1", SourceArticleID: "card-subject-cg-1", SourcePlatform: "twitter", SourceExternalID: "card-subject-cg-1", RawText: "美联储加息0.25%", SubjectText: "美联储", ChangeText: "加息0.25%", Kind: graphmodel.NodeKindObservation, IsPrimary: true, VerificationStatus: graphmodel.VerificationPending}}},
			{ID: "card-subject-cg-2", ArticleID: "card-subject-cg-2", SourcePlatform: "twitter", SourceExternalID: "card-subject-cg-2", CompileVersion: graphmodel.CompileBridgeVersion, CompiledAt: now.Format(time.RFC3339), UpdatedAt: now.Format(time.RFC3339), Nodes: []graphmodel.GraphNode{{ID: "n1", SourceArticleID: "card-subject-cg-2", SourcePlatform: "twitter", SourceExternalID: "card-subject-cg-2", RawText: "欧洲央行放缓缩表", SubjectText: "欧洲央行", ChangeText: "放缓缩表", Kind: graphmodel.NodeKindObservation, IsPrimary: true, VerificationStatus: graphmodel.VerificationPending}}},
		} {
			if err := store.PersistMemoryContentGraph(context.Background(), "u-content-card-subject", sg, now); err != nil {
				return nil, err
			}
		}
		return store, nil
	}
	var stdout, stderr bytes.Buffer
	code := run([]string{"memory", "content-graphs", "--card", "--user", "u-content-card-subject", "--subject", "美联储"}, "/tmp/project", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("content-graphs --card --subject code = %d, stderr = %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "美联储") || strings.Contains(stdout.String(), "欧洲央行") {
		t.Fatalf("stdout = %q, want filtered content graph card output", stdout.String())
	}
}

func TestRunMemoryContentGraphsSupportsAliasSubjectFilter(t *testing.T) {
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
		now := time.Now().UTC()
		if err := store.UpsertCanonicalEntity(context.Background(), memory.CanonicalEntity{
			EntityID:      "driver-fed",
			EntityType:    memory.CanonicalEntityDriver,
			CanonicalName: "美联储",
			Aliases:       []string{"联储"},
			Status:        memory.CanonicalEntityActive,
			CreatedAt:     now,
			UpdatedAt:     now,
		}); err != nil {
			return nil, err
		}
		sg := graphmodel.ContentSubgraph{ID: "alias-cg", ArticleID: "alias-cg", SourcePlatform: "twitter", SourceExternalID: "alias-cg", CompileVersion: graphmodel.CompileBridgeVersion, CompiledAt: now.Format(time.RFC3339), UpdatedAt: now.Format(time.RFC3339), Nodes: []graphmodel.GraphNode{{ID: "n1", SourceArticleID: "alias-cg", SourcePlatform: "twitter", SourceExternalID: "alias-cg", RawText: "联储加息0.25%", SubjectText: "联储", ChangeText: "加息0.25%", SubjectCanonical: "美联储", Kind: graphmodel.NodeKindObservation, IsPrimary: true, VerificationStatus: graphmodel.VerificationPending}}}
		if err := store.PersistMemoryContentGraph(context.Background(), "u-content-alias-filter", sg, now); err != nil {
			return nil, err
		}
		return store, nil
	}
	var stdout, stderr bytes.Buffer
	code := run([]string{"memory", "content-graphs", "--user", "u-content-alias-filter", "--subject", "联储"}, "/tmp/project", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("content-graphs alias subject code = %d, stderr = %s", code, stderr.String())
	}
	var out []graphmodel.ContentSubgraph
	if err := json.Unmarshal(stdout.Bytes(), &out); err != nil {
		t.Fatalf("json.Unmarshal(content-graphs alias) error = %v", err)
	}
	if len(out) != 1 {
		t.Fatalf("out = %#v, want one content graph for alias lookup", out)
	}
}

func TestRunMemoryContentGraphsResolvesAliasToCanonicalSubjectFilter(t *testing.T) {
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
		now := time.Now().UTC()
		if err := store.UpsertCanonicalEntity(context.Background(), memory.CanonicalEntity{
			EntityID:      "driver-fed",
			EntityType:    memory.CanonicalEntityDriver,
			CanonicalName: "美联储",
			Aliases:       []string{"联储"},
			Status:        memory.CanonicalEntityActive,
			CreatedAt:     now,
			UpdatedAt:     now,
		}); err != nil {
			return nil, err
		}
		sg := graphmodel.ContentSubgraph{ID: "canonical-cg", ArticleID: "canonical-cg", SourcePlatform: "twitter", SourceExternalID: "canonical-cg", CompileVersion: graphmodel.CompileBridgeVersion, CompiledAt: now.Format(time.RFC3339), UpdatedAt: now.Format(time.RFC3339), Nodes: []graphmodel.GraphNode{{ID: "n1", SourceArticleID: "canonical-cg", SourcePlatform: "twitter", SourceExternalID: "canonical-cg", RawText: "美联储加息0.25%", SubjectText: "美联储", ChangeText: "加息0.25%", Kind: graphmodel.NodeKindObservation, IsPrimary: true, VerificationStatus: graphmodel.VerificationPending}}}
		if err := store.PersistMemoryContentGraph(context.Background(), "u-content-canonical-filter", sg, now); err != nil {
			return nil, err
		}
		return store, nil
	}
	var stdout, stderr bytes.Buffer
	code := run([]string{"memory", "content-graphs", "--user", "u-content-canonical-filter", "--subject", "联储"}, "/tmp/project", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("content-graphs canonical alias filter code = %d, stderr = %s", code, stderr.String())
	}
	var out []graphmodel.ContentSubgraph
	if err := json.Unmarshal(stdout.Bytes(), &out); err != nil {
		t.Fatalf("json.Unmarshal(content-graphs canonical alias) error = %v", err)
	}
	if len(out) != 1 {
		t.Fatalf("out = %#v, want one content graph for canonical alias lookup", out)
	}
	if out[0].Nodes[0].SubjectText != "美联储" {
		t.Fatalf("out = %#v, want canonical subject presentation in returned payload", out)
	}
}

func TestRunMemoryContentGraphsSupportsSourceAndAliasSubjectFilter(t *testing.T) {
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
		now := time.Now().UTC()
		if err := store.UpsertCanonicalEntity(context.Background(), memory.CanonicalEntity{
			EntityID:      "driver-fed",
			EntityType:    memory.CanonicalEntityDriver,
			CanonicalName: "美联储",
			Aliases:       []string{"联储"},
			Status:        memory.CanonicalEntityActive,
			CreatedAt:     now,
			UpdatedAt:     now,
		}); err != nil {
			return nil, err
		}
		for _, sg := range []graphmodel.ContentSubgraph{
			{ID: "source-alias-cg-1", ArticleID: "source-alias-cg-1", SourcePlatform: "twitter", SourceExternalID: "source-alias-cg-1", CompileVersion: graphmodel.CompileBridgeVersion, CompiledAt: now.Format(time.RFC3339), UpdatedAt: now.Format(time.RFC3339), Nodes: []graphmodel.GraphNode{{ID: "n1", SourceArticleID: "source-alias-cg-1", SourcePlatform: "twitter", SourceExternalID: "source-alias-cg-1", RawText: "美联储加息0.25%", SubjectText: "美联储", ChangeText: "加息0.25%", Kind: graphmodel.NodeKindObservation, IsPrimary: true, VerificationStatus: graphmodel.VerificationPending}}},
			{ID: "source-alias-cg-2", ArticleID: "source-alias-cg-2", SourcePlatform: "twitter", SourceExternalID: "source-alias-cg-2", CompileVersion: graphmodel.CompileBridgeVersion, CompiledAt: now.Format(time.RFC3339), UpdatedAt: now.Format(time.RFC3339), Nodes: []graphmodel.GraphNode{{ID: "n1", SourceArticleID: "source-alias-cg-2", SourcePlatform: "twitter", SourceExternalID: "source-alias-cg-2", RawText: "欧洲央行放缓缩表", SubjectText: "欧洲央行", ChangeText: "放缓缩表", Kind: graphmodel.NodeKindObservation, IsPrimary: true, VerificationStatus: graphmodel.VerificationPending}}},
		} {
			if err := store.PersistMemoryContentGraph(context.Background(), "u-content-source-alias-filter", sg, now); err != nil {
				return nil, err
			}
		}
		return store, nil
	}
	var stdout, stderr bytes.Buffer
	code := run([]string{"memory", "content-graphs", "--user", "u-content-source-alias-filter", "--platform", "twitter", "--id", "source-alias-cg-1", "--subject", "联储"}, "/tmp/project", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("content-graphs source+alias subject code = %d, stderr = %s", code, stderr.String())
	}
	var out []graphmodel.ContentSubgraph
	if err := json.Unmarshal(stdout.Bytes(), &out); err != nil {
		t.Fatalf("json.Unmarshal(content-graphs source+alias) error = %v", err)
	}
	if len(out) != 1 || out[0].SourceExternalID != "source-alias-cg-1" {
		t.Fatalf("out = %#v, want one source-filtered alias match", out)
	}
}

func TestRunMemoryParadigmsCardSupportsSubjectFilter(t *testing.T) {
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
		now := time.Now().UTC()
		for _, sg := range []graphmodel.ContentSubgraph{
			{ID: "card-subject-pg-1", ArticleID: "card-subject-pg-1", SourcePlatform: "twitter", SourceExternalID: "card-subject-pg-1", CompileVersion: graphmodel.CompileBridgeVersion, CompiledAt: now.Format(time.RFC3339), UpdatedAt: now.Format(time.RFC3339), Nodes: []graphmodel.GraphNode{{ID: "n1", SourceArticleID: "card-subject-pg-1", SourcePlatform: "twitter", SourceExternalID: "card-subject-pg-1", RawText: "美联储加息0.25%", SubjectText: "美联储", ChangeText: "加息0.25%", Kind: graphmodel.NodeKindObservation, GraphRole: graphmodel.GraphRoleDriver, IsPrimary: true, VerificationStatus: graphmodel.VerificationPending, TimeBucket: "1w"}, {ID: "n2", SourceArticleID: "card-subject-pg-1", SourcePlatform: "twitter", SourceExternalID: "card-subject-pg-1", RawText: "未来一周美股承压", SubjectText: "美股", ChangeText: "承压", Kind: graphmodel.NodeKindPrediction, GraphRole: graphmodel.GraphRoleTarget, IsPrimary: true, VerificationStatus: graphmodel.VerificationProved, TimeBucket: "1w"}}},
			{ID: "card-subject-pg-2", ArticleID: "card-subject-pg-2", SourcePlatform: "twitter", SourceExternalID: "card-subject-pg-2", CompileVersion: graphmodel.CompileBridgeVersion, CompiledAt: now.Format(time.RFC3339), UpdatedAt: now.Format(time.RFC3339), Nodes: []graphmodel.GraphNode{{ID: "n1", SourceArticleID: "card-subject-pg-2", SourcePlatform: "twitter", SourceExternalID: "card-subject-pg-2", RawText: "欧洲央行放缓缩表", SubjectText: "欧洲央行", ChangeText: "放缓缩表", Kind: graphmodel.NodeKindObservation, GraphRole: graphmodel.GraphRoleDriver, IsPrimary: true, VerificationStatus: graphmodel.VerificationPending, TimeBucket: "1w"}, {ID: "n2", SourceArticleID: "card-subject-pg-2", SourcePlatform: "twitter", SourceExternalID: "card-subject-pg-2", RawText: "欧股承压", SubjectText: "欧股", ChangeText: "承压", Kind: graphmodel.NodeKindPrediction, GraphRole: graphmodel.GraphRoleTarget, IsPrimary: true, VerificationStatus: graphmodel.VerificationProved, TimeBucket: "1w"}}},
		} {
			if err := store.PersistMemoryContentGraph(context.Background(), "u-paradigm-card-subject", sg, now); err != nil {
				return nil, err
			}
		}
		return store, nil
	}
	var stdout, stderr bytes.Buffer
	code := run([]string{"memory", "paradigms", "--card", "--user", "u-paradigm-card-subject", "--subject", "美联储"}, "/tmp/project", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("paradigms --card --subject code = %d, stderr = %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "美联储") || strings.Contains(stdout.String(), "欧洲央行") {
		t.Fatalf("stdout = %q, want filtered paradigm card output", stdout.String())
	}
}

func TestRunMemoryParadigmsSupportsAliasSubjectFilter(t *testing.T) {
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
		now := time.Now().UTC()
		if err := store.UpsertCanonicalEntity(context.Background(), memory.CanonicalEntity{
			EntityID:      "driver-fed",
			EntityType:    memory.CanonicalEntityDriver,
			CanonicalName: "美联储",
			Aliases:       []string{"联储"},
			Status:        memory.CanonicalEntityActive,
			CreatedAt:     now,
			UpdatedAt:     now,
		}); err != nil {
			return nil, err
		}
		sg := graphmodel.ContentSubgraph{ID: "alias-pg", ArticleID: "alias-pg", SourcePlatform: "twitter", SourceExternalID: "alias-pg", CompileVersion: graphmodel.CompileBridgeVersion, CompiledAt: now.Format(time.RFC3339), UpdatedAt: now.Format(time.RFC3339), Nodes: []graphmodel.GraphNode{{ID: "n1", SourceArticleID: "alias-pg", SourcePlatform: "twitter", SourceExternalID: "alias-pg", RawText: "联储加息0.25%", SubjectText: "联储", ChangeText: "加息0.25%", Kind: graphmodel.NodeKindObservation, GraphRole: graphmodel.GraphRoleDriver, IsPrimary: true, VerificationStatus: graphmodel.VerificationPending, TimeBucket: "1w"}, {ID: "n2", SourceArticleID: "alias-pg", SourcePlatform: "twitter", SourceExternalID: "alias-pg", RawText: "未来一周美股承压", SubjectText: "美股", ChangeText: "承压", Kind: graphmodel.NodeKindPrediction, GraphRole: graphmodel.GraphRoleTarget, IsPrimary: true, VerificationStatus: graphmodel.VerificationProved, TimeBucket: "1w"}}}
		if err := store.PersistMemoryContentGraph(context.Background(), "u-paradigm-alias-filter", sg, now); err != nil {
			return nil, err
		}
		return store, nil
	}
	var stdout, stderr bytes.Buffer
	code := run([]string{"memory", "paradigms", "--user", "u-paradigm-alias-filter", "--subject", "联储"}, "/tmp/project", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("paradigms alias subject code = %d, stderr = %s", code, stderr.String())
	}
	var out []contentstore.ParadigmRecord
	if err := json.Unmarshal(stdout.Bytes(), &out); err != nil {
		t.Fatalf("json.Unmarshal(paradigms alias) error = %v", err)
	}
	if len(out) != 1 || out[0].DriverSubject != "美联储" {
		t.Fatalf("out = %#v, want alias lookup to return canonical 美联储 paradigm", out)
	}
}

func TestRunMemoryEventGraphsCardSupportsScopeFilter(t *testing.T) {
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
		sg := graphmodel.ContentSubgraph{ID: "card-scope-eg", ArticleID: "card-scope-eg", SourcePlatform: "twitter", SourceExternalID: "card-scope-eg", CompileVersion: graphmodel.CompileBridgeVersion, CompiledAt: time.Now().UTC().Format(time.RFC3339), UpdatedAt: time.Now().UTC().Format(time.RFC3339), Nodes: []graphmodel.GraphNode{{ID: "n1", SourceArticleID: "card-scope-eg", SourcePlatform: "twitter", SourceExternalID: "card-scope-eg", RawText: "美联储加息0.25%", SubjectText: "美联储", ChangeText: "加息0.25%", Kind: graphmodel.NodeKindObservation, GraphRole: graphmodel.GraphRoleDriver, IsPrimary: true, VerificationStatus: graphmodel.VerificationPending, TimeBucket: "1w"}, {ID: "n2", SourceArticleID: "card-scope-eg", SourcePlatform: "twitter", SourceExternalID: "card-scope-eg", RawText: "未来一周美股承压", SubjectText: "美股", ChangeText: "承压", Kind: graphmodel.NodeKindPrediction, GraphRole: graphmodel.GraphRoleTarget, IsPrimary: true, VerificationStatus: graphmodel.VerificationProved, TimeBucket: "1w"}}}
		if err := store.PersistMemoryContentGraph(context.Background(), "u-event-card-scope", sg, time.Now().UTC()); err != nil {
			return nil, err
		}
		return store, nil
	}
	var stdout, stderr bytes.Buffer
	code := run([]string{"memory", "event-graphs", "--card", "--user", "u-event-card-scope", "--scope", "driver"}, "/tmp/project", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("event-graphs --card --scope code = %d, stderr = %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "Scope: driver") || strings.Contains(stdout.String(), "Scope: target") {
		t.Fatalf("stdout = %q, want only driver card output", stdout.String())
	}
}

func TestRunVerifyQueueSummaryPrintsCounts(t *testing.T) {
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
		now := time.Now().UTC()
		if err := store.EnqueueVerifyQueueItem(context.Background(), graphmodel.VerifyQueueItem{ID: "q-summary-1", ObjectType: graphmodel.VerifyQueueObjectNode, ObjectID: "n1", SourceArticleID: "unit-cli", Priority: 10, ScheduledAt: now.Format(time.RFC3339), Status: graphmodel.VerifyQueueStatusQueued}); err != nil {
			return nil, err
		}
		if err := store.EnqueueVerifyQueueItem(context.Background(), graphmodel.VerifyQueueItem{ID: "q-summary-2", ObjectType: graphmodel.VerifyQueueObjectEdge, ObjectID: "e1", SourceArticleID: "unit-cli", Priority: 5, ScheduledAt: now.Format(time.RFC3339), Status: graphmodel.VerifyQueueStatusQueued}); err != nil {
			return nil, err
		}
		if err := store.MarkVerifyQueueItemRunning(context.Background(), "q-summary-2", now); err != nil {
			return nil, err
		}
		return store, nil
	}
	var stdout, stderr bytes.Buffer
	code := run([]string{"verify", "queue", "--summary"}, "/tmp/project", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("verify queue --summary code = %d, stderr = %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "queued") || !strings.Contains(stdout.String(), "running") || !strings.Contains(stdout.String(), "due_count") || !strings.Contains(stdout.String(), "object_types") {
		t.Fatalf("stdout = %q, want queued/running summary with due_count and object_types", stdout.String())
	}
	if !strings.Contains(stdout.String(), "total_count") {
		t.Fatalf("stdout = %q, want total_count in summary", stdout.String())
	}
	if !strings.Contains(stdout.String(), "pending_age_buckets") {
		t.Fatalf("stdout = %q, want pending_age_buckets in summary", stdout.String())
	}
}

func TestRunVerifyQueueSummaryIncludesEmptyPendingAgeBucketsWhenQueueIsEmpty(t *testing.T) {
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
		return contentstore.NewSQLiteStore(path)
	}
	var stdout, stderr bytes.Buffer
	code := run([]string{"verify", "queue", "--summary"}, "/tmp/project", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("verify queue --summary code = %d, stderr = %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "pending_age_buckets") {
		t.Fatalf("stdout = %q, want empty pending_age_buckets object", stdout.String())
	}
}

func TestRunMemoryEventGraphsCardShowsNoMatchMessageForEmptyFilter(t *testing.T) {
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
		sg := graphmodel.ContentSubgraph{ID: "empty-eg", ArticleID: "empty-eg", SourcePlatform: "twitter", SourceExternalID: "empty-eg", CompileVersion: graphmodel.CompileBridgeVersion, CompiledAt: time.Now().UTC().Format(time.RFC3339), UpdatedAt: time.Now().UTC().Format(time.RFC3339), Nodes: []graphmodel.GraphNode{{ID: "n1", SourceArticleID: "empty-eg", SourcePlatform: "twitter", SourceExternalID: "empty-eg", RawText: "美联储加息0.25%", SubjectText: "美联储", ChangeText: "加息0.25%", Kind: graphmodel.NodeKindObservation, GraphRole: graphmodel.GraphRoleDriver, IsPrimary: true, VerificationStatus: graphmodel.VerificationPending, TimeBucket: "1w"}, {ID: "n2", SourceArticleID: "empty-eg", SourcePlatform: "twitter", SourceExternalID: "empty-eg", RawText: "未来一周美股承压", SubjectText: "美股", ChangeText: "承压", Kind: graphmodel.NodeKindPrediction, GraphRole: graphmodel.GraphRoleTarget, IsPrimary: true, VerificationStatus: graphmodel.VerificationProved, TimeBucket: "1w"}}}
		if err := store.PersistMemoryContentGraph(context.Background(), "u-empty-eg", sg, time.Now().UTC()); err != nil {
			return nil, err
		}
		return store, nil
	}
	var stdout, stderr bytes.Buffer
	code := run([]string{"memory", "event-graphs", "--card", "--user", "u-empty-eg", "--subject", "不存在"}, "/tmp/project", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("event-graphs empty card code = %d, stderr = %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "No event graphs matched") {
		t.Fatalf("stdout = %q, want no-match message", stdout.String())
	}
}

func TestRunMemoryParadigmsCardShowsNoMatchMessageForEmptyFilter(t *testing.T) {
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
		sg := graphmodel.ContentSubgraph{ID: "empty-pg", ArticleID: "empty-pg", SourcePlatform: "twitter", SourceExternalID: "empty-pg", CompileVersion: graphmodel.CompileBridgeVersion, CompiledAt: time.Now().UTC().Format(time.RFC3339), UpdatedAt: time.Now().UTC().Format(time.RFC3339), Nodes: []graphmodel.GraphNode{{ID: "n1", SourceArticleID: "empty-pg", SourcePlatform: "twitter", SourceExternalID: "empty-pg", RawText: "美联储加息0.25%", SubjectText: "美联储", ChangeText: "加息0.25%", Kind: graphmodel.NodeKindObservation, GraphRole: graphmodel.GraphRoleDriver, IsPrimary: true, VerificationStatus: graphmodel.VerificationPending, TimeBucket: "1w"}, {ID: "n2", SourceArticleID: "empty-pg", SourcePlatform: "twitter", SourceExternalID: "empty-pg", RawText: "未来一周美股承压", SubjectText: "美股", ChangeText: "承压", Kind: graphmodel.NodeKindPrediction, GraphRole: graphmodel.GraphRoleTarget, IsPrimary: true, VerificationStatus: graphmodel.VerificationProved, TimeBucket: "1w"}}}
		if err := store.PersistMemoryContentGraph(context.Background(), "u-empty-pg", sg, time.Now().UTC()); err != nil {
			return nil, err
		}
		return store, nil
	}
	var stdout, stderr bytes.Buffer
	code := run([]string{"memory", "paradigms", "--card", "--user", "u-empty-pg", "--subject", "不存在"}, "/tmp/project", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("paradigms empty card code = %d, stderr = %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "No paradigms matched") {
		t.Fatalf("stdout = %q, want no-match message", stdout.String())
	}
}

func TestRunMemoryContentGraphsCardShowsNoMatchMessageForEmptyFilter(t *testing.T) {
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
		now := time.Now().UTC()
		sg := graphmodel.ContentSubgraph{ID: "empty-cg-card", ArticleID: "empty-cg-card", SourcePlatform: "twitter", SourceExternalID: "empty-cg-card", CompileVersion: graphmodel.CompileBridgeVersion, CompiledAt: now.Format(time.RFC3339), UpdatedAt: now.Format(time.RFC3339), Nodes: []graphmodel.GraphNode{{ID: "n1", SourceArticleID: "empty-cg-card", SourcePlatform: "twitter", SourceExternalID: "empty-cg-card", RawText: "美联储加息0.25%", SubjectText: "美联储", ChangeText: "加息0.25%", Kind: graphmodel.NodeKindObservation, IsPrimary: true, VerificationStatus: graphmodel.VerificationPending}}}
		if err := store.PersistMemoryContentGraph(context.Background(), "u-empty-cg-card", sg, now); err != nil {
			return nil, err
		}
		return store, nil
	}
	var stdout, stderr bytes.Buffer
	code := run([]string{"memory", "content-graphs", "--card", "--user", "u-empty-cg-card", "--subject", "不存在"}, "/tmp/project", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("content-graphs empty card code = %d, stderr = %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "No content graphs matched") {
		t.Fatalf("stdout = %q, want no-match message", stdout.String())
	}
}

func TestRunMemoryEventEvidencePrintsPersistedLinks(t *testing.T) {
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
		sg := graphmodel.ContentSubgraph{ID: "ev-cli", ArticleID: "ev-cli", SourcePlatform: "twitter", SourceExternalID: "ev-cli", CompileVersion: graphmodel.CompileBridgeVersion, CompiledAt: time.Now().UTC().Format(time.RFC3339), UpdatedAt: time.Now().UTC().Format(time.RFC3339), Nodes: []graphmodel.GraphNode{{ID: "n1", SourceArticleID: "ev-cli", SourcePlatform: "twitter", SourceExternalID: "ev-cli", RawText: "美联储加息0.25%", SubjectText: "美联储", ChangeText: "加息0.25%", Kind: graphmodel.NodeKindObservation, GraphRole: graphmodel.GraphRoleDriver, IsPrimary: true, VerificationStatus: graphmodel.VerificationPending, TimeBucket: "1w"}, {ID: "n2", SourceArticleID: "ev-cli", SourcePlatform: "twitter", SourceExternalID: "ev-cli", RawText: "未来一周美股承压", SubjectText: "美股", ChangeText: "承压", Kind: graphmodel.NodeKindPrediction, GraphRole: graphmodel.GraphRoleTarget, IsPrimary: true, VerificationStatus: graphmodel.VerificationProved, TimeBucket: "1w"}}}
		if err := store.PersistMemoryContentGraph(context.Background(), "u-ev-cli", sg, time.Now().UTC()); err != nil {
			return nil, err
		}
		graphs, err := store.ListEventGraphs(context.Background(), "u-ev-cli")
		if err != nil {
			return nil, err
		}
		if len(graphs) == 0 {
			return nil, nil
		}
		return store, nil
	}
	var stdout, stderr bytes.Buffer
	code := run([]string{"memory", "event-evidence", "--user", "u-ev-cli"}, "/tmp/project", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("event-evidence code = %d, stderr = %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "event_graph_id") {
		t.Fatalf("stdout = %q, want event evidence payload", stdout.String())
	}
}

func TestRunMemoryParadigmEvidencePrintsPersistedLinks(t *testing.T) {
	prevBuildApp := buildApp
	prevOpenSQLiteStore := openSQLiteStore
	t.Cleanup(func() { buildApp = prevBuildApp; openSQLiteStore = prevOpenSQLiteStore })
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
		sg := graphmodel.ContentSubgraph{ID: "pev-cli", ArticleID: "pev-cli", SourcePlatform: "twitter", SourceExternalID: "pev-cli", CompileVersion: graphmodel.CompileBridgeVersion, CompiledAt: time.Now().UTC().Format(time.RFC3339), UpdatedAt: time.Now().UTC().Format(time.RFC3339), Nodes: []graphmodel.GraphNode{{ID: "n1", SourceArticleID: "pev-cli", SourcePlatform: "twitter", SourceExternalID: "pev-cli", RawText: "美联储加息0.25%", SubjectText: "美联储", ChangeText: "加息0.25%", Kind: graphmodel.NodeKindObservation, GraphRole: graphmodel.GraphRoleDriver, IsPrimary: true, VerificationStatus: graphmodel.VerificationPending, TimeBucket: "1w"}, {ID: "n2", SourceArticleID: "pev-cli", SourcePlatform: "twitter", SourceExternalID: "pev-cli", RawText: "未来一周美股承压", SubjectText: "美股", ChangeText: "承压", Kind: graphmodel.NodeKindPrediction, GraphRole: graphmodel.GraphRoleTarget, IsPrimary: true, VerificationStatus: graphmodel.VerificationProved, TimeBucket: "1w"}}}
		if err := store.PersistMemoryContentGraph(context.Background(), "u-pev-cli", sg, time.Now().UTC()); err != nil {
			return nil, err
		}
		return store, nil
	}
	var stdout, stderr bytes.Buffer
	code := run([]string{"memory", "paradigm-evidence", "--user", "u-pev-cli"}, "/tmp/project", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("paradigm-evidence code = %d, stderr = %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "paradigm_id") {
		t.Fatalf("stdout = %q, want paradigm evidence payload", stdout.String())
	}
}

func TestRunMemoryEventEvidenceSupportsEventGraphIDFilter(t *testing.T) {
	prevBuildApp := buildApp
	prevOpenSQLiteStore := openSQLiteStore
	t.Cleanup(func() { buildApp = prevBuildApp; openSQLiteStore = prevOpenSQLiteStore })
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
		now := time.Now().UTC()
		for _, sg := range []graphmodel.ContentSubgraph{
			{ID: "ee-filter-1", ArticleID: "ee-filter-1", SourcePlatform: "twitter", SourceExternalID: "ee-filter-1", CompileVersion: graphmodel.CompileBridgeVersion, CompiledAt: now.Format(time.RFC3339), UpdatedAt: now.Format(time.RFC3339), Nodes: []graphmodel.GraphNode{{ID: "n1", SourceArticleID: "ee-filter-1", SourcePlatform: "twitter", SourceExternalID: "ee-filter-1", RawText: "美联储加息0.25%", SubjectText: "美联储", ChangeText: "加息0.25%", Kind: graphmodel.NodeKindObservation, GraphRole: graphmodel.GraphRoleDriver, IsPrimary: true, VerificationStatus: graphmodel.VerificationPending, TimeBucket: "1w"}, {ID: "n2", SourceArticleID: "ee-filter-1", SourcePlatform: "twitter", SourceExternalID: "ee-filter-1", RawText: "未来一周美股承压", SubjectText: "美股", ChangeText: "承压", Kind: graphmodel.NodeKindPrediction, GraphRole: graphmodel.GraphRoleTarget, IsPrimary: true, VerificationStatus: graphmodel.VerificationProved, TimeBucket: "1w"}}},
			{ID: "ee-filter-2", ArticleID: "ee-filter-2", SourcePlatform: "twitter", SourceExternalID: "ee-filter-2", CompileVersion: graphmodel.CompileBridgeVersion, CompiledAt: now.Format(time.RFC3339), UpdatedAt: now.Format(time.RFC3339), Nodes: []graphmodel.GraphNode{{ID: "n1", SourceArticleID: "ee-filter-2", SourcePlatform: "twitter", SourceExternalID: "ee-filter-2", RawText: "欧洲央行放缓缩表", SubjectText: "欧洲央行", ChangeText: "放缓缩表", Kind: graphmodel.NodeKindObservation, GraphRole: graphmodel.GraphRoleDriver, IsPrimary: true, VerificationStatus: graphmodel.VerificationPending, TimeBucket: "1w"}, {ID: "n2", SourceArticleID: "ee-filter-2", SourcePlatform: "twitter", SourceExternalID: "ee-filter-2", RawText: "欧股承压", SubjectText: "欧股", ChangeText: "承压", Kind: graphmodel.NodeKindPrediction, GraphRole: graphmodel.GraphRoleTarget, IsPrimary: true, VerificationStatus: graphmodel.VerificationProved, TimeBucket: "1w"}}},
		} {
			if err := store.PersistMemoryContentGraph(context.Background(), "u-ee-filter", sg, now); err != nil {
				return nil, err
			}
		}
		return store, nil
	}
	var stdout, stderr bytes.Buffer
	code := run([]string{"memory", "event-evidence", "--user", "u-ee-filter", "--event-graph-id", "u-ee-filter:driver:美联储:1w"}, "/tmp/project", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("event-evidence filter code = %d, stderr = %s", code, stderr.String())
	}
	var out []contentstore.EventGraphEvidenceLink
	if err := json.Unmarshal(stdout.Bytes(), &out); err != nil {
		t.Fatalf("json.Unmarshal(event-evidence filter) error = %v", err)
	}
	for _, item := range out {
		if item.EventGraphID != "u-ee-filter:driver:美联储:1w" {
			t.Fatalf("filtered links = %#v, want only target event graph id", out)
		}
	}
}

func TestRunMemoryParadigmEvidenceSupportsParadigmIDFilter(t *testing.T) {
	prevBuildApp := buildApp
	prevOpenSQLiteStore := openSQLiteStore
	t.Cleanup(func() { buildApp = prevBuildApp; openSQLiteStore = prevOpenSQLiteStore })
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
		now := time.Now().UTC()
		for _, sg := range []graphmodel.ContentSubgraph{
			{ID: "pe-filter-1", ArticleID: "pe-filter-1", SourcePlatform: "twitter", SourceExternalID: "pe-filter-1", CompileVersion: graphmodel.CompileBridgeVersion, CompiledAt: now.Format(time.RFC3339), UpdatedAt: now.Format(time.RFC3339), Nodes: []graphmodel.GraphNode{{ID: "n1", SourceArticleID: "pe-filter-1", SourcePlatform: "twitter", SourceExternalID: "pe-filter-1", RawText: "美联储加息0.25%", SubjectText: "美联储", ChangeText: "加息0.25%", Kind: graphmodel.NodeKindObservation, GraphRole: graphmodel.GraphRoleDriver, IsPrimary: true, VerificationStatus: graphmodel.VerificationPending, TimeBucket: "1w"}, {ID: "n2", SourceArticleID: "pe-filter-1", SourcePlatform: "twitter", SourceExternalID: "pe-filter-1", RawText: "未来一周美股承压", SubjectText: "美股", ChangeText: "承压", Kind: graphmodel.NodeKindPrediction, GraphRole: graphmodel.GraphRoleTarget, IsPrimary: true, VerificationStatus: graphmodel.VerificationProved, TimeBucket: "1w"}}},
			{ID: "pe-filter-2", ArticleID: "pe-filter-2", SourcePlatform: "twitter", SourceExternalID: "pe-filter-2", CompileVersion: graphmodel.CompileBridgeVersion, CompiledAt: now.Format(time.RFC3339), UpdatedAt: now.Format(time.RFC3339), Nodes: []graphmodel.GraphNode{{ID: "n1", SourceArticleID: "pe-filter-2", SourcePlatform: "twitter", SourceExternalID: "pe-filter-2", RawText: "欧洲央行放缓缩表", SubjectText: "欧洲央行", ChangeText: "放缓缩表", Kind: graphmodel.NodeKindObservation, GraphRole: graphmodel.GraphRoleDriver, IsPrimary: true, VerificationStatus: graphmodel.VerificationPending, TimeBucket: "1w"}, {ID: "n2", SourceArticleID: "pe-filter-2", SourcePlatform: "twitter", SourceExternalID: "pe-filter-2", RawText: "欧股承压", SubjectText: "欧股", ChangeText: "承压", Kind: graphmodel.NodeKindPrediction, GraphRole: graphmodel.GraphRoleTarget, IsPrimary: true, VerificationStatus: graphmodel.VerificationProved, TimeBucket: "1w"}}},
		} {
			if err := store.PersistMemoryContentGraph(context.Background(), "u-pe-filter", sg, now); err != nil {
				return nil, err
			}
		}
		return store, nil
	}
	var stdout, stderr bytes.Buffer
	code := run([]string{"memory", "paradigm-evidence", "--user", "u-pe-filter", "--paradigm-id", "u-pe-filter:美联储:美股:1w"}, "/tmp/project", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("paradigm-evidence filter code = %d, stderr = %s", code, stderr.String())
	}
	var out []contentstore.ParadigmEvidenceLink
	if err := json.Unmarshal(stdout.Bytes(), &out); err != nil {
		t.Fatalf("json.Unmarshal(paradigm-evidence filter) error = %v", err)
	}
	for _, item := range out {
		if item.ParadigmID != "u-pe-filter:美联储:美股:1w" {
			t.Fatalf("filtered links = %#v, want only target paradigm id", out)
		}
	}
}

func TestRunMemoryEventEvidenceShowsNoMatchMessageForUnknownID(t *testing.T) {
	prevBuildApp := buildApp
	prevOpenSQLiteStore := openSQLiteStore
	t.Cleanup(func() { buildApp = prevBuildApp; openSQLiteStore = prevOpenSQLiteStore })
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
		now := time.Now().UTC()
		sg := graphmodel.ContentSubgraph{ID: "empty-ee", ArticleID: "empty-ee", SourcePlatform: "twitter", SourceExternalID: "empty-ee", CompileVersion: graphmodel.CompileBridgeVersion, CompiledAt: now.Format(time.RFC3339), UpdatedAt: now.Format(time.RFC3339), Nodes: []graphmodel.GraphNode{{ID: "n1", SourceArticleID: "empty-ee", SourcePlatform: "twitter", SourceExternalID: "empty-ee", RawText: "美联储加息0.25%", SubjectText: "美联储", ChangeText: "加息0.25%", Kind: graphmodel.NodeKindObservation, GraphRole: graphmodel.GraphRoleDriver, IsPrimary: true, VerificationStatus: graphmodel.VerificationPending, TimeBucket: "1w"}, {ID: "n2", SourceArticleID: "empty-ee", SourcePlatform: "twitter", SourceExternalID: "empty-ee", RawText: "未来一周美股承压", SubjectText: "美股", ChangeText: "承压", Kind: graphmodel.NodeKindPrediction, GraphRole: graphmodel.GraphRoleTarget, IsPrimary: true, VerificationStatus: graphmodel.VerificationProved, TimeBucket: "1w"}}}
		if err := store.PersistMemoryContentGraph(context.Background(), "u-empty-ee", sg, now); err != nil {
			return nil, err
		}
		return store, nil
	}
	var stdout, stderr bytes.Buffer
	code := run([]string{"memory", "event-evidence", "--user", "u-empty-ee", "--event-graph-id", "missing"}, "/tmp/project", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("event-evidence no-match code = %d, stderr = %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "No event evidence matched") {
		t.Fatalf("stdout = %q, want no-match message", stdout.String())
	}
}

func TestRunMemoryParadigmEvidenceShowsNoMatchMessageForUnknownID(t *testing.T) {
	prevBuildApp := buildApp
	prevOpenSQLiteStore := openSQLiteStore
	t.Cleanup(func() { buildApp = prevBuildApp; openSQLiteStore = prevOpenSQLiteStore })
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
		now := time.Now().UTC()
		sg := graphmodel.ContentSubgraph{ID: "empty-pe", ArticleID: "empty-pe", SourcePlatform: "twitter", SourceExternalID: "empty-pe", CompileVersion: graphmodel.CompileBridgeVersion, CompiledAt: now.Format(time.RFC3339), UpdatedAt: now.Format(time.RFC3339), Nodes: []graphmodel.GraphNode{{ID: "n1", SourceArticleID: "empty-pe", SourcePlatform: "twitter", SourceExternalID: "empty-pe", RawText: "美联储加息0.25%", SubjectText: "美联储", ChangeText: "加息0.25%", Kind: graphmodel.NodeKindObservation, GraphRole: graphmodel.GraphRoleDriver, IsPrimary: true, VerificationStatus: graphmodel.VerificationPending, TimeBucket: "1w"}, {ID: "n2", SourceArticleID: "empty-pe", SourcePlatform: "twitter", SourceExternalID: "empty-pe", RawText: "未来一周美股承压", SubjectText: "美股", ChangeText: "承压", Kind: graphmodel.NodeKindPrediction, GraphRole: graphmodel.GraphRoleTarget, IsPrimary: true, VerificationStatus: graphmodel.VerificationProved, TimeBucket: "1w"}}}
		if err := store.PersistMemoryContentGraph(context.Background(), "u-empty-pe", sg, now); err != nil {
			return nil, err
		}
		return store, nil
	}
	var stdout, stderr bytes.Buffer
	code := run([]string{"memory", "paradigm-evidence", "--user", "u-empty-pe", "--paradigm-id", "missing"}, "/tmp/project", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("paradigm-evidence no-match code = %d, stderr = %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "No paradigm evidence matched") {
		t.Fatalf("stdout = %q, want no-match message", stdout.String())
	}
}

func TestRunVerifyShowFallsBackToGraphFirstVerificationState(t *testing.T) {
	prevBuildApp := buildApp
	prevOpenSQLiteStore := openSQLiteStore
	t.Cleanup(func() { buildApp = prevBuildApp; openSQLiteStore = prevOpenSQLiteStore })

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
		subgraph := graphmodel.ContentSubgraph{ID: "verify-show-fallback", ArticleID: "verify-show-fallback", SourcePlatform: "twitter", SourceExternalID: "verify-show-fallback", CompileVersion: graphmodel.CompileBridgeVersion, CompiledAt: time.Now().UTC().Format(time.RFC3339), UpdatedAt: time.Now().UTC().Format(time.RFC3339), Nodes: []graphmodel.GraphNode{{ID: "n1", SourceArticleID: "verify-show-fallback", SourcePlatform: "twitter", SourceExternalID: "verify-show-fallback", RawText: "事实A", SubjectText: "事实A", ChangeText: "事实A", Kind: graphmodel.NodeKindObservation, IsPrimary: true, VerificationStatus: graphmodel.VerificationProved, VerificationReason: "supported", VerificationAsOf: time.Now().UTC().Format(time.RFC3339)}, {ID: "n2", SourceArticleID: "verify-show-fallback", SourcePlatform: "twitter", SourceExternalID: "verify-show-fallback", RawText: "未来一周结论B", SubjectText: "结论B", ChangeText: "未来一周结论B", Kind: graphmodel.NodeKindPrediction, IsPrimary: true, VerificationStatus: graphmodel.VerificationPending, VerificationReason: "waiting", VerificationAsOf: time.Now().UTC().Format(time.RFC3339), NextVerifyAt: time.Now().UTC().Add(time.Hour).Format(time.RFC3339)}}}
		if err := store.UpsertContentSubgraph(context.Background(), subgraph); err != nil {
			return nil, err
		}
		return store, nil
	}
	var stdout, stderr bytes.Buffer
	code := run([]string{"verify", "show", "--platform", "twitter", "--id", "verify-show-fallback"}, "/tmp/project", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("verify show fallback code = %d, stderr = %s", code, stderr.String())
	}
	var shown c.VerificationRecord
	if err := json.Unmarshal(stdout.Bytes(), &shown); err != nil {
		t.Fatalf("json.Unmarshal(verify show fallback) error = %v", err)
	}
	if len(shown.Verification.FactChecks) != 1 || shown.Verification.FactChecks[0].Reason != "supported" {
		t.Fatalf("fallback shown verification = %#v", shown)
	}
	if len(shown.Verification.PredictionChecks) != 1 || shown.Verification.PredictionChecks[0].Reason != "waiting" {
		t.Fatalf("fallback prediction checks = %#v", shown)
	}
}

type compileClientStub struct {
	compile        func(context.Context, c.Bundle) (c.Record, error)
	verify         func(context.Context, c.Bundle, c.Output) (c.Verification, error)
	verifyDetailed func(context.Context, c.Bundle, c.Output) (c.Verification, error)
}

func (s compileClientStub) Compile(ctx context.Context, bundle c.Bundle) (c.Record, error) {
	if s.compile != nil {
		return s.compile(ctx, bundle)
	}
	return c.Record{}, nil
}
func (s compileClientStub) Verify(ctx context.Context, bundle c.Bundle, output c.Output) (c.Verification, error) {
	if s.verify != nil {
		return s.verify(ctx, bundle, output)
	}
	return c.Verification{}, nil
}
func (s compileClientStub) VerifyDetailed(ctx context.Context, bundle c.Bundle, output c.Output) (c.Verification, error) {
	if s.verifyDetailed != nil {
		return s.verifyDetailed(ctx, bundle, output)
	}
	return c.Verification{}, nil
}

func TestRunMemoryEventEvidenceCardPrintsReadableSections(t *testing.T) {
	prevBuildApp := buildApp
	prevOpenSQLiteStore := openSQLiteStore
	t.Cleanup(func() { buildApp = prevBuildApp; openSQLiteStore = prevOpenSQLiteStore })
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
		now := time.Now().UTC()
		sg := graphmodel.ContentSubgraph{ID: "event-evi-card", ArticleID: "event-evi-card", SourcePlatform: "twitter", SourceExternalID: "event-evi-card", CompileVersion: graphmodel.CompileBridgeVersion, CompiledAt: now.Format(time.RFC3339), UpdatedAt: now.Format(time.RFC3339), Nodes: []graphmodel.GraphNode{{ID: "n1", SourceArticleID: "event-evi-card", SourcePlatform: "twitter", SourceExternalID: "event-evi-card", RawText: "美联储加息0.25%", SubjectText: "美联储", ChangeText: "加息0.25%", Kind: graphmodel.NodeKindObservation, GraphRole: graphmodel.GraphRoleDriver, IsPrimary: true, VerificationStatus: graphmodel.VerificationPending, TimeBucket: "1w"}, {ID: "n2", SourceArticleID: "event-evi-card", SourcePlatform: "twitter", SourceExternalID: "event-evi-card", RawText: "未来一周美股承压", SubjectText: "美股", ChangeText: "承压", Kind: graphmodel.NodeKindPrediction, GraphRole: graphmodel.GraphRoleTarget, IsPrimary: true, VerificationStatus: graphmodel.VerificationProved, TimeBucket: "1w"}}}
		if err := store.PersistMemoryContentGraph(context.Background(), "u-event-evi-card", sg, now); err != nil {
			return nil, err
		}
		return store, nil
	}
	var stdout, stderr bytes.Buffer
	code := run([]string{"memory", "event-evidence", "--card", "--user", "u-event-evi-card"}, "/tmp/project", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("event-evidence --card code = %d, stderr = %s", code, stderr.String())
	}
	for _, want := range []string{"Event Evidence", "event_graph_id", "subgraph_id"} {
		if !strings.Contains(stdout.String(), want) {
			t.Fatalf("stdout = %q, want substring %q", stdout.String(), want)
		}
	}
}

func TestRunMemoryParadigmEvidenceCardPrintsReadableSections(t *testing.T) {
	prevBuildApp := buildApp
	prevOpenSQLiteStore := openSQLiteStore
	t.Cleanup(func() { buildApp = prevBuildApp; openSQLiteStore = prevOpenSQLiteStore })
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
		now := time.Now().UTC()
		sg := graphmodel.ContentSubgraph{ID: "paradigm-evi-card", ArticleID: "paradigm-evi-card", SourcePlatform: "twitter", SourceExternalID: "paradigm-evi-card", CompileVersion: graphmodel.CompileBridgeVersion, CompiledAt: now.Format(time.RFC3339), UpdatedAt: now.Format(time.RFC3339), Nodes: []graphmodel.GraphNode{{ID: "n1", SourceArticleID: "paradigm-evi-card", SourcePlatform: "twitter", SourceExternalID: "paradigm-evi-card", RawText: "美联储加息0.25%", SubjectText: "美联储", ChangeText: "加息0.25%", Kind: graphmodel.NodeKindObservation, GraphRole: graphmodel.GraphRoleDriver, IsPrimary: true, VerificationStatus: graphmodel.VerificationPending, TimeBucket: "1w"}, {ID: "n2", SourceArticleID: "paradigm-evi-card", SourcePlatform: "twitter", SourceExternalID: "paradigm-evi-card", RawText: "未来一周美股承压", SubjectText: "美股", ChangeText: "承压", Kind: graphmodel.NodeKindPrediction, GraphRole: graphmodel.GraphRoleTarget, IsPrimary: true, VerificationStatus: graphmodel.VerificationProved, TimeBucket: "1w"}}}
		if err := store.PersistMemoryContentGraph(context.Background(), "u-paradigm-evi-card", sg, now); err != nil {
			return nil, err
		}
		return store, nil
	}
	var stdout, stderr bytes.Buffer
	code := run([]string{"memory", "paradigm-evidence", "--card", "--user", "u-paradigm-evi-card"}, "/tmp/project", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("paradigm-evidence --card code = %d, stderr = %s", code, stderr.String())
	}
	for _, want := range []string{"Paradigm Evidence", "paradigm_id", "subgraph_id"} {
		if !strings.Contains(stdout.String(), want) {
			t.Fatalf("stdout = %q, want substring %q", stdout.String(), want)
		}
	}
}

func writeTestJSONFile(t *testing.T, path string, value any) {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		t.Fatalf("os.WriteFile(%q) error = %v", path, err)
	}
}
