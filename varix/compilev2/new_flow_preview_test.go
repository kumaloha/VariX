package compilev2

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/kumaloha/VariX/varix/compile"
	"github.com/kumaloha/VariX/varix/config"
	"github.com/kumaloha/VariX/varix/storage/contentstore"
	_ "modernc.org/sqlite"
)

func TestPreviewNewFlowForThreeSamples(t *testing.T) {
	if os.Getenv("RUN_NEW_FLOW_PREVIEW") == "" {
		t.Skip("set RUN_NEW_FLOW_PREVIEW=1 to run the new-flow preview")
	}

	root := filepath.Clean("/Users/kuma/Projects/VariX")
	settings := config.DefaultSettings(root)
	store, err := contentstore.NewSQLiteStore(settings.ContentDBPath)
	if err != nil {
		t.Fatalf("NewSQLiteStore() error = %v", err)
	}
	defer store.Close()

	client := NewClientFromConfig(root, nil)
	if client == nil {
		t.Fatal("NewClientFromConfig() returned nil")
	}

	outputDir := filepath.Join(root, ".tmp", "new-compile-flow-preview")
	if err := os.RemoveAll(outputDir); err != nil {
		t.Fatalf("RemoveAll(%q) error = %v", outputDir, err)
	}
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(%q) error = %v", outputDir, err)
	}

	samples := []struct {
		platform   string
		externalID string
	}{
		{platform: "twitter", externalID: "2045106658200682788"},
		{platform: "twitter", externalID: "2043653387271712962"},
		{platform: "weibo", externalID: "QAJ0n0YGU"},
	}
	if os.Getenv("PREVIEW_ALL") == "1" {
		allSamples, err := loadAllRawCaptureKeys(settings.ContentDBPath)
		if err != nil {
			t.Fatalf("loadAllRawCaptureKeys() error = %v", err)
		}
		if len(allSamples) == 0 {
			t.Fatal("no raw captures found for PREVIEW_ALL=1")
		}
		samples = allSamples
	}
	if targetPlatform := os.Getenv("TARGET_PLATFORM"); targetPlatform != "" {
		targetExternalID := os.Getenv("TARGET_EXTERNAL_ID")
		if targetExternalID == "" {
			t.Fatal("TARGET_EXTERNAL_ID is required when TARGET_PLATFORM is set")
		}
		samples = []struct {
			platform   string
			externalID string
		}{
			{platform: targetPlatform, externalID: targetExternalID},
		}
	}

	opts := FlowPreviewOptions{
		StopAfter: os.Getenv("STOP_AFTER"),
	}
	workerCount := envInt("FLOW_WORKERS", 3)
	if workerCount <= 0 {
		workerCount = 3
	}

	results := make([]FlowPreviewResult, len(samples))
	errs := make(chan error, len(samples))
	sem := make(chan struct{}, workerCount)
	var wg sync.WaitGroup
	for i, sample := range samples {
		wg.Add(1)
		go func(idx int, sample struct {
			platform   string
			externalID string
		}) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			raw, err := store.GetRawCapture(context.Background(), sample.platform, sample.externalID)
			if err != nil {
				errs <- fmt.Errorf("%s:%s raw: %w", sample.platform, sample.externalID, err)
				return
			}

			itemCtx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
			defer cancel()

			result, err := client.PreviewFlow(itemCtx, compile.BuildBundle(raw), opts)
			if err != nil {
				errs <- fmt.Errorf("%s:%s flow: %w", sample.platform, sample.externalID, err)
				return
			}
			results[idx] = result
			if err := writePreviewArtifacts(outputDir, result); err != nil {
				errs <- fmt.Errorf("%s:%s write: %w", sample.platform, sample.externalID, err)
				return
			}
			fmt.Printf("[new-flow-step] %s:%s extract done nodes=%d\n", result.Platform, result.ExternalID, len(result.Extract.Nodes))
			if opts.StopAfter == "extract" {
				fmt.Printf("[new-flow] done %s:%s extract=%d\n", result.Platform, result.ExternalID, len(result.Extract.Nodes))
				return
			}
			fmt.Printf("[new-flow-step] %s:%s relations done nodes=%d edges=%d\n", result.Platform, result.ExternalID, len(result.Relations.Nodes), len(result.Relations.Edges))
			if opts.StopAfter == "relations" {
				fmt.Printf("[new-flow] done %s:%s extract=%d relations=%d\n", result.Platform, result.ExternalID, len(result.Extract.Nodes), len(result.Relations.Nodes))
				return
			}
			fmt.Printf("[new-flow-step] %s:%s classify done targets=%d\n", result.Platform, result.ExternalID, len(previewTargetNodes(result.Classify.Nodes)))
			if opts.StopAfter == "classify" {
				fmt.Printf("[new-flow] done %s:%s extract=%d relations=%d classify_targets=%d\n", result.Platform, result.ExternalID, len(result.Extract.Nodes), len(result.Relations.Nodes), len(previewTargetNodes(result.Classify.Nodes)))
				return
			}
			fmt.Printf("[new-flow-step] %s:%s render done drivers=%d targets=%d paths=%d\n", result.Platform, result.ExternalID, len(result.Render.Drivers), len(result.Render.Targets), len(result.Render.TransmissionPaths))
			fmt.Printf("[new-flow] done %s:%s extract=%d relations=%d classify_targets=%d\n",
				result.Platform,
				result.ExternalID,
				len(result.Extract.Nodes),
				len(result.Relations.Nodes),
				len(previewTargetNodes(result.Classify.Nodes)),
			)
		}(i, sample)
	}

	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}

	sort.Slice(results, func(i, j int) bool {
		if results[i].Platform != results[j].Platform {
			return results[i].Platform < results[j].Platform
		}
		return results[i].ExternalID < results[j].ExternalID
	})
	payload, err := json.MarshalIndent(results, "", "  ")
	if err != nil {
		t.Fatalf("MarshalIndent() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(outputDir, "all.json"), payload, 0o644); err != nil {
		t.Fatalf("WriteFile(all.json) error = %v", err)
	}
	fmt.Printf("\n=== NEW FLOW PREVIEW ===\n%s\n", string(payload))
}

