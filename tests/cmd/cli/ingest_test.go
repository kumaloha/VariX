package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"github.com/kumaloha/VariX/varix/ingest"
	"github.com/kumaloha/VariX/varix/ingest/dispatcher"
	"github.com/kumaloha/VariX/varix/ingest/polling"
	"github.com/kumaloha/VariX/varix/ingest/router"
	"github.com/kumaloha/VariX/varix/ingest/types"
	"github.com/kumaloha/VariX/varix/storage/contentstore"
	"path/filepath"
	"strings"
	"testing"
)

type fakeSearchDiscoverer struct{}

func (fakeSearchDiscoverer) Kind() types.Kind { return types.KindSearch }

func (fakeSearchDiscoverer) Platform() types.Platform { return types.PlatformTwitter }

func (fakeSearchDiscoverer) Discover(context.Context, types.FollowTarget) ([]types.DiscoveryItem, error) {
	return nil, nil
}

func TestRunIngestFetchWritesJSONToStdout(t *testing.T) {
	prevNewIngestRuntime := newIngestRuntime
	prevGetwd := getwd
	t.Cleanup(func() {
		newIngestRuntime = prevNewIngestRuntime
		getwd = prevGetwd
	})

	newIngestRuntime = func(projectRoot string) (*ingest.Runtime, error) {
		src := fakeItemSource{
			items: []types.RawContent{{
				Source:     "weibo",
				ExternalID: "QAzzRES0G",
				Content:    "hello",
				AuthorName: "Alice",
				URL:        "https://weibo.com/1182426800/QAzzRES0G",
			}},
		}
		return &ingest.Runtime{
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

func TestRunIngestFetchFollowAuthorSubscribesTwitterPostAuthor(t *testing.T) {
	prevNewIngestRuntime := newIngestRuntime
	prevGetwd := getwd
	t.Cleanup(func() {
		newIngestRuntime = prevNewIngestRuntime
		getwd = prevGetwd
	})

	newIngestRuntime = func(projectRoot string) (*ingest.Runtime, error) {
		store, err := contentstore.NewSQLiteStore(filepath.Join(t.TempDir(), "content.db"))
		if err != nil {
			return nil, err
		}
		t.Cleanup(func() { _ = store.Close() })
		src := fakeItemSource{
			platform: types.PlatformTwitter,
			items: []types.RawContent{{
				Source:     "twitter",
				ExternalID: "2049570595277300120",
				Content:    "tweet text",
				AuthorName: "Robin Brooks",
				AuthorID:   "robin_j_brooks",
				URL:        "https://x.com/robin_j_brooks/status/2049570595277300120?s=20",
			}},
		}
		dispatch := dispatcher.New(router.Parse, []dispatcher.ItemSource{src}, []dispatcher.Discoverer{fakeSearchDiscoverer{}}, nil)
		return &ingest.Runtime{
			Dispatcher: dispatch,
			Polling:    polling.New(store, dispatch, nil),
		}, nil
	}
	getwd = func() (string, error) { return "/tmp/project", nil }

	var stdout, stderr bytes.Buffer
	code := run([]string{"ingest", "fetch", "--follow-author", "https://x.com/robin_j_brooks/status/2049570595277300120?s=20"}, "/tmp/project", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("run() code = %d, stderr = %s", code, stderr.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q, want empty", stderr.String())
	}
	if !strings.Contains(stdout.String(), `"external_id": "2049570595277300120"`) {
		t.Fatalf("stdout = %s, want fetched tweet", stdout.String())
	}
	if !strings.Contains(stdout.String(), `"platform_id": "robin_j_brooks"`) {
		t.Fatalf("stdout = %s, want author subscription", stdout.String())
	}
	if !strings.Contains(stdout.String(), `site:x.com/robin_j_brooks/status`) {
		t.Fatalf("stdout = %s, want author search follow", stdout.String())
	}
}

func TestRunIngestFetchFollowAuthorPrintsPartialResultWhenFetchFails(t *testing.T) {
	prevNewIngestRuntime := newIngestRuntime
	prevGetwd := getwd
	t.Cleanup(func() {
		newIngestRuntime = prevNewIngestRuntime
		getwd = prevGetwd
	})

	newIngestRuntime = func(projectRoot string) (*ingest.Runtime, error) {
		store, err := contentstore.NewSQLiteStore(filepath.Join(t.TempDir(), "content.db"))
		if err != nil {
			return nil, err
		}
		t.Cleanup(func() { _ = store.Close() })
		src := fakeItemSource{
			platform: types.PlatformTwitter,
			fetchFn: func(context.Context, types.ParsedURL) ([]types.RawContent, error) {
				return nil, errors.New("temporary fetch failure")
			},
		}
		dispatch := dispatcher.New(router.Parse, []dispatcher.ItemSource{src}, []dispatcher.Discoverer{fakeSearchDiscoverer{}}, nil)
		return &ingest.Runtime{
			Dispatcher: dispatch,
			Polling:    polling.New(store, dispatch, nil),
		}, nil
	}
	getwd = func() (string, error) { return "/tmp/project", nil }

	var stdout, stderr bytes.Buffer
	code := run([]string{"ingest", "fetch", "--follow-author", "https://x.com/Robin_J_Brooks/status/2049570595277300120?s=20"}, "/tmp/project", &stdout, &stderr)
	if code != 1 {
		t.Fatalf("run() code = %d, want fetch failure exit; stderr=%s stdout=%s", code, stderr.String(), stdout.String())
	}
	if !strings.Contains(stderr.String(), "temporary fetch failure") {
		t.Fatalf("stderr = %q, want fetch error", stderr.String())
	}
	if !strings.Contains(stdout.String(), `"platform_id": "robin_j_brooks"`) {
		t.Fatalf("stdout = %s, want partial author subscription", stdout.String())
	}
	if !strings.Contains(stdout.String(), `"warning": "temporary fetch failure"`) {
		t.Fatalf("stdout = %s, want warning", stdout.String())
	}
}

func TestRunIngestListFollowsShowsSchedule(t *testing.T) {
	prevNewIngestRuntime := newIngestRuntime
	prevGetwd := getwd
	t.Cleanup(func() {
		newIngestRuntime = prevNewIngestRuntime
		getwd = prevGetwd
	})

	newIngestRuntime = func(projectRoot string) (*ingest.Runtime, error) {
		store, err := contentstore.NewSQLiteStore(filepath.Join(t.TempDir(), "content.db"))
		if err != nil {
			return nil, err
		}
		t.Cleanup(func() { _ = store.Close() })
		dispatch := dispatcher.New(router.Parse, nil, []dispatcher.Discoverer{fakeSearchDiscoverer{}}, nil)
		svc := polling.New(store, dispatch, nil)
		_, err = svc.FollowSearch(context.Background(), types.PlatformTwitter, "site:x.com/robin_j_brooks/status")
		if err != nil {
			return nil, err
		}
		return &ingest.Runtime{
			Dispatcher: dispatch,
			Polling:    svc,
		}, nil
	}
	getwd = func() (string, error) { return "/tmp/project", nil }

	var stdout, stderr bytes.Buffer
	code := run([]string{"ingest", "list-follows"}, "/tmp/project", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("run() code = %d, stderr = %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "cadence=3h0m0s") {
		t.Fatalf("stdout = %q, want search cadence", stdout.String())
	}
	if !strings.Contains(stdout.String(), "next_poll_at=") {
		t.Fatalf("stdout = %q, want next_poll_at", stdout.String())
	}
	if !strings.Contains(stdout.String(), "slot=") {
		t.Fatalf("stdout = %q, want slot", stdout.String())
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
