package compilev2

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/kumaloha/VariX/varix/compile"
	"github.com/kumaloha/VariX/varix/ingest/types"
	_ "modernc.org/sqlite"
)

type latestStagePreviewRecord struct {
	Rank       int                      `json:"rank"`
	Platform   string                   `json:"platform"`
	ExternalID string                   `json:"external_id"`
	URL        string                   `json:"url"`
	UpdatedAt  string                   `json:"updated_at"`
	Stage1     latestStagePreviewStage1 `json:"stage1"`
	Stage3     latestStagePreviewStage3 `json:"stage3"`
}

type latestStagePreviewStage1 struct {
	NodeCount     int                          `json:"node_count"`
	EdgeCount     int                          `json:"edge_count"`
	OffGraphCount int                          `json:"off_graph_count"`
	TargetCandidates []latestStagePreviewNode  `json:"target_candidates,omitempty"`
	Nodes         []latestStagePreviewNode     `json:"nodes"`
	Edges         []latestStagePreviewEdge     `json:"edges"`
	OffGraph      []latestStagePreviewOffGraph `json:"off_graph"`
}

type latestStagePreviewStage3 struct {
	NodeCount     int                          `json:"node_count"`
	OffGraphCount int                          `json:"off_graph_count"`
	Drivers       []latestStagePreviewNode     `json:"drivers"`
	Transmission  []latestStagePreviewNode     `json:"transmission"`
	Targets       []latestStagePreviewNode     `json:"targets"`
	Orphans       []latestStagePreviewNode     `json:"orphans"`
	OffGraph      []latestStagePreviewOffGraph `json:"off_graph"`
}

type latestStagePreviewNode struct {
	ID       string `json:"id"`
	Text     string `json:"text"`
	Ontology string `json:"ontology,omitempty"`
}

type latestStagePreviewEdge struct {
	From string `json:"from"`
	To   string `json:"to"`
}

type latestStagePreviewOffGraph struct {
	ID         string `json:"id"`
	Text       string `json:"text"`
	Role       string `json:"role"`
	AttachesTo string `json:"attaches_to,omitempty"`
}

type latestStagePreviewRaw struct {
	Rank       int
	Platform   string
	ExternalID string
	URL        string
	UpdatedAt  string
	Payload    string
}

func TestPreviewLatestTenStage1AndStage3(t *testing.T) {
	if os.Getenv("RUN_LATEST_COMPILE_STAGE_PREVIEW") == "" {
		t.Skip("set RUN_LATEST_COMPILE_STAGE_PREVIEW=1 to run latest compile stage preview")
	}

	root := filepath.Clean("/Users/kuma/Projects/VariX")
	dbPath := filepath.Join(root, "data", "content.db")
	targetPlatform := os.Getenv("TARGET_PLATFORM")
	targetExternalID := os.Getenv("TARGET_EXTERNAL_ID")
	limit := envInt("PREVIEW_LIMIT", 10)
	if limit <= 0 {
		limit = 10
	}
	workers := envInt("PREVIEW_WORKERS", 5)
	if workers <= 0 {
		workers = 5
	}
	itemTimeout := time.Duration(envInt("ITEM_TIMEOUT_SECONDS", 180)) * time.Second
	if itemTimeout <= 0 {
		itemTimeout = 180 * time.Second
	}
	stopAfterStage1 := os.Getenv("STOP_AFTER_STAGE1") == "1"
	var (
		latest []latestStagePreviewRaw
		err    error
	)
	if targetPlatform != "" && targetExternalID != "" {
		workers = 1
		latest, err = loadSpecificStagePreviewRaw(dbPath, targetPlatform, targetExternalID)
	} else {
		latest, err = loadLatestStagePreviewRaw(dbPath, limit)
	}
	if err != nil {
		t.Fatalf("loadLatestStagePreviewRaw() error = %v", err)
	}
	if len(latest) == 0 {
		t.Fatal("no raw captures found")
	}

	client := NewClientFromConfig(root, nil)
	if client == nil {
		t.Fatal("NewClientFromConfig() returned nil")
	}

	outputDir := filepath.Join(root, ".tmp", "latest-compile-stage-preview")
	if err := os.RemoveAll(outputDir); err != nil {
		t.Fatalf("RemoveAll(%q) error = %v", outputDir, err)
	}
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(%q) error = %v", outputDir, err)
	}

	results := make([]latestStagePreviewRecord, len(latest))
	errs := make(chan error, len(latest))
	sem := make(chan struct{}, workers)
	var wg sync.WaitGroup

	for i, item := range latest {
		wg.Add(1)
		go func(idx int, item latestStagePreviewRaw) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			itemCtx, cancel := context.WithTimeout(context.Background(), itemTimeout)
			defer cancel()

			record, err := runLatestStagePreviewItem(itemCtx, client, item, stopAfterStage1)
			if err != nil {
				errs <- fmt.Errorf("%s:%s: %w", item.Platform, item.ExternalID, err)
				return
			}
			results[idx] = record
			if err := writeLatestStagePreviewRecord(outputDir, record); err != nil {
				errs <- err
				return
			}
			fmt.Printf("[preview] done rank=%d %s:%s stage1_nodes=%d stage3_targets=%d\n", record.Rank, record.Platform, record.ExternalID, record.Stage1.NodeCount, len(record.Stage3.Targets))
		}(i, item)
	}

	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}

	sort.Slice(results, func(i, j int) bool { return results[i].Rank < results[j].Rank })
	payload, err := json.MarshalIndent(results, "", "  ")
	if err != nil {
		t.Fatalf("MarshalIndent() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(outputDir, "all.json"), payload, 0o644); err != nil {
		t.Fatalf("WriteFile(all.json) error = %v", err)
	}
	fmt.Printf("\n=== LATEST COMPILE STAGE PREVIEW ===\n%s\n", string(payload))
}

