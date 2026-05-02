package model_test

import (
	"encoding/json"
	"os/exec"
	"testing"
)

func TestModelDoesNotImportLLMRuntime(t *testing.T) {
	cmd := exec.Command("go", "list", "-json", ".")
	cmd.Dir = "."
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("go list model package: %v", err)
	}
	var pkg struct {
		Imports []string
	}
	if err := json.Unmarshal(out, &pkg); err != nil {
		t.Fatalf("parse go list output: %v", err)
	}
	for _, path := range pkg.Imports {
		if path == "github.com/kumaloha/forge/llm" || path == "github.com/kumaloha/VariX/varix/llm" {
			t.Fatalf("model must stay schema-focused and not import LLM runtime package %q", path)
		}
	}
}
