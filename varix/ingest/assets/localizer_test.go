package assets

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kumaloha/VariX/varix/ingest/types"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }

func TestLocalizerStoresImageAndMetadata(t *testing.T) {
	const png1x1 = "\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDR\x00\x00\x00\x01\x00\x00\x00\x01\b\x02\x00\x00\x00\x90wS\xde\x00\x00\x00\fIDATx\x9cc``\x00\x00\x00\x04\x00\x01\xf6\x178U\x00\x00\x00\x00IEND\xaeB`\x82"

	client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"image/png"}},
			Body:       ioNopCloser{strings.NewReader(png1x1)},
		}, nil
	})}

	root := t.TempDir()
	localizer := New(root, client)
	items := []types.RawContent{{
		Attachments: []types.Attachment{{Type: "image", URL: "https://cdn.test/image.png"}},
	}}

	got := localizer.Localize(context.Background(), items)
	att := got[0].Attachments[0]
	if att.DownloadStatus != "stored" {
		t.Fatalf("DownloadStatus = %q", att.DownloadStatus)
	}
	if att.StoredPath == "" {
		t.Fatal("StoredPath empty")
	}
	if _, err := os.Stat(att.StoredPath); err != nil {
		t.Fatalf("stored file missing: %v", err)
	}
	if att.MIMEType != "image/png" {
		t.Fatalf("MIMEType = %q", att.MIMEType)
	}
	if att.ImageWidth != 1 || att.ImageHeight != 1 {
		t.Fatalf("size = %dx%d, want 1x1", att.ImageWidth, att.ImageHeight)
	}
	if !strings.HasPrefix(att.StoredPath, filepath.Join(root, "images", "sha256")) {
		t.Fatalf("StoredPath = %q", att.StoredPath)
	}
}

func TestLocalizerMarksDownloadFailure(t *testing.T) {
	client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusForbidden,
			Header:     http.Header{"Content-Type": []string{"text/plain"}},
			Body:       ioNopCloser{strings.NewReader("nope")},
		}, nil
	})}

	localizer := New(t.TempDir(), client)
	items := []types.RawContent{{Attachments: []types.Attachment{{Type: "image", URL: "https://cdn.test/forbidden.png"}}}}
	got := localizer.Localize(context.Background(), items)
	if got[0].Attachments[0].DownloadStatus != "failed" {
		t.Fatalf("DownloadStatus = %q, want failed", got[0].Attachments[0].DownloadStatus)
	}
}

type ioNopCloser struct{ *strings.Reader }

func (ioNopCloser) Close() error { return nil }
