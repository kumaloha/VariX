package compilev2

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/kumaloha/VariX/varix/compile"
	"github.com/kumaloha/VariX/varix/config"
	"github.com/kumaloha/forge/llm"
)

const defaultDashScopeCompatibleBaseURL = "https://dashscope.aliyuncs.com/compatible-mode/v1"

type runtimeChat interface {
	Call(ctx context.Context, req llm.ProviderRequest) (llm.Response, error)
}

type Client struct {
	runtime     runtimeChat
	model       string
	projectRoot string
}

func NewClientFromConfig(projectRoot string, httpClient *http.Client) *Client {
	apiKey := firstConfiguredValue(projectRoot, "DASHSCOPE_API_KEY", "OPENAI_API_KEY")
	if strings.TrimSpace(apiKey) == "" {
		return nil
	}
	baseURL := firstConfiguredValue(projectRoot, "COMPILE_BASE_URL", "DASHSCOPE_BASE_URL")
	if strings.TrimSpace(baseURL) == "" {
		baseURL = defaultDashScopeCompatibleBaseURL
	}
	model := firstConfiguredValue(projectRoot, "COMPILE_MODEL")
	if strings.TrimSpace(model) == "" {
		model = compile.Qwen36PlusModel
	}
	if httpClient == nil {
		timeout := 1200 * time.Second
		if raw := firstConfiguredValue(projectRoot, "COMPILE_TIMEOUT_SECONDS"); strings.TrimSpace(raw) != "" {
			if seconds, err := strconv.Atoi(raw); err == nil && seconds > 0 {
				timeout = time.Duration(seconds) * time.Second
			}
		}
		httpClient = &http.Client{Timeout: timeout}
	}
	opts := []llm.DashscopeOption{llm.WithAPIKey(apiKey)}
	if strings.TrimSpace(baseURL) != "" {
		opts = append(opts, llm.WithAPIBase(baseURL))
	}
	if httpClient.Timeout > 0 {
		opts = append(opts, llm.WithTimeout(httpClient.Timeout))
	}
	provider, err := llm.NewDashscope(opts...)
	if err != nil {
		return nil
	}
	runtime := llm.NewRuntime(llm.RuntimeConfig{
		Provider: provider,
		LLMConfig: llm.LLMConfig{
			Default: llm.DefaultConfig{
				Model:       strings.TrimSpace(model),
				Search:      false,
				Temperature: 0,
				Thinking:    false,
			},
		},
		MaxAttempts: 3,
	})
	return &Client{runtime: runtime, model: strings.TrimSpace(model), projectRoot: projectRoot}
}

