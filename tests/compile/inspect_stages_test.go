//go:build compile_manual

package compile

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/kumaloha/VariX/varix/config"
)

func TestInspectG04Stages(t *testing.T) {
	if os.Getenv("RUN_COMPILE_INSPECT") == "" {
		t.Skip("set RUN_COMPILE_INSPECT=1 to run long stage inspection")
	}
	root := filepath.Clean("/Users/kuma/Projects/VariX")
	settings := config.DefaultSettings(root)
	raw, err := loadManualRawCapture(context.Background(), settings.ContentDBPath, "twitter", "2045106658200682788")
	if err != nil {
		t.Fatalf("loadManualRawCapture() error = %v", err)
	}
	bundle := BuildBundle(raw)
	client := NewClientFromConfig(root, nil)
	if client == nil {
		t.Fatal("NewClientFromConfig() returned nil")
	}

	ctx := context.Background()

	stage1, err := stage1Extract(ctx, client.runtime, client.model, bundle)
	if err != nil {
		t.Fatalf("stage1Extract() error = %v", err)
	}
	printJSON("STAGE1", stage1)

	stage3, err := stage3Classify(ctx, client.runtime, client.model, bundle, stage1)
	if err != nil {
		t.Fatalf("stage3Classify() error = %v", err)
	}
	printJSON("STAGE3", stage3)

	coverage, err := stageCoverage(ctx, client.runtime, client.model, bundle, stage3, 1)
	if err != nil {
		t.Fatalf("stageCoverage() error = %v", err)
	}
	printJSON("COVERAGE", coverage)

	stage5, err := stage5Render(ctx, client.runtime, client.model, bundle, coverage)
	if err != nil {
		t.Fatalf("stage5Render() error = %v", err)
	}
	printJSON("STAGE5", stage5)
}

func printJSON(label string, value any) {
	payload, _ := json.MarshalIndent(value, "", "  ")
	fmt.Printf("\n=== %s ===\n%s\n", label, string(payload))
}
