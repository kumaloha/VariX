package compilev2

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"text/template"
)

type promptLoader struct {
	promptsDir string
}

func newPromptLoader(promptsDir string) *promptLoader {
	trimmed := strings.TrimSpace(promptsDir)
	if trimmed == "" {
		trimmed = resolvePromptsDir()
	}
	return &promptLoader{promptsDir: trimmed}
}

func (l *promptLoader) render(name string, data any) (string, error) {
	if l == nil || strings.TrimSpace(l.promptsDir) == "" {
		return "", fmt.Errorf("prompt loader has no prompts directory")
	}
	body, err := l.loadPromptBody(name)
	if err != nil {
		return "", err
	}
	tmpl, err := template.New(name).Option("missingkey=error").Parse(string(body))
	if err != nil {
		return "", fmt.Errorf("parse prompt %s: %w", name, err)
	}
	var rendered bytes.Buffer
	if err := tmpl.Execute(&rendered, data); err != nil {
		return "", fmt.Errorf("render prompt %s: %w", name, err)
	}
	return strings.TrimSpace(rendered.String()), nil
}

func (l *promptLoader) loadPromptBody(name string) ([]byte, error) {
	candidate := filepath.Join(l.promptsDir, filepath.FromSlash(name))
	body, err := os.ReadFile(candidate)
	if err == nil {
		return body, nil
	}
	defaultDir := resolvePromptsDir()
	if strings.TrimSpace(defaultDir) != "" && !samePath(defaultDir, l.promptsDir) {
		fallback := filepath.Join(defaultDir, filepath.FromSlash(name))
		body, fallbackErr := os.ReadFile(fallback)
		if fallbackErr == nil {
			return body, nil
		}
	}
	return nil, fmt.Errorf("load prompt %s: %w", name, err)
}

func samePath(a, b string) bool {
	a = filepath.Clean(strings.TrimSpace(a))
	b = filepath.Clean(strings.TrimSpace(b))
	if runtime.GOOS == "windows" {
		return strings.EqualFold(a, b)
	}
	return a == b
}

func resolvePromptsDir() string {
	candidates := make([]string, 0, 3)
	if root := strings.TrimSpace(os.Getenv("VARIX_ROOT")); root != "" {
		candidates = append(candidates, filepath.Join(root, "prompts", "compile"))
	}
	if _, file, _, ok := runtime.Caller(0); ok {
		candidates = append(candidates, filepath.Clean(filepath.Join(filepath.Dir(file), "..", "..", "prompts", "compile")))
	}
	if wd, err := os.Getwd(); err == nil {
		for dir := filepath.Clean(wd); ; dir = filepath.Dir(dir) {
			candidates = append(candidates, filepath.Join(dir, "prompts", "compile"))
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
	return filepath.Join("prompts", "compile")
}

var defaultPromptLoader = newPromptLoader("")

func renderStage1SystemPrompt() (string, error) {
	return defaultPromptLoader.render("extract_system.tmpl", nil)
}

func renderStage1UserPrompt(article string) (string, error) {
	return defaultPromptLoader.render("extract_user.tmpl", map[string]any{
		"Article": article,
	})
}

func renderRefineSystemPrompt() (string, error) {
	return defaultPromptLoader.render("refine_system.tmpl", nil)
}

func renderRefineUserPrompt(article string, nodes string) (string, error) {
	return defaultPromptLoader.render("refine_user.tmpl", map[string]any{
		"Article": article,
		"Nodes":   nodes,
	})
}

func renderAggregateSystemPrompt() (string, error) {
	return defaultPromptLoader.render("aggregate_system.tmpl", nil)
}

func renderAggregateUserPrompt(article string, nodes string) (string, error) {
	return defaultPromptLoader.render("aggregate_user.tmpl", map[string]any{
		"Article": article,
		"Nodes":   nodes,
	})
}

func renderStage3SystemPrompt() (string, error) {
	return defaultPromptLoader.render("classify_system.tmpl", nil)
}

func renderStage2SupportSystemPrompt() (string, error) {
	return defaultPromptLoader.render("support_system.tmpl", nil)
}

func renderStage2SupportUserPrompt(nodes string, article string) (string, error) {
	return defaultPromptLoader.render("support_user.tmpl", map[string]any{
		"Nodes":   nodes,
		"Article": article,
	})
}

func renderStage2SupplementSystemPrompt() (string, error) {
	return defaultPromptLoader.render("supplement_system.tmpl", nil)
}

func renderStage2SupplementUserPrompt(nodes string, article string) (string, error) {
	return defaultPromptLoader.render("supplement_user.tmpl", map[string]any{
		"Nodes":   nodes,
		"Article": article,
	})
}

func renderStage2EvidenceSystemPrompt() (string, error) {
	return defaultPromptLoader.render("evidence_system.tmpl", nil)
}

func renderStage2EvidenceUserPrompt(nodes string, article string) (string, error) {
	return defaultPromptLoader.render("evidence_user.tmpl", map[string]any{
		"Nodes":   nodes,
		"Article": article,
	})
}

func renderStage2ExplanationSystemPrompt() (string, error) {
	return defaultPromptLoader.render("explanation_system.tmpl", nil)
}

func renderStage2ExplanationUserPrompt(nodes string, article string) (string, error) {
	return defaultPromptLoader.render("explanation_user.tmpl", map[string]any{
		"Nodes":   nodes,
		"Article": article,
	})
}

func renderStage3UserPrompt(nodeText, sourceQuote string, predecessors string, successors string) (string, error) {
	return defaultPromptLoader.render("classify_user.tmpl", map[string]any{
		"NodeText":     nodeText,
		"SourceQuote":  sourceQuote,
		"Predecessors": predecessors,
		"Successors":   successors,
	})
}

func renderStage3MainlineSystemPrompt() (string, error) {
	return defaultPromptLoader.render("mainline_system.tmpl", nil)
}

func renderStage3MainlineUserPrompt(article string, nodes string, branchHeads string, candidateEdges string) (string, error) {
	return defaultPromptLoader.render("mainline_user.tmpl", map[string]any{
		"Article":        article,
		"Nodes":          nodes,
		"BranchHeads":    branchHeads,
		"CandidateEdges": candidateEdges,
	})
}

func renderStage4SystemPrompt() (string, error) {
	return defaultPromptLoader.render("validate_system.tmpl", nil)
}

func renderStage4UserPrompt(paragraph, nodes, edges string) (string, error) {
	return defaultPromptLoader.render("validate_user.tmpl", map[string]any{
		"Paragraph":    paragraph,
		"CurrentNodes": nodes,
		"CurrentEdges": edges,
	})
}

func renderStage5TranslateSystemPrompt() (string, error) {
	return defaultPromptLoader.render("translate_system.tmpl", nil)
}

func renderStage5TranslateUserPrompt(payload string) (string, error) {
	return defaultPromptLoader.render("translate_user.tmpl", map[string]any{
		"Payload": payload,
	})
}

func renderStage5SummarySystemPrompt() (string, error) {
	return defaultPromptLoader.render("summary_system.tmpl", nil)
}

func renderStage5SummaryUserPrompt(payload string) (string, error) {
	return defaultPromptLoader.render("summary_user.tmpl", map[string]any{
		"Payload": payload,
	})
}
