package youtube

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/kumaloha/VariX/varix/ingest/sources/audioutil"
)

// extractAudioFn is the function used to extract audio from video.
// Overridable for testing.
var extractAudioFn = audioutil.ExtractAudio

type WhisperAudioTranscriber struct {
	client *http.Client
	asr    *audioutil.Client
}

func NewWhisperAudioTranscriber(projectRoot string, httpClient *http.Client) *WhisperAudioTranscriber {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	return &WhisperAudioTranscriber{
		client: httpClient,
		asr:    audioutil.NewClientFromConfig(projectRoot, httpClient),
	}
}

func (t *WhisperAudioTranscriber) Transcribe(ctx context.Context, videoID string) (string, string, error) {
	if t.asr == nil {
		return "", "", errASRKeyMissing
	}

	tmpDir, err := os.MkdirTemp("", "invarix-yt-audio-*")
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
		"https://www.youtube.com/watch?v="+videoID,
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
		return "", "", fmt.Errorf("youtube audio download missing")
	}

	if err := extractAudioFn(ctx, matches[0], audioPath); err != nil {
		return "", "", err
	}

	text, err := t.asr.TranscribeFile(ctx, audioPath)
	if err != nil {
		switch {
		case errors.Is(err, audioutil.ErrPayloadTooLarge):
			return "", "", fmt.Errorf("youtube transcription failed: payload too large")
		default:
			return "", "", fmt.Errorf("youtube transcription failed: %w", err)
		}
	}
	return text, "whisper", nil
}
