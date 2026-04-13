package audioutil

import (
	"context"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestTranscribeFile_UploadsSingleSmallFile(t *testing.T) {
	var calls int32
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		atomic.AddInt32(&calls, 1)
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(strings.NewReader(`{"text":"single transcript"}`)),
		}, nil
	})}

	dir := t.TempDir()
	audioPath := filepath.Join(dir, "audio.mp3")
	if err := os.WriteFile(audioPath, []byte("small"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	c := &Client{
		httpClient:     client,
		baseURL:        "https://asr.test",
		apiKey:         "test-key",
		model:          "test-model",
		maxUploadBytes: 1024,
		splitter: func(context.Context, string, int) (splitArtifacts, error) {
			t.Fatal("splitter should not be called for small file")
			return splitArtifacts{}, nil
		},
	}

	got, err := c.TranscribeFile(context.Background(), audioPath)
	if err != nil {
		t.Fatalf("TranscribeFile() error = %v", err)
	}
	if got != "single transcript" {
		t.Fatalf("TranscribeFile() = %q, want %q", got, "single transcript")
	}
	if atomic.LoadInt32(&calls) != 1 {
		t.Fatalf("upload calls = %d, want 1", atomic.LoadInt32(&calls))
	}
}

func TestTranscribeFile_SplitsLargeFileAndConcatenatesResults(t *testing.T) {
	var calls int32
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		call := atomic.AddInt32(&calls, 1)
		switch call {
		case 1:
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Content-Type": []string{"application/json"}},
				Body:       io.NopCloser(strings.NewReader(`{"text":"part one"}`)),
			}, nil
		case 2:
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Content-Type": []string{"application/json"}},
				Body:       io.NopCloser(strings.NewReader(`{"text":"part two"}`)),
			}, nil
		default:
			t.Fatalf("unexpected upload call %d", call)
			return nil, nil
		}
	})}

	dir := t.TempDir()
	audioPath := filepath.Join(dir, "audio.mp3")
	if err := os.WriteFile(audioPath, []byte(strings.Repeat("a", 2048)), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	part1 := filepath.Join(dir, "part-001.mp3")
	part2 := filepath.Join(dir, "part-002.mp3")
	if err := os.WriteFile(part1, []byte("p1"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	if err := os.WriteFile(part2, []byte("p2"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	c := &Client{
		httpClient:     client,
		baseURL:        "https://asr.test",
		apiKey:         "test-key",
		model:          "test-model",
		maxUploadBytes: 10,
		splitter: func(_ context.Context, inputPath string, seconds int) (splitArtifacts, error) {
			if inputPath != audioPath {
				t.Fatalf("split input = %q, want %q", inputPath, audioPath)
			}
			if seconds != defaultSegmentSeconds {
				t.Fatalf("segment seconds = %d, want %d", seconds, defaultSegmentSeconds)
			}
			return splitArtifacts{Parts: []string{part1, part2}}, nil
		},
	}

	got, err := c.TranscribeFile(context.Background(), audioPath)
	if err != nil {
		t.Fatalf("TranscribeFile() error = %v", err)
	}
	if got != "part one\n\npart two" {
		t.Fatalf("TranscribeFile() = %q, want concatenated transcript", got)
	}
	if atomic.LoadInt32(&calls) != 2 {
		t.Fatalf("upload calls = %d, want 2", atomic.LoadInt32(&calls))
	}
}

func TestTranscribeFile_FallsBackToSplitOnPayloadTooLarge(t *testing.T) {
	var calls int32
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		call := atomic.AddInt32(&calls, 1)
		if call == 1 {
			return &http.Response{
				StatusCode: http.StatusRequestEntityTooLarge,
				Body:       io.NopCloser(strings.NewReader("too large")),
			}, nil
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(strings.NewReader(`{"text":"chunk transcript"}`)),
		}, nil
	})}

	dir := t.TempDir()
	audioPath := filepath.Join(dir, "audio.mp3")
	if err := os.WriteFile(audioPath, []byte("small-enough"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	part := filepath.Join(dir, "part-001.mp3")
	if err := os.WriteFile(part, []byte("p1"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	c := &Client{
		httpClient:     client,
		baseURL:        "https://asr.test",
		apiKey:         "test-key",
		model:          "test-model",
		maxUploadBytes: 1024,
		splitter: func(_ context.Context, inputPath string, seconds int) (splitArtifacts, error) {
			if inputPath != audioPath {
				t.Fatalf("split input = %q, want %q", inputPath, audioPath)
			}
			return splitArtifacts{Parts: []string{part}}, nil
		},
	}

	got, err := c.TranscribeFile(context.Background(), audioPath)
	if err != nil {
		t.Fatalf("TranscribeFile() error = %v", err)
	}
	if got != "chunk transcript" {
		t.Fatalf("TranscribeFile() = %q, want %q", got, "chunk transcript")
	}
	if atomic.LoadInt32(&calls) != 2 {
		t.Fatalf("upload calls = %d, want 2", atomic.LoadInt32(&calls))
	}
}

func TestTranscribeFile_PropagatesSplitFailure(t *testing.T) {
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusRequestEntityTooLarge,
			Body:       io.NopCloser(strings.NewReader("too large")),
		}, nil
	})}

	dir := t.TempDir()
	audioPath := filepath.Join(dir, "audio.mp3")
	if err := os.WriteFile(audioPath, []byte("small-enough"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	c := &Client{
		httpClient:     client,
		baseURL:        "https://asr.test",
		apiKey:         "test-key",
		model:          "test-model",
		maxUploadBytes: 1024,
		splitter: func(context.Context, string, int) (splitArtifacts, error) {
			return splitArtifacts{}, errors.New("split failed")
		},
	}

	_, err := c.TranscribeFile(context.Background(), audioPath)
	if err == nil {
		t.Fatal("TranscribeFile() error = nil, want split failure")
	}
	if !strings.Contains(err.Error(), "split failed") {
		t.Fatalf("TranscribeFile() error = %v, want split failed", err)
	}
}

func TestTranscribeFile_CleansReturnedSplitArtifactsAndIgnoresStaleSibling(t *testing.T) {
	var calls int32
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		atomic.AddInt32(&calls, 1)
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(strings.NewReader(`{"text":"segment transcript"}`)),
		}, nil
	})}

	audioDir := t.TempDir()
	audioPath := filepath.Join(audioDir, "audio.mp3")
	if err := os.WriteFile(audioPath, []byte(strings.Repeat("a", 2048)), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	splitDir := t.TempDir()
	part := filepath.Join(splitDir, "part-001.mp3")
	staleSibling := filepath.Join(splitDir, "segment-stale.mp3")
	if err := os.WriteFile(part, []byte("owned"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	if err := os.WriteFile(staleSibling, []byte("stale"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	c := &Client{
		httpClient:     client,
		baseURL:        "https://asr.test",
		apiKey:         "test-key",
		model:          "test-model",
		maxUploadBytes: 10,
		splitter: func(context.Context, string, int) (splitArtifacts, error) {
			return splitArtifacts{Parts: []string{part}}, nil
		},
	}

	got, err := c.TranscribeFile(context.Background(), audioPath)
	if err != nil {
		t.Fatalf("TranscribeFile() error = %v", err)
	}
	if got != "segment transcript" {
		t.Fatalf("TranscribeFile() = %q, want segment transcript", got)
	}
	if atomic.LoadInt32(&calls) != 1 {
		t.Fatalf("upload calls = %d, want 1", atomic.LoadInt32(&calls))
	}
	if _, err := os.Stat(part); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("owned part should be removed, stat err = %v", err)
	}
	if _, err := os.Stat(staleSibling); err != nil {
		t.Fatalf("stale sibling should remain untouched, stat err = %v", err)
	}
}

func TestTranscribeFile_CleansSplitArtifactsOnUploadFailure(t *testing.T) {
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusBadGateway,
			Body:       io.NopCloser(strings.NewReader("bad gateway")),
		}, nil
	})}

	audioDir := t.TempDir()
	audioPath := filepath.Join(audioDir, "audio.mp3")
	if err := os.WriteFile(audioPath, []byte(strings.Repeat("a", 2048)), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	splitDir := t.TempDir()
	part := filepath.Join(splitDir, "part-001.mp3")
	if err := os.WriteFile(part, []byte("owned"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	c := &Client{
		httpClient:     client,
		baseURL:        "https://asr.test",
		apiKey:         "test-key",
		model:          "test-model",
		maxUploadBytes: 10,
		splitter: func(context.Context, string, int) (splitArtifacts, error) {
			return splitArtifacts{Parts: []string{part}}, nil
		},
	}

	_, err := c.TranscribeFile(context.Background(), audioPath)
	if err == nil {
		t.Fatal("TranscribeFile() error = nil, want upload failure")
	}
	if _, statErr := os.Stat(part); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("owned part should be removed after upload failure, stat err = %v", statErr)
	}
}

func TestTranscribeFile_CleansSplitArtifactsOnPartialSplitFailure(t *testing.T) {
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusRequestEntityTooLarge,
			Body:       io.NopCloser(strings.NewReader("too large")),
		}, nil
	})}

	audioDir := t.TempDir()
	audioPath := filepath.Join(audioDir, "audio.mp3")
	if err := os.WriteFile(audioPath, []byte("small-enough"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	splitDir := t.TempDir()
	part := filepath.Join(splitDir, "part-001.mp3")
	if err := os.WriteFile(part, []byte("owned"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	c := &Client{
		httpClient:     client,
		baseURL:        "https://asr.test",
		apiKey:         "test-key",
		model:          "test-model",
		maxUploadBytes: 1024,
		splitter: func(context.Context, string, int) (splitArtifacts, error) {
			return splitArtifacts{Parts: []string{part}}, errors.New("partial split failure")
		},
	}

	_, err := c.TranscribeFile(context.Background(), audioPath)
	if err == nil {
		t.Fatal("TranscribeFile() error = nil, want partial split failure")
	}
	if !strings.Contains(err.Error(), "partial split failure") {
		t.Fatalf("TranscribeFile() error = %v, want partial split failure", err)
	}
	if _, statErr := os.Stat(part); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("owned part should be removed after partial split failure, stat err = %v", statErr)
	}
}
