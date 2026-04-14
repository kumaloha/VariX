package audioutil

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"
)

// --- Download tests ---

func TestDownload_RejectsOversizedContentLength(t *testing.T) {
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode:    http.StatusOK,
			ContentLength: 100 << 20, // 100 MB
			Body:          io.NopCloser(strings.NewReader("")),
		}, nil
	})}

	_, err := Download(context.Background(), client, "https://example.com/video.mp4", 50<<20)
	if err == nil {
		t.Fatal("Download() error = nil, want rejection for oversized Content-Length")
	}
	if !strings.Contains(err.Error(), "exceeds") {
		t.Fatalf("Download() error = %v, want 'exceeds' in message", err)
	}
}

func TestDownload_RejectsOversizedBody(t *testing.T) {
	var reqCount int
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		reqCount++
		if r.Method == http.MethodHead {
			return &http.Response{
				StatusCode:    http.StatusOK,
				ContentLength: -1, // Unknown
				Body:          io.NopCloser(strings.NewReader("")),
			}, nil
		}
		// GET returns body larger than limit
		body := strings.Repeat("x", 1024+1)
		return &http.Response{
			StatusCode:    http.StatusOK,
			ContentLength: -1,
			Body:          io.NopCloser(strings.NewReader(body)),
		}, nil
	})}

	_, err := Download(context.Background(), client, "https://example.com/video.mp4", 1024)
	if err == nil {
		t.Fatal("Download() error = nil, want rejection for oversized body")
	}
	if !strings.Contains(err.Error(), "exceeds") {
		t.Fatalf("Download() error = %v, want 'exceeds' in message", err)
	}
}

func TestDownload_Success(t *testing.T) {
	payload := "video-bytes-here"
	var reqCount int
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		reqCount++
		if r.Method == http.MethodHead {
			return &http.Response{
				StatusCode:    http.StatusOK,
				ContentLength: int64(len(payload)),
				Body:          io.NopCloser(strings.NewReader("")),
			}, nil
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(payload)),
		}, nil
	})}

	path, err := Download(context.Background(), client, "https://example.com/video.mp4", 50<<20)
	if err != nil {
		t.Fatalf("Download() error = %v", err)
	}
	defer os.Remove(path)

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if string(data) != payload {
		t.Fatalf("Download() content = %q, want %q", string(data), payload)
	}
}

func TestDownload_RejectsNon2xx(t *testing.T) {
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.Method == http.MethodHead {
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader("")),
			}, nil
		}
		return &http.Response{
			StatusCode: http.StatusForbidden,
			Body:       io.NopCloser(strings.NewReader("forbidden")),
		}, nil
	})}

	_, err := Download(context.Background(), client, "https://example.com/video.mp4", 50<<20)
	if err == nil {
		t.Fatal("Download() error = nil, want error for 403")
	}
	if !strings.Contains(err.Error(), "403") {
		t.Fatalf("Download() error = %v, want status 403 in message", err)
	}
}

// --- ProbeDuration tests ---

func TestProbeDuration_ParsesOutput(t *testing.T) {
	original := ExecProbe
	defer func() { ExecProbe = original }()

	ExecProbe = func(_ context.Context, _ string) (time.Duration, error) {
		return 125 * time.Second, nil
	}

	dur, err := ProbeDuration(context.Background(), "/fake/video.mp4")
	if err != nil {
		t.Fatalf("ProbeDuration() error = %v", err)
	}
	if dur != 125*time.Second {
		t.Fatalf("ProbeDuration() = %v, want 2m5s", dur)
	}
}

// --- ExtractAudio tests ---

func TestExtractAudio_Delegates(t *testing.T) {
	original := ExecExtract
	defer func() { ExecExtract = original }()

	var gotInput, gotOutput string
	ExecExtract = func(_ context.Context, input, output string) error {
		gotInput = input
		gotOutput = output
		return nil
	}

	err := ExtractAudio(context.Background(), "/tmp/video.mp4", "/tmp/audio.mp3")
	if err != nil {
		t.Fatalf("ExtractAudio() error = %v", err)
	}
	if gotInput != "/tmp/video.mp4" {
		t.Fatalf("ExtractAudio input = %q, want /tmp/video.mp4", gotInput)
	}
	if gotOutput != "/tmp/audio.mp3" {
		t.Fatalf("ExtractAudio output = %q, want /tmp/audio.mp3", gotOutput)
	}
}

