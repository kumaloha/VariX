package verify_test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/kumaloha/VariX/varix/storage/contentstore"
	verification "github.com/kumaloha/VariX/varix/verify"
)

func TestDetailedVerifySampleQBy4Fg8tm(t *testing.T) {
	if os.Getenv("RUN_LONG_VERIFY_SAMPLE") == "" {
		t.Skip("set RUN_LONG_VERIFY_SAMPLE=1 to run long sample verify")
	}
	root := filepath.Clean("/Users/kuma/Projects/VariX")
	store, err := contentstore.NewSQLiteStore(filepath.Join(root, "data", "content.db"))
	if err != nil {
		t.Fatalf("NewSQLiteStore() error = %v", err)
	}
	defer store.Close()

	record, err := store.GetCompiledOutput(context.Background(), "weibo", "QBy4Fg8tm")
	if err != nil {
		t.Fatalf("GetCompiledOutput() error = %v", err)
	}
	raw, err := store.GetRawCapture(context.Background(), "weibo", "QBy4Fg8tm")
	if err != nil {
		t.Fatalf("GetRawCapture() error = %v", err)
	}

	client := verification.NewClientFromConfig(root, nil)
	if client == nil {
		t.Fatal("NewClientFromConfig() returned nil")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 14*time.Minute)
	defer cancel()

	verificationResult, err := client.VerifyDetailed(ctx, verification.BuildBundle(raw), record.Output)
	if err != nil {
		t.Fatalf("VerifyDetailed() error = %v", err)
	}

	payload, err := json.MarshalIndent(verificationResult, "", "  ")
	if err != nil {
		t.Fatalf("json.MarshalIndent() error = %v", err)
	}
	fmt.Println(string(payload))
}
