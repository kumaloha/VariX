package audioutil

import (
	"net/http"
	"testing"
	"time"
)

func TestNewClientFromConfigAppliesRetrySettings(t *testing.T) {
	root := t.TempDir()
	t.Setenv("ASR_API_KEY", "test-key")
	t.Setenv("ASR_RETRY_MAX", "4")
	t.Setenv("ASR_RETRY_DELAY_MS", "250")
	client := NewClientFromConfig(root, &http.Client{})
	if client == nil {
		t.Fatal("NewClientFromConfig() = nil")
	}
	if client.rateLimitRetries != 4 {
		t.Fatalf("rateLimitRetries = %d, want 4", client.rateLimitRetries)
	}
	if got := client.retryDelay(3); got.Milliseconds() != 750 {
		t.Fatalf("retryDelay(3) = %v, want 750ms", got)
	}
}

func TestNewClientFromConfigSupportsLegacyRetryEnv(t *testing.T) {
	root := t.TempDir()
	t.Setenv("ASR_API_KEY", "test-key")
	t.Setenv("ASR_RATE_LIMIT_RETRIES", "1")
	client := NewClientFromConfig(root, &http.Client{})
	if client == nil {
		t.Fatal("NewClientFromConfig() = nil")
	}
	if client.rateLimitRetries != 1 {
		t.Fatalf("rateLimitRetries = %d, want 1", client.rateLimitRetries)
	}
}

func TestNewClientFromConfigWithRetryPrefersPlatformOverrides(t *testing.T) {
	root := t.TempDir()
	t.Setenv("ASR_API_KEY", "test-key")
	t.Setenv("ASR_RETRY_MAX", "2")
	t.Setenv("YOUTUBE_ASR_RETRY_MAX", "5")
	t.Setenv("YOUTUBE_ASR_RETRY_DELAY_MS", "400")

	client := NewClientFromConfigWithRetry(root, &http.Client{}, RetryConfig{
		DefaultMax:     3,
		DefaultDelay:   750 * time.Millisecond,
		RetryMaxKeys:   []string{"YOUTUBE_ASR_RETRY_MAX", "ASR_RETRY_MAX", "ASR_RATE_LIMIT_RETRIES"},
		RetryDelayKeys: []string{"YOUTUBE_ASR_RETRY_DELAY_MS", "ASR_RETRY_DELAY_MS"},
	})
	if client == nil {
		t.Fatal("NewClientFromConfigWithRetry() = nil")
	}
	if client.rateLimitRetries != 5 {
		t.Fatalf("rateLimitRetries = %d, want 5", client.rateLimitRetries)
	}
	if got := client.retryDelay(2); got != 800*time.Millisecond {
		t.Fatalf("retryDelay(2) = %v, want 800ms", got)
	}
}

func TestNewClientFromConfigWithRetryAppliesPlatformDefaults(t *testing.T) {
	root := t.TempDir()
	t.Setenv("ASR_API_KEY", "test-key")

	client := NewClientFromConfigWithRetry(root, &http.Client{}, RetryConfig{
		DefaultMax:     4,
		DefaultDelay:   900 * time.Millisecond,
		RetryMaxKeys:   []string{"BILIBILI_ASR_RETRY_MAX", "ASR_RETRY_MAX", "ASR_RATE_LIMIT_RETRIES"},
		RetryDelayKeys: []string{"BILIBILI_ASR_RETRY_DELAY_MS", "ASR_RETRY_DELAY_MS"},
	})
	if client == nil {
		t.Fatal("NewClientFromConfigWithRetry() = nil")
	}
	if client.rateLimitRetries != 4 {
		t.Fatalf("rateLimitRetries = %d, want 4", client.rateLimitRetries)
	}
	if got := client.retryDelay(2); got != 1800*time.Millisecond {
		t.Fatalf("retryDelay(2) = %v, want 1800ms", got)
	}
}
