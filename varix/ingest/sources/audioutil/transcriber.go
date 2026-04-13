package audioutil

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/kumaloha/VariX/varix/ingest/sources/httputil"
	"github.com/kumaloha/VariX/varix/ingest/types"
)

var ErrPayloadTooLarge = errors.New("payload too large")

const (
	defaultMaxUploadBytes = 20 * 1024 * 1024
	defaultSegmentSeconds = 10 * 60

	MaxRemoteVideoBytes    = 50 << 20         // 50 MB
	MaxRemoteVideoDuration = 10 * time.Minute // 10 minutes
)

type RemoteVideoOptions struct {
	MaxDownloadBytes int64
	MaxDuration      time.Duration
}

type RemoteVideoResult struct {
	Transcript            string
	TranscriptMethod      string
	TranscriptDiagnostics []types.TranscriptDiagnostic
}

type splitArtifacts struct {
	Parts      []string
	CleanupDir string
}

// ExecProbe and ExecExtract are overridable for testing from external packages.
var (
	ExecProbe   = probeDurationExec
	ExecExtract = extractAudioExec
)

type Client struct {
	httpClient     *http.Client
	baseURL        string
	apiKey         string
	model          string
	maxUploadBytes int64
	segmentSeconds int
	splitter       func(ctx context.Context, inputPath string, seconds int) (splitArtifacts, error)
}

func NewClient(httpClient *http.Client, baseURL, apiKey, model string) *Client {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	return &Client{
		httpClient:     httpClient,
		baseURL:        strings.TrimRight(baseURL, "/"),
		apiKey:         apiKey,
		model:          model,
		maxUploadBytes: defaultMaxUploadBytes,
		segmentSeconds: defaultSegmentSeconds,
		splitter:       splitWithFFmpeg,
	}
}

func (c *Client) TranscribeFile(ctx context.Context, audioPath string) (string, error) {
	needsSplit, err := c.requiresSplit(audioPath)
	if err != nil {
		return "", err
	}
	if needsSplit {
		return c.transcribeSplit(ctx, audioPath)
	}

	text, err := c.uploadFile(ctx, audioPath)
	if err == nil {
		return text, nil
	}
	if !errors.Is(err, ErrPayloadTooLarge) {
		return "", err
	}
	return c.transcribeSplit(ctx, audioPath)
}

func (c *Client) requiresSplit(audioPath string) (bool, error) {
	limit := c.maxUploadBytes
	if limit <= 0 {
		limit = defaultMaxUploadBytes
	}
	info, err := os.Stat(audioPath)
	if err != nil {
		return false, err
	}
	return info.Size() > limit, nil
}

func (c *Client) transcribeSplit(ctx context.Context, audioPath string) (string, error) {
	segmentSeconds := c.segmentSeconds
	if segmentSeconds <= 0 {
		segmentSeconds = defaultSegmentSeconds
	}
	splitter := c.splitter
	if splitter == nil {
		splitter = splitWithFFmpeg
	}
	artifacts, err := splitter(ctx, audioPath, segmentSeconds)
	if err != nil {
		cleanupSplitArtifacts(artifacts)
		return "", err
	}
	defer cleanupSplitArtifacts(artifacts)

	parts := artifacts.Parts
	texts := make([]string, 0, len(parts))
	for _, part := range parts {
		text, err := c.uploadFile(ctx, part)
		if err != nil {
			return "", err
		}
		if strings.TrimSpace(text) != "" {
			texts = append(texts, text)
		}
	}
	if len(texts) == 0 {
		return "", fmt.Errorf("empty transcription after splitting")
	}
	return strings.Join(texts, "\n\n"), nil
}

