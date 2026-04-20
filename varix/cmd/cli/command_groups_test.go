package main

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/kumaloha/VariX/varix/bootstrap"
	"github.com/kumaloha/VariX/varix/ingest/dispatcher"
	"github.com/kumaloha/VariX/varix/ingest/types"
)

func TestUsageTextShowsDistinctCommandGroups(t *testing.T) {
	got := usageText()
	for _, want := range []string{
		"usage: varix <ingest|compile|verify|memory>",
		"ingest: fetch|follow|list-follows|poll|provenance-run",
		"compile: run|show|summary|compare|card",
		"verify: run|show",
		"memory: accept|accept-batch|list|show-source|jobs|posterior-run|organize-run|organized|global-organize-run|global-organized|global-v2-organize-run|global-v2-organized|global-card|global-v2-card|global-compare",
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
	for _, name := range []string{"fetch", "follow", "list-follows", "poll", "provenance-run"} {
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
