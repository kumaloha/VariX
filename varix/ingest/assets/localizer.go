package assets

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/kumaloha/VariX/varix/ingest/types"
)

const maxImageBytes int64 = 20 << 20 // 20 MiB

type Localizer struct {
	rootDir string
	client  *http.Client
}

func New(rootDir string, client *http.Client) *Localizer {
	if client == nil {
		client = http.DefaultClient
	}
	return &Localizer{rootDir: rootDir, client: client}
}

func (l *Localizer) Localize(ctx context.Context, items []types.RawContent) []types.RawContent {
	if l == nil || strings.TrimSpace(l.rootDir) == "" {
		return items
	}
	out := make([]types.RawContent, 0, len(items))
	for _, item := range items {
		l.localizeRawContent(ctx, &item)
		out = append(out, item)
	}
	return out
}

func (l *Localizer) localizeRawContent(ctx context.Context, item *types.RawContent) {
	if item == nil {
		return
	}
	l.localizeAttachments(ctx, item.Attachments)
	for i := range item.References {
		l.localizeAttachments(ctx, item.References[i].Attachments)
	}
	for i := range item.ThreadSegments {
		l.localizeAttachments(ctx, item.ThreadSegments[i].Attachments)
	}
}

func (l *Localizer) localizeAttachments(ctx context.Context, attachments []types.Attachment) {
	for i := range attachments {
		att := &attachments[i]
		if strings.ToLower(strings.TrimSpace(att.Type)) != "image" || strings.TrimSpace(att.URL) == "" {
			continue
		}
		l.localizeAttachment(ctx, att)
	}
}

func (l *Localizer) localizeAttachment(ctx context.Context, att *types.Attachment) {
	if att == nil {
		return
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, att.URL, nil)
	if err != nil {
		att.DownloadStatus = "failed"
		att.DownloadError = err.Error()
		return
	}
	resp, err := l.client.Do(req)
	if err != nil {
		att.DownloadStatus = "failed"
		att.DownloadError = err.Error()
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		att.DownloadStatus = "failed"
		att.DownloadError = "image download failed"
		return
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxImageBytes+1))
	if err != nil {
		att.DownloadStatus = "failed"
		att.DownloadError = err.Error()
		return
	}
	if int64(len(data)) > maxImageBytes {
		att.DownloadStatus = "failed"
		att.DownloadError = "image exceeds size limit"
		return
	}

	mimeType := http.DetectContentType(data)
	if !strings.HasPrefix(mimeType, "image/") {
		att.DownloadStatus = "failed"
		att.DownloadError = "unsupported image mime type"
		return
	}

	sum := sha256.Sum256(data)
	sha := hex.EncodeToString(sum[:])
	ext := extensionForMime(mimeType)
	relPath := filepath.Join("images", "sha256", sha[:2], sha+ext)
	fullPath := filepath.Join(l.rootDir, relPath)
	if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
		att.DownloadStatus = "failed"
		att.DownloadError = err.Error()
		return
	}
	if _, err := os.Stat(fullPath); err != nil {
		if err := os.WriteFile(fullPath, data, 0o644); err != nil {
			att.DownloadStatus = "failed"
			att.DownloadError = err.Error()
			return
		}
	}

	if cfg, _, err := image.DecodeConfig(bytes.NewReader(data)); err == nil {
		att.ImageWidth = cfg.Width
		att.ImageHeight = cfg.Height
	}
	att.StoredPath = fullPath
	att.SHA256 = sha
	att.MIMEType = mimeType
	att.ByteSize = int64(len(data))
	att.DownloadStatus = "stored"
	att.DownloadError = ""
}

func extensionForMime(mimeType string) string {
	switch strings.ToLower(strings.TrimSpace(mimeType)) {
	case "image/jpeg":
		return ".jpg"
	case "image/png":
		return ".png"
	case "image/gif":
		return ".gif"
	case "image/webp":
		return ".webp"
	default:
		return ".img"
	}
}