func (c *Client) uploadFile(ctx context.Context, audioPath string) (string, error) {
	file, err := os.Open(audioPath)
	if err != nil {
		return "", err
	}
	defer file.Close()

	var requestBody bytes.Buffer
	writer := multipart.NewWriter(&requestBody)
	part, err := writer.CreateFormFile("file", filepath.Base(audioPath))
	if err != nil {
		return "", err
	}
	if _, err := io.Copy(part, file); err != nil {
		return "", err
	}
	if err := writer.WriteField("model", c.model); err != nil {
		return "", err
	}
	if err := writer.Close(); err != nil {
		return "", err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/audio/transcriptions", &requestBody)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Content-Type", writer.FormDataContentType())

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusRequestEntityTooLarge {
		return "", ErrPayloadTooLarge
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("transcription failed: status %d", resp.StatusCode)
	}

	if err := httputil.CheckContentLength(resp, 1<<20); err != nil {
		return "", err
	}
	var decoded struct {
		Text string `json:"text"`
	}
	if err := httputil.DecodeJSONLimited(resp.Body, 1<<20, &decoded); err != nil {
		return "", err
	}
	if strings.TrimSpace(decoded.Text) == "" {
		return "", fmt.Errorf("empty transcription")
	}
	return decoded.Text, nil
}

func splitWithFFmpeg(ctx context.Context, inputPath string, seconds int) (splitArtifacts, error) {
	splitDir, err := os.MkdirTemp("", "invarix-audio-segments-*")
	if err != nil {
		return splitArtifacts{}, err
	}
	pattern := filepath.Join(splitDir, "segment-%03d.mp3")
	if err := exec.CommandContext(
		ctx,
		"ffmpeg",
		"-y",
		"-i", inputPath,
		"-f", "segment",
		"-segment_time", fmt.Sprintf("%d", seconds),
		"-c", "copy",
		pattern,
	).Run(); err != nil {
		if errors.Is(err, exec.ErrNotFound) {
			return splitArtifacts{CleanupDir: splitDir}, fmt.Errorf("tool missing: ffmpeg: %w", err)
		}
		return splitArtifacts{CleanupDir: splitDir}, err
	}

	parts, err := filepath.Glob(filepath.Join(splitDir, "segment-*.mp3"))
	if err != nil {
		return splitArtifacts{CleanupDir: splitDir}, err
	}
	if len(parts) == 0 {
		return splitArtifacts{CleanupDir: splitDir}, fmt.Errorf("audio split produced no segments")
	}
	sort.Strings(parts)
	return splitArtifacts{Parts: parts, CleanupDir: splitDir}, nil
}

// Download fetches a remote video URL to a temp file with size limits.
// The caller is responsible for removing the returned file.
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
		return RemoteVideoResult{
			TranscriptDiagnostics: []types.TranscriptDiagnostic{
				{Stage: "download", Code: "download_failed", Detail: err.Error()},
			},
		}, fmt.Errorf("video download: %w", err)
	}
	defer os.Remove(videoPath)

	dur, err := ProbeDuration(ctx, videoPath)
	if err != nil {
		return RemoteVideoResult{
			TranscriptDiagnostics: []types.TranscriptDiagnostic{
				{Stage: "probe", Code: "probe_failed", Detail: err.Error()},
			},
		}, fmt.Errorf("video probe: %w", err)
	}
	if dur > maxDur {
		return RemoteVideoResult{
			TranscriptDiagnostics: []types.TranscriptDiagnostic{
				{Stage: "probe", Code: "duration_exceeded", Detail: fmt.Sprintf("%.0fs exceeds limit %.0fs", dur.Seconds(), maxDur.Seconds())},
			},
		}, fmt.Errorf("video duration %.0fs exceeds limit %.0fs", dur.Seconds(), maxDur.Seconds())
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

	transcript, err := asr.TranscribeFile(ctx, audioPath)
	if err != nil {
		return RemoteVideoResult{
			TranscriptDiagnostics: []types.TranscriptDiagnostic{
				{Stage: "transcribe", Code: "asr_failed", Detail: err.Error()},
			},
		}, fmt.Errorf("transcription: %w", err)
	}

	return RemoteVideoResult{
		Transcript:       transcript,
		TranscriptMethod: "whisper",
	}, nil
}