func TestExtractAudio_PropagatesError(t *testing.T) {
	original := ExecExtract
	defer func() { ExecExtract = original }()

	ExecExtract = func(_ context.Context, _, _ string) error {
		return fmt.Errorf("tool missing: ffmpeg: exec: not found")
	}

	err := ExtractAudio(context.Background(), "/tmp/video.mp4", "/tmp/audio.mp3")
	if err == nil {
		t.Fatal("ExtractAudio() error = nil, want error")
	}
	if !strings.Contains(err.Error(), "ffmpeg") {
		t.Fatalf("ExtractAudio() error = %v, want ffmpeg in message", err)
	}
}

// --- TranscribeRemoteVideo tests ---

func TestTranscribeRemoteVideo_FullPipeline(t *testing.T) {
	origProbe := ExecProbe
	origExtract := ExecExtract
	defer func() { ExecProbe = origProbe; ExecExtract = origExtract }()

	ExecProbe = func(_ context.Context, _ string) (time.Duration, error) {
		return 2 * time.Minute, nil
	}
	ExecExtract = func(_ context.Context, input, output string) error {
		// Simulate creating the audio file.
		return os.WriteFile(output, []byte("audio-data"), 0o644)
	}

	httpClient := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.Method == http.MethodHead {
			return &http.Response{
				StatusCode:    http.StatusOK,
				ContentLength: 100,
				Body:          io.NopCloser(strings.NewReader("")),
			}, nil
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader("fake-video")),
		}, nil
	})}

	asrClient := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(strings.NewReader(`{"text":"transcribed text"}`)),
		}, nil
	})}

	asr := &Client{
		httpClient:     asrClient,
		baseURL:        "https://asr.test",
		apiKey:         "test-key",
		model:          "test-model",
		maxUploadBytes: 1 << 20,
		splitter: func(context.Context, string, int) (splitArtifacts, error) {
			t.Fatal("splitter should not be called")
			return splitArtifacts{}, nil
		},
	}

	result, err := TranscribeRemoteVideo(context.Background(), httpClient, "https://example.com/video.mp4", asr, RemoteVideoOptions{})
	if err != nil {
		t.Fatalf("TranscribeRemoteVideo() error = %v", err)
	}
	if result.Transcript != "transcribed text" {
		t.Fatalf("Transcript = %q, want %q", result.Transcript, "transcribed text")
	}
	if result.TranscriptMethod != "whisper" {
		t.Fatalf("TranscriptMethod = %q, want %q", result.TranscriptMethod, "whisper")
	}
	if len(result.TranscriptDiagnostics) != 0 {
		t.Fatalf("TranscriptDiagnostics = %v, want empty", result.TranscriptDiagnostics)
	}
}

func TestTranscribeRemoteVideo_SplitsWhenTooLong(t *testing.T) {
	origProbe := ExecProbe
	origExtract := ExecExtract
	defer func() { ExecProbe = origProbe; ExecExtract = origExtract }()

	ExecProbe = func(_ context.Context, _ string) (time.Duration, error) {
		return 15 * time.Minute, nil
	}
	ExecExtract = func(_ context.Context, _, output string) error {
		return os.WriteFile(output, []byte(strings.Repeat("a", 2048)), 0o644)
	}

	httpClient := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.Method == http.MethodHead {
			return &http.Response{
				StatusCode:    http.StatusOK,
				ContentLength: 100,
				Body:          io.NopCloser(strings.NewReader("")),
			}, nil
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader("video")),
		}, nil
	})}

	asrClient := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(strings.NewReader(`{"text":"long transcript"}`)),
		}, nil
	})}

	asr := &Client{
		httpClient:     asrClient,
		baseURL:        "https://asr.test",
		apiKey:         "k",
		model:          "m",
		maxUploadBytes: 1 << 20,
		splitter: func(_ context.Context, inputPath string, _ int) (splitArtifacts, error) {
			part := inputPath + ".part.mp3"
			if err := os.WriteFile(part, []byte("segment"), 0o644); err != nil {
				return splitArtifacts{}, err
			}
			return splitArtifacts{Parts: []string{part}}, nil
		},
	}

	result, err := TranscribeRemoteVideo(context.Background(), httpClient, "https://example.com/video.mp4", asr, RemoteVideoOptions{})
	if err != nil {
		t.Fatalf("TranscribeRemoteVideo() error = %v", err)
	}
	if result.Transcript != "long transcript" {
		t.Fatalf("Transcript = %q, want long transcript", result.Transcript)
	}
}

