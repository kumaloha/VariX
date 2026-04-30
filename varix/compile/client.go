package compile

import (
	"context"
	"fmt"
	"github.com/kumaloha/VariX/varix/config"
	"github.com/kumaloha/forge/llm"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const defaultDashScopeCompatibleBaseURL = "https://dashscope.aliyuncs.com/compatible-mode/v1"

type runtimeChat interface {
	Call(ctx context.Context, req llm.ProviderRequest) (llm.Response, error)
}

type Client struct {
	runtime        runtimeChat
	model          string
	prompts        *promptRegistry
	verifier       VerificationService
	skipValidation bool
}

type noopVerificationService struct{}

func (noopVerificationService) Verify(ctx context.Context, bundle Bundle, output Output) (Verification, error) {
	return Verification{}, nil
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
	return NewClientWithRuntimePromptsAndVerifierOptions(rt, model, prompts, verifier, false)
}

func NewClientWithRuntimePromptsAndVerifierOptions(rt runtimeChat, model string, prompts *promptRegistry, verifier VerificationService, skipValidation bool) *Client {
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
		runtime:        rt,
		model:          strings.TrimSpace(model),
		prompts:        prompts,
		verifier:       verifier,
		skipValidation: skipValidation,
	}
}

func NewClientFromConfig(projectRoot string, httpClient *http.Client) *Client {
	return newClientFromConfig(projectRoot, httpClient, nil, false)
}

func NewClientFromConfigNoVerify(projectRoot string, httpClient *http.Client) *Client {
	return newClientFromConfig(projectRoot, httpClient, noopVerificationService{}, false)
}

func NewClientFromConfigNoVerifyNoValidate(projectRoot string, httpClient *http.Client) *Client {
	return newClientFromConfig(projectRoot, httpClient, noopVerificationService{}, true)
}

func newClientFromConfig(projectRoot string, httpClient *http.Client, verifier VerificationService, skipValidation bool) *Client {
	settings := config.DefaultSettings(projectRoot)
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
		model = Qwen36PlusModel
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
	client := NewClientWithRuntimePromptsAndVerifierOptions(runtime, strings.TrimSpace(model), newPromptRegistry(settings.PromptsDir), verifier, skipValidation)
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
	output, metrics, err := c.compileDirect(ctx, bundle)
	if err != nil {
		return Record{}, err
	}
	return Record{
		UnitID:         bundle.UnitID,
		Source:         bundle.Source,
		ExternalID:     bundle.ExternalID,
		RootExternalID: bundle.RootExternalID,
		Model:          c.model,
		Metrics:        metrics,
		Output:         output,
		CompiledAt:     NowUTC(),
	}, nil
}

func (c *Client) Verify(ctx context.Context, bundle Bundle, output Output) (Verification, error) {
	if c == nil {
		return Verification{}, fmt.Errorf("verify client is nil")
	}
	if c.verifier == nil {
		return Verification{}, nil
	}
	return c.verifier.Verify(ctx, bundle, output)
}

func (c *Client) VerifyDetailed(ctx context.Context, bundle Bundle, output Output) (Verification, error) {
	if c == nil {
		return Verification{}, fmt.Errorf("verify client is nil")
	}
	if c.runtime == nil {
		return Verification{}, fmt.Errorf("verify client runtime unavailable")
	}
	if c.prompts == nil {
		c.prompts = newPromptRegistry("")
	}
	return runDetailedVerifier(ctx, c.runtime, c.model, c.prompts, bundle, output)
}
