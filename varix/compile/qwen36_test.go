package compile

import (
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kumaloha/VariX/varix/ingest/types"
)

func TestBuildQwen36RequestBuildsMultimodalPayload(t *testing.T) {
	dir := t.TempDir()
	imgPath := filepath.Join(dir, "test.png")
	raw := []byte("\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDR\x00\x00\x00\x01\x00\x00\x00\x01\b\x02\x00\x00\x00\x90wS\xde\x00\x00\x00\fIDATx\x9cc``\x00\x00\x00\x04\x00\x01\xf6\x178U\x00\x00\x00\x00IEND\xaeB`\x82")
	if err := os.WriteFile(imgPath, raw, 0o644); err != nil {
		t.Fatal(err)
	}

	bundle := BuildBundle(types.RawContent{
		Source:     "twitter",
		ExternalID: "123",
		Content:    "root body",
		Attachments: []types.Attachment{{
			Type:       "image",
			StoredPath: imgPath,
		}},
	})

	req, err := BuildQwen36ProviderRequest(Qwen36PlusModel, bundle, "Summarize this content.", bundle.TextContext())
	if err != nil {
		t.Fatalf("BuildQwen36ProviderRequest() error = %v", err)
	}
	if req.Model != Qwen36PlusModel {
		t.Fatalf("Model = %q", req.Model)
	}
	if req.System != "Summarize this content." {
		t.Fatalf("System = %q", req.System)
	}
	if len(req.UserParts) != 2 {
		t.Fatalf("payload = %#v", req)
	}
	if req.UserParts[0].Type != "image_url" {
		t.Fatalf("first content part = %#v", req.UserParts[0])
	}
	if !strings.HasPrefix(req.UserParts[0].ImageURL, "data:image/png;base64,") {
		t.Fatalf("ImageURL = %q", req.UserParts[0].ImageURL)
	}
	if req.UserParts[1].Type != "text" || !strings.Contains(req.UserParts[1].Text, "root body") {
		t.Fatalf("text part = %#v", req.UserParts[1])
	}
}

func TestFileToDataURL(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "tiny.jpg")
	raw := []byte{0xff, 0xd8, 0xff, 0xd9}
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := fileToDataURL(path)
	if err != nil {
		t.Fatalf("fileToDataURL() error = %v", err)
	}
	wantSuffix := base64.StdEncoding.EncodeToString(raw)
	if !strings.HasSuffix(got, wantSuffix) {
		t.Fatalf("data url = %q", got)
	}
}