func TestTranscribeRemoteVideo_DefaultOptions(t *testing.T) {
	origProbe := ExecProbe
	origExtract := ExecExtract
	defer func() { ExecProbe = origProbe; ExecExtract = origExtract }()

	// Return duration just under the default limit to verify it's applied.
	ExecProbe = func(_ context.Context, _ string) (time.Duration, error) {
		return 9*time.Minute + 59*time.Second, nil
	}
	ExecExtract = func(_ context.Context, _, output string) error {
		return os.WriteFile(output, []byte("audio"), 0o644)
	}

	httpClient := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.Method == http.MethodHead {
			return &http.Response{
				StatusCode:    http.StatusOK,
				ContentLength: 100,
				Body:          io.NopCloser(strings.NewReader("")),
			}, nil
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader("video")),
		}, nil
	})}

	asrClient := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(strings.NewReader(`{"text":"ok"}`)),
		}, nil
	})}

	asr := &Client{
		httpClient:     asrClient,
		baseURL:        "https://asr.test",
		apiKey:         "k",
		model:          "m",
		maxUploadBytes: 1 << 20,
		splitter: func(context.Context, string, int) (splitArtifacts, error) {
			return splitArtifacts{}, nil
		},
	}

	// Zero-value opts should apply defaults (50MB, 10min).
	result, err := TranscribeRemoteVideo(context.Background(), httpClient, "https://example.com/video.mp4", asr, RemoteVideoOptions{})
	if err != nil {
		t.Fatalf("TranscribeRemoteVideo() error = %v", err)
	}
	if result.Transcript != "ok" {
		t.Fatalf("Transcript = %q, want %q", result.Transcript, "ok")
	}
}

func TestTranscribeRemoteVideo_DownloadFailure(t *testing.T) {
	httpClient := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode:    http.StatusOK,
			ContentLength: 999 << 20, // Way too big
			Body:          io.NopCloser(strings.NewReader("")),
		}, nil
	})}

	asr := &Client{httpClient: http.DefaultClient, baseURL: "https://asr.test", apiKey: "k", model: "m", maxUploadBytes: 1 << 20}

	result, err := TranscribeRemoteVideo(context.Background(), httpClient, "https://example.com/video.mp4", asr, RemoteVideoOptions{})
	if err == nil {
		t.Fatal("TranscribeRemoteVideo() error = nil, want download failure")
	}
	if len(result.TranscriptDiagnostics) == 0 {
		t.Fatal("TranscriptDiagnostics empty, want download diagnostic")
	}
	if result.TranscriptDiagnostics[0].Stage != "download" {
		t.Fatalf("diagnostic stage = %q, want download", result.TranscriptDiagnostics[0].Stage)
	}
}

func TestTranscribeRemoteVideo_FallsBackToDirectAudioExtractionWhenVideoTooLarge(t *testing.T) {
	origProbe := ExecProbe
	origExtract := ExecExtract
	defer func() { ExecProbe = origProbe; ExecExtract = origExtract }()

	ExecProbe = func(_ context.Context, _ string) (time.Duration, error) {
		return 3 * time.Minute, nil
	}
	ExecExtract = func(_ context.Context, input, output string) error {
		if input != "https://example.com/video.mp4" {
			t.Fatalf("ExtractAudio input = %q, want remote media URL", input)
		}
		return os.WriteFile(output, []byte("audio-data"), 0o644)
	}

	httpClient := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode:    http.StatusOK,
			ContentLength: 999 << 20, // force remote video download rejection
			Body:          io.NopCloser(strings.NewReader("")),
		}, nil
	})}

	asrClient := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(strings.NewReader(`{"text":"direct audio transcript"}`)),
		}, nil
	})}

	asr := &Client{
		httpClient:     asrClient,
		baseURL:        "https://asr.test",
		apiKey:         "k",
		model:          "m",
		maxUploadBytes: 1 << 20,
		splitter: func(context.Context, string, int) (splitArtifacts, error) {
			t.Fatal("splitter should not be called")
			return splitArtifacts{}, nil
		},
	}

	result, err := TranscribeRemoteVideo(context.Background(), httpClient, "https://example.com/video.mp4", asr, RemoteVideoOptions{})
	if err != nil {
		t.Fatalf("TranscribeRemoteVideo() error = %v", err)
	}
	if result.Transcript != "direct audio transcript" {
		t.Fatalf("Transcript = %q, want direct audio transcript", result.Transcript)
	}
	if result.TranscriptMethod != "whisper" {
		t.Fatalf("TranscriptMethod = %q, want whisper", result.TranscriptMethod)
	}
}

