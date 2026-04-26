package compile

import (
	"encoding/base64"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/kumaloha/forge/llm"
)

const (
	Qwen36PlusModel = "qwen3.6-plus"
	Qwen3MaxModel   = "qwen3-max"
)

func BuildQwen36ProviderRequest(model string, bundle Bundle, instruction string, prompt string) (llm.ProviderRequest, error) {
	return BuildProviderRequest(model, bundle, instruction, prompt, false)
}

func BuildProviderRequest(model string, bundle Bundle, instruction string, prompt string, search bool) (llm.ProviderRequest, error) {
	parts := make([]llm.ContentPart, 0, 1+len(bundle.LocalImagePaths))
	for _, path := range bundle.LocalImagePaths {
		dataURL, err := fileToDataURL(path)
		if err != nil {
			return llm.ProviderRequest{}, err
		}
		parts = append(parts, llm.ContentPart{
			Type:     "image_url",
			ImageURL: dataURL,
		})
	}
	parts = append(parts, llm.ContentPart{
		Type: "text",
		Text: strings.TrimSpace(prompt),
	})

	return llm.ProviderRequest{
		Model:       strings.TrimSpace(model),
		System:      strings.TrimSpace(instruction),
		UserParts:   parts,
		Temperature: 0,
		Search:      search,
		Thinking:    false,
	}, nil
}

func fileToDataURL(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	mimeType := http.DetectContentType(data)
	if !strings.HasPrefix(mimeType, "image/") {
		switch strings.ToLower(filepath.Ext(path)) {
		case ".jpg", ".jpeg":
			mimeType = "image/jpeg"
		case ".png":
			mimeType = "image/png"
		case ".gif":
			mimeType = "image/gif"
		case ".webp":
			mimeType = "image/webp"
		default:
			return "", fmt.Errorf("unsupported image mime type for %s", path)
		}
	}
	return "data:" + mimeType + ";base64," + base64.StdEncoding.EncodeToString(data), nil
}
