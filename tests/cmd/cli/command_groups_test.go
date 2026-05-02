package main

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/kumaloha/VariX/varix/ingest"
	"github.com/kumaloha/VariX/varix/ingest/dispatcher"
	"github.com/kumaloha/VariX/varix/ingest/types"
)

func TestUsageTextShowsDistinctCommandGroups(t *testing.T) {
	got := usageText()
	for _, want := range []string{
		"usage: varix <ingest|compile|verify|memory|serve>",
		"ingest: fetch|follow|list-authors|list-follows|poll|provenance-run",
		"compile: run|batch-run|show|summary|compare|card",
		"verify: run|show|queue",
		"memory: accept|accept-batch|list|show-source|content-graphs|subject-timeline|subject-horizon|subject-experience|jobs|posterior-run|organize-run|organized|global-organize-run|global-organized|global-synthesis-run|global-synthesis|global-card|global-synthesis-card|global-compare|event-graphs|event-evidence|paradigms|paradigm-evidence|project-all|backfill|cleanup-stale|canonical-entities|canonical-entity-upsert",
		"serve: --addr <host:port>",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("usageText() = %q, want substring %q", got, want)
		}
	}
	if strings.Contains(got, "usage: cli <") {
		t.Fatalf("usageText() = %q, should describe the varix CLI", got)
	}
}

func TestIsIngestCommandOnlyMatchesLegacyIngestAliases(t *testing.T) {
	for _, name := range []string{"fetch", "follow", "list-authors", "list-follows", "poll", "provenance-run"} {
		if !isIngestCommand(name) {
			t.Fatalf("isIngestCommand(%q) = false, want true", name)
		}
	}
	for _, name := range []string{"ingest", "compile", "memory", "run", "accept"} {
		if isIngestCommand(name) {
			t.Fatalf("isIngestCommand(%q) = true, want false", name)
		}
	}
}

func TestRunDirectFetchAliasStillRoutesToIngest(t *testing.T) {
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
	code := run([]string{"fetch", "https://example.com/post"}, "/tmp/project", &stdout, &stderr)
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
	if len(got) != 1 || got[0].ExternalID != "QAzzRES0G" {
		t.Fatalf("got = %#v", got)
	}
}
