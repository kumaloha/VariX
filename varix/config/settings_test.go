package config

import (
	"path/filepath"
	"testing"
	"time"
)

func TestDefaultSettings(t *testing.T) {
	t.Setenv("INVARIX_STORE_BACKEND", "")
	t.Setenv("INVARIX_CONTENT_DB_PATH", "")
	root := "/tmp/invarix-root"
	got := DefaultSettings(root)

	if got.ProjectRoot != root {
		t.Fatalf("ProjectRoot = %q, want %q", got.ProjectRoot, root)
	}
	if got.ContentDBPath != filepath.Join(root, "data", "content.db") {
		t.Fatalf("ContentDBPath = %q", got.ContentDBPath)
	}
	if got.AssetsDir != filepath.Join(root, "data", "assets") {
		t.Fatalf("AssetsDir = %q", got.AssetsDir)
	}
	if got.StoreBackend != "sqlite" {
		t.Fatalf("StoreBackend = %q, want %q", got.StoreBackend, "sqlite")
	}
	if got.PollInterval != 15*time.Minute {
		t.Fatalf("PollInterval = %v, want 15m", got.PollInterval)
	}
}

func TestDefaultSettingsCapturesStoreEnvForValidation(t *testing.T) {
	root := t.TempDir()
	t.Setenv("INVARIX_STORE_BACKEND", "json")
	t.Setenv("INVARIX_CONTENT_DB_PATH", filepath.Join(root, "custom", "content.db"))

	got := DefaultSettings(root)

	if got.StoreBackend != "json" {
		t.Fatalf("StoreBackend = %q, want %q", got.StoreBackend, "json")
	}
	if got.ContentDBPath != filepath.Join(root, "custom", "content.db") {
		t.Fatalf("ContentDBPath = %q", got.ContentDBPath)
	}
}
