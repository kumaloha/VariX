package audioutil

import (
	"context"
	"errors"
	"fmt"
	"github.com/kumaloha/VariX/varix/ingest/sources/httputil"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

func Download(ctx context.Context, client *http.Client, mediaURL string, maxBytes int64) (string, error) {
	// HEAD precheck for Content-Length.
	headReq, err := http.NewRequestWithContext(ctx, http.MethodHead, mediaURL, nil)
	if err != nil {
		return "", err
	}
	headResp, err := client.Do(headReq)
	if err == nil {
		headResp.Body.Close()
		if err := httputil.CheckContentLength(headResp, maxBytes); err != nil {
			return "", fmt.Errorf("video download rejected: %w", err)
		}
	}

	// GET and stream to temp file.
	getReq, err := http.NewRequestWithContext(ctx, http.MethodGet, mediaURL, nil)
	if err != nil {
		return "", err
	}
	getResp, err := client.Do(getReq)
	if err != nil {
		return "", err
	}
	defer getResp.Body.Close()

	if getResp.StatusCode < 200 || getResp.StatusCode >= 300 {
		return "", fmt.Errorf("video download failed: status %d", getResp.StatusCode)
	}

	tmp, err := os.CreateTemp("", "invarix-video-*")
	if err != nil {
		return "", err
	}
	tmpPath := tmp.Name()

	n, err := io.Copy(tmp, io.LimitReader(getResp.Body, maxBytes+1))
	tmp.Close()
	if err != nil {
		os.Remove(tmpPath)
		return "", err
	}
	if n > maxBytes {
		os.Remove(tmpPath)
		return "", fmt.Errorf("video body exceeds %d bytes", maxBytes)
	}
	return tmpPath, nil
}

// ProbeDuration uses ffprobe to determine the duration of a media file.

func ProbeDuration(ctx context.Context, inputPath string) (time.Duration, error) {
	return ExecProbe(ctx, inputPath)
}

func probeDurationExec(ctx context.Context, inputPath string) (time.Duration, error) {
	out, err := exec.CommandContext(ctx,
		"ffprobe",
		"-v", "error",
		"-show_entries", "format=duration",
		"-of", "default=noprint_wrappers=1:nokey=1",
		inputPath,
	).Output()
	if err != nil {
		if errors.Is(err, exec.ErrNotFound) {
			return 0, fmt.Errorf("tool missing: ffprobe: %w", err)
		}
		return 0, fmt.Errorf("ffprobe failed: %w", err)
	}
	secs, err := strconv.ParseFloat(strings.TrimSpace(string(out)), 64)
	if err != nil {
		return 0, fmt.Errorf("ffprobe output not a number: %q", string(out))
	}
	return time.Duration(secs * float64(time.Second)), nil
}

// ExtractAudio extracts mono 16kHz MP3 audio from a video file.

func ExtractAudio(ctx context.Context, inputPath, audioPath string) error {
	return ExecExtract(ctx, inputPath, audioPath)
}

func extractAudioExec(ctx context.Context, inputPath, audioPath string) error {
	if err := exec.CommandContext(ctx,
		"ffmpeg",
		"-y",
		"-i", inputPath,
		"-vn",
		"-ac", "1",
		"-ar", "16000",
		"-codec:a", "libmp3lame",
		"-b:a", "32k",
		audioPath,
	).Run(); err != nil {
		if errors.Is(err, exec.ErrNotFound) {
			return fmt.Errorf("tool missing: ffmpeg: %w", err)
		}
		return fmt.Errorf("audio extraction failed: %w", err)
	}
	return nil
}

// TranscribeRemoteVideo orchestrates download → probe → extract → ASR for a remote video.