func loadLatestStagePreviewRaw(dbPath string, limit int) ([]latestStagePreviewRaw, error) {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, err
	}
	defer db.Close()

	rows, err := db.Query(`SELECT platform, external_id, url, updated_at, payload_json
		FROM raw_captures
		ORDER BY updated_at DESC, created_at DESC
		LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]latestStagePreviewRaw, 0, limit)
	rank := 1
	for rows.Next() {
		var item latestStagePreviewRaw
		item.Rank = rank
		if err := rows.Scan(&item.Platform, &item.ExternalID, &item.URL, &item.UpdatedAt, &item.Payload); err != nil {
			return nil, err
		}
		out = append(out, item)
		rank++
	}
	return out, rows.Err()
}

func loadSpecificStagePreviewRaw(dbPath, platform, externalID string) ([]latestStagePreviewRaw, error) {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, err
	}
	defer db.Close()

	row := db.QueryRow(`SELECT platform, external_id, url, updated_at, payload_json
		FROM raw_captures
		WHERE platform = ? AND external_id = ?`, platform, externalID)

	var item latestStagePreviewRaw
	item.Rank = 1
	if err := row.Scan(&item.Platform, &item.ExternalID, &item.URL, &item.UpdatedAt, &item.Payload); err != nil {
		return nil, err
	}
	return []latestStagePreviewRaw{item}, nil
}

func runLatestStagePreviewItem(ctx context.Context, client *Client, item latestStagePreviewRaw, stopAfterStage1 bool) (latestStagePreviewRecord, error) {
	var raw types.RawContent
	if err := json.Unmarshal([]byte(item.Payload), &raw); err != nil {
		return latestStagePreviewRecord{}, err
	}
	bundle := compile.BuildBundle(raw)

	stage1, err := stage1Extract(ctx, client.runtime, client.model, bundle)
	if err != nil {
		return latestStagePreviewRecord{}, fmt.Errorf("stage1_extract: %w", err)
	}

	record := latestStagePreviewRecord{
		Rank:       item.Rank,
		Platform:   item.Platform,
		ExternalID: item.ExternalID,
		URL:        item.URL,
		UpdatedAt:  normalizePreviewTime(item.UpdatedAt),
		Stage1: latestStagePreviewStage1{
			NodeCount:        len(stage1.Nodes),
			EdgeCount:        len(stage1.Edges),
			OffGraphCount:    len(stage1.OffGraph),
			TargetCandidates: previewTargetCandidates(stage1),
			Nodes:            previewNodes(stage1.Nodes),
			Edges:            previewEdges(stage1.Edges),
			OffGraph:         previewOffGraph(stage1.OffGraph),
		},
	}
	if stopAfterStage1 {
		return record, nil
	}

	stage3, err := stage3Classify(ctx, client.runtime, client.model, bundle, stage1)
	if err != nil {
		return record, fmt.Errorf("stage3_classify: %w", err)
	}

	record.Stage3 = latestStagePreviewStage3{
		NodeCount:     len(stage3.Nodes),
		OffGraphCount: len(stage3.OffGraph),
		Drivers:       previewNodesByRole(stage3.Nodes, roleDriver),
		Transmission:  previewNodesByRole(stage3.Nodes, roleTransmission),
		Targets:       previewTargetGraphNodes(stage3.Nodes),
		Orphans:       previewNodesByRole(stage3.Nodes, roleOrphan),
		OffGraph:      previewOffGraph(stage3.OffGraph),
	}
	return record, nil
}

func previewNodes(nodes []graphNode) []latestStagePreviewNode {
	out := make([]latestStagePreviewNode, 0, len(nodes))
	for _, node := range nodes {
		out = append(out, latestStagePreviewNode{
			ID:       node.ID,
			Text:     node.Text,
			Ontology: node.Ontology,
		})
	}
	return out
}

func previewNodesByRole(nodes []graphNode, role graphRole) []latestStagePreviewNode {
	out := make([]latestStagePreviewNode, 0)
	for _, node := range nodes {
		if node.Role != role {
			continue
		}
		out = append(out, latestStagePreviewNode{
			ID:       node.ID,
			Text:     node.Text,
			Ontology: node.Ontology,
		})
	}
	return out
}

func previewTargetGraphNodes(nodes []graphNode) []latestStagePreviewNode {
	out := make([]latestStagePreviewNode, 0)
	for _, node := range nodes {
		if !node.IsTarget {
			continue
		}
		out = append(out, latestStagePreviewNode{
			ID:       node.ID,
			Text:     node.Text,
			Ontology: node.Ontology,
		})
	}
	return out
}

func previewTargetCandidates(state graphState) []latestStagePreviewNode {
	inDegree := map[string]int{}
	outDegree := map[string]int{}
	for _, n := range state.Nodes {
		inDegree[n.ID] = 0
		outDegree[n.ID] = 0
	}
	for _, e := range state.Edges {
		outDegree[e.From]++
		inDegree[e.To]++
	}
	out := make([]latestStagePreviewNode, 0)
	for _, node := range state.Nodes {
		if inDegree[node.ID] > 0 && outDegree[node.ID] == 0 {
			out = append(out, latestStagePreviewNode{ID: node.ID, Text: node.Text})
		}
	}
	return out
}

func previewEdges(edges []graphEdge) []latestStagePreviewEdge {
	out := make([]latestStagePreviewEdge, 0, len(edges))
	for _, edge := range edges {
		out = append(out, latestStagePreviewEdge{From: edge.From, To: edge.To})
	}
	return out
}

func previewOffGraph(items []offGraphItem) []latestStagePreviewOffGraph {
	out := make([]latestStagePreviewOffGraph, 0, len(items))
	for _, item := range items {
		out = append(out, latestStagePreviewOffGraph{
			ID:         item.ID,
			Text:       item.Text,
			Role:       item.Role,
			AttachesTo: item.AttachesTo,
		})
	}
	return out
}

func normalizePreviewTime(value string) string {
	if parsed, err := time.Parse(time.RFC3339Nano, value); err == nil {
		return parsed.UTC().Format(time.RFC3339)
	}
	return value
}

func envInt(name string, fallback int) int {
	raw := os.Getenv(name)
	if raw == "" {
		return fallback
	}
	value, err := strconv.Atoi(raw)
	if err != nil {
		return fallback
	}
	return value
}

func writeLatestStagePreviewRecord(outputDir string, record latestStagePreviewRecord) error {
	payload, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return err
	}
	name := strconv.Itoa(record.Rank) + "_" + record.Platform + "_" + record.ExternalID + ".json"
	return os.WriteFile(filepath.Join(outputDir, name), payload, 0o644)
}
