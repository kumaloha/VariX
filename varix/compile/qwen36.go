package compile

import (
	"encoding/base64"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

const Qwen36PlusModel = "qwen3.6-plus"

type ChatCompletionRequest struct {
	Model    string                  `json:"model"`
	Messages []ChatCompletionMessage `json:"messages"`
}

type ChatCompletionMessage struct {
	Role    string               `json:"role"`
	Content []ChatCompletionPart `json:"content"`
}

type ChatCompletionPart struct {
	Type     string                  `json:"type"`
	Text     string                  `json:"text,omitempty"`
	ImageURL *ChatCompletionImageURL `json:"image_url,omitempty"`
}

type ChatCompletionImageURL struct {
	URL string `json:"url"`
}

func BuildQwen36Request(bundle Bundle, instruction string) (ChatCompletionRequest, error) {
	content := make([]ChatCompletionPart, 0, 1+len(bundle.LocalImagePaths))
	for _, path := range bundle.LocalImagePaths {
		dataURL, err := fileToDataURL(path)
		if err != nil {
			return ChatCompletionRequest{}, err
		}
		content = append(content, ChatCompletionPart{
			Type:     "image_url",
			ImageURL: &ChatCompletionImageURL{URL: dataURL},
		})
	}
	prompt := strings.TrimSpace(bundle.TextContext())
	if strings.TrimSpace(instruction) != "" {
		prompt = strings.TrimSpace(instruction) + "\n\n" + prompt
	}
	content = append(content, ChatCompletionPart{Type: "text", Text: prompt})

	return ChatCompletionRequest{
		Model: Qwen36PlusModel,
		Messages: []ChatCompletionMessage{{
			Role:    "user",
			Content: content,
		}},
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
