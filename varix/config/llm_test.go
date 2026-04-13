package config

import (
	"path/filepath"
	"testing"
)

func TestLoadLLMConfig(t *testing.T) {
	root := filepath.Clean(filepath.Join("..", ".."))
	cfg, err := LoadLLMConfig(root)
	if err != nil {
		t.Fatalf("LoadLLMConfig() error = %v", err)
	}
	if cfg.Provider != "dashscope" {
		t.Fatalf("Provider = %q, want dashscope", cfg.Provider)
	}
	if cfg.Default.Model != "qwen3-max" {
		t.Fatalf("Default.Model = %q, want qwen3-max", cfg.Default.Model)
	}
}
