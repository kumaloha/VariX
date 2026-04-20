package compilev2

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/kumaloha/VariX/varix/compile"
	"github.com/kumaloha/VariX/varix/config"
	"github.com/kumaloha/VariX/varix/storage/contentstore"
)

func TestInspectG04Stages(t *testing.T) {
	if os.Getenv("RUN_COMPILEV2_INSPECT") == "" {
		t.Skip("set RUN_COMPILEV2_INSPECT=1 to run long stage inspection")
	}
	root := filepath.Clean("/Users/kuma/Projects/VariX")
	settings := config.DefaultSettings(root)
	store, err := contentstore.NewSQLiteStore(settings.ContentDBPath)
	if err != nil {
		t.Fatalf("NewSQLiteStore() error = %v", err)
	}
	defer store.Close()

	raw, err := store.GetRawCapture(context.Background(), "twitter", "2045106658200682788")
	if err != nil {
		t.Fatalf("GetRawCapture() error = %v", err)
	}
	bundle := compile.BuildBundle(raw)
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

	stage2, err := stage2Dedup(ctx, client.runtime, client.model, bundle, stage1)
	if err != nil {
		t.Fatalf("stage2Dedup() error = %v", err)
	}
	printJSON("STAGE2", stage2)

	stage3, err := stage3Classify(ctx, client.runtime, client.model, bundle, stage2)
	if err != nil {
		t.Fatalf("stage3Classify() error = %v", err)
	}
	printJSON("STAGE3", stage3)

	stage4, err := stage4Validate(ctx, client.runtime, client.model, bundle, stage3, 1)
	if err != nil {
		t.Fatalf("stage4Validate() error = %v", err)
	}
	printJSON("STAGE4", stage4)

	stage5, err := stage5Render(ctx, client.runtime, client.model, bundle, stage4)
	if err != nil {
		t.Fatalf("stage5Render() error = %v", err)
	}
	printJSON("STAGE5", stage5)
}

func printJSON(label string, value any) {
	payload, _ := json.MarshalIndent(value, "", "  ")
	fmt.Printf("\n=== %s ===\n%s\n", label, string(payload))
}
