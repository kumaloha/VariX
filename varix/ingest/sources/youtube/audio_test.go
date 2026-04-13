package youtube

import (
	"net/http"
	"testing"
)

func TestNewDefault_UsesInjectedHTTPClientForAudioTranscriber(t *testing.T) {
	client := &http.Client{}

	c := NewDefault(t.TempDir(), client)
	transcriber, ok := c.audio.(*WhisperAudioTranscriber)
	if !ok {
		t.Fatalf("audio = %T, want *WhisperAudioTranscriber", c.audio)
	}
	if transcriber.client != client {
		t.Fatal("audio transcriber should reuse injected http client")
	}
}
