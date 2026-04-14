package bilibili

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/kumaloha/VariX/varix/ingest/sources/audioutil"
)

type WhisperAudioTranscriber struct {
	projectRoot string
	client      *http.Client
}

func NewWhisperAudioTranscriber(projectRoot string) *WhisperAudioTranscriber {
	return &WhisperAudioTranscriber{
		projectRoot: projectRoot,
		client:      http.DefaultClient,
	}
}

func (t *WhisperAudioTranscriber) buildASRClient() *audioutil.Client {
	return audioutil.NewClientFromConfigWithRetry(t.projectRoot, t.client, audioutil.RetryConfig{
		DefaultMax:     6,
		DefaultDelay:   time.Second,
		RetryMaxKeys:   []string{"BILIBILI_ASR_RETRY_MAX", "ASR_RETRY_MAX", "ASR_RATE_LIMIT_RETRIES"},
		RetryDelayKeys: []string{"BILIBILI_ASR_RETRY_DELAY_MS", "ASR_RETRY_DELAY_MS"},
	})
}

func (t *WhisperAudioTranscriber) Transcribe(ctx context.Context, videoID string) (string, string, error) {
	asrClient := t.buildASRClient()
	if asrClient == nil {
		return "", "", errASRKeyMissing
	}

	tmpDir, err := os.MkdirTemp("", "invarix-bili-audio-*")
	if err != nil {
		return "", "", err
	}
	defer os.RemoveAll(tmpDir)

	rawPath := filepath.Join(tmpDir, videoID+".%(ext)s")
	audioPath := filepath.Join(tmpDir, videoID+".mp3")

	if err := exec.CommandContext(
		ctx,
		"yt-dlp",
		"-f", "bestaudio",
		"--no-playlist",
		"-o", rawPath,
		"https://www.bilibili.com/video/"+videoID,
	).Run(); err != nil {
		if errors.Is(err, exec.ErrNotFound) {
			return "", "", fmt.Errorf("tool missing: yt-dlp: %w", err)
		}
		return "", "", err
	}

	matches, err := filepath.Glob(filepath.Join(tmpDir, videoID+".*"))
	if err != nil {
		return "", "", err
	}
	if len(matches) == 0 {
		return "", "", fmt.Errorf("bilibili audio download missing")
	}

	if err := exec.CommandContext(
		ctx,
		"ffmpeg",
		"-y",
		"-i", matches[0],
		"-vn",
		"-ac", "1",
		"-ar", "16000",
		"-codec:a", "libmp3lame",
		"-b:a", "32k",
		audioPath,
	).Run(); err != nil {
		if errors.Is(err, exec.ErrNotFound) {
			return "", "", fmt.Errorf("tool missing: ffmpeg: %w", err)
		}
		return "", "", err
	}

	text, err := asrClient.TranscribeFile(ctx, audioPath)
	if err != nil {
		switch {
		case errors.Is(err, audioutil.ErrPayloadTooLarge):
			return "", "", fmt.Errorf("bilibili transcription failed: payload too large")
		default:
			return "", "", fmt.Errorf("bilibili transcription failed: %w", err)
		}
	}
	return text, "whisper", nil
}