func (c *Client) Compile(ctx context.Context, bundle compile.Bundle) (compile.Record, error) {
	if c == nil || c.runtime == nil {
		return compile.Record{}, fmt.Errorf("compile v2 client is nil")
	}
	debugRunDir := c.startDebugRun(bundle)
	debugV2Stage(bundle, "pipeline", "start")
	totalStart := time.Now()
	stageMetrics := map[string]int64{}
	stageStart := time.Now()
	graph, err := stage1Extract(ctx, c.runtime, c.model, bundle)
	if err != nil {
		debugV2Stage(bundle, "stage1_extract", "error: "+err.Error())
		c.writeDebugArtifact(debugRunDir, "stage1_extract.error.txt", []byte(err.Error()))
		return compile.Record{}, err
	}
	recordStageMetric(stageMetrics, "stage1_extract", time.Since(stageStart))
	debugV2Stage(bundle, "stage1_extract", fmt.Sprintf("done nodes=%d edges=%d off_graph=%d", len(graph.Nodes), len(graph.Edges), len(graph.OffGraph)))
	c.writeDebugJSON(debugRunDir, "stage1_extract.json", graph)
	stageStart = time.Now()
	graph, err = stage2Dedup(ctx, c.runtime, c.model, bundle, graph)
	if err != nil {
		debugV2Stage(bundle, "stage2_dedup", "error: "+err.Error())
		c.writeDebugArtifact(debugRunDir, "stage2_dedup.error.txt", []byte(err.Error()))
		return compile.Record{}, err
	}
	recordStageMetric(stageMetrics, "stage2_dedup", time.Since(stageStart))
	debugV2Stage(bundle, "stage2_dedup", fmt.Sprintf("done nodes=%d edges=%d off_graph=%d", len(graph.Nodes), len(graph.Edges), len(graph.OffGraph)))
	c.writeDebugJSON(debugRunDir, "stage2_dedup.json", graph)
	stageStart = time.Now()
	graph, err = stage3Classify(ctx, c.runtime, c.model, bundle, graph)
	if err != nil {
		debugV2Stage(bundle, "stage3_classify", "error: "+err.Error())
		c.writeDebugArtifact(debugRunDir, "stage3_classify.error.txt", []byte(err.Error()))
		return compile.Record{}, err
	}
	recordStageMetric(stageMetrics, "stage3_classify", time.Since(stageStart))
	debugV2Stage(bundle, "stage3_classify", fmt.Sprintf("done drivers=%d targets=%d", countRole(graph, roleDriver), countRole(graph, roleTarget)))
	c.writeDebugJSON(debugRunDir, "stage3_classify.json", graph)
	stageStart = time.Now()
	graph, err = stage3Relations(ctx, c.runtime, c.model, bundle, graph)
	if err != nil {
		debugV2Stage(bundle, "stage3_relations", "error: "+err.Error())
		c.writeDebugArtifact(debugRunDir, "stage3_relations.error.txt", []byte(err.Error()))
		return compile.Record{}, err
	}
	recordStageMetric(stageMetrics, "stage3_relations", time.Since(stageStart))
	debugV2Stage(bundle, "stage3_relations", fmt.Sprintf("done nodes=%d edges=%d off_graph=%d", len(graph.Nodes), len(graph.Edges), len(graph.OffGraph)))
	c.writeDebugJSON(debugRunDir, "stage3_relations.json", graph)
	stageStart = time.Now()
	graph, err = stage3Classify(ctx, c.runtime, c.model, bundle, graph)
	if err != nil {
		debugV2Stage(bundle, "stage3_reclassify", "error: "+err.Error())
		c.writeDebugArtifact(debugRunDir, "stage3_reclassify.error.txt", []byte(err.Error()))
		return compile.Record{}, err
	}
	recordStageMetric(stageMetrics, "stage3_reclassify", time.Since(stageStart))
	debugV2Stage(bundle, "stage3_reclassify", fmt.Sprintf("done drivers=%d targets=%d", countRole(graph, roleDriver), countRole(graph, roleTarget)))
	c.writeDebugJSON(debugRunDir, "stage3_reclassify.json", graph)
	stageStart = time.Now()
	graph, err = stage4Validate(ctx, c.runtime, c.model, bundle, graph, 1)
	if err != nil {
		debugV2Stage(bundle, "stage4_validate", "error: "+err.Error())
		c.writeDebugArtifact(debugRunDir, "stage4_validate.error.txt", []byte(err.Error()))
		return compile.Record{}, err
	}
	recordStageMetric(stageMetrics, "stage4_validate", time.Since(stageStart))
	debugV2Stage(bundle, "stage4_validate", fmt.Sprintf("done rounds=%d nodes=%d edges=%d", graph.Rounds, len(graph.Nodes), len(graph.Edges)))
	c.writeDebugJSON(debugRunDir, "stage4_validate.json", graph)
	stageStart = time.Now()
	out, err := stage5Render(ctx, c.runtime, c.model, bundle, graph)
	if err != nil {
		debugV2Stage(bundle, "stage5_render", "error: "+err.Error())
		c.writeDebugArtifact(debugRunDir, "stage5_render.error.txt", []byte(err.Error()))
		return compile.Record{}, err
	}
	recordStageMetric(stageMetrics, "stage5_render", time.Since(stageStart))
	debugV2Stage(bundle, "stage5_render", fmt.Sprintf("done drivers=%d targets=%d paths=%d evidence=%d explanation=%d supplementary=%d", len(out.Drivers), len(out.Targets), len(out.TransmissionPaths), len(out.EvidenceNodes), len(out.ExplanationNodes), len(out.SupplementaryNodes)))
	c.writeDebugJSON(debugRunDir, "stage5_render.json", out)
	return compile.Record{
		UnitID:         bundle.UnitID,
		Source:         bundle.Source,
		ExternalID:     bundle.ExternalID,
		RootExternalID: bundle.RootExternalID,
		Model:          c.model,
		Metrics: compile.RecordMetrics{
			CompileElapsedMS:      durationToMilliseconds(time.Since(totalStart)),
			CompileStageElapsedMS: stageMetrics,
		},
		Output:     out,
		CompiledAt: time.Now().UTC(),
	}, nil
}

