package config

import (
	"path/filepath"
	"testing"
)

func TestNewPromptLoaderUsesProjectPromptsDir(t *testing.T) {
	root := filepath.Clean(filepath.Join("..", ".."))
	loader := NewPromptLoader(root)
	if loader == nil {
		t.Fatal("NewPromptLoader() returned nil")
	}
}
