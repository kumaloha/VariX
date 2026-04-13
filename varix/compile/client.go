package compile

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/kumaloha/VariX/varix/config"
)

const defaultDashScopeCompatibleBaseURL = "https://dashscope.aliyuncs.com/compatible-mode/v1"

type Client struct {
	httpClient *http.Client
	baseURL    string
	apiKey     string
	model      string
}

func NewClient(httpClient *http.Client, baseURL, apiKey, model string) *Client {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 180 * time.Second}
	}
	return &Client{
		httpClient: httpClient,
		baseURL:    strings.TrimRight(baseURL, "/"),
		apiKey:     strings.TrimSpace(apiKey),
		model:      strings.TrimSpace(model),
	}
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
	return NewClient(httpClient, baseURL, apiKey, model)
}

func (c *Client) Compile(ctx context.Context, bundle Bundle) (Record, error) {
	if c == nil {
		return Record{}, fmt.Errorf("compile client is nil")
	}
	output, err := c.compileAttempt(ctx, bundle, BuildInstruction()+"\n\n"+BuildPrompt(bundle))
	if err != nil {
		output, err = c.compileAttempt(ctx, bundle, BuildInstruction()+"\n\n"+BuildPrompt(bundle)+"\n\n上一次返回未满足要求。请重试，并确保：1）graph 至少 2 个节点和 1 条边；2）details 不为空对象；3）严格使用允许的节点和边类型。")
		if err != nil {
			return Record{}, err
		}
	}
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

func (c *Client) compileAttempt(ctx context.Context, bundle Bundle, prompt string) (Output, error) {
	reqBody, err := BuildQwen36Request(bundle, prompt)
	if err != nil {
		return Output{}, err
	}
	reqBody.Model = c.model
	rawResp, err := c.createChatCompletion(ctx, reqBody)
	if err != nil {
		return Output{}, err
	}
	return ParseOutput(rawResp)
}

func (c *Client) createChatCompletion(ctx context.Context, payload ChatCompletionRequest) (string, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("compile chat completion failed: status %d", resp.StatusCode)
	}
	var decoded struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&decoded); err != nil {
		return "", err
	}
	if len(decoded.Choices) == 0 || strings.TrimSpace(decoded.Choices[0].Message.Content) == "" {
		return "", fmt.Errorf("empty compile response")
	}
	return decoded.Choices[0].Message.Content, nil
}

func firstConfiguredValue(projectRoot string, keys ...string) string {
	for _, key := range keys {
		if value, ok := config.Get(projectRoot, key); ok && strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