func recordStageMetric(metrics map[string]int64, stage string, duration time.Duration) {
	if metrics == nil {
		return
	}
	metrics[stage] = durationToMilliseconds(duration)
}

func durationToMilliseconds(duration time.Duration) int64 {
	ms := duration.Milliseconds()
	if ms <= 0 {
		return 1
	}
	return ms
}

func (c *Client) Verify(ctx context.Context, bundle compile.Bundle, output compile.Output) (compile.Verification, error) {
	if c == nil {
		return compile.Verification{}, fmt.Errorf("verify client is nil")
	}
	legacy := compile.NewClientFromConfig(c.projectRoot, nil)
	if legacy == nil {
		return compile.Verification{}, fmt.Errorf("verify client config missing")
	}
	return legacy.Verify(ctx, bundle, output)
}

func (c *Client) VerifyDetailed(ctx context.Context, bundle compile.Bundle, output compile.Output) (compile.Verification, error) {
	if c == nil {
		return compile.Verification{}, fmt.Errorf("verify client is nil")
	}
	legacy := compile.NewClientFromConfig(c.projectRoot, nil)
	if legacy == nil {
		return compile.Verification{}, fmt.Errorf("verify client config missing")
	}
	return legacy.VerifyDetailed(ctx, bundle, output)
}

func firstConfiguredValue(projectRoot string, keys ...string) string {
	for _, key := range keys {
		if value, ok := config.Get(projectRoot, key); ok && strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func parseJSONObject(raw string, target any) error {
	raw = strings.TrimSpace(raw)
	raw = strings.TrimPrefix(raw, "```json")
	raw = strings.TrimPrefix(raw, "```")
	raw = strings.TrimSuffix(raw, "```")
	raw = strings.TrimSpace(raw)
	start := strings.Index(raw, "{")
	if start >= 0 {
		raw = raw[start:]
	}
	end := strings.LastIndex(raw, "}")
	if end >= 0 {
		raw = raw[:end+1]
	}
	return json.Unmarshal([]byte(raw), target)
}

func debugV2Stage(bundle compile.Bundle, stageName, message string) {
	if strings.TrimSpace(os.Getenv("COMPILE_STAGE_DEBUG")) == "" {
		return
	}
	unitID := strings.TrimSpace(bundle.UnitID)
	if unitID == "" {
		unitID = strings.TrimSpace(bundle.ExternalID)
	}
	fmt.Fprintf(os.Stderr, "[compilev2-stage] %s %s %s\n", time.Now().UTC().Format(time.RFC3339), stageName, unitID)
	fmt.Fprintf(os.Stderr, "[compilev2-stage] %s %s\n", stageName, message)
}

func (c *Client) startDebugRun(bundle compile.Bundle) string {
	if c == nil || strings.TrimSpace(os.Getenv("COMPILE_STAGE_DEBUG")) == "" || strings.TrimSpace(c.projectRoot) == "" {
		return ""
	}
	unitID := sanitizeDebugPath(firstNonEmpty(bundle.UnitID, bundle.ExternalID, "unknown"))
	ts := time.Now().UTC().Format("20060102T150405Z")
	dir := filepath.Join(c.projectRoot, ".omx", "debug", "compilev2", unitID, ts)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return ""
	}
	_ = os.WriteFile(filepath.Join(dir, "meta.json"), mustJSON(map[string]any{
		"unit_id":          bundle.UnitID,
		"source":           bundle.Source,
		"external_id":      bundle.ExternalID,
		"root_external_id": bundle.RootExternalID,
		"started_at":       ts,
	}), 0o644)
	return dir
}

func (c *Client) writeDebugJSON(dir, name string, value any) {
	if dir == "" {
		return
	}
	_ = os.WriteFile(filepath.Join(dir, name), mustJSON(value), 0o644)
}

func (c *Client) writeDebugArtifact(dir, name string, payload []byte) {
	if dir == "" {
		return
	}
	_ = os.WriteFile(filepath.Join(dir, name), payload, 0o644)
}

func mustJSON(value any) []byte {
	payload, _ := json.MarshalIndent(value, "", "  ")
	return payload
}

func sanitizeDebugPath(value string) string {
	replacer := strings.NewReplacer("/", "_", ":", "_", " ", "_")
	return replacer.Replace(strings.TrimSpace(value))
}
