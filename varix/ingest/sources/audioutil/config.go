package audioutil

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/kumaloha/VariX/varix/config"
)

type RetryConfig struct {
	DefaultMax     int
	DefaultDelay   time.Duration
	RetryMaxKeys   []string
	RetryDelayKeys []string
}

// NewClientFromConfig builds an ASR client from environment/.env config.
// It falls back from ASR_API_KEY to DASHSCOPE_API_KEY and returns nil when
// neither key is configured.
func NewClientFromConfig(projectRoot string, httpClient *http.Client) *Client {
	return NewClientFromConfigWithRetry(projectRoot, httpClient, RetryConfig{
		RetryMaxKeys:   []string{"ASR_RETRY_MAX", "ASR_RATE_LIMIT_RETRIES"},
		RetryDelayKeys: []string{"ASR_RETRY_DELAY_MS"},
	})
}

func NewClientFromConfigWithRetry(projectRoot string, httpClient *http.Client, retryCfg RetryConfig) *Client {
	apiKey := firstConfiguredValue(projectRoot, "ASR_API_KEY", "DASHSCOPE_API_KEY")
	if apiKey == "" {
		return nil
	}

	baseURL := "https://api.openai.com/v1"
	if value := firstConfiguredValue(projectRoot, "ASR_BASE_URL"); value != "" {
		baseURL = strings.TrimRight(value, "/")
	}
	model := "whisper-1"
	if value := firstConfiguredValue(projectRoot, "ASR_MODEL"); value != "" {
		model = value
	}
	client := NewClient(httpClient, baseURL, apiKey, model)
	if client == nil {
		return nil
	}
	if retryCfg.DefaultMax > 0 {
		client.rateLimitRetries = retryCfg.DefaultMax
	}
	if retryCfg.DefaultDelay > 0 {
		client.retryDelay = linearRetryDelay(retryCfg.DefaultDelay)
	}
	maxKeys := retryCfg.RetryMaxKeys
	if len(maxKeys) == 0 {
		maxKeys = []string{"ASR_RETRY_MAX", "ASR_RATE_LIMIT_RETRIES"}
	}
	if value := firstConfiguredValue(projectRoot, maxKeys...); value != "" {
		if retries, err := strconv.Atoi(value); err == nil && retries >= 0 {
			client.rateLimitRetries = retries
		}
	}
	delayKeys := retryCfg.RetryDelayKeys
	if len(delayKeys) == 0 {
		delayKeys = []string{"ASR_RETRY_DELAY_MS"}
	}
	if value := firstConfiguredValue(projectRoot, delayKeys...); value != "" {
		if delayMS, err := strconv.Atoi(value); err == nil && delayMS >= 0 {
			client.retryDelay = linearRetryDelay(time.Duration(delayMS) * time.Millisecond)
		}
	}
	return client
}

func linearRetryDelay(delay time.Duration) func(int) time.Duration {
	return func(attempt int) time.Duration {
		if attempt < 1 {
			attempt = 1
		}
		return time.Duration(attempt) * delay
	}
}

func firstConfiguredValue(projectRoot string, keys ...string) string {
	for _, key := range keys {
		if value, ok := config.Get(projectRoot, key); ok {
			if trimmed := strings.TrimSpace(value); trimmed != "" {
				return trimmed
			}
		}
	}
	return ""
}
