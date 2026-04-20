package compilev2

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
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
	runtime runtimeChat
	model   string
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
		timeout := 180 * time.Second
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
	return &Client{runtime: runtime, model: strings.TrimSpace(model)}
}

func (c *Client) Compile(ctx context.Context, bundle compile.Bundle) (compile.Record, error) {
	if c == nil || c.runtime == nil {
		return compile.Record{}, fmt.Errorf("compile v2 client is nil")
	}
	graph, err := stage1Extract(ctx, c.runtime, c.model, bundle)
	if err != nil {
		return compile.Record{}, err
	}
	graph = stage2Dedup(graph)
	graph, err = stage3Classify(ctx, c.runtime, c.model, bundle, graph)
	if err != nil {
		return compile.Record{}, err
	}
	// Stage 4 validate is intentionally a no-op in the first runnable v2 slice.
	out, err := stage5Render(ctx, c.runtime, c.model, bundle, graph)
	if err != nil {
		return compile.Record{}, err
	}
	return compile.Record{
		UnitID:         bundle.UnitID,
		Source:         bundle.Source,
		ExternalID:     bundle.ExternalID,
		RootExternalID: bundle.RootExternalID,
		Model:          c.model,
		Output:         out,
		CompiledAt:     time.Now().UTC(),
	}, nil
}

func (c *Client) Verify(ctx context.Context, bundle compile.Bundle, output compile.Output) (compile.Verification, error) {
	return compile.Verification{}, fmt.Errorf("compile v2 client does not implement verify")
}

func (c *Client) VerifyDetailed(ctx context.Context, bundle compile.Bundle, output compile.Output) (compile.Verification, error) {
	return compile.Verification{}, fmt.Errorf("compile v2 client does not implement verify")
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
