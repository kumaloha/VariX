package audioutil

import (
	"net/http"
	"strings"

	"github.com/kumaloha/VariX/varix/config"
)

// NewClientFromConfig builds an ASR client from environment/.env config.
// It falls back from ASR_API_KEY to DASHSCOPE_API_KEY and returns nil when
// neither key is configured.
func NewClientFromConfig(projectRoot string, httpClient *http.Client) *Client {
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
	return NewClient(httpClient, baseURL, apiKey, model)
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
