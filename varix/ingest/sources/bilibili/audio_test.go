package bilibili

import (
	"net/http"
	"testing"
)

func TestNewWhisperAudioTranscriberUsesBilibiliRetryDefaults(t *testing.T) {
	t.Setenv("ASR_API_KEY", "test-key")

	transcriber := NewWhisperAudioTranscriber(t.TempDir())
	if transcriber.client != http.DefaultClient {
		t.Fatal("client = nil, want default http client")
	}
	asrClient := transcriber.buildASRClient()
	if asrClient == nil {
		t.Fatal("buildASRClient() = nil")
	}
	if asrClient.RateLimitRetries() != 6 {
		t.Fatalf("rateLimitRetries = %d, want 6", asrClient.RateLimitRetries())
	}
	if got := asrClient.RetryDelay(2).Milliseconds(); got != 2000 {
		t.Fatalf("retryDelay(2) = %dms, want 2000ms", got)
	}
}

func TestNewWhisperAudioTranscriberPrefersBilibiliRetryEnv(t *testing.T) {
	t.Setenv("ASR_API_KEY", "test-key")
	t.Setenv("BILIBILI_ASR_RETRY_MAX", "3")
	t.Setenv("BILIBILI_ASR_RETRY_DELAY_MS", "300")

	transcriber := NewWhisperAudioTranscriber(t.TempDir())
	asrClient := transcriber.buildASRClient()
	if asrClient == nil {
		t.Fatal("buildASRClient() = nil")
	}
	if asrClient.RateLimitRetries() != 3 {
		t.Fatalf("rateLimitRetries = %d, want 3", asrClient.RateLimitRetries())
	}
	if got := asrClient.RetryDelay(2).Milliseconds(); got != 600 {
		t.Fatalf("retryDelay(2) = %dms, want 600ms", got)
	}
}
