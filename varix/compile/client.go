package compile

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

	"github.com/kumaloha/VariX/varix/config"
	varixllm "github.com/kumaloha/VariX/varix/llm"
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

func (c *Client) EnableLLMCache(store varixllm.CacheStore, mode varixllm.CacheMode) {
	if c == nil || c.runtime == nil || store == nil {
		return
	}
	c.runtime = newCachedRuntime(c.runtime, store, mode)
}

func NewClientFromConfig(projectRoot string, httpClient *http.Client) *Client {
	apiKey := config.FirstConfiguredValue(projectRoot, "DASHSCOPE_API_KEY", "OPENAI_API_KEY")
	if strings.TrimSpace(apiKey) == "" {
		return nil
	}
	baseURL := config.FirstConfiguredValue(projectRoot, "COMPILE_BASE_URL", "DASHSCOPE_BASE_URL")
	if strings.TrimSpace(baseURL) == "" {
		baseURL = defaultDashScopeCompatibleBaseURL
	}
	model := config.FirstConfiguredValue(projectRoot, "COMPILE_MODEL")
	if strings.TrimSpace(model) == "" {
		model = varixllm.Qwen36PlusModel
	}
	if httpClient == nil {
		timeout := 1200 * time.Second
		if raw := config.FirstConfiguredValue(projectRoot, "COMPILE_TIMEOUT_SECONDS"); strings.TrimSpace(raw) != "" {
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

func (c *Client) Compile(ctx context.Context, bundle Bundle) (Record, error) {
	if c == nil || c.runtime == nil {
		return Record{}, fmt.Errorf("compile client is nil")
	}
	debugRunDir := c.startDebugRun(bundle)
	debugStage(bundle, "pipeline", "start")
	totalStart := time.Now()
	stageMetrics := map[string]int64{}
	stageStart := time.Now()
	graph, err := stage1Extract(ctx, c.runtime, c.model, bundle)
	if err != nil {
		debugStage(bundle, "extract", "error: "+err.Error())
		c.writeDebugArtifact(debugRunDir, "extract.error.txt", []byte(err.Error()))
		return Record{}, err
	}
	recordStageMetric(stageMetrics, "extract", time.Since(stageStart))
	debugStage(bundle, "extract", fmt.Sprintf("done nodes=%d edges=%d off_graph=%d", len(graph.Nodes), len(graph.Edges), len(graph.OffGraph)))
	c.writeDebugJSON(debugRunDir, "extract.json", graph)
	stageStart = time.Now()
	graph, err = stage1Refine(ctx, c.runtime, c.model, bundle, graph)
	if err != nil {
		debugStage(bundle, "refine", "error: "+err.Error())
		c.writeDebugArtifact(debugRunDir, "refine.error.txt", []byte(err.Error()))
		return Record{}, err
	}
	recordStageMetric(stageMetrics, "refine", time.Since(stageStart))
	debugStage(bundle, "refine", fmt.Sprintf("done nodes=%d off_graph=%d", len(graph.Nodes), len(graph.OffGraph)))
	c.writeDebugJSON(debugRunDir, "refine.json", graph)
	stageStart = time.Now()
	graph, err = stage1Aggregate(ctx, c.runtime, c.model, bundle, graph)
	if err != nil {
		debugStage(bundle, "aggregate", "error: "+err.Error())
		c.writeDebugArtifact(debugRunDir, "aggregate.error.txt", []byte(err.Error()))
		return Record{}, err
	}
	recordStageMetric(stageMetrics, "aggregate", time.Since(stageStart))
	debugStage(bundle, "aggregate", fmt.Sprintf("done nodes=%d aux_edges=%d", len(graph.Nodes), len(graph.AuxEdges)))
	c.writeDebugJSON(debugRunDir, "aggregate.json", graph)
	stageStart = time.Now()
	graph, err = stage2Support(ctx, c.runtime, c.model, bundle, graph)
	if err != nil {
		debugStage(bundle, "support", "error: "+err.Error())
		c.writeDebugArtifact(debugRunDir, "support.error.txt", []byte(err.Error()))
		return Record{}, err
	}
	recordStageMetric(stageMetrics, "support", time.Since(stageStart))
	debugStage(bundle, "support", fmt.Sprintf("done nodes=%d aux_edges=%d", len(graph.Nodes), len(graph.AuxEdges)))
	c.writeDebugJSON(debugRunDir, "support.json", graph)
	stageStart = time.Now()
	graph = collapseClusters(graph)
	recordStageMetric(stageMetrics, "collapse", time.Since(stageStart))
	debugStage(bundle, "collapse", fmt.Sprintf("done nodes=%d off_graph=%d heads=%d", len(graph.Nodes), len(graph.OffGraph), len(graph.BranchHeads)))
	c.writeDebugJSON(debugRunDir, "collapse.json", graph)
	stageStart = time.Now()
	graph, err = stageSemanticCoverage(ctx, c.runtime, c.model, bundle, graph)
	if err != nil {
		debugStage(bundle, "semantic_coverage", "error: "+err.Error())
		c.writeDebugArtifact(debugRunDir, "semantic_coverage.error.txt", []byte(err.Error()))
		return Record{}, err
	}
	recordStageMetric(stageMetrics, "semantic_coverage", time.Since(stageStart))
	debugStage(bundle, "semantic_coverage", fmt.Sprintf("done units=%d", len(graph.SemanticUnits)))
	c.writeDebugJSON(debugRunDir, "semantic_coverage.json", graph)
	stageStart = time.Now()
	graph, err = stage3Mainline(ctx, c.runtime, c.model, bundle, graph)
	if err != nil {
		debugStage(bundle, "relations", "error: "+err.Error())
		c.writeDebugArtifact(debugRunDir, "relations.error.txt", []byte(err.Error()))
		return Record{}, err
	}
	recordStageMetric(stageMetrics, "relations", time.Since(stageStart))
	debugStage(bundle, "relations", fmt.Sprintf("done nodes=%d relations=%d spines=%d", len(graph.Nodes), len(graph.Edges), len(graph.Spines)))
	c.writeDebugJSON(debugRunDir, "relations.json", graph)
	stageStart = time.Now()
	graph, err = stage3Classify(ctx, c.runtime, c.model, bundle, graph)
	if err != nil {
		debugStage(bundle, "classify", "error: "+err.Error())
		c.writeDebugArtifact(debugRunDir, "classify.error.txt", []byte(err.Error()))
		return Record{}, err
	}
	recordStageMetric(stageMetrics, "classify", time.Since(stageStart))
	debugStage(bundle, "classify", fmt.Sprintf("done drivers=%d targets=%d", countRole(graph, roleDriver), countTargets(graph)))
	c.writeDebugJSON(debugRunDir, "classify.json", graph)
	stageStart = time.Now()
	graph, err = stage4Validate(ctx, c.runtime, c.model, bundle, graph, 1)
	if err != nil {
		debugStage(bundle, "validate", "error: "+err.Error())
		c.writeDebugArtifact(debugRunDir, "validate.error.txt", []byte(err.Error()))
		return Record{}, err
	}
	recordStageMetric(stageMetrics, "validate", time.Since(stageStart))
	debugStage(bundle, "validate", fmt.Sprintf("done nodes=%d relations=%d rounds=%d", len(graph.Nodes), len(graph.Edges), graph.Rounds))
	c.writeDebugJSON(debugRunDir, "validate.json", graph)
	stageStart = time.Now()
	out, err := stage5Render(ctx, c.runtime, c.model, bundle, graph)
	if err != nil {
		debugStage(bundle, "render", "error: "+err.Error())
		c.writeDebugArtifact(debugRunDir, "render.error.txt", []byte(err.Error()))
		return Record{}, err
	}
	if err := out.Validate(); err != nil {
		err = fmt.Errorf("render output invalid: %w", err)
		debugStage(bundle, "render", "error: "+err.Error())
		c.writeDebugArtifact(debugRunDir, "render.error.txt", []byte(err.Error()))
		return Record{}, err
	}
	recordStageMetric(stageMetrics, "render", time.Since(stageStart))
	debugStage(bundle, "render", fmt.Sprintf("done drivers=%d targets=%d paths=%d evidence=%d explanation=%d supplementary=%d", len(out.Drivers), len(out.Targets), len(out.TransmissionPaths), len(out.EvidenceNodes), len(out.ExplanationNodes), len(out.SupplementaryNodes)))
	c.writeDebugJSON(debugRunDir, "render.json", out)
	return Record{
		UnitID:         bundle.UnitID,
		Source:         bundle.Source,
		ExternalID:     bundle.ExternalID,
		RootExternalID: bundle.RootExternalID,
		Model:          c.model,
		Metrics: RecordMetrics{
			CompileElapsedMS:      DurationToMilliseconds(time.Since(totalStart)),
			CompileStageElapsedMS: stageMetrics,
		},
		Output:     out,
		CompiledAt: NowUTC(),
	}, nil
}

func recordStageMetric(metrics map[string]int64, stage string, duration time.Duration) {
	if metrics == nil {
		return
	}
	metrics[stage] = DurationToMilliseconds(duration)
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

func debugStage(bundle Bundle, stageName, message string) {
	if strings.TrimSpace(os.Getenv("COMPILE_STAGE_DEBUG")) == "" {
		return
	}
	unitID := strings.TrimSpace(bundle.UnitID)
	if unitID == "" {
		unitID = strings.TrimSpace(bundle.ExternalID)
	}
	fmt.Fprintf(os.Stderr, "[compile-stage] %s %s %s\n", NowUTC().Format(time.RFC3339), stageName, unitID)
	fmt.Fprintf(os.Stderr, "[compile-stage] %s %s\n", stageName, message)
}

func (c *Client) startDebugRun(bundle Bundle) string {
	if c == nil {
		return ""
	}
	debugFlag := strings.TrimSpace(os.Getenv("COMPILE_STAGE_DEBUG"))
	projectRoot := strings.TrimSpace(c.projectRoot)
	if debugFlag == "" || projectRoot == "" {
		return ""
	}
	unitID := sanitizeDebugPath(FirstNonEmpty(bundle.UnitID, bundle.ExternalID, "unknown"))
	ts := NowUTC().Format("20060102T150405Z")
	dir := filepath.Join(projectRoot, ".omx", "debug", "compile", unitID, ts)
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