func loadAllRawCaptureKeys(dbPath string) ([]struct {
	platform   string
	externalID string
}, error) {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, err
	}
	defer db.Close()

	rows, err := db.Query(`SELECT platform, external_id FROM raw_captures ORDER BY updated_at DESC, created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]struct {
		platform   string
		externalID string
	}, 0)
	for rows.Next() {
		var item struct {
			platform   string
			externalID string
		}
		if err := rows.Scan(&item.platform, &item.externalID); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func writePreviewArtifacts(outputDir string, result FlowPreviewResult) error {
	payload, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return err
	}
	base := result.Platform + "_" + result.ExternalID
	if err := os.WriteFile(filepath.Join(outputDir, base+".json"), payload, 0o644); err != nil {
		return err
	}
	for _, step := range []struct {
		name  string
		value any
	}{
		{name: "extract", value: result.Extract},
		{name: "relations", value: result.Relations},
		{name: "classify", value: result.Classify},
		{name: "render", value: result.Render},
	} {
		stepPayload, err := json.MarshalIndent(map[string]any{step.name: step.value}, "", "  ")
		if err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(outputDir, base+"."+step.name+".json"), stepPayload, 0o644); err != nil {
			return err
		}
	}
	return os.WriteFile(filepath.Join(outputDir, base+".mainline.md"), []byte(BuildMainlineMarkdown(result)), 0o644)
}

func previewNodesByRolePreview(nodes []PreviewNode, role string) []PreviewNode {
	out := make([]PreviewNode, 0)
	for _, node := range nodes {
		if node.Role == role {
			out = append(out, node)
		}
	}
	return out
}

func previewTargetNodes(nodes []PreviewNode) []PreviewNode {
	out := make([]PreviewNode, 0)
	for _, node := range nodes {
		if node.IsTarget {
			out = append(out, node)
		}
	}
	return out
}
