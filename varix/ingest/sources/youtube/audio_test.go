package youtube

import (
	"net/http"
	"testing"
)

func TestNewDefault_UsesInjectedHTTPClientForAudioTranscriber(t *testing.T) {
	client := &http.Client{}
	t.Setenv("ASR_API_KEY", "test-key")

	c := NewDefault(t.TempDir(), client)
	transcriber, ok := c.audio.(*WhisperAudioTranscriber)
	if !ok {
		t.Fatalf("audio = %T, want *WhisperAudioTranscriber", c.audio)
	}
	if transcriber.client != client {
		t.Fatal("audio transcriber should reuse injected http client")
	}
	if transcriber.asr == nil {
		t.Fatal("audio transcriber should configure ASR client")
	}
	if transcriber.asr.RateLimitRetries() != 4 {
		t.Fatalf("rateLimitRetries = %d, want 4", transcriber.asr.RateLimitRetries())
	}
}

func TestNewDefault_PrefersYouTubeSpecificRetryEnv(t *testing.T) {
	client := &http.Client{}
	t.Setenv("ASR_API_KEY", "test-key")
	t.Setenv("YOUTUBE_ASR_RETRY_MAX", "6")
	t.Setenv("YOUTUBE_ASR_RETRY_DELAY_MS", "250")

	c := NewDefault(t.TempDir(), client)
	transcriber, ok := c.audio.(*WhisperAudioTranscriber)
	if !ok {
		t.Fatalf("audio = %T, want *WhisperAudioTranscriber", c.audio)
	}
	if transcriber.asr == nil {
		t.Fatal("audio transcriber should configure ASR client")
	}
	if transcriber.asr.RateLimitRetries() != 6 {
		t.Fatalf("rateLimitRetries = %d, want 6", transcriber.asr.RateLimitRetries())
	}
	if got := transcriber.asr.RetryDelay(2); got.Milliseconds() != 500 {
		t.Fatalf("retryDelay(2) = %v, want 500ms", got)
	}
}
