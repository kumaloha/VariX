package ingest

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kumaloha/VariX/varix/ingest/types"
)

func TestNewRuntime_WiresSupportedFollowStrategies(t *testing.T) {
	t.Setenv("INVARIX_STORE_BACKEND", "")
	t.Setenv("INVARIX_CONTENT_DB_PATH", "")
	root := t.TempDir()

	app, err := NewRuntime(root)
	if err != nil {
		t.Fatalf("NewRuntime() error = %v", err)
	}

	for _, tc := range []struct {
		kind     types.Kind
		platform types.Platform
		want     bool
	}{
		{types.KindRSS, types.PlatformRSS, true},
		{types.KindSearch, types.PlatformTwitter, true},
		{types.KindSearch, types.PlatformWeibo, true},
		{types.KindSearch, types.PlatformYouTube, true},
		{types.KindSearch, types.PlatformBilibili, true},
		{types.KindSearch, types.PlatformWeb, true},
		{types.KindNative, types.PlatformWeibo, true},
		{types.KindNative, types.PlatformTwitter, false},
		{types.KindNative, types.PlatformYouTube, false},
		{types.KindNative, types.PlatformBilibili, false},
	} {
		got := app.Dispatcher.SupportsFollow(tc.kind, tc.platform)
		if got != tc.want {
			t.Fatalf("SupportsFollow(%s, %s) = %v, want %v", tc.kind, tc.platform, got, tc.want)
		}
	}
}

func TestNewRuntime_DefaultsToSQLiteStore(t *testing.T) {
	t.Setenv("INVARIX_STORE_BACKEND", "")
	t.Setenv("INVARIX_CONTENT_DB_PATH", "")
	root := t.TempDir()

	_, err := NewRuntime(root)
	if err != nil {
		t.Fatalf("NewRuntime() error = %v", err)
	}

	if _, err := os.Stat(filepath.Join(root, "data", "content.db")); err != nil {
		t.Fatalf("expected sqlite db file to exist, stat error = %v", err)
	}
}

func TestNewRuntime_WiresProvenanceService(t *testing.T) {
	t.Setenv("INVARIX_STORE_BACKEND", "")
	t.Setenv("INVARIX_CONTENT_DB_PATH", "")
	root := t.TempDir()

	app, err := NewRuntime(root)
	if err != nil {
		t.Fatalf("NewRuntime() error = %v", err)
	}
	if app.Provenance == nil {
		t.Fatal("Provenance service is nil")
	}
}

func TestNewRuntime_IgnoresDeprecatedProvenanceJudgeEnv(t *testing.T) {
	t.Setenv("INVARIX_STORE_BACKEND", "")
	t.Setenv("INVARIX_CONTENT_DB_PATH", "")
	t.Setenv("VARIX_PROVENANCE_JUDGE", "llm")
	t.Setenv("DASHSCOPE_API_KEY", "")
	root := t.TempDir()

	app, err := NewRuntime(root)
	if err != nil {
		t.Fatalf("NewRuntime() error = %v", err)
	}
	if app.Provenance == nil {
		t.Fatal("Provenance service is nil")
	}
}

func TestNewRuntime_RejectsDeprecatedJSONStoreBackend(t *testing.T) {
	t.Setenv("INVARIX_STORE_BACKEND", "json")
	t.Setenv("INVARIX_CONTENT_DB_PATH", "")
	root := t.TempDir()

	_, err := NewRuntime(root)
	if err == nil {
		t.Fatal("NewRuntime() error = nil, want deprecated json backend error")
	}
	if !strings.Contains(err.Error(), "json store backend has been removed") {
		t.Fatalf("NewRuntime() error = %q, want json backend removal guidance", err)
	}
}
