package audioutil

import (
	"context"
	"fmt"
	"github.com/kumaloha/VariX/varix/ingest/types"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func transcribeRemoteAudioDirect(ctx context.Context, mediaURL string, asr *Client, maxDur time.Duration) (RemoteVideoResult, error) {
	audioFile, err := os.CreateTemp("", "invarix-remote-audio-*.mp3")
	if err != nil {
		return RemoteVideoResult{}, err
	}
	audioPath := audioFile.Name()
	audioFile.Close()
	defer os.Remove(audioPath)

	if err := ExtractAudio(ctx, mediaURL, audioPath); err != nil {
		return RemoteVideoResult{
			TranscriptDiagnostics: []types.TranscriptDiagnostic{
				{Stage: "extract", Code: "ffmpeg_failed", Detail: err.Error()},
			},
		}, fmt.Errorf("audio extraction: %w", err)
	}

	return transcribePreparedAudio(ctx, asr, audioPath, maxDur, "audio")
}

func transcribePreparedAudio(ctx context.Context, asr *Client, audioPath string, maxDur time.Duration, probeLabel string) (RemoteVideoResult, error) {
	dur, err := ProbeDuration(ctx, audioPath)
	if err != nil {
		return RemoteVideoResult{
			TranscriptDiagnostics: []types.TranscriptDiagnostic{
				{Stage: "probe", Code: "probe_failed", Detail: err.Error()},
			},
		}, fmt.Errorf("%s probe: %w", probeLabel, err)
	}

	var transcript string
	if dur > maxDur {
		transcript, err = asr.transcribeSplit(ctx, audioPath)
		if err != nil {
			return RemoteVideoResult{
				TranscriptDiagnostics: []types.TranscriptDiagnostic{
					{Stage: "transcribe", Code: "asr_failed", Detail: err.Error()},
				},
			}, fmt.Errorf("%s duration %.0fs exceeded split threshold %.0fs: %w", probeLabel, dur.Seconds(), maxDur.Seconds(), err)
		}
	} else {
		transcript, err = asr.TranscribeFile(ctx, audioPath)
		if err != nil {
			return RemoteVideoResult{
				TranscriptDiagnostics: []types.TranscriptDiagnostic{
					{Stage: "transcribe", Code: "asr_failed", Detail: err.Error()},
				},
			}, fmt.Errorf("transcription: %w", err)
		}
	}

	return RemoteVideoResult{
		Transcript:       transcript,
		TranscriptMethod: "whisper",
	}, nil
}

func isRemoteVideoTooLargeError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "exceeds limit") || strings.Contains(msg, "video body exceeds")
}

func cleanupSplitArtifacts(artifacts splitArtifacts) {
	seenFiles := make(map[string]struct{}, len(artifacts.Parts))
	for _, part := range artifacts.Parts {
		if part == "" {
			continue
		}
		if _, ok := seenFiles[part]; ok {
			continue
		}
		seenFiles[part] = struct{}{}
		_ = os.Remove(part)
	}
	if artifacts.CleanupDir != "" {
		_ = os.RemoveAll(artifacts.CleanupDir)
		return
	}
	seenDirs := make(map[string]struct{}, len(artifacts.Parts))
	for _, part := range artifacts.Parts {
		dir := filepath.Dir(part)
		if dir == "" {
			continue
		}
		if _, ok := seenDirs[dir]; ok {
			continue
		}
		seenDirs[dir] = struct{}{}
		_ = os.Remove(dir)
	}
}

func TranscribeRemoteVideo(ctx context.Context, client *http.Client, mediaURL string, asr *Client, opts RemoteVideoOptions) (RemoteVideoResult, error) {
	maxBytes := opts.MaxDownloadBytes
	if maxBytes <= 0 {
		maxBytes = MaxRemoteVideoBytes
	}
	maxDur := opts.MaxDuration
	if maxDur <= 0 {
		maxDur = MaxRemoteVideoDuration
	}

	videoPath, err := Download(ctx, client, mediaURL, maxBytes)
	if err != nil {
		if isRemoteVideoTooLargeError(err) {
			result, fallbackErr := transcribeRemoteAudioDirect(ctx, mediaURL, asr, maxDur)
			if fallbackErr == nil {
				return result, nil
			}
			if len(result.TranscriptDiagnostics) > 0 {
				result.TranscriptDiagnostics = append([]types.TranscriptDiagnostic{
					{Stage: "download", Code: "download_failed", Detail: err.Error()},
				}, result.TranscriptDiagnostics...)
				return result, fmt.Errorf("video download: %w; remote audio fallback: %v", err, fallbackErr)
			}
		}
		return RemoteVideoResult{
			TranscriptDiagnostics: []types.TranscriptDiagnostic{
				{Stage: "download", Code: "download_failed", Detail: err.Error()},
			},
		}, fmt.Errorf("video download: %w", err)
	}
	defer os.Remove(videoPath)

	if _, err := ProbeDuration(ctx, videoPath); err != nil {
		return RemoteVideoResult{
			TranscriptDiagnostics: []types.TranscriptDiagnostic{
				{Stage: "probe", Code: "probe_failed", Detail: err.Error()},
			},
		}, fmt.Errorf("video probe: %w", err)
	}

	audioPath := videoPath + ".mp3"
	defer os.Remove(audioPath)

	if err := ExtractAudio(ctx, videoPath, audioPath); err != nil {
		return RemoteVideoResult{
			TranscriptDiagnostics: []types.TranscriptDiagnostic{
				{Stage: "extract", Code: "ffmpeg_failed", Detail: err.Error()},
			},
		}, fmt.Errorf("audio extraction: %w", err)
	}

	return transcribePreparedAudio(ctx, asr, audioPath, maxDur, "video")
}
