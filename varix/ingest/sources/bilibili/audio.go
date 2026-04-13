package bilibili

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/kumaloha/VariX/varix/config"
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

func (t *WhisperAudioTranscriber) Transcribe(ctx context.Context, videoID string) (string, string, error) {
	apiKey, ok := config.Get(t.projectRoot, "ASR_API_KEY")
	if !ok {
		apiKey, ok = config.Get(t.projectRoot, "DASHSCOPE_API_KEY")
		if !ok {
			return "", "", errASRKeyMissing
		}
	}

	baseURL := "https://api.openai.com/v1"
	if value, ok := config.Get(t.projectRoot, "ASR_BASE_URL"); ok && strings.TrimSpace(value) != "" {
		baseURL = strings.TrimRight(value, "/")
	}
	model := "whisper-1"
	if value, ok := config.Get(t.projectRoot, "ASR_MODEL"); ok && strings.TrimSpace(value) != "" {
		model = strings.TrimSpace(value)
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

	text, err := audioutil.NewClient(t.client, baseURL, apiKey, model).TranscribeFile(ctx, audioPath)
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
