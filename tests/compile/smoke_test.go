//go:build compile_manual

package compile

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/kumaloha/VariX/varix/config"
)

func TestCompileSmokeOnStoredSampleWhenEnabled(t *testing.T) {
	if os.Getenv("RUN_COMPILE_SMOKE") == "" {
		t.Skip("set RUN_COMPILE_SMOKE=1 to run real-sample compile smoke test")
	}
	root := filepath.Clean("/Users/kuma/Projects/VariX")
	settings := config.DefaultSettings(root)
	raw, err := loadManualRawCapture(context.Background(), settings.ContentDBPath, "weibo", "QAJ0n0YGU")
	if err != nil {
		t.Fatalf("loadManualRawCapture() error = %v", err)
	}
	client := NewClientFromConfig(root, nil)
	if client == nil {
		t.Fatal("NewClientFromConfig() returned nil")
	}
	record, err := client.Compile(context.Background(), BuildBundle(raw))
	if err != nil {
		t.Fatalf("Compile() error = %v", err)
	}
	if len(record.Output.Drivers) == 0 || len(record.Output.Targets) == 0 {
		t.Fatalf("compile output too empty: %#v", record.Output)
	}
}