func TestTranscribeRemoteVideo_FallsBackToDirectAudioExtractionAndSplitsWhenTooLong(t *testing.T) {
	origProbe := ExecProbe
	origExtract := ExecExtract
	defer func() { ExecProbe = origProbe; ExecExtract = origExtract }()

	ExecProbe = func(_ context.Context, _ string) (time.Duration, error) {
		return 737 * time.Second, nil
	}
	ExecExtract = func(_ context.Context, input, output string) error {
		if input != "https://example.com/video.mp4" {
			t.Fatalf("ExtractAudio input = %q, want remote media URL", input)
		}
		return os.WriteFile(output, []byte(strings.Repeat("a", 2048)), 0o644)
	}

	httpClient := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode:    http.StatusOK,
			ContentLength: 999 << 20,
			Body:          io.NopCloser(strings.NewReader("")),
		}, nil
	})}

	asrClient := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(strings.NewReader(`{"text":"segmented direct audio transcript"}`)),
		}, nil
	})}

	asr := &Client{
		httpClient:     asrClient,
		baseURL:        "https://asr.test",
		apiKey:         "k",
		model:          "m",
		maxUploadBytes: 1 << 20,
		splitter: func(_ context.Context, inputPath string, _ int) (splitArtifacts, error) {
			part := inputPath + ".part.mp3"
			if err := os.WriteFile(part, []byte("segment"), 0o644); err != nil {
				return splitArtifacts{}, err
			}
			return splitArtifacts{Parts: []string{part}}, nil
		},
	}

	result, err := TranscribeRemoteVideo(context.Background(), httpClient, "https://example.com/video.mp4", asr, RemoteVideoOptions{})
	if err != nil {
		t.Fatalf("TranscribeRemoteVideo() error = %v", err)
	}
	if result.Transcript != "segmented direct audio transcript" {
		t.Fatalf("Transcript = %q, want segmented direct audio transcript", result.Transcript)
	}
}

func TestTranscribeRemoteVideo_ExtractFailure(t *testing.T) {
	origProbe := ExecProbe
	origExtract := ExecExtract
	defer func() { ExecProbe = origProbe; ExecExtract = origExtract }()

	ExecProbe = func(_ context.Context, _ string) (time.Duration, error) {
		return time.Minute, nil
	}
	ExecExtract = func(_ context.Context, _, _ string) error {
		return fmt.Errorf("audio extraction failed: ffmpeg crashed")
	}

	httpClient := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.Method == http.MethodHead {
			return &http.Response{StatusCode: http.StatusOK, ContentLength: 100, Body: io.NopCloser(strings.NewReader(""))}, nil
		}
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader("vid"))}, nil
	})}

	asr := &Client{httpClient: http.DefaultClient, baseURL: "https://asr.test", apiKey: "k", model: "m", maxUploadBytes: 1 << 20}

	result, err := TranscribeRemoteVideo(context.Background(), httpClient, "https://example.com/video.mp4", asr, RemoteVideoOptions{})
	if err == nil {
		t.Fatal("TranscribeRemoteVideo() error = nil, want extract failure")
	}
	if len(result.TranscriptDiagnostics) == 0 {
		t.Fatal("TranscriptDiagnostics empty, want extract diagnostic")
	}
	diag := result.TranscriptDiagnostics[0]
	if diag.Stage != "extract" || diag.Code != "ffmpeg_failed" {
		t.Fatalf("diagnostic = %+v, want stage=extract code=ffmpeg_failed", diag)
	}
}
