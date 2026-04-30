package main

import (
	"bytes"
	"github.com/kumaloha/VariX/varix/storage/contentstore"
	"strings"
	"testing"
)

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

func TestRunServeRequiresAddr(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{"serve", "--addr", ""}, "/tmp/project", &stdout, &stderr)
	if code != 2 {
		t.Fatalf("run() code = %d, want 2", code)
	}
	if !strings.Contains(stderr.String(), "usage: varix serve") {
		t.Fatalf("stderr = %q, want usage", stderr.String())
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

func TestParseCompileLLMCacheMode(t *testing.T) {
	tests := []struct {
		raw  string
		want contentstore.LLMCacheMode
	}{
		{"", contentstore.LLMCacheReadThrough},
		{"read-through", contentstore.LLMCacheReadThrough},
		{"refresh", contentstore.LLMCacheRefresh},
		{"off", contentstore.LLMCacheOff},
	}
	for _, tt := range tests {
		got, err := parseCompileLLMCacheMode(tt.raw)
		if err != nil {
			t.Fatalf("parseCompileLLMCacheMode(%q) error = %v", tt.raw, err)
		}
		if got != tt.want {
			t.Fatalf("parseCompileLLMCacheMode(%q) = %q, want %q", tt.raw, got, tt.want)
		}
	}
	if _, err := parseCompileLLMCacheMode("bogus"); err == nil {
		t.Fatal("parseCompileLLMCacheMode(bogus) error = nil")
	}
}
