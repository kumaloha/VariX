package verify

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"text/template"
)

type promptRegistry struct {
	promptsDir string
}

func newPromptRegistry(promptsDir string) *promptRegistry {
	trimmed := strings.TrimSpace(promptsDir)
	if trimmed == "" {
		trimmed = resolvePromptsDir()
	}
	return &promptRegistry{promptsDir: trimmed}
}

func (r *promptRegistry) verifierInstruction(name string) (string, error) {
	return r.render(filepath.ToSlash(filepath.Join("compile", "verifier", name+".tmpl")), nil)
}

func (r *promptRegistry) render(relativePath string, data any) (string, error) {
	if r == nil || strings.TrimSpace(r.promptsDir) == "" {
		return "", fmt.Errorf("prompt registry has no prompts directory")
	}
	body, err := r.loadPromptBody(relativePath)
	if err != nil {
		return "", err
	}
	tmpl, err := template.New(relativePath).Option("missingkey=error").Parse(string(body))
	if err != nil {
		return "", fmt.Errorf("parse prompt %s: %w", relativePath, err)
	}
	var rendered bytes.Buffer
	if err := tmpl.Execute(&rendered, data); err != nil {
		return "", fmt.Errorf("render prompt %s: %w", relativePath, err)
	}
	return strings.TrimSpace(rendered.String()), nil
}

func (r *promptRegistry) loadPromptBody(relativePath string) ([]byte, error) {
	candidate := filepath.Join(r.promptsDir, filepath.FromSlash(relativePath))
	body, err := os.ReadFile(candidate)
	if err == nil {
		return body, nil
	}
	defaultDir := resolvePromptsDir()
	if strings.TrimSpace(defaultDir) != "" && !samePath(defaultDir, r.promptsDir) {
		fallback := filepath.Join(defaultDir, filepath.FromSlash(relativePath))
		body, fallbackErr := os.ReadFile(fallback)
		if fallbackErr == nil {
			return body, nil
		}
	}
	return nil, fmt.Errorf("load prompt %s: %w", relativePath, err)
}

func samePath(a, b string) bool {
	a = filepath.Clean(strings.TrimSpace(a))
	b = filepath.Clean(strings.TrimSpace(b))
	if runtime.GOOS == "windows" {
		return strings.EqualFold(a, b)
	}
	return a == b
}

func marshalJSON(value any) (string, error) {
	encoded, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return "", err
	}
	return string(encoded), nil
}

func resolvePromptsDir() string {
	candidates := make([]string, 0, 3)
	if root := strings.TrimSpace(os.Getenv("VARIX_ROOT")); root != "" {
		candidates = append(candidates, filepath.Join(root, "prompts"))
	}
	if _, file, _, ok := runtime.Caller(0); ok {
		candidates = append(candidates, filepath.Clean(filepath.Join(filepath.Dir(file), "..", "..", "prompts")))
	}
	if wd, err := os.Getwd(); err == nil {
		for dir := filepath.Clean(wd); ; dir = filepath.Dir(dir) {
			candidates = append(candidates, filepath.Join(dir, "prompts"))
			parent := filepath.Dir(dir)
			if parent == dir {
				break
			}
		}
	}
	for _, candidate := range candidates {
		info, err := os.Stat(candidate)
		if err == nil && info.IsDir() {
			return candidate
		}
	}
	return filepath.Join("prompts")
}
