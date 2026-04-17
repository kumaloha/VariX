package compile

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/kumaloha/VariX/varix/config"
	"github.com/kumaloha/forge/llm"
)

const defaultDashScopeCompatibleBaseURL = "https://dashscope.aliyuncs.com/compatible-mode/v1"

type runtimeChat interface {
	Call(ctx context.Context, req llm.ProviderRequest) (llm.Response, error)
}

type Client struct {
	runtime  runtimeChat
	model    string
	prompts  *promptRegistry
	verifier VerificationService
}

func NewClient(httpClient *http.Client, baseURL, apiKey, model string) *Client {
	if strings.TrimSpace(apiKey) == "" {
		return nil
	}
	opts := []llm.DashscopeOption{
		llm.WithAPIKey(apiKey),
	}
	if strings.TrimSpace(baseURL) != "" {
		opts = append(opts, llm.WithAPIBase(baseURL))
	}
	if httpClient != nil && httpClient.Timeout > 0 {
		opts = append(opts, llm.WithTimeout(httpClient.Timeout))
	}
	provider, err := llm.NewDashscope(opts...)
	if err != nil {
		return nil
	}
	return NewClientWithRuntimeAndPrompts(llm.NewRuntime(llm.RuntimeConfig{
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
	}), strings.TrimSpace(model), newPromptRegistry(""))
}

func NewClientWithRuntime(rt runtimeChat, model string) *Client {
	return NewClientWithRuntimeAndPrompts(rt, model, newPromptRegistry(""))
}

func NewClientWithRuntimeAndPrompts(rt runtimeChat, model string, prompts *promptRegistry) *Client {
	return NewClientWithRuntimePromptsAndVerifier(rt, model, prompts, nil)
}

func NewClientWithRuntimePromptsAndVerifier(rt runtimeChat, model string, prompts *promptRegistry, verifier VerificationService) *Client {
	if rt == nil {
		return nil
	}
	if prompts == nil {
		prompts = newPromptRegistry("")
	}
	if verifier == nil {
		verifier = NewVerificationService(rt, model, prompts)
	}
	return &Client{
		runtime:  rt,
		model:    strings.TrimSpace(model),
		prompts:  prompts,
		verifier: verifier,
	}
}

func NewClientFromConfig(projectRoot string, httpClient *http.Client) *Client {
	settings := config.DefaultSettings(projectRoot)
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
		model = Qwen36PlusModel
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
	client := NewClient(httpClient, baseURL, apiKey, model)
	if client == nil {
		return nil
	}
	client.prompts = newPromptRegistry(settings.PromptsDir)
	return client
}

func (c *Client) Compile(ctx context.Context, bundle Bundle) (Record, error) {
	if c == nil || c.runtime == nil {
		return Record{}, fmt.Errorf("compile client is nil")
	}
	if c.prompts == nil {
		c.prompts = newPromptRegistry("")
	}
	reqs := InferGraphRequirements(bundle)
	systemPrompt, err := c.prompts.buildInstruction(reqs)
	if err != nil {
		return Record{}, err
	}
	userPrompt, err := c.prompts.buildPrompt(bundle)
	if err != nil {
		return Record{}, err
	}
	output, err := c.compileAttempt(ctx, bundle, systemPrompt, userPrompt, reqs)
	if err != nil {
		retryPrompt, retryErr := c.prompts.buildRetryPrompt(bundle, reqs)
		if retryErr != nil {
			return Record{}, retryErr
		}
		output, err = c.compileAttempt(
			ctx,
			bundle,
			systemPrompt,
			retryPrompt,
			reqs,
		)
		if err != nil {
			return Record{}, err
		}
	}
	verification, err := c.verifier.Verify(ctx, bundle, output)
	if err != nil {
		return Record{}, err
	}
	output = projectVerification(output, verification)
	return Record{
		UnitID:         bundle.UnitID,
		Source:         bundle.Source,
		ExternalID:     bundle.ExternalID,
		RootExternalID: bundle.RootExternalID,
		Model:          c.model,
		Output:         output,
		CompiledAt:     time.Now().UTC(),
	}, nil
}

func (c *Client) compileAttempt(ctx context.Context, bundle Bundle, systemPrompt string, userPrompt string, reqs GraphRequirements) (Output, error) {
	req, err := BuildQwen36ProviderRequest(c.model, bundle, systemPrompt, userPrompt)
	if err != nil {
		return Output{}, err
	}
	resp, err := c.runtime.Call(ctx, req)
	if err != nil {
		return Output{}, err
	}
	out, err := ParseOutput(resp.Text)
	if err != nil {
		return Output{}, err
	}
	if err := out.ValidateWithThresholds(reqs.MinNodes, reqs.MinEdges); err != nil {
		return Output{}, err
	}
	return out, nil
}

func firstConfiguredValue(projectRoot string, keys ...string) string {
	for _, key := range keys {
		if value, ok := config.Get(projectRoot, key); ok && strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
