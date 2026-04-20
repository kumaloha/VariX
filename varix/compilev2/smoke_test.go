package compilev2

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/kumaloha/VariX/varix/compile"
	"github.com/kumaloha/VariX/varix/config"
	"github.com/kumaloha/VariX/varix/storage/contentstore"
)

func TestCompileV2SmokeOnStoredSampleWhenEnabled(t *testing.T) {
	if os.Getenv("RUN_COMPILEV2_SMOKE") == "" {
		t.Skip("set RUN_COMPILEV2_SMOKE=1 to run real-sample v2 smoke test")
	}
	root := filepath.Clean("/Users/kuma/Projects/VariX")
	settings := config.DefaultSettings(root)
	store, err := contentstore.NewSQLiteStore(settings.ContentDBPath)
	if err != nil {
		t.Fatalf("NewSQLiteStore() error = %v", err)
	}
	defer store.Close()

	raw, err := store.GetRawCapture(context.Background(), "weibo", "QAJ0n0YGU")
	if err != nil {
		t.Fatalf("GetRawCapture() error = %v", err)
	}
	client := NewClientFromConfig(root, nil)
	if client == nil {
		t.Fatal("NewClientFromConfig() returned nil")
	}
	record, err := client.Compile(context.Background(), compile.BuildBundle(raw))
	if err != nil {
		t.Fatalf("Compile() error = %v", err)
	}
	if len(record.Output.Drivers) == 0 || len(record.Output.Targets) == 0 {
		t.Fatalf("v2 output too empty: %#v", record.Output)
	}
}
