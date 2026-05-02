package audioutil

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"github.com/kumaloha/VariX/varix/ingest/sources/httputil"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type Client struct {
	httpClient       *http.Client
	baseURL          string
	apiKey           string
	model            string
	maxUploadBytes   int64
	segmentSeconds   int
	splitter         func(ctx context.Context, inputPath string, seconds int) (splitArtifacts, error)
	rateLimitRetries int
	retryDelay       func(attempt int) time.Duration
}

func (c *Client) RateLimitRetries() int {
	return c.rateLimitRetries
}

func (c *Client) RetryDelay(attempt int) time.Duration {
	return c.retryBackoff(attempt)
}

func NewClient(httpClient *http.Client, baseURL, apiKey, model string) *Client {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	return &Client{
		httpClient:       httpClient,
		baseURL:          strings.TrimRight(baseURL, "/"),
		apiKey:           apiKey,
		model:            model,
		maxUploadBytes:   defaultMaxUploadBytes,
		segmentSeconds:   defaultSegmentSeconds,
		splitter:         splitWithFFmpeg,
		rateLimitRetries: 2,
		retryDelay: func(attempt int) time.Duration {
			if attempt < 1 {
				attempt = 1
			}
			return time.Duration(attempt) * 500 * time.Millisecond
		},
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
	retries := c.rateLimitRetries
	if retries < 0 {
		retries = 0
	}
	var lastErr error
	for attempt := 0; attempt <= retries; attempt++ {
		text, err := c.uploadFileOnce(ctx, audioPath)
		if err == nil {
			return text, nil
		}
		lastErr = err
		if !errors.Is(err, ErrRateLimited) || attempt == retries {
			return "", err
		}
		if waitErr := sleepWithContext(ctx, c.retryBackoff(attempt+1)); waitErr != nil {
			return "", waitErr
		}
	}
	return "", lastErr
}

func (c *Client) retryBackoff(attempt int) time.Duration {
	if c.retryDelay == nil {
		return time.Duration(attempt) * 500 * time.Millisecond
	}
	return c.retryDelay(attempt)
}

func sleepWithContext(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return nil
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

func (c *Client) uploadFileOnce(ctx context.Context, audioPath string) (string, error) {
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
	if resp.StatusCode == http.StatusTooManyRequests {
		return "", fmt.Errorf("%w: transcription failed: status %d", ErrRateLimited, resp.StatusCode)
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
